// Command styrman is the single server binary: embedded UI, REST/WS API, and
// the Docker orchestrator for per-project Kata microVMs.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kesokaj/styrman/internal/config"
	"github.com/kesokaj/styrman/internal/gitrepo"
	"github.com/kesokaj/styrman/internal/httpapi"
	"github.com/kesokaj/styrman/internal/orchestrator"
	"github.com/kesokaj/styrman/internal/project"
	"github.com/kesokaj/styrman/internal/userstore"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	for _, dir := range []string{cfg.StateDir, cfg.HomesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("state dir unavailable — run deploy/host-setup.sh", "dir", dir, "error", err)
			os.Exit(1)
		}
	}

	migrateLegacy(cfg, logger)
	ensureRepos(cfg, logger)

	users, err := userstore.New(filepath.Join(cfg.StateDir, "users.json"), cfg.AllowedEmails)
	if err != nil {
		logger.Error("userstore load failed", "error", err)
		os.Exit(1)
	}
	if cfg.AuthMode == "header" && len(users.List()) == 0 {
		logger.Error("STYRMAN_AUTH=header but no authorized users — set STYRMAN_ALLOWED_EMAILS")
		os.Exit(1)
	}

	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		logger.Error("orchestrator init failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Adopt pre-existing containers so restarts are seamless (ADR-007), and
	// join running projects' networks now (containerized server: does the
	// network attach + guest ARP refresh at boot, not on first click).
	// Non-blocking: force-removing a running Kata VM can take seconds and
	// must not delay the listener.
	go orch.CleanupLegacy(ctx)
	if adopted, err := orch.Adopt(ctx); err != nil {
		logger.Warn("initial adoption failed (docker down?)", "error", err)
	} else {
		logger.Info("adopted state", "containers", len(adopted))
		for _, st := range adopted {
			if st.State == "running" {
				orch.EnsureSelfAttached(ctx, st.User, st.Project)
			}
		}
	}

	go orch.RunIdleReaper(ctx)

	api := httpapi.New(cfg, users, orch, logger)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// Optional auth-bypassed dev listener; MUST only ever be published on
	// host loopback (see config.DevAddr).
	if cfg.DevAddr != "" {
		devSrv := &http.Server{
			Addr:              cfg.DevAddr,
			Handler:           api.RoutesWithAuth("none"),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			logger.Info("dev listener (NO AUTH) up", "addr", cfg.DevAddr)
			if err := devSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("dev listener failed", "error", err)
			}
		}()
		go func() { <-ctx.Done(); _ = devSrv.Close() }()
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	logger.Info("styrman listening", "addr", cfg.Addr, "auth", cfg.AuthMode, "image", cfg.Image,
		"runtime", cfg.Runtime, "homes", cfg.HomesDir, "version", httpapi.Version)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// migrateLegacy moves the pre-multitenancy layout (one shared home at
// StateDir/home, flat projects.json/conversations.json, containers named
// styrman-<project>) into the default user's namespace. Idempotent: each
// step only fires if the legacy artifact exists.
func migrateLegacy(cfg *config.Config, logger *slog.Logger) {
	_, user := cfg.DefaultUser()

	// 1. Home dir.
	if st, err := os.Stat(cfg.HomeDir); err == nil && st.IsDir() {
		target := cfg.UserHome(user)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			if err := os.Rename(cfg.HomeDir, target); err != nil {
				logger.Error("legacy home migration failed", "from", cfg.HomeDir, "to", target, "error", err)
			} else {
				logger.Info("migrated legacy home", "to", target)
			}
		}
	}

	// 2. Metadata stores.
	userDir := cfg.UserStateDir(user)
	for _, f := range []string{"projects.json", "conversations.json"} {
		legacy := filepath.Join(cfg.StateDir, f)
		if _, err := os.Stat(legacy); err == nil {
			_ = os.MkdirAll(userDir, 0o755)
			target := filepath.Join(userDir, f)
			if _, terr := os.Stat(target); os.IsNotExist(terr) {
				if err := os.Rename(legacy, target); err != nil {
					logger.Error("legacy store migration failed", "file", f, "error", err)
					continue
				}
				logger.Info("migrated legacy store", "file", f, "user", user)
			} else {
				_ = os.Rename(legacy, legacy+".migrated")
			}
		}
	}

	// 3. Previous-scheme containers are removed via the Docker SDK after the
	// orchestrator is up (CleanupLegacy) — the containerized server has no
	// docker CLI.
}

// ensureRepos upholds the R13 invariant on every boot: any project
// workspace without a git repository (pre-R13 projects, or dirs restored
// from backup) gets one initialized in place. Never touches existing repos.
func ensureRepos(cfg *config.Config, logger *slog.Logger) {
	usersDir := filepath.Join(cfg.StateDir, "users")
	des, err := os.ReadDir(usersDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("R13 sweep: users dir unreadable", "dir", usersDir, "error", err)
		}
		return // no users yet — creation handles new projects
	}
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		user := de.Name()
		store, err := project.NewStore(filepath.Join(cfg.UserStateDir(user), "projects.json"))
		if err != nil {
			logger.Warn("R13 sweep: project store unreadable", "user", user, "error", err)
			continue
		}
		for _, p := range store.List() {
			ws := filepath.Join(cfg.UserHome(user), "projects", p.Dir)
			if _, err := os.Stat(ws); err != nil {
				continue // workspace missing — nothing to init
			}
			if gitrepo.IsRepo(ws) {
				continue
			}
			if err := gitrepo.EnsureRepo(ws); err != nil {
				logger.Warn("R13 sweep: git init failed", "user", user, "project", p.ID, "error", err)
			} else {
				logger.Info("R13 sweep: initialized git repo", "user", user, "project", p.Name)
			}
		}
	}
}
