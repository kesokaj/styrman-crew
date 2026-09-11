---
owner: reviewer
date: 2026-09-11
---
# Review — browser-phase2   (2026-09-11)

Range reviewed: `git diff e7f7857..0e543e3` restricted to internal/config/config.go,
internal/httpapi/browser.go, internal/httpapi/browser_test.go, internal/httpapi/proxy.go,
internal/httpapi/server.go, internal/httpapi/shares.go, internal/orchestrator/ports.go,
internal/orchestrator/ports_browser_test.go, image/Dockerfile,
image/configs/styrman-browser, image/entrypoint.sh, .agents/docs/testing/styrman/integration.sh.

Blobs pinned: config.go@d2dbcfc, browser.go@91d41f2, browser_test.go@083e6c1,
proxy.go@920c52a, server.go@7688216, shares.go@957edc8, ports.go@548bc59,
ports_browser_test.go@0d90f2c, Dockerfile@1c9dde1, styrman-browser@b165e4d,
entrypoint.sh@1b56329, integration.sh@0efc323.

`go build ./...`, `go vet ./...`, and `go test ./internal/httpapi/... ./internal/orchestrator/...`
all pass on `0e543e3`. `shellcheck` on the two changed shell scripts shows only two
pre-existing SC2015 infos on lines this diff does not touch.

## Verdict: APPROVE WITH NITS

Nothing broken found. The port-hiding invariant (9222/9229 never previewed, shared,
listed or linked) is enforced at every entry point I could find (`proxyPort`,
`sharePort` — which both `setShares`/`getShares` and `createLink` route through —
and `parseListeningPorts`), and is covered by both Go tests and the new integration
checks. The socat `range=`/no-Origin-allowlist hardening in the launcher and the
`--ignore-scripts` + tree-dump hardening in the Dockerfile match their inline
rationale and don't contradict R12/security-exemption rules. One cosmetic
correctness nit below; nothing rises to a blocking finding.

## Findings

- **[low / confidence: high]** `internal/httpapi/browser.go:49` — `errBrowserPort` is
  a package-level `var` built once with `config.BrowserPort` (9222) baked into the
  message text: `"port %d is the project's browser…"`. When the *actual* rejected
  port is `config.BrowserChromePort` (9229) — e.g. `PUT /api/projects/{p}/ports/9229/shares`
  — the response still says "port 9222 is the project's browser…", misreporting which
  port was rejected. The rejection itself (400) is correct; only the message is wrong.
  `TestBrowserPortRefusedOnPreviewSharesLinks` iterates both 9222 and 9229 but only
  asserts `strings.Contains(body, "project's browser")`, so it doesn't catch the wrong
  number. Fix: build the message with the actual `port` argument inside
  `previewPortAllowed`, e.g. `fmt.Errorf("port %d is the project's browser and cannot
  be previewed or shared", port)`.

## Not examined

- The frontend (`web/`) was out of scope for this dispatch. `config.CloseBrowserUnavailable`
  (4004) is a new WS close code with no corresponding `CLOSE_*` constant or handler
  found in `web/src/lib/api.ts` / `web/src/components/TerminalPane.tsx` as of this
  diff — I did not check whether a Browser pane component exists elsewhere or is
  planned for a later phase; flagging as an open question rather than a finding
  since `web/` is outside the reviewed path list.
  the login-shell recompute in `image/configs/styrman-credentials.sh` (unchanged by
  this diff) recomputes `STYRMAN_CRED_BROKER` from the guest's own IP to survive a
  "network recreation" that leaves the container in place; `styrman-browser`'s
  `peer_range()` is invoked via `docker exec` (no login shell), so it reads whatever
  `STYRMAN_CRED_BROKER` was injected at container-create time. I did not find a
  code path in this diff's scope where the container survives a network rebuild
  without also being recreated (ADR-067's mode toggle rebuilds both), so I did not
  treat this as a live bug, but did not trace `EnsureRunning`/container-recreate
  logic exhaustively to confirm no such path exists.
- Did not run the integration.sh suite or web e2e (require live Kata/Docker
  environment); relied on static reading plus `go test`/`go vet`/`shellcheck`.

## Criteria

1. Dead code — pass. No orphaned functions; `parseListeningPorts` extraction is a
   genuine refactor for testability, old nil-guard removed with equivalent `:=[]Port{}` init.
2. Flow tracing — pass, with the messaging nit above. `browserExec`/`browserStart`/
   `browserStop`/`browserStatusOf` correctly mirror the `ListeningPorts` "Status first,
   re-ask Status on exec failure, never classify exec's error text" pattern (BUG-023
   lesson applied). `previewPortAllowed` is threaded through every port-parsing entry
   point (`proxyPort`, `sharePort` used by shares GET/PUT and `createLink`, and
   `parseListeningPorts`) — verified by reading all handlers that parse `{port}`.
3. Standards conformance — pass. Go error wrapping/handling is consistent with the
   rest of `internal/httpapi`; shell scripts pass shellcheck; Dockerfile pinning
   practices match the file's existing conventions (archive-pinned chromium,
   version-pinned + `--ignore-scripts` npm package).
4. Directory layout — pass. New Go files land beside their siblings
   (`internal/httpapi/browser.go` + `browser_test.go`,
   `internal/orchestrator/ports_browser_test.go` beside `ports.go`).
5. Simplicity / DIY-except-security — pass. No new dependencies; the CDP relay is a
   plain byte/frame pump with no protocol parsing (as its own comment states); the
   `socat range=` + missing `--remote-allow-origins` are network/config controls, not
   hand-rolled crypto/auth, so the security exemption isn't implicated.
6. Decoupling — pass. `previewPortAllowed` is a single, small, shared choke point
   consumed by `proxy.go`/`shares.go`/`ports.go` rather than duplicated per-caller
   logic; `browserUpstreamFn` is a clean test seam on `Server` mirroring the existing
   `guest` seam.
