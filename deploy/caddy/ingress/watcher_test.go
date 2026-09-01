package ingress

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testWatcher() *Watcher {
	return &Watcher{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		// Long enough that no retry timer fires inside a test run — these
		// tests assert the DECISIONS, not the timer.
		retryDelay: time.Hour,
		fatal:      make(chan error, 1),
	}
}

// The 2026-09-01 outage: at boot the initial /load was REJECTED (400 — no
// network for the OIDC fetch) and, in the old binary, reported as success and
// latched. A rejection must surface as an error — and NOT as admin-unreachable,
// because the admin answered and in-process retry will heal it.
func TestReloadCaddyRejectedLoadIsAnErrorButNotUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Error(rw, "loading config: no network", http.StatusBadRequest)
	}))
	defer srv.Close()

	w := testWatcher()
	w.adminAddr = strings.TrimPrefix(srv.URL, "http://")
	err := w.reloadCaddy(context.Background(), "irrelevant")
	if err == nil {
		t.Fatal("a rejected load must be an error — reporting it as success is the outage")
	}
	if errors.Is(err, errAdminUnreachable) {
		t.Fatalf("a 400 means the admin ANSWERED; got unreachable: %v", err)
	}
}

func TestReloadCaddySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := testWatcher()
	w.adminAddr = strings.TrimPrefix(srv.URL, "http://")
	if err := w.reloadCaddy(context.Background(), "irrelevant"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestReloadCaddyTransportFailureIsUnreachable(t *testing.T) {
	// A closed port: reserve one with a listener, then shut it down.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()

	w := testWatcher()
	w.adminAddr = addr
	err := w.reloadCaddy(context.Background(), "irrelevant")
	if !errors.Is(err, errAdminUnreachable) {
		t.Fatalf("connection refused must be errAdminUnreachable, got %v", err)
	}
}

// The admin listener lives in the watcher's own process: once it is gone no
// reload can ever land again, so after adminDownLimit consecutive transport
// failures the watcher must report fatal (process restart is the only heal).
// Rejected loads, however many, must never do that — they retry forever.
func TestConsecutiveUnreachableTurnsFatalButRejectionsDoNot(t *testing.T) {
	ctx := context.Background()

	w := testWatcher()
	for i := 0; i < adminDownLimit*2; i++ {
		w.noteReloadFailure(ctx, errors.New("caddy reload failed with status 400"))
	}
	select {
	case err := <-w.Fatal():
		t.Fatalf("rejected loads must retry, not kill the process: %v", err)
	default:
	}

	for i := 0; i < adminDownLimit-1; i++ {
		w.noteReloadFailure(ctx, errAdminUnreachable)
	}
	select {
	case err := <-w.Fatal():
		t.Fatalf("fatal fired one failure early: %v", err)
	default:
	}
	w.noteReloadFailure(ctx, errAdminUnreachable)
	select {
	case <-w.Fatal():
	default:
		t.Fatalf("%d consecutive transport failures must report fatal", adminDownLimit)
	}

	// And a success in between resets the count (checked via the field: a
	// docker-backed scanAndReload is not constructible here).
	w2 := testWatcher()
	w2.noteReloadFailure(ctx, errAdminUnreachable)
	w2.noteReloadFailure(ctx, errors.New("status 400")) // admin answered → reset
	if w2.adminDown != 0 {
		t.Fatalf("an answering admin must reset the down-counter, got %d", w2.adminDown)
	}
	t.Cleanup(func() {
		for _, x := range []*Watcher{w, w2} {
			x.retryMu.Lock()
			if x.retryTimer != nil {
				x.retryTimer.Stop()
			}
			x.retryMu.Unlock()
		}
	})
}

// Mutation guard for the outage fix itself: reviewer round 2 proved that
// deleting scheduleRetry from noteReloadFailure left the suite green. A
// rejected reload MUST arm the retry timer — without it the 2026-09-01 boot
// race never self-heals — and the fatal path must NOT arm one (the process is
// exiting; a timer would race the exit).
func TestRejectedReloadArmsRetryAndFatalDoesNot(t *testing.T) {
	ctx := context.Background()

	w := testWatcher()
	w.noteReloadFailure(ctx, errors.New("caddy reload failed with status 400"))
	w.retryMu.Lock()
	armed := w.retryTimer != nil
	if w.retryTimer != nil {
		w.retryTimer.Stop()
	}
	w.retryMu.Unlock()
	if !armed {
		t.Fatal("a rejected reload must arm the retry timer")
	}

	w2 := testWatcher()
	w2.adminDown = adminDownLimit - 1 // one transport failure away from fatal
	w2.noteReloadFailure(ctx, errAdminUnreachable)
	select {
	case <-w2.Fatal():
	default:
		t.Fatal("expected the fatal report")
	}
	w2.retryMu.Lock()
	defer w2.retryMu.Unlock()
	if w2.retryTimer != nil {
		w2.retryTimer.Stop()
		t.Error("the fatal path must not arm a retry")
	}
}
