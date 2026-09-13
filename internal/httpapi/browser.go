package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/terminal"
)

// Browser view (ADR-068): one headless Chromium per project microVM, started
// on demand by the in-guest launcher /styrman/bin/styrman-browser, watched by
// the operator through a DevTools-protocol (CDP) screencast the Browser pane
// draws, and driven by the agents through an MCP server registered at
// seeding. This file is the server's whole share of it: three owner-only REST
// verbs over the guestRuntime seam (Status first, then a docker exec of the
// launcher — never a classification of an exec error's text), and a relay
// that pumps text frames between the pane's WebSocket and the guest's
// browser-level DevTools endpoint on the project bridge, knowing nothing of
// the protocol itself.

// browserLauncher is the in-guest launcher's path (image/configs/styrman-browser).
const browserLauncher = "/styrman/bin/styrman-browser"

// browserStatus is what the launcher's `status` verb prints and what the REST
// status route returns.
type browserStatus struct {
	Running bool   `json:"running"`
	Version string `json:"version,omitempty"`
}

// errProjectNotRunning: the verbs refuse to exec into a project that is not
// running — the REST routes answer 409 and the relay closes 4002.
var errProjectNotRunning = errors.New("project is not running")

// previewPortAllowed is the one place the preview proxy, the share list and
// the link mint ask whether a port may leave the owner's hands. The
// browser's two ports are refused, naming the port asked for: a DevTools
// client controls the whole browser (including file:// reads inside the
// guest), so they are owner-only through the relay and never previewable,
// shareable or linkable.
func previewPortAllowed(port int) error {
	if port == config.BrowserPort || port == config.BrowserChromePort {
		return fmt.Errorf("port %d is the project's browser and cannot be previewed or shared", port)
	}
	return nil
}

// browserExec runs one launcher verb as the project user. Status comes
// FIRST, exactly as ListeningPorts does: a project that is not running is
// errProjectNotRunning before any exec, and an exec that fails against a
// container that has since vanished is re-asked through Status rather than
// read off the error text — a real Docker failure against the container we
// are still looking at stays a real error.
func (s *Server) browserExec(ctx context.Context, user, project, guestUser, verb string) (int, string, error) {
	st, err := s.guest.Status(ctx, user, project)
	if err != nil {
		return -1, "", err
	}
	if st.State != "running" {
		return -1, "", errProjectNotRunning
	}
	env := []string{"HOME=/home/" + guestUser, "USER=" + guestUser}
	if verb == "start" {
		// The forwarder admits exactly this address (ADR-068 amendment). The
		// variable is ALWAYS set on a server-run start — empty when the peer
		// is unknown — because the launcher reads "set but empty" as "no
		// forwarder" and only an UNSET variable (an agent-started browser)
		// falls back to the broker's host. Omitting it here would hand the
		// launcher that fallback and undo the fail-closed answer (security
		// review, round 4).
		env = append(env, "STYRMAN_BROWSER_PEER="+s.browserPeer(ctx, project))
	}
	code, out, err := s.guest.Exec(ctx, st.ContainerID, orchestrator.ExecSpec{
		Cmd:  []string{browserLauncher, verb},
		User: guestUser,
		Env:  env,
	})
	if err != nil || code != 0 {
		if now, err2 := s.guest.Status(ctx, user, project); err2 == nil &&
			(now.State != "running" || now.ContainerID != st.ContainerID) {
			return -1, "", errProjectNotRunning
		}
	}
	return code, out, err
}

// browserStatusOf asks the launcher; a not-running project is reported as
// not running rather than as an error, since that is the honest answer.
func (s *Server) browserStatusOf(ctx context.Context, user, project, guestUser string) (browserStatus, error) {
	code, out, err := s.browserExec(ctx, user, project, guestUser, "status")
	if errors.Is(err, errProjectNotRunning) {
		return browserStatus{}, nil
	}
	if err != nil {
		return browserStatus{}, err
	}
	if code != 0 {
		return browserStatus{}, fmt.Errorf("browser status exited %d: %s", code, tail(out, 2048))
	}
	var st browserStatus
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &st); err != nil {
		return browserStatus{}, fmt.Errorf("browser status: unreadable reply: %s", tail(out, 256))
	}
	return st, nil
}

