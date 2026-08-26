# 2026-08-26 — UI onboarding via hidden setup microVM

## Summary

Operator direction: the login prestep belongs in the UI, backed by a
temporary VM that is hidden from the project list, with results stored in the
home folder. Implemented exactly that: `GET /api/setup/status` probes the
shared home host-side; "Open setup" boots `styrman-sys-setup` (label
`styrman.system=setup`, no project label → invisible everywhere) in a new
entrypoint "minimal mode" (no workspace, no DinD → ~1.2 s boot); `/ws/setup`
attaches a terminal running `styrman-setup` then a shell; "Mark setup done"
writes `~/.styrman/setup-done` and destroys the VM. Banner + status
checklist added to the dev console; Phase 2 inherits the same API.

## Files changed

- `image/entrypoint.sh` — STYRMAN_MINIMAL mode (skip workspace + DinD)
- `internal/config` — LabelSystem, SetupContainer/SetupNetwork
- `internal/orchestrator` — createSpec refactor (shared by project + system
  VMs), EnsureSetup/SetupStatus/DestroySetup
- `internal/terminal/bridge.go` — extracted shared serve() pump; ServeSetup
- `internal/httpapi/setup.go` — status/start/done/discard endpoints
- `internal/httpapi/devpage/` — setup banner, checklist, setup terminal
- Specs: api.md (Setup section), guide.md (first-use walkthrough)

## Failures / gotchas during the work

- None functional this round; the ExitCodeWait/teardown work from the review
  pass carried straight over to the setup terminal via the serve() refactor.
- Testing wrote the setup-done marker on the real install; removed it so the
  operator still sees the banner (test isolation note: the flow test ran
  against the live server on purpose — the integration suite covers projects,
  not setup, and a setup-flow section should be added to it).

## Verification

- Full flow exercised live: status → start (1.2 s) → hidden from
  /api/projects → wsprobe shows the guided script with correct ✔/➜ per
  credential → done (204) → container+network removed, marker written
- Integration suite re-run: 18/18
