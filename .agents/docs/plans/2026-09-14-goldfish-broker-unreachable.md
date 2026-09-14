---
owner: explorer
date: 2026-09-14
---
# Recon — broker unreachable at 172.16.1.254 for a resumed stopped project (2026-09-14)

## Question
Why did the credential broker not answer at its fixed bridge IP
(`172.16.1.254:8091`) for the first terminal of an old, stopped project
resumed at 11:38Z, right after a v1.31.0 deploy with zero running microVMs —
rank the top candidate root causes with file:line evidence, cover every
"boot a stopped project" path and whether/when it self-attaches the server
container to the project bridge, what a server-container restart does to
existing attachments, and which tests cover the mechanism.

## Summary
**Top candidate (high confidence): the terminal-open/auto-reconnect path
never self-attaches at all.** `ensureFor` (`internal/httpapi/server.go:257-277`)
is the boot function wired into the WS terminal bridge
(`internal/terminal/bridge.go:41,60-62`) — it calls only `EnsureRunning`,
never `EnsureSelfAttached`. `EnsureRunning` deliberately skips the attach
itself (`internal/orchestrator/orchestrator.go:445-449`, an explicit `NOTE:
no EnsureSelfAttached here`), relying on *other* call sites to do it lazily.
The TerminalPane's own "Start" button (`web/src/components/TerminalPane.tsx`
~423-440) is exactly a WS-reconnect epoch bump, i.e. this exact path — it
never calls `POST /api/projects/{id}/start`, so it never reaches
`startProject` (`internal/httpapi/server.go:1092-1114`), the one handler that
does background-attach. This matches every observed fact: the server log
shows `EnsureRunning`'s own lines (recreate/boot/terminal attached) and
*nothing* self-attach-related (no Info/Warn from `EnsureSelfAttached` at
`orchestrator.go:608-665`), and — decisively — the dockerd journal in the
11:38:30–11:39:00 window shows a single `sbJoin` for the project container
only, none for `styrman-server`; a called `EnsureSelfAttached` would produce
a second `sbJoin` (or a "self-attach ... failed" Warn) regardless of whether
the server's own success path logs anything (it doesn't, by design — see
below). Compounding but not required: this is also the FIRST time this
project's bridge needed an attach from the NEW (post-10:56Z-deploy) server
container, since `docker inspect styrman-server` shows only the ingress
network and the boot-time `Adopt` loop (`cmd/styrman/main.go:94-109`) only
attaches containers that were already *running* at boot — none were.

