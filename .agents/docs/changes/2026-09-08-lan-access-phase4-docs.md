---
owner: changelog
date: 2026-09-08
---

# Per-project LAN access — documentation and records (Phase 4)

Date: 2026-09-08. The documentation for per-project local-network access
(ADR-065) and the project records that go with it. Docs-only: no source
changed in this phase, and no review gate applies beyond a reread; the code
reviews are in the Phase 1 and Phase 2 entries. Committed together with the
Phase 2/3 records (bug records BUG-014..019, the review files, the plan
ticks).

## What changed, by file

- `.agents/docs/deploy/remote.md` — Prerequisites gains the host firewall
  half: `sudo bash deploy/firewall/install.sh` once per host, from
  `/opt/styrman` at the deployed tag, what it installs, that `firewall.conf`
  is discovered once and never touched by the updater, and that a server
  container older than the spool mount keeps seeing the plain directory until
  it is recreated. Procedure notes that the updater's `host_converge` keeps
  the firewall files current. Verify gains the four commands (`iptables -S
  STYRMAN-FWD | head`, the two units' status, `cat
  /run/styrman-firewall/status`, `journalctl -u styrman-firewall`).
- `.agents/docs/specs/styrman/versioning.md` — a boxed note between release
  steps 4 and 5: host-side files ship by `host_converge` from the verified
  SHA (report-only under `--check`); the first install per host is the
  operator's `install.sh` run.
- `deploy/README.md` — a `firewall/` paragraph (script, units, `install.sh`,
  `test.sh`, who calls what).
- `.agents/docs/specs/styrman/deploy.md` — the isolation section's title
  gains "per-project LAN exception since ADR-065"; the closing paragraph
  ("applied by the operator running host-setup.sh") is replaced by two: where
  the script lives now and how it ships (install.sh + host_converge, the
  six-day drift it closes), and the exception itself (the conf grammar and
  its validation, the name-only marker, the spool, the exact head-of-chain
  RETURN and why RETURN/`-s`/`-o`, delete-by-spec as the one deletable rule
  class, the status grammar, the 503 gates, the path unit's latency and
  revive backoff, what stays closed, the two reserved conf keys). The
  "What host-setup.sh pins/verifies" list gains the §5c firewall install.
- `.agents/docs/specs/styrman/guide.md` (done in Phase 3, listed here for
  completeness) — §2 isolation bullet, a §4 "Local network" bullet, a §5
  troubleshooting row.
- `README.md` — the "Hard isolation" bullet: "nor the host; the LAN only when
  an admin allows it for that one project (ADR-065)", applied by the same
  root script within about a second of the switch.
- `AGENTS.md` — the architecture bullet on project networks describes the
  root-owned firewall, the name-only marker, the spool and the status file
  (the server never touches iptables); the integration count line reads 82.
- `.agents/docs/testing/styrman/README.md` — Tier 1: 80 tests / 8 files
  with `lan.test.ts` described; Tier 2: 82 checks, 4 auto-SKIP, the LAN
  block and the opt-in live block described; Tier 4: 86 checks + gate, the
  three new checks described; the ADR-052 section gains how a root script
  is tested WITHOUT root (`deploy/firewall/test.sh` under `unshare -Urnm`
  with a private sysfs, wrapped by `go test`, host chain compared via
  `sudo -n`; `install.sh` is `bash -n` + shellcheck + `--dry-run` only).
- `.agents/rules/guest-writable-paths.md` — one bullet: the marker dir is
  server-only and root-read by name; root refuses symlinks and asserts the
  owner (BUG-009's rule).
- `deploy/.env.example` — a comment: `STYRMAN_FIREWALL_DIR` is set by
  compose, not here; the host half is installed once with `install.sh`; the
  LAN prefix lives in `/etc/styrman/firewall.conf`.
- `TODO.md` — one Inbox item listing the ADR-065 follow-ups (a)–(g): the
  `lanPolicy` knob, the two reserved conf keys needing their own ADR (with
  the `! -o sdb-+ ! -o <ingress br>` guard shape), per-project LAN DNS,
  pinning `default-address-pools`, verifying live-restore across a daemon
  restart, running the base section before docker, and the transitive
  SSH-back path as an accepted documented risk. The 2026-09-02 "DONE"
  deny-list item keeps its tick and gains a correction: the repo change
  shipped, production never received it until ADR-065's delivery path.

## Records written alongside (other agents, same commit)

- bugs: BUG-014..BUG-019 (`.agents/docs/bugs/`, INDEX.md) — the Phase 2/3
  review defects and the e2e-caught switch gap.
- reviews: `2026-09-08-review-lan-phase2-3.md`, `2026-09-08-security-lan-phase2-3.md`.
- plan: `.agents/docs/plans/2026-09-08-per-project-lan-access.md` — Phase 2,
  3 and 4 boxes ticked; the Derive row corrected for the "cannot confirm"
  branch; the PUT contract corrected for the throttled no-op kick.

## Requirements

R12 as amended 2026-09-08 (ADR-065). Nothing else touched.

## Verification

Docs-only: reread against the code as committed in the Phase 2 and 3
commits. Every count cited was measured this session (integration 78/0/4,
e2e 84/0/2, vitest 80/80, test.sh 83/83).

## Agents dispatched

- `changelog` wrote a first draft of this entry that described files,
  sections and TODO changes that do not exist; the author replaced it with
  this one. Recorded so the next reader knows the entry was checked line by
  line against `git diff`, and as input for the retro.

## Still open (operator / Phase 5)

The root actions on the host (converge the drift / `deploy/firewall/install.sh`
from `/opt/styrman` at the tagged commit), the manual applied-state
verification on production, the verifier log, the release.
