package orchestrator

import (
	"errors"
	"testing"
	"time"
)

// brokerOrch is seamOrch with a containerized server: the self-container
// and the broker listener are set, so EnsureSelfAttached has work to do.
func brokerOrch(t *testing.T) (*Orchestrator, *fakeDocker) {
	t.Helper()
	o, fd, cfg := seamOrch(t)
	cfg.SelfContainer = "styrman-server"
	cfg.BrokerAddr = ":8091"
	plant(fd, cfg, true, false)
	fd.networks[netName()].subnet = "172.16.1.0/24"
	fd.networks[netName()].gateway = "172.16.1.1"
	return o, fd
}

func netName() string { return "sdb-" + tProj }

// The broker's fixed address is broadcast−1 of the bridge's REAL subnet
// (BUG-054 was a /16 assumption in the guest; the server side computes it
// from the inspected mask), and the server asks Docker for exactly that
// address when it joins. Nothing else in the suite had ever asserted the
// join itself (BUG-060: three bugs in the family, no direct test).
func TestEnsureSelfAttachedBindsTheBrokerAddressOnTheBridge(t *testing.T) {
	o, fd := brokerOrch(t)
	if fresh := o.EnsureSelfAttached(tUser, tProj); !fresh {
		t.Fatal("first attach must report fresh")
	}
	want := "NetworkConnect(" + netName() + ",styrman-server,ip=172.16.1.254)"
	found := false
	for _, c := range fd.calls {
		if c == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("calls %v, want %s", fd.calls, want)
	}
}

// A fresh attach is memoized for a minute: the proxy calls this per
// request and the terminal bridge per connect, so the steady state must
// be Docker-API-free.
func TestEnsureSelfAttachedMemoizesAFreshAttach(t *testing.T) {
	o, fd := brokerOrch(t)
	o.EnsureSelfAttached(tUser, tProj)
	before := len(fd.calls)
	if fresh := o.EnsureSelfAttached(tUser, tProj); fresh {
		t.Fatal("second call within the TTL must not report fresh")
	}
	if len(fd.calls) != before {
		t.Fatalf("second call made Docker calls: %v", fd.calls[before:])
	}
}

// The static address can be taken (a stale endpoint, an operator's
// container): the attach retries dynamically so the port-preview proxy
// still works, and says so — the broker URL may then miss for that
// project, which is a Warn, not a refused terminal.
func TestEnsureSelfAttachedFallsBackToADynamicAddress(t *testing.T) {
	o, fd := brokerOrch(t)
	attempts := 0
	fd.onCall = func(method string) {
		if method != "NetworkConnect" {
			return
		}
		attempts++
		fd.mu.Lock()
		if attempts == 1 {
			fd.fail["NetworkConnect"] = errors.New("Address already in use")
		} else {
			delete(fd.fail, "NetworkConnect")
		}
		fd.mu.Unlock()
	}
	if fresh := o.EnsureSelfAttached(tUser, tProj); !fresh {
		t.Fatal("the dynamic retry succeeded, so the attach is fresh")
	}
	if attempts != 2 || fd.calls[len(fd.calls)-1] != "NetworkConnect("+netName()+",styrman-server)" {
		t.Fatalf("want a static attempt then a dynamic one; got %d attempts, calls %v", attempts, fd.calls)
	}
}

// "Already attached" is a verified state and is memoized; a GENUINE
// failure is not — the 2026-08-30 review found a stored memo made every
// call for the next minute report "attached" while the proxy dialed
// through a bridge the server never joined.
func TestEnsureSelfAttachedDoesNotMemoizeAFailure(t *testing.T) {
	o, fd := brokerOrch(t)
	fd.fail["NetworkConnect"] = errors.New("daemon unavailable")
	if fresh := o.EnsureSelfAttached(tUser, tProj); fresh {
		t.Fatal("a failed attach is not fresh")
	}
	if _, memo := o.selfAttached.Load(tProj); memo {
		t.Fatal("a failed attach must not be memoized")
	}
	delete(fd.fail, "NetworkConnect")
	fd.fail["NetworkConnect"] = errors.New("endpoint with name styrman-server already exists in network")
	if fresh := o.EnsureSelfAttached(tUser, tProj); fresh {
		t.Fatal("already-attached is not fresh")
	}
	if v, memo := o.selfAttached.Load(tProj); !memo || time.Since(v.(time.Time)) > time.Second {
		t.Fatal("already-attached is a verified state and must be memoized")
	}
}
