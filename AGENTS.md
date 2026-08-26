# AGENTS.md

## Project Overview

**styrman** — a self-hosted web platform for running AI coding agents in isolated projects.
Log in with Google, create a project, pick an agent (`claude` or `agy`), and get a browser
terminal running that agent inside a dedicated Kata microVM. One project = one microVM =
one Docker daemon. A single persistent home directory is shared into every microVM so
credentials (`~/.ssh`, `~/.claude`, `~/.gemini`, gcloud, gh) are configured once, ever.

Successor to `~/workspace/devbox` (multi-tenant Kata shell platform) and
`~/workspace/argolis-devbox` (single-VM cloud shell). Terminal-first: the agent CLI *is*
the chat window. No embedded IDE.

## Setup Commands

```bash
# Build server
go build ./cmd/styrman/

# Test
go test ./...

# Lint
go vet ./...

# Build runtime image (AMD64, local only)
docker build -t styrman-runtime:dev image/

# Run locally (Phase 1: plain HTTP, no auth)
go run ./cmd/styrman/
```

## Architecture

- Single Go binary (`cmd/styrman`): embedded web UI + REST/WS API + Docker orchestrator.
- Per-project Kata microVMs (Docker runtime `mvm`) created via the Docker SDK; state
  recovered from container labels (`styrman.*`) + `projects.json` — no database.
- Terminal transport: Docker exec (TTY) → tmux → WebSocket. Protocol ported from devbox
  (binary frames = PTY bytes, JSON text = resize, close 4000 = exited, 4001 = replaced).
- Shared home at `/var/lib/styrman/home`, virtiofs-mounted as `/home/<user>` in every
  microVM; project workspaces at `~/projects/<name>`; per-project dockerd data-root at
  `~/.dockerd/<name>`.
- Auth (Phase 3): Caddy + caddy-security Google OAuth (reused `devbox-caddy` image),
  identity via `X-Caddy-Email` header + userstore re-check.
- Full rationale: `.agents/docs/architecture/DECISIONS.md` and
  `.agents/docs/plans/2026-08-26-super-devbox-architecture.md`.

## Project-Specific Standards

- Frontend (`web/`): TanStack Start SPA, static export embedded via `go:embed`. No SSR.
- Reuse devbox code by porting files into `internal/` with attribution comments, not by
  importing the devbox module.
- The runtime image is AMD64-only (single NUC target).

## Testing

- Local-first on this machine (Kata runtime `mvm` required — already configured).
- Integration scripts + guides in `.agents/docs/testing/styrman/`.
- Spike results (Phase 0) in `.agents/docs/testing/spikes/`.

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/styrman/main.go` | Server entry point |
| `internal/orchestrator/` | Kata microVM lifecycle via Docker SDK |
| `internal/terminal/` | exec↔WebSocket terminal bridge |
| `internal/project/` | Project store (`projects.json`), git init |
| `image/` | Runtime image: Dockerfile, entrypoint, home configs |
| `web/` | TanStack Start UI |
| `REQUIREMENTS.md` | Non-negotiable requirements |
| `.agents/docs/plans/` | Architecture + implementation plans |
