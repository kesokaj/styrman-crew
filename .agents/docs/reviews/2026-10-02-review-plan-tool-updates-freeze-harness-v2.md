---
owner: reviewer
date: 2026-10-02
---
# Review — plan-tool-updates-freeze-harness-v2   (2026-10-02, cold-reader round 3)

Range reviewed: .agents/docs/plans/2026-10-02-tool-updates-freeze-harness-v2.md (current text on disk, 320 lines) plus the code surfaces it names. Rounds 1 and 2 are overwritten; every item below comes from a fresh read of the current text.

## Status: COMPLETE

## Verdict: APPROVE WITH NITS

design needs revision (three small gaps, none structural); implementation ready (zero BLOCKING questions). Worst first: the hand run's fixture, "a throwaway container of the built image, as a non-root user", does not produce the condition it tests. The `claude update` failure came from `NPM_CONFIG_PREFIX` (`image/skel/bash_exports:8`) pointing at a `~/.npm-global` that only the entrypoint creates, and the skel files reach a home only through the entrypoint (`image/entrypoint.sh:80-90`). A bare `docker run` has neither the variable nor the directories, so `claude update` fails (or targets unwritable `/usr/local`) whatever the fix is. The e2e dead-socket fixture, the RED/GREEN ordering, the `classifyControl` signature and the boot-budget measurement all hold up against the code: the earlier round's objections to them are resolved in the current text.

## Findings

### (1) Gaps
1. **[medium / confidence: high]** Plan "Hand run" (line 252-255) and Phase 1 row 7 (line 280). The throwaway container has no seeded home (see Verdict). The plan must say how the home is produced: run it as `docker exec -u <user>` inside the Tier 2 project container (real entrypoint, real skel, real Kata), or boot the image through the entrypoint. Otherwise the hand run proves nothing about D1, and a bare-container pass of `agy update` says nothing about the project path. `agy update` is the only half a bare container does exercise.
2. **[low / confidence: high]** Scope: D7, D9 and D12 say `by: implementer` (lines 312, 314, 317). The brief's rule is that an added behaviour needs a D-row naming who asked. D7 (per-connection liveness) and D9 (pong not gated by endpoint) are consequences of the approved ping/pong fix and need no operator row. D12 is not: it changes the integration suite's state dir and adds a fail-closed tmpfs guard, and the request never named it. It is justified by evidence (3.26 s on tmpfs vs 16.16 s on disk). Either record the operator's approval, or state in the row that it is the verification of D3's promise ("boot time unchanged") and not a feature. It is the one item that affects the release gate's pass/fail.
3. **[low / confidence: medium]** Interfaces, entrypoint (lines 88-90). `harness-done` is "created when this boot's harness step is over, whatever the outcome", but the whole block sits under `[ -z "${MINIMAL}" ] && [ -x /styrman/bin/sh-agent ]` (`entrypoint.sh:141`). With a missing or non-executable `sh-agent` the marker is never written and an integration wait times out at 150 s. State that the marker is written on that path too (or that the wait is skipped when the binary is absent). Also say that the background step runs in the entrypoint's own shell, so it sees `HM_OK`, `HR_OK` and the decision: the stamp condition (`entrypoint.sh:242`) needs all three, and two of them are now computed before ready.

### (2) Scope
Nothing built beyond the request except D12 (gap 2). `wsprobe -control` is verification tooling for the ping check, justified in Verification. `online`, the 25 s interval, visibility, "Use here" and reclaim are the operator's own words in D4. `focus`, `pageshow` and the browser pane are struck (D10, D11). Three ADRs match three decisions.

