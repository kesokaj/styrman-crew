# TODO

Deferred work items. Not blocking current work, but should be addressed later.

## High Priority

- [ ] Rename the project directory `~/workspace/super-devbox` → `~/workspace/styrman`
      (do between agent sessions — renaming under a live session breaks its cwd) (added 2026-08-26)

## Medium Priority

- [ ] Phase 3 needs Google OAuth client ID/secret, Cloudflare API token, and BASE_DOMAIN
      from the operator — the old devbox `.env` is not on this machine (added 2026-08-26)
- [ ] Decide backup story for `/var/lib/styrman/home` (restic/borg to GCS?) (added 2026-08-26)

## Low Priority

- [ ] Makefile target for Artifact Registry push if styrman ever leaves this host (added 2026-08-26)

## Ideas / Nice-to-Have

- [ ] Artifacts / files-changed right-rail panes (Antigravity-style) fed by git diff (added 2026-08-26)
- [ ] Mobile-friendly layout for quick check-ins from a phone (added 2026-08-26)
