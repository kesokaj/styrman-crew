package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

func TestPreviewHostRewrite(t *testing.T) {
	tok := "0123456789abcdef0123456789abcdef"
	s := &Server{cfg: &config.Config{PreviewBase: "preview.styrman.xyz"}}

	// Valid-token preview hosts rewrite to /link/<token>/... (next runs).
	rewrites := []struct{ host, path, want string }{
		{tok + ".preview.styrman.xyz", "/", "/link/" + tok + "/"},
		{tok + ".preview.styrman.xyz", "/assets/app.js", "/link/" + tok + "/assets/app.js"},
		{tok + ".preview.styrman.xyz:443", "/x", "/link/" + tok + "/x"},
		{"app.styrman.xyz", "/api/projects", "/api/projects"}, // main host untouched
	}
	for _, c := range rewrites {
		var got string
		called := false
		h := s.previewHostRewrite(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called, got = true, r.URL.Path
		}))
		req := httptest.NewRequest("GET", "http://x"+c.path, nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !called || got != c.want {
			t.Errorf("host %q path %q → %q (called=%v), want %q", c.host, c.path, got, called, c.want)
		}
	}

	// A preview host whose subdomain isn't a valid token serves NOTHING (S6):
	// 404, next never runs.
	for _, host := range []string{"nothex.preview.styrman.xyz", "x.y.preview.styrman.xyz"} {
		called := false
		h := s.previewHostRewrite(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
		req := httptest.NewRequest("GET", "http://x/api/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if called || rec.Code != http.StatusNotFound {
			t.Errorf("invalid-token host %q: code=%d called=%v, want 404 + not called", host, rec.Code, called)
		}
	}

	// Off when PreviewBase is empty — even a token-shaped host passes untouched.
	s2 := &Server{cfg: &config.Config{}}
	var got string
	h := s2.previewHostRewrite(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.URL.Path }))
	req := httptest.NewRequest("GET", "http://x/", nil)
	req.Host = tok + ".preview.styrman.xyz"
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "/" {
		t.Errorf("disabled rewrite changed path to %q", got)
	}
}
