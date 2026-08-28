package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/links"
)

// ttlChoices bounds how long a public link may live. Keep the set small and
// finite so a link can't be minted to live effectively forever.
var ttlChoices = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// linkURL renders the public URL for a link. With STYRMAN_PREVIEW_BASE set it's
// the per-token subdomain (its own origin); otherwise the path form, which the
// server serves directly (used in local/dev before the wildcard host exists).
func (s *Server) linkURL(l links.Link) string {
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
	for _, l := range s.links.ListFor(p.ID, time.Now()) {
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
	if l, ok := s.links.Get(token, time.Now()); !ok || l.Project != p.ID {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such link on this project"))
		return
	}
	if err := s.links.Delete(token); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	u := r.Header.Get(HeaderEmail)
	s.logger.Info("public link revoked", "by", u, "project", p.ID, "token", token[:8]+"…")
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
	if !ok {
		// Expired or unknown — same response, no oracle.
		writeErr(w, http.StatusNotFound, fmt.Errorf("link not found or expired"))
		return
	}
	// sandbox=false: the token subdomain is already its own origin, so the app
	// runs normally there and is still isolated from the SPA.
	prefix := "/link/" + token
	s.serveProxy(w, r, l.Owner, l.Project, l.Port, prefix, false)
}

// isLinkPath reports whether a path is the public link route (authGuard exempts
// it: the token is the capability, no login needed).
func isLinkPath(p string) bool { return strings.HasPrefix(p, "/link/") }
