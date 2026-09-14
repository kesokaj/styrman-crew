---
owner: verifier
date: 2026-09-14
---
# Verification — harness mirror refresh to 8046e4d, round 3 (2026-09-14)

Range reviewed: v1.30.1..worktree (round 3)

Round 2 log: `.agents/docs/testing/2026-09-14-verify-harness-refresh-8046e4d-r2.md` (read, not
amended). This round re-verifies the two files that moved since round 2: `image/Dockerfile` (the
`test ! -e /styrman/harness/.git` assertion round 2 proved dead was deleted, replaced by a
comment naming `image/.dockerignore` as the guarantee) and `image/entrypoint.sh` (dispatched as
"one comment clause").

## Status: FAIL

Scope: all four dispatch steps ran to completion. Steps 2 and 3 PASS. Step 1's grep surfaces a
real, non-comment code change in `image/entrypoint.sh` beyond what the dispatch described ("one
comment clause"); step 4's fourth count (`GIT_CONFIG_GLOBAL` occurrences) is 1, not the expected
0, for the same reason. Root cause below.

## Commands

### Step 1 — diff scoped to the two moved files

```
git diff HEAD -- image/Dockerfile image/entrypoint.sh | grep -E '^[-+]' | grep -vE '^[-+]{3}' | grep -vE '^[-+]\s*#'
```
→ rc=0, one line, not the deleted `test ! -e` line and not a comment-continuation:

```
-        timeout 20 runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
+        timeout 20 runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" \
```

`image/Dockerfile`'s diff against HEAD contains only added `#`-comment lines (the `.dockerignore`
guarantee paragraph, 7 lines) — no `test ! -e` line appears at all, consistent with it having been
added and then deleted within this round's uncommitted work (net zero against the v1.30.1 base).
Dockerfile: matches dispatch description.

`image/entrypoint.sh`'s diff against HEAD contains several reworded comment blocks (all correctly
filtered by `grep -vE '^[-+]\s*#'`, confirmed by inspecting the raw diff with `cat -A` — every
comment line has a leading `#`) plus the one code line above: the `env` invocation that runs
`sh-agent install` had `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` removed. This is an
actual behaviour change to the install invocation, not a comment edit. `git log --oneline -- image/entrypoint.sh`
shows no intervening commit; `grep -n 'GIT_CONFIG_GLOBAL\|GIT_CONFIG_NOSYSTEM' image/entrypoint.sh`
confirms the env vars are gone from the actual `runuser`/`env` line and survive only inside the
rewritten comment's prose (line 139: "...the install line carried GIT_CONFIG_GLOBAL=").

Step 1: FAIL — the entrypoint.sh diff is not "one comment clause"; it is a comment rewrite plus a
real removal of `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` from the `sh-agent install`
invocation.

### Step 2 — syntax, lint, binary check

- `bash -n image/entrypoint.sh` → rc=0.
- `shellcheck -S warning image/entrypoint.sh` → rc=0, no warnings.
- `git diff --numstat` (full tree) piped through `awk '$1=="-" && $2=="-"'` → no output, "no binary
  rows" printed by the sentinel.

Step 2: PASS.

### Step 3 — Tier 2 integration

