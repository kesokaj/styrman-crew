---
owner: reviewer
date: 2026-09-13
---
# Cold-reader review, round 3 — plan 2026-09-13-browser-review-fixes (v1.29.0)

Range reviewed: the plan file
`.agents/docs/plans/2026-09-13-browser-review-fixes.md` (revised twice; nothing
implemented — working tree clean at 61608ca apart from the plan and the two
prior cold-read reports, all three untracked), the round-1 report
`.agents/docs/reviews/2026-09-13-coldread-browser-review-fixes.md`, the round-2
report `…-round2.md`, and the code surfaces the plan names:
`image/configs/styrman-browser` (whole file),
`internal/httpapi/{browser.go,browser_test.go,server.go}`,
`internal/terminal/bridge.go`, `internal/orchestrator/{ports.go,browserpeer.go}`,
`internal/identity/identity.go`, `internal/config/config.go`,
`web/src/components/BrowserPane.tsx`, `web/src/routes/p.$project.$term.tsx`,
`web/src/lib/{browserInput.ts,api.ts,api.encoding.test.ts}`,
`deploy/styrman-ctl` (`main`, `setup_auto_update`, `cmd_internal`, `cmd_menu`),
`.agents/docs/testing/styrman/integration.sh:20-40,115-150`,
`web/e2e/run.mjs:1230-1330`, and the vendored
`github.com/coder/websocket@v1.8.15` (`read.go`, `write.go`, `errors.go`).

## Verdict: REQUEST CHANGES (design needs revision / implementation not ready)

**Both prior rounds are fully discharged.** I checked each of round 1's twelve
gaps and round 2's six gaps + seven questions against the revised text and
against the code each cites, and every one is now decided in the plan's own
words (the table below). Nothing I re-derived contradicts the plan's diagnoses.

