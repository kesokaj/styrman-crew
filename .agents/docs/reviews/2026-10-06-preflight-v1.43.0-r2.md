---
owner: main
date: 2026-10-06
---
# Preflight — v1.43.0, round 2   (2026-10-06)

Range reviewed: v1.42.4..ceb6108

Round 1 (`reviews/2026-10-06-preflight-v1.43.0.md`) was NO-SHIP on one high
security finding. Since ceb6108 only records changed (c1e7768: gate reviews,
verification log, BUG-094; this commit: BUG-094 update, the round-1 Outcome,
this report, the release entry): `git diff --name-only ceb6108 HEAD` lists
files under `reviews/`, `testing/2026-*`, `bugs/` and `changes/` only.

## Blocking findings

None.

- security 1 [high / medium] (the GCP VM trusted the exposed `5HpY…`) —
  cleared by the operator: "GCP key removed" (2026-10-06, their own turn of this
  release session). Not independently verified: dave has no gcloud.

## Rows (from round 1, unchanged)

| Row | Result | Evidence |
|---|---|---|
| Verifier | PASS, reaches HEAD (records only after ceb6108) | `testing/2026-10-06-verify-release-gate-v1.42.4-ceb6108.md`: `deploy/verify-commit.sh ceb6108` rc=0; Tier 2 146/0/3 |
| Reviewer (opus) | APPROVE, no findings | `reviews/2026-10-06-review-release-v1.42.4-ceb6108.md` |
| Security | 1 high (cleared), 2 medium, 3 low | `reviews/2026-10-06-security-release-gate-v1.42.4-ceb6108.md` |
| Docs lens | advisory: 2 medium, 2 low | `reviews/2026-10-06-review-v1.42.4-ceb6108-docs.md` |
| Trail freshness | pass | newest `changes/` entry is this release's; no future `date:`; no testing log deleted (`git log --diff-filter=D v1.42.4..HEAD -- .agents/docs/testing/` empty); `changes/` and DECISIONS.md append-only (`--diff-filter=M` shows only appends: DECISIONS.md ADR-090 and ADR-089 amendment 1 at its end); ROADMAP/TODO untouched (Now empty, nothing queued) |

## Non-blocking findings and their decision

- security 2 [medium] (live v2.3.2 `sh-agent` trusts `5HpY…`) — fixed by this release reaching each project's next boot.
- security 3 [medium] (`install.sh` reinstalls the operator's local signer file) — left: operator (replace the local file); refusing the fingerprint by name in `styrman-ctl`/the updater proposed, the operator's call.
- security lows and docs findings 1-4 — the main agent's docs follow-up commit after the tag.

## What was not checked

- The GCP VM's signer list (operator-confirmed only), the operator's local signer file, GitHub SSH signing keys.
- Browser e2e (no tier owed: web changes are comments only).

VERDICT: SHIP
