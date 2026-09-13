package terminal

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// The Origin check runs before anything else: a cross-origin GET on either
// WebSocket entry point is refused with 403 and reaches no store, no record
// and no orchestrator — the bridge here has none of them (nil), so reaching
// them would panic (audit 2026-09-13: both handlers booted a microVM before
// websocket.Accept's own check ran).
func TestWebSocketEntryPointsRefuseCrossOriginBeforeAnySideEffect(t *testing.T) {
	b := NewBridge(nil, &config.Config{}, slog.New(slog.DiscardHandler), nil, nil, nil, nil)
	for _, c := range []struct {
		name string
		h    http.HandlerFunc
	}{{"terminal", b.ServeHTTP}, {"setup", b.ServeSetup}} {
		r := httptest.NewRequest(http.MethodGet, "/ws/x", nil)
		r.Host = "crew.example.test"
		r.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		c.h(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: cross-origin = %d, want 403", c.name, w.Code)
		}
	}
}

func TestSameOriginOrAbsent(t *testing.T) {
	mk := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/ws/x", nil)
		r.Host = "crew.example.test"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	for origin, want := range map[string]bool{
		"":                            true,  // a non-browser client
		"https://crew.example.test":   true,  // same host
		"https://CREW.example.test":   true,  // hosts are case-insensitive
		"https://evil.example":        false, // cross-site
		"https://crew.example.test.x": false, // a longer host is another host
		"null":                        false, // a sandboxed page
	} {
		if got := SameOriginOrAbsent(mk(origin)); got != want {
			t.Errorf("origin %q: %v, want %v", origin, got, want)
		}
	}
}
