---
owner: main
date: 2026-10-02
---
# A terminal that stopped responding recovers without a reload — Phase 2   (2026-10-02)

Phase 2 of `plans/2026-10-02-tool-updates-freeze-harness-v2.md`, through the
`bug` skill: BUG-085. The operator: "if I close the tab for crew it freezed, i
need to refresh the window pretty often". No exact repro; asked, the operator
chose to have both inferred causes fixed.

## The two causes
- **A replaced terminal had no way back.** Opening a terminal in a second tab,
  or in the installed PWA window, closes the first with 4001. The first then
  showed "Opened in another tab" and nothing else, so closing the second left
  it dead until a reload.
- **A dead socket was never noticed.** After sleep or a network change the
  page's `WebSocket` can read `OPEN` on a connection that is gone. The server's
  own ping closes its side; that close cannot reach the page, and the page had
  no check of its own.

An `explorer` given only the symptom, reading the code at `1fe28a0`, ranked the
same two first (`plans/2026-10-02-recon-ui-freeze-diagnosis.md`). Its third —
the retry budget running out, and a WebSocket handshake that never triggers
re-authentication — is half covered: wake events now reconnect a "Disconnected"
pane; an expired session is still caught by the page's polls, which reload to
sign in (BUG-001), not by the terminal's own handshake.

## What changed (ADR-079)
- `web/src/lib/termLiveness.ts` (new): a per-socket liveness check — a ping on
  open, a probe with a 5 s deadline, dead declared once, and never on a
  connection that has not answered a ping (a server without the pong reply
  behaves as before).
- `web/src/components/TerminalPane.tsx`, project terminals only: probes when
  the tab is shown, when the network returns, and every 25 s while visible; an
  unanswered probe drops the socket and reconnects in place, without boot
  intent. The same two wake events reconnect a "Disconnected" pane. A replaced
  pane has a "Use here" button and takes the terminal back by it, or when its
  tab is shown again. Setup panes, and exited/stopped/revoked, are unchanged.
- The bridge's half (`classifyControl`, the pong, a ping not being activity)
  landed in Phase 1 (`11e4f9a`).
- `web/dist/client` rebuilt: 14 hashed asset files replaced by 14 new ones and
  `_shell.html` — the build's own renames, no other deletion.
- `AGENTS.md`, `internal/terminal/bridge.go`: the two remaining sentences that
  said a replaced terminal never reconnects.

## Not fixed, by decision
- The setup pane: it must never reconnect once opened.
- The browser pane has the dead-socket cause too. The operator approved the
  terminal fix; this is a question for them in the handoff.

## Agents dispatched
- `explorer` (the independent diagnosis above), `documentation`
  (`specs/styrman/terminal-protocol.md`, `specs/styrman/guide.md`,
  `testing/styrman/README.md`), `bugs` (BUG-085).
- `verifier`: `testing/2026-10-02-1806-verify-tool-updates-freeze-harness-v2-phase-2.md`, `## Status: PASS`, over the working tree on top of `11e4f9a`.
- Code review and security review of the whole change follow in Phase 3.

## Verification
The verifier's log, every command bare with its exit status, all 0:
- `go build ./...`, `go vet ./...`, `go test ./...` (rerun with `-count=1`: 15 packages ok, none cached), the Caddy ingress module.
- `web`: `npm run check`, `npm run lint`, `npm test` — 16 files, 189 tests (were 15 and 178).
- Tier 2, `integration.sh styrman-runtime:test`: `passed=118 failed=0 skipped=3 aborted=0`.
- Tier 4, `bash web/e2e/run.sh` (with the web rebuild): `passed=96 failed=0 skipped=2`, the two SKIPs the host-firewall previews; the three new checks PASS; the rebuild reproduced `web/dist/client` byte for byte (`git status` identical before and after).
- Go test names 334, unchanged. No suppression or skip added in the diff (grep by the main agent).
- Not run: `vulncheck.sh`, `verify-commit.sh` (the release gate's).

Red before green: the browser suite with the three new checks against the page
before the fix printed `passed=93 failed=3 skipped=2` — the "Use here" click
timing out, and the two recoveries never arriving. A first red run had failed
the same three for the wrong reason (`Please use browser.newContext()`):
Playwright refuses `context.newPage()` on the implicit context of
`browser.newPage()`. The suite now creates an explicit context and the red run
was repeated. That `failed=3` run is also the deliberate-failure run the e2e
rule asks for: every later check passed.

Mutations, each restored from a saved copy: four guards of `termLiveness.ts`
removed one at a time (the answered-a-ping guard, the pending-probe guard,
`received()` clearing the deadline, `stop()`), each failing exactly one of its
11 tests.

Not covered by any tier: a real sleep, a real network change, a real hidden
tab — the e2e emulates all three at the page's own APIs; the 25 s interval and
the `online` trigger in a browser; the idle reaper actually stopping a project
under a pinging tab.

## Learnings for the local host
None: nothing in this phase touches the host path.
