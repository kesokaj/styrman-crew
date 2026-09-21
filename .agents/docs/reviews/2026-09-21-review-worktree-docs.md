---
owner: reviewer
date: 2026-09-21
---
# Review — worktree-docs   (2026-09-21)

Range reviewed: working tree — changed files per `git diff HEAD --name-only` (deleted `image/harness/**` mirror, deleted host harness channel files `internal/orchestrator/harness.go`, `harness_test.go`, `internal/config/{harnessdir,styrman_harness,styrman_ctl_harness}_test.go`, `deploy/update/styrman-harness-update.{service,timer}`, `.agents/docs/testing/styrman/make-scratch-harness.sh`; modified `image/Dockerfile`, `image/entrypoint.sh`, `image/README.md`, `deploy/update/styrman-update`, `deploy/update/styrman-update.service`, `deploy/styrman-ctl`, `deploy/host-setup.sh`, `deploy/docker-compose.yml`, `deploy/verify-commit.sh`, `deploy/vulncheck.sh`, `deploy/README.md`, `deploy/caddy/VENDORED.md`, `internal/config/config.go`, `internal/config/styrman_ctl_test.go`, `internal/config/styrman_update_test.go`, `internal/orchestrator/orchestrator.go`, `README.md`, `AGENTS.md`, `ROADMAP.md`, `.agents/docs/specs/styrman/{api,deploy,guide,runtime-image,versioning}.md`, `.agents/docs/deploy/remote.md`, `.agents/docs/testing/styrman/{README.md,integration.sh,ensure-test-image.sh}`; untracked `internal/config/entrypoint_guard_test.go`)

## Status: COMPLETE

## Verdict: APPROVE

This is the ADR-074 change: the host-side "harness channel" (ADR-073, shipped v1.35.0) is retired and the agent harness (`styrman-harness`) is instead baked into the runtime image at build time as a signed `sh-agent` release binary downloaded from GitHub Releases, with a never-downgrade guard so a project that ran `sh-agent update` itself is left alone on the next image bump. I read the full source diff (config, orchestrator, Dockerfile, entrypoint, styrman-update, styrman-ctl, host-setup.sh, verify-commit.sh, vulncheck.sh, docker-compose.yml) and every changed/added doc and test file named in scope, and grepped the living docs, rules, skills, and test suites for every old-wording token the dispatch listed. The documentation sweep is unusually thorough and internally consistent: README.md's top paragraph, AGENTS.md's architecture bullet and Key Files table, both specs (`deploy.md`'s "Agent harness in the image (ADR-074)" section, `runtime-image.md`, `versioning.md`, `guide.md`), `deploy/README.md`, `.agents/docs/deploy/remote.md`, and the Tier 2/Tier 5 testing README section were all rewritten together, the deleted-mechanism list (`deploy.md:281-284`) names every removed host artifact (`styrman-harness-update.{service,timer}`, `--harness-token-file`, `/opt/styrman-harness`, `/etc/styrman/gh-token-harness`, `STYRMAN_HARNESS_DIR`, the server's read-only binds), and the Tier 2 check-count line (115, `passed=112`) matches `grep -c 'check "' integration.sh` = 115. I found no stale restatement of the retired mechanism and no dropped rule without a new home.

The one place the old design is still described as current — REQUIREMENTS.md R5 — is deliberate per the dispatch (its edit awaits operator approval) and is listed below as `left`, not a defect.

## Findings

None. No stale restatement of ADR-073 (`harness channel`, `STYRMAN_HARNESS_DIR`, `HARNESS_REPO_DIR`, `harness-token-file`, `styrman-harness-update`, `make-scratch-harness`, `/opt/styrman-harness`, `harness_main`, `gh-token-harness`, "plain copy"/"harness-builder" in the harness sense, "source channel tag", "version+commit") survives in a living doc, rule, skill, or test suite: every hit `git grep` returned is either (a) `allowed_signers`/`gh-token` used in the unrelated crew-updater sense (`deploy.md:47,345,346,456,631`, `versioning.md:265`), which was never part of ADR-073 and is correctly untouched, or (b) a hit inside the historical/backward-looking prose that correctly narrates the retirement itself (`AGENTS.md:84`, `deploy.md:281-284`, `runtime-image.md:232`) — these describe what was superseded, in past tense, next to the ADR-074 replacement, which is exactly right for a spec that must still make sense of the image's `ADR-062`/`ADR-073` history. README.md's "Status" changelog entry for the already-shipped v1.35.0 release (`README.md:117-127`) still narrates the old channel, but that is a historical per-release changelog line, not a current-state claim, and this working tree has not yet cut a release for ADR-074 (ROADMAP.md's "Now" correctly points at the open plan instead).

## Criteria
1. Dead code — n/a — docs lens (source-side dead code was not assessed beyond what the deleted-mechanism doc list already accounts for)
2. Flow tracing — n/a — docs lens
3. Standards conformance — n/a — docs lens
4. Directory layout — n/a — docs lens
5. Simplicity (DIY-except-security) — n/a — docs lens
6. Decoupling — n/a — docs lens

## Left
- `REQUIREMENTS.md:47-58` (R5) — still describes the old host harness channel ("carried by the **host's verified harness channel** … ADR-073") as current. **left: operator approval pending**, per the dispatch — not scored as a defect. Note for whoever picks this up: once approved, the edit should also touch the sentence's `image/harness/COMMIT` mirror reference, which is likewise gone.

## Not examined
- The Go source diff (`internal/config/config.go`, `internal/orchestrator/orchestrator.go`, and the deleted/added `*_test.go` files) was read for the docs-lens purpose of confirming the mechanism actually changed as the docs claim, but was not given a full six-criteria code review (out of scope per the dispatch's docs-lens brief).
- `.agents/docs/testing/guide.md` — absent (no such file in this repo; the project's guide is `.agents/docs/testing/styrman/README.md`, which was reviewed).
- `web/e2e/` — grepped for old-wording tokens (no hits) but not read line-by-line; the dispatch's scope list does not include any changed `web/` file, so no e2e change was expected in this diff.
- `image/Dockerfile`'s and `image/entrypoint.sh`'s full byte-for-byte content beyond the diff hunks (read via `git diff`, not `Read` in full) — the diff context was sufficient to verify the doc claims made about them.
- `.agents/docs/changes/` — intentionally not grepped for old wording, per the dispatch's explicit exclusion (changelog/handoffs/reviews/retros/bugs/dated logs are not "living docs" for this check).

## Outcome
