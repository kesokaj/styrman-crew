package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// Handler-level tests for the GIT VERBS, the formatter and the syntax
// checker, made possible by the `guestRuntime` seam.
//
// SCOPE, precisely — an earlier version of this comment claimed "every
// endpoint that reaches into a project's microVM", which was false and is
// exactly the kind of overclaim that lets a regression ship green. Four of
// the fifteen routed call sites are covered here: `gitContext`, `gitExec`,
// `check.go`'s exec and `format.go`'s exec. ELEVEN ARE NOT, and they are
// listed so the gap stays visible: `credharvest.go`'s guestOut and
// pushCredsToRunningProjects, `credentials.go`'s importCredentials,
// `explorer.go`'s importGitCredential (two sites) and listNotifications,
// `git.go`'s gitSummary, `server.go`'s createProject closure and getProject,
// and `stats.go` (two sites). The two CREDENTIAL sites are the ones this
// seam was security-reviewed over, and they remain uncovered.
//
// Before it, `Server.orch` was a concrete *orchestrator.Orchestrator, so
// exercising any of these handlers meant a live Docker daemon and a booted
// Kata microVM. That gap was recorded three separate times — TODO.md's
// checkFile/formatFile item, the v1.22.0 release gate's note on the six new
// git handlers, and BUG-003's residual — before it was finally closed.
//
// What these tests are FOR is the argv. The security decisions in git.go live
// entirely in which arguments get built (`add -A` not `commit -a`,
// `--end-of-options` on the switch path, `-u` on stash push, no `--prune` on
// fetch), and until now nothing could assert them. A comment claiming an
// invariant is not a test; this file is.

// fakeGuest stands in for the orchestrator's exec path.
//
// The mutex is PRECAUTIONARY, not exercised: `guestTest`'s config sets no
// Image, so createProject takes its early return and never starts the boot
// goroutine that would otherwise reach the guest concurrently. Kept because a
// test that does set Image would race it immediately — but do not read the
// lock as evidence that the concurrent path is covered (review, 2026-09-04).
type fakeGuest struct {
	mu    sync.Mutex
	state string // what Status reports; "running" unless a test says otherwise
	// statusErr, when set, makes Status fail — the error half of the seam,
	// which was otherwise untestable.
	statusErr error
	calls     [][]string
	// reply decides each exec's result. nil means "exit 0, empty output".
	reply func(cmd []string) (int, string, error)
	// goingAway is what GuestGoingAway reports for a RUNNING project: a
	// lifecycle change in flight (BUG-088). A non-running state is "going
	// away" whatever it says, as in the real orchestrator.
	goingAway bool
	// statusFor records the (user, project) each Status call was asked about,
	// so a handler that looks up the WRONG tenant can be caught. The real
	// Status binds its ContainerID to that pair; a fake that ignored the
	// arguments would quietly bake in "ownership does not matter".
	statusFor [][2]string
}

func (f *fakeGuest) Status(_ context.Context, user, project string) (orchestrator.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusFor = append(f.statusFor, [2]string{user, project})
	if f.statusErr != nil {
		return orchestrator.Status{}, f.statusErr
	}
	st := f.state
	if st == "" {
		st = "running"
	}
	// The container id is DERIVED from the pair, mirroring the real
	// orchestrator's contract: the id it returns belongs to that user and
	// project, and a caller may exec into it on that basis alone.
	return orchestrator.Status{
		User: user, Project: project,
		ContainerID: "container/" + user + "/" + project,
		State:       st,
	}, nil
}

func (f *fakeGuest) Exec(_ context.Context, _ string, spec orchestrator.ExecSpec) (int, string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, spec.Cmd)
	reply := f.reply
	f.mu.Unlock()
	if reply == nil {
		return 0, "", nil
	}
	return reply(spec.Cmd)
}

