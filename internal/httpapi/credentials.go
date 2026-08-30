package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/identity"
)

// credentialContent returns a stored credential's readable content for the
// Settings "click to read" view. It reveals the AUTHORIZED user's OWN secret
// (scoped by u.Username) — the same value the broker already materializes into
// their projects' homes. Files return their text (binary flagged); env/git
// return the value/token. The disclosure is audit-logged.
func (s *Server) credentialContent(w http.ResponseWriter, r *http.Request) {
	u := identity.FromContext(r.Context())
	if !u.Authorized {
		writeErr(w, http.StatusForbidden, errors.New("not authorized"))
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	// Reading a plaintext secret is worth an audit record.
	s.logger.Info("credential content read", "user", u.Username, "kind", r.PathValue("kind"), "id", r.PathValue("id"))
	kind := r.PathValue("kind")
	id := r.PathValue("id")
	switch kind {
	case "file":
		files, err := s.creds.AllFiles(u.Username)
		if err == nil {
			for _, f := range files {
				if f.ID == id {
					if !utf8.Valid(f.Data) {
						writeJSON(w, http.StatusOK, map[string]any{"path": f.Path, "binary": true})
						return
					}
					writeJSON(w, http.StatusOK, map[string]any{"content": string(f.Data), "path": f.Path})
					return
				}
			}
		}
	case "env":
		env, err := s.creds.AllEnv(u.Username)
		if err == nil {
			if v, ok := env[id]; ok {
				writeJSON(w, http.StatusOK, map[string]any{"content": v, "path": id})
				return
			}
		}
	case "git":
		if c, err := s.creds.GetGit(u.Username, id); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"content": c.Token, "path": c.Host + " (" + c.Username + ")"})
			return
		}
	}
	writeErr(w, http.StatusNotFound, errors.New("credential not found"))
}

// This file is the credential-broker onboarding + management surface (ADR-032):
// the store holds the single live copy of each of a user's credentials and the
// broker serves the CURRENT copy to every project (git helper, env exports,
// file injection). Onboarding extracts credentials ONCE from a reference
// running project (where the tools are already signed in) into the store;
// after that every project — including new ones — is current with no re-auth.

// credentialsList returns value-free descriptors of what the user has onboarded
// (kinds + names, never the secrets), for the Settings status view. When the
// broker is disabled (host-mode dev, no self-container), listing is simply
// empty with brokerEnabled:false — not an error, so the Settings page renders
// cleanly instead of surfacing a failed request.
func (s *Server) credentialsList(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.creds == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []credstore.Item{}, "brokerEnabled": false})
		return
	}
	items, err := s.creds.List(u.Username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errors.New("cannot list credentials"))
		return
	}
	if items == nil {
		items = []credstore.Item{} // "items": [] in JSON, never null
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "brokerEnabled": true})
}

// putTokenCredential stores a manually-entered env-token credential (e.g. an
// npm token as NPM_TOKEN, or an Anthropic API key) — for tokens the user pastes
// rather than extracts from a project.
func (s *Server) putTokenCredential(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" || body.Value == "" {
		writeErr(w, http.StatusBadRequest, errors.New("key and value required"))
		return
	}
	if err := s.creds.PutEnv(u.Username, body.Key, body.Value); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.logger.Info("env credential stored", "user", u.Username, "key", body.Key)
	writeJSON(w, http.StatusOK, map[string]string{"key": body.Key, "status": "stored"})
}

// deleteCredential removes one stored credential by kind ("git"|"env"|"file")
// and name (host | KEY | file id).
func (s *Server) deleteCredential(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	kind, name := r.PathValue("kind"), r.PathValue("name")
	// For a file credential, capture its home-relative path BEFORE deleting so we
	// can also sweep the already-materialized copies out of the user's homes —
	// otherwise "delete" only stops future delivery and leaves the secret on
	// disk in every project (M22).
	var filePath string
	if kind == "file" {
		if files, err := s.creds.AllFiles(u.Username); err == nil {
			for _, f := range files {
				if f.ID == name {
					filePath = f.Path
					break
				}
			}
		}
	}
	if err := s.creds.Delete(u.Username, kind, name); err != nil {
		if errors.Is(err, credstore.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if filePath != "" {
		s.sweepMaterializedFile(u.Username, filePath)
	}
	s.logger.Info("credential deleted", "user", u.Username, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}

// importCredentials extracts every credential it can find from a reference
// RUNNING project into the store, in one pass. Each tool is best-effort: a tool
// that isn't signed in is simply reported "not found", never an error. The
// response maps each tool to what happened, so the UI can show the operator
// exactly what got shared.
func (s *Server) importCredentials(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.creds == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("credential broker disabled"))
		return
	}
	var body struct {
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project == "" {
		writeErr(w, http.StatusBadRequest, errors.New("a running project to import from is required"))
		return
	}
	p, err := sp.projects.Get(body.Project)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.orch.Status(r.Context(), u.Username, p.ID)
	if err != nil || st.State != "running" {
		writeErr(w, http.StatusConflict, errors.New("name a RUNNING project to import from"))
		return
	}
	guest := s.guestUserFor(u.Username)
	report := s.harvestCredentials(r.Context(), u.Username, st.ContainerID, guest)
	s.logger.Info("credential import complete", "user", u.Username, "from", p.ID)
	// Fan the fresh file-tier creds out to every running project right away
	// (already-open terminals see them on the tools' next invocation).
	s.pushCredsToRunningProjects(u.Username, guest)
	writeJSON(w, http.StatusOK, map[string]any{"report": report})
}

// sweepMaterializedFile best-effort removes a file credential's already-
// materialized copies from the user's base home and every per-project home, so
// deleting the credential actually revokes it rather than only stopping future
// delivery (M22). homePath is the stored "~/…" path.
func (s *Server) sweepMaterializedFile(user, homePath string) {
	rel := strings.TrimPrefix(strings.TrimPrefix(homePath, "~/"), "/")
	if rel == "" {
		return
	}
	homes, _ := filepath.Glob(filepath.Join(s.cfg.HomesDir, user, "p", "*"))
	homes = append(homes, s.cfg.UserHome(user))
	for _, h := range homes {
		target := filepath.Join(h, rel)
		// Never let a crafted stored path escape the home root.
		if rp, err := filepath.Rel(h, target); err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(os.PathSeparator)) {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("credential revoke: could not remove materialized copy", "user", user, "path", target, "error", err)
		}
	}
}
