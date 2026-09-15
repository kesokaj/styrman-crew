---
owner: verifier
date: 2026-09-15
---
# Verification — harness refresh a81ef4b (2026-09-15)

Range reviewed: working tree vs HEAD (`git diff HEAD`) — 32 files, all under `image/harness/` (mirror refresh); untracked: none.

## Status: PASS

Scope: complete — steps 1-8 all ran as specified in the brief.

## Commands

1. Root module
   - `go vet ./...` → exit 0
   - `go test ./...` → exit 0 (all packages ok, cache-backed for unchanged ones); log: `.agents/scratch/verify-harness-refresh-a81ef4b/go-test-root.txt`
   - counts: not applicable (root module untouched by this diff; brief only asked for these two bare commands)

2. Ingress module
   - `(cd deploy/caddy/ingress && go test ./...)` → exit 0, `ok github.com/spgo/devbox/caddy/ingress (cached)`; log: `.agents/scratch/verify-harness-refresh-a81ef4b/go-test-ingress.txt`

3. `bash deploy/vulncheck.sh` → exit 3 (govulncheck's own status: 3 = a called vulnerability found)
   - Root module (`.`): 1 finding — GO-2026-4887 (Moby AuthZ plugin bypass, oversized request bodies), `github.com/docker/docker@v28.5.2+incompatible`, no fix available.
   - Ingress module (`deploy/caddy/ingress`): 2 findings — GO-2026-4887 and GO-2026-4883 (Moby off-by-one in plugin privilege validation), same docker/docker version, no fix available.
   - Mirror module (`image/harness`): "No vulnerabilities found."
   - The script's own header comment names exactly these two docker/docker findings (GO-2026-4887/4883) as accepted-per-release with no fix available — rc=3 is the documented, accepted signal, not a build gate failure. Log: `.agents/scratch/verify-harness-refresh-a81ef4b/vulncheck.txt`.

4. Mirror module (`image/harness`, own go.mod)
   - `sha256sum -c --quiet SHA256SUMS` → exit 0.
   - File-list check (verify-commit.sh's method): `diff <(find . -type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort) <(cut -c67- SHA256SUMS)` → empty (no unlisted/missing entries); `find . -type l` → empty (no symlinks).
   - `gofmt -l .` → exit 0, no output (clean).
   - `go vet ./...` → exit 0.
   - `go test ./... -count=1 -v` → exit 0; log `.agents/scratch/verify-harness-refresh-a81ef4b/go-test-mirror.txt`.
     - counts: pass 32, fail 0, skip 1 at (working tree, dirty vs HEAD) for `go test ./... -count=1 -v` in image/harness.
   - Test-list comparison: `go test -list '.*' ./...` in working-tree mirror vs. same command against `git archive HEAD image/harness` extracted to `.agents/scratch/harness-head/image/harness`.
     - **Added:** `TestStatusNamesForeignClaudeMD` (new, in `cmd/sh-agent`). **Removed:** none.
     - HEAD's `cmd/sh-agent` package listed 9 tests, working tree lists 10 (the new one); `internal/harness` package unchanged at 25 tests both sides.
   - vs previous log `.agents/docs/testing/2026-09-14-v1.33.0-phase6-verify.md`: that log did not separately break out the `image/harness` mirror module's own `go test` counts in isolation. `vs previous: none` (first isolated baseline for this module).

5. Covering tests for changed Go source (`cmd/sh-agent/main.go`, `internal/harness/apply.go`)
   - Diff only touches two printed-message strings (plus one comment, not user-visible behavior, not tested):
     - `main.go` `status()`: the "foreign CLAUDE.md" note line → covered by new test `TestStatusNamesForeignClaudeMD` (`cmd/sh-agent/main_test.go:325`), asserting the exact new wording.
     - `apply.go` `Install()`: the "kept" foreign-file message → covered by `TestInstallStatusResyncUninstall` (`internal/harness/harness_test.go:209`), asserting `strings.Contains(out.String(), "move its content into a rule file under rules/")`.
   - Mutation check, in `.agents/scratch/harness-mut` (a `cp -r` of `image/harness`, never the tracked tree):
     - Reverted `main.go`'s note string to HEAD wording → `go test ./cmd/sh-agent/... -run TestStatusNamesForeignClaudeMD -v` → exit 1, test **FAILed** as expected (log: `.agents/scratch/verify-harness-refresh-a81ef4b/mutation-main.txt`).
     - Reverted `apply.go`'s keep-message string to HEAD wording → `go test ./internal/harness/... -run TestInstallStatusResyncUninstall -v` → exit 1, test **FAILed** as expected (log: `.agents/scratch/verify-harness-refresh-a81ef4b/mutation-apply.txt`).
   - Both changed lines are covered by tests that actually exercise the new wording, confirmed by mutation.

6. Mirror fidelity
   - `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness image/harness` → exit 1 (diff found differences), exactly 3 lines, verbatim:
     ```
     Only in /home/kesokaj/workspace/styrman-crew/image/harness: COMMIT
     Only in /home/kesokaj/workspace/styrman-crew/image/harness: SHA256SUMS
     Only in /home/kesokaj/workspace/styrman-crew/image/harness: VENDORED.md
     ```
     This is exactly the expected result documented in `image/harness/VENDORED.md`'s own refresh procedure ("expected: exactly three lines, ... COMMIT, ... SHA256SUMS, ... VENDORED.md"). Log: `.agents/scratch/verify-harness-refresh-a81ef4b/mirror-diff.txt`.
   - `git -C ~/workspace/styrman-harness rev-parse HEAD` → `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8`.
   - `cat image/harness/COMMIT` → `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8` — matches.
   - `git -C ~/workspace/styrman-harness status --porcelain` → empty (clean checkout).

7. Tier 2 integration (`.agents/docs/testing/styrman/integration.sh`)
   - Ran as `bash .agents/docs/testing/styrman/integration.sh > .agents/scratch/verify-harness-refresh-a81ef4b/itest-a81ef4b.log 2>&1; echo "step7_rc=$?"` (dispatched to background due to length per coordinator instruction, output captured to file; `step7_rc=0` read from the task's captured stdout, not inferred from the summary line).
   - Summary line: `passed=94 failed=0 skipped=3 aborted=0`.
   - `ensure-test-image` proof (log lines 1-2): `ensure-test-image: rebuilding styrman-runtime:test: image/ has files newer (2026-09-15T23:41:09) than styrman-runtime:test (2026-09-14T16:13:59)` then `ensure-test-image: styrman-runtime:test rebuilt` — confirms a REBUILD, not "is current".
   - `docker image inspect --format '{{.Created}}' styrman-runtime:test` → `2026-09-15T23:56:53.511007537+02:00` — later than the refresh (23:41), confirming the image just built.
   - `docker run --rm --entrypoint sh styrman-runtime:test -c 'cat /styrman/harness/COMMIT'` → `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8`, matching `image/harness/COMMIT` exactly.
   - Integration log line 27: `PASS boot log records the harness deployed at the mirror's commit` — passed.
   - No Docker Hub 429 (BUG-056) encountered; port 18080 was not reported bound.
   - Log: `.agents/scratch/verify-harness-refresh-a81ef4b/itest-a81ef4b.log`.

8. Tier 5 manual steps 1-2 (against `styrman-runtime:test`)
   - `sh-agent list --repo /styrman/harness` (run via `docker run --rm --entrypoint sh styrman-runtime:test -c '/styrman/bin/sh-agent list --repo /styrman/harness'`) → reported Skills (27), Subagents (7).
   - `find image/harness/skills -mindepth 2 -maxdepth 2 -name SKILL.md | wc -l` → 27 (match).
   - `find image/harness/agents -maxdepth 1 -name '*.md' | wc -l` → 7 (match).
   - `install --dry-run --repo /styrman/harness --home /check` → first line `[dry-run] plain copy, not its own git checkout: the running binary would be installed as is` (the plain-copy dry-run line), then `260 written, 0 unchanged, 0 removed, 0 kept — 260 files managed`.
   - `cat /styrman/harness/COMMIT` → `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8`.
   - `ls /styrman/configs/harness-migrate.sh` → exists.
   - `test ! -e /styrman/harness/.git` → true (printed `NO_GIT_DIR`).

## Root cause (FAIL only)

Not applicable — Status is PASS.

## Coverage of changed code

- `image/harness/cmd/sh-agent/main.go` (status note line) — covered by `TestStatusNamesForeignClaudeMD`; mutation-confirmed.
- `image/harness/internal/harness/apply.go` (keep message line) — covered by `TestInstallStatusResyncUninstall`; mutation-confirmed.
- No added `//nolint`, `//lint:ignore`, `t.Skip`, `.only(`, or similar suppressions found in the diff.
- All other 30 changed files are prose (AGENTS.md, README.md, ROADMAP.md, skills/agents markdown, SHA256SUMS, COMMIT) — exercised indirectly by step 8 (`sh-agent list`/`install` against the built image) and step 4's SHA256SUMS/file-list check, plus step 6's fidelity diff and step 7's integration boot check.

## Not examined

- Web trio (`npm run check`/`lint`/`test`) and browser e2e — not run; diff does not touch `web/`, per the brief.
- Tier 5 manual steps 3-5 — out of scope for this run per the brief.
- The 3 skipped integration checks and the specific list of which they are — not itemized (only the aggregate summary line `passed=94 failed=0 skipped=3 aborted=0` was read); the previous log's skip count is not compared here per-check since the brief's counts-comparison instructions targeted `go test`, not the integration suite.

## Environment

Linux host `dave` (also production: `styrman-server`, `styrman-caddy`, a live project container `styrman-bd08c102-c75d-4f52-a695-2e5819c8c700` — none of these were touched; steps 7/8 touched only `styrman-runtime:test`). Plain Docker (no Colima). Go per repo pin.
