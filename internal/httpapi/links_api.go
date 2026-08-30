package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/links"
)

// ttlChoices bounds how long a public link may live. Keep the set small and
// finite so a link can't be minted to live effectively forever.
var ttlChoices = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// linkURL renders the URL for a link on its kind's host base — public tokens
// on STYRMAN_PREVIEW_BASE, private ones on STYRMAN_PRIVATE_BASE (each a
// per-token subdomain, its own origin). Without a base it's the path form,
// which the server serves directly (local/dev before the wildcard host exists).
func (s *Server) linkURL(l links.Link) string {
	if l.Private() {
		if s.cfg.PrivateBase != "" {
			return fmt.Sprintf("https://%s.%s/", l.Token, s.cfg.PrivateBase)
		}
		return "/shared/" + l.Token + "/"
	}
	if s.cfg.PreviewBase != "" {
		return fmt.Sprintf("https://%s.%s/", l.Token, s.cfg.PreviewBase)
	}
	return "/link/" + l.Token + "/"
}

func (s *Server) linkJSON(l links.Link) map[string]any {
	return map[string]any{
		"token":   l.Token,
		"port":    l.Port,
		"url":     s.linkURL(l),
		"expires": l.Expires,
	}
}

// createLink mints a public TTL link for one exposed port (owner only).
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("public links unavailable"))
		return
	}
	// Fail loudly instead of minting a lie (v1.14.0 polish): without a
	// public wildcard host, the path-form /link/<token> on the MAIN host
	// sits behind the ingress OAuth gate in header mode — the "public" link
	// would silently require a Google login. Dev (auth none) keeps the
	// path form, which the server serves directly.
	if s.cfg.PreviewBase == "" && s.cfg.AuthMode == "header" {
		writeErr(w, http.StatusServiceUnavailable,
			fmt.Errorf("public links need STYRMAN_PUBLIC_DOMAIN configured on the ingress"))
		return
	}
	port, err := sharePort(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	u, _, p, err := s.getProjectFor(r) // owner-only (requester's own store)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		TTL string `json:"ttl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ttl, ok := ttlChoices[body.TTL]
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("ttl must be one of 1h, 24h, 7d"))
		return
	}
	l, err := s.links.Create(p.ID, u.Username, port, u.Email, ttl, time.Now())
	if errors.Is(err, links.ErrTooManyLinks) {
		writeErr(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.logger.Info("public link created", "by", u.Email, "project", p.ID,
		"port", port, "ttl", body.TTL, "expires", l.Expires)
	writeJSON(w, http.StatusOK, s.linkJSON(l))
}

// listLinks lists a project's live public links (owner only).
func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		writeJSON(w, http.StatusOK, map[string]any{"links": []any{}})
		return
	}
	_, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	out := []map[string]any{}
	for _, l := range s.links.ListPublicFor(p.ID, time.Now()) {
		out = append(out, s.linkJSON(l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": out})
}

// revokeLink deletes a public link (owner only). The token must belong to the
// caller's project — otherwise one owner could revoke another's link.
func (s *Server) revokeLink(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("public links unavailable"))
		return
	}
	_, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	token := r.PathValue("token")
	// Private tokens live and die with the port's share list (setShares) —
	// deleting one here would strand shares with no URL.
	if l, ok := s.links.Get(token, time.Now()); !ok || l.Project != p.ID || l.Private() {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such link on this project"))
		return
	}
	if err := s.links.Delete(token); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Context identity, not the raw header — every other audit line does the
	// same, and the header is empty on the local no-auth path.
	s.logger.Info("public link revoked",
		"by", identity.FromContext(r.Context()).Email, "project", p.ID, "token", token[:8]+"…")
	w.WriteHeader(http.StatusNoContent)
}

