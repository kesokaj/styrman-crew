---
owner: reviewer
date: 2026-09-15
---
# Review — worktree-code (2026-09-15)

Range reviewed: working tree, `image/harness/` mirror refresh only (COMMIT `cceee548…` → `a81ef4be…`). Files (blob hashes pinned below): AGENTS.md, COMMIT, README.md, ROADMAP.md, SHA256SUMS, agents/{bugs,changelog,documentation,explorer,reviewer,security,verifier}.md, cmd/sh-agent/{main.go,main_test.go}, context/AGENTS.md, internal/harness/{apply.go,harness_test.go}, skills/VENDORED.md, skills/{bug,distill,feature,golang,handoff,init-project,preflight,retro,review,review-standards}/…, skills/init-project/assets/agents-tree/docs/bugs/INDEX.md. No untracked files in scope.

## Verdict: APPROVE

This is a mechanical vendored-mirror refresh of `styrman-harness` (per R5, "no hooks, scripts, or settings files"; per BUG-058 its own tests are an observation, not this project's gate). All 155 `SHA256SUMS` entries verify against the working-tree files (`sha256sum` re-check, 155/155 match). `go build ./...` and `go test ./...` inside `image/harness` both pass clean. The two real Go edits (`cmd/sh-agent/main.go`, `internal/harness/apply.go`) are wording-only changes to the "foreign CLAUDE.md" message, and each is covered by a new test (`TestStatusNamesForeignClaudeMD`, the added assertion in `TestInstallStatusResyncUninstall`) that asserts the new string and would fail if it reverted — a real, revertible-behavior test, not a tautology. Cross-file consistency checks (README.md / AGENTS.md / apply.go / main.go wording for the CLAUDE.md guidance; the `R2` reference in the new test comment resolving to `image/harness/REQUIREMENTS.md`) all hold. Nothing in this diff touches `styrman-crew`'s own source, tests, or `REQUIREMENTS.md`.

## Findings
None affecting correctness, security, or a stated requirement/rule. No source or test edits outside the harness mirror; no `REQUIREMENTS.md` or `AGENTS.md`-rule violations found.

## Criteria
1. Dead code — pass. No unused symbols/files introduced; the diff only edits doc/skill markdown and two message strings plus their tests.
2. Flow tracing — pass. The two changed Go strings are exercised end-to-end by new tests that fail on revert; `SHA256SUMS` entries recompute correctly for every listed file; `COMMIT` was bumped alongside the content it pins.
3. Standards conformance — pass. Vendored content only; the harness module's own `go build`/`go test` are green, satisfying styrman-crew's "no hooks/gates/CI" posture for this mirror (R5).
4. Directory layout — pass. All changes stay inside the existing `image/harness/` mirror tree; nothing landed outside it.
5. Simplicity — pass. No new dependency; the code change is a one-line string edit with a matching test, not an abstraction.
6. Decoupling — pass. Changes are confined to the vendored harness's own files; no reach-in to `internal/`, `cmd/styrman`, or `web/`.

## Not examined
- Did not diff against the actual upstream `styrman-harness` repo at commit `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8` (no network access) — correctness of the mirror was checked only for internal self-consistency (SHA256SUMS, build, tests, cross-references), not against the upstream source of truth.
- Did not review `image/harness/skills/anti-ui-slop/` or other vendored-verbatim skill directories not in this diff's file list.
- Did not re-run the wider `styrman-crew` test suites (root `go test ./...`, web, integration, e2e) since this range touches none of that code.
