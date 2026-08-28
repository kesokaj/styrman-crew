package conversation

import (
	"fmt"
	"math/rand/v2"
)

// Friendly terminal names (operator item 4, 2026-08-28): a new terminal gets
// a memorable generated name immediately instead of "Terminal 3" — rename
// any time. Docker-style adjective-noun, themed to fit the crew identity.

var nameAdjectives = []string{
	"brisk", "bold", "calm", "clever", "deft", "eager", "fleet", "keen",
	"lively", "lucid", "nimble", "quick", "quiet", "sharp", "steady", "swift",
	"tidy", "trusty", "vivid", "wry", "bright", "crisp", "daring", "earnest",
	"gentle", "hardy", "jolly", "merry", "patient", "sturdy",
}

var nameNouns = []string{
	"anchor", "beacon", "breeze", "buoy", "compass", "current", "dinghy",
	"dock", "ensign", "fathom", "galley", "harbor", "helm", "jib", "keel",
	"knot", "lantern", "mast", "pennant", "quay", "rudder", "sextant",
	"skiff", "sloop", "spinnaker", "tide", "tiller", "wake", "windlass",
	"yawl",
}

// friendlyName returns an adjective-noun name not in taken; after a bounded
// number of collisions it appends the terminal id instead of looping.
func friendlyName(taken map[string]bool, id string) string {
	for range 24 {
		n := nameAdjectives[rand.IntN(len(nameAdjectives))] + " " +
			nameNouns[rand.IntN(len(nameNouns))]
		if !taken[n] {
			return n
		}
	}
	return fmt.Sprintf("%s %s %s",
		nameAdjectives[rand.IntN(len(nameAdjectives))],
		nameNouns[rand.IntN(len(nameNouns))], id)
}
