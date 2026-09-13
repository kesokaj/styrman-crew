---
owner: verifier
date: 2026-09-13
---
# Verification — whole-project /audit (2026-09-13)

Range reviewed: HEAD e56e775 (clean tree; untracked `.agents/docs/{plans,reviews}/2026-09-13-audit-*` files present from a concurrent audit dispatch, not part of this verification's subject and not touched here).

## Status: FAIL

Scope: all named build/vet/format/test/lint/shellcheck/vulncheck commands ran bare with exit status read before any other action; no commit was made. Coverage mapping is complete for Go `internal/`+`cmd/` and the two named script tiers (`deploy/styrman-ctl` via Go test, firewall via `deploy/firewall/test.sh`); `web/src/` coverage mapping is approximate (filename-based, not per-symbol/per-check) due to budget — see "Not examined". Two Go files (`internal/httpapi/projectenv.go`, `internal/httpapi/stats.go`) are confirmed 0% covered by `go tool cover`, both logic-bearing — this alone is FAIL.

## Commands

- `go build ./...` → exit 0. PASS
- `go vet ./...` → exit 0. PASS
- `gofmt -l ./cmd ./internal` → printed nothing, exit 0. PASS
- `go test -race -count=1 ./...` → exit 0, all 15 testable packages `ok` (cmd/styrman 1.0s, internal/config 5.5s, internal/conversation 1.0s, internal/credbroker 1.0s, internal/credstore 1.0s, internal/files 1.0s, internal/gitrepo 1.0s, internal/history 1.0s, internal/httpapi 2.0s, internal/identity 1.0s, internal/links 1.0s, internal/orchestrator 21.7s, internal/project 1.0s, internal/terminal 1.1s, internal/userstore 1.0s); `cmd/wsprobe` and `web` report `[no test files]`. PASS
- `(cd deploy/caddy/ingress && go test -count=1 ./...)` → exit 0, `ok github.com/spgo/devbox/caddy/ingress 2.0s`. PASS
- `(cd web && npm run check)` → `tsc -b`, exit 0, no output. PASS
- `(cd web && npm run lint)` → `oxlint`, exit 0, no output. PASS
- `(cd web && npm test)` → `vitest run`, exit 0: 13 test files passed (13), 171 tests passed (171). PASS
- `bash -n` on all 19 shell scripts (`deploy/host-setup.sh`, `deploy/verify-commit.sh`, `deploy/vulncheck.sh`, `deploy/styrman-ctl`, `deploy/update/styrman-update`, `deploy/firewall/{install,styrman-firewall,test}.sh`, `image/entrypoint.sh`, `image/configs/{harness-migrate,profile-motd,styrman-browser,styrman-credentials,styrman-git-credential,styrman-setup}` — the last two of these six are bash-syntax sourced snippets with no shebang, included per the `image/configs/* that are bash` instruction — `.agents/docs/testing/styrman/{adr-067-sweep,ensure-test-image,integration}.sh`, `web/e2e/run.sh`) → all 19 OK, no output, exit 0 each. PASS
- `shellcheck -S warning` on the same 19 files → 16 files 0 warnings; 3 findings:
  - `image/configs/profile-motd.sh` — SC2148 (error-severity, surfaced by `-S warning`): no shebang / `shell=` directive, so shellcheck can't infer the target shell. File is sourced (no shebang by design, like `styrman-credentials.sh`) but unlike that file it lacks the `# shellcheck shell=sh` directive — a real gap, cosmetic (shellcheck can't check the file's *other* rules without it), no behavior bug found.
  - `image/configs/styrman-credentials.sh:60` — SC2088: `"~/"*) __p="${HOME}/${__p#\~/}" ;;` — tilde doesn't expand inside the quoted case pattern; on inspection this is a case-pattern match (`~/` is a literal string being matched, not expanded), so likely benign, but not exercised by any test — flagging per instructions, not fixing.
  - `web/e2e/run.sh:120` — SC2034: `for i in $(seq 1 30); do` — loop variable unused (it's a retry-count loop, `i` is intentionally unused). Cosmetic.
  Net: shellcheck ran clean of hard failures; 3 style/informational warnings, none altering PASS status of the script tier itself.
- `node --check web/e2e/run.mjs web/scripts/normalize-shell.mjs` → both exit 0, no output. PASS
- `bash deploy/vulncheck.sh` → ran (Docker available, pinned `golang:1.25.13-trixie` builder image pulled/used for both modules). Exit 3 (govulncheck's own "called vulnerability found" status, per the script's documented contract — not a scan failure). Findings: `deploy/caddy/ingress` module is affected by 2 CALLED vulnerabilities in `github.com/docker/docker@v28.5.2+incompatible` — GO-2026-4887 and GO-2026-4883 — both with `Fixed in: N/A`; these are the exact IDs the script's own header comment names as accepted-per-release with no fix available. `image/harness` module: "No vulnerabilities found." Treated as PASS (informational security report row, matches documented accepted findings, not a new regression) — flagged here rather than silently passed, per the instruction to always name what a green/expected-red result actually measured.

## Root cause (FAIL)

`internal/httpapi/projectenv.go` (4 handlers: `listProjectEnv`, `putProjectEnv`, `projectEnvValue`, `deleteProjectEnv` — all branching, request-parsing, error-path logic, not glue) and `internal/httpapi/stats.go` (`projectStats`, `parseStats`, `projectLogs` — `parseStats` in particular parses/branches on external command output) have **zero** Go test coverage. Verified two ways: (a) no `_test.go` file in `internal/httpapi/` references any of these 7 symbols by name; (b) `go test ./internal/httpapi/... -coverprofile=...` + `go tool cover -func` shows literal `0.0%` for every one of the 7 functions. No route to either handler exists in any `_test.go` (`grep` for the registered paths `/api/projects/{project}/env*`, `/stats`, `/logs` in test files: no hits) and no reference in `.agents/docs/testing/styrman/integration.sh`'s test surface was confirmed (not exhaustively re-grepped this pass — see Not examined). These are both routed handlers reachable from the running server (`server.go` registers all 7), so this is a real, currently-shipping coverage gap, not dead code.

By contrast `internal/httpapi/network.go` (`projectNetwork` 100.0%, `setProjectNetwork` 94.8% per the same coverage run) looked uncovered under a naive "test file references the symbol by name" grep (its test, `network_test.go`, drives it only through HTTP dispatch, never naming the handler) but IS genuinely covered — confirmed via `go tool cover`, not the grep heuristic. This is noted because the grep-only method used for the first pass of this map is unreliable for httpapi handlers in general; only the two files above were confirmed uncovered by the stronger (coverage-profile) method.

## Coverage of changed code (mapped: all non-test files under cmd/, internal/, deploy/caddy/ingress/, and the named script trees)

### Go — cmd/, internal/ (39 non-test files; verified via `go test -race -count=1 ./...` PASS plus symbol-grep, with `go tool cover -func` used to arbitrate every ambiguous case)

- COVERED (test in same package references/exercises it, or `go tool cover` shows >0%): cmd/styrman/main.go, internal/config/config.go, internal/conversation/{conversation,names,store}.go, internal/credbroker/broker.go, internal/credstore/credstore.go, internal/files/files.go, internal/gitrepo/gitrepo.go, internal/history/history.go, internal/httpapi/{auth,browser,check,credentials,credharvest,explorer,format,git,links_api,network,profile,provision,proxy,server,setup,shares}.go, internal/identity/identity.go, internal/links/store.go, internal/orchestrator/{browserpeer,docker,exec,orchestrator,ports}.go, internal/project/{store,uuid}.go, internal/terminal/{autostart,bridge}.go, internal/userstore/userstore.go
- COVERED via tier, not a Go test: cmd/wsprobe/main.go — no `_test.go`, but built and run by `.agents/docs/testing/styrman/integration.sh` (confirmed: script builds `${STATE}/wsprobe` and invokes it 7 times against live terminal WS endpoints). Cited in `.agents/docs/testing/2026-09-13-integration-v1.29.0.log.md` (passed=91 failed=0 skipped=3), not re-run this pass per instructions.
- **FAIL — internal/httpapi/projectenv.go** — 0.0% coverage on all 4 functions (`listProjectEnv`, `putProjectEnv`, `projectEnvValue`, `deleteProjectEnv`). Logic-bearing (request parsing, per-key validation, error branching), not glue.
- **FAIL — internal/httpapi/stats.go** — 0.0% coverage on all 3 functions (`projectStats`, `parseStats`, `projectLogs`). Logic-bearing (`parseStats` parses/branches on command output; the other two do request routing + error handling).

### deploy/caddy/ingress — separate module, `go test ./...` PASS covers its own package; not decomposed file-by-file this pass (see Not examined).

### Scripts

- deploy/styrman-ctl — COVERED: run directly by `internal/config/styrman_ctl_test.go` (per AGENTS.md's own description of this test).
- deploy/firewall/styrman-firewall.sh — COVERED: invoked by `deploy/firewall/test.sh` (confirmed via grep).
- deploy/firewall/install.sh, deploy/host-setup.sh, deploy/update/styrman-update, deploy/verify-commit.sh, deploy/vulncheck.sh, image/entrypoint.sh, image/configs/* — NOT re-verified against integration.sh's exact invocation list this pass (a `grep -oE` for their paths inside integration.sh returned no hits, but the script may source or exec them indirectly, e.g. through the image build) — listed as **NOT EXAMINED**, not asserted FAIL, pending a direct trace of integration.sh's script-invocation list.
- .agents/docs/testing/styrman/{adr-067-sweep,ensure-test-image,integration}.sh, web/e2e/run.sh — these ARE the test tiers; self-covering by definition (run in this session or the cited logs).

### web/src/ (47 non-test .ts/.tsx files) — **approximate only, not a verified map**

Unit-tested (co-located `*.test.ts` exists): api.ts, appearance (test exists, no matching `appearance.ts` source file found — logic likely lives inside a `.tsx`, not traced), browserInput.ts, cdp.ts, diffReveal (same caveat as appearance), editorPane.ts, explorerHistory.ts, lan.ts, projectBadge.ts, recentsPanel.ts, sshKey.ts.
Filename-substring hits inside `web/e2e/run.mjs` (weak signal, not a confirmed "this check drives this component"): TerminalPane.tsx, FileEditor.tsx, LanBadge.tsx, ui.tsx, DiffView.tsx, CodeEditor.tsx, fonts.ts, clips.ts, toast.ts, editorPane.ts, projects.ts, lan.ts, api.ts, terminal.ts, setup.tsx, settings.tsx, index.tsx.
Remaining ~20 files (BrowserPane.tsx, DeleteProjectDialog.tsx, DiffPane.tsx, EditorPane.tsx, FileExplorer.tsx, icons.tsx, LocalNetwork.tsx, NewProjectDialog.tsx, NotificationBell.tsx, SetupBanner.tsx, Sidebar.tsx, SidePanel*.tsx, TerminalList.tsx, TerminalStatusBar.tsx, router.tsx, routes/p.$project.$term.tsx, routes/__root.tsx, routeTree.gen.ts [generated, glue], hooks.ts) have no unit test and no filename hit in `run.mjs` — **not confirmed uncovered**, because the e2e suite drives components by user-visible behavior, not by importing their names; a real map needs per-check tracing against `web/e2e/run.mjs`'s actual test bodies, which this pass did not do. Reported as **NOT EXAMINED**, not FAIL.

## Not examined

- Per-file trace of which of `deploy/host-setup.sh`, `deploy/firewall/install.sh`, `deploy/update/styrman-update`, `deploy/verify-commit.sh`, `deploy/vulncheck.sh`, `image/entrypoint.sh`, `image/configs/*` is actually invoked by `.agents/docs/testing/styrman/integration.sh` or by the cited integration/e2e logs — only a shallow path-string grep was done and it found no hits, which could mean "not invoked" or "invoked indirectly" (e.g. baked into the image build the tier boots against).
- `deploy/caddy/ingress` broken down file-by-file (only package-level `go test` PASS confirmed).
- Real per-check trace of `web/e2e/run.mjs` against each `web/src` component/route (only filename-substring grep done — weak evidence, explicitly flagged above).
- Did not re-run or open `.agents/docs/testing/2026-09-13-integration-v1.29.0.log.md` / `-e2e-v1.29.0.log.md` beyond citing the counts given in the task (91/0/3 and 92/0/2) — took them as given, per instructions not to re-run those tiers.
- The untracked `.agents/docs/{plans,reviews}/2026-09-13-audit-*` files present at session start were not read or evaluated — out of scope for this verification pass and left untouched.

## Environment

- OS: Linux 6.12.107+deb13-amd64
- Go: (from `go build`/`go test`, toolchain resolved by go.mod; pinned builder for vulncheck was `golang:1.25.13-trixie` per `deploy/Dockerfile.server`)
- Node: v20.19.2, npm 9.2.0
- Docker: Engine - Community 29.7.2, available and used (no Colima needed — Linux host)
- shellcheck: 0.10.0
