// Package orchestrator manages the lifecycle of per-project Kata microVMs via
// the Docker SDK. The Kata container configuration is ported from devbox's
// shell-launcher orchestrator (see ADR-001): CapAdd instead of --privileged,
// explicit DNS, per-project bridge network, apparmor unconfined.
package orchestrator

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/filters"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// Orchestrator creates, adopts, and reaps project microVMs.
type Orchestrator struct {
	docker *client.Client
	cfg    *config.Config
	logger *slog.Logger

	mu        sync.Mutex             // guards projLocks
	projLocks map[string]*sync.Mutex // per-project create/start serialization
	activity  sync.Map               // project name → time.Time of last terminal activity
	// stopped marks projects deliberately stopped (Stop button / idle reaper).
	// While set, an automatic terminal-WS reconnect must NOT boot the VM back
	// (that made Stop un-stick whenever a tab was open); any intentional boot
	// (EnsureRunning) clears it. Key: key(user, project).
	stopped sync.Map
	// selfAttached memoizes EnsureSelfAttached per project (TTL) so the
	// per-request preview proxy doesn't hit the Docker API every time.
	selfAttached sync.Map

	// ipCache memoizes broker source-IP → (user,project) resolution for a
	// short TTL, so a project calling the broker per git op can't drive
	// unbounded Docker API calls (review finding).
	ipCacheMu sync.Mutex
	ipCache   map[string]ipCacheEntry
}

type ipCacheEntry struct {
	user, project string
	at            time.Time
}

const ipCacheTTL = 30 * time.Second

func (o *Orchestrator) ipCacheGet(ip string) (user, project string, ok bool) {
	o.ipCacheMu.Lock()
	defer o.ipCacheMu.Unlock()
	e, found := o.ipCache[ip]
	if !found || time.Since(e.at) > ipCacheTTL {
		return "", "", false
	}
	return e.user, e.project, true
}

func (o *Orchestrator) ipCachePut(ip, user, project string) {
	o.ipCacheMu.Lock()
	defer o.ipCacheMu.Unlock()
	if o.ipCache == nil {
		o.ipCache = map[string]ipCacheEntry{}
	}
	o.ipCache[ip] = ipCacheEntry{user: user, project: project, at: time.Now()}
}

func (o *Orchestrator) ipCacheDelete(ip string) {
	o.ipCacheMu.Lock()
	defer o.ipCacheMu.Unlock()
	delete(o.ipCache, ip)
}

// containerHoldsIP reports whether the container currently has ip on any of
// its attached networks. The source IP of a broker request is the project
// container's bridge IP (its DinD sub-containers NAT out behind it), so this
// confirms a cached IP→project mapping still points at the SAME live
// container — the guard against a freed subnet/IP being reused by another
// project (possibly another user) after a destroy.
func containerHoldsIP(cinsp container.InspectResponse, ip string) bool {
	if cinsp.NetworkSettings == nil {
		return false
	}
	for _, ep := range cinsp.NetworkSettings.Networks {
		if ep != nil && ep.IPAddress == ip {
			return true
		}
	}
	return false
}

// lockProject returns the mutex for one project, creating it on first use.
// Locking per project keeps a slow cold start (up to 90 s waitReady) from
// stalling attaches to every other project (review finding).
func (o *Orchestrator) lockProject(project string) *sync.Mutex {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.projLocks == nil {
		o.projLocks = map[string]*sync.Mutex{}
	}
	m, ok := o.projLocks[project]
	if !ok {
		m = &sync.Mutex{}
		o.projLocks[project] = m
	}
	return m
}

// New creates an Orchestrator using the default Docker socket.
func New(cfg *config.Config, logger *slog.Logger) (*Orchestrator, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Orchestrator{docker: cli, cfg: cfg, logger: logger}, nil
}

// Status describes a project's container state.
type Status struct {
	User        string `json:"user"`
	Project     string `json:"project"`
	ContainerID string `json:"containerId,omitempty"`
	State       string `json:"state"` // "running", "stopped", "absent"
}

func key(user, project string) string { return user + "/" + project }

// Touch records terminal activity for the idle reaper.
func (o *Orchestrator) Touch(user, project string) { o.activity.Store(key(user, project), time.Now()) }

// Status inspects the project's container.
func (o *Orchestrator) Status(ctx context.Context, user, project string) (Status, error) {
	insp, err := o.docker.ContainerInspect(ctx, config.ContainerName(project))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return Status{User: user, Project: project, State: "absent"}, nil
		}
		return Status{}, err
	}
	st := "stopped"
	if insp.State != nil && insp.State.Running {
		st = "running"
	}
	return Status{User: user, Project: project, ContainerID: insp.ID, State: st}, nil
}

// RunOpts carries the project facts the container needs beyond its UUID.
type RunOpts struct {
	Name      string // friendly name (label + hostname)
	Dir       string // workspace dir under ~/projects
	GuestUser string // in-guest username (per-user setting)
	// Lan is the project's network mode (ADR-067): false = SANDBOX (bridge
	// sdb-<hash>, matched by the root firewall's drops, internet only),
	// true = LAN (bridge lan-<hash>, matched by no styrman rule, so the
	// project reaches whatever the host reaches). It is the caller's copy of
	// Project.LanAccess and it is the ONLY input to the bridge name.
	Lan bool
}

