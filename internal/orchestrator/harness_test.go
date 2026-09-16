package orchestrator

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// The harness channel's server side (ADR-073): a project container mounts
// <HarnessDir>/current/{repo,bin/sh-agent} read-only when the channel has
// delivered and the content dir is trusted; the setup VM never does; and a
// STOPPED container is recreated to gain the binds once the channel is
// present, or to drop them once it is absent (a missing bind source would
// otherwise be created by Docker as a root-owned real directory at
// current/, which the channel could then never swap). An untrusted channel
// changes nothing.

const tSha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// plantChannel lays out one delivered export under dir and returns the
// unresolved `current` path a bind must name.
func plantChannel(t *testing.T, dir string) string {
	t.Helper()
	// The content dir as host-setup.sh leaves it (0755); t.TempDir() is 0775
	// under this host's umask, which the server rightly refuses.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{filepath.Join(dir, tSha, "repo"), filepath.Join(dir, tSha, "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for f, c := range map[string]string{"repo/COMMIT": tSha, "repo/TAG": "v1.0.0", "bin/sh-agent": "#!/bin/sh\n"} {
		if err := os.WriteFile(filepath.Join(dir, tSha, f), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, tSha, "bin", "sh-agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tSha, filepath.Join(dir, "current")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "current")
}

func wantHarnessBinds(src string) []string {
	return []string{src + "/repo:/styrman/harness:ro", src + "/bin/sh-agent:/styrman/bin/sh-agent:ro"}
}

func TestHarnessBindsNameTheUnresolvedCurrentPath(t *testing.T) {
	dir := t.TempDir()
	src := plantChannel(t, dir)
	got, err := harnessBinds(dir)
	if err != nil {
		t.Fatalf("harnessBinds: %v", err)
	}
	if want := wantHarnessBinds(src); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("binds %v, want %v (the symlink itself, so Docker re-resolves it on every start)", got, want)
	}
}

func TestHarnessBindsAbsentChannelIsNotExist(t *testing.T) {
	dir := t.TempDir() // the content dir exists, nothing delivered
	if got, err := harnessBinds(dir); got != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty content dir: binds %v err %v, want nil and fs.ErrNotExist", got, err)
	}
	if got, err := harnessBinds(filepath.Join(dir, "never-created")); got != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing content dir: binds %v err %v, want nil and fs.ErrNotExist", got, err)
	}
}

func TestHarnessBindsDisabledIsNilNil(t *testing.T) {
	if got, err := harnessBinds(""); got != nil || err != nil {
		t.Fatalf("disabled: binds %v err %v, want nil, nil", got, err)
	}
}

func TestHarnessBindsRefuseAnUntrustedChannel(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"world-writable export": func(t *testing.T, dir string) {
			plantChannel(t, dir)
			if err := os.Chmod(filepath.Join(dir, tSha), 0o777); err != nil {
				t.Fatal(err)
			}
		},
		"world-writable content dir": func(t *testing.T, dir string) {
			plantChannel(t, dir)
			if err := os.Chmod(dir, 0o777); err != nil {
				t.Fatal(err)
			}
		},
		"current escapes the content dir": func(t *testing.T, dir string) {
			outside := filepath.Join(filepath.Dir(dir), "outside")
			plantChannel(t, outside)
			if err := os.Symlink(filepath.Join(outside, tSha), filepath.Join(dir, "current")); err != nil {
				t.Fatal(err)
			}
		},
		"current is a real directory": func(t *testing.T, dir string) {
			plantChannel(t, dir)
			if err := os.Remove(filepath.Join(dir, "current")); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(dir, tSha), filepath.Join(dir, "current")); err != nil {
				t.Fatal(err)
			}
		},
		"no sh-agent binary": func(t *testing.T, dir string) {
			plantChannel(t, dir)
			if err := os.Remove(filepath.Join(dir, tSha, "bin", "sh-agent")); err != nil {
				t.Fatal(err)
			}
		},
		"no COMMIT": func(t *testing.T, dir string) {
			plantChannel(t, dir)
			if err := os.Remove(filepath.Join(dir, tSha, "repo", "COMMIT")); err != nil {
				t.Fatal(err)
			}
		},
		"content dir is a symlink": func(t *testing.T, dir string) {
			real := filepath.Join(filepath.Dir(dir), "real")
			plantChannel(t, real)
			if err := os.Remove(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, dir); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "harness")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			plant(t, dir)
			got, err := harnessBinds(dir)
			if got != nil || err == nil || errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("binds %v err %v, want nil and an error that is not fs.ErrNotExist (untrusted, not absent)", got, err)
			}
		})
	}
}

func TestHarnessChannelStateReadsTagAndCommit(t *testing.T) {
	dir := t.TempDir()
	plantChannel(t, dir)
	tag, commit, err := harnessChannelState(dir)
	if err != nil || tag != "v1.0.0" || commit != tSha {
		t.Fatalf("state: tag %q commit %q err %v", tag, commit, err)
	}
}

