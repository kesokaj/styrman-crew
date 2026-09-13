---
owner: reviewer
date: 2026-09-13
---
# Cold-reader review, round 2 — plan 2026-09-13-browser-review-fixes (v1.29.0)

Range reviewed: the plan file
`.agents/docs/plans/2026-09-13-browser-review-fixes.md` (revised after round 1;
nothing implemented — working tree clean at 61608ca apart from the plan and the
round-1 report, both untracked), the round-1 report
`.agents/docs/reviews/2026-09-13-coldread-browser-review-fixes.md`, and the code
surfaces the plan names: `image/configs/styrman-browser`,
`internal/httpapi/{browser.go,browser_test.go,server.go}`,
`internal/terminal/bridge.go`, `internal/orchestrator/{ports.go,browserpeer.go}`,
`web/src/components/BrowserPane.tsx`, `web/src/routes/p.$project.$term.tsx`,
`web/src/lib/{browserInput.ts,cdp.ts,api.ts,api.encoding.test.ts,editorPane.ts}`,
`deploy/styrman-ctl`, `internal/config/styrman_ctl_test.go`,
`.agents/docs/testing/styrman/integration.sh`, `web/e2e/run.mjs:1231-1321`.

## Verdict: REQUEST CHANGES (design needs revision / implementation not ready)

**Round 1 is fully discharged** — I checked all twelve of its gaps against the
revised text and the code, and every one is now decided, with the decision
verifiable against the file it cites (the second `kill_group_file` caller really
is `styrman-browser:110`; the `email == ""` exemption really is `server.go:235`;
the e2e insertion point really does sit between `run.mjs:1292` and `:1309`,
inside the LAN window and before the sandbox rebuild; the `CTL_TMPD` trap shape
is safe — I ran it under `set -euo pipefail` and an empty variable exits 0). The
plan is materially better than round 1 and nothing in it is wrong on the
mechanisms I re-derived.

What is left is one defect the revision INTRODUCED and did not notice: finding
8's comm check disables the orphan cleanup that `styrman-browser:193-195` exists
to perform, on exactly the path where it matters. Four smaller decisions are
still open, all of them interface-shaped rather than design-shaped.

## (1) Gaps — what must still be decided before implementation starts

- **[high / confidence: medium-high] Finding 8's comm check defeats
  `do_start`'s failure cleanup.** The plan's rule is "before signalling,
  require `/proc/<pid>/comm` to match (`chrom*` … `socat`); otherwise just drop
  the file." `do_start`'s failure path calls `do_stop`
  (`image/configs/styrman-browser:195`) precisely because "a failed launch can
  leave orphaned renderer/zygote children behind (seen when the sandbox is
  refused): take the whole group down" (`:193-194`). In that case the GROUP
  LEADER is the process that died — `/proc/<pid>/comm` does not exist — while
  its zygote/renderer children are still in the group. Today
  `kill -TERM -- -${pid}` (`:206`) still reaches them because the group id
  outlives the leader. Under the new rule the file is dropped and NOTHING is
  signalled, so the very orphans the comment names survive, and the next
  `launch` will find a live profile it cannot use. Decide the predicate
  explicitly, e.g. "signal when `pgrep -g <pid>` reports members AND (the
  leader's comm matches OR the leader is gone); drop the file only when the
  group is empty or the leader's comm contradicts the prefix" — a recycled pid
  is still refused, which is what finding 8 is for. The same applies to the
  socat file, though the window there is narrower.
- **[medium / confidence: high] Retry cannot read the HTTP status the plan's
  contract branches on.** Finding 10's Retry is "`POST /browser/start` first
  (its 502 body is the full 2 KiB tail; 409 → phase 'stopped'; 503 → the one
  automatic retry rule)". `request` throws `ApiError` carrying `status`
  (`web/src/lib/api.ts:406-408`), but `class ApiError extends Error`
  (`api.ts:299`) is **not exported**, and no component in `web/src` reads
  `.status` off a caught API error today (grep: zero hits). So the implementer
  must either export `ApiError` — an `api.ts` surface change the plan's
  "Surfaces touched" does not list (it says only "two wrappers removed") — or
  duck-type `(e as { status?: number }).status`. Pick one and say so; the
  wrong pick leaves Retry unable to tell 409 from 502 and showing "Project is
  stopped" never or always.
