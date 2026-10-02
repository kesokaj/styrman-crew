package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/errdefs"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// IPAddress returns the project container's IP on its bridge network, or ""
// if not running.
func (o *Orchestrator) IPAddress(ctx context.Context, project string) (string, error) {
	insp, err := o.docker.ContainerInspect(ctx, config.ContainerName(project))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	if insp.State == nil || !insp.State.Running || insp.NetworkSettings == nil {
		return "", nil
	}
	if net, ok := insp.NetworkSettings.Networks[config.NetworkName(project)]; ok {
		return net.IPAddress, nil
	}
	for _, net := range insp.NetworkSettings.Networks {
		if net.IPAddress != "" {
			return net.IPAddress, nil
		}
	}
	return "", nil
}

// Port is one listening TCP port inside a project's microVM, annotated with
// the owning process and, when the port belongs to a container of the
// project's inner dockerd, the container's image.
type Port struct {
	Port    int    `json:"port"`
	Process string `json:"process,omitempty"`
	Image   string `json:"image,omitempty"`
}

// portScanScript lists LISTEN sockets inside the guest. `ss -ltnH` does the
// socket→pid→comm join in one pass in-kernel (iproute2 is in the image),
// avoiding the previous O(listening × open-fds) /proc/*/fd walk that took
// ~190 ms per port on a busy VM. Output: "<decport> <comm>" per socket.
// Falls back to a cheap /proc scan (no comm) if ss is unavailable.
//
// The inner dockerd's published ports follow in the SAME exec, after the
// sentinel line: the Ports panel polls every 7 s and each exec is a Docker
// round trip plus a process in the guest, so two of them per poll was the
// single most frequent exec the server ran (review finding, 2026-09-13).
// `|| true` keeps the script's status 0 whether or not dockerd exists (system
// VMs have none); the vanished-container guard in ListeningPorts rests on
// the DOCKER exec's own code and error, not on the script's. `timeout 3`
// keeps a wedged or still-booting inner dockerd from spending the exec's
// whole 30 s budget and taking the socket listing down with it — the
// annotation degrades, the rows stay (BUG-039's rule; review, 2026-09-13).
const portScanScript = `
if command -v ss >/dev/null 2>&1; then
  ss -ltnH 2>/dev/null | awk '
    { split($4, a, ":"); port=a[length(a)];
      comm="?"; if (match($0, /users:\(\("[^"]+"/)) { c=substr($0, RSTART+8); sub(/".*/, "", c); comm=c }
      print port, comm }'
else
  awk '$4=="0A" {split($2,a,":"); print strtonum("0x" a[2]), "?"}' /proc/net/tcp /proc/net/tcp6 2>/dev/null | sort -u
fi
printf '%s\n' '` + portScanSentinel + `'
timeout 3 docker ps --format '{{.Image}}|{{.Ports}}' 2>/dev/null || true`

// portScanSentinel separates the socket lines from the docker lines in
// portScanScript's output. Both halves are guest-controlled text (a process
// comm, an image name), so the split is the FIRST line equal to the
// sentinel (surrounding whitespace ignored, so a CR cannot defeat it): the
// worst a guest can do by printing it is hide its own ports from its own
// panel.
const portScanSentinel = "--styrman-docker--"

// ListeningPorts scans the guest for LISTEN sockets ≥1024 with process names
// and inner-docker image attribution.
func (o *Orchestrator) ListeningPorts(ctx context.Context, user, project string) ([]Port, error) {
	st, err := o.Status(ctx, user, project)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return []Port{}, nil
	}
	code, out, err := o.Exec(ctx, st.ContainerID, ExecSpec{
		Cmd: []string{"sh", "-c", portScanScript},
	})
	// Status and Exec are two calls, and the container can go away between
	// them. That used to be a rare race (a stop landing on a poll); an ADR-067
	// mode toggle makes it routine, because the toggle REPLACES the container
	// while the UI is polling this endpoint. A vanished container shows up
	// EITHER way — as a transport error, or as a non-zero exit code from the
	// exec itself (128, 9 observed) — so both are re-asked the same question
	// (GuestGoingAway, which also waits out a stop still in flight). If the
	// guest went away there are no ports to report, and that is an answer
	// rather than an internal error: the same shape the git endpoints use for
	// a guest that is not there. A failure against the container we are STILL
	// looking at, once nothing is changing it, is a real one.
	if err != nil || code != 0 {
		if o.GuestGoingAway(ctx, user, project, st) {
			return []Port{}, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("port scan exited %d", code)
	}

	scan, images := splitPortScan(out)
	return parseListeningPorts(scan, images), nil
}