// A stopped project container created before the channel delivered has no
// harness bind: it is recreated with the two binds on its next start.
func TestEnsureRunningRecreatesAStoppedContainerToAddTheHarnessBinds(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	cfg.HarnessDir = t.TempDir()
	src := plantChannel(t, cfg.HarnessDir)
	plant(fd, cfg, false, false)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if fd.callsTo("ContainerRemove") != 1 || fd.callsTo("ContainerCreate") != 1 {
		t.Fatalf("want one remove and one create, got %v", fd.calls)
	}
	c, _ := fd.byName(configContainerName())
	for _, w := range wantHarnessBinds(src) {
		if !contains(c.binds, w) {
			t.Fatalf("recreated container lacks %q: %v", w, c.binds)
		}
	}
}

// The reverse: the channel is absent (nothing at current) and a stopped
// container still binds it — recreated WITHOUT the binds, so Docker never
// creates the missing source as a root-owned directory inside the content
// dir.
func TestEnsureRunningRecreatesAStoppedContainerToDropTheHarnessBindsWhenTheChannelIsAbsent(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	cfg.HarnessDir = t.TempDir()
	plant(fd, cfg, false, false)
	c, _ := fd.byName(configContainerName())
	c.binds = append(c.binds, wantHarnessBinds(filepath.Join(cfg.HarnessDir, "current"))...)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if fd.callsTo("ContainerRemove") != 1 || fd.callsTo("ContainerCreate") != 1 {
		t.Fatalf("want one remove and one create, got %v", fd.calls)
	}
	c, _ = fd.byName(configContainerName())
	if hasHarnessBind(c.binds) {
		t.Fatalf("recreated container still binds the absent channel: %v", c.binds)
	}
	if _, err := os.Lstat(filepath.Join(cfg.HarnessDir, "current")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the server must not create current itself: %v", err)
	}
}

// An untrusted channel (present, but a mode the server refuses) is not a
// recreate trigger: the stopped container starts as it is.
func TestEnsureRunningLeavesAStoppedContainerWhenTheChannelIsUntrusted(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	cfg.HarnessDir = t.TempDir()
	plantChannel(t, cfg.HarnessDir)
	if err := os.Chmod(filepath.Join(cfg.HarnessDir, tSha), 0o777); err != nil {
		t.Fatal(err)
	}
	plant(fd, cfg, false, false)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if fd.callsTo("ContainerRemove") != 0 || fd.callsTo("ContainerCreate") != 0 || fd.callsTo("ContainerStart") != 1 {
		t.Fatalf("want a plain start, got %v", fd.calls)
	}
}

// A RUNNING container without the binds is left alone (it gains them on its
// next stop/start, like an image upgrade).
func TestEnsureRunningLeavesARunningContainerWithoutTheHarnessBinds(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	cfg.HarnessDir = t.TempDir()
	plantChannel(t, cfg.HarnessDir)
	plant(fd, cfg, true, false)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if fd.callsTo("ContainerRemove") != 0 || fd.callsTo("ContainerCreate") != 0 {
		t.Fatalf("a running container must not be rebuilt, got %v", fd.calls)
	}
}

// A fresh project create carries the binds; the setup VM's create never does.
func TestCreateBindsTheChannelForProjectsNotForTheSetupVM(t *testing.T) {
	o, fd, cfg := seamOrch(t)
	cfg.HarnessDir = t.TempDir()
	src := plantChannel(t, cfg.HarnessDir)
	if _, err := o.EnsureRunning(context.Background(), tUser, tProj, opts(false)); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	c, _ := fd.byName(configContainerName())
	for _, w := range wantHarnessBinds(src) {
		if !contains(c.binds, w) {
			t.Fatalf("new project container lacks %q: %v", w, c.binds)
		}
	}
	if _, err := o.create(context.Background(), setupSpec(cfg)); err != nil {
		t.Fatalf("create setup: %v", err)
	}
	s, ok := fd.byName(setupSpec(cfg).name)
	if !ok || hasHarnessBind(s.binds) {
		t.Fatalf("the setup VM must not mount the channel: %v", s.binds)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func configContainerName() string { return "styrman-" + tProj }

// setupSpec is EnsureSetup's create spec, as orchestrator.go builds it.
func setupSpec(cfg *config.Config) createSpec {
	return createSpec{
		name:      config.SetupContainer(tUser),
		netName:   config.SetupNetwork(tUser),
		hostname:  "styrman-setup",
		guestUser: "x",
		homePath:  cfg.UserHome(tUser),
		extraEnv:  []string{config.EnvProject + "=setup", "STYRMAN_MINIMAL=1"},
		labels:    map[string]string{config.LabelManaged: "true", config.LabelSystem: "setup", config.LabelUser: tUser},
		noRestart: true,
	}
}
