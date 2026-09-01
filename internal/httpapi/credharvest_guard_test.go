package httpapi

import "testing"

// tryAcquire single-flights the credential push (its remaining caller after
// the backup feature was removed 2026-09-01). The compare-and-set semantics
// matter: a plain store let two concurrent operations both pass every check
// (audit 2026-08-30), so the guard must hold until release and be per-key.
func TestTryAcquireSingleFlights(t *testing.T) {
	s := &Server{credPushing: map[string]bool{}}

	rel, ok := s.tryAcquire(s.credPushing, "alice", "-")
	if !ok {
		t.Fatal("first acquire must succeed")
	}
	if _, ok := s.tryAcquire(s.credPushing, "alice", "-"); ok {
		t.Fatal("second acquire for the same key must be refused while held")
	}
	if _, ok := s.tryAcquire(s.credPushing, "bob", "-"); !ok {
		t.Fatal("a different key must not be blocked")
	}
	rel()
	if _, ok := s.tryAcquire(s.credPushing, "alice", "-"); !ok {
		t.Fatal("release must make the key acquirable again")
	}
}
