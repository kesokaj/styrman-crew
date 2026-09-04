package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCfg(t *testing.T) *Config {
	t.Helper()
	return &Config{
		StateDir:    t.TempDir(),
		dockerDisk:  20,
		idleTimeout: 24 * time.Hour,
	}
}

func TestSetOperatorRoundTrip(t *testing.T) {
	c := testCfg(t)
	if err := c.SetOperator(50, 90*time.Minute); err != nil {
		t.Fatal(err)
	}
	if c.DockerDiskGB() != 50 || c.IdleTimeoutD() != 90*time.Minute {
		t.Fatalf("applied = %d/%s", c.DockerDiskGB(), c.IdleTimeoutD())
	}
	// A fresh Config over the same state dir loads the persisted values.
	c2 := &Config{StateDir: c.StateDir, dockerDisk: 20, idleTimeout: 24 * time.Hour}
	c2.loadOverrides()
	if c2.DockerDiskGB() != 50 || c2.IdleTimeoutD() != 90*time.Minute {
		t.Fatalf("reloaded = %d/%s", c2.DockerDiskGB(), c2.IdleTimeoutD())
	}
}

func TestSetOperatorKeepSemantics(t *testing.T) {
	c := testCfg(t)
	// diskGB 0 and idle < 0 both mean "keep".
	if err := c.SetOperator(0, -1); err != nil {
		t.Fatal(err)
	}
	if c.DockerDiskGB() != 20 || c.IdleTimeoutD() != 24*time.Hour {
		t.Fatalf("keep changed values: %d/%s", c.DockerDiskGB(), c.IdleTimeoutD())
	}
	// Idle 0 is a real value ("never"), not keep.
	if err := c.SetOperator(0, 0); err != nil {
		t.Fatal(err)
	}
	if c.IdleTimeoutD() != 0 {
		t.Fatalf("idle 0 not applied: %s", c.IdleTimeoutD())
	}
}

func TestSetOperatorBounds(t *testing.T) {
	c := testCfg(t)
	for _, bad := range []int{1, 4, 501, -3} {
		if err := c.SetOperator(bad, -1); err == nil {
			t.Errorf("disk %d accepted", bad)
		}
	}
	if c.DockerDiskGB() != 20 {
		t.Fatal("rejected value was applied")
	}
}

func TestSetOperatorPersistFailureKeepsLiveState(t *testing.T) {
	c := testCfg(t)
	// Make the state dir unwritable so persistence fails.
	if err := os.Chmod(c.StateDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(c.StateDir, 0o755)
	if err := c.SetOperator(50, time.Hour); err == nil {
		t.Skip("running as root — cannot simulate write failure")
	}
	// Persist-first: live state must be unchanged after a failed write.
	if c.DockerDiskGB() != 20 || c.IdleTimeoutD() != 24*time.Hour {
		t.Fatalf("live state diverged after failed persist: %d/%s",
			c.DockerDiskGB(), c.IdleTimeoutD())
	}
}

func TestLoadOverridesIgnoresGarbageAndOutOfBounds(t *testing.T) {
	c := testCfg(t)
	os.WriteFile(filepath.Join(c.StateDir, "settings.json"), []byte("{truncated"), 0o644)
	c.loadOverrides()
	if c.DockerDiskGB() != 20 {
		t.Fatal("garbage file changed settings")
	}
	// Hand-edited out-of-bounds values don't bypass validation.
	os.WriteFile(filepath.Join(c.StateDir, "settings.json"),
		[]byte(`{"dockerDiskGB": 100000, "idleTimeout": "-5h"}`), 0o644)
	c.loadOverrides()
	if c.DockerDiskGB() != 20 || c.IdleTimeoutD() != 24*time.Hour {
		t.Fatalf("out-of-bounds override applied: %d/%s", c.DockerDiskGB(), c.IdleTimeoutD())
	}
}

// Load()'s env contract itself had no test — every other test builds &Config{}
// directly and bypasses it. STYRMAN_IMAGE lost its styrman-runtime:latest
// fallback (operator rule: no mutable tags), so an unset value must refuse to
// start with a message naming the variable, and an explicit value must load.
func TestLoadRequiresExplicitImage(t *testing.T) {
	for _, k := range []string{
		"STYRMAN_AUTH", "STYRMAN_ADDR", "STYRMAN_DEV_ADDR",
		"STYRMAN_INGRESS_TOKEN", "STYRMAN_USER", "STYRMAN_HOME_DIR",
	} {
		t.Setenv(k, "") // envOr treats empty as unset — neutralize host env
	}
	t.Setenv("STYRMAN_STATE_DIR", t.TempDir())

	t.Setenv("STYRMAN_IMAGE", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "STYRMAN_IMAGE") {
		t.Fatalf("Load() without STYRMAN_IMAGE must refuse and name the variable, got %v", err)
	}

	t.Setenv("STYRMAN_IMAGE", "styrman-runtime:9.9.9")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() with an explicit image failed: %v", err)
	}
	if c.Image != "styrman-runtime:9.9.9" {
		t.Fatalf("Image = %q, want the explicit value", c.Image)
	}
}

