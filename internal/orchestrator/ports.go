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

// ListeningPorts scans the guest's /proc/net/tcp{,6} for LISTEN sockets
// (devbox's portscan approach, via exec instead of an in-guest server).
// Ports below 1024 are skipped — nothing user-launched listens there.
func (o *Orchestrator) ListeningPorts(ctx context.Context, project string) ([]int, error) {
	st, err := o.Status(ctx, project)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return []int{}, nil
	}
	code, out, err := o.Exec(ctx, st.ContainerID, ExecSpec{
		Cmd: []string{"sh", "-c", "cat /proc/net/tcp /proc/net/tcp6 2>/dev/null"},
	})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("port scan exited %d", code)
	}
	seen := map[int]bool{}
	var ports []int
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		// local_address is field 1 ("HEXIP:HEXPORT"), state is field 3
		// ("0A" = LISTEN).
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		hexPort := fields[1][strings.LastIndex(fields[1], ":")+1:]
		p, err := strconv.ParseInt(hexPort, 16, 32)
		if err != nil || p < 1024 || p > 65535 || seen[int(p)] {
			continue
		}
		seen[int(p)] = true
		ports = append(ports, int(p))
	}
	if ports == nil {
		ports = []int{}
	}
	return ports, nil
}
