---
owner: reviewer
date: 2026-09-11
---
# Review — browser-phase3 (2026-09-11)

Range reviewed: `git diff 0e543e3..d239293 --` restricted to the Browser-view
(ADR-068) file set named by the dispatch (web/src/components/BrowserPane.tsx,
web/src/lib/cdp.ts + test, web/src/lib/browserInput.ts + test,
web/src/lib/editorPane.ts + test, web/src/lib/api.ts + encoding test,
web/src/routes/p.$project.$term.tsx, web/src/components/SidePanelPorts.tsx,
internal/httpapi/browser.go + test, internal/httpapi/links_api.go,
internal/httpapi/server.go, internal/orchestrator/browserpeer.go,
image/configs/styrman-browser, image/entrypoint.sh,
.agents/docs/testing/styrman/integration.sh). `web/dist` excluded per
dispatch. Blobs pinned above the findings that cite them; commit `d239293`
throughout.

## Verdict: APPROVE WITH NITS

This is a careful, defense-in-depth piece of work: the peer-gated socat
forwarder fails closed with no peer, `previewPortAllowed` is re-checked at
serve time for stale link/share records (own regression test,
`TestBrowserPortLinkAndShareRecordsDoNotServe`), the relay follows no
redirect and checks Origin before any side effect, and the relay count is
capped. `git diff --numstat` shows no suspicious deletions or binary
content (BUG-005/007 clean). I found one real correctness gap (an
unrecoverable stale-forwarder state after a peer-address change) and one
test-coverage gap for new orchestrator logic; neither blocks, both are worth
fixing before this goes live unattended.

## Findings

- **[medium / confidence: medium]** `image/configs/styrman-browser`
  (blob `091cf5d`) `ensure_forwarder()` — once the socat forwarder is
  listening on `:9222`, `forwarder_up && return 0` skips re-checking whether
  the CURRENT `STYRMAN_BROWSER_PEER` still matches the range socat was
  started with. If the server's address on the project bridge ever changes
  while Chromium/socat are left running (a project that keeps its browser
  session alive across a server-side re-attach that falls back from the
  static broker IP to a dynamic one — `EnsureSelfAttached` in
  `internal/orchestrator/orchestrator.go:640-651` — or any future path that
  legitimately reassigns the peer), the forwarder keeps admitting only the
  OLD peer; the new relay dial from the server is refused by socat's fixed
  `range=`, and the Browser pane gets a persistent "browser endpoint
  unreachable" with no self-heal short of a manual stop/start of the
  browser (which nothing in the UI prompts for). Concrete fix: have
  `ensure_forwarder` compare the running forwarder's admitted range against
  the freshly computed one (e.g. stash the range next to `SOCATPID` and
  `kill_group_file` + relaunch on mismatch) instead of only checking
  liveness. Confidence is medium because the common path (static broker IP,
  same subnet) keeps the peer stable across reattaches — this needs the
  fallback-to-dynamic branch or a subnet change while the guest process
  survives, both narrower than the headline scenario.

- **[low / confidence: medium]** `internal/orchestrator/browserpeer.go`
  (blob `baf55f9`) — `BrowserPeer` (new exported method, both the
  container-inspect branch and the network-gateway-fallback branch) has no
  direct unit test; it is only exercised indirectly through
  `s.browserPeerFn` overrides in `internal/httpapi/browser_test.go`
  (`TestBrowserStartPassesPeer`), which stub the method out entirely. The
  package already has a `fakeDocker` with working `ContainerInspect` and
  `NetworkInspect` stubs (`internal/orchestrator/fakedocker_test.go:105,241`)
  that would make a real test cheap — worth adding given this method now
  gates whether the DevTools forwarder ever answers.

## Not examined

- `internal/orchestrator/browserpeer.go`'s IPv4-only filtering
  (`ip.To4() != nil`) was read but not traced against an IPv6-only bridge
  configuration — out of scope for this deployment (Docker default bridges
  here are IPv4) but flagging as unexamined rather than cleared.
- Did not run `go test ./...`, `web` vitest, or the integration/e2e suites —
  static review only, per the dispatch's file-list scope.
- `image/entrypoint.sh`'s two-line log-truncation change and the
  `.agents/docs/testing/styrman/integration.sh` assertion rewrites were read
  and appear consistent with the 9222/9229 semantics change, but were not
  exercised against a live Kata guest.
- Icon components imported by `BrowserPane.tsx` (`ChevronLeftIcon` etc.) are
  outside the dispatched file list and were assumed pre-existing, not
  verified.

## Criteria

1. **Dead code** — pass. `errBrowserPort` was cleanly removed with its only
   use inlined; no orphaned exports found in the reviewed files.
2. **Flow tracing** — pass with the one gap above. `browserPeer` →
   `browserExec` → guest env is traced correctly and ordered after
   `EnsureSelfAttached` in every caller (`startBrowser`, `browserSocket`);
   the `previewPortAllowed` re-check was correctly propagated to both
   `serveLink` and `serveShared`, not just the mint (`internal/httpapi/links_api.go`
   blob `c99e26e`, lines both new). The one place flow tracing breaks down is
   the stale-forwarder case above — the "peer changed" case was designed for
   (comment says "an unknown peer means no forwarder — fails closed") but the
   "peer changed to a different known value" case falls through to a stale
   accept-list instead of failing closed or updating.
3. **Standards conformance** — pass. Go errors wrapped/propagated
   consistently (`fmt.Errorf` with `%w` where applicable, `errors.Is` used
   correctly); TS promises are all `.catch()`-guarded or awaited in the
   reviewed files; the shell script (`shopt`/`set -e` conventions consistent
   with the project's existing `styrman-browser`) matches sibling scripts.
4. **Directory layout** — pass. New TS libs land in `web/src/lib/`
   alongside their tests; new component in `web/src/components/`; new Go
   file `internal/orchestrator/browserpeer.go` sits with its package,
   correctly small and single-purpose.
5. **Simplicity (DIY-except-security)** — pass. `cdp.ts` is a deliberately
   minimal hand-rolled JSON-RPC-shaped client with no new dependency,
   justified in its own doc comment ("a few dozen lines cover it") — this is
   not the auth/crypto/session carve-out, so DIY is appropriate here, not a
   violation.
6. **Decoupling** — pass. `browserInput.ts` is kept DOM-free and
   independently unit-tested from `BrowserPane.tsx`; `cdp.ts` has no
   dependency on the relay's wire format beyond "text frames"; the Go side's
   `previewPortAllowed` remains the single chokepoint for the preview/share/
   link decision, reused rather than re-implemented at each of the three call
   sites.
