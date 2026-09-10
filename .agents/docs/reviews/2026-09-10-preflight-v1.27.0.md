---
owner: main
date: 2026-09-10
---
# Preflight — v1.27.0   (2026-09-10)

Range reviewed: v1.26.0..fe3d80f (release commit with these artifacts follows;
the last SOURCE change is `0201b8c` — `git diff --stat 0201b8c..HEAD` outside
the docs tree is empty)

Version: **v1.27.0**, MINOR — new behaviour for a user (provisioning state,
terminal revocation, checkable-only gutter, tofu diagnostics), nothing an
existing deployment must react to. No file tracks the version; the signed tag
is the bump.

## Findings, worst first

No open blocking finding. Every finding of the eleven phase reviews and the
three cold-reader rounds was fixed or rebutted in writing in its report.

- **ea6928a** (the stopped-marker move, before the close-out) was a 13-line
  diff read by main, not by the reviewer; the Phase 5 seam tests now cover
  that path (`TestEnsureRunningLeavesARunningContainerAlone` asserts the
  marker is lifted).
- **Operator-side, after deploy:** the boot-time firewall unit is enabled
  only by the operator's next `install.sh` run; until then the 5 s first
  tick is what narrows the boot window. Recorded in the ADR-067 amendment,
  `deploy/remote.md`, and the handoff.
- **BUG-038:** one e2e run exited 2 after a green suite with a bash parse
  message from its trap; the traced rerun on the final tree was clean.
  Observed, not root-caused; not a product defect.

## Rows

| Row | Result | Artifact |
|---|---|---|
| Verification of the COMMIT | PASS — `deploy/verify-commit.sh 0201b8c`: "safe to tag", **0 WARN** (the eight dependency divergences are gone); darwin build; go test -race all packages; vitest 11/147; ingress tests | `testing/2026-09-10-verify-closeout-final.md` (+ per-phase logs `-closeout-phase{1,2,4,5}.md`, `-stopped-marker.md`) |
| Suites on real hardware | PASS — integration 84/0/3 (`.agents/scratch/integration-2026-09-10-final.log`); e2e 87/0/2 exit 0, no leaks (`e2e-2026-09-10-final.log`, traced) | cited in the final verifier log |
| Reviewer | APPROVE per phase (1: nits fixed; 2: nits fixed; 3: request changes → fixed; 4: approve; 5: nits fixed); cold reader 3 rounds, gate closed by the operator on item J | `reviews/2026-09-10-review-closeout-phase{1..5}.md`, `-coldread-todo-closeout.md` |
| Security | Per phase: 1 three lows fixed; 2 five lows, two fixed; 3 two mediums + one low, all fixed; 4 three lows, one fixed, two documented; 5 lows, documented + two assertions added | `reviews/2026-09-10-security-closeout-phase{1..5}.md` |
| Trail freshness | PASS — newest `changes/` entries describe the range; no future dates; `changes/` and `DECISIONS.md` append-only (the one `M` on DECISIONS.md is the ADR-067 amendment, +25/−0); ROADMAP/TODO touched in 6 commits | — |
| Docs freshness | PASS — `specs/styrman/api.md` (provisioning, 4003, tool details), `guide.md`, `deploy/remote.md` (post-upgrade steps, legacy note), testing README (counts, image freshness) | — |
| Secrets scan | PASS — staged diffs scanned on every commit; the only hits are test fixtures (`BEGIN OPENSSH PRIVATE KEY` literals in tests) | — |

## What was not checked

- A fresh Google sign-in after the bump (the operator's).
- The credential-broker reachability test from a LAN project on the compose
  deployment — still owed (`reviews/2026-09-09-security-adr-067.md`).
- The boot-time firewall ordering on the real host (needs the operator's
  `install.sh`; the unit reads `static` until then).
- The create-path `realBelow` re-check has no unit test (noted gap); the
  fake Docker client is minimal by design (documented in it).
- `govulncheck` on the pinned builder: run by the Phase 3 security review
  (only the two accepted docker/docker advisories); not re-run for the
  final commit (no dependency change after Phase 3).

**VERDICT: SHIP** — on gated commit `0201b8c` ("safe to tag" in isolation,
0 WARN), suites green on real hardware, every review row closed in writing.

## Pre-gate commits on the branch

`ea6928a`, `d286f2b`, `7dae197`, `3ce8a37`, `46f3ec2`, `0201b8c`, `fe3d80f`
(all pushed to `origin/main` before the tag).
