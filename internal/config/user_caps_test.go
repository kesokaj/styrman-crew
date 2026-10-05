package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The non-admin caps (ADR-087) persist and reload; nil keeps, 0 is a real
// value (half the host).
func TestUserCapsRoundTripAndKeep(t *testing.T) {
	c := testCfg(t)
	four, eight := 4, 8
	if err := c.UpdateOperator(OperatorUpdate{IdleTimeout: -1, UserMaxCPUs: &four, UserMaxMemoryGiB: &eight}); err != nil {
		t.Fatal(err)
	}
	if cpus, mem := c.UserCaps(); cpus != 4 || mem != 8 {
		t.Fatalf("applied %d/%d", cpus, mem)
	}
	// A disk-only change keeps the caps.
	if err := c.SetOperator(30, -1); err != nil {
		t.Fatal(err)
	}
	c2 := &Config{StateDir: c.StateDir, dockerDisk: 20, idleTimeout: 24 * time.Hour}
	c2.loadOverrides()
	if cpus, mem := c2.UserCaps(); cpus != 4 || mem != 8 || c2.DockerDiskGB() != 30 {
		t.Fatalf("reloaded caps %d/%d disk %d", cpus, mem, c2.DockerDiskGB())
	}
	zero := 0
	if err := c.UpdateOperator(OperatorUpdate{IdleTimeout: -1, UserMaxCPUs: &zero}); err != nil {
		t.Fatal(err)
	}
	if cpus, mem := c.UserCaps(); cpus != 0 || mem != 8 {
		t.Fatalf("0 (half the host) not applied, or memory not kept: %d/%d", cpus, mem)
	}
	neg := -1
	if err := c.UpdateOperator(OperatorUpdate{IdleTimeout: -1, UserMaxCPUs: &neg}); err == nil {
		t.Fatal("a negative cap was accepted")
	}
}

// An idle timeout the environment set outside the API's range must not
// block a change to anything else (plan D22): only changed fields are
// validated.
func TestACapChangeIgnoresAnOutOfRangeIdleFromTheEnvironment(t *testing.T) {
	c := testCfg(t)
	c.idleTimeout = 5 * time.Minute // below the API's 10m floor, legal from env
	two := 2
	if err := c.UpdateOperator(OperatorUpdate{IdleTimeout: -1, UserMaxCPUs: &two}); err != nil {
		t.Fatalf("cap change refused: %v", err)
	}
	if c.IdleTimeoutD() != 5*time.Minute {
		t.Fatalf("idle changed to %s", c.IdleTimeoutD())
	}
}

// A negative cap in a hand-edited settings.json is ignored with a warning,
// never applied.
func TestANegativeCapInSettingsJSONIsIgnored(t *testing.T) {
	c := testCfg(t)
	if err := os.WriteFile(filepath.Join(c.StateDir, "settings.json"), []byte(`{"userMaxCpus":-2,"userMaxMemoryGiB":6}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c.loadOverrides()
	if cpus, mem := c.UserCaps(); cpus != 0 || mem != 6 {
		t.Fatalf("caps %d/%d", cpus, mem)
	}
}
