package ingress

import (
	"strings"
	"testing"
)

// In role-gated mode the role header is set only INSIDE the header_regexp
// matchers, so a request matching none of them used to carry the CLIENT's own
// X-Caddy-Role to the upstream — unlike every other identity header on that
// path, each of which is stripped or unconditionally overwritten (security
// review 2026-09-22). Nothing reads the header today; the day something does,
// this would be an authorization bug that looks like it lives upstream.
func TestRoleGatedStripsClientSuppliedRole(t *testing.T) {
	apps := []App{{
		Name: "dashboard", Hostname: "dashboard", ContainerIP: "172.18.0.4", Port: 8080,
		Mode: ModeRoleGated, Admins: []string{"alice@gmail.com"}, RPS: 50,
	}}
	result := GenerateCaddyfile(apps, testConfig())

	strip := "request_header -" + HeaderRole
	if !strings.Contains(result, strip) {
		t.Fatalf("role-gated app does not strip a client-supplied %s", HeaderRole)
	}
	// Order is the whole point: the strip must precede the matchers that set
	// the role, or it removes the value the ingress just decided on.
	firstMatcher := strings.Index(result, "header_regexp "+UpstreamHeaderRoles)
	if firstMatcher < 0 {
		t.Fatal("no role matcher emitted for a role-gated app")
	}
	if got := strings.Index(result, strip); got > firstMatcher {
		t.Errorf("%s is stripped at byte %d, AFTER the first role matcher at %d", HeaderRole, got, firstMatcher)
	}
}

// Authenticated mode sets the role unconditionally, so it needs no strip —
// and asserting that keeps the test above honest about WHY it is only in one
// branch, rather than turning into "strip everywhere" by accident.
func TestAuthenticatedModeSetsRoleUnconditionally(t *testing.T) {
	apps := []App{{
		Name: "app", Hostname: "app", ContainerIP: "172.18.0.5", Port: 8080,
		Mode: ModeAuthenticated, RPS: 50,
	}}
	result := GenerateCaddyfile(apps, testConfig())

	if !strings.Contains(result, "request_header "+HeaderRole+" \""+RoleUser+"\"") {
		t.Errorf("authenticated mode does not set %s unconditionally", HeaderRole)
	}
}
