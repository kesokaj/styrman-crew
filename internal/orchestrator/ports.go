package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/errdefs"

	"github.com/kesokaj/styrman/internal/config"
)

// IPAddress returns the project container's IP on its bridge network, or ""
// if not running.
func (o *Orchestrator) IPAddress(ctx context.Context, user, project string) (string, error) {
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

// portScanScript runs inside the guest: list LISTEN sockets from
// /proc/net/tcp{,6} with their inodes, then walk /proc/*/fd to map inode →
// pid → comm. Output: "<hexport> <comm>" per socket (comm may be "?").
const portScanScript = `
awk '$4=="0A" {split($2,a,":"); print a[2], $10}' /proc/net/tcp /proc/net/tcp6 2>/dev/null | sort -u | while read -r hexport inode; do
  comm="?"
  for fd in /proc/[0-9]*/fd/*; do
    if [ "$(readlink "$fd" 2>/dev/null)" = "socket:[$inode]" ]; then
      pid=${fd#/proc/}; pid=${pid%%/*}
      comm=$(cat /proc/$pid/comm 2>/dev/null || echo '?')
      break
    fi
  done
  echo "$hexport $comm"
done`

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
	if err != nil {
		return nil, err
	}
	if code != 0 {
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
		hexport, comm, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		p64, err := strconv.ParseInt(hexport, 16, 32)
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
