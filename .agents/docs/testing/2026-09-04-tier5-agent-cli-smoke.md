---
owner: main
date: 2026-09-04
---
# Tier 5 step 4 — the agent CLIs inside a fresh project on `styrman-runtime:1.20.0` (2026-09-04)

Run by the main thread with the operator's go ("You can run the test"). An isolated dev server (own state dir, loopback, auth none) booted a fresh project on the RELEASED image; the operator's own local sign-in state (`~/.claude/.credentials.json`, `~/.gemini/antigravity-cli/`) was copied into the throwaway home for the test and removed with the state dir afterwards. Production and the credential broker were not involved. Script: `.agents/scratch/tier5-smoke.sh` (gitignored); log copied below.

```
[harness-migrate] no previous harness stamp — nothing to migrate
[styrman] agent harness deployed (version 1.20.0, styrman-harness 6893e22)
sh-agent status: repo has 28 skills, 7 agents, 0 commands, 0 rules; drift none — 255 files match the repo

claude -p "…first heading of your global context; how many skills; is /grilling one of them"
(1) `# AGENTS.md — global agent context`
(2) 28 sh-agent skills per that file (plus ~14 built-in Claude Code ones in this session's list) — and yes, `/grilling` is one of the 28.

agy -p "…which global rules file did you load and what is its first heading?"
I loaded [`AGENTS.md`](file:///home/kesokaj/.gemini/config/rules/AGENTS.md) and its first heading is `# AGENTS.md — global agent context`.
```

Verdict: PASS — the seeded harness is what both CLIs load in a project. The plan's e2e golden path is complete; the operator's existing production project picks the harness up on its next stop/start.
