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
