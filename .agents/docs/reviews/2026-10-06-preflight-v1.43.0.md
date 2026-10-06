---
owner: main
date: 2026-10-06
---
# Preflight — v1.43.0 (harness v2.4.1, SHA-256 pin; BUG-094 records)   (2026-10-06)

Range reviewed: v1.42.4..ceb6108

## Blocking

- **[high / medium] security finding 1 — the GCP VM's updater still accepts tags
  signed by the exposed key `SHA256:5HpY…`** (`reviews/2026-10-06-security-release-gate-v1.42.4-ceb6108.md`;
  BUG-094, open). Host state, not code in the range: the release neither
  creates nor worsens it, and the VM would refuse a v1.43.0 tag (signed with
  `2bCW…`, which its list does not yet hold). The gate blocks on any high
  security finding; it is cleared when the operator confirms the VM's
  `/etc/styrman/allowed_signers` no longer lists `5HpY…` (or the VM is
  stopped/deleted), recorded in BUG-094, and the finding is re-checked.

## Rows

| Row | Result | Evidence |
|---|---|---|
| Step 1 — clean tree | pass | `git status --porcelain --untracked-files=all` empty at ceb6108 before dispatch |
| Shadowing agents | none | `.claude/agents/`, `.agents/agents/` empty |
| Verifier (item 1) | PASS, reaches HEAD | `testing/2026-10-06-verify-release-gate-v1.42.4-ceb6108.md`: `deploy/verify-commit.sh ceb6108` rc=0 ("OK: commit ceb61086018c … safe to tag", runtime image `sh-agent v2.4.1`); Tier 2 `integration.sh` rc=0 146/0/3 (matches the README) |
| Verification (item 2) | met | range changed no AGENTS.md command or testing-guide tier definition (testing README edits are descriptive: Tier 5 intro, guard controls, roster line); no plan in the range; ROADMAP Now empty |
| Verification (item 3) | met | no FAIL/BLOCKED log in the range; `testing/2026-10-06-verify-harness-v2.4.1.md` PASS (working-tree run, superseded by the gate log) |
| Reviewer (opus) | APPROVE, no findings | `reviews/2026-10-06-review-release-v1.42.4-ceb6108.md` (COMPLETE) |
| Security | FINDINGS: 1 high, 2 medium, 3 low | `reviews/2026-10-06-security-release-gate-v1.42.4-ceb6108.md` (COMPLETE; `## Not examined` names no release-diff path) |
| Docs lens (advisory) | APPROVE WITH NITS: 2 medium, 2 low | `reviews/2026-10-06-review-v1.42.4-ceb6108-docs.md` |
| Fresh-host path | not owed | the range changes no provisioning step (image build only) |

## Non-blocking findings and their decision

- security 2 [medium] — live projects' v2.3.2 `sh-agent` embeds and trusts `5HpY…` for `sh-agent update`: fixed by shipping this release (v2.4.1 carries no key); to be added to BUG-094's list of trusting places.
- security 3 [medium] — `deploy/gcp/install.sh` installs the operator's local `${SECRETS}/allowed_signers` as the VM's list, likely still holding `5HpY…`: operator replaces that file; refusing the fingerprint by name in `styrman-ctl`/the updater is proposed to the operator.
- security lows (source rests on GitHub; trust records state the target as current; GitHub signing keys unconfirmed): records, to fix with the docs follow-up.
- docs 1 [medium] `gcp.md:79-83` "holds" → "is to list"; docs 2 [medium] the `sh-agent update` caveat for v2.3.2 homes and README "currently carries"; docs 3 [low] `runtime-image.md:288`, `versioning.md:101-106`, testing README:482 name `HARNESS_VERSION` alone; docs 4 [low] the 1,788-file count not re-measured for v2.4.1. Owner: main agent (docs follow-up commit).

## What was not checked

- The GCP VM's real signer list and updater state, and the operator's local `${SECRETS}/allowed_signers` (no gcloud on dave).
- GitHub SSH signing keys (the token gets 404).
- Browser e2e (Tier 3): no tier owed (web changes are comments only, no plan names it).
- Per-package Go/vitest counts inside `verify-commit.sh`.

VERDICT: NO-SHIP
