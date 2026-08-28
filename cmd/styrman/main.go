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

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/files"
	"github.com/kesokaj/styrman-crew/internal/gitrepo"
	"github.com/kesokaj/styrman-crew/internal/httpapi"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/userstore"
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

	// ADR-027 migration AFTER the orchestrator exists: project microVMs keep
	// running across server upgrades (restart: unless-stopped), so a project
	// being migrated must be stopped first — renaming a workspace out from
	// under a live VM would strand agents mid-write.
	migrateWorkspaces(cfg, orch, logger)
	ensureRepos(cfg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Boot recovery MUST NOT delay the listener: after a host reboot a slow
	// dockerd (10 Kata VMs restarting) made /api/health hang for minutes,
	// which the ingress reads as 502. Adopt (state recovery, ADR-007) and the
	// self-attach loop (join running projects' bridges for port-preview) both
	// run in the background now; only the sweep was already async.
	go orch.CleanupLegacy(ctx)
	go func() {
		bctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		adopted, err := orch.Adopt(bctx)
		if err != nil {
			logger.Warn("initial adoption failed (docker down?)", "error", err)
			return
		}
		logger.Info("adopted state", "containers", len(adopted))
		for _, st := range adopted {
			if st.State == "running" {
				orch.EnsureSelfAttached(bctx, st.User, st.Project)
			}
		}
	}()

	go orch.RunIdleReaper(ctx)

	api := httpapi.New(cfg, users, orch, logger)
	// Periodic whole-tree checkpoints of running projects (operator Q6);
	// also covers the idle-reaper's missing pre-stop snapshot.
	go api.RunCheckpointLoop(ctx)
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

// migrateWorkspaces moves pre-ADR-027 workspaces (inside the shared home at
// ~/projects/<dir>) to their per-project storage dirs
// (StateDir/projects/<user>/<uuid>). One-time, per project, on boot — a
// same-filesystem rename, so cheap and atomic. A project whose microVM is
// still running (they survive server upgrades: restart unless-stopped) is
// STOPPED gracefully first — renaming under a live VM would strand it. A
// failed or conflicting move is logged loudly and left for the next boot;
// legacyWorkspace guards in ensureRepos/EnsureRunning keep an unmigrated
// project from being silently recreated empty in the meantime.
func migrateWorkspaces(cfg *config.Config, orch *orchestrator.Orchestrator, logger *slog.Logger) {
	forEachProject(cfg, logger, func(user string, p project.Project) {
		// Confined resolve: refuse a guest-planted symlink escaping the home.
		oldWS, err := files.ContainedPath(cfg.UserHome(user), "projects/"+p.Dir)
		if err != nil {
			logger.Warn("workspace migration: old path rejected", "user", user, "project", p.ID, "error", err)
			return
		}
		if st, err := os.Lstat(oldWS); err != nil || !st.IsDir() {
			return // nothing to migrate; creation/sweep handles the rest
		}
		newWS := cfg.ProjectWorkspace(user, p.ID)
		if _, err := os.Stat(newWS); err == nil {
			// An EMPTY old dir is just the bind's mount-point stub that
			// Docker recreates inside the home on every container start —
			// expected post-migration, not a conflict.
			if entries, rerr := os.ReadDir(oldWS); rerr == nil && len(entries) == 0 {
				return
			}
			logger.Warn("workspace migration CONFLICT: both locations hold content — leaving both untouched, resolve by hand",
				"user", user, "project", p.Name, "old", oldWS, "new", newWS)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if st, err := orch.Status(ctx, user, p.ID); err == nil && st.State == "running" {
			logger.Info("workspace migration: stopping running project first", "user", user, "project", p.Name)
			if err := orch.Stop(ctx, user, p.ID); err != nil {
				logger.Error("workspace migration: stop failed — project NOT migrated this boot",
					"user", user, "project", p.Name, "error", err)
				return
			}
		}
		if err := os.MkdirAll(filepath.Dir(newWS), 0o700); err != nil {
			logger.Error("workspace migration: cannot create projects root", "user", user, "error", err)
			return
		}
		if err := os.Rename(oldWS, newWS); err != nil {
			// EXDEV possible when STYRMAN_HOMES_DIR sits on another mount —
			// the legacyWorkspace guards keep the project safe until resolved.
			logger.Error("workspace migration FAILED — project stays on the old path (guarded) until fixed",
				"user", user, "project", p.Name, "from", oldWS, "to", newWS, "error", err)
			return
		}
		logger.Info("workspace migrated to per-project storage (ADR-027)",
			"user", user, "project", p.Name, "to", newWS)
	})
}

// forEachProject iterates every user's project records on disk.
func forEachProject(cfg *config.Config, logger *slog.Logger, fn func(user string, p project.Project)) {
	usersDir := filepath.Join(cfg.StateDir, "users")
	des, err := os.ReadDir(usersDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("project sweep: users dir unreadable", "dir", usersDir, "error", err)
		}
		return
	}
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		user := de.Name()
		store, err := project.NewStore(filepath.Join(cfg.UserStateDir(user), "projects.json"))
		if err != nil {
			logger.Warn("project sweep: store unreadable", "user", user, "error", err)
			continue
		}
		for _, p := range store.List() {
			fn(user, p)
		}
	}
}

// ensureRepos upholds the R13 invariant on every boot: any project
// workspace without a git repository (pre-R13 projects, or dirs restored
// from backup) gets one initialized in place. Never touches existing repos.
func ensureRepos(cfg *config.Config, logger *slog.Logger) {
	forEachProject(cfg, logger, func(user string, p project.Project) {
		// ADR-027: the workspace path is server-controlled end to end — no
		// home-containment guard needed anymore.
		ws := cfg.ProjectWorkspace(user, p.ID)
		if _, err := os.Stat(ws); err != nil {
			// NEVER create the new location while the old one still holds
			// the real tree (failed migration) — an empty dir here would
			// shadow it permanently (review finding).
			if cfg.LegacyWorkspaceExists(user, p.Dir) {
				logger.Error("R13 sweep: workspace not migrated yet — skipping (see migration errors above)",
					"user", user, "project", p.Name)
				return
			}
			// Missing entirely (e.g. restored metadata without data) — R13
			// says every project IS a repo, so recreate an empty one.
			if err := os.MkdirAll(ws, 0o700); err != nil {
				logger.Warn("R13 sweep: cannot create workspace", "user", user, "project", p.ID, "error", err)
				return
			}
		}
		if gitrepo.IsRepo(ws) {
			return
		}
		if err := gitrepo.EnsureRepo(ws); err != nil {
			logger.Warn("R13 sweep: git init failed", "user", user, "project", p.ID, "error", err)
		} else {
			logger.Info("R13 sweep: initialized git repo", "user", user, "project", p.Name)
		}
	})
}
