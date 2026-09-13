---
owner: reviewer
date: 2026-09-13
---
# Review — httpapi browser/proxy/links/shares/setup/credentials audit   (2026-09-13)

Range reviewed: whole-file `/audit` scan (not a diff) of, at HEAD `e56e775`:
`internal/httpapi/browser.go`, `internal/httpapi/proxy.go`,
`internal/httpapi/links_api.go`, `internal/httpapi/shares.go`,
`internal/httpapi/setup.go`, `internal/httpapi/credharvest.go`,
`internal/httpapi/credentials.go`, judged against `REQUIREMENTS.md` (R1–R13,
principally R3/R4/R8/R10/R12), `AGENTS.md`, `.agents/docs/bugs/INDEX.md`,
`.agents/docs/specs/styrman/guide.md`'s "Browser view" and credential-broker
sections, and the `golang` review-standards checklist.

## Verdict: APPROVE

Nothing broken found. This slice was already the subject of a same-day
(2026-09-13) `/code-review` + security review cycle
(`.agents/docs/changes/2026-09-13-browser-review-fixes.md`,
`.agents/docs/reviews/2026-09-13-review-browser-review-fixes.md`,
`2026-09-13-security-browser-review-fixes.md`) that fixed 1 medium + 8 low
findings across two rounds; independently re-reading the same files turned up
no unfixed defect and no re-introduction of a recorded bug (BUG-002/003/004
guest-writable-path patterns, BUG-032 lock-ordering, BUG-035
mechanism-reaches-every-caller, BUG-045 unreachable `CloseStatus` compare —
all correctly handled here: e.g. `browser.go:543` now uses
`errors.Is(err, websocket.ErrMessageTooBig)`, matching BUG-045's prevention
rule). `go build ./internal/httpapi/...`, `go vet ./internal/httpapi/...`,
and `go test ./internal/httpapi/... -run 'Browser|Proxy|Link|Share|Setup|Cred'`
all ran bare and green (19 tests, rc=0, read before this report was written).
Wiring for csrf/auth exemptions (`isLinkPath`/`isSharedPath`/`isPreviewPath`)
was traced into `auth.go:38,71` and `server.go:605` and is consistent on both
ends. The two items below are low-severity debuggability notes, not defects
a user or attacker can exploit.

## Findings

- **[low / confidence: medium]** `internal/httpapi/credentials.go:36-64` — `credentialContent`'s `switch` on `kind` masks a real credential-store read failure (`s.creds.AllFiles`/`AllEnv`/`GetGit` returning `err != nil`) as the same generic 404 "credential not found" the not-yet-onboarded case returns. An operator debugging a broken broker (disk error, corrupt store) sees only 404s in the Settings "click to read" view with no server-side distinction in the response, though the failure IS logged at `s.logger` elsewhere in the file for other paths (e.g. `credharvest.go:57,177`) — this handler doesn't log the store error at all. Fix: log the store error (`s.logger.Warn`) before falling through to the generic 404, matching the logging discipline used elsewhere in this file.
- **[low / confidence: low]** `internal/httpapi/browser.go:456-553` (`relayWebSockets`) — the two `pump` goroutines and the ping goroutine are not joined before the function returns; `browserSocket`'s `defer s.browserRelays.release(u.Username)` (line 360) frees the relay slot as soon as `relayWebSockets` returns, which can be a few instructions before the losing pump goroutine actually exits (it exits promptly once `Close` makes its blocking `Reader`/`Writer` call error, but there is no synchronization proving that happened). Bounded (goroutines exit on their own within one read/write call) and does not affect correctness of the cap itself since `total`/`byUsr` are already decremented — flagged only because a future change relying on "no relay activity survives past `release`" would be subtly wrong. No fix required unless such a change is made; if it is, add a `sync.WaitGroup` around the two pumps.

## Criteria

