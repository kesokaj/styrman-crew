---
owner: reviewer
date: 2026-09-13
---
# Review — browser-review-fixes, round 2   (2026-09-13)

Range reviewed: c4128f5..211b2f8 — source paths only, as dispatched:
`git diff c4128f5..211b2f8 -- deploy/styrman-ctl image/configs/styrman-browser
internal/httpapi/browser.go internal/httpapi/browser_launcher_test.go
internal/httpapi/browser_test.go internal/orchestrator/ports.go
web/src/components/BrowserPane.tsx` (single commit 211b2f8, working tree clean at
that commit).

Context read first: `.agents/docs/reviews/2026-09-13-review-browser-review-fixes.md`
(1 medium + 5 low) and `.agents/docs/reviews/2026-09-13-security-browser-review-fixes.md`
(8 low, 3 of them named actionable).

## Verdict: APPROVE WITH NITS

Nothing in this range is broken. Each of the six reviewer findings and the three
actionable security findings is answered by the hunk that claims it, and each
answer is correct at the mechanism level, not just at the text level — I checked
the two shell mechanisms empirically rather than by reading, because both are the
kind of construct that reads right and behaves otherwise. The worst thing left is
not in the diff at all: two of the security review's lower findings (the `revoked`
select race and `forwarder_up`'s untied listener check) were neither fixed nor
rebutted in writing, and the changelog records the security round as "3 low, all
fixed", which understates what that report raised. Nothing about that is a
correctness defect in this code; it is the DoD's "fixed or rebutted in writing"
step left half-done.

Verification run for this review (bare commands, status read before writing):
`go vet ./internal/httpapi ./internal/orchestrator` exit 0;
`go test -count=1 -run 'TestLauncherKillGroupFile|TestCloseReason|TestBrowserRelay|TestSplitPortScan|TestParseDockerPorts' ./internal/httpapi ./internal/orchestrator`
exit 0 (both `ok`); `go test -count=8 -run TestLauncherKillGroupFile ./internal/httpapi`
exit 0 with no leftover `sleep` processes afterwards (`pgrep -a -f "sleep 300"`
matched only itself); `(cd web && npm run check)` exit 0 — the TypeScript check
round 1 did not run, so `CLOSE_REVOKED`'s import and the widened `Phase` union
are now type-verified, not just read.

## Findings

- **[low / confidence: high]** `image/configs/styrman-browser:258` — the new
  `[[ "${pid}" =~ ^[0-9]+$ ]] && [ "${pid}" -ge 2 ]` is fail-closed for every
  input I could construct, but a pid file containing a digit string wider than
  int64 makes `[` itself fail loudly: verified on this host,
  `[ 99999999999999999999 -ge 2 ]` prints `bash: [: 99999999999999999999:
  integer expression expected` to stderr and returns 2. The `|| return 0` catches
  the status, so nothing is signalled — the security property holds — but the
  launcher's stderr is exactly what `browserExec` captures and what
  `closeReason(tail(out, 2048))` puts in the 4004 close frame, so a guest-writable
  pid file can inject a bash diagnostic into the operator-visible "browser could
  not start" reason. Not blocking; if it is worth two characters, bound the regex
  (`^[0-9]{1,7}$`) so the arithmetic test can never be reached with an
  unrepresentable number.
- **[low / confidence: high]** carried over, unfixed and unrebutted — the
  security review's finding 4 (`internal/httpapi/browser.go:512-517`) and finding 5
  (`image/configs/styrman-browser:107-111`) are untouched by this commit and
  appear nowhere in `.agents/docs/changes/2026-09-13-browser-review-fixes.md`,
  which says "security (3 low, all fixed)". Finding 4 matters slightly MORE after
  this commit than before it: the `select` still races `revoked` against
  `paneErr`/`upErr`/`pingErr`, so a revoked owner whose pane errors in the same
  instant gets 1000/1011 and the pane's brand-new `revoked` phase — the whole point
  of the BrowserPane hunk — is skipped in favour of "Browser disconnected" plus a
  Reconnect button. Access is still cut on every branch (both conns are closed and
  the re-handshake is refused), so this is a wrong message, not a hole. Finding 5
  is unchanged in kind: `forwarder_up` proves "our socat is alive" and "something
  listens on :9222" without tying the two together. Fix: either latch
  `revoked atomic.Bool` and prefer it when choosing the close code (the shape
  `internal/terminal/bridge.go` already uses), or write the rebuttal into the
  change entry; and correct the entry's "3 low, all fixed" to name what was
  deliberately left.
