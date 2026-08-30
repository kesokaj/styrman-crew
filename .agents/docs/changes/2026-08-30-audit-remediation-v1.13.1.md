# Audit: code/docs/decisions alignment + remediation (v1.13.1)

Date: 2026-08-30. Operator ran `/audit` after the v1.10.0→v1.13.0 release
burst. Method: 6 parallel audit dimensions (API spec, behavior specs,
decisions/REQUIREMENTS/TODO, Go quality, web quality, scripts/docs), every
red/yellow finding adversarially re-verified against the code by a second
agent. Result: 2 red, 11 yellow, ~13 green — all verified findings fixed in
this release (one yellow downgraded to green as unreachable, fixed anyway).

## Red (both fixed)

1. **Restore concurrency (backups.go)** — the `restoring` guard was a plain
   store set AFTER the running-check: two concurrent restores both passed,
   shared one `.restore-trash`, and each could destroy the other's rollback;
   a boot that had already passed its flag check could mount the home
   mid-extract (TOCTOU). Fixed: `tryAcquire` compare-and-set guards
   (restoring + a new backingUp single-flight), acquired FIRST; the whole
   stopped-check + wipe + extract now runs under the orchestrator's
   per-project lifecycle lock (`WithProjectLock` — the same lock
   EnsureRunning takes), so a boot either 409s on the flag or blocks and
   then boots the fully-restored tree. Concurrent restore/backup now 409.
   `deleteProject` gained the symmetric refusal while a restore is in
   flight. Test: `TestTryAcquireIsCompareAndSet`.
2. **20 s client timeout on long backup ops (api.ts)** — createBackup,
   restoreBackup, and createProject(fromBackup) ran under the default 20 s
   abort while the server works synchronously: false-failure toasts, and
   for create-from-backup the documented "project exists but dialog says
   failed → name already exists" dead-end. All three now use a 10-minute
   signal (same pattern as start/stop's 150 s).

## Yellow (all fixed)

- backups.go: same-second double-backup corrupted one shared archive
  (single-flight + `os.CreateTemp`); `fi, _ := os.Stat` nil-deref panic
  path (checked, size falls back to 0); rollback left archive-only partial
  extract entries behind (failed extract now purges all non-`.dockerd`
  entries before renaming originals back).
- settings.tsx: ManageCredsModal marked a Ctrl+C-cancelled step "signed in"
  and green — success is now decided by the harvest REPORT (per-tool
  "stored" prefix match), with an honest "nothing new signed in" otherwise;
  BackupsSection now disables EVERY restore button while one runs.
- A11y: ManageCredsModal, CredViewer and NewProjectDialog gained
  `role="dialog"`/`aria-modal`/labels and Escape-to-close (they had copied
  the pre-Dialog-shell pattern).
- Docs: deploy.md's stale STYRMAN_KEYRING_PW paragraph, runtime-image.md's
  "still installed" keyring packages, README stale at v1.9.1 (+56→60
  integration checks, quick-start pin), testing README count/coverage.

## Green (fixed where cheap, documented otherwise)

- backup.go Extract now rejects writes through symlink parent components
  (defense-in-depth; unreachable from Create — test
  `TestExtractRejectsSymlinkParent`).
- RootRename/RootRemoveContents guard widened to cover a configurable
  HomesDir outside StateDir (was fail-closed refusal).
- styrman-setup: unknown tool now exits 1 without the "Done" epilogue
  (benign keep-answers still exit 0).
- api.md: brokerEnabled field, history-search 50-file bound, stats
  zeroed-fields wording; terminal-protocol/guide: Ctrl+Shift+C copy;
  guide.md: scoped env vars are NOT inside backup archives (they copy from
  the live source project only). Stale keyring comments in bridge.go +
  explorer.go; bridge package doc now lists 4002. TODO.md: agent-memory
  checkpoint item marked shipped via ADR-044; stale auto-snapshot
  rationale repointed at ADR-042/044. api.ts: dead listBackups client
  method removed; SidePanelEnv no longer asserts an empty list before the
  first fetch resolves. .env.example: optional STYRMAN_BROKER_ADDR /
  STYRMAN_VERSION notes.

## Verification

- All 14 Go package suites green (2 new tests); tsc/oxlint/web build clean.
- Deployed v1.13.1 (runtime image + server rebuilt); crew 302 → OAuth gate.
- Live E2E of the hardened paths on the operator's test project: two
  concurrent backups → 200+409; stop; two concurrent restores → 200+409
  with the tree restored intact; start; backup deleted; zero residue.
