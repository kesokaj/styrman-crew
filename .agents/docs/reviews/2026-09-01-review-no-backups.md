# Review — no-backups + token-auth updater   (2026-09-01)

Scope: the entire uncommitted working tree against `HEAD` (`cbd8b05`/`00d1c18`) — 35 tracked
files (`git diff HEAD`) plus untracked `internal/httpapi/inflight_guard_test.go` and the
rebuilt `web/dist` assets. `web/dist` spot-checked for "was it rebuilt", not line-by-line.
Reviewer did not author any of this change set.

## Verdict: REQUEST CHANGES

Nothing in the removal is functionally broken — the build, `go vet`, `gofmt`, `go test ./...`,
`go mod tidy`, `tsc -b`, `bash -n` and `shellcheck -S info` on both changed scripts are all
clean, and the grep sweep finds **zero** dangling references to any removed symbol. What blocks
is smaller and fixable in minutes: `REQUIREMENTS.md` R13 still mandates the very backup layers
this change deletes (a REQUIREMENTS violation is blocking by rule), two now-callerless exported
orchestrator methods (`RootRename`, `WithProjectLock`) were left behind, and the relocated
`tryAcquire` was pasted into the MIDDLE of another function's doc comment, splitting it in two.
Separately, before v1.17.0 ships: the credential-helper in the updater interpolates an
operator-controlled path unquoted into a shell snippet (a path with a space silently yields an
EMPTY password — reproduced), and the now-orphaned `/var/lib/styrman/backups/<user>/…` archives
— which contain each project's home, i.e. materialized ssh keys, gcloud ADC and agent tokens —
have no code path left to list or delete them.

## Findings

- **[high / confidence: high]** `REQUIREMENTS.md:85-88` — R13 still reads "durable backup is two
  layers (operator directive 2026-08-30): per-project user-triggered backup/restore in-app
  (workspace + home tar.gz, ADR-044, shipped v1.13.0) and whole-state volume snapshots (host
  infra, planned)." The change set deletes both. Shipping code that contradicts a live
  requirement is a blocking violation regardless of how well the code is written. Fix: with the
  operator's explicit approval (the 2026-09-01 "no backup at all" directive is that approval,
  but it must land on disk), amend R13 in the SAME commit to state that git is the durability
  substrate and there is no backup layer. `README.md:96-101` already documents the removal —
  R13 is the file left inconsistent. Do not touch it without the approval on record.

