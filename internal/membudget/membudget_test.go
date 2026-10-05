package membudget

import (
	"context"
	"errors"
	"testing"
	"time"
)

func short(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

func TestAcquireAdmitsWithinBudgetAndReleases(t *testing.T) {
	b := New(100, 60)
	rel, err := b.Acquire(context.Background(), "a", 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Acquire(short(t), "a", 20); !errors.Is(err, ErrBusy) {
		t.Fatalf("tenant share exceeded: want ErrBusy, got %v", err)
	}
	rel()
	rel() // idempotent: a second call must not free bytes twice
	if b.used != 0 || len(b.byTenant) != 0 {
		t.Fatalf("after release used=%d byTenant=%v", b.used, b.byTenant)
	}
}

func TestATenantAtItsShareDoesNotBlockAnother(t *testing.T) {
	b := New(100, 50)
	if _, err := b.Acquire(context.Background(), "a", 50); err != nil {
		t.Fatal(err)
	}
	// a waits behind its own share...
	done := make(chan error, 1)
	go func() { _, err := b.Acquire(short(t), "a", 10); done <- err }()
	time.Sleep(10 * time.Millisecond)
	// ...while b is admitted at once.
	if _, err := b.Acquire(short(t), "b", 50); err != nil {
		t.Fatalf("second tenant refused: %v", err)
	}
	if err := <-done; !errors.Is(err, ErrBusy) {
		t.Fatalf("first tenant over its share: want ErrBusy, got %v", err)
	}
}

func TestTheTotalBoundsAllTenants(t *testing.T) {
	b := New(100, 60)
	if _, err := b.Acquire(context.Background(), "a", 60); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Acquire(short(t), "b", 60); !errors.Is(err, ErrBusy) {
		t.Fatalf("total exceeded: want ErrBusy, got %v", err)
	}
	if _, err := b.Acquire(short(t), "b", 40); err != nil {
		t.Fatalf("fits the remaining total: %v", err)
	}
}

func TestAReleaseAdmitsAWaiter(t *testing.T) {
	b := New(100, 100)
	rel, _ := b.Acquire(context.Background(), "a", 80)
	got := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		r, err := b.Acquire(ctx, "b", 50)
		if r != nil {
			r()
		}
		got <- err
	}()
	time.Sleep(20 * time.Millisecond)
	rel()
	if err := <-got; err != nil {
		t.Fatalf("waiter not admitted after release: %v", err)
	}
}

func TestALargeRequestIsClampedToTheShare(t *testing.T) {
	b := New(100, 40)
	rel, err := b.Acquire(short(t), "a", 1000)
	if err != nil {
		t.Fatalf("an oversize request must be admitted alone: %v", err)
	}
	if b.byTenant["a"] != 40 {
		t.Fatalf("reserved %d, want the 40 share", b.byTenant["a"])
	}
	rel()
}

func TestPerTenantFIFO(t *testing.T) {
	b := New(100, 100)
	_, _ = b.Acquire(context.Background(), "a", 50)
	relB, _ := b.Acquire(context.Background(), "a", 40)
	order := make(chan int, 2)
	go func() {
		if _, err := b.Acquire(context.Background(), "a", 50); err == nil { // big, first
			order <- 1
		}
	}()
	time.Sleep(10 * time.Millisecond)
	go func() {
		if _, err := b.Acquire(context.Background(), "a", 5); err == nil { // small, later: fits now but must not overtake
			order <- 2
		}
	}()
	time.Sleep(20 * time.Millisecond)
	select {
	case n := <-order:
		t.Fatalf("waiter %d admitted before any release", n)
	default:
	}
	relB() // 50 used: the first (50) fits, the second then does not
	if first := <-order; first != 1 {
		t.Fatalf("first admitted = %d, want 1", first)
	}
	time.Sleep(20 * time.Millisecond)
	select {
	case n := <-order:
		t.Fatalf("waiter %d admitted past the total", n)
	default:
	}
}

func TestACancelledWaiterUnblocksLaterOnes(t *testing.T) {
	b := New(100, 100)
	rel, _ := b.Acquire(context.Background(), "a", 60)
	defer rel()
	// The big waiter times out; the small one behind it (same tenant) must
	// then be admitted, not stranded.
	go func() { _, _ = b.Acquire(short(t), "a", 50) }()
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := b.Acquire(ctx, "a", 10)
	if err != nil {
		t.Fatalf("later waiter stranded: %v", err)
	}
	r()
}