- **[low / confidence: low]** `deploy/styrman-ctl:943` — the arity check fixes the
  silent `exit 1`, but only half of the security review's `--confirm` finding: the
  global pre-scan still consumes `--dry-run`/`--yes`/`--confirm` from ANY position,
  including a position a verb meant as its own value. The only verb with a
  free-form value is `cmd_internal`'s `__env-set <path> <key> <value>`, which is
  gated on `STYRMAN_CTL_TEST=1` and refuses paths outside `TMPDIR`, so there is no
  operator-reachable failure today; recorded because a future verb with a
  free-form argument inherits the trap silently.

## Each fix, judged

- **`timeout 3` on the docker half (`internal/orchestrator/ports.go:73`) — correct
  and complete.** `timeout` is coreutils, which is Essential in Debian and so
  present in the `debian:trixie` runtime base (`image/Dockerfile:15`; confirmed on
  this trixie host, `dpkg -S /usr/bin/timeout` → `coreutils`). It is invoked as a
  plain external binary, so `sh -c` being dash in the guest changes nothing. A
  wedged dockerd now costs the annotation and not the listing: `timeout` exits 124,
  `|| true` normalises it, the socket lines printed before the sentinel are
  returned, `ListeningPorts` sees code 0 and the vanished-container re-ask is
  unaffected. The degenerate case is also safe — an image without `timeout` gives
  `127`, likewise swallowed by `|| true`, so the annotation degrades rather than
  the endpoint 500ing. BUG-039's rule is satisfied.
- **`pingWithin` (`internal/httpapi/browser.go:540-544`) — correct.** Fresh
  `context.WithTimeout` per side, `defer cancel()` inside the helper so neither
  context outlives its ping, parent `ctx` still the cancel root so a relay teardown
  aborts an in-flight ping. Worst case one tick now spends 30 s across both pings
  against a 30 s ticker; `time.Ticker` drops ticks rather than queueing them, so
  the loop cannot back up and no goroutine or timer leaks.
- **`revoked` phase + header comment (`web/src/components/BrowserPane.tsx`) —
  correct and complete.** `CLOSE_REVOKED` is `4003` (`api.ts:916`) and matches the
  server's `config.CloseAccessRevoked` (`config.go:68`). `busy = phase !== "ready"`
  (`:655`), so the revoked branch renders the overlay with no Reconnect and the
  whole toolbar disabled — the same shape as `TerminalPane`'s revoked state. The
  header comment is now true of the code: 1009 arrives after `opened` is true, so
  it falls to the final `else` (disconnected + Reconnect) and not into the
  one-automatic-retry branch. The `onopen` sequence cannot overwrite the new phase
  either — its `.catch` returns early on `client.closed` (`:426`), which is set by
  the time `onClose` has chosen `revoked`. `tsc -b` exit 0.
- **Half-attached session cleanup (`BrowserPane.tsx:216-225`) — correct; no race
  with the chain's next attach.** The catch nulls `sessionRef.current` BEFORE
  firing the detach, so the next `run` in `attachChain` reads `prev === null` and
  cannot double-detach; the detach names an explicit `sessionId`, and Chromium
  mints a fresh session id per `Target.attachToTarget`, so a detach landing after
  the next attach cannot hit the new session. A detach for an id Chromium already
  dropped rejects, which `.catch(() => {})` absorbs; a client nulled by `onClose`
  in the meantime also nulls `sessionRef`, so `half` is falsy and nothing is sent.
  Nulling `screencastRef` alongside is right: the only other writer
  (`setScreencast`) keys on the session id and bails when `sessionRef` is null.
- **`styrman-ctl` `--confirm` arity + split traps — correct.** `[ $# -ge 2 ] ||
  die "--confirm needs a value"` replaces the silent `shift 2` failure with a
  diagnosed exit. The trap split was verified by execution, not by reading, in a
  `set -euo pipefail` shell: with `CTL_TMPD` set, `kill -INT` → temp dir removed,
  script status 130; with `CTL_TMPD` empty, the failing `[ -n "" ]` does NOT abort
  the handler before `exit 130` (an AND-OR list whose first member fails does not
  trip `set -e`), status still 130. `exit 130` inside the signal trap does run the
  EXIT trap, whose second `rm -rf` on an already-removed path is a no-op returning
  0 and therefore cannot overwrite the 130. Double removal is safe in both orders.
