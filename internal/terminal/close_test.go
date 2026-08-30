package terminal

import (
	"errors"
	"testing"

	"github.com/coder/websocket"
	"github.com/kesokaj/styrman-crew/internal/config"
)

func TestCloseDecision(t *testing.T) {
	normal := int(websocket.StatusNormalClosure)
	cases := []struct {
		name                             string
		browserGone, done, alive, probed bool
		wantCode                         int
	}{
		{"browser gone wins", true, true, false, true, normal}, // peer gone → never 4000
		{"exec not done → reconnect", false, false, false, false, normal},
		{"real exit → 4000", false, true, false, true, config.CloseShellExited},
		{"detach (alive) → reconnect", false, true, true, true, normal},
		{"probe failed → reconnect (not stranded)", false, true, false, false, normal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := closeDecision(c.browserGone, c.done, c.alive, c.probed)
			if got != c.wantCode {
				t.Errorf("closeDecision(%v,%v,%v,%v) code = %d, want %d",
					c.browserGone, c.done, c.alive, c.probed, got, c.wantCode)
			}
		})
	}
}

// aliveFromProbe: exec error = unprobed (fail toward reconnect, I2);
// exit 0 = alive; non-zero = exited.
func TestAliveFromProbe(t *testing.T) {
	if a, p := aliveFromProbe(0, nil); !a || !p {
		t.Fatalf("exit 0 = %v,%v want alive+probed", a, p)
	}
	if a, p := aliveFromProbe(1, nil); a || !p {
		t.Fatalf("exit 1 = %v,%v want dead+probed", a, p)
	}
	if a, p := aliveFromProbe(0, errProbe); a || p {
		t.Fatalf("err = %v,%v want unprobed", a, p)
	}
}

var errProbe = errors.New("exec failed")
