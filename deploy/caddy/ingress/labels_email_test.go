package ingress

import "testing"

// Emails reach the generated Caddyfile verbatim as `match email <value>`, in
// an internet-facing process that holds the docker socket. sanitizeHostname
// has stripped Caddyfile-significant bytes from the hostname since it was
// written; this is the same bar for the email path (audit 2026-09-08).
func TestParseEmailListRejectsCaddyfileBreakers(t *testing.T) {
	for _, bad := range []string{
		"a@b.c\n}\nrespond \"pwned\"", // closes the transform block
		"a@b.c }",                     // brace
		"a@b.c #comment",              // comment byte
		`a@b.c"`,                      // quote
		"a@b.c'",
		"no-at-sign",
		"two@at@signs.com",
		"trailing@dotless",
		"@nolocal.com",
		"spaced out@x.com",
	} {
		if got := parseEmailList(bad); len(got) != 0 {
			t.Errorf("parseEmailList(%q) = %v, want nothing — it reaches the Caddyfile verbatim", bad, got)
		}
	}
}

func TestParseEmailListKeepsRealAddresses(t *testing.T) {
	got := parseEmailList(" a@b.com , a@b.com ,c.d+tag@e.co.uk ")
	if len(got) != 2 || got[0] != "a@b.com" || got[1] != "c.d+tag@e.co.uk" {
		t.Fatalf("parseEmailList = %v, want the two distinct real addresses", got)
	}
}
