package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/userstore"
)

// testServer builds a Server whose Docker client is never dialed — enough for
// provisionRepo, which reaches the guest only through the injected run func.
func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	// SelfContainer non-empty: waitForBroker is a no-op for a host-mode server
	// (there is no bridge and no broker), and these tests exercise the wait.
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"),
		Username: "tester", AuthMode: "none", SelfContainer: "styrman-server"}
	logger := slog.New(slog.DiscardHandler)
	users, err := userstore.New(filepath.Join(dir, "users.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, users, orch, logger)
}

// newProject registers a real project for the tester, because
// setProvisionError deliberately drops messages for a project that no longer
// exists (it must not resurrect an entry a delete already cleared). Returns
// the project's UUID.
func newProject(t *testing.T, s *Server, name string) string {
	t.Helper()
	sp, err := s.space("tester")
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: name, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

// recorder captures the exact order of provisioning steps.
type recorder struct {
	mu    sync.Mutex
	steps []string
	// reply decides each run() call's result, keyed by call index.
	reply func(n int, cmd []string) (int, string, error)
}

func (r *recorder) attach() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, "attach")
	return true
}

func (r *recorder) run(cmd ...string) (int, string, error) {
	r.mu.Lock()
	n := 0
	for _, s := range r.steps {
		if s != "attach" {
			n++
		}
	}
	r.steps = append(r.steps, strings.Join(cmd, " "))
	r.mu.Unlock()
	if r.reply != nil {
		return r.reply(n, cmd)
	}
	return 0, "", nil
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}

// noSleep collapses one server's provisioning delays so tests run instantly.
func noSleep(s *Server) *Server {
	s.provisionDelay, s.brokerProbeInterval = time.Millisecond, time.Millisecond
	return s
}

// THE regression this whole change exists for: the bridge attach — the only
// thing that makes the credential broker reachable — used to run AFTER
// provisioning, so every clone ran without an ssh key or a git credential and
// failed. Attach must come first, and the broker must be confirmed reachable
// before any command that needs a credential.
func TestProvisionRepoAttachesBeforeAnyCredentialedCommand(t *testing.T) {
	s := noSleep(testServer(t))
	rec := &recorder{}
	s.provisionRepo(context.Background(), provisionSpec{
		user: "u", project: "p", name: "proj", workdir: t.TempDir(),
		cloneURL: "https://github.com/o/r.git",
		attach:   rec.attach, run: rec.run,
	})
	steps := rec.snapshot()
	if len(steps) < 3 {
		t.Fatalf("expected attach + probe + clone, got %q", steps)
	}
	if steps[0] != "attach" {
		t.Errorf("attach must be the FIRST step, got %q", steps)
	}
	if !strings.Contains(steps[1], "/healthz") {
		t.Errorf("broker readiness probe must precede the clone, got %q", steps)
	}
	if !strings.Contains(steps[2], "git ") || !strings.Contains(steps[2], "clone") {
		t.Errorf("clone must follow the probe, got %q", steps)
	}
	for _, st := range steps {
		if strings.Contains(st, "clone") && strings.Contains(st, "/healthz") {
			t.Fatalf("probe and clone collapsed into one step: %q", st)
		}
	}
}

// The clone must run through a LOGIN shell: /etc/profile.d/zz-styrman-credentials.sh
// is what writes ~/.ssh/id_ed25519 into a brand-new per-project home. A bare
// exec is exactly the bug (the clone had no key to offer).
func TestProvisionRepoClonesThroughALoginShell(t *testing.T) {
	s := noSleep(testServer(t))
	rec := &recorder{}
	s.provisionRepo(context.Background(), provisionSpec{
		user: "u", project: "p", name: "proj", workdir: t.TempDir(),
		cloneURL: "git@github.com:o/r.git",
		attach:   rec.attach, run: rec.run,
	})
	var clone string
	for _, st := range rec.snapshot() {
		if strings.Contains(st, "clone --") {
			clone = st
		}
	}
	if clone == "" {
		t.Fatal("no clone command ran")
	}
	if !strings.HasPrefix(clone, "bash -lc ") {
		t.Errorf("clone must run in a login shell (bash -lc), got %q", clone)
	}
	// The URL stays a single quoted argv element after `--` so it can never be
	// read as a git option.
	if !strings.Contains(clone, "clone -- 'git@github.com:o/r.git' .") {
		t.Errorf("clone URL not passed as a quoted operand after --: %q", clone)
	}
	if !strings.Contains(clone, "StrictHostKeyChecking=accept-new") {
		t.Errorf("TOFU host-key policy dropped: %q", clone)
	}
}

