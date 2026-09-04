---
owner: reviewer
date: 2026-09-04
---
# Review — guest-seam   (2026-09-04)

Range reviewed: working-tree diff at `/tmp/claude-1000/-home-kesokaj-workspace-styrman-crew/a1fcfb7b-6662-4bce-8e06-59c0ae200ac4/scratchpad/seam.diff` (matches `git status`: modified `check.go`, `credentials.go`, `credharvest.go`, `explorer.go`, `format.go`, `git.go`, `server.go`, `stats.go`; untracked `internal/httpapi/guestexec_test.go`)

## Verdict: APPROVE WITH NITS

Nothing broken. All 15 call sites (verified by grep and by reading each in full: `check.go`, `credentials.go`, `credharvest.go` x2, `explorer.go` x3, `git.go` x3, `server.go` x2, `stats.go` x2) are a pure receiver rename (`s.orch.` → `s.guest.`) with identical arguments, identical error handling, and identical control flow — confirmed the remaining ~20 `s.orch.` call sites are exactly the other orchestrator methods the seam deliberately excludes (`Adopt`, `EnsureRunning`, `Destroy`, `IPAddress`, etc.), so the interface is as narrow as its doc comment claims. `orchestrator.Orchestrator.Status`/`.Exec` signatures match `guestRuntime` exactly; `gofmt -l`, `go vet`, and `go build ./...` are clean, and all 24 new tests pass and were checked against the actual handler source (git.go's checkout/commit/fetch/pull/log/stash/diff, format.go, check.go) line-by-line — each assertion matches a real, currently-enforced argv/status decision, not a restatement of the fake. The one real gap is that the suite's own stated justification for `fakeGuest`'s mutex — concurrent access from the background project-boot goroutine — never actually exercises that path in this test file, so the concurrency design is unverified, not merely undocumented.

## Findings

- **[low / confidence: high]** `internal/httpapi/guestexec_test.go:246` (`fakeGuest` doc comment) and `guestTest` (`:319-336`) — the comment says the fake must be thread-safe because "creating a project kicks off a background boot that also reaches the guest, so a test's assertions race that goroutine." In fact `guestTest`'s `cfg` never sets `Image`, so `createProject` (`server.go:793-797`) takes the `s.cfg.Image == ""` early-return and skips the `go func(){...}()` that calls `s.guest.Exec` entirely — confirmed by reading `createProject` and by the fact none of the 24 tests ever see a stray call in `f.calls` before their own assertions. The mutex itself is correctly implemented and harmless, but the concurrency scenario it was written to guard is not exercised by anything in this file, so a real race in that path (were one introduced later) would not be caught here. Not a bug in the diff as shipped; worth either exercising the real boot path once (set `Image` and let one test race it) or correcting the comment to say the mutex is precautionary, not exercised.

## Criteria

1. Dead code — pass. `s.orch` remains live (used by ~20 other call sites); no orphaned code from the rename.
2. Flow tracing — pass. Traced all 15 call sites caller-to-callee; arguments, contexts, and error propagation are byte-for-byte unchanged from the pre-diff `s.orch` calls. `New()` sets `guest: orch` alongside `orch: orch`, so production behavior is identical (same concrete object behind both fields).
3. Standards conformance — pass, with one non-blocking nit: the new `guestRuntime` doc comment sits directly above the type with no blank line, and it's preceded by (and now absorbs) the pre-existing `// Server wires the API handlers to their dependencies.` comment, so godoc would attribute that whole block to `guestRuntime` and `Server` loses a dedicated doc comment. Cosmetic only — no correctness impact, not raised as a Finding above.
4. Directory layout — pass. Test lands beside the package it covers (`internal/httpapi/guestexec_test.go`), matching existing sibling test files (`check_test.go`, `format_test.go`, `git_verbs_test.go`); no naming collisions with those files' helpers.
5. Simplicity (DIY-except-security) — pass. Two-method interface, single concrete implementation in production plus one test fake — exactly the shape the skill asks for (a real second "caller" being the tests), not speculative; no new dependency.
6. Decoupling — pass. Interface declared in the consuming package (`httpapi`), sized to exactly what 15 call sites need, deliberately excludes the other ~15 orchestrator methods per its own doc comment (verified by grep) — a real seam, not a wrapper-of-everything.
