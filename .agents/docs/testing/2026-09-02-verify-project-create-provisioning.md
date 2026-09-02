# Verification — project-create provisioning (attach-before-clone, login-shell provisioning, provisionError, hostname)   (2026-09-02)

Scope: 503fc8a..working-tree (uncommitted working-tree diff vs HEAD); command run: `git diff HEAD -- internal web/src`

## Status: PASS

Note: `git status` also shows deletions under `.claude/agent-memory/**` and
regenerated `web/dist/client/assets/*` (stale build output churn), but those
paths are outside the requested scope (`internal web/src`) and are not
covered by this log.

## What changed (from the diff)

1. `internal/httpapi/server.go` — `createProject`'s background goroutine now
   calls `s.orch.EnsureSelfAttached` BEFORE clone/GitHub provisioning
   (previously ran after, leaving the credential broker unreachable and
   breaking every clone / `gh repo create`); adds `waitForBroker` (polls
   `$STYRMAN_CRED_BROKER/healthz` in-guest via a login shell, 10× 500ms);
   runs the clone and the commit/`gh repo create` steps through `bash -lc`
   (login shell) so `/etc/profile.d/zz-styrman-credentials.sh` materializes
   credentials; adds a `provisionErrs` map (mirrors the existing
   `deleteErrs` pattern) surfaced as `projectView.ProvisionError` from
   `listProjects` and `getProject`, and cleared on delete. New helpers:
   `waitForBroker`, `guestErrMessage`, `setProvisionError`, `provisionError`.
2. `internal/orchestrator/orchestrator.go` — new `guestHostname(name)`:
   drops the `styrman-` hostname prefix (trims a legal-but-illegal trailing
   dash, falls back to `"project"` if empty).
3. `web/src/lib/api.ts` — new `projectNameFromRepoURL()` (slugs the repo
   URL's last path segment to the server's name rules); `provisionError` on
   the `Project` type. `web/src/components/NewProjectDialog.tsx` — clone-flow
   name auto-derives from the URL until the user edits it by hand.
   `web/src/components/Sidebar.tsx` — "repo setup failed" badge driven by
   `provisionError`.

## Commands run

- `go build ./...` → exit 0, no output.
- `go vet ./...` → exit 0, no output.
- `go test -race ./...` → all packages `ok` (or `[no test files]`), exit 0:
  ```
  ok  	github.com/kesokaj/styrman-crew/cmd/styrman	1.029s
  ?   	github.com/kesokaj/styrman-crew/cmd/wsprobe	[no test files]
  ok  	github.com/kesokaj/styrman-crew/internal/config	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/conversation	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/credbroker	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/credstore	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/files	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/gitrepo	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/history	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/httpapi	1.123s
  ?   	github.com/kesokaj/styrman-crew/internal/identity	[no test files]
  ok  	github.com/kesokaj/styrman-crew/internal/links	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/orchestrator	1.018s
  ok  	github.com/kesokaj/styrman-crew/internal/project	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/terminal	(cached)
  ok  	github.com/kesokaj/styrman-crew/internal/userstore	(cached)
  ?   	github.com/kesokaj/styrman-crew/web	[no test files]
  ```
- `(cd deploy/caddy/ingress && go test ./...)` (separate module, unaffected
  by this diff but required by the local-tier checklist) → `ok
  github.com/spgo/devbox/caddy/ingress (cached)`, exit 0.
- `go test -cover ./internal/httpapi/... ./internal/orchestrator/...` (extra,
  to judge coverage honestly) →
  `ok github.com/kesokaj/styrman-crew/internal/httpapi 0.059s coverage: 36.9% of statements`,
  `ok github.com/kesokaj/styrman-crew/internal/orchestrator 0.005s coverage: 7.2% of statements`.
- `cd web && npm run check` (tsc -b) → exit 0, no output. (`node_modules`
  was already present from an `npm ci` run earlier this session; not
  reinstalled.)
- `cd web && npm run lint` (oxlint) → exit 0, no output.
- Dockerfile check: `git diff HEAD --name-only | grep -i dockerfile` →
  no match. No container image build performed — out of scope per the
  task instructions (no Dockerfile changed).

## Coverage of changed code

Covered by the author's new/existing tests:
- `internal/orchestrator/orchestrator.go` `guestHostname()` — covered by
  `TestGuestHostname` (internal/orchestrator/orchestrator_test.go): prefix
  removal, pass-through, trailing-dash trim, empty→"project" fallback. Good,
  direct coverage of the one pure function this diff adds there.
- `internal/httpapi/server.go` `guestErrMessage()` — covered by
  `TestGuestErrMessage` (internal/httpapi/server_test.go): fatal-line
  selection over the "Cloning into" banner, ssh-diagnosis selection,
  URL-token redaction, empty-output→prefix-only fallback, length cap,
  control-byte stripping. Good, thorough coverage of this pure function.