// A plain `git init` project needs no credentials: it must not pay for the
// broker probe, and must not run anything in the guest at all.
func TestProvisionRepoSkipsBrokerWaitForPlainInit(t *testing.T) {
	s := noSleep(testServer(t))
	rec := &recorder{}
	s.provisionRepo(context.Background(), provisionSpec{
		user: "u", project: "p", name: "proj", workdir: t.TempDir(),
		attach: rec.attach, run: rec.run,
	})
	if got := rec.snapshot(); len(got) != 1 || got[0] != "attach" {
		t.Errorf("plain init should attach and nothing else, got %q", got)
	}
}

// Both GitHub visibilities create the repo with the project's own name, after
// an initial commit, both in login shells (the commit needs the broker-supplied
// git identity; gh needs GH_TOKEN).
func TestProvisionRepoCreatesGitHubRepo(t *testing.T) {
	for _, vis := range []string{"private", "public"} {
		s := noSleep(testServer(t))
		rec := &recorder{}
		s.provisionRepo(context.Background(), provisionSpec{
			user: "u", project: "p", name: "my-proj", workdir: t.TempDir(),
			github: vis,
			attach: rec.attach, run: rec.run,
		})
		steps := rec.snapshot()
		joined := strings.Join(steps, "\n")
		if steps[0] != "attach" {
			t.Errorf("%s: attach must come first, got %q", vis, steps)
		}
		if !strings.Contains(joined, "bash -lc git commit --allow-empty") {
			t.Errorf("%s: no login-shell initial commit: %q", vis, steps)
		}
		want := "bash -lc gh repo create 'my-proj' --" + vis + " --source=. --remote=origin --push"
		if !strings.Contains(joined, want) {
			t.Errorf("%s: want %q in %q", vis, want, steps)
		}
	}
}

// A failed clone must not vanish into the log: it records the reason so the
// project's API view can say WHY it came up as an empty repo, and it still
// leaves a git repo behind (R13).
func TestProvisionRepoRecordsCloneFailure(t *testing.T) {
	s := noSleep(testServer(t))
	id := newProject(t, s, "proj")
	ws := t.TempDir()
	rec := &recorder{reply: func(n int, cmd []string) (int, string, error) {
		if strings.Contains(strings.Join(cmd, " "), "clone") {
			return 128, "Cloning into '.'...\nfatal: Could not read from remote repository.\n", nil
		}
		return 0, "", nil
	}}
	s.provisionRepo(context.Background(), provisionSpec{
		user: "tester", project: id, name: "proj", workdir: ws,
		cloneURL: "git@github.com:o/r.git",
		attach:   rec.attach, run: rec.run,
	})
	got := s.provisionError("tester", id)
	if !strings.HasPrefix(got, "clone failed: ") || !strings.Contains(got, "Could not read from remote repository") {
		t.Errorf("provisionError = %q, want the git diagnosis", got)
	}
	// Another user must never see it.
	if other := s.provisionError("someone-else", id); other != "" {
		t.Errorf("provisionError leaked across users: %q", other)
	}
	// R13: the fallback init still ran.
	if _, err := os.Stat(filepath.Join(ws, ".git")); err != nil {
		t.Errorf("R13 fallback: no repo left behind: %v", err)
	}
}

