// Package config is the single source of truth for styrman's ports, paths,
// label names, env keys, and per-container settings.
package config

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kesokaj/styrman-crew/internal/identity"
)

// Container labels. LabelManaged marks containers styrman owns; state recovery
// adopts everything carrying it (ADR-007).
const (
	LabelManaged = "styrman.managed"
	LabelProject = "styrman.project"
	LabelUser    = "styrman.user"
	LabelName    = "styrman.name" // friendly project name (informational)
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
	EnvCredBroker = "STYRMAN_CRED_BROKER" // credential broker URL on this bridge (ADR-030)
)

// WebSocket close codes, ported verbatim from devbox's terminal protocol.
const (
	// CloseShellExited: the tmux session genuinely ended (user typed exit /
	// agent quit). The frontend must not reconnect into a fresh session
	// without the user asking.
	CloseShellExited = 4000
	// CloseProjectStopped: the project VM was deliberately stopped (Stop
	// button / idle reaper) — the client must NOT auto-reconnect (that would
	// boot the VM back up); an explicit user action (Start) reconnects with
	// boot intent instead.
	CloseProjectStopped = 4002
	// CloseReplaced: a newer WebSocket attached to the same conversation.
	// The frontend must NOT auto-reconnect on this code.
	CloseReplaced = 4001
)

// Config holds runtime configuration resolved from the environment.
type Config struct {
	Addr        string // HTTP listen address
	PreviewBase string // public preview host base, e.g. "public.crew.styrman.xyz" (empty = path-form links only, 1.1)
	PrivateBase string // private (email-gated) preview host base, e.g. "private.crew.styrman.xyz" (empty = path-form only)
	StateDir    string // root state dir (default /var/lib/styrman)
	HomesDir    string // per-user homes root (StateDir/homes)
	HomeDir     string // LEGACY single-user home; migrated to HomesDir on boot
	Image       string // runtime image for project containers
	Runtime     string // docker runtime (mvm = Kata)
	VCPUs       int    // per-project vCPU limit, 0 = Kata's default (STYRMAN_VCPUS)
	MemoryMB    int    // per-project memory MiB, 0 = Kata's default (STYRMAN_MEMORY_MB)
	Username    string // in-guest username; owns the shared home
	UID, GID    int    // host uid/gid the guest user must match

	// Operator-tunable settings (Settings → Runtime). Env supplies the
	// defaults; StateDir/settings.json overrides them and PATCH
	// /api/settings/runtime updates both live — hence the mutex: the idle
	// reaper goroutine reads while a request writes.
	opMu sync.RWMutex
	// persistMu serializes SetOperator's settings.json write OUTSIDE opMu, so
	// readers (the reaper tick, GET /settings) never block on an fsync
	// (2026-08-30 audit-era cleanup).
	persistMu   sync.Mutex
	dockerDisk  int           // per-project docker.img size in GB (new VMs)
	idleTimeout time.Duration // stop project containers idle this long (0 = never)
	// TranscriptTTL: terminal transcripts older than this are pruned by the
	// janitor (0 = keep forever). Transcripts live in each per-project home
	// (~/.styrman/logs, ADR-017) and grew unbounded before v1.14.0.
	TranscriptTTL time.Duration

	// BrokerAddr: the credential broker's listener (ADR-030). Bound on all
	// interfaces so projects reach it on their own bridge; NOT published to
	// the host, so it is a bridge-only, credential-only surface separate
	// from the ingress-gated main API. Empty disables the broker.
	BrokerAddr string

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
	// FirewallRunDir: the root-owned run dir of the host firewall script
	// (ADR-065) — its spool/ (server-writable, root-consumed trigger files)
	// and status (root-written, server-read). STYRMAN_FIREWALL_DIR, default
	// /run/styrman-firewall, bind-mounted into the server container by
	// compose. Empty disables the per-project LAN feature: every marker is
	// removed at boot (root then closes every allowance), enables answer 503,
	// and a record still on reports "cannot confirm".
	FirewallRunDir string
}

