# devshell_prompt / harness

Multi-vendor agent harness for AI-assisted development. Based on the
[**AGENTS.md**](https://agents.md) framework so the same standards, skills, and
workflows work across Antigravity (Gemini), Claude Code, and any other agent
that follows the spec.

## What This Is

A configuration repo that provisions standards, skills, and slash commands into the
appropriate vendor directories. **Claude Code is the source of truth**: `shared/` holds
the vendor-neutral content deployed to Claude verbatim, and `claude/agents/` is the
canonical agent set. Everything in `antigravity/` is derived from those at deploy time.

| Source (in repo)               | Deployed to (Antigravity CLI `agy`)                | Deployed to (Claude Code)       |
|--------------------------------|----------------------------------------------------|---------------------------------|
| `shared/AGENTS.md`             | `~/.gemini/GEMINI.md`                              | `~/.claude/CLAUDE.md`           |
| `shared/skills/` (incl. workflow skills) | `~/.gemini/config/skills/`               | `~/.claude/skills/`             |
| `shared/scripts/format-hook.sh`| `~/.gemini/format-hook.sh`                        | `~/.claude/format-hook.sh`      |
| `shared/scripts/feedback-capture.sh` | *(n/a — agy has no prompt-submit hook event)* | `~/.claude/feedback-capture.sh` |
| `claude/settings.json`         | *(n/a — see settings note below)*                  | `~/.claude/settings.json` (merged) |
| `claude/agents/`               | `~/.gemini/config/plugins/harness/agents/` (derived) | `~/.claude/agents/`           |
| `antigravity/plugins/harness/hooks.json` | `~/.gemini/config/plugins/harness/hooks.json` | *(n/a — Claude hooks live in settings.json)* |
| `claude/scripts/statusline.sh` | *(n/a — agy's statusline is not scriptable)*       | `~/.claude/statusline.sh`       |
| `shared/templates/`            | *(used by `/init-project` workflow)*               | *(used by `/init-project`)*     |

> ℹ️ **Antigravity paths are verified against agy-cli 1.1.11** (which rebuilt the
> `~/.gemini` tree in 2026-08). Global rules load from `~/.gemini/GEMINI.md`; skills
> and plugins are discovered under `~/.gemini/config/`. Custom agents are packaged
> inside the `harness` plugin and derived from `claude/agents/` on every deploy by
> `shared/scripts/derive-agy-agent.py` (keeps name/description, adds
> `subagent: true` / `mainAgent: false`, maps Claude model aliases to agy tiers,
> rewrites skill paths) — **edit agents only in `claude/agents/`**.
>
> **Subagents run proactively on both vendors.** Agent descriptions carry "use
> proactively…" triggers and `shared/AGENTS.md` instructs delegation (reviewer after
> code changes, testing for untested code, changelog + documentation after tasks), so
> the main agent dispatches them automatically — no manual invocation needed. On
> Claude Code, `changelog` and `documentation` also keep per-project agent memory
> (`memory: project`), and `reviewer` preloads the `code-review` skill.
>
> **Settings:** only Claude's `~/.claude/settings.json` is managed (merged, never
> overwritten, via `shared/scripts/merge-settings.py` — repo keys win, string lists
> union, local customization survives). agy-cli's settings file
> (`~/.gemini/antigravity-cli/settings.json`) holds its own auth/trust/UI state with an
> unrelated schema and is never touched; the old `~/.gemini/settings.json` is no longer
> read by agy, and the deploy retires the harness-created copy (backed up first).
> Antigravity hooks ship via the plugin's `hooks.json` instead — the deployed
> `harness-format` hook runs `~/.gemini/format-hook.sh` after
> `write_to_file`/`replace_file_content`/`edit_file`/`create_file` tool calls.

