// Repo provisioning for a newly created project: the clone, or the GitHub
// repo create-and-push, that fills its workspace — plus the credential-broker
// readiness wait both depend on, and the per-project message that tells the
// user when either failed.
//
// Split out of server.go so the sequence lives next to provision_test.go,
// which pins the ORDER of these steps. The bug this file exists to prevent was
// the bridge attach running LAST, after provisioning, which left every clone
// and every `gh repo create` running against an unreachable credential broker.

package httpapi

import (
	"context"
	"time"

	"github.com/kesokaj/styrman-crew/internal/gitrepo"
)

// setProvisionError records why a project's background repo provisioning
// failed (or clears it when msg is empty).
//
// Provisioning outlives the request AND can outlive the project: a delete
// landing mid-clone clears the map, and a later write would resurrect an entry
// that nothing ever cleans up again — an unbounded, user-driven leak (security
// review 2026-09-02). Writes are therefore dropped for a project that is being
// deleted or is already gone.
func (s *Server) setProvisionError(user, projectID, msg string) {
	key := user + "/" + projectID
	if msg != "" && !s.projectExists(user, projectID) {
		return
	}
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	if msg == "" || s.deleting[key] {
		delete(s.provisionErrs, key)
		return
	}
	s.provisionErrs[key] = msg
}

// projectExists reports whether the user still has this project on record.
func (s *Server) projectExists(user, projectID string) bool {
	sp, err := s.space(user)
	if err != nil {
		return false
	}
	_, err = sp.projects.Get(projectID)
	return err == nil
}

// provisionError returns the last provisioning failure for a project, if any.
func (s *Server) provisionError(user, projectID string) string {
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	return s.provisionErrs[user+"/"+projectID]
}

// provisionSpec is one project's repo provisioning: everything createProject's
// background goroutine needs after the microVM is up. attach and run are
// injected so the ORDER of the steps — the thing that was wrong — is testable
// without a Docker daemon.
type provisionSpec struct {
	user     string
	project  string
	name     string
	workdir  string // host-side working tree, for the R13 fallback init
	cloneURL string
	github   string      // "private" | "public" | "" | "none"
	attach   func() bool // reports whether a FRESH bridge attach happened
	run      func(...string) (int, string, error)
}

// wantsGitHub reports whether this spec asks for a GitHub repo.
func (ps provisionSpec) wantsGitHub() bool {
	return ps.github == "private" || ps.github == "public"
}

