---
owner: security
date: 2026-09-11
---
# Security review — styrman-ctl, final (the Phase 3 + Phase 4 remediation)   (2026-09-11)

Range reviewed: `108c4e4..e34bf37` — `deploy/styrman-ctl` only (the dispatch's path filter).
Three commits: `c397653` (round 3), `becbd6b` (drop_chain + `run du`), `b01914a` (round 4),
`e34bf37` (test fix). Diff: +71/-15 on one file.

Blob pinned: `deploy/styrman-ctl` at `e34bf37` = git blob `7bd5c9e`, sha256
`6cde449a7e0ad9b182f5de9c455503c13ee69c843ab10f59e0f3b90f88e3387a`, 886 lines. Every line number
below is that blob.

**The tree MOVED under this review**, as it did under the Phase 4 one. `HEAD` was `e34bf37` when
the review started and is `4c0869b` now ("an empty state or etc dir is deletable ... final
review's high"), via `d4c362d` (BUG-041). The working copy is 891 lines, sha256
`f4a2f47a...`. `4c0869b` is out of range but inside its blast radius and it partially fixes N2
below; N2 is reported against the reviewed range and labelled with its state at `4c0869b`,
because a release could be cut from either commit. Blast radius followed outward:
`deploy/firewall/styrman-firewall.sh` (the installed jumps drop_chain must match),
`deploy/update/styrman-update`, `internal/config/styrman_ctl_test.go`, `deploy/.env.example`,
`internal/userstore/userstore.go`.

## Verdict: FINDINGS

Of the eight items the dispatch names, three are resolved, four partial and one open (and
inverted) — but the round-2/round-4 fixes brought new weaknesses of their own, and the worse of them is N1: `drop_chain` runs
`iptables` with **no `-w`**, every step `|| true`, immediately after level 1 has made dockerd
rewrite iptables four times over (`compose down`, `docker rm -f` of every microVM,
`docker network rm` of every project bridge). A lost race for the xtables lock on the `-D` is
swallowed, and the very next line **flushes STYRMAN-FWD while DOCKER-USER still jumps into it** —
an empty user chain returns, so every private-range DROP that confines a sandbox project is gone
while the hook remains, with `styrman-firewall.{service,timer}` already disabled (`:753`) and
about to be deleted (`:761`) so nothing restores them. The sibling script
`deploy/firewall/styrman-firewall.sh:44-46` added `-w 5` for exactly this reason ("wait for a
concurrent netlink writer (dockerd) instead of failing the whole run"); the new code does not
carry it. The pre-fix behaviour was *safer in this one respect*: at `108c4e4` the failed `-D`
aborted the run under `set -e` with the chain fully intact. N2 is the round-4 marker check
turning `rm_tree` into a mid-plan `die`: on a host whose `/etc/styrman` or `/opt/styrman` does
not carry the expected marker (an interrupted `update --setup`, a half-finished clone), the
**plan subshell** dies under `pipefail`+`set -e` before the confirmation word is asked, so
`uninstall --level 2` and `--level 3` cannot run at all and every secret stays on the host
behind a message that reads like a safety refusal.

On the dispatch's explicit question — **is the system-root list sufficient?** No, but it is a
real improvement. It kills the Phase 4 proof-of-concept (`STYRMAN_STATE_DIR=/var/lib` is now
refused by name) and forces any other target to carry a styrman-shaped marker. It is
exact-match-only, so every FHS directory one level down is unlisted, and the markers are
ordinary directory names: `STYRMAN_STATE_DIR=/home/<dev>` passes the moment that account has a
`~/projects` directory — which is this project's own in-guest convention — and
`STYRMAN_DEPLOY_DIR=<the operator's checkout>` passes unconditionally, because a styrman
checkout *is* `.git` + `deploy/docker-compose.yml`. See F6.

## Disposition of the eight items the dispatch names

| item | status | evidence at `e34bf37` |
|---|---|---|
| Control-byte stripping of displayed values (Ph3 F1) | **partial** | `:288` `base`/`hn` and `:608` `masked()` now go through `printable` — the two halves the finding led with. Still raw: `:313-316` `disk`/`dockerv`/`katav`/`operator` (named in F1 verbatim) and, newly, the `edit` re-prompt at `:653`→`:82,:84`. F1 |
| Effective admin roster (Ph3 F3) | **partial** | `:641-642` render the resolved list and say "these are the admins" in the interactive table. The finding's own scenario — `styrman-ctl edit STYRMAN_ALLOWED_EMAILS=…` on argv — still writes both copies and redeploys without printing the word admin (`:663-672`). F2 |
| `safe.directory` (Ph3, "no ownership assertion", open) | **open, and inverted** | `:390`, `:404` add `-c safe.directory="${REPO}"` to root's git on the *operator's checkout*; the finding was about `git -C "${DEPLOY_DIR}"`, which is unchanged at `:298-299`. An ownership *waiver* shipped where an ownership *assertion* was asked for. F5 |
| Test-seam override gate | **resolved** (for the seven named) | `:38-42` refuses `STYRMAN_ETC_DIR STYRMAN_UNIT_DIR STYRMAN_BIN_DIR STYRMAN_SBIN_DIR STYRMAN_TMPFILES_DIR STYRMAN_KATA_DIR STYRMAN_ENV_FILE` unless `STYRMAN_CTL_TEST=1`; an empty value falls through to the default (fails safe); it runs before `main`, so it covers every verb. Residuals: `cmd_help:843-845` still advertises five of them as knobs (F4), and the gate shares its key with `cmd_internal` (F7) |
| `STYRMAN_STATE_DIR`/`STYRMAN_DEPLOY_DIR` stay knobs | **partial** (Ph4 S2) | mitigated by `looks_like_ours`, not closed. F6 |
| `drop_chain` (Ph4 S1) | **resolved**, S8 **open and now triggerable** | `:697-707`; verified against this host's live rules: all four (parent, chain) pairs match exactly what `styrman-firewall.sh:62,79,103-104,109-110` installs. The S8 flush-while-referenced residual is unchanged and N1 gives it a concrete trigger |
| `looks_like_ours` + the system-root list | **partial** | `:713-727`. F6, and the new N2 |
| `grep -c ''` on the signers file (Ph4 S4) | **resolved** | `:502`. Measured on scratch files: S4's exact case (two signer lines, no trailing newline) now counts **2** and is refused, where `wc -l` counted 1 and `head -1` silently dropped the second; a one-line file counts 1 with or without the newline; an empty file counts 0, passes the count guard, and then fails closed on `is_signers_line` (`:507`). The one-signer *limit* is unchanged by design and now has an accurate guard |
| Symlinked secret files refused (Ph4 S3/F6) | **partial** | `:491`, `:500` refuse a symlinked `--gh-token-file`/`--allowed-signers-file`. No owner check, no parent-writability check, no fingerprint echo, and four separate opens of the path (`-f`, `-L`, `grep`, `head`) keep S3's `/tmp` race open. `--env-file` (`cmd_install:433`) got nothing. F3 |

## Findings

### New — introduced by the fixes

- **N1 [medium / confidence: medium]** `deploy/styrman-ctl:697-707` (`drop_chain`) —
  **`iptables` without `-w`, every step `|| true`, and the flush runs whether or not the unhook
  worked; a lost lock race silently removes sandbox isolation instead of aborting.**
  `drop_chain` is called at `:757-760`, i.e. *after* level 1 has run `docker compose down`
  (`:743`), `docker rm -f` on every styrman-managed container (`:751`) and `docker network rm`
  on every `sdb-`/`lan-` bridge (`:752`) — four operations that make dockerd take the xtables
  lock and rewrite the very chains being torn down. `deploy/firewall/styrman-firewall.sh:44-46`
  wraps every call as `ipt() { "$ip4" -w 5 "$@"; }` with the comment "wait for a concurrent
  netlink writer (dockerd) instead of failing the whole run under set -e (review)". `drop_chain`
  calls the bare binary. Scenario: the `-D` at `:703` returns 4 ("another app is currently
  holding the xtables lock"); `|| true` swallows it; a moment later the lock is free and `:705`
  `-F STYRMAN-FWD` succeeds. `DOCKER-USER` still jumps into `STYRMAN-FWD`, which is now empty —
  an empty user chain returns, so the six private-range DROPs (`10/8`, `172.16/12`,
  `192.168/16`, `169.254/16`, `100.64/10`, `198.18/15`) that make a sandbox project
  internet-only are gone. `:706`'s `-X` then fails "Device or resource busy", also swallowed, so
  the empty chain persists. `:753` has already disabled `styrman-firewall.timer` and `:761`
  deletes its unit, so nothing re-installs the rules; an operator who stops at level 2 and
  brings the stack back by hand gets projects that reach the LAN, the host and each other's
  bridges. Nothing is printed on any of the three failures — "silence from your own
  instrumentation is not evidence" applies literally here. Remediation: add `-w 5` to every
  `iptables`/`ip6tables` invocation in `drop_chain`; re-read `-S "${parent}"` after the delete
  loop and, if any jump remains, `warn` and `return 0` **without** flushing — never flush a
  referenced chain (this is Phase 4 S8's remediation, still unapplied); and count the failures
  so the run can end with "N of M teardown steps failed".

- **N2 [medium / confidence: high]** `deploy/styrman-ctl:730-736` (`rm_tree`) with `:800-802`
  — **the round-4 marker check `die`s inside the plan subshell, so a host whose `/etc/styrman`
  or `/opt/styrman` is not marker-shaped cannot uninstall at level 2 or 3 at all.**
  `rm_tree` now ends in `looks_like_ours "${p}" || die` (`:734`). Under `--dry-run` the guards
  still run for real (only `rm -rf` is behind `run`), and `cmd_uninstall` prints the plan with
  `( DRY=1; uninstall_level "${level}" ) | sed 's/^/  /'` at `:800`. With `set -o pipefail`
  (`:19`) the subshell's `die` makes the pipeline return 1, and `set -e` ends the script —
  before `confirm_word` at `:806` is ever reached. Scenario: `update --setup` is interrupted
  between `install -d … "${ETC_DIR}"` (`:530`) and `install -m 600 … gh-token` (`:531`), or a
  multi-minute `git clone` into `${DEPLOY_DIR}` is interrupted (the `INT TERM HUP` trap at
  `:517` exists precisely because that happens), leaving `/etc/styrman` empty or
  `/opt/styrman/.git` present without `deploy/docker-compose.yml`. The operator then runs
  `uninstall --level 3 --confirm <hostname>`: the plan prints a few lines, the run dies
  "refusing to delete /etc/styrman: no update/, allowed_signers or gh-token inside", and **no**
  level runs — the stack, the microVMs, the GitHub PAT, `allowed_signers`, both copies of
  `deploy/.env` and all of `/var/lib/styrman` stay. The message reads like a guard doing its
  job, not like "your teardown is blocked"; the operator's likely next move is `rm -rf` by hand,
  with no guard at all. The failure direction is safe (nothing is deleted) but the outcome —
  secrets left on a host the operator is decommissioning — is the same one Phase 4 S1 rated
  high. Remediation: evaluate the markers once, up front, and either `warn` and skip that one
  tree while the rest of the teardown proceeds, or refuse *before* printing a plan the run
  cannot execute — never let a guard fire for the first time from inside the plan subshell.
  Mechanism verified in a scratch reproduction of the control flow (`pipefail` + `set -e` +
  `( DRY=1; … ) | sed`): the plan prints, `die` fires, and the line that would ask for the
  confirmation word is never reached. **At `4c0869b` (out of range): PARTIALLY FIXED.** That
  commit adds `local empty=0; [ -z "$(ls -A "${p}" 2>/dev/null)" ] && empty=1` and lets an empty
  `STATE_DIR` or `ETC_DIR` through, which closes the interrupted-`update --setup` half. Still
  open there: `DEPLOY_DIR` is deliberately excluded from the escape hatch ("A deploy clone must
  look like one"), so the interrupted-`git clone` shape — `${DEPLOY_DIR}/.git` present,
  `deploy/docker-compose.yml` absent — still `die`s in the plan subshell and still blocks
  `uninstall --level 2/3` entirely; so does a non-empty `/etc/styrman` that carries none of the
  three markers. Two notes on the escape hatch itself: `ls -A` on a directory it cannot read
  reports empty, so the hatch opens on a read failure rather than closing (root can read
  everything, so this bites only the non-root `--dry-run` plan), and "empty" is re-checked
  nowhere between the guard and the `rm -rf`.

- **N3 [low / confidence: high]** `deploy/styrman-ctl:653` reaching `ask` at `:79-85` — **the
  `edit` table is control-byte-stripped; the prompt that immediately follows it is not.**
  `masked()` (`:608`) now sanitises what the table shows, but selecting a key calls
  `ask v "${k}" "$(env_get "${src}" "${k}")" …`, and `ask` renders the default raw:
  `_shown="${_default}"` (`:79`) → `read -r -p "${_prompt}${_shown:+ [${_shown}]}: "` (`:84`).
  `validate` does not exclude control bytes from the keys that matter: `is_email` (`:156`) is
  `[^[:space:]@,]+@…`, and `[[:space:]]` covers tab/newline/CR but not ESC (`\033`), so an
  `ADMIN_EMAIL` / `STYRMAN_ALLOWED_EMAILS` / `SUPERADMINS` / `GOOGLE_CLIENT_ID` value carrying
  cursor-movement sequences validates, round-trips through `edit`, and paints the root
  operator's terminal at the exact moment they are deciding what to change. Same trust boundary
  Phase 1 and Phase 3 named (whoever can write the checkout, which on this host includes agents
  running as the operator). Remediation: `_shown="$(printable "${_default}")"` in `ask`, and
  add `[:cntrl:]` to the character classes in `is_email` and the three secret-value rules.

- **N4 [low / confidence: high]** `deploy/styrman-ctl:843-845` (`cmd_help`) vs `:38-42` —
  **the tool documents five environment knobs it now refuses to start with.** `help` still
  prints "Paths: STYRMAN_STATE_DIR, STYRMAN_DEPLOY_DIR, STYRMAN_ETC_DIR, STYRMAN_UNIT_DIR,
  STYRMAN_BIN_DIR, STYRMAN_SBIN_DIR, STYRMAN_TMPFILES_DIR (defaults: the production locations…)",
  while `:39` refuses all but the first two. Scenario: an operator relocating `/etc/styrman`
  follows the help text, gets "STYRMAN_ETC_DIR is a test seam, not a production knob", and the
  obvious workaround — the message all but names it, and the script's own comment at `:35`
  spells the variable out — is `STYRMAN_CTL_TEST=1`, which simultaneously unlocks
  `cmd_internal`'s root file-mutation and root-delete hooks (F7). A gate whose bypass is
  advertised on the next screen is a gate with a documented key. Remediation: delete the five
  refused names from the help text (and say the two survivors are knobs), or make the error
  name the supported alternative instead of the seam variable.

### Carried over, still open at `e34bf37`

- **F3 [medium / confidence: medium]** `deploy/styrman-ctl:498-503` (Phase 4 S3, partially
  fixed) — **the symlink refusal landed; the ownership and race problems did not.** The path is
  opened four separate times (`[ -f ]` `:499`, `[ -L ]` `:500`, `grep -c ''` `:502`,
  `head -1` `:503`), and nothing checks who owns it or whether its parent is world-writable.
  Phase 4's scenario is unchanged: `sudo styrman-ctl update --setup --allowed-signers-file
  /tmp/signers` on a host where another local account can create that name first, or can replace
  it between `:500` and `:503` — the one line that wins becomes `/etc/styrman/allowed_signers`
  (`:561`, mode 644), which is the entire trust root `deploy/update/styrman-update` uses to
  decide which signed tag root deploys. `is_signers_line` (`:478`) shape-checks the line but
  says nothing about whose key it is. Remediation as Phase 4 gave it: read the file **once**
  into a variable, assert `stat -c %u` is 0 or `SUDO_USER`, refuse a group/world-writable file
  or parent, and echo `ssh-keygen -lf` so the operator sees the fingerprint they just trusted.
  `--env-file` (`cmd_install:433`) still gets no symlink, mode or owner check at all, and it
  carries `GOOGLE_CLIENT_SECRET` and `CLOUDFLARE_API_TOKEN`.

- **F6 [medium / confidence: high]** `deploy/styrman-ctl:713-727` (`looks_like_ours`) with
  `:23-24` (Phase 4 S2, partially fixed) — **the marker test is satisfied by ordinary
  non-styrman trees, and the system-root list is exact-match only.** The deny list at `:716`
  names 24 paths; `guard_path:223` already rejects single-component paths, so the list's real
  contribution is `/var/lib`, `/var/log`, `/var/tmp`, `/usr/local`. Everything else one level
  down is unlisted — `/usr/lib`, `/usr/share`, `/var/lib/docker`, `/etc/systemd`, `/boot/efi`,
  `/root/<anything>`, `/home/<user>` — and reaching them needs only the marker:
  Measured, by lifting `guard_path` and `looks_like_ours` verbatim out of the pinned blob into a
  scratch probe that prints their verdict and **deletes nothing**:
  1. `STATE_DIR`: any directory containing a child named `homes`, `projects` or `users`. A
     scratch dir carrying only `projects/` → **ACCEPTED**; the same dir carrying only `foo/` →
     refused (so the marker does real work). `/home/<dev>` with a `~/projects` directory is the
     obvious live instance, and `~/projects/<dir>` is this platform's own in-guest convention
     (AGENTS.md, ADR-027). `STYRMAN_STATE_DIR=/home/dev` + `uninstall --level 3 --confirm
     <hostname>` is `rm -rf /home/dev`.
  2. `DEPLOY_DIR`: `.git` **and** `deploy/docker-compose.yml` — which is precisely the operator's
     own styrman checkout. Probed against the real path: `looks_like_ours
     /home/kesokaj/workspace/styrman-crew` with `DEPLOY_DIR` pointed at it → **ACCEPTED**.
     `cmd_install:429` explicitly refuses to install *from* the updater's clone; nothing refuses
     to *uninstall* the working checkout, and level 2 reaches `rm_tree "${DEPLOY_DIR}"` at `:767`
     behind the word `uninstall`, not the hostname. The loss includes uncommitted work and the
     untracked `deploy/.env`.
  3. The deny list's exact-match limitation matters less than it looks: `/var/lib/docker`,
     `/usr/lib` and `/usr/share` are refused anyway for want of a marker (probed: refused). The
     residual is (1) and (2), not the FHS one level down.
  4. The markers are tested with `-d`/`-f`, which follow symlinks, and `guard_path` canonicalises
     only `p` itself — a planted `p/projects` pointing at a directory satisfies `:720`. Only
     reachable by someone who can already write inside `p`, so it is the weakest of the four.
  `4c0869b` (out of range) widens this slightly: any **empty** directory now satisfies
  `STATE_DIR`/`ETC_DIR` with no marker at all. Deleting an empty directory is harmless, so the
  widening is small — but it means the marker is no longer an invariant, only a default.
  The lever is unchanged from Phase 4: a sudoers `SETENV`, `sudo -E`, or a root cron/systemd
  wrapper that exports the variable. Remediation: give the two surviving knobs a grammar
  (`^/[A-Za-z0-9._/-]+$`), require the last component to be `styrman`-shaped, and make the
  deny list prefix-based (`/usr/*`, `/home/*`, `/root/*`, `/boot/*`, `/etc/*` except
  `/etc/styrman`) rather than exact-match.

- **F1 [low / confidence: high]** `deploy/styrman-ctl:313-316` — the four values Phase 3 F1
  named besides `base`/`hn` are still unstripped on the human `status` path (`:348-349`):
  `disk` (`du -sh` field 1), `dockerv` (`docker version --format`), `katav`
  (`${KATA_DIR}/bin/kata-runtime --version`), `operator` (`stat -c %U "${STATE_DIR}"`). All
  four sources are root-equivalent or shape-constrained today (the docker socket is
  root-equivalent on this host; `KATA_DIR` is now a gated override), so the residual is a
  consistency gap rather than a live path — but it is the difference between "every value that
  reaches the screen is stripped" (an invariant one can check) and "the ones we remembered".
  `--json` is safe on all four: `json_str` (`:259-266`) escapes on both branches. One caveat on
  `printable` itself (`:258`, low confidence): `tr -d '\000-\037\177'` deletes C0 and DEL but
  not the C1 range, so a raw `0x9B` byte survives; few UTF-8 terminals treat it as CSI, which is
  why this is a note and not a finding.

- **F2 [low / confidence: high]** `deploy/styrman-ctl:663-672` — Phase 3 F3's remediation had
  two clauses; the table clause shipped (`:641-642`), the argv clause did not. The finding's own
  scenario runs verbatim: `styrman-ctl edit STYRMAN_ALLOWED_EMAILS=me@x,contractor@y` validates,
  writes both `.env` copies, recreates the stack and logs
  `edit: STYRMAN_ALLOWED_EMAILS set in 2 file(s)` — never the word admin, never the LAN toggle,
  never "platform superadmin on every app". The table note is also conditional on `SUPERADMINS`
  holding the exact scaffold literal (`:641`), so a host whose `SUPERADMINS` was ever set to a
  literal list loses the "(these are the admins)" hint entirely. Remediation: print the
  privilege sentence from the write loop, where both paths converge.

- **F5 [low / confidence: medium]** `deploy/styrman-ctl:390`, `:404` — **`safe.directory`
  answers an ownership question by waiving it, and it went to the wrong tree.** The Phase 3
  item was `git -C "${DEPLOY_DIR}"` with no ownership assertion; `:298-299` are unchanged and
  still carry neither an assertion nor `safe.directory`. What shipped instead marks the
  *operator's checkout* safe for root's git — that is the protection git added after
  CVE-2022-24765, and it exists because a repository's own config can cause the git process to
  execute commands. The practical trust delta here is small, because this script already runs
  `bash "${REPO}/deploy/host-setup.sh"` as root (`:437`) and `docker compose -f
  "${REPO}/deploy/docker-compose.yml"` (`:454`), so `REPO` is fully trusted already — which is
  the argument for the change, and should be the comment. Recorded so it is on the record that
  the mechanism is a waiver, scoped to `${REPO}` only (`-c` applies to this invocation, not to
  any config file), and that the DEPLOY_DIR item it was filed against is still open.

- **F7 [low / confidence: high]** `deploy/styrman-ctl:38-42` and `:816-825` — **the new
  override gate and the hook gate share one key, and the diff added a root delete to the hook
  set.** `__rm-tree` is new at `:823`. With `STYRMAN_CTL_TEST=1` in the environment (the
  `sudo -E` / sudoers `env_keep` / exported-in-a-root-shell lever both prior reports named),
  the same single variable simultaneously (a) re-enables `STYRMAN_ETC_DIR` &c. and (b) exposes
  `__env-set` (root rewrite of any regular file, forced to mode 600) and `__rm-tree` (root
  recursive delete). `looks_like_ours` does real work here — the target must already carry a
  marker the caller cannot create without root — so this is bounded, not open-ended. But the
  Phase 1 remediation's second clause (confine `__env-set` to `${ENV_FILE}`/`${CLONE_ENV}`)
  is still unshipped after three rounds, and `__rm-tree` widened the surface it guards.
  Remediation: confine `__env-set` as originally asked, and have `__rm-tree` refuse any path
  that is not under a `STYRMAN_CTL_TEST` scratch prefix.

- **Unchanged and still open from Phase 4** (re-verified against `e34bf37`, no new analysis):
  S5 `docker compose down` without `--volumes` leaves `caddy_data` — the ACME account key and
  every issued TLS private key — after a level-3 "IRREVERSIBLE" uninstall (`:743`). S6 the
  level-3 confirmation word is `hostname` output, and an empty hostname makes a bare Enter
  satisfy it (`:794`, `confirm_word:102`). S7 the `.env` backup at `:769` is `mv` with
  one-second resolution, no `-n`, and no warning that the file it leaves in the checkout holds
  every production secret. S9 the credential helper still interpolates `${ETC_DIR}` unquoted
  into a `sh -c` snippet (`:527`) — now held shut by the `:38-42` gate rather than by the
  construction, which is exactly the dependency F4/N4 puts pressure on. S10 every teardown step
  outside `drop_chain` still aborts the whole run on failure, with no "N of M steps failed"
  summary. F10 `docker logs --tail 30` to stderr on a failed bring-up (`:365`).

## Checked

- **Secrets** — no credential, token or key is added by the diff; no `.env` is committed
  (`git ls-files deploy/.env` is empty). The diff adds no new `echo`/`log` of a secret:
  `masked()` (`:608`) still prints `********` for all four `SECRET_KEYS`, and the new
  `printable` branch is the *non*-secret arm only. The new `SUPERADMINS` note (`:641`) discloses
  the allowed-email roster on the root operator's own terminal — PII, correct for a roster
  editor, already the case for `STYRMAN_ALLOWED_EMAILS`. Secrets-at-rest after the new verb is
  where the real exposure sits and it is unchanged: S5, S7, and now N2 (nothing is deleted at
  all).
- **Injection** — `drop_chain`'s unquoted `${rule}` (`:703`) is word-splitting by design and is
  clean against what is actually installed: verified read-only on this host that all four jumps
  render as `-A <parent> -i sdb-+ -j <chain>` (`iptables -S DOCKER-USER`, `-S INPUT`,
  `ip6tables -S INPUT`, `-S FORWARD`), which split into four tokens with no quote, no IFS
  surprise and no glob (`+` is not a bash glob). Two latent hazards worth naming rather than
  filing: the loop runs without `set -f`, so a future rule word containing `*` would be
  filename-expanded before reaching iptables; and `styrman-firewall.sh:123-128` documents that
  `iptables -S` *quotes* `-m comment --comment` values and that naive word-splitting hands
  iptables a token with literal quotes — `drop_chain` does the naive splitting the sibling
  script warns against, which today is safe only because the installed parent-chain jumps carry
  no comment. `grep -E -- "-j ${chain}\$"` and `sed -E "s/^-A ${parent} //"` interpolate only
  the four literal call-site arguments. `looks_like_ours`'s `case` patterns are quoted. The
  `printable` additions are `tr -d` on stdin — no new parsing boundary. `shellcheck -s bash`
  on the pinned blob: clean, 0 findings.
- **Authz/authn** — nothing here implements authentication; the authz surface is the `.env`
  roster, which is why F2 and F6 are rated where they are. `need_root` still gates
  `status`/`install`/`update`/`edit`/`uninstall`; `cmd_internal` deliberately does not (F7).
  The new `:38-42` gate runs before `main` and therefore before every verb including `help`.
- **Crypto** — nothing hand-rolled in the range. The release-trust decision stays with
  `deploy/update/styrman-update` (`git verify-tag` + `gpg.ssh.allowedSignersFile`); this diff
  only changes how the file that decision reads is *counted* (`:502`) and whether a symlink may
  supply it (`:500`). No session, hashing, JWT or OIDC code.
- **Dependencies** — no new dependency; no Go module or npm change in the range. The script
  gains no new external tool (`grep`, `tr`, `iptables` are already required). `govulncheck`
  **not run**: the range is one bash file with no Go source, and `deploy/vulncheck.sh` builds a
  pinned builder image via `docker build` (minutes) for a diff that changes no dependency —
  stated, not faked. `npm audit` not applicable (no `web/` file in range). `shellcheck` **was**
  run on the pinned blob and is clean.
- **GCP surface** — none. styrman is self-hosted; `.agents/docs/deploy/gcp.md` records "not
  applicable". No IAM binding, service account, `--allow-unauthenticated`, Secret Manager use
  or key file anywhere in the range.
- **BUG-035 (a mechanism reaches every caller, on the path it actually runs)** — applied to
  `drop_chain`, and it **passes** on target selection: the four `(parent, chain)` pairs at
  `:757-760` are exactly the four jumps `styrman-firewall.sh:62,79,103-104,109-110` installs,
  confirmed against the live host rather than by reading the script's words. It **fails** on
  execution conditions: the sibling's `-w 5` (added by an earlier review for the dockerd
  contention this teardown creates) did not travel with the rule (N1).
- **`.agents/rules/writing-files.md`** — `git diff --stat 108c4e4..e34bf37 -- deploy/styrman-ctl`
  is `+71/-15` on one file; the 15 deletions are the old inline iptables block and the three
  replaced lines, all visible in the diff. `git diff --numstat` shows numeric columns (not `-`),
  so nothing in the range is binary-opaque to a reviewer. No file was created or deleted.

## Not examined

- **Docs in and around the range** — `deploy/README.md`, `.agents/docs/specs/styrman/deploy.md`,
  `.agents/docs/deploy/{local,remote}.md` were not read (path-filtered out by the dispatch).
  N4 is the in-file half of what is probably a docs issue too: if any of them list
  `STYRMAN_ETC_DIR` &c. as operator knobs, they are now wrong.
- **`internal/config/styrman_ctl_test.go`** — read only for the harness variables it sets
  (`:79`, `:83-88`, `:371`) to confirm the gate's seam list matches what the test needs. The
  tests were not executed for this review; `e34bf37`'s own message says `b01914a` shipped two
  of them red and this commit fixes them (BUG-041), so a green run would describe `e34bf37`
  and nothing earlier in the range.
- **`deploy/update/styrman-update`, `deploy/host-setup.sh`** as wholes — read only for the
  comparisons this review needed. `deploy/firewall/styrman-firewall.sh` was read for lines
  30-130 (the `ipt()` wrapper, `ensure_jump`, the four jump installations, and the
  comment-quoting warning at `:123-128`).
- **No destructive command was run and no exploit was built.** The probes, all in the session
  scratchpad, were: read-only `sudo -n iptables -S DOCKER-USER/INPUT` and
  `ip6tables -S INPUT/FORWARD` on this host; `git cat-file`/`git diff --numstat` against the
  pinned blob; `shellcheck -s bash` on the pinned blob (exit 0, no output); a scratch `bash`
  script confirming that the new `case … ) [ x ] && note=… ;;` branch at `:640-643` does **not**
  trip `set -e` when the test is false (it does not — the `edit` table survives a `SUPERADMINS`
  that is a literal list); a scratch reproduction of `cmd_uninstall`'s plan-subshell control
  flow for N2; and a scratch probe that lifts `guard_path`+`looks_like_ours` verbatim from the
  pinned blob and prints their verdict for F6. The last two contain no `rm`, no `iptables -D`
  and no write outside the scratchpad.
- **The Phase 3/Phase 4 verification artifacts** were not treated as evidence; every
  "resolved" above was checked against the pinned blob. Plans, handoffs and the conversation
  were not read, per the dispatch.

## Verdict line

FINDINGS — 0 high, 4 medium (N1 `drop_chain` has no `-w` and flushes a chain it may not have
unhooked, silently removing sandbox isolation right after the level-1 docker teardown that makes
the race likely; N2 the round-4 marker check `die`s inside the plan subshell, blocking
`uninstall --level 2/3` entirely on a half-set-up host and leaving every secret in place; F3 the
allowed_signers trust root still has no owner check and is opened four times; F6 `looks_like_ours`
is satisfied by `/home/<dev>` with a `~/projects` directory and by the operator's own checkout,
and the system-root list is exact-match only), 6 low (N3 the `edit` re-prompt is unstripped,
N4 `help` advertises the five refused seam variables, F1 four `status` values still raw,
F2 the argv `edit` path names no privilege, F5 `safe.directory` is a waiver and went to the
wrong tree, F7 one variable keys both the override gate and the new `__rm-tree` root delete),
plus Phase 4's S5-S10 unchanged. Of the eight items the dispatch names: three resolved
(`drop_chain`'s target selection — verified against the live host's four jumps; `grep -c ''` —
measured; the seven-variable test-seam gate), four partial (control-byte stripping, the admin
roster, `looks_like_ours` + the system-root list, symlinked secret files), one open and
inverted (`safe.directory`). Nothing in the range should block a release on its own; N1 and F6
are the two I would fix before the next `uninstall` is run in anger.
