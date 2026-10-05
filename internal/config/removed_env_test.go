package config

import (
	"strings"
	"testing"
)

func TestRemovedSizingVariablesWarnAndAreIgnored(t *testing.T) {
	env := map[string]string{"STYRMAN_VCPUS": "4", "STYRMAN_MEMORY_MB": "8192"}
	got := removedEnvWarnings(func(k string) string { return env[k] })
	if len(got) != 2 || !strings.Contains(got[0], "STYRMAN_VCPUS") || !strings.Contains(got[1], "STYRMAN_MEMORY_MB") {
		t.Fatalf("warnings = %q", got)
	}
	for _, w := range got {
		if !strings.HasPrefix(w, "WARN: ") || !strings.Contains(w, "ignored") {
			t.Errorf("warning does not say it is ignored: %q", w)
		}
	}
	if got := removedEnvWarnings(func(string) string { return "" }); len(got) != 0 {
		t.Fatalf("warned with nothing set: %q", got)
	}
}
