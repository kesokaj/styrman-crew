---
owner: reviewer
date: 2026-09-11
---
# Review — styrman-ctl final (phase 3/4 review responses)   (2026-09-11)

Range reviewed: `git diff 108c4e4..e34bf37 -- deploy/styrman-ctl internal/config/styrman_ctl_test.go`
(commits c397653, becbd6b, b01914a, e34bf37). Blobs pinned:
`deploy/styrman-ctl@7bd5c9e`, `internal/config/styrman_ctl_test.go@23ceff4` (both at `e34bf37`).

## Verdict: REQUEST CHANGES

The round-4 security fix (`looks_like_ours`) is itself the bug this round leaves behind: on any
real production host where auto-update was never configured — a documented, normal state
(`host-setup.sh` prints "auto-update INSTALLED but NOT enabled" and leaves it to a later manual
step) — `/etc/styrman` exists (created unconditionally by `host-setup.sh`'s step 9, `install -d
-m 700 /etc/styrman`) but is empty, carrying none of `update/`, `allowed_signers`, or `gh-token`.
`looks_like_ours` refuses to delete it, so `uninstall_level`'s unconditional `rm_tree
"${ETC_DIR}"` at level 2 dies under `set -e` — aborting even the dry-run *plan preview*, before
the deploy clone, images, `.env` backup, or (level 3) the entire `${STATE_DIR}` are ever reached.
This is the exact class of defect the phase-4 review caught (a `set -e` abort mid-`uninstall_level`
that a dry-run-only test suite structurally cannot see), reintroduced by the fix meant to close a
different one, and it is untested: the shared Go fixture (`ctlSetup`) always pre-populates
`etc/update`, so no test exercises an `ETC_DIR` that is real but marker-less.

All four dispatch-named earlier findings are otherwise genuinely closed, and the specific residual
risks named in the dispatch (word-splitting in `drop_chain`, the override gate's effect on
`host-setup.sh`) check out clean — see below.

## Findings

- **[high / confidence: high]** `deploy/styrman-ctl:713-727` (`looks_like_ours`, `ETC_DIR` branch)
  and `:766` (`rm_tree "${ETC_DIR}"` in `uninstall_level`) — a real `/etc/styrman` created by
  `host-setup.sh:372` (`install -d -m 700 /etc/styrman`, run unconditionally at the end of setup,
  independent of whether auto-update is ever configured — see `host-setup.sh:267-303`'s own
  "auto-update INSTALLED but NOT enabled" branch) has none of the three markers
  `looks_like_ours` requires (`update/`, `allowed_signers`, `gh-token`). `rm_tree` therefore
  `die`s with "refusing to delete /etc/styrman: not styrman's etc dir" on every `uninstall
  --level 2` and `--level 3`, including the dry-run plan preview shown *before* the confirmation
  word is asked (`( DRY=1; uninstall_level "${level}" )` at the top of `cmd_uninstall`), for any
  host that has styrman installed but auto-update not yet (or never) set up. `guard_path` alone
  already accepts this path (canonical, exists, is one of the three configured trees) — only the
  new check rejects it, so this is a regression introduced by round 4, not a pre-existing gap.
  Concrete failure: an operator who declined auto-update runs `sudo styrman-ctl uninstall
  --level 3`; the stack, networks, and unit disables from level 1 succeed, `drop_chain` is
  best-effort so the firewall teardown proceeds, but the script then dies on `rm_tree
  "${ETC_DIR}"` before removing the deploy clone, the images, the `.env` backup, or touching
  `${STATE_DIR}` at all — uninstall is simply broken for this host shape. Fix: extend
  `looks_like_ours`'s `ETC_DIR` branch to also accept an empty-of-anything-else directory (e.g.
  additionally check it contains nothing *but* styrman-shaped entries, or simply drop the
  positive-marker requirement for `ETC_DIR` and rely on `guard_path`'s three-tree allowlist plus
  the existing system-root blocklist, matching how `DEPLOY_DIR`'s absent-when-never-cloned case is
  already handled by `rm_tree`'s own "already absent" short-circuit rather than a marker check).
  Add a test: `ETC_DIR` pointed at a real, empty (host-setup-only, no `update/`) directory must
  still be deletable by `rm_tree`/`uninstall_level 2`.

## Closed by this range (verified)

- **cmd_update diagnostics on a failed tick** — already fixed at the base commit `108c4e4`
  ("a failed updater tick still prints the journal and status before exiting non-zero"), prior to
  this diff range; not reopened.
- **Displayed `.env` values stripped of control bytes** — `printable()` now wraps `status`'s
  `base`/`hn` (`:288`) and `masked()`'s non-secret branch (`:608`); regression test
  `TestStyrmanCtlDisplayValuesArePrintable` plants a raw ESC byte and confirms it never reaches
  `status --json` output while the harmless parameter text survives.
- **Effective admin roster shown in the edit table** — `cmd_edit`'s `SUPERADMINS` row resolves the
  `${STYRMAN_ALLOWED_EMAILS}` reference to the actual email list (`:641`); `STYRMAN_ALLOWED_EMAILS`
  itself is annotated as "who may sign in; also the admins while SUPERADMINS refers to it". Matches
  `internal/userstore`'s actual precedence (superadmins-or-reference).
- **`safe.directory` for root's git on the checkout** — `resolve_version` (`:390`) and
  `https_origin` (`:404`) both now run `git -c safe.directory="${REPO}"`; verified no other
  `git -C "${REPO}"` call site in the file was missed. `git -C "${DEPLOY_DIR}"` call sites
  (`:298-299`, `:536`, `:541`, `:544`) correctly omit it — `DEPLOY_DIR` is root's own clone
  (created by `git clone` running as root at `:544`), so there is no ownership mismatch to
  suppress.
- **Test-seam overrides gated outside the harness** — the six path-override env vars are refused
  unless `STYRMAN_CTL_TEST=1` (`:37-42`); `TestStyrmanCtlOverridesGatedOutsideHarness` confirms.
  Checked the dispatch's specific concern: `deploy/host-setup.sh` never invokes `styrman-ctl` and
  never sets any of the six gated variables (`grep` came back empty), so the gate has zero effect
  on `host-setup.sh`'s own env — no interaction, no regression.
- **Firewall jumps deleted exactly as installed (`drop_chain`)** — the delete now reads
  `iptables -S <parent>`, filters the line ending `-j <chain>`, and replays its exact match spec
  (including `-i sdb-+`) to `-D`, matching `ensure_jump`'s install shape in
  `deploy/firewall/styrman-firewall.sh:55-56` exactly. Checked the dispatch's named risk (word
  -splitting of a listed rule, flagged as a real hazard by `styrman-firewall.sh:124-125`'s own
  comment about naive word-splitting on comment-bearing rules): `drop_chain` is only ever called
  for the four fixed jump chains (`STYRMAN-FWD`/`STYRMAN-IN`/`STYRMAN-FWD6`/`STYRMAN-IN6`), whose
  only installer (`ensure_jump`) never emits a comment or quoted-space token — the rule text is
  always the plain `-i sdb-+ -j <chain>` two-token shape the test fixture models. Not used against
  the per-LAN-project tagged rules (which carry `--comment "..."` and spaces) — those chains are
  only `-F`lushed, never individually `-D`eleted. No false-positive/negative exposure with the
  current callers.
- **`du` under `run`** — `run du -sh "${STATE_DIR}" || true` (`:771`); confirmed absent from the
  dry-run "nothing executed" call-log assertion and present as a plan line in the level-3 preview
  test.
- **Signers line counting** — `grep -c '' "${signers_file}"` replaces `wc -l`, correctly counting
  a final line with no trailing newline; new regression test plants a second signer hidden behind
  a missing trailing newline and confirms it is refused.
- **Symlinked secret files refused** — both `--gh-token-file` and `--allowed-signers-file` gained
  `[ -L "${file}" ] && die ...` ahead of `private_file`.
- **`looks_like_ours` for `STATE_DIR`/`DEPLOY_DIR`** — correctly refuses well-known system roots
  outright and requires positive markers otherwise; matches the real production layout
  (`/var/lib/styrman/{homes,projects}` created eagerly by `host-setup.sh:182-186`,
  `/opt/styrman/deploy/docker-compose.yml` from the repo's own layout). `STATE_DIR`'s check is an
  OR across `homes`/`projects`/`users`, so a fresh host with `homes`/`projects` but no `users/`
  yet (lazily created by the server) still passes — no false negative there, unlike `ETC_DIR`
  above.
- **Test fixture** — the fake `iptables`/`ip6tables` in `TestStyrmanCtlUninstallPlans` now emit
  `-S` output shaped like the real installer (`-A DOCKER-USER -i sdb-+ -j STYRMAN-FWD`, etc.), and
  the deploy-clone fixture gained `deploy/deploy/docker-compose.yml` so `looks_like_ours`'s
  `DEPLOY_DIR` branch passes in tests the way a real clone would. `git diff --numstat`/`git status`
  show only text-diff changes to the two named files — no binary corruption, no unrelated
  deletions (writing-files.md checks).

## Criteria

1. Dead code — pass; no unused code introduced or left behind by this range.
2. Flow tracing — fail: `looks_like_ours`'s `ETC_DIR` branch does not account for the real host
   state `host-setup.sh` itself produces (an empty, marker-less `/etc/styrman`), so the new guard
   breaks its own caller (`rm_tree` inside `uninstall_level`) on that state — see the high finding.
3. Standards conformance — pass otherwise; matches the file's existing best-effort (`|| true`)
   convention for the firewall teardown, and the security-review comments accurately describe what
   the code does everywhere except the case above.
4. Directory layout — pass; all changes stayed inside the two named files, no new files.
5. Simplicity (DIY-except-security) — pass; no new dependency, `drop_chain`/`looks_like_ours` are
   small stdlib-bash functions sized to their one caller.
6. Decoupling — pass; `looks_like_ours` and `drop_chain` are each single-purpose and independently
   testable (as the new `__rm-tree` hook demonstrates), which is exactly how the `ETC_DIR` gap was
   findable by reading the function in isolation against `host-setup.sh`.

## Not examined

- Live execution on a real host or against real `iptables`/`systemctl`/`git` — this review is
  static (script + Go fake-tool test harness reading only).
- `internal/orchestrator`, the vendored Caddy ingress, and everything outside the two named files
  — out of the dispatch's stated scope.
- Whether `looks_like_ours`'s system-root blocklist is exhaustive for every possible
  `STYRMAN_STATE_DIR`/`STYRMAN_DEPLOY_DIR`/`STYRMAN_ETC_DIR` value an operator could set outside
  the test harness (the override gate closes the `sudo -E` vector for the other six knobs, but
  these two remain documented, operator-settable production knobs by design — not re-litigated
  here since it was accepted scope in the round-4 commit and the dispatch did not ask for it).