// EnsureRunning makes sure the project's microVM exists and is running, and
// waits until the entrypoint reports ready. project is the UUID. Returns the
// container ID.
func (o *Orchestrator) EnsureRunning(ctx context.Context, user, project string, opts RunOpts) (string, error) {
	l := o.lockProject(key(user, project))
	l.Lock()
	defer l.Unlock()

	name := config.ContainerName(project)
	id := ""
	insp, err := o.docker.ContainerInspect(ctx, name)
	// Pre-ADR-027 containers bind only the home: after the workspace
	// migration their ~/projects/<dir> would be an EMPTY dir inside the home
	// mount — the project would look wiped. Recreate them once with the
	// workspace bind (rootfs apt state is lost; the manifest replay, ADR-028,
	// wasn't captured by the old entrypoint — accepted one-time cost).
	// STOP first (grace period): a force-remove would skip the entrypoint's
	// SIGTERM trap and tear down the loop-mounted ext4 dirty (review finding).
	if err == nil && !hasWorkspaceBind(insp.HostConfig, o.cfg.ProjectWorkspace(user, project)) {
		o.logger.Info("recreating pre-ADR-027 container to add the workspace bind", "project", project)
		stopTimeout := 30
		if serr := o.docker.ContainerStop(ctx, insp.ID, container.StopOptions{Timeout: &stopTimeout}); serr != nil {
			o.logger.Warn("pre-recreate stop failed — removing anyway", "project", project, "error", serr)
		}
		if rerr := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); rerr != nil {
			return "", fmt.Errorf("recreate for workspace bind: %w", rerr)
		}
		err = errdefs.NotFound(fmt.Errorf("recreated"))
	}
	// Image upgrade (v1.5 review finding): a STOPPED container whose rootfs
	// was built from an OLDER build of cfg.Image keeps that old rootfs across
	// stop/start forever — new profile.d snippets, tools, and entrypoint fixes
	// silently never reach it. Recreate it here so the next boot runs the
	// current image. Only stopped containers: recreating a RUNNING one would
	// kill live terminal sessions (it upgrades on its next stop/start cycle
	// instead). Safe: home + workspace are mounts, and the apt manifest
	// (ADR-028) replays what the user installed.
	if err == nil && (insp.State == nil || !insp.State.Running) {
		if cur, ierr := o.docker.ImageInspect(ctx, o.cfg.Image); ierr == nil && cur.ID != insp.Image {
			o.logger.Info("recreating stopped container for image upgrade",
				"project", project,
				"old", insp.Image[:min(19, len(insp.Image))],
				"new", cur.ID[:min(19, len(cur.ID))])
			if rerr := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); rerr != nil {
				return "", fmt.Errorf("recreate for image upgrade: %w", rerr)
			}
			err = errdefs.NotFound(fmt.Errorf("recreated"))
		}
	}
	// Network mode (ADR-067): the mode IS the bridge interface prefix and it
	// is fixed at `docker network create`, so a container sitting on a bridge
	// whose name disagrees with the project's current mode has to be rebuilt —
	// there is no way to rename a live bridge under it. Same shape as the two
	// recreates above, and STOPPED containers only for the same reason:
	// rebuilding a running one would kill live terminal sessions. The toggle
	// handler stops the project first, and a project left in the wrong mode by
	// a failed toggle is healed on its next start, however it got there.
	if err == nil && (insp.State == nil || !insp.State.Running) {
		mismatch, merr := o.bridgeModeMismatch(ctx, config.NetworkName(project), opts.Lan)
		if merr != nil {
			return "", fmt.Errorf("network mode: cannot inspect the project network, refusing to start on an unverified bridge: %w", merr)
		}
		if mismatch {
			o.logger.Info("recreating stopped container for a network mode change",
				"project", project, "lan", opts.Lan)
			if rerr := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); rerr != nil {
				return "", fmt.Errorf("recreate for network mode: %w", rerr)
			}
			err = errdefs.NotFound(fmt.Errorf("recreated"))
		}
	}
	// Every path below hands the project home to Docker as a bind SOURCE:
	// create passes it in Binds, and START re-resolves the stored, unresolved
	// source on every start (verified on Engine 29.7.2) — so a symlink the
	// guest plants at p/<uuid> from the setup VM would mount a host path as
	// the home on the next Stop/Start, no race needed (security review
	// 2026-09-03 round 3, BUG-003). Refuse a symlink component on every
	// path; a home that does not exist yet is fine (create makes it and
	// checks again).
	if err := projectHomeIsReal(o.cfg.TenantDir(user), o.cfg.ProjectHome(user, project)); err != nil {
		return "", fmt.Errorf("project home: %w", err)
	}
	switch {
	case err == nil:
		id = insp.ID
		if insp.State == nil || !insp.State.Running {
			o.logger.Info("starting stopped project container", "project", project)
			if err := o.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
				// Defense in depth: if the project's network went missing
				// (e.g. an older buggy sweep removed it), recreate it and
				// reconnect the container rather than leaving it unstartable.
				if o.recreateMissingNetwork(ctx, id, config.NetworkName(project), opts.Lan) {
					if err2 := o.docker.ContainerStart(ctx, id, container.StartOptions{}); err2 != nil {
						return "", fmt.Errorf("start %s after network recreate: %w", name, err2)
					}
				} else {
					return "", fmt.Errorf("start %s: %w", name, err)
				}
			}
		}
	case errdefs.IsNotFound(err):
		// The workspace dir must exist BEFORE the bind: Docker would
		// auto-create a missing source as root, and every guest write to the
		// workspace would then fail (same failure class as the home mount).
		ws := o.cfg.ProjectWorkspace(user, project)
		if _, serr := os.Stat(ws); serr != nil {
			// Missing new location + real tree still in the home = failed
			// ADR-027 migration. Creating an empty dir here would shadow the
			// real workspace permanently — refuse and let a boot retry it.
			if o.cfg.LegacyWorkspaceExists(user, opts.Dir) {
				return "", fmt.Errorf("workspace migration incomplete for %s — restart the server to retry", opts.Dir)
			}
		}
		if err := os.MkdirAll(ws, 0o700); err != nil {
			return "", fmt.Errorf("create workspace dir: %w", err)
		}
		// Per-project home (ADR-031): a fresh, individual home the entrypoint
		// seeds from the image (dotfiles + harness). Created before the mount
		// so Docker doesn't auto-create it root-owned.
		phome := o.cfg.ProjectHome(user, project)
		// Created through an os.Root at the tenant dir, not by path:
		// os.MkdirAll follows a symlink planted at `p` and would create a
		// directory anywhere on the host (release review 2026-09-04, BUG-003
		// class). Since ADR-064 the tenant dir homes/<user> is server-only —
		// the guest mounts are base/ and p/<uuid> — so this is belt and braces.
		if err := os.MkdirAll(o.cfg.TenantDir(user), 0o700); err != nil {
			return "", fmt.Errorf("create tenant dir: %w", err)
		}
		if err := mkdirAllConfined(o.cfg.TenantDir(user), filepath.Join("p", project), 0o700); err != nil {
			return "", fmt.Errorf("create project home: %w", err)
		}
		// Re-check now that the home exists: MkdirAll returns nil through a
		// symlink-to-dir (the check above allowed a missing home). The
		// window between this check and the create/start was BUG-003's
		// residual; ADR-064 closed it by keeping the tenant dir server-only.
		if err := realBelow(o.cfg.TenantDir(user), phome); err != nil {
			return "", fmt.Errorf("project home: %w", err)
		}
		// Same rule for the workspace's mount TARGET inside the home bind
		// (~/projects/<dir> → host-side <phome>/projects/<dir>): dockerd
		// creates a missing target as ROOT 0755, leaving a root-owned dir in
		// the per-project home that a NON-ROOT server can never RemoveAll —
		// a wipe-delete then strands the record with "wipe incomplete"
		// forever. Pre-create it as ourselves (0755 like dockerd would, for
		// in-guest traversal); a root server keeps today's exact layout.
		// Through an os.Root at the (verified real) project home: ~/projects
		// is guest-writable and a link there would turn this into a
		// root-owned mkdir on the host (release review 2026-09-04).
		if err := mkdirAllConfined(phome, filepath.Join("projects", opts.Dir), 0o755); err != nil {
			return "", fmt.Errorf("create workspace mountpoint: %w", err)
		}
		id, err = o.create(ctx, createSpec{
			name:          name,
			netName:       config.NetworkName(project),
			lan:           opts.Lan,
			hostname:      guestHostname(opts.Name),
			guestUser:     opts.GuestUser,
			homePath:      phome,
			workspacePath: ws,
			workspaceDst:  "/home/" + opts.GuestUser + "/projects/" + opts.Dir,
			extraEnv: []string{
				config.EnvProject + "=" + project,
				config.EnvProjectDir + "=" + opts.Dir,
				config.EnvDockerDisk + "=" + strconv.Itoa(o.cfg.DockerDiskGB()),
			},
			labels: map[string]string{
				config.LabelManaged: "true",
				config.LabelProject: project,
				config.LabelName:    opts.Name,
				config.LabelUser:    user,
			},
		})
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}

	if err := o.waitReady(ctx, id, 90*time.Second); err != nil {
		return "", fmt.Errorf("project %s did not become ready: %w", name, err)
	}
	// NOTE: no EnsureSelfAttached here — attaching the server's own
	// container to a network reconfigures its netns and can kill in-flight
	// HTTP responses (observed as browser ERR_CONNECTION_REFUSED on the
	// published dev port). The port-preview proxy attaches lazily instead.
	o.Touch(user, project)
	// Any successful EnsureRunning is an intentional boot — lift the
	// deliberately-stopped marker so terminals reconnect normally again.
	o.stopped.Delete(key(user, project))
	return id, nil
}

