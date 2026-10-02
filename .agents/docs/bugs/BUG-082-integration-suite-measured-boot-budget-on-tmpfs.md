---
owner: bugs
date: 2026-10-02
id: BUG-082
status: fixed
area: .agents/docs/testing/styrman/integration.sh (boot-budget check)
---
# BUG-082: the integration suite measured the boot budget on tmpfs and could not see a 13-second cost

## Symptom
Against an image whose harness install (`sh-agent v2.3.2 install`, 1,788 files) ran in front of `/run/styrman/ready`, the suite printed `PASS boot to ready within R1's 15 s hard limit (measured 3.26s; soft target 5 s)` and `passed=112 failed=0 skipped=3`. The same suite with its state dir on `/var/tmp` printed `FAIL boot to ready within R1's 15 s hard limit (measured 16.16s; soft target 5 s)`.

## Repro
Run the suite with `STATE` under `/tmp` and under `/var/tmp` against the same image; `findmnt -no FSTYPE -T /tmp` prints `tmpfs` on this host.

## Root cause
The suite's state dir was `mktemp -d /tmp/styrman-itest-XXXX`, and `/tmp` is tmpfs on this host. The install syncs every file it writes, which costs 0.06 s on tmpfs and 12.8 s on a disk-backed home in a microVM; production homes are on ext4. The check asserted a budget on storage that hides exactly the cost it exists to catch.

## Fix
Ships in the release after v1.39.0.
- `integration.sh`: state dir `mktemp -d /var/tmp/styrman-itest-XXXX`; the boot-budget check names the state dir's filesystem type in its label and FAILS, not SKIPs, when it is tmpfs.
- Decision: ADR-078 (the install itself moved behind ready; boot-to-ready measured 3.01 s and 3.20 s on disk afterwards).

## Prevention
A timing check runs on the storage production uses; a suite that asserts a budget names what it measured on and fails when it cannot measure. A green budget from tmpfs is silence, not evidence.

## Related
BUG-034 (silence from your own instrumentation), BUG-040 (a green run against the wrong server), `deploy/local.md:43`, `reviews/2026-09-22-preflight-v1.38.0.md:52`.
