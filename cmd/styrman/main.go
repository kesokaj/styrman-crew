// Command styrman is the single server binary: embedded UI, REST/WS API, and
// the Docker orchestrator for per-project Kata microVMs.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/credbroker"
	"github.com/kesokaj/styrman-crew/internal/credstore"
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
	// The LAN marker dir names every LAN-enabled project: owner-writable,
	// group-readable for root's script, nothing for other local uids (ADR-065).
	if err := os.MkdirAll(filepath.Join(cfg.NetPolicyDir(), "lan"), 0o750); err != nil {
		logger.Error("netpolicy dir unavailable", "dir", cfg.NetPolicyDir(), "error", err)
		os.Exit(1)
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
	// ADR-064: a setup VM created before the layout change still binds the
	// tenant dir with every project home inside it. Remove those first, so
	// no guest holds the old mount while the base homes are migrated below.
	// Bounded: this is synchronous Docker work ahead of the listener, the
	// shape that once hung startup (the adoption note below).
	sweepCtx, cancelSweep := context.WithTimeout(context.Background(), 10*time.Second)
	n, err := orch.RemoveStaleSetupVMs(sweepCtx)
	cancelSweep()
	if err != nil {
		logger.Warn("stale setup VM sweep failed", "error", err)
	} else if n > 0 {
		logger.Info("removed stale setup VMs (home layout changed, ADR-064)", "count", n)
	}
	migrateBaseHomes(cfg, logger)

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

	go orch.RunIdleReaper(ctx)
	go transcriptJanitor(ctx, cfg, logger)

	api := httpapi.New(cfg, users, orch, logger)

	// Per-project LAN markers mirror the records BEFORE any request can open
	// a store (ADR-065): missing markers are added, markers with no enabled
	// record removed — fail-closed and loud. wantLAN is called INSIDE the
	// marker dir's lock (see netpolicy.Dir.Sync); it reads fresh Store
	// instances from disk, a consistent snapshot because every save is
	// tmp+rename.
	wantLAN := func() (map[string]bool, error) {
		want := map[string]bool{}
		forEachProject(cfg, logger, func(_ string, p project.Project) { want[p.ID] = p.LanAccess })
		return want, nil
	}
	if !api.NetPolicy().Enabled() {
		// "Empty STYRMAN_FIREWALL_DIR disables the feature" has to be true for
		// EXISTING grants too: root would keep applying whatever markers are
		// left, while the UI could no longer see it (security review). With
		// the feature off, every marker is removed — records keep their intent
		// and report "cannot confirm" until the operator re-enables it.
		logger.Warn("per-project LAN access disabled (STYRMAN_FIREWALL_DIR empty): removing every marker; enables are refused")
		wantLAN = func() (map[string]bool, error) { return map[string]bool{}, nil }
	}
	if added, removed, err := api.NetPolicy().Sync(wantLAN); err != nil {
		logger.Error("lan marker sync failed", "error", err)
	} else if added+removed > 0 {
		logger.Info("lan markers synced", "added", added, "removed", removed)
	}
	api.NetPolicy().Kick("boot")
	// A dedicated ticker — NOT the orchestrator's idle reaper, which knows
	// nothing of stores or markers — so record and marker converge without a
	// restart (a crash between store save and marker write, or a store edited
	// by hand).
	go func() {
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, _, err := api.NetPolicy().Sync(wantLAN); err != nil {
					logger.Warn("lan marker re-sync failed", "error", err)
				}
			}
		}
	}()
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
				orch.EnsureSelfAttached(st.User, st.Project)
			}
		}
		// VMs dockerd auto-restarted never pass through EnsureRunning, so
		// their bridges (and the subnet a LAN rule pins) appear without a
		// kick from the request paths — nudge root once now (ADR-065).
		api.NetPolicy().Kick("adopt")
	}()

	// Credential broker (ADR-030): a bridge-only, credential-only listener,
	// separate from the ingress-gated main API. Never published to the host.
	// Only meaningful for a containerized server on project bridges — skip in
	// host-mode dev (SelfContainer empty), where it couldn't function and
	// would just open a port on all host interfaces.
	if cfg.BrokerAddr != "" && cfg.SelfContainer != "" {
		if cstore, cerr := credstore.New(filepath.Join(cfg.StateDir, "credentials")); cerr != nil {
			logger.Error("credential store unavailable — broker disabled", "error", cerr)
		} else {
			api.SetCredStore(cstore) // onboarding endpoints write here
			broker := credbroker.New(cstore, orch, logger)
			brokerSrv := &http.Server{
				Addr:              cfg.BrokerAddr,
				Handler:           broker.Handler(),
				ReadHeaderTimeout: 10 * time.Second,
			}
			go func() {
				logger.Info("credential broker up (bridge-only)", "addr", cfg.BrokerAddr)
				if err := brokerSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("credential broker failed", "error", err)
				}
			}()
			go func() { <-ctx.Done(); _ = brokerSrv.Close() }() // stop on shutdown (M30)
		}
	}
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
	logger.Info("styrman listening", "addr", cfg.Addr, "auth", cfg.AuthMode, "image", cfg.Image,
		"runtime", cfg.Runtime, "homes", cfg.HomesDir, "version", httpapi.Version)
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()

	select {
	case err := <-serverErr:
		// Stopped on its own (e.g. a bind error) before any signal.
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		return
	case <-ctx.Done():
	}
	// Signal received: drain in-flight requests SYNCHRONOUSLY before exit. The
	// old code triggered Shutdown in a goroutine and returned as soon as
	// ListenAndServe unblocked, so the process exited mid-drain (M30). The dev
	// and broker listeners already close on ctx.Done() above.
	logger.Info("shutting down — draining in-flight requests…")
	shutCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warn("graceful shutdown did not fully drain", "error", err)
	}
	<-serverErr // ListenAndServe returns ErrServerClosed once Shutdown completes
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
		// Skip when base/ exists OR the tenant dir already holds anything: a
		// half-migrated host must not get the legacy home renamed on top of
		// its own entries (security review 2026-09-04).
		tenantEntries, _ := os.ReadDir(cfg.TenantDir(user))
		if _, err := os.Stat(target); os.IsNotExist(err) && len(tenantEntries) == 0 {
			// The target is homes/<user>/base since ADR-064: its parent must exist.
			if err := os.MkdirAll(cfg.TenantDir(user), 0o700); err != nil {
				logger.Error("legacy home migration failed", "mkdir", cfg.TenantDir(user), "error", err)
			} else if err := os.Rename(cfg.HomeDir, target); err != nil {
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
// workspace without a git repository (pre-R13 projects) gets one
// initialized in place. Never touches existing repos.
func ensureRepos(cfg *config.Config, logger *slog.Logger) {
	// A boot sweep must not hang forever on a wedged git — bound the whole pass.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
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
		if gitrepo.IsRepo(ctx, ws) {
			return
		}
		if err := gitrepo.EnsureRepo(ctx, ws); err != nil {
			logger.Warn("R13 sweep: git init failed", "user", user, "project", p.ID, "error", err)
		} else {
			logger.Info("R13 sweep: initialized git repo", "user", user, "project", p.Name)
		}
	})
}

