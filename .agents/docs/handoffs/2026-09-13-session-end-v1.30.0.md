# Handoff — 2026-09-13 — v1.30.0 (audit fixes and image supply-chain pins)

Session ID: f720d73c-deb2-46f1-8f4f-e7f3eccbb654
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

After v1.29.0 shipped, the operator asked for "a full /audit … there are
plenty of fixes create a minor and use /preflight then commit and push".
The audit (a 31-agent workflow plus refuters) found two highs — one live in
production since 2026-09-10 (BUG-054: no brokered credentials in login
shells on /24 bridges) — and fourteen mediums; all are fixed and shipped as
v1.30.0.

## Completed

- Audit: `.agents/docs/reviews/2026-09-13-audit-project.md` (summary and
  detail tables; links to 11 review + 5 security artifacts, the inventory,
  the verify log); BUG-054 and BUG-055 recorded.
- Plan `.agents/docs/plans/2026-09-13-audit-fixes.md` — every box ticked;
  three cold-read rounds (cap reached; five one-answer leftovers decided in
  the plan's "Cold-read trail", listed below for the operator).
- Commits `60741b7` (phase 1), `e18d62c` (phases 2–4), `25bdae8` (docs,
  records), `e6ce8c4` (reviewer round), `a002f69` (security round),
  `1e8aa91` (dist + gate draft), `c521a74` (SHIP verdict), `5393ef5`
  (release commit), `ac938de` (verify-commit recorded).
- Tag `v1.30.0` on `5393ef5`, SSH-signed, `git tag -v` good against
  `/etc/styrman/allowed_signers`, pushed 12:51. Auto-updater tick 12:53:
  "v1.30.0 signature OK (allowed signer)", images built 12:54:27,
  containers recreated 12:54:37, "[styrman-update] OK — v1.30.0 is live"
  12:54:43. `sudo bash deploy/styrman-ctl status` 12:55: server healthy on
  `styrman-server:1.30.0` / `styrman-caddy:1.30.0`, deployed-version
  v1.30.0 sha 5393ef5, verdict OK; in-container `GET /api/health` = 200;
  `styrman-runtime:1.30.0` answers `go version go1.27.1`, `v22.23.2`,
  `2.1.270 (Claude Code)`, `agy 1.2.2`, and carries the `/proc/net/tcp`
  launcher and the recompute-free credential snippet.
- Records: release entry `.agents/docs/changes/2026-09-13-v1.30.0-release.md`;
  task entry `2026-09-13-audit-and-fixes.md`; ADR-069 (image supply chain +
  broker-address contract) and the ADR-030 amendment (inner-dockerd
  workloads share the project's identity) appended to DECISIONS.md; gate
  report `.agents/docs/reviews/2026-09-13-preflight-v1.30.0.md` (SHIP;
  verify-commit OK appended); verifier logs
  `testing/2026-09-13-{audit-verify,verify-audit-fixes}.md`; tier logs
  `testing/2026-09-13-{integration,e2e}-v1.30.0.log.md` (91/0/3, 92/0/2);
  reviews `reviews/2026-09-13-{review,security}-audit-fixes.md`.
- Docs: `specs/styrman/{api,runtime-image,guide}.md`, testing README
  (vitest 177/15 files), deploy/README.md, ROADMAP.md, TODO.md (nine
  recorded lows as Inbox items).

## Open / In Progress

- **BUG-054's production proof is the operator's:** open (or create) a
  project on v1.30.0 and, in a terminal, run `env | grep -c GH_TOKEN`
  (expect ≥ 1) or `gh auth status`; a login shell whose broker did not
  answer now prints a MOTD line naming the URL. The host-run test servers
  have no broker, so no tier could take this proof.
- v1.29.0's two operator checks still stand (Settings shows the version
  after a fresh sign-in; a project boots on the new runtime tag) — now for
  1.30.0.
- TODO.md Inbox: three items from v1.29.0 (Stop control, `liveUnknown`,
  harness re-seed cap) and nine recorded audit lows dated 2026-09-13.

## Key Decisions

- The Antigravity CLI (`agy`) is NOT pinned in the image (ADR-069): Google
  publishes it latest-only behind a sha512-verified manifest and it
  self-updates at run time; Go, Node and claude-code are pinned by version
  and publisher sum; base images by digest. A changed upstream artifact now
  fails the image build loudly and the release rolls back.
- The credential snippet trusts the injected broker address; the server
  recreates (never reconnects) a stopped container whose network vanished,
  and only on a NotFound inspect.
- The format temp file moved back INTO the workspace (beside the edited
  file) for config discovery — a deliberate reversal of the 2026-09-01
  review low for format alone; check keeps `~/.styrman/tmp`.
- Cold-read cap (round 3) leftovers decided in the plan under the
  operator's autonomous instruction: `invalidateSelfAttach` kept; the
  updater's test verb `cd`s into the scratch repo; a RETURN trap (later
  made to read a script-scope path); marker rule "any probe failed → set,
  removed only when both succeed"; the overrides WARN only for a non-zero
  value.
- The tiers were not re-run for the ~60-line security-round commit (unit
  tests pin it; stated in the gate report as a judgment).

## Corrections

None logged this session (no feedback.jsonl entry; the operator's messages
were requests and status questions).

## Agent notes (unverified)

- The changelog agent invented evidence THREE times today (BUG-053 recorded
  the second; the third — invented variable names, an invented agent list
  and a self-referential "rewritten by the main thread" narrative in the
  v1.30.0 draft — was caught on read-through). BUG-053's rule says a third
  recurrence graduates into the agent's definition: a `/retro` should make
  the changelog agent refuse any verification line without a cited file.
- Two agents hit their turn limits and were resumed (the audit verifier
  and the security pass); the audit workflow's structured return failed for
  five of 18 scans (the artifacts existed; re-run by hand).
- The harness killed one background task "for low memory" with 60 GB free
  (a feedback draft is queued).

## Environment State

- Branch: main, clean, level with origin. Tag v1.30.0 pushed.
- Production: https://crew.styrman.xyz on 1.30.0 (all three images), deployed 12:54.
- `styrman-runtime:test` rebuilt today from the release tree (pinned
  fetches); scratch logs under `.agents/scratch/` (gitignored).
- gcloud login on the host kept (operator: "You can keep the login").

## Artifacts to Read

- `.agents/docs/reviews/2026-09-13-preflight-v1.30.0.md` — the gate.
- `.agents/docs/changes/2026-09-13-v1.30.0-release.md` — what changed.
- `.agents/docs/reviews/2026-09-13-audit-project.md` — the audit.
- `.agents/docs/plans/2026-09-13-audit-fixes.md` — findings → fixes, the
  cold-read trail.

## Suggested Skills

- `/retro` — the changelog agent's third invented-evidence recurrence and
  the turn-limit resumes are retro input.

## Next Steps

1. Operator: the BUG-054 proof and the two version checks above.
2. `/retro` when convenient.
3. Nothing else is queued; TODO.md holds the recorded lows.
