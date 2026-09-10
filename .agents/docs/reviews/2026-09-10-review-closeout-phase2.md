---
owner: reviewer
date: 2026-09-10
---
# Review — closeout Phase 2 (items H, B, K7, K8, K9, 29, N)   (2026-09-10)

Range reviewed: `.agents/scratch/phase2.diff` (unified diff + `=== NEW FILE`
sections: `internal/httpapi/state_test.go`, `web/src/lib/projectBadge.ts`,
`web/src/lib/projectBadge.test.ts`, `web/scripts/normalize-shell.mjs`)

## Verdict: APPROVE WITH NITS

One real regression: `FileEditor.tsx`'s new per-call `AbortController` for
the syntax-check request is passed straight into `api.checkFile`'s `signal`
param, and `request()` in `api.ts` lets any caller-supplied `signal`
silently replace its own `AbortSignal.timeout(20000)` (no `AbortSignal.any`
combine) — so the syntax-check fetch now has no timeout at all, a real
regression from the pre-diff behavior, and is untested. Everything else
examined (state derivation in `server.go`, the CodeEditor gutter/remount
interaction, the EditorPane discard flow, `projectBadge`, the shell
normalizer, and the e2e drift guard) is correct and matches its stated
intent.

## Findings

- **[medium / confidence: high]** `web/src/lib/api.ts:360-366` + `web/src/components/FileEditor.tsx:186-213` — `request()` builds the fetch options as `{ signal: init?.signal ?? AbortSignal.timeout(20000), ...init, redirect: "manual" }`. Because `...init` is spread *after* the `signal` key, any caller that supplies its own `init.signal` (as `runCheck` now does via `checkAbort`) completely discards the 20s default timeout — there is no `AbortSignal.any` combining the two. Before this diff `checkFile` passed no signal, so every check request was timeout-protected; now it is not. Concrete failure: the guest exec behind `/api/projects/:id/check` hangs (stuck/booting VM, wedged shell) and the user does not type again — the fetch, and the pending promise, stay open indefinitely instead of failing at 20s and showing "syntax check unavailable". Fix: either race the caller's abort signal against a timeout inside `runCheck` (e.g. `setTimeout(() => ac.abort(), 20000)`), or have `request()` combine the two signals (`AbortSignal.any([init.signal, AbortSignal.timeout(20000)])` once the runtime target supports it) so a caller-supplied signal augments rather than replaces the default.
- **[low / confidence: medium]** `web/src/components/FileEditor.tsx:186-213` — the `catch` block does not special-case an aborted request (`e.name === "AbortError"`). Since `checkAbort.current?.abort()` cancels the *previous* in-flight check whenever a new debounce cycle starts while the old one is still pending, that previous call's promise rejects and lands in this same catch, setting `checkNote` to "syntax check unavailable" for the self-inflicted cancellation — a transient, incorrect status shown to the user until the newer call resolves. Not covered by any test (`grep` for `AbortError`/abort-races in `FileEditor.tsx`'s test suite finds nothing). Low severity since it self-corrects once the superseding call completes, but it is a concrete, reachable state (rapid edit → idle → edit → idle cycles with a slow backend).

## Answers to the seven numbered checks

