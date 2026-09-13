---
owner: main
date: 2026-09-13
---
# Preflight — v1.29.0 (2026-09-13)

Range reviewed: v1.28.0..b6a287d

Release: **MINOR** — the user-visible behaviour of a released surface changes
(the Browser pane pauses its screencast while hidden and pushes no viewport
onto the agents' browser; Retry shows the launcher's full log through the
REST start; an open browser relay is revoked with the owner (4003); AltGr
characters type; a server that cannot learn its bridge address fails the
start closed) and nothing an existing deployment must react to. Requested
by the operator: "do a minor and execute a /preflight and then git commit
push".

## VERDICT: SHIP

Every tier is green on the release tree (b6a287d) and every review round's
findings are fixed or rebutted in writing (table below). `deploy/verify-commit.sh`
runs on the release commit before the tag; its result is appended at the end
of this file.

## Findings (FAIL-first)

None blocking.

- **[low / high] Two coverage gaps recorded, not fixed.** (a) The pane's
  `revoked` phase (close 4003) has no automated test: the e2e runs in
  AuthMode=none, where the re-check is nil by design, and `web/` has no
  React renderer for a unit test (`.agents/docs/testing/2026-09-13-verify-v1.29.0.md`,
  first section). The server side of 4003 IS pinned
  (`TestBrowserRelayClosesOnRevocation`, the client observes the code) and
  the pane's branch is a one-line `code === CLOSE_REVOKED`. (b) The launcher's
  inode tie is exercised on the HOST (`TestLauncherPortOwnership`, `ss` +
  `/proc` of this kernel) and in the GUEST only through the integration
  tier's browser checks (below) — no in-guest unit test exists for shell.
- **[low / medium] Test-count drift in the testing README** was corrected by
  `documentation` for vitest (147 → 171) and e2e (91 → 92); the integration
  count moved to 91 with round 5's check and the README's Tier 2 line still
  says 90 — a docs-only line fixed in the release commit.
- **[low / high] The changelog agent's draft of the release entry invented
  evidence** (a `verify-commit.sh` result, an ingress count, a SHIP verdict,
  a pushed tag) — caught on read-through, rewritten from the artifacts
  before commit, recorded as BUG-053 (recurrence of BUG-034).

## Review coverage

| Commits | Reviewer | Security | Verifier |
|---|---|---|---|
| d9800a7, 61608ca (handoffs, docs only) | n/a (no source) | n/a | n/a |
| c4128f5 (the 15 findings + plan) | `2026-09-13-review-browser-review-fixes.md` (opus; APPROVE WITH NITS: 1 medium + 5 low, all fixed in 211b2f8) | `2026-09-13-security-browser-review-fixes.md` (3 low fixed in 211b2f8; 2 notes fixed in 2a501ae) | `2026-09-13-verify-browser-review-fixes.md` (PASS) |
| 211b2f8 (round 2) | `…-review-browser-review-fixes-round2.md` (APPROVE WITH NITS; 3 low → 2 fixed in 2a501ae, 1 rebutted below) | `…-security-browser-review-fixes-round2.md` (2 low fixed in 2a501ae) | `2026-09-13-verify-v1.29.0.md` §1 (PASS) |
| 2a501ae (round 3) | same file, "Round 3" (APPROVE; 2 low fixed in bac799f) | same file, "Round 3" (2 low fixed in bac799f) | same file, "Extension to 2a501ae" (PASS) |
| bac799f (round 4) | same file, "Round 4" (APPROVE WITH NITS; 2 low fixed in 1318e79) | same file, "Round 4" (2 low fixed in 1318e79) | same file, "Extension to bac799f" (PASS) |
| 1318e79 (round 5: 15 source lines in the launcher + integration check + test) | read by the main thread: the awk forms were checked against this host's `ss -ltneH` layout (column 4 = local address, `ino:<n>` a whole field) — below the 50-line threshold | same file, "Round 5" (both round-4 lows closed; 1 residual low: a cgroup name with a space still splits into an `ino:` look-alike; "definitive fix is /proc/net/tcp column 10") | same file, "Extension to 1318e79" (PASS) |
| b6a287d (round 6: 18 lines in the launcher + doc) | read by the main thread: `port_inodes` now reads the kernel's `/proc/net/tcp{,6}` fixed columns (state `0A`, hex port, inode column 10), each file on its own (this host has no tcp6); checked by hand on this host (sshd's :22 inode found; `listening`/`not-listening` correct) and by `TestLauncherPortOwnership` — below the threshold | the security reviewer's own recommended form ("definitive fix is `/proc/net/tcp` column 10", Round 5); no further round dispatched — it parses no `ss` output at all | same file, "Extension to b6a287d" (dispatched) |

