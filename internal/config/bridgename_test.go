package config

import "testing"

// TestBridgeNameForGoldenVector pins the interface-name derivation to the
// same vector deploy/firewall/test.sh asserts in shell
// (`printf 'sdb-%s' "$uuid" | sha1sum | cut -c1-11`) and the live host
// carries: the root firewall script recomputes this name from a marker's
// uuid (ADR-065), so the two derivations must never drift. The shell side
// uses printf, never echo — echo's trailing newline changes the hash.
func TestBridgeNameForGoldenVector(t *testing.T) {
	const uuid = "3e6a772c-b177-4cfc-b209-b14db514bac2"
	if got := BridgeNameFor(NetworkName(uuid)); got != "sdb-e660d58459e" {
		t.Fatalf("BridgeNameFor(NetworkName(%s)) = %q, want sdb-e660d58459e (golden vector shared with deploy/firewall/test.sh)", uuid, got)
	}
	if got := BridgeNameFor(SetupNetwork("alice")); len(got) != 15 || got[:4] != "sdb-" {
		t.Fatalf("setup-VM bridge name %q must be 15 chars with the sdb- prefix (the firewall's wildcard)", got)
	}
	// The setup VM's NETWORK name is not a uuid, so no marker can ever name
	// it — that is what keeps the VM holding fresh sign-ins off the LAN.
	if ProjectFromNetworkName(SetupNetwork("alice")) != "" {
		t.Fatal("setup network must not resolve to a project id")
	}
}