1. **CodeEditor.tsx gutter/remount** — correct. The mount effect's deps are `[filename, readOnly]` (`CodeEditor.tsx:324`), and `FileEditorBody` also forces a remount via `key={st.docIdentity}` (`FileEditor.tsx:414`), so switching a `.go` file to a `.txt` file always remounts before the `...(check ? [...] : [])` spread runs, and `check` (computed in `FileEditor.tsx:423` from `CHECKABLE_EXTENSIONS`) is read fresh at that mount. `checkRef` (`CodeEditor.tsx:218-219,274`) is only needed to keep the *function reference* current across re-renders that don't remount, which is a separate concern and does not conflict.
2. **FileEditor.tsx `runCheck` AbortController** — see findings above: yes, an aborted-previous-request's `AbortError` is caught and surfaces as "syntax check unavailable" (transient, low), and separately the per-call signal drops the 20s safety timeout entirely (medium, the real bug).
3. **EditorPane.tsx Discard** — correct. `setLoaded(null)` changes `loaded`, so `useFileEditorState`'s second arg becomes `EMPTY`; `FileEditor.tsx:123-147`'s render-phase reset (keyed on `docIdentity`) synchronously sets `text`/`savedContent` to `""` in the same render, so `st.dirty` is `false` before the `setDirty(st.dirty)` effect (`EditorPane.tsx:53-55`) ever runs — no re-dirtying. The load effect (`EditorPane.tsx:85-123`) then re-fetches because `loaded` is now null and `same` is false, matching the "next open re-reads the file" comment.
4. **`__stateForTest` export** — acceptable test seam (doc-commented "Not for production callers", read-only accessor into module state that has no other export). Not a correctness issue; no finding.
5. **server.go `provisioning sync.Map`** — correct. `cfg.Image == ""` returns at line 846-848, strictly before `s.provisioning.Store` at line 849, so the test-only no-boot path never marks provisioning (confirmed by reading `createProject` in full). `deriveState` is used at both `listProjects` and `getProject` view sites with consistent precedence (deleting > provisioning > live), and `sync.Map`'s zero value is usable as a struct field with no explicit init. `state_test.go` covers the precedence table and the live end-to-end view (store/delete around a real handler call).
6. **normalize-shell.mjs latin1 round-trip** — correct. Reading and writing through `"latin1"` is a lossless 1-byte-per-char codec, so every byte (including the NUL `.gitattributes` documents) round-trips unchanged, and the replacement string is the same length (13 chars in, 13 zeros out) so no byte-length shift. Failure mode when the `u:<13 digits>` pattern is absent: `process.exit(1)` with a descriptive stderr message, which fails `npm run build` (script is `vite build && tsc -b && node scripts/normalize-shell.mjs`) — fails loud rather than silently shipping a mismatched embed, as the project's writing-files rules would want.
7. **run.mjs drift guard** — correct. The regex `/CHECKABLE_EXTENSIONS[^\n]*=\s*new Set\(\[([^\]]*)\]/s` matches the actual single-line declaration in `api.ts:898-900`; extracts all 14 extensions via `/"([a-z0-9]+)"/g`, asks the live `/check` endpoint for each plus one absent extension (`xyzzy`), catching drift in both directions. No false-positive/negative risk observed against the current source shape (would break loudly, not silently, if the declaration were reformatted across multiple `new Set([` lines with content before the first bracket on a different line — acceptable since `[^\]]*` still spans newlines via the character class regardless of `/s`).
8. **projectBadge precedence** — correct, verified equivalent to the deleted inline JSX. Old: `deleting` wins outright; `deleteError && state !== deleting` next; `provisionError && !deleteError && state !== deleting` last. New: early-return chain (`deleting` → `deleteError` → `provisionError` → `provisioning`) — the later `&&` guards in the old code are redundant once earlier branches return, so the refactor preserves exact precedence and adds the new `provisioning` badge, which the old code had no case for (new state). `projectBadge.test.ts` covers all four branches plus the full precedence chain.

## Criteria

1. Dead code — pass. No orphaned exports or unreachable branches found in the diff; `__stateForTest` is a deliberate, documented test seam, not dead code.
2. Flow tracing — fail on one path: `runCheck`'s abort signal breaks the timeout guarantee documented and relied on elsewhere in `api.ts` (finding above); everything else (state derivation, discard, badge) traces cleanly caller to callee.
3. Standards conformance — pass otherwise; the timeout-drop is the one place this diff's own stated intent ("aborts the previous request instead of letting it run to the 20s timeout") is not actually achieved for the *new* request.
4. Directory layout — pass. New files land where expected: Go test beside its package (`internal/httpapi/state_test.go`), TS lib + colocated test (`web/src/lib/projectBadge.ts`/`.test.ts`), build script under `web/scripts/`.
5. Simplicity (DIY-except-security) — pass. No new dependencies; `deriveState`/`projectBadge` are small pure functions extracted for testability, not speculative abstraction; no auth/crypto surface touched.
6. Decoupling — pass. `projectBadge.ts` and `deriveState` are clean, independently testable seams pulled out of `Sidebar.tsx`/`server.go`; no cross-package reach-ins observed.

## Not examined

- `web/src/lib/sshKey.ts`/`sshKey.test.ts` changes (the `\p{C}` widening and the key-type regex tightening) — included in the diff but outside the seven numbered checks; skimmed only, not traced against the server-side mirror it claims to match.
- `web/dist/client/**` compiled asset diffs — not reviewed as source (build output).
- Full read of `internal/httpapi/state_test.go`'s harness helpers (`newNetHarness`, `doAs`, `decodeLan`) — assumed pre-existing test infrastructure, not re-verified line by line.
- `web/e2e/run.mjs`'s surrounding test (the "network block shows the sandbox default" test after the new drift-guard check) — not in scope of items reviewed.

## Response (main, 2026-09-10)

- **Finding 1 (medium) — fixed.** `request()` now combines a caller's
  signal with its own 20 s bound via `AbortSignal.any([...])` instead of
  letting the spread replace it; the check request keeps its timeout.
- **Finding 2 (low) — fixed.** `runCheck`'s catch returns `[]` without
  touching the note when the abort is its own (a newer cycle superseded the
  controller); CodeMirror drops the stale result.
