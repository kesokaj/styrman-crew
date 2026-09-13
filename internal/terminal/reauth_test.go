package terminal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// RevocationWatch is the whole re-authorization decision: it fires once when
// the check turns false, never before, and never at all for a nil check
// (AuthMode=none) — TODO close-out 2026-09-10, item I.
func TestRevocationWatchFiresOnceWhenAuthorizationIsLost(t *testing.T) {
	var calls atomic.Int32
	var fired atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		RevocationWatch(ctx, 5*time.Millisecond, func() bool { return calls.Add(1) < 3 }, func() { fired.Add(1) })
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watch did not return after the check turned false")
	}
	if fired.Load() != 1 {
		t.Fatalf("onRevoked fired %d times, want 1", fired.Load())
	}
	if calls.Load() != 3 {
		t.Fatalf("check was consulted %d times, want 3 (two true, then false)", calls.Load())
	}
}

func TestRevocationWatchNeverFiresWhileAuthorized(t *testing.T) {
	var fired atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	RevocationWatch(ctx, 5*time.Millisecond, func() bool { return true }, func() { fired.Add(1) })
	if fired.Load() != 0 {
		t.Fatalf("onRevoked fired %d times for a user who stayed authorized", fired.Load())
	}
}

// AuthMode=none: the identity is minted, not stored, so there is nothing to
// re-check — a nil check must return at once and never close a terminal.
func TestRevocationWatchIgnoresANilCheck(t *testing.T) {
	var fired atomic.Int32
	start := time.Now()
	RevocationWatch(context.Background(), time.Millisecond, nil, func() { fired.Add(1) })
	if fired.Load() != 0 || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("nil check: fired=%d elapsed=%s", fired.Load(), time.Since(start))
	}
}
