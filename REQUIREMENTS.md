# Requirements

<!-- Non-negotiable project requirements. Read before every task. -->
<!-- Never modify without explicit user approval. -->

## Scale Targets

| Metric | Target | Notes |
|---|---|---|
| Concurrent running projects (microVMs) | 10+ | On the NUC12 (16 threads / 62 GB); KSM enabled |
| Users | 1 admin, ≤ 5 total | Admin (env-provided) manages access; each user fully isolated |
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
3. **R3 — Per-project home; credentials shared via the broker** (ADR-030/031/032,
   revised 2026-08-28; was "one shared home", ADR-003). Each project gets its OWN
   home at `HomesDir/<user>/p/<uuid>`, seeded from the base image (ADR-031), so
   projects are truly individual — no tool state (`~/go`, `~/.npm`, caches, global
   installs, history) leaks between them. The ONLY thing shared across a user's
   projects is credentials, delivered by the credential broker (ADR-032): git via
   per-op credential helper (nothing stored in the VM); gh/npm/claude via env
   tokens; gcloud ADC + the ssh key via file injection; agy via file injection
   of its OAuth token (`~/.gemini/antigravity-cli/antigravity-oauth-token`;
   ADR-032a amended 2026-08-29 — the earlier keyring tier is dormant) — all
   pulled current on every login shell. docker→registry creds: planned (no adapter yet). Sign in
   to each brokered tool **once**; a refresh in any project propagates to all;
   destroying and recreating any container must lose nothing outside that
   container's runtime state.
4. **R4 — Terminal-first, no embedded IDE**: the agent CLI is the primary interface.
   No code-server / VS Code embed. A lightweight file viewer is the ceiling.
5. **R5 — Agents**: `claude` (Claude Code) and `agy` (Antigravity CLI), selected per
   project at creation. The agent harness (vendored from `devshell_prompt/harness`) and
   the default shell config are carried by the **base image** and seeded identically into
   every project home (ADR-030, revised 2026-08-28; was "deployed into the shared home").
6. **R6 — Terminals are durable records over tmux sessions** (ADR-016):
   survive browser disconnects; single active WebSocket per terminal
   (close-code 4001 replaces); pause keeps record + transcript.
7. **R7 — No database**: state = Docker labels + filesystem + per-user JSON
   stores (`users/<u>/{projects,conversations,profile}.json`). A server
   restart or host reboot recovers everything by adoption. Project identity
   is a UUID (ADR-020); friendly names are labels.
8. **R8 — Auth via Google OAuth at the ingress** (reused devbox-caddy), application-level
   authorization re-checked against the userstore. No unauthenticated exposure.
9. **R9 — Local-only deployment** on this machine. No cloud deploy of styrman itself.
10. **R10 — Secrets**: single gitignored `.env`; per-user secrets in `~/.bash_secrets`
    (never overwritten by provisioning). No secrets in git, images, or labels.
11. **R11 — Reproducible installation** (operator directive 2026-08-26): the whole
    system must install from this repo onto a fresh Debian 13 host — another baremetal
    machine first, or a GCP VM with nested virtualization — via `deploy/host-setup.sh`
    (Docker, Kata `mvm`, daemon config incl. the time-namespaces fix, tuning, systemd).
    No dependence on leftover devbox host state. See ADR-010.
12. **R12 — Project network isolation** (operator directive 2026-08-26):
    project microVMs MUST NOT be able to reach one another over the network.
    Each project gets its own bridge (`sdb-<uuid>`) shared with nothing but
    the styrman server; the server originates connections for the port-preview
    proxy but has `ip_forward=0` so it can never route between projects. Host
    networking is for the Caddy ingress ONLY, never for projects. Verified
    adversarially (a project with NET_ADMIN adding a manual route through the
    server stays blocked). Enforced by naming the bridge interface `sdb-<hash>`
    (so the firewall matches), DOCKER-USER egress drops to RFC1918 + metadata,
    and a self-healing firewall timer (ADR-036). See ADR-015.

13. **R13 — Git is the substrate** (operator directive 2026-08-27): every
    project workspace IS a git repository, without exception — initialized
    (or cloned) at creation, re-initialized by a boot-time sweep if ever
    missing. Git powers the diff view and the git side panel — a real in-guest
    client (status / stage / unstage / discard / commit / push, correct author
    identity + broker push auth). Automatic checkpoints under
    `refs/styrman/checkpoints/*` were REMOVED 2026-08-29 (ADR-035); durable
    backups are now microVM VOLUME snapshots (host infra, planned).

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
