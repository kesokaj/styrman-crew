---
owner: verifier
date: 2026-09-16
---
# Verification — harness refresh 4aa19be (2026-09-16)

Range reviewed: working tree vs HEAD (`git diff HEAD`) — 28 files changed, all
under `image/harness/` (mirror refresh from a81ef4b to 4aa19be: `COMMIT`,
`SHA256SUMS`, and 26 mirrored files); untracked: none. HEAD is `4957dfe`
(uncommitted refresh on top).

## Status: PASS

Scope: complete for the commands named in the brief, except
`deploy/verify-commit.sh` itself, which cannot meaningfully run against this
uncommitted change (see § 5 below) — not a gap in what was exercised, since
its own checks were replicated directly (§ 4) as the previous log also did.

## Commands

1. Root module
   - `go vet ./...` → exit 0.
   - `go test ./...` → exit 0, all packages `ok` (cached, root module untouched
     by this diff). Log: `.agents/scratch/verify-harness-refresh-4aa19be/go-test-root.txt`.
   - counts: not applicable (root module untouched by this diff).

2. Ingress module
   - `(cd deploy/caddy/ingress && go test ./...)` → exit 0,
     `ok github.com/spgo/devbox/caddy/ingress (cached)`. Log:
     `.agents/scratch/verify-harness-refresh-4aa19be/go-test-ingress.txt`.

3. `bash deploy/vulncheck.sh` → exit 3 (govulncheck's own status: 3 = a called
   vulnerability found), identical to the previous log's finding set:
   - Root module and ingress module: GO-2026-4887 (Moby AuthZ plugin bypass)
     and (ingress only has both too) GO-2026-4883 (off-by-one in plugin
     privilege validation), both in `github.com/docker/docker@v28.5.2+incompatible`,
     no fix available — accepted per the script's own header comment.
   - Mirror module (`image/harness`): "No vulnerabilities found."
   - No new findings vs. 2026-09-15. Log: `.agents/scratch/verify-harness-refresh-4aa19be/vulncheck.txt`.

