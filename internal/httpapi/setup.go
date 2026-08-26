package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// setupStatus reports which credentials already exist in the user's home.
// Probed host-side (the server owns the homes), so it works with every
// microVM stopped.
type setupStatus struct {
	GitIdentity bool `json:"gitIdentity"`
	SSHKey      bool `json:"sshKey"`
	GitHub      bool `json:"github"`
	Claude      bool `json:"claude"`
	Keyring     bool `json:"keyring"`
	Done        bool `json:"done"`
	VMRunning   bool `json:"vmRunning"`
}

const setupDoneMarker = ".styrman/setup-done"

func (s *Server) userSetupStatus(ctx context.Context, user string) setupStatus {
	h := s.cfg.UserHome(user)
	st := setupStatus{
		SSHKey:  fileExists(filepath.Join(h, ".ssh", "id_ed25519")),
		GitHub:  fileExists(filepath.Join(h, ".config", "gh", "hosts.yml")),
		Claude:  fileExists(filepath.Join(h, ".claude", ".credentials.json")),
		Keyring: fileExists(filepath.Join(h, ".local", "share", "keyrings", "login.keyring")),
		Done:    fileExists(filepath.Join(h, setupDoneMarker)),
	}
	if data, err := os.ReadFile(filepath.Join(h, ".gitconfig")); err == nil {
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

// finishSetup records completion in the user's home and tears their VM down.
func (s *Server) finishSetup(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	marker := filepath.Join(s.cfg.UserHome(u.Username), setupDoneMarker)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
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

func listNames(dir string) []string {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, de := range des {
		name := strings.TrimSuffix(de.Name(), ".md")
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// toolDetails builds the per-tool settings cards from the user's home:
// identity facts plus the skills/agents/commands each harness carries.
func (s *Server) toolDetails(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	h := s.cfg.UserHome(u.Username)
	read := func(path string) string {
		b, _ := os.ReadFile(filepath.Join(h, path))
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

	// ssh
	pub := strings.TrimSpace(read(".ssh/id_ed25519.pub"))
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
		Ready:     fileExists(filepath.Join(h, ".claude", ".credentials.json")),
		ConfigDir: "~/.claude",
		Skills:    listNames(filepath.Join(h, ".claude", "skills")),
		Agents:    listNames(filepath.Join(h, ".claude", "agents")),
		Commands:  listNames(filepath.Join(h, ".claude", "commands")),
	}
	tools = append(tools, claude)

	// agy
	agy := toolInfo{
		Name:      "agy",
		Ready:     fileExists(filepath.Join(h, ".local", "share", "keyrings", "login.keyring")),
		ConfigDir: "~/.gemini/config",
		Skills:    listNames(filepath.Join(h, ".gemini", "config", "skills")),
		Agents:    listNames(filepath.Join(h, ".gemini", "config", "plugins", "harness", "agents")),
	}
	if agy.Ready {
		agy.Details = []string{"keyring present (Google OAuth token stored)"}
	}
	tools = append(tools, agy)

	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
