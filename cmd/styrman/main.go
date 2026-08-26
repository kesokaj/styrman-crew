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
	"github.com/kesokaj/styrman/internal/conversation"
	"github.com/kesokaj/styrman/internal/httpapi"
	"github.com/kesokaj/styrman/internal/orchestrator"
	"github.com/kesokaj/styrman/internal/project"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	// The state dir and shared home must exist and be writable by this
	// process (deploy/host-setup.sh creates them). Fail loudly otherwise.
	for _, dir := range []string{cfg.StateDir, cfg.HomeDir, filepath.Join(cfg.HomeDir, "projects")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("state dir unavailable — run deploy/host-setup.sh", "dir", dir, "error", err)
			os.Exit(1)
		}
	}

	projects, err := project.NewStore(cfg.ProjectsFile())
	if err != nil {
		logger.Error("project store load failed", "error", err)
		os.Exit(1)
	}
	convs, err := conversation.NewStore(filepath.Join(cfg.StateDir, "conversations.json"))
	if err != nil {
		logger.Error("conversation store load failed", "error", err)
		os.Exit(1)
	}

	orch, err := orchestrator.New(cfg, logger)
	if err != nil {
		logger.Error("orchestrator init failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Adopt pre-existing containers so restarts are seamless (ADR-007).
	if adopted, err := orch.Adopt(ctx); err != nil {
		logger.Warn("initial adoption failed (docker down?)", "error", err)
	} else {
		logger.Info("adopted state", "containers", len(adopted))
	}

	go orch.RunIdleReaper(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(cfg, projects, convs, orch, logger).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	logger.Info("styrman listening", "addr", cfg.Addr, "image", cfg.Image, "runtime", cfg.Runtime,
		"home", cfg.HomeDir, "user", cfg.Username, "uid", cfg.UID, "version", httpapi.Version)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
