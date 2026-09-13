---
owner: reviewer
date: 2026-09-13
---
# Review — browser-review-fixes   (2026-09-13)

Range reviewed: 61608ca..c4128f5 (`git diff 61608ca..c4128f5 -- . ':!web/dist'`, single commit c4128f5)

Blobs pinned by commit: every file below was read at `c4128f5` (`git show c4128f5:<path>`),
so nothing here depends on the working tree, which is dirty with unrelated docs edits
(`.agents/docs/specs/*`, `ROADMAP.md`, two new `BUG-042/043` records — outside this range).

## Verdict: APPROVE WITH NITS

Nothing in this range is broken in the sense of a wrong answer on the happy path, and
the three mechanisms the dispatch pointed at hardest — the streamed relay pump, the
launcher's `kill_group_file`/`forwarder_up` predicates, and the pane's
attach-chain/`screencastRef`/visible-effect interplay — each hold up under adversarial
reading: the pump's buffered channels and `defer cancel()` leave no goroutine or pooled
buffer behind on any of the five select arms, `kill_group_file`'s
"leader is ours OR leader is gone, and the group still has members" predicate is the
right one for a pid file that survives a VM restart, and every interleaving of
`attach`/`setScreencast(true|false)` I could construct is caught by the single
`screencastRef` guard because both writers set it before their first `await`. The worst
thing in the diff is one coupling regression rather than a race: merging the best-effort
`docker ps` annotation into the single port-scan exec makes an unresponsive inner dockerd
able to 500 the whole `/ports` listing, which is the exact failure BUG-039's prevention
rule was written against. The rest are low-severity edges: two documented close codes the
pane does not actually branch on, a shared ping deadline, and a `styrman-ctl` flag-loop
change that turned a diagnosed argument error into a silent `exit 1`.

`go vet ./...` clean; `go test ./internal/httpapi ./internal/orchestrator ./internal/terminal ./internal/config`
run bare, all four `ok` (exit 0, read before writing this).

## Findings

- **[medium / confidence: medium]** `internal/orchestrator/ports.go:60-71` — the
  best-effort inner-docker annotation is now fused into the exec that produces the
  primary listing, so the annotation can take the listing down with it. `|| true`
  covers a *non-zero exit* from `docker ps`, not a *hang*: with the guest's dockerd
  wedged or still coming up on its loop-mounted overlay image, `docker ps` blocks with
  no client-side timeout, the one exec runs into `Exec`'s 30 s guarantee
  (`internal/orchestrator/exec.go:153`), `ListeningPorts` returns the transport error
  (the vanished-container re-ask does not fire — the container is still the same and
  still running), and `GET /api/projects/{id}/ports` 500s. Before this change the
  socket half was a separate exec whose ports were returned whatever the docker call
  did, and only the image attribution was lost. This is BUG-039's recorded rule
  verbatim — "a listing must never 500 on a failed best-effort annotation — degrade the
  annotation, keep the rows, log why" — reintroduced on a different listing.
  Concrete fix: bound the annotation inside the script so its failure stays local, e.g.
  `timeout 3 docker ps --format '{{.Image}}|{{.Ports}}' 2>/dev/null || true`
  (coreutils `timeout` is in the image). The sentinel split already degrades correctly
  to "all scan, nothing attributed" when the docker half produces nothing, so the only
  missing piece is making the docker half unable to consume the exec's whole budget.

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:27-31` vs `:365-392`
  — the file header documents `4003 = access revoked` and `1009 = a frame over the
  relay's limit` as close codes the pane understands, but `client.onClose` branches only
  on `4002` and `4004`; both of the documented codes fall through to the generic
  `disconnected` phase. `api.ts:916` already exports `CLOSE_REVOKED = 4003` and
  `TerminalPane.tsx:26` already has a dedicated `"revoked"` state, so the pane is the
  only surface that does not distinguish it. Concrete effect: an owner removed from the
  allowlist mid-session sees "Browser disconnected. access revoked" with a Reconnect
  button whose every press is guaranteed to fail (the server refuses the handshake), and
  which burns the one automatic handshake retry each time. Fix: import `CLOSE_REVOKED`
  and give 4003 its own phase with no Reconnect, mirroring `TerminalPane`; either give
  1009 its own message or drop it from the header comment so the documented contract and
  the code agree.

- **[low / confidence: high]** `deploy/styrman-ctl:940` — `--confirm) CONFIRM_GIVEN="${2:-}"; shift 2 ;;`
  makes a missing value *silent* where it used to be diagnosed. With the trailing
  `--confirm` (a typo in `sudo styrman-ctl uninstall --confirm`), `${2:-}` no longer
  trips `set -u`, and `shift 2` with one remaining positional returns 1, which `set -e`
  turns into a bare `exit 1` with no output at all — verified:
  `bash -c 'set -euo pipefail; …; f uninstall --confirm'` → `rc=1`, no message. The old
  `CONFIRM_GIVEN="$2"` at least printed `$2: unbound variable`. For an uninstall front
  door, "exited 1, said nothing" is the wrong failure. Fix: check the arity explicitly —
  `--confirm) [ $# -ge 2 ] || die "--confirm needs a value"; CONFIRM_GIVEN="$2"; shift 2 ;;`.

