// Package config is the single source of truth for styrman's ports, paths,
// label names, env keys, and per-container settings.
package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Container labels. LabelManaged marks containers styrman owns; state recovery
// adopts everything carrying it (ADR-007).
const (
	LabelManaged = "styrman.managed"
	LabelProject = "styrman.project"
	LabelAgent   = "styrman.agent"
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
}

// Load resolves configuration from the environment with sensible defaults.
// UID/GID default to the owner of HomeDir if it exists, else the current user.
func Load() (*Config, error) {
	c := &Config{
		Addr:        envOr("STYRMAN_ADDR", "127.0.0.1:8080"),
		StateDir:    envOr("STYRMAN_STATE_DIR", "/var/lib/styrman"),
		Image:       envOr("STYRMAN_IMAGE", "styrman-runtime:latest"),
		Runtime:     envOr("STYRMAN_RUNTIME", "mvm"),
		KeyringPW:   envOr("STYRMAN_KEYRING_PW", "styrman"),
		DockerDisk:  envIntOr("STYRMAN_DOCKER_DISK_GB", 20),
		IdleTimeout: envDurationOr("STYRMAN_IDLE_TIMEOUT", 24*time.Hour),
	}
	c.HomeDir = envOr("STYRMAN_HOME_DIR", filepath.Join(c.StateDir, "home"))

	cur, err := user.Current()
	if err != nil {
		return nil, err
	}
	c.Username = envOr("STYRMAN_USER", cur.Username)
	c.UID, _ = strconv.Atoi(cur.Uid)
	c.GID, _ = strconv.Atoi(cur.Gid)
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
