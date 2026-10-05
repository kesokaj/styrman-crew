---
owner: security
date: 2026-10-05
---
# Security review — project-resources   (2026-10-05)

Range reviewed: 58ddcd3~1..3b67939 (58ddcd3 Phase 1, f112e98 Phase 2, 3b67939 Phase 3), `-- ':!web/dist' ':!.agents/docs'`. Line numbers are those at 3b67939. The working tree changed while this review ran; see Not examined.

## Status: COMPLETE

## Verdict: FINDINGS

Worst problem: every project VM and every setup VM, for every user, is now created at the size of the whole host. That means all host vCPUs, and a memory limit that, with Kata's 2 GiB base added, equals all host RAM. Any project owner can keep it there: there is no admin gate, no per-user or host budget, and the operator's old cap is gone. One tenant's ordinary workload can therefore drive the host out of memory. Host-wide reclaim then stalls every tenant and the shared daemons, and the host OOM killer may kill another tenant's VM. That breaks "each user fully isolated" (REQUIREMENTS.md:11) on availability. The risk is worse because memory reclaim, the safety setting this design relies on, is applied only by a manual `host-setup.sh` re-run that nothing enforces. On this production host it is OFF today (`reclaim_guest_freed_memory = false`). The release that turns on whole-host sizing is committed but not yet tagged (newest tag v1.41.1). The authz on the new endpoint is sound, and the entrypoint's symlink/TOCTOU concern does not cross a boundary (see Checked).

## Findings

- **[medium / confidence: high (mechanism), medium (which process the OOM killer picks)]** `internal/orchestrator/resources.go:105-129` (WantSizing: CPUs and MemoryGiB of 0 become host CPUs and host RAM − 2 GiB), `internal/orchestrator/resources.go:48-54`, `internal/orchestrator/orchestrator.go:837` (the setup VM is always whole-host), `internal/orchestrator/orchestrator.go:1157` (create), `internal/orchestrator/orchestrator.go:384-391` (every stopped container from before ADR-083 is rebuilt at the new size on its next start), `internal/httpapi/resources.go:150-161` (any owner may set 0, meaning the whole host) — **cross-tenant memory exhaustion by default.**
  - **Before and after:** before the range, an unset size meant Kata's 1 vCPU / 2 GiB, and the operator could cap it with STYRMAN_VCPUS / STYRMAN_MEMORY_MB. Now the default is the whole host. There is no aggregate budget. Raising a project to "unlimited" needs no admin, unlike enabling LAN (`internal/httpapi/network.go:111`, `requireAdmin`).
  - **Scenario:** one allowlisted user, at the default size with nothing configured, runs a memory-heavy job. Plain file I/O is enough: the guest page cache grows to fill a whole-host-sized VM, and free page reporting does not hand it back, because it reports only pages on the guest's free lists.
  - **Effect:** host memory runs out. Host-wide reclaim then stalls every tenant's VM and the shared dockerd, containerd and styrman server. Finally the OOM killer kills the process with the highest score. That is usually the largest cloud-hypervisor process, which need not belong to the tenant who caused the pressure. If their oom_score_adj allows it, it can also be the shared daemons.
  - **What ADR-083 says:** it accepts overcommit, but frames exhaustion as "ten projects all touching most of their RAM at once" (`.agents/docs/architecture/DECISIONS.md:2849`). It does not record that one project at the default size can do it alone.
  - **CPU:** every VM gets all host vCPUs, so a busy tenant takes CPU in proportion to its busy vCPU threads. That degrades other tenants; it does not deny them.
  - **Remediation:**
    - Restore an operator-set ceiling and default, and have WantSizing clamp to it. This can be the legacy env read as a cap, or an admin runtime setting. The default should sit below the host, for example host RAM divided by the expected number of concurrent projects.
    - Put 0 (unlimited), and anything above the default, behind `requireAdmin`, as the LAN enable already is.
    - Give dockerd, containerd and the server a protective `OOMScoreAdjust` so the OOM killer never chooses the shared daemons.
    - Evaluate Kata `sandbox_cgroup_only = true` so each VM's host-side memory is accounted.
    - At minimum, add the single-project case and the OOM-victim behaviour to ADR-083's consequences.

