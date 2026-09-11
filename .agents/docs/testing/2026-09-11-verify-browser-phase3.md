---
owner: main
date: 2026-09-11
---
# Verification — Browser view, Phase 3 (SPA) + Phase 2 security round 2   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after
this phase's commit.

## Automated

| Step | Command | Result |
|---|---|---|
| Frontend | `cd web && npm run check && npm run lint && npm test` | tsc clean, oxlint clean, 147 → 171 vitest tests green across 13 files (cdp 6, browserInput 11, editorPane +3, api.encoding +4) |
| Go | `go vet ./... && go test -race -count=1 ./...`; ingress module | green (`.agents/scratch/all-tests-phase3.log`); new: `TestBrowserPortLinkAndShareRecordsDoNotServe`, `TestBrowserRelayFollowsNoRedirect`, `TestBrowserRelayOriginAndCap`, `TestBrowserStartPassesPeer` |
| Launcher | `shellcheck`; Kata probe of the rebuilt image | no peer env → 9222 not listening, loopback 9229 answers; `STYRMAN_BROWSER_PEER=<gateway>` → forwarder up, host admitted (200); `STYRMAN_BROWSER_PEER=1.2.3.4,range=0.0.0.0/0` → refused, no forwarder; `stop` takes both groups down |
| Tier 2 | `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` | `passed=90 failed=0 skipped=3 aborted=0` (README: 93 checks, 3 auto-SKIP on this firewall-hardened host); the two server-side browser checks corrected first for the peer design — a bare-exec'd start is loopback-only, so the ports-list check watches 9229 and the routes check asserts the 9222 forwarder after the server's start. An earlier run was invalid: it shared port 18080 with the hand-walk's local server (`integration.sh` hard-codes that port — follow-up) |

## Hand-walk (Playwright against the built SPA; real Kata project in LAN mode)

Setup: `go build` of the tree, server on `127.0.0.1:18080` with a scratch
state dir and `STYRMAN_IMAGE=styrman-runtime:browser-p1`; a project toggled
to LAN (a host-run server cannot reach a sandbox bridge on this hardened
host); a fixture `python3 -m http.server 4100` inside the guest serving
`index.html` (title "Fixture app", a full-page link to `/two.html`).
Script and screenshots: `.agents/scratch/p3walk/` (scratch, not committed).

| Step | Observed |
|---|---|
| Open `/p/<id>/<term>`, click Browser | first screencast frame within ~2 s (`data-frames` ≥ 1) |
| Type `localhost:4100`, Enter | URL bar `http://localhost:4100/`; tab strip "Fixture app"; page rendered (screenshot 02) |
| New tab | strip shows 2 tabs; switch back |
| Click inside the page | URL bar `http://localhost:4100/two.html`; "two" rendered (screenshot 03) |
| Page back | URL bar `http://localhost:4100/` |
| Ctrl+L on the canvas | address bar focused |
| Terminal pill | terminal still mounted (session survived) |
| Ports tab → "browser" | Browser view pressed, URL bar `http://localhost:4100/` (screenshot 04) |
| Stop the project (API) while the view is open | 48 s; pane "Browser disconnected" (relay 1011) |
| Reconnect on the stopped project | "Project is stopped." + Show terminal + Reconnect (phases: connecting → stopped, stable for 6 s; two runs) |
| Start (API), Reconnect | ready, fresh tab |
| Client errors | one `HTTP 500 /api/projects/<id>/terminals` during the 48 s stop window — the terminals listing, pre-existing, not this feature (noted for the changelog) |

## Not verified here

- The agent path (`claude`/`agy` driving the browser) — the operator's manual walk in Phase 4.
- Sandbox-mode projects from a host-run server: unreachable on this host by design (Phase 4's e2e uses the LAN window).