// transcriptJanitor prunes terminal-transcript files older than
// cfg.TranscriptTTL (default 30d, STYRMAN_TRANSCRIPT_TTL, 0 = keep forever).
// Transcripts live in each per-project home (~/.styrman/logs, ADR-017) and
// grew unbounded before v1.14.0. Runs at startup and every 6h; file-mtime
// based, best-effort — an unreadable dir is skipped, never fatal.
func transcriptJanitor(ctx context.Context, cfg *config.Config, logger *slog.Logger) {
	if cfg.TranscriptTTL <= 0 {
		return
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		pruneTranscripts(cfg, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func pruneTranscripts(cfg *config.Config, logger *slog.Logger) {
	cutoff := time.Now().Add(-cfg.TranscriptTTL)
	var pruned int
	users, err := os.ReadDir(cfg.HomesDir)
	if err != nil {
		return
	}
	for _, u := range users {
		if !u.IsDir() {
			continue
		}
		// The tenant dir homes/<u> is server-only since ADR-064 (the guest
		// mounts are base/ and p/<uuid>, each guest-writable); before that the
		// setup VM mounted the tenant dir itself with the project homes INSIDE
		// it. `p` and every entry under it are still listed and opened through
		// an os.Root at the tenant dir — belt and braces now, once the only defense — a
		// symlink the guest plants at p/<uuid> (or at p itself) toward the
		// host or another tenant is refused, never followed; before this the
		// project root was opened by raw path and would have been (security
		// review 2026-09-03, BUG-003). Inside a project home the walk was
		// already confined (2026-08-30 review).
		tenant, terr := os.OpenRoot(filepath.Join(cfg.HomesDir, u.Name()))
		if terr != nil {
			continue
		}
		projects, err := fs.ReadDir(tenant.FS(), "p")
		if err != nil {
			tenant.Close()
			continue
		}
		for _, p := range projects {
			home, rerr := tenant.OpenRoot(filepath.Join("p", p.Name()))
			if rerr != nil {
				continue
			}
			_ = fs.WalkDir(home.FS(), ".styrman/logs", func(rel string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil //nolint:nilerr // best-effort janitor
				}
				// fs.Stat through the root, not d.Info(): for a root opened
				// with Root.OpenRoot the entry's Info() lstats a path that
				// repeats the sub-root's prefix and fails (Go 1.25), which
				// would silently skip every file.
				fi, ierr := fs.Stat(home.FS(), rel)
				if ierr != nil || !fi.Mode().IsRegular() {
					return nil
				}
				if fi.ModTime().Before(cutoff) {
					if home.Remove(rel) == nil {
						pruned++
					}
				}
				return nil
			})
			home.Close()
		}
		tenant.Close()
	}
	if pruned > 0 {
		logger.Info("transcript janitor pruned old transcripts",
			"files", pruned, "ttl", cfg.TranscriptTTL.String())
	}
}

// migrateBaseHomes moves each tenant's pre-ADR-064 base home into
// homes/<user>/base. Before ADR-064 the setup VM mounted homes/<user>
// itself, which put every project home (p/<uuid>) inside a root guest's
// mount (BUG-002/003). Everything except base/ and p/ moves; p/ is never
// touched. Every operation goes through an os.Root at the tenant dir, so a
// `base` swapped for a symlink between two steps cannot redirect a rename
// outside the tenant (security review 2026-09-04); a `base` that is not a
// real directory is refused up front, and an entry whose destination already
// exists is left in place. Runs after the stale setup VMs are removed, so no
// guest holds the old mount while it works. Idempotent: a tenant with
// nothing outside base/ and p/ is skipped; a symlinked tenant entry is
// skipped. Logs counts, never entry names. Never aborts startup.
func migrateBaseHomes(cfg *config.Config, logger *slog.Logger) {
	users, err := os.ReadDir(cfg.HomesDir)
	if err != nil {
		return
	}
	for _, u := range users {
		if !u.IsDir() { // a symlinked tenant entry reports as not a dir: skipped
			continue
		}
		rt, err := os.OpenRoot(cfg.TenantDir(u.Name()))
		if err != nil {
			logger.Warn("base home migration: cannot open tenant dir", "user", u.Name(), "error", err)
			continue
		}
		moved, left, failed, firstErr, err := migrateTenantIntoBase(rt)
		rt.Close()
		switch {
		case err != nil:
			logger.Error("base home migration refused", "user", u.Name(), "error", err)
		case moved+left+failed == 0:
			// already migrated, or a fresh tenant
		case failed > 0:
			// An entry left at the tenant top level is reachable from no guest:
			// name the first cause (the error carries a path, not a secret).
			logger.Error("base home migration incomplete", "user", u.Name(), "to", cfg.UserHome(u.Name()),
				"entries", moved, "leftInPlace", left, "failed", failed, "firstError", firstErr)
		default:
			logger.Info("migrated base home", "user", u.Name(), "to", cfg.UserHome(u.Name()),
				"entries", moved, "leftInPlace", left, "failed", failed)
		}
	}
}

// migrateTenantIntoBase does one tenant's move inside its confined root.
// It returns the counts and a refusal error when base exists but is not a
// real directory.
func migrateTenantIntoBase(rt *os.Root) (moved, left, failed int, firstErr error, err error) {
	entries, err := fs.ReadDir(rt.FS(), ".")
	if err != nil {
		return 0, 0, 0, nil, err
	}
	var move []string
	for _, e := range entries {
		if e.Name() == "base" || e.Name() == "p" {
			continue
		}
		move = append(move, e.Name())
	}
	if len(move) == 0 {
		return 0, 0, 0, nil, nil
	}
	if st, err := rt.Lstat("base"); err == nil {
		if !st.IsDir() {
			return 0, 0, 0, nil, errors.New("base exists but is not a real directory (a planted symlink?) — nothing moved")
		}
	} else if err := rt.Mkdir("base", 0o700); err != nil {
		return 0, 0, 0, nil, fmt.Errorf("create base: %w", err)
	}
	for _, name := range move {
		dst := "base/" + name
		if _, err := rt.Lstat(dst); err == nil {
			left++ // a partial earlier run, or a stale setup VM recreated it: never replace
			continue
		}
		if err := rt.Rename(name, dst); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		moved++
	}
	return moved, left, failed, firstErr, nil
}
