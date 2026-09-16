---
owner: bugs
date: 2026-09-16
id: BUG-064
status: fixed
area: deploy/update (styrman-update, styrman-update.service)
---
# BUG-064: harness-channel systemd units installed only on the deploy tick, so the tick that ships them runs the OLD updater and never installs them

## Symptom
In the unreleased harness-channel feature (plan
`.agents/docs/plans/2026-09-16-harness-channel.md`, ADR-073), found
2026-09-16 by the main agent reading `deploy/update/styrman-update` before
the release gate — not live, not by a test or a review. The two new units
`deploy/update/styrman-harness-update.{service,timer}` were installed on the
production host only by the updater's `self_install`, which runs only on
the tick that deploys a release (call sites ~lines 1013 and 1041), and that
tick executes the PREVIOUS updater binary, which does not know the new
files. After a v1.35.0 deploy the units would never have reached
`/etc/systemd/system` until the following release or a hand-run
`host-setup.sh`; `styrman-ctl status` would have said "not set up" and
`systemctl is-enabled styrman-harness-update.timer` would have failed,
silently.

## Repro
Not reproduced live; found by reading. `git show 43dc991:deploy/update/styrman-update`
— `self_install` is called only after `deploy` succeeds (the "OK — <tag> is
live" path and the rollback path); `host_converge`, which runs on every
tick, installed only the firewall files and merely ENABLED the harness
timer when its unit file already existed.

## Root cause
BUG-035's shape recurring: a step that installs a file was gated on the
tick that ships it, and a self-updating tool's deploy tick runs the OLD
binary, which cannot see the change that shipped it. The project rule
`.agents/rules/mechanism-reaches-every-caller.md` already says to make the
step unconditional or idempotent so the following tick catches up, never
conditional on the tick that installs it — the implementer applied that to
the ENABLE (`host_converge` enables the timer every tick) but not to the
INSTALL of the unit files themselves.

## Fix
`deploy/update/styrman-update` `host_converge`, commit after 3ede401: the
harness unit pair is installed from the verified tree on every tick when
present there and differing from the installed copies, `--check` reporting
drift only, then `systemctl daemon-reload`; the enable step keeps its
every-tick shape. The script-side default `/opt/styrman-harness` for
`STYRMAN_HARNESS_REPO_DIR` was removed (ADR-052: paths are pinned in units,
never defaulted in the script); `deploy/update/styrman-update.service` now
pins `Environment=STYRMAN_HARNESS_REPO_DIR=/opt/styrman-harness`. No test
tier covers `host_converge` (root-only); verification is the live-host
command after the v1.35.0 deploy's SECOND tick: `systemctl cat
styrman-harness-update.timer` and `ls /etc/systemd/system/styrman-harness-update.*`.

## Prevention
When a self-updating tool gains a NEW file that must land on the host, the
install of that file goes in the every-tick converge path, not only in the
post-deploy self-install — ask "which binary runs the tick that first sees
this file?" for the install AND the enable, and grep both `self_install`
and `host_converge` call sites. This is `.agents/rules/mechanism-reaches-every-caller.md`'s
rule applied to installs, not just enables/flags; the rule stands and
needs no new wording — the miss was applying it to one of two steps
(install vs. enable) on the same new file.

## Related
BUG-035 (same mechanism, its rule already exists and covers this exact
recurrence — `.agents/rules/mechanism-reaches-every-caller.md`), BUG-060
(third occurrence of the same family, noted in that rule's Related
section). Three records now share this rule (BUG-035, BUG-060, BUG-064);
it already lives in `.agents/rules/mechanism-reaches-every-caller.md` and
is actionable (grep both call sites) and generalizable (any self-updating
tool's new-file/new-flag rollout) — the miss here is that the rule was
read but not applied to BOTH steps of the same change, which the rule's
existing wording arguably already implies for a "unit section" scenario
(it says "flag, or default"; installing a whole new file was not named).
No new rule location recommended; the existing rule's wording could add
"including a brand-new unit file's install, not just its enable."
