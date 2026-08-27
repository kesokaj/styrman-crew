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