// browserStart starts the browser (idempotent in the launcher) and returns
// its status. A failed launch surfaces the launcher's log tail verbatim: it
// is the only diagnostic there is, and it reaches the pane.
func (s *Server) browserStart(ctx context.Context, user, project, guestUser string) (browserStatus, error) {
	code, out, err := s.browserExec(ctx, user, project, guestUser, "start")
	if err != nil {
		return browserStatus{}, err
	}
	if code != 0 {
		return browserStatus{}, fmt.Errorf("browser did not start: %s", tail(out, 2048))
	}
	return s.browserStatusOf(ctx, user, project, guestUser)
}

func (s *Server) browserStop(ctx context.Context, user, project, guestUser string) error {
	code, out, err := s.browserExec(ctx, user, project, guestUser, "stop")
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("browser stop exited %d: %s", code, tail(out, 2048))
	}
	return nil
}

// tail keeps the last max bytes of a launcher's output — the log tail is at
// the end, and a runaway log must not become a runaway error body. The cut
// lands on a rune boundary: the output is shown as text, and a split
// multi-byte sequence would render as garbage (or, in a close frame, make
// the frame invalid UTF-8).
func tail(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	cut := len(s) - max
	for cut < len(s) && !utf8RuneStart(s[cut]) {
		cut++
	}
	return "…" + s[cut:]
}

