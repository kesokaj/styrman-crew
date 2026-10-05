package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

// A history search holds several MiB per transcript while it scans. Under
// the server's 4g memory cap (ADR-085), hundreds of searches at once
// against guest-planted 4 MiB transcripts would OOM-kill the server and drop
// every tenant's sessions (security review of v1.42.1, finding 1). Searches
// beyond searchConcurrency are refused with 429 instead of queued.
func TestHistorySearchIsLimitedServerWide(t *testing.T) {
	n := newNetHarness(t)
	logs := filepath.Join(n.state, "homes", identity.DeriveUsername(n.admin), "p", n.pid, ".styrman", "logs", n.pid)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "conv-1.log"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/" + n.pid + "/history/search?q=needle"

	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusOK {
		t.Fatalf("an idle server refused a search: %d %s", w.Code, w.Body.String())
	}
	// Every slot taken by searches in flight.
	for range cap(n.s.searchSlots) {
		n.s.searchSlots <- struct{}{}
	}
	w := doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "search") {
		t.Fatalf("a search past the limit: %d %s, want 429", w.Code, w.Body.String())
	}
	// A slot frees up: searches work again.
	<-n.s.searchSlots
	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusOK {
		t.Fatalf("after a slot freed: %d %s", w.Code, w.Body.String())
	}
	if len(n.s.searchSlots) != cap(n.s.searchSlots)-1 {
		t.Fatalf("a finished search did not give its slot back: %d of %d in use", len(n.s.searchSlots), cap(n.s.searchSlots))
	}
}

// Reading a transcript holds up to 4 MiB, about 30 MiB once JSON-escaped.
// Like search, reads are bounded server-wide: past readConcurrency in flight
// they answer 429 (security review of v1.42.1..ef22f5c, finding 2).
func TestHistoryReadIsLimitedServerWide(t *testing.T) {
	n := newNetHarness(t)
	logs := filepath.Join(n.state, "homes", identity.DeriveUsername(n.admin), "p", n.pid, ".styrman", "logs", n.pid)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "conv-1.log"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/" + n.pid + "/history/conv-1"
	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusOK {
		t.Fatalf("an idle server refused a read: %d %s", w.Code, w.Body.String())
	}
	for range cap(n.s.readSlots) {
		n.s.readSlots <- struct{}{}
	}
	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("a read past the limit: %d, want 429", w.Code)
	}
	<-n.s.readSlots
	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusOK {
		t.Fatalf("after a slot freed: %d", w.Code)
	}
	if len(n.s.readSlots) != cap(n.s.readSlots)-1 {
		t.Fatalf("a finished read kept its slot: %d of %d", len(n.s.readSlots), cap(n.s.readSlots))
	}
}
