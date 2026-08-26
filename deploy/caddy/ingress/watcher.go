package ingress

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Watcher monitors Docker for container events and regenerates the
// Caddyfile when containers are added or removed.
type Watcher struct {
	docker        *client.Client
	cfg           GeneratorConfig
	log           *slog.Logger
	adminAddr     string // Caddy admin API address (e.g., "localhost:2019")

	caddyfileMu   sync.Mutex  // guards lastCaddyfile
	lastCaddyfile string

	debounceMu    sync.Mutex  // guards debounceTimer
	debounceTimer *time.Timer
}

// NewWatcher creates a new Docker container watcher.
func NewWatcher(dockerClient *client.Client, cfg GeneratorConfig, logger *slog.Logger, adminAddr string) *Watcher {
	return &Watcher{
		docker:    dockerClient,
		cfg:       cfg,
		log:       logger,
		adminAddr: adminAddr,
	}
}

// Start begins watching Docker events and performs an initial scan.
func (w *Watcher) Start(ctx context.Context) error {
	// Initial scan
	w.log.Info("performing initial container scan")
	if err := w.scanAndReload(ctx); err != nil {
		w.log.Error("initial scan failed", "error", err)
		// Don't return error — continue watching for events
	}

	// Watch events
	go w.watchEvents(ctx)
	return nil
}

// watchEvents listens for Docker container events and triggers rescans.
func (w *Watcher) watchEvents(ctx context.Context) {
	for {
		w.listenEvents(ctx)

		// If context is cancelled, stop
		if ctx.Err() != nil {
			return
		}

		// Reconnect after a delay
		w.log.Info("docker event stream disconnected, reconnecting in 5s")
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

// listenEvents opens the Docker event stream and processes events.
func (w *Watcher) listenEvents(ctx context.Context) {
	args := filters.NewArgs()
	args.Add("type", "container")

	eventsChan, errChan := w.docker.Events(ctx, events.ListOptions{
		Filters: args,
	})

	w.log.Info("connected to docker event stream")

	for {
		select {
		case event := <-eventsChan:
			if w.isRelevantEvent(event) {
				w.log.Info("relevant docker event",
					"action", event.Action,
					"container", event.Actor.ID[:12],
				)
				w.debouncedRescan(ctx)
			}
		case err := <-errChan:
			if err != nil {
				w.log.Error("docker event stream error", "error", err)
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

// isRelevantEvent checks if a Docker event should trigger a rescan.
func (w *Watcher) isRelevantEvent(event events.Message) bool {
	return event.Type == events.ContainerEventType &&
		(event.Action == events.ActionStart ||
			event.Action == events.ActionStop ||
			event.Action == events.ActionDie ||
			event.Action == events.ActionDestroy)
}

// debouncedRescan schedules a rescan after a short delay to batch rapid events.
func (w *Watcher) debouncedRescan(ctx context.Context) {
	w.debounceMu.Lock()
	defer w.debounceMu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	w.debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
		if err := w.scanAndReload(ctx); err != nil {
			w.log.Error("rescan failed", "error", err)
		}
	})
}

// scanAndReload scans all running containers, generates a Caddyfile,
// and pushes it to Caddy if it has changed.
func (w *Watcher) scanAndReload(ctx context.Context) error {
	apps, err := w.discoverApps(ctx)
	if err != nil {
		return fmt.Errorf("discovering apps: %w", err)
	}

	w.log.Info("discovered apps", "count", len(apps))
	for _, app := range apps {
		w.log.Info("app discovered",
			"hostname", app.Hostname,
			"ip", app.ContainerIP,
			"port", app.Port,
			"mode", app.Mode,
		)
	}

	caddyfile := GenerateCaddyfile(apps, w.cfg)

	// Only reload if config changed
	w.caddyfileMu.Lock()
	changed := caddyfile != w.lastCaddyfile
	if changed {
		w.lastCaddyfile = caddyfile
	}
	w.caddyfileMu.Unlock()

	if !changed {
		w.log.Info("caddyfile unchanged, skipping reload")
		return nil
	}

	w.log.Info("caddyfile changed, reloading caddy")
	return w.reloadCaddy(ctx, caddyfile)
}

// discoverApps scans all running containers and returns those with caddy labels.
func (w *Watcher) discoverApps(ctx context.Context) ([]App, error) {
	containers, err := w.docker.ContainerList(ctx, containertypes.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var apps []App
	for _, c := range containers {
		app, err := w.containerToApp(ctx, c)
		if err != nil {
			w.log.Warn("skipping container",
				"container", containerName(c),
				"error", err,
			)
			continue
		}
		if app != nil {
			apps = append(apps, *app)
		}
	}

	return apps, nil
}

// containerToApp inspects a container and converts it to an App if applicable.
func (w *Watcher) containerToApp(ctx context.Context, c types.Container) (*App, error) {
	name := containerName(c)

	// Get exposed ports from the container config
	inspect, err := w.docker.ContainerInspect(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("inspecting container: %w", err)
	}

	exposedPorts := extractExposedPorts(inspect)

	app, err := ParseLabels(c.Labels, name, exposedPorts, w.cfg.DefaultRPS)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, nil
	}

	// Get container IP
	ip := extractContainerIP(inspect)
	if ip == "" {
		return nil, fmt.Errorf("no IP address found for container %q", name)
	}
	app.ContainerIP = ip

	return app, nil
}

// reloadCaddy pushes a new Caddyfile to Caddy's admin API.
func (w *Watcher) reloadCaddy(ctx context.Context, caddyfile string) error {
	url := fmt.Sprintf("http://%s/load", w.adminAddr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(caddyfile))
	if err != nil {
		return fmt.Errorf("creating reload request: %w", err)
	}
	req.Header.Set("Content-Type", "text/caddyfile")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending reload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caddy reload failed with status %d", resp.StatusCode)
	}

	w.log.Info("caddy reloaded successfully")
	return nil
}

// extractContainerIP returns the IP address to use for proxying to a container.
// For host-network containers, returns 127.0.0.1 (Caddy is also on host).
// For bridge containers, returns the first available bridge IP.
func extractContainerIP(inspect types.ContainerJSON) string {
	if inspect.NetworkSettings == nil {
		return ""
	}
	// Host network mode: container shares host's network stack.
	// Caddy is also on host, so localhost works.
	if inspect.HostConfig != nil && inspect.HostConfig.NetworkMode == "host" {
		return "127.0.0.1"
	}
	for _, network := range inspect.NetworkSettings.Networks {
		if network.IPAddress != "" {
			return network.IPAddress
		}
	}
	return ""
}

// extractExposedPorts returns the list of exposed ports from a container's config.
func extractExposedPorts(inspect types.ContainerJSON) []int {
	if inspect.Config == nil || inspect.Config.ExposedPorts == nil {
		return nil
	}
	var ports []int
	for port := range inspect.Config.ExposedPorts {
		p, err := strconv.Atoi(strings.Split(string(port), "/")[0])
		if err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

// containerName returns the container name without the leading slash.
func containerName(c types.Container) string {
	if len(c.Names) > 0 {
		return strings.TrimLeft(c.Names[0], "/")
	}
	return c.ID[:12]
}
