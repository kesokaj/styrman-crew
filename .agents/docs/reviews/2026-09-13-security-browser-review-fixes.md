---
owner: security
date: 2026-09-13
---
# Security review — browser review fixes   (2026-09-13)

Range reviewed: 61608ca..c4128f5

## Verdict: FINDINGS

No privilege boundary is crossed by this change and nothing here is reachable
by a non-owner: the relay stays owner-only, the launcher runs as the project
user (so every pid/comm decision is same-uid), and the ports parser's new
sentinel cannot be forged from the half a guest controls. The two things worth
fixing are both "guest bytes reach further than the code assumes". First, the
hardened `kill_group_file` accepts `0` as a pid — `/proc/0` never exists, so
the new comm check is SKIPPED, `pgrep -g 0` matches the caller's own group
(verified: procps translates 0 to the caller's pgid), and the next statement is
`kill -TERM -- -0`, i.e. the launcher signals its own process group on the
say-so of a guest-writable file. Second, the launcher's log tail is now copied
verbatim into a WebSocket CLOSE frame, and coder/websocket does not validate
the reason as UTF-8, so one stray byte in `chromium.log` turns the diagnostic
close into a 1006 protocol error. Both are confined to the project owner's own
VM and own pane, which is why neither is above low.

## Findings
- **[low / confidence: high]** `image/configs/styrman-browser:243-258` (`kill_group_file`)
  — the new pid validation is `[[ "${pid}" =~ ^[0-9]+$ ]]`, which accepts `0`.
  `0` is the one value with special meaning to both tools used next:
  `[ -d /proc/0 ]` is false, so the comm check the whole fix rests on is skipped;
  `pgrep -g 0` is documented by procps to mean "the caller's own process group"
  and matches the launcher itself (confirmed on this host); `kill -TERM -- -0`
  then signals that group, followed by `kill -KILL -- -0`. The pid files
  (`~/.styrman/browser/pid`, `socat.pid`) sit in the guest-writable home, so
  their content is exactly the input this code exists to distrust. The blast
  radius is the docker-exec'd launcher's own process group inside the project's
  own microVM (same uid, self-inflicted), and whether that group can extend to
  the guest's init was NOT verified against a live Kata guest. Remediation:
  require `pid -ge 2` (reject `0` and `1`) before either `pgrep -g` or `kill`,
  and treat a missing `/proc/<pid>` as "drop the file, signal nothing" rather
  than as a licence to signal the group.
- **[low / confidence: high]** `internal/httpapi/browser_launcher_test.go:118-126`
  — the "recycled pid" fixture writes `1` into the pid file and calls
  `kill_group_file … chrom` on the DEVELOPER'S machine during `go test ./...`.
  It passes today only because `/proc/1/comm` is `systemd`; if the comm check
  ever regresses (or the suite runs where pid 1's name starts with `chrom`, e.g.
  in a container), the very next statement is `kill -TERM -- -1`, which POSIX
  defines as "every process the caller may signal". A test that guards a kill
  should not use a pid whose negation is a wildcard: spawn a throwaway process
  with a known non-matching comm, or use the test's own pid.

- **[low / confidence: high]** `internal/httpapi/browser.go:400` — `closeReason(tail(out, 2048))`
  puts the tail of `~/.styrman/browser/chromium.log` (guest-writable, arbitrary
  bytes) into the close frame. The rune-boundary cut only guarantees the FIRST
  kept byte is not a continuation byte; an invalid sequence in the middle of
  the kept tail is passed through, and coder/websocket v1.8.15 does not
  validate the reason (`close.go bytesErr` checks length and code only). RFC 6455
  requires the reason to be valid UTF-8, so Chrome fails the connection and the
  pane sees 1006 "disconnected" instead of 4004 + the reason. Self-inflicted
  (the project owner is the only writer and the only viewer), which is why it is
  low. Remediation: run the tail through `strings.ToValidUTF8(out, "�")`
  (or drop non-printables) before `closeReason`, the same way a terminal
  transcript is sanitized before display.
- **[low / confidence: medium]** `internal/httpapi/browser.go:515` — the `select`
  races `revoked` against `paneErr`/`upErr`/`pingErr`. When a revoked user's pane
  also errors in the same instant, Go picks a ready case pseudo-randomly, so the
  client can be closed 1000/1011 instead of the deterministic 4003. The ACCESS
  itself is still cut (both conns are closed on every branch), only the code the
  client sees is nondeterministic; the terminal bridge has the same shape. If the
  4003 signal matters to the pane's logic, latch a `revoked atomic.Bool` in the
  watch callback (as `bridge.go` does) and prefer it when choosing the close code.
- **[low / confidence: medium]** `image/configs/styrman-browser:112` (`forwarder_up`)
  — the liveness test is "pid file names a live process whose comm starts with
  `socat`" AND "something listens on :9222"; the two are not tied to each other.
  A stale-but-live `socat` the user runs for their own purpose plus their own app
  on :9222 still reads as "the forwarder is up", and `ensure_forwarder` then
  returns 0 without replacing it. Same uid, same project, so this is the owner
  pointing the server at their own process — a bounded misroute, not a crossing.
  Remediation if it is worth the code: check the listener's owning pid
  (`ss -ltnpH` → `pid=<n>`) equals the pid file, instead of "port is listening".
- **[low / confidence: high]** `image/configs/styrman-browser:249` (`kill_group_file`)
  — the comm check is a TOCTOU (`/proc/<pid>/comm` read, then `kill -TERM -- -<pid>`),
  and the "leader already gone" branch skips the check entirely and signals the whole
  group named by a guest-writable pid file. It is strictly BETTER than the code it
  replaces (which signalled unconditionally) and is confined to one uid inside one
  microVM, so the pid-file trust is strengthened, not weakened — recorded so the
  residual is on the record rather than assumed gone.
- **[low / confidence: high]** `deploy/styrman-ctl:940` — `--confirm) CONFIRM_GIVEN="${2:-}"; shift 2`
  under `set -euo pipefail`: a trailing `--confirm` with no value makes `shift 2`
  return non-zero and the script exits silently with status 1 and no message. Also,
  the global scan now consumes `--yes`/`--confirm`/`--dry-run` from ANY position,
  including a position a verb meant as its own value. Root-only CLI, operator-supplied
  argv, so this is robustness rather than a boundary. Remediation: `[ $# -ge 2 ] || die "--confirm needs a word"` before the shift.
- **[low / confidence: high]** `web/package.json` (unchanged by this diff) — `npm audit --omit=dev`
  reports 1 high: `js-yaml@4.3.1` (GHSA-2883-xcg3-v3hh) via
  `@tanstack/react-start → @tanstack/start-plugin-core → xmlbuilder2`. Build-time
  only for a statically exported SPA; pre-existing, outside this range, recorded
  so the row is not silently green.

## Cleared on inspection (the questions asked)

- **Sentinel forging (`internal/orchestrator/ports.go`).** The scan half emits
  `"<port> <comm>"` lines, so no line can EQUAL `--styrman-docker--`; and a comm is
  capped at 15 bytes by the kernel while the sentinel is 18, so the string cannot
  even appear inside one. `splitPortScan` takes the FIRST match and `TrimSpace`s,
  so a CR does not defeat it. A guest that prints the sentinel in an image name
  (after the split) only hides its own ports from its own panel. `|| true` keeps
  the exit status meaningful for the vanished-container re-ask, which now rests on
  the single exec's own code. Image names reach the UI as React text nodes, not HTML.
- **Revocation wiring.** `s.stillAuthorized` is assigned in `New` before any
  handler runs; nil in `AuthMode != "header"`, and `relayWebSockets` starts no
  watch when the check is nil — no nil deref in `AuthMode=none`. The `email == ""`
  exemption is the same closure the bridge gets, so the loopback dev listener's
  relays are not closed every minute. `RevocationWatch` fires `onRevoked` once into
  a buffered channel and returns; no goroutine leak.
- **Streamed pump / 1009.** `errors.Is(err, websocket.ErrMessageTooBig)` is correct
  for v1.8.15: `read.go:529` wraps it (`limitReader`), and the library closes the
  offending conn 1009 itself — the old `CloseStatus` comparison could indeed never
  match. Read limits are set on BOTH conns (16 MiB upstream, 1 MiB pane) before any
  pump starts, so the 32 KiB default never applies. An abandoned writer holds only
  the msgWriter mutex; control frames (Ping, Close) take a different mutex, so the
  heartbeat and the close code still get out. `sync.Pool` buffers are per-pump and
  returned on exit.
- **Origin / upstream.** `sameOriginOrAbsent` before any side effect, no redirects
  followed, no Origin on the upstream dial (which is what Chromium's empty
  allowlist admits), browser ports still refused by `previewPortAllowed`.
  `req.Close = true` on discovery stops a kept-alive connection parking one of
  socat's 16 children.
- **Single-peer admission.** `peer_range` still accepts only an anchored IPv4
  literal, so nothing can smuggle a comma into socat's option list; "set but
  unusable" now fails the start instead of leaving an unreachable browser. Note
  the whole forwarder ACL is guest-side and therefore not a control the server
  relies on — the real containment is bridge isolation plus `previewPortAllowed`.
- **`CTL_TMPD` trap.** The value comes from `mktemp -d`, the trap tests `-n`
  before `rm -rf`, and the success path clears the variable before removing the
  trap, so neither `rm -rf ""` nor a second removal of a reused path is reachable.
  `--dry-run` never sets it (no secret touches disk).

## Checked
- **Secrets:** no credential, token or key added; the launcher's log tail that now
  reaches the close frame is Chromium's own output, and `browserExec` passes only
  `HOME`/`USER`/`STYRMAN_BROWSER_PEER`. `styrman-ctl` still stages the GH token in a
  root-only 0700 temp dir, never argv — the trap fix makes the cleanup MORE reliable.
- **Injection:** the one new shell construction (`setsid -f bash -c … _ "$@"`) passes
  every value as a positional parameter, not through the string; `range=` is an
  anchored IPv4 literal; `portScanScript` is a constant with no interpolation of
  guest data; no SQL, no template, no path built from client input.
- **Authz/authn:** relay is owner-only via `getProjectFor` (never `previewAccess`),
  the cap is taken per user, and the new revocation watch only ADDS a check. The one
  exec that replaced two is still behind the same guard.
- **Crypto:** none touched; nothing hand-rolled.
- **Dependencies:** no new module and no new npm package in this range.
  `govulncheck ./...` on the HOST toolchain (go1.25.0, exit 3) reports 31 findings,
  all stdlib-version noise the shipped build does not have — `deploy/vulncheck.sh`
  exists for exactly this. Run in the pinned `golang:1.25.13-trixie` builder it
  gives: root module 2 called (GO-2026-4887, GO-2026-4883 — both
  `github.com/docker/docker@v28.5.2+incompatible`, "Fixed in: N/A"), caddy ingress
  the same 2, `image/harness` clean. That is the documented accepted set, unchanged
  by this diff. `npm audit --omit=dev` in `web/`: 1 high, js-yaml (above).
- **GCP surface:** none — self-hosted; `docs/deploy/gcp.md` is "not applicable".

## Not examined
- `web/src/components/BrowserPane.tsx` beyond the first ~260 diff lines (the
  screencast/visibility latch, pointer capture and rAF mouse batching were read as
  a design change, not audited line by line), `web/src/lib/browserInput.ts`,
  `web/src/routes/p.$project.$term.tsx`, `web/e2e/run.mjs`.
- `web/dist` (excluded from the range by the dispatch).
- The added Go tests (`browser_launcher_test.go`, `ports_browser_test.go`,
  `browser_test.go` additions) were not run; no test suite was executed for this
  review.
- `FileEditor.tsx`'s pre-existing `dangerouslySetInnerHTML` (outside this range).
- Live-host behaviour: nothing was executed inside a Kata guest; the process-group
  reach of a `docker exec` inside a Kata microVM (finding 1's worst case) is
  therefore unverified.
- `deploy/verify-commit.sh`, the compose/deploy files and the caddy ingress were
  not re-read: this range does not touch them.
