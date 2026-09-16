package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/files"
)

// The orchestrator's first unit tests (BUG-003's residual; plan
// .agents/docs/plans/2026-09-10-todo-closeout.md item O): every case plants
// a state in fakeDocker and asserts what ensureRunningLocked / WithProject /
// stopMarked / Status / currentBridge do with it.

const (
	tUser = "alice"
	tProj = "11111111-2222-4333-8444-555555555555"
)

func seamOrch(t *testing.T) (*Orchestrator, *fakeDocker, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{StateDir: dir, HomesDir: filepath.Join(dir, "homes"), Image: "styrman-runtime:test", Username: "tester"}
	files.Trust(cfg.HomesDir)
	for _, d := range []string{cfg.TenantDir(tUser), cfg.ProjectHome(tUser, tProj), cfg.ProjectWorkspace(tUser, tProj)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fd := newFakeDocker()
	return &Orchestrator{docker: fd, cfg: cfg, logger: slog.New(slog.DiscardHandler)}, fd, cfg
}

// plant puts a project container on a bridge of the given mode.
func plant(fd *fakeDocker, cfg *config.Config, running, lan bool) {
	name := config.ContainerName(tProj)
	net := config.NetworkName(tProj)
	fd.containers[name] = &fakeContainer{id: "id-" + name, name: name, image: fd.imageID, running: running,
		binds: []string{cfg.ProjectWorkspace(tUser, tProj) + ":/home/x/projects/p"}, netName: net,
		restart: "unless-stopped"}
	fd.networks[net] = &fakeNetwork{name: net, bridge: config.BridgeNameFor(net, lan)}
}

func opts(lan bool) RunOpts {
	return RunOpts{Name: "p", Dir: "p", GuestUser: "x", Lan: func() (bool, error) { return lan, nil }}
}

// A STOPPED container on a lan- bridge, asked for sandbox: removed, the
// network rebuilt under sdb-, a new container created and started.
func TestEnsureRunningRebuildsAStoppedContainerOnTheWrongBridge(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, true)
	id, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false))
	if err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if id == "" || fd.callsTo("ContainerRemove") != 1 || fd.callsTo("NetworkRemove") != 1 || fd.callsTo("NetworkCreate") != 1 || fd.callsTo("ContainerCreate") != 1 {
		t.Fatalf("want remove+network rebuild+create, got calls %v", fd.calls)
	}
	if b := fd.networks[config.NetworkName(tProj)].bridge; !strings.HasPrefix(b, "sdb-") {
		t.Fatalf("rebuilt bridge is %q, want sdb-", b)
	}
	if o.Stopped(tUser, tProj) {
		t.Fatal("a successful start must lift the stopped marker")
	}
}

// The same, but the network cannot be inspected: refuse to start, touch
// nothing (BUG-031).
func TestEnsureRunningRefusesWhenTheNetworkCannotBeInspected(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, true)
	fd.fail["NetworkInspect"] = errors.New("docker: EOF")
	_, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false))
	if err == nil || !strings.Contains(err.Error(), "refusing to start") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if fd.callsTo("ContainerStart") != 0 || fd.callsTo("ContainerRemove") != 0 {
		t.Fatalf("nothing may be started or removed, got %v", fd.calls)
	}
}

// A project home replaced by a symlink (the guest-writable-paths rule):
// refused before anything is handed to Docker.
func TestEnsureRunningRefusesASymlinkedProjectHome(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	home := cfg.ProjectHome(tUser, tProj)
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), home); err != nil {
		t.Fatal(err)
	}
	_, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false))
	if err == nil || !strings.Contains(err.Error(), "project home") {
		t.Fatalf("want a project-home refusal, got %v", err)
	}
	if fd.callsTo("ContainerStart") != 0 || fd.callsTo("ContainerCreate") != 0 {
		t.Fatalf("Docker must not be handed the home, got %v", fd.calls)
	}
}