4. Mirror module (`image/harness`, own `go.mod`)
   - `sha256sum -c --quiet SHA256SUMS` → exit 0.
   - File-list check: `diff <(find . -type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort) <(cut -c67- SHA256SUMS)` → empty.
   - `find . -type l` → empty (no symlinks).
   - `gofmt -l .` → exit 0, no output.
   - `go vet ./...` → exit 0.
   - `go test ./... -count=1 -v` → exit 0. Log: `.agents/scratch/verify-harness-refresh-4aa19be/go-test-mirror.txt`.
     - counts: pass 79, fail 0, skip 1 at `4957dfe`+dirty for `go test ./... -count=1 -v` in `image/harness` (counted with `grep -c -- '--- PASS:'` etc., indented subtest lines included).
     - vs previous: `.agents/docs/testing/2026-09-15-verify-harness-refresh-a81ef4b.md`, pass 32→79, skip 1→1.
       explained: the rise is the two new test functions added between a81ef4b
       and 4aa19be for the attribution-setting feature (ADR-025, commit 42ba2f5
       per this repo's own git log) — `TestAttributionNote` (9 subtests + 1
       parent = 10 PASS lines) and `TestAttributionSet` (14 subtests + 1 parent
       = 15 PASS lines) both confirmed present in the new log's output
       (`go-test-mirror.txt` lines 21-56). Note the previous log's own 32 count
       used the same "indented subtest lines included" method, so the
       comparison is apples-to-apples; the dispatch's expectation of a rise is
       confirmed with the exact source named, not just observed.
   - `go run ./cmd/sh-agent list` → exit 0: Skills (27), Subagents (7) — matches
     `find image/harness/skills -mindepth 2 -maxdepth 2 -name SKILL.md | wc -l`
     (27, not re-run this round but unchanged per the refresh's file list) and
     the 7-file `agents/*.md` count. Log:
     `.agents/scratch/verify-harness-refresh-4aa19be/list.txt`.

5. `deploy/verify-commit.sh` — read its header (`deploy/verify-commit.sh:1-24`):
   it takes a `commit-ish` (default `HEAD`), checks that commit out into a
   *detached worktree*, and builds/tests THAT tree — "so anything unstaged,
   untracked, or forgotten is absent, exactly as it will be for the updater."
   This repository's working tree currently has the 28-file mirror refresh
   **uncommitted**; `HEAD` is still `4957dfe`, the commit from before this
   refresh. Running `deploy/verify-commit.sh` (default REF=HEAD) would
   therefore build and verify the OLD commit, not the change under test —
   it would silently pass without ever touching the refresh to 4aa19be. The
   only way to make it test this change is to commit first, which the brief
   forbids ("do not run git commands that change the tree"). **Not run**, for
   that reason — this is a property of running the script against an
   uncommitted diff, not a limitation found in this refresh.
   - Correction to the brief's premise: the previous log's "integration
     94/0/3 on the rebuilt runtime image" figure (2026-09-15 log, § 7) came
     from `bash .agents/docs/testing/styrman/integration.sh` (Tier 2), a
     *different* script — not from `deploy/verify-commit.sh`. That prior log
     never actually ran `deploy/verify-commit.sh` full end-to-end either; it
     explicitly replicated only `verify-commit.sh`'s own file-integrity
     checks (sha256sum, file-list diff, symlink find) by hand, the same
     method § 4 above uses here. No `verify-commit.sh` run with a runtime
     image build appears in this project's testing/ history for any harness
     refresh so far.

## Root cause (FAIL only)

Not applicable — Status is PASS.

## Coverage of changed code

- `image/harness/cmd/sh-agent/main.go`, `internal/harness/apply.go`,
  `internal/harness/adopt.go`, `internal/harness/inventory.go`,
  `internal/harness/render.go` — Go source changes exercised by the mirror's
  own `go test ./... -count=1 -v` (§4, 79 pass/0 fail) and `go vet` (clean);
  the attribution feature specifically by `TestAttributionNote` and
  `TestAttributionSet` (confirmed present and passing, §4).
- `image/harness/cmd/sh-agent/main_test.go`,
  `internal/harness/harness_test.go`, `internal/harness/pathguard_test.go` —
  test files themselves, run as part of the same `go test` invocation.
- No added `//nolint`, `//lint:ignore`, `t.Skip`, `.only(`, or similar
  suppressions found in `git diff` over this range.
- Remaining 19 changed files are prose/manifest (`COMMIT`, `README.md`,
  `REQUIREMENTS.md`, `ROADMAP.md`, `SHA256SUMS`, `context/AGENTS.md`, agent
  `.md` files, skill `.md` files) — exercised indirectly by `sh-agent list`
  (§4, count match), the SHA256SUMS/file-list check (§4), and the mirror
  fidelity diff (below).

## Mirror fidelity

- `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness image/harness` → exit 1, exactly 3 lines: `Only in image/harness: COMMIT`, `... SHA256SUMS`, `... VENDORED.md` — matches `image/harness/VENDORED.md`'s documented expected result. Log: `.agents/scratch/verify-harness-refresh-4aa19be/mirror-diff.txt`.
- `git -C ~/workspace/styrman-harness rev-parse HEAD` → `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`.
- `cat image/harness/COMMIT` → `4aa19be7eeb5b15e4b876832bd81807cd5d91b37` — matches.
- `git -C ~/workspace/styrman-harness status --porcelain` → empty (clean checkout).

## Not examined

- `deploy/verify-commit.sh` end-to-end (runtime image build + integration) —
  not run; see § 5 for the exact reason (it verifies a git commit, and this
  refresh is uncommitted).
- Tier 2 integration (`.agents/docs/testing/styrman/integration.sh`) and
  Tier 5 manual steps — not named in this brief and not run this round; last
  run in the 2026-09-15 log at a81ef4b (`passed=94 failed=0 skipped=3`).
- Web trio (`npm run check`/`lint`/`test`) and browser e2e — not run; diff
  does not touch `web/`.
- `find image/harness/skills -mindepth 2 -maxdepth 2 -name SKILL.md | wc -l`
  and the `agents/*.md` count were not independently re-run this round;
  relied on `sh-agent list`'s own reported 27/7 plus the unchanged file set
  (the refresh diff adds/removes no skill or agent files).

## Environment

Linux host, working from `/home/kesokaj/workspace/styrman-crew`. Go 1.25.0
(host); mirror module built with the pinned `golang:1.25.13-trixie` image
inside `deploy/vulncheck.sh` only. Plain Docker (no Colima); no Docker
containers built or run this round (verify-commit.sh and integration.sh both
skipped, see above).
