package terminal

import "testing"

// setupTool is interpolated into `bash -lc 'styrman-setup <tool>'`, so anything
// but a known-safe token must be rejected to "" (full flow) — no shell escaping
// is applied, the allowlist IS the guard.
func TestSetupToolAllowlist(t *testing.T) {
	valid := []string{"git", "ssh", "gh", "gcloud", "claude", "agy"}
	for _, v := range valid {
		if got := setupTool(v); got != v {
			t.Errorf("setupTool(%q) = %q, want %q", v, got, v)
		}
	}
	hostile := []string{
		"", "github", "GIT", "gcloud ", " gcloud", "gcloud;rm -rf /",
		"gcloud' ; id ; '", "$(id)", "`id`", "gcloud\n", "..", "*",
	}
	for _, v := range hostile {
		if got := setupTool(v); got != "" {
			t.Errorf("setupTool(%q) = %q, want \"\" (rejected)", v, got)
		}
	}
}