- **[medium / confidence: high]** `deploy/lib/kata-config.sh:40-69` with `deploy/host-setup.sh` (the only path that enables reclaim), `.agents/docs/architecture/DECISIONS.md:2846` (the self-updater was explicitly not given the Kata config) — **the safety precondition ships separately from the change that depends on it, and nothing enforces it.**
  - **Observed on this host (dave, production):** `/opt/kata/share/defaults/kata-containers/configuration-clh.toml` has `reclaim_guest_freed_memory = false` (mtime May 19). Nothing reads or checks it:
    - `internal/` and `cmd/` mention reclaim or configuration-clh only in comments.
    - `deploy/update/styrman-update` has no Kata step.
  - **Scenario:** a release containing f112e98 is installed (by the self-updater, or `docker compose up`) before `host-setup.sh` is re-run.
    - Every project is rebuilt at whole-host size on its next start, and still at most one idle-stop later for running ones.
    - Reclaim is off, so "every VM keeps whatever it once touched" (`deploy/lib/kata-config.sh:43-46`).
    - The first finding then becomes the steady state rather than an edge case.
  - **GCP VM:** the same applies there. BUG-089 says it ran QEMU, while the sizing arithmetic (`internal/orchestrator/resources.go:17-35`) assumes CLH's 2048 MiB base and 128 MiB hotplug blocks.
  - **Remediation (any of these):**
    - Gate the release on a host-setup generation stamp: a root-owned file written by `host-setup.sh` that the updater checks before it switches versions.
    - Or mount configuration-clh.toml read-only into the server, and fall back to a bounded default while reclaim is off.
    - And add "re-run host-setup.sh on every host" as an explicit step in the versioning.md release procedure for this release.

- **[low / confidence: high]** `internal/config/config.go:238-240`, `internal/config/config.go:327-335` — **removed operator caps fail open.**
  - **Scenario:** an operator set STYRMAN_VCPUS / STYRMAN_MEMORY_MB to bound tenants. On upgrade the values are ignored, with only a WARN on stderr. `orchestrator.go:384-391` then rebuilds each stopped project at whole-host size.
  - **What this is:** a configured bound replaced by the maximum.
  - **Exposure:** tracked deploy config (deploy/, image/, .agents/docs/deploy) does not set either variable. The live impact depends on untracked `.env` files, which this review did not read.
  - **Remediation:** honour a still-set legacy value as a ceiling, clamped in WantSizing, or refuse to start until it is removed. Do not widen silently.

