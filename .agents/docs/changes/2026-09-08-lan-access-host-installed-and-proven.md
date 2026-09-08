---
owner: main
date: 2026-09-08
---

# LAN access: host half installed, feature proven end to end   (2026-09-08)

The last step of ADR-065, and the one that had been waiting on a root
command. Closes the plan
(`.agents/docs/plans/2026-09-08-per-project-lan-access.md`): 44 of 44 boxes.

Full record with commands and outputs:
`.agents/docs/testing/2026-09-08-verify-lan-live.md`.

## What happened

`sudo bash deploy/firewall/install.sh`, run from the deploy clone
`/opt/styrman` at `v1.23.0` (`7bf0607`, worktree clean, installer
byte-identical to the reviewed file), dry-run inspected first. It wrote the
LAN definition — `enp114s0`, `192.168.86.0/24`, discovered from the default
route — and mounted the bounded spool tmpfs. The script and units were
already in place from `host_converge`, so those steps were no-ops on
identical bytes.

Then `STYRMAN_IT_LIVE_LAN=1 bash …/integration.sh styrman-runtime:1.23.0`:
**83 passed, 0 failed, 3 skipped.**

## The two things that could only be proven here

**The spool tmpfs reaches a running container.** The server started before
the tmpfs existed; its own `/proc/<pid>/mountinfo` afterwards shows the
mount, correctly owned and bounded, with no restart. That is what the
`rslave` propagation fix (BUG-016) was for — under the default `rprivate`
the server would have kept writing into the unbounded directory underneath,
where root never drains.

**The allowance actually works, and only where it should.** In one run, on
real hardware: a project with no marker cannot ping the LAN gateway; a
marked project can, within 5 s; the host itself stays unreachable from the
guest in both cases; and removing the marker cuts it again within 5 s. Until
now that path was reasoned from the chain, never observed.

Afterwards: no markers, no `styrman-lan:` rules, spool drained, only the two
production containers running.

## Recorded, not fixed

The opt-in live block races the production server's 2-minute marker sweep —
it plants a marker for a project only the test server knows about, and
production correctly deletes markers it cannot account for. It passed by
timing, not by design, and a collision would look like a feature failure
rather than a test artefact. In `TODO.md`.

## Still unautomated

A human flipping the switch in a browser once. Every layer beneath it is
covered: the API and its gates by handler tests and the integration block,
the reconciler by the namespaced suite and now by this live run, the
unavailable branch by e2e.

## Note on records

Written by the main agent rather than the `changelog` and `verifier` agents:
these are first-hand records of root commands on the production host, and
the `changelog` agent had by this point fabricated content in four separate
entries this session, each rewritten against `git diff` before commit.
