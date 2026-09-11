# styrman-ctl Phase 1: skeleton, status, dry-run, tests     (date: 2026-09-11)

## Summary

Phase 1 of 4 of the new `styrman-ctl` unified installer/updater/admin tool completed: skeleton script with global flags, `status` verb capturing server health and config state, dry-run harness, and test suite. The script is shellcheck-clean and passes 6 unit tests under `.agents/docs/testing/` harness; production status command confirms deployment healthy (v1.27.1, firewall enabled, Kata/Docker versions). Three cold-read rounds resolved five design gaps (setup-VM filter, clone ownership, KSM tmpfiles path, per-key validation, OPERATOR default), one contradiction (project label), and two minor questions (askpass, hostname source) before operator approved proceeding.

## Type

feature, Phase 1 of 4 — skeleton

## Files changed

- `deploy/styrman-ctl` (new, 330 lines, shellcheck-clean): global flags `--dry-run`, `--yes`, `--confirm WORD`; env-overridable paths (STYRMAN_STATE_DIR, STYRMAN_DEPLOY_DIR, STYRMAN_ETC_DIR, STYRMAN_UNIT_DIR, STYRMAN_BIN_DIR, STYRMAN_SBIN_DIR, STYRMAN_TMPFILES_DIR, STYRMAN_KATA_DIR); helper functions (ask, confirm_word, env_get/env_set, validate, guard_path); verbs: help, status [--json], __validate/`__env-set/`__env-get/`__guard-path (internal test hooks), plus unimplemented stubs for install/update/edit/uninstall
- `internal/config/styrman_ctl_test.go` (new, 6 tests): fakes docker/systemctl/iptables/git/du/hostname/kata-runtime on PATH in t.TempDir(); tests for help/unknown/unimplemented verbs, status --json fields and degraded/uninstalled states, validate table (19 valid + 16 invalid), env_set behaviour/refusals including dry-run, guard_path refusals including symlink at configured root
- `deploy/README.md`: added one-paragraph introduction to styrman-ctl (status now; other verbs with later phases)

## Agents dispatched

explorer (recon brief: `.agents/docs/plans/2026-09-11-installer-recon-brief.md`), reviewer (three cold-read rounds: `.agents/docs/reviews/2026-09-11-coldread-installer-ctl-r{1,2,3}.md`), changelog (this entry)

## Verification

- Shellcheck: clean
- Go vet: `go vet ./...` passes
- Unit tests: `go test -race ./internal/config/` green (6 new tests)
- Production status: `sudo deploy/styrman-ctl status` on live host 2026-09-11 — server healthy 1.27.1, deployed v1.27.1 sha 4686792, updater timer enabled, firewall enabled (7 FWD rules, 1 IN), 0 running microVMs, 1 project dir, 174M state, Docker 29.7.2, Kata 3.31.0, verdict OK
- `--json` output matches `docker inspect`, `systemctl is-enabled`, and state files
- Two defects caught by tests before commit: bare `a || b && c` list ended script under `set -e` when nothing installed (now `if`/`|| true`); counting pipelines failed under pipefail when docker/find failed (now `if`/`|| true`)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-11-installer-ctl.md` (Phase 1 of 4 ticked)
- Cold-read findings: `.agents/docs/reviews/2026-09-11-coldread-installer-ctl-r{1,2,3}.md` (gaps resolved, operator approval on round 3)