// guestHostname is the hostname a project's microVM reports — it is what the
// shell prompt (\h) and the terminal status bar show, so it is the project's
// OWN name and nothing else. It used to be prefixed "styrman-", which read as
// "styrman-styrman-harness" for a project named after its repo: the platform's
// name is not information the operator needs on every prompt line (operator
// directive 2026-09-02).
//
// ValidateName already constrains the name to a valid DNS label except that it
// permits a trailing dash, which is not a legal hostname; trim it, and keep a
// non-empty fallback so a hostname is never blank.
func guestHostname(name string) string {
	h := strings.Trim(name, "-")
	if h == "" {
		return "project"
	}
	return h
}

// brokerIPForSubnet returns the fixed broker host address for a bridge:
// broadcast-1 (e.g. 172.19.255.254 for 172.19.0.0/16) — high in the subnet,
// clear of Docker's low-end dynamic allocations.
func brokerIPForSubnet(n *net.IPNet) net.IP {
	ip4 := n.IP.To4()
	if ip4 == nil {
		return nil
	}
	bcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		bcast[i] = ip4[i] | ^n.Mask[i]
	}
	v := binary.BigEndian.Uint32(bcast) - 1
	out := make(net.IP, 4)
	binary.BigEndian.PutUint32(out, v)
	return out
}

// bridgeSubnet returns the first IPv4 subnet of a project bridge.
func (o *Orchestrator) bridgeSubnet(ctx context.Context, netName string) *net.IPNet {
	insp, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{})
	if err != nil {
		return nil
	}
	for _, cfg := range insp.IPAM.Config {
		if _, subnet, perr := net.ParseCIDR(cfg.Subnet); perr == nil && subnet.IP.To4() != nil {
			return subnet
		}
	}
	return nil
}

// brokerIPOnBridge is the broker's fixed address on a bridge, or "" when the
// broker/self-container is disabled (host-mode) — callers then skip the env
// and the static attach.
func (o *Orchestrator) brokerIPOnBridge(ctx context.Context, netName string) string {
	if o.cfg.SelfContainer == "" || o.cfg.BrokerAddr == "" {
		return ""
	}
	subnet := o.bridgeSubnet(ctx, netName)
	if subnet == nil {
		return ""
	}
	ip := brokerIPForSubnet(subnet)
	if ip == nil {
		return ""
	}
	return ip.String()
}

// ResolveProjectByIP maps a source IP on a project bridge to (user,
// project) by network position (ADR-030 credential broker): the server owns
// each sdb-<uuid> bridge, so the subnet the IP falls in identifies the
// project; the container's styrman.user label gives the owner. Since all a
// user's projects share the same credentials in P1, this is identity/audit,
// not a security boundary.
func (o *Orchestrator) ResolveProjectByIP(ctx context.Context, ip string) (user, project string, err error) {
	addr := net.ParseIP(ip)
	if addr == nil {
		return "", "", fmt.Errorf("bad ip %q", ip)
	}
	// Short-TTL cache: a project can call the broker per git operation, and a
	// full resolution is a NetworkList + N×Inspect — cache to bound Docker
	// load. A cache HIT is re-verified with ONE inspect (not the full scan):
	// the cached container must still exist, still be owned by the cached
	// user, and still hold this source IP. Without that re-check a freed
	// subnet/IP reused by a DIFFERENT user's project within the TTL would be
	// served the first user's credentials — a cross-tenant leak (review
	// finding). Verify-on-hit means the mapping can never outlive the
	// container that earned it, so no destroy-path invalidation is needed.
	if u, p, ok := o.ipCacheGet(ip); ok {
		cinsp, cerr := o.docker.ContainerInspect(ctx, config.ContainerName(p))
		if cerr == nil && cinsp.Config.Labels[config.LabelUser] == u && containerHoldsIP(cinsp, ip) {
			return u, p, nil
		}
		o.ipCacheDelete(ip) // stale (container gone, re-owned, or IP reused) — rescan
	}
	nets, err := o.docker.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", "sdb-")),
	})
	if err != nil {
		return "", "", err
	}
	for _, n := range nets {
		// PROJECT bridges only: the "sdb-" name filter is a substring match
		// and also catches sdb-sys-setup-<user>; skip those.
		if !strings.HasPrefix(n.Name, "sdb-") || strings.HasPrefix(n.Name, "sdb-sys-") {
			continue
		}
		insp, err := o.docker.NetworkInspect(ctx, n.ID, network.InspectOptions{})
		if err != nil {
			continue
		}
		for _, cfg := range insp.IPAM.Config {
			_, subnet, perr := net.ParseCIDR(cfg.Subnet)
			if perr != nil || !subnet.Contains(addr) {
				continue
			}
			projectID := strings.TrimPrefix(n.Name, "sdb-")
			cinsp, cerr := o.docker.ContainerInspect(ctx, config.ContainerName(projectID))
			if cerr != nil {
				return "", "", fmt.Errorf("project %s network found but container gone", projectID)
			}
			owner := cinsp.Config.Labels[config.LabelUser]
			if owner == "" {
				return "", "", fmt.Errorf("project %s has no user label", projectID)
			}
			// The caller must actually be THIS project's container, not just
			// something in its subnet — only cache/serve an IP the container
			// really holds, so the cache can never memoize a wrong mapping.
			if !containerHoldsIP(cinsp, ip) {
				return "", "", fmt.Errorf("ip %s in project %s subnet but not held by its container", ip, projectID)
			}
			o.ipCachePut(ip, owner, projectID)
			return owner, projectID, nil
		}
	}
	return "", "", fmt.Errorf("no project bridge owns %s", ip)
}

// hasWorkspaceBind reports whether the container already mounts the
// project's workspace dir (ADR-027).
func hasWorkspaceBind(hc *container.HostConfig, workspace string) bool {
	if hc == nil {
		return false
	}
	for _, b := range hc.Binds {
		if strings.HasPrefix(b, workspace+":") {
			return true
		}
	}
	return false
}

// EnsureSelfAttached joins the server's own container to the project's
// bridge network (no-op for a host-mode server). Docker isolates bridge
// networks from each other, so without this a containerized server cannot
// reach microVM IPs for the port-preview proxy. Returns true when a fresh
// attach happened — which reconfigures this container's netns and may kill
// in-flight connections, so callers on a request path should redirect the
// client to retry rather than proceed. Takes no ctx BY DESIGN: the attach
// must complete even if the triggering client gives up (see the detached
// context below), so a caller-supplied context would only ever be ignored.
func (o *Orchestrator) EnsureSelfAttached(user, project string) bool {
	if o.cfg.SelfContainer == "" {
		return false
	}
	// TTL cache (v1.14.0 polish): the preview proxy calls this per REQUEST,
	// and each miss costs a NetworkInspect + NetworkConnect round-trip that
	// almost always answers "already attached". A 60s memo keeps steady-state
	// previews Docker-API-free; Destroy invalidates so a recreated network
	// re-attaches promptly.
	if v, ok := o.selfAttached.Load(project); ok && time.Since(v.(time.Time)) < time.Minute {
		return false
	}
	// Detached context: this must complete even if the triggering client
	// gives up (a canceled attach left the network unreachable for the
	// next request too — same failure class as the delete-handler bug).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	netName := config.NetworkName(project)
	// Bind the FIXED broker IP (subnet broadcast-1) on this bridge so the
	// broker answers at the same URL from every project (ADR-030). Falls
	// back to a dynamic IP if the static one is somehow taken — the
	// port-preview proxy works either way; only the broker needs the fixed
	// address, and a project created after this attach gets the matching URL
	// in its env.
	var endpoint *network.EndpointSettings
	if ip := o.brokerIPOnBridge(ctx, netName); ip != "" {
		endpoint = &network.EndpointSettings{
			IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: ip},
		}
	}
	err := o.docker.NetworkConnect(ctx, netName, o.cfg.SelfContainer, endpoint)
	if err != nil && endpoint != nil && !strings.Contains(err.Error(), "already exists") {
		// Static IP rejected (e.g. in use) — retry dynamic so port-preview
		// still works; the broker URL for existing projects may then miss,
		// but new projects re-derive on create.
		o.logger.Warn("broker static IP attach failed — retrying dynamic", "project", project, "error", err)
		err = o.docker.NetworkConnect(ctx, netName, o.cfg.SelfContainer, nil)
	}
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			// GENUINE failure: do NOT memoize — a stored memo made every call
			// for the next 60s report "attached" while the proxy dialed
			// through a bridge we never joined (2026-08-30 review). The next
			// request retries the attach, exactly like the pre-memo code.
			o.logger.Warn("self-attach to project network failed", "project", project, "error", err)
			return false
		}
		// Already attached — a verified state worth memoizing.
		o.selfAttached.Store(project, time.Now())
		return false
	}
	o.selfAttached.Store(project, time.Now())
	// Fresh attachment: give the new interface/route a moment to settle.
	time.Sleep(500 * time.Millisecond)
	return true
}