> **Format-hook dependencies (macOS):** the deployed `format-hook.sh` uses
> `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`, falls back to
> `gofmt`) for Go and project-local `prettier` (via `npx --no-install`) for TS/JS/CSS/MD.
> Both are optional — the hook silently no-ops when a formatter is absent.

## Quick Start

```bash
# Deploy for both Antigravity and Claude Code
./setup.sh

# Deploy for one vendor only
./setup.sh --target antigravity
./setup.sh --target claude

# Preview without changes (works at every level)
./setup.sh --dry-run
./setup.sh --target claude --dry-run
./setup.sh --target antigravity --dry-run
```

> ⚠️ The skills deploy uses `rsync --delete`. Anything in
> `~/.gemini/config/skills/` or `~/.claude/skills/` that isn't in `shared/skills/`
> will be removed. (Plugins and scripts deploy WITHOUT `--delete` and back up
> existing files first.) Run with `--dry-run` first if you're not sure.

### Verifying installation

```bash
# Antigravity CLI (agy) — paths verified on agy-cli 1.1.11
wc -l ~/.gemini/GEMINI.md                                  # → 63
ls ~/.gemini/config/skills/ | wc -l                        # → 23 (incl. audit/distill/handoff/init-project/research/retro)
ls ~/.gemini/config/plugins/harness/agents/                 # → changelog.md / documentation.md / reviewer.md / testing.md
agy agents                                                  # → changelog / documentation / reviewer / testing
agy -p "/skills"                                            # quota-free skill listing (agy 1.1.11+)

# Claude Code
wc -l ~/.claude/CLAUDE.md                                  # → 63
ls ~/.claude/skills/ | wc -l                               # → 23
ls ~/.claude/agents/                                       # → changelog / documentation / reviewer / testing (.md)
test -f ~/.claude/settings.json && test -x ~/.claude/format-hook.sh && echo "config ok"
```

## Structure

```
.
├── shared/                  # Single source of truth (vendor-neutral)
│   ├── AGENTS.md            # Global standards (the AGENTS.md spec file)
│   ├── skills/              # 23 skills: 17 domain skills + 6 workflow skills (/audit, /handoff, /retro, …)
│   ├── scripts/             # format-hook.sh + merge-settings.py + derive-agy-agent.py
│   └── templates/           # Project scaffolding templates (AGENTS.md, REQUIREMENTS.md, …)
├── antigravity/             # Antigravity CLI (agy) wrapper — all content derived
│   ├── GEMINI.md            # Copy of shared/AGENTS.md, refreshed by setup.sh
│   ├── plugins/harness/     # Plugin: agents/ (derived from claude/agents/) + hooks.json (format hook)
│   └── setup.sh             # Deploys to ~/.gemini/
├── claude/                  # Claude Code wrapper
│   ├── CLAUDE.md            # Copy of shared/AGENTS.md, refreshed by setup.sh
│   ├── settings.json        # Global baseline, merged into ~/.claude/settings.json
│   ├── agents/              # Custom subagents (changelog, documentation, reviewer, testing)
│   ├── scripts/             # statusline.sh (Claude-only)
│   └── setup.sh             # Deploys to ~/.claude/
└── setup.sh                 # Top-level dispatcher → runs antigravity/ + claude/ setups
```

Claude-specific slash commands (if any) go in `claude/commands/`; the directory is
optional and overlaid onto `~/.claude/commands/` when present.

The learning loop writes to global, vendor-neutral locations shared by all
projects: corrections go to `~/.agents/feedback/log.jsonl` (each entry tagged
with the project path) and `/retro` proposals go to `~/.agents/proposals/`.
Both are created on demand (`mkdir -p`) — nothing ships as an empty placeholder.
In Claude Code, capture is also automatic: a `UserPromptSubmit` hook
(`feedback-capture.sh`) pattern-matches likely corrections and appends them to
the same log, so collection doesn't depend on the model remembering to log.

## Editing Standards

