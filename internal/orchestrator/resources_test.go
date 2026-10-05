package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/system"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// The NUC as `docker info` reported it on 2026-10-05: 16 CPUs, MemTotal
// 65385768 kB = 63853 MiB. 61696 MiB is the largest limit Kata accepted
// there; 61805 MiB (rounded up to 61824) was refused.
var nuc = Host{CPUs: 16, MemMiB: 63853}

func sizingOrch(t *testing.T, operatorDisk int) (*Orchestrator, *fakeDocker) {
	t.Helper()
	cfg := &config.Config{StateDir: t.TempDir()}
	if err := cfg.SetOperator(operatorDisk, -1); err != nil {
		t.Fatal(err)
	}
	fd := newFakeDocker()
	return &Orchestrator{docker: fd, cfg: cfg, logger: slog.New(slog.DiscardHandler)}, fd
}

func TestHostMaxMemory(t *testing.T) {
	for _, tc := range []struct {
		h        Host
		mib, gib int
	}{
		{nuc, 61696, 60},
		{Host{CPUs: 4, MemMiB: 2048 + 128}, 128, 0},
		{Host{CPUs: 4, MemMiB: 2048 + 127}, 0, 0}, // too small to add a block
		{Host{CPUs: 2, MemMiB: 2000}, 0, 0},       // a small Colima VM
		{Host{}, 0, 0},
	} {
		if got := tc.h.MaxMemoryMiB(); got != tc.mib {
			t.Errorf("%+v.MaxMemoryMiB() = %d, want %d", tc.h, got, tc.mib)
		}
		if got := tc.h.MaxMemoryGiB(); got != tc.gib {
			t.Errorf("%+v.MaxMemoryGiB() = %d, want %d", tc.h, got, tc.gib)
		}
	}
}

func TestWantSizing(t *testing.T) {
	o, _ := sizingOrch(t, 30)
	live20 := &Live{Sizing: Sizing{DiskGB: 20}}
	for _, tc := range []struct {
		name string
		h    Host
		r    Resources
		live *Live
		want Sizing
	}{
		{"unlimited is the whole host", nuc, Resources{}, nil,
			Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: 30}},
		{"a set size is kept", nuc, Resources{CPUs: 2, MemoryGiB: 4, DiskGB: 25}, nil,
			Sizing{NanoCPUs: 2e9, MemBytes: 4 << 30, DiskGB: 25}},
		{"CPUs above the host clamp", nuc, Resources{CPUs: 64}, nil,
			Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: 30}},
		{"memory above the ceiling clamps", nuc, Resources{MemoryGiB: 61}, nil,
			Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: 30}},
		{"memory at the ceiling's GiB is kept", nuc, Resources{MemoryGiB: 60}, nil,
			Sizing{NanoCPUs: 16e9, MemBytes: 60 << 30, DiskGB: 30}},
		{"a tiny host sets no memory limit", Host{CPUs: 2, MemMiB: 2000}, Resources{MemoryGiB: 1}, nil,
			Sizing{NanoCPUs: 2e9, MemBytes: 0, DiskGB: 30}},
		{"disk 0 keeps the container's", nuc, Resources{}, live20,
			Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: 20}},
		{"disk 0 with an unreadable container env takes the default", nuc, Resources{}, &Live{},
			Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: 30}},
		{"a recorded disk wins over the container's", nuc, Resources{DiskGB: 40}, live20,
			Sizing{NanoCPUs: 16e9, MemBytes: 61696 << 20, DiskGB: 40}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := o.WantSizing(tc.h, tc.r, tc.live); got != tc.want {
				t.Errorf("WantSizing(%+v, %+v) = %+v, want %+v", tc.h, tc.r, got, tc.want)
			}
		})
	}
}

func TestHostIsReadFromTheDaemonAndCachedOnSuccess(t *testing.T) {
	o, fd := sizingOrch(t, 20)
	fd.fail["Info"] = errors.New("daemon starting")
	if _, err := o.Host(context.Background()); err == nil {
		t.Fatal("Host() succeeded while Info failed")
	}
	delete(fd.fail, "Info")
	fd.info = system.Info{NCPU: 8, MemTotal: 16 << 30}
	h, err := o.Host(context.Background())
	if err != nil || h != (Host{CPUs: 8, MemMiB: 16 << 10}) {
		t.Fatalf("Host() = %+v, %v; want {8 16384}, nil", h, err)
	}
	fd.info = system.Info{NCPU: 1, MemTotal: 1 << 30}
	if h2, _ := o.Host(context.Background()); h2 != h {
		t.Fatalf("Host() asked again after a success: %+v", h2)
	}
	if n := fd.callsTo("Info"); n != 2 {
		t.Fatalf("Info called %d times, want 2 (one failure, one success)", n)
	}
}

func TestHostRefusesAnEmptyAnswer(t *testing.T) {
	o, fd := sizingOrch(t, 20)
	fd.info = system.Info{}
	if _, err := o.Host(context.Background()); err == nil {
		t.Fatal("Host() accepted NCPU 0 / MemTotal 0")
	}
}

func TestLiveSizing(t *testing.T) {
	o, fd := sizingOrch(t, 20)
	name := config.ContainerName(tProj)
	if _, found, err := o.LiveSizing(context.Background(), tProj); found || err != nil {
		t.Fatalf("no container: found=%v err=%v", found, err)
	}
	fd.containers[name] = &fakeContainer{id: "id-" + name, name: name, running: true,
		nanoCPUs: 2e9, memory: 4 << 30, env: []string{"A=b", config.EnvDockerDisk + "=25"}}
	live, found, err := o.LiveSizing(context.Background(), tProj)
	want := Live{Sizing: Sizing{NanoCPUs: 2e9, MemBytes: 4 << 30, DiskGB: 25}, Running: true}
	if !found || err != nil || live != want {
		t.Fatalf("LiveSizing = %+v, %v, %v; want %+v", live, found, err, want)
	}
	fd.fail["ContainerInspect"] = errors.New("daemon gone")
	if _, _, err := o.LiveSizing(context.Background(), tProj); err == nil {
		t.Fatal("an inspect error was folded into an answer")
	}
}

// GET must not wait behind a boot: LiveSizing answers while the project's
// lock is held by someone else.
func TestLiveSizingDoesNotTakeTheProjectLock(t *testing.T) {
	o, _ := sizingOrch(t, 20)
	l := o.lockProject(key(tUser, tProj))
	l.Lock()
	defer l.Unlock()
	done := make(chan struct{})
	go func() {
		_, _, _ = o.LiveSizing(context.Background(), tProj)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("LiveSizing blocked on the project lock")
	}
}

func TestLiveFromToleratesMissingParts(t *testing.T) {
	if got := liveFrom(container.InspectResponse{}); got != (Live{}) {
		t.Fatalf("liveFrom(empty) = %+v", got)
	}
	insp := container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{}}
	if got := liveFrom(insp); got != (Live{}) {
		t.Fatalf("liveFrom(no HostConfig/Config) = %+v", got)
	}
	insp.Config = &container.Config{Env: []string{config.EnvDockerDisk + "=x"}}
	if got := liveFrom(insp); got.DiskGB != 0 {
		t.Fatalf("an unparsable disk env read as %d", got.DiskGB)
	}
}
