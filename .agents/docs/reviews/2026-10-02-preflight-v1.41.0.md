---
owner: main
date: 2026-10-02
---
# Release gate — v1.41.0   (2026-10-02)

Range reviewed: v1.40.0..adef20e

Nothing in the range blocks. Listed first is what is known to be imperfect in
what ships. All of it is low; each item is owned by the first commit after the tag.

## Findings carried into the release (none blocking)
| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | low / high | `deploy/update/styrman-update:147` | The status path's plain-character rule uses bracket ranges, so it depends on the locale. Under the UTF-8 locale systemd passes the unit on this host, `/x/é/…` is accepted. Newlines are refused in every locale; the pinned path is plain. The specs' "any other character is refused" overstates the rule until the fix. (code and security round 4) | left: main, post-tag (`LC_ALL=C`, a UTF-8 hook test) |
| 2 | low / high | `internal/orchestrator/ports.go` `GuestGoingAway` | The 20 s ceiling bounds each parked request (about 200 Docker inspects), not how many one tenant can park. Only signed-in, allowlisted users reach the endpoints. (security round 4) | left: main, post-tag (a per-user in-flight cap or a singleflight) |
| 3 | low / high | `internal/config/styrman_update_status_test.go` `trustedTempDir` | It checks TMPDIR's ancestors' modes, not their owners, so a TMPDIR below another user's directory fails tests without naming the cause. Test-only. (code round 4) | left: main, post-tag |
| 4 | low / medium | ADR-082 | How the two release-signing keys are protected (passphrase, agent, hardware), and whether the NUC's `allowed_signers` lists the Mac key, are not recorded. Root on the NUC and the operator's own knowledge are needed. (security rounds 2 and 3) | left: operator, handoff Next Steps |
| 5 | low / high | `web/` build chain | `js-yaml` GHSA-2883-xcg3-v3hh through the TanStack build tooling. Outside the range, build-time only; carried from v1.40.0. | left: operator (v1.40.0 handoff) |

## Rows

