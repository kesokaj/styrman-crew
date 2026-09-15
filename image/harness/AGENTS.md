# AGENTS.md — styrman-harness

Project rules for this repo. They override the global context, which this repo is the source of (`context/`).

## What this repo is

The agent harness: one global context (agents.md standard), skills (Agent Skills `SKILL.md` standard), subagents, and `sh-agent`, the Go binary that installs them into Claude Code and the Antigravity CLI. No hooks, no gates, no CI.

## Setup commands

```bash
gofmt -l . && go vet ./... && go test -race ./...     # before any completion claim
go run ./cmd/sh-agent install --home .agents/scratch/home  # exercise the installer against a scratch home (gitignored)
go run ./cmd/sh-agent list                              # sanity-check skills and agents parse
```

## Layout

```
context/AGENTS.md      the global context — single source, deployed to both vendors
context/dev-setup.md   environment, stack, GCP defaults — appended to the context at install
agents/*.md            subagents in Claude Code format; Antigravity's agent.md is derived at install
skills/<name>/         skills; .agents/skills is a symlink here so `npx skills add` lands in skills/
commands/, rules/      markdown prompts invoked as /name, and extra rules; both deploy to both vendors verbatim
cmd/sh-agent, internal/harness   the binary and its tests
.agents/docs/          this repo's own trail (plans, changes, ADRs, reviews, testing, deploy/{local,remote,gcp}.md, bugs/INDEX.md)
.agents/scratch/       gitignored scratch space; every generated or temporary file goes here, never /tmp
```

Nothing generated is committed. The deployed files are assembled by `sh-agent install`.

## Rules for working here

- The installer must never run against the operator's real `$HOME` during development. Use `--home .agents/scratch/<name>` for every test (the global scratch rule: never `/tmp`). Deploying to the real home is the operator's job.
- `sh-agent install` rebuilds the binary from its own git checkout with `go build` and lets that build install (BUG-006); the running binary is copied only when `go` is missing or when the tree is a plain copy with no `.git` — a downstream mirror, whose image or VM build step built that binary from the same tree and may keep it read-only (BUG-015). After a change under `cmd/` or `internal/`, `sh-agent status` from the target home must show a `binary` line without a warning before the install is called done.
- Adding a tool to an agent's `tools:` list means adding it to `toolMap` in `internal/harness/derive.go`. An unmapped tool fails the install on purpose: Antigravity hangs a subagent that names a tool it does not recognise.
- Agent `description:` values are one line, double-quoted if they contain a colon, and free of the words "dispatch prompt" or "harness binary" — there is no binary behind them anymore.
- Every skill directory needs a `SKILL.md` with a `description`; the installer refuses one without it. It also refuses any frontmatter value that a strict YAML parser reads as a comment — an unquoted `#` at the start or after a space or tab — because Antigravity silently drops such a skill (BUG-004); quote the value.
- Adding a skill, agent, command, or rule is a file drop — never a code change. If a new kind of customization needs code, that is a design question, not a quick patch.
- Third-party skills are vendored verbatim and listed in `skills/VENDORED.md`; never edit them in place — fork into a differently named skill.
- Stdlib only in the Go code. The binary is a file copier with a manifest; it does not need a dependency.
- Commit and push after each task, per the global git workflow. This repo is on `main`.

## Testing

There is no test framework beyond `go test`. A change to `context/`, `agents/`, or `skills/` is verified by `go run ./cmd/sh-agent install --home .agents/scratch/x` succeeding and `go run ./cmd/sh-agent status --home .agents/scratch/x` reporting no drift, then by one start of each CLI in the sandbox (`.agents/docs/testing/2026-09-03-verify-sandbox.md`) with its log grepped for parse errors. A verification log states what it did not exercise; "complete" or "all skills" is said only after the full matrix in `.agents/docs/testing/skill-matrix.md` has run. Tested means tested from where it runs: after any install into a real home, the three smoke checks in `.agents/docs/testing/2026-09-03-verify-real-home.md` (context questions, one skill load, one agent dispatch) run from that home on each CLI before the install is called done. A change to `context/`, `agents/` or `skills/` also gets `/review --docs` over its range; here the living docs also include `context/`, `agents/`, and every skill directory under `skills/` not listed in `skills/VENDORED.md`, searched at `skills/` because `.agents/skills` is a symlink.

## Key files

| Path | Purpose |
|---|---|
| `context/AGENTS.md` | The global context every session reads |
| `internal/harness/plan.go` | What gets installed where |
| `internal/harness/derive.go` | Claude → Antigravity agent translation |
| `REQUIREMENTS.md` | Non-negotiables for the harness |
| `.agents/docs/plans/2026-09-03-rebuild.md` | The rebuild checklist |
