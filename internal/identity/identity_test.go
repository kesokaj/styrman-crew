package identity

import "testing"

// DeriveUsername is the ONLY derivation from email to username since the
// config-package duplicate was deleted (2026-09-01 audit: the duplicate's
// "import cycle" justification was false, and divergence would have resolved
// one email to two state dirs). It keys per-user homes, state dirs and broker
// isolation, so its output must be a safe Linux username and must be stable.
func TestDeriveUsername(t *testing.T) {
	cases := map[string]string{
		"ostling.simon@gmail.com": "ostling-simon",
		"Simon.Ostling@GMAIL.com": "simon-ostling", // case-folded
		"a_b-c@example.org":       "a_b-c",         // _ and - survive
		"user+tag@example.org":    "usertag",       // + dropped
		"123@example.org":         "user-123",      // must start with a letter
		"@example.org":            "user-",         // empty local part
		"":                        "user-",
		// Dots become dashes BEFORE unsafe runes are dropped, so the slashes
		// leave dashes behind. What matters is that no path separator survives.
		"../../etc/passwd@x.io": "user-----etcpasswd",
		"a b@x.io":              "ab", // whitespace dropped
	}
	for in, want := range cases {
		if got := DeriveUsername(in); got != want {
			t.Errorf("DeriveUsername(%q) = %q, want %q", in, got, want)
		}
	}

	// Length is capped so the slug stays a legal username, and the result is
	// always a safe slug whatever the input.
	long := DeriveUsername(string(make([]byte, 0)) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@x.io")
	if len(long) > 32 {
		t.Errorf("DeriveUsername did not cap length: %d", len(long))
	}
	for _, in := range []string{"ostling.simon@gmail.com", "../x@y.io", "!!!@y.io", "@", ""} {
		got := DeriveUsername(in)
		if got == "" {
			t.Errorf("DeriveUsername(%q) returned an empty username", in)
		}
		if got[0] < 'a' || got[0] > 'z' {
			t.Errorf("DeriveUsername(%q) = %q — must start with a letter", in, got)
		}
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				t.Errorf("DeriveUsername(%q) = %q contains unsafe rune %q", in, got, r)
			}
		}
	}

	// Deterministic: the same email must always resolve to the same state dir.
	if DeriveUsername("ostling.simon@gmail.com") != DeriveUsername("ostling.simon@gmail.com") {
		t.Error("DeriveUsername is not deterministic")
	}
}
