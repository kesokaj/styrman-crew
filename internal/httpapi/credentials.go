package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

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
	if err := s.creds.Delete(u.Username, kind, name); err != nil {
		if errors.Is(err, credstore.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
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
	writeJSON(w, http.StatusOK, map[string]any{"report": report})
}

// harvestCredentials extracts every credential it can find from a RUNNING
// container's guest home into the broker store, best-effort, one pass. Shared
// by the manual "Import from project" API and first-run setup completion (the
// setup VM mounts the user's shared base home, so finishing setup seeds the
// broker directly — no separate project sign-in needed). Returns a per-tool
// report of what happened.
func (s *Server) harvestCredentials(ctx context.Context, user, containerID, guest string) map[string]string {
	report := map[string]string{}

	// gh → git helper credential + GH_TOKEN env (same token; gh keeps it in the
	// guest keyring, so a login shell is needed to reach it).
	if tok, ok := s.guestOut(ctx, containerID, guest, "gh auth token --hostname github.com 2>/dev/null"); ok && tok != "" {
		if err := s.creds.PutGit(user, credstore.GitCred{Host: "github.com", Username: "x-access-token", Token: tok}); err != nil {
			report["git"] = "error"
		} else {
			report["git"] = "stored (github.com)"
		}
		if err := s.creds.PutEnv(user, "GH_TOKEN", tok); err != nil {
			report["gh"] = "error"
		} else {
			report["gh"] = "stored (GH_TOKEN)"
		}
	} else {
		report["gh"] = "not found — sign in with `gh auth login`"
	}

	// git identity → the broker syncs the auth TOKEN above but not the author
	// name/email; per-project homes start without it, so `git commit` in a
	// terminal fails. Store the identity from the shared home as env creds; the
	// login snippet applies it as `git config --global` (per-repo overrides
	// still win, unlike GIT_AUTHOR_* env vars).
	if name, email := s.gitIdentity(user); name != "" && email != "" {
		_ = s.creds.PutEnv(user, "STYRMAN_GIT_NAME", name)
		_ = s.creds.PutEnv(user, "STYRMAN_GIT_EMAIL", email)
		report["git-identity"] = "stored (" + name + ")"
	} else {
		report["git-identity"] = "not found — set git user.name/user.email"
	}

	// claude → the whole ~/.claude/.credentials.json (holds the OAuth refresh
	// token, so each project can refresh itself). File-injection tier.
	if s.importGuestFile(ctx, user, containerID, guest,
		"claude-credentials", "~/.claude/.credentials.json", "0600") {
		report["claude"] = "stored (credentials.json)"
	} else {
		report["claude"] = "not found — run `claude` and sign in"
	}

	// gcloud → Application Default Credentials (covers terraform, client
	// libraries, and gcloud with ADC). File-injection tier.
	if s.importGuestFile(ctx, user, containerID, guest,
		"gcloud-adc", "~/.config/gcloud/application_default_credentials.json", "0600") {
		report["gcloud"] = "stored (ADC)"
	} else {
		report["gcloud"] = "not found — run `gcloud auth application-default login`"
	}

	// ssh → the first private key found. File-injection tier (Kata blocks
	// agent-socket forwarding, so the key is materialized into each home).
	sshDone := false
	for _, key := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		if s.importGuestFile(ctx, user, containerID, guest,
			"ssh-"+key, "~/.ssh/"+key, "0600") {
			report["ssh"] = "stored (" + key + ")"
			sshDone = true
			break
		}
	}
	if !sshDone {
		report["ssh"] = "not found — no ~/.ssh/id_* private key"
	}

	// npm → an authToken from ~/.npmrc, stored as NPM_TOKEN (the skel .npmrc
	// references it). Env-token tier.
	if tok, ok := s.guestOut(ctx, containerID, guest,
		`sed -n 's|^//registry.npmjs.org/:_authToken=||p' ~/.npmrc 2>/dev/null | grep -v '\${' | head -1`); ok && tok != "" {
		if err := s.creds.PutEnv(user, "NPM_TOKEN", tok); err != nil {
			report["npm"] = "error"
		} else {
			report["npm"] = "stored (NPM_TOKEN)"
		}
	} else {
		report["npm"] = "not found — set //registry.npmjs.org/:_authToken in ~/.npmrc"
	}

	// agy → its OAuth token file. This agy build persists a portable
	// {access_token, refresh_token, expiry} JSON at
	// ~/.gemini/antigravity-cli/antigravity-oauth-token (0600) — NOT the
	// gnome-keyring that ADR-032a assumed (the tiny login.keyring never held
	// it). So it harvests as a plain file like claude and, unlike the keyring
	// path, works from the minimal setup VM (no Secret Service daemon there);
	// the refresh_token lets each project renew the token on its own.
	if s.importGuestFile(ctx, user, containerID, guest,
		"agy-oauth-token", "~/.gemini/antigravity-cli/antigravity-oauth-token", "0600") {
		report["agy"] = "stored (oauth token)"
	} else {
		report["agy"] = "not found — run `agy` and sign in"
	}

	return report
}

// guestOut runs a login-shell command as the guest in a project and returns
// trimmed stdout (ok=false on non-zero exit or exec error). The command must
// redirect its own stderr (Exec combines the streams).
func (s *Server) guestOut(ctx context.Context, containerID, guest, script string) (string, bool) {
	code, out, err := s.orch.Exec(ctx, containerID, orchestrator.ExecSpec{
		User: guest,
		Env:  []string{"HOME=/home/" + guest, "USER=" + guest},
		Cmd:  []string{"bash", "-lc", script},
	})
	if err != nil || code != 0 {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// importGuestFile reads a guest file as base64 and stores it as a file
// credential. Returns false when the file is absent/empty (best-effort).
func (s *Server) importGuestFile(ctx context.Context, user, containerID, guest, id, homePath, mode string) bool {
	// homePath is "~/…"; expand for the read, keep "~/…" for the store (the
	// login snippet re-expands it against each project's own $HOME).
	readPath := strings.Replace(homePath, "~/", "/home/"+guest+"/", 1)
	b64, ok := s.guestOut(ctx, containerID, guest, "base64 -w0 "+shellQuote(readPath)+" 2>/dev/null")
	if !ok || b64 == "" {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(data) == 0 {
		return false
	}
	if err := s.creds.PutFile(user, credstore.FileCred{ID: id, Path: homePath, Mode: mode, Data: data}); err != nil {
		s.logger.Warn("file credential store failed", "user", user, "id", id, "error", err)
		return false
	}
	return true
}

// shellQuote single-quotes a path for safe interpolation into a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
