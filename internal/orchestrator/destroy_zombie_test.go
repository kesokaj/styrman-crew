package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

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
