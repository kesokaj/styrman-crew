package ingress

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

// JWT cookie name set by caddy-security.
// AuthCookieName is the portal's session cookie; the public preview block
// strips exactly this cookie before tunnelling into a guest app.
const AuthCookieName = "AUTHP_ACCESS_TOKEN"

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
