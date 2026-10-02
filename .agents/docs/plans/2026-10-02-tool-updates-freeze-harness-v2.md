---
owner: main
date: 2026-10-02
---
# Plan — in-project tool updates, the terminal freeze, harness v2.3.2   (2026-10-02)

## Why
Operator, 2026-10-02: "1.) Update the runtime image to all the latest versions. I got a
feeling I cant update software (claude/agy) in the project in crew. Maybe it is just a bug.
Do some research. 2.) Somethings lets say If I close the tab for crew it freezed, i need to
refresh the window pretty often, can we fix this? 3.) Update the styrman-harness here is the
new agent …/v2.3.2/styrman-harness-v2.3.2.zip download and run this on the runtime image.
this will become a new version. Update everything and update the local machine."

Measured before this plan (image `styrman-runtime:1.38.0`, project user, interactive login shell):
- `agy update` → `Error: directory /usr/local/bin is not fully accessible (readable: true, writable: false)`.
- `claude doctor` → `Can't auto-update: npm global folder isn't writable`: `skel/bash_exports`
  sets `NPM_CONFIG_PREFIX=$HOME/.npm-global` and a fresh home has no such directory. With
  `~/.npm-global/{bin,lib}` present, doctor is clean and `claude update` lands 2.1.287 there.
- `sh-agent v2.3.2 install` writes 1,788 files: 10.9 s on the host's ext4, 12.8 s in a Kata
  microVM on an ext4-backed home, 0.06 s on tmpfs (v1.6.0: 0.02 s). The seed sits before
  `/run/styrman/ready`.
- The freeze, two causes read in the code, no repro from the operator:
  (a) close 4001 leaves `TerminalPane.tsx` at "Opened in another tab" with no control;
  (b) a socket that died during sleep or a network change stays `OPEN` to the page. The
  server does ping every 30 s with a 10 s bound (`internal/terminal/bridge.go:354-377`) and
  closes ITS side, but that close cannot reach a page whose connection is gone, and the
  browser exposes no ping to script: the page has no check of its own, so it neither
  reconnects nor says anything. A pane that used up its 8 retries while away stays
  "Disconnected" until clicked.

## Scope
In:
1. Runtime image pins to the publishers' current versions (in the working tree: Node 22.23.3,
   Claude Code 2.1.287, OpenTofu 1.13.1, yq 4.54.1, chrome-devtools-mcp 1.10.1, both Debian
   base digests; Go 1.27.1 and hadolint 2.15.1 were current).
2. `claude update` and Claude's auto-updater work in a project (D1).
3. `agy update` works in a project (D2).
4. Harness v2.3.2 by ADR-074's path; its file install runs after `ready` (D3, D8).
5. The freeze in a project terminal: a replaced terminal can be taken back; a dead socket is
   noticed and replaced (D4).
6. Release as the next MINOR through `/preflight`.

