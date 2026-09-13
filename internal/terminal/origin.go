package terminal

import (
	"net/http"
	"net/url"
	"strings"
)

// SameOriginOrAbsent is the Origin check every WebSocket entry point applies
// BEFORE any side effect (a VM boot, a docker exec, a guest dial):
// websocket.Accept does the same check, but only at the upgrade — by which
// time ServeHTTP and ServeSetup used to have booted a microVM on a forged
// cross-origin GET (audit 2026-09-13). A browser always sends Origin on a
// WebSocket handshake; a non-browser client may omit it. Shared with the
// browser relay in httpapi, which cannot be imported from here.
func SameOriginOrAbsent(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}
