package httpapi

import (
	"strings"
	"testing"
)

// formatInPlace is the allowlist that decides what command runs in the guest
// for a given extension — its analog checkerFor has had TestCheckerFor since
// it was written, this one had nothing (2026-09-01 audit). The mapping is
// security-relevant: an extension that falls through must return "" so
// formatFile rejects it, rather than composing a command from user input.
func TestFormatInPlace(t *testing.T) {
	cases := map[string]string{
		"go":       "gofmt",
		"tf":       "terraform fmt",
		"tfvars":   "terraform fmt",
		"hcl":      "terraform fmt",
		"rs":       "rustfmt",
		"py":       "black",
		"sh":       "shfmt",
		"bash":     "shfmt",
		"ts":       "prettier",
		"tsx":      "prettier",
		"js":       "prettier",
		"jsx":      "prettier",
		"mjs":      "prettier",
		"cjs":      "prettier",
		"json":     "prettier",
		"css":      "prettier",
		"scss":     "prettier",
		"less":     "prettier",
		"html":     "prettier",
		"htm":      "prettier",
		"md":       "prettier",
		"markdown": "prettier",
		"yaml":     "prettier",
		"yml":      "prettier",
		"vue":      "prettier",
		"graphql":  "prettier",
	}
	for ext, want := range cases {
		got := formatInPlace(ext)
		if got == "" {
			t.Errorf("formatInPlace(%q) = \"\" — a supported extension lost its formatter", ext)
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("formatInPlace(%q) = %q, want it to run %q", ext, got, want)
		}
		// Every command must reference the file through $F and nothing else —
		// the path is never interpolated into the string.
		if !strings.Contains(got, `"$F"`) {
			t.Errorf("formatInPlace(%q) = %q — must pass the file as a quoted \"$F\"", ext, got)
		}
	}

	// Anything not on the allowlist returns "" so formatFile 400s, rather than
	// building a command out of an attacker-supplied extension.
	for _, ext := range []string{
		"txt", "", "exe", "GO", "go;rm -rf /", "../../etc/passwd", "sh ", "yamlx",
	} {
		if got := formatInPlace(ext); got != "" {
			t.Errorf("formatInPlace(%q) = %q, want \"\" — unsupported extensions must not map to a command", ext, got)
		}
	}
}

// The two tables must not drift apart: an extension the editor offers to CHECK
// and to FORMAT should agree about which tool ecosystem it belongs to.
func TestFormatAndCheckAgreeOnGo(t *testing.T) {
	if c := checkerFor("go", "main.go", false); c == nil || c.tool != "gofmt" {
		t.Fatalf("checkerFor(go) changed unexpectedly: %+v", c)
	}
	if f := formatInPlace("go"); !strings.Contains(f, "gofmt") {
		t.Errorf("formatInPlace(go) = %q, want gofmt to match the checker", f)
	}
}

// safeSuffix and formatScript are the shell-injection surface: the extension
// comes from a user-supplied path and is interpolated into a bash script that
// runs in the guest. formatInPlace's allowlist is the first gate; these are
// the second, and check.go's comment calls them defense in depth explicitly.
func TestSafeSuffixStripsEverythingButAlnum(t *testing.T) {
	cases := map[string]string{
		"go":                 "go",
		"tsx":                "tsx",
		"":                   "txt", // never empty — mktemp --suffix=. would be odd
		";rm -rf /":          "rmrf",
		"$(whoami)":          "whoami",
		"`id`":               "id",
		"a b":                "ab",
		"../../etc":          "etc",
		"go\nrm -rf /":       "gormrf",
		"'\"$&|<>":           "txt",
		"aaaaaaaaaaaaaaaaaa": "aaaaaaaaaaaaaaaa", // capped at 16
	}
	for in, want := range cases {
		if got := safeSuffix(in); got != want {
			t.Errorf("safeSuffix(%q) = %q, want %q", in, got, want)
		}
	}
	// Whatever comes out is safe to splice into the script unquoted.
	for _, in := range []string{";rm -rf /", "$(id)", "`x`", "a|b", "a>b", "", "\x00go"} {
		for _, r := range safeSuffix(in) {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
			if !ok {
				t.Errorf("safeSuffix(%q) leaked %q into the shell script", in, r)
			}
		}
	}
}

// The generated script must read EXACTLY n bytes and refuse a short read: the
// exec stream is never half-closed, so a formatter that waited for EOF would
// hang forever, and a truncated read would silently format partial content
// back over the user's file.
func TestFormatScriptReadsAnExactByteCount(t *testing.T) {
	script := formatScript("go", 1234, `gofmt -w "$F"`)
	for _, want := range []string{
		"head -c 1234", // exact count, never EOF-driven
		`-eq 1234`,     // and it verifies what landed
		// A literal backslash-n: it is inside a single-quoted bash printf,
		// so the script carries the two characters, not a newline.
		`STYRMAN_ERR\nshort read`,
		"STYRMAN_OK",
		`gofmt -w "$F"`,
		"trap 'rm -f", // temp files cleaned even on a failure path
	} {
		if !strings.Contains(script, want) {
			t.Errorf("formatScript missing %q:\n%s", want, script)
		}
	}
	// The suffix is interpolated, so it must already be sanitized by the caller.
	if s := formatScript(safeSuffix(";rm -rf /"), 1, "true"); strings.Contains(s, "rm -rf /") {
		t.Errorf("a hostile extension reached the script body:\n%s", s)
	}
}
