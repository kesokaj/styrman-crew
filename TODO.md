# TODO

Deferred work items. Not blocking current work, but should be addressed later.

## High Priority

- [ ] Rename the project directory `~/workspace/super-devbox` → `~/workspace/styrman`
      (do between agent sessions — renaming under a live session breaks its cwd) (added 2026-08-26)

## Medium Priority

- [ ] UI: reduce poll amplification (Sidebar + dashboard each poll listProjects; hoist to one shared source). Partially mitigated in v1.3: usePolled pauses in hidden tabs; dashboard terminal polls calmed to 30s (added 2026-08-26)
- [x] ~~UI: replace two-stage delete confirm()s with a dialog~~ — v1.3 phase 2 removed ALL native popups (2026-08-28)

- [x] ~~Complete claude + agy first-run sign-in in the demo projects~~ — superseded:
      the credential broker now pulls every tool, agy included, into every project's
      first login shell automatically (v1.5.4 setup-seeds-broker; v1.5.5 moved agy
      from the dead keyring-tier assumption to the file tier, ADR-038). No manual
      per-project sign-in remains (added 2026-08-26, closed 2026-08-29)
- [ ] agy native resume: find its session/store mechanism and wire into paneCommand (ADR-011) (added 2026-08-26)
- [ ] Conversation auto-titles from claude transcript summaries (~/.claude/projects) (added 2026-08-26)
- [ ] Integration check for agent-path keyring (DBUS in agent conversations, not just
      kind=shell) — the Phase 1 suite missed a real bug there (added 2026-08-26)
- [x] ~~createProject synchronous~~ — async since ADR-021 (2026-08-26)
- [ ] Migrate docker SDK from pinned docker/docker v28 to the renamed moby modules (added 2026-08-26)
- [ ] Exercise the entrypoint's vfs fallback path (loop-mount failure) in a test (added 2026-08-26)

- [x] ~~systemd unit~~ — superseded: compose restart-unless-stopped covers reboots (2026-08-26)
- [x] ~~Pin + verify SHA-256 of the Kata release tarball in host-setup.sh~~ — done (host-setup.sh verifies; confirmed by the 2026-08-28 audit)
- [ ] Keyring password visible via docker inspect (container env); move to one-shot exec injection (added 2026-08-26)
- [ ] Phase 3 needs Google OAuth client ID/secret, Cloudflare API token, and BASE_DOMAIN
      from the operator — the old devbox `.env` is not on this machine (added 2026-08-26)
