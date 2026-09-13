---
owner: main
date: 2026-09-13
---
# Browser view review fixes (v1.29.0)   (2026-09-13)

## Summary

Fixed the 15 findings of a `/code-review` of the v1.27.1..v1.28.0 range (Browser view, ADR-068: launcher, server CDP relay, SPA pane, port scanning) plus the below-the-cut items the plan marks *do*. The fix diff was then reviewed in its own right: the reviewer found 1 medium + 5 low and the security reviewer 3 low, all fixed in a follow-up batch (below). The plan's cold-read took three rounds (12 → 7 → 7 items, the cap); round 1's blocking item was a Stop control the author had added beyond the review's findings — struck, because it would kill the Chromium the agents share and every other pane's relay (a feature needing its own scope gate); the round-3 leftovers were implementation details with one answer each, decided in the plan under the operator's autonomous instruction and flagged in the handoff.

**Commits:** c4128f5 (the review fixes, plan, cold-reads) and the follow-up batch committed right after this entry (the reviewer's and security reviewer's findings, docs, records).

## Type
task

## Files changed
- `image/configs/styrman-browser` — MCP exec closes start-lock fd 9; launcher `kill_group_file` predicate (pid ≥ 2, comm/group check); `forwarder_up` pid+comm+listen; `peer_range` returns 1 on set-but-unusable; `socat max-children=16`; `do_mcp` execs with `9>&-`
- `internal/httpapi/browser.go`, `browser_test.go` — relay start path one `browserExec("start")` exec; `closeReason` keeps the END of the launcher's output, rune-safe and `strings.ToValidUTF8`; `Conn.Read`/`Write` via `Reader`/`Writer` + pooled buffers; heartbeat ping fresh 15s deadline per side; revocation watch closes pane 4003; `browserEndpoint` sets `req.Close = true`
- `internal/httpapi/browser_launcher_test.go` (new file) — tests pin launcher `PORT`/`CHROME_PORT`/`max-children` to `config`/`browserRelayPerUser`; test `kill_group_file` pid/comm/group predicate; test `peer_range` set-but-unusable return code
- `internal/httpapi/server.go` — new `stillAuthorized func(string) bool` and `browserReauthEvery time.Duration` fields wired to relay
- `internal/orchestrator/ports.go`, `ports_browser_test.go` — single exec with sentinel line split; pure `splitPortScan` and `parseDockerPorts` functions; `timeout 3` on docker ps half (BUG-039's rule)
- `internal/terminal/bridge.go`, `reauth_test.go` — export `RevocationWatch`; call-site updates
- `internal/config/styrman_ctl_test.go` — new test case: `--dry-run status` ≡ `status --dry-run` (global flag parsing)
- `deploy/styrman-ctl` — global flags before verb; `--confirm` requires a value with explicit check; `CTL_TMPD` trap guards with `-n` and exits 130 on INT/TERM/HUP
- `web/src/components/BrowserPane.tsx` — `visible` prop gates viewport write (0×0 on hide); one screencast writer guarded by `screencastRef`; serialized `attach` via ref promise chain; pointer capture on pointerdown; rAF mousemove coalesced; a `revoked` phase on close 4003 (no Reconnect); a half-attached session detached when `attach` fails; Retry via `POST /browser/start`; close client before phase; onerror drains pending; autoRetries reset; one `reconnect()` helper
- `web/src/routes/p.$project.$term.tsx` — `useViewLatch` keyed per terminal; `visible` threaded to BrowserPane
- `web/src/lib/browserInput.ts`, `browserInput.test.ts` — `isChord = (ctrl || meta) && !alt`; AltGr test
- `web/src/lib/api.ts` — `api.browserStatus`/`api.browserStop` removed (dead code, REST routes still live); `ApiError` exported
- `web/src/lib/api.encoding.test.ts` — two removed rows for deleted wrappers
- `.agents/docs/testing/styrman/integration.sh` — port guard captures `ss` into variable, greps after (fail closed on missing `ss`)
- `AGENTS.md` — "82 checks" pointer updated to `.agents/docs/testing/styrman/README.md`
- `web/e2e/run.mjs` — new check: "switching to the terminal and back resumes the screencast" (inserted between port-open and round-trip checks, before network rebuild)
- `web/dist/client/` — rebuilt SPA

## Agents dispatched
reviewer ×3 (cold-read rounds 1–3, claude-opus), verifier, reviewer (diff, claude-opus: APPROVE WITH NITS — 1 medium + 5 low, all fixed), security (3 low, all fixed), documentation, bugs (BUG-042..052), changelog (this entry)

## Verification

**Unit/vet/build:** `go build ./...` exit 0; `go vet ./...` exit 0; `go test -race ./...` exit 0 (all 16 packages ok).
**Go test count:** `internal/httpapi` 130 PASS (new: `TestBrowserRelayClosesOnRevocation`, `TestBrowserRelayOversizeUpstreamFrameIs1009`, `TestCloseReasonFitsAndKeepsRunes`); `internal/orchestrator` 29 PASS; `internal/terminal` 7 PASS; `internal/config` 29 PASS (added case for `--dry-run status`).
**New test files:** `internal/httpapi/browser_launcher_test.go` (3 new tests: pins, peer range, kill-group), `internal/orchestrator/ports_browser_test.go` (added 3 sentinel/parse tests).
**Ingress module:** `(cd deploy/caddy/ingress && go test ./...)` exit 0 (cached, unchanged).
**Frontend:** `(cd web && npm run check && npm run lint && npm test)` — tsc clean, oxlint clean, vitest 171/171 PASS (13 files, including `browserInput.test.ts` AltGr case).
**Shell:** `bash -n image/configs/styrman-browser deploy/styrman-ctl .agents/docs/testing/styrman/integration.sh` exit 0; `shellcheck` exit 0 (both).
**Integration (host-run rebuild `styrman-runtime:test`):** `.agents/scratch/integration-2026-09-13.log` — `passed=90 failed=0 skipped=3` rc=0. Browser checks all PASS: MCP registered, /json/version 200, ports omit 9229, proxy refuses 9222, start/stop/status round-trip, stop ends it.
**E2E (`web/e2e/run.sh`):** `.agents/scratch/e2e-2026-09-13.log` — `passed=92 failed=0 skipped=2` rc=0, including new "switching to the terminal and back resumes the screencast" check.
**E2E re-run on the final tree (after the reviewer/security fixes):** `.agents/scratch/e2e-2026-09-13-final.log` — `passed=92 failed=0 skipped=2` rc=0, all five Browser-view checks PASS.
**Code review cited:** `.agents/docs/reviews/2026-09-13-review-browser-review-fixes.md` (APPROVE WITH NITS, diff verified on committed blobs c4128f5); `.agents/docs/reviews/2026-09-13-security-browser-review-fixes.md` (3 low, all fixed); cold-read rounds 1–3 (`.agents/docs/reviews/2026-09-13-coldread-browser-review-fixes*.md`) cap reached with design questions resolved.

## Requirements
n/a

## Notes
Scope decision recorded in plan (`.agents/docs/plans/2026-09-13-browser-review-fixes.md` section "Scope"): a Stop control for the pane would kill the shared Chromium and every other pane's relay, making it a new destructive user-facing verb needing scope confirmation, confirmation dialog and close code — deferred as a feature. The plan's "Follow-ups noted, not built" items (Stop control, `liveUnknown` terminal flag, harness re-seed cap) are recorded in TODO.md with Inbox entries.