// provisionRepo fills a new project's workspace: a clone, or a GitHub repo
// created and pushed. Both need the user's credentials, and credentials reach
// a project ONLY over the credential broker on its own bridge (ADR-030/032) —
// which the server can only serve once it has joined that bridge (attach).
//
// The order below is the fix for a real outage: attach used to run LAST, after
// provisioning, so every clone and every `gh repo create` ran against an
// unreachable broker and failed with "Permission denied (publickey)" or
// "could not read Username", then silently degraded to an empty repo. Attach
// first, wait for the broker to actually answer from inside the guest, and only
// then run anything that needs a credential.
func (s *Server) provisionRepo(ctx context.Context, ps provisionSpec) {
	// A fresh attach reconfigures this container's netns and can kill in-flight
	// connections. The 201 itself flushed long ago (EnsureRunning takes
	// seconds), but the client's FIRST terminal request is in flight right
	// now — it serializes on this very boot behind the per-project lock, so it
	// returns at almost exactly this moment. The pause lets it land first.
	select {
	case <-ctx.Done():
		return
	case <-time.After(s.provisionDelay):
	}
	// EnsureSelfAttached's "fresh" return tells a REQUEST-path caller to make
	// the client retry (proxy.go). Nothing to redirect here — this goroutine
	// has no client — and it already settles the new route itself, so it is
	// logged rather than acted on.
	if fresh := ps.attach(); fresh {
		s.logger.Info("joined project bridge for provisioning", "project", ps.name)
	}
	if ps.cloneURL == "" && !ps.wantsGitHub() {
		return // plain `git init` — already done server-side before the 201
	}
	// In host-mode dev there is no broker to wait for; waitForBroker says so
	// immediately, and warning about it would be noise.
	if !s.waitForBroker(ctx, ps.run) && s.cfg.SelfContainer != "" {
		s.logger.Warn("credential broker never became reachable — provisioning anyway",
			"project", ps.name)
	}
	switch {
	case ps.cloneURL != "":
		// A LOGIN shell on purpose: /etc/profile.d/zz-styrman-credentials.sh is
		// what materializes ~/.ssh/id_ed25519 and the git identity into this
		// project's (brand new, ADR-031) home. A bare exec skips it, so the
		// clone had no key to offer.
		// StrictHostKeyChecking=accept-new is deliberate TOFU: first contact
		// with a new host is trusted, later key changes are not.
		clone := "git -c core.sshCommand='ssh -o StrictHostKeyChecking=accept-new' " +
			"clone -- " + shellQuote(ps.cloneURL) + " ."
		if code, out, err := ps.run("bash", "-lc", clone); err != nil || code != 0 {
			s.logger.Warn("clone failed — falling back to empty repo",
				"project", ps.name, "url", redactSecrets(ps.cloneURL), "exit", code,
				"output", redactSecrets(out), "error", err)
			s.setProvisionError(ps.user, ps.project, guestErrMessage("clone failed", out))
			if err := gitrepo.EnsureRepo(ctx, ps.workdir); err != nil {
				s.logger.Error("fallback git init failed", "project", ps.name, "error", err)
			}
		}
	case ps.wantsGitHub():
		// Login shell for the same reason as the clone: the commit needs the
		// broker-delivered user.name/user.email, and gh needs GH_TOKEN. The
		// local repo exists either way, so a failure is reported, not fatal.
		if code, out, err := ps.run("bash", "-lc",
			"git commit --allow-empty -m 'Initialize styrman project'"); err != nil || code != 0 {
			s.logger.Warn("initial commit failed", "project", ps.name, "exit", code,
				"output", redactSecrets(out), "error", err)
			s.setProvisionError(ps.user, ps.project, guestErrMessage("initial commit failed", out))
		}
		// Map to a literal rather than interpolating the request's own string:
		// wantsGitHub already constrains it, but a request-derived value should
		// never reach a shell command line even when it is currently safe.
		vis := "--public"
		if ps.github == "private" {
			vis = "--private"
		}
		if code, out, err := ps.run("bash", "-lc",
			"gh repo create "+shellQuote(ps.name)+" "+vis+" --source=. --remote=origin --push"); err != nil || code != 0 {
			s.logger.Warn("github repo create failed (is gh signed in?)",
				"project", ps.name, "exit", code, "output", redactSecrets(out), "error", err)
			s.setProvisionError(ps.user, ps.project, guestErrMessage("GitHub repo create failed", out))
		}
	}
}

// waitForBroker polls the credential broker FROM INSIDE the guest until it
// answers, and reports whether it ever did. Project provisioning (clone,
// GitHub repo create) is worthless without credentials, and the bridge attach
// that makes the broker reachable settles asynchronously — provisioning
// before that is what made every clone fail.
//
// A PLAIN shell, not a login shell: a login shell runs the credential profile
// script, which would materialize the user's ssh key and tokens into the
// project home just to answer "is the broker up?" — up to ten times, and even
// for a project whose clone then fails and gets deleted (security review
// 2026-09-02). The probe therefore uses the broker URL injected into the
// container env. That is safe HERE specifically: the network was created
// seconds ago by this same boot, so the injected address cannot yet be stale
// (the profile script's self-heal exists for a LATER network recreation, M17).
//
// There is no broker at all in host-mode dev, where the server never joins a
// bridge — waiting would just burn ~30s of connect timeouts before failing the
// way it fails instantly.
func (s *Server) waitForBroker(ctx context.Context, run func(...string) (int, string, error)) bool {
	if s.cfg.SelfContainer == "" {
		return false
	}
	for i := 0; i < brokerProbeAttempts; i++ {
		if code, _, err := run("sh", "-c", brokerProbeCmd); err == nil && code == 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(s.brokerProbeInterval):
		}
	}
	return false
}

// brokerProbeCmd asks the broker whether it is reachable yet; the pacing lives
// on the Server (provisionDelay / brokerProbeInterval).
const (
	brokerProbeCmd      = `curl -fsS --connect-timeout 1 --max-time 3 "${STYRMAN_CRED_BROKER:-}/healthz" >/dev/null 2>&1`
	brokerProbeAttempts = 10
)
