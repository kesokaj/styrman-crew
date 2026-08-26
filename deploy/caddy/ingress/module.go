// Package ingress implements a custom Caddy module that watches Docker
// containers for labels and dynamically generates a Caddyfile with HTTPS,
// Google OAuth, per-app rate limiting, and role-based access control.
package ingress

import "github.com/caddyserver/caddy/v2"

// init registers the CaddyIngress module. Required by the Caddy plugin API —
// this is an approved exception to the "no init()" project convention.
func init() {
	caddy.RegisterModule(CaddyIngress{})
}

// CaddyIngress is the Caddy module that provides Docker-based ingress.
type CaddyIngress struct{}

// CaddyModule returns the Caddy module information.
func (CaddyIngress) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "ingress",
		New: func() caddy.Module { return new(CaddyIngress) },
	}
}
