---
owner: bugs
date: 2026-09-21
id: BUG-069
status: fixed
area: deploy/gcp (vm.sh, install.sh)
---
# BUG-069: unreachable ssh to the GCP VM read as "not installed" / "no /dev/kvm"

## Symptom
2026-09-21 about 20:03 UTC: `deploy/gcp/install.sh --version 1.36.0` on the
operator's NUC died at its own nested-virtualization gate with
`install.sh: ERROR no /dev/kvm on styrman: nested virtualization is off or
unsupported for GCP_MACHINE_TYPE=n4-standard-8 …`, about 90 seconds after a
manual `deploy/gcp/vm.sh ssh -- 'test -e /dev/kvm; echo kvm_rc=$?'` had
printed `kvm_rc=0` on the same VM. `vm.sh status` at the same time printed
`styrman not installed` (exit 1).

## Repro
`deploy/gcp/vm.sh ssh -- 'test -e /dev/kvm'` from the NUC printed
`ssh: connect to host 34.51.142.50 port 22: Connection timed out` (exit
255). From the NUC, ICMP to the VM worked, port 80 gave "Connection
refused" (reachable, nothing listening), port 22 gave "Connection timed
out" (dropped). On the VM (reached via `gcloud compute ssh …
--tunnel-through-iap`): `systemctl is-active ssh` active, `ss -ltn` showed
0.0.0.0:22 listening, `iptables -S` only ACCEPT policies, uptime 3 min, no
reboot in the serial console; a hairpin `</dev/tcp/34.51.142.50/22` from
the VM itself was open.

## Root cause
GCP's firewall log for rule `allow-ssh-tcp` (tcp:22 from 0.0.0.0/0, logging
enabled) showed the NUC's public IP ALLOWED at 20:01:08–20:01:29 UTC (the
create wait loop plus the manual check) and nothing from it afterwards,
while internet scanners 77.239.124.210 and 176.248.27.62 reached port 22
ALLOWED at 20:02–20:05, and sshd logged "Connection closed by
authenticating user root 77.239.124.210 … [preauth]" at 20:05:26 — four
minutes after boot. The NUC→internet path (home router 192.168.5.1 or the
ISP; device unknown) dropped port 22 to that address after the burst of
five connections in 21 seconds; not GCP, not the VM.

Two project-code causes compounded the outage into a misdiagnosis:
`vm.sh status` reported any failed ssh as "styrman not installed", and
`install.sh`'s KVM gate reported any failed ssh as "no /dev/kvm" — both
diagnosed a condition through a transport they had not checked. A third,
contributing design cause: port 22 was exposed publicly
(`GCP_TAGS=http,https,ssh`) although the project's `allow-iap` rule
(35.235.240.0/20) already allows IAP TCP forwarding, so the public port was
unnecessary and was scanned and probed within four minutes of boot.

## Fix
`deploy/gcp/lib.sh` gained `ssh_reachable` (one `true` over ssh) and
`ssh_hint` (the `gcloud compute ssh … --troubleshoot` line); `vm.sh status`
prints `ssh to <instance> unreachable — <hint>` and exits 1 before any
styrman check; `install.sh` dies `ssh to <instance> failed — <hint>` before
the KVM gate. ssh and scp go through IAP by default
(`GCP_SSH_VIA_IAP=1` in `deploy/gcp/vm.conf`, `--tunnel-through-iap` on
every `gcloud compute ssh`/`scp`), `GCP_TAGS=http,https` (no public port
22), and the live instance's `ssh` tag was removed (`gcloud compute
instances remove-tags styrman --tags ssh`). Test
`TestGCPSSHUnreachableIsNamed` in `internal/config/gcp_scripts_test.go`
(fake gcloud `FAKE_SSH_RC=255`) asserts status says "unreachable" not "not
installed", and install says "ssh to styrman failed" not "/dev/kvm", naming
`--troubleshoot`. Must-fail verified both ways (deleting `install.sh`'s
`ssh_reachable || die` line, and `vm.sh`'s status ssh block, each producing
`--- FAIL: TestGCPSSHUnreachableIsNamed`; both restored byte-identical,
suite green). `deploy/gcp/vm.sh status` over IAP afterwards correctly
printed `instance styrman RUNNING 34.51.142.50` then `styrman not
installed` (exit 1).

## Prevention
A check made over a remote transport must name the transport's failure
before the condition's: probe reachability first, and never let a message
attribute an unreachable host to the thing it was about to test. A
management port is not exposed publicly when the platform offers a tunnel
(IAP here) — this VM's port 22 was reached by scanners within four minutes
of boot.

## Related
Same family as BUG-034 (silence from your own instrumentation) and BUG-040
(a green run against the wrong server): a signal read as evidence of the
thing under test when it actually measured something else.
