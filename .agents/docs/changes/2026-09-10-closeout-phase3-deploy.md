---
owner: main
date: 2026-09-10
---
# Phase 3 closeout: deploy, scripts, and host integration     (date: 2026-09-10)

## Summary

Phase 3 of plan `2026-09-10-todo-closeout.md` closed: eight shared dependencies converged between the root module and `deploy/caddy/ingress` (go-connections 0.5.0→0.8.1, httpsnoop 1.0.4→1.1.0, go-logr/logr 1.4.3→1.4.4, otelhttp 0.68.0→0.71.0, otel-metric/otel-trace 1.43.0→1.46.0, x/sys 0.46.0→0.47.0), with ingress tests green and Caddy image built at v2.11.4; autoupdate timer's inert `Persistent=true` removed; guest check/format scripts moved temp files to `~/.styrman/tmp` instead of workspace root; STYRMAN_TOOL tofu parser gains dedicated case to handle `tofu fmt -check` output; runtime image freshness guard wired into both test suites; daemon.json merge adds `default-address-pools: 172.16.0.0/12` idempotently; firewall service gains `[Install] WantedBy=multi-user.target` and both installers now `systemctl enable` it, so base rules land at boot right after docker.service; timer's `OnBootSec` drops to 5 s as supporting belt. No regressions: go test -race, integration 84/0, e2e 87/0. Agents: reviewer, security, changelog.

## Changes

**A — Dependency convergence**

`deploy/caddy/ingress/go.mod` — Eight packages bumped to match root module: `go get` each to root version, `go mod tidy`, `go test ./...`, `docker build` at v2.11.4 with `caddy list-modules` confirming ingress capabilities and `caddy version` confirming binary. `deploy/verify-commit.sh` divergence list prints empty after the merge.

**K1 — Remove inert timer flag**

`deploy/update/styrman-update.timer` — `Persistent=true` deleted (inert on monotonic timer with `OnBootSec`/`OnUnitActiveSec`).

**K2, K3 — Verified already closed**

No changes: `deploy/host-setup.sh` step 8b was removed entirely (verified 2026-09-10), and `deploy()` calls `log …; return 1` on checkout error (verified at `styrman-update:560`).

**K4 — Temp files outside workspace**

`internal/httpapi/check.go`, `format.go` — `mktemp -p "$HOME/.styrman/tmp"` after `mkdir -p` (guest owns `~/.styrman/`); temp files never appear in project tree, no git exclude needed. Module-scoped checkers that need in-tree temp go under `.styrman-tmp/` added to `.git/info/exclude` at provisioning (server-side, once). `internal/httpapi` test: `TestScriptsKeepTempFilesOutOfTheWorkspace`.

**K5 — Tofu parser case**

`internal/httpapi/check.go:376-399` — `STYRMAN_TOOL tofu` joins the parser's cases. OpenTofu's `fmt -check` emits file:line:col shape like gofmt; parser handles it identically. `TestParseCheckOutputTofu` measures on the test image.

**K6 — Trap trimming**

`internal/httpapi/{check,format}.go` — Exit traps trimmed to what each script creates (no dead `${F}.err`/`${F}.out` globs or trailing `rm -f` covered by EXIT trap).

**L — Runtime image freshness**

`.agents/docs/testing/styrman/ensure-test-image.sh` (new) — Wired into `integration.sh` and `web/e2e/run.sh` before their first check. Rebuilds `STYRMAN_TEST_IMAGE` tag if missing or `docker image inspect --format '{{.Created}}'` is older than newest mtime under `image/` (excluding `image/harness/.git`); fails FATAL on build error. `STYRMAN_TEST_IMAGE_NO_REBUILD=1` skips. Never touches `:dev` or the `STYRMAN_IMAGE` from `deploy/.env`. Prevents BUG-026 regression.

**M(d) — Default address pools**

`deploy/host-setup.sh` — daemon.json merge adds `default-address-pools: [{"base":"172.16.0.0/12","size":24}]` idempotently (tested dry-run against fixture: changed on first apply, unchanged on second). Existing networks keep their subnets; only new ones draw from the pool. Python3 merge test in the script.

**M(f) — Firewall boot ordering**

`deploy/firewall/styrman-firewall.service` — `[Install] WantedBy=multi-user.target` added. Both installers (`deploy/firewall/install.sh`, updater's `host_converge`) now `systemctl enable` it, so base rules land once right after `docker.service` at boot. Timer's `OnBootSec` drops from 15 s → 5 s as supporting margin. Why: project containers restart at boot on their own (restart policy unless-stopped), so the window between daemon start and the first timer tick was real. Docker preserves pre-existing `DOCKER-USER` chain, so `Before=docker.service` ordering is safe. Takes effect on operator's next `sudo bash deploy/firewall/install.sh` (root); until then only the 5 s tick applies.

## Files changed

- `deploy/caddy/ingress/go.mod` — 8 dependencies bumped
- `deploy/update/styrman-update.timer` — `Persistent=true` removed
- `internal/httpapi/check.go`, `format.go` — temp dir logic
- `internal/httpapi/check_test.go` — `TestScriptsKeepTempFilesOutOfTheWorkspace`
- `internal/httpapi/check_parse.go` — tofu case + test
- `.agents/docs/testing/styrman/ensure-test-image.sh` — new
- `.agents/docs/testing/styrman/integration.sh` — wired freshness check
- `web/e2e/run.sh` — wired freshness check
- `deploy/host-setup.sh` — daemon.json merge
- `deploy/firewall/styrman-firewall.service` — install unit, boot ordering
- `deploy/firewall/install.sh` — enable the service
- Updater's `host_converge` — enable the service

## Agents dispatched

reviewer, security, changelog

## Verification

- `deploy/verify-commit.sh` divergence list empty (checked Phase 6 on the committed tree)
- Caddy image builds at v2.11.4 with security module present
- `go test ./internal/httpapi -race` (check/format tests green)
- `integration.sh` 84 passed / 0 failed / 3 skipped on real Kata
- `web/e2e/run.sh` 87 passed / 0 failed / 0 skipped
- Image freshness rebuild observed when `image/skel` touched
- `systemctl show -p After -p Before styrman-firewall.service` shows boot ordering after fixes applied

## Code reviews

- `reviews/2026-09-10-review-closeout-phase3.md` (approve with nits fixed: install directive enabled, tofu parser verified, script tests)
- `reviews/2026-09-10-security-closeout-phase3.md` (two mediums fixed: inert install flag and re-stamp regression; one low noted)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-10-todo-closeout.md`
- Phase 1: `.agents/docs/changes/2026-09-10-closeout-phase1-server.md`
- Phase 2: `.agents/docs/changes/2026-09-10-closeout-phase2-web.md`
