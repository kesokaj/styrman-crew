---
owner: reviewer
date: 2026-10-05
---
# Cold read, round 3 — project-resources plan (2026-10-05)

Range reviewed: `.agents/docs/plans/2026-10-05-project-resources.md` (a plan file, not a diff). This file replaces round 2's report. Round 2's gaps (BUG-089 task, 10+ VM requirement, `/proc/meminfo` restatements, live-container PUT, required `Resources`, backfill) are closed in this revision; I did not re-litigate them.

Code surfaces read: `internal/orchestrator/orchestrator.go` (`:129-138`, `:196-460`, `:695-760`, `:1060-1110`, `:1205-1260`, `:1520-1560`), `docker.go:25-47`, `fakedocker_test.go`, `seam_test.go`, `internal/httpapi/server.go:80-160`, `network.go:155-215`, `wsboot_attach_test.go`, `internal/config/config.go`, `image/entrypoint.sh:340-420`, `deploy/host-setup.sh:95-115,255-296`, `deploy/update/styrman-update` (`host_converge`), `web/e2e/run.sh`, the live `/usr/local/bin/containerd-shim-mvm-v2` and `configuration-clh.toml` on dave, BUG-089, and the 2026-09-16 security review's ContainerUpdate low.

## Status: COMPLETE

## Verdict: REQUEST CHANGES (design needs revision; implementation not ready: one BLOCKING question)

The worst problem: the release order the plan relies on (D17, Phase 4 task "re-run `sudo deploy/host-setup.sh` on each host BEFORE the server release reaches it") cannot be followed on the GCP VM. The VM's deploy clone only advances through the signed-tag updater, and `host_converge` manages only the firewall files (`deploy/update/styrman-update:770-830`; D6 declines to extend it). So the new `host-setup.sh` reaches the VM only with the release itself, after which the updater has already put the new server on a host that may still run QEMU, whose sizing is unmeasured (plan line 109). Two further criteria use fixtures that do not produce the condition they claim. The code design is otherwise sound and the round-2 fixes hold.

## (1) Gaps