- **[low / confidence: medium]** `internal/httpapi/browser.go:496-509` — the heartbeat
  goroutine derives ONE `browserRelayPing/2` (15 s) deadline and spends it across two
  sequential pings: `pane.Ping(pctx)` then, on the same `pctx`, `upstream.Ping(pctx)`.
  A pane that is slow but alive (the generous timeout is there precisely because a pong
  queues behind a busy screencast) can consume most of the 15 s, leaving the upstream
  ping a fraction of it; the upstream then fails a deadline it never had, and the relay
  is torn down with `StatusGoingAway "no heartbeat"` against a perfectly healthy guest.
  Fix: a fresh `context.WithTimeout` per ping (two lines), or ping both concurrently and
  wait for both.

- **[low / confidence: medium]** `deploy/styrman-ctl:542` — the cleanup trap now covers
  `INT TERM HUP` but does not exit. Bash runs the handler and then *resumes* the
  interrupted function, so a Ctrl-C during the multi-minute clone deletes `${CTL_TMPD}`
  (token file and allowed_signers included) and `setup_auto_update` carries on against
  paths that no longer exist, failing later at whichever command `set -e` catches first.
  The pre-existing trap had the same shape, so this is not a regression, but the line was
  rewritten in this diff and the cheap fix is one word:
  `trap '[ -n "${CTL_TMPD}" ] && rm -rf "${CTL_TMPD}"; exit 130' INT TERM HUP`, keeping
  the bare cleanup on `EXIT`. (The `EXIT` arm itself is safe: a failing `[ -n … ]` as the
  last command in an EXIT trap does not overwrite the script's status — verified in a
  `set -euo pipefail` shell, `rc=0`.)

- **[low / confidence: low]** `web/src/components/BrowserPane.tsx:204-216` — when
  `attach` fails *after* `Target.attachToTarget` returned (a throw from
  `Page.enable`/`applyViewport`), the catch clears `activeRef`/`setActive` but leaves
  `sessionRef.current` pointing at the half-attached session and never sends
  `Target.detachFromTarget`. The pane then shows no selected tab while the toolbar
  (`busy` is still false, phase stayed `ready`) sends `Page.navigate`/`Page.reload` at a
  session whose `Page` domain was never enabled, and the orphaned CDP session lives on in
  Chromium until the next successful attach detaches it. Fix: in the catch, null
  `sessionRef.current` and fire a best-effort `Target.detachFromTarget` for the session
  it just created.

## Criteria

1. **Dead code — pass.** `api.browserStatus`/`api.browserStop` and their two rows in
   `api.encoding.test.ts` were deleted together with their last callers — correct, and
   the surviving server routes (`getBrowser`, `stopBrowser` → `browserStatusOf`,
   `browserStop`) are still a registered REST surface exercised by the integration tier,
   so they are API, not orphans. No commented-out blocks, no unreachable branches;
   `revocationWatch` was exported rather than duplicated. `internal/httpapi/browser.go`'s
   old `closeReason(err.Error())` call site went away cleanly with the new one-exec path.
2. **Flow tracing — pass, with the one coupling flagged above.** Every hop checked:
   `terminal.revocationWatch`→`RevocationWatch` renamed at both call sites and in both
   doc comments; `s.stillAuthorized`/`s.browserReauthEvery` assigned in `New` on the one
   path that reaches `browserSocket` (nil in `AuthMode=none`, which is the intended
   "never re-check"); `relayWebSockets`' new two-parameter signature has exactly two
   callers (production + `browser_test.go`); `visible` threaded from the route into
   `BrowserPane` and nowhere else needed; `isChord` used by both `keyParams` and the
   pane's own Ctrl+L/Ctrl+R shortcuts so AltGr cannot be swallowed by one and inserted
   by the other. Resource lifetimes are clean on every relay arm: `paneErr`/`upErr`/
   `pingErr`/`revoked` are all buffered 1, `defer cancel()` unblocks both readers, and
   the pooled buffer is returned by a `defer` in the goroutine that took it. Abandoning
   the message writer mid-copy (as the comment says it does) leaves `writeMu` held, but
   `coder/websocket`'s `mu.lock` also selects on `c.closed` and both conns are closed
   immediately after the select, so it unwinds — no leak. The library check behind the
   1009 fix is right: `writeFrame` only flushes on `fin`, and an over-limit read really
   does surface as a wrapped `ErrMessageTooBig` rather than a close status.