### (3) Readiness — questions an implementer would ask
Zero BLOCKING. NON-BLOCKING, with the default a careful implementer reaches alone:
- (non-blocking) Gap 1: how the hand run gets a seeded home. Default: `docker exec` in a Tier 2 project container.
- (non-blocking) Gap 3: the marker when `sh-agent` is absent, and variable scope in the background step. Default: write the marker on every exit path of the block.
- (non-blocking) The e2e WebSocket wrapper must intercept the `onmessage` and `onclose` property setters, not only `addEventListener`: `TerminalPane.tsx:~170-215` assigns `ws.onmessage = ...`. A wrapper that misses them does not freeze anything, and the plan's own step "a typed marker does NOT echo within 2 s" fails loudly, so the mistake cannot go unseen.
- (non-blocking) `wsprobe` with both `-send` and `-control`: order unspecified (`-send` sleeps 500 ms first, `cmd/wsprobe/main.go:42-47`). Default: control frame first, only one of them used in the new check.
- (non-blocking) Reclaim from `replaced` must undo `closedForGood`, which `connect()` tests first (`TerminalPane.tsx:~172`), and the unmount cleanup sets it again. The plan says "reconnect in place"; the mechanism is the implementer's.
- (non-blocking) On the unfixed build, e2e criterion 1 leaves the first page on "Opened in another tab", and the `finally` cannot restore it (no button, first page not navigated). The next check, "settings page renders sections" (`run.mjs:1121`), only clicks the sidebar, and the later terminal checks begin with `page.goto` (`run.mjs:1172`), so the red run does not cascade and `failed=3` holds. Checked against `web/e2e/run.mjs:1111-1180`.

## Verified against the code (no finding)
- `controlMessage` (`bridge.go:95-99`) has Type, Cols, Rows: the `classifyControl` return shape type-checks. The current resize guard is at `bridge.go:441`; `touch()` for every client frame is at `bridge.go:438`. The ping is exempted as planned.
- `conn.Write` from `github.com/coder/websocket` is safe from the reader goroutine alongside the exec-stream writer (`bridge.go:410`).
- `run.mjs:49` creates the first page with `browser.newPage()`, so `page.context().newPage()` shares its (implicit) context. Fixture 3 uses a third page; the first page is not wrapped.
- `/usr/local/bin/agy` in `styrman-runtime:test` is a 220 MB regular file, so the planned move to `/opt/agy/agy` plus a symlink is well-formed.
- `docker images` shows `styrman-runtime:test` created 2026-10-02 12:59 and `.agents/scratch/itest-red-disk-backed.log` exists, as the plan cites. `itest-red.log` is correctly not yet present.
- The only consumers of the harness boot log lines are the integration checks (`integration.sh:144, 164, 492-510`): no Go code greps for them. The plan's "wait on `harness-done` before every harness assertion" covers 144-165 and the stop and delete section's 413-428 and 488-510.
- R5 ("the entrypoint installs it") and R6 ("single active WebSocket per terminal, 4001 replaces") stay true under the plan: reclaim replaces the other socket by the existing mechanism. No `REQUIREMENTS.md` edit is needed. Latency SLA: the before-ready work is about 3.4 s (16.16 s minus the 12.8 s install), inside the 5 s soft target.
- ADR-077, 078 and 079 are free: the last ADR in `DECISIONS.md` is 076.
- Docs clauses: every changed command, flag, config key and deploy step has one (`wsprobe -control` and the `/var/tmp` state dir under the testing README; the `~/.npm-global` dirs and `/opt/agy` under `runtime-image.md`).

## Criteria
1. Dead code — pass: the plan removes the "self-updates at run time anyway" comment text and keeps `harness_home_newer` extractable.
2. Flow tracing — pass with gap 3: control frame, `classifyControl`, `serve`, `/ws/setup`, the idle reaper and every harness assertion are traced; the marker path with no `sh-agent` is not.
3. Standards conformance — pass: the restatement list and the grep terms are named; the e2e rule is applied, with the three checks at the end of the terminal section and `finally` restores.
4. Directory layout — pass: `web/src/lib/termLiveness.ts` and its test sit beside each other; Go tests beside the code.
5. Simplicity (DIY-except-security) — pass: no new dependency; the ping/pong is the plain protocol over the existing library; no crypto or auth written here. D12 is the one unrequested piece (gap 2).
6. Decoupling — pass: the liveness module is injected with `sendPing` and `onDead`, so it is unit-testable without a socket.

## Not examined
- `internal/orchestrator/orchestrator.go`'s idle reaper beyond `Touch` (`:146-147`) and `stopIfIdle`'s signature, so the claim "a ping is not counted" rests on the bridge's `touch` call site only.
- `web/src/components/TerminalPane.tsx` outside lines 25-45 and 120-400; the `focusToken` effect and the Browser pane.
- `.agents/docs/specs/styrman/*` bodies beyond the 4001 row of `terminal-protocol.md:130`; `image/README.md`; the grep-derived restatement list was not re-run.
- `internal/config/entrypoint_guard_test.go` beyond its function list; the Dockerfile's agy layout change is a plan, not yet code.
- Test suites, docker builds and the e2e were not run (per the brief).
