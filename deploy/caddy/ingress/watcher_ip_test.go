package ingress

import (
	"testing"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	networktypes "github.com/docker/docker/api/types/network"
)

func inspectWithNetworks(nets map[string]string) types.ContainerJSON {
	m := make(map[string]*networktypes.EndpointSettings, len(nets))
	for name, ip := range nets {
		m[name] = &networktypes.EndpointSettings{IPAddress: ip}
	}
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: &containertypes.HostConfig{},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: m,
		},
	}
}

func TestExtractContainerIP(t *testing.T) {
	// Host-network container → Caddy shares the host stack.
	host := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: &containertypes.HostConfig{NetworkMode: "host"},
		},
		NetworkSettings: &types.NetworkSettings{},
	}
	if got := extractContainerIP(host); got != "127.0.0.1" {
		t.Fatalf("host network: got %q, want 127.0.0.1", got)
	}

	// The styrman server is on the ingress network AND project bridges (sdb-*).
	// Map order is random, so run it enough times to catch a bad pick — the
	// ingress IP must win every time (project-bridge IPs are unreachable from
	// Caddy and were causing 502s on crew.styrman.xyz).
	server := map[string]string{
		"sdb-45b71295abc": "172.19.255.254",
		"sdb-36782cc5def": "172.20.255.254",
		"styrman-ingress": "172.18.0.2",
	}
	for i := 0; i < 100; i++ {
		if got := extractContainerIP(inspectWithNetworks(server)); got != "172.18.0.2" {
			t.Fatalf("server on multiple nets: got %q, want ingress IP 172.18.0.2", got)
		}
	}

	// A preview VM lives only on its own project bridge — that IP is correct.
	preview := map[string]string{"sdb-45b71295abc": "172.19.0.5"}
	if got := extractContainerIP(inspectWithNetworks(preview)); got != "172.19.0.5" {
		t.Fatalf("preview VM: got %q, want 172.19.0.5", got)
	}

	// No usable network → empty.
	empty := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{HostConfig: &containertypes.HostConfig{}},
		NetworkSettings:   &types.NetworkSettings{},
	}
	if got := extractContainerIP(empty); got != "" {
		t.Fatalf("no networks: got %q, want empty", got)
	}
}
