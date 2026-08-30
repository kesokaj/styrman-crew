package orchestrator

import "testing"

// busyReason condenses the in-guest busy probe's output; "" means idle.
func TestBusyReason(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"\n":                      "",
		"  \n\n":                  "",
		"docker\n":                "docker",
		"term:node\n":             "term:node",
		"docker\nterm:node\n":     "docker,term:node",
		"term:docker\nterm:vim\n": "term:docker,term:vim",
	}
	for in, want := range cases {
		if got := busyReason(in); got != want {
			t.Errorf("busyReason(%q) = %q, want %q", in, got, want)
		}
	}
}
