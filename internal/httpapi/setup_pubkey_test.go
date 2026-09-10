package httpapi

import "testing"

// The .pub echoed to the settings page is read without following a symlink,
// but a HARD link to the private key is still read (files.ReadRawNoFollow's
// documented limit), so the content itself must look like a public key
// before it is offered for copying (security review 2026-09-10).
func TestPublicKeyLine(t *testing.T) {
	for _, ok := range []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb8Q7wq1p3E7c9rZk4x2sY6Hk6s user@host",
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7",
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY= me",
		"sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29t yubikey",
	} {
		if !publicKeyLine(ok) {
			t.Errorf("should accept %q", ok)
		}
	}
	for _, bad := range []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW",
		"~/.ssh/id_ed25519",
		"ssh-ed25519",
		"ssh-ed25519 ",
		"ssh-ed25519 not*base64",
		"",
	} {
		if publicKeyLine(bad) {
			t.Errorf("should reject %q", bad)
		}
	}
}
