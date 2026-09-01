# Remove every backup layer; auto-updater switches from an SSH deploy key to a GitHub token

Date: 2026-09-01. Same-evening operator directives, verbatim intent: (a) "No
backup at all. If a user needs backup they should commit to git" — remove
every backup layer, in-app and host, no exceptions; (b) "the host should not
have a key" — the auto-updater's git auth stops being an SSH deploy key; (c)
delete the GCS bucket and its service account — operator-run, no `gcloud` on
this host. Gates the v1.17.0 release, cut in the step immediately after this
commit. Follows the same-day state-loss incident
(`.agents/docs/changes/2026-09-01-incident-state-loss.md`), whose root cause
— a DR layer whose own credentials shared fate with the data they protected
— is part of why the operator chose removal over re-provisioning.

## 1. In-app backup/restore removed (was ADR-044)

Deleted outright: `internal/backup/` (the whole package — archive/extract,
root-assisted quarantine-rename via the server's own container), the five
`/api` backup routes and `internal/httpapi/backups.go`, `FromBackup`
project-seeding at creation, the `RunBackupCadence` goroutine in
`cmd/styrman/main.go`, the config backup knobs (`backupCadence`/`backupKeep`
fields, their env vars, and the `BackupCadence()`/`BackupKeep()`/
`ProjectBackups()`/`UserBackups()` accessors), the Sidebar "Backup" action,
the Settings "Backups" section, and the New-project "From backup" option.
The v1.14.0 pre-wipe safety snapshot goes with it — wipe-delete is now
plainly irreversible, and the `DeleteProjectDialog` wipe checkbox now
**defaults OFF** instead of pre-checked (review + security both flagged the
old checked-by-default against a removed safety net as a data-destruction
risk with no attacker involved).

Collateral cleanup the reviewer caught: `RootRename` and `WithProjectLock`
(`internal/orchestrator/orchestrator.go`) were now-dead exported helpers
whose only callers were the deleted restore path — deleted, along with the
stale "refused restores/wipes" line in `rootOpAllowed`'s comment.
`tryAcquire` (the single-flight compare-and-set the credential-push path
also used) moved to `internal/httpapi/credharvest.go`, its one remaining
caller, with a new `credharvest_guard_test.go`
(`TestTryAcquireSingleFlights`) — first acquire succeeds, a second on the
same key is refused while held, a different key is unaffected, release
frees the key again.

## 2. Host DR layer removed

`deploy/backup/*` (the restic-to-GCS script and its two systemd units) and
host-setup step 8b (restic install, unit install, provisioning
instructions) are gone from the repo. On the **live** host:
`styrman-backup.timer`/`.service` disabled and deleted,
`/usr/local/bin/styrman-host-backup` removed, and
`/etc/styrman/{restic-pw,backup.env}` deleted. The GCS bucket and its
service account are **not** deleted by this change — no `gcloud` runs on
this host by design; that is an explicit operator action (§5).

## 3. REQUIREMENTS.md R13 amended (operator-approved)

R13 previously mandated (2026-08-30 directive) two backup layers: in-app
per-project archive/restore and a planned host-level snapshot layer. The
2026-09-01 "no backup at all" directive supersedes that in writing: R13 now
reads "Durability IS git... There is NO backup layer of any kind... and a
wipe-delete is irreversible," citing ADR-044 (removed) and ADR-055 (this
removal). The reviewer flagged shipping code that contradicted a live
REQUIREMENTS.md line as blocking by rule regardless of how clean the removal
itself was — fixed in the same commit as the code, per the constitution's
requirement that a REQUIREMENTS.md change carry explicit operator approval
on record.

## 4. Updater auth: SSH deploy key → fine-grained GitHub token (was ADR-051's prerequisites)

`deploy/update/styrman-update` no longer authenticates with an SSH deploy
key against a pinned `known_hosts`. It now reads a fine-grained GitHub
token, scoped to this one repository at `Contents: Read-only`, from
`/etc/styrman/gh-token` (root-owned, mode asserted `600` before use, refused
otherwise). The origin is HTTPS. A URL-scoped, per-invocation git credential
helper (`credential.https://github.com.helper=`) answers only for
`github.com` — verified during review that an unscoped form would hand the
token to a probe for *any* host, closed by scoping it. The token path is
never interpolated into the git-evaluated shell string (an earlier version
did, and a path containing a space silently produced an empty password,
reproduced in review) — the helper instead reads `$GH_TOKEN_FILE` from its
own exported environment. Inherited credential helpers are cleared
(`-c credential.helper=`) **unconditionally**, not only when a token file
exists, so an ambient root credential store can never silently answer for a
root-run fetch. `GIT_TERMINAL_PROMPT=0` makes a missing or expired token
fail loudly (fine-grained tokens expire; "cannot see new versions" must not
look identical to "up to date").