- **`[ "${pid}" -ge 2 ]` under `set -u` — correct.** `pid` is always assigned (the
  `$(cat … || true)` substitution yields "" at worst), so `-u` never fires; the
  regex short-circuits, so the arithmetic test only sees digits; and
  `A && B || return 0` returns 0 on both false branches and falls through only when
  both hold. Rejecting 1 as well as 0 costs nothing real — neither chromium nor
  socat can be pid 1 in the guest, whose pid 1 is the entrypoint. See the one
  finding above for the wide-number edge.
- **`strings.ToValidUTF8` in `closeReason` — correct and complete.** It runs
  BEFORE the 120-byte budget check, so the cut arithmetic and the rune-start scan
  operate on valid UTF-8 and the frame stays inside the 123-byte reason limit.
  `closeReason` has exactly one production call site (`browser.go:399`), and it is
  the only close reason carrying guest bytes — every other `closeWith` reason in
  `browserSocket` is a server constant, including the ones derived from
  `browserEndpoint` errors, so no second sanitisation point was missed. The new
  assertion checks both the exact string and `utf8.ValidString`.
- **Owned-`sleep` test fixture — correct, and it holds under repetition.** The
  fixture no longer writes a pid whose negation is a wildcard; the stranger is a
  `setsid`-made group leader the test owns, `t.Cleanup` takes its group down with
  `kill -9 -- -<pid>`, and the `leader.Wait()` goroutine reaps it, which is what
  makes the `/proc/<stranger>` "was it signalled" assertion meaningful rather than
  vacuous. The added `"0\n"`/`"1\n"` rows assert the NEW behaviour (silent drop, no
  "stale, not signalled" line), so deleting the `-ge 2` guard breaks them. Eight
  consecutive runs passed and left no processes behind. One residual, pre-existing
  and not raised as a finding: the leader writes its pid before `exec sleep`, so a
  sufficiently unlucky read could see comm `bash` and make the kill case report
  "stale" — the window is a single `exec` against a Go `fork`/`exec` of bash, and
  it did not reproduce in 8 runs.

## Criteria

1. **Dead code — pass.** Nothing orphaned: `pingWithin` has two callers, the
   `revoked` phase is rendered and set, `startLeader` is called twice, and the old
   inline leader block was moved rather than duplicated (`git diff` shows the
   deletion). No commented-out code, no unreachable branch introduced.
