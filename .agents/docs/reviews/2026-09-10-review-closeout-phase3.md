---
owner: reviewer
date: 2026-09-10
---
# Review — TODO close-out Phase 3 (deploy and scripts)   (2026-09-10)

Range reviewed: working-tree diff captured in `.agents/scratch/phase3.diff`
(items A, K1, K4-K6, L, M(d), M(f) of `.agents/docs/plans/2026-09-10-todo-closeout.md`)
against the full current files.

## Verdict: REQUEST CHANGES

Two concrete defects, both matching exactly the risk the plan itself flagged
and asked to be measured before landing. (1) M(f): `deploy/firewall/install.sh`
never `systemctl enable`s `styrman-firewall.service`, so the new `[Install]
WantedBy=multi-user.target` section it ships is inert — at the next reboot the
service will NOT start at boot as the unit file's own comment and the lowered
timer `OnBootSec=5s` both assume; only the self-heal timer will close the
window, and only after 5 s, not "right after docker.service". (2) K5: `tofu
fmt -check` was measured against the actual runtime image and emits a bare
filename with no `line:col`, so reusing `reGCC` for the new `"tofu"` parser
case means a badly-formatted `.tf` file is reported as `diagnostics: []` —
silently wrong, exactly the failure mode the plan's own conditional ("if `tofu
fmt -check` emits the same shape, else a case of its own") was written to
avoid. Everything else checked out: A's eight dependency bumps match root
exactly, `go mod verify` is clean, the caddy image builds and reports
`v2.11.4` with the `security` module; K1's `Persistent=true` removal is
provably inert (man systemd.timer: only affects `OnCalendar=`); K4's `$HOME`
is set on the exec spec, all eight checkers take the file by path, both traps
match what each script actually creates; L's freshness script is wired before
any check in both suites, never touches `:dev`, and behaves correctly live;
M(d)'s `daemon.json` merge is idempotent (measured: yes then no on a fixture).

## Findings

- **[high / confidence: high]** `deploy/firewall/install.sh:109` — `run
  systemctl start styrman-firewall.service` starts the unit once but never
  enables it (unlike line 110's `enable --now` for the timer). A `[Install]`
  section only takes effect through the enablement symlink `systemctl enable`
  creates; `start` alone does not persist it across reboots. Concrete failure:
  operator runs `install.sh` once, sees the firewall rules verified in the
  script's own printout, believes the boot-time gap is closed per the unit
  comment ("Enabled as a boot-time unit too… the rules go in once, right after
  docker.service") — but on the next reboot only the timer (now 5 s instead
  of the old 15 s) applies the rules, i.e. the "belt and brace" ADR-065
  follow-up (f) shipped only the brace. Fix: `run systemctl enable --now
  styrman-firewall.service` (mirroring the timer's line 110), or `enable`
  then a separate `start` if the ordering relative to `daemon-reload` matters.
  No ADR-067 amendment recording the boot-ordering measurement was found
  either (`.agents/docs/architecture/DECISIONS.md` tail has no 2026-09-10
  entry for this), so the plan's own required record of the DOCKER-USER
  measurement is also missing — low-severity companion to the same item.

- **[medium / confidence: high]** `internal/httpapi/check.go:145-146` (parser)
  and `:137` (tool label) — `tofu fmt -check` was run against
  `styrman-runtime:test` (has `tofu` v1.12.6 installed) on a deliberately
  misformatted `.tf` file: output was the bare filename `bad.tf` only, no
  `line:col:` at all (exit code 3). `reGCC` requires `^[^:]*:(\d+):(\d+):` and
  will never match a bare filename, so `parseCheckOutput`'s new `"tofu"` case
  (reusing `reGCC`) produces zero diagnostics for every `.tf` formatting
  issue — the UI will report `supported: true, diagnostics: []` for a file
  `tofu` itself flagged as needing reformatting. Plan K5 explicitly
  anticipated this ("if `tofu fmt -check` emits the same `file:line:col`
  shape... else a `'tofu'` case of its own, measured at implementation") — the
  measurement was not done, or was done and the wrong branch was taken. Fix:
  give `tofu` its own case, e.g. synthesizing one diagnostic per non-empty
  output line (`{Line: 1, Message: "not gofmt-formatted (tofu fmt -check): "
  + line}`), or switch to `tofu fmt -check -diff` and parse the unified diff.

- **[low / confidence: medium]** `internal/httpapi/check_test.go`,
  `internal/httpapi/format_test.go` — neither test file asserts the new
  `$HOME/.styrman/tmp` / `mkdir -p` script text that K4 introduced (grep for
  `styrman/tmp`, `mkdir -p`, `HOME` in both files returns nothing); the only
  overlapping assertion is the generic substring `"trap 'rm -f"`, which also
  matched the old workspace-relative script. Plan K4 said "do
  `check_test.go`/`format_test.go` assert the script text" — a regression
  back to a workspace-root temp (the exact bug K4 fixes) would not be caught
  by either suite. Not blocking on its own, but pair with the K5 finding
  above: two of Phase 3's five behavior changes ship without a test that
  would catch a silent regression.

## Criteria
1. Dead code — pass. No orphaned code introduced; `Persistent=true` deletion
   removes a genuinely inert line rather than leaving a commented remnant.
2. Flow tracing — fail on the two findings above: `install.sh` → the new
   `[Install]` section is written but never reached by any `enable` call;
   `checkerFor`'s new `"tofu"` case → `parseCheckOutput`'s reused `reGCC`
   branch does not agree with what `tofu fmt -check` actually emits.
3. Standards conformance — otherwise conforms; shell scripts still `set -eu`/
   `set -euo pipefail` correctly, Go changes are minimal and idiomatic.
4. Directory layout — `ensure-test-image.sh` lands beside `integration.sh` in
   `.agents/docs/testing/styrman/`, exactly as the plan specified. Fine.
5. Simplicity (DIY-except-security) — fine; no new dependency, stdlib/shell
   throughout; the dependency bumps are `go get`-driven version alignment,
   not new libraries.
6. Decoupling — fine; `ensure-test-image.sh` is a clean, single-purpose
   script sourced by both suites with no shared mutable state beyond the tag
   argument.

## Not examined
- Phase 3's remaining boxes not covered by the requested paragraphs (none
  outstanding per the dispatch's A/K/L/M scope).
- K2/K3 ("already closed") were not re-verified beyond trusting the plan's
  own note; not re-derived independently.
- `deploy/verify-commit.sh`'s divergence-list output was not run end-to-end
  (only the two underlying checks — `go mod verify` and root/ingress go.mod
  diffing — were run directly).
- The actual boot-time behavior of `styrman-firewall.service` under a real
  reboot (vs. reasoning from systemd semantics + reading `install.sh`) was
  not observed on a live host — no root access / no systemd PID 1 available
  in this environment. The `systemctl enable` gap is established from the
  script text and `man systemd.unit`'s `[Install]` semantics, not from a live
  `systemctl show -p After -p Before` per the plan's own e2e criterion for
  M(f), which is the operator's to run after `install.sh`.
- `web/e2e/run.sh`'s Playwright suite and `integration.sh`'s 82+ checks were
  not run in full (time budget); only the `ensure-test-image.sh` wiring point
  was exercised directly.
- Did not review `deploy/caddy/Dockerfile`'s digest pins for currency (out of
  this dispatch's A/K/L/M scope).

## Response (main, 2026-09-10)

- **Finding 1 (high) — fixed.** `install.sh` now `systemctl enable`s
  `styrman-firewall.service` beside the timer, so the `[Install]` section
  takes effect on the next `install.sh` run (the operator's, root).
- **Finding 2 (medium) — fixed.** `tofu` has its own parser case: a bare
  file name becomes one warning at line 1 ("not formatted — run `tofu
  fmt`"), an `Error:` line one error with its message, tofu's box-drawn
  detail lines are skipped. `TestParseCheckOutputTofu` covers clean, not
  formatted, and a parse error.
- **Finding 3 (low) — fixed.** `TestScriptsKeepTempFilesOutOfTheWorkspace`
  asserts the `~/.styrman/tmp` temp dir in both scripts, the absence of
  `mktemp -p .`, and that each trap names only files it creates.
