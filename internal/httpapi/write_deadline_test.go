package httpapi

import (
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

// The write deadline acquireSlot sets is what frees a slot whose client never
// reads its response: without it the handler blocks in Write for as long as
// the connection lives, holding the slot and the response's memory. This
// needs a real server (httptest's recorder cannot take a deadline) and a
// response larger than the loopback socket buffers.
func TestASlowReaderReleasesItsSlotAtTheWriteDeadline(t *testing.T) {
	defer func(d time.Duration) { historyWriteTimeout = d }(historyWriteTimeout)
	historyWriteTimeout = time.Second
	n := newNetHarness(t)
	logs := filepath.Join(n.state, "homes", identity.DeriveUsername(n.admin), "p", n.pid, ".styrman", "logs", n.pid)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	// '<' JSON-escapes to six bytes: 4 MiB of it is a ~24 MiB response.
	if err := os.WriteFile(filepath.Join(logs, "conv-1.log"), []byte(strings.Repeat("<", 4<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(n.h)
	defer srv.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /api/projects/%s/history/conv-1?bytes=%d HTTP/1.1\r\nHost: x\r\n%s: %s\r\n%s: test-ingress-token\r\n\r\n",
		n.pid, 4<<20, HeaderEmail, n.admin, HeaderIngress)
	// Read nothing. Positive control first: the request did take a slot (a
	// write that never completes holds it until the deadline).
	taken := time.Now().Add(5 * time.Second)
	for len(n.s.readSlots) != 1 {
		if time.Now().After(taken) {
			t.Fatal("the request never took a read slot")
		}
		time.Sleep(2 * time.Millisecond)
	}
	deadline := time.Now().Add(8 * time.Second)
	for len(n.s.readSlots) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the slot is still held 8 s after a %v write deadline", historyWriteTimeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
