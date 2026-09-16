package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// BUG-063: stopLocked's in-guest TERM makes PID 1 exit 0 before Docker has
// been told the container is being stopped, and under `unless-stopped` a
// clean exit is a restart, not a stop (dockerd: "restarting container …
// manualRestart=false"; the ContainerStop that followed then hard-killed
// the rebooted VM). fakeDocker plays that restart manager on the TERM exec
// (fakedocker_test.go), so these two tests fail on the old ordering for the
// real reason: a second boot.

// callIndex returns the position of the first recorded call with the given
// prefix and suffix, or -1.
func callIndex(fd *fakeDocker, prefix, suffix string) int {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	for i, c := range fd.calls {
		if strings.HasPrefix(c, prefix) && strings.HasSuffix(c, suffix) {
			return i
		}
	}
	return -1
}

// Stop turns the restart policy off BEFORE the guest is told to exit, so
// the clean exit stays a stop: no second boot, RestartCount 0, the
// container down and its policy "no".
func TestStopTurnsTheRestartPolicyOffBeforeTheGuestExits(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	if err := o.Stop(context.Background(), tUser, tProj); err != nil {
		t.Fatalf("stop: %v", err)
	}
	c, _ := fd.byName(config.ContainerName(tProj))
	fd.mu.Lock()
	running, restarts, policy := c.running, c.restartCount, c.restart
	fd.mu.Unlock()
	if running || restarts != 0 {
		t.Fatalf("after Stop: running=%v restarts=%d — the daemon restarted the clean exit (BUG-063)", running, restarts)
	}
	if policy != "no" {
		t.Fatalf("after Stop the restart policy must be off, got %q", policy)
	}
	off := callIndex(fd, "ContainerUpdate(", ":no)")
	term := callIndex(fd, "ContainerExecCreate(kill -TERM 1)", ")")
	if off < 0 || term < 0 || off > term {
		t.Fatalf("the policy must be switched off before the in-guest TERM: update at %d, TERM at %d", off, term)
	}
}

// Start turns the policy back on BEFORE ContainerStart: a started project
// restarts with the host again, as restartPolicy promises.
func TestEnsureRunningTurnsTheRestartPolicyBackOnBeforeStart(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	c, _ := fd.byName(config.ContainerName(tProj))
	fd.mu.Lock()
	c.restart = "no" // as Stop leaves it
	fd.mu.Unlock()
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("ensure running: %v", err)
	}
	fd.mu.Lock()
	running, policy := c.running, c.restart
	fd.mu.Unlock()
	if !running || policy != "unless-stopped" {
		t.Fatalf("after Start: running=%v policy=%q (want running, unless-stopped)", running, policy)
	}
	on := callIndex(fd, "ContainerUpdate(", ":unless-stopped)")
	start := callIndex(fd, "ContainerStart(", ")")
	if on < 0 || start < 0 || on > start {
		t.Fatalf("the policy must be back on before ContainerStart: update at %d, start at %d", on, start)
	}
}

// The policy switch is best effort: a rejected update (a shim that will not
// take it) is logged and the stop still goes through — the old ordering,
// with its one-boot race, for that stop and nothing worse — while a
// NotFound answer means there is nothing to stop at all.
func TestStopProceedsWhenTheRestartPolicyUpdateFails(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		o, fd, cfg := seamOrch(t)
		plant(fd, cfg, true, false)
		fd.fail["ContainerUpdate"] = errors.New("shim: update rejected")
		fd.execExit = 1 // the TERM does not take either; the stop must still reach Docker
		if err := o.Stop(context.Background(), tUser, tProj); err != nil {
			t.Fatalf("a rejected policy update must not fail the stop: %v", err)
		}
		c, _ := fd.byName(config.ContainerName(tProj))
		fd.mu.Lock()
		running := c.running
		fd.mu.Unlock()
		if running || fd.callsTo("ContainerStop") != 1 {
			t.Fatalf("the stop must still reach Docker: running=%v stops=%d", running, fd.callsTo("ContainerStop"))
		}
	})
	t.Run("gone", func(t *testing.T) {
		o, fd, cfg := seamOrch(t)
		plant(fd, cfg, true, false)
		fd.fail["ContainerUpdate"] = notFound("container")
		if err := o.Stop(context.Background(), tUser, tProj); err != nil {
			t.Fatalf("nothing to stop is not an error: %v", err)
		}
		if fd.callsTo("ContainerExecCreate") != 0 || fd.callsTo("ContainerStop") != 0 {
			t.Fatalf("a container that is gone gets no TERM and no stop, got %v", fd.calls)
		}
	})
}

// A RUNNING container pinned at "no" — a stop that half-failed with the VM
// still up, marker cleared (BUG-030's scenario) — gets its policy back on
// the next attach without being started or otherwise touched.
func TestEnsureRunningRestoresTheRestartPolicyOnARunningContainer(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	c, _ := fd.byName(config.ContainerName(tProj))
	fd.mu.Lock()
	c.restart = "no"
	fd.mu.Unlock()
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("ensure running: %v", err)
	}
	fd.mu.Lock()
	policy := c.restart
	fd.mu.Unlock()
	if policy != "unless-stopped" {
		t.Fatalf("a running container pinned at %q was not restored", policy)
	}
	if fd.callsTo("ContainerStart") != 0 || fd.callsTo("ContainerStop") != 0 || fd.callsTo("ContainerRemove") != 0 {
		t.Fatalf("restoring the policy must not touch a running container, got %v", fd.calls)
	}
}
