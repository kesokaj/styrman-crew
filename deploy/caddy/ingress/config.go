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
)

// Clean identity headers injected into downstream requests.
const (
	HeaderEmail   = "X-Caddy-Email"
	HeaderUser    = "X-Caddy-User"
	HeaderRole    = "X-Caddy-Role"
	HeaderSubject = "X-Caddy-Subject"
	HeaderPicture = "X-Caddy-Picture"
)

// caddy-security upstream headers (stripped before reaching apps).
const (
	UpstreamHeaderEmail = "X-Token-User-Email"
	UpstreamHeaderName  = "X-Token-User-Name"
	UpstreamHeaderRoles = "X-Token-User-Roles"
	UpstreamHeaderSub   = "X-Token-Subject"
)

// JWT cookie name set by caddy-security.
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

	// DefaultCaddyAdmin is the Caddy admin API listen address.
	// Override via CADDY_ADMIN env var.
	DefaultCaddyAdmin = "localhost:2019"
)

// ─── DYNAMIC: Env var keys ─────────────────────────────────────────────

// Required env vars — program exits with a clear error if unset.
const (
	EnvBaseDomain          = "BASE_DOMAIN"
	EnvAdminEmail          = "ADMIN_EMAIL"
	EnvGoogleClientID      = "GOOGLE_CLIENT_ID"
	EnvGoogleClientSecret  = "GOOGLE_CLIENT_SECRET"
)

// Optional env vars — used if present, graceful if absent.
const (
	EnvSuperAdmins       = "SUPERADMINS" // Platform superadmins — get superadmin role on ALL apps + shell access
	EnvDefaultRPS        = "DEFAULT_RPS"
	EnvCaddyAdmin        = "CADDY_ADMIN"
	EnvCaddyfileBasePath = "CADDYFILE_BASE_PATH"
	EnvCloudflareToken   = "CLOUDFLARE_API_TOKEN"
	EnvAuthDomain        = "AUTH_DOMAIN"  // Absolute FQDN for auth portal (default: auth.<BASE_DOMAIN>)
	EnvShellDomain       = "SHELL_DOMAIN"   // Absolute FQDN for shell (default: shell.<BASE_DOMAIN>)
	EnvAppsDomain        = "APPS_DOMAIN"    // Wildcard base for user apps (default: <BASE_DOMAIN>)
)

// IPDetectionURLs are the services used to detect the server's public IP.
// Order matters — we try each in sequence and return the first valid IPv4 response.
var IPDetectionURLs = []string{
	"https://ifconfig.me/ip",
	"https://api.ipify.org",
	"https://icanhazip.com",
}

// Infrastructure hostnames — reserved for the devbox stack.
// The generator uses these for absolute FQDN routing.
// User containers with these hostnames are rejected during label parsing.
const (
	ShellHostname = "shell"
	AuthHostname  = "auth"
)

// ReservedHostnames are caddy_hostname values that cannot be used by
// user containers. They are reserved for devbox infrastructure.
var ReservedHostnames = map[string]bool{
	ShellHostname: true,
	AuthHostname:  true,
}
