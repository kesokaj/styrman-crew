package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kesokaj/styrman/internal/identity"
	"github.com/kesokaj/styrman/internal/project"
)

// emailRe is a pragmatic e-mail check for the share list. The Google OAuth
// ingress is what actually authenticates the address; this only guards the
// stored value.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// allUsers lists every user that has a state dir (a superset of the loaded
// spaces). Used to resolve a project by UUID across owners for port sharing.
func (s *Server) allUsers() []string {
	des, err := os.ReadDir(filepath.Join(s.cfg.StateDir, "users"))
	if err != nil {
		return nil
	}
	var out []string
	for _, de := range des {
		if de.IsDir() {
			out = append(out, de.Name())
		}
	}
	return out
}

// previewAccess resolves a project by UUID across all owners and decides
// whether the requester may reach its port previews. found is false when no
// such project exists anywhere.
func (s *Server) previewAccess(projectID string, u identity.User) (owner string, p project.Project, allowed, found bool) {
	for _, user := range s.allUsers() {
		sp, err := s.space(user)
		if err != nil {
			continue
		}
		pr, err := sp.projects.Get(projectID)
		if err != nil {
			continue
		}
		return user, pr, previewAllowed(u, user, pr.SharedEmails), true
	}
	return "", project.Project{}, false, false
}

// previewAllowed decides whether requester u may reach the previews of a
// project owned by ownerUser with the given shared-email list. The owner branch
// requires an AUTHORIZED user whose slug matches the owner store — never a
// guest, whose derived Username could collide with an owner's slug (B2). The
// share branch is email-based and case-insensitive.
func previewAllowed(u identity.User, ownerUser string, shared []string) bool {
	if u.Authorized && ownerUser == u.Username {
		return true
	}
	return emailIn(shared, u.Email)
}

func emailIn(list []string, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range list {
		if strings.ToLower(e) == email {
			return true
		}
	}
	return false
}

// isPreviewPath reports whether a path is a port-preview route (the only
// routes a shared, non-authorized Google user may reach).
func isPreviewPath(p string) bool {
	if strings.HasPrefix(p, "/proxy/") {
		return true
	}
	// /p/{project}/{term}/port/{port}/…
	return strings.HasPrefix(p, "/p/") && strings.Contains(p, "/port/")
}

// getShares returns a project's shared-email list (owner only).
func (s *Server) getShares(w http.ResponseWriter, r *http.Request) {
	_, _, p, err := s.getProjectFor(r) // resolves in the requester's OWN store
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	emails := p.SharedEmails
	if emails == nil {
		emails = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"emails": emails})
}

// maxShares / maxEmailLen bound the per-project share list — it is rewritten
// with fsync on every change and linear-scanned on every proxied request.
const (
	maxShares   = 50
	maxEmailLen = 254 // RFC 5321 max
)

// setShares replaces a project's shared-email list (owner only).
func (s *Server) setShares(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Normalize (lowercase, trim, dedupe) and validate.
	seen := map[string]bool{}
	var clean []string
	for _, e := range body.Emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || seen[e] {
			continue
		}
		if len(e) > maxEmailLen || !emailRe.MatchString(e) {
			writeErr(w, http.StatusBadRequest, errInvalidEmail(e))
			return
		}
		seen[e] = true
		clean = append(clean, e)
	}
	if len(clean) > maxShares {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("too many shares (max %d)", maxShares))
		return
	}
	if err := sp.projects.SetShares(p.ID, clean); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Audit: an access-control change is worth a durable log line.
	s.logger.Info("project shares updated", "by", u.Email, "project", p.ID,
		"projectName", p.Name, "count", len(clean))
	if clean == nil {
		clean = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"emails": clean})
}

func errInvalidEmail(e string) error {
	return fmt.Errorf("invalid email: %q", e)
}
