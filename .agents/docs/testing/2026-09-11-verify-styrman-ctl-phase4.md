---
owner: main
date: 2026-09-11
---
# Verification — styrman-ctl, Phase 4 (`uninstall`, docs)   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after the phase commit.

| Step | Command | Result |
|---|---|---|
| Lint | `shellcheck deploy/styrman-ctl` | clean (SC2030/2031 disabled on the deliberate plan-printing subshell) |
| Unit | `go vet ./... && go test -race -count=1 -run StyrmanCtl ./internal/config/` | 13 tests green (2 new: uninstall plans per level, uninstall confirmations; the Phase 1 "uninstall is a stub" assertion removed) |
| Real, dry-run | `sudo deploy/styrman-ctl uninstall --level 3 --dry-run` on the production host | the full plan printed in order: compose `down --remove-orphans` from `/opt/styrman` at 1.27.1, `docker rm -f` of the one (stopped) project VM, `docker network rm` of its `sdb-` network, the four units disabled, the IPv4 and IPv6 chain teardown, the four unit files + daemon-reload, the two binaries, the KSM tmpfiles entry, `rm -rf -- /etc/styrman`, `rm -rf -- /opt/styrman`, `docker image rm` of every `styrman-*` image (≈100 tags on this host), `174M /var/lib/styrman` then `rm -rf -- /var/lib/styrman`; "nothing executed (the word to type for level 3 is: dave)"; afterwards the two stack containers still up and `styrman-firewall.timer` still enabled |

