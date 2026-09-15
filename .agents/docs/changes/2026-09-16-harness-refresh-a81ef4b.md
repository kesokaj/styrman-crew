---
owner: changelog
date: 2026-09-16
---
# Harness refresh to commit a81ef4b (2026-09-16)

## Summary

Vendored harness mirror (`image/harness/`, a plain .git-less copy) refreshed from styrman-harness commit `cceee548ea8bf8ca1e4b3f6b561383a88bea7aa4` to **`a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8`**. Recipe: `image/harness/VENDORED.md` § Refresh run verbatim (`rsync --delete`, COMMIT, SHA256SUMS regenerated over 155 files, `diff -rq` printed exactly the three expected `Only in image/harness` lines: COMMIT, SHA256SUMS, VENDORED.md). The harness upstream: 21 commits of the "harness practices G01–G26" plan, including its Phase 7. Changes to skills, agents, and context text; the installer's `keep` line and `sh-agent status`'s note for a hand-written `~/.claude/CLAUDE.md` now say to move its content into a rule file under `rules/` in the harness repo; `/init-project` can update an older scaffold. Skills count remains 27; subagents remain 7. No crew-side edits outside the mirror. No release cut, nothing tagged or pushed: the release that ships this COMMIT is the operator's, per `.agents/docs/specs/styrman/versioning.md`, and its release entry names the full COMMIT SHA.

## Type

task

## Files changed

- `image/harness/` — 32 modified (30 files upstream changed, plus COMMIT and SHA256SUMS); 209 insertions, 123 deletions; no files added or deleted
  - `cmd/sh-agent/main.go`, `internal/harness/apply.go` — two message strings, one comment
  - `cmd/sh-agent/main_test.go` — one test added (`TestStatusNamesForeignClaudeMD`)
  - `internal/harness/harness_test.go` — assertion added to `TestInstallStatusResyncUninstall`
  - Remaining 26 files: the harness's `AGENTS.md`, `README.md` and `ROADMAP.md`, `context/AGENTS.md`, seven `agents/*.md`, and 15 files under `skills/`

## Agents dispatched

verifier, reviewer (code lens), reviewer (docs lens), security, changelog

## Verification

From the verifier log `.agents/docs/testing/2026-09-15-verify-harness-refresh-a81ef4b.md`, Status PASS: `go vet ./...` rc 0; `go test ./...` rc 0; `(cd deploy/caddy/ingress && go test ./...)` rc 0. `bash deploy/vulncheck.sh` rc 3: GO-2026-4887 (root and ingress modules) and GO-2026-4883 (ingress), in github.com/docker/docker v28.5.2+incompatible, the accepted pair the script names; `image/harness`: "No vulnerabilities found." Mirror module: `sha256sum -c` rc 0; file-list check clean; `gofmt -l .` empty; `go vet ./...` rc 0; `go test ./... -count=1` rc 0, 32 pass, 0 fail, 1 skip; test list vs HEAD: 1 added (`TestStatusNamesForeignClaudeMD`), 0 removed. Mutation check in a scratch copy: reverting each of the two message strings made its covering test FAIL. `bash .agents/docs/testing/styrman/integration.sh` rc 0, `passed=94 failed=0 skipped=3 aborted=0`, against `styrman-runtime:test` rebuilt by `ensure-test-image.sh` from the refreshed tree; the check "boot log records the harness deployed at the mirror's commit" passed; `/styrman/harness/COMMIT` matches. Tier 5 manual steps 1–2: `sh-agent list` shows 27 skills/7 subagents; `install --dry-run` prints plain-copy dry-run line; COMMIT matches; `harness-migrate.sh` present; no `/styrman/harness/.git` in the image. Not run: the web check, lint and unit tests and browser e2e (`web/` untouched); Tier 5 steps 3–5; `deploy/verify-commit.sh`. The verifier was resumed twice: once after it stopped at its turn limit, once after it ended its turn while `integration.sh` still ran as its background task.

## Review

Merged record `.agents/docs/reviews/2026-09-15-review-worktree-merged.md`: code lens (`reviewer`) APPROVE, no findings; docs lens (`reviewer`) APPROVE, no findings; `security` FINDINGS, 2 medium, 3 low, none high (`.agents/docs/reviews/2026-09-15-security-worktree.md`), all five in vendored upstream text and marked `left: styrman-harness` in the record's Outcome.

## Docs

No living styrman doc made stale by the refresh. Documentation was not dispatched.

## Requirements

n/a. Bugs: none recorded.
