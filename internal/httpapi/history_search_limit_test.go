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

// One user holds at most one slot: a second search of theirs while one is
// in flight is 429, so a single tenant cannot take every slot and lock the
// others out (review of v1.42.1..ef22f5c, low).
func TestHistorySearchIsOnePerUser(t *testing.T) {
	n := newNetHarness(t)
	path := "/api/projects/" + n.pid + "/history/search?q=needle"
	user := identity.DeriveUsername(n.admin)
	n.s.searchMu.Lock()
	n.s.searchBusy[user] = true // a search of theirs in flight
	n.s.searchMu.Unlock()
	w := doAs(t, n.h, n.admin, http.MethodGet, path, "")
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "yours") {
		t.Fatalf("a second search by the same user: %d %s, want 429", w.Code, w.Body.String())
	}
	if len(n.s.searchSlots) != 0 {
		t.Fatalf("a refused search took a slot: %d in use", len(n.s.searchSlots))
	}
	n.s.searchMu.Lock()
	delete(n.s.searchBusy, user)
	n.s.searchMu.Unlock()
	if w := doAs(t, n.h, n.admin, http.MethodGet, path, ""); w.Code != http.StatusOK {
		t.Fatalf("after theirs finished: %d %s", w.Code, w.Body.String())
	}
	n.s.searchMu.Lock()
	busy := n.s.searchBusy[user]
	n.s.searchMu.Unlock()
	if busy {
		t.Fatal("a finished search left its user marked busy")
	}
}
