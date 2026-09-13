package ingress

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testConfig returns a GeneratorConfig with all domain fields populated
// for consistent testing. Apps go to <hostname>.apps.example.com.
func testConfig() GeneratorConfig {
	return GeneratorConfig{
		BaseDomain:  "example.com",
		AdminEmail:  "admin@example.com",
		DefaultRPS:  100,
		AuthDomain:  "auth.example.com",
		ShellDomain: "shell.example.com",
		AppsDomain:  "apps.example.com",
	}
}

func TestGenerateCaddyfile_NoApps(t *testing.T) {
	cfg := testConfig()
	result := GenerateCaddyfile(nil, cfg)

	// Should still have global options and HTTP redirect
	assertContains(t, result, "email admin@example.com")
	assertContains(t, result, "acme_dns cloudflare")
	assertContains(t, result, "http:// {")
	assertContains(t, result, "redir https://{host}{uri} permanent")

	// Prometheus metrics always enabled
	assertContains(t, result, "metrics {")
	assertContains(t, result, "per_host")

	// Should NOT have security block or auth portal (no apps need OAuth)
	assertNotContains(t, result, "security {")
	assertNotContains(t, result, "authenticate with myportal")
}

func TestGenerateCaddyfile_PublicApp(t *testing.T) {
	apps := []App{
		{
			Name:        "landing",
			Hostname:    "landing",
			ContainerIP: "172.18.0.2",
			Port:        80,
			Mode:        ModePublic,
			RPS:         100,
		},
	}
	cfg := testConfig()
	result := GenerateCaddyfile(apps, cfg)

	// Public app should have reverse_proxy and rate_limit, but NO authorize
	assertContains(t, result, "landing.apps.example.com {")
	assertContains(t, result, "reverse_proxy 172.18.0.2:80")
	assertContains(t, result, "zone landing_rl")
	assertContains(t, result, "events 100")
	assertNotContains(t, result, "authorize with")
	assertContains(t, result, `X-Caddy-Role "public"`)
	// Public apps must STRIP client-supplied identity/ingress headers (M24), and
	// must never SET a real identity (only the public role).
	assertContains(t, result, "request_header -X-Caddy-Email")
	assertContains(t, result, "request_header -X-Styrman-Ingress")
	assertNotContains(t, result, `X-Caddy-Email "{`)

	// No auth portal needed (all apps are public)
	assertNotContains(t, result, "security {")
}

func TestGenerateCaddyfile_AuthenticatedApp(t *testing.T) {
	apps := []App{
		{
			Name:        "wiki",
			Hostname:    "wiki",
			ContainerIP: "172.18.0.3",
			Port:        3000,
			Mode:        ModeAuthenticated,
			RPS:         100,
		},
	}
	cfg := testConfig()
	result := GenerateCaddyfile(apps, cfg)

	// Authenticated app should have OAuth + authorize, but NO X-Caddy-Role
	assertContains(t, result, "security {")
	assertContains(t, result, "authentication portal myportal")
	assertContains(t, result, "auth.example.com {")
	assertContains(t, result, "wiki.apps.example.com {")
	assertContains(t, result, "authorize with wiki_policy")
	assertContains(t, result, "allow roles authp/user authp/guest")
	assertContains(t, result, "X-Caddy-Email")
	assertContains(t, result, `X-Caddy-Role "user"`)
}

func TestGenerateCaddyfile_RoleGatedApp(t *testing.T) {
	apps := []App{
		{
			Name:        "dashboard",
			Hostname:    "dashboard",
			ContainerIP: "172.18.0.4",
			Port:        8080,
			Mode:        ModeRoleGated,
			Admins:      []string{"alice@gmail.com"},
			Viewers:     []string{"bob@gmail.com"},
			RPS:         50,
		},
	}
	cfg := testConfig()
	cfg.SuperAdmins = []string{"boss@gmail.com"}
	result := GenerateCaddyfile(apps, cfg)

	// Should have transforms for superadmin + app admins + app viewers
	assertContains(t, result, "match email boss@gmail.com")
	assertContains(t, result, "action add role app/dashboard/superadmin")
	assertContains(t, result, "match email alice@gmail.com")
	assertContains(t, result, "action add role app/dashboard/admin")
	assertContains(t, result, "match email bob@gmail.com")
	assertContains(t, result, "action add role app/dashboard/viewer")

	// Authorization policy
	assertContains(t, result, "authorization policy dashboard_policy")
	assertContains(t, result, "allow roles app/dashboard/superadmin app/dashboard/admin app/dashboard/viewer")

	// Site block
	assertContains(t, result, "dashboard.apps.example.com {")
	assertContains(t, result, "authorize with dashboard_policy")
	assertContains(t, result, "events 50")
	assertContains(t, result, "X-Caddy-Role")
	assertContains(t, result, "X-Caddy-Email")
	assertContains(t, result, "reverse_proxy 172.18.0.4:8080")
}

