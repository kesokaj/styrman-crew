// Package orchestrator manages the lifecycle of per-project Kata microVMs via
// the Docker SDK. The Kata container configuration is ported from devbox's
// shell-launcher orchestrator (see ADR-001): CapAdd instead of --privileged,
// explicit DNS, per-project bridge network, apparmor unconfined.
package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"

	"github.com/kesokaj/styrman/internal/config"
)

// Orchestrator creates, adopts, and reaps project microVMs.
type Orchestrator struct {
	docker *client.Client
	cfg    *config.Config
	logger *slog.Logger

	mu        sync.Mutex             // guards projLocks
	projLocks map[string]*sync.Mutex // per-project create/start serialization
	activity  sync.Map               // project name → time.Time of last terminal activity
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
	switch {
	case err == nil:
		id = insp.ID
		if insp.State == nil || !insp.State.Running {
			o.logger.Info("starting stopped project container", "project", project)
			if err := o.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
				return "", fmt.Errorf("start %s: %w", name, err)
			}
		}
	case errdefs.IsNotFound(err):
		id, err = o.create(ctx, createSpec{
			name:      name,
			netName:   config.NetworkName(project),
			hostname:  "styrman-" + opts.Name,
			guestUser: opts.GuestUser,
			homePath:  o.cfg.UserHome(user),
			extraEnv: []string{
				config.EnvProject + "=" + project,
				config.EnvProjectDir + "=" + opts.Dir,
				config.EnvDockerDisk + "=" + strconv.Itoa(o.cfg.DockerDisk),
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
	err := o.docker.NetworkConnect(ctx, config.NetworkName(project), o.cfg.SelfContainer, nil)
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
		_ = o.docker.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true})
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
	for _, c := range list {
		hasUser := c.Labels[config.LabelUser] != ""
		hasName := c.Labels[config.LabelName] != ""
		isSystem := c.Labels[config.LabelSystem] != ""
		if hasUser && (hasName || isSystem) {
			continue
		}
		o.logger.Info("removing previous-layout container", "id", c.ID[:12], "names", c.Names)
		_ = o.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
	}
	// Orphaned previous-scheme networks (best effort).
	nets, err := o.docker.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return
	}
	for _, n := range nets {
		if strings.HasPrefix(n.Name, "sdb-") {
			_ = o.docker.NetworkRemove(ctx, n.ID) // fails harmlessly if in use
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
	extraEnv  []string
	labels    map[string]string
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
			Binds: []string{
				spec.homePath + ":/home/" + spec.guestUser,
			},
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
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
	timeout := 30
	err := o.docker.ContainerStop(ctx, config.ContainerName(project), container.StopOptions{Timeout: &timeout})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

// Destroy removes the project's container and network. The workspace and all
// home-dir state persist; only runtime state is lost.
func (o *Orchestrator) Destroy(ctx context.Context, user, project string) error {
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
func (o *Orchestrator) RunIdleReaper(ctx context.Context) {
	if o.cfg.IdleTimeout <= 0 {
		return
	}
	interval := o.cfg.IdleTimeout / 4
	if interval < 5*time.Minute {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
				if time.Since(last) > o.cfg.IdleTimeout {
					o.logger.Info("stopping idle project", "user", st.User, "project", st.Project, "lastActivity", last)
					if err := o.Stop(ctx, st.User, st.Project); err != nil {
						o.logger.Warn("idle stop failed", "project", st.Project, "error", err)
					}
				}
			}
		}
	}
}
