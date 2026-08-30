# Per-project backup & restore + create-from-backup (v1.13.0)

Date: 2026-08-30. Operator request: "Maybe we should add the backup button
here as well? … Full project lifecycle maybe plus restore from backup?"
Approved placement: Backup in the project's sidebar ⋯ menu (lifecycle),
restore/delete in Settings → Backups, "From backup" in the create dialog.
This is the USER layer; whole-state host snapshots (TODO) stay the
disaster-recovery layer — R13 updated accordingly.

## What shipped

- **`internal/backup` (new)**: tar.gz create/extract of `workspace/` +
  `home/` prefixes. `~/.dockerd` excluded (reproducible cache, GBs).
  Live-tolerant create (vanished files skipped, short reads padded).
  Defensive extract: absolute/`..` paths rejected, only files/dirs/symlinks
  created, ownership restored best-effort (`Lchown` from tar headers).
- **API**: `POST/GET /api/projects/{id}/backups`,
  `POST .../backups/{bid}/restore` (stopped projects only; a `restoring`
  guard 409s every boot path until it settles),
  `GET /api/settings/backups` (all, incl. deleted projects' backups),
  `DELETE /api/settings/backups/{pid}/{bid}` (raw ids — the project record
  may be gone), and `fromBackup: {project, id}` on `POST /api/projects`
  (seeds workspace + home + the source's project env vars before boot).
  Storage: `StateDir/backups/<user>/<projectID>/<stamp>.tar.gz` + meta.
- **Transactional restore**: the current state is QUARANTINED by rename
  (never deleted first) and renamed back if extraction fails. Two E2E
  failures drove this: (1) the naive version cleared the workspace, then
  died on a root-owned dockerd mount stub, leaving the project empty;
  (2) renaming a DIRECTORY to a new parent rewrites its `..` entry and
  needs write ON the dir — impossible for root-owned entries under the
  unprivileged (uid 1000) server. Fix: `orchestrator.RootRename` /
  `RootRemoveContents` run `mv`/`find -delete` as root inside the SERVER'S
  OWN container (`docker exec -u 0` on SelfContainer — no privilege the
  socket-holding server doesn't already have), both path-guarded to
  StateDir. Wipe-delete gained the same root-assisted fallback (pre-v1.11.1
  homes carry root-owned stubs in production too, since the hardened server
  runs unprivileged).
- **UI**: sidebar ⋯ menu → Backup (toast with size); Settings → Backups
  (restore — disabled while the project runs — and delete, "(deleted)"
  badge for orphaned backups); create dialog → "From backup" option with a
  backup picker.

## Verification (live on crew, v1.13.0)

Unit: backup round-trip (content, modes, symlinks, `.dockerd` exclusion,
missing-source tolerance) + hostile-archive traversal rejection; all 14 Go
package suites green; tsc/oxlint/web build clean. Live E2E on the operator's
"test" project: marker planted → backup (300 MB, ~11 s) → marker corrupted →
restore-while-running 409 → stop → restore (3 s) → marker back to ORIGINAL,
trash fully cleaned, git + dotfiles intact → project started again →
create-from-backup booted a clone with the seeded workspace + home →
wipe-delete removed the clone's record + both dirs → cleanup left zero
residue (marker removed, backup deleted, empty backups list).
