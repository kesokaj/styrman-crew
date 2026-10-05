package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// The start path's half of ADR-083: what a container is created with, when a
// stopped one is rebuilt for its size, and when a start is refused.

func sizedOpts(r Resources) RunOpts {
	o := opts(false)
	o.Resources = func() (Resources, error) { return r, nil }
	return o
}

func withOperatorDisk(t *testing.T, cfg *config.Config, gb int) {
	t.Helper()
	if err := cfg.SetOperator(gb, -1); err != nil {
		t.Fatal(err)
	}
}

func created(t *testing.T, fd *fakeDocker) *fakeContainer {
	t.Helper()
	c, ok := fd.byName(config.ContainerName(tProj))
	if !ok {
		t.Fatal("no project container")
	}
	return c
}

func diskEnv(c *fakeContainer) string { return containerEnv(c.env, config.EnvDockerDisk) }

func TestEnsureRunningCreatesAtTheRequestedSize(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	withOperatorDisk(t, cfg, 30)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{CPUs: 2, MemoryGiB: 4, DiskGB: 25})); err != nil {
		t.Fatal(err)
	}
	c := created(t, fd)
	if c.nanoCPUs != 2e9 || c.memory != 4<<30 || diskEnv(c) != "25" {
		t.Fatalf("created with NanoCPUs=%d Memory=%d disk=%q; want 2e9, 4 GiB, 25", c.nanoCPUs, c.memory, diskEnv(c))
	}
}

func TestEnsureRunningCreatesUnlimitedAtTheWholeHost(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	withOperatorDisk(t, cfg, 30)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{})); err != nil {
		t.Fatal(err)
	}
	c := created(t, fd)
	if c.nanoCPUs != 16e9 || c.memory != 61696<<20 || diskEnv(c) != "30" {
		t.Fatalf("created with NanoCPUs=%d Memory=%d disk=%q; want the host and the operator's 30", c.nanoCPUs, c.memory, diskEnv(c))
	}
}

// A container from before ADR-083: no limits, and the disk the operator's
// default was when it was made. Stopped, it is rebuilt at the whole host on
// its next start — and keeps its 20 GB though the default is 30 now.
func TestEnsureRunningRebuildsAPreADR083Container(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	withOperatorDisk(t, cfg, 30)
	plant(fd, cfg, false, false)
	c := created(t, fd)
	c.nanoCPUs, c.memory, c.env = 0, 0, []string{config.EnvDockerDisk + "=20"}
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{})); err != nil {
		t.Fatal(err)
	}
	if fd.callsTo("ContainerRemove") != 1 || fd.callsTo("ContainerCreate") != 1 {
		t.Fatalf("want one remove and one create; calls %v", fd.calls)
	}
	c = created(t, fd)
	if c.nanoCPUs != 16e9 || c.memory != 61696<<20 || diskEnv(c) != "20" {
		t.Fatalf("rebuilt with NanoCPUs=%d Memory=%d disk=%q; want the host and the old 20", c.nanoCPUs, c.memory, diskEnv(c))
	}
}

func TestEnsureRunningLeavesARunningContainerAtAnotherSize(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	c := created(t, fd)
	c.nanoCPUs, c.memory = 0, 0
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{})); err != nil {
		t.Fatal(err)
	}
	if fd.callsTo("ContainerRemove") != 0 || fd.callsTo("ContainerCreate") != 0 {
		t.Fatalf("a running container was rebuilt; calls %v", fd.calls)
	}
}

func TestEnsureRunningLeavesAStoppedContainerAtTheRightSize(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	withOperatorDisk(t, cfg, 30)
	plant(fd, cfg, false, false)
	c := created(t, fd)
	c.nanoCPUs, c.memory, c.env = 2e9, 4<<30, []string{config.EnvDockerDisk + "=25"}
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{CPUs: 2, MemoryGiB: 4, DiskGB: 25})); err != nil {
		t.Fatal(err)
	}
	if fd.callsTo("ContainerRemove") != 0 || fd.callsTo("ContainerCreate") != 0 || fd.callsTo("ContainerStart") != 1 {
		t.Fatalf("want a plain start; calls %v", fd.calls)
	}
}

