// Package membudget is the server's memory budget (ADR-086): a weighted
// semaphore over bytes, with a per-tenant share, so that no tenant's
// requests — and not all tenants' together — can build more response or
// request-body memory than the server's 4g cap (ADR-085) can hold.
//
// A request reserves its worst case before it runs and releases it when it
// is done. When the reservation does not fit it waits, in order behind its
// own tenant's earlier waiters, until it fits or its context ends.
package membudget

import (
	"context"
	"errors"
	"sync"
)

// ErrBusy is returned when a reservation did not fit before its context
// ended.
var ErrBusy = errors.New("server busy, try again")

// Budget is a byte budget with a per-tenant share. The zero value is not
// usable; build one with New.
type Budget struct {
	total, perTenant int64

	mu       sync.Mutex
	used     int64
	byTenant map[string]int64
	waiters  []*waiter
}

type waiter struct {
	tenant  string
	n       int64
	ready   chan struct{} // closed when granted
	granted bool          // set under mu with ready's close
}

// New returns a budget of total bytes, of which one tenant may hold at most
// perTenant at a time.
func New(total, perTenant int64) *Budget {
	return &Budget{total: total, perTenant: min(perTenant, total), byTenant: map[string]int64{}}
}

// Acquire reserves n bytes for tenant, waiting until ctx is done. n is
// clamped to the tenant share, so a request larger than a tenant's share is
// admitted alone rather than refused forever. FIFO holds per tenant: a
// waiter is never overtaken by a later request of the same tenant; across
// tenants, a waiter is admitted as soon as both the total and its own share
// fit, so a tenant at its share never holds up another. The returned release
// is idempotent.
func (b *Budget) Acquire(ctx context.Context, tenant string, n int64) (func(), error) {
	n = max(min(n, b.perTenant), 0)
	b.mu.Lock()
	if !b.tenantWaiting(tenant) && b.fits(tenant, n) {
		b.take(tenant, n)
		b.mu.Unlock()
		return b.releaser(tenant, n), nil
	}
	w := &waiter{tenant: tenant, n: n, ready: make(chan struct{})}
	b.waiters = append(b.waiters, w)
	b.mu.Unlock()

	select {
	case <-w.ready:
		return b.releaser(tenant, n), nil
	case <-ctx.Done():
	}
	b.mu.Lock()
	if w.granted { // granted while ctx ended: keep it, the caller may run
		b.mu.Unlock()
		return b.releaser(tenant, n), nil
	}
	b.remove(w)
	b.dispatch() // a later waiter of this tenant may fit now
	b.mu.Unlock()
	return nil, ErrBusy
}

func (b *Budget) releaser(tenant string, n int64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.used -= n
			if b.byTenant[tenant] -= n; b.byTenant[tenant] <= 0 {
				delete(b.byTenant, tenant)
			}
			b.dispatch()
			b.mu.Unlock()
		})
	}
}

func (b *Budget) fits(tenant string, n int64) bool {
	return b.used+n <= b.total && b.byTenant[tenant]+n <= b.perTenant
}

func (b *Budget) take(tenant string, n int64) {
	b.used += n
	b.byTenant[tenant] += n
}

func (b *Budget) tenantWaiting(tenant string) bool {
	for _, w := range b.waiters {
		if w.tenant == tenant {
			return true
		}
	}
	return false
}

func (b *Budget) remove(w *waiter) {
	for i, x := range b.waiters {
		if x == w {
			b.waiters = append(b.waiters[:i], b.waiters[i+1:]...)
			return
		}
	}
}

// dispatch grants every waiter that fits, in queue order, skipping a tenant
// whose earlier waiter is still blocked (per-tenant FIFO). Caller holds mu.
func (b *Budget) dispatch() {
	blocked := map[string]bool{}
	kept := b.waiters[:0]
	for _, w := range b.waiters {
		if !blocked[w.tenant] && b.fits(w.tenant, w.n) {
			b.take(w.tenant, w.n)
			w.granted = true
			close(w.ready)
			continue
		}
		blocked[w.tenant] = true
		kept = append(kept, w)
	}
	clear(b.waiters[len(kept):])
	b.waiters = kept
}
