---
owner: main
date: 2026-09-11
---
# Verification — Browser view, Phase 4 (e2e, docs, security round 3)   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after
this phase's commit.

## Automated

| Step | Command | Result |
|---|---|---|
| Static + unit | `go vet ./... && go test -race -count=1 ./...`; ingress module; `shellcheck` (launcher, entrypoint); `cd web && npm run check && npm run lint && npx vitest run` | all green; 172 frontend tests (+1: `normalizeUrl` refuses every scheme but http/https/about:blank); new Go tests: `TestBrowserPeer` (4 cases incl. fail-closed), per-user relay cap in `TestBrowserRelayOriginAndCap` (`.agents/scratch/all-tests-phase4.log`) |
| Image + Kata | `docker build` of `image/`; `docker run --runtime mvm`: `start` with `STYRMAN_BROWSER_PEER=<gateway>` → 9222 up, host `curl /json/version` 200; `stop` → ports closed | PASS; the peer validator refuses a multi-line and an option-injecting value (whole-string bash regex); a changed peer replaces the forwarder (`socat.range`) and the host is admitted again |
| Tier 4 e2e | `bash web/e2e/run.sh` (solo, host quiet) | `passed=91 failed=0 skipped=2`, "no uncaught client errors" PASS. The four Browser-view checks, inside the LAN window: "Browser pill opens the browser view and a frame arrives", "typing a localhost URL renders the fixture app" (URL bar + tab title "Browser fixture" + a new frame; screenshot `/tmp/styrman-shots/browser-fixture.png`), "Open in Browser from the Ports panel switches the view and navigates", "terminal session survived the browser round-trip" |

E2E history the same day, for the record: run 1 (concurrent with the
verifier's integration run and docker build) — 4 failures: a pre-existing
focus-after-toggle check (load blip, passed solo twice since — BUG-038's
class), the two fixture-dependent browser checks, and the client-error gate
(`ERR_NETWORK_CHANGED` on the LAN PUT plus a 500 from `/terminals` during
the rebuild window → BUG-039, fixed); run 2 (solo) — the two fixture checks
only: the typed command carried `<!doctype`, and `!` is a history expansion
in the interactive shell ("event not found"), so the fixture never started;
run 3 (solo, `!` removed, prompt awaited after the rebuild) — green.

## Security round 3 (`reviews/2026-09-11-security-browser-phase3.md`, 2 medium / 6 low / 3 info)

Fixed in this phase: per-user relay cap (8) beside the global one (32) plus
a 30 s heartbeat that frees an abandoned pane's slot (medium 1); URL bar
limited to http/https/about:blank with a notice (low); CDP frame/dialog/
target payload shapes checked, dialog text cut to 200 chars (low);
`BrowserPeer` fails closed instead of naming the broker's intended address
(low); the peer validator matches the whole string (info); the forwarder
follows a changed peer (low, already fixed for the Phase 3 reviewer's
medium). Accepted, documented: a container run with `--network=host`
inside the project reaches Chromium's loopback listener (medium 2 —
runtime-image.md "Known boundary", launcher header, plan follow-up: pipe
transport); scheme-blind Origin compare (the ingress terminates TLS, so
the server never sees the scheme); host-mode gateway peer (dev only);
stale link/share records still shown in the owner's UI while 404ing.

## Not verified here

- The agent path end to end (`claude`/`agy` driving the browser through the
  `styrman-browser` MCP server): the operator's manual walk, below.
- Production (the containerized server): the relay's peer is the server
  container's live address on the bridge, exercised only by unit tests and
  the Kata probes here, not by a compose deploy — the release's `/preflight`
  smoke covers it.

## Manual walk for the operator (Phase 4's last box)

1. Open a running project, click **Browser**, type `:3000` (or any port a
   dev server of yours listens on) — the page renders inside the pane.
2. In the project's terminal run `claude` (or `agy`) and ask it: "Open
   http://localhost:3000 in the browser and tell me what the page says."
   The agent's tab appears in the pane's tab strip; its navigation is
   visible as it happens.
3. Ask the agent to take a screenshot or click something — the pane follows.
4. Record the outcome in this file's follow-up (or a new `testing/` log) and
   tick the plan's "Manual agent walk" box.

## Security round 4 (`reviews/2026-09-11-security-browser-phase4.md`, 0 high / 0 medium / 8 low / 2 info) — 2026-09-11, after the Phase 4 commit

Fixed (commit "browser: security round 4"): the fail-closed peer now reaches
the launcher — the server always SETS `STYRMAN_BROWSER_PEER` on its starts
(empty when unknown) and the launcher reads set-but-empty as "no forwarder",
falling back to the broker's host only for an UNSET variable (an
agent-started browser); test `TestBrowserStartPassesPeer` covers the empty
case and the launcher's `peer_range` was exercised with both shapes. The
relay heartbeat pings both sides with a timeout of half the interval (15 s),
so a half-dead guest socket also frees its slot and a pong queued behind a
busy screencast is not mistaken for a dead pane. Accepted for now, with
reasons: the `listTerminals` degradation has no expiry or client marker (a
lasting scan failure reads as "all paused" — the WARN line says why; a
marker is a follow-up); `deleteAllTerminals` deleting records despite kill
failures is pre-existing behaviour outside this feature; the remaining lows
and infos are wording and partials already listed as follow-ups in the plan.
