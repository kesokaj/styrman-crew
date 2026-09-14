---
owner: main
date: 2026-09-14
---
# Handoff — 2026-09-14 — v1.33.0 live: the ten carried items closed, trail empty

Session ID: session_01RUoU1zHUVbathNBFLo27xf        (same machine: `claude --resume`; matches the feedback log's session field)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

Supersedes today's earlier handoffs (`…-session-end-bug-060.md`,
`…-session-end-bug-060-day-end.md`, `…-session-end-v1.32.0.md`): the
operator then asked for a plan closing every carried item and a release.

## Context

After v1.32.0 the operator asked which carried items were worth doing,
then: "NO open tasks should be in roadmap or todo. Make a plan to fix all
these for the next release … run a /preflight … make sure the local
installed on this machine is fully updated and run your own e2e tests and
verifications. Plus all documentation needs to be updated … THIS IS DOD."
The plan `.agents/docs/plans/2026-09-14-v1.33.0-close-out.md` (six
phases, cold-read ready in two rounds) is fully ticked; v1.33.0 is live.

## Completed

- v1.33.0 (release commit `21d8fd7`, tag pushed 14:41:59Z, updater "OK —
  v1.33.0 is live" 16:47:29 local): gate
  `.agents/docs/reviews/2026-09-14-preflight-v1.33.0.md` (SHIP, Deploy
  section observed); release entry
  `.agents/docs/changes/2026-09-14-v1.33.0-release.md`; README status;
  ROADMAP "Now" a comment; TODO headers-only.
- The ten items: (1) live proof — operator step below; (2) BUG-056 — the
  gate names a 429, `remote.md` "Docker Hub (pull budget)", login is the
  operator's; (3) `EnsureSelfAttached` and `ensureFor` tests; (4) retro
  `.agents/docs/retros/2026-09-14-retro.md`, rules
  `.agents/rules/no-live-workarounds.md` and `.agents/rules/changelog-brief.md`, two harness
  proposals queued; (5) TODO/ROADMAP headers; (6) eight
  `.claude/agent-memory` files deleted; (7) `ip_forward=0` asserted at boot
  (`STYRMAN_ALLOW_IP_FORWARD`, compose passthrough) — observed silent on
  the new container; (8) styrman-harness `cceee54` (BUG-016, name
  validation, symlink refusal, LoadState guard, scoped root-steps
  sentence) and the mirror refreshed to it — the mirror's tests pass as a
  plain copy (BUG-058 fixed upstream); (9) BUG-038 mitigated (trap format
  hoisted); (10) ADR-071's caveat resolved in terminal-protocol.md.
- Verification on the released tree: `verify-commit.sh` green on 8aaa5e9
  (full) and 21d8fd7; integration 94/0/3; e2e 92/0/2; vitest 178/178;
  verifier PASS (`.agents/docs/testing/2026-09-14-v1.33.0-phase2-verify.md`, `.agents/docs/testing/2026-09-14-v1.33.0-phase6-verify.md`).
- Per-phase entries under `.agents/docs/changes/` (`2026-09-14-v1.33.0-phase1-trail-hygiene.md` … `2026-09-14-v1.33.0-phase6-verification-and-docs.md`); reviews and
  security for Phase 2 and the release range, all Outcomes written.

## Open / In Progress

- Nothing queued. TODO.md and ROADMAP.md are headers (plus their standing
  comment). The plan file is fully ticked and stays as the record.
- Requirements edits awaiting approval: none.

## Key Decisions

- MINOR: a new operator knob plus a refreshed harness in every home.
- The ip_forward guard fails closed (a tripped guard rolls the deploy
  back); a missing sysctl file warns; any other read error refuses.
- Harness names are ASCII identifier-like; symlinks in skill dirs refused;
  the manifest guard lives in `LoadState` once.
- Item 1's live proof cannot be produced by this deploy (0 projects at
  the restart); it stays the operator's, below, never assumed.

## Corrections

- "I dont want custom fixes. THis is still not a production project. I
  can remove it if it solves it." — feedback log 2026-09-14T15:00:42+02:00.
- "I told you that NO open tasks should be in roadmap or todo." — feedback
  log 2026-09-14T15:30:25+02:00 (restatement of AGENTS.md § Definition of
  Done). Both went into today's retro (project rules applied, harness
  proposals queued).

## Agent notes (unverified)

- Changelog drafts needed corrections again (Phase 2's security line,
  Phase 5's agents line, the harness entry's "backport"/round-2 claims,
  the release entry's "94/94" summary wording); `.agents/rules/changelog-brief.md`
  and the queued harness proposal are the response.
- Coverage gaps named, not closed: nothing tests `main()`'s guard wiring
  (only the extracted `IPForwardOff`); the harness's sandbox CLI-start
  matrix was not run for its one-sentence `context/` change.
- Security residuals recorded, unchanged: `npm audit` 1 high in a
  build-time dependency (js-yaml); the harness's macOS case-collision
  residual and the `.git`-file worktree pointer.

## Environment State

- Branch: main, clean after this handoff's commit, level with origin. Tag
  v1.33.0 on `21d8fd7`, pushed. styrman-harness at `66fdbef` (handoff) on
  `cceee54`, pushed.
- Production: https://crew.styrman.xyz on 1.33.0 (server, caddy, runtime),
  live 16:47:29 local; 0 projects, 0 microVMs.
- Scratch (gitignored): `verify-commit-{8aaa5e9,21d8fd7}.log`,
  `integration-v1.33.0.log`, `e2e-v1.33.0.log`, `unit-web-v1.33.0.log`,
  `updater-v1.33.0.log`, `allowed_signers`, stage lists.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-14-preflight-v1.33.0.md` — the gate,
  dispositions, the observed deploy, what was not checked.
- `.agents/docs/plans/2026-09-14-v1.33.0-close-out.md` — the ten items,
  all ticked; the Interfaces section is the design record.
- `.agents/docs/changes/2026-09-14-v1.33.0-release.md` — the release entry.
- `.agents/docs/retros/2026-09-14-retro.md` — findings and proposals.

## Next Steps

1. Operator, live proof of BUG-060/BUG-054 (only a restart can produce
   the condition): create a project, stop it, let the server restart
   (next release, a reboot, or `sudo deploy/styrman-ctl`), open it from
   the UI — first terminal shows no "credentials: the broker did not
   answer" line and `gh auth status` succeeds. Record the result in the
   v1.33.0 gate report's Deploy section.
2. Operator: a fresh Google sign-in showing 1.33.0 in Settings; start a
   project and see `agent harness deployed (version 1.33.0, styrman-harness
   cceee54 …)` in its boot log.
3. Operator: `docker login` as your user for the gate/suites if the Hub
   budget bites again (`.agents/docs/deploy/remote.md` "Docker Hub"),
   then record the date there.
4. Harness session: `/retro --harness` for the two queued proposals; the
   sandbox CLI-start matrix on cceee54 if wanted; argolis-devbox's mirror
   refresh to cceee54 in its own session.
