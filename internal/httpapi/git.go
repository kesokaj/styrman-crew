package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	// `path`, not `path/filepath`: ws is a GUEST-side POSIX path, and its
	// meaning must not depend on the server's OS.
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kesokaj/styrman-crew/internal/files"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// The git side panel is a real git client (commit/stage/push/diff), NOT the
// old checkpoint machinery. Every git command runs INSIDE the project's guest
// VM: that is where the user's git identity (~/.gitconfig) and the broker
// credential helper live, so commits carry the right author and push
// authenticates. Commands run as direct argv (no shell) — file paths from the
// client can never be interpreted as shell.

var errVMNotRunning = errors.New("start the project to use git")

// gitFile is one changed path in the working tree / index.
type gitFile struct {
	Path      string `json:"path"`
	Staged    string `json:"staged"`             // index code: M A D R C or ""
	Unstaged  string `json:"unstaged"`           // worktree code, or "?" untracked
	Untracked bool   `json:"untracked"`          // no index entry yet
	OrigPath  string `json:"origPath,omitempty"` // rename/copy source
}

type gitStatusView struct {
	State    string    `json:"state"` // running | stopped | absent
	Repo     bool      `json:"repo"`
	Branch   string    `json:"branch,omitempty"`
	Upstream string    `json:"upstream,omitempty"`
	Ahead    int       `json:"ahead"`
	Behind   int       `json:"behind"`
	Clean    bool      `json:"clean"`
	Files    []gitFile `json:"files"`
}

// gitContext resolves the identity user, running container, guest user and
// guest-side workspace path for the request's project.
func (s *Server) gitContext(r *http.Request) (user, cid, guest, ws string, p project.Project, err error) {
	u, _, pr, err := s.getProjectFor(r)
	if err != nil {
		return "", "", "", "", pr, err
	}
	st, serr := s.guest.Status(r.Context(), u.Username, pr.ID)
	if serr != nil {
		return "", "", "", "", pr, serr
	}
	if st.State != "running" {
		return "", "", "", "", pr, errVMNotRunning
	}
	guest = s.guestUserFor(u.Username)
	// The workspace is bind-mounted into the guest at ~/projects/<dir> (ADR-027).
	ws = "/home/" + guest + "/projects/" + pr.Dir
	return u.Username, st.ContainerID, guest, ws, pr, nil
}

// gitIdentity reads the user's committed git name/email from their shared base
// home (host-side). Per-project homes don't carry the identity — the broker
// syncs the auth token but not `user.name`/`user.email` — so the commit path
// injects it from here via `-c` flags. Empty when not configured yet.
func (s *Server) gitIdentity(user string) (name, email string) {
	// A confined read through internal/files, like every other base-home
	// read — this was the one plain-path read left (security review of
	// ADR-064 Phase 1, low; TODO close-out 2026-09-10 item G). Parsed here
	// rather than by `git config --file`: no exec, and the settings card
	// (toolDetails) uses the same parser so the two cannot disagree.
	b, err := files.ReadRaw(s.cfg.UserHome(user), ".gitconfig", 64<<10)
	if err != nil {
		return "", ""
	}
	return gitUserFromConfig(string(b))
}

// gitUserFromConfig returns user.name and user.email from gitconfig text,
// with `git config --file`'s reading of the parts that matter here: section
// headers and keys are case-insensitive; `[user]` only — a subsection
// (`[user "x"]`) is a different section; `=` may carry any whitespace;
// a value may be double-quoted, with \" and \\ unescaped; `#` and `;`
// start a comment outside quotes; the LAST assignment wins, across a
// reopened [user] section. Not honoured: [include] directives (git itself
// does not follow them for --file without --includes).
func gitUserFromConfig(text string) (name, email string) {
	inUser := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			// `[user] ; comment` is valid: the header ends at the first `]`.
			hdr, _, ok := strings.Cut(line[1:], "]")
			inUser = ok && strings.EqualFold(strings.TrimSpace(hdr), "user")
			continue
		}
		if !inUser {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = gitConfigValue(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "name":
			name = v
		case "email":
			email = v
		}
	}
	// No control characters in an identity: the values reach `git -c` argv
	// in the guest and the broker's TSV env, where a decoded "\n" escape
	// would be a record separator (security review 2026-09-10, low).
	return stripControls(name), stripControls(email)
}

