---
owner: security
date: 2026-09-13
---
# Security review — browser review fixes, round 2   (2026-09-13)

Range reviewed: c4128f5..211b2f8

Paths in scope (dispatch): `deploy/styrman-ctl`, `image/configs/styrman-browser`,
`internal/httpapi/browser.go`, `internal/httpapi/browser_launcher_test.go`,
`internal/httpapi/browser_test.go`, `internal/orchestrator/ports.go`,
`web/src/components/BrowserPane.tsx`. Blast radius followed outward into
`internal/orchestrator/exec.go` (the launcher's exec identity),
`internal/httpapi/browser.go`'s other `tail()` sinks, `web/src/lib/api.ts`
(`CLOSE_REVOKED`), `internal/terminal/bridge.go` (the latch this diff's UI
branch depends on) and `image/Dockerfile` (whether `timeout` exists).

## Verdict: FINDINGS

All three of the previous round's security findings are closed, and each was
re-verified by running the thing rather than reading it: the pid guard rejects
`0`/`1`/non-numeric with the precedence the fix assumes, the close reason is
scrubbed before the byte cut, and the test fixture no longer puts pid 1 one
statement away from `kill -TERM -- -1`. Nothing in the new code opens a
privilege boundary. What is left is two soft spots the fixes introduce or leave
standing, both low. The sharper one is that the pid the launcher test's
`t.Cleanup` `kill -9 -- -<pgid>` targets is read back out of the pid FILE, not
taken from the child the test started — so the fixture's notion of "the group I
created" is only as good as `kill_group_file`'s own `rm -f`, i.e. the fixture
trusts the code under test to keep the kill aimed at the right group. The other
is that this diff adds a client-side `revoked` phase whose correctness depends on
close code 4003 arriving deterministically, while the server `select` that emits
it still races `revoked` against three error channels — the pane can show
"disconnected" for a revoked user. Neither is an access crossing.

## The three findings, confirmed closed

1. **`kill_group_file` accepts pid `0` (round 1, finding 1) — CLOSED.**
   `image/configs/styrman-browser:257-260`:

   ```bash
   [[ "${pid}" =~ ^[0-9]+$ ]] && [ "${pid}" -ge 2 ] || return 0
   ```

   The `&&`/`||` precedence question resolves correctly. Shell `&&` and `||`
   are left-associative with equal precedence, so this parses as
   `(regex && -ge) || return 0`. A non-numeric value fails the regex, the `&&`
   short-circuits, `[ "${pid}" -ge 2 ]` is never evaluated, and the AND-list's
   non-zero status is consumed by `|| return 0`. `set -euo pipefail` (the
   launcher sets it) does not abort: the failing command is not the command
   following the final `&&`, which is exactly the errexit exception, and the
   whole list is guarded by `||` in any case. Reproduced in a scratch harness
   with the identical construction under `set -euo pipefail` — `not-a-pid`,
   `0`, `1`, `` (empty), `-1`, `1 2` all rejected; `2` and `4000000000`
   accepted; the script ran to completion with status 0 in every case.
   `pgrep -g 0` and `kill -- -0` (the caller's own group) are now unreachable.
   Covered by `browser_launcher_test.go:158` which now feeds `"0\n"` and
   `"1\n"` through the quiet-drop loop; ran bare, PASS.

2. **Guest bytes verbatim in the close frame (round 1, finding 3) — CLOSED.**
   `internal/httpapi/browser.go:590-594`: `msg = strings.ToValidUTF8(msg, "")`
   is the FIRST statement of `closeReason`, before the `len(msg) <= max` test
   and before the rune-boundary cut. The ordering is the right one: the call
   site is `closeReason(tail(out, 2048))` (`browser.go:399`) and `tail` itself
   can leave an invalid sequence — it only advances `cut` to the first rune
   start and prepends `…`, so a bad sequence deeper in the kept tail survives
   `tail` — but `ToValidUTF8` runs after `tail` has already done its damage, so
   the scrub covers it. The replacement is the empty string, so the byte count
   only shrinks and the 120-byte cap cannot be pushed past. Asserted by
   `browser_test.go:341-345` (`closeReason("bad \xff byte") == "bad  byte"` and
   `utf8.ValidString`); ran bare, PASS.
   The other `tail(out, 2048)` sinks (`browser.go:110`, `:114`, `:128`) reach
   the client as `fmt.Errorf` text inside a JSON body, and `encoding/json`
   coerces invalid UTF-8 to U+FFFD when marshalling a string, so that path was
   never exposed to the same defect and needs no change.

3. **Test fixture wrote pid 1 (round 1, finding 2) — CLOSED.**
   `browser_launcher_test.go:120-145`. The "recycled pid" case now uses
   `startLeader()` — `setsid bash -c 'echo $$ > "$1"; exec sleep 300'`, a
   session leader the test owns whose comm is `sleep`, not `chrom` — and the
   test additionally asserts at `:152-155` that `/proc/<stranger>` still exists
   after the call, i.e. that the stranger was not signalled. Pid `1` was
   demoted into the quiet-drop loop at `:158`, where the new `-ge 2` guard
   returns before any `pgrep -g` or `kill`, so `kill -TERM -- -1` (POSIX:
   every process the caller may signal) is no longer reachable from this test
   at all. Ran bare, PASS.

## Findings

- **[low / confidence: medium]** `internal/httpapi/browser_launcher_test.go:120-145`
  — `startLeader` returns a pid it polls out of the **pid file**, and
  `t.Cleanup(func() { exec.Command("kill", "-9", "--", "-"+stranger) })` at
  `:145` kills that process GROUP. The value is therefore whatever the file
  held, not the child `exec.Command` actually started. In the current flow it
  is always the test's own leader, because the file lives in a private
  `t.TempDir()` and `kill_group_file` removes it (`rm -f "${f}"`) before each
  `startLeader` call — but that is the function under test guaranteeing the
  fixture's identity. A regression that stops removing the pid file, or a
  leader that dies before writing, leaves `startLeader` reading a stale value
  and the cleanup sending `kill -9` to a group nobody named; in the second
  case the same stale value would make the final "the group went down" loop
  stat a nonexistent `/proc/<stale>` and report PASS, so the test could also
  go green while testing nothing. Test-only, developer machine, so low.
  Remediation: set `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}` and
  take `cmd.Process.Pid` as the authoritative pgid (with `Setsid` the child
  IS the session leader, so pid == pgid without depending on whether
  util-linux `setsid` forked), then assert the polled file content equals it
  before proceeding. Related hygiene: the second `startLeader()` at `:167`
  registers no `t.Cleanup`, so a regression in the kill path fails the test
  and leaks a `sleep 300` plus its group for five minutes.

- **[low / confidence: medium]** `internal/httpapi/browser.go:514-520` and
  `web/src/components/BrowserPane.tsx:382` — the new `revoked` phase is a
  client-side branch on close code 4003, but the server `select` that produces
  4003 is unchanged: `case <-revoked` still races `paneErr`, `upErr`, `pingErr`
  and `ctx.Done()`, and Go chooses among ready cases pseudo-randomly. A
  revoked user whose pane or upstream errors in the same instant gets 1000/1011
  instead, so the pane falls into `disconnected` and offers Reconnect. Access is
  still cut on every branch (both conns are closed) and a Reconnect would be
  refused at the handshake, so this is a wrong message rather than a wrong
  outcome — but the UI change this diff makes is only as reliable as the code
  it keys on. Round 1 recorded the race; this diff builds on it without closing
  it. Remediation is already in the tree: `internal/terminal/bridge.go:373-463`
  latches `var revoked atomic.Bool` in the watch callback and prefers it when
  choosing the close code. Doing the same in `relayWebSockets` makes 4003
  deterministic.

## Answers to the specific questions asked

- **The `exit 130` path and the EXIT trap that runs after it
  (`deploy/styrman-ctl:542-545`).** Safe, verified by execution, not by reading.
  A scratch script with the identical two traps under `set -euo pipefail` and
  `shopt -s inherit_errexit`, interrupted mid-`sleep`, exited **130**. Three
  things had to hold and all three do: (a) the signal trap's `rm -rf` leaves
  `CTL_TMPD` set, so the EXIT trap that bash runs on the way out re-removes an
  already-removed path — `rm -rf` on a missing path is a no-op returning 0, and
  the `-f` means no diagnostic; (b) the EXIT trap's final status does NOT
  override the `exit 130` argument (measured), so the status the caller sees is
  the one the signal trap chose; (c) `CTL_TMPD` is non-empty at that point, so
  the `[ -n … ]` guard passes and the AND-list returns 0 anyway. The
  `rm -rf ""` case the round-1 review cleared is still unreachable. The
  success path at `:597` clears `CTL_TMPD` and then `trap - EXIT INT TERM HUP`
  — all four, so no trap survives the function to fire over a stale value.
  One cosmetic residual, not filed as a finding: all three signals report 130
  (128+SIGINT) rather than 143/129 for TERM/HUP, so a supervisor that reads the
  exit status of an interrupted `styrman-ctl` would be told "Ctrl-C". Nothing
  in `deploy/` invokes `styrman-ctl` from a unit or a script (grepped: no
  hits), so nothing currently reads that status.

- **`[ "${pid}" -ge 2 ]` on a non-numeric value under `set -euo pipefail`.**
  Never reached — see finding 1 above; the regex short-circuits. The one input
  class that DOES reach `[ -ge ]` and fails there is an all-digit value too
  large for `intmax_t` (e.g. 26 nines): bash's `test` prints
  `[: <value>: integer expression expected` on stderr and returns 2, the
  `|| return 0` still drops the file, and the script does not abort. Verified in
  the scratch harness. The stderr text carries the guest's own bytes, but the
  regex has already restricted them to `[0-9]+`, and any launcher stderr that
  reaches a close frame now passes through `strings.ToValidUTF8` — so this is
  noise in the log, not a channel. Leading zeros are read base 10 by bash's
  `legal_number` (`010` → 10), not octal, so no surprise there either.

- **The test's `kill -9 -- -<pgid>` only ever targeting the group the test
  created.** True today, by construction of the flow, but not enforced by the
  fixture — this is the first finding above. Concretely: the pidfile is
  `filepath.Join(t.TempDir(), "pid")`, a private 0700 directory no other
  process writes; `setsid` puts the `bash` in a new session so its `$$` is also
  its pgid; the first `startLeader` runs against a non-existent file and the
  second runs after `kill_group_file` removed it, so neither can read a stale
  value in the current code. The gap is that nothing asserts it.

## Cleared on inspection

- **`timeout 3` on the docker half (`internal/orchestrator/ports.go:73`).**
  `timeout` is coreutils and the runtime image is `debian:trixie`
  (`image/Dockerfile:15`), so it is present. The `2>/dev/null` redirection
  applies to the `timeout` process and is inherited by `docker`, so a missing
  `docker` binary (system VMs have no inner dockerd) is silenced exactly as
  before — no new stderr into the scan output. A timeout exits 124, which
  `|| true` absorbs. The sentinel is printed BEFORE the docker half, so a
  wedged dockerd loses the image/port annotation and keeps every socket row —
  the degradation the comment claims. `timeout` does not cover the `ss` /
  `/proc/net/tcp` half, which cannot hang. A guest can shadow `timeout` on its
  own PATH, but it already controls `docker`, `ss` and `awk` in that script and
  both halves are documented as guest-controlled text.
- **`--confirm` arity (`deploy/styrman-ctl:943`).** `[ $# -ge 2 ] || die
  "--confirm needs a value"` before the `shift 2`; `die` writes to stderr and
  exits 1, so the silent `set -e` exit is gone. The second half of round 1's
  note is NOT fixed and is carried forward: the global scan still consumes
  `--yes`/`--confirm`/`--dry-run` from any position, including one a verb meant
  as its own value. Root-only operator argv, so robustness, not a boundary.
- **`pingWithin` (`browser.go:540-544`).** `defer cancel()` in a real function
  body, so no context leak; the only behavioural change is that a slow pane no
  longer eats the budget the upstream ping then needs. Worst case one tick now
  spends a full `browserRelayPing` in two halves; the ticker channel has
  capacity 1 so ticks coalesce rather than pile up, and coder/websocket takes a
  different mutex for control frames, so a stalled data writer still cannot
  block the ping. Detection of a dead upstream is delayed by at most half a
  period — no keepalive is skipped.
- **Launcher exec identity.** `browserExec` passes `User: guestUser`
  (`browser.go:87`; `ExecSpec.User`, `internal/orchestrator/exec.go:18`, where
  `""` would mean root). So `kill_group_file` runs as the project user inside
  the project's own microVM and every pid/comm/kill decision is same-uid —
  round 1's blast-radius framing still holds after this change.
- **`revoked` phase wiring.** `CLOSE_REVOKED = 4003` (`web/src/lib/api.ts:916`),
  the same constant `TerminalPane.tsx:236` uses. The 4003 branch sits BEFORE the
  `!opened && autoRetries.current > 0` branch in `onClose`, so a revoked close
  cannot trigger the automatic one-second retry, and the `revoked` render
  (`BrowserPane.tsx:789`) offers no Reconnect button — the stated contract.
- **Half-attached session detach (`BrowserPane.tsx:218-224`).** Sends
  `Target.detachFromTarget` for a session whose `Page.enable` threw and nulls
  the refs first, so the toolbar cannot keep writing into it; `.catch(() => {})`
  on a best-effort teardown. Client-side only, no server trust change.
- **`kill_group_file`'s "leader already gone" branch
  (`styrman-browser:261-266`) — residual, carried forward from round 1
  finding 6.** When `/proc/<pid>` does not exist the comm check is skipped
  entirely and `kill -TERM`/`-KILL -- -<pid>` still fire on any pgid ≥ 2 that
  `pgrep -g` finds members for, named by a guest-writable file. The `-ge 2` fix
  closes the pid-0 special case, not this one. It stays in scope on purpose
  (a dead Chromium leader whose zygote children hold the group is exactly what
  `do_start` cleans up), and it is same-uid inside one microVM, so a guest can
  signal nothing it could not signal directly. Recorded so the residual is not
  read as gone.

## Checked

- **Secrets:** no credential, token or key added or moved. `styrman-ctl` still
  stages the GH token in a root-only 0700 `mktemp -d` and never in argv; the
  signal trap makes that removal MORE reliable (an interrupted multi-minute
  clone now removes it and stops, instead of resuming over a removed dir).
  Nothing new is logged; the one guest string that reaches a client
  (`closeReason`) is Chromium's own log tail and is now UTF-8-scrubbed.
- **Injection:** no new shell interpolation. `portScanScript` gains a literal
  `timeout 3` prefix on a constant command line — no guest data is
  interpolated into it. The pid value is regex-restricted to `^[0-9]+$` before
  it reaches `pgrep`/`kill`, and `kill -- "-${pid}"` uses `--` plus quoting. No
  SQL, template or path built from client input in this range.
- **Authz/authn:** unchanged. The relay is still owner-only via
  `getProjectFor`; the revocation watch is only ADDED to, never relaxed. The
  new client `revoked` phase is presentation — it grants nothing, and a
  Reconnect from it would still hit the handshake check.
- **Crypto:** none touched, nothing hand-rolled, no session/JWT/password code
  in the range.
- **Dependencies:** no manifest changed in this range —
  `git diff c4128f5..211b2f8 -- go.mod go.sum web/package.json
  web/package-lock.json deploy/caddy/ingress/go.mod` is empty, so the
  accepted set from round 1 is unchanged. `npm audit --omit=dev` in `web/` re-run:
  1 high, `js-yaml` 4.0.0–4.3.1 (GHSA-2883-xcg3-v3hh), build-time only for a
  statically exported SPA, pre-existing and outside this range.
  `govulncheck` was NOT re-run for this round: the meaningful run is
  `bash deploy/vulncheck.sh`, which builds both modules inside the pinned
  `golang:1.25.13-trixie` image, and with no dependency change in the range its
  result is the documented accepted set round 1 recorded (root module and caddy
  ingress: GO-2026-4887 and GO-2026-4883, both
  `github.com/docker/docker@v28.5.2+incompatible`, "Fixed in: N/A";
  `image/harness` clean). Flagged rather than faked.
- **GCP surface:** none — self-hosted; `docs/deploy/gcp.md` is "not applicable".
  No IAM binding, service account, `--allow-unauthenticated` or key file in the
  range.
- **Text, not binary (BUG-007):** every source path in this range shows real
  `+`/`-` line counts in `git diff --stat`; nothing reported "Binary files
  differ". The `web/dist/client/assets/*` bundles in the wider commit are
  generated build output and were not read (see below).

## Not examined

- `web/dist/client/**` — the committed, `go:embed`ed build output changed in
  this commit (11 renamed asset bundles plus `_shell.html`) and was excluded
  from the dispatch's path list. I did NOT verify that the committed `dist`
  is a build of the committed `web/src` and contains nothing else. That is a
  real gap: the embedded bundles are what actually ship to a browser, and a
  diff of minified assets is not reviewable by reading. The honest check is a
  reproducible rebuild (`web/e2e/run.sh` rebuilds before testing), not a read.
- The rest of `web/src/components/BrowserPane.tsx` outside the two changed
  hunks (screencast latch, pointer capture, rAF mouse batching) — unchanged in
  this range and not re-audited; round 1 also left it unexamined.
- `.agents/docs/**` changes in this commit (specs, ADR-068 amendment,
  BUG-042..052, verifier log, changelog, ROADMAP/TODO) — documentation, read
  only for the review file named in the dispatch.
- Live-guest behaviour: nothing was executed inside a Kata microVM. The
  `timeout 3` degradation path, the launcher's `kill_group_file` against a real
  Chromium group, and the process-group reach of a `docker exec` inside a Kata
  guest are all reasoned from the source and from host-side equivalents, not
  observed in the guest.
- Only two tests were run (`TestLauncherKillGroupFile`,
  `TestCloseReasonFitsAndKeepsRunes`, both bare, both PASS, exit 0). The full
  `go test ./...`, the caddy ingress module's tests, the integration script and
  the browser e2e were NOT run — that is the verifier's log
  (`.agents/docs/testing/styrman/2026-09-13-verify-browser-review-fixes.md`),
  which this review did not audit.
- `deploy/styrman-ctl` outside `setup_auto_update` and the `main()` argument
  scan; the uninstall verbs and the menu were not re-read.

---

# Round 3 — the round-2 fixes   (2026-09-13)

Range reviewed: 211b2f8..2a501ae

Paths: `image/configs/styrman-browser`, `internal/httpapi/browser.go`,
`internal/httpapi/browser_launcher_test.go`.

## Verdict: FINDINGS (all four items closed; two new low notes on the new code)

All four items are closed, and three of the four are closed cleanly. The one
that is closed in effect but weak in mechanism is `forwarder_up`: it now ties
the recorded socat pid to the :9222 listener's owner, but it learns the owner by
`grep -o 'pid=[0-9]*'` over the WHOLE `ss -ltnpH` line, and that line carries the
guest-chosen process NAME. A process named `pid=1234` holding the listener emits
a synthetic `1234` into `port_owners`'s output alongside the real owner —
verified against a synthetic ss line (no process involved): the pipeline printed
both `1234` and `99999`. So the new tie defeats the ACCIDENTAL case the review
found (a stale socat plus the user's unrelated app on :9222) but not a
deliberate one. Since every input to `forwarder_up` — the pid file, the
processes, the listener — is already the guest's own and the server relies on
none of it, this stays low. Separately, and not introduced here but sitting
inside the guard this round tightened: `^[0-9]{1,7}$` accepts a ZERO-PADDED pid,
`/proc/0001234` does not exist while `pgrep -g` and `kill -- -N` both resolve it,
so the comm check in `kill_group_file` can still be skipped on demand for a live
group.

## The four items

1. **Test pgid from the process handle, not the pid file — CLOSED.**
   `browser_launcher_test.go:118-141`: `leader.SysProcAttr =
   &syscall.SysProcAttr{Setsid: true}` and `pid := strconv.Itoa(leader.Process.Pid)`.
   With `Setsid` the Go runtime calls `setsid(2)` in the child itself, so the
   child IS the session leader and pid == pgid by construction — no dependence
   on whether util-linux `setsid` forked, and the `exec.LookPath("setsid")`
   skip is correctly dropped (the test already guards `runtime.GOOS != "linux"`).
   The poll loop now WAITS for the pid file to equal that value and
   `t.Fatalf`s if it never does, instead of adopting whatever the file held, so
   the `t.Cleanup` `kill -9 -- -<pgid>` can only ever name the child this test
   started and the "went down" loop can no longer go green on a stale value.
   Round-2 finding 1 closed. Ran bare: `go test ./internal/httpapi/ -run
   TestLauncherKillGroupFile -count=1 -race` → ok, exit 0 (race detector clean,
   so the concurrent `leader.Process.Pid` read alongside the `Wait()` goroutine
   is not a data race either). `go vet ./internal/httpapi/` → exit 0.
   Residual, unchanged and cosmetic: the second `startLeader()` still registers
   no `t.Cleanup`, so a regression in the kill path leaks a `sleep 300` for five
   minutes.

2. **`revokedFlag` latch + `closePane` — CLOSED.** `browser.go:484-501, 529-552`.
   Every one of the five select arms now closes the pane through `closePane`
   (verified by grep: `:532, :535, :538, :544, :546, :551` — no bare `pane.Close`
   remains in the select), and `closePane` substitutes 4003 whenever the flag is
   set. **Memory ordering is sound and needs no mutex.** Go's memory model has
   specified `sync/atomic` operations as sequentially consistent since Go 1.19,
   so `revokedFlag.Store(true)` is a release the `Load()` cannot read stale past
   once it has completed. The callback's ordering is also the right way round:
   `Store(true)` precedes `revoked <- struct{}{}` in program order, the send
   happens-before the receive, and the receive happens-before the `Load` in
   `closePane` — so the `<-revoked` arm is guaranteed to see `true`, and the
   other four arms see it whenever the store has landed, which is the whole
   point. `closePane` is called from exactly one goroutine (one select arm runs,
   then the function returns), so there is no concurrent `pane.Close`. The one
   residual no latch can close is a revocation that has not been DETECTED yet:
   `RevocationWatch` polls every `reauthEvery`, so an arm that fires inside that
   window still reports its own code — correctly, since at that instant the
   server does not know. Substituting 4003 in the `ctx.Done()` and `paneErr`
   arms too is right rather than over-broad: the flag can only be true after a
   real revocation. Round-2 finding 2 closed.

3. **`forwarder_up` ties the pid to the listener's owner — CLOSED in effect,
   weak in mechanism (see finding below).** `styrman-browser:100-117`:
   `port_owners | grep -qx "${pid}"` replaces `port_listening`. The
   coincidence the round-1 note described — a live `socat` the user runs for
   their own purpose plus their own app on :9222 — no longer reads as "up",
   because the recorded pid must be among the owners of THAT socket. Note
   `port_listening` is deliberately retained at `:145` for the "something else
   listens on :9222, fail closed with the reason" branch, which is the right
   use for it (occupancy, not liveness) and not a leftover.

4. **Pid regex bounded to 1–7 digits — CLOSED for its stated purpose.**
   `styrman-browser:117` and `:269`. `/proc/sys/kernel/pid_max` on this host is
   `4194304` and `PID_MAX_LIMIT` on 64-bit Linux is the same 7-digit ceiling, so
   the bound rejects no legitimate pid, and `[ "${pid}" -ge 2 ]` can no longer
   be handed a value wider than `intmax_t` — the `[: …: integer expression
   expected` diagnostic that round 2 recorded as reaching the launcher's output
   is now unreachable. Verified: the 26-nines case that produced it is rejected
   by the regex before `[` runs.

## Findings (new, on the round-3 code)

- **[low / confidence: high]** `image/configs/styrman-browser:103`
  (`port_owners`) — `ss -ltnpH | grep ":${PORT} " | grep -o 'pid=[0-9]*' | cut -d= -f2`
  extracts `pid=` tokens from the ENTIRE line, and the line's `users:((…))`
  field contains the owning process's NAME, which the guest chooses
  (`prctl(PR_SET_NAME)` or the binary's basename, 15 bytes, more than enough for
  `pid=1234`). Confirmed by running the exact pipeline over a synthetic ss line
  `users:(("pid=1234",pid=99999,fd=5))`: it printed `1234` and `99999`. A guest
  that wants `forwarder_up` to vouch for a listener it controls therefore only
  has to name that listener after the pid it recorded — the check reads a
  name-supplied token as a kernel-supplied one. Why it is only low: the pid
  file, the processes, the port and the listener are all the project user's own,
  inside the project's own microVM, and the server relies on none of them (the
  real containment is bridge isolation plus `previewPortAllowed`); the worst
  outcome is the owner pointing their own Browser pane at their own process,
  which they can already do. Remediation, if it is worth the code: take the tie
  from the kernel rather than from ss's text — read the listening socket's inode
  from `/proc/net/tcp` for `:${PORT}` and confirm `/proc/<pid>/fd` contains a
  `socket:[<inode>]` link for it. Stripping the quoted name (`sed 's/"[^"]*"//g'`)
  is NOT a complete fix: `comm` may itself contain a double quote.
- **[low / confidence: high]** `image/configs/styrman-browser:269`
  (`kill_group_file`) — the regex accepts a NON-CANONICAL numeric form. Verified
  on this host: `/proc/1` exists but `/proc/0000001` does not, while `pgrep -g`
  and `kill -- -N` both parse the padded string numerically. So a guest that
  writes `0001234` (7 chars, inside the new bound, reachable for any pgid of six
  digits or fewer) gets `[ -d "/proc/0001234" ]` to read FALSE, the comm check
  skipped entirely as the "leader already gone" branch, and `kill -TERM`/`-KILL
  -- -0001234` delivered to the live group 1234. This is not introduced by this
  diff — `^[0-9]+$` had the same hole — but it sits inside the guard this round
  tightened and it turns round 1's "already-gone leader" residual from an
  accident into something selectable. Same uid inside one microVM, so no
  privilege crossing (signals to another uid's group return EPERM), which is why
  it stays low. Remediation: canonicalise before use — `pid=$((10#${pid}))`
  after the regex, or anchor it as `^[1-9][0-9]{0,6}$`. `forwarder_up` is NOT
  affected: its `grep -qx "${pid}"` compares the raw file text against ss's
  canonical decimal, so a padded value simply fails to match and the forwarder
  is restarted (fail-safe).

## Cleared on inspection

- **`port_owners | grep -qx` under `set -euo pipefail`.** The fail direction is
  safe. Traced and measured: `grep -q` exits on its first match and can SIGPIPE
  the upstream, and with `pipefail` the pipeline then reports 141 even though
  the match succeeded — reproduced with a 200 000-line producer, which returned
  141 and, at top level, aborted the script under `set -e`. Three things keep
  this harmless here. (a) The real producer emits one or two short lines, far
  under the 64 KiB pipe buffer, so `cut` completes before `grep -q` closes;
  the small-output case measured 0. (b) `forwarder_up` is only ever called in a
  condition (`if forwarder_up` at `:138` and `:163`; no other call site), so
  errexit is suppressed and a 141 cannot abort the launcher. (c) A spurious
  non-zero reads as "the forwarder is not up", and `ensure_forwarder` responds
  by REPLACING the forwarder — a restart, not a trust. Same for
  `port_owners`'s own internal pipefail: `ss` failing while the greps match
  yields non-zero and the same fail-safe restart.
- **The `grep ":${PORT} "` window.** `PORT=9222` is a literal at `:47` pinned to
  `config.BrowserPort` by `TestLauncherPinsPortsAndCaps`, not env-derived, so
  nothing outside the script interpolates into the pattern. The trailing space
  plus the leading colon means `:19222 ` and `[fe80::9222]:80` do not match; the
  peer column of a listener is `0.0.0.0:*`, so it cannot supply a false match
  either.
- **Blast radius of the `browser.go` change.** `atomic` is the only new import;
  no new dependency, no manifest change in this range
  (`git diff 211b2f8..2a501ae -- go.mod go.sum` empty). No handler, guard or
  route changed — `closePane` only rewrites a close CODE on a connection every
  arm already closed. Secrets, injection, authz, crypto and GCP surface are
  unchanged from round 2; `npm audit` was not re-run because `web/` is untouched
  by this range, and `govulncheck` likewise (no Go dependency moved).

## Not examined (round 3)

- The rest of commit 2a501ae outside the three named paths (docs, changelog,
  bug records, `web/dist`) — not read.
- No live-guest execution again: `port_owners` was exercised against a synthetic
  `ss` line and against this host's real `ss -ltnpH`, never against a socat
  forwarder inside a Kata microVM, and `forwarder_up` was not run end to end.
- The full suite was not run: only
  `go test ./internal/httpapi/ -run 'TestLauncher|TestCloseReason|TestRelay|TestBrowser' -count=1`
  (ok, exit 0), the same under `-race` for `TestLauncherKillGroupFile` (ok, exit
  0), and `go vet ./internal/httpapi/` (exit 0). The caddy ingress module, the
  integration script and the browser e2e were not run.
- Whether any test covers the new latch or `port_owners` — no test was located
  for either, and I did not write one.

---

# Round 4 — inode ownership and the leading-zero refusal   (2026-09-13)

Range reviewed: 2a501ae..bac799f1b9d95b2342a034045dda731bbdc44e7b (HEAD)

Paths: `image/configs/styrman-browser`, `internal/httpapi/browser_launcher_test.go`.

## Verdict: FINDINGS (both round-3 lows closed; one new low, same family, narrower)

Both round-3 findings are closed. The leading-zero one is closed outright and
verified. The ownership one is closed where I said the weakness was — no process
NAME is parsed any more, and `owns_socket`'s `/proc/<pid>/fd` readlink is exactly
the kernel-authoritative tie I named: procfs generates those entries itself, a
guest cannot `ln -s` into `/proc/<pid>/fd`, and a `socket:[<ino>]` link is proof
that the process really holds that socket. But the weak link did not disappear;
it MOVED, and it is now the only one left. `ss -ltneH` prints a `cgroup:` field
on the same line — confirmed against this host's real output — and
`grep -o 'ino:[0-9]*'` reads the WHOLE line, so a cgroup path segment named
`ino:424242` yields a synthetic inode. Verified on a synthetic ss line: the
current pipeline printed `6756` AND `424242`; anchoring the match to a whole
FIELD printed only `6756`. Consequence and severity are unchanged from round 3 —
everything involved is the project user's own, inside their own microVM, and the
server relies on none of it — so this stays low, and it is genuinely narrower
than what it replaced (a 15-byte comm is trivially set; a cgroup path has to be
created).

## The two items

1. **No process name parsed; inode tie through `/proc/<pid>/fd` — CLOSED for
   the mechanism I named, see the finding below for what replaced it.**
   `styrman-browser:100-108`. `ss -ltneH` drops `-p`, so the `users:(("<comm>",…))`
   field that carried the guest-chosen name is gone from the line entirely —
   the round-3 injection channel is shut. `owns_socket` (`:102-108`) readlinks
   every `/proc/$1/fd/*` for `socket:[$2]`; that is kernel-rendered and
   unforgeable, and it fails closed in both degenerate cases — verified by
   running the exact function: a dead pid (glob misses, no `nullglob`, so the
   loop runs once on the literal, `readlink` fails, returns 1) and a live pid
   that does not hold the inode both returned 1 with the script still alive
   under `set -euo pipefail` + `inherit_errexit`. Cross-uid is fail-closed too:
   `readlink` on another user's `/proc/<pid>/fd` gives EACCES, silenced by
   `2>/dev/null`, so the answer is "not ours". The SIGPIPE hazard round 3
   recorded is gone with the pipe: `for ino in $(port_inodes)` is a command
   substitution, and a failing substitution in a `for` word list does not trip
   errexit (verified: `for x in $(false)` survived under `set -e` +
   `inherit_errexit`). A socket closed between `port_inodes` and `owns_socket`
   reads as "not up" and `ensure_forwarder` restarts — fail-safe.
2. **`^[1-9][0-9]{0,6}$` in both pid files — CLOSED.** `styrman-browser:117`
   (`forwarder_up`) and `:287` (`kill_group_file`). A leading zero is now
   refused before `/proc` is consulted, so the round-3 path — `/proc/0001234`
   absent ⇒ comm check skipped ⇒ `pgrep -g`/`kill -- -0001234` still resolving
   numerically — is unreachable. The bound still admits every legitimate pid
   (`PID_MAX_LIMIT` = 4194304, 7 digits, no leading zero in anything `echo $$`
   or procfs produces) and still keeps `[ -ge 2 ]` away from a value wider than
   `intmax_t`. Covered by `browser_launcher_test.go:219` (`"0"+stranger` added
   to the quiet-drop loop) and `:159-166` (`forwarder_up` refuses a zero-padded
   pid). Ran bare: `go test ./internal/httpapi/ -run
   'TestLauncherPortOwnership|TestLauncherKillGroupFile' -count=1 -v` → both
   PASS, exit 0.

## Findings (new, on the round-4 code)

- **[low / confidence: high]** `image/configs/styrman-browser:101` (`port_inodes`)
  — `ss -ltneH | grep ":${PORT} " | grep -o 'ino:[0-9]*' | cut -d: -f2` matches
  anywhere on the line, and `ss -e` puts a `cgroup:` field there. Real output on
  this host: `… ino:6756 sk:2 cgroup:/system.slice/ssh.service <->`. A cgroup
  whose path contains a segment `ino:<digits>` therefore contributes a synthetic
  inode — confirmed by running the exact pipeline over a synthetic line carrying
  `cgroup:/system.slice/ino:424242`, which printed `6756` and `424242`. To turn
  that into a false "the forwarder is up" a guest still has to hold the recorded
  pid as a live `socat*`-named process AND have it hold a socket with the
  injected inode, all of which is inside its own microVM and all of which it
  could already do; the server relies on none of it (containment is bridge
  isolation plus `previewPortAllowed`), so this is low, and it is strictly
  harder than the comm the round-3 code accepted. Remediation, one line and
  verified: anchor to a whole FIELD —
  `awk '{for(i=1;i<=NF;i++) if($i ~ /^ino:[0-9]+$/){sub(/^ino:/,"",$i); print $i; break}}'`
  — which printed only `6756` on the same crafted line, because a cgroup field
  is always prefixed `cgroup:` and can never equal `^ino:[0-9]+$`. Stronger
  still: take the inode from `/proc/net/tcp`/`tcp6` (column 10) for the port,
  a line that has no cgroup or name field on it at all.
- **[low / confidence: medium]** `internal/httpapi/browser_launcher_test.go:143-166`
  — `TestLauncherPortOwnership` has no POSITIVE control for `forwarder_up`:
  both of its `forwarder_up` cases assert a non-zero return, and every way the
  function can be broken (a wrong `SOCATPID` path, a regex that rejects
  everything, a `port_inodes` that returns nothing) also returns non-zero, so a
  regression that makes `forwarder_up` permanently false stays green. The path
  is in fact right — `DIR="${HOME}/.styrman/browser"` at `:50`, which the test's
  `home/.styrman/browser/socat.pid` matches — and `port_inodes`/`owns_socket`
  are each covered positively, which is why this is only low. Remediation: one
  case where `forwarder_up` returns 0 (a process the test owns whose comm starts
  with `socat` — e.g. a copy or a symlinked `bash` renamed — holding the
  listener), or failing that an assertion that the two negative cases fail for
  DIFFERENT reasons (distinguishable stderr).

## Cleared on inspection

- **Can a guest forge a `/proc/<pid>/fd` entry?** No. Procfs generates the fd
  directory from the process's open-file table; there is no `mknod`, `ln -s` or
  write path into it, and a `socket:[<ino>]` link means the kernel says that
  process holds that socket. What a guest CAN do — open sockets freely, pass fds
  between its own processes with `SCM_RIGHTS`, and pick which of its processes
  is named in the pid file — does not weaken `owns_socket`; it is only ever the
  `port_inodes` half that decides WHICH inode is "the :9222 listener", which is
  the finding above.
- **`owns_socket`'s argument is not validated inside the function.** Today the
  only caller validates `$1` against `^[1-9][0-9]{0,6}$` first, and `"$1"` is
  inside double quotes so the value cannot glob. A future caller passing an
  unvalidated value would path-traverse (`/proc/../../etc/fd/*`). Not reachable
  now; noted as the kind of helper that should carry its own guard.
- **Cost.** `owns_socket` forks one `readlink` per fd, nested inside one
  iteration per listening inode. A forwarder with `max-children=16` has a
  handful of fds; a guest could give the recorded process a very large fd table
  to slow its own `status` exec toward the 30 s budget — self-inflicted, inside
  its own pane.
- **The `grep ":${PORT} "` window, again.** Unchanged from round 3 and still
  sound: `PORT=9222` is a literal at `:47` pinned by
  `TestLauncherPinsPortsAndCaps`; the trailing space plus leading colon excludes
  `:19222 ` and `[fe80::9222]:80`, and a listener's peer column is `0.0.0.0:*`.
- **`forwarder_up`'s precedence.** `[[ … ]] && [ -d … ] && comm_is … || return 1`
  parses as `((A && B) && C) || return 1` — a rejected pid returns 1 before the
  loop, and `comm_is` still runs before any `/proc/<pid>/fd` walk.
- **Scope.** No Go production code changed in this range, no dependency moved
  (`go.mod`/`go.sum`/`web/` untouched), no new secret, no injection sink, no
  authz/authn, crypto or GCP surface touched. `port_listening` is still retained
  at `:162` for the "something else listens, fail closed" branch, which remains
  the correct use for it.

## Not examined (round 4)

- Nothing was run inside a Kata guest again: `port_inodes` was exercised against
  this host's real `ss -ltneH` and against a synthetic line, `owns_socket`
  against a standalone reimplementation of the exact function body and via the
  package test, but `forwarder_up` was never run end to end against a real socat
  forwarder on :9222 — no positive path was observed anywhere, by me or by the
  test (see the second finding).
- Only `go test ./internal/httpapi/ -run
  'TestLauncherPortOwnership|TestLauncherKillGroupFile' -count=1 -v` was run
  (PASS, exit 0). The full suite, the caddy ingress module, the integration
  script and the browser e2e were not run in this round.
- Whether a cgroup path segment containing `ino:<digits>` is actually creatable
  by the project user in the shipped runtime image (cgroup namespace, delegation,
  and whether the in-guest dockerd exposes a `--cgroup-parent` the user controls)
  was NOT tested — the finding rests on the parse, which is demonstrated, not on
  a demonstrated creation path.
- The rest of commit bac799f outside the two named paths.

---

# Round 5 — whole-field ss selection and the positive control   (2026-09-13)

Range reviewed: bac799f..1318e79

Paths: `image/configs/styrman-browser`, `internal/httpapi/browser_launcher_test.go`,
`.agents/docs/testing/styrman/integration.sh`.

## Verdict: FINDINGS (both round-4 lows closed; one residual of the same family, narrower again)

Both round-4 lows are closed, and the awk selection is a real improvement on two
axes at once, each verified by running the exact expression: the round-4
cgroup-fragment attack (`cgroup:/system.slice/ino:424242`) now yields only the
genuine `6756`, and a row for a DIFFERENT port whose line merely contains
`ino:9222` is no longer selected at all — a hazard the old `grep ":9222 "` had
and which nobody had filed. The residual is the last turn of the same screw: awk
splits on whitespace, and a cgroup NAME may contain a space, so
`cgroup:/system.slice/x ino:424242` splits into two fields and the second one
matches `^ino:[0-9]+$`. Demonstrated: that line printed `6756` AND `424242`.
Severity is unchanged and still low — everything involved is the project user's
own, inside their own microVM, the server relies on none of it — and the fix is
one token (`break` after the `print`, since a row carries exactly one real
`ino:` and it precedes `cgroup:` in iproute2's field order).

## The two items

1. **Whole-field selection — CLOSED for what was filed.**
   `styrman-browser:99-108`. `port_listening` and `port_inodes` both select the
   row by `$4 ~ (":" p "$")` and `port_inodes` takes only fields `5..NF` matching
   `^ino:[0-9]+$`. Verified by running the expression over five crafted rows:
   the round-4 cgroup fragment is rejected (prints `6756` only); an `ino:9222`
   token on an `:8080` row selects nothing; `[fe80::1%eth0]:9222` and a plain
   `0.0.0.0:9222` are both selected (no false negative from a bracketed IPv6 or
   a `%scope`); `[fe80::9222]:80` is correctly not selected. The `$` anchor is
   what makes the address column safe regardless of scope contents: ss puts the
   port after the LAST colon of `$4`, so a scope name cannot manufacture a
   `:9222$` for a socket on another port. Column shifting is not possible
   either — the kernel's `dev_valid_name()` rejects `/`, `:` and whitespace in
   an interface name, and iproute2 refuses one client-side (confirmed:
   `ip link add name 'bad name'` → `"name" not a valid ifname`; the `:` case
   needs root and was not run here). `-e` appends its extras after the peer
   column, so `$4`/`$5` do not move; starting the scan at 5 rather than 6 is
   harmless since a peer column (`0.0.0.0:*`) can never match `^ino:[0-9]+$`.
   `port_inodes` is now a single pipeline into awk with no early-quitting
   consumer, so the round-3 SIGPIPE/pipefail question is gone for good, and
   `END { exit !f }` gives `port_listening` the right answer (1) on no match and
   on an `ss` that produced nothing.
2. **Positive control — CLOSED, twice over.**
   `browser_launcher_test.go:171-222`: the test re-execs its own binary through
   a symlink named `socat` (so the child's `comm` is `socat`), has it hold a
   listener on the port, records its pid, and asserts `forwarder_up` SUCCEEDS.
   A permanently-false `forwarder_up` — the regression round 4 said would stay
   green — now fails the test. Ran bare: `go test ./internal/httpapi/ -run
   TestLauncher -count=1 -v` → all four launcher tests PASS, exit 0, so the
   success path is genuinely exercised and not skipped. The integration tier
   adds the end-to-end half I could not get from a unit test — "a second start
   keeps the running forwarder (socat pid unchanged)" proves `forwarder_up`
   returns 0 against a REAL socat inside a real guest, and it catches the
   operational consequence (a never-up forwarder respawning socat on every
   relay connect and dropping the other panes). I did NOT run that tier.

## Finding (new, on the round-5 code)

- **[low / confidence: high]** `image/configs/styrman-browser:106` (`port_inodes`)
  — awk field-splitting is whitespace-based, and a cgroup directory name may
  contain a space (kernfs rejects only `/` and NUL), so `ss`'s unescaped
  `cgroup:<path>` can be split into two fields with the second one being a
  standalone `ino:<digits>`. Verified by running the committed expression over
  `… ino:6756 sk:2 cgroup:/system.slice/x ino:424242 <->`: it printed `6756` and
  `424242`. To reach a false "the forwarder is up" a guest must additionally
  hold the recorded pid as a live `socat*`-named process owning a socket with
  the injected inode — all inside its own microVM, all of which it can already
  do, and the server relies on none of it — so this stays low and is another
  notch harder than round 4's (which needed no space). Remediation, minimal and
  sufficient: `break` immediately after the `print`, because a selected row
  carries exactly one genuine `ino:` field and iproute2 emits it BEFORE
  `cgroup:`. Belt and braces: also `break` on a field matching `^cgroup:`.
  Definitive: read the inode from `/proc/net/tcp`/`tcp6` column 10, whose line
  contains no free text at all — which is where I would stop parsing `ss`
  output for a trust decision.

## Cleared on inspection

- **`TestHelperListener` as a re-exec helper.** The `os/exec` convention
  (`TestHelperProcess` + an env guard) and the same failure mode: it `t.Skip`s
  unless `STYRMAN_HELPER_LISTEN` is set, so a normal run is unaffected, but with
  that variable exported in the environment `go test ./...` would block in
  `select {}` forever rather than fail. The name is project-namespaced and the
  risk is a hang on a developer machine, not a defect in shipped code — noted,
  not filed.
- **The helper handshake.** `ln.Close()` then the child binds: if another
  process takes the port in that window the child's `net.Listen` fails, it exits,
  `io.ReadFull` sees EOF and the parent `t.Fatalf`s — loud, not a silent skip.
  The failure message already prints the child's `comm`, which is the right
  thing to print when this test fails. `t.Cleanup` is registered immediately
  after `Start`, and the helper is not a group leader, so `Process.Kill()` is
  enough and there is no group to orphan.
- **The symlink named `socat` in the test's HOME.** `launcherFunctions` sets
  `PATH` from the environment and `home` is not on it; `comm_is` reads
  `/proc/<pid>/comm`, never PATH. So the symlink cannot be picked up as a
  command by anything the launcher runs.
- **The new integration check's shape.** It compares the socat pid file across
  two `start` calls and stops the browser afterwards, so it does not leave state
  for the following check; it reads the pid through `docker exec … cat`, not
  through a shell that could glob. It asserts a value is unchanged AND non-empty
  (`[ -n "$p1" ]`), so an always-empty read cannot pass it vacuously — the
  round-4 lesson applied.
- **Scope.** No Go production code and no dependency changed in this range
  (`go.mod`, `go.sum`, `web/` untouched); no secret, injection sink, authz path,
  crypto or GCP surface involved.

## Not examined (round 5)

- The integration tier was NOT run: `bash .agents/docs/testing/styrman/integration.sh`
  needs a real Kata guest and a live server, and the new "second start keeps the
  running forwarder" check — the only end-to-end proof that `forwarder_up`
  returns 0 against a real socat — is therefore unverified by me. It should be
  in the verifier's log, not inferred from this review.
- No `ss` was run inside the shipped runtime image: the field layout was
  confirmed against this host's iproute2 and against crafted rows, not against
  the guest's. If the guest's iproute2 differs in field ORDER (specifically if
  `cgroup:` ever preceded `ino:`) the `break`-after-print remediation above
  would need the `^cgroup:` guard instead.
- Whether a cgroup name containing a space is creatable by the project user in
  the shipped image (delegation, cgroup namespace, what the in-guest dockerd
  exposes) was not tested — the finding rests on the demonstrated parse, not on
  a demonstrated creation path.
- The rest of commit 1318e79 outside the three named paths.
