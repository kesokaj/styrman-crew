package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const (
	// reloadRetryDelay paces retries after a failed config load. The outage
	// that motivated it (2026-09-01): at host boot caddy started before the
	// network was up, caddy-security could not fetch Google's OIDC metadata,
	// the initial /load 400ed — and with no retry the ingress sat on the
	// minimal admin-only config, site down, until the next container event
	// (which a quiet host never sends).
	reloadRetryDelay = 15 * time.Second
	// adminDownLimit is how many CONSECUTIVE transport-level failures reaching
	// the admin API the watcher tolerates before declaring the process
	// unrecoverable. The admin listener lives in this same process: if it stops
	// answering, no future reload can ever land, so the only self-heal is a
	// process exit and a docker restart (fresh minimal admin + initial scan).
	adminDownLimit = 4
)

// errAdminUnreachable marks a reload failure at the transport layer — the
// admin API did not answer at all (vs. answering with a rejection).
var errAdminUnreachable = errors.New("caddy admin API unreachable")

// Watcher monitors Docker for container events and regenerates the
// Caddyfile when containers are added or removed.
type Watcher struct {
	docker    *client.Client
	cfg       GeneratorConfig
	log       *slog.Logger
	adminAddr string // Caddy admin API address (e.g., "localhost:2019")

	// scanMu serializes scanAndReload: the initial scan, the event debouncer,
	// and the failed-reload retry timer all call it from their own goroutines.
	// adminDown is guarded by it.
	scanMu    sync.Mutex
	adminDown int

	caddyfileMu   sync.Mutex // guards lastCaddyfile
	lastCaddyfile string

	debounceMu    sync.Mutex // guards debounceTimer
	debounceTimer *time.Timer

	retryMu    sync.Mutex // guards retryTimer
	retryTimer *time.Timer
	retryDelay time.Duration // reloadRetryDelay unless a test shortens it

	fatal chan error // buffered(1); see Fatal
}

// NewWatcher creates a new Docker container watcher.
func NewWatcher(dockerClient *client.Client, cfg GeneratorConfig, logger *slog.Logger, adminAddr string) *Watcher {
	return &Watcher{
		docker:     dockerClient,
		cfg:        cfg,
		log:        logger,
		adminAddr:  adminAddr,
		retryDelay: reloadRetryDelay,
		fatal:      make(chan error, 1),
	}
}

// Fatal reports a condition the watcher cannot recover from in-process (the
// admin API gone). The caller should exit nonzero and let the container
// restart policy recover.
func (w *Watcher) Fatal() <-chan error { return w.fatal }

