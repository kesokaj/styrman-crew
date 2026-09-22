---
owner: main
date: 2026-09-22
---
# Preflight — v1.38.0   (2026-09-22)

Range reviewed: v1.37.0..5e974c8

## VERDICT: SHIP

No blocking finding. Three gate rounds: the round-1 reviewer verdict was
REQUEST CHANGES and is cleared, every finding fixed or recorded as accepted
with a reason below.

## Nothing is broken

The release is a whole-project review remediation. The one behaviour a running
deployment could notice is stated under Risk.

## Rows

| Row | Verdict | Artifact |
|---|---|---|
| Reviewer (round 1) | REQUEST CHANGES — 3 medium, 1 low | `reviews/2026-09-22-review-v1.37.0-HEAD.md` (overwritten by round 2) |
| Reviewer (round 2) | **APPROVE WITH NITS** — fixes verified, 1 new low | `reviews/2026-09-22-review-v1.37.0-HEAD.md` |
| Security (range) | FINDINGS — 5 low, none blocking | `reviews/2026-09-22-security-v1.37.0-HEAD.md` |
| Security (delta) | FINDINGS — 3 low + 1 inherited, none blocking | `reviews/2026-09-22-security-v1.38.0-gatefix.md` |
| Verifier (round 1) | PASS at b6e8f0d — superseded | `testing/2026-09-22-verify-v1.38.0-release.md` |
| Verifier (round 2) | **PASS at 5e974c8** | `testing/2026-09-22-verify-v1.38.0-release-round-2.md` |
| Docs lens | APPROVE WITH NITS — 3 findings, all fixed | `reviews/2026-09-22-review-v1.37.0-HEAD-docs.md` |

Commits 16bc84e and 49eb863..0024de7 carry their own code and security reports
(`-review-phase0b-code.md`, `-security-phase0b.md`, `-review-phase3-code.md`,
`-security-phase3.md`), each with an `## Outcome` naming every finding as
fixed, rebutted or left. The round-2 reviewer confirmed the two commits no
report covers — 01a1d47 (one review document, no source) and 7dc7d38
(documentation plus `deploy/.env.example`, where every changed line is a
comment, checked line by line).

## Verification

Every command bare, exit status read before judging (`.agents/rules/verify-bare.md`):