func (f *fakeGuest) GuestGoingAway(ctx context.Context, user, project string, _ orchestrator.Status) bool {
	if st, err := f.Status(ctx, user, project); err == nil && st.State != "running" {
		return true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.goingAway
}

// matching returns every recorded argv containing `want`, in order.
func (f *fakeGuest) matching(want string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, c := range f.calls {
		for _, w := range c {
			if w == want {
				out = append(out, append([]string(nil), c...))
				break
			}
		}
	}
	return out
}

// lastGit returns the most recent argv whose git subcommand is `sub`, and
// whether one was found. The subcommand is the first word after the
// `git -C <ws>` prefix that gitExec always builds.
func (f *fakeGuest) lastGit(sub string) ([]string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		c := f.calls[i]
		if len(c) >= 4 && c[0] == "git" && c[1] == "-C" && c[3] == sub {
			return append([]string(nil), c...), true
		}
	}
	return nil, false
}

// execCount is stronger than "no git <sub> ran": it proves NOTHING was
// executed in the guest at all.
func (f *fakeGuest) execCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func hasWord(argv []string, w string) bool {
	for _, a := range argv {
		if a == w {
			return true
		}
	}
	return false
}

// guestTest builds a server whose guest calls go to the fake, plus one
// project, and returns the routes and that project's id.
func guestTest(t *testing.T, f *fakeGuest) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Username: "tester", AuthMode: "none"}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg, users, orch, logger)
	s.guest = f
	h := s.Routes()
	return h, makeProject(t, h, "guest-test")
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// ── checkout: the argv shape IS the security decision ──────────────────────

func TestCheckoutSwitchUsesEndOfOptions(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/checkout", `{"branch":"feature/x"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("checkout = %d, want 200: %s", w.Code, w.Body.String())
	}
	argv, ok := f.lastGit("checkout")
	if !ok {
		t.Fatal("no git checkout was executed")
	}
	// A TRAILING `--` protects nothing: git parses left to right and consumes
	// the operand long before it. --end-of-options is the real primitive, and
	// it must come BEFORE the name.
	if !hasWord(argv, "--end-of-options") {
		t.Errorf("switch argv = %v, want --end-of-options", argv)
	}
	for i, a := range argv {
		if a == "--end-of-options" && (i+1 >= len(argv) || argv[i+1] != "feature/x") {
			t.Errorf("--end-of-options must immediately precede the branch: %v", argv)
		}
	}
}

func TestCheckoutCreateUsesDashBAndNotEndOfOptions(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/checkout", `{"branch":"newbr","create":true}`); w.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	argv, ok := f.lastGit("checkout")
	if !ok {
		t.Fatal("no git checkout was executed")
	}
	if !hasWord(argv, "-b") {
		t.Errorf("create argv = %v, want -b", argv)
	}
	// -b takes its argument as a VALUE, so the operand can never be read as an
	// option; --end-of-options here would be eaten as the branch name instead.
	if hasWord(argv, "--end-of-options") {
		t.Errorf("create argv = %v must NOT use --end-of-options", argv)
	}
}

func TestCheckoutRejectsArgvInjectionBeforeAnyGitCall(t *testing.T) {
	for _, bad := range []string{"-b", "--upload-pack=evil", "has space", "nul\x00byte", ""} {
		f := &fakeGuest{}
		h, pid := guestTest(t, f)
		body, _ := json.Marshal(map[string]any{"branch": bad})
		w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/checkout", string(body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("checkout %q = %d, want 400", bad, w.Code)
		}
		// The point is not just the status: nothing may reach git at all.
		if n := f.execCount(); n != 0 {
			t.Errorf("checkout %q made %d guest calls; a rejected name must reach nothing", bad, n)
		}
	}
}

// ── commit: -A vs -a is the difference between including untracked files ───

func TestFastCommitStagesWithAddDashACapitalNotCommitDashA(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/commit", `{"message":"m","all":true}`); w.Code != http.StatusOK {
		t.Fatalf("commit = %d: %s", w.Code, w.Body.String())
	}
	add, ok := f.lastGit("add")
	if !ok {
		t.Fatal("fast commit did not stage — untracked files would be dropped")
	}
	if !hasWord(add, "-A") {
		t.Errorf("stage argv = %v, want add -A", add)
	}
	commit, ok := f.lastGit("commit")
	if !ok {
		t.Fatal("no commit ran")
	}
	// `commit -a` skips untracked files, which is exactly what -A avoids.
	if hasWord(commit, "-a") {
		t.Errorf("commit argv = %v must not use -a", commit)
	}
}

