---
owner: main
date: 2026-09-11
---
# Handoff — 2026-09-11 — v1.28.0 live, retro applied to the harness; nothing queued

Session ID: 20b850ca (Claude Code).
Project: /home/kesokaj/workspace/styrman-crew
Branch: main, clean, pushed (this file's trail commit on top of `d9800a7`).

**Supersedes `2026-09-11-session-end-v1.28.0.md`** (same session, written
right after the release). Everything in that file still holds; read it for
the release detail. This one adds only what happened after it.

## Context

The operator asked for the session retro after v1.28.0 (Browser view +
styrman-ctl) went live, then approved every proposal. The project itself
did not change; the harness that every project reads did.

## Completed

- Retro written: `~/.config/sh-agent/proposals/2026-09-11-retro.md`
  (five proposals, all applied; operator's words: "apply all").
- Harness repo `/home/kesokaj/workspace/styrman-harness` commit `b64ffe8`
  ("harness: apply retro proposals from 2026-09-11"), pushed, deployed
  with `sh-agent install` (6 files written, 250 unchanged). The rules the
  next session reads in `~/.claude/AGENTS.md` therefore changed:
  1. Definition of Done step 4 clears ROADMAP "Now" and TODO; step 5 is
     "commit and push, then `/handoff`" — a handoff closes every task,
     not only a session ending mid-task.
  2. `feature` skill Step 4: the phase test run is its own bare tool
     call whose exit status is read before the commit (BUG-033/041).
  3. Hard rule: a green run is evidence only when shown to have hit the
     thing under test (BUG-040's self-hosting suite on a bound port).
  4. `retro` skill: "run the retro" in the user's own turn invokes it
     (the `disable-model-invocation` flag was removed).
  5. `feature` cold-read brief marks questions BLOCKING / NON-BLOCKING;
     `implementation ready` needs zero blocking ones; the three-round
     question to the user states whether the residue is real.
- Cold-read of this handoff fixed two trail items at source: `ROADMAP.md`
  "Now" now says v1.28.0 SHIPPED (it still read "release in progress";
  `documentation` agent), and the browser plan's Phase 4 checkpoint box
  (line 471) is ticked — it shipped in `eb7b883` but was never marked.
  The only unticked boxes left are the two operator steps.
- Production re-checked before this handoff: `styrman-ctl status --json`
  healthy on v1.28.0 (server/caddy 1.28.0, sha `eb7b883`, no failed
  version, updater + firewall timers active).

## Open / In Progress

- Nothing owed by the agent. The two operator steps from the superseded
  handoff stand: the manual agent walk in the Browser view on production
  (steps at the end of
  `.agents/docs/testing/2026-09-11-verify-browser-phase4.md`) and a
  fresh-host `sudo deploy/styrman-ctl install` on a Debian 13 VM, never
  on production. Tick the two plan boxes when run.

## Key Decisions

- Retro proposals 4 and 5 carried a stated trade-off (uninvited-retro
  guarantee; two-plan sample size) and were applied on the operator's
  "apply all".

## Corrections

- "Vital info. DoD=is when everything is done, no todo no roadmap left and
  documentation and all logs,reviews etc is done and commited and a
  handoff is there." — feedback log 2026-09-11T14:22:19+02:00. Now also in
  the global Definition of Done (retro Proposal 1).

## Agent notes (unverified)

- None new. See the superseded handoff for the release-time notes.

## Environment State

- Branch main clean and pushed; tag v1.28.0 on `eb7b883`.
- No project VMs running; production healthy on 1.28.0.
- Harness installed from `b64ffe8` on 2026-09-11.

## Artifacts to Read

- `.agents/docs/handoffs/2026-09-11-session-end-v1.28.0.md` — the release
  handoff (what shipped, follow-ups, observations).
- `~/.config/sh-agent/proposals/2026-09-11-retro.md` — the retro and the
  exact diffs applied.
- `.agents/docs/reviews/2026-09-11-preflight-v1.28.0.md` — the gate.

## Suggested Skills

- `feature` / `bug` for whatever comes next; note its Step 3 and Step 4
  wording changed today.

## Next Steps

1. Nothing queued. Wait for the operator's next request.
2. Operator: the two steps under "Open" when convenient.