// Load resolves configuration from the environment with sensible defaults.
// UID/GID default to the owner of HomeDir if it exists, else the current user.
func Load() (*Config, error) {
	c := &Config{
		Addr:        envOr("STYRMAN_ADDR", "127.0.0.1:8080"),
		PreviewBase: envOr("STYRMAN_PREVIEW_BASE", ""),
		PrivateBase: envOr("STYRMAN_PRIVATE_BASE", ""),
		StateDir:    envOr("STYRMAN_STATE_DIR", "/var/lib/styrman"),
		Image:       os.Getenv("STYRMAN_IMAGE"), // required — validated below
		Runtime:     envOr("STYRMAN_RUNTIME", "mvm"),
		VCPUs:       envIntOr("STYRMAN_VCPUS", 0),
		MemoryMB:    envIntOr("STYRMAN_MEMORY_MB", 0),
		dockerDisk:  envIntOr("STYRMAN_DOCKER_DISK_GB", 20),
		// 8h (was 24h): an idle-stop only powers the VM off — workspace, home,
		// tmux transcripts and the docker disk all persist, and the next attach
		// boots it back exactly where it left off. Real in-guest work (running
		// DinD containers, a non-shell foreground process in any tmux pane)
		// blocks the stop regardless of this timer (stopIfIdle's busy probe).
		idleTimeout:    envDurationOr("STYRMAN_IDLE_TIMEOUT", 8*time.Hour),
		TranscriptTTL:  envDurationOr("STYRMAN_TRANSCRIPT_TTL", 30*24*time.Hour),
		BrokerAddr:     envOr("STYRMAN_BROKER_ADDR", ":8091"),
		AuthMode:       envOr("STYRMAN_AUTH", "none"),
		DevAddr:        os.Getenv("STYRMAN_DEV_ADDR"),
		SelfContainer:  os.Getenv("STYRMAN_SELF_CONTAINER"),
		IngressToken:   os.Getenv("STYRMAN_INGRESS_TOKEN"),
		FirewallRunDir: envOr("STYRMAN_FIREWALL_DIR", "/run/styrman-firewall"),
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
	// Auth mode must be exactly one we handle; a typo must fail closed, never
	// silently fall through to an unguarded path (M16).
	if c.AuthMode != "none" && c.AuthMode != "header" {
		return nil, fmt.Errorf("STYRMAN_AUTH=%q is invalid — must be \"none\" or \"header\"", c.AuthMode)
	}
	// Header mode authenticates requests by the shared ingress token; an empty
	// token makes the server trust a raw client-supplied X-Caddy-Email from
	// anyone who can reach it on a project bridge — full impersonation, admin
	// included. Refuse to start without one (H2), the same fail-closed
	// discipline already applied to the dev listener above.
	if c.AuthMode == "header" && c.IngressToken == "" {
		return nil, fmt.Errorf("STYRMAN_AUTH=header requires STYRMAN_INGRESS_TOKEN to be set — an empty token lets any project VM spoof identity headers")
	}
	// No image default (operator rule: no mutable tags). The old fallback was
	// styrman-runtime:latest — a silent mutable pin whenever env wiring broke;
	// compose derives styrman-runtime:${STYRMAN_VERSION}, dev runs pass :dev
	// explicitly (README), so an empty value only ever means a broken deploy.
	if c.Image == "" {
		return nil, fmt.Errorf("STYRMAN_IMAGE is not set — deploys use styrman-runtime:<x.y.z> (compose derives it from STYRMAN_VERSION); local dev: STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman")
	}
	// The containment model ASSUMES a VM runtime, so assert it. Every project
	// container is created with CAP_SYS_ADMIN + NET_ADMIN + NET_RAW and
	// apparmor=unconfined (orchestrator.create) — safe only because a Kata
	// microVM, not the host kernel, is the boundary. Under runc those exact
	// flags hand every project user host root. Runtime used to be a free
	// string in a Load that otherwise fails closed on the image, the auth mode
	// and the ingress token (2026-09-01 audit).
	if !vmRuntimes[c.Runtime] {
		if os.Getenv("STYRMAN_ALLOW_UNSAFE_RUNTIME") != "1" {
			return nil, fmt.Errorf(
				"STYRMAN_RUNTIME=%q is not a VM runtime (known: %s) — project containers "+
					"get CAP_SYS_ADMIN and apparmor=unconfined, which is containment only "+
					"when the runtime is a microVM; set STYRMAN_ALLOW_UNSAFE_RUNTIME=1 to "+
					"override deliberately", c.Runtime, strings.Join(knownVMRuntimes(), ", "))
		}
		// Deliberate override: say so loudly and exactly once, at boot.
		fmt.Fprintf(os.Stderr,
			"styrman: WARNING — STYRMAN_RUNTIME=%q is not a VM runtime and "+
				"STYRMAN_ALLOW_UNSAFE_RUNTIME=1 is set. Project containers get "+
				"CAP_SYS_ADMIN with AppArmor disabled; on a non-VM runtime that is "+
				"host root for every project user.\n", c.Runtime)
	}
	// The shared home's owner is authoritative for the in-guest uid/gid.
	if st, err := os.Stat(c.HomeDir); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			c.UID = int(sys.Uid)
			c.GID = int(sys.Gid)
		}
	}
	c.loadOverrides()
	return c, nil
}

