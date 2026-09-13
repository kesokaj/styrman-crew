package orchestrator

import (
	"os/exec"
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

// One exec per poll: the socket lines and the inner-docker lines arrive in
// one output, split at the FIRST line equal to the sentinel. A guest that
// prints the sentinel itself (a comm, an image name) can only hide its own
// ports; an output with no sentinel (an older runtime image) is all scan.
func TestSplitPortScan(t *testing.T) {
	out := "3000 node\n4100 docker-proxy\n" + portScanSentinel + "\nnginx:alpine|0.0.0.0:4100->80/tcp, :::4100->80/tcp\n" + portScanSentinel + "|0.0.0.0:5000->80/tcp\n"
	scan, images := splitPortScan(out)
	if scan != "3000 node\n4100 docker-proxy" {
		t.Fatalf("scan = %q", scan)
	}
	if !reflect.DeepEqual(images, map[int]string{4100: "nginx:alpine", 5000: portScanSentinel}) {
		t.Fatalf("images = %v", images)
	}
	if got := parseListeningPorts(scan, images); !reflect.DeepEqual(got, []Port{{Port: 3000, Process: "node"}, {Port: 4100, Image: "nginx:alpine"}}) {
		t.Fatalf("ports = %+v", got)
	}
	// No sentinel: everything is the scan, nothing is attributed.
	scan, images = splitPortScan("3000 node\n8080 java\n")
	if scan != "3000 node\n8080 java\n" || len(images) != 0 {
		t.Fatalf("no sentinel: scan %q images %v", scan, images)
	}
	// A CR before the newline (a guest with a Windows-flavoured shell) still splits.
	scan, images = splitPortScan("3000 node\r\n" + portScanSentinel + "\r\nimg|0.0.0.0:9000->80/tcp\r\n")
	if len(parseListeningPorts(scan, images)) != 1 || images[9000] != "img" {
		t.Fatalf("CRLF: scan %q images %v", scan, images)
	}
}

func TestParseDockerPorts(t *testing.T) {
	got := parseDockerPorts("nginx:alpine|0.0.0.0:8080->80/tcp, :::8080->80/tcp\nredis:7|\nbroken line\napp:dev|127.0.0.1:5432->5432/tcp\n")
	want := map[int]string{8080: "nginx:alpine", 5432: "app:dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("images = %v, want %v", got, want)
	}
	if got := parseDockerPorts(""); len(got) != 0 {
		t.Fatalf("empty = %v", got)
	}
}

// The merged script is valid shell and always emits the sentinel, with or
// without a dockerd behind it — run on this host, whose `docker ps` may or
// may not answer.
func TestPortScanScriptEmitsSentinel(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	out, err := exec.Command(sh, "-c", portScanScript).Output()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	scan, _ := splitPortScan(string(out))
	if scan == string(out) {
		t.Fatalf("no sentinel line in the script's output:\n%s", out)
	}
}
