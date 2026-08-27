// Package config is the single source of truth for styrman's ports, paths,
// label names, env keys, and per-container settings.
package config

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Container labels. LabelManaged marks containers styrman owns; state recovery
// adopts everything carrying it (ADR-007).
const (
	LabelManaged = "styrman.managed"
	LabelProject = "styrman.project"
	LabelUser    = "styrman.user"
	LabelName    = "styrman.name" // friendly project name (informational)
	LabelAgent   = "styrman.agent"
	// LabelSystem marks hidden system containers (e.g. the onboarding VM);
	// they carry no project label and never appear in project listings.
	LabelSystem = "styrman.system"
)

// SetupContainer names a user's hidden onboarding microVM (see /api/setup).
func SetupContainer(user string) string { return "styrman-sys-setup-" + user }

// SetupNetwork names its bridge network.
func SetupNetwork(user string) string { return "sdb-sys-setup-" + user }

// Env keys passed into project containers (read by image/entrypoint.sh).
const (
	EnvUser       = "STYRMAN_USER"
	EnvUID        = "STYRMAN_UID"
	EnvGID        = "STYRMAN_GID"
	EnvProject    = "STYRMAN_PROJECT"     // the project UUID (disks, transcript paths)
	EnvProjectDir = "STYRMAN_PROJECT_DIR" // workspace dir name under ~/projects
	EnvDockerDisk = "STYRMAN_DOCKER_DISK_GB"
	EnvKeyringPW  = "STYRMAN_KEYRING_PW"
)

// WebSocket close codes, ported verbatim from devbox's terminal protocol.
const (
	// CloseShellExited: the tmux session genuinely ended (user typed exit /
	// agent quit). The frontend must not reconnect into a fresh session
	// without the user asking.
	CloseShellExited = 4000
	// CloseReplaced: a newer WebSocket attached to the same conversation.
	// The frontend must NOT auto-reconnect on this code.
	CloseReplaced = 4001
)

// Agent CLIs a project can be created with. "shell" is used internally for
// plain terminals and is not a valid project agent.
var Agents = map[string]string{
	"claude": "claude",
	"agy":    "agy",
}

// Config holds runtime configuration resolved from the environment.
type Config struct {
	Addr        string        // HTTP listen address
	PreviewBase string        // public preview host base, e.g. "preview.styrman.xyz" (empty = path-form links only, 1.1)
	StateDir    string        // root state dir (default /var/lib/styrman)
	HomesDir    string        // per-user homes root (StateDir/homes)
	HomeDir     string        // LEGACY single-user home; migrated to HomesDir on boot
	Image       string        // runtime image for project containers
	Runtime     string        // docker runtime (mvm = Kata)
	Username    string        // in-guest username; owns the shared home
	UID, GID    int           // host uid/gid the guest user must match
	DockerDisk  int           // per-project docker.img size in GB
	KeyringPW   string        // password for the in-guest gnome-keyring
	IdleTimeout time.Duration // stop project containers idle this long (0 = never)

	// AuthMode "none" (localhost dev) or "header" (behind the Caddy ingress,
	// which injects X-Caddy-Email after Google OAuth). AllowedEmails seeds
	// the userstore from the environment.
	AuthMode      string
	AllowedEmails []string
	// SelfContainer: when the server itself runs in a container, its name —
	// the orchestrator attaches it to each project network so the port-preview
	// proxy can reach microVM IPs across docker's bridge isolation. Empty =
	// host process (reaches all bridges natively).
	SelfContainer string
	// DevAddr: optional second listener with auth bypassed, for testing
	// without the OAuth ingress. Publish it on host loopback ONLY
	// (compose: "127.0.0.1:8080:8081") — anyone who can reach it is operator.
	DevAddr string
	// IngressToken: when set, header-mode requests must carry it in
	// X-Styrman-Ingress (injected by the Caddy ingress). Blocks a project VM
	// on the shared bridge from reaching :8080 and spoofing X-Caddy-Email.
	// Empty = disabled (current behavior) — enable with the ingress.
	IngressToken string
}

