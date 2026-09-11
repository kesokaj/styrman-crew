package orchestrator

import (
	"context"
	"net"

	"github.com/docker/docker/api/types/network"

	"github.com/kesokaj/styrman-crew/internal/config"
)

// BrowserPeer is this server's OWN address on the project's bridge — the one
// source the guest's DevTools forwarder admits (ADR-068 amendment): the
// launcher receives it as STYRMAN_BROWSER_PEER on every `start` and starts
// no forwarder without it, so an unknown peer fails CLOSED rather than
// falling back to a guess about who the gateway is (security review,
// 2026-09-11).
//
// A containerized server reads its live address on that network from its
// own container (the broker's fixed address when the static attach held, the
// dynamic one otherwise — either way the address the relay really dials
// from); a host-run dev server reaches the bridge as its gateway, which the
// network's IPAM config names. "" when neither is known.
func (o *Orchestrator) BrowserPeer(ctx context.Context, project string) string {
	netName := config.NetworkName(project)
	if o.cfg.SelfContainer != "" {
		// The LIVE address only: naming the broker's intended static address
		// when the inspect fails could admit an address this server does not
		// hold (security review, round 3) — an unknown peer fails closed.
		insp, err := o.docker.ContainerInspect(ctx, o.cfg.SelfContainer)
		if err != nil || insp.NetworkSettings == nil {
			return ""
		}
		if ep, ok := insp.NetworkSettings.Networks[netName]; ok && net.ParseIP(ep.IPAddress) != nil {
			return ep.IPAddress
		}
		return ""
	}
	insp, err := o.docker.NetworkInspect(ctx, netName, network.InspectOptions{})
	if err != nil {
		return ""
	}
	for _, c := range insp.IPAM.Config {
		if ip := net.ParseIP(c.Gateway); ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}
