// Package terminal bridges browser WebSockets to tmux sessions inside styrman
// microVMs over Docker's exec API (ADR-002). The wire protocol is ported
// verbatim from devbox: binary frames carry raw PTY bytes, JSON text frames
// carry control messages ({"type":"resize","cols":N,"rows":N}), close code
// 4000 means the shell genuinely exited, 4001 means this connection was
// replaced by a newer one and the client must not auto-reconnect.
package terminal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/conversation"
	"github.com/kesokaj/styrman/internal/identity"
	"github.com/kesokaj/styrman/internal/orchestrator"
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
	// provides the keyring env agy needs. Run several terminals to work with
	// several agents at once.
	paneCmd := "bash -l"
	// Strict: the record must exist (created via POST). Auto-creating here
	// let stale reconnects resurrect deleted terminals.
	if _, ok := store.Get(projName, convID); !ok {
		http.Error(w, "unknown terminal", http.StatusNotFound)
		return
	}
	store.Touch(projName, convID)
	tmuxName := conversation.Prefix + convID

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
// session; when the script's shell exits the client gets close 4000.
func (b *Bridge) ServeSetup(w http.ResponseWriter, r *http.Request) {
	user := identity.FromContext(r.Context()).Username
	guest := b.guestFor(user)
	containerID, err := b.orch.EnsureSetup(r.Context(), user, guest)
	if err != nil {
		b.logger.Error("ensure setup vm failed", "user", user, "error", err)
		http.Error(w, "failed to start setup environment", http.StatusInternalServerError)
		return
	}
	b.serve(w, r, session{
		containerID: containerID,
		user:        user,
		guest:       guest,
		key:         user + "/system/setup",
		tmuxName:    "styrman-setup",
		workdir:     "/home/" + guest,
		paneCmd:     "bash -lc 'styrman-setup; exec bash -l'",
		project:     "", // system VM: no idle tracking
	})
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
	conn.SetReadLimit(-1)

	// Replace any previous connection for this session (close 4001; the
	// frontend must not auto-reconnect on it, or two clients would fight over
	// the tmux session in a reconnect death-spiral).
	if old, loaded := b.activeConns.LoadAndDelete(s.key); loaded {
		old.(*websocket.Conn).Close(websocket.StatusCode(config.CloseReplaced), "replaced by new connection")
	}
	b.activeConns.Store(s.key, conn)
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

	// The exec ends either because the session truly exited (user typed exit,
	// agent quit) or because our attach was torn down. Only a finished exec
	// gets the semantic close code (4000); everything else closes normally and
	// the client may silently reconnect. ExitCodeWait polls briefly: dockerd's
	// exec bookkeeping lags stream EOF by a few ms, and misreading it would
	// send a normal close where 4000 belongs (review finding).
	if code, done := ex.ExitCodeWait(context.Background(), time.Second); done {
		b.logger.Info("session exited", "session", s.key, "exitCode", code)
		conn.Close(websocket.StatusCode(config.CloseShellExited), "shell exited")
	} else {
		conn.Close(websocket.StatusNormalClosure, "detached")
	}
	hcancel()
	wg.Wait()
}
