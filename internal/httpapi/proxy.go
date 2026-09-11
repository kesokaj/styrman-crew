package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
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
	// Validate the port BEFORE the cross-owner scan so garbage input is
	// cheap-rejected without touching the filesystem / spacesMu (I8).
	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil || port < 1024 || port > 65535 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid port"))
		return
	}
	// The browser's DevTools port is owner-only through its own relay, never
	// through the (shareable, CSP-sandboxed) preview path (ADR-068).
	if err := previewPortAllowed(port); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	u := identity.FromContext(r.Context())
	// Resolve the project across owners (a shared guest doesn't own it) and
	// enforce the per-project share list: the owner always has access, a
	// shared Google email has preview-only access, everyone else is refused.
	ownerUser, p, allowed, found := s.previewAccess(r.PathValue("project"), port, u)
	if !found {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such project"))
		return
	}
	if !allowed {
		writeErr(w, http.StatusForbidden, fmt.Errorf("you don't have access to this project's ports"))
		return
	}
	// Same-origin preview (served on the main styrman host): sandbox it so a
	// malicious app can't script the SPA's API as the viewer.
	pfx := proxyPrefix(r)
	s.serveProxy(w, r, ownerUser, p.ID, port, pfx, pfx, true)
}

// serveProxy runs the shared preview-proxy path: attach in the owner's
// namespace, resolve the guest IP, and reverse-proxy to it under prefix. Used
// by both the authenticated port preview (sandbox=true, same origin as the SPA)
// and the public link handler (sandbox=false, its own token subdomain origin).
// extPrefix is the VISITOR-FACING prefix for rewriting absolute Location
// headers: equal to prefix on path-form routes, but EMPTY on a token host,
// where previewHostRewrite already folded /link/<token> into the internal
// path — using the internal prefix there doubled the token in redirects
// (2026-08-30 review).
func (s *Server) serveProxy(w http.ResponseWriter, r *http.Request, ownerUser, projectID string, port int, prefix, extPrefix string, sandbox bool) {
	if fresh := s.orch.EnsureSelfAttached(ownerUser, projectID); fresh {
		// The attach just reconfigured this container's network namespace — this
		// request's own connection may be doomed. Bounce the browser to the URL
		// it actually used. On a preview host previewHostRewrite rewrote the
		// path, so r.URL.String() would double the /link/<token> prefix (B1) —
		// redirect to the stashed original instead.
		redirectTo := r.URL.String()
		if orig, ok := r.Context().Value(origPathKey{}).(string); ok {
			redirectTo = orig
		}
		http.Redirect(w, r, redirectTo, http.StatusTemporaryRedirect)
		return
	}
	ip, err := s.orch.IPAddress(r.Context(), projectID)
	if err != nil {
		s.logger.Error("preview: ip lookup failed", "project", projectID, "error", err)
		writeErr(w, http.StatusBadGateway, fmt.Errorf("preview is not available right now"))
		return
	}
	if ip == "" {
		// S2: don't leak the project UUID / run-state to a link visitor.
		writeErr(w, http.StatusBadGateway, fmt.Errorf("preview is not available right now"))
		return
	}
	s.proxyFor(ip, port, prefix, extPrefix, sandbox).ServeHTTP(w, r)
}

// cookieDomainRe matches the `; Domain=…` attribute of a Set-Cookie header.
var cookieDomainRe = regexp.MustCompile(`(?i);\s*domain=[^;]*`)

// stripCookieDomains removes the Domain attribute from every Set-Cookie the
// previewed app returns, making its cookies host-only (S1: blocks cookie
// tossing onto parent/sibling styrman hosts).
func stripCookieDomains(resp *http.Response) {
	cs := resp.Header["Set-Cookie"]
	for i, c := range cs {
		cs[i] = cookieDomainRe.ReplaceAllString(c, "")
	}
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
func (s *Server) proxyFor(ip string, port int, prefix, extPrefix string, sandbox bool) *httputil.ReverseProxy {
	key := fmt.Sprintf("%s:%d:%t:%s|%s", ip, port, sandbox, prefix, extPrefix)
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
			// Normalize with path.Clean (v1.14.0 polish: no "../" sequences
			// reach the guest app) — trailing slash preserved, RawPath dropped
			// so the outbound URL is re-encoded from the cleaned Path, never a
			// smuggled encoding.
			p0 := strings.TrimPrefix(pr.In.URL.Path, prefix)
			if p0 == "" {
				p0 = "/"
			}
			trail := strings.HasSuffix(p0, "/") && p0 != "/"
			p0 = path.Clean(p0)
			if trail && p0 != "/" {
				p0 += "/"
			}
			pr.Out.URL.Path = p0
			pr.Out.URL.RawPath = ""
			pr.SetXForwarded()
			// N22: never hand the user's styrman session/identity to a
			// project's app — code in a VM could capture the caddy-security
			// cookie or replay the ingress token. Explicit set + a prefix sweep
			// so a future X-Caddy-* header is stripped by default, not leaked.
			for _, h := range []string{
				"Cookie", "Authorization", "Proxy-Authorization",
				"X-Styrman-Ingress",
			} {
				pr.Out.Header.Del(h)
			}
			for h := range pr.Out.Header {
				if strings.HasPrefix(http.CanonicalHeaderKey(h), "X-Caddy-") {
					pr.Out.Header.Del(h)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// The token / session lives in the URL and cookies — never let the
			// previewed app leak the capability URL via Referer (S3).
			resp.Header.Set("Referrer-Policy", "no-referrer")
			// The guest app sees Host <ip>:<port>, so its ABSOLUTE redirects
			// point at the raw guest address — unreachable (and topology-
			// leaking) for the visitor. Rewrite them back under the preview
			// prefix (v1.14.0 polish).
			if loc := resp.Header.Get("Location"); loc != "" {
				if lu, lerr := url.Parse(loc); lerr == nil && lu.Host == target.Host {
					lu.Scheme, lu.Host = "", ""
					if !strings.HasPrefix(lu.Path, "/") {
						lu.Path = "/" + lu.Path
					}
					lu.Path = extPrefix + lu.Path
					resp.Header.Set("Location", lu.String())
				}
			}
			// S1: a previewed app must not set a cookie scoped to a parent
			// domain (Domain=styrman.xyz) — that would toss a cookie onto every
			// styrman host (session fixation). Force every app cookie host-only
			// by stripping the Domain attribute.
			stripCookieDomains(resp)
			if sandbox {
				// N22: same-origin preview → sandbox into a unique opaque origin
				// so a malicious app can't fetch('/api/...') as the viewer. The
				// public-link path skips this: its token subdomain is already a
				// separate origin, and sandboxing would break the app's own
				// same-origin calls.
				resp.Header.Set("Content-Security-Policy",
					"sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			// Log the detail, serve an opaque error: the dial error carries
			// the guest IP:port, and link visitors must not learn topology
			// (audit finding; same rationale as the S2 handling above).
			s.logger.Warn("port proxy upstream error", "error", err)
			writeErr(w, http.StatusBadGateway, errors.New("preview is not available right now"))
		},
		FlushInterval: 100 * time.Millisecond,
	}
	s.proxies[key] = p
	return p
}
