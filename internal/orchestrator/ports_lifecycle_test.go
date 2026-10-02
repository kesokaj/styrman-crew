package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// BUG-088: a project being stopped is still reported RUNNING, with the same
// container id, until its PID 1 exits — and an exec the shutdown kills exits
// non-zero (9, observed on Kata during a mode toggle). BUG-023's re-check
// asks only "is it gone or replaced?", so that window answered the Ports
// panel's poll with a 500. Every lifecycle change holds the project's lock
// for its whole duration; after a failed exec, GuestGoingAway keeps asking
// Docker while that lock is held, answers as soon as the container is down or
// replaced, and asks once more when the lock comes free — within a ceiling
// (lifecycleWaitMax). Holding the lock is NOT itself the answer (review of
// v1.40.0..5571b57, finding 1): an attach's no-op EnsureRunning and the idle
// reaper's busy probe hold it too, and a real failure in that window must
// still be reported.

type portsResult struct {
	ports []Port
	err   error
}

// scanWhileLocked holds the project's lock, starts ListeningPorts against a
// running container whose exec exits 9, lets it reach the wait, runs change
// (the lifecycle change's effect, or nothing), and releases the lock.
func scanWhileLocked(t *testing.T, change func(fd *fakeDocker)) portsResult {
	t.Helper()
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9

	l := o.lockProject(key(tUser, tProj))
	l.Lock()
	done := make(chan portsResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		p, err := o.ListeningPorts(ctx, tUser, tProj)
		done <- portsResult{p, err}
	}()
	waitInTheLoop(t, fd)
	select {
	case r := <-done:
		l.Unlock()
		t.Fatalf("ListeningPorts answered while the lock was held: %#v", r)
	default:
	}
	change(fd)
	l.Unlock()
	select {
	case r := <-done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("ListeningPorts did not answer after the lock was released")
	}
	return portsResult{}
}

// waitInTheLoop returns once the scan's exec has failed and GuestGoingAway
// has asked Docker at least twice: the first look, and one from the wait —
// proof the scan is waiting, not merely scheduled (a fixed sleep could pass
// without the wait ever running, under load).
func waitInTheLoop(t *testing.T, fd *fakeDocker) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for fd.callsTo("ContainerExecInspect") < 1 || fd.callsTo("ContainerInspect") < 3 {
		if time.Now().After(deadline) {
			t.Fatal("the scan never reached GuestGoingAway's wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func setContainer(fd *fakeDocker, fn func(c *fakeContainer)) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fn(fd.containers[config.ContainerName(tProj)])
}

func TestListeningPortsFailureDuringAStopIsNoPorts(t *testing.T) {
	r := scanWhileLocked(t, func(fd *fakeDocker) {
		setContainer(fd, func(c *fakeContainer) { c.running = false })
	})
	if r.err != nil || r.ports == nil || len(r.ports) != 0 {
		t.Fatalf("a scan killed by a stop in flight must be no ports, got %#v %v", r.ports, r.err)
	}
}

func TestListeningPortsFailureDuringAModeToggleIsNoPorts(t *testing.T) {
	r := scanWhileLocked(t, func(fd *fakeDocker) {
		setContainer(fd, func(c *fakeContainer) { c.id = "id-recreated" })
	})
	if r.err != nil || r.ports == nil || len(r.ports) != 0 {
		t.Fatalf("a scan killed by a container replacement must be no ports, got %#v %v", r.ports, r.err)
	}
}

// The review's case: the lock was held by something that left the guest as
// it was. The failure is the command's and is reported.
func TestListeningPortsFailureUnderANoOpLockHolderIsAnError(t *testing.T) {
	r := scanWhileLocked(t, func(*fakeDocker) {})
	if r.err == nil || !strings.Contains(r.err.Error(), "port scan exited 9") {
		t.Fatalf("want the scan's failure reported, got %#v %v", r.ports, r.err)
	}
}

func TestListeningPortsFailureWithNothingInFlightIsAnError(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9

	_, err := o.ListeningPorts(context.Background(), tUser, tProj)
	if err == nil || !strings.Contains(err.Error(), "port scan exited 9") {
		t.Fatalf("want the scan's failure reported, got %v", err)
	}
	if l := o.lockProject(key(tUser, tProj)); !l.TryLock() {
		t.Fatal("the project's lock is held after ListeningPorts returned")
	} else {
		l.Unlock()
	}
}

// A request that gives up while the change is still in flight gets its
// answer at once (the failure), and the wait never takes the lock it could
// not get.
func TestListeningPortsWaitIsBoundedByTheRequest(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9

	l := o.lockProject(key(tUser, tProj))
	l.Lock()
	defer l.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 4*lifecycleRecheck)
	defer cancel()
	start := time.Now()
	_, err := o.ListeningPorts(ctx, tUser, tProj)
	if err == nil {
		t.Fatal("a request that gave up while the lock was held must get the failure")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("the wait outlived its request by far: %v", d)
	}
}

// A mode toggle holds the lock through stop, recreate and waitReady (up to
// 90 s), longer than the page waits for a poll. The wait looks at the guest
// on every tick, so a scan killed by the stop answers as soon as the
// container is down — not when the lock comes free (gate review of
// 5571b57..57179a4, finding 1).
func TestListeningPortsAnswersAsSoonAsTheGuestIsGone(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9

	l := o.lockProject(key(tUser, tProj))
	l.Lock()
	defer l.Unlock() // held for the whole test: the toggle is still running
	done := make(chan portsResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		p, err := o.ListeningPorts(ctx, tUser, tProj)
		done <- portsResult{p, err}
	}()
	waitInTheLoop(t, fd)
	setContainer(fd, func(c *fakeContainer) { c.running = false })
	select {
	case r := <-done:
		if r.err != nil || r.ports == nil || len(r.ports) != 0 {
			t.Fatalf("want no ports once the container is down, got %#v %v", r.ports, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the scan kept waiting on the lock after the container was already down")
	}
}

// The wait has a ceiling of its own (gate security review of
// 57179a4..624808d, finding 1): a request whose client never gives up must
// not park on a toggle's 90 s waitReady, asking Docker ten times a second.
// Past lifecycleWaitMax the failure is reported as it stands.
func TestListeningPortsWaitHasACeiling(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9
	saved := lifecycleWaitMax
	lifecycleWaitMax = 4 * lifecycleRecheck
	defer func() { lifecycleWaitMax = saved }()

	l := o.lockProject(key(tUser, tProj))
	l.Lock()
	defer l.Unlock() // never released while the scan waits
	done := make(chan error, 1)
	go func() {
		_, err := o.ListeningPorts(context.Background(), tUser, tProj) // a client that never gives up
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "port scan exited 9") {
			t.Fatalf("past the ceiling the failure is reported, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the wait has no ceiling: it outlived lifecycleWaitMax by far")
	}
}
