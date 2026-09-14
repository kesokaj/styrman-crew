# A mechanism reaches every caller, on the path it actually runs (BUG-035)

`styrman-firewall.service` gained `[Install] WantedBy=multi-user.target`, but
neither `install.sh` nor the updater's `host_converge` ran `systemctl enable`
for it — the mechanism was defined but not propagated to every caller that
must invoke it. Fixed same day, then recurred same day: the fix added the
`enable` call to `host_converge`, but inside the `if [ "${changed}" = "1" ]`
guard. The tick that installed the new unit file ran the PREVIOUS updater
binary (no enable line yet); the next tick, running the new updater, saw
nothing changed and skipped the block — `is-enabled` still read `disabled`
after v1.27.0 shipped.

- When adding a unit section, flag, or default: grep every
  installer/caller that must honour it and list them explicitly in the
  change entry. A guard living only in one caller's default argument is
  invisible to a new caller supplying its own argument — restate it, do
  not rely on inheritance.
- When the step is added to a self-updating tool's converge/install path:
  ask which VERSION of the tool executes the FIRST run after the change
  ships. A self-updating tool's install tick runs the OLD binary — a step
  gated on "did something change this tick" can never see the change that
  shipped it. Make the step unconditional or idempotent (`systemctl
  enable` is both) so the following tick catches up, never conditional on
  the tick that installs it.
- Verify with the exact live-host command the bug record names
  (`systemctl is-enabled <unit>`), never by reading the unit file's text
  or trusting a green install script — same discipline as BUG-034.

## Related

BUG-035 (both the original defect and its same-day recurrence).

BUG-060 (2026-09-14): the credential-broker attach reached the create,
start, mode-toggle and port-preview paths but not the terminal-WebSocket
boot callback — the path every project page opens with — so after every
server restart the first terminal of a stopped project had no credentials.
Before adding or moving a step a booted VM depends on, grep every
`EnsureRunning(` caller in `internal/httpapi` and name each in the change.
