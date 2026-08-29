// Package credstore is the credential broker's source of truth (ADR-030/032):
// one per-user store, server-owned, that projects never touch directly.
// Onboarding writes credentials here (extracted once from a reference project,
// or entered by hand); the broker reads and serves them so every project uses
// the current copy and a refresh propagates to all.
//
// Three credential kinds, matching the broker's delivery tiers (ADR-032 §tiers):
//   - git   — per-host {username, token}, served to git's credential helper.
//   - env   — a shell variable (GH_TOKEN, NPM_TOKEN, CLAUDE_CODE_OAUTH_TOKEN…),
//     exported into every login shell (env-token tier).
//   - file  — a file materialized into the guest home (gcloud ADC, ssh key…),
//     written at login (file-injection tier).
//
// Files are 0600 under StateDir/credentials/<user>/, one JSON file per item:
// git-<host>.json, env-<KEY>.json, file-<id>.json.
package credstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ErrNotFound is returned when a credential has not been onboarded yet.
var ErrNotFound = errors.New("credential not found")

// envKeyRe bounds a shell variable name — the value is exported verbatim into a
// login shell, so the KEY must be a plain identifier (never anything the login
// snippet could misparse). Uppercase/lowercase letters, digits, underscore.
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// GitCred is a git/GitHub credential for one host.
type GitCred struct {
	Host     string `json:"host"`     // e.g. "github.com"
	Username string `json:"username"` // e.g. "x-access-token" or the login
	Token    string `json:"token"`
}

// EnvCred is a single shell variable delivered to every login shell.
type EnvCred struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// FileCred is a file materialized into the guest home at login. Path is
// home-relative ("~/…" or a bare relative path); Mode is an octal string.
type FileCred struct {
	ID   string `json:"id"`   // stable identifier / store filename key
	Path string `json:"path"` // e.g. "~/.ssh/id_ed25519" or ".config/gcloud/…"
	Mode string `json:"mode"` // octal, e.g. "0600"
	Data []byte `json:"-"`    // raw bytes; marshaled base64 in dataB64
}

// fileCredWire is the on-disk / on-wire shape (base64 data).
type fileCredWire struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	DataB64 string `json:"data"`
}

// KeyringCred is a Secret Service item materialized into each project's
// gnome-keyring at login (ADR-032 keyring tier — agy). The attribute shape is
// zalando/go-keyring's: {service, username}; Label is cosmetic.
type KeyringCred struct {
	ID       string `json:"id"`      // stable store key, e.g. "agy"
	Service  string `json:"service"` // attribute: service (e.g. "antigravity")
	Username string `json:"username"`
	Label    string `json:"label"`
	Secret   []byte `json:"-"` // raw bytes; marshaled base64 in secretB64
}

// keyringCredWire is the on-disk shape (base64 secret).
type keyringCredWire struct {
	ID        string `json:"id"`
	Service   string `json:"service"`
	Username  string `json:"username"`
	Label     string `json:"label"`
	SecretB64 string `json:"secret"`
}

// Item is a value-free descriptor of one stored credential, for the Settings
// status view (never carries the secret). ID is the delete key (host for git,
// KEY for env, the store id for file/keyring); Name is the human label.
type Item struct {
	Kind string `json:"kind"` // "git" | "env" | "file" | "keyring"
	ID   string `json:"id"`   // delete key: host | KEY | file id | keyring id
	Name string `json:"name"` // display: host | KEY | path | service (username)
}

// Store is the per-user credential store rooted at a base dir.
type Store struct {
	base string // StateDir/credentials
	mu   sync.RWMutex
}

// New roots a store at baseDir (created if missing).
func New(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}
	return &Store{base: baseDir}, nil
}

func (s *Store) userDir(user string) string { return filepath.Join(s.base, user) }

// atomicWrite writes 0600 via temp+rename (never a torn credential file).
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// ─── git tier ───────────────────────────────────────────────────────────────

// PutGit stores (or replaces) the git credential for a user+host.
func (s *Store) PutGit(user string, c GitCred) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.userDir(user), "git-"+safe(c.Host)+".json"), append(data, '\n'))
}

// GetGit returns the git credential for a user+host, or ErrNotFound.
func (s *Store) GetGit(user, host string) (GitCred, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(filepath.Join(s.userDir(user), "git-"+safe(host)+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return GitCred{}, ErrNotFound
		}
		return GitCred{}, err
	}
	var c GitCred
	if err := json.Unmarshal(data, &c); err != nil {
		return GitCred{}, err
	}
	return c, nil
}

