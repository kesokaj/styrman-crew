# TODO

Deferred work items. Not blocking current work, but should be addressed later.

> **State (2026-08-30):** the full `/code-review ultra` remediation is complete
> and deployed (v1.9.1) — see `.agents/docs/changes/2026-08-30-security-review-
> remediation-v1.9.0.md` and ADR-039/040. Nothing below is review debt; these
> are intentional future features/infra (volume snapshots, ops panel, warm VM
> pool, MCP server, docker-SDK migration, gcloud account-login sync, agy resume)
> plus minor, non-urgent polish. Items marked done here reflect that remediation.

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
- [x] ~~Integration check for agent-path keyring (DBUS in agent conversations)~~ —
      moot: the keyring tier was removed entirely (v1.9.0, ADR-040); credentials
      flow via env/file/git-helper tiers, no keyring (2026-08-30)
- [x] ~~createProject synchronous~~ — async since ADR-021 (2026-08-26)
- [ ] Migrate docker SDK from pinned docker/docker v28 to the renamed moby modules (added 2026-08-26)
- [ ] Exercise the entrypoint's vfs fallback path (loop-mount failure) in a test (added 2026-08-26)

- [x] ~~systemd unit~~ — superseded: compose restart-unless-stopped covers reboots (2026-08-26)
- [x] ~~Pin + verify SHA-256 of the Kata release tarball in host-setup.sh~~ — done (host-setup.sh verifies; confirmed by the 2026-08-28 audit)
- [x] ~~Keyring password visible via docker inspect (container env)~~ — moot:
      the keyring tier was removed entirely (v1.9.0, ADR-040), so STYRMAN_KEYRING_PW
      is no longer set on project containers at all (2026-08-30)
- [x] ~~Phase 3 needs Google OAuth client ID/secret, Cloudflare API token, BASE_DOMAIN~~
      — provided; crew.styrman.xyz is live behind Google OAuth + Cloudflare
      (deploy/.env configured, secrets rotated in the 2026-08-28 reinstall)