NOT COVERED by any automated test — named explicitly, as instructed:

- `internal/httpapi/server.go` `waitForBroker()` — zero test coverage. The
  polling loop (10 attempts × 500ms, ctx-cancellation early-exit, success on
  first 0-exit `curl`) is exercised nowhere; `go test -cover` numbers above
  confirm this — httpapi sits at 36.9%, and grepping
  `internal/httpapi/server_test.go` for `waitForBroker` returns no hits.
- The reordering itself (`EnsureSelfAttached` now runs BEFORE
  clone/provisioning instead of after) — this is the core bug fix the diff
  claims to make, and there is no test that asserts the goroutine calls
  `EnsureSelfAttached` before `run(...)` for the clone/gh-create branches, or
  a fake/mocked orchestrator that would catch a regression reintroducing the
  old (broken) order. `TestCloneURLValidation` (the one other test touching
  this handler) only exercises request validation, not the background
  goroutine's provisioning sequence.
- `setProvisionError` / `provisionError` (get/set on the new `provisionErrs`
  map) and the `ProvisionError` field's plumbing through `listProjects` /
  `getProject` / `deleteProject` — no direct unit test. (Low risk: the
  pattern is copy-pasted from the already-shipped `deleteErrs` mechanism,
  but it is still new code, untested as written.)
- The `bash -lc` login-shell switch for clone / commit / `gh repo create`,
  and the `shellQuote()` usage on `req.CloneURL` and `p.Name` inside those
  shell strings — no test constructs a project name or clone URL containing
  shell metacharacters and asserts the resulting command line is safe /
  correct. `TestCloneURLValidation` only checks that invalid URL shapes are
  rejected before reaching this code, not the shell-string construction
  itself.
- `web/src/lib/api.ts` `projectNameFromRepoURL()` — brand-new exported
  function, zero test coverage. `web/` has no unit-test framework
  (`package.json` scripts are `dev`/`build`/`lint`/`preview`/`check` only;
  no `vitest`/`jest`/`testing-library` dependency; `find web/src -iname
  '*.test.*' -o -iname '*.spec.*'` returns nothing), so this cannot be
  covered without adding one. The only browser-level suite is Playwright
  e2e (`web/e2e/run.mjs`), and `grep` for `cloneUrl|clone|NewProjectDialog|
  createProject` against `web/e2e/run.mjs` returns no hits — the clone-URL
  create flow is not in the e2e suite either.
- `web/src/components/NewProjectDialog.tsx` — the name-auto-derive-until-
  edited behavior (`nameEdited` state, `effectiveName`, the "Named after the
  repo: …" hint text) — no unit test (no framework) and no e2e coverage
  (not exercised by `web/e2e/run.mjs`).
- `web/src/components/Sidebar.tsx` — the new "repo setup failed" badge
  (conditional on `provisionError && !deleteError && state !== "deleting"`)
  — no unit test, no e2e coverage.

## Verdict

All commands the local tier requires ran clean: `go build`, `go vet`,
`go test -race` (root module), `go test` (vendored Caddy ingress module),
`tsc -b` (`npm run check`), and `oxlint` (`npm run lint`) all pass with no
failures. However, changed code without a covering test is a FAIL condition
per the verification contract, and this diff has substantial changed
surface with no automated coverage: the entire attach-before-provisioning
ordering fix (`waitForBroker`, and the reorder itself — arguably the core
bug this diff fixes), the login-shell/shellQuote provisioning path, the
`provisionErrs` plumbing, and the whole frontend surface
(`projectNameFromRepoURL`, `NewProjectDialog`'s auto-name behavior, the
Sidebar badge) are all untested. Green test runs do not offset this.

**Overall: FAIL — coverage gap**, not a red suite. Every command listed
above passed; the fail reason is exclusively the uncovered code named above.

## Environment

- Tier: local only (no remote/GCP access used or requested).
- OS: Linux 6.12.107+deb13-amd64 (native, not Colima — this repo's
  container tier is Docker/Kata on Linux directly).
- No Dockerfile changed in this diff; no image build performed (out of
  scope per task instructions, and not required by the coverage verdict
  above either).
- Go: root module (`github.com/kesokaj/styrman-crew`) and the vendored
  Caddy ingress module (`deploy/caddy/ingress`, module
  `github.com/spgo/devbox/caddy/ingress`) both tested independently as two
  separate modules, per `.agents/docs/testing/styrman/README.md`.
- Frontend: `web/node_modules` was already present (installed via `npm ci`
  earlier this session per task instructions); not reinstalled here.
