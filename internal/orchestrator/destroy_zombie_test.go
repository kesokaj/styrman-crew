package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// A force-remove that lands while a Kata VM is exiting is refused by Docker
// with "PID … is zombie and can not be killed": the container's PID is the
// cloud-hypervisor process, and the shim has not reaped it yet. Destroy
// must wait that window out, not fail the delete (found by Tier 2, 2026-10-05).
const zombieErr = `cannot remove container "styrman-x": could not kill container: container c93 PID 1312994 is zombie and can not be killed. Use the --init option when creating containers to run an init inside the container that forwards signals and reaps processes`

func TestDestroyRetriesAContainerWhoseVMIsStillExiting(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	defer func(d time.Duration) { zombieRetryEvery = d }(zombieRetryEvery)
	zombieRetryEvery = time.Millisecond
	fd.fail["ContainerRemove"] = errors.New(zombieErr)
	removes := 0
	fd.onCall = func(method string) {
		if method == "ContainerRemove" {
			removes++
			if removes == 3 {
				fd.mu.Lock()
				delete(fd.fail, "ContainerRemove")
				fd.mu.Unlock()
			}
		}
	}
	if err := o.Destroy(context.Background(), tUser, tProj); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if removes != 3 {
		t.Fatalf("ContainerRemove called %d times, want 3 (two zombie refusals, then success)", removes)
	}
	if _, ok := fd.byName(config.ContainerName(tProj)); ok {
		t.Fatal("container still there")
	}
}

// Any other failure is returned at once, as before.
func TestDestroyReturnsOtherRemoveErrorsAtOnce(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	fd.fail["ContainerRemove"] = errors.New("permission denied")
	if err := o.Destroy(context.Background(), tUser, tProj); err == nil {
		t.Fatal("Destroy succeeded past a remove error")
	}
	if n := fd.callsTo("ContainerRemove"); n != 1 {
		t.Fatalf("ContainerRemove called %d times, want 1", n)
	}
}

// The wait is bounded by the caller's context.
func TestDestroyGivesUpWhenTheContextEnds(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	defer func(d time.Duration) { zombieRetryEvery = d }(zombieRetryEvery)
	zombieRetryEvery = time.Millisecond
	fd.fail["ContainerRemove"] = errors.New(zombieErr)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := o.Destroy(ctx, tUser, tProj); err == nil {
		t.Fatal("Destroy succeeded while every remove was refused")
	}
}

// The setup VM's removal goes through the same wait (it is full-size too).
func TestDestroySetupRetriesAVMStillExiting(t *testing.T) {
	o, fd, _ := seamOrch(t)
	if _, err := o.EnsureSetup(context.Background(), tUser, "x"); err != nil {
		t.Fatal(err)
	}
	defer func(d time.Duration) { zombieRetryEvery = d }(zombieRetryEvery)
	zombieRetryEvery = time.Millisecond
	fd.fail["ContainerRemove"] = errors.New(zombieErr)
	removes := 0
	fd.onCall = func(method string) {
		if method == "ContainerRemove" {
			removes++
			if removes == 2 {
				fd.mu.Lock()
				delete(fd.fail, "ContainerRemove")
				fd.mu.Unlock()
			}
		}
	}
	if err := o.DestroySetup(context.Background(), tUser); err != nil {
		t.Fatalf("DestroySetup: %v", err)
	}
	if removes != 2 {
		t.Fatalf("ContainerRemove called %d times, want 2", removes)
	}
}

// The setup-VM reaper removes an abandoned VM through the same wait, so a
// VM still exiting is not left behind as an exited container the reaper
// skips from then on.
func TestReapSetupVMsWaitsOutAnExitingVM(t *testing.T) {
	o, fd, _ := seamOrch(t)
	if _, err := o.EnsureSetup(context.Background(), tUser, "x"); err != nil {
		t.Fatal(err)
	}
	c, ok := fd.byName(config.SetupContainer(tUser))
	if !ok {
		t.Fatal("no setup container")
	}
	c.startedAt = time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	fd.list = true
	defer func(d time.Duration) { zombieRetryEvery = d }(zombieRetryEvery)
	zombieRetryEvery = time.Millisecond
	fd.fail["ContainerRemove"] = errors.New(zombieErr)
	removes := 0
	fd.onCall = func(method string) {
		if method == "ContainerRemove" {
			removes++
			if removes == 3 {
				fd.mu.Lock()
				delete(fd.fail, "ContainerRemove")
				fd.mu.Unlock()
			}
		}
	}
	o.ReapSetupVMs(context.Background(), time.Hour)
	if _, ok := fd.byName(config.SetupContainer(tUser)); ok {
		t.Fatalf("the abandoned setup VM is still there after %d removes", removes)
	}
}

// A young setup VM is left alone.
func TestReapSetupVMsLeavesAYoungVM(t *testing.T) {
	o, fd, _ := seamOrch(t)
	if _, err := o.EnsureSetup(context.Background(), tUser, "x"); err != nil {
		t.Fatal(err)
	}
	c, _ := fd.byName(config.SetupContainer(tUser))
	c.startedAt = time.Now().Format(time.RFC3339Nano)
	fd.list = true
	o.ReapSetupVMs(context.Background(), time.Hour)
	if _, ok := fd.byName(config.SetupContainer(tUser)); !ok {
		t.Fatal("a young setup VM was reaped")
	}
}

// The pre-ADR-027 recreate stops the container and then removes it; a stop
// that left the VM still exiting must not fail the start (BUG-090).
func TestThePreADR027RecreateWaitsOutAnExitingVM(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	created(t, fd).binds = nil // no workspace bind: the pre-ADR-027 shape
	defer func(d time.Duration) { zombieRetryEvery = d }(zombieRetryEvery)
	zombieRetryEvery = time.Millisecond
	fd.fail["ContainerRemove"] = errors.New(zombieErr)
	removes := 0
	fd.onCall = func(method string) {
		if method == "ContainerRemove" {
			removes++
			if removes == 2 {
				fd.mu.Lock()
				delete(fd.fail, "ContainerRemove")
				fd.mu.Unlock()
			}
		}
	}
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if fd.callsTo("ContainerCreate") != 1 || !hasWorkspaceBind(&container.HostConfig{Binds: created(t, fd).binds}, cfg.ProjectWorkspace(tUser, tProj)) {
		t.Fatalf("want one recreate with the workspace bind; calls %v", fd.calls)
	}
}
