package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/membudget"
)

// The server-wide memory budget (ADR-086). Every API request reserves its
// route's worst-case response and body memory before it runs; past the
// budget it waits up to budgetWait, then gets 503. The numbers keep the
// budgeted memory well inside GOMEMLIMIT=3GiB under the 4g cap (ADR-085).
// With total = 4 x perTenant, a tenant at its share never holds up another
// while at most three tenants are at theirs (plan D16, accepted).
const (
	budgetTotal     = 1 << 30
	budgetPerTenant = 256 << 20
)

// budgetWait is how long a request waits for the budget before its 503; a
// var so a test can shorten it.
var budgetWait = 2 * time.Second

const mib = 1 << 20

// weightFn prices one request in bytes; 0 means the route is not charged.
type weightFn func(r *http.Request) int64

func fixed(n int64) weightFn { return func(*http.Request) int64 { return n } }

// editorBody: the 8 MiB editor body decoded into a string, then re-encoded.
func editorBody(r *http.Request) int64 { return 3*body(r, 8<<20) + 2*mib }

// historyRead: the one size known up front — ?bytes clamped as readHistory
// clamps it, history.Read's 256 KiB default when absent — times the raw
// read, its stripped copy and up to 6x JSON escaping, plus slack.
func historyRead(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
	if n <= 0 {
		n = 256 << 10
	}
	n = min(n, 4<<20)
	return 9*n + mib
}

const (
	// execWeight: one guest exec's combined output is capped at
	// maxExecOutput (2 MiB, stdout and stderr share the buffer,
	// orchestrator/exec.go), held raw and up to 6x JSON-escaped.
	execWeight = 14 * mib
	// defaultWeight is the response share of an unlisted route: small JSON.
	defaultWeight = 2 * mib
)

// routeWeight prices the routes whose memory is not small JSON, keyed by the
// mux pattern. A pattern not listed costs defaultWeight + body(r). Weight 0:
// streamed or WebSocket traffic, which holds no per-request buffer (a
// reservation held for a WebSocket's life would starve the budget).
var routeWeight = map[string]weightFn{
	"GET /ws/term/{project}/{term}": fixed(0),
	"GET /ws/setup":                 fixed(0),
	"GET /ws/browser/{project}":     fixed(0),
	"GET /":                         fixed(0),
	"GET /api/health":               fixed(0),
	"GET /api/version":              fixed(0),

	"GET /api/projects/{project}/download":      fixed(0),
	"GET /api/projects/{project}/home/download": fixed(0),

	"GET /api/projects/{project}/file":      fixed(14 * mib), // files.MaxReadBytes 2 MiB x (1 + 6 escaped)
	"GET /api/projects/{project}/home/file": fixed(14 * mib),
	"PUT /api/projects/{project}/file":      editorBody,
	"PUT /api/projects/{project}/home/file": editorBody,
	"POST /api/projects/{project}/format":   editorBody,
	"POST /api/projects/{project}/check":    editorBody,

	// Multipart is streamed to disk: parts buffers only.
	"POST /api/projects/{project}/upload":      fixed(4 * mib),
	"POST /api/projects/{project}/home/upload": fixed(4 * mib),

	// files.List bounds a listing at 5000 entries and maxListBytes of names
	// and paths: 2 MiB x (1 + 6 escaped).
	"GET /api/projects/{project}/files":      fixed(14 * mib),
	"GET /api/projects/{project}/home/files": fixed(14 * mib),

	// <= 5000 transcripts, guest-chosen names <= 255 B plus metadata.
	"GET /api/projects/{project}/history":      fixed(16 * mib),
	"GET /api/projects/{project}/history/{id}": historyRead,
	// searchOne holds a 4 MiB tail, its string, a stripped copy, two
	// ReplaceAll copies and a lowered copy up to 3x on invalid UTF-8.
	"GET /api/projects/{project}/history/search": fixed(32 * mib),

	"GET /api/projects/{project}/logs": fixed(4 * mib), // Logs keeps a 256 KiB tail
	// credstore caps a file credential at 256 KiB.
	"GET /api/settings/credentials/{kind}/{id}/content": fixed(4 * mib),

	"GET /api/git/summary": fixed(4 * execWeight), // 4 concurrent execs (git.go)

	"GET /api/projects/{project}/git/status":    fixed(execWeight),
	"GET /api/projects/{project}/git/diff":      fixed(execWeight),
	"POST /api/projects/{project}/git/stage":    fixed(execWeight),
	"POST /api/projects/{project}/git/unstage":  fixed(execWeight),
	"POST /api/projects/{project}/git/discard":  fixed(execWeight),
	"POST /api/projects/{project}/git/commit":   fixed(execWeight),
	"POST /api/projects/{project}/git/push":     fixed(execWeight),
	"GET /api/projects/{project}/git/branches":  fixed(execWeight),
	"GET /api/projects/{project}/git/log":       fixed(execWeight),
	"POST /api/projects/{project}/git/fetch":    fixed(execWeight),
	"POST /api/projects/{project}/git/pull":     fixed(execWeight),
	"POST /api/projects/{project}/git/checkout": fixed(execWeight),
	"POST /api/projects/{project}/git/stash":    fixed(execWeight),
}

// uncharged reports the path prefixes that only tunnel bytes to a guest app
// (registered for every method): never charged, never given a body deadline.
func uncharged(p string) bool {
	return isPreviewPath(p) || isLinkPath(p) || isSharedPath(p)
}