// ─── Operator-tunable settings ──────────────────────────────────────────────

// operatorOverrides is the persisted shape of StateDir/settings.json.
type operatorOverrides struct {
	DockerDiskGB int    `json:"dockerDiskGB,omitempty"`
	IdleTimeout  string `json:"idleTimeout,omitempty"`
}

func (c *Config) overridesPath() string {
	return filepath.Join(c.StateDir, "settings.json")
}

// loadOverrides applies persisted operator settings over the env defaults.
// A missing file is normal; an unparseable one is loudly ignored (silently
// reverting to env defaults would look like lost settings). The same bounds
// as the API apply — a hand-edited file doesn't get to bypass them.
func (c *Config) loadOverrides() {
	data, err := os.ReadFile(c.overridesPath())
	if err != nil {
		return
	}
	var o operatorOverrides
	if err := json.Unmarshal(data, &o); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: ignoring unparseable %s: %v\n", c.overridesPath(), err)
		return
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if o.DockerDiskGB >= 5 && o.DockerDiskGB <= 500 {
		c.dockerDisk = o.DockerDiskGB
	}
	if o.IdleTimeout != "" {
		if d, err := time.ParseDuration(o.IdleTimeout); err == nil && d >= 0 {
			c.idleTimeout = d
		}
	}
}

// DockerDiskGB is the docker.img size for NEWLY created project VMs.
func (c *Config) DockerDiskGB() int {
	c.opMu.RLock()
	defer c.opMu.RUnlock()
	return c.dockerDisk
}

// IdleTimeoutD is the current idle-stop threshold (0 = never). The reaper
// reads this on every tick, so a change applies without a restart.
func (c *Config) IdleTimeoutD() time.Duration {
	c.opMu.RLock()
	defer c.opMu.RUnlock()
	return c.idleTimeout
}

// SetOperator validates, persists, then applies the tunable settings —
// persist FIRST, so a write failure never leaves live state diverged from
// what the next restart will load. diskGB 0 / idle < 0 mean "keep".
func (c *Config) SetOperator(diskGB int, idle time.Duration) error {
	if diskGB != 0 && (diskGB < 5 || diskGB > 500) {
		return fmt.Errorf("docker disk must be 5-500 GB")
	}
	// persistMu serializes concurrent SetOperator calls end-to-end; the value
	// lock (opMu) is only held for the snapshot and the final in-memory apply,
	// so readers never block on the fsync below.
	c.persistMu.Lock()
	defer c.persistMu.Unlock()
	c.opMu.RLock()
	next := operatorOverrides{
		DockerDiskGB: c.dockerDisk,
		IdleTimeout:  c.idleTimeout.String(),
	}
	c.opMu.RUnlock()
	if diskGB != 0 {
		next.DockerDiskGB = diskGB
	}
	if idle >= 0 {
		next.IdleTimeout = idle.String()
	}
	data, _ := json.MarshalIndent(next, "", "  ")
	// Atomic temp+fsync+rename, like every other store: a crash mid-write
	// must never leave a truncated settings.json to be silently discarded.
	tmp := c.overridesPath() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("persist settings: %w", err)
	}
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, c.overridesPath())
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("persist settings: %w", err)
	}
	c.opMu.Lock()
	c.dockerDisk = next.DockerDiskGB
	if idle >= 0 {
		c.idleTimeout = idle
	}
	c.opMu.Unlock()
	return nil
}

// UserHome returns a user's home directory on the host.
//
// NOTE (ADR-031): homes are PER-PROJECT now. UserHome remains the per-user
// BASE (HomesDir/<user>) — used for the setup VM and as the parent of the
// per-project homes. Project microVMs mount ProjectHome instead.
// TenantDir is HomesDir/<user>: the server-only parent of base/ and p/. No
// guest ever mounts it (ADR-064); what a guest CAN write is a separate bind
// below it — base/ (the setup VM) and p/<uuid> (each project).
func (c *Config) TenantDir(user string) string { return filepath.Join(c.HomesDir, user) }

// UserHome is the setup VM's home, HomesDir/<user>/base (ADR-064). It was the
// tenant dir itself, which put every project home inside a root guest's
// mount — the layout behind BUG-002/003.
func (c *Config) UserHome(user string) string { return filepath.Join(c.TenantDir(user), "base") }

