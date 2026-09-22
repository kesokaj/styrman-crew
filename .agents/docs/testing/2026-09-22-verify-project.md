---
owner: verifier
date: 2026-09-22
---
# Verification — project   (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff.

## Status: FAIL

Scope: Tier 1 (go build/vet/test, caddy ingress module, web check/lint/test) plus a source-to-test coverage map over the whole repo. Kata integration script, browser e2e, container builds, and vulncheck NOT run (need live host / not Tier 1, excluded by dispatch). `testing/guide.md` already present, no changes needed.

## Commands

- `go build ./cmd/styrman/` → exit 0.
- `go vet ./...` → exit 0.
- `go test -v -count=1 ./...` → exit 0. Output saved to `.agents/scratch/verify-project/go-test-1v.txt`.
  counts: pass 311, fail 0, skip 1 at c05219c for `go test -v -count=1 ./...`.
  vs previous: none (no earlier log in testing/ has a counts line for this exact full-suite command; the only prior counts lines found are for a `-run`-narrowed subset, e.g. `.agents/docs/testing/2026-09-21-verify-v1.36.0-release-round-2.md`'s 19-test narrowed run, not comparable).
- `(cd deploy/caddy/ingress && go test -v -count=1 ./...)` → exit 0. Output saved to `.agents/scratch/verify-project/go-test-caddy.txt`.
  counts: pass 53, fail 0, skip 0 at c05219c for `(cd deploy/caddy/ingress && go test -v -count=1 ./...)`.
  vs previous: none found.
- `(cd web && npm run check)` → exit 0 (`tsc -b`, no output). Saved to `.agents/scratch/verify-project/web-check.txt`.
- `(cd web && npm run lint)` → exit 0 (`oxlint`, no output). Saved to `.agents/scratch/verify-project/web-lint.txt`.
- `(cd web && npm test)` → exit 0 (`vitest run`). Saved to `.agents/scratch/verify-project/web-test.txt`.
  counts: Test Files 15 passed (15), Tests 178 passed (178) — matches the count `testing/styrman/README.md` names for the frontend Tier 1 suite (178 tests across 15 files, 2026-09-14).
  vs previous: matches the README's stated baseline; no dedicated prior verify log found with a differing count.

All six Tier 1 commands green; no `--- FAIL:` or unexpected `--- SKIP:` lines (the one Go skip is `internal/config`'s expected environment-gated skip, consistent with the suite's normal shape — see `internal/config/styrman_update_test.go`'s ssh-keygen/gpg-format skip noted in `testing/styrman/README.md`).

## Root cause
n/a — no failures.

## Coverage of changed code

Full-repo source→test map (no diff to scope against; every top-level area named).

### Go — root module (`go.mod` at repo root)
Every package under `./...` except two has a `*_test.go` beside it (confirmed by listing each `go list ./...` package dir for `*_test.go`):
- `cmd/wsprobe` — **NOT COVERED**: `cmd/wsprobe/main.go` (87 lines) has no test file. It is documented as a manual/integration dev client for the terminal WebSocket ("lets shell scripts verify the full browser path without a browser"), not itself under `go test`; still, no automated test exercises it directly.
- `web` (the Go package placeholder for `go:embed`) — no `.go` test possible; it's an embed directive, exercised indirectly by every `internal/httpapi` test that serves the SPA.
- All other packages (`cmd/styrman`, `internal/config`, `internal/conversation`, `internal/credbroker`, `internal/credstore`, `internal/files`, `internal/gitrepo`, `internal/history`, `internal/httpapi`, `internal/identity`, `internal/links`, `internal/orchestrator`, `internal/project`, `internal/terminal`, `internal/userstore`) — covered, all green.

### Go — vendored Caddy ingress module (`deploy/caddy/ingress`, separate `go.mod`)
Every package under its `./...` has a `*_test.go` — no gaps found.

### Frontend (`web/src`)
47 non-test `.ts`/`.tsx` source files under `web/src` (excluding `routeTree.gen.ts`) vs 15 `*.test.ts(x)` files. Direct/indirect coverage confirmed by reading imports:
- Covered by a unit test: `lib/api.ts` (api.test.ts, api.encoding.test.ts, api.session.test.ts, api.timeout.test.ts), `lib/clips.ts`, `lib/diffReveal.ts`, `lib/cdp.ts`, `lib/browserInput.ts`, `lib/lan.ts`, `lib/editorPane.ts`, `lib/recentsPanel.ts`, `lib/explorerHistory.ts`, `lib/sshKey.ts`, `lib/projectBadge.ts`, `lib/hooks.ts` and `lib/terminal.ts` (both dynamically imported by `appearance.test.ts`), `components/CodeEditor.tsx` (partially — `appearance.test.ts` only asserts its `FONT_STACKS["google-sans-code"]` constant stays byte-identical, not the component's render/editor logic).
- **NOT unit-covered** (no `*.test.ts(x)` imports them), matching `testing/styrman/README.md`'s own statement that "most TypeScript/React logic is still covered only by type-checking + lint plus, where applicable, Tier 4 e2e" — Tier 4 was NOT run this pass, so these are unverified in this run (FAIL lines per file, named as the dispatch requires):
  - `src/components/BrowserPane.tsx`
  - `src/components/DeleteProjectDialog.tsx`
  - `src/components/DiffPane.tsx`
  - `src/components/DiffView.tsx`
  - `src/components/EditorPane.tsx`
  - `src/components/FileEditor.tsx`
  - `src/components/FileExplorer.tsx`
  - `src/components/icons.tsx`
  - `src/components/LanBadge.tsx`
  - `src/components/LocalNetwork.tsx`
  - `src/components/NewProjectDialog.tsx`
  - `src/components/NotificationBell.tsx`
  - `src/components/SetupBanner.tsx`
  - `src/components/Sidebar.tsx`
  - `src/components/SidePanelClips.tsx`
  - `src/components/SidePanelEnv.tsx`
  - `src/components/SidePanelGit.tsx`
  - `src/components/SidePanelHistory.tsx`
  - `src/components/SidePanelPorts.tsx`
  - `src/components/SidePanel.tsx`
  - `src/components/TerminalList.tsx`
  - `src/components/TerminalPane.tsx`
  - `src/components/TerminalStatusBar.tsx`
  - `src/components/ui.tsx`
  - `src/lib/fonts.ts`
  - `src/lib/projects.ts`
  - `src/lib/toast.ts`
  - `src/router.tsx`
  - `src/routes/index.tsx`
  - `src/routes/p.$project.$term.tsx`
  - `src/routes/__root.tsx`
  - `src/routes/settings.tsx`
  - `src/routes/setup.tsx`
  Many of the above are named explicitly in `testing/styrman/README.md`'s Tier 4 description as e2e-exercised (project creation, terminals, Files/Ports/History/Git/Clipboard tabs, editor pane, theme, deep links, setup page) — that suite was excluded from this pass per the dispatch, so their coverage is "not examined here", not confirmed absent; `web/src/lib/editorPane.test.ts`'s own header comment documents one piece of `EditorPane.tsx`/`editorPane.ts`'s behavior (per-terminal draft parking) as deliberately left to Tier 4 rather than unit-tested.

### Shell / deploy / image scripts
Not Go or TS, and not unit-tested by `go test`/`vitest`; covered (per `testing/styrman/README.md`) by `bash -n`, shellcheck, `--dry-run` plans inside `internal/config/*_test.go` (e.g. `gcp_scripts_test.go`, `styrman_ctl_test.go`, `styrman_harness_test.go`), the root-namespace `deploy/firewall/test.sh`, or a manual/Tier 2/Tier 3 procedure — none of these ran in this pass beyond the Go tests already counted above (which include the `--dry-run`-driven `internal/config` script tests, already green in the `go test ./...` run).

## Not examined
- `.agents/docs/testing/styrman/integration.sh` (Tier 2, 115 checks) — needs live Kata/Docker host, excluded by dispatch
- `web/e2e/run.sh` (Tier 4 Playwright, ~92 checks) — needs live host + browser, excluded by dispatch; this is the suite that exercises most of the "NOT unit-covered" frontend files listed above
- `bash deploy/vulncheck.sh` — container-based security scan, excluded by dispatch (not part of Tier 1)
- Any `docker build` / container smoke test — excluded by dispatch
- `deploy/verify-commit.sh`, Tier 5 manual harness-image checks, and the root-run updater self-install harness in `README.md` — all manual/host-specific, out of scope for a repo-wide unit pass

## Environment
Linux (host), go version go1.27.1 linux/amd64, HEAD c05219c, working tree clean except one untracked non-source file (`.agents/docs/reviews/2026-09-22-review-project-code.md`, not part of the change set under test).