// Another recreate (here: a newer image) must not lose the old disk size
// either: want is decided before the container is removed.
func TestAnImageRecreateKeepsTheContainersDisk(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	withOperatorDisk(t, cfg, 30)
	plant(fd, cfg, false, false)
	c := created(t, fd)
	c.env = []string{config.EnvDockerDisk + "=20"}
	c.image = "sha256:image-old"
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{})); err != nil {
		t.Fatal(err)
	}
	if fd.callsTo("ContainerCreate") != 1 || diskEnv(created(t, fd)) != "20" {
		t.Fatalf("want one recreate keeping disk 20; got disk %q, calls %v", diskEnv(created(t, fd)), fd.calls)
	}
}

// The missing-network recursion removes the container and starts again; the
// second pass has no container to read, so the disk decided on the first one
// is carried into it.
func TestTheMissingNetworkRecreateKeepsTheContainersDisk(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	withOperatorDisk(t, cfg, 30)
	plant(fd, cfg, false, false)
	created(t, fd).env = []string{config.EnvDockerDisk + "=20"}
	delete(fd.networks, config.NetworkName(tProj))
	fd.fail["ContainerStart"] = errors.New("network sdb-x not found")
	starts := 0
	fd.onCall = func(method string) {
		if method == "ContainerStart" {
			starts++
			if starts == 2 {
				fd.mu.Lock()
				delete(fd.fail, "ContainerStart")
				fd.mu.Unlock()
			}
		}
	}
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{})); err != nil {
		t.Fatal(err)
	}
	if fd.callsTo("ContainerCreate") != 1 || diskEnv(created(t, fd)) != "20" {
		t.Fatalf("want one recreate keeping disk 20; got disk %q, calls %v", diskEnv(created(t, fd)), fd.calls)
	}
}

func TestEnsureRunningRefusesWithoutAResourcesReader(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	op := opts(false)
	op.Resources = nil
	_, err := o.EnsureRunning(context.Background(), tUser, tProj, op)
	if !errors.Is(err, errNoResources) {
		t.Fatalf("err = %v, want errNoResources", err)
	}
	if fd.callsTo("ContainerStart") != 0 || fd.callsTo("ContainerCreate") != 0 {
		t.Fatalf("a start without a size went ahead; calls %v", fd.calls)
	}
}

func TestEnsureRunningRefusesWhenTheRecordCannotBeRead(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	op := opts(false)
	op.Resources = func() (Resources, error) { return Resources{}, errors.New("disk on fire") }
	_, err := o.EnsureRunning(context.Background(), tUser, tProj, op)
	if err == nil || !strings.Contains(err.Error(), "cannot read the project record") {
		t.Fatalf("err = %v", err)
	}
	if fd.callsTo("ContainerStart") != 0 {
		t.Fatalf("started anyway; calls %v", fd.calls)
	}
}

func TestEnsureRunningRefusesWhenTheHostSizeIsUnknown(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	fd.fail["Info"] = errors.New("daemon starting")
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, sizedOpts(Resources{})); err == nil {
		t.Fatal("started without knowing the host's size")
	}
	if fd.callsTo("ContainerStart") != 0 {
		t.Fatalf("started anyway; calls %v", fd.calls)
	}
}

func TestTheSetupVMIsCreatedUnlimited(t *testing.T) {
	o, fd, _ := seamOrch(t)
	if _, err := o.EnsureSetup(context.Background(), tUser, "x"); err != nil {
		t.Fatal(err)
	}
	c, ok := fd.byName(config.SetupContainer(tUser))
	if !ok {
		t.Fatal("no setup container")
	}
	if c.nanoCPUs != 16e9 || c.memory != 61696<<20 || diskEnv(c) != "" {
		t.Fatalf("setup VM NanoCPUs=%d Memory=%d disk=%q; want the host and no disk env", c.nanoCPUs, c.memory, diskEnv(c))
	}
}