- **[medium / confidence: medium] `splitPortScan`'s sentinel matching rule is
  undefined where it is guest-controlled.** The plan defines the absent-sentinel
  case (good — that was round-1 gap 6) but not the MATCH: whole line or
  substring, first occurrence or last. Both halves of the merged output are
  guest-controlled — the scan half prints a process `comm`
  (`ports.go` awk, `users:(("<comm>"`), the docker half prints an image name.
  A last-occurrence or substring rule lets a container image named
  `--styrman-docker--` move the boundary, feeding `docker ps` lines into
  `parseListeningPorts` and blanking the Ports panel. Decide: the first line
  **exactly equal** to the sentinel splits; everything before it is the scan.
- **[medium / confidence: medium] Finding 4's visible-effect and finding 7's
  serialized `attach` are two independent writers of the screencast.** Finding 7
  serializes `attach` through a promise chain in a ref; the visible effect is
  not in that chain. `attach` sets `sessionRef.current` before it awaits
  `applyViewport` (`BrowserPane.tsx:127`, then `:136`), so a `visible` flip
  during that await makes the effect see a live session and call
  `Page.startScreencast`, after which `attach` reaches its own (now true)
  visible check and starts it again. Chromium tolerates the duplicate, and the
  plan's e2e check (count + 1) passes either way, so this will not be caught.
  Say whether the visible effect enqueues onto the same chain, or whether the
  start is made idempotent by a `screencastOnRef` flag.
- **[medium / confidence: medium] Finding 11 needs the "is
  `STYRMAN_BROWSER_PEER` SET" test in a second place, and the plan does not say
  where it lives.** `peer_range()` already makes that distinction
  (`styrman-browser:76`, `${STYRMAN_BROWSER_PEER+set}`) but collapses both
  outcomes to the empty string, and `ensure_forwarder` only ever sees ""
  (`:99-103`). The new "set-but-unusable → exit 1, unset → exit 0" rule
  therefore requires either a second `+set` test inside `ensure_forwarder` or a
  second return channel out of `peer_range`. A duplicated test is the
  `.agents/rules/mechanism-reaches-every-caller.md` shape; name the one that is
  intended. (Third case, unstated but implied and correct: unset **and** the
  `STYRMAN_CRED_BROKER` fallback yielding a non-IPv4 host stays exit 0.)
- **[low / confidence: medium] Finding 13 may delete the attribute the e2e gates
  on.** `web/e2e/run.mjs:1242` gates on
  `page.locator("canvas[data-frames]").count()` being non-zero BEFORE reading the
  count, and `:1278` reads `browserFramesOf()` before any frame in that check.
  Today the attribute exists from first render (`data-frames={frames}`,
  `BrowserPane.tsx:572`). If `frames` becomes purely imperative
  (`canvas.dataset.frames`), the attribute must still be present with value `0`
  at mount, or the e2e's `before` read and its gate break. Say that the JSX keeps
  `data-frames="0"` and only the UPDATES are imperative.

## Notes that are NOT gaps (checked and cleared)

- The plan's "the vanished-container guard rests on the DOCKER exec's code and
  error (`ports.go:85-94`, BUG-023)" reads ambiguously but is correct:
  `ports.go:85-94` guards the Docker **Exec API** call, and a vanished container
  makes Docker report 128 before the merged script's always-zero status matters.
- Finding 11 does not break the browser e2e: the checks at `run.mjs:1272-1307`
  pass today, which is only possible if the forwarder comes up, which is only
  possible if `BrowserPeer`'s host-run branch (`browserpeer.go:39-48`) already
  resolves an IPv4 gateway. Fail-closed changes nothing there.
- `TestBrowserRelayCloseCodes` (`browser_test.go:271-290`) asserts
  `strings.Contains(ce.Reason, "No usable sandbox")` on a short reason — the
  plan's "unchanged" is right. `TestCloseReasonFitsAndKeepsRunes`
  (`:304-316`) does assert the head prefix and must be rewritten, as the plan
  says. `closeReason` has exactly one production caller (`browser.go:378`), so
  dropping the head loses nothing else.
- The nil-check trap in finding 2 is already solved by precedent: `bridge.go:376-378`
  builds `var check func() bool` and only assigns it when `b.stillAuthorized != nil`.
  Copying that shape avoids a nil-deref in `AuthMode=none`; the plan's "nil check
  = no watch" is sufficient guidance.
