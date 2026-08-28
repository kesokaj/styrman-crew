package orchestrator

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestHasWorkspaceBind(t *testing.T) {
	ws := "/var/lib/styrman/projects/alice/uuid-1"
	cases := []struct {
		hc   *container.HostConfig
		want bool
	}{
		{nil, false},
		{&container.HostConfig{}, false},
		{&container.HostConfig{Binds: []string{"/var/lib/styrman/homes/alice:/home/alice"}}, false},
		{&container.HostConfig{Binds: []string{
			"/var/lib/styrman/homes/alice:/home/alice",
			ws + ":/home/alice/projects/demo",
		}}, true},
		// A different project's workspace must not count.
		{&container.HostConfig{Binds: []string{
			"/var/lib/styrman/projects/alice/uuid-2:/home/alice/projects/demo",
		}}, false},
		// Prefix collision: uuid-10 must not match uuid-1.
		{&container.HostConfig{Binds: []string{
			ws + "0:/home/alice/projects/demo",
		}}, false},
	}
	for i, c := range cases {
		if got := hasWorkspaceBind(c.hc, ws); got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}