2. **Flow tracing — pass.** `pingWithin` traced from the heartbeat goroutine to
   `Conn.Ping` and back through `pingErr`; `closeReason` traced from
   `browserExec`'s output through `tail` to the single `closeWith`; the new close
   code traced server→wire→`onClose`→`Phase`→render→`busy`; `kill_group_file`'s new
   guard traced to both its call sites' predicates (`comm_is`, `pgrep -g`, `kill --
   -pid`); the `styrman-ctl` traps traced to `setup_auto_update`'s success path,
   which clears `CTL_TMPD` before removing the trap, so neither handler can
   `rm -rf ""` or a reused path. No installer/converge step is implicated by this
   range (no unit file, flag or default changed).
3. **Standards conformance — pass.** Go: no discarded errors, contexts derived and
   cancelled, `go vet` clean. TypeScript: no floating promise (the new detach is
   explicitly `.catch`-ed with a stated reason), `tsc -b` clean. Shell: both scripts
   keep `set -euo pipefail`; `.agents/rules/verify-bare.md` was honoured for this
   review's own commands (each run bare, status read before the report was written).
   `.agents/rules/writing-files.md`: `git diff --stat` for the range shows
   `browser_launcher_test.go` at +32/−22 — a move, matching the diff text — and no
   file described as "added" carries deletions; every changed path rendered as
   text, not "Binary files differ".
4. **Directory layout — pass.** No new files in this range; every hunk lands in the
   file that already owned the mechanism.
5. **Simplicity (DIY-except-security) — pass.** No new dependency; every fix is
   smaller than the alternative (one coreutils word, one four-line helper, one
   stdlib call, one `[ $# -ge 2 ]`). Nothing hand-rolled in the security exemption:
   the revocation check still delegates to `terminal.RevocationWatch` and the
   userstore.
6. **Decoupling — pass.** `pingWithin` is a pure helper over `*websocket.Conn` with
   no relay state; the sanitisation sits inside `closeReason` rather than at each
   call site; `startLeader` is local to its test. No new cross-package reach-in.

## Not examined

- `web/dist/**` — excluded by the dispatch's path list. The bundle changed in this
  commit and was NOT checked for freshness against `web/src`.
- Everything else in commit 211b2f8 outside the seven dispatched paths: the ADR-068
  amendment, `BUG-042..052` and `INDEX.md`, the four spec docs, the verifier log,
  `ROADMAP.md`/`TODO.md`, `deploy/README.md` and the changelog entry were read only
  where they bear on a finding (the changelog's "3 low, all fixed" claim).
- `web/e2e/run.sh`, `.agents/docs/testing/styrman/integration.sh` and the full
  `go test ./...` / `npm test` / `npm run lint` tiers were not run; I ran the
  targeted Go tests, the repeat loop, `go vet` on the two packages and `tsc -b`.
  The changelog's claim of a clean `go test -race ./...` and 92/2 e2e is therefore
  cited, not reproduced.
- Nothing was executed inside a real Kata guest: `timeout`'s presence is argued
  from the Debian base's Essential set and confirmed on a trixie host, not observed
  in `styrman-runtime`. A one-line check worth doing on the next live run:
  `docker exec <ctr> sh -c 'command -v timeout'`.
- The vendored Caddy ingress module — untouched by this range, its separate
  `go test ./...` not run.
- `govulncheck`/`npm audit` were not re-run; this range adds no dependency.

---

## Round 3 — commit 2a501ae   (2026-09-13)

Range reviewed: 211b2f8..2a501ae
(`git diff 211b2f8..2a501ae -- image/configs/styrman-browser internal/httpapi/browser.go
internal/httpapi/browser_launcher_test.go`; single commit, tree clean at it.)

### Verdict: APPROVE

All four fixes are correct and complete, and none introduces a new defect. The one
thing worth recording is a construct, not a bug: `port_owners | grep -qx "${pid}"`
runs under the script's `set -o pipefail`, so its status is *not* only `grep`'s —
I reproduced a `141` from an upstream SIGPIPE on a large stream, but could not
reach it at the real size in 2000 attempts. The rebuttal of my round-2 low/low on
global flag position is accepted: documented behaviour, test-gated verb, existing
cases cover it.

Verification for this section (bare, status read first): `go vet ./internal/httpapi`
exit 0; `go test -count=6 -race -run 'TestLauncher|TestBrowserRelay|TestCloseReason'
./internal/httpapi` exit 0; `bash -n image/configs/styrman-browser` exit 0;
`shellcheck image/configs/styrman-browser` exit 0.

### Findings

- **[low / confidence: high on the mechanism, low on reachability]**
  `image/configs/styrman-browser:100,117` — `forwarder_up`'s last conjunct is a
  PIPELINE evaluated under `set -o pipefail` (line 45), so a match can be reported
  as a failure: `grep -qx` exits at the first matching line, and if the upstream
  `cut`/`grep -o` still has bytes to write it dies of SIGPIPE and pipefail returns
  141. Demonstrated on this host with a long stream (`MISREPORT rc=141`); NOT
  reachable at the real size — 2000 iterations over a 17-owner list with the match
  on the FIRST line gave 0 misreports, because the whole listing fits in one pipe
  write that completes before `grep` decides. Recording it because the consequence
  is not a retry: a false `forwarder_up` falls into `ensure_forwarder`'s
  `elif port_listening` arm, which `return 1`s, `do_start` returns that status
  (`:229 return $?`), and the pane gets 4004 "something else listens on :9222 (not
  the forwarder this launcher started)" — a hard, misleading failure for a healthy
  forwarder. This is the `.agents/rules/verify-bare.md` family (a decision taken on
  a pipeline's status rather than the command's). One-line hardening if wanted:
  `grep -qx "${pid}" <<<"$(port_owners)"`, or `case "$(port_owners)" in` — the
  status then belongs to the match, not to whoever was still writing.
- **[low / confidence: medium]** `image/configs/styrman-browser:100-117` — the new
  `port_owners` predicate has no unit coverage: `TestLauncherKillGroupFile` and
  `TestLauncherPeerRange` exercise the other two launcher functions through
  `launcherFunctions`, and nothing calls `forwarder_up`. Its only coverage is the
  integration tier's "browser status/start/stop routes answer for the owner (9222
  up, hidden)" (`integration.sh:148`), which is exactly why that re-run on the
  rebuilt image is load-bearing rather than a formality — `do_start` propagates
  `ensure_forwarder`'s status, so a guest where `ss -p` cannot name the owner turns
  every `start` into a 4004. Record the re-run's result in the change entry; a
  `launcherFunctions` case that binds a port, writes the pid and asserts
  `forwarder_up` true/false would make the predicate testable without Kata.

