// Package terminal bridges browser WebSockets to tmux sessions inside styrman
// microVMs over Docker's exec API (ADR-002). The wire protocol is ported
// verbatim from devbox: binary frames carry raw PTY bytes, JSON text frames
// carry control messages ({"type":"resize","cols":N,"rows":N}), close code
// 4000 means the shell genuinely exited, 4001 means this connection was
// replaced by a newer one and the client must not auto-reconnect, 4002 means
// the project was deliberately stopped and only a boot-intent connect
// (?boot=1) may bring it back (ADR-042).
package terminal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/conversation"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// TermStore is the slice of the per-user terminal store the bridge needs.
type TermStore interface {
	Get(project, id string) (conversation.Record, bool)
	Touch(project, id string)
}

// Bridge serves /ws/term/{project}/{term} and /ws/setup.
type Bridge struct {
	orch     *orchestrator.Orchestrator
	cfg      *config.Config
	logger   *slog.Logger
	storeFor func(user string) (TermStore, error)
	ensure   func(ctx context.Context, user, projectID string) (containerID, guestUser, dir string, err error)
	guestFor func(user string) string

	// activeConns maps a session key → the current *websocket.Conn. A new
	// connection closes the previous one with CloseReplaced so only one client
	// attaches to a tmux session at a time (duplicate attaches garble output).
	// swapMu makes the load-old / store-new swap atomic — otherwise two
	// near-simultaneous attaches could both see an empty slot, both attach,
	// and the tmux -D kick spins the frontend into a reconnect loop.
	swapMu      sync.Mutex
	activeConns sync.Map
}

// NewBridge creates a Bridge. storeFor resolves a user's terminal store.
func NewBridge(orch *orchestrator.Orchestrator, cfg *config.Config, logger *slog.Logger,
	storeFor func(user string) (TermStore, error),
	ensure func(ctx context.Context, user, projectID string) (string, string, string, error),
	guestFor func(user string) string,
) *Bridge {
	return &Bridge{orch: orch, cfg: cfg, logger: logger, storeFor: storeFor, ensure: ensure, guestFor: guestFor}
}

type controlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

func clientSize(r *http.Request) (cols, rows uint16) {
	cols, rows = 80, 24
	if v, err := strconv.ParseUint(r.URL.Query().Get("cols"), 10, 16); err == nil && v > 0 {
		cols = uint16(v)
	}
	if v, err := strconv.ParseUint(r.URL.Query().Get("rows"), 10, 16); err == nil && v > 0 {
		rows = uint16(v)
	}
	return cols, rows
}

