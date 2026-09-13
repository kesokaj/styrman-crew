---
owner: reviewer
date: 2026-09-13
---
# Audit — internal/terminal, internal/conversation, internal/history, internal/files   (2026-09-13)

Scope: all non-test `.go` files under `internal/terminal/`, `internal/conversation/`,
`internal/history/`, `internal/files/`, at commit `e56e775` (clean tree). This is
an `/audit` scan against recorded intent (AGENTS.md, REQUIREMENTS.md R1–R13,
the three newest `.agents/docs/changes/` entries, `.agents/docs/specs/styrman/`,
`.agents/docs/bugs/INDEX.md`), not a diff review — there is no PR range; the
whole slice as it stands at HEAD was read.

Files read in full: `internal/terminal/bridge.go`, `internal/terminal/autostart.go`,
`internal/conversation/conversation.go`, `internal/conversation/store.go`,
`internal/conversation/names.go`, `internal/history/history.go`,
`internal/files/files.go`. Callers traced in `internal/httpapi/explorer.go`,
`internal/httpapi/server.go`, `internal/httpapi/git.go`, `internal/config/config.go`.

## Verdict: APPROVE WITH NITS

Nothing broken enough to block. This slice is unusually well hardened —
almost every non-obvious line carries a comment citing the review or bug
(BUG-002/003/004/039 etc.) that shaped it, and cross-checking against
`bugs/INDEX.md` and `REQUIREMENTS.md` R3/R6/R7/R13 turned up no
reintroduced defect and no requirement violation. The two things worth
fixing: one real dead-code/attractive-nuisance function in `internal/files`,
and one place where the package's own documented error-propagation
discipline (established elsewhere in the same slice, e.g. `history.Read`'s
comment) is not applied to the analogous case in `files.Read`.

## Findings