- `do_mcp`'s fd-9 leak (finding 1) is the only leak: `launch`'s setsid
  (`:165`) and `ensure_forwarder`'s (`:115`) already carry `9>&-`.
- `styrman-ctl`'s new flag rule does not disturb the existing Go cases:
  `styrman_ctl_test.go` already passes `--dry-run` after the verb and after
  internal-hook arguments (`:133,:287,:322`), and today's loop already scans
  everything post-verb, so "as today" for `__*` is accurate.

## (2) Readiness — questions a first-pass implementer must ask

1. **BLOCKING** — `kill_group_file`: when `/proc/<pid>/comm` does not exist
   (the leader died, the group still has members — `do_start`'s failure path at
   `styrman-browser:195`), signal the group anyway or drop the file? The plan
   says drop; `:193-194` says the whole point is to signal.
2. NON-BLOCKING — Is `ApiError` exported from `api.ts` for Retry's status
   branching, or is the status read by cast?
3. NON-BLOCKING — `splitPortScan`: whole-line, first-occurrence match on the
   sentinel?
4. NON-BLOCKING — Does the `visible` effect share finding 7's attach chain, or
   is `Page.startScreencast` guarded by an "already on" ref?
5. NON-BLOCKING — Where does the "`STYRMAN_BROWSER_PEER` is set" test live for
   finding 11: a second `+set` check in `ensure_forwarder`, or a second output
   from `peer_range`?
6. NON-BLOCKING — Does the canvas keep `data-frames="0"` in JSX once frame
   counting goes imperative?
7. NON-BLOCKING — Retry now costs three guest execs (REST start + its status,
   then the relay's own start). Intended, or should Retry skip the REST call
   when the last close was not 4004?

## Criteria (applied to the plan as the artifact)

1. **Dead code** — pass. `api.browserStatus`/`browserStop` are referenced only
   by `api.encoding.test.ts:68,70` and are deleted with their rows;
   `api.browserStart` (`api.ts:561`) becomes live via Retry. `browserStart`'s
   status-after-start survives for the REST route only (finding 12), so nothing
   is orphaned.
2. **Flow tracing** — FAIL, once: `kill_group_file` → `do_stop` →
   `do_start`'s failure path (`styrman-browser:195`) is the hop the revision
   does not follow. The round-1 failures are both repaired (the `:110` caller is
   named; the Stop→`onClose` path is gone with the Stop control).
3. **Standards conformance** — pass. Stdlib only (`sync.Pool`, `io.CopyBuffer`),
   no new dependency, verification commands are bare per
   `.agents/rules/verify-bare.md`, new tests go into a NEW file
   (`browser_launcher_test.go`) per `.agents/rules/writing-files.md`, and finding
   8 explicitly invokes `.agents/rules/mechanism-reaches-every-caller.md`.
4. **Directory layout** — pass. `internal/httpapi/browser_launcher_test.go`
   reaching `../../image/configs/styrman-browser` is the right home
   (`browserRelayPerUser` is unexported there, `browser.go:262`; `config` is
   imported).
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth or crypto;
   the revocation watch reuses the vetted `revocationWatch` (`bridge.go:75`)
   rather than a second copy. Striking the Stop control removed the only piece
   of net-new destructive surface.
6. **Decoupling** — pass. `splitPortScan`/`parseDockerPorts`/`isChord`/
   `useViewLatch` are pure and separately testable; the four phases (launcher /
   server / SPA / ops) touch disjoint files and could run in parallel.

## Not examined

- `deploy/styrman-ctl` outside `main` (`:923-969`), `setup_auto_update`
  (`:520-588`), `cmd_internal` (`:869-887`) and `cmd_menu`.
- `.agents/docs/testing/styrman/integration.sh` beyond the host `ss` guard
  (`:27`) and the browser block (`:118-146`).
- `web/e2e/run.mjs` outside 1231-1321; `web/e2e/run.sh` not read.
- `internal/orchestrator/ports.go` above line 55 (`portScanScript`'s awk body
  read only in part) and `parseListeningPorts` itself.
- `.agents/docs/specs/styrman/{guide,terminal-protocol}.md` and the ADR-068
  amendment — Phase 4 docs deliverables, not yet written.
- No test tier was run: nothing is implemented. The one command I did execute
  was a five-line bash reproduction of the proposed `CTL_TMPD` trap under
  `set -euo pipefail`, to confirm an empty variable does not change the exit
  status (it exits 0).

design needs revision
implementation not ready
