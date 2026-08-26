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
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
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

	s.proxyFor(ip, port, proxyPrefix(r, port)).ServeHTTP(w, r)
}

// proxyPrefix reconstructs the mount prefix from the request path so the
// same handler serves every shape: /proxy/{p}/{port}/…,
// /p/{p}/port/{port}/… and /p/{p}/{term}/port/{port}/…. The port segment is
// always last, hence LastIndex (a terminal id could be numeric too).
func proxyPrefix(r *http.Request, port int) string {
	seg := "/" + strconv.Itoa(port) + "/"
	if i := strings.LastIndex(r.URL.Path, seg); i >= 0 {
		return r.URL.Path[:i+len(seg)-1]
	}
	return strings.TrimSuffix(r.URL.Path, "/")
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
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("upstream: %w", err))
		},
		FlushInterval: 100 * time.Millisecond,
	}
	s.proxies[key] = p
	return p
}
