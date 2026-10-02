package ingress

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// BUG-087, the 2026-10-02 outage: crew was down for six hours after a host
// boot, with the ingress logging "caddy reloaded successfully" and then
// "caddyfile unchanged, skipping reload" forever.
//
// Caddy's /load adapts the Caddyfile first and, when the adapter has
// WARNINGS, writes them to the response before it loads anything
// (caddyconfig/load.go, handleLoad, caddy v2.11.4) — which sends a 200. A load
// that then fails (at boot: no network for the OIDC metadata fetch) can no
// longer change the status; caddy logs "superfluous response.WriteHeader" and
// only the BODY carries the error object. The 2026-09-01 fix checked the
// status alone, so this failure read as success and was latched.

// The body caddy sent that morning, in shape: the warnings array, then the
// APIError object, under a 200.
const maskedFailureBody = `[{"file":"Caddyfile","line":2,"message":"Caddyfile input is not formatted; run 'caddy fmt --overwrite' to fix inconsistencies"}]` +
	`{"error":"loading config: loading new config: loading http app module: provision http: server srv0: failed to fetch metadata for OAuth 2.0 authorization server"}` + "\n"

func maskedAdmin(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Write first, as caddy does with its warnings: the status is 200
		// from here on, whatever happens next.
		_, _ = rw.Write([]byte(body))
	}))
}

func TestReloadCaddyFailedLoadBehindA200IsAnError(t *testing.T) {
	srv := maskedAdmin(maskedFailureBody)
	defer srv.Close()

	w := testWatcher()
	w.adminAddr = strings.TrimPrefix(srv.URL, "http://")
	err := w.reloadCaddy(context.Background(), "irrelevant")
	if err == nil {
		t.Fatal("a load caddy reports as failed in the body must be an error, whatever the status — reporting it as success is the outage")
	}
	if errors.Is(err, errAdminUnreachable) {
		t.Fatalf("the admin ANSWERED; a masked failure is a rejection (retried in-process), got unreachable: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to fetch metadata") {
		t.Errorf("the error should carry caddy's own message so the log says why: %v", err)
	}
}

// Warnings alone are not a failure: the same 200, the same array, no error
// object. Treating them as one would reload caddy every retry tick forever.
func TestReloadCaddyWarningsAloneAreSuccess(t *testing.T) {
	srv := maskedAdmin(`[{"file":"Caddyfile","line":2,"message":"Caddyfile input is not formatted; run 'caddy fmt --overwrite' to fix inconsistencies"}]`)
	defer srv.Close()

	w := testWatcher()
	w.adminAddr = strings.TrimPrefix(srv.URL, "http://")
	if err := w.reloadCaddy(context.Background(), "irrelevant"); err != nil {
		t.Fatalf("warnings without an error object are a successful load, got %v", err)
	}
}

// loadError reads what caddy's /load wrote: any number of JSON values, of
// which only an object with a non-empty "error" string means failure.
func TestLoadErrorReadsOnlyCaddysErrorObject(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"empty body", "", ""},
		{"warnings only", `[{"message":"not formatted"}]`, ""},
		{"error only", `{"error":"loading config: boom"}`, "loading config: boom"},
		{"warnings then error", `[{"message":"w"}]{"error":"loading config: boom"}`, "loading config: boom"},
		{"an empty error string is not a failure", `{"error":""}`, ""},
		{"an object without error", `{"result":"ok"}`, ""},
		{"error is not a string", `{"error":42}`, ""},
		{"not JSON at all", `load complete`, ""},
		{"a warning that merely mentions error", `[{"message":"\"error\": not really"}]`, ""},
	}
	for _, c := range cases {
		if got := loadError([]byte(c.body)); got != c.want {
			t.Errorf("%s: loadError = %q, want %q", c.name, got, c.want)
		}
	}
}