// EnsureSetup boots the hidden system microVM used for first-time onboarding
// (styrman-setup). It carries no project label, so it never appears in
// project listings; minimal mode skips workspace + DinD for a fast boot.
func (o *Orchestrator) EnsureSetup(ctx context.Context, user, guestUser string) (string, error) {
	l := o.lockProject(config.SetupContainer(user))
	l.Lock()
	defer l.Unlock()

	// base/ becomes a bind SOURCE on every path below (Docker re-resolves a
	// stored source on each start): refuse a symlink component now; a base
	// that does not exist yet is fine (the create path makes it and checks
	// again). Security review 2026-09-04, same rule as projectHomeIsReal.
	if err := projectHomeIsReal(o.cfg.TenantDir(user), o.cfg.UserHome(user)); err != nil {
		return "", fmt.Errorf("setup home: %w", err)
	}
	insp, err := o.docker.ContainerInspect(ctx, config.SetupContainer(user))
	if err != nil && !errdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect setup container: %w", err)
	}
	id := ""
	exists := err == nil
	// If a setup VM exists but was built for a different guest username (the
	// user changed it during first-run), remove it so it is recreated with
	// the chosen name.
	if exists && containerGuest(insp.Config.Env) != guestUser {
		o.logger.Info("setup VM guest changed — recreating", "user", user,
			"from", containerGuest(insp.Config.Env), "to", guestUser)
		if err := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			// Don't fall through to create — it would fail "name already in
			// use" and surface a misleading 500 with no self-heal.
			return "", fmt.Errorf("remove stale setup container: %w", err)
		}
		exists = false
	}
	// A setup VM created before ADR-064 binds the tenant dir homes/<user>
	// (with every project home inside it) instead of homes/<user>/base:
	// recreate it so the layout change takes effect without an operator step.
	if exists && setupHomeStale(insp.Mounts, guestUser, o.cfg.UserHome(user)) {
		o.logger.Info("setup VM home moved — recreating", "user", user, "want", o.cfg.UserHome(user))
		if err := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("remove stale setup container: %w", err)
		}
		exists = false
	}
	if exists {
		id = insp.ID
		if insp.State == nil || !insp.State.Running {
			if err := o.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
				return "", fmt.Errorf("start setup container: %w", err)
			}
		}
	} else {
		// The setup VM's home is homes/<user>/base (ADR-064): created before
		// the mount so Docker doesn't make it root-owned, through an os.Root
		// at the server-only tenant dir, and required to be a real directory
		// before it becomes a bind source (a planted symlink is refused).
		if err := os.MkdirAll(o.cfg.TenantDir(user), 0o700); err != nil {
			return "", fmt.Errorf("create tenant dir: %w", err)
		}
		if err := mkdirAllConfined(o.cfg.TenantDir(user), "base", 0o700); err != nil {
			return "", fmt.Errorf("create setup home: %w", err)
		}
		if err := realBelow(o.cfg.TenantDir(user), o.cfg.UserHome(user)); err != nil {
			return "", fmt.Errorf("setup home: %w", err)
		}
		id, err = o.create(ctx, createSpec{
			name:      config.SetupContainer(user),
			netName:   config.SetupNetwork(user),
			hostname:  "styrman-setup",
			guestUser: guestUser,
			homePath:  o.cfg.UserHome(user),
			extraEnv: []string{
				config.EnvProject + "=setup",
				"STYRMAN_MINIMAL=1",
			},
			labels: map[string]string{
				config.LabelManaged: "true",
				config.LabelSystem:  "setup",
				config.LabelUser:    user,
			},
			noRestart: true, // ephemeral onboarding VM — reap, don't resurrect
		})
		if err != nil {
			return "", err
		}
	}
	if err := o.waitReady(ctx, id, 90*time.Second); err != nil {
		return "", fmt.Errorf("setup container did not become ready: %w", err)
	}
	return id, nil
}

// containerGuest extracts the in-guest username (STYRMAN_USER) from a
// container's env, or "" if absent.
func containerGuest(env []string) string {
	prefix := config.EnvUser + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

// SetupStatus reports whether the hidden onboarding microVM is running.
func (o *Orchestrator) SetupStatus(ctx context.Context, user string) (bool, error) {
	insp, err := o.docker.ContainerInspect(ctx, config.SetupContainer(user))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return insp.State != nil && insp.State.Running, nil
}

// DestroySetup removes the hidden onboarding microVM and its network.
func (o *Orchestrator) DestroySetup(ctx context.Context, user string) error {
	err := o.docker.ContainerRemove(ctx, config.SetupContainer(user), container.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err := o.docker.NetworkRemove(ctx, config.SetupNetwork(user)); err != nil && !errdefs.IsNotFound(err) {
		o.logger.Warn("setup network remove failed", "error", err)
	}
	return nil
}

// CleanupLegacy removes managed containers from previous naming schemes:
// anything without a user label (pre-multitenancy) or, for project
// containers, without a name label (pre-UUID). Their data lives in the
// homes; containers are disposable and recreate on demand.
func (o *Orchestrator) CleanupLegacy(ctx context.Context) {
	collectKeep := func(cs []container.Summary) (map[string]bool, []container.Summary) {
		keep := map[string]bool{}
		var legacy []container.Summary
		for _, c := range cs {
			hasUser := c.Labels[config.LabelUser] != ""
			hasName := c.Labels[config.LabelName] != ""
			isSystem := c.Labels[config.LabelSystem] != ""
			if hasUser && (hasName || isSystem) {
				if isSystem {
					keep[config.SetupNetwork(c.Labels[config.LabelUser])] = true
				} else if proj := c.Labels[config.LabelProject]; proj != "" {
					keep[config.NetworkName(proj)] = true
				}
				continue
			}
			legacy = append(legacy, c)
		}
		return keep, legacy
	}
	listAll := func() ([]container.Summary, error) {
		return o.docker.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("label", config.LabelManaged+"=true")),
		})
	}
	list, err := listAll()
	if err != nil {
		// Loud, not silent (audit finding): a cleanup that never ran should be
		// visible in the boot log.
		o.logger.Warn("legacy cleanup skipped: container list failed", "error", err)
		return
	}
	// Networks that belong to a CURRENT-scheme container (running OR stopped)
	// must be kept — removing a stopped project's network would make its next
	// EnsureRunning fail "network not found" with no recreate path (R7).
	_, legacy := collectKeep(list)
	for _, c := range legacy {
		o.logger.Info("removing previous-layout container", "id", c.ID[:12], "names", c.Names)
		_ = o.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
	}
	// Re-list containers immediately before the network sweep (audit finding):
	// this runs concurrently with the HTTP listener, and a project created in
	// the window between the first snapshot and here has its sdb- network
	// created but not yet attached — the stale keep-set would delete it.
	keepNets := map[string]bool{}
	if fresh, err := listAll(); err == nil {
		keepNets, _ = collectKeep(fresh)
	} else {
		o.logger.Warn("legacy cleanup: network sweep skipped (container re-list failed)", "error", err)
		return
	}
	// Remove only sdb- networks NOT referenced by a current container.
	nets, err := o.docker.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		o.logger.Warn("legacy cleanup: network list failed", "error", err)
		return
	}
	for _, n := range nets {
		if strings.HasPrefix(n.Name, "sdb-") && !keepNets[n.Name] {
			_ = o.docker.NetworkRemove(ctx, n.ID) // truly orphaned; best effort
		}
	}
}

