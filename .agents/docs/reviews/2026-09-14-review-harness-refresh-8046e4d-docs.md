---
owner: reviewer
date: 2026-09-14
---
# Review — harness-refresh-8046e4d-docs   (2026-09-14)

Range reviewed: v1.30.1..worktree (working tree; changed files per `git diff HEAD --name-only` + untracked files under `image/harness/`)

## Verdict: APPROVE

Docs-lens review only (no source/config/test surface reviewed for correctness beyond cross-checking the facts the docs assert). Every doc-facing file touched by this refresh (`README.md`, `.agents/docs/specs/styrman/runtime-image.md`, `.agents/docs/testing/styrman/README.md`, `image/README.md`, `image/harness/VENDORED.md`, `image/harness/skills/VENDORED.md`, `deploy/verify-commit.sh`, `image/entrypoint.sh`) was checked against the stated facts (mirror commit, skill/subagent counts, plain-copy install behavior, dropped `GIT_CONFIG_GLOBAL` line, integration-check counts) and against the code that actually implements them (`image/harness/cmd/sh-agent/main.go`, `image/harness/skills/`, `.agents/docs/testing/styrman/integration.sh`). Everything matches. No statement in the changed docs contradicts the code, and no changed interface/command/deploy-step surfaced in the diff is left undocumented.

## Findings

None that meet the bar (a doc statement that no longer matches the code, or an undocumented changed interface/command/deploy step).

Checked and confirmed consistent (not findings, recorded as verification):
- `image/harness/skills/` now has 27 dirs after removing `audit/` and `gcloud-auth-verification/` and adding `review/` (untracked) — matches `runtime-image.md:235`, `image/README.md:8`, `.agents/docs/testing/styrman/README.md` (27 skills / 7 agy agents, three places), and `integration.sh:114` (`27 skills and 7 agy agents seeded`).
- `integration.sh` now has exactly 97 `check`/`skip` call sites (97 `check "` matches via full grep, 5 `skip "` sites reachable as 3 in a green run) — matches the README's `passed=94 failed=0 skipped=3` claim (97 total, 3 always-skip on firewall-hardened hosts). The harness section gained the two new checks (`sh-agent status labels the image mirror a plain copy`, `boot log records the harness deployed at the mirror's commit`), bringing the harness-related check count from 5 to 7 (6 in the "agent harness" block + the boot→ready check), matching the testing README's "seven checks" wording.
- `image/entrypoint.sh`'s install line dropped `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` and the surrounding comment correctly explains why (plain-copy install never calls git, per harness BUG-015) — confirmed against `main.go`'s `IsCheckout`/plain-copy path. The unrelated `GIT_CONFIG_GLOBAL` reference in `.agents/docs/specs/styrman/api.md:43` is a different subsystem (`internal/gitrepo`, host-side bootstrap git) and is untouched/still accurate — not a stale duplicate.
- `sh-agent install --dry-run` and `status` output strings quoted in `.agents/docs/testing/styrman/README.md` (`[dry-run] plain copy, not its own git checkout: the running binary would be installed as is`; `repo /styrman/harness (plain copy, not its own git checkout; update is refused)`) match `main.go:143` and `main.go:234` verbatim.
- `image/harness/skills/VENDORED.md`'s removal of the `gcloud-auth-verification` row (merged into `gcloud-setup`) matches the actual directory listing (no `gcloud-auth-verification/` dir remains, `gcloud-setup/` present).
- `image/harness/VENDORED.md`'s new "A plain copy, by design" section (replacing the old "Known cosmetic gap") accurately describes `IsCheckout`, the empty manifest commit field, and the exact `sh-agent status` wording.
- `deploy/verify-commit.sh`'s comment change from "151 files" to "150-odd files" is a reasonable approximation of the current `SHA256SUMS` (154 lines) — no material mismatch.
- No live (non-history) doc still says "28 skills", "Skills (28)", `/audit` as a command, or references `gcloud-auth-verification` as installed; all remaining hits are in `.agents/docs/{changes,reviews,testing,plans,handoffs}` dated records, which are history, not findings, per the brief.
- `.agents/docs/specs/styrman/versioning.md`'s release procedure (release entry in `changes/` names the new `COMMIT`) matches what `VENDORED.md`'s updated refresh section now says.
- The new `--vendor` flag on `sh-agent install`/`status`/`update` is an internal capability of the vendored harness tool; `image/entrypoint.sh`'s own invocation omits it (default behavior unchanged, still installs both vendors), so no styrman-side doc needed updating for it.

## Criteria (docs lens — source/test/config review out of scope)

1. Dead code — n/a — docs lens
2. Flow tracing — n/a — docs lens
3. Standards conformance — n/a — docs lens
4. Directory layout — n/a — docs lens
5. Simplicity (DIY-except-security) — n/a — docs lens
6. Decoupling — n/a — docs lens

## Not examined

- No correctness/security review of the Go source changes in `image/harness/internal/harness/*.go`, `image/harness/cmd/sh-agent/main.go`, or the new `main_test.go` — this dispatch was docs-lens only.
- `.agents/docs/specs/styrman/deploy.md` does not exist in this repo (checked; not a gap — `.agents/docs/deploy/{local,remote,gcp}.md` is the actual deploy-guide location per `AGENTS.md`, and none of those three reference stale harness facts).
- Did not verify every one of the ~150 individual `SHA256SUMS` hashes or diff every unchanged file under `image/harness/skills/` byte-for-byte against upstream 8046e4d5f — relied on directory listings and the targeted greps above.
- Did not check whether a `.agents/docs/changes/` entry for this specific harness-refresh work exists yet (working tree is uncommitted; newest entry on disk is `2026-09-13-v1.30.1-release.md`) — out of scope per the brief (changes/ entries are history, not findings).
