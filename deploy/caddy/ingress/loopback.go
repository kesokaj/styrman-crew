package ingress

import (
	"net"
	"strconv"
	"strings"
)

// isLoopbackAddr reports whether a "host:port" binds loopback only. An endpoint
// that can reconfigure the ingress must never be reachable off-host. A port is
// REQUIRED: a bare host ("127.0.0.1") would make Caddy bind :80 while silently
// passing a host-only check (review finding), so anything without host:port is
// rejected — as is ":2019" (every interface) and any non-loopback host.
func isLoopbackAddr(addr string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil || port == "" {
		return false // must be host:port
	}
	// SplitHostPort does NOT validate the port. This value is emitted verbatim
	// into the generated Caddyfile (admin/origins) and used as the reload URL,
	// so a non-numeric "port" (whitespace, braces, a second directive) must
	// never pass the guard — and only the CANONICAL decimal form is accepted
	// ("+2019"/"07" would bind one thing and print another; security review).
	n, perr := strconv.ParseUint(port, 10, 16)
	if perr != nil || n < 1 || port != strconv.FormatUint(n, 10) {
		return false
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false // ":2019" binds every interface
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
