---
owner: main
date: 2026-09-11
---
# Verification — Browser view, Phase 2 (server: verbs, REST, relay, port refusals) + Phase 1 review fixes   (2026-09-11)

Written by main; the `verifier` agent is dispatched over the committed tree
right after this phase's commit (its log will sit beside this one).

## Commands and results

| Step | Command | Result |
|---|---|---|
| Vet + unit | `go vet ./... && go test -race -count=1 ./...` | green, every package (`.agents/scratch/go-test-phase2b.log`) |
| Ingress module | `(cd deploy/caddy/ingress && go test -count=1 ./...)` | green |
| New unit tests | `go test -race -run 'Browser\|CloseReason\|ParseListeningPorts' ./internal/httpapi/ ./internal/orchestrator/` | 7 tests: status/start/stop routes, 503 on a fresh attach (REST + WS, no exec attempted), the 400 refusals on five routes for 9222 and 9229, the relay pumping both ways with an Origin-refusing fake DevTools server, close codes 4002/4004/1011, `closeReason` rune-safe cut, `parseListeningPorts` hiding both browser ports |
| Lint | `shellcheck image/configs/styrman-browser`; `shellcheck -S warning image/entrypoint.sh` | clean |
| Build | `docker build -t styrman-runtime:browser-p1 … image/` | green; `/styrman/npm-global.tree.json` records the resolved npm tree (chrome-devtools-mcp 1.9.0) |
| Kata probe (peer restriction) | `docker run --runtime mvm` of the built image, launcher as an unprivileged user | env unset → socat admits the bridge gateway: host `curl http://<ip>:9222/json/version` 200; `STYRMAN_CRED_BROKER=http://10.99.99.99:8091` → host refused (000); in-guest `127.0.0.1:9229` answers either way; `styrman-browser mcp` answers an MCP `initialize` over stdio as `chrome_devtools` 1.9.0 |
| Kata probe (origins) | same, with the launcher's `--remote-allow-origins=*` removed | WebSocket upgrade with no Origin → 101; `Origin: http://evil.example` → 403; `Origin: http://localhost:9222` → 403 |
| Tier 2 | `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` (image rebuilt by `ensure-test-image.sh`) | `passed=90 failed=0 skipped=3` (README: 93 checks, 3 auto-SKIP on this firewall-hardened host); all six browser checks PASS; boot→ready 2.72 s |

PASS. Full Tier 2 output: `.agents/scratch/integration-browser-p2-final.txt`
(scratch, not committed). A first run before the peer-restriction change
also passed 90/0/3 (`integration-browser-p2.txt`).

## Not verified here

- The live relay end to end (SPA ⇄ server ⇄ guest DevTools) — the server on
  this host cannot reach a sandbox bridge; Phase 4's e2e drives it inside the
  LAN window.
- The SPA (Phase 3) and the operator's manual agent walk (Phase 4).
