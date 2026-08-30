// Package orchestrator manages the lifecycle of per-project Kata microVMs via
// the Docker SDK. The Kata container configuration is ported from devbox's
// shell-launcher orchestrator (see ADR-001): CapAdd instead of --privileged,
// explicit DNS, per-project bridge network, apparmor unconfined.
package orchestrator

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
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

// LastActivity returns the last recorded terminal activity for a project
// (zero time if none) — lets the checkpoint loop skip idle trees.
func (o *Orchestrator) LastActivity(user, project string) time.Time {
	if v, ok := o.activity.Load(key(user, project)); ok {
		return v.(time.Time)
	}
	return time.Time{}
}

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
	switch {
	case err == nil:
		id = insp.ID
		if insp.State == nil || !insp.State.Running {
			o.logger.Info("starting stopped project container", "project", project)
			if err := o.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
				// Defense in depth: if the project's network went missing
				// (e.g. an older buggy sweep removed it), recreate it and
				// reconnect the container rather than leaving it unstartable.
				if o.recreateMissingNetwork(ctx, id, config.NetworkName(project)) {
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
		if err := os.MkdirAll(phome, 0o700); err != nil {
			return "", fmt.Errorf("create project home: %w", err)
		}
		// Same rule for the workspace's mount TARGET inside the home bind
		// (~/projects/<dir> → host-side <phome>/projects/<dir>): dockerd
		// creates a missing target as ROOT 0755, leaving a root-owned dir in
		// the per-project home that a NON-ROOT server can never RemoveAll —
		// a wipe-delete then strands the record with "wipe incomplete"
		// forever. Pre-create it as ourselves (0755 like dockerd would, for
		// in-guest traversal); a root server keeps today's exact layout.
		if err := os.MkdirAll(filepath.Join(phome, "projects", opts.Dir), 0o755); err != nil {
			return "", fmt.Errorf("create workspace mountpoint: %w", err)
		}
		id, err = o.create(ctx, createSpec{
			name:          name,
			netName:       config.NetworkName(project),
			hostname:      "styrman-" + opts.Name,
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
	if exists {
		id = insp.ID
		if insp.State == nil || !insp.State.Running {
			if err := o.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
				return "", fmt.Errorf("start setup container: %w", err)
			}
		}
	} else {
		// The setup VM uses the per-user BASE home (onboarding lives here);
		// create it before the mount so Docker doesn't make it root-owned.
		if err := os.MkdirAll(o.cfg.UserHome(user), 0o700); err != nil {
			return "", fmt.Errorf("create setup home: %w", err)
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
}

// create builds a network and container. Caller holds the relevant lock.
func (o *Orchestrator) create(ctx context.Context, spec createSpec) (string, error) {
	netName := spec.netName
	createdNetwork := false
	if _, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{}); err != nil {
		if !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("inspect network %s: %w", netName, err)
		}
		if _, err := o.docker.NetworkCreate(ctx, netName, network.CreateOptions{Driver: "bridge", Options: map[string]string{"com.docker.network.bridge.name": config.BridgeNameFor(netName)}}); err != nil {
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
	return o.stopLocked(ctx, project)
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
	return o.stopLocked(ctx, project)
}

// WithProjectLock runs fn while holding the per-project lifecycle lock — the
// same lock EnsureRunning/Stop/stopIfIdle/Destroy serialize on. Restore uses
// it so a boot can never virtiofs-mount the home while the tree is being
// quarantined/extracted (audit 2026-08-30: the restoring flag alone was a
// TOCTOU — a boot that had already passed the flag check could interleave).
func (o *Orchestrator) WithProjectLock(user, project string, fn func() error) error {
	l := o.lockProject(key(user, project))
	l.Lock()
	defer l.Unlock()
	return fn()
}

// invalidateSelfAttach drops the self-attach memo for the project owning a
// bridge (netName is config.NetworkName(project), i.e. "sdb-"+project).
func (o *Orchestrator) invalidateSelfAttach(netName string) {
	o.selfAttached.Delete(strings.TrimPrefix(netName, "sdb-"))
}

// rootOpAllowed confines the root-assisted file ops to server state: StateDir
// plus HomesDir, which is independently configurable (STYRMAN_HOMES_DIR) and
// need not sit under StateDir (audit: the StateDir-only guard fail-closed
// refused restores/wipes on such deployments).
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

// RootRename moves from → to as root inside the server's own container.
// Renaming a DIRECTORY into a different parent rewrites its ".." entry, which
// requires write permission ON THE DIRECTORY ITSELF — impossible for the
// unprivileged server when the entry is root-owned (dockerd mount stubs, or
// files the guest created via sudo, which virtiofs maps to host root). Same
// guard as RootRemoveContents: both paths must live under StateDir.
func (o *Orchestrator) RootRename(ctx context.Context, from, to string) error {
	if o.cfg.SelfContainer == "" {
		return fmt.Errorf("root rename unavailable outside the containerized server")
	}
	cf, ct := filepath.Clean(from), filepath.Clean(to)
	if !o.rootOpAllowed(cf) || !o.rootOpAllowed(ct) {
		return fmt.Errorf("root rename refused outside the state/homes dirs")
	}
	rctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	code, out, err := o.Exec(rctx, o.cfg.SelfContainer, ExecSpec{
		User: "0",
		Cmd:  []string{"mv", "-T", cf, ct},
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("root rename exited %d: %s", code, strings.TrimSpace(out))
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
func (o *Orchestrator) recreateMissingNetwork(ctx context.Context, containerID, netName string) bool {
	if _, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{}); err == nil {
		return false // network exists; the start failure was something else
	}
	o.logger.Warn("project network missing — recreating", "network", netName)
	if _, err := o.docker.NetworkCreate(ctx, netName, network.CreateOptions{Driver: "bridge", Options: map[string]string{"com.docker.network.bridge.name": config.BridgeNameFor(netName)}}); err != nil {
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
	var buf strings.Builder
	// Docker multiplexes stdout/stderr into one stream; demux and interleave.
	// The tail option bounds the window to <=2000 lines, so buffering it all
	// and trimming the FRONT is safe — a first-N-bytes cap silently dropped
	// the NEWEST lines, the ones a logs view exists for (2026-08-30 review).
	_, _ = stdcopy.StdCopy(&buf, &buf, rc)
	out := buf.String()
	if max := 256 << 10; len(out) > max {
		out = out[len(out)-max:]
		if i := strings.IndexByte(out, '\n'); i >= 0 {
			out = out[i+1:] // start at a line boundary
		}
		out = "… (older lines truncated) …\n" + out
	}
	return out, nil
}