- Port 18080 confirmed free: `ss -ltn | grep 18080` → no match, rc=1. Run not blocked.
- `docker image inspect styrman-runtime:test --format '{{.Created}}'` → `2026-09-14T12:33:10+02:00`,
  matching the dispatch's stated 12:33 rebuild — image is current for this tree.
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` → rc=0. Final line:
  `passed=94 failed=0 skipped=3 aborted=0` — matches expected exactly. Log:
  `.agents/scratch/step3-r3-integration.log`.
- Harness-section lines (`=== agent harness (ADR-062...) ===`), all PASS:
  ```
  PASS sh-agent reports no drift in the project home
  PASS CLAUDE.md is the two-line import stub
  PASS 27 skills and 7 agy agents seeded
  PASS sh-agent status labels the image mirror a plain copy
  PASS boot log records the harness deployed at the mirror's commit
  PASS no previous-harness artifacts (shell gates, GEMINI.md, settings.json)
  ```

Step 3: PASS.

### Step 4 — container checks

```
docker run --rm --entrypoint sh styrman-runtime:test -c 'test ! -e /styrman/harness/.git && echo no-git; cat /styrman/harness/COMMIT; grep -c "test ! -e" /styrman/entrypoint.sh; grep -c GIT_CONFIG_GLOBAL /styrman/entrypoint.sh'
```
→ rc=0, output:
```
no-git
8046e4d5f24b009bd2461ebef28931cade7a4acd
0
1
```
Three of four match expected (`no-git`, the COMMIT SHA, `0` for `test ! -e`). The fourth is `1`,
not the expected `0`.

Step 4: FAIL on the fourth count.

## Root cause

The entrypoint.sh comment rewrite documenting the removal of the `GIT_CONFIG_GLOBAL`/
`GIT_CONFIG_NOSYSTEM` guard literally quotes the old code — "...the install line carried
`GIT_CONFIG_GLOBAL=`..." at line 139 — so `grep -c GIT_CONFIG_GLOBAL /styrman/entrypoint.sh`
inside the built image counts that one textual occurrence. The env vars themselves ARE gone from
the actual `env` invocation (confirmed both on the host source and via the step-1 diff); the
container-side grep does not distinguish code from comment prose, so the expected-0 assertion in
the dispatch is wrong given the comment's own wording, not because the underlying code change is
incomplete. This is the same fact surfacing twice: step 1's diff already showed the entrypoint.sh
change is not comment-only (it also drops the two env vars from the real install command), and
step 4's count is a direct, mechanical consequence of the new comment's chosen wording.

Whether dropping `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` from the `sh-agent install`
invocation is itself correct (the comment argues install never calls git on a plain-copy harness,
so the guard is now moot) is a design question for review, not something this verification pass
judges — flagged here only because the dispatch characterized the entrypoint.sh diff as
comment-only and specified an expected grep count that the diff's own new wording contradicts.

## Coverage of changed code

- `image/Dockerfile` (deleted `test ! -e` assertion, replaced by a comment citing
  `image/.dockerignore`) — step 1's diff and step 4's `no-git`/`test ! -e` count=0 confirm the
  assertion is gone and no `.git` is present in the built image; consistent with round 2's finding
  that the assertion was dead code. No test exercises the comment itself (prose, not covered by
  design).
- `image/entrypoint.sh` (`GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM` removed from the `sh-agent
  install` invocation) — **NOT COVERED**: no test in the changed set (integration.sh's 94 checks,
  the harness section) exercises the removed-guard path specifically (e.g. a home with a
  `~/.gitconfig` `include` pointing at a FIFO, which the removed comment describes as the original
  threat model) to confirm the timeout-only defence the new comment claims is sufficient still
  holds. Step 3's green run does not touch this path — the harness-section checks assert
  installation succeeded, not that a hostile home's git config cannot stall it.

## Not examined

- Whether removing `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` reintroduces the
  2026-09-03-reviewed risk (a `~/.gitconfig` `include` of a FIFO hanging `git rev-parse`) under
  any code path other than `sh-agent install` on a plain-copy harness — not traced through
  `image/harness/internal/harness/*` in this pass; the comment's claim that "install never runs
  git on a plain copy" was not independently verified against the harness source in this round
  (round 2 verified a different mechanism, `sh-agent status`, not `install`'s internals).
  Recommend `security` or `reviewer` confirm this before the change lands, given it removes an
  existing defensive env var.
- Whether this is a NEW code change since round 2 or was already present in the tree at round 2's
  time but outside that round's four-item scope — `git log -p` / reflog on the working tree state
  at round-2's timestamp was not consulted.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Docker image used: `styrman-runtime:test`, created `2026-09-14T12:33:10+02:00` (already current
  for this tree; no rebuild triggered, no Docker Hub pulls performed — BUG-056 avoided).
- Integration log: `.agents/scratch/step3-r3-integration.log`.
