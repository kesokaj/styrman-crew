package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman/internal/identity"
)

// listPorts reports the project's in-guest listening ports (≥1024).
func (s *Server) listPorts(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ports, err := s.orch.ListeningPorts(r.Context(), u.Username, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

// proxyPort reverse-proxies /proxy/{project}/{port}/... to the project
// container's IP — the port-preview feature. httputil.ReverseProxy passes
// WebSocket upgrades through, so dev servers with HMR work too. The proxy
// sits behind authGuard like every other route.
func (s *Server) proxyPort(w http.ResponseWriter, r *http.Request) {
	u := identity.FromContext(r.Context())
	// Resolve the project across owners (a shared guest doesn't own it) and
	// enforce the per-project share list: the owner always has access, a
	// shared Google email has preview-only access, everyone else is refused.
	owner, p, allowed, found := s.previewAccess(r.PathValue("project"), u)
	if !found {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such project"))
		return
	}
	if !allowed {
		writeErr(w, http.StatusForbidden, fmt.Errorf("you don't have access to this project's ports"))
		return
	}
	u.Username = owner // attach/IP lookups run in the owner's namespace
	name := p.ID
	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil || port < 1024 || port > 65535 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid port"))
		return
	}
	if fresh := s.orch.EnsureSelfAttached(r.Context(), u.Username, name); fresh {
		// The attach just reconfigured this container's network namespace —
		// this request's own connection may be doomed. Send the browser
		// around again on a fresh connection.
		http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
		return
	}
	ip, err := s.orch.IPAddress(r.Context(), u.Username, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ip == "" {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("project %s is not running", name))
		return
	}

	s.proxyFor(ip, port, proxyPrefix(r)).ServeHTTP(w, r)
}

// proxyPrefix reconstructs the mount prefix from the matched route values
// (N21) — deterministic regardless of the app's own path, which the old
// LastIndex("/<port>/") mis-trimmed when it repeated the port segment
// (e.g. /3000/x/3000/y). Shapes: /proxy/{project}/{port}/… and
// /p/{project}/{term}/port/{port}/….
func proxyPrefix(r *http.Request) string {
	project := r.PathValue("project")
	port := r.PathValue("port")
	if strings.HasPrefix(r.URL.Path, "/proxy/") {
		return "/proxy/" + project + "/" + port
	}
	return "/p/" + project + "/" + r.PathValue("term") + "/port/" + port
}

// proxyFor caches one ReverseProxy per target (devbox pattern); entries are
// cheap and pruned wholesale when stale.
func (s *Server) proxyFor(ip string, port int, prefix string) *httputil.ReverseProxy {
	key := fmt.Sprintf("%s:%d%s", ip, port, prefix)
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	if s.proxies == nil {
		s.proxies = map[string]*httputil.ReverseProxy{}
	}
	if p, ok := s.proxies[key]; ok {
		return p
	}
	if len(s.proxies) > 128 { // stale-IP hygiene after many restarts
		s.proxies = map[string]*httputil.ReverseProxy{}
	}
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", ip, port)}
	p := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// Strip the /proxy/{p}/{port} prefix so in-guest apps see
			// root-relative paths.
			pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, prefix)
			if pr.Out.URL.Path == "" {
				pr.Out.URL.Path = "/"
			}
			pr.SetXForwarded()
			// N22: never hand the user's styrman session/identity to a
			// project's app — code in a VM could capture the caddy-security
			// cookie or replay the ingress token.
			for _, h := range []string{
				"Cookie", "Authorization",
				"X-Caddy-Email", "X-Caddy-User", "X-Caddy-Role",
				"X-Caddy-Subject", "X-Caddy-Picture", "X-Styrman-Ingress",
			} {
				pr.Out.Header.Del(h)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// N22: sandbox the preview into a unique opaque origin so it is
			// NOT same-origin with the SPA — a malicious app cannot
			// fetch('/api/...') as the user or read styrman's cookies.
			resp.Header.Set("Content-Security-Policy",
				"sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("upstream: %w", err))
		},
		FlushInterval: 100 * time.Millisecond,
	}
	s.proxies[key] = p
	return p
}