// A successful clone records nothing — an empty message is what keeps the
// "repo setup failed" badge off a healthy project.
func TestProvisionRepoRecordsNothingOnSuccess(t *testing.T) {
	s := noSleep(testServer(t))
	id := newProject(t, s, "proj")                       // a REAL project: setProvisionError drops
	rec := &recorder{}                                   // writes for one that does not exist, which
	s.provisionRepo(context.Background(), provisionSpec{ // would make this
		user: "tester", project: id, name: "proj", workdir: t.TempDir(), // pass
		cloneURL: "https://github.com/o/r.git", // vacuously (review 2026-09-02)
		attach:   rec.attach, run: rec.run,
	})
	if got := s.provisionError("tester", id); got != "" {
		t.Errorf("successful clone recorded %q", got)
	}
	// Guard the guard: the same spec with a failing clone MUST record, or this
	// test is proving nothing.
	fail := &recorder{reply: func(int, []string) (int, string, error) { return 1, "fatal: nope", nil }}
	s.provisionRepo(context.Background(), provisionSpec{
		user: "tester", project: id, name: "proj", workdir: t.TempDir(),
		cloneURL: "https://github.com/o/r.git",
		attach:   fail.attach, run: fail.run,
	})
	if got := s.provisionError("tester", id); got == "" {
		t.Error("a failing clone recorded nothing — the success assertion is vacuous")
	}
}

// waitForBroker retries until the broker answers, and gives up rather than
// hanging the boot forever.
func TestWaitForBroker(t *testing.T) {
	s := noSleep(testServer(t))

	calls := 0
	ok := s.waitForBroker(context.Background(), func(cmd ...string) (int, string, error) {
		calls++
		if !strings.Contains(strings.Join(cmd, " "), "/healthz") {
			t.Errorf("probe is not a broker healthz call: %q", cmd)
		}
		if calls < 3 {
			return 7, "", nil // broker not up yet
		}
		return 0, "", nil
	})
	if !ok || calls != 3 {
		t.Errorf("waitForBroker = %v after %d calls, want true after 3", ok, calls)
	}

	if s.waitForBroker(context.Background(), func(...string) (int, string, error) { return 7, "", nil }) {
		t.Error("waitForBroker must give up when the broker never answers")
	}

	// A canceled boot stops probing immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.waitForBroker(ctx, func(...string) (int, string, error) { return 7, "", nil }) {
		t.Error("waitForBroker must not report success on a canceled context")
	}
}

// setProvisionError is the only writer; an empty message clears the entry so a
// project never keeps a stale badge.
func TestProvisionErrorLifecycle(t *testing.T) {
	s := testServer(t)
	id := newProject(t, s, "proj")
	if got := s.provisionError("tester", id); got != "" {
		t.Errorf("fresh project has a message: %q", got)
	}
	s.setProvisionError("tester", id, "clone failed: nope")
	if got := s.provisionError("tester", id); got != "clone failed: nope" {
		t.Errorf("provisionError = %q", got)
	}
	s.setProvisionError("tester", id, "")
	if got := s.provisionError("tester", id); got != "" {
		t.Errorf("cleared message still present: %q", got)
	}

	// Provisioning outlives the project: a message for a record that is gone,
	// or one being deleted, must NOT be stored — nothing would ever clear it
	// again (security review 2026-09-02).
	s.setProvisionError("tester", "no-such-project", "clone failed: nope")
	if got := s.provisionError("tester", "no-such-project"); got != "" {
		t.Errorf("message resurrected for a vanished project: %q", got)
	}
	s.deletingMu.Lock()
	s.deleting["tester/"+id] = true
	s.deletingMu.Unlock()
	s.setProvisionError("tester", id, "clone failed: too late")
	if got := s.provisionError("tester", id); got != "" {
		t.Errorf("message stored for a project being deleted: %q", got)
	}
}

