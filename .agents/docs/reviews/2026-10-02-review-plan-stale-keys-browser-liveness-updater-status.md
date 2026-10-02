---
owner: reviewer
date: 2026-10-02
---
# Review — plan-stale-keys-browser-liveness-updater-status, round 3   (2026-10-02)

Range reviewed: cold-reader check, round 3, of `.agents/docs/plans/2026-10-02-stale-keys-browser-liveness-updater-status.md` (HEAD b03bc91), from a fresh read of the plan and the code surfaces it names. No diff. Read: `deploy/update/styrman-update` (24-70, 205-345, 405-475, 500-570, 756-835), `deploy/update/styrman-update.service`, `deploy/styrman-ctl` (525-560, 640-682), `deploy/docker-compose.yml` (grep), `internal/httpapi/explorer.go:487-520`, `internal/httpapi/auth.go:85-100`, `internal/config/styrman_ctl_test.go:505-545`, test names in `internal/config/`, `web/e2e/run.mjs` (1148, 1215-1345, 1398-1510, 1570-1690), `web/e2e/run.sh` head, `web/src/components/TerminalPane.tsx:400-430`, `web/src/components/BrowserPane.tsx` (232-262, 376-400, greps), `web/src/lib/cdp.ts` (greps), `specs/styrman/terminal-protocol.md:180-230`, `DECISIONS.md` ADR numbering.

## Status: COMPLETE

## Verdict: APPROVE WITH NITS

design needs revision (three small gaps, none that makes two careful implementers build different features); implementation ready (zero BLOCKING questions). Every round-2 item re-checked against the current plan text is resolved: the criterion-1 fixture now uses terminal 2 (matches `run.mjs:1314,1369`; the suite page is on `/p/${PID}/1` at `:1545`), the `signature` code is set inside the failure branch with an `on_exit` precedence, `styrman-ctl`'s `UPDATER_ENV` (`:539`) is a named caller, plants use a current `checkedAt`, `since` is read with `sed`, and the pair is validated in one `case`. The worst remaining problem is a verify clause that can pass without running the code it names (finding 1).

## Findings (gaps)
- **[medium / confidence: high]** Plan, Phase 2 task 3, verify `go test ./internal/config/ -run 'TestUpdater|TestStyrmanCtl'` vs `internal/config/styrman_update_test.go:18,133` (tests are named `TestStyrmanUpdate…`, none starts `TestUpdater`) and the plan's "NEW file `internal/config/styrman_update_status_test.go`" whose test names are not given. The pattern `TestUpdater` matches no existing test, and `TestStyrmanCtl` always matches, so the command exits PASS even if the new file's tests are never selected or never exist; the clause "each new test fails before its code exists" then rests on the implementer's naming luck. Fix: name the new tests `TestStyrmanUpdateStatus*` and run `-run 'TestStyrmanUpdate|TestStyrmanCtl'`, or drop the `-run` filter.
- **[medium / confidence: medium]** Plan, "The page": every failing notice "ends 'This host stays on <deployed>.'" when `deployed` is non-empty, including `deploy-failed` ("did not come up and could not be rolled back"). In `styrman-update:826-835` the deployed-version state file is rewritten only on a SUCCESSFUL rollback, so after a failed rollback `deployed` still holds the previous tag while the box is running whatever the failed deploy left. The notice would assert the opposite of its own first sentence, on the one code where the operator most needs the truth. Fix: omit the sentence for `deploy-failed` (or word it "last recorded version <deployed>"), and add that case to `updaterNotice.test.ts`.
- **[low / confidence: medium]** Plan, `on_exit`/trap: the script runs under `set -euo pipefail` (`styrman-update:27`) and the plan installs `trap on_exit EXIT` at `:248`, above the root check, the `STATE_DIR` guards and the `DOCKER_CONFIG` case. `on_exit` reads `STATUS_ARMED`, `STATUS_RESULT`, `STATUS_CODE`, `STATUS_DEPLOYED`, `STATUS_LATEST` and `EXIT_RM_DIR`, none of which exist yet when `fail "must run as root"` (`:251`) or an ADR-052 refusal (`:256-277`) fires; an unset variable under `set -u` aborts the handler and can change the exit status. The plan does not say the globals are initialised before the trap, nor that `on_exit` captures `$?` first and is defined above the two new hooks (which sit at `:205-232`, before the trap line). A careful implementer will do it; the text-pin test should assert the initialisation, and one `__exit-status`-style case should run the refusal path (not armed → writes nothing, exit status unchanged).
- **[low / confidence: low]** Plan, Failure modes: `SIGTERM` from systemd's 45-minute `TimeoutStartSec` kill reaches `on_exit` only because bash runs the EXIT trap on a fatal signal; the status then reads `failing`/`error` (143). Probably the wanted outcome, but the plan decides it nowhere (default: leave as is). Not tested by any hook.

