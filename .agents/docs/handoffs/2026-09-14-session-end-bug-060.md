---
owner: main
date: 2026-09-14
---
# Handoff — 2026-09-14 — BUG-060 fixed and pushed (unreleased): terminal-WS boot now joins the project bridge

Session ID: session_01RUoU1zHUVbathNBFLo27xf        (same machine: `claude --resume`; matches the feedback log's session field)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator opened an old (stopped) project on the live site right after
the v1.31.0 deploy and the first terminal's MOTD said "credentials: the
broker did not answer (2026-09-14T11:38:46Z http://172.16.1.254:8091)".
Diagnosed against the live host (this machine is the production box),
fixed under the bug skill, brought through the Definition of Done, committed
and pushed — NOT released. The live project is still without a broker
until the next release or the workaround in Next Steps.

## Completed

- Diagnosis (live): `styrman-server` sat on `styrman-ingress` only; the
  project bridge `sdb-32c61123-…` (172.16.1.0/24) held only the guest; a
  `curl` from inside the guest to `172.16.1.254:8091` timed out; dockerd's
  journal showed no join for the server. The goldfish diagnosis
  (`.agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md`) agreed.
- Root cause: `ensureFor` (`internal/httpapi/server.go`), the terminal
  WebSocket bridge's boot callback, only called `EnsureRunning`, which never
  self-attaches by design; every other boot path attaches in a goroutine
  after its response. So after every server restart the first terminal of a
  stopped project had no broker. Silent before v1.30.1's MOTD (BUG-054).
- Fix (commit `d285015`, pushed): `wsBoot` + `wsBooter` seam in
  `internal/httpapi/server.go` — boot, then `EnsureSelfAttached`
  synchronously BEFORE the socket upgrade; the orchestrator NOTE names which
  caller attaches when. Tests: `internal/httpapi/wsboot_attach_test.go`
  (red before the fix), `internal/terminal/ensure_before_upgrade_test.go`.
- Definition of Done: verifier PASS twice
  (`.agents/docs/testing/2026-09-14-wsboot-attach-verify.md`); reviewer
  APPROVE WITH NITS, the substantive one fixed by the terminal test
  (`.agents/docs/reviews/2026-09-14-review-wsboot-attach.md`); security
  three lows, two rebutted, one pre-existing carried below
  (`.agents/docs/reviews/2026-09-14-security-wsboot-attach.md`); docs
  (AGENTS.md bullet, specs `deploy.md`/`guide.md`, testing README);
  BUG-060 record + INDEX; `.agents/rules/mechanism-reaches-every-caller.md`
  gained the instance; change entry
  `.agents/docs/changes/2026-09-14-bug-060-terminal-ws-boot-attach.md`.

## Open / In Progress

- Not released: the fix is in `main` only. Production (1.31.0) still has
  the defect; a release is the operator's call ("ship" → `/preflight`,
  PATCH: it only fixes released behaviour).
- Live proof outstanding (unit-verified only — the live join was blocked by
  the permission classifier in this session): after the deploy, open a
  project that was STOPPED across the server restart from the UI; the first
  terminal must show no "broker did not answer" line and `gh auth status`
  must succeed. This is also BUG-054's still-outstanding proof.
- Requirements edits awaiting approval: none.

## Key Decisions

- Attach placed in the WS boot callback (`wsBoot`), not inside
  `EnsureRunning`: the orchestrator's NOTE (netns reconfiguration can
  disturb in-flight responses on a published port) stands, and this path is
  the only one with no response to protect and a login shell that needs the
  broker at once. The "fresh" return is ignored there on purpose.
- A two-method interface seam (`wsBooter`) instead of a Docker fake at the
  httpapi level — same shape as the mode toggle's `Project` fake.
- Security's ip_forward=0 finding (compose sets it, nothing asserts it at
  runtime) is pre-existing and unrelated to this diff; carried here rather
  than into TODO (a deploy change, reviewed on its own if wanted).

## Corrections

- none this session (no feedback-log entry for this session).

## Agent notes (unverified)

- Coverage gaps named by the verifier, not fixed: `EnsureSelfAttached` has
  no unit test of its own (the orchestrator fake's `NetworkInspect` returns
  no subnet, so a static-IP assertion needs the fake extended);
  `ensureFor`'s record lookup is not directly tested.
- The changelog agent's draft again needed two read-through corrections
  ("matching the pattern of every other boot path" — the opposite is the
  point; security "all rebutted" — one was carried). The `/retro` item from
  the v1.30.0/v1.31.0 handoffs still stands.
- The documentation agent split a sentence in the testing README (the
  `passed=87` integration count landed inside the BUG-060 guidance); fixed
  by hand before the commit.

## Environment State

- Branch: main, clean, level with origin at `d285015` (this handoff's
  commit follows). No tag.
- Production: https://crew.styrman.xyz on 1.31.0; project
  `styrman-32c61123-…` (sdfsf) was running at the end of the session,
  server NOT attached to its bridge.
- Scratch: `.agents/scratch/stage-bug060.txt` (gitignored).

## Artifacts to Read

- `.agents/docs/bugs/BUG-060-terminal-ws-boot-never-joined-the-project-bridge.md` — symptom, live evidence, root cause, prevention rule.
- `.agents/docs/changes/2026-09-14-bug-060-terminal-ws-boot-attach.md` — files, agents, verification limits.
- `.agents/docs/reviews/2026-09-14-security-wsboot-attach.md` — the three lows and their Outcome.
- `.agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md` — the independent diagnosis brief.

## Next Steps

1. Operator, immediate workaround on the live host for the running project
   (the same operation the fix performs; reversible with `docker network
   disconnect`): `docker network connect --ip 172.16.1.254
   sdb-32c61123-9d5c-4cea-a3bf-162c9d1ac41b styrman-server`, then open a
   new terminal in that project — the MOTD line must be gone. Opening the
   project's port preview once does the same through the server.
2. Operator: decide on the release (`/preflight`, PATCH). Then the live
   proof above, which closes BUG-054's proof too.
3. Operator, optional: a runtime assertion for `ip_forward=0` on the server
   container (security finding 3, pre-existing) — a deploy change, its own
   task.
4. `/retro` when a session next collects two corrections; the changelog
   read-through pattern is still the standing candidate.
