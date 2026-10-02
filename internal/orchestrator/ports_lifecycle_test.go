package orchestrator

import (
	"context"
	"strings"
	"testing"
)

// BUG-088: a project being stopped is still reported RUNNING, with the same
// container id, until its PID 1 exits — and an exec the shutdown kills exits
// non-zero (9, observed on Kata during a mode toggle). BUG-023's re-check
// asks only "is it gone or replaced?", so that window answered the Ports
// panel's poll with a 500. A lifecycle change holds the project's lock for
// its whole duration; a failed exec while that lock is held is the change's,
// not the command's.

func TestListeningPortsFailureDuringALifecycleChangeIsNoPorts(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9 // the exec the shutdown killed; inspect still says running

	l := o.lockProject(key(tUser, tProj)) // a stop / mode toggle / boot in flight
	l.Lock()
	ports, err := o.ListeningPorts(context.Background(), tUser, tProj)
	l.Unlock()
	if err != nil {
		t.Fatalf("a scan killed by a lifecycle change in flight must be no ports, not an error: %v", err)
	}
	if ports == nil || len(ports) != 0 {
		t.Fatalf("want an empty list, got %#v", ports)
	}
}

// The other half: with NOTHING in flight, the same failure against the same
// running container is a real one and is still reported.
func TestListeningPortsFailureWithNothingInFlightIsAnError(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.execExit = 9

	_, err := o.ListeningPorts(context.Background(), tUser, tProj)
	if err == nil || !strings.Contains(err.Error(), "port scan exited 9") {
		t.Fatalf("want the scan's failure reported, got %v", err)
	}
	// And the lock was not left held by the check itself.
	if l := o.lockProject(key(tUser, tProj)); !l.TryLock() {
		t.Fatal("the project's lock is held after ListeningPorts returned")
	} else {
		l.Unlock()
	}
}
