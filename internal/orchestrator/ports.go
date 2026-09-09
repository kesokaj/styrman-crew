package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
const portScanScript = `
if command -v ss >/dev/null 2>&1; then
  ss -ltnH 2>/dev/null | awk '
    { split($4, a, ":"); port=a[length(a)];
      comm="?"; if (match($0, /users:\(\("[^"]+"/)) { c=substr($0, RSTART+8); sub(/".*/, "", c); comm=c }
      print port, comm }'
else
  awk '$4=="0A" {split($2,a,":"); print strtonum("0x" a[2]), "?"}' /proc/net/tcp /proc/net/tcp6 2>/dev/null | sort -u
fi`

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
	// exec itself (128, observed) — so both are re-asked the same question. If
	// the project is no longer the running container we scanned, there are no
	// ports to report, and that is an answer rather than an internal error:
	// the same shape the git endpoints use for a guest that is not there.
	// A failure against the container we are STILL looking at is a real one.
	if err != nil || code != 0 {
		if now, err2 := o.Status(ctx, user, project); err2 == nil &&
			(now.State != "running" || now.ContainerID != st.ContainerID) {
			return []Port{}, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("port scan exited %d", code)
	}

	// Inner-docker port → image map (best effort; dockerd may be absent in
	// system VMs). docker-proxy owns published ports, so attribute those to
	// the container image instead of the proxy process.
	images := map[int]string{}
	if c2, out2, err2 := o.Exec(ctx, st.ContainerID, ExecSpec{
		Cmd: []string{"sh", "-c", `docker ps --format '{{.Image}}|{{.Ports}}' 2>/dev/null`},
	}); err2 == nil && c2 == 0 {
		for _, line := range strings.Split(strings.TrimSpace(out2), "\n") {
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
	}

	seen := map[int]bool{}
	var ports []Port
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
	if ports == nil {
		ports = []Port{}
	}
	return ports, nil
}
