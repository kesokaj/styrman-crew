package httpapi

import "testing"

// gitUserFromConfig feeds the commit author and the credential harvest, so a
// stricter reading than git's would silently drop a user's identity from
// every commit. These are the shapes `git config --file` accepts for the
// two keys (TODO close-out 2026-09-10, item G).
func TestGitUserFromConfig(t *testing.T) {
	for _, c := range []struct{ in, name, email string }{
		{"[user]\n\tname = Ada Lovelace\n\temail = ada@example.com\n", "Ada Lovelace", "ada@example.com"},
		{"[USER]\nName=Ada\nEMAIL=a@b\n", "Ada", "a@b"},                                     // case-insensitive, no spaces
		{"[user]\n  name   =   Ada  \n", "Ada", ""},                                         // whitespace around =
		{"[user]\nname = \"Ada \\\"the\\\" Countess\"\n", "Ada \"the\" Countess", ""},       // quoted with escapes
		{"[user]\nname = Ada # not part\nemail = a@b ; nor this\n", "Ada", "a@b"},           // comments
		{"[user]\nname = First\n[core]\n\tname = wrong\n[user]\nname = Last\n", "Last", ""}, // last wins, reopened, other sections skipped
		{"[user \"sub\"]\nname = Sub\n[user]\nemail = e@x\n", "", "e@x"},                    // a subsection is not [user]
		{"[user] ; trailing comment\nname = Ada\n", "Ada", ""},                              // header with a comment (review nit)
		{"[user]\nname = \"a\\nb\\tc\\qd\"\n", "abcqd", ""},                                 // escapes decode, then control chars are stripped
		{"", "", ""},
		{"garbage without sections\nname = nope\n", "", ""},
	} {
		name, email := gitUserFromConfig(c.in)
		if name != c.name || email != c.email {
			t.Errorf("%q: got (%q, %q), want (%q, %q)", c.in, name, email, c.name, c.email)
		}
	}
}
