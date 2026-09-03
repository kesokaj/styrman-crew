---
owner: main
date: 2026-09-03
---
# E2E — harness replacement, Phase 5 (2026-09-03)

Run by the main thread (not the verifier): the scripts are in `.agents/scratch/` (gitignored) and the raw logs beside them; the numbers below are copied from those logs.

## Golden path — `integration.sh styrman-runtime:dev` on real Kata

`passed=62 failed=0 skipped=3` (the three firewall SKIPs). The five harness checks all PASS inside the booted project: `sh-agent status` no drift; `CLAUDE.md` is the two-line import stub; 28 skills and 7 agy agents; no `~/.agents/bin`, `~/.gemini/GEMINI.md`, `~/.claude/settings.json`; **boot→ready 1.69 s** from the container's own timestamps (R1 soft target 5 s, hard limit 15 s; the old harness measured ~2.7 s on 2026-09-01).

## Upgrade path — a project seeded by `styrman-runtime:1.19.1`, restarted on `:dev`

Isolated server and state dir; the project was created on the 1.19.1 image (stamp `1.19.1`, old `settings.json` and `~/.agents/bin` present, 29 entries under `~/.claude/skills`), a user file planted at `~/.claude/keep-me.txt`, the project stopped, the server restarted with `STYRMAN_IMAGE=styrman-runtime:dev`, the project started:

```
[harness-migrate] moved .claude/CLAUDE.md … moved .agents/bin        (12 lines)
[harness-migrate] moved 12 path(s) to ~/.styrman/harness-backup-20260903-212552; 0 left in place
[styrman] agent harness deployed (version dev-1788470115, styrman-harness 6893e22)
```

Afterwards in the project home: stamp = the dev version; one backup dir holding `.claude/{CLAUDE.md,settings.json,skills,agents,statusline.sh,format-hook.sh,feedback-capture.sh}`, `.gemini/{GEMINI.md,format-hook.sh,config/…}`, `.agents/bin`; `settings.json`, `GEMINI.md`, `.agents/bin`, `plugins/harness` gone from the home; `CLAUDE.md` = `@AGENTS.md` stub; 28 skills, 7 agy agents; `keep-me.txt` intact; `sh-agent status` inside the VM: `drift none — 255 files match the repo`. A second stop/start: no new migration lines, still one backup dir.

## Not run here

`claude -p` / `agy` inside a project need the operator's brokered sign-in (no credentials in an isolated state dir) — the manual Tier 5 step 4, to be done on the released version.
