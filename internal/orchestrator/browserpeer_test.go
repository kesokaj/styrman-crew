package orchestrator

import (
	"context"
	"log/slog"
	"testing"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// BrowserPeer names the ONE address the guest's DevTools forwarder admits
// (ADR-068 amendment): the server's own address on the project bridge. A
// wrong answer fails closed (the relay is refused), a missing one starts no
// forwarder — so both branches are pinned here against the fake Docker
// client rather than left to the httpapi stub.
func TestBrowserPeer(t *testing.T) {
	const project = "11111111-2222-4333-8444-555555555555"
	netName := config.NetworkName(project)

	t.Run("host-run server is the bridge gateway", func(t *testing.T) {
		fd := newFakeDocker()
		fd.networks[netName] = &fakeNetwork{name: netName, bridge: "sdb-x", gateway: "172.16.5.1"}
		o := &Orchestrator{docker: fd, cfg: &config.Config{}, logger: slog.New(slog.DiscardHandler)}
		if got := o.BrowserPeer(context.Background(), project); got != "172.16.5.1" {
			t.Fatalf("peer = %q, want the gateway", got)
		}
	})

	t.Run("containerized server reads its own live address on that network", func(t *testing.T) {
		fd := newFakeDocker()
		fd.networks[netName] = &fakeNetwork{name: netName, bridge: "sdb-x", gateway: "172.16.5.1"}
		fd.containers["styrman-server"] = &fakeContainer{
			id: "self", name: "styrman-server", running: true,
			ips: map[string]string{netName: "172.16.5.254", "other": "10.0.0.9"},
		}
		o := &Orchestrator{docker: fd, cfg: &config.Config{SelfContainer: "styrman-server"}, logger: slog.New(slog.DiscardHandler)}
		if got := o.BrowserPeer(context.Background(), project); got != "172.16.5.254" {
			t.Fatalf("peer = %q, want the server's address on the project bridge, never the gateway", got)
		}
	})

	t.Run("containerized server whose own inspect fails or lacks the network yields nothing", func(t *testing.T) {
		fd := newFakeDocker()
		fd.networks[netName] = &fakeNetwork{name: netName, bridge: "sdb-x", gateway: "172.16.5.1"}
		o := &Orchestrator{docker: fd, cfg: &config.Config{SelfContainer: "styrman-server", BrokerAddr: ":8091"}, logger: slog.New(slog.DiscardHandler)}
		if got := o.BrowserPeer(context.Background(), project); got != "" {
			t.Fatalf("peer = %q with no self container, want empty (never the broker's intended address)", got)
		}
		fd.containers["styrman-server"] = &fakeContainer{id: "self", name: "styrman-server", running: true, ips: map[string]string{"other": "10.0.0.9"}}
		if got := o.BrowserPeer(context.Background(), project); got != "" {
			t.Fatalf("peer = %q when not on the project network, want empty", got)
		}
	})

	t.Run("unknown network or absent address yields nothing (fail closed)", func(t *testing.T) {
		fd := newFakeDocker()
		o := &Orchestrator{docker: fd, cfg: &config.Config{}, logger: slog.New(slog.DiscardHandler)}
		if got := o.BrowserPeer(context.Background(), project); got != "" {
			t.Fatalf("peer = %q for a missing network, want empty", got)
		}
		fd.networks[netName] = &fakeNetwork{name: netName, bridge: "sdb-x"} // no gateway
		if got := o.BrowserPeer(context.Background(), project); got != "" {
			t.Fatalf("peer = %q for a network without a gateway, want empty", got)
		}
	})
}
