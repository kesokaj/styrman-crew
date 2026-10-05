package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// fakeDocker stands in for the Engine behind the dockerAPI seam (docker.go):
// (fakeContainer fields are read and written under f.mu — the unmark
// goroutine inspects while a test may still be mutating; review nit.)
// an in-memory table of containers and networks, a per-method scripted
// error, and a call log. "Absent" answers are errdefs.NotFound, so the
// orchestrator's eleven IsNotFound branches are the ones exercised. Every
// exec succeeds at once with execExit (waitReady's probe, stopLocked's
// kill), unless the method is scripted to fail.
// Deliberately minimal (security review of the close-out's phase 5):
// ContainerList/NetworkList return nothing, Remove ignores Force and
// endpoints, Create overwrites a duplicate name — a test of CleanupLegacy,
// Adopt, ResolveProjectByIP or a non-Force remove must extend the fake
// first, or it passes vacuously.
type fakeDocker struct {
	mu         sync.Mutex
	containers map[string]*fakeContainer // by name
	networks   map[string]*fakeNetwork   // by name
	imageID    string
	fail       map[string]error
	execExit   int
	// info is what Info returns: the host's size (ADR-083).
	info system.Info
	// list opts a test into ContainerList returning the table (filtered by
	// label); off, it returns nothing, as every older test assumes.
	list   bool
	calls  []string
	onCall func(method string) // observe the orchestrator's state mid-call (not under f.mu)
}

type fakeContainer struct {
	id, name, image string
	running         bool
	binds           []string
	netName         string
	// ips: the container's address per network, as ContainerInspect reports
	// it (BrowserPeer reads the server's own).
	ips map[string]string
	// restart is the restart policy name ("unless-stopped", "no"); the
	// exec of stopLocked's in-guest TERM plays the daemon's restart manager
	// against it (BUG-063), and restartCount is what `docker inspect` would
	// show afterwards.
	restart      string
	restartCount int
	// nanoCPUs, memory and env are what ContainerCreate was given and what
	// ContainerInspect reports back (HostConfig.Resources, Config.Env), so
	// the sizing recreate (ADR-083) has something to compare.
	nanoCPUs, memory int64
	env              []string
	// labels and startedAt feed ContainerList's label filter and
	// ContainerInspect's State.StartedAt (the setup-VM reaper reads both).
	labels    map[string]string
	startedAt string
}

type fakeNetwork struct {
	name, bridge string
	gateway      string // IPAM gateway, as NetworkInspect reports it
	subnet       string // IPAM subnet (CIDR), the broker address derives from it
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		containers: map[string]*fakeContainer{},
		networks:   map[string]*fakeNetwork{},
		imageID:    "sha256:image-current",
		fail:       map[string]error{},
		info:       system.Info{NCPU: 16, MemTotal: 63853 << 20},
	}
}

func notFound(what string) error { return errdefs.NotFound(errors.New("no such " + what)) }

func (f *fakeDocker) record(call string) error {
	method, _, _ := strings.Cut(call, "(")
	if f.onCall != nil {
		f.onCall(method)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.fail[method]
}

func (f *fakeDocker) callsTo(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, method+"(") {
			n++
		}
	}
	return n
}

func (f *fakeDocker) byName(name string) (*fakeContainer, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.containers[name]; ok {
		return c, true
	}
	for _, c := range f.containers {
		if c.id == name {
			return c, true
		}
	}
	return nil, false
}

func (f *fakeDocker) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	if err := f.record("ContainerInspect(" + id + ")"); err != nil {
		return container.InspectResponse{}, err
	}
	c, ok := f.byName(id)
	if !ok {
		return container.InspectResponse{}, notFound("container")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{
		ID: c.id, Image: c.image, State: &container.State{Running: c.running, StartedAt: c.startedAt}, RestartCount: c.restartCount,
		HostConfig: &container.HostConfig{Binds: append([]string(nil), c.binds...),
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(c.restart)},
			Resources:     container.Resources{NanoCPUs: c.nanoCPUs, Memory: c.memory}},
	}, Config: &container.Config{Env: append([]string(nil), c.env...)}}
	if len(c.ips) > 0 {
		resp.NetworkSettings = &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}}
		for n, ip := range c.ips {
			resp.NetworkSettings.Networks[n] = &network.EndpointSettings{IPAddress: ip}
		}
	}
	return resp, nil
}

