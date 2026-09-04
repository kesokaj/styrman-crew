package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	// `path`, not `path/filepath`: ws is a GUEST-side POSIX path, and its
	// meaning must not depend on the server's OS.
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	st, serr := s.orch.Status(r.Context(), u.Username, pr.ID)
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
	cfg := filepath.Join(s.cfg.UserHome(user), ".gitconfig")
	get := func(key string) string {
		out, err := exec.Command("git", "config", "--file", cfg, key).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return get("user.name"), get("user.email")
}

// gitExec runs `git -C <ws> <args>` in the guest and returns (exitCode, output).
func (s *Server) gitExec(ctx context.Context, cid, guest, ws string, args ...string) (int, string, error) {
	return s.orch.Exec(ctx, cid, orchestrator.ExecSpec{
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
		// e.g. "nothing to commit" or a missing identity — surface git's message.
		writeErr(w, http.StatusConflict, errors.New(firstLine(out)))
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
		args = []string{"push", "-u", "origin", strings.TrimSpace(br)}
	}
	code, out, err := s.gitExec(ctx, cid, guest, ws, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if code != 0 {
		writeErr(w, http.StatusBadGateway, errors.New(firstLine(out)))
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
		// Untracked files have no diff base; show the whole file as an addition.
		_, out, _ = s.gitExec(ctx, cid, guest, ws,
			"diff", "--no-color", "--no-index", "--", "/dev/null", path)
	} else {
		if path != "" {
			args = append(args, "--", path)
		}
		_, out, _ = s.gitExec(ctx, cid, guest, ws, args...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "running", "diff": out})
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
		writeErr(w, http.StatusConflict, errors.New(firstLine(out)))
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
			st, serr := s.orch.Status(ctx, u.Username, p.ID)
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
