---
owner: main
date: 2026-09-13
---
# Plan — Browser view review fixes (v1.29.0)   (2026-09-13)

## Why

A `/code-review` of the v1.27.1..v1.28.0 range (the Browser view, ADR-068:
in-guest launcher, server CDP relay, SPA pane) returned 15 findings and a
set of below-the-cut items. The operator's instruction (2026-09-13): "the app
is working. Make sure the code is excellent and works super optimized. When
done if you need changes do a minor and execute a /preflight and then git
commit push". The findings are fixed here; the release is a MINOR (new
behaviour reaches the pane: Retry through the REST start with the launcher's
full log, revocation of an open browser relay, the screencast pausing while
the pane is hidden).

## Scope

In: the 15 findings below, plus the below-the-cut items marked *do*.
Out (rebutted or deferred, with reasons): items marked *skip*, and — after
the cold-read (round 1) — a **Stop control in the pane**: it would kill the
Chromium the agents share (ADR-068 D1; `do_stop` takes the whole process
group) and every other pane's relay, which makes it a new destructive
user-facing verb needing its own scope confirmation, confirmation dialog and
close code, not a review fix. The REST `POST /browser/stop` stays (operator
API, integration tier); the SPA's unused `api.browserStatus`/`api.browserStop`
wrappers are removed (dead code), `api.browserStart` becomes live (Retry).
Follow-ups noted, not built: see the end.

## Findings → fixes

