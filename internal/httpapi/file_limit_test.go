package httpapi

import (
	"net/http"
	"testing"
)

// The editor's file endpoints hold up to ~12 MiB of JSON per read and accept
// 8 MiB bodies. Under the server's 4g cap (ADR-085), a few hundred of them
// in flight would OOM-kill it for every tenant (security addendum of v1.42.2,
// finding 1). They share fileConcurrency slots, taken before any body is read;
// past that they answer 429.
func TestEditorFileEndpointsAreLimitedServerWide(t *testing.T) {
	n := newNetHarness(t)
	base := "/api/projects/" + n.pid
	calls := []struct{ method, path, body string }{
		{http.MethodGet, base + "/file?path=a.txt", ""},
		{http.MethodPut, base + "/file?path=a.txt", `{"content":"x"}`},
		{http.MethodPost, base + "/format", `{"path":"a.txt","content":"x"}`},
		{http.MethodPost, base + "/check", `{"path":"a.txt","content":"x"}`},
	}
	for range cap(n.s.fileSlots.ch) {
		n.s.fileSlots.ch <- struct{}{}
	}
	for _, c := range calls {
		if w := doAs(t, n.h, n.admin, c.method, c.path, c.body); w.Code != http.StatusTooManyRequests {
			t.Errorf("%s %s with every slot taken: %d %s, want 429", c.method, c.path, w.Code, w.Body.String())
		}
	}
	for range cap(n.s.fileSlots.ch) {
		<-n.s.fileSlots.ch
	}
	for _, c := range calls {
		if w := doAs(t, n.h, n.admin, c.method, c.path, c.body); w.Code == http.StatusTooManyRequests {
			t.Errorf("%s %s on an idle server: 429", c.method, c.path)
		}
	}
	if len(n.s.fileSlots.ch) != 0 {
		t.Fatalf("finished requests kept %d slots", len(n.s.fileSlots.ch))
	}
}