- **[high / confidence: high] Release order is not actionable (D17, Phase 4, Risks).** The plan says the order goes in the release notes, deploy guides and gate report. Facts: (a) the GCP VM clone is root's, updated by the tag-gated updater, which verifies signatures; running `host-setup.sh` from an unreleased commit means running unsigned code, against the model in `deploy/update/styrman-update`; (b) `host_converge` (`:770-830`) installs only `deploy/firewall/*`, and `kata-config.sh` would not be applied by it; (c) a root re-run of `host-setup.sh` has other effects the plan does not name: step 7 (`:255-296`) rebuilds the runtime image for the newest tag if absent and exits FATAL without `GH_TOKEN` or `/etc/styrman/gh-token`. Decide one of: accept the window (server updates first, a QEMU host may refuse project starts at unlimited RAM until the operator re-runs host-setup, and say that failure is expected and recoverable); hold the release tag until the VM is converged by a signed path; or take the one-step re-run from the release tag's clone. The task should say which, with the token requirement.
- **[medium / confidence: high] The "attach keeps the size" criterion does not produce the condition it claims (Verification last bullet; Phase 4 Tier 2).** It cites the 2026-09-16 security review's unproven low: whether Kata mishandles an empty `LinuxResources` on a LIVE sandbox. In a stop/start cycle the live-sandbox update is `stopLocked`'s (`orchestrator.go:1236`), after which the VM is powered off and the guest cgroup is rebuilt from `HostConfig` on the next start. The update at `:366` runs against a STOPPED container (Stop left the policy at "no"). So `cpu.max`/`memory.max` read after the cycle pass whether or not Kata drops limits. The condition needs a running container whose policy disagrees (`docker update --restart no` on a running project, then an attach) with the cgroup read before any stop. Also, the criterion names `ContainerUpdate` and `orchestrator.go:366`: an implementation detail in a Verification bullet; state the observable.
- **[medium / confidence: high] No real-daemon proof that an unchanged project is not recreated.** `Sizing` equality (`liveFrom(insp).Sizing != want`, plan `:183-185`; PUT `live.Sizing != want`) depends on Docker round-tripping `NanoCpus` and `Memory` exactly and on the disk env. Every test that checks "a matching stopped one is not recreated" and "running + live == want → no Stop" runs on `fakeDocker` (`fakedocker_test.go:116-138`), which returns whatever the fake stored. If the real daemon reports anything different (rounding, a field Kata or Docker rewrites), every start recreates the container and every PUT returns `rebuilt:true`, and no listed test sees it. Add one Tier 2 assertion: the container ID is unchanged across stop/start at a non-default size. The plan's claim "Tier 2 cannot plant the drift" (Verification, unit-only bullet) is also unchecked: `docker update --cpus/--memory` on a stopped container changes its `HostConfig`, so a pre-change-sized stopped container is plantable in Tier 2.
- **[medium / confidence: medium] `want`/live is lost across the `recreateMissingNetwork` recursion.** The plan computes `want` once after the first inspect "so keep what it has survives the image, workspace and bridge recreates". The start-failure path (`orchestrator.go:384-390`) removes the container and re-enters `ensureRunningLocked`; the recursion inspects again, finds no container, and for an old record (`DiskGB` 0) falls to the operator default. The new env then understates the image (grow-only, so no data loss), GET shows the smaller number as the effective disk and `minDiskGB` is too low. Say how `live` survives (pass it to the recursion) or accept it in a D-row.
- **[medium / confidence: medium] `kata_clh_reclaim` has no failure state.** Tested states are `= false` and `= true` only. On a Kata upgrade that moves or removes the key (the file is overwritten from the tarball), an anchored `sed` changes nothing and the function prints `unchanged` while reclaim is off. State that a missing key is `changed`-less failure (non-zero, named), and add that state to the fixture list. The verify clause says "toml line reads `= true`" for fixtures that contain the line, so the missing-key case is untested by construction.
- **[medium / confidence: medium] `LiveSizing`'s "without the project lock" is tested only on the fake.** The httpapi test "GET does not wait on a held project lock" runs on `fakeMode` and proves only that the handler does not call `WithProject`. The orchestrator task lists `LiveSizing found/absent/error` and no test that a held `lockProject(...)` does not block it. Add it, red-first against a body that locks.
- **[low / confidence: high] `Host` is both a field name and the exported method.** The plan has `Host() Host` on `*Orchestrator` and says `seamOrch` "gets `Host{16, 63853}`". An `Orchestrator` struct field named `Host` collides with the method. Name the field (`host`) or say so. Also `New` (`orchestrator.go:129-138`) makes no Docker call today, takes no context, and `dockerAPI` (`docker.go:25-47`) has no `Info`; the plan makes `Info` fatal in `New` without a context or timeout, though `cmd/styrman/main.go:91-94` warns that synchronous Docker work before the listener hung startup once. State the signature change, the `Info` addition to the interface and fake, and a bound.
- **[low / confidence: medium] "About 21G" and literal 20/21/19 are not stable observables.** Tier 2/e2e disk criteria read `df` of `/var/lib/docker-project` ("about 21G"); `df` of an ext4 file of N GiB shows N minus metadata overhead, and the offline grow's overhead differs from a fresh `mkfs`. State the assertion as a byte delta against the pre-grow `df -B1` (or the image's size), with its tolerance. The literals 20/21/19 hold only while the operator default is 20 (`config.go:175`; `run.sh:125` sets none) and the run's `settings.json` override is absent; read the default from `GET /resources` instead.
- **[low / confidence: medium] PUT body decoding is unspecified.** "all three required; a field missing or not an integer → 400" cannot be done with `int` fields, since a missing field decodes to 0 and 0 is valid for `cpus`/`memoryGiB`. Say pointer fields (or raw map) and what a `2.5` or `"2"` yields (400 naming the field, not a generic decode error).
- **[low / confidence: medium] `liveFrom`'s disk rule is ambiguous.** "an unparsable or absent disk env reads as 0", then `WantSizing` for `DiskGB 0` uses live disk "when it exists and parses". Say whether a parsed `0` counts. Default: no, fall to the operator default.
- **[low / confidence: high] Task text that is wrong against the code.** The existing-tests task adapts `RunOpts{}` literals in `wsboot_attach_test.go:41,55` with a `Resources` reader, but `fakeWSBooter.EnsureRunning` ignores its opts (`:16-21`); nothing there reads it, so the change is dead. The Phase 2 fake task's verify clause pipes the test run into `grep -c` and expects "exit 0": the exit status is grep's, against `.agents/rules/verify-bare.md`; run bare, filter the saved output.
- **[low / confidence: medium] Docs clause missing on two tasks.** The entrypoint-grow task changes the meaning of `STYRMAN_DOCKER_DISK_GB` from "size of NEW images" to "grow existing images at boot"; no docs clause (the Phase 4 docs task names api.md, deploy.md, guide.md, AGENTS.md only). The `GET`/`PUT` task has one (api.md).

## (2) Scope

The operator's request: default unlimited, per-project CPU/RAM/disk (default disk 20 GB), a checkbox UI, editable afterwards. D1-D13 and D20 carry the operator's "go all". What the plan builds beyond that, and who asked:

