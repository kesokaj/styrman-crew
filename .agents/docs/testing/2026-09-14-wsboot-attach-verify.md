---
owner: verifier
date: 2026-09-14
---
# Verification — wsBoot bridge-attach (uncommitted working tree)

Range reviewed: working tree vs HEAD (d938605) — uncommitted changes:
- `internal/httpapi/server.go` (modified, +30/-2)
- `internal/orchestrator/orchestrator.go` (modified, comment-only, +3/-1)
- `internal/httpapi/wsboot_attach_test.go` (new file)

## Status: PASS

Scope: complete — all seven steps in the dispatch ran against the live working tree.

## Commands

1. `gofmt -l internal/` → printed nothing. `gofmt_rc=0`.
2. `go vet ./...` → no output. `vet_rc=0`.
3. `go test ./... -count=1` → `rc=0`. All packages `ok`:
   `cmd/styrman`, `internal/config`, `internal/conversation`, `internal/credbroker`,
   `internal/credstore`, `internal/files`, `internal/gitrepo`, `internal/history`,
   `internal/httpapi`, `internal/identity`, `internal/links`,
   `internal/orchestrator`, `internal/project`, `internal/terminal`,
   `internal/userstore` (`cmd/wsprobe`, `web` — no test files, as expected).
4. `go test ./internal/httpapi/ -run TestWSBoot -count=1 -v` → `rc=0`, both named:
   ```
   === RUN   TestWSBootAttachesTheServerToTheProjectBridge
   --- PASS: TestWSBootAttachesTheServerToTheProjectBridge (0.00s)
   === RUN   TestWSBootDoesNotAttachWhenTheBootFails
   --- PASS: TestWSBootDoesNotAttachWhenTheBootFails (0.00s)
   PASS
   ok  	github.com/kesokaj/styrman-crew/internal/httpapi	0.009s
   ```
5. `(cd deploy/caddy/ingress && go test ./...)` → `rc=0`:
   `ok  	github.com/spgo/devbox/caddy/ingress	(cached)` (separate module, unchanged by this diff, run per Definition of Done).

6. Test-count / binary checks (`.agents/rules/writing-files.md`):
   - `git diff --stat -- internal/httpapi/server.go internal/orchestrator/orchestrator.go` →
     `2 files changed, 32 insertions(+), 2 deletions(-)` — no bulk deletion.
   - `git diff -U0 -- internal/httpapi/server.go internal/orchestrator/orchestrator.go | grep -c '^-func Test'` → `0` (no deleted test functions).
   - `internal/httpapi/wsboot_attach_test.go` is untracked (new file), so `git diff --numstat`
     against it is empty (no baseline to diff); checked instead with `file`:
     `internal/httpapi/wsboot_attach_test.go: ASCII text` — confirms it is TEXT, not a raw-byte
     binary write (BUG-007 pattern). `git status --porcelain` shows only the expected `??` line.

7. Coverage claim:
   - `wsBoot` is defined at `internal/httpapi/server.go:300` and is called directly by both
     new tests in `wsboot_attach_test.go` (lines 41 and 55), against a `fakeWSBooter` that
     records `EnsureRunning` / `EnsureSelfAttached` calls — confirmed PASS in step 4.
   - `ensureFor` (`internal/httpapi/server.go:261`) calls `wsBoot` at line 277
     (`id, err := wsBoot(ctx, s.orch, user, p.ID, ...)`) — confirmed by grep.
   - **`ensureFor` itself (the project-record lookup wrapping the `wsBoot` call) is NOT
     covered by a new test** — the new tests call `wsBoot` directly with a fake, bypassing
     `ensureFor`'s record lookup entirely. This is a real coverage gap in the new work, not
     just a pre-existing one.
   - **`EnsureSelfAttached`** (`internal/orchestrator/orchestrator.go:619`) has no unit test
     of its own anywhere in `internal/orchestrator/*_test.go` — confirmed by grep returning
     no matches. The orchestrator.go diff in this change is comment-only (documents why
     `ensureRunningLocked` does NOT call `EnsureSelfAttached`, and points at `httpapi.wsBoot`
     instead). This gap pre-dates this change and is not introduced by it.

## Root cause (FAIL only)

N/A — verdict is PASS.

## Coverage of changed code

- `internal/httpapi/server.go` (`wsBoot` function) — covered directly by
  `internal/httpapi/wsboot_attach_test.go` (both new tests, PASS).
- `internal/httpapi/server.go` (`ensureFor` function, the caller of `wsBoot`) — NOT covered
  by a new test; the new tests exercise `wsBoot` in isolation via `fakeWSBooter`, not through
  `ensureFor`'s record-lookup path. Flagged, not fixed.
- `internal/orchestrator/orchestrator.go` — comment-only change, no behavior to cover.
  `EnsureSelfAttached` remains without any unit test in the orchestrator package
  (pre-existing gap, unrelated to this diff).

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Go: `go version` not separately captured this run; build/test/vet all succeeded against
  the repo's pinned toolchain via `go test`/`go vet` as invoked above.
