# Requirements

<!-- Non-negotiable project requirements. Read before every task. -->
<!-- Never modify without explicit user approval. -->

## Scale Targets

| Metric | Target | Notes |
|---|---|---|
| Concurrent running projects (microVMs) | 10+ | On the NUC12 (16 threads / 62 GB); KSM enabled |
| Users | 1 primary, ≤ 5 total | Single-operator system; userstore supports more |
| Projects total (incl. stopped) | 100+ | Stopped projects cost only disk |

## Latency SLA

| Metric | Target | Hard Limit |
|---|---|---|
| New project → agent prompt visible | < 5 s | < 15 s |
| Reopen stopped project | < 5 s | < 15 s |
| Terminal keystroke echo (LAN) | < 50 ms | < 150 ms |

## Critical Requirements

1. **R1 — One project = one Kata microVM** (Docker runtime `mvm`). Never plain runc for
   project workloads; never a shared host `docker.sock` inside a project.
2. **R2 — Per-project Docker-in-Docker**: each microVM runs its own `dockerd` with a
   project-scoped data-root. Projects must not share Docker state.
3. **R3 — One persistent shared home**: `/var/lib/styrman/home` mounted as `$HOME` in
   every microVM. Sign in to claude / agy / gh / gcloud / SSH **once ever**; destroying
   and recreating any container must lose nothing outside that container's runtime state.
4. **R4 — Terminal-first, no embedded IDE**: the agent CLI is the primary interface.
   No code-server / VS Code embed. A lightweight file viewer is the ceiling.
5. **R5 — Agents**: `claude` (Claude Code) and `agy` (Antigravity CLI), selected per
   project at creation. Harness deployed from `devshell_prompt/harness` into the shared
   home for both vendors.
6. **R6 — Conversations are tmux sessions**: survive browser disconnects; single active
   WebSocket per conversation (close-code 4001 replaces).
7. **R7 — No database**: state = Docker labels + filesystem + `projects.json`.
   A server restart or host reboot recovers everything by adoption.
8. **R8 — Auth via Google OAuth at the ingress** (reused devbox-caddy), application-level
   authorization re-checked against the userstore. No unauthenticated exposure.
9. **R9 — Local-only deployment** on this machine. No cloud deploy of styrman itself.
10. **R10 — Secrets**: single gitignored `.env`; per-user secrets in `~/.bash_secrets`
    (never overwritten by provisioning). No secrets in git, images, or labels.

## Constraints

- Go stdlib-first server; approved deps: `coder/websocket`, `creack/pty`, `docker/docker`
  SDK. Every addition must be justified.
- Frontend: TanStack Start + TypeScript + Tailwind, fully contained in `web/`.
- Runtime image: Debian, AMD64 only.
- Host: Docker with Kata `mvm` runtime (already configured); no Kubernetes.

## Out of Scope

- Multi-host / clustering.
- Embedded IDE (code-server, VS Code web).
- Custom chat UI or agent API integration — the CLI in a terminal is the interface.
- Public multi-tenant SaaS hardening (rate limits, billing, abuse).