## Relevant files
- `internal/httpapi/server.go:257-277` `ensureFor` — WS bridge's boot function; calls `EnsureRunning` only.
- `internal/terminal/bridge.go:41,59-62` — `ensure` field/param type, wired to `ensureFor` in `New` at `server.go:245`.
- `internal/orchestrator/orchestrator.go:189-449` `EnsureRunning`/`ensureRunningLocked` — boots/recreates the container; lines 301, 325 are the "recreating..." log lines seen; line 445-449 is the explicit "no EnsureSelfAttached here" comment/design note.
- `internal/orchestrator/orchestrator.go:608-665` `EnsureSelfAttached` — the ONLY place that joins the server container to a project bridge and binds the fixed broker IP (`brokerIPOnBridge`); success is memoized (`o.selfAttached` sync.Map, 60s TTL) and logs NOTHING on success, only `Warn` on failure (line 652, 661) — so a clean success is silent by design; this is why log absence alone doesn't distinguish "succeeded" from "never called" and the dockerd journal is the decisive evidence instead.
- Call sites of `EnsureSelfAttached` (the complete list): `internal/httpapi/server.go:909` (createProject, background goroutine, provisionSpec.attach), `internal/httpapi/server.go:1114` (startProject, background goroutine after 1s sleep), `internal/httpapi/network.go:241` (LAN/sandbox mode toggle, background), `internal/httpapi/browser.go:250` (`browserUpstream`, DevTools proxy path, synchronous), `internal/httpapi/proxy.go:82` (port-preview proxy, synchronous), `cmd/styrman/main.go:109` (boot-time `Adopt`, only for containers already `running` at that boot).
- `internal/httpapi/server.go:1092-1114` `startProject` — the Sidebar "start" action's handler; the ONE path a plain resume goes through that does self-attach (background, 1s after the response).
- `web/src/components/TerminalPane.tsx` (~63, 99-102, 179-254, 418-440) — "Start" button is an `epoch` bump that reconnects the terminal WS; per its own comment (line 191) "may boot a stopped project VM" via the reconnect, i.e. via `ensureFor`, NOT via `POST .../start`.
- `web/src/components/Sidebar.tsx` (78-140, 331-333) — separate "start" action that DOES call `api.startProject(p.id)` → `POST /api/projects/{id}/start` → `startProject`.
- `image/configs/styrman-credentials.sh:36-44` — the MOTD credential check: single `curl -fsS --connect-timeout 1 --max-time 5`, no retry loop — one miss is permanent for that shell (matches the MOTD's own "open a new terminal to retry" wording).
- `cmd/styrman/main.go:82-109` — boot-time `Adopt`; self-attaches only containers found `running` at that boot moment (none were, per the symptom's "zero running microVMs" at deploy).

## Flows
1. **Broken path (candidate #1, matches evidence):** browser TerminalPane "Start" → WS reconnect (epoch bump) → `terminal.Bridge` → `ensureFor` (`server.go:257`) → `orch.EnsureRunning` (boots/recreates container, waits ready, returns) → WS attaches, guest login shell runs `profile-motd.sh`/`styrman-credentials.sh` → single curl to `STYRMAN_CRED_BROKER` (env baked in at container create, `orchestrator.go` create spec) → times out because the server container was never joined to `sdb-32c61123-…` and so never bound `172.16.1.254` on that bridge.
2. **Working path (not what the evidence shows happened here):** Sidebar "Start" → `POST /api/projects/{id}/start` → `startProject` → `EnsureRunning` (sync) → respond 200 → goroutine sleeps 1s → `EnsureSelfAttached` (binds broker IP on `sdb-…`, or falls back to dynamic + warns) → *next* terminal open (or a retry) reaches the broker.
3. Server container restart (a new deploy, e.g. the 10:56Z v1.31.0 rollout) starts on ZERO project networks — `NetworkConnect` state lives in the OLD container's netns and is not carried over; `docker inspect` confirms only `styrman-ingress` is attached now. Every project bridge needs a fresh `EnsureSelfAttached` after such a restart; the only automatic (non-request-triggered) mechanism for that is the boot-time `Adopt` loop, and it only covers projects that happen to be `running` at that exact boot instant.

## Conventions
- Self-attach is intentionally lazy/request-triggered per `orchestrator.go:445-449`'s comment (avoiding netns churn killing in-flight HTTP), pushed out to individual call sites rather than centralized in `EnsureRunning`/`ensureFor` — this is exactly the shape the project's own rule `.agents/rules/mechanism-reaches-every-caller.md` (BUG-035) warns about: a mechanism defined once but not propagated to every caller that needs it.
- Success paths for this mechanism are silent by design (comment at `orchestrator.go` near the memoize-on-success line); only failure logs. This makes dockerd's own journal, not the server's slog output, the authoritative independent evidence for "was NetworkConnect ever attempted."

## Risks
- The gap is structural, not incidental: `ensureFor` is the boot function for BOTH the WS auto-reconnect AND (per `server.go`) presumably other WS-triggered resumes — any caller reaching a stopped project exclusively through the terminal WS (no explicit `/start` POST) gets no self-attach, forever, until some other action on that project (LAN toggle, browser pane open, port preview, or the Sidebar start button) happens to trigger one.
- Even the "working" path (`startProject`) has a real, separate race: `EnsureSelfAttached` runs in a background goroutine 1s after the HTTP response, while the terminal WS can attach and run the login shell's one-shot, no-retry credential curl before that goroutine's `NetworkConnect` (+500ms settle sleep) completes — this session's evidence points at candidate #1, not this race, but the race is real and un-tested.
- Zero test coverage of the whole mechanism: no `_test.go` file anywhere references `EnsureSelfAttached`, `ensureFor`'s attach behavior, or `brokerIPOnBridge` (checked `internal/orchestrator/*_test.go`, `internal/httpapi/*_test.go`) — so a future refactor could re-detach this path silently again.

## Open questions (falsification tests for the ranking)
- **Falsifies/confirms #1 directly:** the styrman-server ACCESS/request log for the 11:35-11:39Z window (not included in the given facts) — a `POST /api/projects/32c61123-…/start` would mean the Sidebar path was used instead, pointing at the race (candidate #3) rather than the missing-call (candidate #1). Not examined here (log excerpt not provided/available).
- **Corroborates #1 further:** a wider dockerd-journal grep (`sbJoin.*styrman-server` since the 10:56Z deploy) — if it never appears for ANY project since the deploy, that shows the new server container has attached to nothing yet and strengthens #1/the restart-corollary; not run (no host access in this session, facts were given as a transcript).
- Not examined: `internal/httpapi/server.go:245`'s exact `New(...)` wiring line confirming `ensureFor` is the literal function passed to `NewBridge` (inferred from the matching signature at `bridge.go:60-62` and the field name `ensure`, not re-verified against the call line itself).
- Not examined: whether any OTHER WS-reconnect-only entry point besides `TerminalPane`'s Start button also routes through `ensureFor` without ever passing through `startProject` (e.g. resuming via a deep-link or the setup VM flow) — only the two UI actions named in the dispatch were traced.

## Ranked candidates
1. **(High) Terminal WS resume path never calls `EnsureSelfAttached`** — `internal/httpapi/server.go:257-277` (`ensureFor`) + `internal/orchestrator/orchestrator.go:445-449` (deliberate omission in `EnsureRunning`) + `web/src/components/TerminalPane.tsx` Start button being a pure WS-reconnect. Evidence: server log has only `EnsureRunning`'s lines, zero self-attach-related Info/Warn; dockerd journal shows no `sbJoin` for `styrman-server` in the window (decisive, since success is otherwise silent). Falsified by: an access log showing `POST .../start` was actually called before 11:38:44, or a dockerd `sbJoin` for `styrman-server` on `sdb-32c61123-…` appearing shortly after (which the given journal excerpt does not show).
2. **(Medium, corollary of #1) Fresh server container has zero project-bridge attachments since the 10:56Z deploy** — `cmd/styrman/main.go:94-109` (`Adopt` only self-attaches containers `running` at boot; none were) + `docker inspect styrman-server` showing only `styrman-ingress`. This doesn't independently explain the incident (it's subsumed by #1: even a "correct" caller would have had to attach fresh) but confirms there was no lingering attachment to fall back on.
3. **(Low given the journal, but a real latent bug) Race between `startProject`'s background self-attach and the login shell's one-shot credential check** — `internal/httpapi/server.go:1114` (1s-delayed goroutine) vs `image/configs/styrman-credentials.sh:44` (single `curl`, `--connect-timeout 1 --max-time 5`, no retry). Would produce the exact same MOTD line even on the "correct" Sidebar path. Weighed down by: the dockerd journal's absence of any `sbJoin` for `styrman-server` in the whole 30s window argues `EnsureSelfAttached` was never invoked at all here, not merely late — falsified/confirmed by the same access-log check as #1.

diagnosis complete
