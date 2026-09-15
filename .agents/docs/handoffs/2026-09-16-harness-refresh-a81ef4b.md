---
owner: main
date: 2026-09-16
---
# Handoff — 2026-09-16 — harness mirror refreshed to styrman-harness a81ef4b (committed locally, not shipped)

Session ID: 0db4ddf4-7e66-44e8-8384-6341b5b827fd        (same machine: `claude --resume 0db4ddf4-7e66-44e8-8384-6341b5b827fd`)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

A headless run, where no reply could arrive, asked for `image/harness/` to be
refreshed from styrman-harness `cceee548` to
`a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8` exactly as
`image/harness/VENDORED.md` § Refresh says. The refresh was then to go
through the Definition of Done up to a local commit and this handoff: no
push, tag, release or `/preflight`.

## Completed

- `5418f19` is the refresh: 32 files under `image/harness/` (30 changed
  upstream, plus `COMMIT` and `SHA256SUMS`), and the trail below. `diff -rq`
  printed exactly the three expected lines; `sha256sum -c` over 155 files
  exited 0. Record: `.agents/docs/changes/2026-09-16-harness-refresh-a81ef4b.md`.
- Tests row: verifier PASS,
  `.agents/docs/testing/2026-09-15-verify-harness-refresh-a81ef4b.md`.
  - Root and ingress tests exit 0, vet exit 0.
  - vulncheck exits 3: only the accepted docker/docker pair.
  - Mirror tests: 32 pass, 0 fail, 1 skip; one test added, mutation-checked.
  - `integration.sh` exits 0 with `passed=94 failed=0 skipped=3 aborted=0`,
    on `styrman-runtime:test` rebuilt from the refreshed tree.
  - Tier 5 steps 1–2 pass.
- Review row: merged record
  `.agents/docs/reviews/2026-09-15-review-worktree-merged.md`.
  - Code lens APPROVE.
  - Docs lens APPROVE: no styrman doc is stale, and the counts stay 27 skills
    and 7 subagents.
  - Security: 2 medium and 3 low, all in vendored upstream text; each is
    `left: styrman-harness` in the Outcome.
- Docs row: nothing to change, so `documentation` was not dispatched.
  TODO.md and ROADMAP.md stay headers-only.

## Open / In Progress

- Nothing from this task is queued for an agent. The refresh is committed,
  not shipped: running projects keep the cceee54 harness until a release
  bumps the image version (ADR-046, ADR-062).
- Not run: the web check, lint and unit tests and browser e2e (`web/`
  untouched); Tier 5 steps 3–5 (step 4 needs a sign-in);
  `deploy/verify-commit.sh`, which is a release step.
- The handoff cold-reader check (new in this harness) finds four older
  plans with unticked boxes outside code fences that ROADMAP "Now" does not
  name:
  - `.agents/docs/plans/2026-09-03-harness-migration.md`
  - `.agents/docs/plans/2026-09-08-lan-full-parity.md`
  - `.agents/docs/plans/2026-09-11-browser-view.md`
  - `.agents/docs/plans/2026-09-11-installer-ctl.md`

  They predate this session. Nobody has checked whether those boxes are
  missed ticks or unfinished work.
- Requirements edits awaiting approval: none.

## Key Decisions

- The five security findings are `left: styrman-harness`, neither fixed nor
  rebutted here, because the mirror is verbatim and
  `image/harness/VENDORED.md:4-5` says every file is edited upstream.
- No styrman-side workaround was added, because nothing in styrman reads the
  two reworded sh-agent messages: `integration.sh` greps the unchanged
  plain-copy line, and the entrypoint discards install's stdout.
- No bug record: nothing in this project's work failed. The verifier's turn
  problems are in the agent notes below instead.

## Corrections

- none (a headless run, with no user turn after the request).

## Agent notes (unverified)

- The changelog draft needed 9 corrections before staging, applied with Edit
  (the `.agents/rules/changelog-brief.md` family):
  - 1 invented fact: "the harness `main` on 2026-09-15".
  - 1 narrowed claim: "Phase 7 of" the plan, where the range is the whole
    plan.
  - 2 wrong facts: 28 remaining files instead of 26; both GO IDs attributed
    to the root module.
  - 5 omissions: the review record, the release status, the not-run list,
    the mirror-check detail, and the bugs line.
- The verifier stopped at its 30-turn limit in step 5. When resumed, it put
  `integration.sh` in the background and ended its turn while the suite ran,
  the subagent form of styrman-harness BUG-031. A second resume let it
  finish. Its log's step 7 says the backgrounding was "per coordinator
  instruction", but the brief asked for one foreground call with a long
  timeout.
- The code reviewer's Not examined says it could not compare the mirror with
  upstream "(no network access)". The checkout is local, and the security
  reviewer and the main agent both ran that comparison (`diff -rq`).
- One `cd` into `image/harness` carried over into the next command
  (`.agents/rules/writing-files.md`, Working directory). The working
  directory was back at the repo root before any agent was dispatched.
- Upstream bugs still open at a81ef4b ship with the mirror
  (styrman-harness `.agents/docs/bugs/INDEX.md`: BUG-019 to BUG-023,
  BUG-031).

## Environment State

- Branch: main, clean after this handoff's commit. It is 2 commits ahead of
  origin/main (`5418f19` and this handoff), unpushed by instruction.
- `styrman-runtime:test` was rebuilt 2026-09-15 23:56 from the refreshed tree
  (its COMMIT is a81ef4b). Scratch logs are under
  `.agents/scratch/verify-harness-refresh-a81ef4b/`.
- Production was not inspected beyond `docker ps` (styrman-server,
  styrman-caddy and one project container running). Nothing on it was
  touched.

## Artifacts to Read

- `.agents/docs/changes/2026-09-16-harness-refresh-a81ef4b.md`: what changed
  and how it was verified.
- `.agents/docs/reviews/2026-09-15-review-worktree-merged.md`: lens verdicts
  and the five `left` findings.
- `.agents/docs/reviews/2026-09-15-security-worktree.md`: the findings in
  full, input for the harness session.
- `.agents/docs/testing/2026-09-15-verify-harness-refresh-a81ef4b.md`: the
  verifier log.
- `image/harness/VENDORED.md`: the refresh recipe, and why only a release
  re-seeds projects.

## Next Steps

1. Operator: ship the refresh. Push `main` and cut the release per
   `.agents/docs/specs/styrman/versioning.md` through `/preflight`; the
   release entry names COMMIT `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8`.
   - Version, recommended: a MINOR (`v1.34.0`) by the Git rule in AGENTS.md.
     Project homes gain behaviour (the `/init-project` update path and the
     rewritten skills, agents and context), and nothing breaks.
   - In that gate, treat a missing or partial security row as blocking
     (merged record, finding 2).
2. Operator, in a styrman-harness session:
   - Tick plan task 7.6 for crew (`5418f19`). The argolis-devbox half
     (`deploy/harness/`) was not done here.
   - Then the plan's Phase 7 checkpoint and Done list.
   - Triage the five security findings, and the verifier's early turn end as
     BUG-031 evidence.
   - Watch the "final harness commit" wording in 7.6. The recipe mirrors
     `ROADMAP.md` (rsync excludes only `.agents` and the three local files),
     so a harness Done commit that empties its ROADMAP leaves this mirror one
     file behind. If the release must carry that commit, re-run § Refresh
     at it before releasing.
3. Any session: triage the four older plans listed under Open, ticking each
   box with its `changes/` evidence or closing the plan. Not started here.
