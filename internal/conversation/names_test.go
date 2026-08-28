package conversation

import (
	"strings"
	"testing"
)

func TestFriendlyNameAvoidsCollisions(t *testing.T) {
	taken := map[string]bool{}
	for range 100 {
		n := friendlyName(taken, "x")
		if taken[n] {
			t.Fatalf("duplicate name generated: %q", n)
		}
		taken[n] = true
	}
}

func TestFriendlyNameFallbackWhenExhausted(t *testing.T) {
	// Every combination taken → the fallback must still return something
	// unique-ish by appending the id.
	taken := map[string]bool{}
	for _, a := range nameAdjectives {
		for _, n := range nameNouns {
			taken[a+" "+n] = true
		}
	}
	got := friendlyName(taken, "7")
	if got == "" || !strings.HasSuffix(got, " 7") {
		t.Fatalf("fallback name = %q, want ' 7' suffix", got)
	}
}
