---
owner: main
date: 2026-09-11
---
# Verification — styrman-ctl, Phase 1 (skeleton, status, dry-run harness, tests)   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after the phase commit.

| Step | Command | Result |
|---|---|---|
| Lint | `shellcheck deploy/styrman-ctl` | clean (SC2034 disabled on four variables the later phases use; the SUPERADMINS literal moved into a variable so no directive sits inside a case branch) |
| Unit | `go vet ./... && go test -race -count=1 -run StyrmanCtl ./internal/config/` | 6 tests green; every path under `t.TempDir()`, fake tools on PATH, a production path in any output fails the test |
| Real, read-only | `sudo deploy/styrman-ctl status` and `status --json` on the production host | exit 0, verdict OK: server healthy `styrman-server:1.27.1` / `styrman-caddy:1.27.1`, URL `https://crew.styrman.xyz`, deployed `v1.27.1` sha `4686792…`, updater timer enabled/active with the clone at `4686792 (v1.27.1)`, firewall service enabled (oneshot, inactive between runs) and timer enabled/active, chains STYRMAN-FWD 7 rules / STYRMAN-IN 1, 0 running microVMs, 1 project dir, 174M, Docker 29.7.2, Kata 3.31.0, operator kesokaj — each figure cross-checked against `docker inspect`, `systemctl is-enabled`, `iptables -S` and the three state files |

PASS. Defects caught by the tests before commit: a bare `a || b && c` list
ended the script under `set -e` when nothing was installed (exit 1, no
output); the counting pipelines failed under `pipefail` when `docker`/`find`
failed. Both fixed (`if`, `|| true`).

Not verified: `install`, `update`, `edit`, `uninstall` (later phases); a
fresh-host run (the operator's, or a nested-virt VM).
