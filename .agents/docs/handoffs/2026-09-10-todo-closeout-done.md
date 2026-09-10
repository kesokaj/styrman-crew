---
owner: main
date: 2026-09-10
---
# Handoff — 2026-09-10 — the TODO close-out is done and shipped as v1.27.0

Session ID: f720d73c (Claude Code). Project: /home/kesokaj/workspace/styrman-crew.
Branch: main. Released as **`v1.27.0`** at 13:30 (release commit `face4ee`,
gated commit `0201b8c`; gate `reviews/2026-09-10-preflight-v1.27.0.md`,
VERDICT: SHIP). The updater's 13:30:58 tick built all three images; the
server was healthy at 13:32:01; firewall chains unchanged. Five phase
commits: `7dae197`, `3ce8a37`, `46f3ec2`, `0201b8c`, then the trail.

**Supersedes `2026-09-10-v1.26.0-shipped.md`.**

## Context

The operator: "Plan ALL todo items and make one big fix for all that is
left or remove from todo / roadmap" → a numbered per-item proposal → "go
all" → "Strike both" on the ingress pair after the plan's third cold-reader
round. Plan `plans/2026-09-10-todo-closeout.md` (28 boxes, all ticked);
recon brief and the three-round cold-read report beside it.

## Completed

- **Struck with reasons (12):** vulnerability-scan notes ×2, the deleted
  firewall LAN section, ADR-065 follow-ups, v1.22.0 coverage tracing, the
  transient e2e 404, accepted harness lows, the un-nesting ADR (done by
  ADR-064), the harness-repo items (the operator carries them there), the
  bearer-token note, and the ingress pair (redirect trust cannot be narrowed
  under this domain layout; `caddy validate` in the gate declined).
- **Fixed (17 items, five phases):** Phase 1 server — reply written after
  the lock, key gate tested through the handler, full key-type enum and a
  sanitised comment, two-sample unmark in a goroutine keyed by stop time,
  legacy workspace migration deleted, gitconfig parsed in Go through a
  confined read. Phase 2 web — `\p{C}` key hygiene, byte-stable build
  (`u:0`), abortable checks with the timeout kept, gutter only for
  checkable files (+ e2e drift guard), Discard discards, `projectBadge`,
  `provisioning` state. Phase 3 deploy — eight ingress deps converged
  (divergence list now EMPTY in the gate), timer flag, temp files out of the
  workspace, tofu label + parser, image freshness script (refuses production
  tags), Docker address pool, boot-time firewall unit + 5 s tick (ADR-067
  amendment). Phase 4 — terminal re-authorization every 60 s, close 4003.
  Phase 5 — `dockerAPI` seam, fake, the orchestrator's first ten unit tests
  (BUG-003's residual closed).
- **Reviews:** reviewer and security per phase (11 reports), findings fixed
  or rebutted in each report's Response; the cold reader's three rounds.
- **Verification on the committed tree:** `verify-commit.sh 0201b8c` "safe
  to tag", 0 WARN; go test -race all packages; vitest 11/147; ingress tests;
  integration 84/0/3; e2e 87/0/2 exit 0, no leaks.
- **Records:** BUG-034..038; TODO.md has ONE open item (the operator's
  backups clean-up); ROADMAP "Now" says so; docs updated (api.md, guide.md,
  deploy/remote.md, testing README).

## Open

1. **Operator, now (v1.27.0 is deployed):** `sudo bash deploy/firewall/install.sh`
   — this ENABLES the boot-time unit. Checked on the live host after both
   post-deploy updater ticks (13:31 and 13:36): `systemctl is-enabled
   styrman-firewall.service` → `disabled`. Cause: the updater's converge
   step enabled it only when a unit file changed, and the tick that
   installed the unit ran the previous updater (BUG-035 recurrence; the
   enable is now unconditional in the working tree, ships next tag; rule
   graduated to `.agents/rules/mechanism-reaches-every-caller.md`). Until the
   operator runs it, only the timer's 5 s first tick narrows the boot
   window. Then `sudo bash deploy/host-setup.sh` (address pool; restarts
   Docker, live-restore keeps VMs up). Verify: `is-enabled` → `enabled`.
2. ~~Operator, TODO's last item: the backups clean-up~~ — **verified gone
   2026-09-10** with the operator's Google Cloud login on this host: no
   such bucket (404), only the default compute service account, no other
   project, and `/var/lib/styrman/backups/` absent. TODO.md has no open
   item. The Google Cloud CLI now lives under `~/google-cloud-sdk` with the
   operator's user credentials in `~/.config/gcloud` — `gcloud auth revoke`
   if the host should not keep them.
3. **Owed (agent):** the credential-broker reachability test from a LAN
   project on the compose deployment (`reviews/2026-09-09-security-adr-067.md`).
4. **Released** as v1.27.0 (MINOR). A fresh Google sign-in after the bump
   is the operator's check. The updater fix above (unconditional enable) is
   the one unreleased source change on the branch.
5. BUG-038: the e2e runner once exited 2 after a green suite with a bash
   parse message from its trap; the traced rerun was clean. Observed, not
   root-caused.
6. Noted gaps, not TODO: the create-path `realBelow` re-check has no unit
   test; the fake is minimal (documented); the `serve` goroutine and the
   frontend revoked state have no unit test.

## Key Decisions

- Item J struck by the operator at the three-round cap (gate honoured).
- Terminal revocation only, by design (projects keep running for a removed
  user until an admin stops them) — in the guide.
- Phases 2 and 4 in one commit (shared files); stated in the message.
- Commit trailers: none (AGENTS.md's git rule).

## Corrections

- None from the operator this session beyond the decisions above.

## Agent notes

- Four agents hit turn limits (a reviewer, a security reviewer, the docs
  agent, a changelog agent); each was resumed with SendMessage and finished.
- The changelog agent's ADR amendment claimed a measurement never made
  (BUG-034); caught before commit and rewritten.
- The Phase 2/3 review artifacts were snapshots that went stale while later
  phases moved the tree; reviewers read the live files and said so.

## Environment State

- `main` pushed through `face4ee` (tag `v1.27.0`) + this trail. Production
  `styrman-server:1.27.0` healthy, `styrman-caddy:1.27.0`,
  `styrman-runtime:1.27.0` built. `styrman-runtime:test` current (2026-09-09). No leaked test
  containers, networks or state dirs. `styrman-caddy:verify` image exists
  locally from the Phase 3 build (harmless; `docker rmi` if unwanted).

## Artifacts to Read

- `plans/2026-09-10-todo-closeout.md`, `reviews/2026-09-10-coldread-todo-closeout.md`
- `changes/2026-09-10-closeout-phase{1-server,2-web,3-deploy,4-terminal-reauth,5-docker-seam}.md`
- `testing/2026-09-10-verify-closeout-final.md`
- `bugs/BUG-034` … `BUG-038`

## Next Steps

1. Operator: `install.sh` (enables the boot unit), `host-setup.sh`; the
   app sign-in after v1.27.0 (done per the operator; key row and
   provisioning chip to eyeball).
2. Agent: the broker-reachability test on the compose deployment; the
   updater's unconditional enable ships with the next tag.
