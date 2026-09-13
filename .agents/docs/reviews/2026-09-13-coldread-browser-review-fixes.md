---
owner: reviewer
date: 2026-09-13
---
# Cold-reader review — plan 2026-09-13-browser-review-fixes (v1.29.0)

Range reviewed: the plan file `.agents/docs/plans/2026-09-13-browser-review-fixes.md`
(no diff; nothing implemented yet — `git status` clean at 0378825) plus the code
surfaces it names: `image/configs/styrman-browser`, `internal/httpapi/browser.go`,
`browser_test.go`, `server.go`, `internal/terminal/bridge.go`,
`internal/orchestrator/ports.go`, `web/src/components/BrowserPane.tsx`,
`web/src/routes/p.$project.$term.tsx`, `web/src/lib/{browserInput,cdp,api,editorPane}.ts`,
`deploy/styrman-ctl`, `.agents/docs/testing/styrman/integration.sh`,
`web/e2e/run.mjs:1230-1330`.

## Verdict: REQUEST CHANGES (design needs revision / implementation not ready)

The plan is unusually precise — 15 findings, each with the mechanism and the
test that pins it, and every diagnosis I checked against the code is correct
(the `main()` verb-before-flags bug at `deploy/styrman-ctl:923`, the latch at
`p.$project.$term.tsx:56`, the `browserStart` double-exec at `browser.go:122-129`,
the two-exec poll at `ports.go:72-119`, `keyParams`'s `chord` at
`browserInput.ts:155`, the `local tmpd` trap at `styrman-ctl:533-535`). What it
does NOT decide is the one thing that is not a refactor: **a Stop control that
kills a browser shared with running agents** (R4/ADR-068), with no decision on
confirmation, on the other open relays it tears down, or on what the agents'
MCP session does afterwards. Two smaller gaps will bite in the first hour of
implementation: a second `kill_group_file` caller the plan never mentions
(`set -u` abort), and a Stop flow whose stated ordering makes the new `off`
phase unreachable.

## (1) Gaps — what must be decided before implementation starts

