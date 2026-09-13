# Handoff — 2026-09-13 — v1.29.0 live (Browser view review fixes)

Session ID: f720d73c-deb2-46f1-8f4f-e7f3eccbb654
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator asked for a `/code-review` of the v1.27.1..v1.28.0 Browser
view with "make sure the code is excellent and works super optimized. When
done if you need changes do a minor and execute a /preflight and then git
commit push". The review returned 15 findings; all are fixed, six further
review rounds on the fix diff are closed, and v1.29.0 is deployed.

## Completed

- Plan `.agents/docs/plans/2026-09-13-browser-review-fixes.md` — all boxes
  ticked; three cold-read rounds (cap reached; the leftovers were
  implementation details, decided in the plan's "Cold-read trail").
- Commits `c4128f5` (the 15 findings), `211b2f8` (round 2), `2a501ae`,
  `bac799f`, `1318e79`, `b6a287d` (rounds 3–6, all on the launcher's
  forwarder ownership check — see the gate report's "On the mechanism that
  took five rounds"), `b6bc2bf` (release commit), `855bbc3` (gate report
  records verify-commit OK).
- Tag `v1.29.0` on `b6bc2bf`, SSH-signed, `git tag -v` good against
  `/etc/styrman/allowed_signers`. Auto-updater tick at 10:23: images built,
  containers recreated 10:24:03, "[styrman-update] OK — v1.29.0 is live"
  10:24:18. `sudo bash deploy/styrman-ctl status`: server healthy on
  `styrman-server:1.29.0` / `styrman-caddy:1.29.0`, `STYRMAN_IMAGE=styrman-runtime:1.29.0`,
  deployed-version v1.29.0 sha b6bc2bf, firewall timer active, verdict OK.
  In-container `GET /api/health` = 200.
- Records: release entry `.agents/docs/changes/2026-09-13-v1.29.0-release.md`;
  task entry `2026-09-13-browser-review-fixes.md`; ADR-068 amendment;
  BUG-042..053; gate report `.agents/docs/reviews/2026-09-13-preflight-v1.29.0.md`
  (SHIP); verifier logs `testing/2026-09-13-verify-{browser-review-fixes,v1.29.0}.md`;
  tier logs `testing/2026-09-13-{integration-v1.29.0,integration-v1.29.0-r5,e2e-v1.29.0}.log.md`;
  reviews `reviews/2026-09-13-{review,security}-browser-review-fixes*.md`.
- Docs: specs (terminal-protocol, guide, api, runtime-image), testing
  README (counts: vitest 171/13 files, integration 91/0/3, e2e 92/0/2),
  deploy/README.md, ROADMAP.md "Now" (v1.29.0 shipped).

## Open / In Progress

Nothing owed by the agent. Two checks stay the operator's, not blocking:

- Settings page shows 1.29.0 after a fresh sign-in (versioning.md step 5:
  a fresh OAuth login, not an existing session, is the caddy-security proof).
- Open a project and see it boot on `styrman-runtime:1.29.0` (the tiers
  booted projects on the same tree's image; production's front door is the
  operator's session). Also the two older ones from v1.28.0: the manual
  agent walk in the Browser view and a fresh-host `styrman-ctl install`.

TODO.md Inbox (deferred work, dated 2026-09-13): a Stop control in the
pane (needs a scope decision — it kills the browser the agents share), a
`liveUnknown` flag on the terminals listing, a cap on harness re-seed
attempts.

## Key Decisions

- A Stop control was struck from this release (cold-read round 1): a new
  destructive user-facing verb, not a review fix — BUG-052.
- The cold-read cap (three rounds) was reached with only implementation
  details left; they were decided in the plan under the operator's
  autonomous instruction rather than asked. Flagged here for review: the
  plan's "Cold-read trail" lists all seven.
- `forwarder_up` (launcher) was kept, not deleted, after five rounds on
  the same mechanism: `start` runs on every relay connect and must be
  idempotent (a respawned socat drops the other panes). Final form reads
  the listening socket's inode from `/proc/net/tcp{,6}` and matches it in
  `/proc/<pid>/fd` — no `ss` output parsed.
- Unused SPA wrappers `api.browserStatus`/`api.browserStop` deleted; the
  REST routes stay (operator API, integration tier).

## Corrections

None logged this session (no feedback.jsonl entry; the operator's messages
were status questions: "is it deployed yet?", "What is left?").

## Agent notes (unverified)

- The changelog agent invented evidence in the release-entry draft for the
  second release in a row (BUG-034 → BUG-053). The bug record proposes
  graduating the rule into the agent's definition on a third recurrence;
  a `/retro` would be the place to decide it now.
- The harness killed one background Bash task "for low memory" while the
  host had 60 GB free (during a vite build); a feedback draft is queued.
  Workaround used: build the SPA in the foreground, then run e2e with
  `STYRMAN_E2E_SKIP_WEB=1`.
- The pane's `revoked` phase (close 4003) has no automated test: the e2e
  runs in AuthMode=none, where the re-check is nil by design.

## Environment State

- Branch: main at the handoff commit (this file + ROADMAP), clean; level
  with origin. Tag v1.29.0 pushed.
- Production: https://crew.styrman.xyz on 1.29.0 (all three images).
- Running processes: none left by the session; the e2e/integration
  scratch dirs are cleaned (0 `/tmp/styrman-e2e-*`).
- `styrman-runtime:test` was rebuilt today from the release tree.
- gcloud login on the host kept (operator: "You can keep the login").

## Artifacts to Read

- `.agents/docs/reviews/2026-09-13-preflight-v1.29.0.md` — the gate, review
  coverage table, the mechanism note, what was not checked.
- `.agents/docs/changes/2026-09-13-v1.29.0-release.md` — what changed for a
  user, the commit list, the evidence paths.
- `.agents/docs/plans/2026-09-13-browser-review-fixes.md` — findings → fixes
  table and the cold-read trail.
- `.agents/docs/bugs/INDEX.md` — BUG-042..053.

## Suggested Skills

- `/retro` — one process bug recurred (changelog inventing evidence) and
  one review-loop lesson (six rounds on one launcher check) are retro input.

## Next Steps

1. Operator: fresh sign-in, Settings shows 1.29.0; open a project, confirm
   it boots on `styrman-runtime:1.29.0`; the Browser view agent walk.
2. Nothing else is queued. New work starts from TODO.md's three Inbox items
   or a new request.
