package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/files"
)

// setupStatus reports which credentials already exist in the user's home.
// Probed host-side (the server owns the homes), so it works with every
// microVM stopped.
type setupStatus struct {
	GitIdentity bool `json:"gitIdentity"` // required
	SSHKey      bool `json:"sshKey"`      // required
	GitHub      bool `json:"github"`      // optional
	Gcloud      bool `json:"gcloud"`      // optional (ADC)
	Claude      bool `json:"claude"`      // optional
	Agy         bool `json:"agy"`         // optional (Antigravity)
	Done        bool `json:"done"`
	VMRunning   bool `json:"vmRunning"`
}

const setupDoneMarker = ".styrman/setup-done"

func (s *Server) userSetupStatus(ctx context.Context, user string) setupStatus {
	// All reads are confined to the user's home (os.Root): a guest-planted
	// symlink pointing at another tenant's file is refused, not followed.
	h := s.cfg.UserHome(user)
	st := setupStatus{
		SSHKey: files.Exists(h, ".ssh/id_ed25519"),
		GitHub: files.Exists(h, ".config/gh/hosts.yml"),
		Gcloud: files.Exists(h, ".config/gcloud/application_default_credentials.json"),
		Claude: files.Exists(h, ".claude/.credentials.json"),
		// agy persists its OAuth token as a plain file (ADR-038 superseded the
		// keyring assumption of ADR-032a); readiness keys on that token file.
		Agy:  files.Exists(h, ".gemini/antigravity-cli/antigravity-oauth-token"),
		Done: files.Exists(h, setupDoneMarker),
	}
	if data, err := files.ReadRaw(h, ".gitconfig", 0); err == nil {
		st.GitIdentity = strings.Contains(string(data), "email")
	}
	if running, err := s.orch.SetupStatus(ctx, user); err == nil {
		st.VMRunning = running
	}
	return st
}

func (s *Server) getSetupStatus(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userSetupStatus(r.Context(), u.Username))
}

// startSetup boots the user's hidden onboarding microVM. The terminal itself
// is attached via GET /ws/setup.
func (s *Server) startSetup(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if _, err := s.orch.EnsureSetup(ctx, u.Username, s.guestUserFor(u.Username)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "running"})
}

// seedBrokerFromSetup harvests every credential the user just signed into in
// the running setup VM straight into the broker store, so every project (present
// and future) pulls them on login — no "sign in again inside a project" step.
// The setup VM mounts the user's shared base home (orchestrator.EnsureSetup).
// Best-effort: a disabled broker or an empty harvest returns an empty report,
// never an error — it must never block completing or extending setup. The VM
// must be running (started via /api/setup/start or an attached /ws/setup).
func (s *Server) seedBrokerFromSetup(user string) map[string]string {
	if s.creds == nil {
		return map[string]string{}
	}
	hctx, hcancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer hcancel()
	report := s.harvestCredentials(hctx, user, config.SetupContainer(user), s.guestUserFor(user))
	s.logger.Info("setup seeded the credential broker", "user", user, "report", report)
	// Immediately re-materialize the file-tier creds into every RUNNING
	// project — without this, already-open terminals keep failing ("I signed
	// in but nothing happened in the project") until a brand-new shell.
	s.pushCredsToRunningProjects(user, s.guestUserFor(user))
	return report
}

// harvestSetup seeds the broker from the RUNNING setup VM without marking setup
// done or tearing the VM down — the "add one more credential later" flow keeps
// the VM up so the user can sign in to several tools in one sitting, harvesting
// after each. Returns the per-tool report so the UI refreshes what's now shared.
func (s *Server) harvestSetup(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": s.seedBrokerFromSetup(u.Username)})
}