### The two questions asked

- **Can `port_owners | grep -qx "${pid}"` misreport under `pipefail` and the `&&`
  chain?** Parsing first: `[[ … ]] && [ -d … ] && comm_is … && port_owners | grep -qx …`
  binds as `… && (port_owners | grep -qx …)` — a pipeline is a higher-precedence
  construct than `&&`, so the pipeline is the final conjunct and its status is
  `forwarder_up`'s. Verified by execution, not by reading: with a real `ss` listener
  and the matching pid → `MATCH`, function status 0; with a wrong pid → no match,
  status 1; `port_owners` itself exits 0 when it prints owners. It cannot report a
  NON-owner as a match. The only misreport direction is the false negative above
  (SIGPIPE + pipefail), verified reachable in principle and not in practice. Note
  too that when nothing listens, `grep` inside `port_owners` exits 1 and pipefail
  makes the function non-zero — which is the wanted answer anyway, and it is
  consumed in a condition, so `set -e` never fires.
- **Can `ss -p` name the owner for the launcher's own uid inside the guest?** Yes.
  `ss -p` resolves owners by walking `/proc/*/fd`, which needs no capability for
  processes of the same uid; socat is started by the launcher, so it is always the
  same uid. Confirmed on this host as uid 1000, unprivileged:
  `ss -ltnpH` → `users:(("python3",pid=1585243,fd=3))`. The guest adds nothing that
  breaks it (no `hidepid`, full iproute2 in `image/Dockerfile:121`), and socat's
  `fork` children inherit the listening socket, so `ss` lists parent and children on
  the one line and `grep -o 'pid=[0-9]*'` yields each — "any of the owners" is the
  right test. The residual is the failure MODE, not the mechanism: if `ss -p` ever
  came back nameless, `forwarder_up` would be permanently false and `start` would
  fail closed with a misleading message, which is what makes the integration re-run
  the evidence that matters.

### Each fix, judged

- **`^[0-9]{1,7}$` in both `kill_group_file:264` and `forwarder_up:117` — correct
  and complete.** Seven digits covers Linux's `PID_MAX_LIMIT` (4194304), so no legal
  pid is rejected, and the wide-number `[: integer expression expected` I raised in
  round 2 can no longer be reached — the regex now fails before `[ "${pid}" -ge 2 ]`
  is evaluated. Applying it to `forwarder_up` as well is more than my finding asked
  and is right: that function feeds `/proc/${pid}` and `comm_is` with the same
  guest-writable bytes. Fail-closed behaviour is unchanged in both (`|| return 0`
  and the `&&` chain).
- **Ownership-tied `forwarder_up` — correct.** It now proves the recorded pid IS
  one of the processes holding the :9222 listener, closing the security review's
  "tied only by coincidence". `grep ":${PORT} "` cannot match a peer column on a
  LISTEN row (peer is `0.0.0.0:*`), and a stranger's listener yields either no
  `pid=` the file names or a non-matching one, so the `elif port_listening` "fail
  closed with the reason" path still catches the user's own app on the port. See
  the two findings above for the pipeline status and the missing unit test.
