package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/caddyserver/caddy/v2"
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	"github.com/docker/docker/client"
)

// init registers the "caddy ingress" subcommand. Required by the Caddy CLI plugin API —
// this is an approved exception to the "no init()" project convention.
func init() {
	caddycmd.RegisterCommand(caddycmd.Command{
		Name:  "ingress",
		Short: "Run Caddy as a Docker ingress with label-based discovery",
		Long: `Watches Docker for containers with caddy labels and dynamically
generates a Caddyfile with HTTPS, Google OAuth, rate limiting, and role-based
access control. Configuration is driven entirely by container labels.`,
		Func: cmdIngress,
	})
}

func cmdIngress(flags caddycmd.Flags) (int, error) {
	caddy.TrapSignals()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := loadConfig(logger)
	if err != nil {
		logger.Error("configuration error", "error", err)
		return 1, err
	}

	// Start Caddy with minimal config (admin API only)
	adminAddr := getEnvOrDefault(EnvCaddyAdmin, DefaultCaddyAdmin)
	err = caddy.Run(&caddy.Config{
		Admin: &caddy.AdminConfig{
			Listen: "tcp/" + adminAddr,
		},
	})
	if err != nil {
		logger.Error("failed to start caddy", "error", err)
		return 1, err
	}

	// Connect to Docker
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		logger.Error("failed to connect to docker", "error", err)
		return 1, err
	}
	defer dockerClient.Close()

	// Ping Docker to verify connection
	_, err = dockerClient.Ping(context.Background())
	if err != nil {
		logger.Error("failed to ping docker", "error", err)
		return 1, err
	}
	logger.Info("connected to docker")

	// Ensure Cloudflare DNS records are correct
	if cfToken := os.Getenv(EnvCloudflareToken); cfToken != "" {
		if err := syncDNS(cfToken, cfg, logger); err != nil {
			logger.Warn("cloudflare dns sync failed (continuing without it)", "error", err)
		}
	} else {
		logger.Warn("CLOUDFLARE_API_TOKEN not set, skipping DNS sync")
	}

	// Start the watcher
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	watcher := NewWatcher(dockerClient, cfg, logger, adminAddr)
	if err := watcher.Start(ctx); err != nil {
		logger.Error("watcher failed to start", "error", err)
		return 1, err
	}

	logger.Info("caddy ingress running",
		"domain", cfg.BaseDomain,
		"admin_addr", adminAddr,
	)

	// Block until signal
	<-ctx.Done()
	logger.Info("shutting down")
	return 0, nil
}

// loadConfig reads environment variables into a GeneratorConfig.
// BASE_DOMAIN, ADMIN_EMAIL, GOOGLE_CLIENT_ID, and GOOGLE_CLIENT_SECRET
// are required — set them in .env.
func loadConfig(log *slog.Logger) (GeneratorConfig, error) {
	// Validate all required env vars upfront so failures are caught at startup.
	for _, key := range []string{EnvBaseDomain, EnvAdminEmail, EnvGoogleClientID, EnvGoogleClientSecret} {
		if os.Getenv(key) == "" {
			return GeneratorConfig{}, fmt.Errorf("%s is required but not set — add it to your .env file", key)
		}
	}

	baseDomain := os.Getenv(EnvBaseDomain)
	adminEmail := os.Getenv(EnvAdminEmail)

	cfg := GeneratorConfig{
		BaseDomain:        baseDomain,
		AdminEmail:        adminEmail,
		CaddyfileBasePath: os.Getenv(EnvCaddyfileBasePath),
		AuthDomain:        getEnvOrDefault(EnvAuthDomain, "auth."+baseDomain),
		ShellDomain:       getEnvOrDefault(EnvShellDomain, "shell."+baseDomain),
		AppsDomain:        getEnvOrDefault(EnvAppsDomain, baseDomain),
		PreviewDomain:     os.Getenv(EnvPreviewDomain),
		PrivateDomain:     os.Getenv(EnvPrivateDomain),
		IngressToken:      os.Getenv(EnvIngressToken),
		JWTKey:            os.Getenv(EnvJWTKey),
	}

	if superAdmins := os.Getenv(EnvSuperAdmins); superAdmins != "" {
		cfg.SuperAdmins = parseEmailList(superAdmins)
	}

	rps := getEnvOrDefault(EnvDefaultRPS, strconv.Itoa(DefaultRPS))
	parsed, err := strconv.Atoi(rps)
	if err != nil {
		log.Warn("invalid "+EnvDefaultRPS+", using default", "value", rps, "default", DefaultRPS)
		parsed = DefaultRPS
	}
	cfg.DefaultRPS = parsed

	log.Info("config loaded",
		"base_domain", cfg.BaseDomain,
		"auth_domain", cfg.AuthDomain,
		"shell_domain", cfg.ShellDomain,
		"apps_domain", cfg.AppsDomain,
		"admin_email", cfg.AdminEmail,
		"super_admins", strings.Join(cfg.SuperAdmins, ","),
		"default_rps", cfg.DefaultRPS,
	)

	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// syncDNS detects the server's public IP and ensures the required Cloudflare
// DNS records (auth A, shell A, *.apps wildcard) are pointing to it.
func syncDNS(token string, cfg GeneratorConfig, logger *slog.Logger) error {
	ip, err := DetectPublicIP()
	if err != nil {
		return fmt.Errorf("detecting public IP: %w", err)
	}
	logger.Info("detected public ip", "ip", ip)

	cf, err := NewCloudflareClient(token, cfg.BaseDomain, logger)
	if err != nil {
		return fmt.Errorf("creating cloudflare client: %w", err)
	}

	return cf.EnsureDNS(ip, cfg.AuthDomain, cfg.ShellDomain, cfg.AppsDomain, cfg.PreviewDomain, cfg.PrivateDomain)
}
