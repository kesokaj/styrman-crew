package config

import (
	"fmt"
	"os"
	"strings"
)

// IPForwardProc is the kernel's IPv4 forwarding switch as the server
// container sees it.
const IPForwardProc = "/proc/sys/net/ipv4/ip_forward"

// IPForwardOff reports whether IPv4 forwarding is off in the network
// namespace this process runs in, reading the sysctl at procPath (the
// IPForwardProc constant in production; a test hands it a file).
//
// Why the server cares: it joins every project bridge it serves (the
// port-preview proxy, the credential broker at each bridge's fixed
// address), so with forwarding on its netns could route between two
// project bridges and void the per-project isolation R12 promises. The
// compose deploy sets `net.ipv4.ip_forward=0` on the server container;
// nothing asserted it at runtime until v1.33.0 (security review of the
// BUG-060 fix, 2026-09-14). Only a containerized server is checked: a
// host-mode dev server shares the host's netns, where Docker itself needs
// forwarding on, and it never attaches to a bridge at all.
func IPForwardOff(procPath string) (bool, error) {
	b, err := os.ReadFile(procPath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", procPath, err)
	}
	return strings.TrimSpace(string(b)) == "0", nil
}

// IPForwardMustRefuse reports whether a CONTAINERIZED server must refuse to
// start, given what the sysctl read returned and whether the operator set the
// override. It exists as its own function because the rule is a security
// decision and main() is not reachable from a test.
//
// Any error refuses, including fs.ErrNotExist. Until 2026-09-22 a missing
// file warned and continued, on the reasoning that an exotic kernel should
// not take the service down; that made the one guard standing behind R12
// report "fine" in the single case where it had measured nothing — the shape
// of BUG-014/028/029/031. Inside a Linux container that file is effectively
// always present, so its absence is a reason to look, not to proceed. The
// operator keeps a documented way through: STYRMAN_ALLOW_IP_FORWARD=1.
func IPForwardMustRefuse(off bool, err error, override bool) bool {
	if override {
		return false
	}
	if err != nil {
		return true
	}
	return !off
}
