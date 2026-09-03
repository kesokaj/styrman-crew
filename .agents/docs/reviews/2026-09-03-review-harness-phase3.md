---
owner: reviewer
date: 2026-09-03
---
# Review — harness-phase3   (2026-09-03)

Range reviewed: `git diff -- internal/httpapi/setup.go web/src/routes/settings.tsx`, plus untracked `internal/httpapi/setup_roster_test.go`

## Verdict: APPROVE WITH NITS

Nothing broken. `rosterHome` correctly leaves readiness on the base home and only redirects the skills/agents/commands roster to the newest seeded project home; a stale/missing project home is handled (`files.Exists` returns `false` on `os.OpenRoot` failure, loop just continues); `rh == ""` correctly yields `nil` roster slices which `omitempty` drops from the JSON; the frontend guards `toolData` being undefined and never breaks on it. Verified `go test ./internal/httpapi/ -run TestRoster -v` passes and `go vet ./internal/httpapi/...` is clean. One real (low-severity) precision gap in the `.bak.` filter, plus a naming-consistency nit.

## Findings
- **[low / confidence: medium]** `internal/httpapi/setup.go:171` — `strings.Contains(n, ".bak.")` is an unanchored substring match, not the documented pattern `<name>.md.bak.<timestamp>`. A legitimately named file containing the literal substring `.bak.` anywhere (e.g. a skill/agent file that itself has "bak" in a compound name) would be silently dropped from the roster with no way to tell from the UI. Low impact because harness file names are vendored/controlled, not user-authored, but the filter would be more precise anchored to the actual backup suffix, e.g. `regexp.MustCompile(`\.bak\.\d{8}-\d{6}$`)` or at least `strings.Contains(n, ".md.bak.")`.
- **[low / confidence: low]** `web/src/routes/settings.tsx:194-200` — `rosterOf` labels the `agents` count "subagents" while the backend/API field and the rest of the codebase (`AGENTS.md`, `toolInfo.Agents`) call them "Agents"/"agents". Not a functional bug (values are correct, no consumer keys off the string), but worth confirming the UI wording matches the term used elsewhere on the same page before shipping — no other card currently says "subagents".

Neither finding blocks: both are precision/consistency notes, not concrete correctness or security failures. The `rosterHome` stat cost (≤ ~100+ small `os.OpenRoot`+`Stat` calls, worst case, on a 20 s poll) is not a performance concern at this scale and the common case (newest project already seeded) is O(1).

## Criteria
1. Dead code — pass: old `.gemini/config/plugins/harness/agents` path is fully replaced, no orphaned references; `listNames`'s old dotfile-strip-then-check reordering is behaviorally identical, not dead, just reworded.
2. Flow tracing — pass: `who(r)` → `rosterHome(user, sp)` → `sp.projects.List()` (newest-first, confirmed in `project/store.go:247`) → `files.Exists`/`files.ListNames` (confined `os.Root`, confirmed in `files/files.go`) → `toolInfo` → JSON `omitempty` → frontend `rosterOf`/`detailOf` all consistent end to end; single call site (`server.go:340`), no stale callers.
3. Standards conformance — pass: Go error/zero-value handling idiomatic, comments explain why not what, matches `golang` skill conventions.
4. Directory layout — pass: test lives beside `setup.go` as `setup_roster_test.go` in the same package, no parallel test tree.
5. Simplicity (DIY-except-security) — pass: no new dependency, stdlib string ops, matches D5's spec directly; no auth/crypto surface touched.
6. Decoupling — pass: `rosterHome` is a small new seam on `*Server` scoped to settings, doesn't reach into `project`/`files` internals, and the frontend change is isolated to `rosterOf` with no ripple into `detailOf` or other cards.