- **[high / confidence: high] Stop kills the agents' browser too, and the plan
  never says so.** `POST /browser/stop` → `browserStop` (`browser.go:132-141`) →
  the launcher's `do_stop` (`image/configs/styrman-browser:216-221`), which
  `kill -TERM`s Chromium's whole process group. That is the SAME Chromium the
  agents drive through `chrome-devtools-mcp` (`do_mcp`, launcher:232-239) and the
  same one every other pane of that project is relayed to. ADR-068/R4's premise
  is a shared browser. The plan adds the control ("a Stop control (`POST
  /browser/stop`, new `off` phase with Start)", finding 10) and a guide line, but
  decides none of: does the operator get a confirmation naming the agents? do
  other open relays get a specific close code instead of the generic 1011
  "browser disconnected" (`browser.go:466-471`)? is an agent's in-flight MCP
  session expected to recover on its next tool call (it will re-`start`, with a
  fresh profile-less browser and every tab gone)? Decide before code: this is a
  new destructive user-facing verb, not a review fix.
- **[high / confidence: high] `ensure_forwarder` is a second caller of
  `kill_group_file` and the plan only re-fits `do_stop`.** Interface says
  `kill_group_file <pidfile> <comm-prefix>`; Phase 1 says "`do_stop` passes the
  prefixes". `image/configs/styrman-browser:110` calls
  `kill_group_file "${SOCATPID}"` with ONE argument, inside `ensure_forwarder`'s
  peer-changed branch. Under the file's `set -euo pipefail` (launcher:47) a
  `local want="$2"` in the new signature is an unbound-variable abort on the
  exact path that exists to recover a moved server address. Name the call site
  (`.agents/rules/mechanism-reaches-every-caller.md`, BUG-035 family) or give the
  prefix a default.
- **[high / confidence: high] The stated Stop ordering makes the `off` phase
  unreachable.** Failure modes say "the client closes first, then `POST
  /browser/stop`". `CdpClient.close()` calls `handleClose(1000, "closed by
  client")` (`cdp.ts:91-100`), which fires `onClose`; the pane's handler
  (`BrowserPane.tsx:275-297`) sends any non-4002/4004 code on an opened socket to
  `setPhase("disconnected")`. So the pane shows "Browser disconnected." and the
  plan's own e2e check ("Stop browser turns the pane off") fails. Decide the
  suppression (a `stoppingRef` consulted in `onClose`, or set `off` after the
  close) and decide what phase a FAILED stop leaves behind — "stays on the page"
  is ambiguous once the socket is already gone: the relay is dead but the
  browser is alive, and `off`+Start would then be a lie.
- **[medium / confidence: high] `Server.stillAuthorized` omits the empty-email
  exemption the bridge depends on.** The plan's interface says only "nil in
  AuthMode=none". The live closure is `func(email string) bool { return email ==
  "" || users.IsAuthorized(email) }` (`server.go:228-236`) — the `email == ""`
  arm exists because the loopback dev listener mints an identity that is in no
  store (`identity.User.Email` is "" there, `identity.go:40`). An implementer who
  writes `users.IsAuthorized(email)` from the plan's text alone closes every dev
  and e2e browser relay with 4003 after `browserReauthEvery`. Restate the rule,
  or say explicitly "reuse the same variable New already builds".
- **[medium / confidence: medium] Where the new e2e check goes is undecided, and
  this file has a bug record for exactly that.** The browser checks sit inside
  the LAN window (`run.mjs:1230-1321`) and the block ends with "disabling returns
  it to sandbox", which REBUILDS the container and kills both the `:4100` fixture
  and the browser. A Stop/Start check placed after it tests nothing; placed
  before the round-trip check it changes what "terminal session survived"
  survives. BUG-021 is the same mistake at the same seam. Name the insertion
  point.
- **[medium / confidence: medium] `splitPortScan` has no defined behaviour when
  the sentinel is absent.** The verification list names a "sentinel absent" case
  but not its answer. Absent sentinel = an older runtime image (the launcher and
  the script ship in `image/`, the server does not) or truncated exec output. If
  it returns `("", nil)` the Ports panel silently reads "no ports"; it should
  return the whole output as the scan with an empty image map. Related: the
  merged script ends `docker ps … || true`, so the SCRIPT's status is always 0 —
  state that this is deliberate and that BUG-023's guard still rests on the
  DOCKER exec code (`ports.go:85-94`), not on the script's.
- **[medium / confidence: medium] Finding 11 changes the REST contract for every
  deployment whose peer is unknowable.** "peer SET but unusable → exit 1" turns
  `BrowserPeer` returning "" (`browserpeer.go:31-37,41,48` — inspect failure,
  server not on the project network, no IPv4 gateway) into a 502 from `POST
  /browser/start` AND a 4004 on the relay, where today the browser still starts
  and only the pane cannot connect. That is the right direction (fail closed),
  but say it out loud for `getBrowser`/`stopBrowser` (unchanged: they never set
  the variable) and confirm the integration checks at `integration.sh:143-144`
  are expected to keep passing on the non-containerized test server.
- **[low / confidence: medium] The launcher pin test has no home.** Phase 2 wants
  `TestLauncherPinsPortsAndCaps` to pin `PORT`/`CHROME_PORT`/`max-children`
  against `config.BrowserPort`/`config.BrowserChromePort` and
  `browserRelayPerUser` — but `browserRelayPerUser` is unexported in `httpapi`
  (`browser.go:258-264`) while the ports live in `config`. Name the package and
  how it reaches `image/configs/styrman-browser` (a relative path from the test's
  package dir).
- **[low / confidence: medium] `CTL_TMPD` under `set -u`.** `styrman-ctl:20` is
  `set -euo pipefail`. A global must be initialised (`CTL_TMPD=""`) or the
  EXIT/INT/TERM/HUP trap fires an unbound-variable error; and line 587 clears
  only `trap - EXIT`, leaving INT/TERM/HUP armed on a path that has already been
  removed — blank the variable after cleanup. A `rm -rf` under a trap on a
  variable that may be empty is BUG-006 territory; say which guard is used.
- **[low / confidence: medium] `visible` does not cover the mount-while-hidden
  case.** `requestBrowser` sets view and request together (`editorPane.ts:160`),
  so the normal path mounts visible — but a request that is still pending when
  the user clicks Terminal leaves the pane mounted, hidden, and about to run its
  first `attach()` (`BrowserPane.tsx:109-144`), which unconditionally
  `Page.startScreencast`s. Say whether `attach` skips the screencast when
  `visible` is false, or only `applyViewport` does.
- **[low / confidence: medium] Frame-boundary change from finding 14.** `Reader`/
  `Writer` + `io.CopyBuffer` streams a message; a source error mid-message leaves
  a TRUNCATED frame on the wire where today's `Read`/`Write` is atomic per
  message. `CdpClient.handleMessage` drops malformed JSON (`cdp.ts:110-115`), so
  it is not fatal, but a truncated REPLY leaves its promise pending until close.
  Also state that the destination `Writer` is closed per message (otherwise
  frames merge) and that the type is carried through. (Checked: coder/websocket
  v1.8.15's `msgReader`/`msgWriter` implement neither `WriteTo` nor `ReadFrom`,
  so the pooled buffer IS used — the mechanism works.)
- **[low / confidence: medium] `styrman-ctl` flag collection edge cases.**
  "flags collected from the whole argv, verb = first non-flag" must say what
  `styrman-ctl --dry-run` with no verb does (menu under DRY?), and that
  `--confirm WORD` still consumes its value wherever it appears — `cmd_internal`
  (`styrman-ctl:869-887`) receives whatever is left and validates paths, so a
  test-hook argument that begins with `--` would now be eaten.

## (2) Readiness — questions a first-pass implementer must ask

1. **BLOCKING** — Stop kills the browser the agents share (and every other open
   relay to that project). Confirmation dialog, or not? What close code do the
   other panes get? Is an agent's MCP session expected to just re-`start`?
2. **BLOCKING** — After a FAILED `POST /browser/stop` (the client is already
   closed), what does the pane show: `off` with Start, `disconnected` with
   Reconnect, or a notice on a dead pane?
3. NON-BLOCKING — Does `Start` from the `off` phase POST `/browser/start` first
   (like Retry) or only bump `epoch` and let the relay's own start do it?
4. NON-BLOCKING — Does `Server.stillAuthorized` keep the `email == ""` exemption
   (i.e. is it literally the variable `New` already builds)?
5. NON-BLOCKING — Which package owns `TestLauncherPinsPortsAndCaps`, given
   `browserRelayPerUser` is unexported in `httpapi`?
6. NON-BLOCKING — Where exactly in `run.mjs` does the Stop/Start check go
   relative to the sandbox-restore rebuild at ~1321?
7. NON-BLOCKING — `splitPortScan` with no sentinel: whole output as scan, or an
   error?
8. NON-BLOCKING — Does `attach()` skip `Page.startScreencast` when the pane
   mounts hidden, or is the visible-effect alone enough?
9. NON-BLOCKING — `kill_group_file`'s comm prefix: default argument, or is
   `ensure_forwarder:110` updated too?
10. NON-BLOCKING — `styrman-ctl --dry-run` with no verb: menu, help, or error?
11. NON-BLOCKING — Does the 4004 reason keeping only the TAIL (dropping the
    "browser did not start: " prefix) match the intent, given
    `TestBrowserRelayCloseCodes` and `TestCloseReasonFitsAndKeepsRunes`
    (`browser_test.go:271-317`) both assert on the head today?

## Criteria (applied to the plan as the artifact)

1. **Dead code** — pass. The plan retires genuinely unused surface: `api.browserStart/
   Stop` are referenced only by `api.encoding.test.ts:68-70` today and become live
   callers. No orphan is created; `browserStart`'s status-after-start survives for
   the REST route (finding 12) rather than being left dangling.
2. **Flow tracing** — FAIL, twice: `kill_group_file`'s second caller
   (launcher:110) and the Stop→`onClose`→phase path (`cdp.ts:99` → 
   `BrowserPane.tsx:284-296`). A third hop is under-specified: the 4003 close
   races the relay's own `pane.Close(StatusNormalClosure)` in the select at
   `browser.go:459-476`, so `TestBrowserRelayClosesOnRevocation` must assert the
   CLIENT observed 4003, not merely that the socket closed (BUG-036 family).
3. **Standards conformance** — pass. Stdlib-only (`sync.Pool`, `io.CopyBuffer`),
   no new dependency, errors keep their guard shape, tests named per change. The
   verification section respects `.agents/rules/verify-bare.md` (bare commands,
   no piped `&&` chains into a commit).
4. **Directory layout** — pass, with the one unnamed test file above (#5).
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth/crypto; the
   revocation watch REUSES the vetted `revocationWatch` rather than a second
   implementation. The one simplicity worry is exporting
   `terminal.RevocationWatch` (`bridge.go:75`) so `httpapi` can use a 20-line
   ticker helper — a `terminal` import for something with no terminal semantics;
   a local helper or a tiny shared package would decouple better. Not a finding,
   a note.
6. **Decoupling** — pass. `splitPortScan`/`parseDockerPorts`/`isChord`/
   `useViewLatch` all move logic to pure, separately testable units, and the
   phases split cleanly (launcher / server / SPA / ops) so three engineers could
   work them in parallel.

## Not examined

- `deploy/styrman-ctl` beyond `main`, `setup_auto_update`, `cmd_internal` and the
  path/DRY preamble (the plan touches only those).
- `internal/config/styrman_ctl_test.go` — the existing cases the new `--dry-run
  status` case must join.
- `web/e2e/run.mjs` outside 1230-1330, and `web/e2e/run.sh`.
- `.agents/docs/specs/styrman/{guide,terminal-protocol}.md` and the ADR-068 entry
  (docs deliverables of Phase 4).
- I did not run `go test`, `npm run check` or any tier: nothing is implemented
  yet, so there was nothing to verify.

design needs revision
implementation not ready