func TestCommitWithoutAllDoesNotStage(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/commit", `{"message":"m"}`); w.Code != http.StatusOK {
		t.Fatalf("commit = %d: %s", w.Code, w.Body.String())
	}
	if _, staged := f.lastGit("add"); staged {
		t.Error("a plain commit must not stage anything")
	}
}

func TestFastCommitLeavesTreeStagedWhenTheCommitFails(t *testing.T) {
	// Documented behaviour (api.md): staging succeeded, the commit did not,
	// and nothing is auto-unwound because a reset could discard staging the
	// user did by hand.
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) >= 4 && cmd[3] == "commit" {
			return 1, "nothing to commit, working tree clean", nil
		}
		return 0, "", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/commit", `{"message":"m","all":true}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("failed commit = %d, want 409", w.Code)
	}
	if _, staged := f.lastGit("add"); !staged {
		t.Error("the add should still have run")
	}
	if !strings.Contains(w.Body.String(), "nothing to commit") {
		t.Errorf("409 should carry git's own line, got %s", w.Body.String())
	}
}

// ── fetch / pull ───────────────────────────────────────────────────────────

func TestFetchIsAllAndNeverPrune(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/fetch", ""); w.Code != http.StatusOK {
		t.Fatalf("fetch = %d: %s", w.Code, w.Body.String())
	}
	argv, ok := f.lastGit("fetch")
	if !ok {
		t.Fatal("no fetch ran")
	}
	// --all keeps guest-chosen remote NAMES out of argv entirely.
	if !hasWord(argv, "--all") {
		t.Errorf("fetch argv = %v, want --all", argv)
	}
	// --prune obeys the guest-writable remote.<name>.fetch refspec and can
	// delete LOCAL branches. Removed as a security decision; keep it removed.
	if hasWord(argv, "--prune") {
		t.Errorf("fetch argv = %v must NOT prune", argv)
	}
}

func TestPullIsFastForwardOnlyAndRefusesWithoutUpstream(t *testing.T) {
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) >= 5 && cmd[3] == "rev-parse" && hasWord(cmd, "@{upstream}") {
			return 1, "", nil // no upstream configured
		}
		return 0, "", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/pull", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("pull without upstream = %d, want 409", w.Code)
	}
	if _, ran := f.lastGit("pull"); ran {
		t.Error("pull must not run when there is no upstream")
	}

	f2 := &fakeGuest{}
	h2, pid2 := guestTest(t, f2)
	if w := do(t, h2, http.MethodPost, "/api/projects/"+pid2+"/git/pull", ""); w.Code != http.StatusOK {
		t.Fatalf("pull = %d: %s", w.Code, w.Body.String())
	}
	argv, _ := f2.lastGit("pull")
	// A conflicted merge in a VM with no interactive git is unfinishable.
	if !hasWord(argv, "--ff-only") {
		t.Errorf("pull argv = %v, want --ff-only", argv)
	}
}

// ── log: the limit clamp ───────────────────────────────────────────────────

func TestLogLimitIsClamped(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"", "--max-count=50"},
		{"?limit=7", "--max-count=7"},
		{"?limit=0", "--max-count=50"},
		{"?limit=201", "--max-count=50"},
		{"?limit=-5", "--max-count=50"},
		{"?limit=abc", "--max-count=50"},
		{"?limit=200", "--max-count=200"},
	} {
		f := &fakeGuest{}
		h, pid := guestTest(t, f)
		if w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/log"+tc.query, ""); w.Code != http.StatusOK {
			t.Fatalf("log%s = %d", tc.query, w.Code)
		}
		argv, ok := f.lastGit("log")
		if !ok {
			t.Fatalf("log%s did not run", tc.query)
		}
		if !hasWord(argv, tc.want) {
			t.Errorf("log%s argv = %v, want %s", tc.query, argv, tc.want)
		}
	}
}