- **`revokedFlag atomic.Bool` + `closePane` — correct and complete.** The flag is
  stored BEFORE the send on the buffered `revoked` channel, so any select arm that
  wins after the watcher fired observes it and substitutes 4003; every arm now goes
  through `closePane` (including `ctx.Done()` and both `upErr` branches), so there
  is no path left that can deliver a reconnectable code to a revoked owner. The
  `<-revoked` arm passing 4003 into a function that would substitute it anyway is
  redundant, not wrong. No new goroutine, no new lock, nothing captured by value
  that should be by reference; `-race` clean over six runs of the relay tests. The
  only residual is inherent: a revocation that lands AFTER the close was already
  sent still shows the other code, which no latch can fix.
- **Test leader via `SysProcAttr{Setsid: true}` — correct, and strictly better.**
  The pid now comes from the process handle rather than from the pid file, which is
  the input under test, so the fixture can no longer be fooled by its own fixture;
  the wait loop asserts the file CONVERGES to that pid instead of trusting whatever
  it finds. Dropping the `exec.LookPath("setsid")` skip is right — there is no
  `setsid` binary in the path any more — and the group cleanup still works because
  `Setsid: true` makes the child a session and group leader, so `kill -9 -- -<pid>`
  names its group. Six race-enabled runs passed and left no processes behind
  (`pgrep -a -f "sleep 300"` matched only itself). One pre-existing residual,
  unchanged and not reproduced in 14 runs: the leader writes the pid file before
  `exec sleep`, so an unlucky read could see comm `bash`. The file carries no build
  tag, so `syscall.SysProcAttr{Setsid:…}` ties the package's tests to unix
  (linux/darwin build fine, windows would not) — the project targets both of those
  only, so this is a note, not a finding.
- **Rebuttal accepted.** My round-2 low/low on global flags being consumed from any
  position: documented behaviour, the only free-form verb is `STYRMAN_CTL_TEST=1`
  gated and `TMPDIR`-confined, and `internal/config/styrman_ctl_test.go` covers the
  position-independence. Closed, no further action.

### Criteria (round 3 delta)

1. **Dead code — pass.** `port_listening` still has a caller (`ensure_forwarder`'s
   `elif`), so the new `port_owners` did not orphan it; nothing else left behind.
2. **Flow tracing — pass.** `forwarder_up` traced to both its call sites and through
   `ensure_forwarder`'s `return 1` into `do_start:229 return $?` and on to the
   relay's 4004 — that propagation is what turns the missing unit test into a real
   (if low) risk. `closePane` traced through all five select arms.
3. **Standards conformance — pass.** `bash -n` and `shellcheck` exit 0, `go vet`
   exit 0, `set -euo pipefail` retained; the one place the project's verify-bare
   rule is in tension is the finding above.
4. **Directory layout — pass.** No new files.
5. **Simplicity — pass.** No new dependency; the latch is nine lines of stdlib
   `sync/atomic` and the ownership check is one `ss` invocation the script already
   depended on.
6. **Decoupling — pass.** `port_owners` is a separate one-line function rather than
   more predicate inside `forwarder_up`; `closePane` is a local closure over one
   flag, no state added to the relay's signature.

### Not examined (round 3)

- The integration tier ("9222 up, hidden") and the e2e tier were NOT run here; the
  `ss -p` answer is verified on this trixie HOST as uid 1000, not inside a Kata
  guest. The coordinator's re-run on the rebuilt image is the evidence for the
  guest, and its result belongs in the change entry.
- No test exercises `forwarder_up`, `port_owners` or the `revokedFlag` race
  directly; I read them and reasoned, and ran the launcher's existing function
  tests and the relay tests under `-race`.
- Everything in 2a501ae outside the three dispatched paths (docs, bug records,
  `web/dist`) was not reviewed.

---

## Round 4 — commit bac799f   (2026-09-13)

Range reviewed: 2a501ae..bac799f (HEAD)
(`git diff 2a501ae..HEAD -- image/configs/styrman-browser
internal/httpapi/browser_launcher_test.go`; single commit, tree otherwise clean.)

### Verdict: APPROVE WITH NITS

