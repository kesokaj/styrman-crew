package orchestrator

import "testing"

func TestContainerGuest(t *testing.T) {
	cases := []struct {
		env  []string
		want string
	}{
		{[]string{"PATH=/bin", "STYRMAN_USER=simon", "HOME=/home/simon"}, "simon"},
		{[]string{"STYRMAN_USER="}, ""},
		{[]string{"PATH=/bin"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := containerGuest(c.env); got != c.want {
			t.Errorf("containerGuest(%v) = %q, want %q", c.env, got, c.want)
		}
	}
}

func TestRestartPolicy(t *testing.T) {
	if p := restartPolicy(false); p.Name != "unless-stopped" {
		t.Errorf("project VM policy = %q, want unless-stopped", p.Name)
	}
	if p := restartPolicy(true); p.Name != "no" {
		t.Errorf("ephemeral VM policy = %q, want no", p.Name)
	}
}

// The hostname is user-visible on every prompt line and in the terminal status
// bar, so it carries the project's name alone — no platform prefix — and stays
// a legal DNS label.
func TestGuestHostname(t *testing.T) {
	cases := map[string]string{
		"styrman-harness": "styrman-harness", // NOT "styrman-styrman-harness"
		"api":             "api",
		"my-project-":     "my-project", // ValidateName allows a trailing dash; a hostname must not
		"-":               "project",    // never blank
	}
	for in, want := range cases {
		if got := guestHostname(in); got != want {
			t.Errorf("guestHostname(%q) = %q, want %q", in, got, want)
		}
	}
}
