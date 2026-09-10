package httpapi

import (
	"github.com/kesokaj/styrman-crew/internal/files"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The .pub echoed to the settings page is read without following a symlink,
// but a HARD link to the private key is still read (files.ReadRawNoFollow's
// documented limit), so the content itself must look like a public key
// before it is offered for copying (security review 2026-09-10).
func TestPublicKeyLine(t *testing.T) {
	for _, ok := range []string{
		"ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjE=",
		"ssh-dss AAAAB3NzaC1kc3MAAACBAI",
		"sk-ecdsa-sha2-nistp256@openssh.com AAAAInNrLWVjZHNhLXNoYTItbmlzdHAyNTZAb3BlbnNzaC5jb20= key",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb8Q7wq1p3E7c9rZk4x2sY6Hk6s user@host",
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7",
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY= me",
		"sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29t yubikey",
	} {
		if got, accepted := publicKeyLine(ok); !accepted || got != ok {
			t.Errorf("should accept %q unchanged, got %q %v", ok, got, accepted)
		}
	}
	for _, bad := range []string{
		"ssh- AAAA",
		"sk- A",
		"sk-ant-api03-Zm9vYmFyYmF6 foobar",
		"ssh-ed25519x AAAA",
		"ecdsa-sha2-nistp999 AAAA",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW",
		"~/.ssh/id_ed25519",
		"ssh-ed25519",
		"ssh-ed25519 ",
		"ssh-ed25519 not*base64",
		"",
	} {
		if _, accepted := publicKeyLine(bad); accepted {
			t.Errorf("should reject %q", bad)
		}
	}
}

// The comment is echoed to every API consumer, so control and formatting
// characters are stripped and its whitespace collapsed server-side — the
// browser is not the only reader (TODO close-out item 7).
func TestPublicKeyLineSanitisesTheComment(t *testing.T) {
	body := "AAAAC3NzaC1lZDI1NTE5AAAAIGb8Q7wq1p3E7c9rZk4x2sY6Hk6s"
	for in, want := range map[string]string{
		"ssh-ed25519 " + body + " user\x1b[31mhost":        "ssh-ed25519 " + body + " user[31mhost",
		"ssh-ed25519 " + body + " a\u202eb\u200bc":         "ssh-ed25519 " + body + " abc",
		"ssh-ed25519 " + body + "\tmy\u00a0laptop   key\t": "ssh-ed25519 " + body + " my laptop key",
		"ssh-ed25519 " + body + " my   laptop\u2003key":    "ssh-ed25519 " + body + " my laptop key",
	} {
		got, ok := publicKeyLine(in)
		if !ok {
			t.Errorf("%q should be accepted", in)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// The handler, not only the function (release review 2026-09-10, low): a
// .pub that is a symlink to the private key, a hard link to it, or a byte
// copy of it must leave the ssh tool with no Details and not Ready; a real
// key line is echoed. Below a files.Trust base, as production runs.
func TestToolDetailsEchoesOnlyAPublicKey(t *testing.T) {
	priv := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW\n-----END OPENSSH PRIVATE KEY-----\n"
	real := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb8Q7wq1p3E7c9rZk4x2sY6Hk6s user@host\n"
	cases := []struct {
		name  string
		plant func(t *testing.T, dir string)
		want  []string
	}{
		{"symlink to the private key", func(t *testing.T, dir string) {
			if err := os.Symlink("id_ed25519", filepath.Join(dir, "id_ed25519.pub")); err != nil {
				t.Fatal(err)
			}
		}, nil},
		{"hard link to the private key", func(t *testing.T, dir string) {
			if err := os.Link(filepath.Join(dir, "id_ed25519"), filepath.Join(dir, "id_ed25519.pub")); err != nil {
				t.Skipf("hard links unsupported: %v", err)
			}
		}, nil},
		{"a copy of the private key", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte(priv), 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil},
		{"a real key", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte(real), 0o644); err != nil {
				t.Fatal(err)
			}
		}, []string{strings.TrimSpace(real)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			files.Trust(s.cfg.HomesDir)
			dir := filepath.Join(s.cfg.UserHome("tester"), ".ssh")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte(priv), 0o600); err != nil {
				t.Fatal(err)
			}
			c.plant(t, dir)
			ssh := toolsByName(t, s)["ssh"]
			if len(ssh.Details) != len(c.want) || (len(c.want) == 1 && ssh.Details[0] != c.want[0]) {
				t.Fatalf("details %q, want %q", ssh.Details, c.want)
			}
			if ssh.Ready != (len(c.want) == 1) {
				t.Fatalf("ready=%v with details %q", ssh.Ready, ssh.Details)
			}
		})
	}
}