func TestGenerateCaddyfile_AllThreeModes(t *testing.T) {
	apps := []App{
		{
			Name: "public-app", Hostname: "public-app",
			ContainerIP: "172.18.0.2", Port: 80,
			Mode: ModePublic, RPS: 100,
		},
		{
			Name: "wiki", Hostname: "wiki",
			ContainerIP: "172.18.0.3", Port: 3000,
			Mode: ModeAuthenticated, RPS: 100,
		},
		{
			Name: "dashboard", Hostname: "dashboard",
			ContainerIP: "172.18.0.4", Port: 8080,
			Mode:   ModeRoleGated,
			Admins: []string{"alice@gmail.com"}, Viewers: []string{"bob@gmail.com"},
			RPS: 50,
		},
	}
	cfg := testConfig()
	cfg.SuperAdmins = []string{"boss@gmail.com"}
	result := GenerateCaddyfile(apps, cfg)

	// All three site blocks present
	assertContains(t, result, "public-app.apps.example.com {")
	assertContains(t, result, "wiki.apps.example.com {")
	assertContains(t, result, "dashboard.apps.example.com {")

	// Auth portal present (non-public apps exist)
	assertContains(t, result, "auth.example.com {")

	// HTTP redirect always present
	assertContains(t, result, "http:// {")
}

func TestGenerateCaddyfile_SuperAdminDedup(t *testing.T) {
	apps := []App{
		{
			Name: "myapp", Hostname: "myapp",
			ContainerIP: "172.18.0.2", Port: 80,
			Mode:        ModeRoleGated,
			SuperAdmins: []string{"boss@gmail.com"}, // Same as SUPERADMINS
			RPS:         100,
		},
	}
	cfg := testConfig()
	cfg.SuperAdmins = []string{"boss@gmail.com"}
	result := GenerateCaddyfile(apps, cfg)

	// boss@gmail.com should appear only once (not duplicated by env + label)
	count := strings.Count(result, "match email boss@gmail.com")
	if count != 1 {
		t.Errorf("expected 1 transform for boss@gmail.com, got %d", count)
	}
}

func TestGenerateCaddyfile_WithBaseFile(t *testing.T) {
	// Create a temp Caddyfile.base
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "Caddyfile.base")
	baseContent := "example.com {\n    redir https://www.example.com{uri} permanent\n}\n"
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("writing temp base file: %v", err)
	}

	cfg := testConfig()
	cfg.CaddyfileBasePath = basePath
	result := GenerateCaddyfile(nil, cfg)

	// Base content should be prepended
	assertContains(t, result, "FROM Caddyfile.base")
	assertContains(t, result, "example.com {")
	assertContains(t, result, "redir https://www.example.com{uri} permanent")

	// Generated marker should follow
	assertContains(t, result, "GENERATED by caddy-ingress")
}

func TestGenerateCaddyfile_NoBaseFile(t *testing.T) {
	cfg := testConfig()
	cfg.CaddyfileBasePath = "/nonexistent/path"
	result := GenerateCaddyfile(nil, cfg)

	// Should NOT have the base file marker
	assertNotContains(t, result, "FROM Caddyfile.base")
	// Should still have generated marker
	assertContains(t, result, "GENERATED by caddy-ingress")
}

func TestGenerateCaddyfile_DeterministicOrder(t *testing.T) {
	apps := []App{
		{Name: "zzz", Hostname: "zzz", ContainerIP: "172.18.0.2", Port: 80, Mode: ModePublic, RPS: 100},
		{Name: "aaa", Hostname: "aaa", ContainerIP: "172.18.0.3", Port: 80, Mode: ModePublic, RPS: 100},
		{Name: "mmm", Hostname: "mmm", ContainerIP: "172.18.0.4", Port: 80, Mode: ModePublic, RPS: 100},
	}
	cfg := testConfig()

	result1 := GenerateCaddyfile(apps, cfg)
	result2 := GenerateCaddyfile(apps, cfg)

	if result1 != result2 {
		t.Error("expected deterministic output, but got different results")
	}

	// Should be sorted: aaa, mmm, zzz
	aaaIdx := strings.Index(result1, "aaa.apps.example.com")
	mmmIdx := strings.Index(result1, "mmm.apps.example.com")
	zzzIdx := strings.Index(result1, "zzz.apps.example.com")
	if aaaIdx > mmmIdx || mmmIdx > zzzIdx {
		t.Error("expected alphabetical order: aaa, mmm, zzz")
	}
}