// createSpec parameterizes container creation for project and system VMs.
type createSpec struct {
	name      string
	netName   string
	hostname  string
	guestUser string // in-guest username; its home is homePath
	homePath  string // host path mounted as /home/<guestUser>
	// workspacePath/workspaceDst: the project's own storage dir (ADR-027),
	// bind-mounted OVER ~/projects/<dir> inside the home mount. Empty for
	// system VMs (setup) which have no workspace.
	workspacePath string
	workspaceDst  string
	extraEnv      []string
	labels        map[string]string
	noRestart     bool // ephemeral (setup VM): don't auto-restart after reboot
	lan           bool // ADR-067 network mode; always false for system VMs
}

// createBridgeNetwork is the ONLY place a styrman bridge network is created.
// Both call sites (create, recreateMissingNetwork) go through it so a future
// third one cannot forget the mode and silently build a project an unnamed —
// and therefore unconfined — bridge (ADR-067).
func (o *Orchestrator) createBridgeNetwork(ctx context.Context, netName string, lan bool) error {
	_, err := o.docker.NetworkCreate(ctx, netName, network.CreateOptions{
		Driver:  "bridge",
		Options: map[string]string{"com.docker.network.bridge.name": config.BridgeNameFor(netName, lan)},
	})
	return err
}

// bridgeModeMismatch reports whether the project's existing network carries a
// bridge interface name that disagrees with the wanted mode (ADR-067) — i.e.
// whether a mode change is pending and the container must be rebuilt to apply
// it.
//
// A true return force-removes a container, so it never guesses true — a
// Docker hiccup can never destroy a project's container. A missing network is
// NOT a mismatch (recreateMissingNetwork owns that case and builds the bridge
// under the right name, so the mode is applied on that start anyway). Any
// OTHER inspect error is RETURNED, and the caller REFUSES to start rather
// than guess: it once returned false there (BUG-031), which on a revocation
// restarted the just-stopped container on its old lan- bridge with the record
// already saying sandbox. Refusing leaves the container down — the safe state
// — and the next start retries the check.
func (o *Orchestrator) bridgeModeMismatch(ctx context.Context, netName string, lan bool) (bool, error) {
	insp, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			// No network at all: recreateMissingNetwork owns that case and
			// builds the bridge under the right name anyway. Not a mismatch.
			return false, nil
		}
		// Any OTHER inspect failure is "I cannot tell", and that must not
		// read as "no mismatch": on a revocation the caller has just stopped
		// the container, and starting it on an unverified bridge would put
		// it back on lan- with the record already saying sandbox — a
		// fail-open nothing would correct (security review, 2026-09-09).
		// Surface the error; the container stays down, which is the safe
		// state, and the next start retries the check.
		return false, err
	}
	cur := insp.Options["com.docker.network.bridge.name"]
	if cur == "" {
		// A network Docker auto-named (br-<hash>). No styrman code path
		// creates one — create() and recreateMissingNetwork both set the
		// option — so this means manual intervention. Rebuild it: an unnamed
		// bridge matches no firewall rule, so the project is silently
		// unconfined until it is. Loud, because it costs a container rebuild.
		o.logger.Warn("project network has no styrman bridge name — it matches no firewall rule; rebuilding",
			"network", netName)
		return true, nil
	}
	return cur != config.BridgeNameFor(netName, lan), nil
}

// create builds a network and container. Caller holds the relevant lock.
func (o *Orchestrator) create(ctx context.Context, spec createSpec) (string, error) {
	netName := spec.netName
	wantBridge := config.BridgeNameFor(netName, spec.lan)
	createdNetwork := false
	insp, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{})
	if err == nil && insp.Options["com.docker.network.bridge.name"] != wantBridge {
		// ADR-067 mode change: the bridge name is immutable, so the network is
		// rebuilt. By now the project's own container is gone (EnsureRunning's
		// mode branch removed it, or it never existed), so the only endpoint
		// left is our own port-preview/broker attach — NetworkRemove refuses
		// while ANY endpoint remains, running or stopped. Same two calls, and
		// the same SelfContainer guard, that Destroy uses.
		o.logger.Info("rebuilding project network for a mode change",
			"network", netName, "from", insp.Options["com.docker.network.bridge.name"], "to", wantBridge)
		if o.cfg.SelfContainer != "" {
			_ = o.docker.NetworkDisconnect(ctx, netName, o.cfg.SelfContainer, true)
		}
		if rerr := o.docker.NetworkRemove(ctx, netName); rerr != nil && !errdefs.IsNotFound(rerr) {
			return "", fmt.Errorf("rebuild network %s for mode change: %w", netName, rerr)
		}
		o.invalidateSelfAttach(netName)
		err = errdefs.NotFound(fmt.Errorf("rebuilt for mode change"))
	}
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("inspect network %s: %w", netName, err)
		}
		if err := o.createBridgeNetwork(ctx, netName, spec.lan); err != nil {
			return "", fmt.Errorf("create network %s: %w", netName, err)
		}
		createdNetwork = true
		// A FRESH bridge means any self-attach memo for this project points
		// at a network that no longer exists — invalidate so the preview
		// proxy re-attaches promptly (2026-08-30 review).
		o.invalidateSelfAttach(netName)
	}
	// On a failed create, tear down the network ONLY if we created it here — an
	// existing project bridge (e.g. a container re-create on a running project)
	// must survive — and surface the cleanup error instead of dropping it.
	removeOurNetwork := func() {
		if !createdNetwork {
			return
		}
		if err := o.docker.NetworkRemove(context.Background(), netName); err != nil && !errdefs.IsNotFound(err) {
			o.logger.Warn("cleanup: removing project network failed", "network", netName, "error", err)
		}
	}

	name := spec.name
	o.logger.Info("creating microVM", "container", name, "image", o.cfg.Image, "runtime", o.cfg.Runtime)

	env := []string{
		config.EnvUser + "=" + spec.guestUser,
		config.EnvUID + "=" + strconv.Itoa(o.cfg.UID),
		config.EnvGID + "=" + strconv.Itoa(o.cfg.GID),
	}
	// Credential broker address (ADR-030): a FIXED per-bridge IP (subnet
	// broadcast-1) the server also binds on attach — identical URL from every
	// project, so the git helper needs no per-project hint file. Empty when
	// the broker or the server-container is disabled (host-mode dev).
	if brokerIP := o.brokerIPOnBridge(ctx, netName); brokerIP != "" {
		env = append(env, config.EnvCredBroker+"=http://"+brokerIP+o.cfg.BrokerAddr)
	}
	env = append(env, spec.extraEnv...)

	resp, err := o.docker.ContainerCreate(ctx,
		&container.Config{
			Image:    o.cfg.Image,
			Hostname: spec.hostname,
			Env:      env,
			Labels:   spec.labels,
			// Tty stays false (the zero value) ON PURPOSE: Logs() relies on the
			// daemon-multiplexed stream, where StdCopy's frame headers are
			// daemon-controlled. With Tty:true StdCopy would parse GUEST bytes
			// as frame headers (up to 4 GiB allocation per frame) and the
			// tailBuffer byte-cap would not save the server (review 2026-09-01).
		},
		&container.HostConfig{
			Runtime: o.cfg.Runtime,
			// For Docker-in-Docker inside the Kata VM. --privileged breaks
			// Kata device hotplug, hence explicit capabilities (devbox ADR).
			CapAdd: []string{"SYS_ADMIN", "NET_ADMIN", "NET_RAW"},
			// Kata VMs don't reach Docker's embedded DNS (127.0.0.11).
			DNS:         []string{"8.8.8.8", "1.1.1.1"},
			Sysctls:     map[string]string{"net.ipv4.ip_forward": "1"},
			SecurityOpt: []string{"apparmor=unconfined"},
			Binds: func() []string {
				b := []string{spec.homePath + ":/home/" + spec.guestUser}
				if spec.workspacePath != "" {
					b = append(b, spec.workspacePath+":"+spec.workspaceDst)
				}
				return b
			}(),
			RestartPolicy: restartPolicy(spec.noRestart),
			// Per-project sizing: 0 leaves Kata's own default (1 vCPU / 2 GB);
			// the operator can cap/raise it via STYRMAN_VCPUS / STYRMAN_MEMORY_MB
			// so project VMs aren't silently stuck on the runtime default (low
			// finding). Kata reads these container limits to size the guest.
			Resources: container.Resources{
				NanoCPUs: int64(o.cfg.VCPUs) * 1_000_000_000,
				Memory:   int64(o.cfg.MemoryMB) * 1024 * 1024,
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{netName: {}},
		},
		nil,
		name,
	)
	if err != nil {
		removeOurNetwork()
		return "", fmt.Errorf("create container %s: %w", name, err)
	}
	if err := o.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = o.docker.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
		removeOurNetwork()
		return "", fmt.Errorf("start container %s: %w", name, err)
	}
	return resp.ID, nil
}

