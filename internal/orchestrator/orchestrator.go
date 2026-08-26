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

	mu       sync.Mutex // serializes create/start per process
	activity sync.Map   // project name → time.Time of last terminal activity
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
	Project     string `json:"project"`
	ContainerID string `json:"containerId,omitempty"`
	State       string `json:"state"` // "running", "stopped", "absent"
}

// Touch records terminal activity for the idle reaper.
func (o *Orchestrator) Touch(project string) { o.activity.Store(project, time.Now()) }

// Status inspects the project's container.
func (o *Orchestrator) Status(ctx context.Context, project string) (Status, error) {
	insp, err := o.docker.ContainerInspect(ctx, config.ContainerName(project))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return Status{Project: project, State: "absent"}, nil
		}
		return Status{}, err
	}
	st := "stopped"
	if insp.State != nil && insp.State.Running {
		st = "running"
	}
	return Status{Project: project, ContainerID: insp.ID, State: st}, nil
}

// EnsureRunning makes sure the project's microVM exists and is running, and
// waits until the entrypoint reports ready. Returns the container ID.
func (o *Orchestrator) EnsureRunning(ctx context.Context, project string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

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
		id, err = o.create(ctx, project)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}

	if err := o.waitReady(ctx, id, 90*time.Second); err != nil {
		return "", fmt.Errorf("project %s did not become ready: %w", project, err)
	}
	o.Touch(project)
	return id, nil
}

// create builds the per-project network and container. Caller holds o.mu.
func (o *Orchestrator) create(ctx context.Context, project string) (string, error) {
	netName := config.NetworkName(project)
	if _, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{}); err != nil {
		if !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("inspect network %s: %w", netName, err)
		}
		if _, err := o.docker.NetworkCreate(ctx, netName, network.CreateOptions{Driver: "bridge"}); err != nil {
			return "", fmt.Errorf("create network %s: %w", netName, err)
		}
	}

	name := config.ContainerName(project)
	o.logger.Info("creating project microVM", "project", project, "image", o.cfg.Image, "runtime", o.cfg.Runtime)

	resp, err := o.docker.ContainerCreate(ctx,
		&container.Config{
			Image:    o.cfg.Image,
			Hostname: name,
			Env: []string{
				config.EnvUser + "=" + o.cfg.Username,
				config.EnvUID + "=" + strconv.Itoa(o.cfg.UID),
				config.EnvGID + "=" + strconv.Itoa(o.cfg.GID),
				config.EnvProject + "=" + project,
				config.EnvDockerDisk + "=" + strconv.Itoa(o.cfg.DockerDisk),
				config.EnvKeyringPW + "=" + o.cfg.KeyringPW,
			},
			Labels: map[string]string{
				config.LabelManaged: "true",
				config.LabelProject: project,
			},
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
				o.cfg.HomeDir + ":/home/" + o.cfg.Username,
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
		return "", fmt.Errorf("create container %s: %w", name, err)
	}
	if err := o.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = o.docker.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
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
func (o *Orchestrator) Stop(ctx context.Context, project string) error {
	timeout := 30
	err := o.docker.ContainerStop(ctx, config.ContainerName(project), container.StopOptions{Timeout: &timeout})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

// Destroy removes the project's container and network. The workspace and all
// home-dir state persist; only runtime state is lost.
func (o *Orchestrator) Destroy(ctx context.Context, project string) error {
	err := o.docker.ContainerRemove(ctx, config.ContainerName(project), container.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err := o.docker.NetworkRemove(ctx, config.NetworkName(project)); err != nil && !errdefs.IsNotFound(err) {
		o.logger.Warn("network remove failed", "project", project, "error", err)
	}
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
		if p == "" {
			continue
		}
		st := "stopped"
		if strings.HasPrefix(c.Status, "Up") {
			st = "running"
		}
		out[p] = Status{Project: p, ContainerID: c.ID, State: st}
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
			for p, st := range statuses {
				if st.State != "running" {
					continue
				}
				last := time.Time{}
				if v, ok := o.activity.Load(p); ok {
					last = v.(time.Time)
				}
				if last.IsZero() || time.Since(last) > o.cfg.IdleTimeout {
					o.logger.Info("stopping idle project", "project", p, "lastActivity", last)
					if err := o.Stop(ctx, p); err != nil {
						o.logger.Warn("idle stop failed", "project", p, "error", err)
					}
				}
			}
		}
	}
}
