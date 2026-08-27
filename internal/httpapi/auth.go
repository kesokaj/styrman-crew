package httpapi

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/kesokaj/styrman/internal/identity"
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
func (s *Server) authGuard(mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
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
				writeErr(w, http.StatusForbidden, fmt.Errorf("%s is not authorized for styrman", email))
				return
			}
			u = identity.User{
				Email:    email,
				Username: identity.DeriveUsername(email),
				Admin:    s.users.IsAdmin(email),
			}
		} else {
			email, username := s.cfg.DefaultUser()
			u = identity.User{Email: email, Username: username, Admin: true}
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
