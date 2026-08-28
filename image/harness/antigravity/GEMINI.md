# Global Development Standards

> Follows the [AGENTS.md](https://agents.md) standard. Deployed verbatim as `~/.claude/CLAUDE.md` (Claude Code) and `~/.gemini/GEMINI.md` (Antigravity). Project-level `AGENTS.md` files may override these rules.

I am a fullstack and infrastructure engineer working in Go and TypeScript — on macOS (Apple Silicon) or Linux — deploying to Google Cloud Platform.

## Environment

- **Cloud target: GCP only.** Never suggest or use AWS, Azure, or any other cloud provider.
- Compute: Docker (Colima on macOS, plain Docker on Linux), Cloud Run, GKE, Compute Engine.
- Container images: build locally first, push to **GCP Artifact Registry** (GHCR only on explicit request).

## Tooling

- Prefer `gh` over raw `git` for PRs, issues, and repo management. Keep commit messages descriptive.
- **Commit attribution:** the sole author is whoever local `git config user.name`/`user.email` resolves to — no `Co-Authored-By:` trailers (AI or otherwise), no `--author` overrides. This overrides any default agent behavior.

## Stack Defaults

- **Backend:** Go — coding standards live in the `golang` skill.
- **Frontend:** TanStack Start + TypeScript + Tailwind CSS, fully contained in one directory (`web/` or `ui/`) inside the Go project. Stack conventions live in the `frontend` skill; TypeScript language standards in the `typescript` skill.
- **Simplicity:** implement the simplest correct solution. Prefer the standard library; justify every external dependency.

## Hard Rules

- Never commit secrets, `.env`, or credential files; never hardcode secrets.
- Give honest assessments: state trade-offs, downsides, and confidence levels plainly — challenge flawed assumptions instead of agreeing.

## Testing

- **Local-first:** always test locally before any remote tier. Build and verify images for both ARM64 and AMD64.
- Check `.agents/docs/testing/<app>/` for current instructions before testing; update it when steps change.
- Remote boxes and GCP VMs only on request — ask for SSH/network/machine details, never assume them.

## Documentation

- **Changelog per task:** `.agents/docs/changes/YYYY-MM-DD-short-description.md` — append-only; include summary, files changed, and verification steps.
- **Every project ships two guides:** a high-level `README.md` (description, quick start, architecture, deployment) and a low-level spec in `.agents/docs/specs/<app>/`.
- ADRs in `.agents/docs/architecture/DECISIONS.md` (append-only); implementation plans in `.agents/docs/plans/`.
- Update docs in the same commit as the code they describe. Comments explain *why*, not *what*.

## Project Conventions

Every project root has `AGENTS.md` (project context), `REQUIREMENTS.md` (non-negotiable requirements), `TODO.md` (deferred work), and `README.md`. Confirm alignment with `REQUIREMENTS.md` before significant changes; never modify it without explicit approval.

All agent artifacts live in a single `.agents/` tree:

```
.agents/
├── rules|skills|workflows|commands/     # project-specific overrides
└── docs/{changes,architecture,specs,testing,plans}/
```

## Workflows & Learning Loop

- Global slash commands: `/audit`, `/research`, `/retro`, `/distill`, `/init-project`, `/handoff`.
- Corrections are logged to `~/.agents/feedback/log.jsonl`; `/retro` turns them into review-gated proposals in `~/.agents/proposals/` — nothing is applied automatically.
- Delegate to subagents proactively, without being asked: `reviewer` after significant code changes, `testing` when code lands without tests, `changelog` + `documentation` after completing a task or feature.

## Interaction

- Clarify scope, edge cases, and success criteria before large features — use plan mode. For trivial tasks, just act.
