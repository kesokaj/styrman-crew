---
owner: main
date: 2026-09-11
---
# styrman-ctl Phase 2: `install` and `update --setup` commands     (date: 2026-09-11)

## Summary

Phase 2 of 4 completed: the `install` and `update --setup` commands fully implemented with precondition guards, multi-step orchestration, and 9 unit tests (3 new). The script validates six must-fill `.env` keys with re-prompt on error (unattended: from `--env-file`); resolves version from tag or `--version` flag; orchestrates `host-setup.sh` → `docker build` → `docker compose up` → healthcheck wait; prompts for auto-update setup (skipped unattended without secret files or with `--skip-auto-update`). The `update --setup` command materializes credentials securely (root-only temp file, never argv), clones or fetches the deployment repo under a one-shot `GIT_ASKPASS` helper (nothing persisted in git config), validates and installs the `allowed_signers` line, and starts the update timer/service. Two defects caught by tests before commit; Phase 1 reviewer's two nits (`.env` path not env-overridable, unbounded octet/port validation) are closed.

## Type

feature, Phase 2 of 4 — install and auto-update setup

## Files changed

- `deploy/styrman-ctl` (enhanced, +270 lines): `install [--user U] [--version X.Y.Z] [--env-file F] [--gh-token-file F] [--allowed-signers-file F] [--skip-auto-update]` orchestrates preconditions, env-file prompts with validation, version resolution, container build, compose up, and healthcheck wait. `update --setup [--gh-token-file F] [--allowed-signers-file F]` materializes credentials from files or hidden prompts into root-only temp, rewrites origin to https, clones/fetches under `GIT_ASKPASS`, installs `.env` and `allowed_signers`, enables update timer. `ask` now opens `/dev/tty` to decide "no terminal"; locals carry underscores to avoid shadowing caller's variables. `is_loopback_addr`, `is_octet`, `is_port` validation added (closes Phase 1 low finding). `STYRMAN_ENV_FILE` env override added (closes Phase 1 medium finding).
- `internal/config/styrman_ctl_test.go` (3 new tests, 9 total): `TestStyrmanCtlInstallPlan` validates ordered plan (host-setup → build → compose → wait), env file untouched, six env keys prompted and planned for write. `TestStyrmanCtlInstallRefusals` asserts must-fill key missing named in error, bad version rejected, no operator dies, install from inside clone refused. `TestStyrmanCtlUpdateSetupPlan` validates ordered plan (ssh rewritten to https, askpass env on clone line, no credential helper in git config), existing clone fetched, malformed signers rejected before plan, token string absent from all output.

## Agents dispatched

reviewer + verifier (Phase 1 findings: STYRMAN_ENV_FILE and is_octet/is_port fixes verified), changelog (this entry). Reviewer, security, and verifier over Phase 2 commit to follow.

## Verification

- Shellcheck: clean
- Go vet: `go vet ./...` passes
- Unit tests: `go test -race ./internal/config/` green (9 tests, 3 new for Phase 2)
- Real, read-only dry-run on production host: `sudo deploy/styrman-ctl install --dry-run --version 1.27.1 --skip-auto-update` outputs ordered plan (host-setup → build → compose → wait healthy), nothing executed
- Real, read-only dry-run on production host: `sudo deploy/styrman-ctl update --setup --dry-run` outputs ordered plan (token install → fetch/clone → chown → .env copy → signers install → enable timer → start service), token string absent from output, nothing executed
- Defects caught by tests before commit: `ask` read `/dev/tty` on permission bit (now opens device to decide); `ask` local `v` shadowed caller's target (locals now `_v`); unattended error named variable not key; `is_loopback_addr` lost `BASH_REMATCH` to helpers (now copies groups first)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-11-installer-ctl.md` (Phase 2 of 4 ticked)
- Phase 1 review: `.agents/docs/reviews/2026-09-11-review-styrman-ctl-phase1.md` (APPROVE WITH NITS: medium/low findings closed this phase)
- Phase 1 verifier: `.agents/docs/testing/2026-09-11-verifier-styrman-ctl-phase1.md` (PASS, 7 steps)
- Phase 2 verification: `.agents/docs/testing/2026-09-11-verify-styrman-ctl-phase2.md` (PASS, 2 dry-runs)