The inode tie is the right shape and it closes both of my round-3 findings — the
SIGPIPE-under-`pipefail` construct is gone (inodes are captured into a `for` word
list, no pipe into a consumer that quits), and `forwarder_up` finally has unit
coverage. Nothing that existed is broken. One thing IS new, and it comes from the
switch to `-e`: `grep ":${PORT} "` is still matched against the WHOLE line, and
`ss -e` adds `ino:<n> sk:<n>` tokens that have the same `:<digits><space>` shape
the port column has, so a foreign socket whose inode (or `sk`) is literally the
port number now matches a line that has nothing to do with :9222. With `-p` in
round 3 the extra tokens were `pid=`/`fd=` and could not collide; with `-e` they
can. It is a one-token fix and, per the project's "second round on the same
mechanism" rule, I am flagging it as a thing to record-or-fix once, not as an
invitation to a round 5 — the mechanism's purpose (idempotent `start`, so a relay
connect does not respawn socat and drop the other panes) is now clearly stated and
the design is right.

Verification (bare, status read first): `go test -count=4 -run TestLauncher
./internal/httpapi` exit 0; `go vet ./internal/httpapi` exit 0;
`bash -n image/configs/styrman-browser` exit 0; `shellcheck image/configs/styrman-browser`
exit 0. `grep -rn port_owners` over the tree: no stale reference to the removed
function.

### Findings

- **[low / confidence: high on the mechanism, low on reachability]**
  `image/configs/styrman-browser:101` — `port_inodes` filters with
  `grep ":${PORT} "` over the whole `ss -ltneH` line, and `-e`'s output is
  `… 0.0.0.0:22 0.0.0.0:* ino:6756 sk:2 cgroup:/… <->` (verified on this host).
  A listening socket whose inode is exactly 9222 therefore produces
  `ino:9222 `, which matches, and `grep -o 'ino:[0-9]*' | cut -d: -f2` then reports
  THAT socket's inode as a ":9222 listener" — demonstrated:
  `echo 'LISTEN 0 128 0.0.0.0:22 0.0.0.0:* ino:9222 sk:2 cgroup:/…' | grep ":9222 "
  | grep -o 'ino:[0-9]*' | cut -d: -f2` → `9222`. `sk:9222 ` collides the same way.
  The direction matters: this is a false POSITIVE, unlike round 3's false negative.
  If the recorded (live, comm-`socat`) pid happens to hold that socket — the
  realistic instance is the launcher's own socat listening somewhere else with an
  inode equal to 9222 — `forwarder_up` returns true while nothing listens on :9222,
  `ensure_forwarder` returns 0 without starting a forwarder, and the server's
  upstream dial fails: the pane gets 4004 "browser endpoint unreachable" and stays
  that way, because every later `start` takes the same early return. Socket inodes
  in a freshly booted microVM are small enough for 9222 to be an ordinary value,
  which is why I am not calling the reachability negligible outright. Fix, one
  token: match the address COLUMN instead of the line —
  `ss -ltneH 2>/dev/null | awk -v p=":${PORT}" '$4 ~ p"$" {…}'`, or keep grep and
  anchor the field with `grep -E "[0-9]:${PORT}[[:space:]]"`. If you would rather
  record it than reopen the file, the residual is "a 1-in-inode-space chance of a
  visible, non-security 4004", and that is a defensible place to stop.
- **[low / confidence: medium]** `internal/httpapi/browser_launcher_test.go:111-167`
  — `TestLauncherPortOwnership` pins both halves of the predicate and both refusals,
  but there is still no POSITIVE `forwarder_up` case: nothing asserts it returns
  TRUE for a real forwarder, because that needs a process whose comm starts with
  `socat`. A regression that made `forwarder_up` always false would pass every unit
  test in this package and surface only as the failure the mechanism exists to
  prevent — each relay connect respawning socat and dropping the other panes, or a
  hard 4004 through `ensure_forwarder`'s `elif port_listening`. The integration tier
  is the only place that can see it; the cheapest pin there is to call
  `styrman-browser start` twice and assert the socat pid file is UNCHANGED, which
  tests idempotence directly rather than inferring it from "9222 is up".

### The round-4 changes, judged

- **`port_inodes` + `owns_socket` — correct in structure, with the grep caveat
  above.** Dropping `-p` removes guest-chosen process NAMES from the parsed line
  entirely, which is the right answer to the security review: the only guest input
  left on that line is numeric. `owns_socket` compares
  `readlink /proc/<pid>/fd/*` against the exact string `socket:[<ino>]`, which
  cannot be spoofed inside /proc, and degrades correctly when the fd directory does
  not exist (the unmatched glob readlinks to nothing, the loop ends, `return 1`).
  Tying the pid to the listening socket's inode is the definitive form of "is this
  MY forwarder" — strictly stronger than the name-based check it replaces, and the
  comment correctly notes socat's `fork` parent keeps the listening socket, which is
  the pid the file records.
