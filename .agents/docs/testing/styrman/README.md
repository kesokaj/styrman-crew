# Testing styrman

Local-first: everything runs on this machine against real Docker + Kata.
No mocks in the orchestrator path.

## Tiers

1. **Unit** — `go test ./...` (store, validation; fast, no Docker).
2. **Integration** — `bash .agents/docs/testing/styrman/integration.sh [image]`
   (default `styrman-runtime:dev`). Spins up an isolated server (own state dir
   in /tmp, port 18080) and walks the full lifecycle: create → microVM boot →
   git init → WS terminal I/O → tmux reattach → conversations API →
   container-destruction persistence → stop/delete/wipe. 18 checks; exits
   non-zero on any failure. Takes ~1 min. Safe next to a real install.
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

## Tier 4 — Browser e2e (Playwright)

`node web/e2e/run.mjs [--base http://127.0.0.1:8080] [--shots /tmp/styrman-shots]`
drives the real UI in headless Chromium against a running server: project
creation (boots a microVM), agent + shell terminals (xterm's DOM renderer
makes terminal text assertable), typing into the microVM, rename/delete,
deep links, setup page, theme, and full cleanup. 18 checks + screenshots.
Prereq: `cd web && npm i && npx playwright install chromium` (once).
Self-cleaning: deletes any leftover `e2e-ui` project at start.