What remains is smaller than either earlier round and lives entirely inside
finding 11's new launcher control flow and finding 14's new pump. Two of the
three are cases where the plan states an INVARIANT ("fail closed", "no
truncated frame is ever delivered as complete") whose enabling mechanism the
interface sketch does not contain — and the obvious first-pass code makes the
invariant false. One is a scope edge the plan's own Scope section rules out but
Phase 1 re-opens without saying so: a `start` that now exits 1 while a Chromium
the agents are using is running.

## Round-1 and round-2 discharge (verified line by line)

| Prior gap | Plan's own words that decide it | Verified against |
|---|---|---|
| R2-1 comm check defeats `do_start`'s orphan cleanup | finding 8: "signal the group when `pgrep -g <pid>` reports members AND (… comm matches … OR the leader is gone …, launcher:193-195)" | `styrman-browser:193-206` — the leader-gone arm is exactly the path the comment names |
| R2-2 Retry cannot read the HTTP status | finding 10: "read off the exported `ApiError` (`e instanceof ApiError ? e.status : 0`)"; Surfaces: "`ApiError` exported so the pane can branch on `status`" | `api.ts:299` is unexported today; `api.ts:407` throws it with `.status` |
| R2-3 sentinel match rule undefined | finding 15: "the FIRST line exactly equal to `--styrman-docker--` splits"; "a substring or last-occurrence rule would let an image named like the sentinel move the boundary" | both halves guest-controlled: `ports.go:56-58` comm, `docker ps --format '{{.Image}}'` |
| R2-4 two independent screencast writers | finding 4: "ONE writer …: `setScreencast(on)` guarded by `screencastRef` … a second call for the same session is a no-op, so the effect flipping during `attach`'s awaits cannot double-start" | `BrowserPane.tsx:127-141` is the await window described |
| R2-5 where the "`+set`" test lives | finding 11: "lives in ONE place, `peer_range` (its existing `+set` test) … `ensure_forwarder` does `range="$(peer_range)" \|\| { … }`" | `styrman-browser:76` is that test; `:99-103` the caller |
| R2-6 `data-frames` may vanish | finding 13: "the JSX keeps a static `data-frames="0"` (present from first render — `run.mjs:1242` … `:1278`)" | `run.mjs:1238-1242`, `BrowserPane.tsx:572` |
| R2-Q7 Retry's three execs | finding 10: "intended: it runs only on an explicit click after a failed launch" | — |
| R1 Stop control (3 gaps) | Scope: struck, "a new destructive user-facing verb needing its own scope confirmation … not a review fix"; REST stop kept | `do_stop` at `styrman-browser:217-222` does take the whole group |
| R1 `stillAuthorized` empty-email exemption | finding 2: "the SAME `stillAuthorized` variable `New` already builds … (with its `email == "" \|\|` exemption …)"; Interfaces cite `server.go:228-236` | `server.go:235` is literally that closure; `identity.go:40` is why |
| R1 e2e insertion point | Verification: "between 'Open in Browser from the Ports panel…' and 'terminal session survived the browser round-trip' … BEFORE 'disabling returns it to sandbox' (BUG-021's seam)" | `run.mjs:1291-1322` — the named gap is real and before the rebuild |
| R1 sentinel-absent behaviour | Interfaces: "no sentinel → `(out, map{})`"; finding 15: "never 'no ports'" | — |
| R1 pin test home | Surfaces + Interfaces: `internal/httpapi/browser_launcher_test.go` | `browserRelayPerUser` is unexported at `browser.go:262`; `config` imported at `browser.go:17` |
| R1 `CTL_TMPD` guards | Interfaces: `CTL_TMPD=""` global, `[ -n … ] && rm -rf`, "clear all four signals" | `styrman-ctl:502,535,587` — today `local tmpd` and `trap - EXIT` only |
| R1 mount-while-hidden | finding 4: "A mount-while-hidden … therefore attaches without a screencast until shown" | — |
| R1 frame-boundary semantics | finding 14: "one frame per message, type carried through"; "A source error mid-message ends the relay — both sides closed" | see gap 2 below: the mechanism is missing, not the decision |
| R1 finding-11 contract change | finding 11: "REST contract change, said out loud … `GET /browser` and `POST /browser/stop` never set the variable: unchanged" | `browser.go:74-83` sets the env only for `start`; `browserpeer.go:41-48` is the integration tier's path |
| R1 4004 tail decision | finding 10: "the head goes; the pane's own heading says it"; both tests named | `browser_test.go:271-317` — `TestBrowserRelayCloseCodes` asserts a SHORT reason, so it really is unchanged |

## (1) Gaps — what must still be decided before implementation starts

- **[medium / confidence: high] Finding 11 leaves a running Chromium behind a
  failed `start`, and the plan does not say whether that is intended.**
  `do_start` calls `ensure_forwarder` only AFTER the probe succeeds
  (`styrman-browser:178-188`): on the new "peer is set but unusable" path the
  browser is already up, and `return $?` now propagates 1. The plan's Failure
  modes section describes only what the CLIENT sees ("4004 … and 502 with the
  same text"), never what the guest is left in. An implementer has two plausible
  readings, and one of them is destructive: today the ONLY failing exit from
  `do_start` runs `do_stop` immediately before it (`:193-195`), so "make the new
  failure behave like the old one" means killing the process group of the
  Chromium the agents are driving through `chrome-devtools-mcp` — precisely the
  act the Scope section struck as out of scope for this release. Decide in the
  plan: `ensure_forwarder`'s failure returns 1 WITHOUT `do_stop`, browser left
  running (idempotent; a later start with a good peer succeeds, and the agents'
  MCP session is untouched).
- **[medium / confidence: medium] Finding 14's "no truncated frame is ever
  delivered as complete" is not produced by the interface the plan sketches.**
  The sketch is "per message: `Reader` → `Writer(typ)` → `io.CopyBuffer` … →
  `Close()`". In `coder/websocket@v1.8.15`, `msgWriter.Close()` (`write.go:233`)
  is what sets FIN — it is the act that turns the bytes already written into a
  *complete* message. The idiomatic first-pass rendering of that sketch is
  `w, err := dst.Writer(ctx, typ); defer w.Close()`, which on a mid-message read
  error flushes a truncated frame to the pane as complete, making the plan's
  stated invariant false. State the rule: the writer is closed ONLY when
  `io.CopyBuffer` returned nil; on a copy error the writer is abandoned and the
  pump returns, so no FIN is ever sent for that message. (Impact is bounded —
  `cdp.ts:110-115` drops malformed JSON — but the plan asserts the invariant, so
  the code must implement it.)
- **[low / confidence: high] `ensure_forwarder`'s empty-range branch is not
  restated alongside the new `|| return 1`.** The Interfaces entry gives only
  `range="$(peer_range)" || { echo …; return 1; }`. `peer_range` still returns 0
  with EMPTY output in the legitimate unset-and-no-usable-broker case (the plan's
  own "unset with a non-IPv4 broker fallback: also exit 0"), so the existing
  `[ -z "${range}" ] && … return 0` guard at `styrman-browser:100-103` must
  survive. An implementer who replaces that guard with the new one spawns socat
  with `range=` empty inside its option list — the forwarder fails to come up,
  `ensure_forwarder` returns 1, and every agent-started (`mcp` verb) browser
  stops starting. One sentence in the Interfaces entry fixes it.
- **[low / confidence: medium] The new `forwarder_up` makes a live forwarder with
  a lost pid file unrecoverable.** Today `forwarder_up` matches ANY `:9222`
  listener (`styrman-browser:91`); finding 11 requires "pid in `socat.pid` alive
  with comm `socat` AND listening on `:9222`". In the (narrow) state where socat
  lives but `socat.pid` is gone or truncated, the launcher will try to bind a
  port it already holds, fail ten probes, and `start` exits 1 — where today it
  returns 0 and the pane works. Say whether that is accepted (it is the same
  "fail closed and tell the user" direction as the rest of finding 11) or whether
  a listening-but-unowned `:9222` should be reported with its own message; the
  plan's Failure modes only covers "a stranger on :9222 (the user's own app)".
- **[low / confidence: medium] Retry's unlisted HTTP statuses.** Finding 10
  enumerates 502 / 409 / 503 / success, and the reading expression
  `e instanceof ApiError ? e.status : 0` implies a `0` case exists — but the plan
  never says what the pane does with `0`, `404` (project record gone,
  `browser.go:156`), `401` (the re-auth reload path, `api.ts:384`) or `500`.
  Nor whether the Retry button is disabled while the POST is in flight; it is not
  today (`BrowserPane.tsx:611-619`), so a double click costs six guest execs.
  Name the default arm ("anything else: stay in `unavailable`, show the message")
  and the in-flight state.
- **[low / confidence: medium] The 1009 test needs a 16 MiB message or an
  injectable limit, and the plan picks neither.** `browserRelayReadLimit` is a
  `const` (`browser.go:259`), so "test with an upstream frame over
  `browserRelayReadLimit`" as written means pushing >16 MiB through a loopback
  relay in a unit test. Either say that is acceptable, or say the constant
  becomes a package `var` the test shrinks — that is a `browser.go` change the
  Surfaces list does not currently imply.
- **[low / confidence: medium] "verb = first non-flag" is not defined precisely
  enough to keep `--help` working.** `deploy/styrman-ctl:936` matches
  `help|-h|--help` as a VERB, which works today only because `main` takes `$1`
  unconditionally (`:923`). Under the new rule an implementer who treats any
  `-`-prefixed token as a flag breaks `styrman-ctl --help` and `-h`. Say that
  "flag" means exactly `--dry-run`, `--yes`, `--confirm` and that every other
  token — leading dashes included — is a candidate verb, as today's `*)` arm
  already does.

## (2) Readiness — questions a first-pass implementer must ask

1. **BLOCKING** — On the new `start` failure (peer SET but not an IPv4 literal,
   probe already green): is the running Chromium left alone, or torn down the way
   the only other `do_start` failure does at `styrman-browser:195`? The plan's
   Scope section forbids new destructive verbs; Phase 1 does not say this is one.
2. NON-BLOCKING — Is the destination `Writer` closed only on a successful
   `io.CopyBuffer` (so a mid-message source error never sends FIN)?
3. NON-BLOCKING — Does `ensure_forwarder` keep its `[ -z "${range}" ] → return 0`
   branch beside the new `peer_range` non-zero branch?
4. NON-BLOCKING — `forwarder_up`: live socat with a missing `socat.pid` — accepted
   as a hard `start` failure, or its own message?
5. NON-BLOCKING — Retry: what does the pane do on a status other than
   502/409/503, and is the button disabled while the POST is in flight?
6. NON-BLOCKING — `TestBrowserRelayOversizeUpstreamFrameIs1009`: a real 16 MiB
   message, or does `browserRelayReadLimit` become a test-shrinkable `var`?
7. NON-BLOCKING — `styrman-ctl`: does "first non-flag" still let `--help`/`-h`
   reach the verb `case`?

## Criteria (applied to the plan as the artifact)

1. **Dead code** — pass. `api.browserStatus`/`browserStop` have exactly one
   reference each (`api.encoding.test.ts:68,70`) and go with their rows;
   `api.browserStart` (`api.ts:561`) becomes live through Retry; `browserStart`
   (`browser.go:121`) keeps its one remaining caller `startBrowser` (`:189`)
   after finding 12 takes the relay off it. Nothing is orphaned.
2. **Flow tracing** — FAIL, once: `ensure_forwarder`'s new non-zero return →
   `do_start:179/187` → `do_stop` question (gap 1). Every other hop I followed is
   traced in the plan itself, including the three `kill_group_file` callers
   (`:110,:218,:220`), the `+set` test's single home, and the fact that
   `GET /browser` / `POST /browser/stop` never set `STYRMAN_BROWSER_PEER`
   (`browser.go:74`).
3. **Standards conformance** — pass. Stdlib only (`sync.Pool`, `io.CopyBuffer`);
   `websocket.ErrMessageTooBig` exists and is wrapped by the library
   (`read.go:529`), so `errors.Is` is the right test and today's
   `CloseStatus(err) == StatusMessageTooBig` really is dead (`browser.go:467`);
   verification commands are bare per `.agents/rules/verify-bare.md`; the new
   test goes into a new, non-colliding file per `.agents/rules/writing-files.md`;
   finding 8 invokes `.agents/rules/mechanism-reaches-every-caller.md` by name.
4. **Directory layout** — pass. Every new file lands beside what it tests;
   `internal/httpapi/browser_launcher_test.go` reaching
   `../../image/configs/styrman-browser` is the right home given
   `browserRelayPerUser` is unexported there.
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth or crypto;
   the revocation watch reuses the vetted `revocationWatch` (`bridge.go:75`)
   instead of a second ticker; no new dependency anywhere.
6. **Decoupling** — pass. `splitPortScan`/`parseDockerPorts`/`isChord`/
   `useViewLatch`/`setScreencast` are pure or ref-local and separately testable;
   the four phases touch disjoint files (launcher / server / SPA / ops) and could
   run in parallel.

## Not examined

- `deploy/styrman-ctl` outside `main` (`:922-948`), `setup_auto_update`
  (`:502-588`), `cmd_internal` (`:869-887`), `cmd_menu` (`:910-920`) and
  `cmd_help`; `internal/config/styrman_ctl_test.go` not opened this round
  (round 2 checked the existing `--dry-run` cases).
- `internal/orchestrator/ports.go` below `parseListeningPorts`'s first lines, and
  `Orchestrator.Exec`'s stdout/stderr multiplexing (whether the merged script's
  stderr can interleave around the sentinel — bounded either way, since both
  halves already redirect `2>/dev/null`).
- `web/e2e/run.mjs` outside 1230-1330; `web/e2e/run.sh` not read.
- `web/src/lib/cdp.ts` and `editorPane.ts` beyond what rounds 1–2 quoted.
- `.agents/docs/specs/styrman/{guide,terminal-protocol}.md` and the ADR-068
  amendment — Phase 4 deliverables, not written yet.
- No test tier was run: nothing is implemented. The only commands executed were
  reads and greps, plus inspection of the vendored
  `coder/websocket@v1.8.15` sources to confirm `ErrMessageTooBig` is exported and
  wrapped and that `msgWriter.Close` is what sets FIN.

design needs revision
implementation not ready
