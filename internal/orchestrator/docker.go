package orchestrator

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// dockerAPI is the orchestrator's view of the Docker Engine client: exactly
// the methods orchestrator code calls, with client.Client's own signatures
// (docker v28), so *client.Client satisfies it unchanged and a fake can
// stand in for tests. Same shape as internal/httpapi's guestRuntime /
// modeRuntime seams — a caller-scoped subset, not a mirror of the SDK.
// Generated from `grep -oh 'o\.docker\.[A-Za-z]*' internal/orchestrator/*.go`
// on 2026-09-10 (19 methods; BUG-003's residual, plan
// .agents/docs/plans/2026-09-10-todo-closeout.md item O); ContainerUpdate
// added 2026-09-16 (BUG-063: the restart policy is switched off for a Stop
// and back on for a Start).
type dockerAPI interface {
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerUpdate(ctx context.Context, containerID string, updateConfig container.UpdateConfig) (container.UpdateResponse, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerExecCreate(ctx context.Context, containerID string, options container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)
	ContainerExecResize(ctx context.Context, execID string, options container.ResizeOptions) error
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkInspect(ctx context.Context, networkID string, options network.InspectOptions) (network.Inspect, error)
	NetworkRemove(ctx context.Context, networkID string) error
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
	NetworkDisconnect(ctx context.Context, networkID, containerID string, force bool) error
	NetworkList(ctx context.Context, options network.ListOptions) ([]network.Summary, error)
	ImageInspect(ctx context.Context, imageID string, inspectOpts ...client.ImageInspectOption) (image.InspectResponse, error)
	Ping(ctx context.Context) (types.Ping, error)
}

// Compile-time proof that the real client satisfies the seam.
var _ dockerAPI = (*client.Client)(nil)