1. **Dead code** — pass. Every helper in the seven files (`browserPeer`, `previewAllowed`, `emailIn`, `sharePort`, `tail`, `closeReason`, `pingWithin`, `browserEndpoint`, `stripCookieDomains`, `proxyPrefix`, `listNames`, `rosterHome`, `publicKeyLine`, `tryAcquire`, `guestOut`, `importGuestFile`, `shellQuote`, `seedBrokerFromSetup`, etc.) has at least one non-test caller; none are orphaned.
2. **Flow tracing** — pass. `isLinkPath`/`isSharedPath`/`isPreviewPath` (defined in `links_api.go`/`shares.go`) are consumed correctly by both `auth.go`'s `authGuard`/`csrfGuard` exemptions and `server.go:605`; `previewPortAllowed` is applied consistently at every entry point that can expose a port (`sharePort`, `proxyPort`, `serveLink`, `serveShared`); `browserRelayPerUser`/`browserRelayMax` caps match the BUG-043 "toward the cap of 8" record. Error paths are wrapped or logged at every hop I traced; no discarded `err`.
3. **Standards conformance** — pass against the `golang` checklist: contexts threaded through blocking calls, timeouts on every guest exec and HTTP client (`browserHTTPClient`, discovery request, dial), `resp.Body`/WS conns closed on every path I traced, structured `slog` logging with no secret values in log fields (only kind/id/host, per R10).
4. **Directory layout** — pass. All seven files sit in `internal/httpapi/` per project convention; tests exist beside the code (`browser_test.go`, `shares_test.go`, `credentials_test.go`, `credharvest_guard_test.go`, `setup_pubkey_test.go`, `setup_roster_test.go`, `shared_link_test.go`, `preview_host_test.go`) — `proxy.go` has no `proxy_test.go` of its own, but `proxyFor`/`serveProxy` are exercised by `shared_link_test.go`/`preview_host_test.go`, which is an acceptable feature-based split, not a layout violation.
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth/crypto here (identity/session verification lives in `auth.go`, unreviewed by this pass but wired correctly per criterion 2); the WebSocket relay is stdlib + `coder/websocket` (an approved dependency per `REQUIREMENTS.md`'s Constraints); no speculative abstractions found — `credharvest.go`'s header comment explicitly justifies staying in package `httpapi` rather than a new package.
6. **Decoupling** — pass. `browser.go` is a single cohesive concern (three REST verbs + one relay) despite its 621 lines, not a grab-bag; `credharvest.go` is deliberately split from `credentials.go`'s HTTP surface; `shares.go`/`links_api.go` divide cleanly (share-list authorization vs. link minting/serving) with a narrow shared surface (`previewPortAllowed`, `sharePort`, `serveProxy`).

## Not examined

- `internal/httpapi/auth.go`, `server.go`, `network.go` in full (only the specific lines needed to verify wiring from the seven target files were read).
- `internal/orchestrator`, `internal/credstore`, `internal/links`, `internal/project`, `internal/files` internals (their exported surface was trusted as correct where called; `project.Store.List()`'s "newest first" claim in `setup.go`'s `rosterHome` comment was spot-checked and confirmed at `internal/project/store.go:271-280`, but the packages were not otherwise reviewed).
- Frontend (`web/`) consumers of these endpoints (`web/src/lib/api.ts`, `BrowserPane.tsx`, etc.) — out of scope per the dispatch's file list.
- `deploy/caddy/ingress` (separate Go module) and the `X-Styrman-Ingress`/`X-Caddy-Email` trust chain beyond the two call sites cited above.
- Runtime behavior against a live Kata microVM (`.agents/docs/testing/styrman/integration.sh`, `web/e2e/run.sh`) was not re-run; the change entry's own 2026-09-13 runs (rc=0, 90/0/3 and 92/0/2) were read but not reproduced by this review.
- `-race` was not run by this review (only plain `go test`); the change entry records a passing `go test -race ./...` for the same commit lineage.