func (f *fakeDocker) ContainerCreate(_ context.Context, cfg *container.Config, hc *container.HostConfig, nc *network.NetworkingConfig, _ *ocispec.Platform, name string) (container.CreateResponse, error) {
	if err := f.record("ContainerCreate(" + name + ")"); err != nil {
		return container.CreateResponse{}, err
	}
	c := &fakeContainer{id: "id-" + name, name: name, image: f.imageID}
	if cfg != nil && cfg.Image != "" {
		c.image = f.imageID
	}
	if hc != nil {
		c.binds = hc.Binds
		c.restart = string(hc.RestartPolicy.Name)
		c.nanoCPUs, c.memory = hc.NanoCPUs, hc.Memory
	}
	if cfg != nil {
		c.env = append([]string(nil), cfg.Env...)
		c.labels = cfg.Labels
	}
	if nc != nil {
		for n := range nc.EndpointsConfig {
			c.netName = n
		}
	}
	f.mu.Lock()
	f.containers[name] = c
	f.mu.Unlock()
	return container.CreateResponse{ID: c.id}, nil
}

func (f *fakeDocker) ContainerUpdate(_ context.Context, id string, cfg container.UpdateConfig) (container.UpdateResponse, error) {
	if err := f.record("ContainerUpdate(" + id + ":" + string(cfg.RestartPolicy.Name) + ")"); err != nil {
		return container.UpdateResponse{}, err
	}
	c, ok := f.byName(id)
	if !ok {
		return container.UpdateResponse{}, notFound("container")
	}
	f.mu.Lock()
	c.restart = string(cfg.RestartPolicy.Name)
	f.mu.Unlock()
	return container.UpdateResponse{}, nil
}

func (f *fakeDocker) ContainerStart(_ context.Context, id string, _ container.StartOptions) error {
	if err := f.record("ContainerStart(" + id + ")"); err != nil {
		return err
	}
	c, ok := f.byName(id)
	if !ok {
		return notFound("container")
	}
	f.mu.Lock()
	c.running = true
	f.mu.Unlock()
	return nil
}

func (f *fakeDocker) ContainerStop(_ context.Context, id string, _ container.StopOptions) error {
	if err := f.record("ContainerStop(" + id + ")"); err != nil {
		return err
	}
	c, ok := f.byName(id)
	if !ok {
		return notFound("container")
	}
	f.mu.Lock()
	c.running = false
	f.mu.Unlock()
	return nil
}

func (f *fakeDocker) ContainerRemove(_ context.Context, id string, _ container.RemoveOptions) error {
	if err := f.record("ContainerRemove(" + id + ")"); err != nil {
		return err
	}
	c, ok := f.byName(id)
	if !ok {
		return notFound("container")
	}
	f.mu.Lock()
	delete(f.containers, c.name)
	f.mu.Unlock()
	return nil
}