// ─── env tier ───────────────────────────────────────────────────────────────

// PutEnv stores (or replaces) a shell-variable credential. The value must be a
// single line with no control characters: the in-guest snippet delivers env
// creds line-by-line (KEY\tVALUE), so a newline or tab in the value would
// inject an extra exported variable into every login shell (ADR-032 review).
func (s *Store) PutEnv(user, key, value string) error {
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("invalid env key %q", key)
	}
	if hasCtrl(value) {
		return fmt.Errorf("env value for %q contains a control character", key)
	}
	// Real tokens are well under 1 KiB; the cap keeps a paste mistake (a whole
	// file as the "value") from being exported into every login shell.
	if len(value) > 4096 {
		return fmt.Errorf("env value for %q exceeds 4096 bytes", key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(EnvCred{Key: key, Value: value}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.userDir(user), "env-"+key+".json"), append(data, '\n'))
}

// AllEnv returns every env credential for a user as a key→value map (empty when
// none are onboarded).
func (s *Store) AllEnv(user string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	entries, err := os.ReadDir(s.userDir(user))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "env-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.userDir(user), e.Name()))
		if err != nil {
			continue
		}
		var c EnvCred
		if err := json.Unmarshal(data, &c); err != nil || !envKeyRe.MatchString(c.Key) || hasCtrl(c.Value) {
			continue
		}
		out[c.Key] = c.Value
	}
	return out, nil
}

// ─── file tier ──────────────────────────────────────────────────────────────

// PutFile stores (or replaces) a file credential. id is a stable key; path is
// validated home-relative (no absolute, no "..").
func (s *Store) PutFile(user string, c FileCred) error {
	if c.ID == "" || !envKeyRe.MatchString(strings.ReplaceAll(c.ID, "-", "_")) {
		return fmt.Errorf("invalid file id %q", c.ID)
	}
	if err := validRelPath(c.Path); err != nil {
		return err
	}
	if c.Mode == "" {
		c.Mode = "0600"
	}
	if _, err := strconv.ParseUint(c.Mode, 8, 32); err != nil {
		return fmt.Errorf("invalid mode %q", c.Mode)
	}
	// A credential file is a token/key/ADC — never large. Cap it so a hostile or
	// runaway guest can't push megabytes into the store (and thence into every
	// login shell). 256 KiB is generous next to real credentials (ssh key ~3 KB).
	if len(c.Data) > 256<<10 {
		return fmt.Errorf("file credential %q too large: %d bytes (max 256 KiB)", c.ID, len(c.Data))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := fileCredWire{ID: c.ID, Path: c.Path, Mode: c.Mode, DataB64: base64.StdEncoding.EncodeToString(c.Data)}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.userDir(user), "file-"+c.ID+".json"), append(data, '\n'))
}