// Load resolves configuration from the environment with sensible defaults.
// UID/GID default to the owner of HomeDir if it exists, else the current user.
func Load() (*Config, error) {
	c := &Config{
		Addr:          envOr("STYRMAN_ADDR", "127.0.0.1:8080"),
		PreviewBase:   envOr("STYRMAN_PREVIEW_BASE", ""),
		StateDir:      envOr("STYRMAN_STATE_DIR", "/var/lib/styrman"),
		Image:         envOr("STYRMAN_IMAGE", "styrman-runtime:latest"),
		Runtime:       envOr("STYRMAN_RUNTIME", "mvm"),
		KeyringPW:     envOr("STYRMAN_KEYRING_PW", "styrman"),
		DockerDisk:    envIntOr("STYRMAN_DOCKER_DISK_GB", 20),
		IdleTimeout:   envDurationOr("STYRMAN_IDLE_TIMEOUT", 24*time.Hour),
		AuthMode:      envOr("STYRMAN_AUTH", "none"),
		DevAddr:       os.Getenv("STYRMAN_DEV_ADDR"),
		SelfContainer: os.Getenv("STYRMAN_SELF_CONTAINER"),
		IngressToken:  os.Getenv("STYRMAN_INGRESS_TOKEN"),
	}
	for _, e := range strings.Split(os.Getenv("STYRMAN_ALLOWED_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			c.AllowedEmails = append(c.AllowedEmails, e)
		}
	}
	c.HomeDir = envOr("STYRMAN_HOME_DIR", filepath.Join(c.StateDir, "home"))
	c.HomesDir = envOr("STYRMAN_HOMES_DIR", filepath.Join(c.StateDir, "homes"))

	// Username: env wins; user.Current() is best-effort — it fails in a
	// static binary without $USER (the containerized server), where
	// STYRMAN_USER is required instead.
	c.Username = os.Getenv("STYRMAN_USER")
	c.UID, c.GID = os.Getuid(), os.Getgid()
	if cur, err := user.Current(); err == nil && c.Username == "" {
		c.Username = cur.Username
	}
	if c.Username == "" {
		return nil, fmt.Errorf("cannot determine username — set STYRMAN_USER")
	}
	// The dev listener bypasses auth entirely, so it MUST bind loopback only.
	// The server container is joined to project bridges for port-preview, so a
	// 0.0.0.0 bind would hand every project VM unauthenticated admin — refuse
	// to start rather than trust a compose comment.
	if c.DevAddr != "" && !isLoopbackAddr(c.DevAddr) {
		return nil, fmt.Errorf("STYRMAN_DEV_ADDR=%q must bind a loopback address (127.0.0.1/::1) — it bypasses auth", c.DevAddr)
	}
	// Same danger via a forgotten env var: auth=none on a non-loopback main
	// listener is an open admin API. Behind the ingress the mode is "header".
	if c.AuthMode == "none" && !isLoopbackAddr(c.Addr) {
		return nil, fmt.Errorf("STYRMAN_AUTH=none requires STYRMAN_ADDR to be loopback (got %q) — it has no authentication", c.Addr)
	}
	// The shared home's owner is authoritative for the in-guest uid/gid.
	if st, err := os.Stat(c.HomeDir); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			c.UID = int(sys.Uid)
			c.GID = int(sys.Gid)
		}
	}
	return c, nil
}

// ProjectsFile is the metadata store path.
func (c *Config) ProjectsFile() string { return filepath.Join(c.StateDir, "projects.json") }

// UserHome returns a user's home directory on the host.
func (c *Config) UserHome(user string) string { return filepath.Join(c.HomesDir, user) }

// UserStateDir returns where a user's metadata stores live.
func (c *Config) UserStateDir(user string) string {
	return filepath.Join(c.StateDir, "users", user)
}

// DefaultUser is the identity used when auth is off (dev door / local dev):
// the first allowed email if configured (so the loopback door acts as the
// admin), else the OS operator.
func (c *Config) DefaultUser() (email, username string) {
	if len(c.AllowedEmails) > 0 {
		return c.AllowedEmails[0], usernameFromEmail(c.AllowedEmails[0])
	}
	return "", c.Username
}

// usernameFromEmail is identity.DeriveUsername without the import cycle.
func usernameFromEmail(email string) string {
	name, _, _ := strings.Cut(email, "@")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		default:
			return -1
		}
	}, name)
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "user-" + name
	}
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

// ContainerName returns the container name for a project. Identity is the
// project UUID — globally unique, so no user prefix is needed and friendly
// names can repeat freely.
func ContainerName(projectID string) string { return "styrman-" + projectID }

// NetworkName returns its bridge network name.
func NetworkName(projectID string) string { return "sdb-" + projectID }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDurationOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// isLoopbackAddr reports whether a host:port listen address binds only the
// loopback interface. An empty or "*"/0.0.0.0 host is NOT loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
