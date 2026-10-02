package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// BUG-088 at the browser endpoint: the launcher's exec killed by a lifecycle
// change in flight (the project still reads running, same container) is "not
// running", not a launcher failure; with nothing in flight the same failure
// is still reported.
func TestBrowserStatusDuringALifecycleChangeIsNotRunning(t *testing.T) {
	killed := func([]string) (int, string, error) { return 9, "", nil }

	f := &fakeGuest{reply: killed, goingAway: true}
	_, h, pid := browserTest(t, f, attached("10.0.0.2:9222"))
	w := do(t, h, http.MethodGet, "/api/projects/"+pid+"/browser", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"running":false`) {
		t.Fatalf("exec killed by a lifecycle change: %d %s, want 200 running:false", w.Code, w.Body.String())
	}

	f2 := &fakeGuest{reply: killed}
	_, h2, pid2 := browserTest(t, f2, attached("10.0.0.2:9222"))
	w = do(t, h2, http.MethodGet, "/api/projects/"+pid2+"/browser", "")
	if w.Code == http.StatusOK {
		t.Fatalf("exec failure with nothing in flight must be reported, got %d %s", w.Code, w.Body.String())
	}
}