- **[medium / confidence: high]** `internal/files/files.go:174-199` (`ContainedPath`) —
  Dead code: zero callers outside its own test (`internal/files/files_test.go:147-168`).
  Its doc comment says it exists "chiefly" for exec-based tools like git
  (`git -C <path>`), but `internal/httpapi/git.go` never calls it — every git
  operation there runs inside the guest via `docker exec`, never against a
  host path. The function is also the one place in the package that is
  *not* confined by `os.Root`: it resolves via `filepath.EvalSymlinks` with
  an explicit check-then-use window (the comment says as much: "no
  check-then-use window" for everything else, implying this one has one).
  An unused function with a TOCTOU window, in a package whose entire
  raison d'être (per its header comment) is eliminating exactly that
  window, is a landmine for the next caller who reaches for it. Fix: delete
  it and its test, or if a real host-path caller is still planned, wire it
  in now and note the caller in the doc comment.

- **[medium / confidence: medium]** `internal/files/files.go:296-298` (`Read`) —
  `n, _ := io.ReadFull(f, buf)` discards the error. `io.ReadFull` returns
  `nil` (full buffer), `io.EOF` (nothing read) or `io.ErrUnexpectedEOF`
  (partial, short file) for the expected "file shorter than the 2 MiB cap"
  cases — all fine to ignore. But it also returns the underlying reader's
  *other* errors verbatim (e.g. a real disk/NFS I/O fault mid-read), and
  those are silently swallowed here: the function proceeds to serve
  `buf[:n]` as `FileContent.Content` with `Truncated` computed from
  `st.Size() > int64(n)`, i.e. a genuine I/O failure is presented to the
  file-viewer UI as an ordinary "file bigger than the cap, showing the
  first N bytes" truncation, with no error surfaced anywhere. This is the
  exact failure mode the package itself calls out and fixes two paragraphs
  over in the sibling package: `internal/history/history.go:244-246`'s
  comment on its own `io.ReadAll` reads "a real I/O error must not be
  presented as a silently short transcript." `files.Read` doesn't apply
  that same discipline to itself. Fix: capture the error, and treat
  anything other than `io.EOF`/`io.ErrUnexpectedEOF` as a hard error
  (`return nil, err`).

- **[low / confidence: low]** `internal/terminal/bridge.go:459-471` —
  Close-code precedence: `revoked.Load()` sets `code, reason =
  CloseAccessRevoked, "access revoked"` (line 460-464), but the very next
  unconditional check `if !gone && s.project != "" &&
  b.orch.Stopped(s.user, s.project)` (line 469) overwrites `code`/`reason`
  to `CloseProjectStopped` without checking `revoked`. The actual close
  frame the browser sees was already sent by the `RevocationWatch`
  callback at line 384 (`conn.Close(..., "access revoked")`), so the
  client-visible behavior is unaffected — this second `conn.Close` call at
  line 477 is a no-op on an already-closed connection. The only
  consequence is the server log line at line 475 can say "project stopped"
  for a session that was actually closed for access revocation, if an
  admin revocation and an idle-timeout stop land on the same session at
  nearly the same moment. Cosmetic/log-only; fix by adding `&&
  !revoked.Load()` to the line-469 guard if it's worth the one-line change.

- **[low / confidence: low]** `REQUIREMENTS.md` Constraints ("approved deps:
  `coder/websocket`, `creack/pty`, `docker/docker` SDK") vs.
  `internal/files/files.go:29` (`golang.org/x/sys/unix`, direct dependency
  in `go.mod:8`) — `x/sys/unix` is not in the requirements' approved-deps
  list, though its one use (`ReadRawNoFollow`, `files.go:448-489`) is
  justified in-code by name and date ("x/sys/unix, not syscall: ...
  security review 2026-09-10"). This reads more like a stale requirements
  table than an actual violation of intent, but as written the doc and the
  dependency disagree. Fix: add it to the Constraints list in one line, or
  note in the comment that the requirements table is non-exhaustive.

## Criteria

1. **Dead code** — FAIL: `files.ContainedPath` has no production caller (see finding above). Everything else exported in the four packages traces to a real caller (checked every exported symbol in `files.go`, `history.go`, `conversation.go`, `store.go`, `bridge.go` against `internal/httpapi` and each other).
2. **Flow tracing** — mostly pass; one gap (`files.Read`'s discarded I/O error, above). Context/cancellation in `terminal/bridge.go` is threaded correctly end to end (`hctx` cancels the exec, the heartbeat, the revocation watch, and unblocks the WS reader on any exit path); every `os.Root`/file handle in `files.go` and `history.go` is closed on every return path (verified by inspection, `defer` used consistently).
3. **Standards conformance** — pass, with the one inconsistency above (files.go vs. history.go's own stated discipline on I/O errors). Error wrapping: no case found where a wrapped `err` was discarded with `_` inappropriately (the two `_ = ...` cases in `bridge.go`/`conversation.go` are deliberate best-effort operations, commented as such).
4. **Directory layout** — pass: tests sit beside their source in each package; no stray files; `internal/` placement matches `golang` skill and `AGENTS.md`'s Key Files table.
5. **Simplicity** — pass overall (stdlib-first; `x/sys/unix` and `coder/websocket` are the only non-stdlib touches in this slice, both justified in comments); the REQUIREMENTS.md approved-deps list is just out of date (low finding above). No hand-rolled auth/crypto/session logic in this slice.
6. **Decoupling** — pass: `terminal.TermStore` is a two-method interface sized to what `Bridge` needs, declared where consumed; `files`/`history`/`conversation` have a clean one-directional dependency (`history` → `files`, `httpapi` → all three) with no reach-into-internals found.

## Not examined

- `internal/terminal/autostart_test.go`, `close_test.go`, `reauth_test.go`,
  `setuptool_test.go`, and all `_test.go` files in the other three packages
  — read for context (grepped, not read line-by-line) but not reviewed as
  their own artifact per the dispatch (non-test `.go` files only).
- Callers outside the four packages beyond `internal/httpapi` (e.g.
  `cmd/styrman` wiring of `terminal.NewBridge`, `internal/orchestrator`'s
  `Exec`/`ExecSpec`/`Status` implementations that `conversation.Manager`
  and `terminal.Bridge` depend on) — traced only far enough to confirm the
  call shape matches, not reviewed for their own defects.
- `internal/httpapi/browser.go` — shares `RevocationWatch` and the
  reauth pattern with `terminal/bridge.go` (per the 2026-09-13 browser
  changes entry) but is outside the four directories named in scope.
- Runtime behavior — no test run or build performed as part of this audit;
  findings are from static reading only.
