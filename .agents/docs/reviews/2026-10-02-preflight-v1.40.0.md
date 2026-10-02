---
owner: main
date: 2026-10-02
---
# Release gate — v1.40.0   (2026-10-02)

Range reviewed: v1.39.0..7540d53

Nothing in the range blocks. Listed first is what is known to be imperfect in
what ships.

## Findings carried into the release (none blocking)
| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | low / medium | `web/src/components/TerminalPane.tsx:352-355`; `specs/styrman/terminal-protocol.md:175-177` | A terminal pane that has NOT given up keeps the keys typed while it reconnects; if the machine sleeps inside that retry window, the reconnect on resume sends keys that may be hours old. The comment and the spec call them "seconds old", which is false across a sleep. Pre-existing behaviour of the queue (not new in this range); needs a running project and a sleep inside the ~90 s window. | accepted: shipped as it is and named in the release entry. A third change to the same queue in one gate is where the rounds rule says to stop; the fix (drop queued keys older than ~30 s) is the handoff's first Next Step. |
| 2 | low / medium | `specs/styrman/versioning.md:281-286`, `deploy/gcp.md:79-83` | A second release-signing key (the operator's Mac) is trusted by the GCP VM's updater; the docs now record it, no ADR decides it. | left: operator — whether that key is a sanctioned signer is theirs to decide (handoff). This release is signed with the original key, `SHA256:5HpYQJjpG+…`, which both hosts list. |
| 3 | low / low | `specs/styrman/deploy.md:160-161`, `specs/styrman/guide.md:693` | The docs name `docker restart styrman-caddy` as the recovery for a host not yet on the ingress fix; `.agents/rules/no-live-workarounds.md` says not to offer host-side workarounds. | left: operator — it records the step the operator chose for a full-site outage on 2026-10-02 ("Yes, restart it now (Recommended)"); the rule was written about broken projects. Keep or reword is theirs (handoff). |
| 4 | low / low | `deploy/gcp.md:161-176` | The "Claude via Vertex AI" steps may omit the Application Default Credentials step. Unconfirmed. | left: operator — their own procedure from the previous session (handoff). |
| 5 | low / high | `web/` build chain | `js-yaml@4.3.1`, GHSA-2883-xcg3-v3hh, through `@tanstack/start-plugin-core`. Outside the range, build-time only; nothing of it is in `web/dist/client`. | left: the next task (handoff). |

## Rows

**Verification — met.**
1. A verifier log reaches the gated head and passes: `testing/2026-10-02-verify-v1.40.0-release-r2.md`, `owner: verifier`, `Range reviewed: v1.39.0..7540d53`, `## Status: PASS`. After `7540d53` only records and the release commit's own files change.
2. What the release changed was run. The files that define verification changed in the range — `testing/styrman/integration.sh` (state dir on disk, `wait_seed`, seven checks) and `web/e2e/run.mjs` (an explicit browser context, four checks) — and both were quoted to the verifier and run as they are on disk. Tiers the plan (`plans/2026-10-02-tool-updates-freeze-harness-v2.md`) names: unit, integration, e2e, and one hand run.
   - unit: `go build`, `go vet`, `go test -count=1 ./...` → 0 (pass 394, skip 1; 336 test names, none removed); the ingress module → 0 (pass 63, among them the three masked-load tests); web `check`, `lint`, `test` → 0 (16 files, 189 tests).
   - builds: `deploy/Dockerfile.server` → 0; `deploy/caddy` → 0; the runtime image through the suite's `ensure-test-image.sh`.
   - integration: `passed=119 failed=0 skipped=3 aborted=0` (the three SKIPs are the host-firewall previews); `PASS boot to ready within R1's 15 s hard limit (measured 3.21s on ext2/ext3; soft target 5 s)`.
   - e2e: `passed=97 failed=0 skipped=2`; the rebuild reproduced `web/dist/client`.
   - the hand run, the main agent's tier: `testing/2026-10-02-1903-verify-tool-updates-hand-run.md`, `Range reviewed: v1.39.0..7540d53`, `## Status: PASS`, 7 of 7 rows (a real booted project: `agy update` 1.2.2 → 1.2.15, `claude update` 2.1.270 → 2.1.287).
3. No failure is left standing: every log in the range is PASS. The earlier logs — `testing/2026-10-02-1744-…-phase-1.md`, `-1806-…-phase-2.md`, `2026-10-02-verify-v1.40.0-release.md` (at `4dd51bf`), `2026-10-02-1840-…-hand-run.md` — are PASS and superseded by later runs, not cleared failures.

**Reviewer — met.** `reviews/2026-10-02-review-v1.39.0-4dd51bf-code.md` (opus; `## Status: COMPLETE`, APPROVE WITH NITS) and `reviews/2026-10-02-review-4dd51bf-7540d53-code.md` (opus; COMPLETE, APPROVE WITH NITS) cover the range between them. Their findings: one medium (the README claimed v1.39.0 ran on the NUC; the second signer undocumented) — fixed in `7540d53`; lows fixed in `7540d53` or carried above (2, 3, 4). Before the gate the same code had two rounds of `/review` (`reviews/2026-10-02-review-1fe28a0-4460e1b-merged.md`).

**Security — met.** `reviews/2026-10-02-security-v1.39.0-4dd51bf.md` and `reviews/2026-10-02-security-4dd51bf-7540d53.md`, both `## Status: COMPLETE`, both with a `## Not examined` section that ends "Release-diff paths: none left unread". No high, no medium at the gate. The one medium of the whole change — agy's directory world-writable — was found by the first `/review` round, fixed in `39ccf89` (BUG-086, ADR-080) and verified fixed in three later security reads. Lows: the stale keystroke replay after a pane gave up — fixed in `7540d53` with a browser check that failed first; carried above: 1, 2, 5.

**Docs lens (advisory).** `reviews/2026-10-02-review-v1.39.0-4dd51bf-docs.md`, COMPLETE, APPROVE WITH NITS: four lows, all fixed in `7540d53`.

**Trail freshness.**
- The newest `changes/` entry is this release's; the three before it are this work's.
- No artifact under the docs root is dated in the future.
- No verification log was deleted in the range.
- `changes/` has no modified past entry. `architecture/DECISIONS.md`: appended ADR-077 to ADR-080, and ONE edit to a past line — ADR-077's heading gained its "superseded by ADR-080" marker, the marker ADR-062 and ADR-073 carry (a finding of the security review).
- `ROADMAP.md` and `TODO.md`: `ROADMAP.md` "Now" named the plan during the work and is empty again; `TODO.md` is empty.

**Project rules.**
- `.agents/rules/shared-path-learnings.md`: the shared path changed (the runtime image, the entrypoint, `styrman-ctl install`, the ingress). **Fresh-host path unexercised** — no new VM was brought up for this release. `host-setup.sh` changed only in the text it prints.
- `deploy/verify-commit.sh` and `bash deploy/vulncheck.sh`: see below.
- `REQUIREMENTS.md`: unchanged; R5's wording still holds (the entrypoint installs the harness and never downgrades). The Latency SLA's 15 s hard limit is what moved the harness install behind ready.

| Command | Exit | Result |
|---|---|---|
| `bash deploy/vulncheck.sh` | 3 | the documented accepted pair, GO-2026-4887 and GO-2026-4883 in `github.com/docker/docker@v28.5.2+incompatible`, both `Fixed in: N/A` — the same as at v1.38.0; nothing new |
| `deploy/verify-commit.sh HEAD` (at `7540d53`) | 0 | **"OK: commit 7540d5330a92 builds and tests clean in isolation — safe to tag."** — the commit in a detached worktree: Go build/vet/test, the ingress module, `web/dist` freshness, the frontend trio, the server image, and the runtime image ("carries sh-agent v2.3.2: ### Skills (36) ### Subagents (8)"). The release commit adds only records, `README.md` and `ROADMAP.md` on top of it; it is verified the same way before the tag is signed. |

## What was not checked
- **The fresh-host path** (above).
- **A real sleep, a real network change, a real hidden tab**: the browser suite emulates all three at the page's own APIs.
- **A real host reboot with the fixed ingress.** The fix was reproduced on the real caddy build in containers (no network → retries; network attached → loads), not by rebooting a host.
- **The idle reaper stopping a project under a pinging tab**: `TestApplyControl` proves a ping never reaches the activity clock; the reaper's timer was not driven.
- **The content of harness v2.3.2** (reviewed in its own repository's gate) and of the bumped base images; `web/dist/client` was compared with its source by the security reviews and reproduced by the verifier's rebuild, not read line by line by the code reviews.
- **The two `/review` lens reports and every agent's "Not examined" items** are in their own files; none names a path of the release diff.
- **Deploy on the NUC**: this host's updater cannot fetch (BUG-084); it takes the tag only after the operator replaces `/etc/styrman/gh-token`.

## VERDICT: SHIP