func TestLogParsesNulSeparatedOutput(t *testing.T) {
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) >= 4 && cmd[3] == "log" {
			// A subject containing a tab and a pipe: NUL separation is what
			// makes those safe to pass through.
			return 0, "abc123def\x00abc123d\x00fix: a\tsubject | with junk\x00Ada\x002026-09-04T10:00:00+02:00\n", nil
		}
		return 0, "", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/log", "")
	var got struct {
		Repo    bool `json:"repo"`
		Commits []struct{ Hash, ShortHash, Subject, Author, Date string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad JSON: %v: %s", err, w.Body.String())
	}
	if len(got.Commits) != 1 {
		t.Fatalf("commits = %d, want 1: %s", len(got.Commits), w.Body.String())
	}
	c := got.Commits[0]
	if c.Hash != "abc123def" || c.ShortHash != "abc123d" || c.Author != "Ada" {
		t.Errorf("parsed wrong: %+v", c)
	}
	if c.Subject != "fix: a\tsubject | with junk" {
		t.Errorf("subject mangled: %q", c.Subject)
	}
}

func TestLogWithNoRepoIs200NotAnError(t *testing.T) {
	// A read reports absence as a STATE, matching gitStatus. The panel needs
	// an empty state regardless, and an empty list is the honest answer.
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) >= 4 && cmd[3] == "log" {
			return 128, "fatal: not a git repository", nil
		}
		return 0, "", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/log", "")
	if w.Code != http.StatusOK {
		t.Fatalf("log with no repo = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"repo":false`) {
		t.Errorf("want repo:false, got %s", w.Body.String())
	}
}

// ── stash: the closed op set, and -u ───────────────────────────────────────

func TestStashOpEnumIsClosed(t *testing.T) {
	for _, bad := range []string{"drop", "clear", "apply", "", "--all", "PUSH"} {
		f := &fakeGuest{}
		h, pid := guestTest(t, f)
		body, _ := json.Marshal(map[string]string{"op": bad})
		w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/stash", string(body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("stash op %q = %d, want 400", bad, w.Code)
		}
		if n := f.execCount(); n != 0 {
			t.Errorf("stash op %q made %d guest calls; the enum is closed", bad, n)
		}
	}
}

func TestStashPushCapturesUntrackedFiles(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/stash", `{"op":"push"}`); w.Code != http.StatusOK {
		t.Fatalf("stash push = %d: %s", w.Code, w.Body.String())
	}
	argv, _ := f.lastGit("stash")
	// Without -u a "stash my work" button strands exactly the new files the
	// user thought it had put away.
	if !hasWord(argv, "-u") {
		t.Errorf("stash push argv = %v, want -u", argv)
	}
}

