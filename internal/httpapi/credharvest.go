package httpapi

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
)

// This file holds the credential-harvest business logic — extracting every
// signed-in tool's credential out of a running guest home into the broker
// store. It's kept separate from the HTTP handler surface in credentials.go
// (the handlers just resolve the request and call harvestCredentials). It stays
// in package httpapi because it leans on shared Server utilities (gitIdentity,
// orch, creds); pulling it into its own package would only entangle those.

// harvestCredentials extracts every credential it can find from a RUNNING
// container's guest home into the broker store, best-effort, one pass. Shared
// by the manual "Import from project" API and first-run setup completion (the
// setup VM mounts the user's shared base home, so finishing setup seeds the
// broker directly — no separate project sign-in needed). Returns a per-tool
// report of what happened.
func (s *Server) harvestCredentials(ctx context.Context, user, containerID, guest string) map[string]string {
	report := map[string]string{}

	// gh → git helper credential + GH_TOKEN env (same token). A login shell is
	// needed for gh's config; STYRMAN_SKIP_CRED_PULL keeps it reading the real
	// token, not the broker's copy (see guestOut).
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
		err1 := s.creds.PutEnv(user, "STYRMAN_GIT_NAME", name)
		err2 := s.creds.PutEnv(user, "STYRMAN_GIT_EMAIL", email)
		if err1 != nil || err2 != nil {
			// Don't report "stored" when the write failed (low finding).
			s.logger.Warn("git identity store failed", "user", user, "nameErr", err1, "emailErr", err2)
			report["git-identity"] = "error"
		} else {
			report["git-identity"] = "stored (" + name + ")"
		}
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

	// gcloud → Application Default Credentials (terraform, client libraries).
	// File-injection tier.
	if s.importGuestFile(ctx, user, containerID, guest,
		"gcloud-adc", "~/.config/gcloud/application_default_credentials.json", "0600") {
		report["gcloud-adc"] = "stored (ADC)"
	} else {
		report["gcloud-adc"] = "not found — run `gcloud auth application-default login`"
	}

	// gcloud CLI account login → `gcloud auth login` (distinct from ADC) keeps
	// the account + refresh token in credentials.db and the active account/
	// project in the config; without these, `gcloud <command>` in a project has
	// no active account. File-injection tier: credentials.db is the credential;
	// active_config + configurations/config_default set the active account (the
	// DEFAULT config — a named config isn't covered). access_tokens.db is a
	// cache gcloud regenerates, so it's intentionally not synced.
	if s.importGuestFile(ctx, user, containerID, guest,
		"gcloud-credentials-db", "~/.config/gcloud/credentials.db", "0600") {
		s.importGuestFile(ctx, user, containerID, guest,
			"gcloud-active-config", "~/.config/gcloud/active_config", "0644")
		s.importGuestFile(ctx, user, containerID, guest,
			"gcloud-config-default", "~/.config/gcloud/configurations/config_default", "0644")
		report["gcloud-cli"] = "stored (account login)"
	} else {
		report["gcloud-cli"] = "not found — run `gcloud auth login`"
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
	// ~/.gemini/antigravity-cli/antigravity-oauth-token (0600) — harvested as a
	// plain file like claude; the refresh_token lets each project renew on its
	// own (ADR-038: file tier, not keyring).
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
	code, out, err := s.guest.Exec(ctx, containerID, orchestrator.ExecSpec{
		User: guest,
		// Skip the pull-on-login overwrite for this shell so a harvest reads the
		// guest's REAL current files, not the broker's stale copy (H6).
		Env: []string{"HOME=/home/" + guest, "USER=" + guest, "STYRMAN_SKIP_CRED_PULL=1"},
		Cmd: []string{"bash", "-lc", script},
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

// tryAcquire is a compare-and-set on one of the in-flight maps (audit
// 2026-08-30: a plain store let two concurrent operations both pass every
// check and interleave). Lives here since the credential push became its
// only caller (the backup feature, the other user, was removed 2026-09-01).
func (s *Server) tryAcquire(m map[string]bool, user, projectID string) (release func(), ok bool) {
	key := user + "/" + projectID
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	if m[key] {
		return nil, false
	}
	m[key] = true
	return func() {
		s.deletingMu.Lock()
		defer s.deletingMu.Unlock()
		delete(m, key)
	}, true
}

// pushCredsToRunningProjects re-materializes freshly harvested credentials
// into every RUNNING project of the user, immediately. Mechanism: exec a
// NO-OP LOGIN shell in each guest — /etc/profile.d's credential snippet
// (ADR-032) pulls from the broker and writes the file-tier credentials into
// the home. FILE creds (gcloud, claude, ssh, agy) are read per-invocation by
// their tools, so even shells opened long ago start working the moment the
// files land; only env-tier creds still need a new shell. Without this, "I
// signed in but nothing happened in the project" was the default experience
// for every already-open terminal (operator report, 2026-08-30). Async and
// best-effort by design — a wedged guest must never block the harvest reply.
func (s *Server) pushCredsToRunningProjects(user, guest string) {
	// Single-flight per user (2026-08-30 review): repeated harvest clicks
	// otherwise stack concurrent pushes — several profile.d shells writing
	// the same guest credential files at once. A request landing while a
	// push is in flight is NOT simply dropped (audit: guests the in-flight
	// loop already visited would miss the newer harvest): it sets a rerun
	// flag the in-flight goroutine honors with one more full pass.
	release, ok := s.tryAcquire(s.credPushing, user, "-")
	if !ok {
		s.deletingMu.Lock()
		s.credPushRerun[user] = true
		s.deletingMu.Unlock()
		return
	}
	go func() {
		handedOff := false
		defer func() {
			if !handedOff {
				release()
			}
			if rec := recover(); rec != nil {
				s.logger.Error("credential push panicked", "user", user, "panic", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		statuses, err := s.orch.Adopt(ctx)
		if err != nil {
			s.logger.Warn("credential push: list failed", "user", user, "error", err)
			return
		}
		n := 0
		for _, st := range statuses {
			if st.User != user || st.State != "running" || st.ContainerID == "" {
				continue
			}
			ectx, ecancel := context.WithTimeout(ctx, 30*time.Second)
			// A login shell that does nothing — profile.d does the work.
			_, _, err := s.guest.Exec(ectx, st.ContainerID, orchestrator.ExecSpec{
				User: guest,
				Env:  []string{"HOME=/home/" + guest, "USER=" + guest},
				Cmd:  []string{"bash", "-lc", ":"},
			})
			ecancel()
			if err != nil {
				s.logger.Warn("credential push failed", "user", user, "project", st.Project, "error", err)
				continue
			}
			n++
		}
		if n > 0 {
			s.logger.Info("credentials pushed to running projects", "user", user, "projects", n)
		}
		// A harvest completed while we were pushing — one more pass so the
		// guests visited before it still get the newest credentials.
		s.deletingMu.Lock()
		rerun := s.credPushRerun[user]
		delete(s.credPushRerun, user)
		s.deletingMu.Unlock()
		if rerun {
			handedOff = true
			release() // free the slot BEFORE re-entering (it re-acquires)
			s.pushCredsToRunningProjects(user, guest)
		}
	}()
}
