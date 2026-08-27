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

- **SPA hydration mismatch (2026-08-27, Batch E e2e):** the prerendered
  `_shell.html` triggers React #418 (hydration text mismatch) on first client
  render — recoverable (React re-renders), functionally harmless, but a real
  server/client divergence. e2e filters it as a known-benign warning. Track
  the source (likely a prerendered dynamic value) and make the initial render
  deterministic.

- **Deferred deploy hardening (Batch D, 2026-08-27):**
  - **I14** docker.sock read-only proxy for Caddy (Caddy has host-root Docker
    access; `:ro` bind doesn't restrict socket connect). Needs a
    `docker-socket-proxy` sidecar + careful live-ingress test — do in a
    dedicated session like the JWT-key change.
  - **I13 (partial)** OpenTofu `--skip-verify` + unpinned Go/claude/node/yq in
    image/Dockerfile — pin versions + drop skip-verify, validating the image
    still builds. (Kata tarball checksum: DONE in host-setup.sh.)
  - **I15** make host-setup.sh the single convergent fresh-install entry point
    (build runtime image, run home-init, print compose command) + per-user
    harness in ensureHome; align deploy.md. (R11/R5.)

- **WON'T DO (operator decision 2026-08-27):**
  - **I14** docker-socket proxy for Caddy — declined.
  - **I13** version-pinning Go/Node/claude/yq in the runtime image — declined:
    the terminal is a dev environment and intentionally ships current tool
    versions; pinning would defeat the purpose. (Kata tarball checksum in
    host-setup.sh stays — that's supply-chain, not version pinning.)

- **Multi-user harness (I15 residual, 2026-08-27):** host-setup.sh + home-init.sh
  now seed the OPERATOR's home (dotfiles + agent harness) and converge to the
  compose bring-up. But additional users' homes are created by ensureHome
  (internal/httpapi/server.go) inside the server container, which has no
  devshell_prompt/harness checkout — so users 2+ get dotfiles but NOT the
  harness (R5 gap for multi-user). Decide: bake the harness into the server
  image, or mount it, then have ensureHome deploy it. Single-operator installs
  are unaffected.