// ServeHTTP attaches a project terminal: {project} and {term} path values
// plus optional cols/rows query params.
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := identity.FromContext(r.Context()).Username
	projName := r.PathValue("project")
	convID := r.PathValue("term")
	store, err := b.storeFor(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := conversation.ValidateID(convID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Every terminal is a login shell (ADR-014/016): the user launches
	// claude/agy themselves — both are in the image, and /etc/profile.d
	// materializes the broker credentials (env vars + files, ADR-032). Run
	// several terminals to work with several agents at once.
	paneCmd := "bash -l"
	// Strict: the record must exist (created via POST). Auto-creating here
	// let stale reconnects resurrect deleted terminals.
	rec, ok := store.Get(projName, convID)
	if !ok {
		http.Error(w, "unknown terminal", http.StatusNotFound)
		return
	}
	// Autostart (operator Q8): the command runs when the tmux SESSION is
	// created — `tmux new -A` reattaches an existing session untouched, so a
	// reconnect never re-runs it; a pause or VM restart (session gone) does.
	if rec.Command != "" {
		paneCmd = autostartPaneCmd(rec.Command)
	}
	store.Touch(projName, convID)
	tmuxName := conversation.Prefix + convID

	// Deliberately-stopped gate: only a connection carrying boot intent
	// (`?boot=1` — the first connect of a freshly opened/resumed pane, i.e. a
	// user action) may boot a project the user stopped or the idle reaper
	// reaped. An AUTOMATIC reconnect (retry loop after a transient close)
	// carries no boot param and gets close 4002 instead — otherwise pressing
	// Stop with a terminal tab open boots the VM straight back up.
	if r.URL.Query().Get("boot") != "1" && b.orch.Stopped(user, projName) {
		if conn, err := websocket.Accept(w, r, nil); err == nil {
			conn.Close(websocket.StatusCode(config.CloseProjectStopped), "project stopped")
		}
		return
	}

	// Ensure the microVM is up before upgrading — first attach after idle-stop
	// pays the (~seconds) start cost here. The callback resolves the record
	// (dir, guest username) the orchestrator needs.
	containerID, guest, dir, err := b.ensure(r.Context(), user, projName)
	if err != nil {
		b.logger.Error("ensure running failed", "project", projName, "error", err)
		http.Error(w, "failed to start project", http.StatusInternalServerError)
		return
	}

	workdir := "/home/" + guest + "/projects/" + dir
	b.serve(w, r, session{
		containerID: containerID,
		user:        user,
		guest:       guest,
		key:         user + "/" + projName + "/" + tmuxName,
		tmuxName:    tmuxName,
		workdir:     workdir,
		paneCmd:     paneCmd,
		project:     projName,
	})
}

// ServeSetup attaches the hidden onboarding microVM running styrman-setup
// (no project, not listed anywhere). Reconnects re-enter the same tmux
// session; when styrman-setup exits the pane ends and the client gets close
// 4000 — the frontend finalizes setup and tears the VM down on that signal,
// so the VM auto-closes on completion instead of idling at a shell prompt.
func (b *Bridge) ServeSetup(w http.ResponseWriter, r *http.Request) {
	user := identity.FromContext(r.Context()).Username
	guest := b.guestFor(user)
	containerID, err := b.orch.EnsureSetup(r.Context(), user, guest)
	if err != nil {
		b.logger.Error("ensure setup vm failed", "user", user, "error", err)
		http.Error(w, "failed to start setup environment", http.StatusInternalServerError)
		return
	}
	// Optional targeted tool (?tool=gcloud) → run just that step, for the "add
	// one credential later" flow. STRICTLY allowlisted: the value is
	// interpolated into a shell command, so only known-safe tokens pass.
	tool := setupTool(r.URL.Query().Get("tool"))
	paneCmd := "bash -lc 'styrman-setup'"
	tmuxName := "styrman-setup"
	if tool != "" {
		paneCmd = "bash -lc 'styrman-setup " + tool + "'"
		tmuxName = "styrman-setup-" + tool // distinct session per tool
	}
	b.serve(w, r, session{
		containerID: containerID,
		user:        user,
		guest:       guest,
		key:         user + "/system/setup/" + tool,
		tmuxName:    tmuxName,
		workdir:     "/home/" + guest,
		// No trailing `exec bash -l`: styrman-setup is a guided one-shot, so
		// when it exits the pane should end (→ close 4000 → frontend finalizes)
		// rather than drop the user into an idle interactive shell.
		paneCmd: paneCmd,
		project: "", // system VM: no idle tracking
	})
}

// setupTools is the allowlist of tools styrman-setup accepts as an argument.
var setupTools = map[string]bool{
	"git": true, "ssh": true, "gh": true, "gcloud": true, "claude": true, "agy": true,
}

// setupTool returns v only if it's an allowlisted tool name, else "" (full flow).
func setupTool(v string) string {
	if setupTools[v] {
		return v
	}
	return ""
}

// session describes one terminal attachment target.
type session struct {
	containerID string
	user        string // identity slug (activity keys)
	guest       string // in-guest username (exec identity, paths)
	key         string // activeConns key
	tmuxName    string
	workdir     string
	paneCmd     string
	project     string // "" for system VMs
}

// serve upgrades the WebSocket and pumps it against a tmux session in the
// target container. Shared by conversation and setup terminals.
func (b *Bridge) serve(w http.ResponseWriter, r *http.Request, s session) {
	cols, rows := clientSize(r)

	// Default Origin verification (Origin host must match Host) stays ON:
	// WebSockets bypass CORS, and without this any web page the operator
	// visits could drive a terminal. The Phase 3 ingress preserves Host, so
	// same-origin keeps working behind Caddy.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	// Cap a single client frame at 16 MiB — enough for a large paste, but a
	// buggy/hostile client can't OOM the server with an unbounded frame.
	conn.SetReadLimit(16 << 20)

	// Atomically replace any previous connection for this session (close 4001;
	// the frontend must not auto-reconnect on it, or two clients would fight
	// over the tmux session in a reconnect death-spiral). The old Close runs
	// in a goroutine: a dead peer's close handshake can block ~5-10 s, which
	// must not stall this new attach (it blew the "reopen < 5 s" SLA).
	b.swapMu.Lock()
	if old, loaded := b.activeConns.LoadAndDelete(s.key); loaded {
		oc := old.(*websocket.Conn)
		go oc.Close(websocket.StatusCode(config.CloseReplaced), "replaced by new connection")
	}
	b.activeConns.Store(s.key, conn)
	b.swapMu.Unlock()
	defer b.activeConns.CompareAndDelete(s.key, conn)

	home := "/home/" + s.guest
	spec := orchestrator.ExecSpec{
		User:    s.guest,
		WorkDir: s.workdir,
		// -x/-y seed the size for a newly created session (avoids the
		// double-prompt redraw); -c sets the pane's start directory. -D
		// detaches stale clients (e.g. left by a crashed server). Safe with
		// -A because a replaced browser gets close 4001 and never reconnects,
		// so there is no detach/reattach spiral.
		Cmd: []string{"tmux", "new-session", "-A", "-D", "-s", s.tmuxName,
			"-x", strconv.Itoa(int(cols)), "-y", strconv.Itoa(int(rows)),
			"-c", s.workdir, s.paneCmd},
		Env: []string{
			"HOME=" + home,
			"USER=" + s.guest,
			"SHELL=/bin/bash",
			"TERM=xterm-256color",
			"LANG=en_US.UTF-8",
			"LC_ALL=en_US.UTF-8",
		},
	}

	ex, err := b.orch.StartInteractive(r.Context(), s.containerID, spec, uint(cols), uint(rows))
	if err != nil {
		b.logger.Error("exec start failed", "session", s.key, "error", err)
		conn.Close(websocket.StatusInternalError, "failed to attach")
		return
	}
	defer ex.Close()

	b.logger.Info("terminal attached", "session", s.key)

	touch := func() {
		if s.project != "" {
			b.orch.Touch(s.user, s.project)
		}
	}

	// hctx governs the whole attachment: cancelled by browser disconnect (WS
	// reader), stream EOF, or handler exit. The watcher below turns that into
	// ex.Close(), which unblocks the stream reader — without it, a closed
	// browser tab would leave the exec, hijacked conn, and a ghost tmux
	// client attached forever (review finding).
	hctx, hcancel := context.WithCancel(r.Context())
	defer hcancel()
	go func() {
		<-hctx.Done()
		ex.Close()
	}()

	var wg sync.WaitGroup

	// browserGone marks that the WebSocket peer died (read error or missed
	// heartbeat) — as opposed to the tmux session ending. When it's set, the
	// close code is moot (nobody's listening) and we skip the has-session probe
	// (N16/I3: don't pay an extra exec round-trip on every tab close).
	var browserGone atomic.Bool

	// Heartbeat: detect dead connections faster than TCP keepalive would.
	// Each ping gets its own deadline — Ping blocks until pong or ctx done,
	// so an undeadlined ping on a half-open connection never returns.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-hctx.Done():
				return
			case <-ticker.C:
				pctx, pcancel := context.WithTimeout(hctx, 10*time.Second)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					browserGone.Store(true)
					hcancel()
					return
				}
			}
		}
	}()

	// exec stream → WebSocket. EOF here means the tmux client detached or the
	// pane command exited; the exit-code check below disambiguates.
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		defer hcancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := ex.Read(buf)
			if n > 0 {
				touch()
				if writeErr := conn.Write(hctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → exec stream (stdin + resize). A read error here is a browser
	// disconnect: do NOT signal the shell — the tmux session must survive it.
	// hcancel tears down our attach; the tmux session lives on.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer hcancel()
		for {
			msgType, data, err := conn.Read(hctx)
			if err != nil {
				// Only a genuine peer failure marks the browser gone; a read
				// error caused by our own hcancel (session exit/detach) leaves
				// hctx cancelled and must NOT suppress the 4000 close code.
				if hctx.Err() == nil {
					browserGone.Store(true)
				}
				return
			}
			touch()
			if msgType == websocket.MessageText {
				var msg controlMessage
				if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
					_ = ex.Resize(hctx, uint(msg.Cols), uint(msg.Rows))
				}
				continue
			}
			if _, err := ex.Write(data); err != nil {
				return
			}
		}
	}()

	<-streamDone

	// Decide the close code. The has-session probe is a side effect, so it runs
	// only where its answer matters — the browser is still here AND the exec
	// finished — never on a plain tab close (I3). browserGone → the peer's gone,
	// code is moot. done+probed disambiguate `tmux detach` (session lives →
	// reconnect) from a real exit (4000). Probe error → fail toward reconnect,
	// not a stranded 4000 (I2). ExitCodeWait polls briefly: dockerd's exec
	// bookkeeping lags stream EOF by a few ms.
	gone := browserGone.Load()
	var exitCode int
	var done, alive, probed bool
	if !gone {
		if exitCode, done = ex.ExitCodeWait(context.Background(), time.Second); done {
			alive, probed = b.sessionAlive(s)
		}
	}
	code, reason := closeDecision(gone, done, alive, probed)
	// Deterministic stop signal: when the project is deliberately stopped
	// (Stop button / idle reaper), every pane close is 4002 — without this,
	// the ExitCodeWait race lands some panes on 4000 ("Paused") and others on
	// a transient close, so the user sees inconsistent states on Stop.
	if !gone && s.project != "" && b.orch.Stopped(s.user, s.project) {
		code, reason = config.CloseProjectStopped, "project stopped"
	}
	if code == config.CloseShellExited {
		b.logger.Info("session exited", "session", s.key, "exitCode", exitCode)
	} else {
		b.logger.Info("terminal closed", "session", s.key, "reason", reason)
	}
	conn.Close(websocket.StatusCode(code), reason)
	hcancel()
	wg.Wait()
}