- [ ] Backup, host layer: implement whole-state **volume snapshots** of `/var/lib/styrman` (operator decision 2026-08-29). Host-infra dependent — the root FS is plain ext4 without LVM (verified 2026-08-30), so this means scheduled restic/borg to GCS; `/var/lib/styrman/{homes,projects,users,backups,credentials}` is the whole state. The USER layer shipped separately in v1.13.0 (per-project backup/restore in-app, ADR-044) — this item is the disaster-recovery complement, not superseded (added 2026-08-26, updated 2026-08-30)
- [ ] Update the clean-reinstall procedure doc (`.agents/docs/changes/2026-08-28-clean-reinstall.md` or wherever it's canonicalized next) to explicitly call out "restart the server after scrubbing StateDir/users" as a required step, not an implicit one — its omission caused the v1.5.2 first-login 400 fixed in v1.5.3 (ADR-033) (added 2026-08-29)
- [x] ~~Remove the now-dormant broker keyring tier~~ — done in the 2026-08-30
      security review remediation (v1.9.0, commit 8ca6368): `credstore.
      KeyringCred`/`PutKeyring`, `GET /keyring`, and the `secret-tool store`
      snippet block are all deleted, not just unused. See ADR-040 (added
      2026-08-29, closed 2026-08-30)
- [x] ~~Broker-sync the gcloud ACCOUNT login (`gcloud auth login`)~~ — DONE
      2026-08-30 (v1.9.3): harvestCredentials now also grabs `credentials.db` +
      `active_config` + `configurations/config_default`, so `gcloud <command>` is
      authenticated in every project, not just ADC. Re-seed once (re-run setup,
      or Settings → Import from a gcloud-signed-in project). Named (non-default)
      gcloud configs aren't covered (added 2026-08-29, closed 2026-08-30)

## Low Priority

- [ ] Transcript rotation for ~/.styrman/logs (unbounded today) (added 2026-08-26)
- [ ] Ops panel: per-project disk/memory usage, container logs. (v1.3 dashboard covers live terminal tails; resource stats still missing) (added 2026-08-26)
- [ ] Git status badges in the sidebar (added 2026-08-26)
- [x] ~~Split `web/src/components/SidePanel.tsx` into per-tab files~~ — DONE
      2026-08-30 (v1.9.1): 1008-line file → a ~95-line shell + SidePanelPorts/
      History/Git/Clips.tsx (commit 85e7a65)
- [x] ~~Move `(*Server).harvestCredentials` out of the handler file~~ — DONE
      2026-08-30 (v1.9.1): harvestCredentials + guestOut + importGuestFile +
      shellQuote → `internal/httpapi/credharvest.go` (kept in `httpapi` because
      gitIdentity/guestUserFor are shared Server utils; commit 85e7a65)

- [ ] Makefile target for Artifact Registry push if styrman ever leaves this host (added 2026-08-26)

## Post-v1.3 roadmap (operator-approved 2026-08-28, from competitor research)

- [x] ~~Checkpoints that include `~/.claude` (and agy state) so a restore also
      rolls back the agent's memory (Replit/Sprites pattern)~~ — substantially
      shipped by v1.13.0's per-project backup/restore (ADR-044): a backup
      archives the whole per-project home, `~/.claude` and agy state
      included, so restoring one already rolls back agent memory too.
      Remainder: an AUTOMATIC backup cadence — today's backups are
      manually triggered from the sidebar ⋯ menu, there's no schedule
      (closed 2026-08-30)
- [ ] Platform API as an MCP server — agents manage their own ports/snapshots/
      restarts (Fly Sprites pattern)
- [ ] Warm VM pool of 1 — pre-booted generic microVM claimed on new-project
      (Daytona/Coder prebuild pattern)

## Ideas / Nice-to-Have

- [ ] Artifacts / files-changed right-rail panes (Antigravity-style) fed by git diff (added 2026-08-26)
- [x] ~~Mobile-friendly layout~~ — v1.3 phase 2: sidebar drawer + top bar below md, side panel overlay below xl (2026-08-28)

- **Notifications scan efficiency** (2026-08-27, reviewer): `GET /api/notifications` runs one sequential tmux exec per running project per poll and `Live()` re-fetches container status the caller already has. Fine at current scale; if project counts grow, add a `LiveIn(ctx, containerID, guest)` variant, scan projects concurrently (bounded errgroup), and cache per-user results for a few seconds.
- [x] ~~profile.go GuestUser race~~ — fixed: profileStore has a sync.RWMutex; verified by the 2026-08-28 audit (go test -race clean)
- [x] ~~Idle-reaper auto-snapshot~~ — the v1.3 periodic checkpoint loop this
      referenced was itself removed in v1.7.0; superseded by two later,
      independent guarantees instead: idle stop is non-destructive and
      busy-guarded (v1.11.0, ADR-042 — a stop only powers the VM off, and
      the reaper's busy probe skips a project with a running DinD container
      or a non-shell foreground process, so there's nothing to lose), plus
      manual per-project backups on demand (v1.13.0, ADR-044) (2026-08-28,
      rationale updated 2026-08-30)

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
  - **I13 (skip-verify part DONE 2026-08-30, v1.9.0):** OpenTofu `--skip-verify`
    dropped so its signature is checked (image rebuilds cleanly). Version-pinning
    Go/claude/node/yq stays declined (WON'T DO below — dev env ships current
    tools). Docker itself is now pinned via `DOCKER_VERSION` in host-setup.sh.
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
- [ ] UI: residual SidePanel-area cleanups (the file SPLIT itself is DONE,
      v1.9.1) — dedupe the "first terminal or create" navigation helper;
      `PublicLinks`' sticky poll override; `useMediaQuery` for the panel's
      1280px breakpoint.
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
      `wsLocks` — fine at current scale. (The checkpoint-loop `snapped` map is
      gone — the checkpoint loop was removed in v1.7.0.)
- [x] ~~Image: `--skip-verify` on the OpenTofu installer~~ — dropped in v1.9.0
      (2026-08-30); the installer now verifies its signature (gnupg is present).
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