// waitReady polls for the entrypoint's ready marker.
func (o *Orchestrator) waitReady(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		code, _, err := o.Exec(ctx, containerID, ExecSpec{Cmd: []string{"test", "-f", "/run/styrman/ready"}})
		if err == nil && code == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timeout after %s", timeout)
}

// Stop stops a project's container (home and workspace persist). Holds the
// per-project lock so a stop cannot interleave with EnsureRunning's
// create/start/waitReady (audit finding: an HTTP stop racing a terminal
// attach could power the VM off mid-boot and leave the API reporting
// "running" for a stopped VM).
func (o *Orchestrator) Stop(ctx context.Context, user, project string) error {
	l := o.lockProject(key(user, project))
	l.Lock()
	defer l.Unlock()
	// Mark BEFORE stopping (under the lock): the container stop kills every
	// attached terminal exec, and their auto-reconnects must already see the
	// marker — otherwise the first one boots the VM straight back up.
	o.stopped.Store(key(user, project), true)
	if err := o.stopLocked(ctx, project); err != nil {
		// If the container is still running, "deliberately stopped" is
		// false: leaving the marker would have every terminal auto-reconnect
		// refused with close 4002 for a project running exactly as before,
		// after an error that says nothing changed (BUG-030). Marking first
		// is still right (see above); it is undone only when the stop
		// provably did not happen — see unmarkIfStillRunning for why not
		// unconditionally.
		o.unmarkIfStillRunning(ctx, user, project)
		return err
	}
	return nil
}

// unmarkIfStillRunning clears the "deliberately stopped" marker after a stop
// FAILED — but only when the container is provably still running. A stop can
// half-succeed (the daemon took the request, the transport returned an error)
// or race a reaper tick against a user's Stop; in those cases the container
// IS down, and clearing the marker would let the next terminal auto-reconnect
// boot it straight back (the exact resurrection the marker exists to stop).
// An inspect that fails or shows anything but Running keeps the marker: the
// user can always press Start, whereas a resurrected VM is a surprise.
func (o *Orchestrator) unmarkIfStillRunning(_ context.Context, user, project string) {
	// A FRESH budget, not the caller's ctx: the likeliest reason the stop
	// failed is that ctx expired (2 min from stopProject, 3 from the mode
	// toggle), and an inspect on a dead ctx fails at once — which would keep
	// the marker stuck in exactly BUG-030's own scenario, a wedged stop on a
	// container that is still up. Five seconds is ample for one inspect.
	ictx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	insp, err := o.docker.ContainerInspect(ictx, config.ContainerName(project))
	if err == nil && insp.State != nil && insp.State.Running {
		o.stopped.Delete(key(user, project))
	}
}

// Stopped reports whether the project was deliberately stopped (Stop button
// or idle reaper) and not intentionally booted since. The terminal bridge
// consults this so automatic WS reconnects don't resurrect a stopped VM.
func (o *Orchestrator) Stopped(user, project string) bool {
	_, ok := o.stopped.Load(key(user, project))
	return ok
}