// The whole point of recording a provisioning failure is that the user sees
// it: POST /api/projects answers 201 long before the clone runs, so the
// project list is the only channel back. Exercise the real HTTP plumbing.
func TestProvisionErrorSurfacesInTheProjectAPI(t *testing.T) {
	s := testServer(t)
	h := s.Routes()

	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/projects",
		strings.NewReader(`{"name":"cloned","cloneUrl":"https://github.com/o/r.git"}`)))
	if post.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", post.Code, post.Body.String())
	}
	var created struct {
		ID             string `json:"id"`
		ProvisionError string `json:"provisionError"`
	}
	if err := json.Unmarshal(post.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	list := func() string {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("list = %d: %s", w.Code, w.Body.String())
		}
		var got struct {
			Projects []struct {
				ID             string `json:"id"`
				ProvisionError string `json:"provisionError"`
			} `json:"projects"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		for _, p := range got.Projects {
			if p.ID == created.ID {
				return p.ProvisionError
			}
		}
		t.Fatalf("project %s missing from the list", created.ID)
		return ""
	}

	// omitempty: a healthy project carries no message at all.
	if got := list(); got != "" {
		t.Errorf("fresh project already reports %q", got)
	}
	if strings.Contains(post.Body.String(), "provisionError") {
		t.Errorf("201 body should omit an empty provisionError: %s", post.Body.String())
	}

	// The user who owns the project sees the reason it came up empty…
	s.setProvisionError("tester", created.ID, "clone failed: fatal: repository not found")
	if got := list(); got != "clone failed: fatal: repository not found" {
		t.Errorf("list provisionError = %q", got)
	}
	// …and so does the single-project view.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/projects/"+created.ID, nil))
	if !strings.Contains(w.Body.String(), "repository not found") {
		t.Errorf("GET /api/projects/{id} dropped provisionError: %s", w.Body.String())
	}

	// Deleting the project drops the message with it — nothing outlives the
	// record it describes.
	del := httptest.NewRecorder()
	h.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, "/api/projects/"+created.ID, nil))
	if del.Code != http.StatusAccepted {
		t.Fatalf("delete = %d: %s", del.Code, del.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.provisionError("tester", created.ID) == "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("provisionError outlived the deleted project: %q", s.provisionError("tester", created.ID))
}

// The other outcome of a create: the microVM never started. Both credentialed
// paths must SAY so — the GitHub arm used to fail silently here, which is the
// same defect class as the clone bug this file exists to prevent. R13 still
// has to hold for a clone (verifier 2026-09-02).
func TestBootFailedReportsAndKeepsR13(t *testing.T) {
	t.Run("clone says so and still leaves a repo", func(t *testing.T) {
		s := testServer(t)
		id := newProject(t, s, "proj")
		ws := t.TempDir()
		s.bootFailed(context.Background(), provisionSpec{
			user: "tester", project: id, name: "proj", workdir: ws,
			cloneURL: "https://github.com/o/r.git",
		})
		if got := s.provisionError("tester", id); got != "clone skipped: the project VM did not start" {
			t.Errorf("provisionError = %q", got)
		}
		if _, err := os.Stat(filepath.Join(ws, ".git")); err != nil {
			t.Errorf("R13: no repo left behind after a failed boot: %v", err)
		}
	})

	for _, vis := range []string{"private", "public"} {
		t.Run("github "+vis+" says so", func(t *testing.T) {
			s := testServer(t)
			id := newProject(t, s, "proj")
			ws := t.TempDir()
			s.bootFailed(context.Background(), provisionSpec{
				user: "tester", project: id, name: "proj", workdir: ws,
				github: vis,
			})
			if got := s.provisionError("tester", id); got != "GitHub repo not created: the project VM did not start" {
				t.Errorf("provisionError = %q — the GitHub path must not fail silently", got)
			}
		})
	}

	t.Run("plain init reports nothing", func(t *testing.T) {
		s := testServer(t)
		id := newProject(t, s, "proj")
		ws := t.TempDir()
		s.bootFailed(context.Background(), provisionSpec{
			user: "tester", project: id, name: "proj", workdir: ws,
		})
		// Its repo was created server-side before the 201, so a failed boot
		// costs it nothing and there is nothing to report.
		if got := s.provisionError("tester", id); got != "" {
			t.Errorf("plain init reported %q", got)
		}
		if _, err := os.Stat(filepath.Join(ws, ".git")); err == nil {
			t.Error("bootFailed initialised a repo for a project that needs no guest work")
		}
	})
}