## Scope
Nothing built beyond `## Why` without a D-row. `stale`/`unknown` D11, the two hooks D3/ADR-052 shape, the shared `STYRMAN_UPDATE_STATUS_FILE` name D10, `styrman-ctl`'s `UPDATER_ENV` D14, the any-frame probe D12, the hidden-pane probe D15, the 256-entry cap is the existing `outbox.length < 256` (`TerminalPane.tsx:418`). `latest`, `deployed` and `since` are each consumed by a named sentence. No extra endpoint (the `updater` key rides `GET /api/settings`, `explorer.go:488`), no second shape. Acknowledged limit, not a gap: the host that motivated item 5 (BUG-084) shows nothing until it takes this release.

## Questions for the author
1. NON-BLOCKING — what are the new Go test names, and what `-run` pattern selects them? (default: `TestStyrmanUpdateStatus*`, pattern `TestStyrmanUpdate|TestStyrmanCtl`)
2. NON-BLOCKING — does `deploy-failed` carry "This host stays on <deployed>."? (default: no)
3. NON-BLOCKING — are the `STATUS_*` globals and `EXIT_RM_DIR` initialised before the trap, and is `on_exit` defined above the hooks? (default: yes to both)
4. NON-BLOCKING — is `failing`/`error` on a SIGTERM timeout intended? (default: yes)
5. NON-BLOCKING — the e2e `STATE` is `mktemp -d /tmp/styrman-e2e-XXXX` (`run.sh:18`) while the integration `STATE` is under `/var/tmp`; the plan only needs `${STATE}/status/updater.json`, which both give. No decision needed.

## Criteria
1. Dead code — pass: no unused piece; the one `case` default replaced the earlier fallback; the hand-run trap is moved, not duplicated.
2. Flow tracing — partial: the criterion-1 fixture (terminal 2, `installSocketFixture` extended past `__termSockets`' `/ws/term/` filter, `run.mjs:1275`), the probe → `reconnect()` path (`BrowserPane.tsx:235`, effect keyed on `epoch`), the `drain()` replacing `outbox.splice` (`TerminalPane.tsx:408-413`), the arm point after `flock` (`:336`) and every `fail`/`exit` site trace through. The `deploy-failed` sentence and the unset-variable path in `on_exit` do not.
3. Standards conformance — partial: describing-text list is grep-derived and names the lines (`never auto-reconnects` is real, `BrowserPane.tsx:26-31`); `mechanism-reaches-every-caller` honoured (unit, compose, `styrman-ctl`); `shared-path-learnings` honoured (fresh-host path stated as unexercised, "Learnings" section promised); `writing-files` honoured (new test file). The verify clause in finding 1 is the standards miss.
4. Directory layout — pass: `web/src/lib/`, `internal/updatestatus/`, `deploy/update/`, tests beside code.
5. Simplicity (DIY-except-security) — pass: shell and stdlib only, no dependency; the root-written file is checked JSON with no crypto or auth primitive.
6. Decoupling — pass: root writes, a read-only directory mount, the server only reads and re-validates, the page maps codes to its own sentences; `termOutbox.ts` and `CdpClient.alive` are testable seams.

## Not examined
`host_converge` and `deploy()` bodies (`styrman-update:575-755`) beyond the exit paths listed, so whether either can `fail` and thereby turn a good deploy into `failing`/`error` was not traced; `BrowserPane.tsx` bodies beyond `reconnect`, the connection effect's head and `onClose` (the hidden-pane claim of D15 was taken from lines 35-37, 210, 449-453 only); `cdp.ts` `handleMessage` body; `internal/httpapi` test helpers for a non-admin identity (one exists at `network_test.go:128`, shape not read); `integration.sh` plant mechanics beyond `STATE`/server start; `setVisibility` body (`run.mjs:1148`); `web/src/lib/api.ts` and `settings.tsx`; ADR-081/082 free-ness confirmed only by the last ADR being 080. The Docker bind-source claim in Risks was not re-run (no docker, as instructed). No tests were run.

design needs revision
implementation ready
