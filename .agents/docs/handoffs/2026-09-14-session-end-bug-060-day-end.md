---
owner: main
date: 2026-09-14
---
# Handoff — 2026-09-14 — day end: BUG-060 fixed on main, unreleased; operator recreates the affected project

Session ID: session_01RUoU1zHUVbathNBFLo27xf        (same machine: `claude --resume`; matches the feedback log's session field)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

Supersedes `.agents/docs/handoffs/2026-09-14-session-end-bug-060.md` (same
session, written before the last two exchanges); that file's Completed,
Key Decisions and Artifacts sections still hold and are not repeated here.

## Context

The operator opened a stopped project on the live site after the v1.31.0
deploy and its first terminal reported "the broker did not answer". Root
cause and fix are BUG-060 (commit `d285015`, pushed, NOT released). The
operator then asked whether deleting and recreating the project clears it
(yes, until the next server restart while it is stopped) and declined any
hand-applied workaround. The session ends here for the day.

## Completed

- BUG-060 diagnosed live, fixed, tested, reviewed, documented, recorded,
  committed `d285015` and pushed; handoff `5570e8b`. Details in the
  superseded handoff and `.agents/docs/changes/2026-09-14-bug-060-terminal-ws-boot-attach.md`.
- Answered the delete-and-recreate question: a new project attaches on
  creation (its first terminal may race the one-second background join
  once); the defect recurs on 1.31.0 for any project stopped across a
  server restart until the fix ships.

## Open / In Progress

- Not released. Production is 1.31.0 with the defect. A release is the
  operator's call ("ship" → `/preflight`, PATCH — it only fixes released
  behaviour).
- Live proof outstanding (unit-verified only): after the deploy, open from
  the UI a project that was STOPPED across the server restart — no "broker
  did not answer" MOTD line, `gh auth status` succeeds. Closes BUG-054's
  proof too.
- The affected project `sdfsf` (`styrman-32c61123-…`): the operator intends
  to delete and recreate it; the trail does not record whether that
  happened.
- Requirements edits awaiting approval: none.

## Key Decisions

- No host-side or UI workaround for the live project (operator, below); the
  earlier handoff's Next Step 1 (a `docker network connect` one-liner) is
  withdrawn. Pre-production projects are disposable; the platform gets the
  fix.
- Security's pre-existing `ip_forward=0` observation (compose sets it,
  nothing asserts it at runtime) stays carried here, not in TODO.

## Corrections

- "I dont want custom fixes. THis is still not a production project. I can
  remove it if it solves it." — feedback log 2026-09-14T15:00:42+02:00
  (category convention, signal correction, rule_ref null). One correction
  this session: below the `/retro` threshold; the harness retro ran today
  (retros index, 2026-09-14T08:28Z).

## Agent notes (unverified)

- Carried from the superseded handoff: the changelog agent's draft needed
  two read-through corrections (eighth and ninth in three days); the
  documentation agent split a sentence in the testing README; coverage gaps
  named by the verifier and not fixed (`EnsureSelfAttached` has no unit
  test; `ensureFor`'s lookup is untested).
- The operator's "no custom fixes" preference is saved as a memory for this
  project; it is not yet a project rule — a `/retro` candidate once a
  second instance appears.

## Environment State

- Branch: main, clean, level with origin at `5570e8b` before this handoff's
  commit. No tag; last release tag v1.31.0 on `c3b4417`.
- Production: https://crew.styrman.xyz on 1.31.0. At session end the
  project `styrman-32c61123-…` was still running with the server not
  attached to its bridge (the operator may have deleted it since).
- Scratch: `.agents/scratch/stage-bug060.txt`, `stage-handoff.txt`
  (gitignored).

## Artifacts to Read

- `.agents/docs/handoffs/2026-09-14-session-end-bug-060.md` — the full
  session record this file supersedes (diagnosis, fix, reviews).
- `.agents/docs/bugs/BUG-060-terminal-ws-boot-never-joined-the-project-bridge.md` — root cause and prevention rule.
- `.agents/docs/changes/2026-09-14-bug-060-terminal-ws-boot-attach.md` — files, agents, verification limits.
- `.agents/docs/reviews/2026-09-14-security-wsboot-attach.md` — the three lows and their Outcome.

## Next Steps

1. Operator: release when ready (`/preflight`, PATCH), then the live proof
   above.
2. Operator: delete and recreate `sdfsf` if not already done (no other
   action on the live host is wanted).
3. Optional, its own task: a runtime assertion for `ip_forward=0` on the
   server container (security finding 3, pre-existing).
4. `/retro` when a session next collects two corrections; standing
   candidates: the changelog read-through pattern, and "no custom live
   workarounds" as a project rule.
