# Phase 1 closeout: server fixes (lock timing, keys, git identity)     (date: 2026-09-10)

## Summary

Phase 1 of plan `2026-09-10-todo-closeout.md` closed: network-mode PUT reply timing fixed to release the project lock before writing to the ResponseWriter; public key symlink/hardlink detection hardened; `publicKeyLine` now validates full OpenSSH key-type tokens and strips formatting from comments; `unmarkIfStillRunning` marker uses `CompareAndDelete` for concurrent stops; pre-ADR-027 workspace migration code removed; `.gitconfig` parsing moved to confined read path with full escape/comment handling. `go test -race ./...` passed over both modules, darwin cross-compile verified.

## Changes

**C. Network-mode PUT reply lock timing**

`internal/httpapi/network.go` — The network-mode PUT handler's reply was held inside `WithProject`'s lock, written after release. Tests added: `TestNetworkReplyIsWrittenAfterTheLockIsReleased` (ResponseWriter records lock state at WriteHeader; mutation-checked by writing inside the closure) and `TestNetworkNoDecisionIsA500` (fail-closed to 500 when no decision is made).

**D. Public key symlink/hardlink detection**

`cmd/styrman/setup_pubkey_test.go` — New `TestToolDetailsEchoesOnlyAPublicKey` runs through the real handler under a `files.Trust` base: a `.pub` that is a symlink, hard link, or copy of the private key yields no details and not-ready status; only a real key is echoed.

**5+7. publicKeyLine validation and comment sanitization**

`internal/cmd/setup.go` — `publicKeyLine` now requires a full OpenSSH key-type token (`ssh-ed25519|rsa|dss`, `ecdsa-sha2-nistp256/384/521`, `sk-ssh-ed25519@openssh.com`, `sk-ecdsa-sha2-nistp256@openssh.com`), accepts a TAB separator, and returns the line with comments stripped of control/formatting characters and whitespace collapsed (`stripControls` + `strings.Fields`). Tests extended in `internal/cmd/setup_test.go`: `TestPublicKeyLineSanitisesTheComment` and existing key-type coverage kept.

**E. Concurrent stop marker handling**

`internal/orchestrator/orchestrator.go` — `unmarkIfStillRunning` marker's value is now the stop's start `time.Time`; clear uses `CompareAndDelete` so a newer stop keeps its own marker. `Warn` logs added for marker retained (inspect error / container down) and marker cleared.

**F. Pre-ADR-027 workspace migration removed**

`cmd/styrman/main.go` — Deleted `migrateWorkspaces`, `config.LegacyWorkspaceExists`, guard in `ensureRepos`, and "workspace migration incomplete" branch in `ensureRunningLocked`. Removed benign "workspace migration: old path rejected" WARN at server start. Upgrade note added to `deploy/remote.md` by `documentation`: hosts holding `homes/<user>/base/projects/<dir>` must move manually before upgrade.

`cmd/styrman/migrate_test.go` — Deleted `TestMigrateWorkspaces` and `TestEnsureReposRefusesUnmigrated`; 6 tests → 4 (remaining four untouched).

**G. Confined .gitconfig parsing**

`internal/gitrepo/git.go` — `gitIdentity` now reads `.gitconfig` through `files.ReadRaw` (confined root) and parses with `gitUserFromConfig` (case-insensitive `[user]`/keys, `=` spacing, quoted values with escapes, `#`/`;` comments, last wins, subsections unsupported). Removed `git config --file` exec.

`internal/gitrepo/gitconfig_test.go` — New, 9 test cases covering edge cases.

## Files changed

- `internal/httpapi/network.go` — lock release timing; fail-closed 500 reply
- `cmd/styrman/setup_pubkey_test.go` — symlink/hardlink/copy detection
- `internal/cmd/setup.go` — publicKeyLine key-type validation, comment sanitization
- `internal/cmd/setup_test.go` — extended coverage
- `internal/orchestrator/orchestrator.go` — CompareAndDelete marker, concurrent stop handling
- `cmd/styrman/main.go` — workspace migration code deleted; 1 WARN removed
- `cmd/styrman/migrate_test.go` — 2 tests deleted (6 → 4)
- `internal/gitrepo/git.go` — confined .gitconfig read and parse
- `internal/gitrepo/gitconfig_test.go` — new, 9 cases
- `deploy/remote.md` — upgrade note (pre-ADR-027 homes must migrate before upgrade)

## Agents dispatched

verifier (integration run logged), reviewer (diff review), security (git parsing, key handling), documentation (upgrade path in deploy guide), changelog

## Verification

- `go test -race ./...` green over both modules (styrman + caddy/ingress)
- darwin amd64 cross-compile verified
- verifier log: `.agents/docs/testing/2026-09-10-verify-closeout-phase1.md` (full output + pass counts)
- Code reviews logged in `.agents/docs/reviews/2026-09-10-review-closeout-phase1.md` and `.agents/docs/reviews/2026-09-10-security-closeout-phase1.md` (findings recorded in commit message)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-10-todo-closeout.md`
- ADR-027 (workspace architecture; legacy migration deleted)
- ADR-064 (per-project homes; guard paths remain; new test under `files.Trust`)