// GET /api/projects/{project}/browser — owner only.
func (s *Server) getBrowser(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.browserStatusOf(r.Context(), u.Username, p.ID, sp.profile.GuestUser())
	if err != nil {
		s.logger.Warn("browser status failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("browser status is not available right now"))
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// POST /api/projects/{project}/browser/start — owner only. 409 when the
// project is not running, 503 + Retry-After on a fresh bridge attach (the
// reply may never arrive through a netns that was just reconfigured, so the
// client's one retry is the contract), 502 with the launcher's log tail
// when Chromium does not come up.
func (s *Server) startBrowser(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if _, fresh, err := s.browserUpstream(r.Context(), u.Username, p.ID); err != nil {
		s.logger.Warn("browser: upstream lookup failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusBadGateway, fmt.Errorf("browser is not available right now"))
		return
	} else if fresh {
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("attaching to the project network — retry"))
		return
	}
	st, err := s.browserStart(r.Context(), u.Username, p.ID, sp.profile.GuestUser())
	if errors.Is(err, errProjectNotRunning) {
		writeErr(w, http.StatusConflict, errProjectNotRunning)
		return
	}
	if err != nil {
		s.logger.Warn("browser start failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// POST /api/projects/{project}/browser/stop — owner only; 204, or 409 when
// the project is not running.
func (s *Server) stopBrowser(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	err = s.browserStop(r.Context(), u.Username, p.ID, sp.profile.GuestUser())
	if errors.Is(err, errProjectNotRunning) {
		writeErr(w, http.StatusConflict, errProjectNotRunning)
		return
	}
	if err != nil {
		s.logger.Warn("browser stop failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// browserPeer is the server's own address on the project bridge, the one
// source the guest's forwarder admits. Tests override it.
func (s *Server) browserPeer(ctx context.Context, project string) string {
	if s.browserPeerFn != nil {
		return s.browserPeerFn(ctx, project)
	}
	return s.orch.BrowserPeer(ctx, project)
}

// browserUpstream resolves where the project's DevTools endpoint answers —
// "<guest-ip>:9222" on the project bridge — after making sure this server is
// attached to that bridge. fresh reports an attach that happened just now,
// which may have doomed the current connection (see EnsureSelfAttached).
// host is "" when the project is not running. A test may replace it.
func (s *Server) browserUpstream(ctx context.Context, user, project string) (host string, fresh bool, err error) {
	if s.browserUpstreamFn != nil {
		return s.browserUpstreamFn(ctx, user, project)
	}
	fresh = s.orch.EnsureSelfAttached(user, project)
	ip, err := s.orch.IPAddress(ctx, project)
	if err != nil || ip == "" {
		return "", fresh, err
	}
	return fmt.Sprintf("%s:%d", ip, config.BrowserPort), fresh, nil
}

// browserRelayReadLimit bounds a frame from the guest: a screencast frame is
// a base64 JPEG inside JSON — ~100–200 KiB at 1280×800 — and 16 MiB is the
// same ceiling the terminal bridge uses. The pane's own frames are tiny.
// browserRelayMax caps concurrent relays server-wide and browserRelayPerUser
// per user: each relay costs a docker exec, two goroutines and up to 17 MiB
// of frame buffers, and a global cap alone would let one user's abandoned
// panes lock every other user out (security review, rounds 2 and 3). Slots
// free themselves: the pane side is pinged every browserRelayPing and a
// pane that stops answering is closed.
const (
	browserRelayReadLimit = 16 << 20
	browserPaneReadLimit  = 1 << 20
	browserRelayMax       = 32
	browserRelayPerUser   = 8
	browserRelayPing      = 30 * time.Second
)

// relayCounter tracks live relays globally and per user.
type relayCounter struct {
	mu    sync.Mutex
	total int
	byUsr map[string]int
}

// acquire takes a slot for user, or reports which cap refused it.
func (c *relayCounter) acquire(user string) (ok bool, why string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byUsr == nil {
		c.byUsr = map[string]int{}
	}
	if c.total >= browserRelayMax {
		return false, "server-wide"
	}
	if c.byUsr[user] >= browserRelayPerUser {
		return false, "per-user"
	}
	c.total++
	c.byUsr[user]++
	return true, ""
}

func (c *relayCounter) release(user string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total--
	if c.byUsr[user] <= 1 {
		delete(c.byUsr, user)
	} else {
		c.byUsr[user]--
	}
}

// browserHTTPClient talks to the guest's DevTools HTTP server and dials its
// WebSocket. It follows NO redirect: what answers on the guest's port is the
// project user's to control, and this server sits on every project bridge —
// a followed redirect would let one project steer a connection into another
// (R12; security review 2026-09-11).
var browserHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// GET /ws/browser/{project} — the CDP relay, owner only (authGuard resolves
// the identity; getProjectFor is the ownership check, never previewAccess).
//
// Order matters: everything that may fail as plain HTTP fails BEFORE the
// upgrade, because a browser follows no redirect and reads no body on a
// WebSocket handshake — a failed handshake is what the pane retries. Once
// the socket is accepted, every close the relay sends is a config.Close*
// code or a standard 1000/1009/1011.
func (s *Server) browserSocket(w http.ResponseWriter, r *http.Request) {
	if !terminal.SameOriginOrAbsent(r) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("cross-origin request refused"))
		return
	}
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if ok, why := s.browserRelays.acquire(u.Username); !ok {
		s.logger.Warn("browser: relay cap reached", "user", u.Username, "project", p.ID, "cap", why)
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("too many browser connections"))
		return
	}
	defer s.browserRelays.release(u.Username)
	host, fresh, err := s.browserUpstream(r.Context(), u.Username, p.ID)
	if err != nil {
		s.logger.Warn("browser: upstream lookup failed", "project", p.ID, "error", err)
		writeErr(w, http.StatusBadGateway, fmt.Errorf("browser is not available right now"))
		return
	}
	if fresh {
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("attaching to the project network — retry"))
		return
	}
	closeWith := func(code int, reason string) {
		if conn, err := websocket.Accept(w, r, nil); err == nil {
			conn.Close(websocket.StatusCode(code), reason)
		}
	}
	if host == "" {
		closeWith(config.CloseProjectStopped, "project stopped")
		return
	}
	// ONE exec: the launcher's start is idempotent and its exit code is the
	// answer; the status verb the REST start adds would be a second exec (and
	// a second inspect) on every relay for nothing the dial below does not
	// prove (review finding, 2026-09-13).
	code, out, err := s.browserExec(r.Context(), u.Username, p.ID, sp.profile.GuestUser(), "start")
	if err != nil {
		if errors.Is(err, errProjectNotRunning) {
			closeWith(config.CloseProjectStopped, "project stopped")
			return
		}
		s.logger.Warn("browser start failed", "project", p.ID, "error", err)
		closeWith(config.CloseBrowserUnavailable, "browser is not available right now")
		return
	}
	if code != 0 {
		s.logger.Warn("browser did not start", "project", p.ID, "exit", code, "tail", tail(out, 512))
		// The close frame carries the END of the launcher's output — its log
		// tail is the diagnostic; the pane's own heading says the rest, and
		// Retry fetches the full 2 KiB through the REST start.
		closeWith(config.CloseBrowserUnavailable, closeReason(tail(out, 2048)))
		return
	}
	wsURL, err := browserEndpoint(r.Context(), host)
	if err != nil {
		s.logger.Warn("browser: endpoint discovery failed", "project", p.ID, "error", err)
		closeWith(config.CloseBrowserUnavailable, "browser endpoint unreachable")
		return
	}
	// The upstream dial carries NO Origin header — that is what admits it:
	// Chromium's DevTools server refuses any Origin (the launcher sets no
	// --remote-allow-origins), so a page inside the browser can never open
	// this socket. Host = the guest IP, an IP literal Chromium accepts.
	dialCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	upstream, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{HTTPClient: browserHTTPClient})
	cancel()
	if err != nil {
		s.logger.Warn("browser: upstream dial failed", "project", p.ID, "error", err)
		closeWith(config.CloseBrowserUnavailable, "browser endpoint unreachable")
		return
	}
	upstream.SetReadLimit(browserRelayReadLimit)
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		upstream.Close(websocket.StatusNormalClosure, "")
		return
	}
	conn.SetReadLimit(browserPaneReadLimit)
	// The owner is re-checked while the relay lives, exactly as the terminal
	// bridge re-checks its session (nil in AuthMode=none: the identity is in
	// no store). A relay is a DevTools client over the project's browser,
	// which is nothing a revoked user should keep holding.
	var check func() bool
	if s.stillAuthorized != nil {
		email := u.Email
		check = func() bool { return s.stillAuthorized(email) }
	}
	relayWebSockets(r.Context(), conn, upstream, check, s.browserReauthEvery)
}

// relayBuffers hands the pumps their copy buffers: a screencast frame is
// streamed through 32 KiB at a time instead of being materialized whole
// (up to 16 MiB) on every hop.
var relayBuffers = sync.Pool{New: func() any { b := make([]byte, 32<<10); return &b }}

// relayWebSockets pumps messages both ways until either side ends. The pane
// closing is normal (the upstream is closed 1000); the guest side dropping
// (Chromium crashed, project stopped) closes the pane with 1011 so it shows
// "Browser disconnected" and offers Reconnect; a frame over the read limit
// closes it 1009; a revoked owner (stillAuthorized turning false, checked
// every reauthEvery; nil = never) closes it 4003.
//
// Each message is streamed: Reader → Writer of the same type → CopyBuffer →
// Close. The Close is what completes the message (FIN), so it happens ONLY
// after a clean copy — on a mid-message error the writer is abandoned and
// the relay ends, and no truncated frame is ever delivered as complete.
func relayWebSockets(ctx context.Context, pane, upstream *websocket.Conn, stillAuthorized func() bool, reauthEvery time.Duration) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pump := func(dst, src *websocket.Conn) error {
		buf := relayBuffers.Get().(*[]byte)
		defer relayBuffers.Put(buf)
		for {
			typ, r, err := src.Reader(ctx)
			if err != nil {
				return err
			}
			w, err := dst.Writer(ctx, typ)
			if err != nil {
				return err
			}
			if _, err := io.CopyBuffer(w, r, *buf); err != nil {
				return err
			}
			if err := w.Close(); err != nil {
				return err
			}
		}
	}
	// Both pumps are joined before this function returns — the caller
	// releases the relay slot on return, and a pump still copying a frame
	// into a closing socket would otherwise outlive its slot (audit
	// 2026-09-13).
	var pumps sync.WaitGroup
	defer pumps.Wait()
	paneErr := make(chan error, 1)
	upErr := make(chan error, 1)
	pumps.Add(2)
	go func() { defer pumps.Done(); paneErr <- pump(upstream, pane) }()
	go func() { defer pumps.Done(); upErr <- pump(pane, upstream) }()
	revoked := make(chan struct{}, 1)
	var revokedFlag atomic.Bool
	if stillAuthorized != nil {
		go terminal.RevocationWatch(ctx, reauthEvery, stillAuthorized, func() {
			revokedFlag.Store(true)
			revoked <- struct{}{}
		})
	}
	// A revocation that lands in the same instant as another arm (the select
	// picks a ready case at random) still reaches the pane as 4003, the one
	// code it must not mistake for a reconnectable drop (security review,
	// 2026-09-13).
	closePane := func(code websocket.StatusCode, reason string) {
		if revokedFlag.Load() {
			code, reason = websocket.StatusCode(config.CloseAccessRevoked), "access revoked"
		}
		pane.Close(code, reason)
	}
	// Heartbeat toward BOTH sides: a tab that went away without a close
	// frame (laptop lid, network drop) or a guest whose TCP half-died must
	// not hold the slot and the guest exec forever — r.Context() is no
	// longer cancelled once the socket is hijacked. Same shape as the
	// terminal bridge's ping loop; the timeout is generous because a pong
	// waits behind the frames already queued on a busy screencast.
	pingErr := make(chan error, 1)
	go func() {
		t := time.NewTicker(browserRelayPing)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// A fresh deadline per side: a slow-but-alive pane must not
				// spend the budget the guest's pong then fails.
				err := pingWithin(ctx, pane, browserRelayPing/2)
				if err == nil {
					err = pingWithin(ctx, upstream, browserRelayPing/2)
				}
				if err != nil {
					pingErr <- err
					return
				}
			}
		}
	}()
	select {
	case <-revoked:
		upstream.Close(websocket.StatusNormalClosure, "")
		closePane(websocket.StatusCode(config.CloseAccessRevoked), "access revoked")
	case <-pingErr:
		upstream.Close(websocket.StatusNormalClosure, "")
		closePane(websocket.StatusGoingAway, "no heartbeat")
	case <-paneErr:
		upstream.Close(websocket.StatusNormalClosure, "")
		closePane(websocket.StatusNormalClosure, "")
	case err := <-upErr:
		// A limit breach is reported as a wrapped ErrMessageTooBig, not as a
		// close status (the library closes the UPSTREAM 1009 itself); the
		// old CloseStatus comparison could never match.
		if errors.Is(err, websocket.ErrMessageTooBig) {
			closePane(websocket.StatusMessageTooBig, "frame too large")
		} else {
			closePane(websocket.StatusInternalError, "browser disconnected")
		}
		upstream.Close(websocket.StatusNormalClosure, "")
	case <-ctx.Done():
		upstream.Close(websocket.StatusNormalClosure, "")
		closePane(websocket.StatusNormalClosure, "")
	}
}