// finishSetup records completion in the user's home and tears their VM down.
func (s *Server) finishSetup(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Seed the broker from the setup VM BEFORE tearing it down.
	s.seedBrokerFromSetup(u.Username)
	// Confined write: refuses to follow a symlinked marker planted at
	// another tenant's file (was an arbitrary-overwrite vector).
	stamp := []byte(time.Now().UTC().Format(time.RFC3339) + "\n")
	if err := files.Save(s.cfg.UserHome(u.Username), setupDoneMarker, stamp, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dctx, dcancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer dcancel()
	if err := s.orch.DestroySetup(dctx, u.Username); err != nil {
		s.logger.Warn("setup vm teardown failed", "user", u.Username, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// discardSetup tears the user's VM down without marking done.
func (s *Server) discardSetup(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.DestroySetup(ctx, u.Username); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toolInfo is one per-tool card on the settings page.
type toolInfo struct {
	Name      string   `json:"name"`
	Ready     bool     `json:"ready"`
	Details   []string `json:"details,omitempty"`
	Skills    []string `json:"skills,omitempty"`
	Agents    []string `json:"agents,omitempty"`
	Commands  []string `json:"commands,omitempty"`
	ConfigDir string   `json:"configDir,omitempty"`
}

// oldHarnessBackup matches the previous harness's re-seed backups
// (`<file>.bak.<YYYYmmdd-HHMMSS>`, e.g. `testing.md.bak.20260901-065226`),
// which are not agents. Anchored to that exact shape so a legitimately named
// entry containing `.bak.` elsewhere is not hidden.
var oldHarnessBackup = regexp.MustCompile(`\.bak\.[0-9]{8}-[0-9]{6}$`)

// listNames enumerates a harness dir (skills/agents/commands) relative to a
// home, confined to it. An empty home lists nothing.
func listNames(home, rel string) []string {
	if home == "" {
		return nil
	}
	var out []string
	for _, n := range files.ListNames(home, rel) {
		if strings.HasPrefix(n, ".") || oldHarnessBackup.MatchString(n) {
			continue
		}
		out = append(out, strings.TrimSuffix(n, ".md"))
	}
	sort.Strings(out)
	return out
}

// rosterHome is the home the settings page reads the harness roster from:
// the user's most recently created project whose home carries the sh-agent
// manifest. The base home is never seeded (the setup VM runs MINIMAL), so
// before ADR-062 the roster was always empty. No seeded project → "" and the
// roster fields stay empty.
func (s *Server) rosterHome(user string, sp *userSpace) string {
	for _, p := range sp.projects.List() { // newest first
		h := s.cfg.ProjectHome(user, p.ID)
		if files.Exists(h, ".config/sh-agent/state.json") {
			return h
		}
	}
	return ""
}

// toolDetails builds the per-tool settings cards: identity facts and
// readiness from the user's base home (where the broker keeps credentials
// current), the skills/agents/commands roster from a seeded project home
// (rosterHome — every project gets the identical set, R5).
func (s *Server) toolDetails(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	h := s.cfg.UserHome(u.Username)
	rh := s.rosterHome(u.Username, sp)
	read := func(rel string) string {
		b, _ := files.ReadRaw(h, rel, 0)
		return string(b)
	}

	var tools []toolInfo

	// git
	gitcfg := read(".gitconfig")
	git := toolInfo{Name: "git", Ready: strings.Contains(gitcfg, "email")}
	for _, line := range strings.Split(gitcfg, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name = ") || strings.HasPrefix(line, "email = ") {
			git.Details = append(git.Details, line)
		}
	}
	tools = append(tools, git)

	// ssh — the .pub is echoed to the UI and offered for copying, so it is
	// read WITHOUT following a symlink: a .pub pointed at the PRIVATE key
	// beside it is inside the confined root and ReadRaw would happily follow
	// it (security review 2026-09-10 — the previous check claimed to refuse
	// this and did not). O_NOFOLLOW does not cover a HARD link, so the
	// content is validated too: one line, and it must have a public-key
	// shape (`<type> <base64>…`) — a private-key header, or anything else,
	// is "no key".
	pub := ""
	if b, err := files.ReadRawNoFollow(h, ".ssh/id_ed25519.pub", 8192); err == nil {
		line, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
		if line = strings.TrimSpace(line); publicKeyLine(line) {
			pub = line
		}
	}
	ssh := toolInfo{Name: "ssh", Ready: pub != ""}
	if pub != "" {
		ssh.Details = []string{pub}
	}
	tools = append(tools, ssh)

	// github (gh)
	hosts := read(".config/gh/hosts.yml")
	gh := toolInfo{Name: "github", Ready: hosts != "", ConfigDir: "~/.config/gh"}
	for _, line := range strings.Split(hosts, "\n") {
		if strings.Contains(line, "user:") {
			gh.Details = append(gh.Details, strings.TrimSpace(line))
		}
	}
	tools = append(tools, gh)

	// claude
	claude := toolInfo{
		Name:      "claude",
		Ready:     files.Exists(h, ".claude/.credentials.json"),
		ConfigDir: "~/.claude",
		Skills:    listNames(rh, ".claude/skills"),
		Agents:    listNames(rh, ".claude/agents"),
		Commands:  listNames(rh, ".claude/commands"),
	}
	tools = append(tools, claude)

	// agy — readiness keys on its OAuth token file (ADR-038; agy no longer
	// uses the gnome-keyring the broker once seeded).
	agy := toolInfo{
		Name:      "agy",
		Ready:     files.Exists(h, ".gemini/antigravity-cli/antigravity-oauth-token"),
		ConfigDir: "~/.gemini/config",
		Skills:    listNames(rh, ".gemini/config/skills"),
		// One directory per agent, `<name>/agent.md` (sh-agent's derived
		// layout; the old harness's plugins/harness/agents is gone).
		Agents: listNames(rh, ".gemini/config/agents"),
	}
	if agy.Ready {
		agy.Details = []string{"OAuth token stored"}
	}
	tools = append(tools, agy)

	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

// publicKeyLine reports whether line has the shape OpenSSH writes to a .pub:
// a key type, a space, a base64 body, optionally a comment — on one line.
func publicKeyLine(line string) bool {
	typ, rest, ok := strings.Cut(line, " ")
	if !ok || !(strings.HasPrefix(typ, "ssh-") || strings.HasPrefix(typ, "ecdsa-") || strings.HasPrefix(typ, "sk-")) {
		return false
	}
	body, _, _ := strings.Cut(strings.TrimLeft(rest, " "), " ")
	if body == "" {
		return false
	}
	for _, r := range body {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=') {
			return false
		}
	}
	return true
}
