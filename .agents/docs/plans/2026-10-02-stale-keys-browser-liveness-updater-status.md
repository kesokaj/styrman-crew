---
owner: main
date: 2026-10-02
---
# Plan — stale keystrokes, browser-view recovery, updater status in Settings   (2026-10-02)

## Why
The v1.40.0 handoff (`handoffs/2026-10-02-1921-v1.40.0-updates-freeze-harness-ingress.md`,
Next Steps 3) put five decisions to the operator, each with a recommendation. The
operator, 2026-10-02: "go with your recommendations. Fix those then cut a new version".
Asked which shape item 5 should take: "Sure, for both local and gcp. Remember, now we are
only deplying to local not gcp. But if you find something we need to fix for both
alternatives. Why do we need a special gcp fix?" — and, on the restatement "no GCP-specific
piece; one change in the shared code … crew's Settings page shows admins when the
auto-updater is failing (and why)": "Yes, updater status in Settings (Recommended)".

The five, as decided:
1. Keys queued while a terminal pane reconnects can be replayed hours later when the
   machine slept inside the retry window (gate finding 1 of v1.40.0) — fix.
2. The browser view has the dead-socket cause too — fix it the same way.
3. The Mac key that signed v1.39.0 is trusted by the GCP VM with no decision record —
   write the ADR.
4. The docs name `docker restart styrman-caddy` as the recovery for a host not yet on the
   ingress fix — keep.
5. Nothing tells the operator when a host's auto-updater is failing (BUG-084: ten days
   unnoticed) — show it to admins in Settings, by shared code, the same on every host.

## Scope
In: items 1, 2, 3, 5 above; item 4 is a recorded decision with no change; a release
through `/preflight`.

Out:
- An outside monitor of the site (a GCP uptime check): the operator asked why a GCP-specific
  fix would be needed and chose the shared-code status instead. A site that is down cannot
  show its own banner; the ingress heals the known cause of that by itself since v1.40.0.
- Replacing this host's updater token (BUG-084): the operator's step, unchanged.
- The setup pane's reconnect behaviour.

Follow-ups noted, not built: none.

## Surfaces touched
Recon: `plans/2026-10-02-recon-updater-status.md`.
- Item 1: new `web/src/lib/termOutbox.ts`, `web/src/components/TerminalPane.tsx`.
- Item 2: `web/src/lib/cdp.ts`, `web/src/components/BrowserPane.tsx`.
- Item 5: `deploy/update/styrman-update`, `deploy/update/styrman-update.service`,
  `deploy/docker-compose.yml`, new `internal/updatestatus/`, `internal/config/config.go`,
  `internal/httpapi/explorer.go` (the settings handler), `web/src/lib/api.ts`, new
  `web/src/lib/updaterNotice.ts`, `web/src/routes/settings.tsx`.
- `web/dist/client` rebuilt. Tests: `web/src/lib/*.test.ts`, `internal/config/`,
  `internal/updatestatus/`, `internal/httpapi/`, `.agents/docs/testing/styrman/integration.sh`,
  `web/e2e/run.mjs`, `web/e2e/run.sh`, the count lines in `testing/styrman/README.md`.