func (f *fakeDocker) ContainerList(_ context.Context, opts container.ListOptions) ([]container.Summary, error) {
	if err := f.record("ContainerList()"); err != nil {
		return nil, err
	}
	if !f.list {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []container.Summary
next:
	for _, c := range f.containers {
		for _, kv := range opts.Filters.Get("label") {
			k, v, _ := strings.Cut(kv, "=")
			if c.labels[k] != v {
				continue next
			}
		}
		out = append(out, container.Summary{ID: c.id, Names: []string{"/" + c.name}, Labels: c.labels})
	}
	return out, nil
}

func (f *fakeDocker) ContainerLogs(_ context.Context, id string, _ container.LogsOptions) (io.ReadCloser, error) {
	if err := f.record("ContainerLogs(" + id + ")"); err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeDocker) ContainerExecCreate(_ context.Context, id string, opts container.ExecOptions) (container.ExecCreateResponse, error) {
	cmd := strings.Join(opts.Cmd, " ")
	if err := f.record("ContainerExecCreate(" + cmd + ")"); err != nil {
		return container.ExecCreateResponse{}, err
	}
	c, ok := f.byName(id)
	if !ok {
		return container.ExecCreateResponse{}, notFound("container")
	}
	// stopLocked's in-guest TERM (when it takes, execExit 0): PID 1 exits 0
	// and the container's task ends. The daemon's restart manager then does
	// what the policy says — a clean exit under `unless-stopped` with no
	// stop on record is a RESTART, one more boot and RestartCount+1
	// (BUG-063, dockerd: "restarting container … manualRestart=false");
	// under "no" the container stays down.
	if cmd == "kill -TERM 1" && f.execExit == 0 {
		f.mu.Lock()
		c.running = false
		if c.restart == "unless-stopped" || c.restart == "always" {
			c.running = true
			c.restartCount++
		}
		f.mu.Unlock()
	}
	return container.ExecCreateResponse{ID: "exec-1"}, nil
}

func (f *fakeDocker) ContainerExecAttach(_ context.Context, id string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
	if err := f.record("ContainerExecAttach(" + id + ")"); err != nil {
		return types.HijackedResponse{}, err
	}
	// An already-finished command: the peer end closes at once, so the
	// stdcopy demux sees EOF and Exec proceeds to the exit-code inspect.
	ours, theirs := net.Pipe()
	_ = theirs.Close()
	return types.HijackedResponse{Conn: ours, Reader: bufio.NewReader(ours)}, nil
}

func (f *fakeDocker) ContainerExecInspect(_ context.Context, id string) (container.ExecInspect, error) {
	if err := f.record("ContainerExecInspect(" + id + ")"); err != nil {
		return container.ExecInspect{}, err
	}
	return container.ExecInspect{ExecID: id, ExitCode: f.execExit, Running: false}, nil
}

func (f *fakeDocker) ContainerExecResize(_ context.Context, id string, _ container.ResizeOptions) error {
	return f.record("ContainerExecResize(" + id + ")")
}

func (f *fakeDocker) NetworkCreate(_ context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	if err := f.record("NetworkCreate(" + name + ")"); err != nil {
		return network.CreateResponse{}, err
	}
	f.mu.Lock()
	f.networks[name] = &fakeNetwork{name: name, bridge: opts.Options["com.docker.network.bridge.name"]}
	f.mu.Unlock()
	return network.CreateResponse{ID: "net-" + name}, nil
}

func (f *fakeDocker) NetworkInspect(_ context.Context, name string, _ network.InspectOptions) (network.Inspect, error) {
	if err := f.record("NetworkInspect(" + name + ")"); err != nil {
		return network.Inspect{}, err
	}
	f.mu.Lock()
	n, ok := f.networks[name]
	f.mu.Unlock()
	if !ok {
		return network.Inspect{}, notFound("network")
	}
	insp := network.Inspect{Name: n.name, ID: "net-" + n.name, Options: map[string]string{"com.docker.network.bridge.name": n.bridge}}
	if n.gateway != "" || n.subnet != "" {
		insp.IPAM.Config = []network.IPAMConfig{{Subnet: n.subnet, Gateway: n.gateway}}
	}
	return insp, nil
}

func (f *fakeDocker) NetworkRemove(_ context.Context, name string) error {
	if err := f.record("NetworkRemove(" + name + ")"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.networks[name]; !ok {
		return notFound("network")
	}
	delete(f.networks, name)
	return nil
}

// NetworkConnect records the static address when the caller asked for one
// (`NetworkConnect(net,id,ip=A.B.C.D)`), else the two-field form — callsTo
// counts both, since record splits on the first "(".
func (f *fakeDocker) NetworkConnect(_ context.Context, name, id string, ep *network.EndpointSettings) error {
	if ep != nil && ep.IPAMConfig != nil && ep.IPAMConfig.IPv4Address != "" {
		return f.record(fmt.Sprintf("NetworkConnect(%s,%s,ip=%s)", name, id, ep.IPAMConfig.IPv4Address))
	}
	return f.record(fmt.Sprintf("NetworkConnect(%s,%s)", name, id))
}

func (f *fakeDocker) NetworkDisconnect(_ context.Context, name, id string, _ bool) error {
	return f.record(fmt.Sprintf("NetworkDisconnect(%s,%s)", name, id))
}

func (f *fakeDocker) NetworkList(_ context.Context, _ network.ListOptions) ([]network.Summary, error) {
	if err := f.record("NetworkList()"); err != nil {
		return nil, err
	}
	return nil, nil
}

func (f *fakeDocker) ImageInspect(_ context.Context, id string, _ ...client.ImageInspectOption) (image.InspectResponse, error) {
	if err := f.record("ImageInspect(" + id + ")"); err != nil {
		return image.InspectResponse{}, err
	}
	return image.InspectResponse{ID: f.imageID}, nil
}

func (f *fakeDocker) Ping(_ context.Context) (types.Ping, error) {
	if err := f.record("Ping()"); err != nil {
		return types.Ping{}, err
	}
	return types.Ping{APIVersion: "1.47"}, nil
}

func (f *fakeDocker) Info(_ context.Context) (system.Info, error) {
	if err := f.record("Info()"); err != nil {
		return system.Info{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info, nil
}

var _ dockerAPI = (*fakeDocker)(nil)