- **SIGPIPE removed — correct and complete.** `for ino in $(port_inodes)` captures
  the producer's full output before any matching, so no consumer can quit early on
  it and no `pipefail` status can be inherited from a signalled writer. My round-3
  finding is closed. The discarded status of the substitution is the right call and
  safe: verified that `for i in $(<failing command>)` does not trip `errexit`, and
  both call sites invoke `forwarder_up` in an `if` condition anyway. Word splitting
  and globbing on the unquoted substitution are safe because `grep -o 'ino:[0-9]*'`
  guarantees digits only.
- **`^[1-9][0-9]{0,6}$` in both functions — correct and complete.** It keeps the
  round-3 upper bound (7 digits ≥ `PID_MAX_LIMIT` 4194304) and adds the leading-zero
  refusal the security review asked for: `/proc/0123` does not exist, so the comm
  check would have been skipped, while `pgrep -g 0123` / `kill -- -0123` resolve the
  number — the same asymmetry that made `0` dangerous. Applied identically in
  `kill_group_file` and `forwarder_up`, and `kill_group_file`'s table now has a
  `"0"+stranger` row, so the guard cannot be deleted silently.
- **`TestLauncherPortOwnership` — sound, not vacuous.** The inode is taken from the
  test's own `SyscallConn`/`/proc/self/fd` rather than from the thing under test, so
  `port_inodes` is checked against an independently obtained truth; `owns_socket`
  is asserted in BOTH directions (own listener accepted, inode 1 refused), so
  neither branch can be removed without a failure; the two `forwarder_up` cases
  refuse a non-socat owner and a zero-padded pid. The `ss` LookPath skip is honest.
  Four consecutive runs passed. See the second finding for what it still cannot
  cover.
- **The "what is it for" answer is the right one.** `forwarder_up` exists to make
  `start` idempotent for every relay connect; that is a real requirement (a respawn
  drops the other panes' relays), so the mechanism stays and the inode tie is its
  definitive form. Rounds 2→4 were three passes over the same predicate, each
  strictly smaller in trust surface (any listener → live socat + any listener →
  ss-named owner → the socket's own inode), and the code got SHORTER in the last
  step. That is the pattern the rule wants, not the one it warns about.

### Criteria (round 4 delta)

1. **Dead code — pass.** `port_owners` was deleted with its last caller; no
   reference survives anywhere in the tree. `port_listening` still serves
   `ensure_forwarder`'s `elif`.
2. **Flow tracing — pass.** `forwarder_up` → `ensure_forwarder` (both the early
   `return 0` and the post-spawn wait loop) → `do_start:229 return $?` → the relay's
   exit-code branch → 4004; the false-positive finding is stated in those terms.
   Both call sites are condition contexts, so the function's `return 1` cannot
   `errexit` the script.
3. **Standards conformance — pass.** `shellcheck` and `bash -n` clean, `go vet`
   clean, `set -euo pipefail` retained; no pipeline decides a branch any more, which
   is `.agents/rules/verify-bare.md`'s rule applied to the launcher.
4. **Directory layout — pass.** No new files; the new test sits with the other
   launcher tests.
5. **Simplicity — pass.** No dependency added; the replacement drops a three-stage
   pipe for a `for` loop over `readlink`, and reads less of `ss`'s output format
   than before, not more.
6. **Decoupling — pass.** `port_inodes` and `owns_socket` are two small functions
   with one job each, separately testable — which is exactly what made the new unit
   test possible.

### Not examined (round 4)

- Nothing was run inside a Kata guest: `ss -ltneH`'s field layout and the
  `/proc/<pid>/fd` behaviour are verified on this trixie host, not in
  `styrman-runtime`. The integration tier's browser check on the rebuilt image
  remains the guest-side evidence, and it is still the only positive coverage of
  `forwarder_up`.
- No real `socat` was involved anywhere in my checks (it is not installed on this
  host), so the "socat `fork` parent keeps the listening socket" premise is read
  from socat's semantics, not observed.
- Everything in bac799f outside the two dispatched paths.