// AllFiles returns every file credential for a user (empty when none).
func (s *Store) AllFiles(user string) ([]FileCred, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []FileCred
	entries, err := os.ReadDir(s.userDir(user))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "file-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.userDir(user), e.Name()))
		if err != nil {
			continue
		}
		var w fileCredWire
		if err := json.Unmarshal(data, &w); err != nil || validRelPath(w.Path) != nil {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(w.DataB64)
		if err != nil {
			continue
		}
		out = append(out, FileCred{ID: w.ID, Path: w.Path, Mode: w.Mode, Data: raw})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ─── keyring tier ───────────────────────────────────────────────────────────

// PutKeyring stores (or replaces) a Secret Service item credential. The
// service/username/label are interpolated into the login snippet's TSV
// delivery, so they must be single-line printable strings; the secret is
// arbitrary bytes (delivered base64).
func (s *Store) PutKeyring(user string, c KeyringCred) error {
	if c.ID == "" || !envKeyRe.MatchString(strings.ReplaceAll(c.ID, "-", "_")) {
		return fmt.Errorf("invalid keyring id %q", c.ID)
	}
	if c.Service == "" || hasCtrl(c.Service) || c.Username == "" || hasCtrl(c.Username) || hasCtrl(c.Label) {
		return fmt.Errorf("keyring service/username/label must be non-empty single-line strings")
	}
	if len(c.Secret) == 0 {
		return fmt.Errorf("empty keyring secret")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := keyringCredWire{ID: c.ID, Service: c.Service, Username: c.Username, Label: c.Label,
		SecretB64: base64.StdEncoding.EncodeToString(c.Secret)}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.userDir(user), "keyring-"+c.ID+".json"), append(data, '\n'))
}

// AllKeyring returns every keyring credential for a user (empty when none).
func (s *Store) AllKeyring(user string) ([]KeyringCred, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []KeyringCred
	entries, err := os.ReadDir(s.userDir(user))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "keyring-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.userDir(user), e.Name()))
		if err != nil {
			continue
		}
		var w keyringCredWire
		if err := json.Unmarshal(data, &w); err != nil ||
			w.Service == "" || hasCtrl(w.Service) || w.Username == "" || hasCtrl(w.Username) || hasCtrl(w.Label) {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(w.SecretB64)
		if err != nil || len(raw) == 0 {
			continue
		}
		out = append(out, KeyringCred{ID: w.ID, Service: w.Service, Username: w.Username, Label: w.Label, Secret: raw})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ─── management ─────────────────────────────────────────────────────────────

// List returns value-free descriptors of everything stored for a user, for the
// Settings status view.
func (s *Store) List(user string) ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	entries, err := os.ReadDir(s.userDir(user))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".json") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(s.userDir(user), n))
		// A corrupt store file yields zero values — skip it rather than emit a
		// ghost row (empty ID) the UI can render but never delete.
		switch {
		case strings.HasPrefix(n, "git-"):
			var c GitCred
			if json.Unmarshal(data, &c) == nil && c.Host != "" {
				out = append(out, Item{Kind: "git", ID: c.Host, Name: c.Host})
			}
		case strings.HasPrefix(n, "env-"):
			var c EnvCred
			if json.Unmarshal(data, &c) == nil && c.Key != "" {
				out = append(out, Item{Kind: "env", ID: c.Key, Name: c.Key})
			}
		case strings.HasPrefix(n, "file-"):
			var w fileCredWire
			if json.Unmarshal(data, &w) == nil && w.ID != "" {
				out = append(out, Item{Kind: "file", ID: w.ID, Name: w.Path})
			}
		case strings.HasPrefix(n, "keyring-"):
			var w keyringCredWire
			if json.Unmarshal(data, &w) == nil && w.ID != "" {
				out = append(out, Item{Kind: "keyring", ID: w.ID, Name: w.Service + " (" + w.Username + ")"})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Delete removes one credential by kind+name. Kinds: "git" (name=host),
// "env" (name=KEY), "file" (name=id).
func (s *Store) Delete(user, kind, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var fn string
	switch kind {
	case "git":
		fn = "git-" + safe(name) + ".json"
	case "env":
		if !envKeyRe.MatchString(name) {
			return fmt.Errorf("invalid env key %q", name)
		}
		fn = "env-" + name + ".json"
	case "file":
		if name == "" || strings.ContainsAny(name, "/\\") {
			return fmt.Errorf("invalid file id %q", name)
		}
		fn = "file-" + name + ".json"
	case "keyring":
		if name == "" || strings.ContainsAny(name, "/\\") {
			return fmt.Errorf("invalid keyring id %q", name)
		}
		fn = "keyring-" + name + ".json"
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
	err := os.Remove(filepath.Join(s.userDir(user), fn))
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	return err
}

// validRelPath rejects anything that could escape the guest home: absolute
// paths and any ".." segment. A leading "~/" is allowed (the login snippet
// strips it). The materialized target is always under $HOME.
func validRelPath(p string) error {
	if p == "" {
		return errors.New("empty path")
	}
	rel := strings.TrimPrefix(p, "~/")
	if strings.HasPrefix(rel, "/") {
		return fmt.Errorf("path must be home-relative: %q", p)
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return fmt.Errorf("path escapes home: %q", p)
		}
	}
	return nil
}

// hasCtrl reports whether s contains any control character (incl. newline,
// carriage return, tab, and DEL) — such a value must never reach the login
// snippet's line-based env delivery.
func hasCtrl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// safe reduces a host to a filename-safe token (no path separators).
func safe(host string) string {
	out := make([]rune, 0, len(host))
	for _, r := range host {
		if r == '/' || r == '\\' || r == '.' || r == ':' {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "default"
	}
	return string(out)
}