- Describing text that changes with the behaviour, in the same commits:
  `web/src/components/TerminalPane.tsx` (the queue comments at the `outbox` declaration and
  in `reconnectNow`), `web/src/components/BrowserPane.tsx` (the header comment's "never
  auto-reconnects" lines 26-31 and the JSX comment near line 768),
  `specs/styrman/terminal-protocol.md` (the "Typed-while-down queue" bullet; the "Not
  covered: the browser pane" bullet; the browser relay section near line 193),
  `specs/styrman/guide.md` (the "Terminal stopped responding after sleep" row, which ends
  "The browser pane is not covered"), `specs/styrman/deploy.md` and
  `specs/styrman/versioning.md` (the updater), `specs/styrman/api.md` (the settings
  payload), `deploy/README.md`. The list is re-derived by grep before the review
  (`seconds old`, `not covered`, `never auto-reconnects`, `browser pane`, `silently`,
  `failed-version`) and every hit not named here is added to the change.

## Interfaces

**Item 1 — `web/src/lib/termOutbox.ts`.**
```ts
export const OUTBOX_MAX_AGE_MS = 30_000;
export const OUTBOX_MAX_ENTRIES = 256;
export interface Outbox {
  push(data: string): void;   // ignored once the queue holds maxEntries
  drain(): string[];          // everything, oldest first, and empties — or [] (and empties)
                              // when the OLDEST queued chunk is older than maxAgeMs
  clear(): void;
  readonly size: number;
}
export function createOutbox(o?: { maxEntries?: number; maxAgeMs?: number; now?: () => number }): Outbox;
```
All or nothing: a queue whose oldest chunk has aged out is dropped whole, never trimmed
(half a command line is worse than none). The age is measured with `Date.now()` (wall
clock: it moves on across a sleep, timers do not). `TerminalPane` replaces its array with
one `createOutbox()` per connection effect: `push` where it pushed, `drain()` in
`flushOutbox`, `clear()` where it emptied the array.

**Item 2 — browser view.**
- `CdpClient.alive(timeoutMs = 5000): Promise<boolean>`: sends `Browser.getVersion` and
  resolves true as soon as ANY frame arrives from the relay inside the window — the reply,
  an error reply, or an unrelated event such as a screencast frame: a relay that is busy
  streaming is alive, and a reply queued behind frames must not read as dead. It resolves
  false when nothing at all arrives within `timeoutMs`, and false when the client is
  closed or closes under the probe.
- `BrowserPane`, in its connection effect: a probe runs when `visibilitychange` goes to
  visible, on `online`, and every 25 s while the document is visible; only for a socket
  that has opened and is not closed, never two at once. "Visible" is the DOCUMENT's
  visibility only: a pane that is mounted but not the selected view inside the app (the
  terminal pill is selected) probes and reconnects all the same, so it is live when its
  user switches back (D15). The pane reconnects — its existing
  `reconnect()`, the path of its Reconnect button — only when the probe resolved false AND
  the client is still not closed, i.e. the probe TIMED OUT on a socket that reads open. A
  socket that closed (4002, 4003, 4004, or any close, under a probe or not) keeps today's
  behaviour exactly: `onClose` chooses the state and its button, nothing reconnects by
  itself.

**Item 5 — updater status.**

*One name, one path.* `STYRMAN_UPDATE_STATUS_FILE` names the file for BOTH sides, and
neither side has a default (ADR-052's shape): the updater's unit pins it
(`Environment=STYRMAN_UPDATE_STATUS_FILE=/etc/styrman/status/updater.json` in
`styrman-update.service`), and `deploy/docker-compose.yml` pins the same value in the
server's `environment:` and mounts the directory read-only
(`- /etc/styrman/status:/etc/styrman/status:ro`). Unset on the updater's side: no status
is written. Unset on the server's side — `go run`, the test suites unless they set it —
the feature is off: no read, no `updater` key. Docker creates a missing bind source as a
root-owned 0755 directory, so a host that never set up auto-update starts fine and simply
has no file. Nothing the server's uid can write is on the updater's path (D3).

*Content* (one JSON object on one line, exactly these six keys, every value validated
before it is written):
```json
{"result":"failing","code":"fetch-auth","checkedAt":"2026-10-02T17:16:19Z","since":"2026-09-22T09:14:02Z","deployed":"v1.38.0","latest":""}
```
- `result`: `ok` | `failing`.
- `code`: `ok` when `result` is `ok`. When `failing`: `fetch-auth` (git fetch failed and
  its output matched `Authentication failed`, `Invalid username or token` or `could not
  read Username`), `fetch` (git fetch failed otherwise), `no-tags`, `signature`
  (`verify_tag` refused `latest`), `rolled-back` (`latest` did not come up and the
  previous version was restored), `deploy-failed` (it did not come up and there was
  nothing to roll back to, or the rollback failed too), `latched` (`latest` failed earlier
  and is not retried), `error` (every other `fail`).
- `checkedAt`: UTC, `date -u +%FT%TZ`, when this tick ended.
- `since`: when `failing`, the `checkedAt` of the first tick of the current failing
  streak (carried over from the previous file when that file's `result` was `failing` and
  its `since` matches `^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]{8}Z$`, else this tick's
  `checkedAt`); `""` when ok.
- `deployed`: the tag this host runs, `vX.Y.Z` (`^v[0-9]+\.[0-9]+\.[0-9]+$`), or `""` when
  the updater has not recorded one (`none`, an empty or malformed state file).
- `latest`: the newest stable tag this tick saw, same pattern, or `""` when the tick
  ended before it knew (every fetch failure, `no-tags`).
No other text from the remote, and no free-form message, ever reaches the file (D4).

*The updater.*
- Three globals carry what the handler writes: `STATUS_DEPLOYED` — set when the status is
  armed, from the `deployed-version` state file, and again after a deploy (`LATEST`) or a
  rollback (`PREVIOUS`); `STATUS_LATEST` — set on the line after `LATEST` is computed
  (`:415`); `STATUS_CODE` (with `STATUS_RESULT=failing` where the exit status alone would
  say otherwise). `write_status <result> <code>` reads the two tag globals; a value that
  does not match the tag pattern is written as `""`. The PAIR is what is checked, in one
  `case`: `ok:ok` and `failing:<one of the eight failing codes>` are written as given;
  every other pair — an unknown result, an unknown code, `ok` with a failing code,
  `failing` with `ok` — is written as `failing`/`error`.
- The previous file is read without `jq` (the updater uses none): `head -c 4096` of a
  regular, non-symlink file; `since` is carried only when that text contains
  `"result":"failing"` and one `sed` capture of
  `"since":"\([0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}T[0-9:]\{8\}Z\)"` returns a value. The
  one-line, fixed-key-order form `write_status` itself writes is what makes that enough;
  anything else in the file's place just starts a new streak.
- `write_status` never fails the tick: on any refusal it logs one WARN and returns 0. It
  refuses unless the file is named `updater.json` and its directory passes
  `status_dir_ok` — a real directory (not a symlink), owned by `TRUST_UID`, not
  group/world-writable. A directory that does not exist is created with `mkdir -m 755`
  (so its owner is the caller, which is `TRUST_UID` on a real tick and in the hooks) and
  only when its PARENT passes the same `status_dir_ok`; then re-checked. It writes a
  `mktemp` file in that directory, `chmod 644`, `mv -f` onto the name.
- Initialisation (D20): the six globals `STATUS_FILE`, `STATUS_ARMED=0`, `STATUS_RESULT`,
  `STATUS_CODE`, `STATUS_DEPLOYED`, `STATUS_LATEST` and `EXIT_RM_DIR` are assigned at the
  top of the script, beside the other defaults, and `write_status`/`on_exit` are defined
  there too — all above the test hooks and the trap — so under `set -euo pipefail` an
  early refusal (not root, a bad `STATE_DIR`) runs the handler with every variable set.
  `on_exit` captures `$?` as its first statement and exits with it unchanged.
- `on_exit`, ONE EXIT handler for every mode: `trap on_exit EXIT` is installed on the
  line after the argument loop (`:248`, below the test hooks and above the root check), so
  production — where `DOCKER_CONFIG` is under `/run` and the script sets no trap today —
  runs it too. The hand-run arm of the `DOCKER_CONFIG` case (`:303-307`) stops setting its
  own trap and sets `EXIT_RM_DIR="${DOCKER_CONFIG}"`, which `on_exit` removes. When the
  status is armed, `on_exit` decides in this order: (1) `STATUS_RESULT` is set (only the
  latched exit sets it) → that result with `STATUS_CODE`; (2) exit status 0 → `ok`/`ok`,
  whatever `STATUS_CODE` holds — a code alone never makes a clean exit failing; (3) a
  non-zero exit status → `failing` with `STATUS_CODE`, or `error` when no site set one.
- Armed (`STATUS_ARMED=1`) on the line after the lock is taken (`flock -n 9`, `:336`), and
  only when `CHECK_ONLY=false`. Not armed, so nothing is written: the test hooks,
  `--check`, a tick that found the lock busy, and every refusal above the lock (not root,
  a bad `STATE_DIR`, no clone) — the previous file stays, and the server's staleness rule
  is what shows those.
- Sites: the fetch failure (`git fetch`'s output is captured into a variable, printed to
  the log as it is today, and matched for the three auth strings → `fetch-auth`, else
  `fetch`); `no-tags`; `signature` INSIDE the failure branch, never before the call
  (`verify_tag "${LATEST}" || { STATUS_CODE=signature; fail …; }`, `:450`) — every code
  site has that shape, so a check that passes leaves `STATUS_CODE` as it was;
  `STATUS_RESULT=failing STATUS_CODE=latched` before the latched `exit 0` (`:769`);
  nothing to set on "already up to date" and on "OK — … is live" (exit 0 → `ok`), except
  `STATUS_DEPLOYED="${LATEST}"` on the latter; `deploy-failed` before the two `fail`s that
  leave the box without a working rollback; `rolled-back` (and
  `STATUS_DEPLOYED="${PREVIOUS}"`) before the `fail` that follows a successful rollback.
- Two gated test hooks beside the existing two, refused unless `STYRMAN_UPDATE_TEST=1`,
  both with `TRUST_UID="$(id -u)"`:
  `styrman-update __write-status <file> <result> <code> <deployed> <latest>` calls
  `write_status`; `styrman-update __exit-status <file> <exit status> [<result> <code>]`
  arms the status, installs `on_exit` and exits with that status — the same function and
  the same decision a real tick runs.
- Callers that deliver this (`.agents/rules/mechanism-reaches-every-caller.md`): the
  `Environment=` line is in `styrman-update.service` itself, which `host-setup.sh:305`
  and the updater's own `self_install` both copy whole; the mount and the server's
  environment line are in the one compose file every deploy path uses. `deploy/styrman-ctl`
  runs the updater by hand with its own `UPDATER_ENV` (`:539`; `--check` at `:647`,
  `--force` at `:678`), which by its own comment names ALL of the updater's inputs: it
  gains `STYRMAN_UPDATE_STATUS_FILE=${ETC_DIR}/status/updater.json`, so a forced redeploy
  records its result at once instead of leaving a `latched` notice up until the next
  timer tick (`--check` is never armed and writes nothing). With the default `ETC_DIR`
  that is the path the unit and the compose file pin; a scratch `STYRMAN_ETC_DIR` (the
  tests, ADR-052) keeps the write inside the scratch tree. The existing "every input is
  named" assertion in `internal/config/styrman_ctl_test.go` gains this variable (D14). The tick that
  deploys this release runs the PREVIOUS updater; `self_install` then installs the new
  script and unit, so the first status is written one tick (5 minutes) later.

*The server.* `internal/updatestatus`:
```go
type Status struct {
	State     string `json:"state"`               // "ok" | "failing" | "stale" | "unknown"
	Code      string `json:"code,omitempty"`      // "ok" or a failing code from the list above
	CheckedAt string `json:"checkedAt,omitempty"` // RFC 3339, as written
	Since     string `json:"since,omitempty"`
	Deployed  string `json:"deployed,omitempty"`
	Latest    string `json:"latest,omitempty"`
}
// StaleAfter: the timer ticks every 5 minutes and one tick may run for 45
// (TimeoutStartSec), so an hour without a write means the updater is not running.
const StaleAfter = 60 * time.Minute
// Read returns ok=false when path is "" (the feature is off) or the file does not exist
// (auto-update never set up, or an updater older than this change): say nothing.
func Read(path string, now time.Time) (s Status, ok bool)
```
For a non-empty path whose file exists, `Read` reads at most 4 KiB, decodes strictly
(unknown keys refused, exactly one JSON value) and re-validates every field: `checkedAt`
must parse with `time.Parse(time.RFC3339, …)`; `result` `ok` requires `code` `ok` and an
empty `since`; `result` `failing` requires one of the eight failing codes and a `since`
that parses the same way; `deployed` and `latest` are each empty or match the tag
pattern. Anything else that goes wrong — a permission error, a directory in the file's
place, a decode or validation failure — is `State: "unknown"` with no other field, never
"ok" (BUG-014). A valid file whose `checkedAt` is more than `StaleAfter` before `now` is
`State: "stale"` with its fields kept; otherwise `State` is the file's `result`.
`config.Config` gains `UpdateStatusFile` from `STYRMAN_UPDATE_STATUS_FILE`, default `""`.
The settings handler adds `"updater": <Status>` only for an admin and only when `Read`
returned ok=true.

*The page.* `web/src/lib/updaterNotice.ts`:
`updaterNotice(u: UpdaterStatus | undefined): { tone: "danger" | "warn"; text: string } | null`.
- null for `undefined` and for `state: "ok"`.
- `failing` (tone danger), by code, each starting "Auto-update has been failing since
  <since>": `fetch-auth` "GitHub rejected this host's token (/etc/styrman/gh-token)";
  `fetch` "the host cannot reach the repository"; `no-tags` "the repository has no
  release tag"; `signature` "<latest> is not signed by a key this host trusts";
  `rolled-back` "<latest> did not come up and was rolled back"; `deploy-failed` "<latest>
  did not come up and could not be rolled back"; `latched` "<latest> failed earlier and
  is not retried"; `error` and any other value "the updater stopped with an error (see
  `journalctl -u styrman-update.service`)". When `deployed` is not empty the notice ends
  "This host stays on <deployed>." — except for `deploy-failed`, which never says it: after
  a failed rollback the state file still names the previous tag while the box runs whatever
  the failed deploy left (`styrman-update:826-835`), so the sentence would be false (D19); when `latest` is empty in a sentence that names it,
  the words "the newest release" stand in.
- `stale` (tone warn): "Auto-update has not run since <checkedAt>."
- `unknown` (tone warn): "Auto-update status cannot be read."
Dates are shown as written (UTC). `settings.tsx` renders the notice as one block directly
under the page heading, for admins only (`data.isAdmin`), with
`data-testid="updater-notice"` and `role="status"`; text only, never HTML.

## Failure modes
- The status directory is missing with an untrusted parent, not the caller's, a symlink
  or group/world-writable: the updater logs a WARN and writes nothing; the tick's own
  outcome is unchanged. The server shows the previous file as `stale` after an hour, or
  nothing when there is none.
- A host whose updater predates this change (this host until it takes the release): no
  file, nothing shown. The first status is written on the tick after the release deploys.
- The timer is disabled, or the unit refuses above the lock: `stale` after an hour.
- A file with unknown keys, a bad code or tag, over 4 KiB, or unreadable: `unknown`.
- `STYRMAN_UPDATE_STATUS_FILE` unset on the server (a dev run): no read, nothing shown.
- Not an admin: no `updater` key in the settings payload, nothing rendered.
- The site is down: nothing can be shown (Scope, Out).
- Browser view: a probe that gets no frame at all for 5 s on a living but silent relay
  reconnects a healthy pane — one reconnect; the page being viewed in the guest is
  untouched. Frames flowing during the window count as alive.
- A tick killed by the unit's 45-minute `TimeoutStartSec` (SIGTERM): bash runs the EXIT
  handler on the fatal signal, so the status reads `failing`/`error` (D21). A SIGKILL
  after the stop timeout writes nothing; the hour's staleness shows it.
- Queue: keys typed more than 30 s before the socket came back are dropped silently, as
  keys typed at a closed pane always were.

## Verification
Tiers: unit, integration (Tier 2), e2e (Tier 4). No tier of the main agent's own.
- Unit, Go: `internal/updatestatus` (empty path and missing file → ok=false; each state;
  unknown key, bad code, bad tag, oversized file, a directory in the file's place →
  unknown; stale at `StaleAfter` plus one second, not at exactly `StaleAfter`); the
  settings handler (admin with a file gets `updater`; admin with the path unset or no
  file, and a non-admin with a file, get no key); a NEW file `internal/config/styrman_update_status_test.go`
  (`.agents/rules/writing-files.md`: `styrman_update_test.go` exists and is not rewritten)
  (every test named `TestStyrmanUpdateStatus*`, D18) through the two hooks: a written file has exactly the six keys on one line and
  validates; a pair outside the list (an unknown code, `ok` with a failing code, an unknown result) is
  written as `failing`/`error` and a tag that does not match as `""`; `since` is carried across two failing writes and cleared by an ok
  one; a missing directory under a trusted parent is created 0755; a missing directory
  under a group-writable parent, a symlinked directory and a group-writable directory are
  each refused with nothing written and exit status 0; no temp file is left;
  `__exit-status` maps exit 0 → ok/ok, exit 1 → failing/error, and exit 0 with an
  explicit `failing latched` → failing/latched. One test pins the script's TEXT — the
  trap is installed above the `DOCKER_CONFIG` case and not inside it, the status is armed
  on the line after `flock` and not under `--check`, each site sets its code, and the globals are assigned above the
  trap line: a guard for what the hooks cannot reach, NOT evidence of a real tick. One
  hook case runs the handler UNARMED with a non-zero status (the shape of an early
  refusal): nothing is written and the exit status is unchanged.
- Unit, web: `termOutbox.test.ts` (order, the cap, the age rule at its boundary,
  all-or-nothing, clear), `cdp.test.ts` (`alive`: reply, error reply, an unrelated event
  inside the window, nothing for the whole window with fake timers, close under the
  probe, already closed), `updaterNotice.test.ts` (null cases; each code's sentence;
  empty `deployed` and empty `latest`; `deploy-failed` never says "stays on"; `stale`;
  `unknown`).
- Integration: the suite starts its server with
  `STYRMAN_UPDATE_STATUS_FILE="${STATE}/status/updater.json"` (`STATE` is the suite's own
  `mktemp -d` under `/var/tmp`). A planted `failing` file takes its `checkedAt` and
  `since` from `date -u +%FT%TZ` at the moment of the plant — a copied literal would be
  more than an hour old and read `stale`. NEW checks: with no file, `GET /api/settings` has no `updater`
  key; with a `failing`/`fetch-auth` file planted, `.updater.state == "failing"` and
  `.updater.code == "fetch-auth"`; with garbage planted, `.updater.state == "unknown"`.
  The plant is removed afterwards.
- E2E: the criteria below.
  - Criterion 1's fixture: a throwaway page from `context.newPage()` with
    `installSocketFixture` (extended to list `/ws/browser/` sockets) installed before its
    `goto` of `/p/<project>/2` — terminal 2, as the two existing throwaway pages use
    (`run.mjs:1314`, `:1369`), because a page that opens a terminal takes its socket from
    whoever held it (close 4001) and the suite's own page holds terminal 1
    (`run.mjs:1545`); terminal 2 exists until the cleanup at the suite's end. It clicks the "Browser" pill, waits for a first screencast
    frame, asserts exactly one open browser socket, freezes it (sends dropped, events
    withheld, `readyState` still OPEN), emulates hidden → visible, and waits for a new
    unfrozen open browser socket and the pane's `data-phase="ready"`. It sits inside the
    suite's LAN window after "switching to the terminal and back resumes the screencast",
    is closed in a `finally`, and does not touch the suite's own page: its terminal socket
    is terminal 2's, and the browser relay has no replace semantics
    (`specs/styrman/terminal-protocol.md`, "Browser relay"), so a second viewer of the
    project's browser does not displace the first — the existing check that follows
    ("terminal session survived the browser round-trip") still runs on the suite's page
    and terminal 1.
  - Criterion 2's fixture: `web/e2e/run.sh` exports
    `STYRMAN_UPDATE_STATUS_FILE="${STATE}/status/updater.json"` before it starts its
    server, so the server and `run.mjs` (which reads `process.env.STYRMAN_UPDATE_STATUS_FILE`
    — the one name, no second variable) see the same path; unset, the check FAILS saying
    so, it does not skip. The check, on the Settings page, asserts no notice, creates the
    directory and writes a `failing`/`fetch-auth` file from the test process with
    `checkedAt` and `since` set to the current time (`new Date().toISOString()` cut to
    whole seconds), reloads, asserts the notice's text, and removes the file in a
    `finally`. It navigates nothing the following checks depend on (they start from
    Settings).
  - `.agents/rules/e2e-checks-leave-the-suite-as-they-found-it.md`: each check is run
    once red against the build before its change — the pass mark of each such run is
    `failed=1`, every other check passing — and the count line in the testing README
    changes in the same commit.
- Red before green: the web unit tests against stubs; the Go tests against the hooks
  before `write_status` exists; each e2e check and the three integration checks against
  the build before the change.

E2E criteria:
1. A browser view whose connection stopped carrying data without closing streams again,
   without a page reload, after the tab is shown again.
2. An admin whose host reports a failing updater sees, on the Settings page, a notice that
   says auto-update is failing and since when; with no report there is no notice.

Not covered by any tier: a real updater tick as root on a host (the hooks run the writer
and the exit decision, not the tick); the status file appearing on a real host after a
release (this host cannot take a release until BUG-084's token is replaced); the
fresh-host path (`.agents/rules/shared-path-learnings.md`: unexercised — no new VM is
brought up; `host-setup.sh` is not changed, it copies the unit file as before); a real
sleep for the queue's age rule (the clock is injected); the 25 s interval and `online` in
a browser.

## Phases
- [x] Cold-reader check — three rounds (`reviews/2026-10-02-review-plan-stale-keys-browser-liveness-updater-status.md`, overwritten each round); round 3: `design needs revision`, `implementation ready`, four non-blocking gaps; at the cap the operator, 2026-10-02: "Fix all things before cut a new version or defer the things or skip." — all four fixed in the plan (D18–D21)
### Phase 1 — the page: stale keys, browser-view recovery
- [x] E2E criterion 1's check written and run red against the unfixed build — verify: `bash web/e2e/run.sh`, AI — expect: `failed=1`, that check, every other passing — got `passed=97 failed=1 skipped=2`, that check (20 s wait for the new socket), 2026-10-02
- [x] `termOutbox.ts` with its tests, wired into `TerminalPane` — verify: `cd web && npm test`, AI — expect: PASS; the age and all-or-nothing tests fail against a plain array queue — docs: specs/styrman/terminal-protocol.md — 203 tests pass; with the age rule removed 4 tests fail
- [x] `CdpClient.alive` with its tests; the pane's probe — verify: `npm test`; e2e criterion 1, AI — expect: PASS — docs: specs/styrman/terminal-protocol.md, specs/styrman/guide.md — PASS; with frame waiters not called 3 `alive` tests fail; e2e criterion 1 passes
- [x] `web/dist/client` rebuilt; count lines updated — verify: `bash web/e2e/run.sh`, AI — expect: `failed=0` — docs: testing/styrman/README.md — `passed=98 failed=0 skipped=2`, 2026-10-02
- [x] Checkpoint — expect: Tier 1 and Tier 4 green — evidence: `testing/2026-10-02-verify-stale-keys-browser-liveness-updater-status-phase-1.md` (PASS: Go 394 pass/1 skip, ingress 63, web 203 in 17 files, Tier 4 `passed=98 failed=0 skipped=2`), 2026-10-02
### Phase 2 — updater status in Settings
- [ ] ADR-081 (the status file: where it lives, who writes and reads it, what it may contain) appended — verify: read, AI — expect: present
- [ ] The NEW integration checks and e2e criterion 2's check written and run red against the unfixed build — verify: `integration.sh`, `web/e2e/run.sh`, AI — expect: Tier 2: the two checks that expect an `updater` key fail and the "no key" check passes; Tier 4: `failed=1`, that check
- [ ] `write_status`, `on_exit`, the code sites, the two hooks; the unit's `Environment=STYRMAN_UPDATE_STATUS_FILE=` line; the same variable in `styrman-ctl`'s `UPDATER_ENV` and its test — verify: `go test ./internal/config/ -run 'TestStyrmanUpdate|TestStyrmanCtl' -v`, AI — expect, besides PASS: every `TestStyrmanUpdateStatus*` name listed in the output — expect: PASS; each new test fails before its code exists — docs: specs/styrman/versioning.md (the updater, the status file, `STYRMAN_UPDATE_STATUS_FILE`), specs/styrman/deploy.md, deploy/README.md
- [ ] The compose mount and the server's `STYRMAN_UPDATE_STATUS_FILE` line — verify: `docker compose -f deploy/docker-compose.yml config` with a scratch env file, AI — expect: exit 0, the mount listed read-only, the variable in the server's environment — docs: specs/styrman/deploy.md
- [ ] `internal/updatestatus`, `Config.UpdateStatusFile`, the settings payload — verify: `go test ./internal/updatestatus/ ./internal/httpapi/ ./internal/config/`; the integration checks, AI — expect: PASS — docs: specs/styrman/api.md
- [ ] `updaterNotice.ts`, the Settings block; `web/dist/client` rebuilt — verify: `npm test`; e2e criterion 2, AI — expect: PASS — docs: specs/styrman/guide.md, testing/styrman/README.md
- [ ] Checkpoint — expect: Tier 1, Tier 2, Tier 4 green — evidence: verify log path, date
### Phase 3 — records and release
- [ ] ADR-082 (two release signers; a host deploys only tags signed by a key in its own `allowed_signers`) appended; item 4 recorded as D5 — verify: read, AI — expect: present — docs: specs/styrman/versioning.md, deploy/gcp.md
- [ ] Definition of Done: reviewer, security (a root script, a root-written file the server parses, a new mount), documentation, `/review --docs`, bug records, change entries (Phase 2's entry carries a "Learnings for the local host" section, `.agents/rules/shared-path-learnings.md`, and lists the callers of the status variable by name) — verify: reports' `## Status`, AI — expect: no open high or medium
- [ ] `/preflight` cuts the next MINOR — verify: its gate report, AI — expect: tag signed with the original key and pushed
- [ ] Checkpoint — expect: handoff on disk; this host's update still waits on BUG-084's token — evidence: handoff path, date

## Risks
Assumptions:
- The relay passes `Browser.getVersion` and the in-guest Chromium answers it within 5 s — checked by e2e criterion 1 (a healthy pane must NOT reconnect: the check asserts one open socket before the freeze).
- Docker creates a missing bind source as a root-owned 0755 directory, even under a 0700 parent, and the container's non-root user can read it and cannot write it — checked 2026-10-02: `docker run --user <uid> -v <scratch 0700 dir>/status:/etc/styrman/status:ro …` printed `drwxr-xr-x 2 0 0 … /etc/styrman/status`, "No such file" for the json, and `Read-only file system` on a write. The real `/etc/styrman/status` on a host is the same dockerd code path; not run there.
- `self_install` installs the edited unit file and reloads systemd, so the `Environment=` line reaches a host by the release alone — read in `deploy/update/styrman-update:503-560` (it installs `styrman-update`, `.service`, `.timer` and runs `daemon-reload`); not run.
- The e2e server runs with `STYRMAN_AUTH=none`, where the caller is an admin (`internal/httpapi/auth.go:92`) — checked by criterion 2's check.

## Decisions
D1 — 2026-10-02 — Items 1, 2, 3 and 5 are fixed and item 4 is kept, as recommended in the v1.40.0 handoff — by: operator, "go with your recommendations. Fix those then cut a new version"
D2 — 2026-10-02 — Item 5 is the updater's status shown to admins in Settings, by shared code, with no GCP-specific piece — by: operator, "Yes, updater status in Settings (Recommended)"; "Why do we need a special gcp fix?"
D3 — 2026-10-02 — The status file lives in a root-only directory mounted read-only into the server, not under `/var/lib/styrman` (the recon's first suggestion): that tree is owned by the server's uid, and root writing into it is the BUG-009 shape (a planted symlink turns a status write into a root write elsewhere). One compose mount is the price — by: implementer; ADR-081
D4 — 2026-10-02 — The file carries codes and validated tags only, never text from the remote or from git; the server re-validates and the page maps codes to its own sentences — by: implementer, from the recon's risk 3
D5 — 2026-10-02 — The docs keep `docker restart styrman-caddy` as the recovery for a host not yet on the ingress fix — by: operator (D1; the handoff's recommendation was "keep (it is what you chose on 2026-10-02)")
D6 — 2026-10-02 — The queue's age rule is all-or-nothing on its OLDEST chunk, and 30 s — by: implementer (the v1.40.0 security review proposed "about 30 s"; a trimmed queue would send half a line)
D7 — 2026-10-02 — The browser pane probes on the same three triggers as the terminal (shown, online, every 25 s while visible) and reconnects through its existing `reconnect()` only on a probe that timed out; a socket that closed keeps its manual button — by: implementer, "fix it the same way" (D1); the timed-out-only rule by cold read round 1 (gap 5)
D8 — 2026-10-02 — The second release signer is accepted and written down (ADR-082), not removed: removing it needs the GCP VM, which this machine cannot reach — by: operator (D1; the handoff listed "write the ADR" first)
D9 — 2026-10-02 — The EXIT handler is installed once, on the line after the argument loop, for every mode; the hand-run arm of the `DOCKER_CONFIG` case hands its directory to it instead of setting a trap — by: cold read round 1 (gap 1: the script's only trap sits in a branch production never takes)
D10 — 2026-10-02 — One variable name, `STYRMAN_UPDATE_STATUS_FILE`, on both sides, with no default on either: the unit and the compose file pin it. A server without it has the feature off, so a dev run on a host whose `/etc/styrman` is root-only shows nothing instead of "cannot be read" — by: cold read round 1 (gaps 3 and 11); the variable exists at all because the two suites must point the server at a scratch file (ADR-052's shape)
D11 — 2026-10-02 — `stale` and `unknown` are states of their own: a refusal above the lock and a disabled timer write nothing, so only age can show them; an unreadable file must never read as "ok" (BUG-014). `StaleAfter` is 60 minutes, above the unit's 45-minute `TimeoutStartSec` — by: implementer; the threshold by cold read round 1 (gap 7)
D12 — 2026-10-02 — `alive()` counts any frame inside the window as proof, not only its own reply: a relay streaming screencast frames must not be reconnected because one reply queued behind them — by: cold read round 1 (gap 6)
D13 — 2026-10-02 — An ok status has the single code `ok`; the file's `latest` is used by the sentences of `signature`, `rolled-back`, `deploy-failed` and `latched`; the page's sentence for `error` is also what any other value gets, with no separate fallback path — by: cold read round 1 (scope notes, gap 4)
D14 — 2026-10-02 — `styrman-ctl`'s hand runs name the status file too (`UPDATER_ENV`, `${ETC_DIR}/status/updater.json`): its own rule is that a hand invocation names every input of the updater, and a forced redeploy should clear a `latched` notice at once — by: cold read round 2 (gap 3)
D15 — 2026-10-02 — A Browser pane that is mounted but not the selected view probes like a shown one; only the document's visibility gates the probe — by: cold read round 2 (gap 7); the reconnect of a pane nobody is looking at is invisible and leaves it ready
D16 — 2026-10-02 — E2E criterion 1's throwaway page opens terminal 2, never terminal 1 — by: cold read round 2 (gap 1: a terminal socket is taken from its holder with 4001, and the suite's page holds terminal 1)
D17 — 2026-10-02 — `on_exit` treats a clean exit as `ok` whatever `STATUS_CODE` holds, and every code site sets its code inside its failure branch — by: cold read round 2 (gap 2)
D18 — 2026-10-02 — The hook tests are named `TestStyrmanUpdateStatus*` and the phase runs `-run 'TestStyrmanUpdate|TestStyrmanCtl' -v`, reading the names in the output — by: cold read round 3 (gap 1: `TestUpdater` matched nothing, so the run passed without them); operator at the cap, "Fix all things before cut a new version"
D19 — 2026-10-02 — The `deploy-failed` notice does not say "This host stays on <deployed>" — by: cold read round 3 (gap 2); operator, same words
D20 — 2026-10-02 — The status globals and both functions are defined at the top, above the hooks and the trap; `on_exit` keeps the exit status — by: cold read round 3 (gap 3); operator, same words
D21 — 2026-10-02 — A tick killed by its timeout reads `failing`/`error`; no separate code — by: cold read round 3 (question 4, default kept); operator, same words
Rejected: a GCP uptime check (the operator: "Why do we need a special gcp fix?"); the status file under `/var/lib/styrman` (D3); a free-text reason in the file (D4); trimming an aged queue to its fresh tail (D6); a status write for refusals above the lock (the status directory's own trust is not established there; staleness covers them); a default path for the status file on the server (D10); `install -d -o root` for the status directory (it cannot run in the non-root hook, so its branch would be untested: `mkdir` makes the caller the owner in both); hand runs that write no status, caught up by the next timer tick (D14); a second variable to carry the status path from `run.sh` to `run.mjs`.
