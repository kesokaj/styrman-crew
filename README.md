# styrman

Self-hosted web platform for running AI coding agents in isolated projects.

Log in, hit **New Project**, pick an agent — [Claude Code](https://claude.ai/code)
(`claude`) or [Antigravity CLI](https://antigravity.google/product/antigravity-cli)
(`agy`) — and styrman gives you a browser terminal running that agent inside a dedicated
**Kata Containers microVM**, in its own workspace, with its own Docker daemon. Jump
between projects and conversations like a chat app; under the hood every conversation is
a tmux session you can always reattach to.

*Styrman* (Swedish): helmsman / first mate.

## Why

- **The agent CLI is the chat window.** No custom chat UI to maintain — the terminal is
  the interface, just organized like a conversation app (projects → conversations).
- **Hard isolation.** 1 project = 1 microVM (own kernel, own dockerd). An agent running
  wild in one project cannot touch another.
- **Set up credentials once, ever.** A single persistent home directory is shared into
  every microVM: `~/.ssh`, `~/.claude`, `~/.gemini`, gcloud, gh all survive container
  rebuilds, project deletion, and host reboots.

## Architecture

```
Browser ──HTTPS──▶ Caddy (Google OAuth)          [Phase 3]
                      │  X-Caddy-Email
                      ▼
              styrman server (Go, one binary)
                ├── embedded web UI (TanStack Start)
                ├── REST /api/projects, /api/.../conversations
                ├── WS   /ws/term/{project}/{conversation}
                └── Docker SDK ──▶ per-project Kata microVM (runtime: mvm)
                                     ├── tmux session per conversation → claude / agy
                                     ├── own dockerd (data-root ~/.dockerd/<project>)
                                     └── /home/<user> ← shared persistent (virtiofs)
```

No database: state lives in Docker labels, the filesystem, and one `projects.json`.

## Status

Pre-alpha — Phase 0 (scaffold + de-risking spikes). See
`.agents/docs/plans/2026-08-26-super-devbox-implementation-plan.md`.

## Requirements

- Linux host with Docker and Kata Containers registered as runtime `mvm`, `/dev/kvm`
- Go 1.24+, Node 22+ (UI build)

## Quick start (Phase 1, localhost)

```bash
go run ./cmd/styrman/        # then open http://localhost:8080
```

## Lineage

Combines and supersedes two earlier projects: `devbox` (multi-tenant Kata shell platform
with Caddy OAuth ingress) and `argolis-devbox` (single-VM cloud shell on GCP). Decision
history: `.agents/docs/architecture/DECISIONS.md`.