- [ ] Backup: implement microVM **volume snapshots** of `/var/lib/styrman` (operator decision 2026-08-29, replacing the git auto-checkpoints removed in v1.7.0). Host-infra dependent — the FS must support snapshots (LVM/ZFS/btrfs) or use scheduled restic/borg to GCS; `/var/lib/styrman/{homes,projects,users}` is the whole state, ADR-027 source lives under projects/ (added 2026-08-26, updated 2026-08-29)
- [ ] Update the clean-reinstall procedure doc (`.agents/docs/changes/2026-08-28-clean-reinstall.md` or wherever it's canonicalized next) to explicitly call out "restart the server after scrubbing StateDir/users" as a required step, not an implicit one — its omission caused the v1.5.2 first-login 400 fixed in v1.5.3 (ADR-033) (added 2026-08-29)
- [ ] Remove the now-dormant broker keyring tier: with agy moved to the file tier in v1.5.5 (`~/.gemini/antigravity-cli/antigravity-oauth-token`), nothing uses `credstore.KeyringCred`/`PutKeyring`, the broker `GET /keyring` endpoint, or the `secret-tool store` block in `styrman-credentials.sh`. Left dormant in v1.5.5 to keep the change small; delete as a separate cleanup — ADR-032a is superseded on the agy-storage point by ADR-038 (added 2026-08-29)
- [ ] Broker-sync the gcloud ACCOUNT login (`gcloud auth login`, `~/.config/gcloud/`),
      not just ADC — today only `gcloud auth application-default login` (ADC) syncs
      via the broker's file tier (covers terraform + client libraries); per-project
      `gcloud` CLI commands still need a manual, per-project account sign-in (noted
      in v1.8.0's setup-optional-gcloud change) (added 2026-08-29)

## Low Priority

- [ ] Transcript rotation for ~/.styrman/logs (unbounded today) (added 2026-08-26)
- [ ] Ops panel: per-project disk/memory usage, container logs. (v1.3 dashboard covers live terminal tails; resource stats still missing) (added 2026-08-26)
- [ ] Git status badges in the sidebar (added 2026-08-26)

- [ ] Makefile target for Artifact Registry push if styrman ever leaves this host (added 2026-08-26)

## Post-v1.3 roadmap (operator-approved 2026-08-28, from competitor research)

- [ ] Checkpoints that include `~/.claude` (and agy state) so a restore also
      rolls back the agent's memory (Replit/Sprites pattern)
- [ ] Platform API as an MCP server — agents manage their own ports/snapshots/
      restarts (Fly Sprites pattern)
- [ ] Warm VM pool of 1 — pre-booted generic microVM claimed on new-project
      (Daytona/Coder prebuild pattern)

## Ideas / Nice-to-Have

- [ ] Artifacts / files-changed right-rail panes (Antigravity-style) fed by git diff (added 2026-08-26)
- [x] ~~Mobile-friendly layout~~ — v1.3 phase 2: sidebar drawer + top bar below md, side panel overlay below xl (2026-08-28)

- **Notifications scan efficiency** (2026-08-27, reviewer): `GET /api/notifications` runs one sequential tmux exec per running project per poll and `Live()` re-fetches container status the caller already has. Fine at current scale; if project counts grow, add a `LiveIn(ctx, containerID, guest)` variant, scan projects concurrently (bounded errgroup), and cache per-user results for a few seconds.
- [x] ~~profile.go GuestUser race~~ — fixed: profileStore has a sync.RWMutex; verified by the 2026-08-28 audit (go test -race clean)
- [x] ~~Idle-reaper auto-snapshot~~ — covered by the v1.3 periodic checkpoint loop (10-min cadence for active projects; worst-case loss = one interval) (2026-08-28)

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
  - **I15** — DONE for the parts that still exist (2026-08-28): host-setup
    builds the runtime image (version-stamped) and prints the compose command;
    home-init/ensureHome were removed by ADR-031 (image seeds homes).

- **WON'T DO (operator decision 2026-08-27):**
  - **I14** docker-socket proxy for Caddy — declined.
  - **I13** version-pinning Go/Node/claude/yq in the runtime image — declined:
    the terminal is a dev environment and intentionally ships current tool
    versions; pinning would defeat the purpose. (Kata tarball checksum in
    host-setup.sh stays — that's supply-chain, not version pinning.)

- [x] ~~Multi-user harness (I15 residual)~~ — closed by ADR-030/031: the base
  image carries the harness and the entrypoint seeds every per-project home;
  ensureHome and home-init.sh no longer exist (2026-08-28 audit).

## Deferred from the 2026-08-28 full audit (fix-worthy, not urgent)

- [ ] UI: route the three hand-rolled modals (NewProjectDialog,
      DeleteProjectDialog, FileViewer) through the shared `Dialog` primitive
      (role/aria/Escape), and give `Dialog` a focus trap + focus restore.
- [ ] UI: make the primary rows (Sidebar project, TerminalList terminal,
      FileExplorer entry) keyboard-reachable (`role="button"`/tabIndex or real
      buttons).
- [ ] UI: split the 770-line `SidePanel.tsx` into `components/panel/*`;
      dedupe the "first terminal or create" navigation helper; `PublicLinks`'
      sticky poll override; `useMediaQuery` for the panel's 1280px breakpoint.
- [ ] UI: surface `usePolled` errors at the remaining call sites
      (NotificationBell, SidePanel tabs, terminal route) — dashboard, sidebar,
      settings, and SetupBanner were fixed in the audit.
- [ ] API client: handle 401/403 centrally in `request()` (re-auth prompt)
      or drop the unused `ApiError.status`.
- [ ] Go: `Settings.homeDir` (and the Settings UI "Your home" row) still
      reports the per-user BASE home; post-ADR-031 decide what "your home"
      means there. Same question for `GET /api/settings/tools`, which probes
      the setup-VM home for skills/commands (empty for brand-new users).
- [ ] Go: `EnsureSelfAttached` discards its ctx param (drop it);
      `IPAddress` ignores `user`; `SetOperator` holds its lock across fsync;
      dedupe `NewUUID` (project/conversation) and `usernameFromEmail`.
- [ ] Go: unbounded (tiny) maps never pruned: `projLocks`, `ipCache`,
      `wsLocks`, checkpoint-loop `snapped` — fine at current scale.
- [ ] Image: `--skip-verify` on the OpenTofu installer (supply-chain; needs
      cosign in the build env to drop).
- [ ] Deploy: compose hands Caddy the whole `.env` (`env_file`) — narrow to
      an explicit `environment:` list (it only needs OAuth/CF/JWT/domains).
- [ ] SPA hydration mismatch (React #418) — see above; still open.
- [ ] Conversation `AgentSession` field is written+validated but never read
      (ADR-014 parked native resume) — remove or implement with agy resume.

## Post-1.0.0 (from the pre-1.0.0 review, 2026-08-27)

- [x] ~~Public TTL preview links on `*.preview.styrman.xyz`~~ — shipped
      2026-08-27, see ADR-025 (internal/links, internal/httpapi/links_api.go,
      previewHostRewrite, ingress wildcard + wildcard DNS/cert). Stateful
      token store instead of a signed blob so links are revocable/listable.
- [x] ~~Per-port share scoping~~ — shipped 2026-08-27, see ADR-024
      (`project.Project.SharedPorts map[int][]string`, keyed per port, not
      project-wide).
- **Proxy attach cache** — cache `EnsureSelfAttached` per project (TTL) instead
  of a Docker `NetworkConnect` per proxied request.
- **`projectID→owner` index** so `previewAccess` doesn't scan all user dirs.
- **Proxy path normalization** — `path.Clean` + `RawPath` on the forwarded URL.
- **Bridge sessionAlive integration test** behind a small `Exec` interface.

## Post-1.1 (from the 1.1 review, 2026-08-27)

- **S9** — migrate a legacy `sharedEmails` (project-wide) field into `SharedPorts`
  on load, or log affected projects. Only matters for unreleased intermediate data.
- **S10** — force-close proxied WebSocket/stream connections at link expiry
  (ReverseProxy doesn't re-check the token after the upgrade handshake).
- Guest app sees `Host: <ip>:<port>` — rewrite `Location` on absolute redirects.
- `linkURL` returns a login-gated path form when STYRMAN_PREVIEW_BASE is unset;
  surface "public links need STYRMAN_PREVIEW_DOMAIN" in the UI or 503 the mint.