| Command | Exit | Counts |
|---|---|---|
| `go build ./cmd/styrman/` | 0 | — |
| `go vet ./...` | 0 | — |
| `go test ./...` | 0 | **329** pass, 0 fail, 1 skip (311 at the review's start; no test removed) |
| `(cd deploy/caddy/ingress && go test ./...)` | 0 | 55 pass (53 before) |
| `(cd web && npm run check / lint / test)` | 0 | 15 files, 178 tests |
| `deploy/verify-commit.sh HEAD` | 0 | **"OK: commit 5e974c8be336 … safe to tag"** |
| `bash .agents/docs/testing/styrman/integration.sh` | 0 | Tier 2 `passed=112 failed=0 skipped=3` |
| `bash web/e2e/run.sh` | 0 | Tier 4 `passed=93 failed=0 skipped=2` |
| `bash deploy/vulncheck.sh` | 3 | the documented accepted pair, below |

Tier 2 and Tier 4 were run by the main agent at an earlier commit than
5e974c8; the commits between them and the release commit change
`deploy/gcp/vm.sh`, `deploy/verify-commit.sh`, `web/e2e/run.mjs` and four
documents, none of which either suite exercises. Stated rather than implied.

`deploy/verify-commit.sh` is MODIFIED BY THIS RELEASE, so its green run is
evidence for the new gate shape, not for the one v1.37.0 shipped.

## Findings accepted, not fixed

Each was a decision, and this is the record of it.

- **[low/med] `deploy/gcp/vm.sh`** — the remote's answer is still interpolated
  into `vm_ssh "sudo ${ctl} status"`. The characters it can contain are now
  bounded (see Fixed), and the security review probed the new regex with 16
  hostile candidates — `$(id)`, `;id`, a slash in the username, `..`, `/./`,
  trailing and embedded newlines, a UTF-8 accented username, 33 characters —
  all refused, legitimate and 32-character accepted, with `$` confirmed to
  anchor at end-of-string under bash's ERE. Closing the interpolation means
  re-deriving the path remotely (`… && exec sudo "$p" status`) and deleting
  the allowlist: correct, and a redesign rather than a gate fix. Follow-up.
- **[low/med] `deploy/gcp/vm.sh:114`** — an ssh failure after the reachability
  probe passed aborts with ssh's exit code and no output, an undocumented code
  where the header documents 0/1/2/3. This is
  `.agents/rules/remote-check-names-transport-first.md`'s own shape, and it is
  PRE-EXISTING, from 07741b2 (BUG-073), not introduced here. Follow-up.
- **[low/med] `STYRMAN_ALLOW_IP_FORWARD=1` is sticky and invisible after boot**
  — one Warn line, then R12 is unasserted on every later restart. Accepted:
  the knob is the operator's deliberate override and the Warn is at the moment
  it takes effect. Follow-up if it ever becomes a supported mode.
- **[low/high] The operator's residential IP address appears in three
  committed trail files.** The repository is private (verified), so there is
  no live exposure. Removing it from history would mean rewriting published
  history, which this project forbids. **Raised with the operator; their
  decision, not the gate's.**
- **Dependencies:** `govulncheck` exits 3 in both modules with GO-2026-4887 and
  GO-2026-4883, both in `github.com/docker/docker@v28.5.2+incompatible`, both
  `Fixed in: N/A` — the pair `deploy/vulncheck.sh:9-13` records as accepted per
  release. No third ID appeared. `npm audit` reports js-yaml 4.3.1, build-time
  only, not in `dist`.

## Findings fixed during the gate

Round 1 → d2c1620: `specs/api.md` and `testing/styrman/README.md` still stated
the `ip_forward` rule this release reversed; `web/e2e/run.mjs` waited on a
sentinel the typed command already contained, so the terminal's own echo
satisfied it before any file existed; `deploy/verify-commit.sh` discarded npm
output; `deploy/gcp/vm.sh`'s allowlist used a `case` pattern whose `*` matched
`/`, `;`, a space and `$(` alike; `specs/versioning.md` described the
superseded file-name comparison.

Round 2 → 5e974c8: the `vm.sh` refusal message named a condition it had not
measured (a legitimate account with a dot, uppercase or over 32 characters
lands in the same branch); `verify-commit.sh`'s npm failure tail, which is
pasted into committed gate reports, is redacted.

Two of those fixes broke tests on their first attempt and the tests caught
them: a second `vm_ssh` round-trip broke `TestGCPVMStatusExitCodes`, and
rewording the refusal message broke two tests that assert its phrase.

## Trail

- Newest `changes/` entry describes this work. No future `date:` anywhere.
- `ROADMAP.md` "Now" is empty; the plan it named is closed. `TODO.md` empty.
- **Finding — append-only violated once, by the main agent.** Commit 9e50118
  modified `changes/2026-09-22-review-remediation-phase-3.md` (+15/−4) to
  correct an overclaim that Phase 3's security review had found. `changes/` is
  append-only; the compliant route was to leave it and record the correction in
  the Phase 4 entry and the security report's `## Outcome`, which was also
  done — so the correction exists in three places, one of which should not have
  been the original. Not blocking (Step 3 findings do not block). The other two
  hits are clean: b6e8f0d appends ADR-076 with zero deletions, and 07741b2 is
  from the previous session.

## Risk of this release

`cmd/styrman/main.go` now REFUSES to start a containerized server when
`/proc/sys/net/ipv4/ip_forward` cannot be read; it previously warned and
continued. The round-2 reviewer judged this cannot strand either host: the only
new refusal is `ErrNotExist` on a path that necessarily exists once compose's
`net.ipv4.ip_forward=0` sysctl was accepted, the `STYRMAN_ALLOW_IP_FORWARD=1`
escape is plumbed at `docker-compose.yml:110` and documented in four places,
and a tripped guard fails the updater's health check and rolls the deploy back.
The security review judged the fail-closed trade correct and found no
tenant-reachable denial of service: only host root, compose write access or
in-container RCE can make that sysctl unreadable.

**Watch the NUC's updater tick rather than assuming it.**

## What was not checked

- **`web/src/lib/api.ts` and `web/src/components/FileExplorer.tsx` have no unit
  test.** The round-2 verifier reported these as FAIL lines and added that
  Tier 4 "doesn't assert `truncated` either" — **that part is wrong, and is
  corrected here rather than carried forward.** `web/e2e/run.mjs:292` is the
  check "a directory past the listing cap says so"; it creates 5200 files and
  asserts the notice appears for that directory and not for the workspace root.
  It passed in the Tier 4 run (`passed=93`), and suppressing the notice's
  render made it fail — so the behaviour is covered end to end and proved by
  mutation. The verifier grepped for the literal string `truncated` and missed
  a check that asserts the rendered text. What is genuinely absent is a UNIT
  test, because the project has no component-testing framework; standing one up
  is a dependency decision, recorded and not taken.
- `07741b2`'s `deploy/gcp/install.sh` half (BUG-072) is covered by no report in
  this range, the round-2 reviewer's included.
- The fresh-host path was not exercised against a VM this release
  (`.agents/rules/shared-path-learnings.md` wants that said, not implied). The
  GCP instance is deleted; `deploy/gcp/*` changed here, so the next bring-up is
  its first real exercise.
- Tier 2 and Tier 4 ran at an earlier commit than the release commit, as above.
- The security reviews name their own `## Not examined` sections; no path from
  the release diff is named there.
- `specs/styrman/terminal-protocol.md` and most of `api.md`'s response shapes
  were not compared against the Go structs.