func TestStashListReturnsTypedEntriesNotPorcelain(t *testing.T) {
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) >= 4 && cmd[3] == "stash" {
			return 0, "stash@{0}\x00WIP on main: abc note\nstash@{1}\x00On feature: other\n", nil
		}
		return 0, "", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/git/stash", `{"op":"list"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("stash list = %d", w.Code)
	}
	var got struct {
		Entries []struct{ Ref, Subject string } `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Ref != "stash@{0}" ||
		got.Entries[0].Subject != "WIP on main: abc note" {
		t.Fatalf("parsing is the SERVER's job; got %+v", got.Entries)
	}
}

// ── diff?commit= ───────────────────────────────────────────────────────────

func TestCommitDiffUsesShowAndChecksItsExitCode(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/diff?commit=abc1234", ""); w.Code != http.StatusOK {
		t.Fatalf("commit diff = %d: %s", w.Code, w.Body.String())
	}
	argv, ok := f.lastGit("show")
	if !ok {
		t.Fatal("commit diff did not use git show")
	}
	// `<hash>~1..<hash>` fails outright on a ROOT commit, which every freshly
	// created project has exactly one of.
	if !hasWord(argv, "--format=") || !hasWord(argv, "--patch") {
		t.Errorf("show argv = %v, want --format= --patch", argv)
	}

	// A well-formed but unknown revision must be 404, not a 200 with an empty
	// diff — the latter is indistinguishable from "this commit changed nothing".
	f2 := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		if len(cmd) >= 4 && cmd[3] == "show" {
			return 128, "fatal: bad object", nil
		}
		return 0, "", nil
	}}
	h2, pid2 := guestTest(t, f2)
	if w := do(t, h2, http.MethodGet, "/api/projects/"+pid2+"/git/diff?commit=abc1234", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown revision = %d, want 404", w.Code)
	}
}

func TestCommitDiffRejectsABadHashBeforeAnyGitCall(t *testing.T) {
	for _, bad := range []string{"--output=/tmp/x", "HEAD", "abc1234^", "../etc", "abc123"} {
		f := &fakeGuest{}
		h, pid := guestTest(t, f)
		w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/diff?commit="+bad, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("diff?commit=%q = %d, want 400", bad, w.Code)
		}
		if n := f.execCount(); n != 0 {
			t.Errorf("diff?commit=%q made %d guest calls; a bad hash must reach nothing", bad, n)
		}
	}
}

// The untracked branch must KEEP ignoring git's exit code: `git diff
// --no-index` exits 1 when the files DIFFER, which is its success case.
func TestUntrackedDiffIgnoresExitCodeOne(t *testing.T) {
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		return 1, "diff --git a/new.txt b/new.txt\n+hello\n", nil
	}}
	h, pid := guestTest(t, f)
	w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/diff?path=new.txt&untracked=true", "")
	if w.Code != http.StatusOK {
		t.Fatalf("untracked diff = %d, want 200 — exit 1 means 'files differ'", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hello") {
		t.Errorf("the diff body was dropped: %s", w.Body.String())
	}
}

// ── reads report a stopped VM as a state, not a failure ────────────────────

func TestReadEndpointsReportStoppedAs200(t *testing.T) {
	for _, path := range []string{"/git/branches", "/git/log"} {
		f := &fakeGuest{state: "stopped"}
		h, pid := guestTest(t, f)
		w := do(t, h, http.MethodGet, "/api/projects/"+pid+path, "")
		if w.Code != http.StatusOK {
			t.Errorf("%s with a stopped VM = %d, want 200", path, w.Code)
		}
		if !strings.Contains(w.Body.String(), `"state":"stopped"`) {
			t.Errorf("%s should report state:stopped, got %s", path, w.Body.String())
		}
	}
}

func TestMutationsRefuseAStoppedVM(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"/git/fetch", ""},
		{"/git/pull", ""},
		{"/git/checkout", `{"branch":"x"}`},
		{"/git/stash", `{"op":"push"}`},
	} {
		f := &fakeGuest{state: "stopped"}
		h, pid := guestTest(t, f)
		w := do(t, h, http.MethodPost, "/api/projects/"+pid+tc.path, tc.body)
		if w.Code < 400 {
			t.Errorf("%s with a stopped VM = %d, want a 4xx", tc.path, w.Code)
		}
	}
}

// ── checkFile / formatFile: the gap TODO.md recorded first ─────────────────

func TestFormatReachesTheGuestWithTheContentOnStdin(t *testing.T) {
	// The guest script answers with a STYRMAN_OK / STYRMAN_ERR sentinel so the
	// formatter's own stderr cannot be mistaken for formatted output.
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		return 0, "STYRMAN_OK\npackage main // formatted\n", nil
	}}
	h, pid := guestTest(t, f)
	body, _ := json.Marshal(map[string]string{"path": "a.go", "content": "package main\n"})
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/format", string(body))
	if w.Code != http.StatusOK {
		t.Fatalf("format = %d: %s", w.Code, w.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("format never reached the guest")
	}
	last := f.calls[len(f.calls)-1]
	if len(last) < 3 || last[0] != "bash" || last[1] != "-c" {
		t.Fatalf("format argv = %v, want bash -c <script>", last)
	}
	// The content goes through a temp file fed by stdin — never interpolated
	// into the script, which is what keeps it free of shell metacharacters.
	if strings.Contains(last[2], "package main") {
		t.Error("file content was interpolated into the shell script")
	}
	if !strings.Contains(w.Body.String(), "formatted") {
		t.Errorf("formatted content not returned: %s", w.Body.String())
	}
}

