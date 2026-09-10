---
owner: reviewer
date: 2026-09-10
---
# Review — closeout phase1 (2026-09-10)

Range reviewed: uncommitted working-tree diff saved at `.agents/scratch/phase1.diff`
(server side of `.agents/docs/plans/2026-09-10-todo-closeout.md`): `network.go`
reply-after-lock, `setup.go` pubkey sanitisation, `git.go` gitconfig parser,
`orchestrator.go` async `unmarkIfStillRunning`, and the legacy-workspace-migration
deletion (`main.go`, `config.go`, `orchestrator.go`, tests). Web-side changes
present in the working tree (`web/package.json`, `web/src/lib/sshKey.ts`,
`web/src/lib/sshKey.test.ts`, `web/scripts/`) are **not** in `phase1.diff` and
were not reviewed here.

## Verdict: APPROVE WITH NITS

Nothing broken in the network-handler reply restructuring, the `stopped`
marker's move to a comparable `time.Time`/`CompareAndDelete` scheme, or the
legacy-migration deletion (confirmed dead — no remaining references, clean
build, all touched-package tests green). The one real defect is in
`gitUserFromConfig`/`gitConfigValue`: two divergences from `git config`'s own
documented semantics, both verified against a real `git config --file`,
neither covered by `gitconfig_test.go`.

## Findings

- **[low / confidence: high]** `internal/httpapi/git.go:316-319` (section-header
  detection in `gitUserFromConfig`) — a `[user]` header followed by a trailing
  comment on the same line (`[user] ; bob`, `[user] # comment`) is valid
  gitconfig syntax that real `git config --file` parses correctly (verified:
  `git config --file test.gitconfig user.name` returns `Bob` for
  `[user] ; this is bob\nname = Bob\n`). The Go parser's
  `strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")` requires the line
  to end in `]`, so with a trailing comment it never matches and `inUser`
  stays false — the user's name/email is silently dropped for a `.gitconfig`
  edited by hand with a comment on the section line. Fix: strip a trailing
  `#`/`;` comment (outside quotes — none apply to a bare header) before
  matching, mirroring the same comment-stripping already done for value lines.

