---
owner: reviewer
date: 2026-10-05
---
# Review — project-resources   (2026-10-05)

Range reviewed: 58ddcd3~1..3b67939 (web/dist and .agents/docs excluded). The uncommitted working-tree edits to `.agents/docs/testing/styrman/integration.sh` and `web/e2e/run.mjs` are outside the range; I read them in passing but they are not reviewed. `go test` for orchestrator, httpapi, config and project was run bare and passed (rc 0); that is not a mutation proof of any test.

## Status: COMPLETE

## Verdict: APPROVE WITH NITS

One real defect: the entrypoint's Docker-disk grow cannot retry after a partial failure, although its comment says it does, so the record and the UI can claim a larger disk than the filesystem has. The Go side holds up. Every start path goes through `runOptsFor`, the lock order matches network.go, a size or host-info read failure refuses the start, and the PUT validation is complete. The two other findings are low-severity design edges.

## Findings
- **[medium / high]** `image/entrypoint.sh:394-410` — the grow is not retryable after `truncate` succeeds. The gate is `HAVE_BYTES < WANT_BYTES` on the image FILE, and `truncate -s` sets the file to WANT. If `resize2fs` then fails (or the VM is stopped or killed between `truncate` and `resize2fs`, e.g. the SIGTERM trap), the next boot reads HAVE == WANT and skips the whole block. The ext4 filesystem stays at the old size while the record, GET and UI report the larger disk, and every later PUT sees a floor it cannot be at. The block comment ("the next boot retries") and the WARN "keeping the current size" are wrong for this branch. Fix: gate on the filesystem's size (`dumpe2fs -h` block count x block size vs. WANT) or run `resize2fs` after every clean e2fsck (it no-ops when the sizes match). Add a case to the integration check that plants file-bigger-than-fs and expects a grow.
- **[low / medium]** `internal/httpapi/resources.go:127-143`, `:152-160` — any project owner, not only an admin, can set `diskGB` up to 500 per project, with no per-user or per-host quota. Before this change the size was a server-wide operator setting (`STYRMAN_DOCKER_DISK_GB`, 5-500). The image is sparse, so a tenant can grow every one of their projects to 500 GB and then fill the shared state volume. The entrypoint only WARNs on `df` shortage (`entrypoint.sh:399-402`). ADR-083 records "owner edits" but does not weigh disk exhaustion across tenants. Consider an operator-set ceiling (config) for `maxDiskGB`; if the omission is accepted, say so in the ADR.
- **[low / medium]** `internal/orchestrator/orchestrator.go:389-396` plus `internal/orchestrator/resources.go` — rollout on a host whose shim is still the QEMU symlink (any host before `host-setup.sh` is re-run, including the GCP VM) removes every stopped project's container and then fails to start it at full-host memory. The failure is loud and fail-closed, and the ADR documents it, but nothing in the server checks which hypervisor is active before the irreversible `ContainerRemove`, so the first attempt on each project destroys the old container for a start that cannot work. Optional: refuse the resize recreate (not the start) until the shim wrapper is present, or surface the hypervisor in the settings page.

## Checked and found sound
- Every `EnsureRunning` caller goes through `runOptsFor` (`server.go:109`, `:343` via `ensureFor`, `:985` create, `:1180` start, `network.go:230`, `resources.go:234`). A nil `Resources` refuses the start (`errNoResources`, tested).
- Lock order: `WithProject` (project lock) -> `netMu` inside `persist`, the same as network.go. The record is re-read under the lock, so a delete during the wait is a 404. `LiveSizing` and the GET handler take no lock (tested).
- Fail-closed: a record read error, a host-info error and an inspect error each refuse the start or the PUT and leave the record alone (tests exist).
- The missing-network recursion pins the disk size decided before the container was removed (`orchestrator.go:432-446`). The image-upgrade and network-mode recreates keep the disk because `want` is computed before any removal.
- PUT validation: pointer fields (null/missing -> 400), type errors name the field, CPUs 0 or 1..host, memory 0 or 1..ceiling, disk floor taken from the live container under the lock, ceiling 500, global body cap in server.go:707. No overflow path.
- `host-setup.sh` / `kata-config.sh`: same-directory `mktemp` + `mv -f` replaces a symlink itself instead of writing through it (tested by the symlink case), mode is checked by content on every run, a toml without the key is an error, and the test runs against a scratch prefix it creates.

## Criteria
1. Dead code — pass. `Sizing`, `Live`, `Host` and the new interface methods all have callers; the removed `VCPUs`/`MemoryMB` fields have no remaining references (grep: only the AGENTS.md note, the removal warning and its test).
2. Flow tracing — one finding (entrypoint grow, above). Start paths, locking and error propagation trace cleanly. The new tests can fail: the real `WantSizing` and recreate logic run against the fake Docker, and the fake's `Info` and inspect carry the sizes.
3. Standards conformance — pass. `AGENTS.md` and the config comments were updated with the change.
4. Directory layout — pass. `deploy/lib/` is a new directory, but `verify-commit.sh` now covers it with `bash -n` and shellcheck.
5. Simplicity — pass, stdlib only, no new dependency, nothing in the auth/crypto area.
6. Decoupling — pass. `sizeRuntime` is declared in the consumer, like `modeRuntime`, and the orchestrator keeps the sizing rules.

## Not examined
- `internal/project/store_resources_test.go`, `internal/httpapi/resources_test.go` beyond the harness and test list (lines 1-120 read), `web/src/lib/resources.test.ts`, `api.encoding.test.ts`.
- No must-fail mutation was run on any new test; "tests can fail" is judged by reading.
- The idle reaper's interaction with a resource PUT in flight.
- The uncommitted integration.sh and e2e additions, and the `.agents/docs` trail (testing README count lines, ADR text), by instruction.
- The real Kata behaviour (limits added to the base VM, 128 MiB rounding) was taken from the code comments and ADR, not reproduced.

## Outcome
(main agent, 2026-10-05)
1. **Fixed.** `image/entrypoint.sh` now gates the grow on the FILESYSTEM size (`dumpe2fs -h`: block count × block size), not the file's, and only truncates when the file is smaller than wanted. Checked in a Kata guest (`styrman-runtime:test`, the grow block run alone against a scratch image): a plain 1→2G grow; the finding's trigger, a file already at 3G with a 2G filesystem, now grows the filesystem to 3G (the old file-size gate skips this case by construction); at size, a no-op; a smaller request never shrinks. Tier 2's disk check exercises the boot path.
2. **Operator's choice (low), deferred.** Owners may set the disk to 500 GB with no per-user or host quota. The bound 5–500 existed before as the operator setting; per-owner editing is what the operator asked for (plan D1, D21). Recommendation to the operator: defer, and track a per-user disk budget as a follow-up.
3. **Rebutted: accepted by the operator.** A host still on the QEMU shim can refuse starts until `host-setup.sh` is re-run. That is plan D28 (operator: "B sounds smarter.") and is in ADR-083's Consequences and the deploy guides.
