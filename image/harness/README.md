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
| `shared/scripts/resume-context.sh`, `trail-check.sh` | `~/.agents/bin/` (invoked as explicit steps inside skill bodies, e.g. `/handoff`, `/preflight`) | `~/.agents/bin/` (wired as the `SessionStart`/`Stop` hooks in `settings.json`) |
| `shared/templates/`            | *(used by `/init-project` workflow)*               | *(used by `/init-project`)*     |

> ℹ️ **Antigravity paths are verified against agy-cli 1.1.11** (which rebuilt the
> `~/.gemini` tree in 2026-08). Global rules load from `~/.gemini/GEMINI.md`; skills
> and plugins are discovered under `~/.gemini/config/`. Custom agents are packaged
> inside the `harness` plugin and derived from `claude/agents/` on every deploy by
> `shared/scripts/derive-agy-agent.py` (keeps name/description, adds
> `subagent: true` / `mainAgent: false`, maps Claude model aliases to agy tiers,
> rewrites skill paths) — **edit agents only in `claude/agents/`**.
>
> **Six subagents, dispatched via the Definition of Done.** `shared/AGENTS.md` (the
> "Global Agent Constitution", deployed verbatim to both vendors) makes dispatch the
> only compliant path: `reviewer` (any tracked-source change), `verifier` (before any
> completion claim or commit — replaces the deleted `testing` agent, and FAILs on
> uncovered changed code), `changelog` (every completed task), `documentation`
> (behavior/interface/plan changes), `security` (auth/crypto/secrets/deps touched),
> and `explorer` (recon spanning 10+ files). Each agent's own `description` front-loads
> its trigger (e.g. "Dispatch after any change to tracked source files…") rather than a
> generic "use proactively" phrase — see `SUB_AGENTS.md` for the full roster. On Claude
> Code this is backstopped by the deterministic shell gates below; **proactive
> (model-initiated) dispatch on Antigravity is unverified** — the derived agents are
> confirmed reachable via `invoke_subagent`. On Claude Code, `changelog` and
> `documentation` also keep per-project agent memory (`memory: project`), and
> `reviewer` preloads the `review-standards` skill.
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

> **Deterministic shell gates live in `~/.agents/bin/`, not in either vendor's home
> directory.** Both `claude/setup.sh` and `antigravity/setup.sh` copy
> `shared/scripts/resume-context.sh` and `shared/scripts/trail-check.sh` there
> (backing up any existing copy first) so the two vendors share one script, wired
> differently per vendor's discovery model:
> - **Claude Code** registers them as real hooks in `settings.json` —
>   `SessionStart` runs `resume-context.sh` (prints at most 4 lines orienting the
>   agent in an existing harness project: the Resume Protocol reading order, the
>   newest handoff + age, whether the trail is behind HEAD, and any missing root
>   file) and `Stop` runs `trail-check.sh` (blocks the stop with a JSON
>   `{"decision":"block",...}` when source changed this session but today's
>   `changes/`, `reviews/`, or `testing/` entry is missing).
> - **Antigravity has no equivalent hook events**, so the same scripts are invoked
>   as an explicit first step written into the relevant skill bodies instead (e.g.
>   `/handoff` and `/preflight` both open with "On Antigravity, first run
>   `~/.agents/bin/resume-context.sh`…"); `trail-check.sh --check` is the
>   human-readable freshness report used there and by `/preflight`.
>
> Both scripts are no-ops (exit 0, no output) outside a git repo or a project that
> hasn't opted in — they only activate once a `changes/` directory exists under the
> project's docs root.

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
> will be removed. Scripts and subagents deploy via plain file copy — no
> `--delete`, existing files backed up first (Claude's setup also explicitly
> retires named agents the harness has since dropped, e.g. `testing.md`).
> `~/.gemini/config/plugins/harness/` is the one exception on the Antigravity
> side: it's fully harness-owned (agents derived from `claude/agents/`, plus
> `hooks.json`), so it **does** sync with a scoped `rsync --delete` — a derived
> agent removed from the repo must not resurrect on the next deploy. Any other
> plugin directory under `~/.gemini/config/plugins/` (user-installed plugins) is
> left untouched. Run with `--dry-run` first if you're not sure.

### Verifying installation

