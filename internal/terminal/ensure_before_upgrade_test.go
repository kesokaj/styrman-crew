package terminal

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/conversation"
	"github.com/kesokaj/styrman-crew/internal/identity"
)

type oneRecordStore struct{}

func (oneRecordStore) Get(_, _ string) (conversation.Record, bool) {
	return conversation.Record{}, true
}
func (oneRecordStore) Touch(_, _ string) {}

// The boot callback runs BEFORE the socket is upgraded: the callback is
// where the server joins the project's bridge so the login shell the
// terminal spawns can reach the credential broker (httpapi.wsBoot, BUG-060),
// and that has to be settled while the response is still plain HTTP. The
// request carries no handshake headers, so a handler that upgraded first
// would be refused by websocket.Accept's client-request check with 426 and
// an `Upgrade: websocket` header, and the callback would never run; this
// test asserts the callback was reached, with the caller's identity, and
// that its error came back as a plain 500 with no Upgrade header.
func TestBootCallbackRunsBeforeTheUpgrade(t *testing.T) {
	var got []string
	ensure := func(_ context.Context, user, projectID string) (string, string, string, error) {
		got = append(got, user+"/"+projectID)
		return "", "", "", errors.New("kata: boot timed out")
	}
	b := NewBridge(nil, &config.Config{}, slog.New(slog.DiscardHandler),
		func(string) (TermStore, error) { return oneRecordStore{}, nil },
		ensure, func(user string) string { return user }, nil)
	// boot=1 is the user-action connect that may boot a stopped project (the
	// gate short-circuits before the nil orchestrator is consulted).
	r := httptest.NewRequest(http.MethodGet, "/ws/term/p1/conv-1?boot=1", nil)
	r.SetPathValue("project", "p1")
	r.SetPathValue("term", "conv-1")
	r = identity.WithUser(r, identity.User{Username: "alice", Email: "alice@example.com", Authorized: true})
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if len(got) != 1 || got[0] != "alice/p1" {
		t.Fatalf("boot callback calls = %v, want exactly [alice/p1]", got)
	}
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "failed to start project") {
		t.Fatalf("status=%d body=%q, want a plain 500 carrying the boot failure", w.Code, w.Body.String())
	}
	if w.Header().Get("Upgrade") != "" {
		t.Fatalf("Upgrade header %q set before the boot callback's result was known", w.Header().Get("Upgrade"))
	}
}