Out:
- The setup pane (`target.kind === "setup"`): it must never reconnect once opened (a
  reconnect's `tmux new-session -A` restarts a finished `styrman-setup`, the v1.10.0 bug);
  the page's behaviour there is unchanged.
- Delivering the harness from the zip: the same release carries the signed binary ADR-074
  already verifies (D3).
- Updating this host as root: the operator installs a fresh `/etc/styrman/gh-token` (D5).

Follow-ups noted, not built:
- The browser pane has cause (b) too (a dead relay socket shows a frozen frame); the operator
  approved the terminal fix only. Found in: the freeze diagnosis. Goes to the handoff's Next
  Steps as a question for the operator.
- `sh-agent install` syncs every file (1,788 → 11 s); a harness-repo fix would make the
  install near-instant again. Found in: research for this plan.

## Surfaces touched
- `image/Dockerfile` (pins; agy layout), `deploy/Dockerfile.server` (one base digest),
  `image/entrypoint.sh` (home dirs; harness seed), `image/README.md`.
- `internal/terminal/bridge.go` (control frame), `cmd/wsprobe/main.go` (send/print a text frame).
- `web/src/components/TerminalPane.tsx`, new `web/src/lib/termLiveness.ts`; `web/dist/client` rebuilt.
- Tests: `internal/terminal/`, `internal/config/` (entrypoint), `web/src/lib/termLiveness.test.ts`,
  `.agents/docs/testing/styrman/integration.sh`, `web/e2e/run.mjs`, the count lines in
  `.agents/docs/testing/styrman/README.md`.
- Describing text the change makes false, changed in the same commits:
  - code comments: `image/entrypoint.sh` (the harness block's header and its cap
    arithmetic), `image/Dockerfile` (agy "self-updates at run time anyway"; the agents'
    install comment), `integration.sh` (the comment at the boot-budget check),
    `internal/terminal/bridge.go:1-8` (package doc: the protocol) and `:300-304` ("a
    replaced browser … never reconnects"), `internal/config/config.go:69-71`
    (`CloseReplaced`), `web/src/components/TerminalPane.tsx:30-39` (the protocol doc
    comment), `:221-226` (the 4001 branch) and `:276-284` (the outbox comment);
  - docs: `image/README.md` (27 skills, 7 subagents), `specs/styrman/runtime-image.md`,
    `specs/styrman/deploy.md` ("Agent harness in the image"),
    `specs/styrman/terminal-protocol.md` (the 4001 row, control frames),
    `specs/styrman/guide.md` (the "Terminal shows replaced" row), `README.md` (the harness
    paragraph), the project `AGENTS.md` only where a restated number is found.
  The list is re-derived by grep before the review (`v1.6.0`, `27 skills`, `ahead of`,
  `80 s`, `self-updates`, `4001`, `never reconnect`, `NEVER`), and every hit not named here
  is added to the change.

## Interfaces
**Entrypoint (non-minimal boots).**
- The standard home dirs created as the user on every boot gain `~/.npm-global/bin` and
  `~/.npm-global/lib`.
- `/run/styrman/harness-done` is removed at the top of the boot together with
  `/run/styrman/ready`, and created when this boot's harness step is over, whatever the
  outcome (deployed, kept, stamp already current, failed).
- The harness block is split in two; its conditions, log lines and stamp are unchanged:
  - Before ready, as today, when the stamp differs: `harness-migrate.sh` (20 s cap), the
    never-downgrade decision, and the two MCP registrations (20 s each). They stay in front
    because they write `~/.claude.json`, which the first login shell's credential snippet
    (`configs/styrman-credentials.sh:105-120`) and the user's own `claude` also write: before
    ready no terminal exists, so nothing can race them (D8). Worst case before ready: 60 s,
    under the server's 90 s ready wait. The registrations now run BEFORE the install instead
    of after it; neither depends on the other's files (see Risks).
  - After the `project … ready` line, in the background: `sh-agent install` (120 s cap) when
    the decision was "install", then the stamp and the pass or WARN lines, then
    `touch /run/styrman/harness-done`. When the stamp was already current the background
    step only writes the marker.
- The background step runs in a subshell of the entrypoint's own shell, so it reads the
  foreground's results (`HM_OK`, `HR_OK`, the decision, `HARNESS_ID`) as inherited
  variables; the stamp condition is the same three-way AND as today.
- `harness-done` is written on EVERY non-minimal boot, also when `/styrman/bin/sh-agent` is
  missing or not executable (today's guard at `entrypoint.sh:147` skips the whole block): a
  waiter must never time out on an image without the binary (D15).
- `harness_home_newer` stays a top-level function (its test extracts it by its boundaries).
- A shutdown does not wait for a running install: the stamp is written last, so an
  interrupted install is repeated on the next boot (install is idempotent).

**Image.** agy's binary lives at `/opt/agy/agy`; `/opt/agy` is mode 0777; `/usr/local/bin/agy`
is a symlink to it. The build fails if the installer leaves no agy to move. Root never
executes agy after the build: the entrypoint's one call is under `runuser -u <user>`
(`entrypoint.sh:239-241`), `styrman-setup` runs as the user, and the server execs as the guest
user; the project user is root-equivalent in its own VM already (passwordless sudo,
`entrypoint.sh:55`), so the mode opens no boundary.

**Terminal WebSocket.** A text frame `{"type":"ping"}` from the client is answered with the
text frame `{"type":"pong"}`, written with a 5 s bound; a failed write ends the attachment
like any other write failure. A ping is NOT activity: it does not call the idle reaper's
`touch` (today every client frame does, `bridge.go:438`; a 25 s ping would keep a project
with an open tab awake forever). Every other text frame is handled as today (resize, or
ignored), activity included. Go, beside `controlMessage`:
```go
// classifyControl parses one text frame. ok is false when it is not a control
// message the bridge knows how to read (the frame is then ignored, as today).
// reply is non-nil when the frame must be answered. activity is false only
// for a ping.
func classifyControl(data []byte) (msg controlMessage, ok bool, reply []byte, activity bool)
```
ping → (`{Type:"ping"}`, true, pong bytes, false); resize → (the parsed message with its cols
and rows, true, nil, true); unparsable or unknown type → (zero value, false, nil, true). The
caller resizes on `ok && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0`, as today. The
reply lives in the shared `serve`, so `/ws/setup` answers too; no client sends a ping there (D9).

**`web/src/lib/termLiveness.ts`.** One instance per WebSocket, created with it in `connect()`
and stopped when that socket closes or is dropped.
```ts
export interface Liveness {
  opened(): void;    // socket opened: send one ping, no deadline (capability discovery)
  received(): void;  // any frame arrived: clear a pending deadline
  pong(): void;      // a pong arrived: this connection answers pings; clear the deadline
  probe(): void;     // if this connection has answered a ping and no deadline is pending:
                     // send a ping, start the deadline; on expiry call onDead exactly once
  stop(): void;      // clear timers; nothing fires after this
}
export function createLiveness(o: { sendPing: () => void; onDead: () => void; timeoutMs?: number }): Liveness; // default 5000
```
A connection that never answered a ping (a server without this change) is never declared dead (D7).

**Terminal pane (project terminals only; a setup pane takes none of this).**
- Text frames from the server are control messages (`pong` → `liveness.pong()`); binary
  frames are PTY bytes; both call `liveness.received()`.
- Wake events: `visibilitychange` to visible, and `online`. On a wake event:
  - socket open → `liveness.probe()`;
  - socket still connecting → nothing (its open handler pings);
  - `disconnected`, or waiting on a reconnect timer → reconnect now, retry counter reset, no
    boot intent;
  - `replaced` → on `visibilitychange` to visible only: reclaim (reconnect in place, same
    xterm, scrollback kept, no boot intent). `online` never reclaims.
- A 25 s interval calls `liveness.probe()` while the document is visible, and does nothing
  else: it never reconnects a `disconnected` pane and never reclaims.
- `onDead`: the socket's handlers are detached, it is closed, and a reconnect starts at once
  with the retry counter reset and without boot intent; state `reconnecting`.
- `replaced` also shows a "Use here" button that reclaims.
- `exited`, `stopped`, `revoked` are untouched by every trigger.

**integration.sh.** The state dir is `mktemp -d /var/tmp/styrman-itest-XXXX` (was `/tmp`).
The boot-budget check reads the state dir's filesystem type with `stat -f -c %T`, names it
in its label, and FAILS with "state dir on tmpfs: the boot budget cannot be measured" when it
is tmpfs — fail closed, no SKIP, so a release gate never reads an unmeasured budget as met.
No environment knob.

**wsprobe.** `-control <json>` sends one text frame after connecting; a text frame received
is printed as `[text:<data>]`.

## Failure modes
- Install fails or times out: the existing WARN lines, no stamp, `harness-done` still
  written; the project is usable without the harness and the next boot repeats it.
- A `claude`/`agy` started before the install finishes sees no skills until it is restarted
  (accepted, D3). The browser MCP registration is in place before the first terminal.
- A user's `sh-agent update` typed during the first seconds after a boot that seeds can be
  overwritten by the image's older release; running it again fixes it (accepted: the binary
  has no lock, and the window is the 13 s after a first boot or a release).
- A user editing `~/.claude/settings.json` in that same window can lose the edit or the
  attribution key to the installer's read-modify-write (accepted, same window).
- The seed's `claude mcp add` runs the image's claude while the user's shell may run a newer
  one from `~/.npm-global`: both write the same `~/.claude.json` shape (accepted).
- agy update interrupted: agy's own `.old` file remains in `/opt/agy` until its next update.
- Server without pong support (a rollback under an open page): pings go unanswered, the
  connection is never declared dead, behaviour is today's.
- Reclaim or wake-reconnect against a stopped project: the server answers 4002, the pane
  shows "Project stopped" with Start, the VM is not booted.
- Two pages on one terminal, both visible: the second holds it; the first shows "Opened in
  another tab" and "Use here". Neither reconnects on its own. Two devices re-shown at the
  same moment can take the terminal from each other once each, then it rests.

## Verification
Tiers: unit, integration (Tier 2, real Kata), e2e (Tier 4, Playwright), and one hand run.
- Unit: `go test ./...` — new: `classifyControl` (ping → pong and no activity; resize keeps
  its size and is activity; garbage → ignored, activity). `cd web && npm test` — new:
  `termLiveness.test.ts`. One Go test pins the entrypoint's text (marker cleared at the top;
  the install call sits below the ready line): a cheap guard for the release gate's Tier 1,
  NOT the evidence — the order is proven by Tier 2 below.
- Integration: all NEW checks are written first, then the suite is run ONCE against the image
  as it stands (`styrman-runtime:test` built 2026-10-02: pins and harness v2.3.2, none of
  this plan's other changes) with the unfixed server. That run is the RED. Its log is
  `.agents/scratch/itest-red.log`, and its expectation is exact:
  - FAIL "ready line precedes the `agent harness deployed` line" (the discriminating check
    for the install's placement);
  - FAIL "`~/.npm-global/bin` and `lib` exist in the project home";
  - FAIL "claude doctor reports auto-updates usable" (prints `Auto-updates:`, the positive
    control, and no `Can't auto-update`);
  - FAIL "project user can write agy's directory; `/usr/local/bin/agy` resolves into it";
  - FAIL "a ping text frame is answered with a pong" (`[text:{"type":"pong"}]`);
  - the existing boot-budget check, now on a disk-backed state dir, prints its measured
    value. Measured 2026-10-02 with a scratch copy of the suite on `/var/tmp`:
    `boot to ready … measured 16.16s`, FAIL, `passed=111 failed=1 skipped=3` (log
    `.agents/scratch/itest-red-disk-backed.log`), against 3.26 s on tmpfs. So this check is
    expected red as well, at about 16 s; the margin over 15 s is about 1 s, so if a run
    reads under 15 s the value is recorded and the five checks above still decide the RED;
  - every other check passes.
  GREEN after Phase 1 (and the ping check after Phase 2's server change): `failed=0`, the
  boot-budget check under 15 s on ext4, and every harness assertion made only after waiting
  (bounded, 150 s) for `/run/styrman/harness-done`. The existing check "browser MCP server
  registered for claude and agy" (`integration.sh:166-167`), made after that wait, is the
  proof that the install does not clobber the registrations that now precede it.
  Background of the state-dir change: the 2026-10-02 run on `/tmp` printed `boot to ready …
  measured 3.26s` and `passed=112 failed=0` for the same image, because `/tmp` is tmpfs here
  and the install's per-file sync costs nothing on it (D12).
- E2E: the criteria below, with these fixtures (they are how the suite produces the
  condition, not part of the criteria):
  - Criteria 1–2 use the suite's first page (on terminal 1 after "deep link reattaches
    terminal 1") and a second page from `page.context().newPage()` (same context, so the
    same session) that opens the same terminal URL.
  - hide/show is EMULATED on the page under test: `document.visibilityState` and `hidden`
    are overridden and a `visibilitychange` event dispatched (headless Chromium does not
    hide a background page).
  - Criterion 3 uses a THIRD, throwaway page from `page.context().newPage()` on terminal 2
    (created by "second terminal opens"), with `addInitScript` registered on that page
    before its `goto`. The script wraps `window.WebSocket`: per socket instance a `frozen`
    flag drops `send` and withholds `message` and `close` events, and the wrapper records
    the text frames each socket received. The check proves the terminal works, waits up to
    5 s for that socket's first pong, freezes it, proves the freeze (a typed marker does NOT
    echo within 2 s), emulates hide/show, then types a second marker and expects its echo.
    The replacement socket is a new, unfrozen instance. Against the unfixed build no pong
    arrives, the check freezes anyway, and it fails at the last step — the right reason.
    The first page is never touched by this check.
  - `.agents/rules/e2e-checks-leave-the-suite-as-they-found-it.md`: the three checks sit
    after "deep link reattaches terminal 1"; none navigates the first page; each closes its
    extra page and removes the visibility overrides in a `finally`; 1–2 end with the first
    page holding terminal 1 again (the `finally` clicks "Use here" if it is still replaced,
    tolerant of a page where the button does not exist). The red run below is their
    deliberate-failure run: the pass mark is `failed=3` with every later check passing. The
    count line in the testing README changes in the same commit.
- Hand run (not in a suite: it downloads ~220 MB and its result moves with the publishers):
  inside a REAL booted project of the built image (a project created through a locally run
  server with a scratch state dir, entered with `docker exec -u <project user>` and an
  interactive login shell — a bare container has no seeded home, no `NPM_CONFIG_PREFIX` and
  no `~/.npm-global`, and would reproduce the original failure whatever the fix, D14):
  `agy update` then `agy --version`, and `claude update` then a new interactive login
  shell's `claude --version` and `claude doctor`.

E2E criteria:
1. A terminal opened in a second tab makes the first say it is open elsewhere; the first can
   take it back with one click and typed input reaches the shell again.
2. The same first tab takes the terminal back by itself when it is hidden and shown again.
3. A terminal whose connection stopped carrying data without closing works again, without a
   page reload, after the tab is shown again.
Each of 1–3 is first run against the unfixed build and must fail there.

Not covered by any tier: a real sleep/wake, a real network change, a real hidden tab (the e2e
emulates all three at the page's own APIs); `agy update`'s download path (hand run only); the
idle reaper actually stopping a project under a pinging tab (the unit test proves a ping is
not counted; the reaper's timer is not driven); the 25 s interval and the `online` trigger in
a browser (their logic is the unit-tested `probe()`; the e2e drives the visibility trigger).

## Phases
- [x] Cold-reader check — round 3 (the cap): `implementation ready`, `design needs revision` on three narrow notes (reviews/2026-10-02-review-plan-tool-updates-freeze-harness-v2.md); operator override 2026-10-02, same turn: "Proceed with all three (Recommended)" — the notes are resolved as D12 (approved), D14, D15
### Phase 1 — runtime image
- [x] Blocker — two Tier 1 tests red since v1.39.0; `styrman-ctl install` writes an optional key the answers file carries (D18) — verify: `go test ./internal/config/`, AI — expect: PASS; `TestStyrmanCtlInstallPlan` and `TestGCPLibEnvGetMatchesStyrmanCtl` FAIL at HEAD 1fe28a0 — docs: deploy/README.md, deploy/gcp/install.sh header, specs/styrman/deploy.md, .agents/docs/deploy/gcp.md — found in: Phase 1 Tier 1 — done 2026-10-02: both tests fail in a worktree of 1fe28a0, pass at v1.38.0 and in the tree; BUG-081
- [x] Suite first: integration.sh on `/var/tmp` with the fail-closed tmpfs guard; the five NEW checks written; wsprobe `-control`; then the RED run — verify: `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test > .agents/scratch/itest-red.log`, AI — expect: exactly the five named checks FAIL (plus the boot-budget check if its measured value is 15 s or more), everything else passes — docs: .agents/docs/testing/styrman/README.md — done 2026-10-02: `.agents/scratch/itest-red.log` shows the five named checks and the boot budget (16.40 s) FAIL; the run aborted after 107 passes because the suite file was edited under it (BUG-083), so "everything else passes" rests on the later full runs
- [x] Pins bumped and proven by a build — verify: `docker build image/` with the token secret, AI — expect: exit 0; the image prints `sh-agent v2.3.2`, `2.1.287 (Claude Code)` — docs: specs/styrman/runtime-image.md (the versions it restates) — done 2026-10-02: build exit 0; the image prints `sh-agent v2.3.2`, `2.1.287 (Claude Code)`
- [x] agy at `/opt/agy` (0777) behind the `/usr/local/bin/agy` symlink — verify: its integration check, AI — expect: PASS — docs: specs/styrman/runtime-image.md — done 2026-10-02: PASS in the verifier's Tier 2
- [x] `~/.npm-global/{bin,lib}` created at boot — verify: its two integration checks, AI — expect: PASS — docs: specs/styrman/runtime-image.md — done 2026-10-02: both PASS (the PATH clause is part of the dirs check)
- [x] Harness install in the background after ready, `harness-done` marker, 120 s cap; the false comments rewritten; integration.sh waits on `harness-done` before every harness assertion — verify: `go test ./internal/config/ -run Entrypoint`; the integration harness section, AI — expect: unit PASS; the order check PASS; boot→ready under 15 s on ext4 — docs: specs/styrman/runtime-image.md, specs/styrman/deploy.md, image/README.md, README.md — done 2026-10-02: unit PASS; order check PASS; boot→ready 3.23 s on ext4 in the verifier's run
- [x] Full suite on the rebuilt image; count line updated — verify: `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test`, AI — expect: every check passes except "a ping text frame is answered with a pong" (the server change is Phase 2) — docs: .agents/docs/testing/styrman/README.md — done 2026-10-02: `passed=118 failed=0 skipped=3` (121 checks) — the ping check passes too, the bridge's pong having moved into this phase (D17)
- [x] Hand run of `agy update` and `claude update` inside a real booted project of the built image (D14) — verify: hand, AI — expect: agy reports a newer or current version without the directory error; a new shell's `claude doctor` prints no `Can't auto-update` — done 2026-10-02: with agy 1.2.2 and claude 2.1.270 planted, agy → 1.2.14 in `/opt/agy`, claude → 2.1.287 in `~/.npm-global`, doctor clean (logs `.agents/scratch/hand-update-run*.log`)
- [x] ADR-077 (in-project tool updates; amends ADR-069's note on agy), ADR-078 (harness install after ready; amends ADR-074's boot placement) appended — verify: read, AI — expect: both present, ADR-069 and ADR-074 untouched — done 2026-10-02 (ADR-079 too, D17)
- [x] Checkpoint — expect: Tier 1 and Tier 2 green on the built image — evidence: testing/2026-10-02-1744-verify-tool-updates-freeze-harness-v2-phase-1.md (`## Status: PASS`; Tier 2 `passed=118 failed=0 skipped=3 aborted=0`; Go test names 330 → 334), 2026-10-02
### Phase 2 — the freeze
- [x] E2E checks 1–3 written and run red against the unfixed build — verify: `bash web/e2e/run.sh`, AI — expect: `failed=3`, exactly those three, every other check passes — done 2026-10-02: `passed=93 failed=3 skipped=2`, the three new checks, each for its own reason (`.agents/scratch/e2e-red-2.log`); a first run failed them on the fixture (`Please use browser.newContext()`), fixed before the fix (BUG-085)
- [x] `classifyControl` + the bridge's pong write — verify: `go test ./internal/terminal/`; the integration ping check, AI — expect: PASS — docs: specs/styrman/terminal-protocol.md — done in Phase 1 (D17), 2026-10-02
- [x] `termLiveness.ts` with its unit tests — verify: `cd web && npm test`, AI — expect: the new tests pass; each fails with its guard removed — done 2026-10-02: 11 tests; four guards removed one at a time each failed one test
- [x] Terminal pane: control frames, wake events, interval probe, reclaim, "Use here"; the false comments rewritten — verify: e2e 1–3, AI — expect: PASS — docs: specs/styrman/terminal-protocol.md, specs/styrman/guide.md — done 2026-10-02: e2e 1–3 PASS
- [x] `web/dist/client` rebuilt; e2e and vitest count lines updated — verify: `bash web/e2e/run.sh`, AI — expect: `failed=0` — docs: .agents/docs/testing/styrman/README.md — done 2026-10-02: `passed=96 failed=0 skipped=2`; vitest 189 tests in 16 files
- [x] ADR-079 (ping/pong control frame; a replaced terminal may be reclaimed when its page is shown again or by its button) appended — verify: read, AI — expect: present — done in Phase 1 (D17), 2026-10-02
- [x] Checkpoint — expect: Tier 1, Tier 2, Tier 4 green — evidence: testing/2026-10-02-1806-verify-tool-updates-freeze-harness-v2-phase-2.md (`## Status: PASS`; Tier 2 `passed=118 failed=0 skipped=3 aborted=0`; Tier 4 `passed=96 failed=0 skipped=2`), 2026-10-02
### Phase 3 — release
- [ ] Definition of Done: reviewer, security (the WS control frame's input parsing; image dependencies; `/opt/agy` mode 0777), documentation, `/review --docs`, bug records, change entry — verify: reports' `## Status`, AI — expect: no open high or medium
- [ ] `/preflight` cuts the next MINOR — verify: its gate report, AI — expect: tag signed and pushed
- [ ] This host takes the tag — verify: `docker ps` shows the new version, who: user (installs `/etc/styrman/gh-token`), then AI reads the result — expect: `styrman-server:<new>` healthy
- [ ] Checkpoint — expect: handoff on disk, nothing queued but the operator step — evidence: handoff path, date

## Risks
Assumptions:
- agy accepts a 0777 root-owned directory as writable and replaces a root-owned binary in it — checked by the hand run (observed once in a probe of the 1.38.0 image: 1.2.2 → 1.2.14).
- `claude doctor` runs without credentials inside the guest and prints `Auto-updates:` — checked by the RED run's doctor check, which must fail on the `Can't auto-update` line and not on a missing `Auto-updates:` line (observed in the probe).
- Harness v2.3.2 against what the entrypoint and R5 expect — checked 2026-10-02 with the release binary on this host: `sh-agent version` prints `sh-agent v2.3.2`; `state.json` carries `"repo": "embedded:v2.3.2"` on its own line (the entrypoint's `sed` reads it); `list` prints `### Skills (36)` / `### Subagents (8)`; a fresh install writes `~/.claude/settings.json` with the attribution key and nothing else, no hooks, no scripts outside skill directories; an install over a home seeded by v1.6.0 (the binary from `styrman-runtime:1.38.0`) reports `1681 written, 107 unchanged, 52 removed`, `status --home` exits 0, `python` and `anti-ui-slop` are gone, settings.json is unchanged. Tier 2's existing harness checks re-prove the fresh-seed half in the guest.
- `sh-agent install` leaves the MCP registries alone when they exist before it (`~/.claude.json`, `~/.gemini/config/mcp_config.json` are not in its manifest) — checked by the existing integration check at `integration.sh:166-167`, run after the `harness-done` wait.
- Emulating a dead socket and a hidden tab at the page's APIs is what the page would see from the real thing — not checkable in a suite; stated under Verification.

## Decisions
D1 — 2026-10-02 — Claude updates into the project home: the entrypoint creates `~/.npm-global/{bin,lib}` every boot; the home's copy (~234 MB once updated) wins on PATH — by: operator, "Both as proposed (Recommended)"
D2 — 2026-10-02 — agy updates in place in `/opt/agy` inside the container; a container rebuild falls back to the image's agy — by: operator, "Both as proposed (Recommended)"
D3 — 2026-10-02 — Harness v2.3.2 stays on ADR-074's signed release binary (not the zip), and its seed runs in the background after ready — by: operator, "Background after ready (Recommended)" ("Boot time unchanged. A claude started in the first ~11 s of a fresh project sees no skills until restarted.")
D4 — 2026-10-02 — The freeze is fixed for two inferred causes — by: operator, "Infer both, fix both (Recommended)" ("Fix the replaced-tab dead end (Use here button, reclaim when the tab is shown again) and the unnoticed dead socket (ping/pong liveness check, reconnect on wake)."); the question's long form named the triggers: "checked when the tab becomes visible, the network comes back, and every 25 s"
D5 — 2026-10-02 — The operator installs a fresh updater token on this host; no root step is run by the agent — by: operator, "I install a new token (Recommended)"
D6 — 2026-10-02 — Reclaim fires only on the reclaiming page's own hidden→visible transition and on its button. Being replaced produces no such transition, so two pages cannot trade the terminal without one of them being re-shown — by: cold read round 1 (the rationale "user action" was imprecise: the event also fires on unlock and device wake)
D7 — 2026-10-02 — Liveness is learned per connection (a deadline is enforced only after that connection answered a ping), so a page talking to a server without pong support never reconnect-loops — by: implementer
D8 — 2026-10-02 — Only `sh-agent install` moves behind ready; migrate and the MCP registrations stay in front, where nothing can race their writes to `~/.claude.json`. Narrows D3's "the seed" to its slow part; boot time is as D3 promised — by: cold read round 1 (G2)
D9 — 2026-10-02 — The pong reply is not gated by endpoint: it lives in the shared `serve`, and a gate would be a branch no client can observe — by: cold read round 1 (scope note), implementer
D10 — 2026-10-02 — Triggers are the three the operator approved (visible, online, 25 s interval); `focus` and `pageshow` are struck — by: cold read round 1 (scope)
D11 — 2026-10-02 — Browser-pane liveness is struck from this plan and listed under Follow-ups: the operator approved the terminal fix — by: cold read round 1 (scope)
D12 — 2026-10-02 — The integration suite's state dir moves from `/tmp` to `/var/tmp`, fixed, with a fail-closed tmpfs guard on the boot-budget check: a budget measured on tmpfs cannot see a sync-per-file cost, and production homes are on ext4. This is what the Latency SLA's check needs to be evidence for D3; no knob. `web/e2e/run.sh` keeps `/tmp` (it asserts no timing) — by: operator, "Proceed with all three (Recommended)" (note 2: "You approve moving the integration suite's state from /tmp (tmpfs) to /var/tmp so the boot budget is measured on real disk."); knob removed by cold read round 2
D13 — 2026-10-02 — The WebSocket wrapper for e2e criterion 3 lives on a throwaway third page (terminal 2), installed with `addInitScript` before that page's first `goto`; the suite's first page is never wrapped — by: cold read round 2 (Q1)
D14 — 2026-10-02 — The update hand run happens inside a real booted project, never a bare container — by: cold read round 3 (gap 1); operator, "Proceed with all three (Recommended)"
D15 — 2026-10-02 — `harness-done` is written on every non-minimal boot, the missing-binary case included; the background step inherits the foreground's results — by: cold read round 3 (gap 3); operator, "Proceed with all three (Recommended)"
D16 — 2026-10-02 — Defaults for round 3's non-blocking questions: the e2e wrapper intercepts the `onmessage`/`onclose` property setters; wsprobe sends `-control` before `-send`; a reclaim clears the pane's closed-for-good state — by: implementer
D17 — 2026-10-02 — The bridge's pong (`classifyControl`, the bounded reply, its unit test) and ADR-079 move from Phase 2 into Phase 1, so Phase 1's Tier 2 is green without an expected-red check; the page-side work stays in Phase 2 — by: implementer
D18 — 2026-10-02 — A blocker found by Phase 1's Tier 1 is fixed in this plan: two Go tests red since v1.39.0 (167f605), one of them a real defect — `styrman-ctl install --env-file` dropped a `CLOUDFLARE_API_TOKEN` the answers file carried once the key left the must-fill list. `install` now writes optional keys the answers file gives (`OPTIONAL_FILL`), validated; the `private_file` drift test asserts the GNU/BSD difference `deploy/gcp/lib.sh` has on purpose. Through the `bug` skill, record BUG-081 — by: implementer; found in: Phase 1 Tier 1
D19 — 2026-10-02 — The bug skill's independent diagnosis ran after the fix was drafted, not before it (the plan's cold reads had stood in for it): an `explorer` given only the symptom and the code at `1fe28a0` ranked the same two causes first (`plans/2026-10-02-recon-ui-freeze-diagnosis.md`). Its third candidate, a WebSocket handshake that never triggers re-authentication, is left as it is: the page's polls reload on an expired session (BUG-001) — by: implementer
D20 — 2026-10-02 — The e2e suite creates an explicit browser context (`browser.newContext()`), because Playwright refuses `context.newPage()` on the implicit one; D13's "same browser context" needed it — by: implementer, from the first red run
Rejected: copying agy into every home (213 MB and seconds of boot per project); `DISABLE_AUTOUPDATER` (no in-project updates at all); the whole seed before ready with a higher cap (first boots at ~15 s against the Latency SLA's hard limit); the whole seed behind ready (its `claude mcp add` would race the first login shell's write of `~/.claude.json`); server-pushed heartbeats in place of ping/pong (a wake would wait a full interval before noticing); serialising terminals on `harness-done` (it would put the 13 s back in front of the first prompt); a SKIP for the boot budget on tmpfs (a gate would read an unmeasured budget as met).
