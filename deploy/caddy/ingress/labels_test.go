package ingress

import (
	"reflect"
	"testing"
)

func TestParseLabels_NotEnabled(t *testing.T) {
	labels := map[string]string{"some_label": "value"}
	app, err := ParseLabels(labels, "myapp", []int{8080}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app != nil {
		t.Fatal("expected nil app when caddy label is not set")
	}
}

func TestParseLabels_CaddyFalse(t *testing.T) {
	labels := map[string]string{"caddy": "false"}
	app, err := ParseLabels(labels, "myapp", []int{8080}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app != nil {
		t.Fatal("expected nil app when caddy=false")
	}
}

func TestParseLabels_MinimalPublic(t *testing.T) {
	labels := map[string]string{
		"caddy":        "true",
		"caddy_public": "true",
	}
	app, err := ParseLabels(labels, "landing", []int{80}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app == nil {
		t.Fatal("expected app, got nil")
	}
	if app.Mode != ModePublic {
		t.Errorf("expected ModePublic, got %d", app.Mode)
	}
	if app.Hostname != "landing" {
		t.Errorf("expected hostname 'landing', got %q", app.Hostname)
	}
	if app.Port != 80 {
		t.Errorf("expected port 80, got %d", app.Port)
	}
	if app.RPS != 100 {
		t.Errorf("expected default RPS 100, got %d", app.RPS)
	}
}

func TestParseLabels_Authenticated(t *testing.T) {
	labels := map[string]string{
		"caddy": "true",
	}
	app, err := ParseLabels(labels, "wiki", []int{3000}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Mode != ModeAuthenticated {
		t.Errorf("expected ModeAuthenticated, got %d", app.Mode)
	}
	if len(app.Admins) != 0 || len(app.Viewers) != 0 {
		t.Error("expected no admins/viewers for authenticated mode")
	}
}

func TestParseLabels_RoleGated(t *testing.T) {
	labels := map[string]string{
		"caddy":         "true",
		"caddy_admins":  "alice@gmail.com, bob@gmail.com",
		"caddy_viewers": "charlie@gmail.com",
		"caddy_rps":     "50",
	}
	app, err := ParseLabels(labels, "dashboard", []int{8080}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Mode != ModeRoleGated {
		t.Errorf("expected ModeRoleGated, got %d", app.Mode)
	}
	wantAdmins := []string{"alice@gmail.com", "bob@gmail.com"}
	if !reflect.DeepEqual(app.Admins, wantAdmins) {
		t.Errorf("admins: got %v, want %v", app.Admins, wantAdmins)
	}
	wantViewers := []string{"charlie@gmail.com"}
	if !reflect.DeepEqual(app.Viewers, wantViewers) {
		t.Errorf("viewers: got %v, want %v", app.Viewers, wantViewers)
	}
	if app.RPS != 50 {
		t.Errorf("expected RPS 50, got %d", app.RPS)
	}
}

func TestParseLabels_PortFromLabel(t *testing.T) {
	labels := map[string]string{
		"caddy":      "true",
		"caddy_port": "9090",
	}
	// Label takes priority over EXPOSE
	app, err := ParseLabels(labels, "myapp", []int{8080, 3000}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Port != 9090 {
		t.Errorf("expected port 9090 from label, got %d", app.Port)
	}
}

func TestParseLabels_MultipleExposePorts_NoLabel(t *testing.T) {
	labels := map[string]string{"caddy": "true"}
	_, err := ParseLabels(labels, "myapp", []int{8080, 3000}, 100)
	if err == nil {
		t.Fatal("expected error for multiple EXPOSE ports without caddy_port")
	}
}

func TestParseLabels_NoExposePorts_NoLabel(t *testing.T) {
	labels := map[string]string{"caddy": "true"}
	_, err := ParseLabels(labels, "myapp", nil, 100)
	if err == nil {
		t.Fatal("expected error for no EXPOSE ports and no caddy_port")
	}
}

func TestParseLabels_InvalidPort(t *testing.T) {
	labels := map[string]string{
		"caddy":      "true",
		"caddy_port": "abc",
	}
	_, err := ParseLabels(labels, "myapp", []int{8080}, 100)
	if err == nil {
		t.Fatal("expected error for invalid caddy_port")
	}
}

func TestParseLabels_PortOutOfRange(t *testing.T) {
	labels := map[string]string{
		"caddy":      "true",
		"caddy_port": "99999",
	}
	_, err := ParseLabels(labels, "myapp", []int{8080}, 100)
	if err == nil {
		t.Fatal("expected error for port out of range")
	}
}

func TestParseLabels_InvalidRPS(t *testing.T) {
	labels := map[string]string{
		"caddy":     "true",
		"caddy_rps": "0",
	}
	_, err := ParseLabels(labels, "myapp", []int{8080}, 100)
	if err == nil {
		t.Fatal("expected error for RPS < 1")
	}
}

func TestParseLabels_HostnameOverride(t *testing.T) {
	labels := map[string]string{
		"caddy":          "true",
		"caddy_hostname": "custom-name",
	}
	app, err := ParseLabels(labels, "container_name", []int{8080}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Hostname != "custom-name" {
		t.Errorf("expected hostname 'custom-name', got %q", app.Hostname)
	}
}

func TestParseLabels_HostnameSanitization(t *testing.T) {
	labels := map[string]string{"caddy": "true"}
	app, err := ParseLabels(labels, "/My_Container", []int{80}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Hostname != "my-container" {
		t.Errorf("expected sanitized hostname 'my-container', got %q", app.Hostname)
	}
}

func TestParseLabels_PublicOverridesRoles(t *testing.T) {
	labels := map[string]string{
		"caddy":         "true",
		"caddy_public":  "true",
		"caddy_admins":  "alice@gmail.com",
		"caddy_viewers": "bob@gmail.com",
	}
	app, err := ParseLabels(labels, "myapp", []int{80}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Mode != ModePublic {
		t.Errorf("expected ModePublic (public overrides roles), got %d", app.Mode)
	}
}

func TestParseLabels_DuplicateEmails(t *testing.T) {
	labels := map[string]string{
		"caddy":        "true",
		"caddy_admins": "alice@gmail.com, alice@gmail.com, bob@gmail.com",
	}
	app, err := ParseLabels(labels, "myapp", []int{80}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantAdmins := []string{"alice@gmail.com", "bob@gmail.com"}
	if !reflect.DeepEqual(app.Admins, wantAdmins) {
		t.Errorf("expected deduplicated admins %v, got %v", wantAdmins, app.Admins)
	}
}

func TestParseLabels_CaddyTrueVariants(t *testing.T) {
	for _, val := range []string{"true", "True", "TRUE", "yes", "YES", "1"} {
		labels := map[string]string{"caddy": val}
		app, err := ParseLabels(labels, "myapp", []int{80}, 100)
		if err != nil {
			t.Fatalf("caddy=%q: unexpected error: %v", val, err)
		}
		if app == nil {
			t.Fatalf("caddy=%q: expected app, got nil", val)
		}
	}
}

func TestParseEmailList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"alice@gmail.com", []string{"alice@gmail.com"}},
		{"alice@gmail.com, bob@gmail.com", []string{"alice@gmail.com", "bob@gmail.com"}},
		{" alice@gmail.com , , bob@gmail.com , ", []string{"alice@gmail.com", "bob@gmail.com"}},
	}
	for _, tt := range tests {
		got := parseEmailList(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseEmailList(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"/myapp", "myapp"},
		{"My_App", "my-app"},
		{"/My_Complex_App", "my-complex-app"},
		{"already-clean", "already-clean"},
	}
	for _, tt := range tests {
		got := sanitizeHostname(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeHostname(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseLabels_ReservedHostnameBlocked(t *testing.T) {
	// User container trying to claim "shell" or "auth" should be rejected
	for _, reserved := range []string{"shell", "auth"} {
		labels := map[string]string{
			"caddy":          "true",
			"caddy_hostname": reserved,
			"caddy_port":     "8080",
			"caddy_public":   "true",
		}
		_, err := ParseLabels(labels, "my-user-app", []int{8080}, 100)
		if err == nil {
			t.Errorf("expected error for reserved hostname %q on non-devbox container, got nil", reserved)
		}
	}
}

func TestParseLabels_ReservedHostnameAllowedForInfra(t *testing.T) {
	// devbox-launcher should be allowed to use "shell"
	labels := map[string]string{
		"caddy":          "true",
		"caddy_hostname": "shell",
		"caddy_port":     "3000",
		"caddy_admins":   "admin@example.com",
	}
	app, err := ParseLabels(labels, "devbox-launcher", []int{3000}, 100)
	if err != nil {
		t.Fatalf("expected devbox- container to use reserved hostname, got error: %v", err)
	}
	if app == nil {
		t.Fatal("expected app, got nil")
	}
	if app.Hostname != "shell" {
		t.Errorf("expected hostname 'shell', got %q", app.Hostname)
	}
}