PASS. Not verified: a real `uninstall` at any level (this host is
production), a real `install` on a fresh host, the level-1-then-install
round trip — the plan's fresh-host e2e is the operator's (or a nested-virt
VM's), recorded when run.

Observation, not this plan's: the host carries ≈100 `styrman-*` image tags
from earlier releases and verification builds; `uninstall --level 2` would
remove them all, and a prune of the old ones is an operator housekeeping
item (`docker image prune` does not touch tagged images).

## Security round 2 (Phase 2 report, `reviews/2026-09-11-security-styrman-ctl-phase2.md`) — dispositions in this phase's commit

- F1 (medium, `awk -v` decoded `\n` in a value): RESOLVED in Phase 3 (awk `ENVIRON`, backslash refused, test).
- F2 (medium, the one-shot askpass answered any host; no https assertion; inherited helpers not cleared): FIXED — the token now reaches git the way `host-setup.sh` 8c and the updater do: `-c credential.helper=` clears every inherited helper and an inline helper scoped to `credential.https://<origin host>.helper` reads the root-only token file; `https_origin` accepts only a plain `https://host/path` origin. Tests assert the scoped helper on both clone and fetch, the absence of any askpass, and that `ext::`, `file://` and an option-shaped origin are refused before any plan.
- F3 (low, origin as a bare positional): FIXED — `clone -- "${origin}"` plus the allow-list above.
- F4 (low, `fetch --tags` offered the token to whatever remote an existing clone had): FIXED — the existing clone's origin (normalized to https) must equal the expected origin or the run dies; test plants a clone pointing elsewhere.
- F5 (low, `__` hooks reachable): RESOLVED in Phase 3 (gated on `STYRMAN_CTL_TEST=1`).
- F6 (low, secret files read regardless of mode): FIXED — a group- or world-readable `--gh-token-file` is refused with "chmod 600 it first" (test); `--allowed-signers-file` with more than one line is refused instead of silently dropping lines.
- F7 (low, temp secrets cleaned by EXIT only): FIXED — the trap covers INT, TERM and HUP.
- F8 (low, `is_signers_line` unanchored): FIXED — anchored, one optional trailing comment field.
- F9 (low, `$`/`{`/`}`/backslash accepted in secrets that compose interpolates): FIXED — `validate` refuses them for the three secret keys (tests).
- F10 (info, `docker logs` tail on a failed bring-up): accepted — it is the operator's terminal; a unit-driven install is not a supported path.

After these: shellcheck clean; 13 tests green; the real `update --setup --dry-run` on this host still prints its nine-line plan (the clone's origin matched the checkout's).

## Security round 3 (Phase 3 report, `reviews/2026-09-11-security-styrman-ctl-phase3.md`, no high) — dispositions in the follow-up commit

- Medium (values from `.env` reached the edit table and the status URL unstripped): FIXED — `masked()` and the URL go through `printable`; test plants an escape sequence in `BASE_DOMAIN` and asserts the byte is gone from `status --json`.
- Medium (`$`/`{`/`}` accepted in the three secrets that compose interpolates): already FIXED in the Phase 4 commit (refused; tests).
- Medium (the table showed `SUPERADMINS`'s `${STYRMAN_ALLOWED_EMAILS}` reference, not the effective admin roster): FIXED — the row resolves the reference and says these are the admins; the `STYRMAN_ALLOWED_EMAILS` row says it also decides the admins while the reference stands.
- Open items closed too: root's `git` on the operator's checkout names it `safe.directory` (as the updater does); the path overrides other than `STYRMAN_STATE_DIR`/`STYRMAN_DEPLOY_DIR` are refused unless `STYRMAN_CTL_TEST=1` (the `sudo -E` lever the report named; test); the `__` hooks stay gated.
- Accepted: `is_email_list` does not normalize (the server's userstore does); argv flag placement is bash's usual.

After these: shellcheck clean; 15 tests green; the real `status` and `update --setup --dry-run` on this host unchanged in behaviour.

## Reviewer, Phase 4 (`reviews/2026-09-11-review-styrman-ctl-phase4.md`, REQUEST CHANGES) — dispositions

- HIGH (the chain-jump deletes named `-D PARENT -j CHAIN`, but the firewall script installs the jumps with `-i sdb-+`, so `-D` could never match and, without `|| true`, would abort levels 2/3 mid-teardown — after the level-3 word, leaving the state tree untouched): FIXED — `drop_chain` lists the parent chain's jumps with `-S`, deletes each exactly as listed, then flushes and deletes the chain, every step best-effort; the fakes now list the jumps the real way and the tests assert the exact `-D DOCKER-USER -i sdb-+ -j STYRMAN-FWD` (and IPv6) lines; the real level-2 dry-run on this host now shows those exact lines.
- LOW (`du -sh` ran for real during the plan preview): FIXED — wrapped in `run`; the plan lists it, the dry-run "nothing executed" test now checks it too.

## Security, Phase 4 (`reviews/2026-09-11-security-styrman-ctl-phase4.md`) — dispositions

- S1 (high, the chain-jump deletes): already FIXED at `becbd6b` (the report confirms).
- S2 (medium, `STYRMAN_STATE_DIR`/`STYRMAN_DEPLOY_DIR` could point level 3's delete at `/var/lib`): FIXED — `rm_tree` now also requires `looks_like_ours`: a state tree must contain `homes/`, `projects/` or `users/`, a deploy clone `.git` + `deploy/docker-compose.yml`, the etc dir `update/`, `allowed_signers` or `gh-token`, and a list of system roots (`/`, `/etc`, `/var`, `/var/lib`, `/opt`, `/usr`, `/home`, …) is refused outright; tests: a bare directory as the state tree and `/var/lib` are refused before any plan, the real state tree is planned.
- S3/S4 (medium, the signers file's line count missed a file without a trailing newline; no symlink check on the secret files): FIXED — `grep -c ''` counts the last line, a symlinked `--allowed-signers-file` or `--gh-token-file` is refused; test plants a second signer behind a missing newline.
- S8 (low, `-F` on a chain whose jump delete failed): accepted — every step is best-effort and the flush of a still-referenced chain is harmless; `-X` then fails loudly in the plan's own words.
- F4/F6/F8 partials: F6's owner check is the mode check plus the symlink refusal above (owner is the operator's own file by construction); F4's origin check is in; F8's anchor is in. F10 (info) accepted.

After these: shellcheck clean; 15 tests green; the real `uninstall --level 3 --dry-run` on this host still plans the three trees (they carry the markers) and nothing else changed.