| # | Where | Defect | Fix |
|---|-------|--------|-----|
| 1 | `image/configs/styrman-browser` `do_mcp` | `exec chrome-devtools-mcp` inherits fd 9 (the start lock opened by `do_start`); the MCP server holds the lock for its life, so the pane's next `start` blocks 20 s and fails | `exec … 9>&-` |
| 2 | `internal/httpapi/browser.go` `relayWebSockets` | no revocation watch: a user removed from the allowlist keeps an open browser relay | export `terminal.RevocationWatch`; the Server keeps the SAME `stillAuthorized` variable `New` already builds for the bridge (with its `email == "" \|\|` exemption for the loopback dev listener, nil in AuthMode=none); the relay closes the pane with `CloseAccessRevoked` (4003) when the check turns false |
| 3 | `web/src/routes/p.$project.$term.tsx` | `browserUsed` (and the editor/diff latches) latch across terminal switches: every terminal visited after the first browser use mounts a hidden `BrowserPane` = one relay + one launcher exec each | one latch keyed by `${project}/${term}`, reset on key change |
| 4 | `BrowserPane.tsx` `applyViewport` / `attach` | a hidden pane (box 0×0) pushes a 320×200 viewport onto the agent's browser and keeps the screencast running | `visible` prop mirrored in a ref; `applyViewport` returns at 0×0; ONE writer for the screencast: `setScreencast(on)` guarded by `screencastRef` (the sessionId currently streaming, or null) — `attach` calls it after the metrics when visible, the visible effect calls it (off on hide; metrics, then on, on show) when a session exists; a second call for the same session is a no-op, so the effect flipping during `attach`'s awaits cannot double-start. A mount-while-hidden (a pending "Open in Browser" when the user clicks Terminal) therefore attaches without a screencast until shown |
| 5 | launcher `max-children=4` vs server caps 8/32; `/json/version` keep-alive parks a socat child | the 5th connection per project is refused | `req.Close = true` on the discovery GET (the dial is hijacked, never pooled); `max-children=16` ≥ 2 × `browserRelayPerUser`, pinned by a Go test that reads the launcher |
| 6 | `browserInput.ts` `keyParams` | `Ctrl+Alt` (AltGr) counted as a chord → no `text` → AltGr characters never insert | `isChord = (ctrl \|\| meta) && !alt`, exported, used by the pane's own shortcuts too; test |
| 7 | `BrowserPane.tsx` `attach` | two attaches interleave (two rapid tab clicks): an orphan screencast session; `activeRef` set only on success | serialize through a promise chain held in a ref; mark `activeRef`/`setActive` before the await, revert on rejection |
| 8 | launcher `kill_group_file` | pid files survive a VM restart (bind-mounted home) and a recycled pid gets `kill -TERM -- -pid` | predicate, exactly: signal the group when `pgrep -g <pid>` reports members AND (the leader's `/proc/<pid>/comm` matches the prefix — `chrom*` for the browser leader, `socat` for the forwarder — OR the leader is gone, i.e. `/proc/<pid>` is absent: that is `do_start`'s failure path, a dead leader whose zygote/renderer children still hold the group, launcher:193-195). Drop the file WITHOUT signalling when the group is empty, or when the leader exists and its comm contradicts the prefix (a recycled pid). The residual — a same-uid process that reused exactly this pid, became a group leader, forked and died — is the user's own processes and is accepted. BOTH callers pass the prefix: `do_stop` (two calls) and `ensure_forwarder`'s peer-changed branch (line 110) — the rule in `.agents/rules/mechanism-reaches-every-caller.md`. NOT "clear both pid files at launch": the forwarder's lifetime is independent of Chromium's by design (`ensure_forwarder` starts it after a browser the MCP server started) |
| 9 | `BrowserPane.tsx` canvas | a drag that ends outside the canvas never sends `mouseReleased` — stuck button in the guest | pointer capture on pointerdown (mouse events follow the capture), released on pointerup |
| 10 | `browser.go` `closeReason`/`tail`; pane | `closeReason` keeps the FIRST 120 bytes ("browser did not start: chromium did not answer…") and loses the log lines; `tail` byte-slices (can split a rune); the SPA never uses the REST start; a rejected handshake's `client.close()` overwrites the reason just set | `closeReason` keeps the END ("…" + last bytes, cut on a rune boundary — the "browser did not start: " head goes; the pane's own heading says it); `tail` rune-safe; Retry (the button of the `unavailable` phase only; Reconnect elsewhere just bumps `epoch`) = `POST /browser/start` first (its 502 body is the full 2 KiB tail; 409 → phase "stopped"; 503 → the one automatic retry rule; success → reconnect). Retry therefore costs three guest execs (REST start + its status, then the relay's own start) — intended: it runs only on an explicit click after a failed launch. The status is read off the exported `ApiError` (`e instanceof ApiError ? e.status : 0`); close the client BEFORE setting reason/phase. Tests `TestBrowserRelayCloseCodes` (short reason, unchanged) and `TestCloseReasonFitsAndKeepsRunes` (rewritten: asserts the END is kept and no split rune) |
| 11 | launcher `forwarder_up`, `ensure_forwarder` | `forwarder_up` matches ANY :9222 listener; `SOCATRANGE` written before the spawn; a server-run start with no usable peer exits 0 and the pane fails later with "endpoint unreachable" | `forwarder_up` = pid in `socat.pid` alive with comm `socat` AND listening on `:9222`; write the range after the listener is confirmed; when `STYRMAN_BROWSER_PEER` is SET but not an IPv4 literal, exit 1 with the reason (unset = MCP-started: loopback-only, exit 0; unset with a non-IPv4 broker fallback: also exit 0). The "is SET" test lives in ONE place, `peer_range` (its existing `+set` test): it prints the range and RETURNS 1 only for set-but-unusable; `ensure_forwarder` does `range="$(peer_range)" || { echo …; return 1; }`. REST contract change, said out loud: a server whose `BrowserPeer` is "" (containerized server not on the project network / inspect failed; host-run server whose network has no IPv4 gateway) now gets 502 from `POST /browser/start` and 4004 on the relay instead of a browser that starts and cannot be reached — fail closed. `GET /browser` and `POST /browser/stop` never set the variable: unchanged. The integration tier's host-run server learns the peer from the network's IPAM gateway (`browserpeer.go:41-48`), so its checks (`integration.sh:143-146`) keep passing |
| 12 | `browser.go` `browserSocket` | `browserStart` = start exec + status exec (+ two inspects) on every relay | the relay calls `browserExec(…, "start")` directly; the REST start keeps status-after-start |
| 13 | `BrowserPane.tsx` frames + mousemove | `setFrames` re-renders React per frame; every mousemove calls `getBoundingClientRect` and sends | the JSX keeps a static `data-frames="0"` (present from first render — `run.mjs:1242` gates on the attribute's existence and `:1278` reads it before any frame) and only the UPDATES are imperative through `canvas.dataset.frames`; rect cached by the ResizeObserver + pointerenter; mousemove coalesced to one per animation frame, flushed before a press/release so order holds |
| 14 | `browser.go` `relayWebSockets` | `Conn.Read`/`Write` materialize every frame (≤16 MiB); the 1009 branch compares a wrapped `ErrMessageTooBig` against a close status and never fires | per message: `Reader` → `Writer(typ)` → `io.CopyBuffer` with a `sync.Pool` of 32 KiB buffers → `Close()` (one frame per message, type carried through). A source error mid-message ends the relay — both sides closed — so no truncated frame is ever delivered as complete. `errors.Is(err, websocket.ErrMessageTooBig)` → pane closed 1009 "frame too large"; test with an upstream frame over `browserRelayReadLimit` |
| 15 | `internal/orchestrator/ports.go` | two execs per 7 s poll (`ss` then `docker ps`) | one script with a sentinel line; pure `splitPortScan` + `parseDockerPorts`; `parseListeningPorts` unchanged. The script ends `… \|\| true`, so ITS status is always 0 by design; the vanished-container guard rests on the DOCKER exec's code and error (`ports.go:85-94`, BUG-023) as before. Match rule: the FIRST line exactly equal to `--styrman-docker--` splits; everything before is the scan, everything after the docker lines. Both halves are guest-controlled (a comm, an image name), so a substring or last-occurrence rule would let an image named like the sentinel move the boundary; with the exact-first rule the worst a guest can do is hide its own ports from its own panel. Sentinel absent (an older runtime image, or truncated output) = the whole output is the scan, empty image map — never "no ports" |

Below the cut — *do*: `deploy/styrman-ctl` global flags before the verb
(`styrman-ctl --dry-run status`); the `setup_auto_update` trap naming a
`local` (the root-only token temp dir leaks after the function returns);
`integration.sh` port guard fails open under `pipefail` when `ss` is missing
(fail closed: require `ss`, capture then grep); `AGENTS.md` "82 checks"
(point at the README, which is the source of truth); `img.onerror` drains
`pending`; `autoRetries` reset on a successful open; three reconnect
handlers → one `reconnect()`; a Go test pinning the launcher's
`PORT`/`CHROME_PORT`/`max-children` to `config`/`browserRelayPerUser`.

Below the cut — *skip*: `entrypoint.sh` mcp-add "unbounded" (it is once per
boot under a 20 s timeout, and a cap would need an operator-cleared counter —
stays the browser plan's follow-up); `server.go` liveness-failure-reads-as-dead
(an API field + sidebar change; BUG-039's degrade-not-500 is the deliberate
shape; noted as follow-up); `sameOriginOrAbsent` vs `csrfGuard` EqualFold
(both correct for browser-sent headers; changing a reviewed guard for
symmetry is not worth a security round); `previewPortAllowed` ×4 call sites
(one function, four honest callers); `openFileInPane` hand-listed fields
(each is commented for a reason); `styrman-ctl uninstall` under
`STYRMAN_STATE_DIR` (`STATE_DIR` already honours it, line 23); sharing the
ports/terminals polling (a SPA polling restructure, out of scope).

## Surfaces touched

- `image/configs/styrman-browser` (launcher; image rebuild at release)
- `internal/httpapi/browser.go`, `browser_test.go`, `server.go` (stillAuthorized field),
  new `internal/httpapi/browser_launcher_test.go`
- `internal/terminal/bridge.go`, `reauth_test.go` (export `RevocationWatch`)
- `internal/orchestrator/ports.go`, `ports_browser_test.go` (+ new test cases)
- `web/src/components/BrowserPane.tsx`, `web/src/routes/p.$project.$term.tsx`,
  `web/src/lib/browserInput.ts` (+ test), `web/src/lib/api.ts` (two wrappers
  removed; `ApiError` exported so the pane can branch on `status`) +
  `api.encoding.test.ts` (their two rows)
- `deploy/styrman-ctl`, `internal/config/styrman_ctl_test.go`
- `.agents/docs/testing/styrman/integration.sh`, `AGENTS.md`
- `web/e2e/run.mjs` (one new browser check), `web/dist/client` (rebuilt)
- Docs: `specs/styrman/guide.md` (Browser view: hidden pane pauses the
  screencast; Retry shows the launcher's log), `specs/styrman/terminal-protocol.md`
  (4003 on the browser relay; 1009 "frame too large"), `image/configs` comments;
  ADR-068 amendment via `changelog`.

## Interfaces

- `terminal.RevocationWatch(ctx, interval, check func() bool, onRevoked func())` — the
  existing unexported function, exported; behaviour unchanged (nil check never fires).
  `httpapi` already imports `terminal`; a second copy of a 20-line ticker is worse
  than the import.
- `Server.stillAuthorized func(email string) bool` = the variable `New` builds
  today (`server.go:228-236`), stored on the struct and still handed to
  `NewBridge`; `Server.browserReauthEvery time.Duration` (60 s; tests shrink it).
- `relayWebSockets(ctx, pane, upstream *websocket.Conn, stillAuthorized func() bool, every time.Duration)`;
  nil check = no watch. The watch goroutine only SIGNALS (a `revoked` channel);
  the select's `revoked` case is the one place the pane is closed
  (`CloseAccessRevoked`, "access revoked") and the upstream 1000 — so the test
  asserts the CLIENT observed 4003, deterministically.
- `browserSocket` start path: `code, out, err := s.browserExec(ctx, u, p, guest, "start")`;
  `err` → 4002 if `errProjectNotRunning`, else 4004 "browser is not available";
  `code != 0` → 4004 with `closeReason(tail(out, 2048))`.
- `closeReason(msg)`: ≤120 bytes; when cut, "…" + the END on a rune boundary.
  `tail(s, max)`: rune-safe (advances the cut to a rune start).
- `browserEndpoint`: `req.Close = true`.
- ports: `portScanScript` ends with `echo '--styrman-docker--'; docker ps --format '{{.Image}}|{{.Ports}}' 2>/dev/null || true`;
  `splitPortScan(out) (scan string, images map[int]string)` — split at the first line exactly equal to the sentinel; no sentinel → `(out, map{})`;
  `parseDockerPorts(lines string) map[int]string`.
- Launcher: `kill_group_file <pidfile> <comm-prefix>` (callers: `do_stop` ×2,
  `ensure_forwarder` ×1) with the finding-8 predicate; `forwarder_up` reads
  `socat.pid`; `peer_range` returns 1 for set-but-unusable (prints nothing),
  0 otherwise; `ensure_forwarder` exits 1 with the message on that return;
  `do_mcp` execs with `9>&-`; socat `max-children=16`.
- SPA: `BrowserPane({project, term, visible, onShowTerminal})`; `Phase` unchanged
  (`connecting | ready | stopped | unavailable | disconnected`); `isChord(e)` in
  browserInput.ts; `setScreencast(on)`/`screencastRef` inside the pane;
  `export class ApiError` in api.ts; the route's `useViewLatch(key, pane)` returning
  `{editor, diff, browser}` booleans; `api.browserStatus`/`api.browserStop` deleted.
- `styrman-ctl main`: flags (`--dry-run`, `--yes`, `--confirm WORD`) are
  collected from the WHOLE argv, `--confirm` consuming its value wherever it
  sits (as the current loop already does after the verb); verb = first
  non-flag; no verb = the menu (under `DRY=1` when given, exactly as today's
  `styrman-ctl` alone but planning only). `__*` internal verbs receive the
  remaining args as today.
- `styrman-ctl` temp dir: global `CTL_TMPD=""` at the top; the trap body is
  `[ -n "${CTL_TMPD}" ] && rm -rf "${CTL_TMPD}"` (an empty variable removes
  nothing — the BUG-006 guard); after the normal cleanup, blank the variable
  and clear all four signals (`trap - EXIT INT TERM HUP`).

## Failure modes

- Revoked while the pane is open: close 4003, pane phase "disconnected" with
  reason "access revoked"; Reconnect fails the handshake (403) as today.
- Retry after a failed launch: the POST's 502 body (launcher tail) is shown in
  the `<pre>`; a 409 (project stopped) shows "Project is stopped"; a 503
  (fresh attach) is retried once by the same one-retry rule; success → reconnect.
- Server-run start with no peer: 4004 "…no usable peer address for :9222
  (STYRMAN_BROWSER_PEER is set but not an IPv4 address)" — and 502 with the
  same text from the REST start.
- Stale pid with a recycled pid: the file is removed, nothing is signalled.
- A stranger on :9222 (the user's own app): `forwarder_up` false → the launcher
  tries to bind, socat fails, `start` exits 1 with "forwarder did not come up"
  (the log tail names the bind error).
- Oversize frame from the guest: pane closed 1009 "frame too large" (the
  pane treats it as a disconnect with Reconnect, as today's protocol doc says).

## Verification

- Unit: `go test ./...` (httpapi: revocation close 4003 observed by the client,
  relay start path = exactly one `start` exec and zero `status` execs,
  closeReason tail, tail rune-safe, `Connection: close` on the discovery GET,
  1009 on an oversize upstream frame, launcher pins; orchestrator: splitPortScan
  with and without the sentinel, parseDockerPorts; config: `--dry-run status`
  ordering; terminal: RevocationWatch tests renamed),
  `cd web && npm run check && npm run lint && npm test` (isChord, keyParams AltGr).
- Launcher: `bash -n`, `shellcheck`; the integration tier's browser checks
  against a rebuilt `styrman-runtime:test` (start/status/stop, ports omit 9229,
  MCP registration, 9222 up and hidden).
- e2e (`bash web/e2e/run.sh`): existing browser checks + one new check,
  "switching to the terminal and back resumes the screencast" — inserted
  between "Open in Browser from the Ports panel…" and "terminal session
  survived the browser round-trip", i.e. INSIDE the LAN window and BEFORE
  "disabling returns it to sandbox" (whose rebuild kills the fixture and the
  browser — BUG-021's seam). It reads the frame count, clicks Terminal, clicks
  Browser, and waits for count + 1: `Page.startScreencast` always emits a
  first frame, so the count rises only if the show path restarted it.
- E2E criteria: open the Browser view, load the fixture, switch to Terminal and
  back (a frame arrives), Open in Browser from Ports navigates.

## Phases

### Phase 1 — Guest launcher (findings 1, 5, 8, 11)
- [x] `do_mcp` execs with `9>&-` — verify: `bash -n`, shellcheck; integration "MCP registered" check unchanged (AI)
- [x] `kill_group_file <pidfile> <comm-prefix>`; all three callers pass it — verify: `grep -n kill_group_file` shows 4 lines (definition + 3 callers, each with two args); integration start/stop check (AI)
- [x] `forwarder_up` by pid+comm+listen; range written after confirmation; set-but-unusable peer exits 1 — verify: integration "9222 up, hidden" check (AI)
- [x] `max-children=16` with the comment tying it to the server cap — verify: Go pin test (Phase 2) (AI) — Phase 1 tier evidence 2026-09-13: integration `passed=90 failed=0 skipped=3` on the rebuilt `styrman-runtime:test`, all six browser checks PASS

### Phase 2 — Server relay + ports (findings 2, 5, 10, 12, 14, 15; pin test)
- [x] export `terminal.RevocationWatch`; `Server.stillAuthorized`/`browserReauthEvery`; relay watch → 4003 — verify: `TestBrowserRelayClosesOnRevocation` (client sees 4003) (AI)
- [x] relay start path = one `browserExec("start")` — verify: `TestBrowserRelayPumpsBothWays` asserts exactly one start AND zero status execs (AI)
- [x] `closeReason` keeps the tail; `tail` rune-safe — verify: `TestCloseReasonFitsAndKeepsRunes` rewritten (AI)
- [x] `browserEndpoint` `req.Close = true` — verify: fakeDevTools records `Connection: close` on `/json/version`, asserted in the pump test (AI)
- [x] pump via `Reader`/`Writer` + pooled `io.CopyBuffer`; `errors.Is(ErrMessageTooBig)` — verify: `TestBrowserRelayOversizeUpstreamFrameIs1009` + existing relay tests, `-race` (AI)
- [x] ports single exec with sentinel; pure split/parse — verify: `TestSplitPortScan` (with / without sentinel), `TestParseDockerPorts`, existing hide-browser-ports test (AI)
- [x] `internal/httpapi/browser_launcher_test.go` reads `../../image/configs/styrman-browser` and pins `PORT`, `CHROME_PORT`, `max-children` — verify: `TestLauncherPinsPortsAndCaps` (AI)

### Phase 3 — SPA (findings 3, 4, 6, 7, 9, 10, 13; onerror drain; autoRetries; one reconnect; dead wrappers)
- [x] `isChord` + `keyParams` AltGr — verify: vitest (AI)
- [x] route `useViewLatch` keyed per terminal; `visible` prop — verify: `npm run check`, e2e round-trip check (AI)
- [x] BrowserPane: visible → screencast stop/start (attach honours it); 0×0 guard; serialized `attach`; pointer capture; imperative frames; cached rect + rAF mousemove with flush; Retry via POST; close-before-set; onerror drain; autoRetries reset; one `reconnect()` — verify: check/lint/test + e2e (AI)
- [x] `api.ts`: delete `browserStatus`/`browserStop` + their `api.encoding.test.ts` rows — verify: `npm run check`, `npm test` (AI)
- [x] e2e: "switching to the terminal and back resumes the screencast" at the named insertion point — verify: `bash web/e2e/run.sh` (AI) — 2026-09-13: passed=92 failed=0 skipped=2 (`.agents/scratch/e2e-2026-09-13.log`; the run was split into a foreground `npm run build` + `STYRMAN_E2E_SKIP_WEB=1` after the harness killed the first attempt during the vite build)

### Phase 4 — Ops scripts, docs, release
- [x] `styrman-ctl` flag ordering + `CTL_TMPD` trap — verify: `TestStyrmanCtl…` new case (`--dry-run status` ≡ `status --dry-run`); `bash -n` (AI)
- [x] `integration.sh` guard fails closed — verify: `bash -n`; a run with `ss` shadowed by a failing stub exits 1 before any check (AI)
- [x] `AGENTS.md` count → README pointer — verify: read (AI)
- [x] Definition of Done: verifier (all tiers named above), reviewer (diff), security (relay/auth/launcher), documentation, changelog (+ADR-068 amendment), bugs — then `/preflight v1.29.0` — DONE 2026-09-13: gate `.agents/docs/reviews/2026-09-13-preflight-v1.29.0.md` (SHIP), tag v1.29.0 on b6bc2bf, live at 10:24 (handoff `.agents/docs/handoffs/2026-09-13-session-end-v1.29.0.md`)

## Risks

- Stopping the screencast on hide: a tab the agent navigates while hidden shows
  its new frame only after show + `startScreencast` — by design, the strip is
  still live (target events keep flowing).
- `max-children=16`: a user with 8 panes on one project plus discovery GETs fits;
  the MCP server uses loopback, never socat.
- The launcher change needs the runtime image rebuilt at release (the updater
  does it) and re-seeds nothing (no harness change).
- Finding 11 turns an unknowable peer into a visible failure; a deployment that
  relied on "the browser starts, only the pane fails" sees a 502 now. Production
  is containerized with the static attach, so its peer is known.

## Follow-ups noted, not built

- A Stop control in the pane (needs a scope decision: shared browser, other
  panes' close code, confirmation naming the agents).
- A `liveUnknown` flag on the terminals listing when the liveness scan fails.
- A cap on consecutive harness re-seed attempts (from the browser plan).

## Cold-read trail

- Round 3: `.agents/docs/reviews/2026-09-13-coldread-browser-review-fixes-round3.md` —
  both prior rounds discharged; seven remaining items, each an implementation
  detail with one non-scope-changing answer, decided here (the three-round cap
  is reached; the operator's standing instruction for this task was
  autonomous — "do a minor and execute a /preflight and then git commit push" —
  so these decisions are recorded rather than asked, and the handoff flags them
  for review):
  1. A `start` that fails in `ensure_forwarder` (peer set but unusable, or a
     stranger on :9222) leaves Chromium RUNNING — `do_start` returns 1 without
     `do_stop` on that path; only the launch timeout (launcher:190-196) tears
     the group down. Idempotent: a later start with a good peer succeeds and
     the agents' MCP session is untouched.
  2. The relay's destination `Writer` is closed (FIN) only when
     `io.CopyBuffer` returned nil; on a copy error it is abandoned and the pump
     returns, so no truncated frame is ever completed.
  3. `ensure_forwarder` keeps its `[ -z "${range}" ] → return 0` branch (unset
     peer, loopback-only) beside the new `peer_range` non-zero branch.
  4. `forwarder_up` false while `:9222` listens (a stranger, or a live socat
     whose pid file is gone) → its own message, "something else listens on
     :9222 (not the forwarder this launcher started)", exit 1 — fail closed,
     no bind attempt.
  5. Retry: any status other than 502/409/503 (0, 404, 500; 401 is api.ts's
     reload path) stays `unavailable` and shows the message; the button is
     disabled while the POST is in flight.
  6. The 1009 test sends a real message over `browserRelayReadLimit` (16 MiB +
     1) through the loopback relay; the constants stay constants.
  7. `styrman-ctl`: "flag" means exactly `--dry-run`, `--yes`, `--confirm <v>`;
     every other token, `--help`/`-h` included, is a verb candidate, as
     today's `*)` arm already treats it.

- Round 2: `.agents/docs/reviews/2026-09-13-coldread-browser-review-fixes-round2.md` —
  one blocking (the comm check would have defeated `do_start`'s orphan
  cleanup: predicate rewritten in finding 8) and six non-blocking (ApiError
  exported; sentinel exact-first-line rule; one screencast writer guarded by
  `screencastRef`; the `+set` test stays in `peer_range` with a return code;
  static `data-frames="0"`; Retry's three execs intended and confined to the
  `unavailable` phase) — all decided above.

- Round 1: `.agents/docs/reviews/2026-09-13-coldread-browser-review-fixes.md` —
  design needs revision / implementation not ready. Every gap addressed above
  (Stop control struck; the second `kill_group_file` caller named; Stop
  ordering moot; `stillAuthorized` = the bridge's variable; e2e insertion point
  named; sentinel-absent behaviour; finding 11 contract change stated;
  pin-test home; `CTL_TMPD` guards; mount-while-hidden; frame-boundary
  semantics; `styrman-ctl` flag edge cases; the 4004 tail decision).
