package httpapi

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

// HeaderEmail is the identity header the Caddy ingress injects after Google
// OAuth (same convention as devbox). It is only trusted when auth mode is
// "header" — which requires the server to be reachable exclusively through
// the ingress (compose puts it on an internal network).
const HeaderEmail = "X-Caddy-Email"

// HeaderIngress carries the optional shared secret proving a request came
// through the Caddy ingress (see config.IngressToken).
const HeaderIngress = "X-Styrman-Ingress"

// authGuard resolves the request identity and enforces R8's second layer:
// the ingress proves identity, the userstore proves permission. Mode "none"
// (loopback dev listener, local dev) acts as the configured default user
// with admin rights; mode "header" requires an authorized HeaderEmail.
// /api/health stays open for container healthchecks.
func (s *Server) authGuard(mode string, mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		// Public preview links: the unguessable token IS the capability, so
		// these need no identity. But they must still prove they came through
		// the ingress (S7) — otherwise a project VM on the shared bridge could
		// hit /link/<token> on :8080 directly and be proxied into ANOTHER
		// project's port, defeating R12. The ingress injects the token on the
		// preview block; a direct VM request lacks it.
		// Judged by the ROUTE the request takes, never its decoded path: a
		// raw "/link/../x" is not a link (audit 2026-09-13), and neither is
		// ".../terminals/..%2F..%2F..%2F..%2Flink%2Fx", which cleans to
		// /link/x but routes to an API handler (security addendum
		// 07f5517..561c23b, finding 1; ADR-086 amendment 2).
		route := routePath(mux, r)
		if isLinkPath(route) {
			if s.cfg.IngressToken != "" &&
				subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderIngress)), []byte(s.cfg.IngressToken)) != 1 {
				writeErr(w, http.StatusForbidden, fmt.Errorf("request did not come through the ingress"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		var u identity.User
		if mode == "header" {
			// Ingress token (opt-in): the server container is joined to project
			// bridges for the port-preview proxy, so :8080 is reachable from a
			// project VM which could otherwise spoof X-Caddy-Email. When a
			// token is configured, only the ingress (which injects it) is
			// trusted; a direct request from a project VM lacks it.
			if s.cfg.IngressToken != "" &&
				subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderIngress)), []byte(s.cfg.IngressToken)) != 1 {
				writeErr(w, http.StatusForbidden, fmt.Errorf("request did not come through the ingress"))
				return
			}
			email := r.Header.Get(HeaderEmail)
			if email == "" {
				writeErr(w, http.StatusUnauthorized, fmt.Errorf("no identity header — is the ingress in front?"))
				return
			}
			if !s.users.IsAuthorized(email) {
				// A Google user who isn't a styrman user may still reach a
				// project's port PREVIEW if its owner shared it with them
				// (proxyPort enforces the per-project share list). Every other
				// route is refused — they never see the dashboard, terminals,
				// or files. Authorized stays false and Username is left empty so
				// a slug collision can never satisfy an ownership check.
				if !isPreviewPath(route) && !isSharedPath(route) {
					writeErr(w, http.StatusForbidden, fmt.Errorf("%s is not authorized for styrman", email))
					return
				}
				u = identity.User{Email: email}
				next.ServeHTTP(w, identity.WithUser(r, u))
				return
			}
			u = identity.User{
				Email:      email,
				Username:   identity.DeriveUsername(email),
				Admin:      s.users.IsAdmin(email),
				Authorized: true,
			}
		} else if mode == "none" {
			email, username := s.cfg.DefaultUser()
			u = identity.User{Email: email, Username: username, Admin: true, Authorized: true}
		} else {
			// Unknown mode — fail closed. config.Load already rejects anything
			// but "none"/"header" (M16); this is the defense-in-depth backstop so
			// a future mode can never default to the admin dev-door.
			writeErr(w, http.StatusUnauthorized, fmt.Errorf("server auth misconfigured"))
			return
		}
		next.ServeHTTP(w, identity.WithUser(r, u))
	})
}

// requireAdmin gates admin-only handlers (user management).
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !identity.FromContext(r.Context()).Admin {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return false
	}
	return true
}
