---
owner: main
date: 2026-09-14
---
# Preflight — v1.33.0 (2026-09-14)

Range reviewed: v1.32.0..8aaa5e9 (plus the release commit)

Release: **MINOR** — a new operator knob (`STYRMAN_ALLOW_IP_FORWARD`) and
a refreshed agent harness in every project home (styrman-harness
`cceee548ea8bf8ca1e4b3f6b561383a88bea7aa4`, up from `8046e4d`: the root-steps
sentence every project VM receives is now conditioned on the project's
recorded permission; the mirror's own tests pass as a plain copy). The rest
fixes released behaviour: a containerized server refuses to start with
`ip_forward` on (R12 asserted, not assumed); the release gate names a Docker
Hub 429 (BUG-056); the e2e cleanup trap's format string is hoisted
(BUG-038 mitigated); direct tests for the bridge attach (BUG-035/054/060's
family); the trail emptied (TODO/ROADMAP headers-only, eight stale
agent-memory files gone); a retro with two project rules. Plan:
`.agents/docs/plans/2026-09-14-v1.33.0-close-out.md`, six phases, each
its own commit (b21a3c1, 8b89e90, 7e484ea, 3b88baa, 641ad79, 8aaa5e9).

Pre-gate commits: the six phase commits above plus the plan (71d7a5d) and
the v1.32.0 handoff (0a922c1). Three edits landed after the gate rows ran,
all answering the release review's findings — `deploy/verify-commit.sh`
(the 429 regex, the tail in both branches, logs outside the worktree) and
`.agents/docs/deploy/remote.md` (the measured budget) — they ride in the
release commit and are re-verified by `deploy/verify-commit.sh` on it.

## VERDICT: SHIP

`deploy/verify-commit.sh 8aaa5e9` green on a detached worktree (every
step, web included); the release review APPROVES WITH NITS (three lows,
all fixed); security's three findings are fixed or rebutted in writing;
the docs lens reports nothing stale; the integration suite (94/0/3) and the
browser e2e (92/0/2) are green on this tree at the README's counts; trail
append-only and current; TODO and ROADMAP headers-only (ROADMAP "Now"
emptied in the release commit).

## Findings (FAIL-first)

None blocking.

- **[medium / high — FIXED]** Security #1 = release review #1: the gate's
  429 classifier matched a bare `429` anywhere in the build log (a digest,
  a timing) and that branch hid the tail. Now `toomanyrequests|too many
  requests`, tail in both branches; exercised with a fake `docker`.
- **[low / high — FIXED]** Release review #2 = security #2: the server
  build's log sat inside its own build context. Logs now live in a
  separate `LOGDIR` removed by `cleanup`.
- **[low / high — FIXED]** Release review #3: `remote.md` said "100 per
  6 h" where BUG-056 measured `100;w=3600`; now states the measured value.
- **[low / high — REBUTTED]** Security #3: the `fs.ErrNotExist` branch
  warns instead of refusing — the Warn line makes "unknown" distinguishable
  from "asserted"; the trade-off is recorded in code and docs.
- Phase 2's own rows (8b89e90): reviewer APPROVE; security 1 medium FIXED
  (compose passthrough), 1 low FIXED (read-error branch), 1 low REBUTTED
  (IPv4-only: bridges are created without EnableIPv6) —
  `.agents/docs/reviews/2026-09-14-{review,security}-v1.33.0-phase2.md`.
- The harness's own rows at cceee54 (in styrman-harness): reviewer APPROVE
  WITH NITS (fixed); security 1 high (symlinks in skill dirs deployed as
  their target), 2 medium — all FIXED there; residuals recorded.
- Cold-reader check of the plan: two rounds, `design ready` /
  `implementation ready` (`.agents/docs/reviews/2026-09-14-coldread-plan-v1.33.0.md`).

## Review rows

- Code (release range, opus): `.agents/docs/reviews/2026-09-14-review-v1.32.0-8aaa5e9-release.md`
  — APPROVE WITH NITS; Outcome written. Confirmed the mirror byte-identical
  to styrman-harness cceee54 (`sha256sum -c` 155/155, `diff -rq`).
- Security (release range): `.agents/docs/reviews/2026-09-14-security-v1.32.0-8aaa5e9-release.md`
  — FINDINGS, Outcome written; the compose passthrough fail-closed for
  every accidental value; `remote.md` consistent with the updater's
  `/run`-only `DOCKER_CONFIG`; the mirror cited to the harness's review.
- Docs lens: `.agents/docs/reviews/2026-09-14-review-v1.32.0-8aaa5e9-docs.md`
  — APPROVE, nothing stale.
- Phase rows: `…-review-v1.33.0-phase2.md`, `…-security-v1.33.0-phase2.md`;
  verifier logs `.agents/docs/testing/2026-09-14-v1.33.0-phase2-verify.md`
  and `…-phase6-verify.md` (PASS). Phases 1, 3, 4: below the reviewer
  threshold, read by the main agent (their change entries say so); all
  covered by the release review above.
- Cross-repo: styrman-harness `.agents/docs/{reviews,testing}/2026-09-14-*upstream-items*`,
  handoff `2026-09-14-upstream-items-from-styrman-crew.md`.

## Verification observed by the main agent (bare, exit status read)

- `deploy/verify-commit.sh 8aaa5e9` → rc 0, "OK: commit 8aaa5e95de06
  builds and tests clean in isolation — safe to tag" (root module
  build/vet/test -race, ingress module, dependency agreement, harness
  mirror SHA256SUMS + builder pin, web/dist freshness, shellcheck +
  systemd-analyze, gofmt, frontend check/lint/unit, server image, runtime
  image 27/7, plain-copy mirror). Log `.agents/scratch/verify-commit-8aaa5e9.log`.