// The sentinel is what separates "the formatter rewrote your file" from "the
// formatter printed an error"; without it a syntax error would be saved as
// content. 422, not 500: the request was fine, the formatter refused it.
func TestFormatSurfacesTheFormattersOwnError(t *testing.T) {
	f := &fakeGuest{reply: func(cmd []string) (int, string, error) {
		return 0, "STYRMAN_ERR\na.go:3:1: expected declaration", nil
	}}
	h, pid := guestTest(t, f)
	body, _ := json.Marshal(map[string]string{"path": "a.go", "content": "package main\n"})
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/format", string(body))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("formatter error = %d, want 422: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "expected declaration") {
		t.Errorf("the formatter's own message should reach the user: %s", w.Body.String())
	}
}

func TestFormatRejectsAnUnsupportedExtensionWithoutTouchingTheGuest(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	body, _ := json.Marshal(map[string]string{"path": "a.unknownext", "content": "x"})
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/format", string(body)); w.Code != http.StatusBadRequest {
		t.Errorf("format of an unsupported type = %d, want 400", w.Code)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if len(c) > 0 && c[0] == "bash" {
			t.Errorf("an unsupported format still ran a script: %v", c)
		}
	}
}

func TestCheckReportsUnsupportedRatherThanRunningAChecker(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	body, _ := json.Marshal(map[string]string{"path": "a.unknownext", "content": "x"})
	w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/check", string(body))
	if w.Code != http.StatusOK {
		t.Fatalf("check = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"supported":false`) {
		t.Errorf("want supported:false, got %s", w.Body.String())
	}
}

func TestCheckRunsACheckerForASupportedType(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	body, _ := json.Marshal(map[string]string{"path": "a.sh", "content": "echo hi\n"})
	if w := do(t, h, http.MethodPost, "/api/projects/"+pid+"/check", string(body)); w.Code != http.StatusOK {
		t.Fatalf("check = %d: %s", w.Code, w.Body.String())
	}
	if len(f.matching("-c")) == 0 {
		t.Error("a supported type should have run a checker in the guest")
	}
}

// ── the seam's own contract ────────────────────────────────────────────────

// Status is what binds an Exec to the right tenant's microVM. A handler that
// looked up the wrong user would exec into someone else's VM, so assert the
// pair the handler actually asks about.
func TestHandlersLookUpTheRequestingUsersOwnProject(t *testing.T) {
	f := &fakeGuest{}
	h, pid := guestTest(t, f)
	if w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/git/log", ""); w.Code != http.StatusOK {
		t.Fatalf("log = %d", w.Code)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.statusFor) == 0 {
		t.Fatal("no Status lookup happened")
	}
	for _, pair := range f.statusFor {
		if pair[0] != "tester" {
			t.Errorf("Status asked about user %q, want the request's own user", pair[0])
		}
		if pair[1] != pid {
			t.Errorf("Status asked about project %q, want %q", pair[1], pid)
		}
	}
}

// The error half of the seam: a Status failure must surface, not be treated
// as a running VM. Untestable before the interface existed.
func TestAStatusFailureIsSurfacedNotIgnored(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
	}{
		{"log", http.MethodGet, "/git/log", ""},
		{"branches", http.MethodGet, "/git/branches", ""},
		{"fetch", http.MethodPost, "/git/fetch", ""},
		{"checkout", http.MethodPost, "/git/checkout", `{"branch":"x"}`},
	} {
		f := &fakeGuest{statusErr: errors.New("docker is unreachable")}
		h, pid := guestTest(t, f)
		w := do(t, h, tc.method, "/api/projects/"+pid+tc.path, tc.body)
		if w.Code < 400 {
			t.Errorf("%s with a failing Status = %d, want an error status", tc.name, w.Code)
		}
		if n := f.execCount(); n != 0 {
			t.Errorf("%s executed %d commands despite Status failing", tc.name, n)
		}
	}
}
