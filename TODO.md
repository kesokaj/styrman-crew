# TODO

Deferred work items. Not blocking current work, but should be addressed later.

## High Priority

- [ ] Rename the project directory `~/workspace/super-devbox` → `~/workspace/styrman`
      (do between agent sessions — renaming under a live session breaks its cwd) (added 2026-08-26)

## Medium Priority

- [ ] Complete claude + agy first-run sign-in in the demo projects (operator's browser
      needed once; agy's remote-OAuth code flow works through the web terminal) (added 2026-08-26)
- [ ] Integration check for agent-path keyring (DBUS in agent conversations, not just
      kind=shell) — the Phase 1 suite missed a real bug there (added 2026-08-26)
- [ ] createProject boots the microVM synchronously; make async with progress events
      for the Phase 2 UI (added 2026-08-26)
- [ ] Migrate docker SDK from pinned docker/docker v28 to the renamed moby modules (added 2026-08-26)
- [ ] Exercise the entrypoint's vfs fallback path (loop-mount failure) in a test (added 2026-08-26)

- [ ] Phase 3 needs Google OAuth client ID/secret, Cloudflare API token, and BASE_DOMAIN
      from the operator — the old devbox `.env` is not on this machine (added 2026-08-26)
- [ ] Decide backup story for `/var/lib/styrman/home` (restic/borg to GCS?) (added 2026-08-26)

## Low Priority

- [ ] Makefile target for Artifact Registry push if styrman ever leaves this host (added 2026-08-26)

## Ideas / Nice-to-Have

- [ ] Artifacts / files-changed right-rail panes (Antigravity-style) fed by git diff (added 2026-08-26)
- [ ] Mobile-friendly layout for quick check-ins from a phone (added 2026-08-26)
