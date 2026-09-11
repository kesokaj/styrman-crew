---
owner: main
date: 2026-09-11
---
# styrman-ctl Phase 3: `update` and `edit` commands     (date: 2026-09-11)

## Summary

Phase 3 of 4 completed: the `update` and `edit` commands fully implemented. The `update` command requires `systemctl is-enabled styrman-update.timer = enabled` (else dies with `--setup` hint), runs `systemctl start styrman-update.service` (a oneshot blocking until the tick ends), prints journal tail and status; `--redeploy` clears a latched failed-version by running `styrman-update --force` with `STYRMAN_UPDATE_STATE_DIR` and `STYRMAN_DEPLOY_DIR` explicit. The `edit` command presents a numbered table of 19 editable keys (secrets masked, computed keys read-only), accepts interactive prompts (three re-tries on validation) or unattended `KEY=VALUE` arguments; validates everything before any write, writes both `.env` copies when the clone exists, re-applies by recreating the stack at the currently deployed version without `--build`. Dry-run mode now prints `+ env_set FILE KEY` without touching the file (Phase 1/2 tests updated). Two new unit tests (`TestStyrmanCtlEditPlan`, `TestStyrmanCtlUpdatePlan`) + Phase 1 "update is a stub" assertion removed. Real read-only dry-runs on production host verified both commands produce the planned output and change nothing.

## Type

feature, Phase 3 of 4 — update and edit

## Files changed

- `deploy/styrman-ctl` (enhanced, +160 lines): `update [--redeploy]` requires enabled timer (or `--setup` hint), runs oneshot service, prints last 20 journal lines and status; `--redeploy` invokes `styrman-update --force` with state/deploy dirs explicit. `edit [KEY=VALUE …] [--no-apply]` presents 25 keys (19 editable + 6 computed read-only) in `.env.example` order, prompts (secrets masked), validates before any write, writes both copies when clone exists, re-applies at deployed version without `--build`, waits healthy and prints status; unattended from arguments. `env_set` under `--dry-run` prints `+ env_set FILE KEY` and touches nothing (previously created/removed a temp file).
- `internal/config/styrman_ctl_test.go` (2 new tests, 11 total): `TestStyrmanCtlUpdatePlan` asserts plain `update` starts service + prints journal, `--redeploy` uses `--force` with envs explicit, both refuse from non-root. `TestStyrmanCtlEditPlan` asserts both copies written per key, recreate from clone at deployed 1.27.1 without `--build`, file untouched on `--no-apply`, four key types refuse before any write (computed key, unknown key, invalid duration, hostname CADDY_ADMIN), no-terminal and no-.env hints. Phase 2 test's "plain update is a stub" assertion removed.

## Agents dispatched

verifier (Phase 2, PASS, 6 steps on production read-only), changelog (this entry). Reviewer, security, and verifier over Phase 3 commit to follow.

## Verification

- Shellcheck: clean
- Go vet: `go vet ./...` passes
- Unit tests: `go test -race ./internal/config/` green (11 tests, 2 new for Phase 3)
- Real, read-only dry-run on production host: `sudo deploy/styrman-ctl edit --dry-run STYRMAN_IDLE_TIMEOUT=9h` outputs plan: `env_set /opt/styrman/deploy/.env STYRMAN_IDLE_TIMEOUT`, then `docker compose -f /opt/styrman/deploy/docker-compose.yml --env-file /opt/styrman/deploy/.env up -d` at v1.27.1, wait healthy, status; nothing changed
- Real, read-only dry-run on production host: `sudo deploy/styrman-ctl update --dry-run` outputs plan: `systemctl start styrman-update.service`, then `journalctl -u styrman-update.service -n 20`; nothing changed
- Real, read-only dry-run on production host: `sudo deploy/styrman-ctl update --redeploy --dry-run` outputs plan: `env STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update STYRMAN_DEPLOY_DIR=/opt/styrman /usr/local/bin/styrman-update --force`, then journal; nothing changed
- Not verified: a real `edit` (production `.env` not to be changed for test) and a real updater tick (nothing newer than v1.27.1 to deploy); both are the operator's step when wanted.

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-11-installer-ctl.md` (Phase 3 of 4 ticked)
- Verifier over Phase 2: `.agents/docs/testing/2026-09-11-verifier-styrman-ctl-phase2.md` (PASS, 6 steps, read-only on production)
- Phase 3 verification: `.agents/docs/testing/2026-09-11-verify-styrman-ctl-phase3.md` (PASS, 4 steps)
