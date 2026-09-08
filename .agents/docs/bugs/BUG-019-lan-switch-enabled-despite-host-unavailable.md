---
owner: bugs
date: 2026-09-08
id: BUG-019
status: fixed
area: internal/netpolicy, web/src (per-project LAN access, ADR-065)
---
# BUG-019: LAN switch showed enabled though the host firewall half was absent

## Symptom
With no status file present at all, the server correctly derives "off" for
a record whose `lanAccess` is off — but the UI's Switch control read that
"off" state as "the control is available and simply unset", so it rendered
an ENABLED switch even though the host half (firewall support) was entirely
absent and enabling would fail. Caught by the first browser e2e run before
review, not by the security or code review.

## Repro
`bash web/e2e/run.sh` against a build with no `STYRMAN_FIREWALL_DIR` status
file present; the LAN switch in Settings/LocalNetwork rendered interactive
instead of disabled-with-reason.

## Root cause
The UI inferred a capability ("can this host grant LAN access at all") from
a state value ("is LAN access on for this project") that is legitimately
"off" for a reason unrelated to capability — an absent status file and an
off-but-available record produce the same derived state, so the switch
could not tell them apart.

## Fix
The admin-facing reply now carries an explicit `host.available` field
separate from the per-project on/off state; the Switch is disabled (with the
reason shown) whenever `host.available` is false, rather than inferring
availability from the project's own state. In the working tree per the
e2e run; lands with the Phase 2 checkpoint commit that follows this record.

## Prevention
When a control's enabled/disabled state depends on a capability the server
already knows, expose that capability explicitly (a dedicated field) rather
than inferring it from a state value that is legitimately reachable through
more than one cause — the same principle as BUG-014's fail-open reporting,
applied to the switch rather than the chip.

## Related
`.agents/docs/plans/2026-09-08-per-project-lan-access.md`; BUG-014 (same
status-derivation surface, chip/reporting angle, security-found).