1. Edit `shared/AGENTS.md` — this is the canonical file for all agents. Edit
   subagents in `claude/agents/` — the Antigravity plugin copies are derived.
2. Run `./setup.sh` to redeploy.
   - The script refreshes the in-repo `antigravity/GEMINI.md` and `claude/CLAUDE.md` copies from `shared/AGENTS.md`, and regenerates `antigravity/plugins/harness/agents/` from `claude/agents/`.
   - It then deploys those into `~/.gemini/` and `~/.claude/`.
3. Commit the source files together with the refreshed derived copies so the repo never ships out-of-sync derived files.
4. Changes take effect in the next AI session.

**Do not edit `antigravity/GEMINI.md`, `claude/CLAUDE.md`, or
`antigravity/plugins/harness/agents/` directly.** They are overwritten on every
`setup.sh` run.

For project-specific overrides, use the project's `AGENTS.md` or `.agents/rules/` —
do not modify this repo for single-project concerns.

## Cross-Vendor Compatibility (AGENTS.md framework)

Every project scaffolded by `/init-project` gets an `AGENTS.md` at its root.
This vendor-neutral file is read by Antigravity, Claude Code, Codex, Cursor,
VS Code, and any other agent that follows the [AGENTS.md spec](https://agents.md).

Globally, the same `shared/AGENTS.md` is deployed as `~/.gemini/GEMINI.md`
(Antigravity-native filename) and `~/.claude/CLAUDE.md` (Claude-native filename)
so both agents read identical rules without needing project-level config.

## How Skills Work

Each skill follows a **progressive disclosure** architecture:

1. **Metadata** (name + description in YAML frontmatter) — always in context; triggers skill loading
2. **SKILL.md** — loaded when the skill triggers (< 500 lines)
3. **references/** — loaded on demand for deep-dive patterns (unlimited size)

The same `SKILL.md` frontmatter format is recognised by both Antigravity and
Claude Code, so skills port 1:1.

To create a new skill, use the `skill-creator` skill or see `shared/skills/skill-creator/SKILL.md`.

## How Slash Commands Work

Workflows ship as **skills** — both vendors expose a skill at
`skills/<name>/SKILL.md` as the slash command `/<name>` (Claude Code merged
commands into skills; agy-cli auto-converts skills to slash commands). One
source, one deploy target, two vendors:

- `shared/skills/audit/SKILL.md` → `/audit`
- `shared/skills/retro/SKILL.md` → `/retro`
- `shared/skills/distill/SKILL.md` → `/distill`
- `shared/skills/research/SKILL.md` → `/research`
- `shared/skills/init-project/SKILL.md` → `/init-project`
- `shared/skills/handoff/SKILL.md` → `/handoff`

The workflow skills set `disable-model-invocation: true` so they stay
user-triggered in Claude Code (agy ignores that key). Claude-specific commands
(if any) go in `claude/commands/` and are overlaid onto `~/.claude/commands/`.

## Key Design Decisions

- **AGENTS.md is the global contract.** Project-level `AGENTS.md` files override only what differs.
- **`shared/` is the single source of truth.** Vendor folders are wrappers, not editable copies.
- **No symlinks.** Files in vendor folders are real copies, kept in sync by `setup.sh`.
- **Solo commit attribution.** Every git commit lists only the human author — no `Co-Authored-By:` trailers of any kind (no AI, no other contributors). See the Tooling section of `shared/AGENTS.md`.
- **Skills explain *when NOT to use* them.** This prevents context pollution from irrelevant skills loading.
- **Documentation is a first-class deliverable.** Every project gets `.agents/docs/` with changelogs, ADRs, specs, and testing guides.
- **Requirements are immutable gates.** `REQUIREMENTS.md` is read before every task and never modified without approval.
- **The harness learns from corrections.** A deterministic hook and the `feedback` skill log corrections to `~/.agents/feedback/log.jsonl`; `/retro` proposes improvements. Nothing changes without explicit approval.
