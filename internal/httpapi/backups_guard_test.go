package httpapi

import "testing"

// The in-flight guards must be compare-and-set: a plain store let two
// concurrent restores both proceed and destroy each other's rollback
// quarantine (2026-08-30 audit).
func TestTryAcquireIsCompareAndSet(t *testing.T) {
	s := &Server{restoring: map[string]bool{}, backingUp: map[string]bool{}}

	rel1, ok := s.tryAcquire(s.restoring, "alice", "p1")
	if !ok {
		t.Fatal("first acquire must succeed")
	}
	if _, ok := s.tryAcquire(s.restoring, "alice", "p1"); ok {
		t.Fatal("second acquire of a held slot must fail")
	}
	// Distinct project / distinct map are independent slots.
	if _, ok := s.tryAcquire(s.restoring, "alice", "p2"); !ok {
		t.Fatal("different project must not be blocked")
	}
	if _, ok := s.tryAcquire(s.backingUp, "alice", "p1"); !ok {
		t.Fatal("different map must not be blocked")
	}
	if !s.restoringState("alice", "p1") {
		t.Fatal("restoringState must reflect the held slot")
	}
	rel1()
	if s.restoringState("alice", "p1") {
		t.Fatal("release must clear the slot")
	}
	if _, ok := s.tryAcquire(s.restoring, "alice", "p1"); !ok {
		t.Fatal("reacquire after release must succeed")
	}
}
