package httpapi

import (
	"fmt"
	"net/http"

	"github.com/kesokaj/styrman/internal/userstore"
)

// HeaderEmail is the identity header the Caddy ingress injects after Google
// OAuth (same convention as devbox). It is only trusted when auth mode is
// "header" — which requires the server to be reachable exclusively through
// the ingress (compose puts it on an internal network).
const HeaderEmail = "X-Caddy-Email"

// authGuard enforces R8's second layer: the ingress proves identity, the
// userstore proves permission. mode "none" (default) is for localhost
// development; mode "header" requires an authorized HeaderEmail on every
// request. /api/health stays open for container healthchecks.
func authGuard(mode string, users *userstore.Store, next http.Handler) http.Handler {
	if mode != "header" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		email := r.Header.Get(HeaderEmail)
		if email == "" {
			writeErr(w, http.StatusUnauthorized, fmt.Errorf("no identity header — is the ingress in front?"))
			return
		}
		if !users.IsAuthorized(email) {
			writeErr(w, http.StatusForbidden, fmt.Errorf("%s is not authorized for styrman", email))
			return
		}
		next.ServeHTTP(w, r)
	})
}