3. **Standards conformance — pass.** Go: errors wrapped or deliberately mapped, no
   discarded `err`, `log/slog` structured logging, `go vet` clean. TypeScript: no
   floating promises (every `.catch` present or `void`-ed with a stated reason), caught
   errors narrowed rather than cast. Shell: `set -euo pipefail` on both scripts;
   `.agents/rules/verify-bare.md` respected — the integration guard now captures `ss`
   into a variable and greps afterwards instead of guarding the pipeline's status, which
   is the same rule applied to a precondition check. The launcher lacks
   `shopt -s inherit_errexit` (BUG-010's rule), but that rule is scoped to root scripts
   and `styrman-browser` runs as the project user inside its own VM — not raised as a
   finding.
4. **Directory layout — pass.** `browser_launcher_test.go` beside `browser.go`,
   `ports_browser_test.go` beside `ports.go`, `browserInput.test.ts` beside
   `browserInput.ts`; `useViewLatch` lives in the one route that uses it; the launcher
   stays in `image/configs/`. The Go test reaching `../../image/configs/styrman-browser`
   to pin `PORT`/`CHROME_PORT`/`max-children` against the server constants is the right
   shape for a convention that spans a language boundary.
5. **Simplicity (DIY-except-security) — pass.** No new dependencies. Nothing
   hand-rolled in the security exemption: auth is still the ingress header plus the
   userstore, and the relay's revocation check reuses the terminal bridge's watcher
   rather than inventing a second one. The two simplifications in the diff (one exec on
   the relay path, one exec per port poll) each delete code; the second is the finding
   above, and its fix is one `timeout 3`, not a revert. The streamed pump is genuinely
   longer than `Read`/`Write`, but it buys a bounded 32 KiB working set per direction
   instead of a 16 MiB materialisation per screencast frame — complexity that buys
   something observable.
6. **Decoupling — pass.** `RevocationWatch` is exported at the right grain (a function
   over a `func() bool`, no `Bridge` state leaking into `httpapi`), and `splitPortScan`/
   `parseDockerPorts` are pure functions split out of `ListeningPorts`, which is what
   let the sentinel rule be unit-tested without Docker. `useViewLatch` collapses three
   copy-pasted latches into one seam, and the "a fourth pane view should find ONE
   pattern here" comment it replaces is now true. `browserPeerFn`/`browserUpstreamFn`
   remain the test seams; no cross-package reach-ins.

## Mutation sanity of the new tests

Spot-checked the three assertions most likely to pass vacuously (BUG-036's rule):
`TestBrowserRelayOversizeUpstreamFrameIs1009` fails against the old
`websocket.CloseStatus(err) == StatusMessageTooBig` comparison (that was the point of
the fix); `TestLauncherPeerRange`'s "server peer empty" case distinguishes exit 1 from
exit 0, so the fail-closed branch cannot be deleted silently; `TestSplitPortScan`
asserts both halves *and* the sentinel-absent fallback, so removing the sentinel
handling breaks it. `TestBrowserRelayClosesOnRevocation` depends on `checks.Add(1) < 3`,
i.e. it needs the watcher to actually poll — deleting the `go RevocationWatch(...)` line
makes it hang to the 10 s context and fail.

## Not examined

- `web/dist/**` — excluded from the diff by the dispatch; the committed SPA bundle was
  not checked for freshness against the `web/src` changes in this range.
- The vendored Caddy ingress module (`deploy/caddy/ingress`) — untouched by this range
  and its separate `go test ./...` was not run.
- `web/` Tier 1 (`npm run check && npm run lint && npm test`) was NOT run — the
  TypeScript changes were reviewed by reading only, so type errors and lint findings in
  `BrowserPane.tsx`, `p.$project.$term.tsx`, `browserInput.ts` and `api.ts` are not
  ruled out by this review.
- `web/e2e/run.sh` / `run.mjs` were not executed; the new "switching to the terminal and
  back resumes the screencast" check was read, and its premise (`Page.startScreencast`
  always emits a first frame, so `before + 1` cannot pass without a real restart) is
  sound, but it was not observed failing with the visible-effect's `setScreencast(true)`
  removed.
- `.agents/docs/testing/styrman/integration.sh` was read for the `ss` guard only; the
  suite was not run (needs real Kata).
- The launcher was reviewed as source plus its Go-driven function tests; it was NOT
  exercised inside a real microVM, so `ensure_forwarder`'s socat rebind behaviour when
  the peer address changes under a live forwarder is reasoned about, not observed. The
  narrow window there (a replacement socat racing the old listener's teardown, which
  `kill_group_file` only confirms via `pgrep -g` before a final `SIGKILL` it does not
  wait on) did not rise to a finding because `socat` handles `SIGTERM` and the wait loop
  covers 5 s; it is the one place I would look first if "forwarder did not come up on
  :9222" ever shows up in the wild.
- The four plan/cold-read documents in the range
  (`.agents/docs/plans/2026-09-13-browser-review-fixes.md` and the three
  `coldread-*` files, ~900 lines) were not reviewed as documents; the code was reviewed
  on its own merits as the dispatch asked.
- `git diff --numstat` was not used to re-verify text-vs-binary on the changed files
  (BUG-007's rule) — all of them rendered as readable text in `git show`/`git diff`,
  which answers the same question.