- **[low / confidence: high]** `internal/httpapi/git.go:350-364`
  (`gitConfigValue`'s escape handling in a quoted string) — real
  `git config --file` recognizes `\n`, `\t`, `\b` as their control-character
  equivalents inside a double-quoted value (verified: a quoted value
  `"line1\nline2"` round-trips through real git as `line1<LF>line2`, two
  bytes not three). This parser's `esc` branch writes the character
  immediately following any backslash literally, so the same input decodes to
  `line1nline2` — a real functional divergence from git's own reading of the
  same file, silently mis-parsing (rather than erroring on) a shape git
  itself accepts. Low impact for `user.name`/`user.email` specifically (an
  embedded control char in those fields is rare), but it is exactly the class
  of divergence the function's own doc comment warns about ("a stricter
  reading than git's would silently drop a user's identity"), and the two
  cases together weren't hit by the test matrix in `gitconfig_test.go` for
  this task's own listed scenarios (comment-on-header-line, `\n` escape).

Both are non-blocking (low severity: no security impact, degrade gracefully
to "identity not detected" or a garbled character rather than crashing or
misattributing a commit to someone else), but are concrete, verified
divergences from git's own grammar in code whose whole justification is
"the two [places] cannot disagree" with `git config`.

## Answers to the six specific checks

1. **Network handler (`internal/httpapi/network.go`)** — every path inside
   the `WithProject` closure assigns to `res` (`fail(...)`/`ok(...)`); no path
   calls `writeErr`/`writeJSON` on `w` from inside the closure. `res` is
   initialized fail-closed before the call, so a runtime whose `WithProject`
   never invokes the closure (exercised by `neverMode`/`TestNetworkNoDecisionIsA500`)
   answers 500 "NOT changed" rather than a zero-value 200. Reply is written
   only after `WithProject` returns (`TestNetworkReplyIsWrittenAfterTheLockIsReleased`
   confirms via `lockedWriter`). Clean.

2. **`gitUserFromConfig` vs git semantics** — see Findings above: fails on a
   commented section header and on `\n`/`\t`/`\b` escapes inside a quoted
   value. CRLF line endings work correctly (`strings.TrimSpace` strips the
   trailing `\r`, verified against real git). The existing `gitconfig_test.go`
   cases (quoted `\"`/`\\`, trailing `#`/`;` on a value line, last-assignment-wins,
   subsection exclusion, case-insensitivity) all match real git and pass.

3. **`unmarkIfStillRunning`'s `CompareAndDelete`** — cannot delete a newer
   stop's marker: `stopMarked`/`stopIfIdle` are only called under the
   per-project lock (`Stop` and the reaper's `stopIfIdle` both take
   `lockProject`), so a second stop always creates a fresh `marker` and
   `Store`s it (or `Delete`s it via a start, orchestrator.go:442) before or
   after the first stop's goroutine runs; either way the map's current value
   differs from the first goroutine's captured `marker`, so its
   `CompareAndDelete` is a safe no-op. `grep` confirms every `o.stopped.Store`
   call site now stores a `time.Time` (orchestrator.go:1114, 1257) — no
   remaining call stores `true` (the old `bool` marker), so the value type
   change is consistent across the package.

4. **`time.Now()` as a comparable `CompareAndDelete` value** — safe here: the
   same `marker` variable (with its monotonic reading intact) is passed by
   value from `Store` through to the later `CompareAndDelete` without any
   operation (`Round`/`Truncate`/serialization) that would strip the
   monotonic component, so `==` inside `sync.Map` compares bit-identical
   values. No bug.

5. **`publicKeyLine`'s tab normalisation / `strings.Fields` on the comment** —
   `strings.Fields` splits on `unicode.IsSpace` and rejoins with single ASCII
   spaces; it only collapses whitespace runs (including the tab→space
   normalisation and Unicode space variants like NBSP), it does not drop any
   non-whitespace comment content. Verified against `TestPublicKeyLineSanitisesTheComment`'s
   cases (control byte, bidi/zero-width marks via `stripControls`, tab and
   NBSP collapsing) — matches intent, no content loss.

6. **Deletion in `ensureRunningLocked`/`ensureRepos`** — the removed
   `LegacyWorkspaceExists` guard is the only behavior lost, and it is a
   deliberate, documented trade: without automatic migration, a project whose
   pre-ADR-027 workspace was never moved now gets a fresh empty workspace
   dir created over it (silently shadowing the real tree) instead of
   refusing to boot. This is called out explicitly in the diff's own
   `.agents/docs/deploy/remote.md` addition as a pre-upgrade manual step, and
   the plan states only one known deployment is affected (already migrated
   at v1.19.x). Not a hidden regression, but flagging it as a legitimate
   residual risk for any deployment that did not follow the documented
   upgrade order.

## Criteria

1. **Dead code** — pass. `migrateWorkspaces`, `LegacyWorkspaceExists`, and
   their tests are fully removed with no remaining references (`grep` clean,
   `go build ./...` clean).
2. **Flow tracing** — pass except the two gitconfig parser gaps above;
   `gitIdentity`'s two callers (`credharvest.go:52`, `git.go:394`) are
   unaffected by the signature (unchanged `(name, email string)`); the
   `unmarkIfStillRunning` signature change (`ctx` dropped, `marker` added) is
   consistent at both call sites (`stopMarked`, `stopIfIdle`).
3. **Standards conformance** — pass; errors wrapped with `%w` throughout;
   goroutine in `unmarkIfStillRunning` has a bounded, explicit exit (two
   samples, ~12s max, no leak).
4. **Directory layout** — pass; new test `internal/httpapi/gitconfig_test.go`
   sits beside `git.go`; no stray files.
5. **Simplicity** — pass; hand-rolled gitconfig parser is a reasonable
   stdlib-only substitute for `exec.Command("git", "config", ...)` (removes a
   process spawn per call) but is exactly why criterion 2's divergences
   matter — it must track git's real grammar or explicitly document what it
   does not cover (`[include]` is already called out; the two gaps above are
   not).
6. **Decoupling** — pass; `gitUserFromConfig`/`gitConfigValue` are private,
   used only within the package; the `reply` type in `network.go` is a small
   local closure-scoped type, not exported.

## Not examined

- Web-side changes in the working tree (`web/package.json`,
  `web/src/lib/sshKey.ts`, `web/src/lib/sshKey.test.ts`, `web/scripts/`) —
  outside `phase1.diff`'s scope as handed to this review.
- `.agents/docs/plans/2026-09-10-todo-closeout.md` and other plan/handoff
  prose beyond the one `remote.md` hunk in the diff.
- Did not run the full integration suite (`.agents/docs/testing/styrman/*.sh`)
  or browser e2e — only `go build ./...`, `go vet ./...`, and
  `go test ./internal/httpapi/... ./internal/orchestrator/... ./cmd/styrman/... ./internal/config/...`
  (all green).

## Response (main, 2026-09-10)

Both nits fixed with test cases: a `[user] ; comment` header now matches
(the header ends at the first `]`), and `gitConfigValue` decodes git's
`\n` `\t` `\b` escapes (any other escaped character stands for itself) —
after which `gitUserFromConfig` strips control characters from the result,
per the security review's finding 2.
