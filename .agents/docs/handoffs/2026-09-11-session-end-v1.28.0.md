---
owner: main
date: 2026-09-11
---
# Handoff — 2026-09-11 — v1.28.0 live: Browser view + styrman-ctl; nothing queued

Session ID: 20b850ca (Claude Code). Project: /home/kesokaj/workspace/styrman-crew.
Branch: main, clean, pushed (release commit `eb7b883`, tag `v1.28.0` SSH-signed,
this handoff's trail commit on top). Production: **v1.28.0** (server, caddy,
runtime), deployed by the auto-updater at 15:36–15:37, `styrman-ctl status`
verdict OK at 15:37.

**Supersedes `2026-09-10-session-end-v1.27.1.md`.**

## Context

One long session, 2026-09-11. The operator asked for two features: a
browser inside the app that the agents can use for testing, and one
interactive file to install/update/edit/uninstall the whole deployment.
Both were planned (`feature` workflow: scope, plan file, three cold-read
rounds each, the operator waived the "ready" token after round 3 in both
cases), built in four phases each with per-phase reviewer/security/verifier
passes, documented, and shipped together as v1.28.0 on the operator's
"ship it". The operator also restated the Definition of Done ("everything
done, no TODO, no ROADMAP left, docs and every log/review committed, a
handoff there") — saved to memory and the feedback log.

## Completed

- **Browser view** (ADR-068 + amendment; R4 gained one operator-approved
  sentence): `image/` carries Debian chromium + `chrome-devtools-mcp` +
  `configs/styrman-browser` (launcher; MCP registered for claude and agy at
  seeding); server routes `GET/POST /api/projects/{id}/browser[/start|/stop]`
  + relay `GET /ws/browser/{id}` (`internal/httpapi/browser.go`); the SPA's
  fourth pill, `BrowserPane.tsx`, `cdp.ts`, `browserInput.ts`, "browser" on
  Ports rows. Tests: integration 93 checks (90/0/3 here), e2e 91/0/2,
  frontend 172, Go unit tests for relay/peer. Four security rounds; every
  high fixed (shareable DevTools port, `--remote-allow-origins`, redirect
  steering, the forwarder's peer); accepted risks written down (shared
  profile — the guide's warning; `--network=host` inner containers).
- **styrman-ctl** (`deploy/styrman-ctl`, ~900 lines bash; `internal/config/styrman_ctl_test.go`,
  15 tests over `--dry-run` plans with fake tools under a scratch prefix):
  `status [--json]`, `install`, `update` / `--redeploy` / `--setup`, `edit`,
  `uninstall --level 1|2|3` with typed words, a menu, `--dry-run` everywhere.
  Five reviewer and five security rounds; the last three rounds' dispositions
  are in `testing/2026-09-11-verify-styrman-ctl-phase4.md`. Every run on
  this host was `status` or `--dry-run`; no real install/uninstall happened.
- **Docs**: guide.md (§1, §2 "Browser view", §4 "Using the browser"), api.md,
  terminal-protocol.md, runtime-image.md "Browser", deploy.md "styrman-ctl",
  deploy/README.md quick start, remote.md/local.md pointers, README.md.
- **Bugs**: BUG-039 (terminals listing 500 during a stop — fixed), BUG-040
  (integration suite fixed port — fixed, `STYRMAN_ITEST_PORT` + bind check),
  BUG-041 (a piped verification guarded a commit — recurrence of BUG-033;
  rule graduated to `.agents/rules/verify-bare.md`).
- **Release**: gate `reviews/2026-09-11-preflight-v1.28.0.md` (SHIP);
  `deploy/verify-commit.sh` OK on `4c0869b` and `b021cfc`; release entry
  `changes/2026-09-11-v1.28.0-release.md`; tag verified against
  `/etc/styrman/allowed_signers` ("Good git signature"); the updater's first
  tick after the push built the three images and recreated the stack; live:
  server healthy `styrman-server:1.28.0`, `styrman-caddy:1.28.0`,
  `STYRMAN_IMAGE=styrman-runtime:1.28.0`, ingress 302 → Google auth,
  STYRMAN-FWD 7 rules / STYRMAN-IN 1 (unchanged), all three units enabled,
  `deployed-version v1.28.0` sha `eb7b883`, no failed-version.

## Open / In Progress

- Nothing owed by the agent. Two steps are the operator's, not blocking:
  1. **Manual agent walk** in the Browser view on production: open a
     project, click Browser, then in the terminal ask `claude` (or `agy`) to
     open `http://localhost:<port>` and describe the page — its tab appears
     in the strip. Instructions at the end of
     `testing/2026-09-11-verify-browser-phase4.md`. Record the outcome in
     `testing/` and tick the plan's "Manual agent walk" box.
  2. **Fresh-host `styrman-ctl install`** on a Debian 13 VM (or
     `uninstall --level 1` + `install`) — R11's own criterion; never on the
     production host. Tick the plan's "Fresh-host e2e" box when run.
- Follow-ups listed in both plans (not TODO items by the operator's rule
  that TODO/ROADMAP stay empty unless they ask): pipe transport for DevTools,
  a separate agent browser profile, stale link/share sweep, lockfile install
  of chrome-devtools-mcp, re-seed cap, idle browser stop, a `doctor` verb,
  shipping `styrman-ctl` to `/usr/local/bin` via `host_converge`.
- Observations, not tasks: a project stop takes ≈48 s on this host with or
  without a browser; ≈90 `styrman-*` image tags from old releases and
  verification builds sit on the host (housekeeping).

## Key Decisions

- Browser architecture: real headless Chromium per project, CDP screencast
  to the SPA, MCP for both agents (operator's choice over iframe/VNC).
- DevTools reachability: Chromium loopback-only (9229); a socat door on 9222
  admitting ONE peer — the server's own bridge address, passed as
  `STYRMAN_BROWSER_PEER` (empty = no door); no `--remote-allow-origins`.
- styrman-ctl wraps the existing scripts, never replaces them; `update` is
  the auto-updater only (break-glass compose stays a documented command);
  uninstall deletes only trees named `styrman` that carry styrman's own
  markers, never under a home or system prefix; marker misses warn and skip.
- Versions: v1.28.0 MINOR. No commit trailers (AGENTS.md's git rule).

## Corrections

- "DoD=is when everything is done, no todo no roadmap left and documentation
  and all logs,reviews etc is done and commited and a handoff is there." —
  feedback log 2026-09-11 (restatement); memory `definition-of-done-operator`.

## Agent notes (unverified)

- Reviewers were dispatched over ranges while later commits landed; each
  report pins the blobs it read and says so. Two documentation-agent memory
  files under `.claude/agent-memory/` are tracked and rode in the release
  commit, as in earlier sessions.
- The Go temp dir is group-writable (775): secret-file tests keep their
  files in a 0700 subdirectory.

## Environment State

- Branch main clean and pushed; tag v1.28.0 on `eb7b883`.
- Production healthy on 1.28.0; no project VMs running at handoff time; the
  operator's one project dir intact (174M).
- Scratch (`.agents/scratch/`, gitignored): the hand-walk scripts and
  screenshots, test logs. Local images `styrman-runtime:{browser-p1,verify-*}`
  from this session (harmless). `.agents/scratch/ctl/sec/` holds FAKE
  token/signers files used for dry-runs only.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-11-preflight-v1.28.0.md` — the gate.
- `.agents/docs/changes/2026-09-11-v1.28.0-release.md` — the release entry.
- `.agents/docs/plans/2026-09-11-browser-view.md`, `…-installer-ctl.md` —
  the plans (all boxes ticked except the two operator steps).
- `.agents/docs/testing/2026-09-11-verify-browser-phase4.md`,
  `…-verify-styrman-ctl-phase4.md` — the dispositions of every finding.
- `.agents/docs/bugs/INDEX.md` — BUG-039..041.

## Suggested Skills

- `feature` / `bug` for whatever comes next; `preflight` for the next release.

## Next Steps

1. Nothing queued. Wait for the operator's next request.
2. Operator: the two steps under "Open" when convenient.
