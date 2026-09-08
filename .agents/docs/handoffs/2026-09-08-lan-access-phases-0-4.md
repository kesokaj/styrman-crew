# Handoff — 2026-09-08 — per-project LAN access, Phases 0–4 shipped on main, Phase 5 open

Session ID: 01Y4pGqaiDtJG3vxeEUMvtS3
Project: /home/kesokaj/workspace/styrman-crew
Branch: main, clean, fully pushed (HEAD `f86fe08`)

## Context

The operator asked for a per-project, runtime-switchable toggle that lets a
project's microVM reach the local network (ADR-065). Research, plan, the
R12 amendment and four implementation phases landed this session; the
release and the operator's root actions on the host did not.

## Completed

- Plan + research: `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-08-per-project-lan-access.md` (`d7a00d2`); cold-reader passed round two.
- Phase 0: ADR-065 + the R12 amendment (`9aff5d6`, approval = the operator's "Go do work" to the seven decisions D1–D7, quoted in the ADR amendment entry); doc drift fixes (`dbe66a8`).
- Phase 1 host half (`893bf03`): `deploy/firewall/{styrman-firewall.sh,.service,.timer,.path,install.sh,test.sh}`, host-setup §3b → §5c, the updater's `host_converge()`, verify-commit gates, `internal/netpolicy` stub + netns wrapper test (83 checks).
- Phase 2 server half (`4ebd5d9`): `internal/netpolicy` (markers, spool, status, Derive), `internal/httpapi/network.go` (GET/PUT `/api/projects/{id}/network`), store/config/main wiring, compose bind (`rslave`), integration.sh block.
- Phase 3 UI (`03fedc8`): `web/src/components/LocalNetwork.tsx` in the Ports tab, `LanBadge.tsx`, `lib/lan.ts` (+11 vitest), Settings "Host firewall", e2e checks, rebuilt `web/dist`.
- Phase 4 docs (`3aa7acf`) and the DoD records (`f86fe08`: verifier log, ROADMAP "Now", local.md).
- Gates on `3aa7acf`: `deploy/verify-commit.sh` OK; go race tests ok; vitest 80/80; integration 78/0/4 (twice); e2e 84/0/2; verifier PASS, 0 FAIL rows.
- Reviews: four passes (reviewer ×2, security ×2), 19 findings — all fixed except three rebutted in writing; BUG-008..BUG-019 recorded.

## Open / In Progress

- **Production is unchanged as of this handoff.** The auto-mode classifier blocked the agent's `sudo` on the host twice; nothing root-side was run. Still true when written: the live `/usr/local/sbin/styrman-firewall.sh` (2026-08-30) has FOUR drops, the repo six (100.64/10, 198.18/15). A later session tells whether the operator has since acted with two read-only probes: `sudo -n iptables -S STYRMAN-FWD | grep -c -- '-j DROP'` (4 = drift still live, 6 = converged) and `cat /run/styrman-firewall/status` (absent = host half not installed; `version=3` = installed).
- **Phase 5 (operator):** release via `/preflight`; then once the updater has deployed the tag, `sudo bash /opt/styrman/deploy/firewall/install.sh` (installs the host half, discovers `192.168.86.0/24 via enp114s0`, mounts the spool tmpfs, and converges the drift); then the manual applied-state check (Ports tab → on → chip pending → on; in-guest `ping 192.168.86.1` succeeds, `ping 192.168.86.58` fails; off cuts it within a second) logged into `.agents/docs/testing/`. The opt-in `STYRMAN_IT_LIVE_LAN=1 bash .agents/docs/testing/styrman/integration.sh` block automates that proof once the host half is installed.
- The plan's remaining unticked boxes (Phase 0 converge, Phase 1 converge-this-host, Phase 3 manual check, Phase 5) are exactly the items above.
- Follow-ups recorded in `TODO.md` (ADR-065 (a)–(g)): `lanPolicy` knob, the two reserved conf keys need their own ADR, LAN DNS, `default-address-pools` pin, live-restore check, base-before-docker, the transitive SSH-back path.

## Key Decisions

- Root decides what "local network" means (root-owned `/etc/styrman/firewall.conf`, discovered once by `install.sh`); the non-root server only says on/off per project via a name-only marker file and a spool kick; root writes a status file the UI derives from. Rejected: a privileged helper container, a root daemon, Docker-native options, PathModified triggers (events lost while the oneshot runs — measured).
- One head-of-chain RETURN per (project, prefix), pinned `-i bridge -o uplink -s bridge-subnet -d cidr`; RETURN not ACCEPT (keeps Docker's cross-bridge DROP); deletions by full spec, never by rule number.
- Enable is admin-only on the admin's own project; disable never refused; host itself and published ports stay closed (D3); LAN prefix reaches admins only.
- Fail-closed everywhere: base rules first, unreadable status with a record on reads "cannot confirm" (never "unavailable"), an empty `STYRMAN_FIREWALL_DIR` removes every marker.
- The `changelog` agent's Phase 2–4 drafts invented files, behaviours and review outcomes; the author rewrote them against `git diff`. Its output must be read line by line before commit (a `SendFeedback` draft was queued for `/feedback`).

## Corrections

- None. No user correction this session; `~/.config/sh-agent/feedback.jsonl` has no 2026-09-08 entry (newest 2026-09-03T22:35:25).

## Agent notes (unverified)

- End-to-end LAN reachability from a real Kata VM through the RETURN + MASQUERADE path was NOT observed on this host (no root half installed); rule contents and chain traversal are verified, reachability is inferred until Phase 5's live check.
- Docker 29.7 + live-restore preserving `DOCKER-USER`/`STYRMAN-*` across a daemon restart is unverified (exposure ≤65 s LAN-closed).
- "Go do work" was taken as approval of the R12 amendment text verbatim; the ADR-065 amendment entry says so and how to revert if the operator disagrees.
- The operator asked to be told when to switch models; the final report said "switch now" (remaining work is routine).

## Environment State

- Branch: `main`, clean. `f86fe08` was HEAD when this was written; the commit that adds this handoff lands on top of it and is the only later commit from this session — anything beyond that is a later session's.
- Running processes: none from this session. Production containers `styrman-server`/`styrman-caddy` on v1.22.0, untouched.
- Pending: nothing uncommitted. Scratch under `/tmp/claude-1000/.../scratchpad/` is disposable (logs of the integration/e2e/verify-commit runs live there, cited in the verifier log).

## Artifacts to Read

- `/home/kesokaj/workspace/styrman-crew/.agents/docs/plans/2026-09-08-per-project-lan-access.md` — the design, interfaces, decisions D1–D7, phase boxes (unticked = remaining).
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/changes/2026-09-08-lan-access-phase{1,2,3,4}*.md` — what shipped per phase, reviews and fixes.
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/testing/2026-09-08-verify-lan-access.md` — the verifier's PASS log with the coverage trace and "not checked".
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/reviews/2026-09-08-security-lan-phase1.md` and `-phase2-3.md` — read before touching the firewall script or `network.go`.
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/bugs/INDEX.md` — BUG-008..019 are new.
- `/home/kesokaj/workspace/styrman-crew/.agents/docs/deploy/remote.md` — the install.sh step and verify commands for Phase 5.

## Suggested Skills

- `preflight` — the release is the next step.
- `bash` — if the operator's install.sh run surfaces anything host-side.
- dispatch the `reviewer`/`security` agents for any change to `deploy/firewall/` or `internal/httpapi/network.go`.

## Next Steps

1. Operator: `/preflight` to release; then `sudo bash /opt/styrman/deploy/firewall/install.sh`; confirm `cat /run/styrman-firewall/status` shows `version=3` and `sudo iptables -S STYRMAN-FWD` shows six DROPs.
2. Manual applied-state check on production (plan Phase 3) — or `STYRMAN_IT_LIVE_LAN=1 bash .agents/docs/testing/styrman/integration.sh styrman-runtime:<version>` — and log it into `.agents/docs/testing/`; tick the plan's remaining boxes; changelog entry.
3. Decide the ADR-065 follow-ups in `TODO.md` when a real need appears (the `lanPolicy` knob first if a tenant needs self-serve).
