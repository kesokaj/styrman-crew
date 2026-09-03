---
owner: verifier
date: 2026-09-03
---
# Verification — fail-to-fetch, round 2   (2026-09-03)

Range reviewed: working tree vs HEAD — `web/src/lib/api.ts` (security-review fixes),
new `web/src/lib/api.session.test.ts` (5 tests), `web/dist/client` rebuild. Round 1 log:
`.agents/docs/testing/2026-09-03-verify-fail-to-fetch.md`.

## Status: PASS

Scope: complete for the items named in the dispatch — web check/lint/test, Go build/vet/test
(root module), vendored caddy ingress tests, and web/dist integrity. Did not run
`web/e2e/run.sh` or the integration script (excluded by dispatch).

## Commands

- `cd web && npm run check` → PASS. Output: `tsc -b` (no errors, exit 0).
- `npm run lint` → PASS. Output: `oxlint` (no errors, exit 0).
- `npm test` → PASS. `Test Files 3 passed (3)` / `Tests 16 passed (16)`, Duration 278ms.
- `npx vitest run src/lib/api.session.test.ts --reporter=verbose` → PASS, 5/5:
  ```
  ✓ request() when the ingress session has expired > re-authenticates once instead of surfacing 'Failed to fetch' until F5  42ms
  ✓ request() when the ingress session has expired > bounds the reload loop when one endpoint redirects while another stays healthy  3ms
  ✓ request() when the ingress session has expired > clears the counter only after the page has stayed healthy  1ms
  ✓ request() when the ingress session has expired > ignores a malformed counter instead of letting it disable the cap  1ms
  ✓ request() when the ingress session has expired > uploadFiles re-authenticates too, instead of a bare 'upload failed'  28ms
  Test Files  1 passed (1)
       Tests  5 passed (5)
  ```
- `go build ./... && go vet ./... && go test ./...` (repo root) → PASS. `go build` and
  `go vet` produced no output (exit 0). `go test ./...`: all 12 tested packages `ok`
  (cmd/styrman, internal/{config,conversation,credbroker,credstore,files,gitrepo,history,
  httpapi,identity,links,orchestrator,project,terminal,userstore}); cmd/wsprobe and web
  report `[no test files]`.
- `(cd deploy/caddy/ingress && go test ./...)` → PASS: `ok github.com/spgo/devbox/caddy/ingress`.

## web/dist integrity

- Scripts referenced from `web/dist/client/_shell.html` (grep matched nothing on this
  environment's default `grep` for this file — worked around with a Python regex scan;
  file content is confirmed plain UTF-8 text via `od -c`/`python3`, not binary):
  `assets/index-BA7F6YGy.js`, `assets/jsx-runtime-DE3RlOCf.js`,
  `assets/rolldown-runtime-hePW80VL.js`, `assets/routes-PE0_8PtQ.js` — all 4 present on disk.
- `opaqueredirect` string search across `web/dist/client/assets/*.js`: appears in exactly
  one file, `web/dist/client/assets/index-BA7F6YGy.js` (count 1).
- `git status --short -- web/dist` → all entries under `web/dist/client/...` (one modified
  `_shell.html`, 14 deleted old-hash chunks, 14 new-hash chunks added) — confined to
  `web/dist/client`, consistent with a clean `npm run build` re-hash. No files outside
  `web/dist/client` touched.

## Coverage of changed code (`git diff -- web/src/lib/api.ts`)

- Counter shape validation (`Number.isInteger(n)`, `n >= 0`, `typeof at === "number"`,
  `Number.isFinite(at)`) — covered by "ignores a malformed counter instead of letting it
  disable the cap" (seeds `n: -1000`, asserts the cap still fires at n=1, not disabled).
- `redirect: "manual"` placed after the `...init` spread (request path) — covered by
  "re-authenticates once instead of surfacing 'Failed to fetch' until F5" and "bounds the
  reload loop..." — both rely on the mocked `fetch` returning `opaqueredirect` only when
  `init?.redirect === "manual"`, which only happens if the spread does not clobber it.
- Deferred, guarded clear of the `reauth` counter (`setTimeout` + `if (reAuthTriggered)
  return`, `REAUTH_CLEAR_AFTER_MS`) — covered by "clears the counter only after the page
  has stayed healthy" (advances 9s: still set; advances 1 more: cleared) and by "bounds the
  reload loop..." (advances 20s past the delay, counter still non-null because
  `reAuthTriggered` was true, guarding the clear).
- `uploadFiles` redirect handling (`redirect: "manual"` added to the upload fetch, and
  `res.status === 401 || res.type === "opaqueredirect"` check) — covered by "uploadFiles
  re-authenticates too, instead of a bare 'upload failed'".

No gaps found — every changed hunk has a directly corresponding test in
`api.session.test.ts`.

## Environment

OS: Linux 6.12.107+deb13-amd64. Go: per repo `go.mod` (root module — build/vet/test all
ran clean). Node/npm: project's pinned toolchain via `web/package.json` (vitest v4.1.11,
tsc -b, oxlint). Docker/Colima: not used for this round (no Dockerfile changes in scope,
per dispatch — round did not touch containers).