- Kata/mvm runtime: not exercised — no integration or e2e suite was run per dispatch scope
  (Go unit tests + vet + gofmt + ingress module tests only).

## Not examined

- `bash .agents/docs/testing/styrman/integration.sh` (real Kata integration) — out of
  dispatch scope, not run.
- `web/e2e/run.sh` (browser e2e) — out of scope; no web/ change in this diff.
- `web/` frontend checks (`npm run check`/`lint`/`test`) — out of scope; no web/ change.
- `bash deploy/vulncheck.sh` — not requested, not run.
- `.agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md` (new untracked file in
  git status) — not part of the named changed paths for this verification; not reviewed.

## Round 2

Tree changed since Round 1: new `internal/terminal/ensure_before_upgrade_test.go`
(one test, added for the reviewer's finding), plus docs-only edits (AGENTS.md,
`.agents/docs/specs/styrman/{deploy,guide}.md`,
`.agents/docs/testing/styrman/README.md`, `.agents/docs/bugs/BUG-060-*.md` +
`INDEX.md`, `.agents/rules/mechanism-reaches-every-caller.md`, two new review
files). Go source unchanged since Round 1, confirmed by `git hash-object`:
- `internal/httpapi/server.go` → `51bc7e0c62a957b7d3758103054a0f06c5fb7709` (matches Round 1)
- `internal/orchestrator/orchestrator.go` → `f5cd95db4056e0317fe6c1003a4cf94010ab8372` (matches Round 1)
- `internal/httpapi/wsboot_attach_test.go` → `222f36c7f070d0107c621420d8a1d65295699ab9` (unchanged, new-file blob)

### Status: PASS (unchanged)

### Commands

- `gofmt -l internal/` → no output. `gofmt_rc=0`.
- `go vet ./...` → no output. `vet_rc=0`.
- `go test ./... -count=1` → `rc=0`. All packages `ok` (same list as Round 1,
  `internal/terminal` now `ok  0.079s` with the new test included;
  `cmd/wsprobe`/`web` — no test files, as before).
- `go test ./internal/terminal/ -run TestBootCallbackRunsBeforeTheUpgrade -count=1 -v` → `rc=0`:
  ```
  === RUN   TestBootCallbackRunsBeforeTheUpgrade
  --- PASS: TestBootCallbackRunsBeforeTheUpgrade (0.00s)
  PASS
  ok  	github.com/kesokaj/styrman-crew/internal/terminal	0.003s
  ```
- `go test ./internal/httpapi/ -run TestWSBoot -count=1 -v` → `rc=0`, both still PASS by name:
  `TestWSBootAttachesTheServerToTheProjectBridge`, `TestWSBootDoesNotAttachWhenTheBootFails`.
- `(cd deploy/caddy/ingress && go test ./...)` → `rc=0`, `ok (cached)` — unchanged module.
- `file internal/terminal/ensure_before_upgrade_test.go` → `Unicode text, UTF-8 text` — TEXT,
  not binary (BUG-007 check).
- `git status --porcelain` — every path accounted for, no unexpected ` D`:
  ```
   M .agents/docs/bugs/INDEX.md
   M .agents/docs/specs/styrman/deploy.md
   M .agents/docs/specs/styrman/guide.md
   M .agents/docs/testing/styrman/README.md
   M .agents/rules/mechanism-reaches-every-caller.md
   M AGENTS.md
   M internal/httpapi/server.go
   M internal/orchestrator/orchestrator.go
  ?? .agents/docs/bugs/BUG-060-terminal-ws-boot-never-joined-the-project-bridge.md
  ?? .agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md
  ?? .agents/docs/reviews/2026-09-14-review-wsboot-attach.md
  ?? .agents/docs/reviews/2026-09-14-security-wsboot-attach.md
  ?? .agents/docs/testing/2026-09-14-wsboot-attach-verify.md
  ?? internal/httpapi/wsboot_attach_test.go
  ?? internal/terminal/ensure_before_upgrade_test.go
  ```
  The two ` M` lines on `server.go`/`orchestrator.go` are the same Round 1 diffs (hashes
  confirmed unchanged above) — not a new edit.

### Coverage note (Round 2)

The new `TestBootCallbackRunsBeforeTheUpgrade` closes the reviewer's finding by testing
the terminal package's boot-before-upgrade ordering directly (not through `httpapi.ensureFor`).
The Round 1 gap stands unchanged: `ensureFor` (the record-lookup wrapper around `wsBoot` in
`internal/httpapi/server.go`) still has no direct test, and `EnsureSelfAttached` in
`internal/orchestrator/orchestrator.go` still has no unit test in the orchestrator package.
Neither is addressed by this round's new file.

### Not examined (Round 2)

Same exclusions as Round 1 (integration.sh, web/e2e, npm checks, vulncheck.sh) — docs-only
edits and the new terminal test did not change that scope. Docs files were not proofread for
content correctness; only checked for presence in `git status` as declared.
