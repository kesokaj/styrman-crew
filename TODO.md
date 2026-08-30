# TODO

Deferred work. Nothing here is review debt — the 2026-08-30 `/code-review
ultra` remediation (v1.9.x) and the same-day `/audit` remediation (v1.13.1)
are fully shipped; see `.agents/docs/changes/`. Completed items are removed
from this file (git history + changelogs are the record). Restructured
2026-08-30 (operator: "I want my todo/roadmap to be clean").

## Quick wins (next working session, one commit)

- [ ] Integration suite: auto-SKIP the 3 proxy/link checks when host→bridge
      traffic is firewalled (hardened host; the test server runs as a host
      process — production's containerized server is exempt). Suite should
      exit green at 57/57+3 skipped instead of failing 57/60 forever.
- [ ] Transcript rotation for `~/.styrman/logs` — unbounded today; cap by
      size/age at boot or in the reaper tick (added 2026-08-26).
- [ ] Clean-reinstall doc: add "restart the server after scrubbing
      StateDir/users" as an explicit required step (its omission caused the
      v1.5.2 first-login 400, ADR-033) (added 2026-08-29).
- [ ] Go vestige sweep: remove the never-read conversation `AgentSession`
      field (or wire it with agy resume, below); drop `EnsureSelfAttached`'s
      discarded ctx param; dedupe `NewUUID` + `usernameFromEmail`;
      `SetOperator` holds its lock across fsync.

## Build next (operator-approved priority)

- [ ] **Host-layer backups** — whole-state backup of `/var/lib/styrman`
      (operator decision 2026-08-29). Root FS is plain ext4 without LVM
      (verified 2026-08-30) → scheduled restic/borg to GCS;
      `{homes,projects,users,backups,credentials}` is the whole state. The
      USER layer shipped in v1.13.0 (per-project backup/restore, ADR-044) —
      this is the disaster-recovery complement.
- [ ] **Backup cadence + pre-delete snapshot** — daily (or configurable)
      auto-backup per project with keep-last-N retention, plus an automatic
      backup before a wipe-delete. Completes the "restore rolls back agent
      memory" roadmap item (its manual half shipped v1.13.0).

## Operator actions

- [ ] Sign in to gcloud (Settings → Credentials: ADC + CLI rows are grey).
- [ ] Delete the dead `STYRMAN_KEYRING_PW=` line from `deploy/.env`
      (gitignored; server ignores it).
- [ ] Rename `~/workspace/super-devbox` → `~/workspace/styrman` — BETWEEN
      agent sessions (renaming under a live session breaks its cwd)
      (added 2026-08-26).

## Roadmap (later, rough value order)

- [ ] Warm VM pool of 1 — pre-booted generic microVM claimed on new-project
      (Daytona/Coder prebuild pattern).
- [ ] Ops panel: per-project disk/memory usage, container logs (dashboard
      covers live terminal tails; resource stats missing) (added 2026-08-26).
- [ ] Platform API as an MCP server — agents manage their own ports/backups/
      restarts (Fly Sprites pattern).
- [ ] agy native resume: find its session/store mechanism and wire into
      paneCommand (ADR-011) (added 2026-08-26).
- [ ] API client: central 401/403 handling in `request()` (re-auth prompt);
      Deploy: narrow Caddy's `env_file` to an explicit `environment:` list
      (it only needs OAuth/CF/JWT/domains).
- [ ] Conversation auto-titles from claude transcript summaries
      (`~/.claude/projects`) (added 2026-08-26).
- [ ] Git status badges in the sidebar (added 2026-08-26).
- [ ] UI: reduce poll amplification (Sidebar + dashboard each poll
      listProjects; hoist to one shared source; partially mitigated in v1.3).
- [ ] UI polish batch (bundle into one session): route the hand-rolled
      modals through the shared `Dialog` primitive + give it a focus trap
      and focus restore (v1.13.1 added role/aria/Escape directly, shell
      conversion pending); keyboard-reachable primary rows (Sidebar project,
      TerminalList, FileExplorer); surface `usePolled` errors at the
      remaining call sites (NotificationBell, SidePanel tabs, terminal
      route); SidePanel residuals (dedupe first-terminal navigation helper,
      PublicLinks sticky poll override, `useMediaQuery` for the 1280px
      breakpoint); SPA hydration mismatch (React #418 on the prerendered
      shell — benign, make the initial render deterministic).
- [ ] `Settings.homeDir` semantics post-ADR-031: the Settings "Your home"
      row + `GET /api/settings/tools` still probe the per-user BASE home —
      decide what "your home" means with per-project homes.
- [ ] Preview-proxy residuals (only matter as usage grows): cache
      `EnsureSelfAttached` per project (TTL); `projectID→owner` index so
      `previewAccess` doesn't scan user dirs; `path.Clean` + `RawPath` on
      forwarded URLs; force-close proxied WebSockets at link expiry (S10);
      migrate any legacy `sharedEmails` into `SharedPorts` on load (S9);
      rewrite `Location` on absolute redirects (guest sees `Host: ip:port`);
      surface "public links need STYRMAN_PREVIEW_DOMAIN" instead of a
      login-gated linkURL when unset; bridge sessionAlive integration test
      behind a small Exec interface; notifications scan efficiency
      (`LiveIn` variant + bounded concurrent scan + short cache) if project
      counts grow.

## Dropped 2026-08-30 (operator may veto — say the word and they return)

- Moby SDK migration (pinned docker/docker v28 works; churn without user
  value — revisit only when forced by an upstream break).
- Artifact Registry push Makefile target (conditional on styrman ever
  leaving this host — resurrect then).
- Pruning the tiny unbounded maps (`projLocks`, `ipCache`, `wsLocks`) —
  non-issue at single-operator scale, documented in code.
- Exercising the entrypoint's vfs fallback in a test — the fallback has
  never fired; revisit if it ever does.

## WON'T DO (operator decisions, 2026-08-27)

- **I14** docker-socket read-only proxy for Caddy — declined.
- **I13** version-pinning Go/Node/claude/yq in the runtime image — declined:
  the terminal is a dev environment and intentionally ships current tools
  (the Kata tarball checksum in host-setup.sh stays — that's supply-chain
  verification, not version pinning).
