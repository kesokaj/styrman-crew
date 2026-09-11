---
owner: reviewer
date: 2026-09-11
---
# Review — browser-phase4 (2026-09-11)

Range reviewed: `git diff d239293..de3ea47` restricted to internal/httpapi/browser.go,
internal/httpapi/browser_test.go, internal/httpapi/server.go,
internal/orchestrator/browserpeer.go, internal/orchestrator/browserpeer_test.go,
internal/orchestrator/fakedocker_test.go, image/configs/styrman-browser,
web/src/components/BrowserPane.tsx, web/src/lib/browserInput.ts,
web/src/lib/browserInput.test.ts, web/e2e/run.mjs,
.agents/docs/testing/styrman/integration.sh (web/dist and docs excluded).

Blobs read at `de3ea47`: internal/httpapi/browser.go `e67f603`,
internal/httpapi/browser_test.go `538d521`, internal/httpapi/server.go `d71bee3`,
internal/orchestrator/browserpeer.go `d135a1f`, internal/orchestrator/browserpeer_test.go
`c56b04f`, internal/orchestrator/fakedocker_test.go `3064e2b`, image/configs/styrman-browser
`db148ec`, web/src/components/BrowserPane.tsx `e78b418`, web/src/lib/browserInput.ts
`9455672`, web/src/lib/browserInput.test.ts `09b0067`, web/e2e/run.mjs `f631005`,
.agents/docs/testing/styrman/integration.sh `d8eb83d`.

## Verdict: APPROVE

Nothing broken found. This is a tightly-scoped hardening pass (per-user relay
cap, fail-closed `BrowserPeer`, heartbeat so an abandoned pane frees its slot,
guest-controlled CDP payloads type-checked instead of cast, a URL-scheme
allowlist, and a test-infra port-collision guard for BUG-040) and every piece
traced cleanly end to end: `relayCounter.acquire/release` is correctly paired
around the single `defer` in `browserSocket`, the heartbeat goroutine exits on
`ctx.Done()` (deferred `cancel()` in `relayWebSockets` guarantees this on every
return path, no goroutine leak), `BrowserPeer`'s callers already treat `""` as
"no forwarder" so the round-3 fail-closed change is safe, and `brokerIPOnBridge`
still has two other call sites so removing its use from the `SelfContainer`
branch does not orphan it. The new `normalizeUrl` scheme-vs-host-port regex
was traced against every case in its own test (including the
`javascript://%0a…`, `view-source:`, and `127.0.0.1:9000/x` edge cases) and
holds: any attacker string that survives the negative lookahead as a "port"
shape is inert because it is then prefixed with `http://`, never executed as
a scheme. The `styrman-browser` socat-range replacement logic waits for the
old process group to die (via `kill_group_file`'s poll loop) before rebinding
with `reuseaddr`, so no bind race. `listTerminals`' swallow-and-log of a
`tmux.Live` failure is a deliberate, logged degradation (R6: records are the
durable truth) rather than a hidden failure.

## Findings

None rising to the correctness/security/requirements bar.

- **[low / confidence: low]** `.agents/docs/testing/styrman/integration.sh:26-29` — BUG-040's own record notes the fix is partial ("PID-confirming probe not done"): the new guard refuses to start when the port is already bound at launch time, but does not confirm mid-run that the probe answering `${API}` is still this run's own process (a second process could theoretically bind the port after the guard's check races with `go run`'s own bind). Already tracked in `.agents/docs/bugs/INDEX.md`, not a new defect — noted for completeness only.

## Criteria

1. Dead code — pass. No orphaned exports; `brokerIPOnBridge` retains its two other callers after the `BrowserPeer` fail-closed change.
2. Flow tracing — pass. `relayCounter` acquire/release pairs correctly; heartbeat goroutine's exit path traced through `ctx.Done()`; `browserPeer`/`BrowserPeer` empty-string contract is honored by both the httpapi caller and the shell launcher (`peer_range`/`ensure_forwarder` treat no-peer as loopback-only, not an error).
3. Standards conformance — pass. Go error wrapping/logging and TS promise handling (`.catch(() => {})` on fire-and-forget CDP sends) match existing patterns in the same files; shell script follows the existing set -euo pipefail / flock / process-group-kill idioms already in the file.
4. Directory layout — pass. All new/changed files stay beside their existing counterparts (test files next to source, e2e checks appended to `run.mjs`).
5. Simplicity — pass. No new dependencies; the per-user cap is a small mutex-guarded map, no new abstraction layers.
6. Decoupling — pass. `browser.go`'s relay code, `browserpeer.go`'s network lookup, and the frontend's `browserInput.ts` remain independently editable seams; the shell launcher's peer-admission logic stays self-contained.

## Not examined

- `web/src/components/BrowserPane.tsx` lines outside the diffed hunks (rest of the component, e.g. resize/session-management logic) — only the changed hunks were traced.
- Full `web/e2e/run.mjs` file beyond the new browser-view block, and whether `mainContains`/`shot`/`panel` helpers it calls are unaffected (they are not in the diff, assumed pre-existing and correct).
- Runtime behavior — no `go test`/`npm test` run was executed as part of this review; findings are from static reading only.