func TestGenerateCaddyfile_HTTPSRedirectAlwaysPresent(t *testing.T) {
	cfg := testConfig()

	// Even with no apps, HTTP redirect should be present
	result := GenerateCaddyfile(nil, cfg)
	assertContains(t, result, "http:// {\n    redir https://{host}{uri} permanent\n}")
}

func TestGenerateCaddyfile_SuperAdminsOnAuthenticatedApps(t *testing.T) {
	apps := []App{
		{
			Name: "wiki", Hostname: "wiki",
			ContainerIP: "172.18.0.2", Port: 80,
			Mode: ModeAuthenticated, RPS: 100,
		},
	}
	cfg := testConfig()
	cfg.SuperAdmins = []string{"boss@gmail.com"}
	result := GenerateCaddyfile(apps, cfg)

	// Superadmin should get a role transform even on authenticated apps
	assertContains(t, result, "match email boss@gmail.com")
	assertContains(t, result, "action add role app/wiki/superadmin")
	// Policy should allow both superadmin role and authp/user
	assertContains(t, result, "allow roles app/wiki/superadmin authp/user authp/guest")
}

// --- helpers ---

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q, but it didn't.\nFull output:\n%s", needle, haystack)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("expected output NOT to contain %q, but it did.\nFull output:\n%s", needle, haystack)
	}
}

func TestIngressTokenHeaderInjected(t *testing.T) {
	apps := []App{{Hostname: "app", ContainerIP: "172.18.0.4", Port: 8080, Mode: ModeAuthenticated}}
	cfg := GeneratorConfig{BaseDomain: "styrman.xyz", AdminEmail: "a@b.c", IngressToken: "s3cr3t-tok"}
	result := GenerateCaddyfile(apps, cfg)
	assertContains(t, result, `request_header X-Styrman-Ingress "s3cr3t-tok"`)
	// The client-supplied header is stripped even when a token IS set (the
	// strip precedes the inject, so a forged value can never survive).
	assertContains(t, result, "request_header -X-Styrman-Ingress")
	// INJECTION absent when unset — but the defensive strip must remain, so a
	// token-less deploy still can't relay a forged header upstream.
	cfg.IngressToken = ""
	out := GenerateCaddyfile(apps, cfg)
	if strings.Contains(out, `request_header X-Styrman-Ingress`) {
		t.Fatal("token header must not be injected when IngressToken is empty")
	}
	assertContains(t, out, "request_header -X-Styrman-Ingress")
}

func TestPreviewWildcardBlock(t *testing.T) {
	apps := []App{
		{Hostname: "styrman", ContainerIP: "10.0.0.5", Port: 8080, Mode: ModeAuthenticated, RPS: 100, PreviewTarget: true},
	}
	cfg := GeneratorConfig{
		BaseDomain: "styrman.xyz", AdminEmail: "a@b.c", AppsDomain: "styrman.xyz",
		AuthDomain: "auth.styrman.xyz", PreviewDomain: "preview.styrman.xyz",
		IngressToken: "tok123", DefaultRPS: 100,
	}
	out := GenerateCaddyfile(apps, cfg)
	for _, want := range []string{
		"*.preview.styrman.xyz {",
		"reverse_proxy 10.0.0.5:8080",
		"header_up Host {host}",
		"request_header -X-Caddy-Email",
		"request_header X-Styrman-Ingress",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preview wildcard block missing %q\n%s", want, out)
		}
	}
	// Disabled when PreviewDomain is empty.
	cfg.PreviewDomain = ""
	if strings.Contains(GenerateCaddyfile(apps, cfg), "preview.styrman.xyz {") {
		t.Error("preview block emitted with PreviewDomain empty")
	}
}