// ProjectHome is a project's OWN home directory (ADR-031): each project gets
// a fresh, individual home so no tool state leaks between projects. The only
// thing shared across a user's projects is credentials (via the broker,
// ADR-030). Keyed by UUID; the parent is never mounted into any guest.
func (c *Config) ProjectHome(user, project string) string {
	return filepath.Join(c.TenantDir(user), "p", project)
}

// LegacyWorkspaceExists reports whether a project's PRE-ADR-027 workspace is
// still sitting in the shared home — the signal that migration hasn't
// completed and nothing may create (and thereby permanently shadow) the new
// location with an empty dir.
func (c *Config) LegacyWorkspaceExists(user, projectDir string) bool {
	p := filepath.Join(c.UserHome(user), "projects", projectDir)
	st, err := os.Lstat(p)
	if err != nil || !st.IsDir() {
		return false
	}
	// An empty dir is only the bind's mount-point stub (Docker recreates it
	// inside the home on every start) — not a real unmigrated tree.
	// Unreadable counts as content: fail safe, never shadow real data.
	entries, err := os.ReadDir(p)
	return err != nil || len(entries) > 0
}

// ProjectWorkspace is the host-side working tree of one project (ADR-027):
// per-project storage OUTSIDE the shared home, bind-mounted into the microVM
// at ~/projects/<dir>. Keyed by UUID — names are renamable. The parent
// (StateDir/projects/<user>) is never mounted into any guest, so the path
// itself cannot be redirected by a guest symlink; contents are guest-written.
func (c *Config) ProjectWorkspace(user, projectID string) string {
	return filepath.Join(c.StateDir, "projects", user, projectID)
}

// UserStateDir returns where a user's metadata stores live.
func (c *Config) UserStateDir(user string) string {
	return filepath.Join(c.StateDir, "users", user)
}

// NetPolicyDir is the server-only tree the per-project LAN markers live in
// (ADR-065): NetPolicyDir()/lan/<uuid>, an empty file whose NAME is the whole
// grammar the root firewall script reads. Never a bind source — it sits
// beside users/ and projects/, never under a mounted home — so no guest can
// plant a symlink in it (.agents/rules/guest-writable-paths.md).
func (c *Config) NetPolicyDir() string {
	return filepath.Join(c.StateDir, "netpolicy")
}

// DefaultUser is the identity used when auth is off (dev door / local dev):
// the first allowed email if configured (so the loopback door acts as the
// admin), else the OS operator.
func (c *Config) DefaultUser() (email, username string) {
	if len(c.AllowedEmails) > 0 {
		return c.AllowedEmails[0], identity.DeriveUsername(c.AllowedEmails[0])
	}
	return "", c.Username
}

// vmRuntimes are the Docker runtimes that put a project in its OWN kernel.
// "mvm" is this deployment's Kata runtime name (deploy/host-setup.sh registers
// it in daemon.json); the kata-* names are the upstream defaults.
var vmRuntimes = map[string]bool{
	"mvm":          true,
	"kata":         true,
	"kata-runtime": true,
	"kata-qemu":    true,
	"kata-clh":     true,
	"kata-fc":      true,
}

// knownVMRuntimes lists them in a stable order for the error message.
func knownVMRuntimes() []string {
	out := make([]string, 0, len(vmRuntimes))
	for r := range vmRuntimes {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// ContainerName returns the container name for a project. Identity is the
// project UUID — globally unique, so no user prefix is needed and friendly
// names can repeat freely.
func ContainerName(projectID string) string { return "styrman-" + projectID }

// NetworkName returns its bridge network name.
func NetworkName(projectID string) string { return "sdb-" + projectID }

// ProjectFromNetworkName inverts NetworkName ("" when netName isn't a plain
// project bridge — e.g. the setup VM's sdb-sys-setup-<user>). Keeps the
// prefix in exactly one file (2026-08-30 audit).
func ProjectFromNetworkName(netName string) string {
	rest, ok := strings.CutPrefix(netName, "sdb-")
	if !ok || strings.HasPrefix(rest, "sys-setup-") {
		return ""
	}
	return rest
}

// BridgeNameFor is the Linux bridge INTERFACE name for a styrman network. Docker
// would otherwise auto-name it br-<hash> — which the project→host/LAN isolation
// firewall (matching `-i sdb-+`) cannot target — and interface names are capped
// at 15 chars, so the full NetworkName is too long. A short sha1-derived
// `sdb-<11hex>` (15 chars) keeps both the firewall match and uniqueness.
func BridgeNameFor(netName string) string {
	sum := sha1.Sum([]byte(netName))
	return "sdb-" + hex.EncodeToString(sum[:])[:11]
}

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