```bash
# Antigravity CLI (agy) — paths verified on agy-cli 1.1.11
wc -l ~/.gemini/GEMINI.md                                  # → 69
ls -d ~/.gemini/config/skills/*/ | wc -l                   # → 28 (incl. audit/distill/handoff/init-project/preflight/research/retro)
ls ~/.gemini/config/plugins/harness/agents/                 # → changelog.md / documentation.md / explorer.md / reviewer.md / security.md / verifier.md
agy plugin validate ~/.gemini/config/plugins/harness        # → "✔ agents: 6 processed" / "✔ hooks: 1 processed" (verified live, agy 1.1.22, 2026-08-31). `agy agents` lists nothing — these are subagent-only, mainAgent:false.
agy -p "/skills"                                            # quota-free skill listing (agy 1.1.11+)

# Claude Code
wc -l ~/.claude/CLAUDE.md                                  # → 69
ls -d ~/.claude/skills/*/ | wc -l                          # → 28
ls ~/.claude/agents/                                       # → changelog / documentation / explorer / reviewer / security / verifier (.md)
test -f ~/.claude/settings.json && test -x ~/.claude/format-hook.sh && echo "config ok"

# Both vendors — shared shell gates
ls ~/.agents/bin/                                          # → resume-context.sh trail-check.sh
~/.agents/bin/trail-check.sh --check                       # → human-readable trail-freshness report
```

## Structure

```
.
├── shared/                  # Single source of truth (vendor-neutral)
│   ├── AGENTS.md            # Global Agent Constitution (the AGENTS.md spec file, 69 lines)
│   ├── skills/              # 28 skills: 7 workflow skills (/audit, /distill, /handoff,
│   │                        #   /init-project, /preflight, /research, /retro) + 21 others
│   │                        #   (language/infra + review-standards + 5 vendored — see VENDORED.md)
│   ├── scripts/             # format-hook.sh, feedback-capture.sh, merge-settings.py,
│   │                        #   derive-agy-agent.py, resume-context.sh, trail-check.sh
│   └── templates/           # Project scaffolding templates (AGENTS.md, REQUIREMENTS.md, ROADMAP.md, …)
├── antigravity/             # Antigravity CLI (agy) wrapper — all content derived
│   ├── GEMINI.md            # Copy of shared/AGENTS.md, refreshed by setup.sh
│   ├── plugins/harness/     # Plugin: agents/ (derived from claude/agents/, 6) + hooks.json (format hook)
│   └── setup.sh             # Deploys to ~/.gemini/ (+ shared/scripts/{resume-context,trail-check}.sh → ~/.agents/bin/)
├── claude/                  # Claude Code wrapper
│   ├── CLAUDE.md            # Copy of shared/AGENTS.md, refreshed by setup.sh
│   ├── settings.json        # Global baseline (incl. SessionStart/Stop hooks), merged into ~/.claude/settings.json
│   ├── agents/              # 6 custom subagents: changelog, documentation, explorer, reviewer, security, verifier
│   ├── scripts/             # statusline.sh (Claude-only)
│   └── setup.sh             # Deploys to ~/.claude/ (+ shared/scripts/{resume-context,trail-check}.sh → ~/.agents/bin/)
└── setup.sh                 # Top-level dispatcher → runs antigravity/ + claude/ setups
```

Both vendor setup scripts also deploy `shared/scripts/resume-context.sh` and
`trail-check.sh` to `~/.agents/bin/` — a vendor-neutral location outside either
vendor's home directory, since both hooks (Claude Code) and skill-body steps
(Antigravity) reference it by the same path.

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

To create a new skill, follow the SKILL.md standard by hand (frontmatter with
`name` + trigger-front-loaded `description`, lean body, lazy-loaded `references/`).
The harness no longer ships its own `skill-creator` (it was Gemini-adapted and
steered agents away from using subagents; Claude Code's built-in `skill-creator`
is unaffected and still works).

**Vendored (non-authored) skills** are tracked in `shared/skills/VENDORED.md` — a
provenance manifest recording, per skill, its upstream source repo, pinned
commit or digest, license, and the exact command to re-fetch an update. Never
edit vendored content in place; fork it into a differently-named skill instead.
Installing a skill globally with `npx skills add -g` writes straight into the
agent's own skill dir and gets wiped by the next harness deploy (`rsync
--delete`) — vendor it here to keep it.

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
- `shared/skills/preflight/SKILL.md` → `/preflight`

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
