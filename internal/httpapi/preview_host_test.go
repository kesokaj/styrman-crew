package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kesokaj/styrman/internal/config"
)

func TestPreviewHostRewrite(t *testing.T) {
	tok := "0123456789abcdef0123456789abcdef"
	s := &Server{cfg: &config.Config{PreviewBase: "preview.styrman.xyz"}}
	cases := []struct {
		host, path, want string
	}{
		{tok + ".preview.styrman.xyz", "/", "/link/" + tok + "/"},
		{tok + ".preview.styrman.xyz", "/assets/app.js", "/link/" + tok + "/assets/app.js"},
		{tok + ".preview.styrman.xyz:443", "/x", "/link/" + tok + "/x"}, // host:port
		{"app.styrman.xyz", "/api/projects", "/api/projects"},          // main host untouched
		{"nothex.preview.styrman.xyz", "/", "/"},                       // invalid token: no rewrite
	}
	for _, c := range cases {
		var got string
		h := s.previewHostRewrite(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = r.URL.Path
		}))
		req := httptest.NewRequest("GET", "http://x"+c.path, nil)
		req.Host = c.host
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != c.want {
			t.Errorf("host %q path %q → %q, want %q", c.host, c.path, got, c.want)
		}
	}

	// Off when PreviewBase is empty.
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
