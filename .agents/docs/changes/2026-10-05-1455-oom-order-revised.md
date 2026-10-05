---
owner: main
date: 2026-10-05
---
# OOM order revised before v1.42.1's tag (ADR-085 supersedes ADR-084)   (2026-10-05)

Operator: "B go", the full OOM fix in v1.42.1. This corrects
`changes/2026-10-05-1433-oom-protection-and-review-follow-ups.md`, whose
`-1000` never shipped (the release was re-gated before its tag).

- **Why:** the v1.42.1 security review
  (`reviews/2026-10-05-security-v1.42.1-v1.42.0-2ba3f79.md`) found a **high**.
  At -1000 every child the server starts is unkillable, including the host-side
  `git` that reads guest-writable `.git/config` files; a 12.9 MB config cost 616
  MiB, measured, so a tenant could make the kernel kill everything else and
  hang the host. Also:
  - dockerd at -500 still died first;
  - more sudo token lines remained;
  - the Tier 2 stopped-VM gate passed when `docker ps` failed.
  The code review's medium agreed: ADR-084's claimed restart guard did not
  exist.
- **Compose:** `caddy` and `styrman` at `oom_score_adj: -999` (a tie with the
  VMs goes to the largest process, a VM), and `mem_limit` 1g / 4g (measured use
  about 40 / 9 MiB), so a leak is an in-container OOM kill that
  `restart: unless-stopped` recovers. `compose_oom_test.go` pins both.
- **Host-side git:** `internal/gitrepo` starts git, sets the child's
  `oom_score_adj` back to 0 (raising needs no privilege), then waits.
  `oom_test.go` was red against a stub.
- **dockerd:** `host-setup.sh` writes
  `/etc/systemd/system/docker.service.d/50-styrman-oom.conf` (-999, no start
  limit) through the new `deploy/lib/docker-oom.sh`. It runs `daemon-reload` on
  change and sets the running dockerd's score at once, with no Docker restart.
  `docker-oom-test.sh` (20 checks on a scratch dir) runs under `go test`
  (`TestDockerOOMScript`).
- **Reaper (code review medium):** `TestReapSetupVMsPassIsBoundedAsAWhole`
  covers the whole-pass bound; it took 401 ms against a 200 ms bound with
  per-VM bounds put back. The pass logs when it runs out of time and leaves the
  rest for the next tick.
- **Token off sudo command lines:** `remote.md`, `specs/styrman/deploy.md`
  (Bring-up, Rebuilding a host) and `host-setup.sh`'s FATAL hint now say
  `export GH_TOKEN` in your own shell, then `sudo --preserve-env=GH_TOKEN`, or
  use `--gh-token-file` / `/etc/styrman/gh-token`.
- **Tier 2:** the stopped-VM gate captures `docker ps` and fails if it fails.
- **Correction to the 1433 entry:** it says the young-VM reaper test was
  mutation-checked against the one-shot remove. That test never reaches a
  remove; only `TestReapSetupVMsWaitsOutAnExitingVM` was.
- An intermittent e2e git-pane 409 is recorded as an open bug (BUG-091).

Diff read by me; the re-gate reviews it.

## Learnings for the local host
After v1.42.1, the one `host-setup.sh` re-run also moves dockerd to -999.
Check with `cat /proc/$(pidof dockerd)/oom_score_adj` (expect -999) and
`cat /proc/$(docker inspect -f '{{.State.Pid}}' styrman-server)/oom_score_adj`
(expect -999).