// A RUNNING container is left alone whatever the mode says: rebuilding it
// would kill live terminals; the toggle handler stops it first on purpose.
func TestEnsureRunningLeavesARunningContainerAlone(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, true)
	o.stopped.Store(key(tUser, tProj), time.Now())
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatal(err)
	}
	if fd.callsTo("ContainerStop") != 0 || fd.callsTo("ContainerRemove") != 0 || fd.callsTo("ContainerStart") != 0 {
		t.Fatalf("a running container must not be touched, got %v", fd.calls)
	}
	if o.Stopped(tUser, tProj) {
		t.Fatal("an intentional start lifts the marker even when nothing had to be done")
	}
}

// The mode is read under the lock, from RunOpts.Lan, at the moment the
// bridge is decided — a reader that fails refuses the start.
func TestEnsureRunningRefusesWhenTheModeCannotBeRead(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	o2 := opts(false)
	o2.Lan = func() (bool, error) { return false, errors.New("record gone") }
	_, err := o.EnsureRunning(context.Background(), tUser, tProj, o2)
	if err == nil || !strings.Contains(err.Error(), "cannot read the project record") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if fd.callsTo("ContainerStart") != 0 {
		t.Fatal("nothing may start on an unknown mode")
	}
}

// WithProject: stop, persist, start under one hold — and a start that fails
// leaves the record written and the marker set (the container is down).
func TestWithProjectStopPersistStartOrderAndFailedStart(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, true)
	fd.fail["ContainerStart"] = errors.New("kata: boot failed")
	var order []string
	o.WithProject(tUser, tProj, func(pr Project) {
		st, err := pr.Status(context.Background())
		if err != nil || st.State != "running" {
			t.Fatalf("status: %v %v", st, err)
		}
		if mm, err := pr.ModeMismatch(context.Background(), false); err != nil || !mm {
			t.Fatalf("mismatch: %v %v", mm, err)
		}
		if err := pr.Stop(context.Background()); err != nil {
			t.Fatalf("stop: %v", err)
		}
		order = append(order, "stop")
		order = append(order, "persist")
		if _, err := pr.EnsureRunning(context.Background(), opts(false)); err == nil {
			t.Fatal("the scripted start failure must surface")
		}
		order = append(order, "start")
	})
	if strings.Join(order, ",") != "stop,persist,start" {
		t.Fatalf("order %v", order)
	}
	if !o.Stopped(tUser, tProj) {
		t.Fatal("after a failed restart the marker must stay: the container is down and a reconnect must not resurrect it")
	}
}

// stopMarked with a failing stop: the marker is cleared only when the
// container is provably still running, after two samples; kept when it is
// down (BUG-030 + the two-sample rule).
func TestStopMarkedClearsTheMarkerOnlyWhenStillRunning(t *testing.T) {
	for _, still := range []bool{true, false} {
		o, fd, cfg := seamOrch(t)
		plant(fd, cfg, true, false)
		fd.execExit = 1 // the in-guest TERM does not take; fall through to ContainerStop
		fd.fail["ContainerStop"] = errors.New("docker stop: timeout")
		if !still {
			// The stop half-succeeded: the daemon took it down anyway.
			fd.fail["ContainerStop"] = errors.New("transport: EOF")
			c, _ := fd.byName(config.ContainerName(tProj))
			fd.mu.Lock()
			c.running = false
			fd.mu.Unlock()
		}
		// Mark BEFORE stop: when Docker receives the stop, the marker must
		// already be set (BUG-030's ordering), or a reconnect racing the
		// stop boots the VM back.
		markedAtStop := false
		fd.onCall = func(m string) {
			if m == "ContainerStop" {
				markedAtStop = o.Stopped(tUser, tProj)
			}
		}
		if err := o.Stop(context.Background(), tUser, tProj); err == nil {
			t.Fatal("the scripted stop failure must surface")
		}
		if !markedAtStop {
			t.Fatal("the marker was not set before the stop reached Docker")
		}
		deadline := time.Now().Add(4 * time.Second)
		for o.Stopped(tUser, tProj) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if got := o.Stopped(tUser, tProj); got == still {
			t.Fatalf("still running=%v: marker present=%v (want cleared iff still running)", still, got)
		}
		if still && fd.callsTo("ContainerInspect") != 2 {
			t.Fatalf("the marker may clear only after TWO Running samples, got %d inspects", fd.callsTo("ContainerInspect"))
		}
	}
}

