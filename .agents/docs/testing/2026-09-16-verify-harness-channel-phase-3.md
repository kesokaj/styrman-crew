---
owner: verifier
date: 2026-09-16
---
# Verification — harness-channel-phase-3   (2026-09-16)

Range reviewed: 28212ac..worktree

## Status: PASS

Scope: complete — all six requested steps ran against the pre-built
`styrman-runtime:itest-harness` image (built 2026-09-16 from the working
tree's `image/`, not rebuilt here).

## Commands

1. `bash -n image/entrypoint.sh` → exit 0
   `bash -n .agents/docs/testing/styrman/integration.sh` → exit 0
   `bash -n .agents/docs/testing/styrman/make-scratch-harness.sh` → exit 0
   `shellcheck -S warning image/entrypoint.sh` → exit 0, no output
   `shellcheck -S warning .agents/docs/testing/styrman/integration.sh` → exit 0, no output
   `shellcheck -S warning .agents/docs/testing/styrman/make-scratch-harness.sh` → exit 0, no output

2. `docker run --rm --entrypoint sh styrman-runtime:itest-harness -c 'grep -c HARNESS_ID /styrman/entrypoint.sh'` → exit 0, output `3` (≥3 expected) — the image carries the new entrypoint.

3. `bash .agents/docs/testing/styrman/make-scratch-harness.sh "$PWD/.agents/scratch/verify-hc" styrman-runtime:itest-harness` → exit 0. Last stdout line: `/home/kesokaj/workspace/styrman-crew/.agents/scratch/verify-hc/content/current`. `cat current/repo/TAG` → `v1.0.0`. Log showed `[styrman-update] harness: v1.0.0 (8ec96d8) is current` and the smoke's `### Skills (27)` / `### Subagents (7)` lines.
   Same command with third arg `v1.0.1` → exit 0. `current/repo/TAG` → `v1.0.1`; `current/repo/COMMIT` → `67acf721a8e5033d4572c0923f66d499d81f7549`, differing from the first run's `8ec96d81aedae90233764b7f7dec66c8ca47b196`. Log: `[styrman-update] harness: v1.0.1 (67acf72) is current`.
   `.agents/scratch/verify-hc` removed afterward (`rm -rf`, confirmed empty).

4. Tier 2, run once in background, port 18080 confirmed free before start (`ss -ltn | grep 18080` → exit 1, nothing bound):
   `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:itest-harness > .agents/scratch/itest-verify-phase3.log 2>&1` → the appended `tier2 exit=$?` line reads `tier2 exit=0`.
   Result line: `passed=113 failed=0 skipped=3 aborted=0` — matches the expected count exactly.
   `grep -E 'FAIL|SKIP'` of the log: only the 3 expected host-firewall SKIPs —
   `SKIP pretty URL proxies to the app (host firewall blocks host→bridge; prod unaffected)`,
   `SKIP legacy proxy URL still works (host firewall)`,
   `SKIP public link serves the app (host firewall blocks host→bridge; prod unaffected)`.
   No FAIL lines.
   Every channel/harness/stamp/swap line, all PASS:
   - `PASS scratch channel delivered v1.0.0 (make-scratch-harness.sh; log: /tmp/styrman-itest-bhXw/harness-channel.log)`
   - `PASS server start logged the harness channel (tag v1.0.0, its commit)`
   - `PASS guest /styrman/harness is the channel's export (COMMIT equals the host's current)`
   - `PASS guest /styrman/harness is read-only (a :ro bind, even for root)`
   - `PASS guest sh-agent is the channel's binary (sha256 equals content/current/bin/sh-agent)`
   - `PASS no .git in the mounted harness (a plain copy, as the mirror was)`
   - `PASS boot log names the channel as the harness source (tag v1.0.0)`
   - `PASS boot log records the harness deployed at the channel's commit`
   - `PASS no previous-harness artifacts (shell gates, GEMINI.md)`
   - `PASS harness stamp equals version+commit after boot`
   - `PASS BUG-062 setup: stamp removed, a directory planted in settings.json's place`
   - `PASS the seed still completed around the refused key (stamp rewritten)`
   - `PASS BUG-062 undo: plant removed, stamp removed for one more seed`
   - `PASS scratch channel swapped current to v1.0.1 while the VM is stopped (a new commit)`
   - `PASS restarted VM mounts the swapped channel (guest COMMIT is v1.0.1's, Docker re-resolved current)`
   - `PASS the swap re-seeded the home from the new tag (boot log: source channel tag v1.0.1)`

5. `go build ./...` bare → exit 0.
   `go vet ./...` bare → exit 0.
   No Go source changed this phase (only shell scripts + docs); build/vet confirm nothing else broke.

## Root cause  (FAIL only)
N/A — Status PASS.

## Coverage of changed code

- `image/entrypoint.sh` new lines (`HARNESS_COMMIT`/`HARNESS_ID`/`HARNESS_SRC` computation, the stamp comparison against `HARNESS_ID`, the `source ${HARNESS_SRC}` deployed-line clause) — covered by Tier 2's `.agents/docs/testing/styrman/integration.sh` checks: "guest /styrman/harness is the channel's export", "boot log names the channel as the harness source (tag v1.0.0)", "boot log records the harness deployed at the channel's commit", "harness stamp equals version+commit after boot", the BUG-062 stamp-rewrite/undo pair, and the swap sequence ("scratch channel swapped current to v1.0.1", "restarted VM mounts the swapped channel", "the swap re-seeded the home from the new tag") — this last group specifically exercises the `HARNESS_ID` re-seed-on-swap logic.
- `.agents/docs/testing/styrman/make-scratch-harness.sh` — covered by its own by-hand invocation in step 3 above (two runs, v1.0.0 then v1.0.1) and by Tier 2's own use of it to build the scratch channel before the server starts ("scratch channel delivered v1.0.0").
- No suppressions (`//nolint`, `t.Skip`, `.only(`, etc.) were added in this diff — the diff touches only `image/entrypoint.sh`, `integration.sh`, and the new helper, all shell.
- The runtime image `styrman-runtime:itest-harness` was built by the main agent before this run (2026-09-16, from the working tree's `image/`); this run did not rebuild it and took its correctness as given per the dispatch.

## Not examined

- `.agents/docs/plans/2026-09-16-harness-channel.md`, `.agents/docs/specs/styrman/runtime-image.md`, `.agents/docs/testing/styrman/README.md` diffs were not separately reviewed for prose accuracy (docs review is out of scope for this dispatch; only the shell scripts' syntax/lint and the running behaviour were checked).
- Tier 1 (`go test ./...`, the vendored Caddy ingress module) was not run this pass — no Go source changed this phase, and the dispatch's step 5 only asked to confirm `go build`/`go vet`.
- Phase 3's Checkpoint box in the plan is left unticked by this verifier (per the verifier role, it reports; ticking the plan is the main agent's action).

## Environment

OS: Linux 6.12.107+deb13-amd64. Go: `go build`/`go vet` ran against the repo's pinned toolchain (not separately queried for version this run). shellcheck 0.10.0. Docker: local daemon, image `styrman-runtime:itest-harness` (8dcb4a177526) present, not rebuilt. Port 18080 confirmed free before the Tier 2 run.