- **[medium / confidence: high]** `internal/orchestrator/orchestrator.go:992` (`RootRename`) and
  `:926` (`WithProjectLock`) — both are now DEAD. Verified by grep across the tree: the only
  callers were `internal/httpapi/backups.go` (restore's quarantine rename at old line 445 and
  the restore's lock at old line 379). `RootRemoveContents` survives (still used by the wipe at
  `server.go:815`), these two do not. `WithProjectLock`'s doc comment even documents the deleted
  restore path ("Restore uses it so a boot can never virtiofs-mount the home while the tree is
  being quarantined/extracted"). Verdict: delete both, and drop the now-orphaned reference in
  `rootOpAllowed`'s comment (`:944`, "refused restores/wipes").

- **[medium / confidence: high]** `internal/httpapi/credharvest.go:188-194` — `tryAcquire` was
  inserted INTO the middle of `pushCredsToRunningProjects`'s doc comment. The file now reads:

  ```go
  // pushCredsToRunningProjects re-materializes freshly harvested credentials
  // into every RUNNING project of the user, immediately. Mechanism: exec a
  // tryAcquire is a compare-and-set on one of the in-flight maps (audit
  ...
  func (s *Server) tryAcquire(...)
  ...
  // NO-OP LOGIN shell in each guest — /etc/profile.d's credential snippet
  func (s *Server) pushCredsToRunningProjects(...)
  ```

  Two sentences of the push's rationale are now attached to `tryAcquire`, and the push's own doc
  begins mid-sentence. `gofmt`/`vet` cannot catch this. Fix: put `tryAcquire` (with its own
  comment) ABOVE the `pushCredsToRunningProjects` comment block — e.g. right after `shellQuote`
  — and restore the two orphaned lines to the head of the push's comment.

- **[medium / confidence: high]** `deploy/update/styrman-update:157-158` — the token file path is
  interpolated unquoted into the credential-helper shell snippet:
  `printf 'password=%s\n' \"\$(cat ${GH_TOKEN_FILE})\"`. Reproduced in a scratch sandbox: with
  the default path it returns the token correctly; with a path containing a space the helper
  emits `cat: …/tok: No such file` twice and returns `password=` (EMPTY) — git then fails
  authentication with a misleading 403 rather than the real cause. `${GH_TOKEN_FILE}` is
  operator input (`STYRMAN_GH_TOKEN_FILE`), so the same hole is a root command-execution surface
  for any existing path containing `;`/`$( )`/backticks (the `[ -f ]` test gates it, but a file
  can legally be named that). Fix: don't interpolate the path at all — `export GH_TOKEN_FILE`
  and have the helper read `"$GH_TOKEN_FILE"` from its inherited environment, or single-quote
  the path with `'` → `'\''` escaping. The token itself is correctly kept out of the URL and out
  of `ps` (only the helper text, not the secret, appears in argv) — that part is right.

- **[medium / confidence: medium]** Orphaned secret-bearing archives on the host (no file to
  point at — this is a release/ops finding). `internal/backup/backup.go:22` excluded only
  `.dockerd`, so every `<StateDir>/backups/<user>/<project>/<ts>.tar.gz` contains the full
  per-project HOME: `~/.ssh`, `~/.config/gcloud` ADC, claude/agy tokens. With `backups.go`,
  `RunBackupCadence` and the retention pruner all gone, nothing lists, prunes or deletes them
  ever again — and `.agents/docs/testing/2026-09-01-verify-harness-upgrade-v1.15.0-rev2.md:181`
  already recorded such a directory surviving a `wipe=true` delete. Fix: an explicit,
  operator-run removal step in the release notes/changelog after the operator decides whether
  anything in there is worth salvaging (it is now the ONLY copy). Reviewer did not and must not
  touch `/var/lib/styrman` (ADR-052).

- **[medium / confidence: medium]** `web/src/components/DeleteProjectDialog.tsx:23` —
  `const [wipe, setWipe] = useState(true)`. The wipe checkbox still defaults to CHECKED, but the
  server-side pre-wipe safety backup it was sized against is gone (`server.go:785-788` says so
  outright: "the UI's confirm dialog is the only gate"). Type `delete` + Enter now destroys
  workspace, git history and home with no recourse. Fix (operator's call): default it to
  unchecked, or keep the default and strengthen the copy to name what is unrecoverable. Flagging
  because the safety-net removal changed the risk of an unchanged default.

- **[medium / confidence: high]** `deploy/host-setup.sh:411, 417, 420` — PRE-EXISTING (present at
  `HEAD`, not introduced here), but inside the exact heredoc this review was asked to verify.
  `<<AU` is UNQUOTED, so in step 3 the un-escaped backticks and `$( )` are evaluated when
  host-setup RUNS: rendering the section (verified in a scratch shell) executes `git verify-tag`
  and `git config --global` as the setup user, prints their errors into the setup log, and
  leaves the instructions reading "A bare  exits 0 for a good-but-UNTRUSTED signature" and
  "no  needed"; `"$(cat ~/.ssh/id_ed25519.pub)"` renders EMPTY (root has no key), `$HOME`
  expands, and the trailing `\` on :417 joins two lines. The operator would paste a broken
  `allowed_signers` command. Fix: escape them (`` \` ``, `\$(`, `\$HOME`, `\\`), or better,
  switch the whole heredoc to `<<'AU'` and expand the three real variables another way. The
  NEW lines (:382, :385-387) are correctly escaped — verified rendering matches intent.

- **[low / confidence: high]** `deploy/update/styrman-update:146-159` — `-c credential.helper=`
  (the "clear inherited helpers" guard) is applied ONLY inside the `if [ -f "${GH_TOKEN_FILE}" ]`
  branch. With no token file, a stray root credential store could still answer for the fetch —
  exactly what the comment says must not happen. Fix: seed `GIT_CRED_ARGS=(-c credential.helper=)`
  unconditionally and append the helper inside the branch. (Empty-array expansion under `set -u`
  is safe here: bash 5.2 on Debian 13, verified.)

- **[low / confidence: high]** `deploy/host-setup.sh:387` — the clone command hardcodes
  `https://github.com/<owner>/<repo>.git` although `${ORIGIN}` is computed at `:354` and was used
  by the old instruction (`${ORIGIN:-<origin>}`). ORIGIN is now only used for an `-n` test. Fix:
  print the real origin (rewritten to https if it is an `ssh://`/`git@` URL) so the operator does
  not hand-substitute it.

- **[low / confidence: high]** `deploy/host-setup.sh:345` and `:438` — leftovers from the removed
  8b: the 8c comment still says "Like the backup timer, this is installed but only ENABLED…",
  and a bare `install -d -m 700 /etc/styrman` now dangles under the "(Step 8b … was REMOVED)"
  note. Fix: reword :345, and move the `install -d` into 8c with a one-line why (it holds
  `update/`, `allowed_signers`, `gh-token`).

- **[low / confidence: medium]** `internal/httpapi/inflight_guard_test.go` — the test does hold
  the line: it would go red on a plain-store regression (second acquire must be refused), on a
  per-key regression, and on a release regression. Two gaps worth a line each: it never exercises
  the mutex (no concurrent/`-race` case, so dropping `s.deletingMu` would still pass), and the
  file name matches no source file (`review-standards` §4 wants `foo_test.go` beside `foo.go`).
  Simplest fix for both: move `tryAcquire` into its own `internal/httpapi/inflight.go` and rename
  the test `inflight_test.go`; optionally add a 2-goroutine acquire race case.

- **[low / confidence: high]** Documentation lane, listed only so it is not lost. Re-checked at
  the end of the review: the parallel documentation pass has since updated `README.md`,
  `deploy/README.md` and `.agents/docs/specs/styrman/{api,deploy,guide}.md` (README:123 no longer
  advertises backup units — good). Still stale: `TODO.md:110-112` and `:124-127`, which track the
  deleted `deploy/backup/styrman-host-backup` and the restic/GCS secrets as open work. Not this
  reviewer's lane, but they must not ship as-is.

## Criteria

1. **Dead code — FAIL.** The removal itself is thorough: grep for `doBackup`, `errBackupBusy`,
   `restoring`, `backingUp`, `restoringState`, `seedFromBackup`, `readBackups`, `backupIDRe`,
   `projectIDRe`, `BackupMeta`, `fromBackup`, `ProjectBackups`/`UserBackups`,
   `BackupCadence`/`BackupKeep`, `RunBackupCadence`, `restic`, `deploy-key`, `known_hosts`,
   `GIT_SSH` across `*.go`/`*.ts`/`*.tsx`/`*.sh`/`*.yml`/Dockerfiles returns only comments;
   `web/dist` contains no `fromBackup`/`/api/settings/backups`; `web/e2e/run.mjs` and
   `.agents/docs/testing/styrman/integration.sh` are clean; `go mod tidy` produces no diff;
   `envIntOr` still has three callers. What fails the criterion is the *collateral*:
   `RootRename` and `WithProjectLock` are now callerless (finding 2).
2. **Flow tracing — PASS (with the doc-comment defect noted).** `deleteProject` re-traced end to
   end: the `deleting[key]` CAS under `deletingMu` is unchanged; the `defer` still both records a
   panic into `deleteErrs` and clears `deleting`; the detached 5-minute context is unaffected by
   the removed retry loop; `Destroy` failure → `deleteErrs` + `return` (record survives, UI shows
   `deleteError`); the wipe branch's `wipeFailed` fail-closed path and the M18 "never remove the
   record after a partial wipe" guard are untouched; on success the record, terminals, links and
   project env are cleaned in order. `deleteErrs` lifecycle is sound (cleared at attempt start,
   only ever written on failure). Removing the `restoring` guards from `ensureFor`,
   `startProject` and `deleteProject` leaves no gap — no second writer of those trees exists any
   more. `tryAcquire`'s single caller (`credharvest.go:224`, key `user+"/-"`) and the
   `credPushRerun` coalescing at `:272-275` still share `deletingMu` and behave identically.
   Frontend: `createProject` lost its 600 s `AbortSignal`, which is correct — the 201 now returns
   after mkdir + `EnsureRepo`, well inside the 20 s default (`api.ts:257`); the boot is
   background.
3. **Standards conformance — PASS.** `go build ./...`, `go vet ./...`, `gofmt -l` (clean),
   `go test ./... -count=1` (all `ok`), `go mod tidy` (no diff), `tsc -b` (exit 0),
   `bash -n` + `shellcheck -S info` on `styrman-update` and `-S warning` on `host-setup.sh`
   (both silent). Error wrapping and the no-floating-promise rules are unaffected by the diff.
   The one security-exempt area touched — the updater's git auth — correctly delegates to git's
   own credential-helper protocol and TLS instead of hand-rolling anything; no DIY crypto.
4. **Directory layout — PASS (nit).** Deletions only, in place. `inflight_guard_test.go` sits
   beside the code it covers and matches the package's topic-named test convention
   (`isolation_test.go`, `phase3_test.go`), but its name maps to no source file — see the low
   finding.
5. **Simplicity (DIY-except-security) — PASS.** A net −1,439 lines removing a whole feature and
   an entire package; no new dependency (`go mod tidy` clean, no npm change). The updater's auth
   swaps an SSH key + pinned `known_hosts` trust anchor for a repo-scoped read-only token over
   TLS — strictly less bespoke machinery, and the token is kept out of the URL and out of argv.
   `GIT_TERMINAL_PROMPT=0` is exported before any `git` invocation and does make an
   expired/missing token fail fast. The one blemish is the unquoted path interpolation.
6. **Decoupling — PASS.** The `httpapi ← internal/backup` seam is gone entirely; nothing else
   imported that package. `tryAcquire` landing in `credharvest.go` follows its only caller, which
   is the right instinct even though the placement wrecked a comment; a two-line
   `internal/httpapi/inflight.go` would be cleaner still. No cross-package reach-ins introduced;
   the frontend deletions are confined to the four files that owned the feature.

## What blocks what

**(a) Blocks the COMMIT** — all three are minutes of work:
1. `REQUIREMENTS.md` R13 must be amended (operator-approved) in the same commit — high.
2. Delete `RootRename` and `WithProjectLock` (dead) — medium.
3. Un-split the `pushCredsToRunningProjects` doc comment in `credharvest.go` — medium.

**(b) Blocks the v1.17.0 RELEASE** (in addition to the above):
4. Quote/env-pass the token path in the updater's credential helper — medium.
5. Decide and record the fate of `/var/lib/styrman/backups/<user>/…` (credential-bearing
   archives, now unreachable by any code path); the removal must be an explicit operator step in
   the release notes — medium.
6. DoD rows: the **security** row landed in parallel while this review was being written
   (`.agents/docs/reviews/2026-09-01-security-no-backups.md`, verdict FINDINGS) — read it
   together with this one; its findings are not folded in here. Still outstanding: the
   **verifier** log (local tier: tests + ARM64/AMD64 image builds) and the **changelog** row.
   This review is neither.
7. Operational precondition already flagged by the operator: `/etc/styrman/gh-token` does not
   exist yet, so the enabled 5-minute updater timer fails at fetch until the token is minted.
   Deliberate — but it must not be mistaken for a regression introduced by the tag.

Not blocking, fix when convenient: the `-c credential.helper=` scope, the hardcoded clone URL,
the two 8b leftovers in `host-setup.sh`, the delete-dialog default, the test's missing race case,
and the pre-existing unescaped `` ` ``/`$( )` in the step-3 heredoc (worth fixing now, while the
file is open — it silently corrupts the tag-signing instructions the operator is told to follow).