- `bash .agents/docs/testing/styrman/integration.sh` → `passed=94 failed=0
  skipped=3`, rc 0, the four harness checks PASS (log
  `.agents/scratch/integration-v1.33.0.log`).
- `bash web/e2e/run.sh` → `passed=92 failed=0 skipped=2`, rc 0, no
  "unexpected EOF", no leaked `/tmp/styrman-e2e-*` dirs or managed
  containers (log `.agents/scratch/e2e-v1.33.0.log`).
- `go vet ./...` rc 0; `go test ./... -count=1` rc 0; ingress ok; web
  check/lint rc 0, vitest 178/178 (log `.agents/scratch/unit-web-v1.33.0.log`).
- `(cd image/harness && go test ./... -count=1)` rc 0 — the plain copy's
  tests green (BUG-058's condition).
- After the three post-review edits: `bash -n` rc 0, `shellcheck -S
  warning deploy/verify-commit.sh` rc 0; `build_image` re-exercised with a
  fake `docker` (rate limit → named + tail, rc 1; a digest containing
  `4291f00` → generic branch, rc 1; success → rc 0).
- Live pre-check: `docker exec styrman-server cat /proc/sys/net/ipv4/ip_forward`
  → `0` on the running 1.32.0 container, so the new boot guard passes on
  the same compose file.

## Trail freshness

- Newest `changes/` entries: the six phase entries of today, newest
  `2026-09-14-v1.33.0-phase6-verification-and-docs.md` — describe the
  release diff. Pass.
- No `date:` in the future under `.agents/docs`. Pass.
- Append-only: `git log --diff-filter=M v1.32.0..HEAD -- .agents/docs/changes
  .agents/docs/architecture/DECISIONS.md` empty. Pass.
- ROADMAP.md and TODO.md touched in the range (71d7a5d, b21a3c1). Report-only.

## What was not checked

- Item 1's live proof (a project stopped across a server restart, opened
  from the UI, its first terminal reaching the broker) — needs a browser
  and a stopped project at the restart; 0 projects exist. Recorded as the
  operator's step in the handoff; this deploy restarts the server with no
  project to prove it on.
- The harness's sandbox CLI-start matrix and real-home smoke checks for the
  one-sentence `context/` change (named as a gap in its verifier log).
- `deploy/vulncheck.sh` was run by the security agent inside the pinned
  builder: only the two accepted no-fix docker/docker advisories; not
  re-run by the main agent. `npm audit`: one high in a build-time
  dependency (js-yaml), pre-existing, carried.
- The Docker Hub `docker login` (item 2's operator half) — documented, not
  done; the gate ran within the anonymous budget today.

## Deploy (observed after the tag)

(filled in after the updater tick)
