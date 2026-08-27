# Testing styrman

Local-first: everything runs on this machine against real Docker + Kata.
No mocks in the orchestrator path.

## Tiers

1. **Unit** — `go test ./...` (store, validation; fast, no Docker).
2. **Integration (56 checks)** — `bash .agents/docs/testing/styrman/integration.sh [image]`
   (default `styrman-runtime:dev`). Isolated server (own state dir, port
   18080) through the full lifecycle: legacy-layout migration, async create
   → microVM boot, terminals (records, rename, pause, close-all),
   WS I/O + reattach, transcripts, file explorer (incl. traversal guards),
   ports + all preview URL shapes, git diff + checkpoints (R13), settings,
   persistence across container destruction, UUID identity + friendly
   rename, wipe. Safe next to a real install.
3. **Manual / UX** — things scripts can't judge:
   - `go run ./cmd/styrman/` → http://127.0.0.1:8080/dev/ in a browser
   - create a project each for claude and agy; complete each agent's first-run
     sign-in in the terminal (one time ever, thanks to the shared home)
   - reload the tab mid-session → same scrollback; open a second tab → first
     shows "replaced" (4001) and does NOT reconnect
   - type `exit` in a shell conversation → "session exited" (4000)
   - resize the window → TUI reflows (tmux + ContainerExecResize)

## Prereqs

- `styrman-runtime` image built: `docker build -t styrman-runtime:dev image/`
- Kata runtime `mvm` working: `docker run --runtime mvm --rm debian:trixie uname -r`
  (host-setup.sh verifies this; if it fails with "invalid namespace type" see
  ADR-009 — the daemon.json time-namespaces flag is missing)

## Rebuild-and-retest loop

```bash
go test ./... && bash .agents/docs/testing/styrman/integration.sh
```

Image changes: rebuild the image first; the integration script always creates
fresh containers so it picks the new image up automatically.

## Tier 4 — Browser e2e (Playwright, 33 checks)

`bash web/e2e/run.sh [image-tag]` — self-hosts: builds the server and starts
its own instance on a host loopback port with an isolated state dir, then
runs the Playwright checks against it. Production has no auth-bypass dev
door to test against anymore (2026-08-27 security review; see
`../../specs/styrman/deploy.md`), and a host-loopback server is unreachable
from project microVMs either way.
Drives the real UI in headless Chromium against that server: project
creation (boots a microVM), agent + shell terminals (xterm's DOM renderer
makes terminal text assertable, plus the link-rescue bar and copy-on-select),
typing into the microVM, the Files/Ports/History/Git/Clips side-panel tabs,
rename/delete, deep links, setup page, theme, and full cleanup — 33 checks +
screenshots.
Prereq: `cd web && npm i && npx playwright install chromium` (once).
Self-cleaning: deletes any leftover `e2e-ui` project at start.
To point the Playwright checks at an already-running server instead, call
`node web/e2e/run.mjs [--base http://127.0.0.1:8080] [--shots /tmp/styrman-shots]`
directly.
