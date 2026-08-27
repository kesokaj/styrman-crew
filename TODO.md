# TODO

Deferred work items. Not blocking current work, but should be addressed later.

## High Priority

- [ ] Rename the project directory `~/workspace/super-devbox` → `~/workspace/styrman`
      (do between agent sessions — renaming under a live session breaks its cwd) (added 2026-08-26)

## Medium Priority

- [ ] UI: reduce poll amplification (Sidebar/ConversationList/ConversationPage each poll the same project; hoist to one shared source) (added 2026-08-26)
- [ ] UI: replace two-stage delete confirm()s with a dialog + explicit wipe checkbox (added 2026-08-26)

- [ ] Complete claude + agy first-run sign-in in the demo projects (operator's browser
      needed once; agy's remote-OAuth code flow works through the web terminal) (added 2026-08-26)
- [ ] agy native resume: find its session/store mechanism and wire into paneCommand (ADR-011) (added 2026-08-26)
- [ ] Conversation auto-titles from claude transcript summaries (~/.claude/projects) (added 2026-08-26)
- [ ] Integration check for agent-path keyring (DBUS in agent conversations, not just
      kind=shell) — the Phase 1 suite missed a real bug there (added 2026-08-26)
- [x] ~~createProject synchronous~~ — async since ADR-021 (2026-08-26)
- [ ] Migrate docker SDK from pinned docker/docker v28 to the renamed moby modules (added 2026-08-26)
- [ ] Exercise the entrypoint's vfs fallback path (loop-mount failure) in a test (added 2026-08-26)

- [x] ~~systemd unit~~ — superseded: compose restart-unless-stopped covers reboots (2026-08-26)
- [ ] Pin + verify SHA-256 of the Kata release tarball in host-setup.sh (added 2026-08-26)
- [ ] Keyring password visible via docker inspect (container env); move to one-shot exec injection (added 2026-08-26)
- [ ] Phase 3 needs Google OAuth client ID/secret, Cloudflare API token, and BASE_DOMAIN
      from the operator — the old devbox `.env` is not on this machine (added 2026-08-26)
- [ ] Decide backup story for `/var/lib/styrman/home` (restic/borg to GCS?) (added 2026-08-26)

## Low Priority

- [ ] Transcript rotation for ~/.styrman/logs (unbounded today) (added 2026-08-26)
- [ ] Ops panel: per-project disk/memory usage, container logs (added 2026-08-26)
- [ ] Git status badges in the sidebar (added 2026-08-26)

- [ ] Makefile target for Artifact Registry push if styrman ever leaves this host (added 2026-08-26)

## Ideas / Nice-to-Have

- [ ] Artifacts / files-changed right-rail panes (Antigravity-style) fed by git diff (added 2026-08-26)
- [ ] Mobile-friendly layout for quick check-ins from a phone (added 2026-08-26)

- **Notifications scan efficiency** (2026-08-27, reviewer): `GET /api/notifications` runs one sequential tmux exec per running project per poll and `Live()` re-fetches container status the caller already has. Fine at current scale; if project counts grow, add a `LiveIn(ctx, containerID, guest)` variant, scan projects concurrently (bounded errgroup), and cache per-user results for a few seconds.
- **profile.go GuestUser race** (2026-08-27, reviewer, pre-existing): `GuestUser()` reads `p.GuestName` without a mutex while `setGuestUser` writes it; add a `sync.RWMutex`.
- [ ] Idle-reaper auto-snapshot before reaping a project (ADR-022/R13 covers manual/stop/pause/close/close-all/pre-restore triggers, but not the idle reaper) — needs the project's workspace `Dir` plumbed into the reaper's orchestrator, or a callback hook it can invoke before stopping (added 2026-08-27)