// serveLink is the PUBLIC handler for /link/{token}/… — no identity required
// (authGuard exempts it). A valid, unexpired token grants access to exactly
// one port of the owning project; anything else 404s (never leaks the SPA).
func (s *Server) serveLink(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	if !links.ValidToken(token) {
		http.NotFound(w, r)
		return
	}
	l, ok := s.links.Get(token, time.Now())
	// A PRIVATE token must never be served on the no-login route: /link is
	// authGuard-exempt, so honoring it here would strip the email gate off a
	// share link. Same 404 as unknown — no oracle.
	if !ok || l.Private() {
		writeErr(w, http.StatusNotFound, fmt.Errorf("link not found or expired"))
		return
	}
	// Sandbox only the path form: a request that arrived via a token host
	// (previewHostRewrite stashed the original path) runs on its own origin
	// and needs no sandbox; the /link/… path form is served on the MAIN
	// origin, where an unsandboxed app could script the SPA's API (N22).
	prefix := "/link/" + token
	_, viaTokenHost := r.Context().Value(origPathKey{}).(string)
	// S10: bound every PUBLIC exchange by the link's expiry — an upgraded
	// WebSocket otherwise outlives revalidation forever (ReverseProxy only
	// checks the token at the handshake). PRIVATE links have no expiry;
	// their revocation is the share list (see serveShared).
	// Public links always carry a TTL, but guard the zero case anyway — a
	// zero Expires fed to WithDeadline is an ALREADY-EXPIRED context (the
	// bug that killed every PRIVATE preview; see serveShared).
	r2 := r
	if !l.Expires.IsZero() {
		ctx, cancel := context.WithDeadline(r.Context(), l.Expires)
		defer cancel()
		r2 = r.WithContext(ctx)
	}
	extPrefix := prefix
	if viaTokenHost {
		extPrefix = "" // the token host's visitor URL has no /link|/shared prefix
	}
	s.serveProxy(w, r2, l.Owner, l.Project, l.Port, prefix, extPrefix, !viaTokenHost)
}

// serveShared handles /shared/{token}/… — the email-gated share route, reached
// via the OAuth-gated private preview host (or path-form on the main host).
// authGuard has already resolved a verified identity; a private token grants
// access only if that identity is the owner or on the port's CURRENT share
// list — revoking an email cuts access immediately, the token never does.
func (s *Server) serveShared(w http.ResponseWriter, r *http.Request) {
	if s.links == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	if !links.ValidToken(token) {
		http.NotFound(w, r)
		return
	}
	l, ok := s.links.Get(token, time.Now())
	// A public token on the gated route is refused too: kinds never cross.
	if !ok || !l.Private() {
		writeErr(w, http.StatusNotFound, fmt.Errorf("link not found"))
		return
	}
	u := identity.FromContext(r.Context())
	sp, err := s.space(l.Owner)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("link not found"))
		return
	}
	p, err := sp.projects.Get(l.Project)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("link not found"))
		return
	}
	if !previewAllowed(u, l.Owner, p.SharedPorts[l.Port]) {
		// Deliberate trade-off vs serveLink's no-oracle 404: the caller here is
		// an AUTHENTICATED Google user holding a 128-bit token, and "ask the
		// owner to share with your email" is the difference between a usable
		// flow and a support ping. Confirming the token exists to them is
		// accepted.
		writeErr(w, http.StatusForbidden,
			fmt.Errorf("this preview hasn't been shared with %s — ask the project owner", u.Email))
		return
	}
	prefix := "/shared/" + token
	// Same sandbox rule as serveLink: unsandboxed only on the token's own
	// origin. The /shared/… path form runs on the MAIN origin — authGuard and
	// csrfGuard both exempt it, so an unsandboxed app there could call
	// /api/... with the viewer's session (the N22 escalation).
	_, viaTokenHost := r.Context().Value(origPathKey{}).(string)
	// PRIVATE links have NO expiry — revocation is the share list, not time
	// (links.EnsurePrivate never sets Expires). Feeding the zero time to
	// WithDeadline yields an ALREADY-EXPIRED context that 502'd every private
	// preview (2026-08-30 review, blocking). Bound only when a real expiry
	// exists (today: never, for this kind — purely defensive).
	r2 := r
	if !l.Expires.IsZero() {
		ctx, cancel := context.WithDeadline(r.Context(), l.Expires)
		defer cancel()
		r2 = r.WithContext(ctx)
	}
	extPrefix := prefix
	if viaTokenHost {
		extPrefix = "" // the token host's visitor URL has no /link|/shared prefix
	}
	s.serveProxy(w, r2, l.Owner, l.Project, l.Port, prefix, extPrefix, !viaTokenHost)
}

// isLinkPath reports whether a path is the public link route (authGuard exempts
// it: the token is the capability, no login needed).
func isLinkPath(p string) bool { return strings.HasPrefix(p, "/link/") }

// isSharedPath reports whether a path is the email-gated share route (authGuard
// resolves identity but lets non-styrman Google users through to it; serveShared
// enforces the share list).
func isSharedPath(p string) bool { return strings.HasPrefix(p, "/shared/") }
