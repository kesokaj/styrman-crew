# Requirements — styrman-harness

> Immutable without the operator's explicit approval. Agents never edit this file.

## R1 — One source, two vendors

`context/AGENTS.md` (plus `context/*.md`) is the single global context, deployed to Claude Code and the Antigravity CLI. Subagents are authored once in Claude Code format under `agents/`; the Antigravity flavour is derived at install time. Nothing generated is committed.

## R2 — Standards, not vendor formats

The context follows the agents.md standard and is named `AGENTS.md` at every level (`~/.claude/CLAUDE.md` and a project's `CLAUDE.md` are one-line imports). Skills follow the Agent Skills `SKILL.md` standard so that anything from skills.sh or any other source drops into `skills/` unchanged.

## R3 — No hooks, no gates, no CI

The harness is context, skills, and subagents. Behaviour comes from what is written in them, not from lifecycle hooks, commit gates, or pipelines. Git is backup and versioning only.

## R4 — One binary, stdlib only, easy on and off

`sh-agent` is a single Go binary with no dependencies. `install` is idempotent and overwrites only files it manages; `uninstall` removes exactly what its manifest lists; `update` pulls the repo from git and reinstalls. A hand-written `~/.claude/CLAUDE.md` is never overwritten.

## R5 — Hand-added customizations are first class

Adding a skill, subagent, command, or rule is: put the file in `skills/`, `agents/`, `commands/`, or `rules/` (by hand, by `npx skills add` from the repo root, or by `sh-agent adopt`), then `sh-agent install`. No registration, no manifest edits, no code. The installer maps each directory to its place under both vendors.

## R6 — Development never touches the real home

Every test and every development run of the installer uses `--home` with a scratch directory. Deploying to the operator's real `$HOME` is the operator's action.

## R7 — Projects remember and stay resumable

Every project keeps its scratch in `.agents/scratch/` (gitignored), its failures as numbered records in `.agents/docs/bugs/`, its progress in plan files with task checkboxes, and its retro and distill records in `.agents/docs/retros/`, so a zero-context session can resume and does not repeat a mistake. The harness's own state (manifest, feedback log, the harness proposal queue, the cross-project index) lives in `~/.config/sh-agent/` and nowhere else; a project's retro and distill records are the project's own state, in its trail (ADR-015, clarified 2026-09-13 with the operator's approval).

## Known limits

- **Antigravity global rules path.** The CLI's customization root is `~/.gemini/config/`; rules there are documented for the workspace form (`.agents/rules/*.md`) and by analogy for the global one. Verified on the Linux box 2026-09-03 (`.agents/docs/testing/2026-09-03-verify-real-home.md`): the CLI reads rules, skills, and agents from there. Confirmed on the Mac for Antigravity on 2026-09-04 (`.agents/docs/testing/2026-09-04-verify-mac-real-home.md`); Claude Code on the Mac is unverified.
- **Antigravity tool names cannot be enumerated.** An unrecognised name in an agent's `tools:` hangs the subagent, so `sh-agent install` fails on any tool it cannot map rather than guessing.
- **Antigravity IDE is out of scope.** Only the `agy` CLI is targeted; the IDE's global paths are unverified.

## Non-goals

- Hooks of any kind, on either vendor. Researched and rejected 2026-09-03 (ADR-007, `.agents/docs/plans/2026-09-03-research-hooks.md`); the harness is enforced by reading, not by gates.
- Plugin or marketplace packaging on either vendor: both namespace skills and agents, which would rename all of them.
- Editing `settings.json` or any vendor setting: the harness is files in the customization directories, nothing else.
- Symlinking the vendor directories to one shared store: neither CLI documents reading the other's directory, and a copy has no dangling-symlink failure mode.
