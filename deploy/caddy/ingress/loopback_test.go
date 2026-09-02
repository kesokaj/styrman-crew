package ingress

import (
	"net"
	"strings"
	"testing"
)

// The admin API reconfigures the whole ingress, in a process that runs
// network_mode:host holding the docker socket. A non-loopback bind hands the
// platform to anyone who can reach the port — the audit's high finding.
func TestIsLoopbackAddr(t *testing.T) {
	loopback := []string{
		"127.0.0.1:2019", "[::1]:2019", "127.53.0.1:2019",
	}
	notLoopback := []string{
		":2019",        // every interface — the dangerous default shape
		"0.0.0.0:2019", //
		"[::]:2019",    //
		"192.168.1.5:2019",
		"10.0.0.1:2019",
		"example.com:2019",
		// A NAME is refused even when it conventionally resolves to loopback.
		// The guard cannot verify a name: /etc/hosts and NSS decide what it
		// binds, at bind time, so accepting it on a string match asserted the
		// contract instead of establishing it (tightened 2026-09-02). The
		// default is now the 127.0.0.1 literal.
		"localhost:2019",
		"LOCALHOST:2019",
		"localhost.:2019",
		"127.0.0.1", // bare host, no port — would bind :80, must be rejected
		"::1",       //
		"",
		// SplitHostPort accepts any port STRING; the value flows verbatim into
		// the generated Caddyfile, so a non-numeric or out-of-range "port"
		// must be refused (security review).
		"127.0.0.1:port",
		"127.0.0.1:0",
		"127.0.0.1:65536",
		"localhost:2019 {",
	}
	for _, a := range loopback {
		if !isLoopbackAddr(a) {
			t.Errorf("isLoopbackAddr(%q) = false, want true", a)
		}
	}
	for _, a := range notLoopback {
		if isLoopbackAddr(a) {
			t.Errorf("isLoopbackAddr(%q) = true — this would expose the admin API", a)
		}
	}
}

// The generated config emits the admin block (defense-in-depth against
// browser-origin requests). This does NOT by itself gate the admin API — the
// loadConfig loopback guard and the >=2.11.3 floor do — but the block must be
// present and well-formed, and must never be emitted for a non-loopback addr.
func TestGeneratedConfigNamesAdminOrigins(t *testing.T) {
	cfg := testConfig()
	cfg.AdminListen = "127.0.0.1:2019"
	out := GenerateCaddyfile(nil, cfg)
	if !strings.Contains(out, "admin 127.0.0.1:2019") || !strings.Contains(out, "origins 127.0.0.1:2019") {
		t.Errorf("generated Caddyfile omits the admin block:\n%s", out)
	}
}

// The shipped default must itself pass the guard — otherwise the ingress
// refuses to start on a host that sets no CADDY_ADMIN override, which is the
// normal deployment (verified 2026-09-02: the live container, the production
// .env and .env.example all leave it unset).
func TestDefaultCaddyAdminPassesTheGuard(t *testing.T) {
	if !isLoopbackAddr(DefaultCaddyAdmin) {
		t.Fatalf("DefaultCaddyAdmin = %q does not pass isLoopbackAddr — the ingress would not start", DefaultCaddyAdmin)
	}
	if net.ParseIP(strings.Split(DefaultCaddyAdmin, ":")[0]) == nil {
		t.Errorf("DefaultCaddyAdmin = %q must be an IP literal, not a name", DefaultCaddyAdmin)
	}
}