**Verification — met.**
1. A verifier log reaches HEAD and passes: `testing/2026-10-02-verify-release-adef20e.md`, `owner: verifier`, `Range reviewed: v1.40.0..adef20e`, `## Status: PASS`. After `adef20e` only records change (this report, the round-4 reviews' Outcome sections, the release entry, `README.md`, `ROADMAP.md` and the plan's ticks, all in the release commit).
2. What the release changed was run. The plan names unit, integration (Tier 2) and e2e (Tier 4); no tier is the main agent's own. The log shows:
   - `deploy/verify-commit.sh adef20e` → "OK: commit adef20e89ca9 builds and tests clean in isolation — safe to tag". This covers Go build, vet and `-race` tests for both modules, gofmt, the `web/dist` freshness rebuild, web check, lint and test, and the server image build.
   - `deploy/vulncheck.sh` → rc 3, only the accepted GO-2026-4887/4883.
   - Targeted: 13 `TestStyrmanUpdateStatus*` and 7 `TestListeningPorts*` (`-race`) PASS.
   - Tier 2: `passed=122 failed=0 skipped=3` (125 checks; the 3 SKIPs are the host-firewall previews).
   - Tier 4: `passed=99 failed=0 skipped=2` (101 checks); the rebuild changed nothing in `web/dist`.
   - Added skips: two `t.Skip`, each with its reason (no bash; running as root).
   The files that define verification changed in the range: `integration.sh` (three updater-status checks, a server env line), `web/e2e/run.mjs` (a browser-view dead-socket check, a Settings notice check, the fixture's browser sockets) and `web/e2e/run.sh` (exports `STYRMAN_UPDATE_STATUS_FILE`). Each new check was run once red against the build before its change: Tier 4 `passed=97 failed=1`, then `passed=98 failed=1`, each that check alone; Tier 2 `passed=120 failed=2`, the two checks that expect an `updater` key.
3. No failure is left standing.
   - `testing/2026-10-02-verify-stale-keys-browser-liveness-updater-status-phase-2.md`: its first run was FAIL, Tier 4 "no uncaught client errors" on one `/ports` 500. That was BUG-088, fixed in `5571b57`, and the same log, updated in place (`Previous run:`), is PASS.
   - `testing/2026-10-02-verify-release-v1.40.0-57179a4.md` is IN PROGRESS. It holds no failure: its two completed commands, `verify-commit.sh 57179a4` and vulncheck, passed. The main agent stopped it when the gate's docs lens required a spec change, and `testing/2026-10-02-verify-release-624808d.md` (PASS) and the `adef20e` log above supersede it.
   - Every other log in the range is PASS: phase 1, review fixes, 624808d.

**Reviewer — met.** Every commit in the range has a COMPLETE code review on opus:
- `reviews/2026-10-02-review-v1.40.0-5571b57-code.md`: REQUEST CHANGES; medium fixed in `57179a4`.
- `reviews/2026-10-02-review-5571b57-57179a4-code.md`: APPROVE WITH NITS; fixed in `624808d`.
- `reviews/2026-10-02-review-57179a4-624808d-code.md`: APPROVE WITH NITS; fixed in `adef20e`.
- `reviews/2026-10-02-review-624808d-adef20e-code.md`: APPROVE WITH NITS; carried items 1 and 3.
Each report ends with an `## Outcome` section. No high finding is open, and no REQUEST CHANGES is open: the one there was is answered and re-reviewed.

**Security — met.** Four COMPLETE reports, each with a `## Not examined` section that names only the generated `web/dist/client`, which was scanned for secrets:
- `reviews/2026-10-02-security-v1.40.0-5571b57.md`
- `reviews/2026-10-02-security-5571b57-57179a4.md`
- `reviews/2026-10-02-security-57179a4-624808d.md`
- `reviews/2026-10-02-security-624808d-adef20e.md`
No high or medium finding in any of them. Every finding is fixed or carried above (items 1, 2 and 4).

**Docs lens.** Each commit has one:
- `reviews/2026-10-02-review-v1.40.0-5571b57-docs.md`: one low, fixed.
- `reviews/2026-10-02-review-5571b57-57179a4-docs.md`: one medium, a false `api.md` row that this round had added, deleted in `624808d`; one low, fixed.
- `reviews/2026-10-02-review-57179a4-624808d-docs.md`: two lows, fixed in `adef20e`.
- `reviews/2026-10-02-review-624808d-adef20e-docs.md`: APPROVE, no findings.

**Review cap.** The cap of three rounds was reached at `624808d` with five lows open. The operator chose (2026-10-02, this session): "Fix all five, then ship". The fourth round's lows are carried above.

**Trail freshness.** All pass. The newest `changes/` entries describe this range. No `date:` is in the future. No verification log was deleted. `changes/` was never modified, and `DECISIONS.md` was only appended to (ADR-081, ADR-082 and its custody paragraph). `ROADMAP.md` was touched in the range.

**Shadowing agents.** None: `.claude/agents/` and `.agents/agents/` are empty.

## What was not checked
- **Fresh-host path unexercised.** The release changes the shared host path: the updater unit gains `Environment=STYRMAN_UPDATE_STATUS_FILE=`, the compose file a read-only `/etc/styrman/status` mount. No new VM was brought up. On this host a throwaway compose service showed that Docker creates a missing bind source as an empty root-owned 0755 directory that is read-only inside.
- A real root updater tick writing the status file on a host. The hooks run the writer and the exit decision, not the tick. This host cannot take the release until BUG-084's token is replaced.
- The status file appearing after a release, and the Settings notice on a live host.
- A real sleep for the keystroke age rule (the clock is injected); the 25 s probe interval and the `online` trigger in a real browser.
- The GCP VM: not reachable from this machine (no `gcloud`). Its own updater takes every signed tag.

VERDICT: SHIP
