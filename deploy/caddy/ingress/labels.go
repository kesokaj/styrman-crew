package ingress

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AccessMode determines how authentication is handled for an app.
type AccessMode int

const (
	// ModePublic means no authentication required.
	ModePublic AccessMode = iota
	// ModeAuthenticated means any Google user can access.
	ModeAuthenticated
	// ModeRoleGated means only listed emails can access with assigned roles.
	ModeRoleGated
)

// App represents a discovered Docker container that should be proxied.
type App struct {
	// Name is the container name or service name (used for logging/zone names).
	Name string
	// Hostname is the subdomain (e.g., "grafana" → grafana.{BASE_DOMAIN}).
	Hostname string
	// ContainerIP is the bridge IP of the container (e.g., "172.18.0.2").
	ContainerIP string
	// Port is the container port to proxy to.
	Port int
	// Mode is the access mode for this app.
	Mode AccessMode
	// Admins is the list of admin email addresses (ModeRoleGated only).
	Admins []string
	// Viewers is the list of viewer email addresses (ModeRoleGated only).
	Viewers []string
	// SuperAdmins is the list of superadmin email addresses (ModeRoleGated only).
	SuperAdmins []string
	// RPS is the rate limit in requests per second per client IP.
	RPS int
	// PreviewTarget marks this app as the backend for the public preview
	// wildcard (*.PREVIEW_DOMAIN). Only the styrman server sets it.
	PreviewTarget bool
}

// ParseLabels extracts an App from a Docker container's labels.
// containerName is the container name (without leading slash).
// exposedPorts is the list of EXPOSE ports from the image (used as fallback).
// defaultRPS is the default rate limit if caddy_rps is not set.
// Returns nil if the container should not be proxied (caddy != "true").
func ParseLabels(labels map[string]string, containerName string, exposedPorts []int, defaultRPS int) (*App, error) {
	if !isTrue(labels[LabelCaddy]) {
		return nil, nil
	}

	app := &App{
		Name: containerName,
		RPS:  defaultRPS,
	}

	// Hostname: label override or container name — sanitize BOTH so a hostname
	// can never inject into the generated Caddyfile.
	if h := strings.TrimSpace(labels[LabelHostname]); h != "" {
		app.Hostname = sanitizeHostname(h)
	} else {
		app.Hostname = sanitizeHostname(containerName)
	}

	// Block reserved hostnames for non-infrastructure containers.
	// Only styrman-* containers (infrastructure) may use reserved hostnames.
	if ReservedHostnames[app.Hostname] && !strings.HasPrefix(containerName, "styrman-") {
		return nil, fmt.Errorf("hostname %q is reserved for styrman infrastructure", app.Hostname)
	}

	// Port: label > EXPOSE > error
	if p := strings.TrimSpace(labels[LabelPort]); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("parsing caddy_port %q: %w", p, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("caddy_port %d out of range (1-65535)", port)
		}
		app.Port = port
	} else if len(exposedPorts) == 1 {
		app.Port = exposedPorts[0]
	} else if len(exposedPorts) > 1 {
		return nil, fmt.Errorf("container %q has multiple EXPOSE ports %v, set caddy_port label", containerName, exposedPorts)
	} else {
		return nil, fmt.Errorf("container %q has no EXPOSE ports and no caddy_port label", containerName)
	}

	// Access mode
	if isTrue(labels[LabelPublic]) {
		app.Mode = ModePublic
	} else {
		admins := parseEmailList(labels[LabelAdmins])
		viewers := parseEmailList(labels[LabelViewers])
		superAdmins := parseEmailList(labels[LabelSuperAdmins])
		if len(admins) > 0 || len(viewers) > 0 || len(superAdmins) > 0 {
			app.Mode = ModeRoleGated
			app.Admins = admins
			app.Viewers = viewers
			app.SuperAdmins = superAdmins
		} else {
			app.Mode = ModeAuthenticated
		}
	}

	app.PreviewTarget = isTrue(labels[LabelPreviewTarget])

	// Rate limit
	if r := strings.TrimSpace(labels[LabelRPS]); r != "" {
		rps, err := strconv.Atoi(r)
		if err != nil {
			return nil, fmt.Errorf("parsing caddy_rps %q: %w", r, err)
		}
		if rps < 1 {
			return nil, fmt.Errorf("caddy_rps must be >= 1, got %d", rps)
		}
		app.RPS = rps
	}

	return app, nil
}

// emailRe is the shape an entry must have to reach the generated Caddyfile.
// Deliberately strict: no whitespace, no braces, no newlines — the bytes that
// would end a `transform user` block early and let the rest of the value be
// read as Caddyfile directives.
var emailRe = regexp.MustCompile(`^[^@\s{}"'#]+@[^@\s{}"'#]+\.[^@\s{}"'#]+$`)

// parseEmailList splits a comma-separated email string into a deduplicated,
// trimmed slice. Empty strings are filtered out, and so is anything that is
// not email-shaped.
//
// The validation is the point. These values come from container labels and
// from SUPERADMINS, and they are written VERBATIM into the generated Caddyfile
// as `match email <value>`. sanitizeHostname (below) strips
// Caddyfile-significant bytes from the hostname for exactly this reason and
// says so — "defense in depth, even though labels are set by the server" — and
// the email path had no equivalent (audit 2026-09-08). Both sources are
// operator-controlled today; this is the same defence-in-depth the hostname
// already gets, in an internet-facing process that holds the docker socket.
func parseEmailList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	seen := make(map[string]bool, len(parts))
	var result []string
	for _, p := range parts {
		email := strings.TrimSpace(p)
		if email == "" || seen[email] || !emailRe.MatchString(email) {
			continue
		}
		seen[email] = true
		result = append(result, email)
	}
	return result
}

// isTrue checks if a string value represents true.
func isTrue(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "true" || s == "yes" || s == "1"
}

// sanitizeHostname converts a container name into a valid subdomain.
// Strips leading slashes (Docker adds them) and replaces underscores with hyphens.
func sanitizeHostname(name string) string {
	name = strings.TrimLeft(name, "/")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ToLower(name)
	// Keep only DNS-label-safe characters so a hostname value can never carry
	// Caddyfile-significant bytes (spaces, braces, newlines) into the generated
	// config — defense in depth, even though labels are set by the server.
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			return r
		}
		return -1
	}, name)
}
