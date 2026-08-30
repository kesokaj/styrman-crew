# Setup terminal: clean Ctrl+C exit + targeted per-tool sign-in / harvest-without-teardown (v1.10.0)

Date: 2026-08-30. Two operator-facing problems with first-run setup, fixed
together and shipped as v1.10.0.

## 1. Ctrl+C in the setup terminal restarted `styrman-setup` instead of exiting

**Root cause:** SIGINT killed the bash script (a signaled exit, not a normal
one). The tmux session then ended, but the browser terminal treated the
socket close as transient and reconnected; `tmux new-session -A` (ADR-002's
attach-or-create pattern) recreated the already-ended session, so
`styrman-setup` ran again from the top — Ctrl+C looked like a restart loop
rather than an exit.

**Fix (`image/configs/styrman-setup`):** added
`trap 'printf "\n"; exit 0' INT` so Ctrl+C now exits cleanly (exit 0). The
pane ends, the bridge's close-4000 signal fires, and the UI finalizes
instead of the client reconnecting into a recreated session.

## 2. Adding one credential after setup required rerunning the whole flow

Previously the only way to add a single later credential (e.g. signing into
gcloud after the fact) was to re-run the complete onboarding flow; the
greyed-out credential rows in Settings weren't actionable.

**Fix, four parts:**

- **`image/configs/styrman-setup` refactor.** Split into per-tool functions
  behind a dispatch: `styrman-setup` with no args still runs the full flow
  unchanged; `styrman-setup <tool>...` (`git|ssh|gh|gcloud|claude|agy`) runs
  only that tool's step, ensuring git identity first when the target isn't
  `git`/`ssh` itself.
- **`internal/terminal/bridge.go` (`ServeSetup`).** `GET /ws/setup` now
  accepts an optional `?tool=` query param. Because that value is
  interpolated into the shell command, it is strictly allowlisted through a
  new `setupTool()` helper — only the six known tool names pass through;
  anything else (including shell metacharacters, spaces, or command
  substitution attempts) resolves to `""` (full flow). Targeted runs get
  their own tmux session, `styrman-setup-<tool>`, separate from the main
  `styrman-setup` session. New test `internal/terminal/setuptool_test.go`
  (`TestSetupToolAllowlist`) covers both the six valid tool names and a set
  of hostile inputs.
- **`internal/httpapi/setup.go` + `server.go`.** New
  `POST /api/setup/harvest` seeds the credential broker from the RUNNING
  setup VM without marking setup done and without tearing the VM down —
  returns the per-tool harvest report. The extraction logic shared with
  `finishSetup` (ADR-034) was pulled into a common `seedBrokerFromSetup(user)`
  helper used by both `finishSetup` and the new `harvestSetup`.
- **Frontend.** `web/src/lib/api.ts` gained `setupHarvest`, `setupDiscard`,
  and a `tool` param on `termSocketUrl`. `web/src/components/TerminalPane.tsx`'s
  setup `TermTarget` variant now carries an optional `tool`, included in the
  pane's reconnect key. `web/src/routes/settings.tsx`: the grey/red
  credential rows are now clickable, opening a new `ManageCredsModal` that
  boots the setup VM once, runs `styrman-setup <tool>` in a lazy-loaded
  embedded terminal, harvests on each tool's pane exit while keeping the VM
  up, refreshes the credential rows, and tears the VM down only on "Done".
  `TerminalPane` is lazy-loaded from the settings route so xterm.js stays out
  of the settings bundle. `web/src/routes/setup.tsx`'s `onExited` handler is
  now status-aware: it finalizes with `POST /api/setup/done` only when git
  and ssh are both present, otherwise it calls `DELETE /api/setup` and leaves
  the setup banner up — an incomplete setup (e.g. Ctrl+C before git/ssh) is
  never marked done.

## Verification

- `go build ./...` clean.
- `go test ./internal/httpapi ./internal/terminal ./internal/gitrepo` green,
  including the new `TestSetupToolAllowlist`.
- Frontend: `tsc --noEmit` clean, `oxlint` clean, `npm run build` OK
  (`TerminalPane` split out into its own lazy chunk).
- Deployed: `styrman-runtime:dev` and `styrman-server` rebuilt
  (`STYRMAN_VERSION=1.10.0`), server recreated and healthy (logs report
  version 1.10.0); the stale setup VM was recycled so it now serves the new
  arg-aware script. The two running project VMs were left untouched.
  `crew.styrman.xyz` returns the expected OAuth redirect (302 →
  `auth.styrman.xyz`).
- Verified in the rebuilt image: `styrman-setup` carries the SIGINT trap plus
  the `run_one` dispatch; `bash -n` is clean; `styrman-setup agy` runs the
  targeted dispatch. Auth'd probe: `POST /api/setup/harvest` → HTTP 200.

## Release

Shipped live as styrman-server v1.10.0 alongside a rebuilt
`styrman-runtime:dev`.
