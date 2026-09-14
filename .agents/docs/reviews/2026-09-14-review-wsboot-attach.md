---
owner: reviewer
date: 2026-09-14
---
# Review — wsboot-attach (BUG-060 fix: terminal-WS boot self-attaches before upgrade)

Range reviewed: uncommitted working-tree diff of `internal/httpapi/server.go`,
`internal/orchestrator/orchestrator.go` (`git diff` against HEAD) plus the new
untracked `internal/httpapi/wsboot_attach_test.go`, read whole.

Pinned blobs (HEAD `d938605d9ff1eee1f93c7522dbc4c72953ef59d5`):
- `internal/httpapi/server.go` → `51bc7e0c62a957b7d3758103054a0f06c5fb7709`
- `internal/orchestrator/orchestrator.go` → `f5cd95db4056e0317fe6c1003a4cf94010ab8372`
- `internal/httpapi/wsboot_attach_test.go` → `222f36c7f070d0107c621420d8a1d65295699ab9`

## Verdict: APPROVE WITH NITS

The fix is correct and the comments describe true behaviour. I traced
`bridge.go`'s `ServeHTTP` (unchanged in this diff) and confirmed `b.ensure`
(wired to `s.ensureFor` → `wsBoot`) runs at line 172, strictly before
`b.serve` performs the WebSocket upgrade at line 180 — the ordering claim in
the new `wsBoot` doc comment is true today. `EnsureRunning`'s "no
EnsureSelfAttached here" NOTE was updated accurately to describe both the
new synchronous WS path and the pre-existing goroutine-after-response paths
(`createProject`, `startProject`, LAN toggle, `provisionRepo`, boot-time
`Adopt`) — I checked each call site and the description holds. Build, vet,
and the two new tests pass. The one real gap is that nothing pins the
ordering *relative to the WS upgrade* itself — only `wsBoot`'s internal
call order is tested with a fake; a future reordering inside `ServeHTTP`
would silently break the invariant the comments assert, with no test to
catch it.

## Findings

- **[low / confidence: medium]** `internal/terminal/bridge.go:172,180` (unchanged, but the invariant the diff's comments assert lives here) — the new test (`wsboot_attach_test.go`) only pins that `wsBoot` calls `EnsureRunning` then `EnsureSelfAttached` in order; it does not pin that `ServeHTTP` calls the boot function before `b.serve`'s upgrade. Nothing regresses today, but the specific property the dispatch asked about ("call ordering relative to the WebSocket upgrade") is asserted only in a comment, not in a test that would fail if someone moved `b.ensure(...)` after `b.serve(...)`. Consider a `bridge_test.go` case (fake `ensure`/upgrade order) if this path gets touched again.

- **[low / confidence: low]** `internal/httpapi/wsboot_attach_test.go:38`, `internal/httpapi/server.go:296` — both cite "BUG-060" as an established id, but `.agents/docs/bugs/INDEX.md` has no BUG-060 entry yet (latest is BUG-059). Not a code defect — likely sequenced for the `bugs` subagent later in the Definition of Done — but flagging so the record doesn't get dropped: the comments/tests now forward-reference an id that must exist on disk before this lands.

- **Non-finding, checked and confirmed sound:** ignoring `EnsureSelfAttached`'s "fresh" bool in `wsBoot` is correct, not an oversight. Its doc comment says a request-path caller should redirect on `fresh==true` because a fresh attach can disturb *in-flight* connections; here nothing has been written to the response yet (no upgrade has happened), and `EnsureSelfAttached` itself sleeps 500ms after a fresh attach before returning, so the settle window elapses before `b.serve` ever calls `websocket.Accept`. The `wsBoot` comment's rationale matches the code exactly.

## Criteria

1. **Dead code** — pass. No orphaned code; the old inline `s.orch.EnsureRunning` call is fully replaced by `wsBoot`.
2. **Flow tracing** — pass. `EnsureRunning`'s ctx flows through as before; `EnsureSelfAttached` correctly takes no ctx (by its own documented design, detached background context). Error path: a boot failure returns early and does not attach (pinned by `TestWSBootDoesNotAttachWhenTheBootFails`). Traced every other `EnsureSelfAttached` call site (5 of them) to confirm the new NOTE's "goroutine after response" characterization is accurate for all of them.
3. **Standards conformance** — pass. Idiomatic Go, small consumer-defined interface, doc comments explain "why" per project convention.
4. **Directory layout** — pass. Test sits beside the code it covers (`wsboot_attach_test.go` next to `server.go`'s package), new file name doesn't collide with anything (writing-files.md guard).
5. **Simplicity (DIY-except-security)** — pass. No new dependency; the interface has a real second implementation-shape need (test double), not speculative — matches the existing `guestRuntime`/`modeRuntime` pattern in the same file.
6. **Decoupling** — pass. `wsBooter` is a minimal, consumer-defined seam (2 methods) satisfied structurally by `*orchestrator.Orchestrator`; no cross-package reach-in, no existing caller's signature changed.

## Not examined

- Did not run the full `go test ./...` or the integration/e2e suites (out of scope: dispatch limited review to these three paths).
- Did not verify BUG-060's own record file/INDEX entry beyond confirming it does not yet exist — that's the `bugs` subagent's job, noted above only as a sequencing flag.
- Did not review `internal/terminal/bridge.go` as a diff (it is unchanged) — only read it to verify the ordering claim the new comments make.

## Outcome

- **[low/medium] ordering not pinned relative to the upgrade — FIXED.**
  `internal/terminal/ensure_before_upgrade_test.go`
  (`TestBootCallbackRunsBeforeTheUpgrade`): a Bridge with a failing boot
  callback served through an `httptest.ResponseRecorder` (which cannot be
  hijacked, so a handler that upgraded first would fail there and never
  reach the callback). Asserts the callback ran once with the caller's
  identity, the response is a plain 500 carrying the boot failure, and no
  `Upgrade` header was set. `go test ./internal/terminal/` green.
- **[low/low] BUG-060 cited before it exists — ACCEPTED, sequencing.** The
  `bugs` agent is dispatched with that id in the same change; the record
  and its INDEX row ride in the fix commit (bug skill, step 5).
- **Non-finding (fresh return ignored) — agreed**, no change.
