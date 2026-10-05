---
owner: main
date: 2026-10-05
---
# v1.42.1 work: the platform survives an OOM; v1.42.0 review follow-ups   (2026-10-05)

Operator: "go v1.42.1". This answers the v1.42.0 gate's accepted findings
(`reviews/2026-10-05-preflight-v1.42.0.md`, rows 1, 2, 4–8).

- **OOM protection (ADR-084).** `deploy/docker-compose.yml` runs `caddy` and
  `styrman` at `oom_score_adj: -1000`.
  - Project VMs run at −999 (cloud-hypervisor inherits it from containerd's
    shim) and are sized to the whole host. Before this, the server and Caddy
    at 0 would have been the kernel's first pick (security review of v1.42.0,
    finding 1, measured on dave).
  - −1000 is the only value below the VMs, and sshd already runs at it on dave.
  - Pinned by `internal/config/compose_oom_test.go`, which was red before the
    compose edit. `docker compose config` renders both. ADR-083's Consequences
    line about which process an OOM kills is superseded by ADR-084.
- **The token is off the sudo command line.** The post-release step in
  `deploy/remote.md`, `deploy/gcp.md` and `specs/styrman/deploy.md` is now
  `sudo bash /opt/styrman/deploy/host-setup.sh --user <operator>`;
  `host-setup.sh` reads `/etc/styrman/gh-token` itself (security finding 2).
- **`forceRemove` everywhere a running VM is removed** (BUG-090 follow-ups):
  - `ReapSetupVMs` has one deadline for the whole pass (`reapPassBound`, 1 min)
    instead of one per VM;
  - the pre-ADR-027 recreate's remove after a stop is switched over;
  - the `zombieRetryEvery` comment is corrected.
  - Tests: `TestReapSetupVMsWaitsOutAnExitingVM` and
    `TestReapSetupVMsLeavesAYoungVM`, possible now that the fake Docker's
    `ContainerList` filters by label behind an opt-in `list` flag, and
    `TestThePreADR027RecreateWaitsOutAnExitingVM`. Each was mutation-checked:
    putting back the one-shot remove fails it.
- **Tier 2:** the grow-retry check's host-side `truncate` now runs only on a
  stopped VM, on a regular non-link file whose `realpath` is its path, with
  `-c` (security addendum, low 1).
- **Docs** (the docs lens's stale spots):
  - `runtime-image.md`: the disk env row and boot step 8 now describe the grow;
  - `api.md`: `STYRMAN_DOCKER_DISK_GB` is the new-project default;
  - `deploy.md`: `nproc` when unlimited, and the OOM setting;
  - `testing/styrman/README.md`: Tier 1 now names the new test files and the
    vitest count (230 / 19).
- **e2e:** a forced-fail run of both Resources checks (see the verifier log
  for this release).

Diff read by me. Reviews and verification are this release's gate.

## Learnings for the local host
The OOM setting takes effect when the auto-updater recreates the containers
at v1.42.1; check with `cat /proc/$(docker inspect -f '{{.State.Pid}}' styrman-server)/oom_score_adj`
(expect −1000).
