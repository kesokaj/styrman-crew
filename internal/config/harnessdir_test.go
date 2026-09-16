package config

import "testing"

// STYRMAN_HARNESS_DIR names the host's harness channel content dir (ADR-073);
// unset or empty means the channel is disabled and the image's baked mirror
// seeds every project.
func TestHarnessDirFromEnv(t *testing.T) {
	for _, k := range []string{
		"STYRMAN_AUTH", "STYRMAN_ADDR", "STYRMAN_DEV_ADDR",
		"STYRMAN_INGRESS_TOKEN", "STYRMAN_USER", "STYRMAN_HOME_DIR",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("STYRMAN_STATE_DIR", t.TempDir())
	t.Setenv("STYRMAN_IMAGE", "styrman-runtime:9.9.9")

	t.Setenv("STYRMAN_HARNESS_DIR", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HarnessDir != "" {
		t.Fatalf("HarnessDir = %q, want empty (disabled) when the variable is unset", c.HarnessDir)
	}

	t.Setenv("STYRMAN_HARNESS_DIR", "/var/lib/styrman/harness")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HarnessDir != "/var/lib/styrman/harness" {
		t.Fatalf("HarnessDir = %q, want the variable's value", c.HarnessDir)
	}
}
