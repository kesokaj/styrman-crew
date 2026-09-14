---
owner: main
date: 2026-09-14
---
# Handoff — 2026-09-14 — v1.32.0 live (BUG-060 fix shipped); its live proof needs a project stopped across a server restart

Session ID: session_01RUoU1zHUVbathNBFLo27xf        (same machine: `claude --resume`; matches the feedback log's session field)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

Supersedes `2026-09-14-session-end-bug-060-day-end.md` and
`2026-09-14-session-end-bug-060.md` (same session): the operator then said
"preflight that shit and push a minor", so the release happened after all.

## Context

The BUG-060 fix (terminal-WebSocket boot joins the server to the project's
bridge before the upgrade) was gated with `/preflight` and shipped as
v1.32.0 — MINOR by the operator's explicit call, a PATCH by the versioning
rule; the gate report records both. The updater deployed it at 15:20:56
local. The session ends for the day.

## Completed

- Gate `.agents/docs/reviews/2026-09-14-preflight-v1.32.0.md` (SHIP):
  `deploy/verify-commit.sh` green on 340b045 (full) and on the release
  commit 3730551 (web skipped, untouched); release review on opus APPROVE
  (`.agents/docs/reviews/2026-09-14-review-v1.31.0-340b045-release.md`: finding 1 accepted as the gate
  condition, finding 2 fixed — a test comment); docs lens
  (`.agents/docs/reviews/2026-09-14-review-v1.31.0-340b045-docs.md`: ADR-021 contradicted → ADR-071
  appended; terminal-protocol.md note); the fix's pre-release review and
  security reports cited for the byte-identical blobs.
- Release commit `3730551`, tag `v1.32.0` SSH-signed with `git -c` (no
  config change), verified against the host's allowed signers, pushed
  13:19:19Z. Updater: signature OK, three images built, "OK — v1.32.0 is
  live" 15:20:56. `styrman-ctl status` verdict OK on 1.32.0 (sha 3730551),
  health `{"status":"ok"}`, public URL 302, broker up, runtime image
  1.32.0. Release entry `.agents/docs/changes/2026-09-14-v1.32.0-release.md`;
  README status + release line; ROADMAP "Now" names the entry.
- Earlier today, same session: the fix itself (`d285015`), BUG-060,
  handoffs — see the superseded handoffs.

## Open / In Progress

- **Live proof of the fix not yet observed** — and not observable today
  without another server restart: the condition is a project STOPPED
  across a server restart, opened from the UI. 0 projects existed at
  deploy time, so a project created now keeps its creation-time attach
  until the next restart; create → stop → open proves nothing. The
  discriminating check (operator): create a project, stop it, restart the
  server (next release, host reboot, or `sudo deploy/styrman-ctl`), open it
  from the UI — no "credentials: the broker did not answer" line, `gh auth
  status` succeeds. Closes BUG-054's outstanding proof too.
- Operator, versioning.md step 5: a fresh Google sign-in showing 1.32.0 in
  Settings.
- Requirements edits awaiting approval: none.

## Key Decisions

- MINOR at the operator's word after being told the rule says PATCH;
  recorded in the gate report, not re-argued.
- ADR-071 amends ADR-021 for the WS handler path only: attach on the
  request path before the upgrade; the client's reconnect loop is the
  counterpart of the proxy's 307; the published-port caveat is stated for
  the dev topology, production has no published server port.
- The test-comment correction (release review finding 2) rode in the
  release commit and was re-verified by `verify-commit.sh` on that commit
  rather than becoming its own pre-release commit.
- Security finding 3 (`ip_forward=0` unasserted at runtime) stays carried
  here as an optional task, not in TODO.

## Corrections

- "I dont want custom fixes. THis is still not a production project. I can
  remove it if it solves it." — feedback log 2026-09-14T15:00:42+02:00
  (convention / correction / rule_ref null). One this session; the harness
  retro ran today (retros index 2026-09-14T08:28Z), none due.

## Agent notes (unverified)

- The changelog agent's release entry needed two corrections again (named
  340b045 as "the sole substantive commit" — the fix is d285015 and 340b045
  a handoff; invented "integration Tier 1 passing" for the verifier). Its
  ADR-071 draft was verbatim from the brief and needed none. The
  documentation agent split a README sentence earlier in the day. The
  read-through pattern stands as the `/retro` candidate.
- The gate ran the release review on opus and the docs lens on the default
  reviewer model; the docs lens found the ADR contradiction, the code review
  did not — worth keeping the docs lens on every release.

## Environment State

- Branch: main, clean; HEAD = release commit `3730551` + this handoff's
  commit; tag v1.32.0 on 3730551, pushed.
- Production: https://crew.styrman.xyz on 1.32.0 (server, caddy, runtime),
  live 15:20:56 local; 0 projects, 0 microVMs.
- Scratch (gitignored): `verify-commit-340b045.log`,
  `verify-commit-3730551.log`, `updater-v1.32.0.log`, `allowed_signers`,
  stage lists.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-14-preflight-v1.32.0.md` — the gate, every
  finding's disposition, the observed deploy, the live-proof status.
- `.agents/docs/changes/2026-09-14-v1.32.0-release.md` — the release entry.
- `.agents/docs/architecture/DECISIONS.md` ADR-071 — why the WS path
  attaches on the request path.
- `.agents/docs/bugs/BUG-060-terminal-ws-boot-never-joined-the-project-bridge.md`.

## Next Steps

1. Operator: the discriminating live check above, when a server restart
   next happens (or is made to happen); record the result in the gate
   report's Deploy section and close BUG-054/BUG-060's proof line in
   ROADMAP "Now".
2. Operator: fresh sign-in showing 1.32.0 in Settings.
3. Optional, its own task: a runtime assertion for `ip_forward=0` on the
   server container (security finding 3, pre-existing).
4. `/retro` when a session next collects two corrections; standing
   candidates: the changelog read-through pattern; "no custom live
   workarounds" as a project rule.
