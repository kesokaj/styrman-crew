---
owner: main
date: 2026-09-16
---
# BUG-064: the harness unit pair is installed on every updater tick (2026-09-16)

## Summary

Found by reading `deploy/update/styrman-update` before the v1.35.0 gate: the two new units `styrman-harness-update.{service,timer}` reached `/etc/systemd/system` only through `self_install`, which runs only on the tick that deploys a release — and that tick executes the previous updater, which does not know them. After the deploy the timer could never have been enabled (BUG-035's shape, applied to the enable but not to the install). `host_converge` now installs the pair from the verified tree on every tick when it differs, like the firewall files; the crew unit pins the clone's path.

## Type

Bug fix (unreleased code; BUG-064)

## Changed

- `deploy/update/styrman-update` — `host_converge`: the harness unit pair installed from `deploy/update/` at the verified `SELF_HEAD` when present there (a pre-ADR-073 tree skips it) and differing from the installed copy, `--check` reporting the count of differing files and installing nothing, `systemctl daemon-reload` on a change; the timer enable that follows now requires `STYRMAN_HARNESS_REPO_DIR` set (the script-side default `/opt/styrman-harness` removed, ADR-052).
- `deploy/update/styrman-update.service` — `Environment=STYRMAN_HARNESS_REPO_DIR=/opt/styrman-harness` with the comment.
- `.agents/docs/specs/styrman/deploy.md` — the harness-channel subsection's unit-installation sentence rewritten to say so.
- `.agents/docs/plans/2026-09-16-harness-channel.md` — D24.
- `.agents/docs/bugs/BUG-064-*.md`, `INDEX.md` — the record (`bugs` agent).

## Verification

`bash -n` and `shellcheck -S warning deploy/update/styrman-update` clean; `systemd-analyze verify deploy/update/styrman-update.service` rc 0; `go test ./internal/config/ -count=1` ok (the two updater test files drive the script; `host_converge` itself is root-only and has no test tier — the live-host check after the v1.35.0 deploy's second tick is `ls /etc/systemd/system/styrman-harness-update.*`). The diff (33 lines over two files) was read by the main agent; the whole-feature `reviewer` and `security` dispatches over `567b81f..43dc991` were already running and do not cover this commit — the release gate's reviewer row covers it.

## Agents dispatched

`bugs` (BUG-064).