// stopLocked is Stop's body; the caller must hold the per-project lock
// (stopIfIdle already does — the lock is not reentrant).
func (o *Orchestrator) stopLocked(ctx context.Context, project string) error {
	name := config.ContainerName(project)
	// Kata's `docker stop` powers the microVM off without delivering SIGTERM
	// to the in-guest init, so the entrypoint's graceful-shutdown trap (clean
	// dockerd stop + ext4 unmount) never runs — leaving a dirty journal (I9).
	// Trigger it ourselves by signalling PID 1 inside the guest, give it a
	// moment, then stop. Best effort: a failed exec (already stopping) just
	// falls through to ContainerStop.
	if code, _, err := o.Exec(ctx, name, ExecSpec{Cmd: []string{"kill", "-TERM", "1"}}); err == nil && code == 0 {
		o.waitStopped(ctx, name, 12*time.Second)
	}
	timeout := 30
	err := o.docker.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

// waitStopped polls until the container is no longer running, up to max.
func (o *Orchestrator) waitStopped(ctx context.Context, name string, max time.Duration) {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		insp, err := o.docker.ContainerInspect(ctx, name)
		if err != nil || insp.State == nil || !insp.State.Running {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// stopIfIdle stops the project only if it is STILL idle under the per-project
// lock — the idle reaper's decision is based on a snapshot, and a user may
// have attached (EnsureRunning Touches under the same lock) between the
// snapshot and now. Without this, the reaper can kill a terminal that was
// just opened, losing its tmux state (review finding I2). idle is the tick's
// timeout value — re-reading it here could see a concurrent change to 0
// ("never"), which as a comparison bound would mean "stop everything".
func (o *Orchestrator) stopIfIdle(ctx context.Context, user, project string, idle time.Duration) error {
	l := o.lockProject(key(user, project))
	l.Lock()
	defer l.Unlock()
	if idle <= 0 {
		return nil // timeout flipped to "never" mid-tick
	}
	if v, ok := o.activity.Load(key(user, project)); ok {
		if time.Since(v.(time.Time)) <= idle {
			return nil // touched since the reap decision — leave it running
		}
	}
	// The activity map only sees ATTACHED terminals (keystrokes + output).
	// A detached long build, a `docker run -d`, or a closed tab with claude
	// still working would all look idle — probe the guest for real work
	// before pulling the plug: any running DinD container, or any tmux pane
	// whose foreground process isn't a plain shell, blocks the stop.
	if reason := o.projectBusy(ctx, project); reason != "" {
		o.logger.Info("idle-stop skipped — project busy", "project", project, "reason", reason)
		o.Touch(user, project) // full fresh idle window before re-checking
		return nil
	}
	// Same marker as an explicit Stop: an open tab's auto-reconnect used to
	// boot the VM right back after the reaper stopped it.
	o.stopped.Store(key(user, project), true)
	if err := o.stopLocked(ctx, project); err != nil {
		// Same rule as Stop, same guard: only a container still running gets
		// its marker back — a reaper tick can race a user's Stop, and then
		// the project IS down and must stay marked.
		o.unmarkIfStillRunning(ctx, user, project)
		return err
	}
	return nil
}

// invalidateSelfAttach drops the self-attach memo for the project owning a
// bridge; the prefix inversion lives in config (single source of truth,
// 2026-08-30 audit). No-op for system-VM bridges (never memoized).
func (o *Orchestrator) invalidateSelfAttach(netName string) {
	if p := config.ProjectFromNetworkName(netName); p != "" {
		o.selfAttached.Delete(p)
	}
}

// rootOpAllowed confines the root-assisted file ops to server state: StateDir
// plus HomesDir, which is independently configurable (STYRMAN_HOMES_DIR) and
// need not sit under StateDir (audit: the StateDir-only guard fail-closed
// refused wipes on such deployments).
func (o *Orchestrator) rootOpAllowed(p string) bool {
	clean := filepath.Clean(p)
	for _, root := range []string{o.cfg.StateDir, o.cfg.HomesDir} {
		if root != "" && strings.HasPrefix(clean, filepath.Clean(root)+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// RootRemoveContents empties dir using root inside the server's OWN container
// (docker exec -u 0 on SelfContainer, which mounts the state dir). The server
// runs unprivileged (uid 1000), but pre-v1.11.1 homes carry ROOT-owned
// bind-mount stubs dockerd created (e.g. <phome>/projects/<dir>) that an
// unprivileged RemoveAll can never delete — and the server legitimately holds
// the docker socket, so this is not an escalation it doesn't already have.
// Guard: only paths under StateDir, and only when containerized.
func (o *Orchestrator) RootRemoveContents(ctx context.Context, dir string) error {
	if o.cfg.SelfContainer == "" {
		return fmt.Errorf("root remove unavailable outside the containerized server")
	}
	clean := filepath.Clean(dir)
	if !o.rootOpAllowed(clean) {
		return fmt.Errorf("root remove refused outside the state/homes dirs: %s", dir)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	code, out, err := o.Exec(rctx, o.cfg.SelfContainer, ExecSpec{
		User: "0",
		Cmd:  []string{"find", clean, "-mindepth", "1", "-delete"},
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("root remove exited %d: %s", code, strings.TrimSpace(out))
	}
	return nil
}

// busyProbeScript runs in the guest as root: prints "docker" if any DinD
// container is running, "term:<cmd>" for each tmux pane whose foreground
// process isn't a plain shell. Empty output = genuinely idle.
const busyProbeScript = `docker ps -q 2>/dev/null | grep -q . && echo docker
for s in /tmp/tmux-*/default; do
  [ -S "$s" ] && tmux -S "$s" list-panes -a -F '#{pane_current_command}' 2>/dev/null
done | grep -vxE 'bash|sh|-bash|-sh' | sed 's/^/term:/' | sort -u`

// projectBusy reports why the project's guest is still doing real work
// ("" = idle). Best-effort: an exec failure (container already gone or
// wedged) reads as idle so the reaper still converges.
func (o *Orchestrator) projectBusy(ctx context.Context, project string) string {
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	code, out, err := o.Exec(pctx, config.ContainerName(project),
		ExecSpec{Cmd: []string{"sh", "-c", busyProbeScript}})
	if err != nil || code != 0 {
		return ""
	}
	return busyReason(out)
}

// busyReason condenses the probe output to a single log-friendly reason.
func busyReason(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, ",")
}

// Destroy removes the project's container and network. The workspace and all
// home-dir state persist; only runtime state is lost. Holds the per-project
// lock: without it, a terminal WS reconnect's EnsureRunning could recreate
// the container/network mid-removal and leave an orphan behind.
func (o *Orchestrator) Destroy(ctx context.Context, user, project string) error {
	l := o.lockProject(key(user, project))
	l.Lock()
	defer l.Unlock()
	err := o.docker.ContainerRemove(ctx, config.ContainerName(project), container.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	// Detach ourselves first or the network removal fails on the active
	// endpoint (containerized server only).
	if o.cfg.SelfContainer != "" {
		_ = o.docker.NetworkDisconnect(ctx, config.NetworkName(project), o.cfg.SelfContainer, true)
	}
	if err := o.docker.NetworkRemove(ctx, config.NetworkName(project)); err != nil && !errdefs.IsNotFound(err) {
		o.logger.Warn("network remove failed", "project", project, "error", err)
	}
	o.activity.Delete(key(user, project))
	o.stopped.Delete(key(user, project))
	o.selfAttached.Delete(project) // a recreated network must re-attach
	return nil
}

// Ping cheaply checks that the Docker daemon is reachable — used by the health
// probe, which is unauthenticated, so it must stay lightweight (a full Adopt
// list there let a shared-bridge VM drive real daemon load).
func (o *Orchestrator) Ping(ctx context.Context) error {
	_, err := o.docker.Ping(ctx)
	return err
}

// Adopt lists all styrman-managed containers, reconciling reality after a
// server restart (ADR-007). Returns project name → Status.
func (o *Orchestrator) Adopt(ctx context.Context) (map[string]Status, error) {
	list, err := o.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", config.LabelManaged+"=true")),
	})
	if err != nil {
		return nil, err
	}
	out := map[string]Status{}
	for _, c := range list {
		p := c.Labels[config.LabelProject]
		u := c.Labels[config.LabelUser]
		if p == "" || u == "" {
			continue // system VMs and pre-multitenancy legacy containers
		}
		st := "stopped"
		if c.State == container.StateRunning {
			st = "running"
		}
		out[key(u, p)] = Status{User: u, Project: p, ContainerID: c.ID, State: st}
	}
	return out, nil
}

// RunIdleReaper stops containers whose last terminal activity is older than
// the configured idle timeout. Blocks until ctx is done.
//
// The timeout is operator-tunable at runtime (PATCH /api/settings/runtime),
// so the loop ALWAYS runs on a fixed short tick and re-reads the value once
// per tick: starting disabled must not preclude enabling later, a shortened
// timeout must apply on the next tick (not the old timeout/4 cadence), and —
// critically — a tick must use ONE consistent value throughout. Reading the
// value per comparison meant a flip to 0 ("never") mid-run made every
// `time.Since(last) > 0` true and reaped EVERYTHING (review finding).
func (o *Orchestrator) RunIdleReaper(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idle := o.cfg.IdleTimeoutD()
			if idle <= 0 {
				continue // "never" — but keep ticking in case it changes
			}
			// Reap abandoned onboarding VMs too (no project label, so Adopt
			// skips them) — they otherwise leak a Kata VM per abandoned setup.
			o.ReapSetupVMs(ctx, idle)
			statuses, err := o.Adopt(ctx)
			if err != nil {
				o.logger.Warn("idle reaper list failed", "error", err)
				continue
			}
			for k, st := range statuses {
				if st.State != "running" {
					continue
				}
				last := time.Time{}
				if v, ok := o.activity.Load(k); ok {
					last = v.(time.Time)
				}
				// A running container with no recorded activity was adopted
				// after a server restart — start its idle clock now instead
				// of stopping it on the first tick (review finding).
				if last.IsZero() {
					o.Touch(st.User, st.Project)
					continue
				}
				if time.Since(last) > idle {
					o.logger.Info("stopping idle project", "user", st.User, "project", st.Project, "lastActivity", last)
					if err := o.stopIfIdle(ctx, st.User, st.Project, idle); err != nil {
						o.logger.Warn("idle stop failed", "project", st.Project, "error", err)
					}
				}
			}
		}
	}
}

// recreateMissingNetwork recreates a project's bridge network if it is absent
// and reconnects the container to it. Returns true if it recreated (so the
// caller should retry the start). Best effort — false on any failure.
func (o *Orchestrator) recreateMissingNetwork(ctx context.Context, containerID, netName string, lan bool) bool {
	if _, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{}); err == nil {
		return false // network exists; the start failure was something else
	}
	o.logger.Warn("project network missing — recreating", "network", netName, "lan", lan)
	if err := o.createBridgeNetwork(ctx, netName, lan); err != nil {
		o.logger.Error("network recreate failed", "network", netName, "error", err)
		return false
	}
	if err := o.docker.NetworkConnect(ctx, netName, containerID, nil); err != nil {
		o.logger.Error("network reconnect failed", "network", netName, "error", err)
		return false
	}
	// The recreated bridge is a new network: drop any stale self-attach memo
	// so the server rejoins it for the preview proxy (2026-08-30 review).
	o.invalidateSelfAttach(netName)
	return true
}

// restartPolicy picks the container restart policy. Project VMs restart with
// the host (seamless recovery); the setup VM is ephemeral so it does not
// come back after a reboot and can be reaped when onboarding is abandoned.
func restartPolicy(noRestart bool) container.RestartPolicy {
	if noRestart {
		return container.RestartPolicy{Name: "no"}
	}
	return container.RestartPolicy{Name: "unless-stopped"}
}

// ReapSetupVMs stops+removes onboarding VMs that have been running longer
// than maxAge (abandoned setup — the tab was closed without done/discard).
func (o *Orchestrator) ReapSetupVMs(ctx context.Context, maxAge time.Duration) {
	list, err := o.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", config.LabelSystem+"=setup")),
	})
	if err != nil {
		return
	}
	for _, c := range list {
		insp, err := o.docker.ContainerInspect(ctx, c.ID)
		if err != nil || insp.State == nil || !insp.State.Running {
			continue
		}
		started, perr := time.Parse(time.RFC3339Nano, insp.State.StartedAt)
		if perr != nil || time.Since(started) < maxAge {
			continue
		}
		o.logger.Info("reaping abandoned setup VM", "id", c.ID[:12], "age", time.Since(started))
		_ = o.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
	}
}

// Logs returns the tail of a project container's logs (the microVM console:
// entrypoint boot steps, dockerd, kernel messages) — the ops-panel "what is
// this VM doing" view (v1.14.0). Output is demuxed and capped at 256 KiB.
func (o *Orchestrator) Logs(ctx context.Context, project string, tail int) (string, error) {
	if tail <= 0 || tail > 2000 {
		tail = 300
	}
	rc, err := o.docker.ContainerLogs(ctx, config.ContainerName(project), container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(tail),
	})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	const maxBytes = 256 << 10
	// Docker multiplexes stdout/stderr into one stream; demux and interleave.
	//
	// tailBuffer, not strings.Builder: `Tail` bounds LINES, not bytes, and the
	// guest is root in its own VM and chooses the line length — 2000 lines can
	// be gigabytes, buffered here on the SHARED server (audit 2026-09-01; the
	// same H4/H7 threat exec.go's cappedBuffer exists for). It also cannot be
	// cappedBuffer, which keeps the FIRST n bytes and so drops the newest lines
	// a logs view exists for (2026-08-30 review). This keeps the tail, bounded.
	buf := &tailBuffer{max: maxBytes}
	_, _ = stdcopy.StdCopy(buf, buf, rc)
	// tailBuffer guarantees len <= maxBytes; trimLogTail (unit-tested) handles
	// the partial first line and the banner without any indexing.
	return trimLogTail(buf.String(), buf.Dropped(), buf.StartsAtLineBoundary()), nil
}

