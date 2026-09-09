package config

import (
	"strings"
	"testing"
)

// TestBridgeNameForGoldenVector pins the interface-name derivation to the
// same vector deploy/firewall/test.sh asserts in shell
// (`printf 'sdb-%s' "$uuid" | sha1sum | cut -c1-11`) and the live host
// carries: the root firewall script recomputes this name from a marker's
// uuid (ADR-065), so the two derivations must never drift. The shell side
// uses printf, never echo — echo's trailing newline changes the hash.
func TestBridgeNameForGoldenVector(t *testing.T) {
	const uuid = "3e6a772c-b177-4cfc-b209-b14db514bac2"
	if got := BridgeNameFor(NetworkName(uuid), false); got != "sdb-e660d58459e" {
		t.Fatalf("BridgeNameFor(NetworkName(%s), false) = %q, want sdb-e660d58459e — an EXISTING project's bridge must never be renamed (ADR-067)", uuid, got)
	}
	if got := BridgeNameFor(SetupNetwork("alice"), false); len(got) != 15 || got[:4] != "sdb-" {
		t.Fatalf("setup-VM bridge name %q must be 15 chars with the sdb- prefix (the firewall's wildcard)", got)
	}
	// The setup VM's NETWORK name is not a uuid, so no marker can ever name
	// it — that is what keeps the VM holding fresh sign-ins off the LAN.
	if ProjectFromNetworkName(SetupNetwork("alice")) != "" {
		t.Fatal("setup network must not resolve to a project id")
	}
}

// TestBridgeNameForModeIsThePrefix pins ADR-067's whole mechanism: the mode is
// the four-character prefix and nothing else. The 11-hex suffix must be
// IDENTICAL across modes, because it hashes the network name only — that is
// what lets a toggle rebuild one project's bridge without touching any other,
// and what makes `ip link` legible when both modes are present.
func TestBridgeNameForModeIsThePrefix(t *testing.T) {
	const uuid = "3e6a772c-b177-4cfc-b209-b14db514bac2"
	net := NetworkName(uuid)
	sandbox, lan := BridgeNameFor(net, false), BridgeNameFor(net, true)

	if sandbox != "sdb-e660d58459e" {
		t.Fatalf("sandbox bridge = %q, want sdb-e660d58459e", sandbox)
	}
	if lan != "lan-e660d58459e" {
		t.Fatalf("lan bridge = %q, want lan-e660d58459e", lan)
	}
	if sandbox[4:] != lan[4:] {
		t.Fatalf("suffix must not depend on the mode: sandbox %q vs lan %q", sandbox, lan)
	}
	if len(lan) != 15 {
		t.Fatalf("lan bridge %q is %d chars; the kernel caps interface names at 15", lan, len(lan))
	}
}

// TestLanBridgeEscapesTheFirewallWildcard is the security property in test
// form. The root firewall jumps on `-i sdb-+`, so a LAN bridge must NOT carry
// that prefix — and, just as important, a SANDBOX bridge must always carry it.
// If this test ever fails in the sandbox direction, projects that should be
// confined are silently on the LAN.
func TestLanBridgeEscapesTheFirewallWildcard(t *testing.T) {
	for _, uuid := range []string{
		"3e6a772c-b177-4cfc-b209-b14db514bac2",
		"00000000-0000-4000-8000-000000000000",
		"ffffffff-ffff-4fff-bfff-ffffffffffff",
	} {
		net := NetworkName(uuid)
		if got := BridgeNameFor(net, false); !strings.HasPrefix(got, "sdb-") {
			t.Fatalf("sandbox bridge %q for %s escapes the firewall wildcard — the project would be unconfined", got, uuid)
		}
		if got := BridgeNameFor(net, true); strings.HasPrefix(got, "sdb-") {
			t.Fatalf("lan bridge %q for %s still matches `-i sdb-+` — LAN mode would do nothing", got, uuid)
		}
	}
}

// TestSetupVMIsNeverLanCapable: the setup VM holds freshly harvested
// credentials, so it must stay sandboxed. Two independent guards say so — its
// network name is not a uuid (no project record can name it), and callers pass
// lan=false. This pins the first; the second is the orchestrator's.
func TestSetupVMIsNeverLanCapable(t *testing.T) {
	if ProjectFromNetworkName(SetupNetwork("alice")) != "" {
		t.Fatal("setup network must not resolve to a project id")
	}
	if got := BridgeNameFor(SetupNetwork("alice"), false); !strings.HasPrefix(got, "sdb-") || len(got) != 15 {
		t.Fatalf("setup VM bridge %q must be a 15-char sdb- name", got)
	}
}