- **[low / confidence: medium]** `internal/httpapi/resources.go:192`, `image/entrypoint.sh:397-401` — **Docker disk is user-settable to 500 GB per project, with no check against host capacity.**
  - **No capacity check:** nothing compares the request to host capacity (the GCP VM's disk is 300 GB). The entrypoint only WARNs when space is short, then grows the file anyway. The file is sparse, so the cost lands when the guest writes.
  - **Incremental risk is small:** a guest can already fill its own home and workspace bind mounts, which have no quota (no quota mechanism found in internal/ or deploy/).
  - **Impact if filled:** the host filesystem under /var/lib/styrman fills. Every tenant's writes fail, and the server's per-user JSON stores cannot save.
  - **Remediation:** per-project filesystem quotas (XFS/ext4 project quotas on `homes/<u>/p/<uuid>` and `projects/<u>/<uuid>`), or cap maxDiskGB by statfs of the state dir.

- **[low / confidence: high — latent]** `deploy/lib/kata-config.sh:26-28` — **`kata_shim_wrapper` interpolates its `prefix` argument unquoted into a script that root executes.**
  - **Where it runs:** the script is `/usr/local/bin/containerd-shim-mvm-v2`, which containerd runs as root on every sandbox start.
  - **Not exploitable today:** the only production caller passes the constant `/opt/kata`, and the test passes a mktemp path.
  - **Future risk:** a caller that passed a prefix containing spaces or shell metacharacters would plant root-run code. A configurable Kata location is one way that could happen.
  - **Remediation:** validate the prefix against `^/[A-Za-z0-9._/-]+$` and refuse anything else, or emit it with `printf %q`.

## Checked

- **Secrets:**
  - I scanned the 2,204 added lines of the range for key, token, password, private-key and provider-token shapes. Result: 0 matches (grep rc 1). A control line matched (rc 0), so the scan works.
  - No secret-shaped paths are in the range.
- **Injection / input parsing:**
  - The PUT body decodes into `*int` fields. Type errors and missing fields return 400.
  - The global guard caps every non-GET body at 1 MiB and rejects a cross-origin `Origin` (`internal/httpapi/server.go:682-708`).
  - Bounds: cpus is 0 or 1..host, memoryGiB is 0 or 1..ceiling, and diskGB runs from the floor to 500, checked under the project lock. Negatives are refused, and the store is written only after validation.
  - The ID that reaches Docker comes from the store (getProjectFor), not from the client.
  - `$((DISK_GB * …))` (`image/entrypoint.sh:394`) evaluates an env value. That is safe only because the server writes it with `strconv.Itoa` and the guest cannot change its container env. It would become a bash arithmetic-injection sink if DISK_GB were ever read from a guest-writable source.
  - `kata_clh_reclaim`'s sed is anchored.
  - host-setup.sh sources `lib/` from its own checkout, so there is no new trust boundary. The GCP install verifies the tag signature first.
- **Authz / authn:**
  - Both routes sit on the same authenticated mux as the other project routes. getProjectFor scopes to the caller's own store, so another user's project returns 404 for GET and PUT, exactly like an unknown ID. `TestResourcesIsolationAndDeleting` (`internal/httpapi/resources_test.go:195-212`) covers both cases.
  - The body is parsed before the ownership check, but the reply is the same 400 either way, so there is no existence oracle.
  - There is no admin cross-user path (rejected in ADR-083).
  - Every EnsureRunning caller (`server.go:343/985/1180`, `resources.go:234`, `network.go:230`) goes through runOptsFor. A RunOpts with no Resources refuses the start (fail-closed).
  - The missing admin gate on whole-host sizing is the first finding.
- **Entrypoint symlink/TOCTOU (dispatch concern): no finding.**
  - The guest user has NOPASSWD sudo (`image/entrypoint.sh:56`), so guest root and the guest user are one principal.
  - `${IMG}` resolves inside the guest over virtio-fs, so a planted link cannot reach host paths.
  - The host never opens the image, and the explorer refuses `.dockerd` (`internal/httpapi/explorer.go:125`).
  - e2fsck and resize2fs parsing a guest-crafted image run inside the VM, and the VM is the boundary.
  - `truncate` already drops privileges with runuser.
- **Crypto:** nothing touched; nothing hand-rolled.
- **Dependencies:**
  - No go.mod, go.sum or package*.json change. The new import, `docker/docker/api/types/system`, comes from the existing module.
  - `govulncheck ./...` on the local go1.25.0 toolchain exited 3:
    - 29 stdlib advisories, all fixed at or below go1.25.13. The shipped server binary is built with the pinned golang:1.25.13 builder (`deploy/Dockerfile.server:9`), so these come from the local toolchain, not the shipped binary.
    - 2 advisories in docker/docker v28.5.2+incompatible: GO-2026-4883 and GO-2026-4887. Both are daemon-side (plugin privilege, AuthZ plugin bypass), with "Fixed in: N/A". This version was already in use before the range, and the reported call paths come through package init and older client calls. The range did not introduce them.
  - The project's own `deploy/vulncheck.sh` was not run.
  - `npm audit --omit=dev` (web/): found 0 vulnerabilities.
- **GCP surface:** no IAM, deploy/gcp or Cloud Run file is in the range.
- **Kata hypervisor switch:**
  - I compared configuration-clh.toml with configuration-qemu.toml on this host: enable_annotations, seccomp, sandbox_cgroup_only, guest seccomp, valid paths, virtiofsd args, debug console and selinux.
  - They are identical except that CLH sets `disable_seccomp = false`, so the VMM's seccomp is on, and the nvdimm setting differs. No isolation regression.
  - The wrapper is written via mktemp in the same root-owned directory and then renamed, and it is checked by content on every run.
  - `shellcheck -S warning deploy/lib/*.sh` is clean (rc 0).
- **Info exposure:**
  - GET exposes the host's CPU count and RAM ceiling to tenants. A whole-host guest sees the same through nproc and /proc/meminfo, so this is no finding.
  - Docker daemon error text can reach the client on a 503 (`resources.go:76`). That is an existing pattern of low value.

## Not examined

- **Concurrent edits:** uncommitted working-tree edits appeared while this review ran: `image/entrypoint.sh` (+7/−3 against HEAD), `AGENTS.md`, `.agents/docs/*`, `integration.sh` and `web/e2e/run.mjs`. They are outside the range and were not reviewed.
- **Excluded by the dispatch:** `web/dist` and `.agents/docs`.
- **Test files not read in full:**
  - Go: resources_start_test.go, the rest of resources_test.go, store_resources_test.go, fakedocker_test.go, seam_test.go and removed_env_test.go.
  - Grep only: kata_config_script_test.go and kata-config-test.sh.
  - Web: tests not read.
- **Frontend:** ResourcesDialog.tsx, Sidebar.tsx, settings.tsx and resources.ts were grepped for unsafe sinks (none found) but not read line by line.
- **Not measured on the host** (both bear on which process the OOM killer picks, in the first finding):
  - Where Kata places the VMM's host-side cgroup with `sandbox_cgroup_only = false`.
  - The oom_score_adj of dockerd, containerd, the server and cloud-hypervisor.
- **Not run:** `deploy/vulncheck.sh` (the pinned-builder govulncheck). The Caddy ingress module was not scanned; the range does not touch it.
- **Not exercised:** the fresh-host path (GCP VM).
- **Not read:** the untracked `deploy/.env`, which decides the live impact of the removed-caps finding.

## Outcome
(main agent, 2026-10-05)
1. **Medium, whole host by default, no admin gate: rebutted, the operator's explicit decision.** The operator: "I dont want to limit cpu or ram. a workspace can take the full machine if it can", and "Default should be unlimited". Owner editing was approved (plan D1, D21). Users are ≤ 5 (REQUIREMENTS Scale Targets). Ten idle full-size VMs used 11.7 GB on the NUC (ADR-083). The cross-tenant noisy-neighbour risk is real under heavy simultaneous load, and goes back to the operator as a follow-up (an admin-only cap, or a per-user budget).
2. **Medium, reclaim depends on a manual re-run: accepted, plan D28.** The re-run is in the release notes, `deploy.md`, `remote.md` and `gcp.md`. On dave it is the first post-release step, and it needs the operator's root permission per request.
3. **Low, removed env vars only warn: by design (plan D5).** Neither is set on dave or in the compose file.
4. **Low, no host-capacity check on 500 GB: same as the code review's finding 2.** Deferred to the operator as a follow-up.
5. **Low, unquoted prefix in the generated wrapper: rebutted.** `kata_shim_wrapper`'s prefix comes only from `host-setup.sh`'s literal `/opt/kata` (root-run, not user input). Quoting it would change the wrapper's bytes and rewrite dave's working wrapper for no gain.
