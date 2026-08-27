package terminal

import (
	"testing"

	"github.com/coder/websocket"
	"github.com/kesokaj/styrman/internal/config"
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
