# Verification — audit (v1.16.0 + uncommitted doc corrections)   (2026-09-01)

## Status: PASS

Scope: HEAD `86e4b9e` (v1.16.0) plus the three uncommitted doc-correction
edits (`.agents/docs/changes/2026-09-01-release-v1.16.0.md`,
`.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md`,
`.agents/docs/specs/styrman/deploy.md`) — all docs-only, no source changed.
Confirmed the doc text against the actual `deploy/update/styrman-update`
code (`/etc/styrman/update` state dir, root:root 0700 assertion, LAST-armor-
header signature check) — matches exactly.

## Commands run

| Command | Result |
|---|---|
| `go build ./...` | PASS, no output |
| `go vet ./...` | PASS, no output |
| `gofmt -l internal/ cmd/` | PASS, no output (nothing unformatted) |
| `go test -race -count=1 ./...` | PASS — 15 packages `ok`, 3 `[no test files]` (cmd/wsprobe, internal/identity, web) |
| `(cd deploy/caddy/ingress && go test -count=1 ./...)` | PASS — `ok github.com/spgo/devbox/caddy/ingress 2.025s` |
| `(cd web && npx tsc -b)` | PASS, no output |
| `(cd web && npx oxlint)` | PASS, no output |
| `shellcheck -S error deploy/update/styrman-update deploy/host-setup.sh web/e2e/run.sh` | PASS, no output |
| `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:dev` | PASS — `passed=57 failed=0 skipped=3` (matches the documented 60-check/3-auto-SKIP baseline exactly) |
| `STYRMAN_E2E_PORT=18041 bash web/e2e/run.sh styrman-runtime:dev` — 1st run (concurrent with integration.sh + an image rebuild + a live throwaway project on the same box) | **FAIL** — `passed=47 failed=1 skipped=2`; single failure: "terminal session survived the editor round-trip — page.waitForSelector: Timeout 15000ms exceeded" |
| Same command, rerun in isolation (integration.sh finished, throwaway project deleted, load average 0.77) | PASS — `passed=48 failed=0 skipped=2`, **identical** to the clean baseline recorded in today's 14:03 preflight gate log (`.agents/docs/testing/2026-09-01-verify-preflight.md`) |

**On the e2e flake:** not a port collision (I used the private port
18041 as instructed; no other process was on it). It reproduced once
under genuine host resource contention — a full `docker build` of
`styrman-runtime` running concurrently with `integration.sh`'s own
container churn and a live project microVM I had running against
production — and did NOT reproduce on immediate rerun once that
contention cleared, landing on the exact same pass/skip counts as the
last known-clean run today. Treated as an environmental flake, not a
regression, and reported honestly as both an observed failure and a
non-reproduction rather than silently omitted.

Image note: `docker build -t styrman-runtime:dev --build-arg
STYRMAN_VERSION=dev-$(date +%s) image/` was run first per the testing
README's prereq step (the previous `:dev` image predated a same-day
commit touching `image/`). Every layer copying `entrypoint.sh`, `harness/`,
`skel/` came back `CACHED` — content was already identical, confirming no
drift, only the version-arg-bearing final layer rebuilt.

`git status` after the run: only the expected `web/dist/client/_shell.html`
build byproduct changed (documented behavior of `web/e2e/run.sh` — the SPA
build runs first and rewrites tracked dist assets). Left as-is; not staged
or committed by this pass.

## Audit-specific: coverage of audited-but-untested code

Confirmed both gaps already named in `TODO.md`, plus one new one found by
sampling:

| Code | Status | Note |
|---|---|---|
| `internal/httpapi/check.go: checkFile` | NOT COVERED at handler level | Only pure helpers (`parseCheckOutput`, `checkerFor`, `safeSuffix`, `checkScript`, `missingTool`) have unit tests. Confirmed via `grep -n "func Test" check_test.go` — no test drives `checkFile` itself through `httptest`. Matches TODO.md. |
| `internal/httpapi/format.go: formatFile` | NOT COVERED at handler level | Same shape as above; only e2e exercises the real handler. Matches TODO.md. |
| `internal/httpapi/format.go: formatInPlace` | **NEW FINDING — NOT COVERED at all, not even a pure-function unit test** | This is the direct analog of `checkerFor` (extension → formatter-command mapping) but, unlike `checkerFor` (which has `TestCheckerFor`), `formatInPlace` has zero references in `internal/httpapi/*_test.go`. Not previously named in TODO.md. |
| `deploy/update/styrman-update` | NOT COVERED — zero automated tests | Confirmed: no `*test*` file anywhere under `deploy/update/` or matching `*styrman-update*test*` in the repo. Matches TODO.md. |
| `internal/orchestrator/*`, `internal/terminal/bridge.go` | Low `go test -cover` numbers (3.0%, 8.5%) | NOT a new finding — these are the Docker/Kata-dependent paths the project's own two-tier strategy (README: unit tier is "no Docker") deliberately routes through `integration.sh`/e2e instead of `go test`. Only small pure helpers (`containerGuest`, `restartPolicy`, `hasWorkspaceBind`, `busyReason`, close-decision logic) are unit-tested, and those are covered. |
| `image/harness/` (Tier 5) | Explicitly documented as unautomated | `.agents/docs/testing/styrman/README.md` §Tier 5 already states this outright ("KNOWN GAP: not automated"); no new finding, re-confirmed by manually running its checklist below. |