- **`limits.maxDiskGB` (constant 500) and `minDiskGB`**: a value that never varies; 5-500 is already restated at `config.go:242,309` and `explorer.go:557`, so this is a fourth copy. Reuse one constant. (D4 names the range; no D-row asks for it as an API field.)
- **`rebuilt` in the PUT response** (a second response shape beside `network.go`'s `lanView`): needed by the e2e criteria, no D-row.
- **D22, renaming `lanRebuildTimeout` to `rebuildTimeout` in `network.go` and `api.ts:499`'s comment**: churn in a feature the request did not name; author "cold read". Keeping the old name and adding a shared constant would do.
- **D25 (RAM row hidden on a tiny host), D27, D17-D19**: internal or edge-case, authored by "cold read"/"implementer"; no operator input needed except D25's UI state, which is minor.
- Both `MaxMemoryMiB` and `MaxMemoryGiB`: two shapes for one ceiling.

None of these is large; each is a gap only until a D-row names its asker or it is cut.

## (3) Readiness — questions an implementer would have to ask

1. BLOCKING — Release order: how does the new `host-setup.sh` reach the GCP VM before the server release, given the signed-tag updater and `host_converge` managing only the firewall? What does an operator do on a QEMU host between the auto-update and the re-run, and what does the re-run need (GH token for step 7)? Two careful implementers would write different release notes and deploy guides, and one of them would write a step that cannot be done.
2. NON-BLOCKING — Attach criterion: how to produce a live-sandbox `ContainerUpdate` without a stop? (Default: policy flip on a running project, cgroup read before any stop.)
3. NON-BLOCKING — Add a Tier 2 check that the container ID is unchanged across stop/start. (Default: yes.)
4. NON-BLOCKING — `live` through the `recreateMissingNetwork` recursion. (Default: pass it.)
5. NON-BLOCKING — Missing `reclaim_guest_freed_memory` key. (Default: fail loudly, test it.)
6. NON-BLOCKING — Field name for `Host`, `New`'s context and bound, `Info` in `dockerAPI` and the fake. (Default: `host`, a 10 s bound, add to both.)
7. NON-BLOCKING — PUT body decode: pointer fields, 400 naming the field for a non-integer. (Default: yes.)
8. NON-BLOCKING — A parsed disk env of 0. (Default: falls to the operator default.)
9. NON-BLOCKING — Disk assertion tolerance and source of the default. (Default: byte delta from `df -B1`, default read from GET.)
10. NON-BLOCKING — A held-lock test of `LiveSizing` at the orchestrator layer. (Default: add.)
11. NON-BLOCKING — Drop the dead `wsboot_attach_test.go` edit; run the pass-count clause bare. (Default: yes.)
12. NON-BLOCKING — Docs clause for the entrypoint grow. (Default: guide.md and deploy.md in Phase 4.)

## Criteria (applied to the plan)
1. Dead code — fail (minor): the `wsboot_attach_test.go` `Resources` edit has no reader; the `maxDiskGB` field is a constant restated a fourth time.
2. Flow tracing — fail (minor): the `recreateMissingNetwork` recursion loses `live`; the `ContainerUpdate` criterion does not reach the live-sandbox path; the real-daemon equality of `Sizing` is untested.
3. Standards conformance — fail (minor): the release order violates the signed-tag trust model; the verify clause pipes into `grep` (`verify-bare.md`); a docs clause is missing on the entrypoint grow. The describing-text rule is otherwise followed: `VCPUs`/`MemoryMB` prose, `config.go:324`, api.md/deploy.md are named.
4. Directory layout — pass: `deploy/lib/kata-config.sh` is a new directory (`deploy/gcp/lib.sh` is the precedent), `resources.go`, `ResourcesDialog.tsx` and the shell test land where conventions say.
5. Simplicity (DIY-except-security) — pass: stdlib only, no auth or crypto. Extras listed in (2).
6. Decoupling — pass: `Host()` and `runOptsFor` are narrow, and the `Resources` reader mirrors `Lan`.

## Not examined
- `web/src/lib/api.ts`, `Sidebar.tsx`, `settings.tsx`, `ResourcesDialog` bodies (the plan's line references there were not re-read).
- `integration.sh` and `run.mjs` bodies beyond `run.sh`'s server start, so whether they can read `nproc`/`cpu.max` in a guest, and the README count lines, were not checked.
- `internal/project/store.go` and `userSpace` wiring (the `SetLanAccess` mirror).
- Whether `docker update` on a stopped Kata container persists `HostConfig` limits (inferred from moby behaviour in the 2026-09-16 review, not run).
- QEMU shim behaviour (the plan states it is unmeasured).
- Whether `resize2fs` over virtiofs of a FULL image at 20 → 500 GB stays inside the boot budget (the plan lists it as a risk).

design needs revision
implementation not ready
