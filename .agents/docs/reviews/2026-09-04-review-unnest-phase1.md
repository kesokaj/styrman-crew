---
owner: reviewer
date: 2026-09-04
---
# Review — un-nest project homes, Phase 1 implementation   (2026-09-04)

Range reviewed: uncommitted diff (`git diff`) touching `internal/config/config.go`,
`internal/config/config_test.go`, `cmd/styrman/main.go`, `internal/orchestrator/orchestrator.go`,
`internal/orchestrator/realbelow_test.go`, `internal/httpapi/server.go`,
`.agents/rules/guest-writable-paths.md`, plus the new untracked `cmd/styrman/migrate_test.go`.

## Verdict: APPROVE WITH NITS

This is a clean, well-tested implementation of the Phase 1 plan
(`.agents/docs/plans/2026-09-04-unnest-project-homes.md`, which itself went through three
rounds of cold-read and shipped "implementation ready"). Every caller audit from that plan
review checks out against the actual code: all four `UserHome`-meaning-tenant-dir sites in
`EnsureRunning` (226/272/275/282) moved to `TenantDir`, both `EnsureSetup` sites correctly kept
`UserHome` (the setup VM's home really is the base home), `createProject` in `server.go` moved
correctly, and every "no-change" `UserHome` caller (`setup.go`, `credentials.go`, `git.go`,
`explorer.go`, `config.go:372`, `main.go:228/315`) is still correct under the new meaning. The
`os.Rename`/`os.Mkdir` calls in `migrateBaseHomes` are safe against the guest races the dispatch
asked about — `rename(2)` never dereferences a symlink source, and `os.Mkdir` on an existing path
(symlink or not) fails closed rather than following it — and the test suite (`migrate_test.go`,
`realbelow_test.go`, `config_test.go`) exercises the two/three interesting adversarial shapes
(planted `base` symlink, symlinked tenant entry, already-migrated tenant, trailing-slash bind
source). `go build ./...`, `go vet ./...`, and the touched packages' tests all pass. Nothing here
violates R1/R3/R12. The two things below are worth a follow-up but do not block this change.

## Findings

- **[low / confidence: high]** `.agents/rules/guest-writable-paths.md:38-40` — the closing
  paragraph still reads "The residual is the race between the check and the operation; the
  durable fix is to stop nesting tenant trees under guest-writable directories (`TODO.md`, the
  un-nesting ADR proposal)," describing un-nesting as a still-pending future change. The
  rewritten opening paragraph of the same file now states the opposite — that ADR-064 already
  moved the tenant dir out of the guest-writable path. Left as-is, the file contradicts itself:
  a future reader (this rule is explicitly meant to be "consult[ed] before touching an area it
  names") could reasonably conclude un-nesting is still an open proposal. Fix: update or drop
  the closing paragraph in the same edit (it is in scope — the file is part of this diff).

- **[low / confidence: medium]** `cmd/styrman/main.go` `migrateBaseHomes` — `os.Rename(tenant/name, base/name)` is not tested against a name collision at the destination. The concrete path: server A migrates a tenant (e.g. `.claude/` moves into `base/.claude/`) while an old-layout setup VM is still bound to the tenant dir (an acknowledged scenario per the plan's Failure modes); if the still-running guest recreates a directory with the same name at the tenant top level before the *next* server restart re-runs the migration, that second run's `os.Rename` onto an existing non-empty `base/.claude` fails with `ENOTEMPTY` and is only reported as a generic "move failed" log line, with the guest's newer copy stranded permanently at the tenant top level (never migrated, never visible to `files.Trust`-confined callers since `openRoot` only reaches `base/`+`p/`). No data is lost (the plan's own accepted trade-off — "recoverable by hand"), but this specific collision shape has no test coverage and the log line doesn't name it distinctly from any other rename failure. Not blocking; worth a one-line test/log improvement if this path sees real multi-restart-during-migration traffic.

## Criteria

1. Dead code — pass. No orphaned functions or leftover `UserHome`-as-tenant-dir call sites; the old boundary comments were rewritten in place rather than left dangling.
2. Flow tracing — pass. Every changed function's callers were traced (`EnsureRunning`, `EnsureSetup`, `createProject`, `migrateLegacy`→`migrateBaseHomes`→`migrateWorkspaces` ordering); errors are wrapped and logged on every path; `setupHomeStale` is total (nil mounts → stale, safe default).
3. Standards conformance — pass. Matches the project's existing symlink-defense idiom (`os.Root` confinement, `realBelow`/`mkdirAllConfined`) and Go error-wrapping conventions; `gofmt -l` clean.
4. Directory layout — pass. New test lands beside the code it covers (`cmd/styrman/migrate_test.go`); no new files elsewhere.
5. Simplicity (DIY-except-security) — pass. Pure stdlib (`os.Rename`/`os.ReadDir`/`os.Lstat`); no new dependency; no auth/crypto surface touched.
6. Decoupling — pass. `TenantDir`/`UserHome`/`ProjectHome` stay three small, independently testable accessors; `setupHomeStale` is an isolated pure helper with its own unit test.