// A newer stop's marker survives an older watch: the delete compares the
// stored start time.
func TestUnmarkDoesNotClearANewerStopsMarker(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, true, false)
	old := time.Now().Add(-time.Minute)
	newer := time.Now()
	o.stopped.Store(key(tUser, tProj), newer)
	o.unmarkIfStillRunning(tUser, tProj, old)
	time.Sleep(2500 * time.Millisecond)
	if !o.Stopped(tUser, tProj) {
		t.Fatal("the older watch cleared a marker a newer stop set")
	}
}

func TestStatusMapsAbsentStoppedRunningAndError(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	if st, err := o.Status(context.Background(), tUser, tProj); err != nil || st.State != "absent" {
		t.Fatalf("absent: %v %v", st, err)
	}
	plant(fd, cfg, false, false)
	if st, _ := o.Status(context.Background(), tUser, tProj); st.State != "stopped" {
		t.Fatalf("stopped: %v", st)
	}
	c, _ := fd.byName(config.ContainerName(tProj))
	fd.mu.Lock()
	c.running = true
	fd.mu.Unlock()
	if st, _ := o.Status(context.Background(), tUser, tProj); st.State != "running" {
		t.Fatalf("running: %v", st)
	}
	fd.fail["ContainerInspect"] = errors.New("connection refused")
	if _, err := o.Status(context.Background(), tUser, tProj); err == nil {
		t.Fatal("an inspect error must be an error, never absent (BUG-029)")
	}
}

func TestCurrentBridgeFoundMissingAndError(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	net := config.NetworkName(tProj)
	if _, found, err := o.currentBridge(context.Background(), net); err != nil || found {
		t.Fatalf("missing: found=%v err=%v", found, err)
	}
	plant(fd, cfg, false, true)
	if b, found, err := o.currentBridge(context.Background(), net); err != nil || !found || !strings.HasPrefix(b, "lan-") {
		t.Fatalf("found: %q %v %v", b, found, err)
	}
	fd.fail["NetworkInspect"] = errors.New("EOF")
	if _, _, err := o.currentBridge(context.Background(), net); err == nil {
		t.Fatal("an inspect error must be returned, never folded into not-found")
	}
}

// A STOPPED container whose bridge vanished: the network is recreated and the
// container is REMOVED and created again (never reconnected), so its env
// names the broker address of the bridge it is actually on (BUG-054,
// ADR-069). One level of recursion, one create.
func TestEnsureRunningRecreatesTheContainerWhenItsNetworkVanished(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	delete(fd.networks, config.NetworkName(tProj))
	// The first start fails (Docker: "network not found"); the fake fails every
	// ContainerStart until the second one is observed.
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
	id, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false))
	if err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if id == "" || fd.callsTo("NetworkCreate") != 1 || fd.callsTo("ContainerRemove") != 1 || fd.callsTo("ContainerCreate") != 1 || fd.callsTo("NetworkConnect") != 0 {
		t.Fatalf("want network create + container remove + create, no reconnect; got calls %v", fd.calls)
	}
	if starts != 2 {
		t.Fatalf("starts = %d, want the failed one and the fresh container's", starts)
	}
}

// A network inspect that FAILS (a daemon hiccup, not NotFound) must not be
// read as "network missing": the container is left alone and the start error
// surfaces (security review of the audit fixes, 2026-09-13).
func TestEnsureRunningLeavesTheContainerWhenTheNetworkCannotBeInspectedAfterAFailedStart(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	plant(fd, cfg, false, false)
	fd.fail["ContainerStart"] = errors.New("start failed")
	fd.fail["NetworkInspect"] = errors.New("daemon hiccup")
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err == nil {
		t.Fatal("EnsureRunning succeeded past a failed start")
	}
	if fd.callsTo("ContainerRemove") != 0 || fd.callsTo("NetworkCreate") != 0 {
		t.Fatalf("a non-NotFound inspect error removed or recreated: %v", fd.calls)
	}
}
