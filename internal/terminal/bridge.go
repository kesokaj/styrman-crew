// Package terminal bridges browser WebSockets to tmux sessions inside project
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
	"github.com/kesokaj/styrman/internal/orchestrator"
	"github.com/kesokaj/styrman/internal/project"
)

// Bridge is the /ws/term/{project}/{conv} handler.
type Bridge struct {
	orch     *orchestrator.Orchestrator
	projects *project.Store
	cfg      *config.Config
	logger   *slog.Logger

	// activeConns maps project/conv → the current *websocket.Conn. A new
	// connection closes the previous one with CloseReplaced so only one client
	// attaches to a tmux session at a time (duplicate attaches garble output).
	activeConns sync.Map
}

// NewBridge creates a Bridge.
func NewBridge(orch *orchestrator.Orchestrator, projects *project.Store, cfg *config.Config, logger *slog.Logger) *Bridge {
	return &Bridge{orch: orch, projects: projects, cfg: cfg, logger: logger}
}

type controlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

// ServeHTTP expects {project} and {conv} path values plus optional
// cols/rows/kind query params (kind=shell attaches a plain login shell
// instead of the project's agent CLI).
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	projName := r.PathValue("project")
	convID := r.PathValue("conv")
	proj, err := b.projects.Get(projName)
	if err != nil {
		http.Error(w, "unknown project", http.StatusNotFound)
		return
	}
	if err := conversation.ValidateID(convID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cols, rows := uint16(80), uint16(24)
	if v, err := strconv.ParseUint(r.URL.Query().Get("cols"), 10, 16); err == nil && v > 0 {
		cols = uint16(v)
	}
	if v, err := strconv.ParseUint(r.URL.Query().Get("rows"), 10, 16); err == nil && v > 0 {
		rows = uint16(v)
	}

	// The pane command: the agent CLI is the conversation (R4); kind=shell
	// gives a plain terminal into the same microVM. `tmux new -A` attaches if
	// the session already exists (the command is then ignored), so reconnects
	// land in the same session with scrollback and running processes intact.
	// The agent runs under a login shell so /etc/profile.d applies — the
	// keyring env (DBUS_SESSION_BUS_ADDRESS) in particular, which agy's
	// Secret Service auth needs. tmux runs a single-string command via sh.
	paneCmd := "bash -lc 'exec " + config.Agents[proj.Agent] + "'"
	if r.URL.Query().Get("kind") == "shell" {
		paneCmd = "bash -l"
	}

	// Ensure the microVM is up before upgrading — first attach after idle-stop
	// pays the (~seconds) start cost here.
	containerID, err := b.orch.EnsureRunning(r.Context(), projName)
	if err != nil {
		b.logger.Error("ensure running failed", "project", projName, "error", err)
		http.Error(w, "failed to start project", http.StatusInternalServerError)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // TLS terminates at the ingress (Phase 3)
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)

	// Replace any previous connection for this conversation (close 4001;
	// the frontend must not auto-reconnect on it, or two clients would fight
	// over the tmux session in a reconnect death-spiral).
	key := projName + "/" + convID
	if old, loaded := b.activeConns.LoadAndDelete(key); loaded {
		old.(*websocket.Conn).Close(websocket.StatusCode(config.CloseReplaced), "replaced by new connection")
	}
	b.activeConns.Store(key, conn)
	defer b.activeConns.CompareAndDelete(key, conn)

	home := "/home/" + b.cfg.Username
	workdir := home + "/projects/" + projName
	sess := conversation.Prefix + convID
	spec := orchestrator.ExecSpec{
		User:    b.cfg.Username,
		WorkDir: workdir,
		// -x/-y seed the size for a newly created session (avoids the
		// double-prompt redraw); -c sets the pane's start directory.
		Cmd: []string{"tmux", "new-session", "-A", "-s", sess,
			"-x", strconv.Itoa(int(cols)), "-y", strconv.Itoa(int(rows)),
			"-c", workdir, paneCmd},
		Env: []string{
			"HOME=" + home,
			"USER=" + b.cfg.Username,
			"SHELL=/bin/bash",
			"TERM=xterm-256color",
			"LANG=en_US.UTF-8",
			"LC_ALL=en_US.UTF-8",
		},
	}

	ex, err := b.orch.StartInteractive(r.Context(), containerID, spec, uint(cols), uint(rows))
	if err != nil {
		b.logger.Error("exec start failed", "project", projName, "conv", convID, "error", err)
		conn.Close(websocket.StatusInternalError, "failed to attach")
		return
	}
	defer ex.Close()

	b.logger.Info("terminal attached", "project", projName, "conv", convID, "kind", paneCmd)
	ctx := r.Context()
	var wg sync.WaitGroup

	// Heartbeat: detect dead connections faster than TCP keepalive would.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := conn.Ping(ctx); err != nil {
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
		buf := make([]byte, 32*1024)
		for {
			n, err := ex.Read(buf)
			if n > 0 {
				b.orch.Touch(projName)
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			b.orch.Touch(projName)
			if msgType == websocket.MessageText {
				var msg controlMessage
				if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
					_ = ex.Resize(ctx, uint(msg.Cols), uint(msg.Rows))
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
	// gets the semantic close code; everything else closes normally and the
	// client may silently reconnect.
	if code, done := ex.ExitCode(context.Background()); done {
		b.logger.Info("conversation exited", "project", projName, "conv", convID, "exitCode", code)
		conn.Close(websocket.StatusCode(config.CloseShellExited), "shell exited")
	} else {
		conn.Close(websocket.StatusNormalClosure, "detached")
	}
	wg.Wait()
}