// Start begins watching Docker events and performs an initial scan.
func (w *Watcher) Start(ctx context.Context) error {
	// Initial scan
	w.log.Info("performing initial container scan")
	if err := w.scanAndReload(ctx); err != nil {
		// Not fatal: a failed reload has already scheduled its own retry
		// (scanAndReload), which is exactly how the boot race resolves —
		// the network comes up, a retry lands, the site serves.
		w.log.Error("initial scan failed (retry scheduled)", "error", err)
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
	w.scanMu.Lock()
	defer w.scanMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	apps, err := w.discoverApps(ctx)
	if err != nil {
		// Retry here too: a transient docker-API hiccup inside a retry tick
		// must continue the chain, not terminate it — a dead chain is the
		// wedged-at-boot outage all over again (review finding).
		w.scheduleRetry(ctx)
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
	w.caddyfileMu.Unlock()

	if !changed {
		w.log.Info("caddyfile unchanged, skipping reload")
		return nil
	}

	w.log.Info("caddyfile changed, reloading caddy")
	if err := w.reloadCaddy(ctx, caddyfile); err != nil {
		// Do NOT record this as the last-applied config: a failed reload must be
		// retried on the next scan, not masked as "unchanged" — otherwise one
		// bad reload wedges the ingress on the stale live config forever (M23).
		w.noteReloadFailure(ctx, err)
		return err
	}
	w.adminDown = 0
	w.caddyfileMu.Lock()
	w.lastCaddyfile = caddyfile
	w.caddyfileMu.Unlock()
	return nil
}

// noteReloadFailure decides how a failed reload heals. A REJECTED load (the
// admin answered, e.g. 400 because an upstream fetch failed at boot) retries
// forever — caddy 2.11 keeps both the admin endpoint and the running config
// intact across a rejected load, verified live, so retrying is always safe. A
// TRANSPORT failure means the admin listener in this same process is gone;
// after adminDownLimit consecutive ones nothing in-process can ever fix it,
// and the watcher reports fatal instead. Caller holds scanMu.
func (w *Watcher) noteReloadFailure(ctx context.Context, err error) {
	if errors.Is(err, errAdminUnreachable) {
		w.adminDown++
		if w.adminDown >= adminDownLimit {
			select {
			case w.fatal <- fmt.Errorf("admin API unreachable %d consecutive times, last: %w", w.adminDown, err):
			default:
			}
			return
		}
	} else {
		w.adminDown = 0
	}
	w.scheduleRetry(ctx)
}

// scheduleRetry arms (or re-arms) a one-shot retry of the full scan+reload.
// Each failed attempt schedules the next, so a persistent failure becomes a
// steady retryDelay-paced loop rather than a wedge or a hot spin.
func (w *Watcher) scheduleRetry(ctx context.Context) {
	w.retryMu.Lock()
	defer w.retryMu.Unlock()
	if w.retryTimer != nil {
		w.retryTimer.Stop()
	}
	w.retryTimer = time.AfterFunc(w.retryDelay, func() {
		if ctx.Err() != nil {
			return
		}
		w.log.Info("retrying failed caddy reload")
		if err := w.scanAndReload(ctx); err != nil {
			w.log.Error("reload retry failed", "error", err)
		}
	})
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
		// Transport-level: the admin listener did not answer at all. Marked so
		// noteReloadFailure can tell "rejected" from "gone" (only the latter
		// is beyond in-process recovery).
		return fmt.Errorf("%w: %v", errAdminUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caddy reload failed with status %d", resp.StatusCode)
	}

	// A 200 is not proof of a load (BUG-087, the 2026-10-02 outage). When the
	// Caddyfile adapts with WARNINGS, caddy writes them to the response
	// before it loads anything (caddyconfig/load.go, handleLoad) — that sends
	// the 200 — and a load that then fails can only say so in the body, as
	// its {"error": …} object. At boot, with no network for the OIDC
	// metadata fetch, exactly that happened: this function logged success,
	// the Caddyfile was latched as applied, every later scan said
	// "unchanged", and caddy sat on its admin-only config for six hours.
	// Returned as a plain error, not errAdminUnreachable: the admin answered,
	// and the in-process retry (noteReloadFailure) is what heals it.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("caddy reload: reading the admin API's response: %v", err)
	}
	if msg := loadError(body); msg != "" {
		return fmt.Errorf("caddy reported a failed load behind status 200: %s", msg)
	}

	w.log.Info("caddy reloaded successfully")
	return nil
}

// extractContainerIP returns the IP address to use for proxying to a container.
// For host-network containers, returns 127.0.0.1 (Caddy is also on host).
func extractContainerIP(inspect types.ContainerJSON) string {
	if inspect.NetworkSettings == nil {
		return ""
	}
	// Host network mode: container shares host's network stack.
	// Caddy is also on host, so localhost works.
	if inspect.HostConfig != nil && inspect.HostConfig.NetworkMode == "host" {
		return "127.0.0.1"
	}
	nets := inspect.NetworkSettings.Networks
	// Prefer the ingress network. The styrman SERVER is ALSO attached to project
	// bridges (sdb-*, for the broker + preview proxy) whose IPs Caddy can't
	// reliably reach; a bare map iteration is random-order, so after a project
	// network was recreated it began returning a project-bridge IP → the app was
	// proxied to a dead address (502s on crew.styrman.xyz).
	if n, ok := nets["styrman-ingress"]; ok && n != nil && n.IPAddress != "" {
		return n.IPAddress
	}
	// Otherwise avoid project bridges (sdb-*); prefer any other network.
	for name, n := range nets {
		if n != nil && n.IPAddress != "" && !strings.HasPrefix(name, "sdb-") {
			return n.IPAddress
		}
	}
	// Last resort — a preview VM has only its own sdb-* network, which is right.
	for _, n := range nets {
		if n != nil && n.IPAddress != "" {
			return n.IPAddress
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

// loadError returns caddy's own error message when the body of a /load
// response carries one, "" otherwise. The body is a sequence of JSON values:
// the adapter's warnings array when there were any, then — only on failure —
// caddy's APIError object, {"error": "…"}. Only that object means failure:
// an empty body, warnings alone, any other value and an undecodable tail are
// all a load caddy did not report as failed (treating them as failures would
// re-load caddy on every retry tick forever).
func loadError(body []byte) string {
	dec := json.NewDecoder(bytes.NewReader(body))
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			return ""
		}
		if obj, ok := v.(map[string]any); ok {
			if msg, ok := obj["error"].(string); ok && msg != "" {
				return msg
			}
		}
	}
}
