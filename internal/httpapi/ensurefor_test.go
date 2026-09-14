package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/project"
)

// ensureFor is the terminal bridge's boot callback: it resolves the record
// the orchestrator needs and only then boots (and, since BUG-060, joins the
// project's bridge). Its two guards run BEFORE any Docker call, which is
// what these tests pin — the orchestrator here has no daemon to talk to,
// so reaching it would surface as a dial error, not the guard's message.

// An unknown project id is refused with the store's not-found error: a
// stale reconnect must never boot (or create) anything for a record that
// no longer exists.
func TestEnsureForRefusesAnUnknownProjectBeforeBooting(t *testing.T) {
	s := testServer(t)
	_, _, _, err := s.ensureFor(context.Background(), "tester", "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("err = %v, want project.ErrNotFound", err)
	}
}

// A project whose background destroy is in flight is refused by name: a
// terminal WS auto-reconnecting while its container is force-removed
// would otherwise resurrect the microVM as an orphan with no record.
func TestEnsureForRefusesAProjectBeingDeleted(t *testing.T) {
	s := testServer(t)
	sp, err := s.space("tester")
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Add(project.Project{Name: "doomed", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	s.deletingMu.Lock()
	s.deleting["tester/"+p.ID] = true
	s.deletingMu.Unlock()
	_, _, _, err = s.ensureFor(context.Background(), "tester", p.ID)
	if err == nil || !strings.Contains(err.Error(), "being deleted") {
		t.Fatalf("err = %v, want the delete-in-flight refusal", err)
	}
}