// gitConfigValue unquotes one value: a leading double quote starts a quoted
// string in which \" \\ \n \t \b are git's escapes (any other escaped
// character stands for itself); otherwise the value runs to the first
// unquoted # or ; and is trimmed.
func gitConfigValue(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "\"") {
		if i := strings.IndexAny(v, "#;"); i >= 0 {
			v = v[:i]
		}
		return strings.TrimSpace(v)
	}
	var out strings.Builder
	esc := false
	for _, r := range v[1:] {
		switch {
		case esc:
			switch r {
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			case 'b':
				out.WriteByte('\b')
			default:
				out.WriteRune(r)
			}
			esc = false
		case r == '\\':
			esc = true
		case r == '"':
			return out.String()
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// gitExec runs `git -C <ws> <args>` in the guest and returns (exitCode, output).
func (s *Server) gitExec(ctx context.Context, cid, guest, ws string, args ...string) (int, string, error) {
	return s.guest.Exec(ctx, cid, orchestrator.ExecSpec{
		User: guest,
		Env: []string{
			"HOME=/home/" + guest,
			"USER=" + guest,
			// Stop git's repository discovery from walking ABOVE the
			// workspace. R13 says the workspace is always a repo, but if
			// `<ws>/.git` is ever missing — mid-reinit, or removed from inside
			// the guest — git would otherwise search upward and operate on
			// whatever repo it found next, with the project HOME as the work
			// tree. `add -A` there would stage the docker disk image and the
			// broker's materialized credentials, and the push endpoint would
			// then send them to a guest-controlled origin. With the ceiling
			// set, discovery fails and the command exits non-zero instead,
			// which every caller here already handles (gitStatus reports
			// repo:false; the rest surface git's message).
			// The ceiling itself is not searched, so this permits <ws> and
			// forbids its parent (security review 2026-09-04).
			"GIT_CEILING_DIRECTORIES=" + path.Dir(ws),
			"GIT_DISCOVERY_ACROSS_FILESYSTEM=0",
		},
		Cmd: append([]string{"git", "-C", ws}, args...),
	})
}

// gitStatus reports branch, upstream tracking, and every changed path.
func (s *Server) gitStatus(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		if errors.Is(err, errVMNotRunning) {
			writeJSON(w, http.StatusOK, gitStatusView{State: "stopped"})
			return
		}
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	code, out, err := s.gitExec(ctx, cid, guest, ws,
		"status", "--porcelain=v2", "--branch", "--untracked-files=all")
	view := gitStatusView{State: "running", Repo: true, Files: []gitFile{}}
	if err != nil || code != 0 {
		// Not a git repo (or git failed) — report cleanly, not a 500.
		view.Repo = false
		view.Clean = true
		writeJSON(w, http.StatusOK, view)
		return
	}
	parseGitStatus(out, &view)
	view.Clean = len(view.Files) == 0
	writeJSON(w, http.StatusOK, view)
}

func parseGitStatus(out string, v *gitStatusView) {
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case '#':
			f := strings.Fields(line)
			if len(f) < 3 {
				continue
			}
			switch f[1] {
			case "branch.head":
				v.Branch = f[2]
			case "branch.upstream":
				v.Upstream = f[2]
			case "branch.ab":
				// "+<ahead> -<behind>"
				if len(f) >= 4 {
					v.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[2], "+"))
					v.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[3], "-"))
				}
			}
		case '1': // ordinary: 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			f := strings.SplitN(line, " ", 9)
			if len(f) < 9 {
				continue
			}
			v.Files = append(v.Files, fileFromXY(f[1], f[8]))
		case '2': // renamed/copied: ... <path>\t<origPath>
			f := strings.SplitN(line, " ", 10)
			if len(f) < 10 {
				continue
			}
			path, orig, _ := strings.Cut(f[9], "\t")
			gf := fileFromXY(f[1], path)
			gf.OrigPath = orig
			v.Files = append(v.Files, gf)
		case '?': // untracked: ? <path>
			if len(line) < 3 { // a hostile/garbled "?" line must not panic line[2:]
				continue
			}
			v.Files = append(v.Files, gitFile{
				Path: strings.TrimSpace(line[2:]), Unstaged: "?", Untracked: true,
			})
		case 'u': // unmerged (conflict): u <xy> ... <path>
			f := strings.SplitN(line, " ", 11)
			if len(f) >= 11 {
				v.Files = append(v.Files, gitFile{Path: f[10], Staged: "U", Unstaged: "U"})
			}
		}
	}
}

