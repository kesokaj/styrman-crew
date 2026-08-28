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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"

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
	// Short-TTL cache: a project can call the broker per git operation, and
	// resolution is a NetworkList + N×Inspect — cache to bound Docker load.
	if u, p, ok := o.ipCacheGet(ip); ok {
		return u, p, nil
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
// client to retry rather than proceed.
func (o *Orchestrator) EnsureSelfAttached(_ context.Context, user, project string) bool {
	if o.cfg.SelfContainer == "" {
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
			o.logger.Warn("self-attach to project network failed", "project", project, "error", err)
		}
		return false
	}
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
	list, err := o.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", config.LabelManaged+"=true")),
	})
	if err != nil {
		return
	}
	// Networks that belong to a CURRENT-scheme container (running OR stopped)
	// must be kept — removing a stopped project's network would make its next
	// EnsureRunning fail "network not found" with no recreate path (R7).
	keepNets := map[string]bool{}
	for _, c := range list {
		hasUser := c.Labels[config.LabelUser] != ""
		hasName := c.Labels[config.LabelName] != ""
		isSystem := c.Labels[config.LabelSystem] != ""
		if hasUser && (hasName || isSystem) {
			if isSystem {
				keepNets[config.SetupNetwork(c.Labels[config.LabelUser])] = true
			} else if proj := c.Labels[config.LabelProject]; proj != "" {
				keepNets[config.NetworkName(proj)] = true
			}
			continue
		}
		o.logger.Info("removing previous-layout container", "id", c.ID[:12], "names", c.Names)
		_ = o.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
	}
	// Remove only sdb- networks NOT referenced by a current container.
	nets, err := o.docker.NetworkList(ctx, network.ListOptions{})
	if err != nil {
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
	if _, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{}); err != nil {
		if !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("inspect network %s: %w", netName, err)
		}
		if _, err := o.docker.NetworkCreate(ctx, netName, network.CreateOptions{Driver: "bridge"}); err != nil {
			return "", fmt.Errorf("create network %s: %w", netName, err)
		}
	}

	name := spec.name
	o.logger.Info("creating microVM", "container", name, "image", o.cfg.Image, "runtime", o.cfg.Runtime)

	env := []string{
		config.EnvUser + "=" + spec.guestUser,
		config.EnvUID + "=" + strconv.Itoa(o.cfg.UID),
		config.EnvGID + "=" + strconv.Itoa(o.cfg.GID),
		config.EnvKeyringPW + "=" + o.cfg.KeyringPW,
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
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{netName: {}},
		},
		nil,
		name,
	)
	if err != nil {
		_ = o.docker.NetworkRemove(context.Background(), netName)
		return "", fmt.Errorf("create container %s: %w", name, err)
	}
	if err := o.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = o.docker.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
		_ = o.docker.NetworkRemove(context.Background(), netName)
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

// Stop stops a project's container (home and workspace persist).
func (o *Orchestrator) Stop(ctx context.Context, user, project string) error {
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
	return o.Stop(ctx, user, project)
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
	if _, err := o.docker.NetworkCreate(ctx, netName, network.CreateOptions{Driver: "bridge"}); err != nil {
		o.logger.Error("network recreate failed", "network", netName, "error", err)
		return false
	}
	if err := o.docker.NetworkConnect(ctx, netName, containerID, nil); err != nil {
		o.logger.Error("network reconnect failed", "network", netName, "error", err)
		return false
	}
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
