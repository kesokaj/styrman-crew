---
owner: main
date: 2026-09-10
---
# Handoff — 2026-09-10 — the TODO close-out is done; unreleased

Session ID: f720d73c (Claude Code). Project: /home/kesokaj/workspace/styrman-crew.
Branch: main. Last release `v1.26.0` (11:07 today). Five phase commits since:
`7dae197`, `3ce8a37`, `46f3ec2`, `0201b8c`, plus this trail commit.

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

1. **Operator, after the next tag:** `sudo bash deploy/firewall/install.sh`
   (enables the boot-time unit; until then only the 5 s tick narrows the boot
   window) and `sudo bash deploy/host-setup.sh` (address pool; restarts
   Docker, live-restore keeps VMs up). Verify: `systemctl is-enabled
   styrman-firewall.service` → enabled.
2. **Operator, TODO's last item:** delete the GCS backup bucket + service
   account; shred `/var/lib/styrman/backups/`.
3. **Owed (agent):** the credential-broker reachability test from a LAN
   project on the compose deployment (`reviews/2026-09-09-security-adr-067.md`).
4. **Release:** everything above is unreleased. `/preflight` when the
   operator says so; MINOR (new behaviour: provisioning state, 4003,
   checkable-only gutter; nothing broken).
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

- `main` pushed through `0201b8c` + this commit. Production still
  `v1.26.0`. `styrman-runtime:test` current (2026-09-09). No leaked test
  containers, networks or state dirs. `styrman-caddy:verify` image exists
  locally from the Phase 3 build (harmless; `docker rmi` if unwanted).

## Artifacts to Read

- `plans/2026-09-10-todo-closeout.md`, `reviews/2026-09-10-coldread-todo-closeout.md`
- `changes/2026-09-10-closeout-phase{1-server,2-web,3-deploy,4-terminal-reauth,5-docker-seam}.md`
- `testing/2026-09-10-verify-closeout-final.md`
- `bugs/BUG-034` … `BUG-038`

## Next Steps

1. Operator: say "ship it" for a MINOR release; then the two root commands.
2. Agent: the broker-reachability test on the compose deployment.
