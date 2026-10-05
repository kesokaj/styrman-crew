---
owner: main
date: 2026-10-05
---
# Per-project resources, Phase 2: the server sizes each project (ADR-083)   (2026-10-05)

Plan: `.agents/docs/plans/2026-10-05-project-resources.md` (Phase 2). Every
project VM used to run at Kata's 1 vCPU / 2 GiB default; now each project
has its own size, the whole host by default.

- **Sizing** (`internal/orchestrator/resources.go`, new):
  - `Host` comes from `docker info` on first need and is cached only once a
    read succeeds.
  - `WantSizing` treats 0 CPUs as the host's count and 0 GiB as
    `floor((host MiB − 2048)/128)×128`. Larger requests are clamped with a
    WARN. On a host too small for a memory limit, none is set.
  - A disk of 0 (a record from before this change) keeps the container's
    current size, otherwise the operator default.
  - `LiveSizing` reads a container's size without the project lock.
- **Start path** (`orchestrator.go`):
  - `RunOpts.Resources` is required: nil, an unreadable record or an unknown
    host size refuses the start.
  - The size is decided from the container before any recreate removes it,
    and the missing-network recursion carries the disk into its second pass.
  - A stopped container at another size is recreated. Every container from
    before this change has no limits, so each existing project rebuilds once.
  - `create` sets `NanoCPUs`/`Memory` from the sizing and the disk env from
    it. The setup VM is created unlimited.
  - `dockerAPI` gains `Info`.
- **Config:** `STYRMAN_VCPUS`/`STYRMAN_MEMORY_MB` are removed. If set, they
  print a WARN and are ignored (`removedEnvWarnings`).
- **Record** (`internal/project/store.go`): new fields `cpus`, `memoryGiB` and
  `diskGB`, plus `SetResources`. It is a no-op when nothing changed and rolls
  back when the save fails.
- **API** (`internal/httpapi/resources.go`, new):
  - `GET /api/projects/{project}/resources` returns the values, the limits and
    `pending` (running at another size). It takes no project lock.
  - `PUT` validates cpus, memoryGiB and grow-only diskGB 5–500, then decides
    against the live container under the project lock: a running mismatch is
    stopped, persisted and started again; otherwise it only persists. The
    record write is under `netMu`, and a project deleted mid-change gives 404.
  - Every start site now builds its options through `runOptsFor`
    (`network.go`, wsBoot `server.go`, background create boot, `startProject`).
  - `createProject` records the operator's disk default.
  - `lanRebuildTimeout` is renamed `rebuildTimeout`.
- **Entrypoint:** before the mount, the Docker disk image is grown offline when
  `STYRMAN_DOCKER_DISK_GB` is larger (`e2fsck -f`, then `truncate`, then
  `resize2fs`; never shrinks). A failure WARNs and boot continues.
- **Tests:** 35 new top-level tests: orchestrator (`resources_test.go`,
  `resources_start_test.go`), httpapi (`resources_test.go`), project
  (`store_resources_test.go`) and config (`removed_env_test.go`). The fake
  Docker gains `Info`, Resources and Env; the seam tests' `plant()`/`opts()`
  get a size.
  - Must-fail mutations, each failing its tests: no resize recreate, no disk
    carried through the recursion, no live read before a recreate, the PUT
    never or always rebuilding, `pending` always false, the disk floor
    removed.

Verification: `verifier`, `.agents/docs/testing/2026-10-05-verify-project-resources-phase-2.md`
(PASS: build, vet, `go test -race` 16 packages ok, 401 top-level PASS vs 366
at HEAD, ingress module ok, entrypoint `bash -n` + shellcheck). Not covered
yet: the entrypoint grow and real Kata sizing (Tier 2, Phase 4), and
createProject's `diskGB` (Tier 2). Docs (`api.md`, `deploy.md`) are pending
the Phase 4 documentation dispatch. Diff read by me; the code review covers
the whole change at Phase 4.

## Learnings for the local host
none
