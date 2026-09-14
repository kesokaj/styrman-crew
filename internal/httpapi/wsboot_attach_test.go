package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// fakeWSBooter records the orchestrator calls the terminal-WS boot path makes,
// in order, so a test can pin that the server joins the project's bridge
// AFTER the boot and BEFORE the bridge upgrades the socket (BUG-060).
type fakeWSBooter struct {
	calls   []string
	bootErr error
}

func (f *fakeWSBooter) EnsureRunning(_ context.Context, user, project string, _ orchestrator.RunOpts) (string, error) {
	f.calls = append(f.calls, "EnsureRunning("+user+","+project+")")
	if f.bootErr != nil {
		return "", f.bootErr
	}
	return "id-" + project, nil
}

func (f *fakeWSBooter) EnsureSelfAttached(user, project string) bool {
	f.calls = append(f.calls, "EnsureSelfAttached("+user+","+project+")")
	return true
}

// The WS bridge's boot callback is the ONE boot path with no HTTP response to
// protect and a login shell about to run, so it must join this server to the
// project's bridge itself: the shell pulls its credentials from the broker's
// fixed address on that bridge, which answers only once the server holds it.
// Every other boot path attaches in a goroutine after its response; this one
// never did, so after every server restart the first terminal of a stopped
// project reported "the broker did not answer" (BUG-060, live 2026-09-14).
func TestWSBootAttachesTheServerToTheProjectBridge(t *testing.T) {
	f := &fakeWSBooter{}
	id, err := wsBoot(context.Background(), f, "alice", "p1", orchestrator.RunOpts{})
	if err != nil || id != "id-p1" {
		t.Fatalf("wsBoot: id=%q err=%v", id, err)
	}
	want := []string{"EnsureRunning(alice,p1)", "EnsureSelfAttached(alice,p1)"}
	if len(f.calls) != len(want) || f.calls[0] != want[0] || f.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v (boot, then attach, before the WS upgrade)", f.calls, want)
	}
}

// A boot that fails is reported as such; there is no bridge worth joining and
// the bridge answers the client with the error, not a half-attached socket.
func TestWSBootDoesNotAttachWhenTheBootFails(t *testing.T) {
	f := &fakeWSBooter{bootErr: errors.New("kata: boot timed out")}
	if _, err := wsBoot(context.Background(), f, "alice", "p1", orchestrator.RunOpts{}); err == nil {
		t.Fatal("want the boot error back")
	}
	if len(f.calls) != 1 || f.calls[0] != "EnsureRunning(alice,p1)" {
		t.Fatalf("calls = %v, want the failed boot only", f.calls)
	}
}