On the **live** host: the GitHub deploy key was revoked, its key files and
`known_hosts` deleted, and `/opt/styrman` re-pointed at the `https://`
origin. The 5-minute update timer now fails loudly at every fetch — expected
and deliberate — until the operator mints and installs the token (§5).

`deploy/host-setup.sh` step 8c was rewritten around the token flow (mint,
provision at `/etc/styrman/gh-token` with `read -rs` so the token never
lands in terminal scrollback or a styrman terminal transcript, clone with
the same host-scoped inline helper). In the course of rewriting it, the
reviewer also caught a **pre-existing** bug in step 3's heredoc: `<<AU` was
unquoted, so the backticks and `$( )` in the tag-signing instructions were
evaluated when `host-setup.sh` itself *ran* rather than staying literal text
for the operator to read and paste — verified by rendering it in a scratch
shell, which executed `git verify-tag`/`git config --global` as the setup
user and printed broken, already-executed instructions. Fixed by escaping
the literal backticks/`$( )`/`\` in that heredoc; the reviewer re-verified
the corrected rendering matches intent.

## 5. Operator actions still open (not done by any agent)

- Mint the fine-grained GitHub token and install it at
  `/etc/styrman/gh-token` (root:root, `600`) — the updater fails loudly
  until then, by design.
- Delete the GCS bucket `styrman-crew-backups-driven-eft-h4it` and its
  backup service account in project `driven-eft-h4it` — no `gcloud` on this
  host, operator-run.
- Shred `/var/lib/styrman/backups/` on the host. Security found a live
  ~10 MB `tar.gz` there (a project workspace *and its per-project home* —
  ssh keys, gcloud ADC, claude/agy tokens materialized per ADR-030/032) with
  every code path that could list or delete it now gone — an orphaned,
  unencrypted, credential-bearing archive with no lifecycle. No agent
  touches `/var/lib/styrman` (ADR-052); this is the operator's to shred.

## 6. Gates

- **Reviewer** — REQUEST CHANGES
  (`reviews/2026-09-01-review-no-backups.md`): three commit-blockers (the
  R13 contradiction, the two dead orchestrator helpers, `tryAcquire` pasted
  into the middle of `pushCredsToRunningProjects`'s doc comment — all fixed
  same pass) plus release-blockers (the unquoted token-path interpolation,
  the orphaned archive's fate) folded into the fixes above. Lower-severity
  items not fixed in this pass and carried to `TODO.md`: the credential
  helper's `-c credential.helper=` scope note duplicated the security
  finding (fixed, see §4); a hardcoded clone URL in host-setup.sh's
  instructions instead of the computed `${ORIGIN}`; two leftover 8b
  references in host-setup.sh comments; `inflight_guard_test.go` has no
  concurrent/`-race` case and its filename maps to no `inflight.go` source
  file.
- **Security** — FINDINGS
  (`reviews/2026-09-01-security-no-backups.md`): the orphaned
  credential-bearing archive (medium, §5 above — operator action, not
  code); the wipe-default checkbox (medium — fixed, §1); the unscoped
  credential helper (medium — fixed, §4); the conditional-only
  `credential.helper=` reset (low — fixed, §4); `read -r` echoing the
  token to the terminal during provisioning (low — fixed, §4, now
  `read -rs`); the unquoted `GH_TOKEN_FILE` interpolation (low — fixed,
  §4). One info-level finding recorded rather than fixed: a bearer token
  has no possession binding, unlike an SSH key — accepted trade, recorded
  in ADR-056.
- **Verifier** — PASS (`testing/2026-09-01-verify-no-backups.md`): main
  module `go test -race -count=1 ./...` all packages `ok` (`internal/backup`
  correctly absent from the list); `tsc -b` and `oxlint` clean; a dist-
  staleness grep for the removed "From backup" string returned zero matches
  both before and after the e2e rebuild; `bash -n` + `shellcheck -S error`
  clean on both changed scripts; the updater's token matrix run live as
  root against nested scratch paths only (`/var/lib/styrman`/`/etc/styrman`
  untouched throughout, per ADR-052) — no token file (loud fetch failure,
  no hang), world-readable token (perms refusal before any fetch), valid-
  shape root-600 token (past the guard, GitHub rejects the fake value
  specifically — proves the helper is wired); `docker compose config`
  render has zero `backup` occurrences; a scratch server image build
  succeeds; `integration.sh` 57 passed / 0 failed / 3 skipped and
  `web/e2e/run.sh` 48 passed / 0 failed / 2 skipped, both matching the
  pre-change baseline exactly.
- **Documentation** — swept in the same pass: `README.md`, `deploy/README.md`,
  `.agents/docs/specs/styrman/{api,deploy,guide}.md`, plus a new durability
  section explaining "commit and push" as the substrate.

## Verification

Reports on disk: `reviews/2026-09-01-review-no-backups.md`,
`reviews/2026-09-01-security-no-backups.md`,
`testing/2026-09-01-verify-no-backups.md`. No fix claimed only in chat.
