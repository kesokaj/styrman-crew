# Handoff — 2026-09-03 — BUG-001 fixed; harness replacement (scope B) Phases 1–4 committed; release awaits the operator

Session: Claude Code, resumed from `2026-09-03-harness-migration.md`, working on this repo.
Project: /home/kesokaj/workspace/styrman-crew
Branch: main, clean, pushed. HEAD `5e86c2a`.

## Context

The operator's start prompt (2026-09-03) ordered three items: (1) the "fail to fetch" bug through the `bug` skill; (2) replace the image-seeded harness with `sh-agent` from `styrman-harness` per the plan, cold-read first, with the operator's answers (D1 mirror at a recorded commit; D3 move never delete; minor bump, MINIMAL excluded; D5 roster repointed, `*.bak.*` filtered), one commit and changelog per phase, `/preflight` for the release; (3) ongoing: work by the new harness and fix old-way leftovers in passing. Mid-session directive, verbatim: "Remember that the new harness should fully replace the old. ALL old occurances of the old harness needs to go. Event settings.json files for gemnini and claude."

## Completed

- **BUG-001** (`c1a6eb9`): the ingress session token (caddy-security default 900 s) expires; the `/api/*` redirect chain is cross-origin so the SPA's `fetch()` rejected before the 2026-08-30 re-auth handling could fire. `request()` and `uploadFiles` now use `redirect: "manual"` and treat an opaque redirect as an expired session (one toast, one reload, capped); the counter clears only after 10 s of a healthy page, is shape-checked, and a corrupt record is overwritten. Six tests in `web/src/lib/api.session.test.ts`; two security rounds; `web/dist` rebuilt. Not yet released.
- **Plan cold-read** (`2f022f5`): reviewer's 3 medium / 4 low folded in with the operator's answers; the mid-session directive became D8 (no settings files seeded).
- **Phase 1** (`44a6f88`): `image/harness/` = mirror of `styrman-harness` at `6893e2218414617da2bb88259b905ca3882b2659` + `COMMIT`, `VENDORED.md`, `SHA256SUMS`; Dockerfile builder stage; `deploy/verify-commit.sh` checks the mirror's integrity/file list/symlinks, builds the runtime image from the commit and compares its `sh-agent list` counts, asserts both Go pins; `vulncheck.sh` scans the third module.
- **Phase 2** (`536d0b3`): entrypoint seeds with `sh-agent install` after `configs/harness-migrate.sh` moves the old harness (both `settings.json` files included) to `~/.styrman/harness-backup-<ts>/`; `timeout 20` each, git off the home's config; host fixtures and a real-VM upgrade path verified.
- **Phase 3** (`cd11dd0`): settings roster from the newest seeded project home; cards show "N skills · M subagents". Security: `files.Trust` tenant confinement (BUG-002), the janitor and the orchestrator's bind source guarded on create AND start (BUG-003); rule `.agents/rules/guest-writable-paths.md`.
- **Phase 4** (`5e86c2a`): `integration.sh` +5 harness checks (65), green on real Kata (62 passed, boot→ready 1.69 s); Tier 5, `runtime-image.md`, `image/README.md`, `versioning.md`, ROADMAP, TODO, ADR-062; docs cold-read clean.
- Records: `changes/2026-09-03-{fail-to-fetch-session-expiry,harness-replacement-phase1..4}.md`; `bugs/BUG-001..003`; reviews and testing logs named `…-harness-phase*` and `…-fail-to-fetch*`; `testing/2026-09-03-e2e-harness-upgrade-path.md`.

## Open / In Progress — the operator's decisions

1. **Release `v1.20.0` (Phase 5, `/preflight`) — blocked on the operator by the three-round rule.** The Phase 3 security review did not converge: round 3 found the bind-source check gated only container creation while Docker re-resolves a stored bind source on every start; fixed after round 3 (`projectHomeIsReal` at the top of `EnsureRunning`, test added), no fourth round run. Options: run `/preflight` (it dispatches a fresh reviewer + security pass over the whole release diff) or a targeted fourth round first. `deploy/verify-commit.sh 5e86c2a` result: `.agents/scratch/verify-commit-5e86c2a.log` (run at the end of the session; the Phase 2 commit passed it end to end).
2. **`REQUIREMENTS.md` R5** still says "vendored from `devshell_prompt/harness`"; proposed one-line edit: "The agent harness (a mirror of `styrman-harness` at `image/harness/COMMIT`, installed by `sh-agent`)". Immutable without the operator.
3. **Token lifetime** (`TODO.md`): 900 s default means a toast + reload every 15 minutes now that expiry is handled; the security review suggests `crypto default token lifetime 28800` deliberately.
4. **Un-nesting ADR** (`TODO.md`): project homes live inside the mounted base home; the residual check-to-Docker window closes only by moving them.
5. Manual Tier 5 step 4 (`claude -p` / `agy` inside a project) needs the operator's sign-in, on the released version.

## Key Decisions

- D1–D8 in the plan; ADR-062 appended. No settings files seeded for either tool (operator directive); Claude Code in a project runs in its default permission mode, as on the operator's own machine.
- Security fixes were kept in-phase (not deferred) because two were highs in shipped code paths; their class is now a project rule.

## Corrections

- Operator, mid-session: the old `settings.json` files go too (the plan's D3 had kept them with two keys stripped). Logged to `~/.config/sh-agent/feedback.jsonl`.

## Agent notes (unverified)

- The dev image tag `styrman-runtime:dev` is safe to rebuild: production boots `styrman-runtime:${STYRMAN_VERSION}` (1.19.1). A throwaway server on this host adopts production's containers via Docker labels (read-only in practice; the verifier stopped its live probe for that reason — `testing/2026-09-03-verify-harness-phase3-round2.md`).
- `sh-agent version` prints `(devel)` and the manifest commit is empty in the image (no `.git`); upstream items in `TODO.md`.

## Environment State

- Running: none started by this session (integration and upgrade-path servers were stopped and their state dirs removed). Production (`styrman-server`, `styrman-caddy`, the operator's project) untouched.
- Images: `styrman-runtime:dev` = the Phase 4 tree; `:test` built by the verifiers.

## Artifacts to Read

- `.agents/docs/plans/2026-09-03-image-harness-replacement.md` (ticked; progress notes at the end)
- `.agents/docs/reviews/2026-09-03-security-harness-phase3.md` (three rounds; the non-convergence)
- `.agents/docs/bugs/INDEX.md`, `.agents/rules/guest-writable-paths.md`
- `TODO.md` (the decisions), `ROADMAP.md` "Now"

## Next Steps

1. Operator: answer the five items above.
2. On a go: `/preflight` for `v1.20.0` (versioning.md release procedure; the tag deploys through the auto-updater), then Tier 5 step 4 on production.
3. Apply the R5 edit with ADR-062's reference on approval.