// realBelow reports an error when any path component of target below base
// is a symlink: base is server-controlled, everything below it may be
// guest-written. Both must be clean absolute paths with target under base.
func realBelow(base, target string) error {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is not below %s", target, base)
	}
	cur := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink (guest-planted?) — refusing to use it as a mount source", cur)
		}
	}
	return nil
}

// projectHomeIsReal is realBelow for a project home that may not exist yet:
// a missing component is fine (the create path makes it and re-checks), a
// symlink anywhere below the base home is not.
func projectHomeIsReal(base, phome string) error {
	err := realBelow(base, phome)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// mkdirAllConfined creates rel below base through an os.Root, so a symlink
// the guest planted on the way cannot redirect the mkdir outside base.
func mkdirAllConfined(base, rel string, perm os.FileMode) error {
	rt, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer rt.Close()
	return rt.MkdirAll(rel, perm)
}

// setupHomeStale reports whether an existing setup container binds a home
// other than want at /home/<guestUser>. True after ADR-064 moved the setup
// VM's home from homes/<user> to homes/<user>/base, and for a container with
// no recognisable home bind at all — either way it is recreated.
func setupHomeStale(mounts []container.MountPoint, guestUser, want string) bool {
	dst := "/home/" + guestUser
	for _, m := range mounts {
		if m.Destination == dst {
			return filepath.Clean(m.Source) != filepath.Clean(want)
		}
	}
	return true
}

// RemoveStaleSetupVMs force-removes every setup container whose home bind
// is not the current base home (ADR-064 moved it from homes/<user> to
// homes/<user>/base). Run at startup before the base homes are migrated,
// so no guest holds the old mount while the server renames under it; the
// per-user check in EnsureSetup remains for anything created in between.
// A setup VM is ephemeral onboarding state — the broker already holds what
// it harvested — so an interrupted session loses nothing.
func (o *Orchestrator) RemoveStaleSetupVMs(ctx context.Context) (int, error) {
	list, err := o.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", config.LabelSystem+"=setup")),
	})
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, c := range list {
		insp, err := o.docker.ContainerInspect(ctx, c.ID)
		if err != nil || insp.Config == nil {
			continue
		}
		user := insp.Config.Labels[config.LabelUser]
		if user == "" || !setupHomeStale(insp.Mounts, containerGuest(insp.Config.Env), o.cfg.UserHome(user)) {
			continue
		}
		if err := o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			o.logger.Warn("stale setup VM: remove failed", "user", user, "error", err)
			continue
		}
		o.logger.Info("stale setup VM removed (home layout changed, ADR-064)", "user", user)
		removed++
	}
	return removed, nil
}
