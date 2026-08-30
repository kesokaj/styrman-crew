# Automated backups, both layers: restic→GCS + in-app cadence & pre-wipe net (v1.14.0)

Date: 2026-08-30. Operator: "1-6 just go. Create the cheapest GCS bucket in
sweden region." Completes the backup story from ADR-044 (manual layer,
v1.13.0) and closes the 2026-08-29 volume-snapshot TODO (host is ext4
without LVM, so restic-to-GCS is the mechanism).

## Host layer (disaster recovery)

Bucket `gs://styrman-crew-backups-driven-eft-h4it` — europe-north2
(Stockholm), Nearline (cheapest practical: Coldline/Archive min-storage
penalties trigger on restic prunes; at ~1 GB state the delta is cents).
Dedicated SA `styrman-backup@…` with objectAdmin on that bucket only; key +
restic password root-only under /etc/styrman (password also handed to the
operator for their password manager). `styrman-host-backup` script +
systemd timer daily 03:00 (Persistent). Excludes reproducible docker disks
and StateDir/backups. Retention 7d/4w/6m + prune. Verified: first snapshot
(1.05 GiB) pushed AND a restore round-trip
(`restic restore --include …/users`) succeeded.

## In-app layer (per-project automation)

- `doBackup` extracted from the handler and shared by three callers: the
  API, a new `RunBackupCadence` scheduler (every STYRMAN_BACKUP_CADENCE,
  default 24h, 0=off; first pass ~1min after boot), and the pre-wipe net.
- Retention: after every backup the newest STYRMAN_BACKUP_KEEP (default 5)
  archives per project are kept, older ones pruned (meta first so a
  half-pruned pair never lists as restorable).
- Pre-wipe safety net: `DELETE ?wipe=true` snapshots the project FIRST and
  FAILS CLOSED — if the safety backup fails, nothing is deleted and the
  record carries the error. Answers the operator's open question
  ("auto-backup before delete?") with yes.
- Transcript janitor (Phase 1, same release): per-project ~/.styrman/logs
  files older than STYRMAN_TRANSCRIPT_TTL (720h default) pruned 6-hourly.

## Verification (live)

Deployed v1.14.0: cadence first pass backed up both projects (~300 MB each);
a throwaway project's wipe-delete auto-created backup 20260830T133137Z which
SURVIVED the deletion (listed under Settings → Backups as "(deleted)",
restorable via create-from-backup) — then cleaned up. All 14 Go suites
green; compose/env/api.md/guide.md/deploy.md updated in the same commits.
