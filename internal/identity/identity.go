// Package identity maps authenticated emails to per-user usernames and home
// directories (multi-tenancy: every account gets its own home disk; projects
// are namespaced per user).
package identity

import (
	"context"
	"net/http"
	"strings"
)

// DeriveUsername converts an email to a Linux-safe username slug, e.g.
// "ostling.simon@gmail.com" → "ostling-simon". Ported from devbox.
func DeriveUsername(email string) string {
	name, _, _ := strings.Cut(email, "@")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		default:
			return -1
		}
	}, name)
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "user-" + name
	}
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

type ctxKey struct{}

// User identifies the requester.
type User struct {
	Email    string // "" in auth-none mode with no default email
	Username string // home-dir + in-guest username slug
	Admin    bool
	// Authorized is true for a real styrman user (loopback dev, or an email in
	// the userstore). It is FALSE for a preview-only guest — a Google account
	// that authenticated at the ingress but is not a styrman user and may reach
	// ONLY the port previews shared with its email. Never treat the derived
	// Username as an ownership claim for an unauthorized user (a slug collision
	// would otherwise grant a guest an owner's projects).
	Authorized bool
}

// WithUser stores the user on the request context.
func WithUser(r *http.Request, u User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
}

// FromContext returns the request's user. The auth middleware always sets it.
func FromContext(ctx context.Context) User {
	u, _ := ctx.Value(ctxKey{}).(User)
	return u
}