func pingWithin(ctx context.Context, c *websocket.Conn, d time.Duration) error {
	pctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return c.Ping(pctx)
}

// browserEndpoint asks the guest's DevTools HTTP server for its
// browser-level WebSocket URL and rewrites the host to the address we can
// actually reach (Chromium reports whatever Host it was asked with; the
// forwarder path keeps that our guest IP, but the rewrite costs nothing and
// never trusts the guest's answer for WHERE to connect).
func browserEndpoint(ctx context.Context, host string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+host+"/json/version", nil)
	if err != nil {
		return "", err
	}
	// Connection: close — a kept-alive discovery connection would park one of
	// the guest forwarder's bounded children (socat max-children) for the
	// idle timeout, doing nothing. The WebSocket dial below is hijacked and
	// never pooled, so it needs no such flag.
	req.Close = true
	resp, err := browserHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/json/version: %s", resp.Status)
	}
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&v); err != nil {
		return "", fmt.Errorf("/json/version: %w", err)
	}
	wu, err := url.Parse(v.WebSocketDebuggerURL)
	if err != nil || !strings.HasPrefix(wu.Path, "/devtools/browser/") {
		return "", fmt.Errorf("/json/version: no browser endpoint in reply")
	}
	return "ws://" + host + wu.Path, nil
}

// closeReason fits a message into a close frame's 123-byte reason budget,
// keeping its END: the launcher's output ends with the log lines that say
// why, and the head ("chromium did not answer on :9229 within 15 s; last log
// lines:") is the part the pane can do without. The cut lands on a rune
// boundary — a split multi-byte sequence would make the close frame invalid
// UTF-8 and the browser would drop the reason.
func closeReason(msg string) string {
	const max = 120
	// The launcher's log is guest-controlled bytes; a close frame's reason
	// must be valid UTF-8 or the browser reports 1006 instead of 4004.
	msg = strings.ToValidUTF8(msg, "")
	if len(msg) <= max {
		return msg
	}
	const mark = "…" // 3 bytes
	cut := len(msg) - (max - len(mark))
	for cut < len(msg) && !utf8RuneStart(msg[cut]) {
		cut++
	}
	return mark + msg[cut:]
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