// Every project container is created with CAP_SYS_ADMIN, NET_ADMIN, NET_RAW
// and apparmor=unconfined. That is containment ONLY when the runtime is a
// microVM; under runc the same flags are host root for every project user.
// Load must therefore refuse a non-VM runtime rather than assume one
// (2026-09-01 audit: Runtime was a free string in a Load that otherwise fails
// closed on the image, the auth mode and the ingress token).
func TestLoadRefusesNonVMRuntime(t *testing.T) {
	base := func(t *testing.T) {
		t.Helper()
		t.Setenv("STYRMAN_IMAGE", "styrman-runtime:test")
		t.Setenv("STYRMAN_STATE_DIR", t.TempDir())
		t.Setenv("STYRMAN_AUTH", "none")
	}

	// NB: not "" — an unset STYRMAN_RUNTIME correctly falls back to "mvm".
	// Case and whitespace variants ARE refused: they are typos, not runtimes.
	for _, rt := range []string{"runc", "crun", "MVM", "kata ", "runsc", "docker-runc"} {
		t.Run("refuses "+rt, func(t *testing.T) {
			base(t)
			t.Setenv("STYRMAN_RUNTIME", rt)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted STYRMAN_RUNTIME=%q — escape-capable caps on a non-VM runtime", rt)
			} else if !strings.Contains(err.Error(), "not a VM runtime") {
				t.Errorf("unhelpful error for %q: %v", rt, err)
			}
		})
	}

	for _, rt := range []string{"mvm", "kata", "kata-qemu", "kata-clh"} {
		t.Run("accepts "+rt, func(t *testing.T) {
			base(t)
			t.Setenv("STYRMAN_RUNTIME", rt)
			if _, err := Load(); err != nil {
				t.Errorf("Load rejected VM runtime %q: %v", rt, err)
			}
		})
	}

	// The override exists so a deliberate operator is not locked out, but it
	// has to be explicit — not a typo-friendly default.
	t.Run("explicit override is honoured", func(t *testing.T) {
		base(t)
		t.Setenv("STYRMAN_RUNTIME", "runc")
		t.Setenv("STYRMAN_ALLOW_UNSAFE_RUNTIME", "1")
		if _, err := Load(); err != nil {
			t.Errorf("explicit override still refused: %v", err)
		}
	})
}

// The home layout since ADR-064: the tenant dir is server-only; the setup
// VM's home and every project home are separate directories below it, so no
// guest mount contains another guest's home.
func TestHomeLayoutKeepsGuestMountsApart(t *testing.T) {
	c := &Config{HomesDir: "/x/homes"}
	if got := c.TenantDir("u"); got != "/x/homes/u" {
		t.Errorf("TenantDir = %q", got)
	}
	if got := c.UserHome("u"); got != "/x/homes/u/base" {
		t.Errorf("UserHome = %q", got)
	}
	if got := c.ProjectHome("u", "id"); got != "/x/homes/u/p/id" {
		t.Errorf("ProjectHome = %q", got)
	}
	if strings.HasPrefix(c.ProjectHome("u", "id"), c.UserHome("u")+"/") {
		t.Error("a project home must not sit inside the setup VM's home")
	}
}
