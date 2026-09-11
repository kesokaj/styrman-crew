---
owner: reviewer
date: 2026-09-11
---
# Cold-read review — Browser view plan (2026-09-11)

Document reviewed: `.agents/docs/plans/2026-09-11-browser-view.md`, plus the
code surfaces it names (`internal/orchestrator/{orchestrator,ports,exec}.go`,
`internal/httpapi/{proxy,shares,links_api,explorer,server}.go`,
`internal/config/config.go`, `internal/terminal/bridge.go`,
`image/entrypoint.sh`, `REQUIREMENTS.md`, `.agents/docs/architecture/DECISIONS.md`
ADR-062, `web/src/lib/editorPane.ts`, `web/src/routes/p.$project.$term.tsx`).

This is an unusually well-grounded plan — most cited line numbers and
patterns (the `editorUsed`/`diffUsed` hidden-not-unmounted latch, the
`serveProxy`/`IPAddress` "" = not running convention, `sharePort`'s existing
1024–65535 validation) check out exactly against the real code. The gaps
below are real but narrow.

## (1) Gaps — undecided before implementation

- **`ErrNotRunning` detection mechanism is unspecified.** `BrowserStart/Stop/Status`
  (plan lines 197–200) are said to return `ErrNotRunning` for a stopped
  project, but the plan never says how that's detected. `internal/orchestrator/exec.go:148`
  (`Exec`) returns only a generically wrapped Docker error
  (`"exec create: %w"`) on `ContainerExecCreate` — it does not classify
  "not running" vs. any other Docker failure. The codebase's own idiom for
  this exact question already exists and is used two functions away:
  `ListeningPorts` (`internal/orchestrator/ports.go:64-71`) calls
  `o.Status(ctx, user, project)` first and checks `st.State != "running"`
  before ever calling `Exec`. The plan should say "call `Status` first, as
  `ListeningPorts` does" — otherwise an implementer is equally likely to
  parse the Exec error string, which would silently misclassify real Docker
  failures (exec-create timeouts, daemon errors) as "not running" → wrong
  409/4002 instead of a 500/1011, or vice versa.
- **The `1012` WS close path (row 327 of the failure-mode table) has no
  defined constant and no test.** Every other close code in this codebase is
  a named constant in `internal/config/config.go:58-71` with a comment
  dictating client reconnect behavior (`CloseShellExited`, `CloseProjectStopped`,
  `CloseAccessRevoked`, `CloseReplaced`), and the "Surfaces touched" table
  (plan line 91) only adds `CloseBrowserUnavailable = 4004` — `1012` is a
  bare magic number appearing only in the failure-mode table, used nowhere
  else in the plan's Interfaces section (§ Server — `internal/httpapi/browser.go`,
  step 2, only names 4002 for "not running"). The Phase 2 verify line
  (line 388) commits to testing "4002 / 4004 / 1011" — `1012` is not in that
  list. Left as-is, this is an untested, unspecified failure path invented
  fresh for this feature (the terminal bridge, `internal/terminal/bridge.go`,
  never calls `EnsureSelfAttached` at all — Exec goes over the Docker socket,
  not the bridge network — so there is no existing WS-plus-attach precedent
  to copy). Before Phase 2: add the constant, decide the exact trigger
  (`EnsureSelfAttached` returns `fresh=true` *after* `Accept`, since a WS
  upgrade can't be HTTP-redirected the way `serveProxy` redirects a plain
  proxy request — worth stating explicitly, since it's the reason 1012 exists
  instead of reusing `serveProxy`'s redirect), and add it to the unit-test list.
- **Combined stamp-gating logic for the third entrypoint step is not spelled
  out.** `image/entrypoint.sh:127-150` currently gates the harness-version
  stamp write on `HM_OK=1 && HI_OK=1` for exactly two steps. The plan adds a
  third step (`claude mcp add` + `agy mcp add`) "both required for the stamp
  to be written" (line 171) but doesn't say whether that's a third `MO_OK`
  variable ANDed in, whether `claude`'s and `agy`'s registration are graded
  independently or as one unit, or whether a `claude mcp remove ... || true`
  failure (which is swallowed) can ever fail the gate at all. Minor — an
  implementer can pattern-match the existing `HM_OK`/`HI_OK` style
  correctly — but it's the exact mechanism BUG-035 (`.agents/rules/mechanism-reaches-every-caller.md`)
  is about: a stamp gate that silently drops one of three required
  conditions reads as "done" when the MCP step failed. Worth one explicit
  line before Phase 1.

## (2) Readiness — questions a first-pass implementer would still have to ask

1. In `internal/orchestrator/browser.go`, do `BrowserStart`/`Stop`/`Status`
   call `o.Status` first (matching `ListeningPorts`) or classify the `Exec`
   error, to produce `ErrNotRunning`?
2. What is the named constant and exact trigger condition for the `1012`
   close code, and should it be added to the Phase 2 unit-test list
   alongside 4002/4004/1011?
3. What exactly gates the entrypoint stamp write once a third step (MCP
   registration, two sub-commands) is added — one combined flag, or are
   `claude` and `agy` registration graded separately, and does the swallowed
   `mcp remove` failure ever count?

## Verdict

`design needs revision` — three gaps above, all narrow and locally
resolvable, but the `ErrNotRunning` detection point is a genuine correctness
question (guessing wrong misclassifies real Docker failures as "project
stopped" or vice versa) that the plan should pin down before Phase 2 starts,
and the untested 1012 path is exactly the shape of thing that ships as a
silent gap in coverage.

Not `implementation not ready` in the strong sense — nothing here blocks
starting Phase 1 (image work), and questions 1–3 only bite in Phase 2. But
answering them now avoids a mid-phase pause.

## Not examined

- `image/Dockerfile`'s actual apt/npm steps (only grepped for existing port
  usage), `image/configs/*` (none of the referenced configs exist yet — new
  files, nothing to cross-check).
- `web/src/lib/api.ts`, `web/src/components/{BrowserPane,SidePanelPorts}.tsx`,
  `web/e2e/run.mjs`, `.agents/docs/testing/styrman/integration.sh` — Phase 3/4
  surfaces, not read against the plan's claims in this pass.
- `coder/websocket` library semantics (read-limit behavior, close-frame
  120-byte reason truncation) — took the plan's claims at face value.
- Chromium sandboxing under Kata (Phase 1's own open risk, explicitly
  deferred to a probe task in the plan itself).
