---
owner: bugs
date: 2026-09-22
id: BUG-073
status: fixed
area: deploy/gcp/vm.sh
---
# BUG-073: `vm.sh status` reported "not installed" against a fully installed, serving GCP host

## Symptom
2026-09-22, verifying the first successful GCP bring-up at v1.37.0:
`deploy/gcp/vm.sh status` printed `styrman not installed on styrman —
deploy/gcp/install.sh` and exited 1, against a host that was fully
installed and serving — `https://crew.gcp.styrman.xyz/` answered `302` to
the sign-in, both containers ran `styrman-server:1.37.0` and
`styrman-caddy:1.37.0`, and the updater's clone sat at `2c15ea4 (v1.37.0)`.

## Repro
`deploy/gcp/vm.sh status` against the live GCP VM, host in the state
described above.

## Root cause
`cmd_status` probed `test -x /usr/local/bin/styrman-ctl`. Nothing ever
installs `styrman-ctl` into PATH — checked on both hosts:
`/usr/local/bin/styrman-ctl` is absent on the production NUC and on the
GCP VM, while `/opt/styrman/deploy/styrman-ctl` (the updater's clone)
exists on both, and `~/styrman-crew/deploy/styrman-ctl` on the VM.
`deploy/host-setup.sh` installs only `styrman-update` into
`/usr/local/bin` (its line 304) plus a containerd shim symlink. The probe
could therefore never succeed on any host, so `status` could only ever
report "not installed" — right for the wrong reason on an empty VM, which
is why earlier runs looked correct. It also made the plan's E2E criterion
"`vm.sh status` exit 0" unreachable.

## Fix
Same day, `deploy/gcp/vm.sh`: `cmd_status` now asks the host which copy it
has — the first executable of `/opt/styrman/deploy/styrman-ctl`,
`$HOME/styrman-crew/deploy/styrman-ctl`, `/usr/local/bin/styrman-ctl` —
reports "not installed" only when none exists, validates the returned path
against exactly those shapes before it reaches a `sudo` command line, and
runs `sudo <path> status`, whose exit status becomes the verb's. Tests in
`internal/config/gcp_scripts_test.go` (`TestGCPVMStatusExitCodes`): no
copy → exit 1 and "not installed"; the clone copy with remote status
exiting 0 → exit 0; with it exiting 1 → exit 1; the home-checkout copy →
exit 0; a path the host was not asked about
(`/tmp/evil; rm -rf /`) → refused with "unexpected styrman-ctl path".
Must-fail verified by restoring the old `/usr/local/bin` probe
(`--- FAIL: TestGCPVMStatusExitCodes`), then restored from a byte copy
(`cmp` identical). Verified live afterwards: `deploy/gcp/vm.sh status`
against the GCP host exits 0 and prints the host's own status table
(server healthy, `styrman-server:1.37.0`, clone at `2c15ea4 (v1.37.0)`).

## Prevention
A check that asserts something is INSTALLED must probe a path something
actually creates — name the installer line that creates it, or ask the
host where it is; a probe that can never succeed reports the same answer
forever and reads as correct on exactly the hosts where the answer happens
to be right. The fix asks the host rather than hard-coding a guess, and a
value the remote returns is validated before it reaches a privileged
command line.

## Related
BUG-072 (same live bring-up: a check probing the name the caller intended
rather than the one the host claims). BUG-034 (silence from your own
instrumentation). BUG-040 (a green run against the wrong server).