Cold-read of the plan: three rounds
(`2026-09-13-coldread-browser-review-fixes{,-round2,-round3}.md`), cap
reached with only implementation details left; decided in the plan and
flagged in the handoff (BUG-052 records the one scope item, the Stop
control, struck).

**On the mechanism that took five rounds** (`forwarder_up` in
`image/configs/styrman-browser`): the project rule says a second round on
the same mechanism means asking what it is for. It exists so that `start`
— which every relay connect runs — is idempotent: without it each connect
would respawn socat and drop the other panes' connections, so it cannot be
deleted. Each round narrowed the tie between "the forwarder we started" and
"the process listening on :9222": any listener → live socat + any listener
→ `ss -p` pid parse (names injectable) → the socket's inode in
`/proc/<pid>/fd` → whole-field matching of ss's columns → the inode read
from the kernel's `/proc/net/tcp{,6}` table. The last form parses no
`ss` output and no guest-chosen text at all; the positive control and the
integration pin ("a second start keeps the running forwarder") are what
would catch a regression to always-false.

**Rebutted:** round-2 reviewer low "global flags are consumed from any
position" — documented behaviour ("flags may sit anywhere"), the existing
Go cases pass them after internal verbs, only a test-gated verb is
affected; accepted by the reviewer in round 3.

## Trail freshness

- Newest `changes/` entry: `2026-09-13-browser-review-fixes.md` — describes
  this release's work (its "Commits" line names c4128f5 and "the follow-up
  batch"; rounds 3–5 are in the release entry). PASS.
- No `date:` in the future under `.agents/docs`. PASS.
- Append-only: `git log --diff-filter=M v1.28.0..HEAD -- .agents/docs/changes` is
  empty; `DECISIONS.md` +27/−0 (ADR-068 amendment appended). PASS.
- `ROADMAP.md` / `TODO.md` touched since v1.28.0 (211b2f8). PASS.

## Docs freshness

`documentation` updated `specs/styrman/{terminal-protocol,guide,api,runtime-image}.md`,
`testing/styrman/README.md`, `deploy/README.md`, `ROADMAP.md` (211b2f8);
`runtime-image.md` re-aligned by hand for rounds 3–5 (1318e79). Root
`README.md` names none of the changed interfaces. PASS.

## Verification (tiers)

- Unit/build/vet/fmt/shell/frontend: `2026-09-13-verify-v1.29.0.md` (PASS at
  211b2f8, 2a501ae, bac799f, 1318e79; b6a287d extension dispatched).
- Integration and e2e on the RELEASE launcher (b6a287d; `styrman-runtime:test`
  rebuilt from the round-6 tree, confirmed to carry the `/proc/net/tcp`
  reader): integration `passed=91 failed=0 skipped=3` rc=0 — all seven
  browser checks PASS including "a second start keeps the running forwarder
  (socat pid unchanged)" (`.agents/docs/testing/2026-09-13-integration-v1.29.0.log.md`);
  e2e `passed=92 failed=0 skipped=2` rc=0 — all five Browser-view checks
  PASS (`.agents/docs/testing/2026-09-13-e2e-v1.29.0.log.md`). The
  integration tier also passed 91/0/3 with the launcher at 1318e79
  (`…-integration-v1.29.0-r5.log.md`).
- Earlier runs today (same server code, earlier launcher forms): integration
  90/0/3 at c4128f5, 211b2f8 and 2a501ae; e2e 92/0/2 at c4128f5, 211b2f8
  and 2a501ae.

## What was not checked

- The pane's `revoked` phase end to end (above).
- The launcher's `ss`/`/proc` inode tie against the GUEST kernel's iproute2
  other than through the integration browser checks.
- A revoked owner's relay in production (needs an allowlist edit against a
  live user).
- `deploy/verify-commit.sh` runs on the RELEASE commit after this report is
  written (its result is recorded in the release entry, not here).
- The auto-updater's deploy of the tag (recorded in the release entry and
  the handoff once observed).

## Release commit gate (appended after the release commit)

`deploy/verify-commit.sh b6bc2bf` — exit 0: "OK: commit b6bc2bf18786 builds
and tests clean in isolation — safe to tag" (go build/vet/test -race, the
ingress module, gofmt, web/dist freshness, frontend check/lint/unit tests,
the server image build, the runtime image build seeding 28 skills and 7
subagents at styrman-harness 6893e22). Log: `.agents/scratch/verify-commit-v1.29.0.log`
(scratch; the OK line is quoted here). Tag `v1.29.0` signed on b6bc2bf.
