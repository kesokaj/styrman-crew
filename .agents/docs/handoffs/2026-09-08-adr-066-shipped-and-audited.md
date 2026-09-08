---
owner: main
date: 2026-09-08
---
# Handoff — 2026-09-08 — ADR-066 shipped (v1.24.0), audited, fixed and re-released (v1.24.1)

Session ID: 67002801-03d2-4484-a6a0-e7d86ceaecc5
Project: /home/kesokaj/workspace/styrman-crew
Branch: main. HEAD `e0c9596` at the time of writing, tag `v1.24.1` live in
production. This handoff and a plan tick land in the commit that follows it —
so the state below describes the tree once that commit exists, which a
cold reader flagged as a self-reference worth stating rather than glossing.

**Supersedes `2026-09-08-v1.24.0-shipped.md` and `2026-09-08-adr-066-no-ship.md`.**
The second was written at a NO-SHIP verdict that was later cleared; it is kept
only for the finding history.

## Context

The operator flipped the v1.23.0 per-project LAN switch for the first time,
found the LAN gateway reachable but the host itself not, and restated the
intent: ON should mean "exactly the same network with full access". That
became ADR-066 — designed, built through six phases, gated, shipped as
v1.24.0, then audited whole-project, fixed, and re-released as v1.24.1.

## Completed

- **ADR-066 + two amendments** (`architecture/DECISIONS.md`): full
  same-network parity outbound, host and host-published ports each behind a
  root-owned conf opt-in that defaults closed; the credential latch. R12
  amended with the operator's explicit approval of the exact text.
- **Six phases**, plan `plans/2026-09-08-lan-full-parity.md` at **29 of 30 boxes** — the one
  unticked is the live published-port proof, which needs an operator opt-in.
- **v1.24.0 released and deployed**; gate `reviews/2026-09-08-preflight-v1.24.0.md`.
- **Whole-project audit** (`reviews/2026-09-08-audit-project.md`): no high
  findings, 11 medium, 11 low. Ten fixed with mutation-checked tests, one
  identified as a false finding, nine recorded as not-now with reasons.
- **v1.24.1 released and deployed** — the audit fixes, including a
  cross-tenant DoS. Server healthy, `deployed-version=v1.24.1`, no failed latch.
- Suites: firewall `test.sh` **128/0**, browser e2e 89/0/2-skipped, vitest 85,
  integration 79/0/4, both Go modules clean, `verify-commit.sh` green.

## Open / In Progress

- **THE feature has never been used.** No LAN allowance has ever been applied
  by the v4 script on real hardware, and the published-port path has never run
  live. This is the only claim in the gate report with no evidence behind it.
- **Blocked on one fact**: `install.sh --lan-allow-host 1` correctly refuses
  while this host accepts sshd password authentication (R12, now enforced in
  code). Hardening needs **the operator's OWN client public key** — the key on
  this host is the host's own, and hardening without theirs locks them out.
  Sequence: `deploy/remote.md`.
- The auto-mode classifier denied `sudo bash …/install.sh --lan-allow-host 1`
  twice on 2026-09-08, so the opt-in has never been attempted. The operator can
  run it directly or add a Bash permission rule.
- Nine audit findings deliberately not fixed, with reasons in the audit report
  — refactors and design decisions, chiefly splitting `internal/httpapi`
  (10,288 lines, 7 subsystems) and the credential-harvest import flow.
- `TODO.md`: eight shared dependencies still diverge between the two Go
  modules; `styrman-update` still has no real test harness; the e2e wrapper's
  exit code is unreliable.

## Key Decisions

- **Parity is OUTBOUND, not L2 membership.** True membership was rejected: it
  breaks the `sdb-*` naming every R12 rule keys on, breaks the broker's
  bridge-only listener, and would make two LAN-on projects L2 peers —
  contradicting R12's unconditional cross-project clause.
- **The credential withholding LATCHES.** The live-state question had no
  reliable answer; latching deletes it. See Corrections.
- **The published-port exclusion is a positive allowlist**, because
  `! -o sdb-+ ! -o <ingress>` cannot be written — iptables takes one
  `--out-interface` per rule (probed live).
- **v1.24.1 is a PATCH**, not a minor: fixes only, no new capability.
- **Deletion beat repair twice** — the latch and the rewritten sshd gate are
  both shorter than what they replaced and closed findings three rounds of
  patching had not.

## Corrections

- "I think there is a misunderstanding here. A project does not need to have its ssh key copied on the host. If enough if we have full network access." — feedback log 2026-09-08T22:59:58+02:00
- "If you fixed stuff this needs to be a minor." — feedback log 2026-09-08T22:59:58+02:00

## Agent notes (unverified)

- **Six subagent dispatches hit their turn limits** across this session and the
  previous one, several writing nothing. Mitigated by a harness change (agents
  now told to write their artifact early), but two still stalled afterwards —
  "write early" was read as "write the headings early". Budget for a resume.
- The audit's recurring shape — a guard correct where written, absent where
  not — was found in code predating ADR-066, so it looks like a property of
  the codebase rather than of that feature's development. Unproven inference.
- `govulncheck` reports both modules affected by GO-2026-4887/4883 with
  `Fixed in: N/A`. No upgrade exists; daemon-side Moby defects reached through
  `init` traces of the client SDK.

## Environment State

- Branch `main`, tags `v1.24.0` and `v1.24.1`. Clean and pushed as of the
  commit that carries this file; nothing else is outstanding.
- Production: `styrman-server:1.24.1` healthy, `styrman-caddy:1.24.1`, one
  project VM still on `styrman-runtime:1.23.0` (picks up 1.24.1 on next start).
- Firewall: script version 4, `lan_allow_host=0`, `lan_allow_published_ports=0`,
  `STYRMAN-IN` is `-j DROP` only, `STYRMAN-FWD` six DROPs and **zero**
  allowance rules. The feature is inert.
- Nothing running from this session. Scratch under `/tmp/claude-1000/…` is
  disposable.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-08-audit-project.md` — the audit, with a
  dispositions table: what was fixed, the one false finding, and the nine
  not-now items with reasons. **Read this before touching `internal/`.**
- `.agents/docs/reviews/2026-09-08-preflight-v1.24.0.md` — the release gate:
  16 findings across three rounds and how each was closed.
- `.agents/docs/deploy/remote.md` — the sshd hardening sequence, and the
  opt-in commands. The blocker lives here.
- `.agents/docs/plans/2026-09-08-lan-full-parity.md` — **29 of 30**. Exactly
  one box is open: the live published-port proof. The release box was ticked
  on 2026-09-08 after v1.24.0 shipped; a cold reader found it still open,
  which is why the count moved.
- `.agents/docs/architecture/DECISIONS.md` — ADR-066 and both amendments.
- `~/.config/sh-agent/proposals/2026-09-08-retro.md` — all four applied.

## Suggested Skills

- `bash` — anything in `deploy/firewall/` or `deploy/update/`.
- `preflight` — for the next release.
- Dispatch `reviewer` and `security` for any change to the firewall scripts,
  `internal/credbroker`, or `internal/httpapi/network.go`.

## Next Steps

1. **Get the operator's client public key**, harden sshd per `deploy/remote.md`
   with a second session held open, then `install.sh --lan-allow-host 1`.
2. **Flip the switch on a real project and prove it** — the plan's last box and
   the only unevidenced claim in the record.
3. Optionally `--lan-allow-published-ports 1` and run
   `STYRMAN_IT_LIVE_LAN=1 integration.sh` to close the published-port proof.
4. Pick up `TODO.md` when a real need appears; `styrman-update`'s test harness
   is the highest-value item there.
