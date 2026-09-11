---
owner: reviewer
date: 2026-09-11
---
# Review — cold-read of `.agents/docs/plans/2026-09-11-installer-ctl.md`   (2026-09-11)

Range reviewed: plan file `.agents/docs/plans/2026-09-11-installer-ctl.md` (no diff — nothing is implemented yet) plus the code surfaces it names: `deploy/host-setup.sh`, `deploy/firewall/install.sh`, `deploy/docker-compose.yml`, `deploy/.env.example`, `deploy/README.md`, `internal/config/firewall_script_test.go`, `.agents/docs/specs/styrman/{deploy,versioning}.md`, `.agents/rules/*`, `REQUIREMENTS.md`. `deploy/update/styrman-update` was skimmed for the interface it exposes (`STYRMAN_UPDATE_STATE_DIR`, `--force`) but not read line-by-line.

## Verdict: DESIGN NEEDS REVISION

The plan is unusually thorough for a wrapper of existing scripts, and most of the hard edges (ADR-052 path discipline, the path-guard on deletes, `--dry-run` as the test seam, secrets never echoed) are already decided well. The one finding that actually matters is a correctness bug baked into the design, not the prose: uninstall level 1's container-selection criterion (`label styrman.project`) does not match the setup-VM containers the same host also runs, so level 1 as specified leaves them running and can make its own `docker network rm sdb-*` step fail on a still-attached network.

## Gaps (must be decided before implementation)

1. **[high / confidence: high]** Uninstall level 1's container filter is under-specified and wrong for setup VMs. The plan (line ~253-254) says: "for each container `styrman-*` (label `styrman.project`): `docker rm -f`". But `internal/config/config.go:28-33` defines two disjoint labels — `LabelProject = "styrman.project"` for per-project containers and `LabelSystem = "styrman.system"` for the per-user setup VM (set at `internal/orchestrator/orchestrator.go:749`, `LabelSystem: "setup"`) — and `orchestrator.go:809-815` branches `if isSystem {...} else if proj := c.Labels[LabelProject]; proj != "" {...}`, i.e. a setup-VM container never carries `styrman.project`. It IS still named `styrman-sys-setup-<user>` (`config.ContainerName`, `config.go:448`) and its network IS `sdb-sys-setup-<user>` (matches the `sdb-*` glob the plan uses for network removal, `ProjectFromNetworkName`, `config.go:456-459`). So level 1 as written: removes the network glob correctly but never removes the setup-VM container that network is still attached to, so `docker network rm sdb-sys-setup-<user>` fails (network busy) partway through a step the plan calls idempotent-per-run. Must decide the container filter explicitly — `label styrman.project OR label styrman.system=setup`, or name-glob `styrman-*` outright — and state it in the plan text, not just the network glob.

2. **[medium / confidence: medium]** Ownership of the deploy clone and its `.env` is undecided and contradicts existing convention. `host-setup.sh`'s own printed instructions (`deploy/host-setup.sh`, the 8c heredoc) do `sudo chown -R root:root ${STYRMAN_DEPLOY_DIR}` right after cloning, then `sudo cp .../.env .../deploy/.env` (inheriting root ownership from the `sudo cp`). The plan's `update --setup` step 3 instead says `install -m 600 -o "${OPERATOR}"` for that same file, and step 2 (the clone) says nothing about ownership at all. This is a real behavior change from what's documented today (root-owned clone, unreadable/unwritable by the operator account) and the plan doesn't say whether that's intentional or an oversight. Needs an explicit decision before Phase 2.

3. **[medium / confidence: medium]** Uninstall level 2's "the tmpfiles entry" (line ~257) doesn't name a file, and the file it plausibly means doesn't exist on a current host. `deploy/firewall/install.sh` only *removes* an obsolete `${TMPFILES_DIR}/styrman-firewall.conf` left by ADR-065/066 hosts — a fresh install creates no firewall tmpfiles entry. The tmpfiles entry a fresh host actually has is `/etc/tmpfiles.d/styrman-ksm.conf`, written unconditionally by `host-setup.sh` step 4 (KSM host tuning) and never mentioned anywhere else in the uninstall list. Level 2 needs to name the exact path(s) it deletes, and decide whether host-wide KSM tuning (which is not styrman-specific — it merges pages for any workload) belongs in a styrman uninstall at all.

4. **[medium / confidence: medium]** `edit`'s `validate()` is only specified for the six must-fill keys, `CADDY_ADMIN`, and the computed-key refusal, but `edit`'s table exposes every key in `.env.example` (line 235), including several with real format constraints the plan never assigns a rule to: `STYRMAN_IDLE_TIMEOUT`/`STYRMAN_TRANSCRIPT_TTL` (Go duration strings), `DEFAULT_RPS` (numeric), `STYRMAN_PUBLIC_DOMAIN`/`STYRMAN_PRIVATE_DOMAIN` (hostnames), `STYRMAN_BROKER_ADDR` (host:port). Since `edit`'s re-apply step recreates the live stack, an unvalidated bad value here breaks a running host at the recreate step, not at prompt time. Needs either an explicit "no validation beyond non-empty for these" decision or per-key rules before Phase 3.

5. **[low / confidence: medium]** The Interfaces section defines `host-setup.sh`'s own `--user`/`$SUDO_USER` default but never states `styrman-ctl`'s own default for `OPERATOR` before it forwards `--user` to `host-setup.sh` (Invocation table only shows `[--user U]` as optional). A first implementer has to guess between "ctl defaults to `$SUDO_USER` itself" and "ctl requires `--user` explicitly."

## Readiness

Not implementation-ready. Questions a first-pass implementer would have to ask the author before writing code:
- Which label(s)/name pattern does uninstall level 1 use to select containers for removal — is the setup VM in scope for level 1 at all, and if so how (finding 1)?
- Should the deploy clone and its `.env` end up root-owned (matching `host-setup.sh`'s documented steps) or operator-owned as `update --setup` step 3 currently says (finding 2)?
- What exact path(s) does uninstall level 2's "the tmpfiles entry" delete, and is host-wide KSM tuning in scope for a styrman uninstall (finding 3)?
- What validation, if any, applies to the ~8 optional `.env.example` keys `edit` exposes beyond the six must-fill keys and `CADDY_ADMIN` (finding 4)?
- Does `styrman-ctl install` default `OPERATOR` to `$SUDO_USER` itself, or always require `--user` (finding 5)?
- (Minor, not blocking) Does the one-shot `GIT_ASKPASS` helper used for the initial `git clone` in `update --setup` step 2 also cover the `git -C ... fetch` branch for an already-existing clone, or does re-running `--setup` need the token file again by a different path?
- (Minor, not blocking) What exact command produces "the host's name as printed" for uninstall level 3's confirmation (`hostname`? `hostname -f`? `/etc/hostname`?)?

## Not examined

`deploy/update/styrman-update`'s full body (only its documented interface — `STYRMAN_UPDATE_STATE_DIR`, `--check`/`--force`, `host_converge`, rollback — was cross-checked against the plan, via the recon brief and grep, not read end to end), `deploy/README.md`'s current full text (only referenced for the quick-start pointer the plan proposes changing), `.agents/docs/specs/styrman/deploy.md` in full (only `versioning.md` and the recon brief's citations were read closely), and no code was written or run — this is a plan-only cold read.

design needs revision
