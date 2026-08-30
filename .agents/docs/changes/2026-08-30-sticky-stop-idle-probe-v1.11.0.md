# Sticky project stop, busy-aware idle reaper, 8h default TTL (v1.11.0)

Date: 2026-08-30. Operator report: "When I press stop I expect the project
microvm to stop… The terminals is multiple terminals open on the same
project microvm. … If I run something in docker the terminal is not idle."

## 1. Stop didn't stick with terminal tabs open

Every terminal WebSocket connect called `EnsureRunning` — and when a VM
stopped, the dying exec's close surfaced as TRANSIENT (the aliveness probe
on a stopped container fails → "detached" 1000), so every open pane
auto-reconnected and booted the VM straight back up. Stop was impossible
while any tab was open; the idle reaper was likewise defeated by open tabs.

**Fix — deliberately-stopped marker + boot intent (ADR-042):**

- `internal/orchestrator/orchestrator.go`: new `stopped` map. Set by
  `Stop()` (before the container stop, under the project lock) and by the
  reaper's `stopIfIdle`; cleared by any successful `EnsureRunning` and by
  `Destroy`. New accessor `Stopped(user, project)`.
- `internal/terminal/bridge.go` (ServeHTTP): a connect WITHOUT `?boot=1`
  to a stopped project is answered with close **4002** (`config.
  CloseProjectStopped`) before any boot. `?boot=1` = user intent.
- `web`: `TerminalPane` sends `boot=1` only on the FIRST connect of an
  effect run (opening/switching to a pane, or pressing the button — user
  actions); automatic retries never do. Close 4002 renders "Project
  stopped — everything is kept; Start boots it right back" with a
  **Start** button (epoch bump → boot intent). `api.ts`: `CLOSE_STOPPED`,
  `boot` param on `termSocketUrl`.

## 2. Idle reaper now probes for real work

The activity map only sees ATTACHED terminals (keystrokes + output). A
detached build, a `docker run -d`, or a closed tab with an agent still
working all looked idle. `stopIfIdle` now runs an in-guest probe before
pulling the plug — either of these blocks the stop and resets the idle
clock (logged with the reason):

- any RUNNING DinD container (`docker ps -q`),
- any tmux pane whose foreground process isn't a plain shell
  (`tmux -S /tmp/tmux-*/default list-panes -a -F '#{pane_current_command}'`
  as root — verified against a live guest).

Probe failure (container already wedged/gone) reads as idle so the reaper
still converges. New `busyReason` helper + `busy_test.go`.

## 3. Default idle TTL 24h → 8h

A stop only powers the VM off — workspace, home, transcripts, and the
docker disk persist; the next attach resumes exactly where it left off.
Changed in `internal/config/config.go`, `deploy/docker-compose.yml`
fallback, `deploy/.env.example`, and the operator's live `deploy/.env`
(had an explicit 24h pin that would have overridden the new default; no
`settings.json` runtime override exists on the live host).

## Test tooling

- `cmd/wsprobe`: prints `[close:<code>]` on a server-initiated close so
  scripts can assert protocol behavior.
- `integration.sh`: 4 new checks — stopped project refuses a plain
  reconnect with 4002 AND stays stopped; `boot=1` boots it and the shell
  answers.

## Verification

- `go build`/`vet` clean; unit suites green (new `TestBusyReason`).
- web `tsc`/oxlint/build clean.
- Busy-probe commands verified against a live guest VM as root.
- Integration suite: same 52 passes as a stashed-HEAD baseline run; the 4
  pre-existing failures are ENVIRONMENTAL (3 proxy checks blocked by the
  v1.9.0 host firewall — the test server runs as a host process, unlike
  production's containerized server — plus one pre-existing async-delete
  record-lingering failure reproduced identically on clean HEAD; tracked
  separately). The 4 new 4002/boot checks pass.