// fileFromXY turns a porcelain-v2 XY code ("MM", ".M", "A." …) + path into a
// gitFile. "." means unchanged in that column.
func fileFromXY(xy, path string) gitFile {
	f := gitFile{Path: path}
	if len(xy) == 2 {
		if xy[0] != '.' {
			f.Staged = string(xy[0])
		}
		if xy[1] != '.' {
			f.Unstaged = string(xy[1])
		}
	}
	return f
}

// gitStage stages a path (git add) — or everything with {"all":true}.
func (s *Server) gitStage(w http.ResponseWriter, r *http.Request) {
	s.gitMutate(w, r, func(body gitPathBody) []string {
		if body.All {
			return []string{"add", "-A"}
		}
		return []string{"add", "--", body.Path}
	})
}

// gitUnstage removes a path from the index (git reset), or all with {"all":true}.
func (s *Server) gitUnstage(w http.ResponseWriter, r *http.Request) {
	s.gitMutate(w, r, func(body gitPathBody) []string {
		if body.All {
			return []string{"reset", "-q", "HEAD"}
		}
		return []string{"reset", "-q", "HEAD", "--", body.Path}
	})
}

// gitDiscard throws away worktree changes for a path (git checkout / clean for
// untracked). Destructive, so it is its own endpoint.
func (s *Server) gitDiscard(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	var body gitPathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeErr(w, http.StatusBadRequest, errors.New("path required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	// Untracked files aren't in the index — checkout won't remove them; clean does.
	if body.Untracked {
		s.gitRun(w, ctx, cid, guest, ws, "clean", "-fd", "--", body.Path)
		return
	}
	s.gitRun(w, ctx, cid, guest, ws, "checkout", "--", body.Path)
}

type gitPathBody struct {
	Path      string `json:"path"`
	All       bool   `json:"all"`
	Untracked bool   `json:"untracked"`
}

func (s *Server) gitMutate(w http.ResponseWriter, r *http.Request, args func(gitPathBody) []string) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	var body gitPathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !body.All && body.Path == "" {
		writeErr(w, http.StatusBadRequest, errors.New("path or all required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	s.gitRun(w, ctx, cid, guest, ws, args(body)...)
}

// gitCommit commits the staged changes with the given message.
func (s *Server) gitCommit(w http.ResponseWriter, r *http.Request) {
	user, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	var body struct {
		Message string `json:"message"`
		// All stages EVERYTHING before committing (the UI's fast-commit).
		All bool `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("a commit message is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if body.All {
		// `add -A`, NOT `commit -a`: -a skips untracked files, and a button
		// labelled "commit everything" that quietly omits new files is worse
		// than no button. Staging and committing share one handler and one
		// timeout so they cannot be interleaved with another request.
		//
		// If this succeeds and the commit below then fails, the tree is left
		// STAGED. That is observable, so it is documented in specs/styrman/api.md
		// and surfaced by the UI rather than silently unwound — an automatic
		// reset here could discard staging the user did by hand beforehand.
		if code, out, err := s.gitExec(ctx, cid, guest, ws, "add", "-A"); err != nil || code != 0 {
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			// guestErrMessage, not firstLine: this text is guest-controlled,
			// and the helper strips control characters, redacts a token that
			// rode in on a URL, and caps the length (security review).
			writeErr(w, http.StatusConflict,
				errors.New(guestErrMessage("git add failed", out)))
			return
		}
	}
	// Inject the author from the shared home (per-project homes carry no
	// identity). `-c` flags precede the subcommand; direct argv keeps a name
	// with spaces safe. If none is configured yet, git falls back and reports
	// its own "who are you" error, which we surface.
	args := []string{"commit", "-m", body.Message}
	if name, email := s.gitIdentity(user); name != "" && email != "" {
		args = append([]string{
			"-c", "user.name=" + name, "-c", "user.email=" + email,
		}, args...)
	}
	code, out, err := s.gitExec(ctx, cid, guest, ws, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if code != 0 {
		// e.g. "nothing to commit" or a missing identity — surface git's
		// message, stripped/redacted/capped like every other guest-controlled
		// string (the phase-5 fix reached gitRun but missed this site).
		writeErr(w, http.StatusConflict, errors.New(guestErrMessage("git commit", out)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": strings.TrimSpace(out)})
}

// gitPush pushes the current branch, setting upstream on first push.
func (s *Server) gitPush(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	// Nothing committed yet → nothing to push (and rev-parse HEAD would be junk).
	if code, _, _ := s.gitExec(ctx, cid, guest, ws, "rev-parse", "--verify", "-q", "HEAD"); code != 0 {
		writeErr(w, http.StatusConflict, errors.New("no commits yet — make a commit first"))
		return
	}
	// No upstream yet → set it to origin/<branch> on first push.
	args := []string{"push"}
	if code, _, _ := s.gitExec(ctx, cid, guest, ws, "rev-parse", "--abbrev-ref", "@{upstream}"); code != 0 {
		_, br, _ := s.gitExec(ctx, cid, guest, ws, "rev-parse", "--abbrev-ref", "HEAD")
		branch := strings.TrimSpace(br)
		// This name is GUEST-controlled: plumbing can create `refs/heads/-foo`,
		// which rev-parse reports verbatim and `push -u origin -foo` then reads
		// as options (verified on git 2.47.3: -f + -o o). gitCheckout gates its
		// branch name; this sibling did not (security review of the v1.22.0
		// release). --end-of-options does not work for push, so validate.
		if !validBranchName(branch) {
			writeErr(w, http.StatusConflict,
				errors.New("current branch name is not pushable: "+guestErrMessage("branch", branch)))
			return
		}
		args = []string{"push", "-u", "origin", branch}
	}
	code, out, err := s.gitExec(ctx, cid, guest, ws, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if code != 0 {
		// The MOST remote-influenced string in this file: a push failure carries
		// the server's sideband output, and the remote URL comes from the
		// guest-writable .git/config. guestErrMessage strips controls, redacts a
		// credential riding in a URL, and caps the length.
		writeErr(w, http.StatusBadGateway, errors.New(guestErrMessage("git push", out)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": strings.TrimSpace(out)})
}

// gitDiff returns the diff for one path (worktree by default, staged with
// ?staged=true), or the whole working tree when no path is given.
func (s *Server) gitDiff(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		if errors.Is(err, errVMNotRunning) {
			writeJSON(w, http.StatusOK, map[string]any{"state": "stopped", "diff": ""})
			return
		}
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	// A single commit's diff. `git show --format= --patch`, NOT
	// `git diff <hash>~1..<hash>`: `~1` fails outright on a ROOT commit, while
	// show renders it correctly against the empty tree. The empty --format=
	// suppresses the commit header, which the pane renders itself from the
	// subject it was handed. A merge commit shows an empty patch (git's
	// default without -m/--cc); the pane's "No changes" state covers that.
	if c := r.URL.Query().Get("commit"); c != "" {
		if !validCommitHash(c) {
			writeErr(w, http.StatusBadRequest, errors.New("invalid commit hash"))
			return
		}
		// This branch DOES check the exit code, unlike the two below. git show
		// exits non-zero on an unknown revision, and without the check a
		// well-formed-but-nonexistent hash would return 200 with an empty diff
		// — indistinguishable from "this commit changed nothing".
		code, out, err := s.gitExec(ctx, cid, guest, ws,
			"show", "--format=", "--patch", "--no-color", c, "--")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if code != 0 {
			writeErr(w, http.StatusNotFound, errors.New("unknown revision"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"state": "running", "diff": out})
		return
	}
	args := []string{"diff", "--no-color"}
	if r.URL.Query().Get("staged") == "true" {
		args = append(args, "--cached")
	}
	path := r.URL.Query().Get("path")
	untracked := r.URL.Query().Get("untracked") == "true"
	// The two branches below deliberately DISCARD the exit code, and must keep
	// doing so: `git diff --no-index` exits 1 when the files DIFFER, which is
	// the normal success case for an untracked file. Adding a code check here
	// would turn every untracked-file diff into an error.
	var out string
	if untracked && path != "" {
		// `--no-index` takes git OUT of repository mode, so the path is no
		// longer confined to the work tree and `?path=../../../etc/shadow`
		// would read any guest-readable file back through the API. The
		// requester owns a root shell in that same microVM, so no privilege
		// boundary is crossed — it is a confused deputy, and closing it costs
		// one call (audit 2026-09-08). Every other branch here stays inside
		// the repo because git itself confines it.
		rel, rerr := scopeRel(path)
		if rerr != nil || strings.HasPrefix(rel, "../") {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("path must be workspace-relative"))
			return
		}
		// Untracked files have no diff base; show the whole file as an addition.
		_, out, _ = s.gitExec(ctx, cid, guest, ws,
			"diff", "--no-color", "--no-index", "--", "/dev/null", rel)
	} else {
		if path != "" {
			args = append(args, "--", path)
		}
		_, out, _ = s.gitExec(ctx, cid, guest, ws, args...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "running", "diff": out})
}

// ─── v1.22.0 verbs: fetch, pull, branches, checkout, log, stash ────────────
//
// READ endpoints (branches, log) report absence as 200 with a flag, matching
// gitStatus/gitDiff: a missing repo and a stopped VM are STATES of the world,
// not failed requests, and the panel already renders them calmly. MUTATIONS
// (fetch, pull, checkout, stash) keep 4xx, matching gitCommit/gitPush: an
// action that could not happen IS a failure. Do not "unify" these.

// gitFetch updates remote-tracking refs without touching the working tree.
func (s *Server) gitFetch(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	// NO --prune. The refspec fetch obeys is `remote.<name>.fetch` from the
	// workspace's .git/config, which the GUEST can write: with
	// `+refs/heads/*:refs/heads/ns/*`, a pruning fetch deletes local branches
	// (demonstrated, security review 2026-09-04). No privilege boundary is
	// crossed — a guest shell could delete its own refs anyway — but the
	// realistic actor here is a prompt-injected agent in the VM, and the
	// trigger would be the operator's un-confirmed Fetch click. The tidiness
	// --prune buys is not worth being that trigger.
	//
	// --all keeps guest-chosen REMOTE NAMES out of argv entirely; the
	// --refmap alternative is fatal when combined with --all and would put
	// them back. Do not "improve" this into that.
	s.gitRun(w, ctx, cid, guest, ws, "fetch", "--all")
}

// gitPull fast-forwards the current branch. --ff-only on purpose: a merge that
// conflicts would leave a half-merged tree in a VM with no interactive git to
// finish it, and no way for this API to help.
func (s *Server) gitPull(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if code, _, _ := s.gitExec(ctx, cid, guest, ws, "rev-parse", "--abbrev-ref", "@{upstream}"); code != 0 {
		writeErr(w, http.StatusConflict, errors.New("no upstream branch — push first"))
		return
	}
	s.gitRun(w, ctx, cid, guest, ws, "pull", "--ff-only")
}

type gitBranchesView struct {
	State   string   `json:"state"`
	Repo    bool     `json:"repo"`
	Current string   `json:"current,omitempty"`
	Local   []string `json:"local"`
	Remote  []string `json:"remote"`
}

// gitBranches lists local and remote-tracking branches.
func (s *Server) gitBranches(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		if errors.Is(err, errVMNotRunning) {
			writeJSON(w, http.StatusOK, gitBranchesView{State: "stopped", Local: []string{}, Remote: []string{}})
			return
		}
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	view := gitBranchesView{State: "running", Repo: true, Local: []string{}, Remote: []string{}}
	code, out, err := s.gitExec(ctx, cid, guest, ws,
		"for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil || code != 0 {
		// Not a repo (or git failed) — the same soft answer gitStatus gives.
		view.Repo = false
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Local = splitLines(out)
	if _, cur, err := s.gitExec(ctx, cid, guest, ws, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		view.Current = strings.TrimSpace(cur)
	}
	if _, rem, err := s.gitExec(ctx, cid, guest, ws,
		"for-each-ref", "--format=%(refname:short)", "refs/remotes"); err == nil {
		view.Remote = splitLines(rem)
	}
	writeJSON(w, http.StatusOK, view)
}

// gitCheckout switches branches, optionally creating one from HEAD.
func (s *Server) gitCheckout(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	var body struct {
		Branch string `json:"branch"`
		Create bool   `json:"create"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("a branch name is required"))
		return
	}
	// Validate BEFORE any git call: this string is about to become argv.
	if !validBranchName(body.Branch) {
		writeErr(w, http.StatusBadRequest, errors.New("invalid branch name"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// Option handling, verified against git 2.47.3 rather than assumed:
	//
	//   create=true  -> `checkout -b <name> --`. Already immune: -b consumes
	//                   the next argv entry as its VALUE, so a dash-leading
	//                   name is a bad branch name, not an option.
	//   create=false -> `checkout --end-of-options <name> --`. The operand
	//                   would otherwise sit in the option stream, and a
	//                   TRAILING `--` does not save it — git parses left to
	//                   right and has consumed the operand long before it
	//                   reaches the separator. (`git checkout --orphan=x --`
	//                   really does create an orphan branch.) --end-of-options
	//                   is git's actual primitive for this and turns the same
	//                   input into "invalid reference". It cannot be used with
	//                   -b, where it would be eaten as the branch name — which
	//                   is fine, since that form is already safe.
	//
	// validBranchName still rejects a leading "-" first; this is the second
	// gate the previous comment only CLAIMED to be (security review 2026-09-04).
	args := []string{"checkout"}
	if body.Create {
		args = append(args, "-b", body.Branch)
	} else {
		args = append(args, "--end-of-options", body.Branch)
	}
	// Trailing `--`: separates the ref from any pathspec. It does not, and
	// never did, terminate options.
	args = append(args, "--")
	s.gitRun(w, ctx, cid, guest, ws, args...)
}

type gitCommitView struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Subject   string `json:"subject"`
	Author    string `json:"author"`
	Date      string `json:"date"`
}

type gitLogView struct {
	State   string          `json:"state"`
	Repo    bool            `json:"repo"`
	Commits []gitCommitView `json:"commits"`
}

// gitLog lists recent commits, newest first.
func (s *Server) gitLog(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		if errors.Is(err, errVMNotRunning) {
			writeJSON(w, http.StatusOK, gitLogView{State: "stopped", Commits: []gitCommitView{}})
			return
		}
		writeErr(w, http.StatusNotFound, err)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 200 {
			limit = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	view := gitLogView{State: "running", Repo: true, Commits: []gitCommitView{}}
	// NUL-separated so a subject containing anything at all still parses.
	code, out, err := s.gitExec(ctx, cid, guest, ws, "log",
		"--max-count="+strconv.Itoa(limit),
		"--format=%H%x00%h%x00%s%x00%an%x00%aI")
	if err != nil || code != 0 {
		// No repo, or no commits yet. Both are 200: an empty list is the
		// honest representation of "nothing to show", and the panel needs an
		// empty state regardless. (gitPush's 409 for "no commits" is right
		// THERE because pushing nothing is a failed action; listing nothing
		// is not.)
		// Always false here: reaching this branch with code == 0 requires
		// err != nil, and orchestrator.Exec returns -1 on error. Written
		// plainly rather than as an expression that only looks conditional.
		view.Repo = false
		writeJSON(w, http.StatusOK, view)
		return
	}
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\x00")
		if len(f) != 5 {
			continue
		}
		view.Commits = append(view.Commits, gitCommitView{
			Hash: f[0], ShortHash: f[1], Subject: f[2], Author: f[3], Date: f[4],
		})
	}
	writeJSON(w, http.StatusOK, view)
}

type gitStashEntry struct {
	Ref     string `json:"ref"`
	Subject string `json:"subject"`
}

// gitStash runs one of a closed set of stash operations.
func (s *Server) gitStash(w http.ResponseWriter, r *http.Request) {
	_, cid, guest, ws, _, err := s.gitContext(r)
	if err != nil {
		s.gitCtxErr(w, err)
		return
	}
	var body struct {
		Op string `json:"op"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("an op is required"))
		return
	}
	args, ok := stashOps[body.Op]
	if !ok {
		writeErr(w, http.StatusBadRequest, errors.New(`op must be "push", "pop" or "list"`))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if body.Op == "list" {
		entries := []gitStashEntry{}
		code, out, err := s.gitExec(ctx, cid, guest, ws, args...)
		if err == nil && code == 0 {
			for _, line := range splitLines(out) {
				if ref, subject, found := strings.Cut(line, "\x00"); found {
					entries = append(entries, gitStashEntry{Ref: ref, Subject: subject})
				}
			}
		}
		// A repo with no stash exits 0 with no output; a missing repo exits
		// non-zero. Both are an empty list — parsing porcelain is the SERVER's
		// job, and the panel gets a typed array either way.
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
		return
	}
	// push/pop. A conflicting `pop` exits non-zero, writes conflict markers
	// into the tree AND KEEPS the stash entry (git's own behaviour, and the
	// safe half of it). gitRun surfaces git's line as a 409; nothing here
	// attempts to auto-resolve, because unwinding could destroy the very work
	// the user was trying to restore. The UI says the entry was kept.
	s.gitRun(w, ctx, cid, guest, ws, args...)
}

// splitLines returns non-empty trimmed lines.
func splitLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimRight(l, "\r"); strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if lines == nil {
		return []string{}
	}
	return lines
}

// gitRun executes a git command and answers 200 with its output, or an error
// with git's first output line.
func (s *Server) gitRun(w http.ResponseWriter, ctx context.Context, cid, guest, ws string, args ...string) {
	code, out, err := s.gitExec(ctx, cid, guest, ws, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if code != 0 {
		// guestErrMessage, not firstLine: this text is written by the guest
		// and, for fetch/pull, partly by a REMOTE server (sideband `remote:`
		// lines) and by remote names the guest put in .git/config. It lands in
		// a toast, so control and bidi bytes could spoof the UI. The helper
		// strips them, redacts a credential riding in a URL, and caps the
		// length (security review 2026-09-04).
		writeErr(w, http.StatusConflict, errors.New(guestErrMessage("git", out)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": strings.TrimSpace(out)})
}

func (s *Server) gitCtxErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errVMNotRunning) {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeErr(w, http.StatusNotFound, err)
}

// maxRefLen bounds a branch name before it reaches argv. Git itself has no
// hard limit short of the filesystem's, but nothing a human types needs more.
const maxRefLen = 255

// validBranchName is the SECURITY gate on a user-supplied branch name — not a
// reimplementation of git-check-ref-format(1).
//
// The distinction matters. What must be caught here is what argv makes
// dangerous: a leading "-" (git would read the name as an OPTION, e.g.
// `--output=…`), embedded NUL/newline/control bytes, and unbounded length.
// Those are rejected outright. Everything else that merely makes a ref
// INVALID — "a..b", "a@{0}", "a.lock", a trailing "/" — is left to git, which
// rejects it authoritatively and returns its own message; duplicating those
// rules in Go would be hand-rolling a spec we do not own, and drifting from it
// silently. `gitExec` builds a []string and never a shell string, so quoting
// is not part of the exposure.
func validBranchName(s string) bool {
	if s == "" || len(s) > maxRefLen {
		return false
	}
	if s[0] == '-' {
		return false
	}
	// Reject invalid UTF-8 BEFORE ranging. argv carries bytes, but `range`
	// over a string decodes runes, and a lone 0x9B (a C1 control as a raw
	// byte) decodes to utf8.RuneError — which would sail past the C1 check
	// below. Caught by TestValidBranchName; without this the byte reaches git.
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		// C0, DEL and C1: a control byte can hide the rest of a name from a
		// log line or a confirmation dialog.
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return false
		}
		// Space is not a *security* problem in argv, but git forbids it in a
		// ref and rejecting it here gives a better message than git's.
		if r == ' ' {
			return false
		}
	}
	return true
}

// stashOps is a CLOSED set. The op never reaches argv as data — it selects a
// fixed argument slice — so an unknown op is a 400 rather than anything git
// ever sees.
var stashOps = map[string][]string{
	// -u: WITH untracked files. Same reasoning as `add -A` over `commit -a`
	// for fast commit — a "stash my work" button that silently leaves new
	// files in the tree loses exactly the work its user thought they had put
	// away. Ignored files are NOT included (that would need -a); build output
	// is not work in progress.
	"push": {"stash", "push", "-u"},
	"pop":  {"stash", "pop"},
	"list": {"stash", "list", "--format=%gd%x00%gs"},
}

// validCommitHash accepts only an abbreviated-or-full hex object name.
//
// This is the first user-supplied string this file turns into git argv, so it
// is checked before it can reach one. The hex-only rule also rules out a
// leading "-" (which git would read as an option, not a revision) and every
// refspec form — `HEAD@{1}`, `..`, `^`, a branch name — none of which the UI
// ever sends: the diff pane only ever passes a hash it got from git log.
func validCommitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if s == "" {
		return "git command failed"
	}
	return s
}

// gitSummaryItem backs one sidebar git badge.
type gitSummaryItem struct {
	ProjectID string `json:"projectId"`
	Branch    string `json:"branch,omitempty"`
	Dirty     int    `json:"dirty"`
	Ahead     int    `json:"ahead,omitempty"`
	Behind    int    `json:"behind,omitempty"`
}

// gitSummary returns branch + dirty count for every RUNNING project in ONE
// call — the sidebar's git badges (operator item, v1.14.0). Batched
// deliberately: N per-project polls would multiply guest execs (the same
// poll-amplification the shared projects store removes client-side). Stopped
// projects are skipped, not booted.
func (s *Server) gitSummary(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	guest := sp.profile.GuestUser()
	// Bounded fan-out under ONE overall deadline (2026-08-30 review): the
	// serial version cost N×8s worst-case with no cap — a wedged VM stalled
	// every other badge and stacked 30s polls behind it.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	items := []gitSummaryItem{}
	for _, p := range sp.projects.List() {
		wg.Add(1)
		go func(p project.Project) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			st, serr := s.guest.Status(ctx, u.Username, p.ID)
			if serr != nil || st.State != "running" {
				return
			}
			code, out, gerr := s.gitExec(ctx, st.ContainerID, guest, "/home/"+guest+"/projects/"+p.Dir,
				"status", "--porcelain=v2", "--branch", "--untracked-files=all")
			if gerr != nil || code != 0 {
				return // not a repo / VM busy — no badge beats a wrong badge
			}
			v := gitStatusView{Files: []gitFile{}}
			parseGitStatus(out, &v)
			mu.Lock()
			items = append(items, gitSummaryItem{
				ProjectID: p.ID, Branch: v.Branch, Dirty: len(v.Files),
				Ahead: v.Ahead, Behind: v.Behind,
			})
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