// GuestGoingAway answers, after an exec into ran.ContainerID failed, whether
// the failure belongs to the guest going away rather than to the command:
// whether the project is no longer that running container (BUG-023). A
// container being stopped is still reported running, with the same id, until
// its PID 1 exits, and the exec the shutdown killed exits non-zero (9,
// observed on Kata; BUG-088) — so while the answer is "still the same" and a
// lifecycle change holds the project's lock, it keeps asking until the
// guest is gone or the lock comes free. A stop or a mode toggle reads
// stopped or replaced as soon as its container is down; a change that
// leaves the guest as it was (an attach's no-op EnsureRunning, the idle
// reaper's busy probe) reads the same when it lets go, and the failure stays
// the command's. A held lock alone never answers the question. Bounded by
// ctx and by lifecycleWaitMax: a wait that ends first gets false, and its
// caller reports the failure. Never call it while holding the project's
// lock.
func (o *Orchestrator) GuestGoingAway(ctx context.Context, user, project string, ran Status) bool {
	gone := func() bool {
		now, err := o.Status(ctx, user, project)
		return err == nil && (now.State != "running" || now.ContainerID != ran.ContainerID)
	}
	ctx, cancel := context.WithTimeout(ctx, lifecycleWaitMax)
	defer cancel()
	l := o.lockProject(key(user, project))
	for {
		if gone() {
			return true
		}
		// Asked on every tick, not only once the lock is free: a mode
		// toggle holds the lock through waitReady (up to 90 s), far longer
		// than the page waits for a poll, while its container is already
		// down after the first second or two (gate review, finding 1).
		if l.TryLock() {
			l.Unlock()
			return gone()
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(lifecycleRecheck):
		}
	}
}

// lifecycleWaitMax bounds GuestGoingAway's wait whatever the request's own
// deadline: a mode toggle holds the lock through waitReady (up to 90 s), the
// page gives up on a poll after 20 s, and a client that never gives up must
// not keep asking Docker ten times a second for the rest of the toggle (gate
// security review of 57179a4..624808d, finding 1). Past it the failure is
// reported as it stands. A var only so a test can shorten it.
var lifecycleWaitMax = 20 * time.Second

// lifecycleRecheck is how often GuestGoingAway looks again for the end of a
// lifecycle change in flight.
const lifecycleRecheck = 100 * time.Millisecond

// splitPortScan separates portScanScript's output into the socket lines and
// the inner-docker port → image map. No sentinel (an older runtime image
// whose script had none, or truncated output) means the whole output is
// the scan and nothing is attributed — never "no ports".
func splitPortScan(out string) (scan string, images map[int]string) {
	images = map[int]string{}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == portScanSentinel {
			return strings.Join(lines[:i], "\n"), parseDockerPorts(strings.Join(lines[i+1:], "\n"))
		}
	}
	return out, images
}

// parseDockerPorts maps published host ports to their image from
// `docker ps --format '{{.Image}}|{{.Ports}}'` lines (best effort; dockerd
// may be absent in system VMs). docker-proxy owns published ports, so the
// caller attributes those to the container image instead of the proxy.
func parseDockerPorts(lines string) map[int]string {
	images := map[int]string{}
	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		img, ports, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		for _, part := range strings.Split(ports, ",") {
			// e.g. "0.0.0.0:8080->80/tcp"
			if h, _, ok := strings.Cut(strings.TrimSpace(part), "->"); ok {
				if _, pstr, ok := strings.Cut(h, ":"); ok {
					if p, err := strconv.Atoi(pstr); err == nil {
						images[p] = img
					}
				}
			}
		}
	}
	return images
}

// parseListeningPorts turns portScanScript's "<port> <comm>" lines into the
// port list, attributing inner-docker published ports to their image. Pure,
// so the filtering rules are unit-testable without a guest.
//
// The project's browser ports (ADR-068) are dropped: a DevTools client
// controls the whole browser, so the port must never be previewed, shared or
// linked — and the Ports panel is where every one of those starts. The
// Browser pane reaches it through its own owner-only relay instead.
func parseListeningPorts(out string, images map[int]string) []Port {
	seen := map[int]bool{}
	ports := []Port{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		portStr, comm, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			portStr, comm = strings.TrimSpace(line), "?"
		}
		p64, err := strconv.ParseInt(portStr, 10, 32)
		p := int(p64)
		if err != nil || p < 1024 || p > 65535 || seen[p] {
			continue
		}
		if p == config.BrowserPort || p == config.BrowserChromePort {
			continue
		}
		seen[p] = true
		port := Port{Port: p, Process: comm}
		if comm == "?" {
			port.Process = ""
		}
		if img, ok := images[p]; ok {
			port.Image = img
			if port.Process == "docker-proxy" {
				port.Process = ""
			}
		}
		ports = append(ports, port)
	}
	return ports
}
