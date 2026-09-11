package orchestrator

import (
	"reflect"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// The browser's two ports (ADR-068) never reach the Ports panel: the panel is
// where every preview, share and link starts, and a DevTools client owns the
// whole browser. Pure parser, so this needs no guest.
func TestParseListeningPortsHidesBrowserPorts(t *testing.T) {
	out := "3000 node\n9222 socat\n9229 chromium\n80 nginx\n4100 docker-proxy\n4100 docker-proxy\nbogus\n"
	got := parseListeningPorts(out, map[int]string{4100: "nginx:alpine"})
	want := []Port{
		{Port: 3000, Process: "node"},
		{Port: 4100, Image: "nginx:alpine"}, // docker-proxy attributed to the image, listed once
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ports = %+v, want %+v", got, want)
	}
	for _, p := range got {
		if p.Port == config.BrowserPort || p.Port == config.BrowserChromePort {
			t.Fatalf("browser port %d leaked into the list", p.Port)
		}
	}
	if got := parseListeningPorts("", nil); got == nil || len(got) != 0 {
		t.Fatalf("empty scan must be an empty (non-nil) list, got %#v", got)
	}
}
