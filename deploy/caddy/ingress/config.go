package ingress

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// config.go — Single source of truth for all caddy-ingress constants,
// defaults, and env var keys. Values are grouped by lifecycle:
//   - Static:  baked into binary, never changes
//   - Default: sensible fallback, overridable via env var
//   - Dynamic: env var keys (required or optional)

// ─── STATIC: Baked into binary, never changes ─────────────────────────

// Docker label keys used by container owners to configure ingress behavior.
const (
	LabelCaddy       = "caddy"
	LabelPort        = "caddy_port"
	LabelHostname    = "caddy_hostname"
	LabelPublic      = "caddy_public"
	LabelAdmins      = "caddy_admins"
	LabelViewers     = "caddy_viewers"
	LabelSuperAdmins = "caddy_superadmins"
	LabelRPS         = "caddy_rps"
	// LabelPreviewTarget marks the container that backs the public preview
	// wildcard (*.PREVIEW_DOMAIN). Only the styrman server sets it.
	LabelPreviewTarget = "caddy_preview_target"
)

// Clean identity headers injected into downstream requests.
const (
	HeaderEmail   = "X-Caddy-Email"
	HeaderUser    = "X-Caddy-User"
	HeaderRole    = "X-Caddy-Role"
	HeaderSubject = "X-Caddy-Subject"
	// HeaderIngress proves to the upstream that a request came through this
	// ingress (not from a container on a shared bridge spoofing X-Caddy-Email
	// directly). Set to the shared STYRMAN_INGRESS_TOKEN when present.
	HeaderIngress = "X-Styrman-Ingress"
)

// caddy-security upstream headers (stripped before reaching apps).
const (
	UpstreamHeaderEmail = "X-Token-User-Email"
	UpstreamHeaderName  = "X-Token-User-Name"
	UpstreamHeaderRoles = "X-Token-User-Roles"
	UpstreamHeaderSub   = "X-Token-Subject"
)

// CookiePrefix is this deployment's caddy-security cookie prefix: "AUTHP"
// plus the first 8 upper-case hex digits of sha256(base domain). The
// plugin's default prefix is the same "AUTHP" everywhere, so a browser
// holding a session for a deployment on a PARENT domain sent both
// deployments' same-named cookies to the child, and the child's authorizer
// found no valid one: an endless sign-in redirect (BUG-074). The portal
// names every cookie <prefix>_<NAME> (session id, access token, ...).
func CookiePrefix(baseDomain string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSuffix(baseDomain, "."))))
	return "AUTHP" + strings.ToUpper(hex.EncodeToString(sum[:4]))
}

// AuthCookieName is this deployment's session (access token) cookie, the
// one every authorization policy reads.
func AuthCookieName(baseDomain string) string { return CookiePrefix(baseDomain) + "_ACCESS_TOKEN" }

// PortalCookieStrip matches EVERY deployment's portal cookies, the legacy
// unprefixed AUTHP_* names included: the public preview and public-app
// blocks cut them all out before tunnelling into a guest app. Stripping only
// this deployment's name would hand a guest app the legacy cookie (valid for
// a session lifetime after the rename, signed with the same pinned key) and
// a parent-domain deployment's cookie, either replayable under this
// deployment's derivable name. Raw: Caddy compiles it with Go's regexp.
const PortalCookieStrip = `(?:^|;\s*)AUTHP[0-9A-F]*_[A-Z_]+=[^;]*`

// Role strings used in X-Caddy-Role header values.
const (
	RolePublic     = "public"
	RoleUser       = "user"
	RoleAdmin      = "admin"
	RoleViewer     = "viewer"
	RoleSuperAdmin = "superadmin"
)

// ─── DEFAULTS: Sensible fallback, overridable via env var ──────────────

const (
	// DefaultRPS is the per-client-IP rate limit when caddy_rps label is unset.
	// Override via DEFAULT_RPS env var.
	DefaultRPS = 100

	// DefaultCaddyAdmin is the Caddy admin API listen address. An IP literal,
	// not "localhost": isLoopbackAddr no longer accepts a NAME, because a name
	// is resolved by the OS at bind time and so cannot be verified here
	// (2026-09-01 review; changed 2026-09-02). Override via CADDY_ADMIN — the
	// override must also be a loopback IP literal with a port.
	DefaultCaddyAdmin = "127.0.0.1:2019"
)

// ─── DYNAMIC: Env var keys ─────────────────────────────────────────────

// Required env vars — program exits with a clear error if unset.
const (
	EnvBaseDomain         = "BASE_DOMAIN"
	EnvAdminEmail         = "ADMIN_EMAIL"
	EnvGoogleClientID     = "GOOGLE_CLIENT_ID"
	EnvGoogleClientSecret = "GOOGLE_CLIENT_SECRET"
)

// Optional env vars — used if present, graceful if absent.
const (
	EnvSuperAdmins       = "SUPERADMINS" // Platform superadmins — get superadmin role on ALL apps + shell access
	EnvDefaultRPS        = "DEFAULT_RPS"
	EnvCaddyAdmin        = "CADDY_ADMIN"
	EnvCaddyfileBasePath = "CADDYFILE_BASE_PATH"
	EnvCloudflareToken   = "CLOUDFLARE_API_TOKEN"
	EnvAuthDomain        = "AUTH_DOMAIN"           // Absolute FQDN for auth portal (default: auth.<BASE_DOMAIN>)
	EnvShellDomain       = "SHELL_DOMAIN"          // Absolute FQDN for shell (default: shell.<BASE_DOMAIN>)
	EnvAppsDomain        = "APPS_DOMAIN"           // Wildcard base for user apps (default: <BASE_DOMAIN>)
	EnvPreviewDomain     = "PREVIEW_DOMAIN"        // Public preview wildcard base, e.g. public.<app domain> (empty = off)
	EnvPrivateDomain     = "PRIVATE_DOMAIN"        // Email-gated preview wildcard base, e.g. private.<app domain> (empty = off)
	EnvIngressToken      = "STYRMAN_INGRESS_TOKEN" // shared secret injected as X-Styrman-Ingress on authenticated routes
	EnvJWTKey            = "STYRMAN_JWT_KEY"       // stable HMAC key for caddy-security JWTs (survives restarts)
)

// IPDetectionURLs are the services used to detect the server's public IP.
// Order matters — we try each in sequence and return the first valid IPv4 response.
var IPDetectionURLs = []string{
	"https://ifconfig.me/ip",
	"https://api.ipify.org",
	"https://icanhazip.com",
}

// Infrastructure hostnames — reserved for the styrman stack.
// The generator uses these for absolute FQDN routing.
// User containers with these hostnames are rejected during label parsing.
const (
	ShellHostname = "shell"
	AuthHostname  = "auth"
)

// ReservedHostnames are caddy_hostname values that cannot be used by
// user containers. They are reserved for styrman infrastructure.
var ReservedHostnames = map[string]bool{
	ShellHostname: true,
	AuthHostname:  true,
}
