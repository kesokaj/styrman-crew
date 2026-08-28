package config

import (
	"os"
	"path/filepath"
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