func TestPrivateWildcardBlock(t *testing.T) {
	apps := []App{
		{Hostname: "styrman", ContainerIP: "10.0.0.5", Port: 8080, Mode: ModeAuthenticated, RPS: 100, PreviewTarget: true},
	}
	cfg := GeneratorConfig{
		BaseDomain: "styrman.xyz", AdminEmail: "a@b.c", AppsDomain: "styrman.xyz",
		AuthDomain: "auth.styrman.xyz", PrivateDomain: "private.crew.styrman.xyz",
		IngressToken: "tok123", DefaultRPS: 100,
	}
	out := GenerateCaddyfile(apps, cfg)
	for _, want := range []string{
		"*.private.crew.styrman.xyz {",
		"authorize with styrman_policy",
		`request_header X-Caddy-Email "{http.request.header.X-Token-User-Email}"`,
		"request_header X-Styrman-Ingress",
		"header_up Host {host}",
		"reverse_proxy 10.0.0.5:8080",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("private wildcard block missing %q\n%s", want, out)
		}
	}
	// The block must strip client-supplied claim headers BEFORE authorize.
	stripIdx := strings.Index(out, "*.private.crew.styrman.xyz {")
	block := out[stripIdx:]
	if s, a := strings.Index(block, "request_header -X-Token-User-Email"), strings.Index(block, "authorize with"); s < 0 || a < 0 || s > a {
		t.Errorf("claim-header strip must precede authorize (strip=%d authorize=%d)", s, a)
	}
	// Disabled when PrivateDomain is empty.
	cfg.PrivateDomain = ""
	if strings.Contains(GenerateCaddyfile(apps, cfg), "private.crew.styrman.xyz {") {
		t.Error("private block emitted with PrivateDomain empty")
	}
	// Never emitted for a PUBLIC target app: the block reuses the app's auth
	// policy, which only exists for authenticated apps.
	cfg.PrivateDomain = "private.crew.styrman.xyz"
	apps[0].Mode = ModePublic
	if strings.Contains(GenerateCaddyfile(apps, cfg), "*.private.crew.styrman.xyz {") {
		t.Error("private block emitted for a public target app")
	}
}

// The portal's session lifetime is set explicitly (ADR-063): caddy-security's
// 900 s default made every project reload on the quarter hour once BUG-001
// surfaced expiry. A missing directive means the plugin default is back.
func TestGenerateCaddyfile_SessionLifetimeIsExplicit(t *testing.T) {
	apps := []App{{Name: "wiki", Hostname: "wiki", ContainerIP: "172.18.0.3", Port: 3000, Mode: ModeAuthenticated, RPS: 100}}
	result := GenerateCaddyfile(apps, testConfig())
	assertContains(t, result, "authentication portal myportal")
	assertContains(t, result, "crypto default token lifetime 28800")
	if SessionLifetimeSeconds != 8*60*60 {
		t.Errorf("SessionLifetimeSeconds = %d, want a working day (28800)", SessionLifetimeSeconds)
	}
}

// Audit 2026-09-13: the portal's redirect trust is label-bounded (a bare
// suffix trusted "evil<base>"), the session cookie is SameSite=Lax, and the
// public preview block cuts that one cookie out of the header before
// tunnelling into a guest app (the app's own cookies survive).
func TestGenerateCaddyfile_PortalRedirectSuffixCookieAndPreviewCookieStrip(t *testing.T) {
	apps := []App{{Name: "crew", Hostname: "crew", ContainerIP: "172.18.0.2", Port: 8080, Mode: ModeAuthenticated, RPS: 100}}
	cfg := testConfig()
	result := GenerateCaddyfile(apps, cfg)
	assertContains(t, result, "trust login redirect uri domain suffix ."+cfg.BaseDomain+" path prefix /")
	assertNotContains(t, result, "domain suffix "+cfg.BaseDomain+" path")
	assertContains(t, result, "cookie samesite lax")
	if cfg.PreviewDomain != "" {
		assertContains(t, result, `request_header Cookie "(?:^|;\s*)`+AuthCookieName+`=[^;]*" ""`)
	}
	// Caddy compiles that pattern with Go's regexp: it must compile here too,
	// and cut exactly the portal cookie out of a header that carries others.
	re := regexp.MustCompile(`(?:^|;\s*)` + AuthCookieName + `=[^;]*`)
	if got := re.ReplaceAllString("a=1; "+AuthCookieName+"=tok.en; b=2", ""); got != "a=1; b=2" {
		t.Fatalf("cookie strip regex: %q", got)
	}
	if got := re.ReplaceAllString(AuthCookieName+"=tok", ""); got != "" {
		t.Fatalf("cookie strip regex (only cookie): %q", got)
	}
	if out := os.Getenv("STYRMAN_CADDYFILE_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(result), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSanitizeHostnameRefusesEmpty(t *testing.T) {
	labels := map[string]string{LabelCaddy: "true", LabelHostname: "___", LabelPort: "80"}
	if _, err := ParseLabels(labels, "c1", []int{80}, 100); err == nil {
		t.Fatal("a hostname that sanitizes to nothing was accepted")
	}
}
