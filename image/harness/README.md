# styrman-harness

A personal agent harness for **Claude Code** and the **Antigravity CLI** (`agy`): one global context file in the [agents.md](https://agents.md) standard, 27 skills in the [Agent Skills](https://agentskills.io) `SKILL.md` standard, seven subagents, and `sh-agent` — a small Go binary that installs, resyncs, inspects, and removes all of it. No hooks, no gates, no CI.

## Install

```bash
git clone git@github.com:kesokaj/styrman-harness.git ~/workspace/styrman-harness
cd ~/workspace/styrman-harness
go run ./cmd/sh-agent install     # builds the binary from the checkout, deploys everything, installs the build to ~/.local/bin/sh-agent
sh-agent status                   # should end with "drift none"
```

Make sure `~/.local/bin` is on your `PATH`. Requires Go 1.24+ and git; nothing else.

Harness state (feedback log, retro proposals and index) lives in `~/.config/sh-agent/`, outside any project, so Claude Code asks once per session before the first read or write there. To stop the prompt, add the directory yourself to `permissions.additionalDirectories` in `~/.claude/settings.json`; the installer never touches vendor settings (ADR-010).

## Daily commands

| Command | What it does |
|---|---|
| `sh-agent status` | Shows what is installed, from which commit, the running binary's build (with a warning when it is older than the checkout), and every file that drifted. Exit 1 on drift. |
| `sh-agent diff` | Unified diff of every drifted file (installed vs repo). |
| `sh-agent install` (or `sync`) | From its own git checkout, rebuilds the binary with `go build`, then deploys the repo; from a plain copy (no `.git`) deploys the running binary as is (`--vendor antigravity\|claude\|all`). Idempotent; removes files a previous install wrote that the repo no longer has. |
| `sh-agent update` | `git fetch` the harness repo; if behind, `pull --ff-only`, rebuild, and reinstall. `--check` only reports (exit 2 when behind). |
| `sh-agent uninstall` | Removes exactly the files it installed (from its manifest) and nothing else. |
| `sh-agent list` | Prints the skill and agent inventory. |
| `sh-agent render` | Prints the assembled context file. |
| `sh-agent adopt <name\|path>` | Copies a skill installed elsewhere (for example by `npx skills add -g`) into `skills/`. |
| `sh-agent version` | Prints the build version. |

Every command accepts `--vendor VENDOR` (target `all` [default], `antigravity` / `agy`, or `claude`), `--home DIR` (install somewhere other than `$HOME`, used by the tests) and `--repo DIR`. Without `--repo`, sh-agent finds the checkout from `$SH_AGENT_REPO`, the working directory, or the path it recorded at the last install — so `sh-agent update` works from anywhere.

## The loop: edit on one machine, pull on another

```bash
# machine A
vim context/dev-setup.md && sh-agent install && git commit -am "context: tweak" && git push
# machine B
sh-agent update      # fetches, pulls, rebuilds, reinstalls
```

## What gets installed where

| Source in this repo | Claude Code | Antigravity CLI |
|---|---|---|
| `context/AGENTS.md` + `context/dev-setup.md` + generated inventory (names only; `sh-agent list` has the descriptions) | `~/.claude/AGENTS.md`, imported by a one-line `~/.claude/CLAUDE.md` (`@AGENTS.md`) | `~/.gemini/config/rules/AGENTS.md` |
| `skills/<name>/` | `~/.claude/skills/<name>/` | `~/.gemini/config/skills/<name>/` |
| `agents/<name>.md` (Claude format) | `~/.claude/agents/<name>.md` | `~/.gemini/config/agents/<name>/agent.md` (tool and model names translated at install) |
| the binary | `~/.local/bin/sh-agent` | same |
| manifest, feedback log, retro proposals, `retros.jsonl` (cross-project retro/distill index) | `~/.config/sh-agent/` | same |

An existing `~/.claude/CLAUDE.md` that sh-agent did not write is left alone; add the line `@AGENTS.md` to it yourself.

## Adding things by hand — no code

Every customization is a file in a directory. Drop it in, run `sh-agent install`, commit. Nothing to register. The installer refuses a `SKILL.md` without a `description`, and any frontmatter value that a strict YAML parser would read as a comment (an unquoted `#` after a space or tab) — Antigravity silently drops such a skill, so quote it.

| Put it in | What it is | Claude Code gets | Antigravity gets |
|---|---|---|---|
| `skills/<name>/SKILL.md` | a skill (Agent Skills standard); the model loads it when the description matches, and `/name` types it | `~/.claude/skills/<name>/` | `~/.gemini/config/skills/<name>/` |
| `agents/<name>.md` | a subagent in Claude Code format (`name`, `description`, `tools`, `model`, …) | `~/.claude/agents/<name>.md` | `~/.gemini/config/agents/<name>/agent.md`, tools and model translated |
| `commands/<name>.md` (or `workflows/`) | a plain markdown prompt invoked as `/name`; frontmatter `description` optional | `~/.claude/commands/<name>.md` | `~/.gemini/config/workflows/<name>.md` |
| `rules/<name>.md` | an always-on or path-scoped rule (Claude: `paths:` frontmatter; Antigravity: `trigger: glob` + `globs:`; both ignore the other's keys) | `~/.claude/rules/<name>.md` | `~/.gemini/config/rules/<name>.md` |
| `context/<name>.md` | more global context, appended after `AGENTS.md` | part of `~/.claude/AGENTS.md` | part of `~/.gemini/config/rules/AGENTS.md` |

A skill is the portable way to write a command: both vendors run any skill as `/name` without translation, and the model can load it itself when the description matches — every harness skill works both ways, so the agent can run `/handoff` or `/retro` for you. Add `disable-model-invocation: true` only to a skill that must never fire on its own (none of the harness's own do). Prefer a skill over `commands/` for anything new.

### From skills.sh

Any directory with a `SKILL.md` dropped into `skills/` is a skill. From [skills.sh](https://skills.sh):

```bash
cd ~/workspace/styrman-harness
npx skills add vercel-labs/skills -s find-skills -y   # lands in skills/ via the .agents/skills symlink
sh-agent install
git add skills/find-skills skills-lock.json && git commit -m "skill: find-skills" && git push
```

Record third-party provenance in `skills/VENDORED.md`. A skill you already installed globally with `npx skills add -g` can be pulled in with `sh-agent adopt <name>`.

## Layout

```
context/     AGENTS.md (the global context) and dev-setup.md (environment, stack, GCP rule)
agents/      seven subagents in Claude Code format — the single source; Antigravity's flavour is derived
skills/      27 skills, each a directory with SKILL.md (+ references/, assets/); `feature` and `bug` are the work protocols and load themselves; `review` is the review command on both CLIs
commands/    markdown prompts invoked as /name (empty; prefer skills)
rules/       extra always-on or path-scoped rules (empty)
cmd/sh-agent  the binary; internal/harness is its logic and tests
.agents/     this repo's own trail (docs/, incl. bugs/INDEX.md), scratch/ (gitignored), and a skills -> ../skills symlink for the skills CLI
```

## Working on the harness

```bash
gofmt -l . && go vet ./... && go test -race ./...
go run ./cmd/sh-agent install --home .agents/scratch/home   # never test against your real $HOME; scratch lives in the repo, never /tmp
```

See `AGENTS.md` for the repo rules and `.agents/docs/plans/2026-09-03-rebuild.md` for the rebuild checklist.