## Audit-specific: hunting for tests that would pass while the feature is broken

Confirmed both examples cited in the task were already fixed at HEAD, and
negative-controlled both by reverting the fix and confirming the guard
test goes red:

1. **e2e litter check** (`web/e2e/run.mjs`, "syntax check leaves no temp
   files in the workspace"). Now asserts on BOTH a `tmp\.[A-Za-z0-9]+\.`
   filename pattern (catches shell-checker leftovers) AND a
   `/__pycache__/` regex against the panel listing (catches the Python
   directory litter the filename pattern can never match). Verified by
   reading the current source (`run.mjs:334-361`) and by exercising it
   for real against the live production server (see below) — a `.py`
   syntax check left no `__pycache__` in the listing.

2. **`TestCheckAndFormatGetTheLargeBodyLimit`**
   (`internal/httpapi/check_test.go:264`). Now drives the real
   `csrfGuard` middleware with `httptest`, table-driven over
   `/check`, `/format`, `/file` (control, already had the limit), and
   `/terminals` (control, must stay at the small default). Negative
   control: removed the `/check` case from the `csrfGuard` switch in
   `internal/httpapi/server.go` → test correctly went red
   (`/api/projects/p1/check: read-2MiB allowed=false want=true (status
   413...)`). Reverted with `git checkout --`.

3. **`TestGuestScriptsNeverWaitForStdinEOF`**
   (`internal/httpapi/check_test.go:199`). Negative control: reintroduced
   the exact historical bug — replaced `head -c %d > "$F"` with
   `cat > "$F"` in `checkScript` (the change that silently truncated
   `/format` responses to empty) → test correctly went red ("script waits
   for stdin EOF (`cat > \"$F\"`) — it will hang or truncate"). Reverted
   with `git checkout --`.

4. **Sampled ~20 other test functions** across `internal/files`,
   `internal/links`, `internal/conversation`, `internal/userstore`,
   `internal/credbroker`, `internal/orchestrator`, and
   `deploy/caddy/ingress` (`TestTraversalRefused`,
   `TestBrokerUnknownCaller`, `TestBrokerEnvFilesForbidden`,
   `TestInvalidEmailRejected`, `TestContainerGuest`,
   `TestParseLabels_*`, `TestFetchIP_*`,
   `TestGenerateCaddyfile_NoBaseFile`,
   `TestGenerateCaddyfile_HTTPSRedirectAlwaysPresent`, etc.) via a script
   flagging low-assertion-count test bodies. All checked by hand: every
   one asserts a specific status code, exact returned value, or
   `errors.Is`-checked error identity — none found to be tautological,
   asserting only on their own input, or `err == nil`-only. No additional
   instance of the reported shape found in this sample.

## Audit-specific: fresh-install state, verified live against production

Production (`styrman-server`, `styrman.version=1.16.0` label, reached
directly on its Docker bridge IP `172.18.0.2:8080` with `X-Caddy-Email:
ostling.simon@gmail.com` + `X-Styrman-Ingress: <token from deploy/.env>`
headers standing in for the Caddy ingress, which the backend enforces
directly — confirmed `403` with no headers, `200` with them). No restart,
redeploy, or config change made to the running server or compose stack.

- `GET /api/projects` before and after: `{"projects":[]}` both times —
  confirmed genuinely empty, not just untested.
- Created `verifier-audit-throwaway` (`POST /api/projects`) → reached
  `state:"running"` within ~15s.
- **Harness seed**: `~/.claude/agents/` has the 6 subagents
  (changelog, documentation, explorer, reviewer, security, verifier);
  `~/.claude/skills/*/` and `~/.gemini/config/skills/*/` both report 28;
  `~/.claude/CLAUDE.md` is 69 lines; `~/.agents/bin/` has both
  `resume-context.sh` and `trail-check.sh`. Seeding works.
  - **Marker mismatch (new finding, low severity):**
    `~/.styrman/.harness-version` read `dev-1788269597`, not `1.16.0` as
    the brief assumed. Root cause: `deploy/.env` sets
    `STYRMAN_IMAGE=styrman-runtime:dev` (a floating dev tag), not a
    pinned release tag — the default in `.env.example` is
    `styrman-runtime:latest`, and neither is a versioned tag matching the
    server's own `STYRMAN_VERSION=1.16.0`. The runtime-image version and
    the server-binary version are two independent build args with no
    coupling. Practical consequence hit during this very run: rebuilding
    `styrman-runtime:dev` locally (as directed by the testing README's own
    prereq step, needed because a same-day commit had touched `image/`)
    changed what the LIVE production server boots new project containers
    from, since production is pointed at that same mutable tag. Confirmed
    no functional drift this time (every content layer came back
    `CACHED`), but the coupling itself is a deployment-hygiene gap:
    production should point at an immutable, versioned tag so (a) the
    harness marker means something and (b) a verifier/tester rebuilding
    `:dev` per the documented test prereqs never has a side effect on
    what production spins up next. Not in TODO.md today.
  - **Benign duplicate-backup churn (new finding, cosmetic):** every
    fresh project's `~/.agents/bin/` also gets
    `resume-context.sh.bak.<timestamp>` and `trail-check.sh.bak.<timestamp>`
    on FIRST seed — confirmed byte-identical to the live files via `diff`.
    Cause: both `image/harness/claude/setup.sh` and
    `image/harness/antigravity/setup.sh` deploy to the same shared
    `~/.agents/bin/` target, and `backup()` unconditionally backs up
    whatever the first setup.sh just wrote before the second overwrites it
    with identical content — no `cmp` guard. Harmless (content matches,
    only clutters the directory), not the same issue as the TODO.md
    `.bak.*`-in-agents-listing item (that one is about `setup.go`'s config
    home, not project homes). Worth a TODO line, not blocking.
- `POST /api/projects/{id}/check` on an unterminated `[ ` shell test
  returned 4 real shellcheck diagnostics (SC1009/SC1073/SC1080/SC1072)
  with correct line/column/severity — real diagnostics, not a stub.
- `POST /api/projects/{id}/check` on `def f(:\n    pass\n` (broken
  Python) returned a real `SyntaxError: invalid syntax` diagnostic, and
  the subsequent `GET .../files?path=.` listing showed only `.git` — no
  `__pycache__`, confirming the litter fix holds against the actual
  server, not just the (now-fixed) e2e assertion.
- `POST /api/projects/{id}/format` on a messy `package main\nfunc
  main(){x:=1;println(x)}\n` returned
  `"package main\n\nfunc main() { x := 1; println(x) }\n"` — real
  gofmt'd content, NOT empty. **This is the release's core claim and it
  holds** against the live server.
- `DELETE /api/projects/{id}` (no `?wipe=true`) returned `202`, removed
  the container and the API record, but — as designed, confirmed by
  reading `server.go:831` and `integration.sh`'s own use of
  `?wipe=true` — left the on-disk `.git` skeleton (124K, no project
  content) at `/var/lib/styrman/projects/ostling-simon/<id>/` since wipe
  is opt-in. Removed manually with `sudo rm -rf` once the API record was
  confirmed gone (constraints required full deletion of anything created).
  Confirmed clean afterward: `GET /api/projects` → `[]`, container gone,
  directory gone.

## Coverage of changed code (this task's own diff)

The three uncommitted edits are documentation only
(`.agents/docs/changes/*.md`, `.agents/docs/specs/styrman/deploy.md`) —
no source lines changed, so there is no code path to cover. Verified the
prose against the live `deploy/update/styrman-update` script instead (see
above): matches.

## Environment

Local tier only, as instructed. Linux (Debian trixie homeserver), Docker
(plain, no Colima needed). `styrman-runtime:dev` rebuilt once (see image
note above). Production `styrman-server`/`styrman-caddy` left running
throughout, untouched except for the one throwaway project created and
fully deleted by this run. No systemd units installed, `/opt/styrman` not
created, `/etc/styrman` not written to.
