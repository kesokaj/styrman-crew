---
owner: verifier
date: 2026-09-10
---
# Verification — stopped marker moved before waitReady (2026-09-10)

Range reviewed: uncommitted working tree change to `internal/orchestrator/orchestrator.go` only (`+10/-3`). No other files touched.

## Status: PASS

Scope: gofmt, go vet, go test -race (full suite), darwin/arm64 cross-build, and diff-integrity check all ran and passed; the Kata integration suite (`.agents/docs/testing/styrman/integration.sh`) was explicitly excluded per instruction (run separately by the main thread on this same tree) and is not part of this verification's PASS. Unit coverage of the specific changed branch (start succeeds, `waitReady` times out) is absent — see Coverage below.

## Commands
- `gofmt -l internal/ cmd/` → no output, exit 0 (no unformatted files)
- `go vet ./...` → no output, exit 0
- `go test ./... -count=1 -race` → all packages `ok`:
  cmd/styrman, internal/config, internal/conversation, internal/credbroker, internal/credstore, internal/files, internal/gitrepo, internal/history, internal/httpapi, internal/identity, internal/links, internal/orchestrator, internal/project, internal/terminal, internal/userstore (cmd/wsprobe, web: no test files). exit 0.
- `GOOS=darwin GOARCH=arm64 go build ./...` → no output, exit 0
- `git diff --numstat -- internal/orchestrator/orchestrator.go` → `10  3  internal/orchestrator/orchestrator.go` (no `-` column; content is real text, not binary)

## Root cause (n/a — PASS)
Not applicable; no failures observed in any command above.

## Coverage of changed code
- `internal/orchestrator/orchestrator.go`, `ensureRunningLocked`: the reordering (`o.stopped.Delete` moved from after `waitReady` to immediately after the start switch, before `waitReady`) is exercised only incidentally by the existing `internal/orchestrator` suite's happy-path tests (start succeeds, `waitReady` succeeds). There is **no unit test for the specific branch this change targets** — start succeeds, `waitReady` times out, and the stopped marker must already be cleared at that point. `internal/orchestrator` has no fake/mock Docker client (BUG-003 residual noted in project rules), so a test that forces `waitReady` to time out after a real start cannot currently be written without such a fake.
  - **Gap: FAIL-worthy if isolated** — the race/ordering fix itself (the reason for the diff) is unverified by any automated test; it is only verified by inspection/review of the diff and by the fact that no existing test regressed. Full behavioral confidence would require either (a) a fake Docker client enabling `waitReady`-timeout injection, or (b) an integration-level check (Kata suite, run separately, not part of this log).

## Environment
- OS: Linux 6.12.107+deb13-amd64
- Go: `go test`/`go vet`/`gofmt` run against the module's pinned toolchain (repo `go.mod`); no version override applied in this session.
- Docker/Colima: not applicable to this verification (no container build/run steps required by the listed commands).
- Kata integration suite: intentionally NOT run here; main thread is running it separately on this same working tree.