// closeDecision maps the post-stream state to a WebSocket close code (extracted
// for testing). CloseShellExited (4000) tells the client the shell genuinely
// ended ("Paused"); StatusNormalClosure lets it silently reconnect.
func closeDecision(browserGone, done, alive, probed bool) (code int, reason string) {
	switch {
	case browserGone:
		return int(websocket.StatusNormalClosure), "peer gone"
	case !done:
		return int(websocket.StatusNormalClosure), "detached"
	case probed && !alive:
		return config.CloseShellExited, "shell exited"
	case probed && alive:
		return int(websocket.StatusNormalClosure), "detached"
	default: // probe failed — don't strand a possibly-live session
		return int(websocket.StatusNormalClosure), "detached (probe failed)"
	}
}

// sessionAlive reports whether the tmux session still exists in the guest —
// used to tell a `tmux detach` (session lives) from a real shell exit. Returns
// probed=false when the check itself failed (dockerd hiccup, timeout), so the
// caller can pick a safe fallback instead of trusting a guessed answer.
//
// The target is prefixed with '=' to force EXACT matching: without it tmux
// resolves `conv-1` to the live `conv-10` (prefix match), so a real exit of
// terminal 1 would look alive whenever a conv-1x session exists (B3).
func (b *Bridge) sessionAlive(s session) (alive, probed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	code, _, err := b.orch.Exec(ctx, s.containerID, orchestrator.ExecSpec{
		User: s.guest,
		Cmd:  []string{"tmux", "has-session", "-t", "=" + s.tmuxName},
		Env:  []string{"HOME=/home/" + s.guest, "USER=" + s.guest},
	})
	if err != nil {
		b.logger.Warn("has-session probe failed", "session", s.key, "error", err)
	}
	return aliveFromProbe(code, err)
}

// aliveFromProbe maps a has-session exec result to (alive, probed) — pure so
// the close-decision path is unit-testable end to end with close_test.go:
// an exec ERROR means the probe never answered (probed=false → the caller
// fails toward reconnect, I2); exit 0 = session alive, non-zero = gone.
func aliveFromProbe(code int, err error) (alive, probed bool) {
	if err != nil {
		return false, false
	}
	return code == 0, true
}
