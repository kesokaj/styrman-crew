// Package config is the single source of truth for styrman's ports, paths,
// label names, env keys, and per-container settings.
package config

import (
	"fmt"
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
	LabelAgent   = "styrman.agent"
	// LabelSystem marks hidden system containers (e.g. the onboarding VM);
	// they carry no project label and never appear in project listings.
	LabelSystem = "styrman.system"
)

// Hidden onboarding microVM (runs styrman-setup; see /api/setup).
const (
	SetupContainer = "styrman-sys-setup"
	SetupNetwork   = "sdb-sys-setup"
)

// Env keys passed into project containers (read by image/entrypoint.sh).
const (
	EnvUser       = "STYRMAN_USER"
	EnvUID        = "STYRMAN_UID"
	EnvGID        = "STYRMAN_GID"
	EnvProject    = "STYRMAN_PROJECT"
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
	StateDir    string        // root state dir (default /var/lib/styrman)
	HomeDir     string        // shared home mounted into every microVM
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
}

// Load resolves configuration from the environment with sensible defaults.
// UID/GID default to the owner of HomeDir if it exists, else the current user.
func Load() (*Config, error) {
	c := &Config{
		Addr:          envOr("STYRMAN_ADDR", "127.0.0.1:8080"),
		StateDir:      envOr("STYRMAN_STATE_DIR", "/var/lib/styrman"),
		Image:         envOr("STYRMAN_IMAGE", "styrman-runtime:latest"),
		Runtime:       envOr("STYRMAN_RUNTIME", "mvm"),
		KeyringPW:     envOr("STYRMAN_KEYRING_PW", "styrman"),
		DockerDisk:    envIntOr("STYRMAN_DOCKER_DISK_GB", 20),
		IdleTimeout:   envDurationOr("STYRMAN_IDLE_TIMEOUT", 24*time.Hour),
		AuthMode:      envOr("STYRMAN_AUTH", "none"),
		DevAddr:       os.Getenv("STYRMAN_DEV_ADDR"),
		SelfContainer: os.Getenv("STYRMAN_SELF_CONTAINER"),
	}
	for _, e := range strings.Split(os.Getenv("STYRMAN_ALLOWED_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			c.AllowedEmails = append(c.AllowedEmails, e)
		}
	}
	c.HomeDir = envOr("STYRMAN_HOME_DIR", filepath.Join(c.StateDir, "home"))

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

// ContainerName returns the container name for a project.
func ContainerName(project string) string { return "styrman-" + project }

// NetworkName returns the per-project bridge network name.
func NetworkName(project string) string { return "sdb-" + project }

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
