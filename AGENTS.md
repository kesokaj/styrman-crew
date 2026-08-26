# AGENTS.md

## Project Overview

**styrman** — a self-hosted, multi-user web platform for running AI coding
agents in isolated projects. Sign in with Google, create a project (UUID
identity, friendly renamable name), and get browser terminals inside a
dedicated Kata microVM with both `claude` and `agy` preinstalled. One
project = one microVM = one Docker daemon; one user = one persistent home
disk (credentials once, forever; users fully isolated from each other).
Live at https://app.styrman.xyz.

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

# Run locally (loopback, no auth) / deploy production
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/
docker compose -f deploy/docker-compose.yml up -d --build

# Test suites
bash .agents/docs/testing/styrman/integration.sh   # 49 checks vs real Kata
node web/e2e/run.mjs                               # 29 Playwright checks
```

## Architecture

- Single Go binary (`cmd/styrman`): embedded TanStack Start SPA + REST/WS
  API + Docker orchestrator. Deployed containerized (compose) behind the
  vendored Caddy ingress; a loopback-only dev listener bypasses auth.
- Per-project Kata microVMs (runtime `mvm`), containers `styrman-<uuid>`;
  state from labels + per-user JSON stores (`/var/lib/styrman/users/<u>/`)
  — no database. Project creation is async (instant 201, background boot).
- Multi-tenant: identity from the ingress header → username slug → own home
  at `/var/lib/styrman/homes/<u>`; configurable in-guest "terminal
  username". Admins = env-provided emails.
- Terminals: durable records over tmux sessions; transcripts piped to
  `~/.styrman/logs/<projectId>/`. WS protocol: binary = PTY bytes, JSON
  text = resize, close 4000 = exited, 4001 = replaced.
- Per-project dockerd: overlay2 on loop-mounted `~/.dockerd/<uuid>.img`.
- Project networks are isolated from each other (R12); the server joins
  them in the background for the port-preview proxy (ip_forward=0).
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
- Browser e2e: `node web/e2e/run.mjs` (Playwright; prereq `npx playwright install chromium`).
- Spike results (Phase 0) in `.agents/docs/testing/spikes/`.

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/styrman/main.go` | Server entry point + legacy-layout migration |
| `internal/identity/` | email → username slug, request identity |
| `internal/orchestrator/` | Kata microVM lifecycle, ports, legacy cleanup |
| `internal/terminal/` | exec↔WebSocket terminal bridge |
| `internal/project/` | Per-user project store (UUID identity) |
| `internal/conversation/` | Terminal records + tmux session manager |
| `internal/files/`, `internal/history/` | File explorer, transcripts + git diff |
| `internal/httpapi/` | REST/WS routing, auth guard, settings, proxy |
| `internal/userstore/` | Allowlist + admin flags |
| `image/` | Runtime microVM image: Dockerfile, entrypoint, styrman-setup |
| `deploy/` | host-setup, compose, vendored caddy ingress, server Dockerfile |
| `web/` | TanStack Start SPA (dist/client committed + embedded) |
| `REQUIREMENTS.md` | Non-negotiable requirements (R1–R12) |
| `.agents/docs/specs/styrman/guide.md` | The living full guide |