// bodyCap is the request-body cap for a matched mux PATTERN: budgetGuard
// applies it, and prices a body of unknown length at it. Uploads carry real
// payloads; editor saves, /format and /check carry file content (2 MiB read
// cap x JSON escaping); everything else, an unmatched request included, is
// small JSON. Keyed on the pattern, never the request path: a path is
// decoded, so ".../terminals/x%2Fupload" ends in "/upload" while it routes to
// {term} (security review of v1.42.3..5a8afa1, finding 1).
func bodyCap(pattern string) int64 {
	switch {
	case strings.HasSuffix(pattern, "/upload"):
		return 512 << 20
	case strings.HasSuffix(pattern, "/file"), strings.HasSuffix(pattern, "/format"), strings.HasSuffix(pattern, "/check"):
		return 8 << 20
	}
	return 1 << 20
}

// hasBody reports whether a request carries a body the handler may read.
func hasBody(r *http.Request) bool {
	return r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 &&
		r.Method != http.MethodGet && r.Method != http.MethodHead
}

// body is the request body's size for pricing: the declared length, never
// more than the cap c budgetGuard enforces (a forged Content-Length cannot
// buy a bigger reservation than the body that can actually arrive).
func body(r *http.Request, c int64) int64 {
	if !hasBody(r) {
		return 0
	}
	if r.ContentLength >= 0 {
		return min(r.ContentLength, c)
	}
	return c
}

// bodyDeadline gives a request that carries a body slotReadTimeout to send
// it, so a trickling client cannot hold a reservation or a slot (plan D18).
// The deadline ends with the body: net/http clears it when the body hits
// EOF, before its background read starts (go1.25 net/http/server.go
// startBackgroundRead), so it never cancels the request's context
// mid-handler — pinned by TestTheBodyDeadlineEndsWithTheBody (plan D21).
// The production writer supports it; httptest's recorder does not, and
// there the request simply has no deadline.
func bodyDeadline(w http.ResponseWriter, r *http.Request) {
	if hasBody(r) {
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(slotReadTimeout))
	}
}

// bodyTimedOut reports whether a body read failed on the read deadline,
// which the handlers answer with 408 instead of 400.
func bodyTimedOut(err error) bool { return errors.Is(err, os.ErrDeadlineExceeded) }

// tenantOf is the request's tenant for the budget and the slot shares; a
// request with no user (a preview-only visitor) shares the "-" tenant.
func tenantOf(r *http.Request) string {
	if u := identity.FromContext(r.Context()).Username; u != "" {
		return u
	}
	return "-"
}

// budgetGuard caps every routed request's body by its matched route
// (bodyCap) and charges the request its weight against s.budget. It sits
// inside authGuard (the tenant is known) and csrfGuard, and around the mux,
// so no route can miss it.
func (s *Server) budgetGuard(mux *http.ServeMux) http.Handler {
	var lastWarn atomic.Int64
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uncharged(path.Clean(r.URL.Path)) {
			mux.ServeHTTP(w, r)
			return
		}
		_, pattern := mux.Handler(r)
		limit := bodyCap(pattern)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		n := defaultWeight + body(r, limit)
		if f, ok := routeWeight[pattern]; ok {
			n = f(r)
		}
		if n == 0 {
			mux.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), budgetWait)
		release, err := s.budget.Acquire(ctx, tenantOf(r), n)
		cancel()
		if err != nil {
			if now := time.Now().Unix(); now-lastWarn.Load() >= 10 {
				lastWarn.Store(now)
				slog.Warn("memory budget full", "tenant", tenantOf(r), "pattern", pattern, "weight", n)
			}
			w.Header().Set("Retry-After", "1")
			writeErr(w, http.StatusServiceUnavailable, membudget.ErrBusy)
			return
		}
		defer release()
		if !strings.HasSuffix(pattern, "/upload") {
			bodyDeadline(w, r)
		}
		mux.ServeHTTP(w, r)
	})
}

// slotPool bounds one kind of heavy request server-wide, and each tenant to
// half of it (plan D7), so one tenant cannot take every slot.
type slotPool struct {
	ch        chan struct{}
	perTenant int
	mu        sync.Mutex
	held      map[string]int
}

func newSlotPool(n int) *slotPool {
	return &slotPool{ch: make(chan struct{}, n), perTenant: max(1, n/2), held: map[string]int{}}
}

// acquireSlot takes one slot of p for the request's tenant without waiting,
// or answers 429 and returns nil. It also bounds how long the response may
// take to be written (a client that never reads would otherwise hold the
// slot and the response's memory) and, for a request with a body, how long
// the body may take to arrive.
func (s *Server) acquireSlot(w http.ResponseWriter, r *http.Request, p *slotPool, msg string) func() {
	t := tenantOf(r)
	p.mu.Lock()
	if p.held[t] >= p.perTenant {
		p.mu.Unlock()
		writeErr(w, http.StatusTooManyRequests,
			fmt.Errorf("you already have %d of these running — try again when one finishes", p.perTenant))
		return nil
	}
	select {
	case p.ch <- struct{}{}:
		p.held[t]++
		p.mu.Unlock()
	default:
		p.mu.Unlock()
		writeErr(w, http.StatusTooManyRequests, errors.New(msg))
		return nil
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(historyWriteTimeout))
	bodyDeadline(w, r)
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			if p.held[t]--; p.held[t] <= 0 {
				delete(p.held, t)
			}
			p.mu.Unlock()
			<-p.ch
		})
	}
}
