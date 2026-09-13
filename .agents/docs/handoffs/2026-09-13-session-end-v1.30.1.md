# Handoff — 2026-09-13 — v1.30.1 (close-out: TODO and ROADMAP empty)

Session ID: f720d73c-deb2-46f1-8f4f-e7f3eccbb654
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

After v1.30.0 the operator asked whether TODO and ROADMAP were empty; they
were not. A numbered go/strike list was presented; the operator approved
every recommendation ("If we pin stuff make it easy to upgrade. Do not
overcommit, it works. /preflight when done and push commit to git and make
sure the new versions runs and the documentation is up to date."). Two
items were fixed, ten struck with reasons (ADR-070), a pin-bump helper
added, and v1.30.1 (PATCH) cut.

## Completed

- Fixed: the firewall script's missing-`xt_physdev` state goes through
  `warn()` into its exit path and `styrman-ctl status` reports
  `enabled/failed` as DEGRADED; the installer and the firewall suite
  tolerate the oneshot's degraded exit (units still enabled); the guest
  entrypoint passes the harness version and stamp path as arguments.
- Added `image/bump-pins.sh` (check / `--apply`): refreshes the ADR-069
  pins from the publishers' own sources, shape-checks every value, rewrites
  beside the files and verifies per file; documented in
  `specs/styrman/runtime-image.md` and named in AGENTS.md.
- Struck (ADR-070, reasons per item): Stop control, `liveUnknown`, re-seed
  cap, `explorer.go` split, credstore host naming, session-expiry re-check,
  the REQUIREMENTS.md dependency line, host-setup nits, js-yaml,
  FileExplorer polling, two cosmetic Go cleanups.
- TODO.md Inbox empty; ROADMAP "Now" empty (the standing rule, ADR-045).
- Commits `336a5d1`, `441507e`, `48d930b`, `a1e35e6`, `175abe2`, `5a4dcde`
  (release commit). Gate `.agents/docs/reviews/2026-09-13-preflight-v1.30.1.md`
  (SHIP): reviewer and security in two rounds (converging on the same
  three real items, all fixed), verifier PASS through `a1e35e6`, tiers on
  the rebuilt image — integration 92/0/3 (new check: the harness stamp
  equals the image version), e2e 92/0/2, the firewall suite `PASS=26
  FAIL=0` under `unshare -Urnm`.
- Release: `deploy/verify-commit.sh 5a4dcde` failed once at 13:33 on Docker
  Hub's anonymous pull rate limit (429 on the pinned base; BUG-056 — no tag
  was pushed into it), passed at 14:04 after the window returned; tag
  `v1.30.1` on `5a4dcde`, SSH-signed, `git tag -v` good against
  `/etc/styrman/allowed_signers`, pushed 14:04. Updater tick 14:07:
  "signature OK (allowed signer)", containers recreated 14:08:07,
  "[styrman-update] OK — v1.30.1 is live" 14:08:18. `styrman-ctl status`
  14:08: server healthy on `styrman-server:1.30.1` / `styrman-caddy:1.30.1`,
  deployed sha 5a4dcde, firewall service enabled/inactive (a good oneshot
  run) + timer active, verdict OK; in-container `/api/health` 200;
  `styrman-runtime:1.30.1` answers `go1.27.1`, `v22.23.2`, `2.1.270
  (Claude Code)`, `agy 1.2.2` and carries the argument-form stamp write;
  the converged host firewall script carries the `warn` form.

## Open / In Progress

- Operator checks (outside the roadmap, not blocking): BUG-054's production
  proof — open a project on the current release and run `gh auth status`
  (or `env | grep -c GH_TOKEN`) in a terminal; a fresh sign-in showing the
  current version in Settings.
- Nothing is queued in TODO.md or ROADMAP.md.

## Key Decisions

- Pins stay by version + publisher sum with the helper for bumps; the
  trust model is TLS plus the publishers' sums (no signature check, npm
  publishes no sum) — stated in the helper and the spec, recorded in the
  gate.
- The firewall oneshot exits 1 when degraded; callers that must not abort
  (installer, suite) tolerate it; `styrman-ctl` and systemd show it.

## Corrections

None logged (no feedback.jsonl entry; the operator's messages were
approvals and status questions).

## Agent notes (unverified)

- The changelog agent's release-entry draft needed corrections again
  (range, commit list, two claims) — the fourth read-through correction
  today; the `/retro` item from the v1.30.0 handoff stands.
- Docker Hub's anonymous rate limit (100 pulls/hour/IP) blocked
  `verify-commit` for 30 minutes and could equally block the auto-updater's
  own build on a busy day (BUG-056). A `docker login` on this host with any
  Docker Hub account lifts the limit; the operator's call — record it in
  `.agents/docs/deploy/remote.md` when decided.

## Environment State

- Branch: main, clean, level with origin. Tag v1.30.1 on 5a4dcde, pushed.
- Production: https://crew.styrman.xyz on 1.30.1 (all three images), deployed 14:08.
- `styrman-runtime:test` rebuilt from the close-out tree.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-13-preflight-v1.30.1.md` — the gate.
- `.agents/docs/changes/2026-09-13-v1.30.1-release.md` — what changed.
- ADR-070 in `.agents/docs/architecture/DECISIONS.md` — fixed vs struck.

## Suggested Skills

- `/retro` — four invented-evidence corrections of the changelog agent in
  one day, and two agent turn-limit resumes.

## Next Steps

1. Operator: the two checks above.
2. `/retro`.
3. Nothing else is queued.
