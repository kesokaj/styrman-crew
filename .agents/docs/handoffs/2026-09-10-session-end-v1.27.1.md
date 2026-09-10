---
owner: main
date: 2026-09-10
---
# Handoff — 2026-09-10 — session end: v1.27.1 live, TODO empty, nothing queued

Session ID: f720d73c (Claude Code). Project: /home/kesokaj/workspace/styrman-crew.
Branch: main, clean, pushed (last commit `c2c858f` + the trail commit that
carries this file). Production: `v1.27.1` (server, caddy, runtime).

**Supersedes `2026-09-10-todo-closeout-done.md`** (which keeps the detail of
the close-out; this one is the resume point).

## Context

One long session, 2026-09-10: closed the v1.25.0 handoff's operator items,
restructured the network-mode PUT (v1.26.0), showed the ssh public key in
Settings (v1.26.0), then the operator's "Plan ALL todo items and make one
big fix" → the TODO close-out (v1.27.0), then the updater fix (v1.27.1),
the post-upgrade host steps, and the broker reachability test on production.

## Completed (all shipped or on the trail)

- **Releases today:** v1.26.0 (11:07), v1.27.0 (13:31), v1.27.1 (14:38).
  Gates: `reviews/2026-09-10-preflight-v1.2{6.0,7.0,7.1}.md`.
- **TODO.md: 0 open items.** Twelve struck with reasons by the operator's
  decision; the rest fixed (plan `plans/2026-09-10-todo-closeout.md`, 27/27 — one box was removed when item J was struck).
- **ROADMAP "Now":** nothing open for the operator; "Next" holds nothing.
- **Host:** `install.sh` and `host-setup.sh` run by main as root (unit
  `enabled` after docker.service; Docker address pool 172.16.0.0/12);
  the v1.27.1 updater re-enables the unit every tick (proven live by
  disabling it and watching the next tick).
- **Broker reachability test** run on production with two users, all probes
  as expected: `testing/2026-09-10-broker-reachability-live.md`. Test user
  and its three leftover directories removed afterwards
  (`changes/2026-09-10-broker-test-user-leftovers-removed.md`).
- **Backups clean-up** verified already gone (bucket 404, no backup service
  account, no host archive) with the operator's Google Cloud login; the CLI
  stays installed at `~/google-cloud-sdk` with that login (operator's choice).
- **Bug records:** BUG-032 … BUG-038 (`bugs/INDEX.md`); rule graduated:
  `.agents/rules/mechanism-reaches-every-caller.md`.
- `.claude/settings.json`: Bash allowed without prompting in this project
  (operator's instruction).

## Open / In Progress

- Nothing owed. Observed-only: BUG-038 (the e2e runner's one-off exit 2 with
  a bash parse message; the traced rerun was clean).
- Noted gaps, deliberately not TODO items: the create-path `realBelow`
  re-check has no unit test; the fake Docker client is minimal (documented
  in it); the terminal `serve` goroutine and the pane's revoked state have
  no unit test.
- The operator signed in after v1.27.0 and raised nothing. That is not a
  sign-off on the "provisioning" chip or the key row, and none was asked
  for: the check is optional, not owed.
- BUG-038 stays an observation, not a task: the operator struck the same
  class (a one-off, non-reproducing e2e blip) during the close-out. Only a
  recurrence reopens it.
- The noted test gaps below are accepted: the operator's "go all" approved
  the plan that listed them as gaps, and the phase reviews recorded them.

## Key Decisions

- sshd keeps password login on this host — the operator's decision;
  `deploy/remote.md` keeps the hardening procedure as advice.
- Item J (ingress redirect trust, `caddy validate` gate) struck at the
  cold-reader's three-round cap — the operator's "Strike both".
- Terminal revocation is terminal-only by design (guide says so).
- Versions: v1.26.0 and v1.27.0 MINOR (new behaviour), v1.27.1 PATCH.
- No commit trailers (AGENTS.md's git rule over the harness attribution).

## Corrections

- "Im ok with password login, dont fuss with ssh-key. It does not matter."
  — feedback log 2026-09-10 (the sshd override entry).

## Agent notes (unverified)

- Four agents hit turn limits today and were resumed with SendMessage; two
  review artifacts went stale while later phases moved the tree — reviewers
  read the live files and said so. Not a rule, an observation.
- The changelog agent once wrote an ADR sentence claiming a measurement never
  made (BUG-034); main caught it before commit. Read agent-written ADR text
  before committing it.

## Environment State

- Branch main, clean after this trail commit, pushed. Tags v1.26.0, v1.27.0,
  v1.27.1 signed and pushed.
- Production healthy on 1.27.1; firewall chains unchanged (8 rules in
  STYRMAN-FWD); `styrman-firewall.service` enabled; no leaked test
  containers, networks or state dirs; `styrman-caddy:verify` local image
  from a Phase 3 build (harmless).
- Tooling: `~/google-cloud-sdk` (logged in as the operator), goimports in
  `~/go/bin`.

## Artifacts to Read

- `.agents/docs/handoffs/2026-09-10-todo-closeout-done.md` — the close-out's
  detail (phases, reviews, what each fixed).
- `.agents/docs/plans/2026-09-10-todo-closeout.md` — the plan, all ticked.
- `.agents/docs/reviews/2026-09-10-preflight-v1.27.1.md` — newest gate.
- `.agents/docs/testing/2026-09-10-broker-reachability-live.md` — the test.
- `.agents/docs/bugs/INDEX.md` — BUG-032..038.

## Suggested Skills

- `preflight` — for the next release, whenever there is one.
- `feature` / `bug` — for whatever the operator brings next; nothing is queued.

## Next Steps

1. Nothing is queued. Wait for the operator's next request.
2. If the operator wants a check: open the app, create a project from a
   clone URL and watch the "provisioning" chip; the Settings key row.
