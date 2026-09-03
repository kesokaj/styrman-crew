# Harness replacement, phase 1: vendor and build     (date: 2026-09-03)

## Summary

The image now seeds the new `styrman-harness` (28 skills, 7 subagents, `sh-agent`, no hooks/scripts) instead of the old harness. `image/harness/` is a verbatim mirror of the `styrman-harness` checkout at commit `6893e2218414617da2bb88259b905ca3882b2659` (recorded in `image/harness/COMMIT`), refreshed by hand with `rsync -a --delete --exclude .git --exclude .agents --exclude .gitignore --exclude /COMMIT --exclude /VENDORED.md ~/workspace/styrman-harness/ image/harness/`. New `image/harness/VENDORED.md` documents provenance and the refresh procedure; `image/harness/SHA256SUMS` pins 151 files and is regenerated as the last step of every refresh. `image/Dockerfile` now runs a `harness-builder` stage `FROM golang:1.25.13-trixie` (same pin as `deploy/Dockerfile.server`) to build `cmd/sh-agent` with `CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off -trimpath`; the runtime stage copies `/styrman/bin/sh-agent` and `/styrman/harness/`, then `chmod -R go-w` on the tree. `image/.dockerignore` added. Gates learned about the third module: `deploy/verify-commit.sh` now runs `sha256sum -c` over the mirror, diffs the file list against `SHA256SUMS`, refuses symlinks, `go vet`s the module, asserts both Dockerfiles' golang pin equal, builds the runtime image and compares `sh-agent list`'s Skills/Subagents counts; `deploy/vulncheck.sh` scans `image/harness` too. Two slips fixed on the way: the old rsync had an unanchored `--exclude VENDORED.md` that silently kept the old tree's `shared/skills/VENDORED.md` and dropped the harness's own; `SHA256SUMS` regenerated (it had been generated before the final `VENDORED.md` edit).

## Type

feature, phase 1 of 5

## Files changed

- `image/harness/` — complete replacement: `context/`, `skills/`, `agents/`, `commands/`, `rules/`, `cmd/`, `internal/`, `go.mod`, plus repo docs (`README.md`, `AGENTS.md`, `CLAUDE.md`, `REQUIREMENTS.md`, `ROADMAP.md`, `TODO.md` verbatim); added `COMMIT` and rewritten `VENDORED.md`
- `image/harness/COMMIT` — styrman-harness SHA
- `image/harness/VENDORED.md` — provenance and refresh procedure
- `image/harness/SHA256SUMS` — pinned 151 files
- `image/Dockerfile` — added `harness-builder` stage, build `sh-agent`, copy to `/styrman/bin`, `chmod -R go-w`
- `image/.dockerignore` — new
- `deploy/verify-commit.sh` — added mirror/runtime-image/pin assertions (`sha256sum -c`, file-list diff, symlink check, `go vet`, toolchain pin equality, `sh-agent list` count checks)
- `deploy/vulncheck.sh` — added `image/harness` module scan
- `git show v1.19.1:image/harness/…` — old tree recoverable from git history

## Agents dispatched

verifier (`.agents/docs/testing/2026-09-03-verify-harness-phase1.md` PASS; round 2 PASS after security fixes), security (`.agents/docs/reviews/2026-09-03-security-harness-phase1.md`: round 1 = 2 medium / 8 low gates blind to module + unverifiable `COMMIT`, fixed; round 2 = mediums closed, provenance downgraded to low then closed by file-list diff; remaining items rebutted and recorded in TODO.md as upstream `styrman-harness` scope).

## Verification

- `docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/` succeeds
- `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:dev list --repo /styrman/harness` reports Skills (28), Subagents (7)
- `diff -rq --exclude .git --exclude .agents --exclude .gitignore ~/workspace/styrman-harness image/harness` shows only `COMMIT` and `VENDORED.md`
- `sh-agent version` prints `(devel)` (mirror has no `.git`); manifest commit is empty

## Requirements

R5 (identical defaults from the base image) stands unchanged. Proposed one-line edit to R5's "vendored from `devshell_prompt/harness`" wording awaits operator approval (REQUIREMENTS.md is immutable without it).

## Related

- ADR-062 (D1–D8: decisions for the entire replacement)
- Plan: `.agents/docs/plans/2026-09-03-image-harness-replacement.md`
