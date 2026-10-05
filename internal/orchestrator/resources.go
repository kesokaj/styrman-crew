package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// Per-project sizing (ADR-083). Kata ADDS a container's limits to its own
// base VM (configuration-clh.toml: default_vcpus = 1, default_memory = 2048)
// and caps the workload at exactly the limit through the guest cgroup. An
// UNSET limit is not "unlimited": it is the 1 vCPU / 2 GiB base. So
// "unlimited" is spelled out as the host's own size, measured on the NUC
// 2026-10-05:
//   - vCPUs clamp at the host's count, and Docker refuses NanoCPUs above it
//     ("range of CPUs is from 0.01 to 16.00");
//   - memory does NOT clamp: base + limit, rounded UP to the 128 MiB hotplug
//     block, above host RAM fails the start ("The requested hotplug memory
//     addition is not a valid size").

// kataBaseMemoryMiB is configuration-clh.toml's default_memory: Kata adds a
// container's memory limit on top of it.
const kataBaseMemoryMiB = 2048

// memoryHotplugBlockMiB is the granularity Cloud Hypervisor hot-adds memory
// in; a limit is rounded up to it.
const memoryHotplugBlockMiB = 128

// hostProbeTimeout bounds the one `docker info` call (plan Open row).
const hostProbeTimeout = 10 * time.Second

// Host is the machine's size as the Docker daemon reports it (`docker
// info`: NCPU, MemTotal). The daemon's count is the one it validates
// NanoCPUs against, and it is the same on every platform the server runs on.
type Host struct{ CPUs, MemMiB int }

// MaxMemoryMiB is the largest memory limit Kata can add to its base on this
// host, or 0 when the host is too small to add any (under 2176 MiB) — then
// no memory limit is set at all and the VM gets Kata's default.
func (h Host) MaxMemoryMiB() int {
	m := (h.MemMiB - kataBaseMemoryMiB) / memoryHotplugBlockMiB * memoryHotplugBlockMiB
	if m <= 0 {
		return 0
	}
	return m
}

// MaxMemoryGiB is MaxMemoryMiB in whole GiB: the most a user may set by hand.
func (h Host) MaxMemoryGiB() int { return h.MaxMemoryMiB() / 1024 }

// Resources is a project's request, from its record. CPUs and MemoryGiB 0
// mean unlimited (the whole host). DiskGB 0 — a record from before ADR-083 —
// means "keep what the container has".
type Resources struct{ CPUs, MemoryGiB, DiskGB int }

// Sizing is what a container is, or would be, created with.
type Sizing struct {
	NanoCPUs int64 // HostConfig.NanoCPUs
	MemBytes int64 // HostConfig.Memory; 0 = no limit
	DiskGB   int   // the STYRMAN_DOCKER_DISK_GB the entrypoint sizes docker.img to
}

// Live is a container's current sizing, read from `docker inspect`.
type Live struct {
	Sizing
	Running bool
}

// Host returns the machine's size, asking the daemon on first need and
// caching the answer only once it succeeds: a failure is returned and the
// next call asks again (a slow dockerd after a reboot must not leave the
// server sizing at zero forever, and must not block startup either).
func (o *Orchestrator) Host(ctx context.Context) (Host, error) {
	o.hostMu.Lock()
	defer o.hostMu.Unlock()
	if o.host.CPUs > 0 {
		return o.host, nil
	}
	ctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()
	info, err := o.docker.Info(ctx)
	if err != nil {
		return Host{}, fmt.Errorf("host size: docker info: %w", err)
	}
	if info.NCPU <= 0 || info.MemTotal <= 0 {
		return Host{}, fmt.Errorf("host size: docker info reported %d CPUs and %d bytes of memory", info.NCPU, info.MemTotal)
	}
	o.host = Host{CPUs: info.NCPU, MemMiB: int(info.MemTotal >> 20)}
	return o.host, nil
}

// WantSizing computes what a container for r should be created with on host
// h. live is the existing container's state, nil when there is none; only a
// DiskGB of 0 consults it. A request above the host is clamped, with a WARN,
// never refused: a project must not become unstartable because the hardware
// shrank (plan D13).
func (o *Orchestrator) WantSizing(h Host, r Resources, live *Live) Sizing {
	cpus := r.CPUs
	if cpus > h.CPUs {
		o.logger.Warn("project CPUs above the host's; using the host's", "requested", cpus, "host", h.CPUs)
	}
	if cpus <= 0 || cpus > h.CPUs {
		cpus = h.CPUs
	}
	maxMiB := h.MaxMemoryMiB()
	memMiB := r.MemoryGiB * 1024
	if memMiB > maxMiB && maxMiB > 0 {
		o.logger.Warn("project memory above the host's ceiling; using the ceiling", "requestedGiB", r.MemoryGiB, "ceilingMiB", maxMiB)
	}
	if memMiB <= 0 || memMiB > maxMiB {
		memMiB = maxMiB // 0 on a host too small to add any: no limit
	}
	disk := r.DiskGB
	if disk <= 0 && live != nil {
		disk = live.DiskGB
	}
	if disk <= 0 {
		disk = o.cfg.DockerDiskGB()
	}
	return Sizing{NanoCPUs: int64(cpus) * 1_000_000_000, MemBytes: int64(memMiB) << 20, DiskGB: disk}
}

// LiveSizing inspects the project's container WITHOUT the project lock — it
// is a read, and the Resources dialog must not wait behind a boot. found is
// false when there is no container; an inspect error is returned, never
// folded into "absent".
func (o *Orchestrator) LiveSizing(ctx context.Context, project string) (live Live, found bool, err error) {
	insp, err := o.docker.ContainerInspect(ctx, config.ContainerName(project))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return Live{}, false, nil
		}
		return Live{}, false, err
	}
	return liveFrom(insp), true, nil
}

// liveFrom reads a container's sizing out of its inspect response.
func liveFrom(insp container.InspectResponse) Live {
	var l Live
	if insp.ContainerJSONBase != nil {
		if insp.HostConfig != nil {
			l.NanoCPUs, l.MemBytes = insp.HostConfig.NanoCPUs, insp.HostConfig.Memory
		}
		l.Running = insp.State != nil && insp.State.Running
	}
	if insp.Config != nil {
		// Unparsable or absent reads as 0: "unknown", which WantSizing
		// replaces with the operator's default (plan Open row).
		l.DiskGB, _ = strconv.Atoi(containerEnv(insp.Config.Env, config.EnvDockerDisk))
	}
	return l
}

// containerEnv returns the value of key in a container's env, "" if absent.
func containerEnv(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, prefix); ok {
			return v
		}
	}
	return ""
}

// errNoResources is the refusal of a start whose RunOpts carries no record
// reader: an unknown size is never guessed as "unlimited" (plan D18).
var errNoResources = errors.New("resources: no record reader — refusing to start at an unknown size")
