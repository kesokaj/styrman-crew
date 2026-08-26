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
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ports, err := s.orch.ListeningPorts(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sort.Ints(ports)
	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

// proxyPort reverse-proxies /proxy/{project}/{port}/... to the project
// container's IP — the port-preview feature. httputil.ReverseProxy passes
// WebSocket upgrades through, so dev servers with HMR work too. The proxy
// sits behind authGuard like every other route.
func (s *Server) proxyPort(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	if _, err := s.projects.Get(name); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil || port < 1024 || port > 65535 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid port"))
		return
	}
	ip, err := s.orch.IPAddress(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ip == "" {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("project %s is not running", name))
		return
	}

	prefix := "/proxy/" + name + "/" + strconv.Itoa(port)
	s.proxyFor(ip, port, prefix).ServeHTTP(w, r)
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
