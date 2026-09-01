---
name: project-styrman-docs-backup-removal-v1.17.0
description: 2026-09-01 (sixth pass) doc sync — whole backup feature removed, auto-updater git auth switched deploy-key→gh-token
metadata:
  type: project
---

Sixth same-calendar-day 2026-09-01 doc-sync pass (after harness re-sync,
editor-pane/e2e, auto-updater+check, preflight-security ssh-keyscan removal,
and the new versioning.md — all logged in [[project-styrman-docs]]). Two
unrelated operator decisions landed together, v1.16.1→v1.17.0 prep:

1. **"No backup at all. If a user needs backup they should commit to git."**
   — EVERY backup layer removed, both app and host: `internal/backup/`
   (whole package), `internal/httpapi/backups.go`, the 5 `/api/.../backups`
   routes, `fromBackup` create-project field, the v1.14.0 pre-wipe safety
   backup (now a NOTE comment in `server.go` recording it's deliberately
   irreversible), `deploy/backup/` (restic→GCS script + systemd units),
   `STYRMAN_BACKUP_CADENCE`/`STYRMAN_BACKUP_KEEP` config, and the whole
   frontend surface (Sidebar Backup action, NewProjectDialog "From backup",
   settings.tsx BackupsSection, api.ts backup functions). Verified by
   grepping `internal/`, `deploy/docker-compose.yml`, `.env.example`,
   `internal/config/config.go` — all genuinely clean, zero references left
   in code.
2. **"The host should not have a key"** — the auto-updater's git auth
   switched from an SSH deploy key (`/etc/styrman/deploy-key` +
   `/etc/styrman/known_hosts`) to a FINE-GRAINED GitHub token (repo-scoped,
   `Contents: Read-only`) at `/etc/styrman/gh-token`, fed to git over HTTPS
   via an inline `-c credential.helper=` clear + a helper function reading
   that file (never embedded in the URL). `GIT_TERMINAL_PROMPT=0` makes a
   missing/expired token fail LOUDLY at fetch instead of hanging. Verified
   against `deploy/update/styrman-update` (the `GIT_CRED_ARGS`/`GH_TOKEN_FILE`
   block) and `deploy/host-setup.sh` step 8c's printed instructions (the
   authoritative source for the exact operator-facing commands — matched
   verbatim in `deploy.md`'s rewritten prerequisite #1).

**Files touched**: `api.md` (dropped the 5 backup rows + `fromBackup` field
+ `STYRMAN_BACKUP_*` env row; the "## Git (real client)" section's old
"backups come from microVM volume snapshots... not yet built" line replaced
with the current "no backup layer, durability is git" framing), `deploy.md`
(deleted the whole "## Host-layer backups (disaster recovery)" section +
its "### Restore on a new host" subsection, replaced with a new short
"## Durability (no backup layer...)" section; Auto-update prerequisite #1
fully rewritten for the gh-token flow; Clean reinstall's wipe list dropped
`backups` from the dir list; the Credential-broker section's "Back it up
with the rest of the state dir" line — now false — reworded to "there is no
backup layer... re-onboard via setup on the rebuilt host"), `guide.md`
(§1 idea 5's backup sentence, the "Per-project backups" bullet — replaced
wholesale with a "There is no backup layer" bullet naming the
`wipe=true` irreversibility explicitly, §3 install comment dropped
"+ backup" from the systemd units list and "deploy-key" → "GitHub token",
§4 git bullet's "Back up via Backup/Settings→Backups" replaced, "Where
things live" table dropped the `backups/<user>/<uuid>/` row and the
"— back up" annotations on the home/workspace/credentials rows since
there's genuinely nothing backing them up now, the Reboots/"Disaster
recovery" paragraph rewritten to "A lost host is not recoverable"), root
`README.md` (Status paragraph's "automated backups on BOTH layers" bullet
rewritten past-tense + removed-notice, quick-start comment dropped
"+ backup" from the systemd units list), `deploy/README.md` (dropped the
whole `backup/` bullet, folded a one-line durability note into the
walkthrough pointer sentence instead).

**Deliberately NOT touched**: `versioning.md` (its auto-updater summary only
POINTS at deploy.md for prerequisites, never described the deploy-key
mechanism itself — grepped, nothing stale to fix), `AGENTS.md` (no
backup/updater-auth mentions at all — grepped, clean), `.agents/docs/
testing/styrman/README.md` (its ADR-052 section mentions "`/etc/styrman`,
which held the restic/GCS DR secrets before the auto-updater existed" — this
is correctly PAST TENSE, describing the 2026-09-01 state-loss incident
history, not a live feature; left alone on purpose), `guide.md`'s §7
Roadmap paragraph ("...backup automation — SHIPPED that release") — also
correctly historical, don't "fix" it to look like backups are still live,
and don't delete it either, it's an accurate shipped/retired record per
ADR-045.

**Correction to an earlier memory finding**: the 2026-09-01 (third pass)
entry in [[project-styrman-docs]] said `deployed-sha` was "written and never
read" by the rollback path. Re-read `deploy/update/styrman-update` this
session — that is **no longer true** (and may have already been fixed by
the time of that finding, not re-verified): `PREVIOUS_SHA` IS read and used
(`rollback_ref="${PREVIOUS_SHA}"; rollback_trust="trusted"`), skipping
`verify_tag` on a SHA-based rollback exactly as the script's own comments
always claimed. Don't repeat the "written but unread" claim without
re-grepping the rollback block first — this is exactly the kind of
"wired vs. not" fact that flips between fast-moving sessions.

**Flagged, not fixed (out of scope for this pass)**: root `README.md`'s
version stamp still says `v1.14.1` in the "## Status" heading, several
releases behind the actual `HEAD` (v1.16.1 in progress toward v1.17.0 per
the dispatching task). This was already stale independent of the backup
change — only the backup-specific sentence inside that paragraph was fixed
this pass. A full version-stamp sweep needs its own dedicated session
(same pattern as the e2e-count/check-count corrections — see
[[project-styrman-docs]]): re-verify against `git tag`/`CHANGES` before
touching, don't guess the number.

See [[project-styrman-docs]] for the full doc map and every prior session's
history, [[feedback-styrman-doc-edits]] for the re-read-before-Edit lesson
(needed again this session — api.md's backup-route table rows had subtly
different trailing whitespace than a first paste attempt expected).
