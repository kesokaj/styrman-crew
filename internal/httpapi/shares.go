package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/links"
	"github.com/kesokaj/styrman-crew/internal/project"
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
// whether the requester may reach the preview of THIS port. found is false when
// no such project exists anywhere.
func (s *Server) previewAccess(projectID string, port int, u identity.User) (owner string, p project.Project, allowed, found bool) {
	for _, user := range s.allUsers() {
		sp, err := s.space(user)
		if err != nil {
			continue
		}
		pr, err := sp.projects.Get(projectID)
		if err != nil {
			continue
		}
		return user, pr, previewAllowed(u, user, pr.SharedPorts[port]), true
	}
	return "", project.Project{}, false, false
}

// previewAllowed decides whether requester u may reach a port preview of a
// project owned by ownerUser, given the emails shared for THAT port. The owner
// branch requires an AUTHORIZED user whose slug matches the owner store — never
// a guest, whose derived Username could collide with an owner's slug (B2). The
// share branch is email-based and case-insensitive.
func previewAllowed(u identity.User, ownerUser string, sharedForPort []string) bool {
	if u.Authorized && ownerUser == u.Username {
		return true
	}
	return emailIn(sharedForPort, u.Email)
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

// sharePort parses + range-checks the {port} path value.
func sharePort(r *http.Request) (int, error) {
	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil || port < 1024 || port > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return port, nil
}

// getShares returns the shared-email list for one port (owner only).
func (s *Server) getShares(w http.ResponseWriter, r *http.Request) {
	port, err := sharePort(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_, _, p, err := s.getProjectFor(r) // resolves in the requester's OWN store
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	emails := p.SharedPorts[port]
	if emails == nil {
		emails = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"emails": emails,
		"url":    s.sharedURL(p.ID, port),
	})
}

// sharedURL returns the port's private share URL, or "" when the port has no
// live share link (or the links store is unavailable).
func (s *Server) sharedURL(project string, port int) string {
	if s.links == nil {
		return ""
	}
	l, ok := s.links.GetPrivate(project, port)
	if !ok {
		return ""
	}
	return s.linkURL(l)
}

// maxShares / maxEmailLen bound the per-project share list — it is rewritten
// with fsync on every change and linear-scanned on every proxied request.
const (
	maxShares   = 50
	maxEmailLen = 254 // RFC 5321 max
)

// setShares replaces the shared-email list for one port (owner only).
func (s *Server) setShares(w http.ResponseWriter, r *http.Request) {
	port, err := sharePort(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
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
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	if err := sp.projects.SetPortShares(p.ID, port, clean); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// The private share link lives and dies with the list: mint (idempotent —
	// the URL stays stable across edits) while anyone is shared, drop it when
	// the list empties so the token stops resolving at all. A failure here is
	// surfaced, not swallowed: the emails ARE saved at this point, but the
	// owner must know the link state didn't follow.
	if s.links != nil {
		if len(clean) > 0 {
			if _, err := s.links.EnsurePrivate(p.ID, u.Username, port, u.Email, time.Now()); err != nil {
				s.logger.Error("share link mint failed", "project", p.ID, "port", port, "error", err)
				code := http.StatusInternalServerError
				if errors.Is(err, links.ErrTooManyLinks) {
					code = http.StatusConflict
				}
				writeErr(w, code, fmt.Errorf("emails saved, but the private link could not be created: %w", err))
				return
			}
		} else if err := s.links.DeletePrivate(p.ID, port); err != nil {
			s.logger.Error("share link revoke failed", "project", p.ID, "port", port, "error", err)
			writeErr(w, http.StatusInternalServerError,
				fmt.Errorf("emails cleared, but the private link could not be removed: %w", err))
			return
		}
	}
	// Audit: an access-control change is worth a durable log line.
	s.logger.Info("port shares updated", "by", u.Email, "project", p.ID,
		"projectName", p.Name, "port", port, "count", len(clean))
	if clean == nil {
		clean = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"emails": clean,
		"url":    s.sharedURL(p.ID, port),
	})
}

func errInvalidEmail(e string) error {
	return fmt.Errorf("invalid email: %q", e)
}
