# Incident — production state loss: /var/lib/styrman emptied, /etc/styrman deleted

Date: 2026-09-01. Occurred during work on the tag-gated auto-updater
(ADR-050/ADR-051), before v1.16.0 was tagged. This is an operational
incident record, not a code change — no files were edited to produce this
entry.

## What happened

- `/var/lib/styrman` — the entire production state directory — was found
  **empty and root-owned**, recreated at 15:03. It held every project home,
  every workspace, the per-user JSON stores, and all in-app per-project
  backups. The server then crash-looped with `mkdir
  /var/lib/styrman/homes: permission denied`, because it runs as the
  operator uid and no longer owned the directory.
- `/etc/styrman` was **deleted outright** by a `sudo rm -rf /etc/styrman`
  issued as test cleanup while exercising the new updater. That directory
  held the disaster-recovery secrets: `backup-sa.json` (the GCS
  service-account key) and `restic-pw` (the restic repository password).
- The exact command that emptied `/var/lib/styrman` was **not identified**.
  It happened while privileged updater tests and parallel verification
  agents were running against paths adjacent to production state. Recorded
  honestly: no specific command was proven responsible.

## Damage assessment

- Recovery from the still-running containers was investigated and
  **failed**. The Kata mounts still referenced the deleted directories
  (shown as `//deleted` in `/proc/<pid>/mountinfo`), but reading through
  them returned empty, 0 bytes. The hope that "the data is still held open
  by running processes" did not hold.
- The daily restic backup to GCS had run successfully at 03:00 that
  morning, so a snapshot existed — but the credentials needed to read it
  had just been deleted along with the rest of `/etc/styrman`. The restic
  password was also kept, separately, in the operator's password manager;
  that out-of-band copy is the *only* reason the bucket is not permanently
  unreadable.
- Two operator projects (`test3321`, `asdasd`) and one abandoned e2e test
  project were lost.

## Resolution

The operator judged the platform to still be in prototyping and asked for
a clean first-time state rather than a restore. All project containers and
per-project networks were removed; `/var/lib/styrman` was chowned back to
the operator uid; `/etc/styrman` was recreated root-owned `0700` but
**empty**; the server was restarted and came up healthy on v1.16.0 with 0
projects. A later verification pass confirmed the fresh install genuinely
works (harness seeds, `/check` returns diagnostics, `/format` returns real
content — see `.agents/docs/changes/2026-09-01-audit-v1.16.0.md`).

## Still outstanding — this is an open action, not a closed one

`/etc/styrman/backup-sa.json` and `/etc/styrman/restic-pw` have **not**
been re-provisioned. Until they are, the daily 03:00
`styrman-backup.timer` **will fail**. The GCS bucket contents themselves
are unharmed — only the local credentials to reach them are gone. Tracked
in `TODO.md`.

## Lessons

1. **Privileged tests must be confined to a scratch prefix.** The
   updater's default state path pointed under `/var/lib/styrman`, and
   tests were run as root without always overriding it — an unreleased
   feature was exercised on the same host, against paths overlapping,
   production state.
2. **A cleanup step must never `rm -rf` a directory without establishing
   what else lives there.** `/etc/styrman` was created for the DR secrets
   long before the updater existed, and was treated in the test cleanup as
   though the updater owned it exclusively.
3. **The DR layer's own keys lived only on the host they protect.** A
   backup whose credentials share a fate with the data it protects is not
   a backup. Storing the restic password out-of-band (password manager) is
   what kept this recoverable in principle — that redundancy is what
   worked, not the on-host copy.
4. Two independent audit findings recorded the same day compound this: the
   DR script's comment names `GOOGLE_PROJECT` while the code exports
   `GOOGLE_PROJECT_ID`, and with no `backup.env` a fresh host silently
   targets the *original* deployment's bucket
   (`deploy/backup/styrman-host-backup:17`) — so a naive re-provision could
   write into the wrong repository. Cross-reference
   `.agents/docs/reviews/2026-09-01-audit-whole-project.md`.

## Verification

n/a — this entry records an incident and its resolution, not a tested
change. The fresh install's health was independently confirmed by the
same-day whole-project audit (`.agents/docs/changes/2026-09-01-audit-v1.16.0.md`,
"Live fresh-install check on the running server"). The two re-provisioning
actions and the scratch-prefix test convention are tracked as open items in
`TODO.md`; the scratch-prefix convention is also recorded as
`ADR-052` in `.agents/docs/architecture/DECISIONS.md`.
