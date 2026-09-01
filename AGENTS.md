# AGENTS.md

## Project Overview

**styrman-crew** (repo name; the platform itself is *styrman*) — a self-hosted, multi-user web platform for running AI coding
agents in isolated projects. Sign in with Google, create a project (UUID
identity, friendly renamable name), and get browser terminals inside a
dedicated Kata microVM with both `claude` and `agy` preinstalled. One
project = one microVM = one Docker daemon = one home (truly individual,
ADR-031); credentials are the one shared thing, kept current by the
credential broker (ADR-030/032) — sign in once, every project stays signed
in. Users fully isolated from each other.
Live at https://crew.styrman.xyz.

Successor to `~/workspace/devbox` (multi-tenant Kata shell platform) and
`~/workspace/argolis-devbox` (single-VM cloud shell). Terminal-first: the agent CLI *is*
the chat window. No embedded IDE.

## Setup Commands

```bash
# Build server
go build ./cmd/styrman/

# Test
go test ./...
# The vendored Caddy ingress is a SEPARATE Go module — the root `go test ./...`
# does NOT cover it; run its (security-critical) tests explicitly:
(cd deploy/caddy/ingress && go test ./...)

# Lint
go vet ./...

# Build runtime image (AMD64, local only)
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/  # the version keys harness re-seeding

# Run locally (loopback, no auth) / deploy production
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/
STYRMAN_VERSION=<x.y.z> docker compose -f deploy/docker-compose.yml up -d --build
# Compose refuses to run with STYRMAN_VERSION unset (no mutable tags); the
# runtime image must be built separately at the SAME version first — full
# release/rollback procedure + the three-image lineup:
# .agents/docs/specs/styrman/versioning.md

# Test suites
bash .agents/docs/testing/styrman/integration.sh   # 60 checks; counts live in .agents/docs/testing/styrman/README.md vs real Kata
bash web/e2e/run.sh                                # browser e2e (Playwright, self-hosted; check counts: .agents/docs/testing/styrman/README.md)
```

## Architecture

- Single Go binary (`cmd/styrman`): embedded TanStack Start SPA + REST/WS
  API + Docker orchestrator. Deployed containerized (compose) behind the
  vendored Caddy ingress, which injects a shared `X-Styrman-Ingress` token
  the server requires alongside `X-Caddy-Email`; production runs no
  auth-bypass listener (the opt-in loopback-only dev listener is for local
  testing only and is unset in the compose deploy).
- Per-project Kata microVMs (runtime `mvm`), containers `styrman-<uuid>`;
  state from labels + per-user JSON stores (`/var/lib/styrman/users/<u>/`)
  — no database. Project creation is async (instant 201, background boot).
- Multi-tenant: identity from the ingress header → username slug; each
  PROJECT mounts its own home at `/var/lib/styrman/homes/<u>/p/<uuid>`
  (ADR-031; `homes/<u>` is just the base/setup-VM home). Configurable
  in-guest "terminal username". Admins = env-provided emails.
- Per-project workspace storage (ADR-027): each working tree lives at
  `/var/lib/styrman/projects/<u>/<uuid>`, bind-mounted into the microVM at
  `~/projects/<dir>`; the project home holds everything else.
- Credential broker (ADR-030/032): a bridge-only listener (`:8091`) serves
  the CURRENT git/env/file credentials to each project (caller = bridge IP);
  a login-shell snippet materializes them. Setup completion auto-harvests them
  into the broker, so every brokered tool (git/gh/ssh/claude/agy/gcloud) is
  signed in on every project — no per-project sign-in. Manage/add via Settings →
  Credentials (status-colored: green synced / grey not-set / red set-not-synced).
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
- Browser e2e: `bash web/e2e/run.sh` (builds the web SPA first — the server
  embeds `web/dist/client` via `go:embed`, so a stale `dist` would silently
  test the wrong assets; skip with `STYRMAN_E2E_SKIP_WEB=1` for server-only
  iteration — then self-hosts its own server on host loopback with an
  isolated state dir, then runs `web/e2e/run.mjs`'s Playwright checks against
  it — production has no auth-bypass listener to test against instead;
  prereq `npx playwright install chromium`; check counts:
  `.agents/docs/testing/styrman/README.md`, the source of truth — currently
  51 passed + 2 auto-SKIP).
- Spike results (Phase 0) in `.agents/docs/testing/spikes/`.
- Any privileged (root) test must point state/target dirs at a scratch
  prefix, never a default that aliases `/var/lib/styrman`/`/etc/styrman` —
  ADR-052; convention detailed in
  `.agents/docs/testing/styrman/README.md`.

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/styrman/main.go` | Server entry point + legacy-layout migration |
| `internal/identity/` | email → username slug, request identity |
| `internal/orchestrator/` | Kata microVM lifecycle, ports, legacy cleanup |
| `internal/terminal/` | exec↔WebSocket terminal bridge |
| `internal/project/` | Per-user project store (UUID identity) |
| `internal/conversation/` | Terminal records + tmux session manager |
| `internal/files/`, `internal/history/` | File explorer, transcripts (full-text search) |
| `internal/gitrepo/` | Hardened bootstrap-only host-side git (`init`/`rev-parse`); the interactive Git client itself runs in-guest (`internal/httpapi/git.go`) |
| `internal/httpapi/` | REST/WS routing, auth guard, settings, proxy |
| `internal/userstore/` | Allowlist + admin flags |
| `internal/links/` | Public/private preview-link store |
| `internal/credstore/`, `internal/credbroker/` | Credential broker: store + bridge-only HTTP (ADR-030/032) |
| `internal/config/` | Env config, paths, operator-tunable settings |
| `image/` | Runtime microVM image: Dockerfile, entrypoint, styrman-setup |
| `deploy/` | host-setup, compose, vendored caddy ingress, server Dockerfile |
| `web/` | TanStack Start SPA (dist/client committed + embedded) |
| `REQUIREMENTS.md` | Non-negotiable requirements (R1–R13) |
| `.agents/docs/specs/styrman/guide.md` | The living full guide |
