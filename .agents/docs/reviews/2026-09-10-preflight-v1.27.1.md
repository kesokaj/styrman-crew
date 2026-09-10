---
owner: main
date: 2026-09-10
---
# Preflight — v1.27.1   (2026-09-10)

Range reviewed: v1.27.0..fcd30ec (release commit with these artifacts
follows; the one SOURCE change is `63aceb1`, `deploy/update/styrman-update`,
+8/−4)

Version: **v1.27.1**, PATCH — fixes released behaviour only: the boot-time
firewall unit shipped in v1.27.0 was never enabled by the updater
(BUG-035 recurrence); the enable now runs on every converge tick.

## Findings, worst first

No open finding. The change is twelve lines of shell, read by main (below
the reviewer threshold) and recorded with its reason in
`changes/2026-09-10-updater-enables-firewall-unit-every-tick.md`. The
security surface is unchanged: the same `systemctl enable` the operator's
`install.sh` already runs, now idempotent on every tick.

## Rows

| Row | Result | Artifact |
|---|---|---|
| Verification of the COMMIT | PASS — `verify-commit.sh HEAD`: "safe to tag", divergence list empty; `bash -n`; shellcheck clean at warning level (one pre-existing INFO elsewhere in the file); go tests | `testing/2026-09-10-verify-v1.27.1.md` |
| Reviewer | Below threshold: the diff read by main; no reviewer dispatched | this report |
| Security | No auth/secret/input/dependency surface changed; no row | — |
| Trail freshness | PASS — newest `changes/` entries are this release's; no future dates; append-only; ROADMAP/TODO touched today | — |
| Docs freshness | PASS — `deploy/remote.md` already describes the boot-time unit; nothing else changed behaviour | — |
| Secrets scan | PASS — no secret-shaped path staged; the broker test log reproduces no values | — |

## Live results recorded in the range (not code)

- Post-v1.27.0 host steps run by main with the operator's authority:
  firewall unit `enabled`, Docker address pool in place, stack survived.
- Credential-broker reachability test on production with two users: every
  probe as expected (`testing/2026-09-10-broker-reachability-live.md`).

## What was not checked

- `deploy/vulncheck.sh` (no dependency change).
- The fix's own proof is the deploy: after the v1.27.1 tick,
  `systemctl is-enabled styrman-firewall.service` must still read
  `enabled` (it does now thanks to `install.sh`; a host without that run is
  what the fix is for — not reproducible here without disabling it, which
  main will do as a controlled check after the deploy: `systemctl disable`,
  wait one tick, expect `enabled`).

**VERDICT: SHIP**

## Pre-gate commits on the branch

`63aceb1`, `ca6c62b`, `60ec4dd`, `fcd30ec` (all pushed before the tag).
