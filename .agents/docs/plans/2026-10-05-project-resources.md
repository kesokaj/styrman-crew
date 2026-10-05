---
owner: main
date: 2026-10-05
---
# Plan — per-project resources (CPU / RAM / Docker disk)   (2026-10-05)

## Why
Operator, 2026-10-05: "I dont want to limit cpu or ram. a workspace can take
the full machine if it can." Then: "Default should be unlimited. And I want a
checkbox to set cpu/ram/disk per project. So default cpu=unlimited,
ram=unlimited, disk=20gb. I want to be able to set these per project
afterwards." Scope settled in two numbered rounds, both answered "go all"
(see Decisions D1–D14).

Today every project VM gets Kata's default size, because the server passes
zero limits (`internal/orchestrator/orchestrator.go:1080-1087`): 1 vCPU and
2 GiB, measured in a live guest (`nproc`=1, `MemTotal` 1986 MiB). The only
knobs are two server-wide env vars (`STYRMAN_VCPUS`, `STYRMAN_MEMORY_MB`)
that production doesn't set. The Docker disk is a server-wide operator
setting (20 GB), fixed into the container's env when it's created.

## Scope
In:
- Default for every project VM, including the setup VM: CPU unlimited, RAM
  unlimited, meaning the host's full size (D2).
- Per-project setting of CPU (whole cores), RAM (whole GiB) and Docker disk
  (GB), editable by the project's owner after creation, in a "Resources…"
  dialog in the project's sidebar menu. CPU and RAM each have an "Unlimited"
  checkbox; the disk is a number, grow-only.
- A change applies right away on a running project (stop, recreate at the
  new size, start: home and workspace survive), or at its next start if the
  project is stopped.
- A stopped project whose container is sized differently from its record is
  recreated on its next start. Every existing project therefore rebuilds
  once (D7); its disk keeps its current size (D24).
- `STYRMAN_VCPUS` / `STYRMAN_MEMORY_MB` are removed; if either is set, the
  server logs a WARN and ignores it.
- Host: `host-setup.sh` writes the `containerd-shim-mvm-v2` wrapper that
  selects Cloud Hypervisor (today hand-made on dave only, so a fresh host
  runs QEMU; recorded as a bug) and sets `reclaim_guest_freed_memory = true`.

Out:
- Admin editing another user's project (D1: no cross-user lookup exists).
- Setting resources in the create dialog (D9).
- Shrinking a Docker disk.
- Extending the self-updater's `host_converge` to manage Kata config (D6).
- An online (no-restart) disk grow (D3).

Follow-ups noted, not built: resources in the create dialog; reporting a
disk grow that failed in the guest back to the UI (D12).

## Surfaces touched
Recon brief: `.agents/docs/plans/2026-10-05-recon-project-resources.md`.
- `internal/config/config.go` — drop `VCPUs`/`MemoryMB` (`:106-107`, `:173-174`),
  WARN if the env vars are set; `DockerDiskGB()` doc wording (`:324`).
- `internal/orchestrator/orchestrator.go` — host sizing at `New`, `RunOpts.Resources`,
  sizing in `create` (`:1080-1087`), the disk env (`:443`), a sizing-mismatch
  recreate in `ensureRunningLocked` beside `bridgeModeMismatch` (`:319-337`),
  `EnsureSetup` (`:703`).
- New `internal/orchestrator/resources.go` — host probe, ceilings, effective sizing.
- `internal/project/store.go` — three fields on `Project` (`:24`), `SetResources`
  (mirror `SetLanAccess` `:233`).
- `internal/httpapi/` — new `resources.go` (GET/PUT, mirroring
  `network.go:57-~200`), routes in `server.go:493-495`, every `RunOpts{…}`
  construction site passes `Resources` (grep `RunOpts{`), `createProject`
  (`server.go:859`) records the disk default.
- `image/entrypoint.sh:363-392` — grow the image offline before the mount.
- `deploy/host-setup.sh:104-107` + new `deploy/lib/kata-config.sh` — wrapper; idempotent clh toml step.
- `internal/httpapi/server.go:83-103` — `modeRuntime` (`Host`, `LiveSizing`, `WantSizing`), `runOptsFor`, `resourcesOf`.
- `web/src/lib/api.ts` (`:484-510`), `web/src/components/Sidebar.tsx`
  (`:116-122`, `:338`, dialogs `:397-404`), new `web/src/components/ResourcesDialog.tsx`,
  `web/src/routes/settings.tsx:875,1001` (label).
- Tests: `internal/orchestrator/fakedocker_test.go` (extended), `seam_test.go`
  and `internal/httpapi/wsboot_attach_test.go` (adapted: `Host`, `Resources`),
  new `resources_test.go` files in orchestrator / httpapi / project;
  `web/src/lib/api.encoding.test.ts`; `.agents/docs/testing/styrman/integration.sh`;
  `web/e2e/run.mjs`.
- Docs: `.agents/docs/specs/styrman/api.md` (`:11`, `:270`), `deploy.md:139`,
  `guide.md`, `deploy/README.md` / `.agents/docs/deploy/remote.md` (root re-run of
  host-setup), `.agents/docs/testing/styrman/README.md` (counts), AGENTS.md
  Architecture (one line), ADR-083.

## Interfaces

### Kata sizing facts (measured on dave, 2026-10-05, Kata 3.31.0, clh)
- Kata ADDS the Docker limit to its base (`default_vcpus = 1`,
  `default_memory = 2048`): `--cpus 4 --memory 8g` → guest `nproc` 5,
  `MemTotal` ≈ 10 GiB; the in-guest cgroup caps the workload at exactly
  4 CPUs / 8 GiB (`cpu.max 400000 100000`, `memory.max 8589934592`).
- vCPUs clamp at the host count (`--cpus 16` → 16). Docker itself refuses
  `NanoCPUs` above the host count: "range of CPUs is from 0.01 to 16.00".
- Memory does NOT clamp: if base + limit, rounded UP to 128 MiB, exceeds host
  RAM, the start fails ("The requested hotplug memory addition is not a valid
  size"). Host `MemTotal` 65385768 kB = 63853 MiB; 61696 MiB works, 61805 MiB fails.
- Boot: 1.1 s at default size, 1.8 s at 16 vCPU / 60 GiB (SLA < 5 s).
- Ten VMs at once, each `--cpus 16 --memory 61696m`, idle (`sleep`): host
  memory in use 4127 → 15844 MiB, 48008 MiB still available; each guest
  reports 16 CPUs and the full `MemTotal`.
- Offline grow of an empty 20 GB image to 500 GB in the runtime image under
  Kata: `e2fsck -f -p` 31 ms (rc 0), `resize2fs` 110 ms (rc 0).
- At the computed ceiling (`--memory 61696m`) the guest's `MemTotal` is
  65210884 kB, 99.7% of the host's 65385768 kB.
- The server container sees the host: `docker exec styrman-server nproc` → 16,
  `MemTotal` 65385768 kB, identical to the host.
- Offline grow works in a Kata guest on the virtiofs home: in
  `styrman-runtime:1.41.1` under `--runtime mvm`, `mkfs.ext4` a 1G image, write
  a file with `debugfs`, `e2fsck -f -p`, `truncate -s 2G`, `resize2fs` →
  block count 524288 × 4096 = 2 GiB, the file reads back intact.
- All of the above is Cloud Hypervisor. QEMU (what a host without the clh
  wrapper runs) was not measured: dave has no QEMU shim on PATH, and
  installing one is a root host change.

### Orchestrator (`internal/orchestrator/resources.go`)
```go
// kataBaseMemoryMiB is configuration-clh.toml's default_memory: Kata adds a
// container's memory limit on top of it.
const kataBaseMemoryMiB = 2048
const memoryHotplugBlockMiB = 128

// Host is the machine's size as the Docker daemon reports it (`docker
// info`: NCPU, MemTotal), read once at New. The daemon's count is the one
// it validates NanoCPUs against ("range of CPUs is from 0.01 to 16.00"), and
// it works on every platform the server runs on (macOS dev included).
type Host struct{ CPUs, MemMiB int }

// MaxMemoryMiB = floor((MemMiB - kataBaseMemoryMiB) / 128) * 128, or 0 when
// that is <= 0 (a host under 2176 MiB, e.g. a small Colima VM).
func (h Host) MaxMemoryMiB() int
// MaxMemoryGiB = MaxMemoryMiB() / 1024 (what the UI may set by hand).
func (h Host) MaxMemoryGiB() int

// Resources is a project's request; 0 = unlimited (CPUs, MemoryGiB) or
// "keep what the container has" (DiskGB, records older than this change).
type Resources struct{ CPUs, MemoryGiB, DiskGB int }

// Sizing is what a container is (or would be) created with.
type Sizing struct {
	NanoCPUs int64 // CPUs, or Host.CPUs for 0; clamped to Host.CPUs; × 1e9
	MemBytes int64 // MemoryGiB×1024 MiB, or MaxMemoryMiB for 0; clamped to MaxMemoryMiB; × 2^20.
	               // MaxMemoryMiB 0 → MemBytes 0 (no limit: Kata's default size).
	DiskGB   int   // DiskGB; for 0: the live container's STYRMAN_DOCKER_DISK_GB
	               // when it exists and parses, else cfg.DockerDiskGB()
}

// Live is a container's current sizing, read from `docker inspect`.
type Live struct {
	Sizing
	Running bool
}

// WantSizing computes the sizing for r. live is the container's current
// state (nil when it does not exist); only DiskGB 0 consults it.
func (o *Orchestrator) WantSizing(r Resources, live *Live) Sizing

// LiveSizing inspects the project's container WITHOUT taking the project
// lock (a read; GET must not wait behind a boot). found=false when there is
// no container. An inspect error is returned, never folded into "absent".
func (o *Orchestrator) LiveSizing(ctx context.Context, project string) (live Live, found bool, err error)
```
- `New` reads `Host` through `docker.Info`; an error is a fatal startup
  error (no silent fallback to Kata's default). `Host() Host` is exported for
  the API's limits. A zero `Host` (only reachable in tests that build an
  `Orchestrator` literal) yields zero limits, i.e. Kata's default size; the
  seam tests set a `Host` (Phase 2, existing-tests task).
- `RunOpts` gains `Resources func() (Resources, error)`, read under the
  project lock at the same point as `Lan` (`ensureRunningLocked:273`). It is
  REQUIRED: a nil `Resources` refuses the start ("resources: no record
  reader — refusing to start at an unknown size"), so a construction site
  that forgets it fails on its first start instead of silently re-sizing a
  limited project to unlimited (fail closed, BUG-031/035/060 shape). A read
  error refuses the start the same way ("resources: cannot read the project
  record").
- In `ensureRunningLocked`, right after the first `ContainerInspect` (`:278`)
  and BEFORE any recreate removes the container, compute
  `want := o.WantSizing(r, liveFrom(insp))`, so "keep what it has" survives
  the image, workspace and bridge recreates too.
- `createSpec` gains `sizing Sizing`; `create` sets
  `container.Resources{NanoCPUs: s.NanoCPUs, Memory: s.MemBytes}` from it. The
  project path adds `STYRMAN_DOCKER_DISK_GB=<want.DiskGB>` to its `extraEnv`
  (`:440-444`, replacing `o.cfg.DockerDiskGB()`); `EnsureSetup` passes
  `sizing: o.WantSizing(Resources{}, nil)` and no disk env (minimal mode, no
  dockerd).
- Sizing mismatch, STOPPED containers only (same block shape as the image and
  bridge recreates, `:319-337`): recreate when `liveFrom(insp).Sizing !=
  want`. Log `"recreating stopped container for a resource change"`.
- `liveFrom(insp)` reads `HostConfig.NanoCPUs`, `HostConfig.Memory`, and the
  env via a new helper `containerEnv(env []string, key string) string` beside
  `containerGuest`, nil-safe on `insp.Config == nil`; an unparsable or
  absent disk env reads as 0.
- `EnsureSetup`: an existing setup VM is NOT recreated for size. Setup VMs
  are short-lived: `ReapSetupVMs` (`:1563`) stops and removes any that has run
  longer than its max age since start, so the next one is created at the new
  size.

### Store (`internal/project/store.go`)
```go
CPUs      int `json:"cpus,omitempty"`      // 0 = unlimited
MemoryGiB int `json:"memoryGiB,omitempty"` // 0 = unlimited
DiskGB    int `json:"diskGB,omitempty"`    // 0 = keep what the container has (records older than this change)

func (s *Store) SetResources(id string, cpus, memoryGiB, diskGB int) error // no-op on equal; rollback on save error (M20)
```
`createProject` stores `DiskGB: s.cfg.DockerDiskGB()` at creation (D11). No
migration: an old record's 0 means "keep the container's current disk", so a
later change of the Settings default never recreates or grows an existing
project (D16). The first PUT writes an explicit value.

### httpapi wiring
- One constructor, `runOptsFor(sp *userSpace, p project.Project, guest string)
  orchestrator.RunOpts`, beside `lanOf` (`server.go:95`), sets `Name`, `Dir`,
  `GuestUser`, `Lan: lanOf(...)` and `Resources: resourcesOf(sp, p.ID)`. All
  four sites use it: `network.go:228`, `server.go:284` (wsBoot), `server.go:947`
  (background create boot), `server.go:1143` (start). The change entry lists
  them (rule mechanism-reaches-every-caller).
- `modeRuntime` (`server.go:83-88`) gains `Host() orchestrator.Host`,
  `LiveSizing(...)` and `WantSizing(...)`; the test fakes implement them.

### HTTP (`internal/httpapi/resources.go`), owner only
`GET /api/projects/{project}/resources` → 200
```json
{"cpus":0,"memoryGiB":0,"diskGB":20,"pending":false,
 "limits":{"maxCpus":16,"maxMemoryGiB":60,"minDiskGB":20,"maxDiskGB":500}}
```
Unlocked: record + `LiveSizing`. `diskGB` is `WantSizing(record, live).DiskGB`
(the effective value); `minDiskGB` = max(5, that). `pending` = found &&
`live.Running` && `live.Sizing != WantSizing(record, live)` (after the
upgrade: every project still running at the old size). The dialog then says
"Running at its previous size; Save restarts it at this one." A `LiveSizing`
error makes `pending` false and is logged; GET never fails on it.
`maxMemoryGiB` 0 (tiny host) → the dialog hides the RAM row ("not settable
on this host") and the PUT accepts only `memoryGiB` 0.

`PUT /api/projects/{project}/resources`, body `{"cpus":int,"memoryGiB":int,"diskGB":int}`,
all three required → 200 with the GET body plus `"rebuilt":bool`.
- 400: a field missing or not an integer; `cpus` not 0 or 1..maxCpus;
  `memoryGiB` not 0 or 1..maxMemoryGiB; `diskGB` < minDiskGB or > 500. The
  message names the field and its range.
- 404: not the caller's project (before any other check).
- 409: project is being deleted (`deletingState`, as in network.go).
- Decided against the LIVE container (the `setProjectNetwork` shape, security
  review 2026-09-09 round 3). Under `WithProject`: re-read the record;
  `LiveSizing`; `want := WantSizing(request, live)`.
  - Running and `live.Sizing != want` → `Stop`, persist, `EnsureRunning(runOptsFor(...))`,
    then the background `EnsureSelfAttached` after 1 s exactly as
    `network.go` does → `rebuilt:true`.
  - Running and `live.Sizing == want` → persist if the record differs, no
    Stop → `rebuilt:false` (a record-only drift never kills sessions).
  - Not running, or no container → persist if the record differs →
    `rebuilt:false`; the next start applies it.
  - A `LiveSizing` error → 500 "cannot inspect the project; nothing changed".
- Persist = `SetResources` under `s.netMu` (the `persist` closure in
  `network.go`, `server.go:143-147`), so it cannot race `deleteProject`'s
  record removal. `project.ErrNotFound` from it → 404 "project was deleted",
  as `network.go` maps it at both persist sites.
- Bound: the existing `lanRebuildTimeout` (3 min) is reused, renamed
  `rebuildTimeout` with its references; the web client's 210 s stays paired
  with it (BUG-022).
- slog Info `"project resources changed"` with `by`, `project`,
  `projectName`, old and new values.

### Entrypoint (`image/entrypoint.sh`, after the fsck, before the mount)
If `stat -c %s "${IMG}"` < `DISK_GB × 2^30`: run the N30 free-space warning
(`:366-371`) for the difference, `e2fsck -f -p` (exit 0 or 1 = clean or
corrected → continue; ≥ 2 → skip the grow), `runuser -u <user> -- truncate -s
${DISK_GB}G`, `resize2fs`. Any step failing → `log "WARN: docker image grow
to ${DISK_GB}G failed at <step>"` and boot continues on the image as it is (a
truncated-but-not-resized image still mounts: the filesystem is smaller than
the file). A larger image is never shrunk.

### Host (`deploy/host-setup.sh` + new `deploy/lib/kata-config.sh`)
Both steps are functions in a new sourceable file, each taking its target
path as an argument so a test can point it at a scratch prefix (ADR-052):
`kata_shim_wrapper <shim-path> <kata-prefix>` and `kata_clh_reclaim
<toml-path>`; each prints `changed` or `unchanged`. `host-setup.sh` sources
it and calls them with `/usr/local/bin/containerd-shim-mvm-v2` and
`/opt/kata`.
- Wrapper: the shim path holds the script
  `KATA_CONF_FILE=<kata-prefix>/share/defaults/kata-containers/configuration-clh.toml
  exec <kata-prefix>/bin/containerd-shim-kata-v2 "$@"` (with a `#!/bin/bash`
  line), written when the file is missing, a symlink, or differs from that
  content: an atomic temp file plus rename, mode 0755. A host where the
  symlink is replaced gets clh only for sandboxes started after the change.
- Reclaim: in `configuration-clh.toml`, set `reclaim_guest_freed_memory = true`
  with an anchored `sed` on that one key, only when it isn't already true;
  log changed or unchanged. No Docker restart: the shim reads its config per
  sandbox start.

### Web
- `api.ts`: `projectResources(id)`, `setProjectResources(id, body)`, 210 s timeout.
- `ResourcesDialog.tsx`: rows CPU and RAM, each an "Unlimited" checkbox
  (checked when the value is 0) and a number input enabled when it's
  unchecked (min 1, max from `limits`); under CPU the hint "The terminal
  shows one more CPU than this (the VM's own); the workload is capped at this
  number." Disk is a number input (min `minDiskGB`, max 500) with the note
  "grow only". `pending` shows its line. Before Save: "Saving restarts this
  project if it is running. Home and workspace are kept; anything installed
  outside them (except apt packages, which are replayed) is lost." A server
  400 message is shown in the dialog.
- `Sidebar.tsx`: a "Resources…" item after Rename.
- `settings.tsx`: the label "Docker disk / project" becomes "Default Docker
  disk for new projects".

## Failure modes
- Host probe (`docker info`) fails at startup → the server refuses to start,
  naming the Docker call.
- Record unreadable at start → the start is refused with the resources error
  (same as the `Lan` read).
- A stored value above the host's size (hardware shrank) → clamped to the
  host's size, WARN logged; the start proceeds (D13).
- PUT: Stop fails → 500, "project still running at its old size", record
  unchanged. `SetResources` fails after Stop → 500, "project stopped; record
  unchanged; start it to resume at its old size". `EnsureRunning` fails after
  the record is written → 500, "project stopped; new size saved; it applies on
  the next start". In each case the record never claims a size the
  container doesn't have, unless the container is down.
- Kata rejects the size at start (the ceiling math is wrong for this host,
  e.g. an unmeasured QEMU host) → `EnsureRunning`'s existing error path; the
  record keeps the request, and the UI shows the server error. No retry loop:
  the start fails once per attempt.
- A `RunOpts` built without `Resources` → that start is refused with the
  "no record reader" error (a programming error surfaced, never a silent
  unlimited).
- Disk grow fails in the guest → WARN in the project's entrypoint log;
  dockerd starts on the old size; the next boot retries (D12).
- Existing project on its first start after the upgrade → recreated once (its
  CPU/RAM differ; its disk does not): apt packages are replayed from the
  manifest (ADR-028); anything else only in the container filesystem is lost
  (same as a mode toggle).
- `STYRMAN_VCPUS`/`STYRMAN_MEMORY_MB` set → WARN at startup, ignored.

## Verification
Tiers: unit (`go test -race ./...`), web (`npm run check && npm run lint &&
npm test`), host-setup shell test (`host-setup-kata.sh`), integration Tier 2
(`integration.sh`, real Kata), browser e2e (`web/e2e/run.sh`), host-setup on
dave (root, operator).

E2E criteria:
- Golden path: a new project's terminal reports `nproc` equal to the host's
  CPU count, and `MemTotal` at least 97% of host RAM (measured 99.7%). In the
  Resources dialog the owner sets CPU 2 and RAM 4 GiB and saves; the project
  restarts and its guest reports `cpu.max` `200000 100000`, `memory.max` 4 GiB
  and `nproc` 3. The dialog reopens showing 2 / 4. Ticking Unlimited again and
  saving restores `nproc` = host count.
- Disk: raising Disk from 20 to 21 and saving → `df` of
  `/var/lib/docker-project` in the guest reports about 21G after the restart;
  entering 19 is refused with the range message.
- Edge: `cpus` = host + 1 → 400 naming `cpus`.
- Changed while stopped: a stopped project whose resources are changed (PUT
  → `rebuilt:false`) runs at the new size on its next start (`cpu.max`
  `200000 100000`).
- Unit-only cases (no single-identity Tier 2/e2e run can produce them): another
  user's project → 404 (two `userSpace`s); a pre-change container (NanoCPUs
  0, Memory 0, an old disk env) is recreated on start at host CPU/RAM and keeps
  its disk; a running project at another size gets `pending:true` and Save
  rebuilds it (the new server never creates a container at a size other than
  its record's, and the store is held in memory, so Tier 2 cannot plant the
  drift).
- Attach keeps the size: on a RUNNING project whose restart policy is flipped
  (the live-sandbox `ContainerUpdate` with zero `Resources`,
  `orchestrator.go:366`, reached by a start call on a running container), the
  guest cgroup and `HostConfig.NanoCpus`/`Memory` still read the set values,
  read before any stop (security review 2026-09-16, the unproven low).
- An unchanged project is not recreated: its container ID is the same across
  a stop/start cycle.

## Phases
- [x] Cold-reader check — cap reached after 3 rounds (.agents/docs/reviews/2026-10-05-cold-read-project-resources.md, round 3: design needs revision / implementation not ready, one BLOCKING: release order on the GCP VM). Operator at the cap, 2026-10-05: "B sounds smarter." (one release; re-run host-setup on the GCP VM right after it updates — D28); the non-blocking residue is the `Open:` rows under Decisions

### Phase 1 — Host: Kata wrapper + memory reclaim
- [x] Bug record for the hand-made `containerd-shim-mvm-v2` wrapper: BUG-089 written 2026-10-05 (`.agents/docs/bugs/BUG-089-kata-clh-shim-wrapper-hand-made-on-dave-only.md`, open); the `bugs` agent updates it to fixed at this phase's checkpoint
- [x] `deploy/lib/kata-config.sh` (two functions) sourced by host-setup.sh — verify: `bash -n` both files; a new shell test, `deploy/lib/kata-config-test.sh` (D29), sources the lib and runs it against `.agents/scratch/kata-test/` (ADR-052) for four wrapper starting states — missing, a symlink to the shim, a script with other content, already correct — and two toml states (`= false`, already `= true`), each run twice — who: AI — expect: first run `changed` for every non-correct state and `unchanged` for the correct ones, second run `unchanged` for all; afterwards the wrapper is a regular 0755 file with the exact content and the toml line reads `reclaim_guest_freed_memory = true` with the rest of the file byte-identical (`diff`) — docs: `deploy/README.md`, `.agents/docs/deploy/remote.md`, `.agents/docs/testing/styrman/README.md` (the new script's entry and how to run it)
- [-] Apply on dave: `sudo deploy/host-setup.sh` — verify: `cat /usr/local/bin/containerd-shim-mvm-v2`; `grep reclaim_guest /opt/kata/share/defaults/kata-containers/configuration-clh.toml`; a fresh `docker run --runtime mvm` shows `cloud-hypervisor` in `ps` — who: user (no standing root permission in `docs/deploy/local.md`) — expect: wrapper behavior unchanged, reclaim true — superseded by Phase 4 § Post-release host step (D28: the re-run comes after the release reaches each host)
- [x] Checkpoint — expect: the test script passes twice; BUG-089 → fixed; changes entry; commit — evidence: .agents/docs/testing/2026-10-05-1312-verify-project-resources-phase-1.md (PASS: go test -race 425 pass/0 fail, kata-config-test.sh 39/0, bash -n and shellcheck exit 0), 2026-10-05

### Phase 2 — Server: sizing, record, API, entrypoint
- [x] ADR-083 appended to DECISIONS.md before this phase's code (records the 10-VM measurement and that the REQUIREMENTS targets stand, D23) — who: AI — expect: entry present
- [x] Extend `fakedocker_test.go`: `ContainerCreate` stores `HostConfig.Resources` and `Config.Env`; `ContainerInspect` returns them (non-nil `Config`); `Info` returns a scriptable NCPU/MemTotal — verify: `go test ./internal/orchestrator/ -v | grep -c '^--- PASS'` before and after — who: AI — expect: exit 0, same count
- [x] `Host` from `docker.Info` + `WantSizing` + ceilings (`resources.go`) — verify: unit `TestWantSizing*` (table: 0→host, n→n, n>host→clamp, MiB rounding against a 63853 MiB host → 61696, a 2000 MiB host → MemBytes 0, DiskGB 0 with live 30 → 30, DiskGB 0 with no container → operator default) — who: AI — expect: pass; red first with zero bodies
- [ ] Remove `VCPUs`/`MemoryMB`; WARN when set — verify: `internal/config` test sets the env, asserts WARN on stderr and no field (as `removedEnvWarnings`, `internal/config/removed_env_test.go`) — who: AI — expect: pass — docs: `.agents/docs/specs/styrman/api.md`, `deploy.md` — in progress: code and tests done (verifier phase 2 PASS); blocker: none — docs pending the Phase 4 `documentation` dispatch
- [x] Store fields + `SetResources` — verify: `internal/project` test: round-trip, no-op on equal, rollback on save failure, old JSON without the fields loads as 0 — who: AI — expect: pass
- [x] Existing orchestrator and httpapi tests adapted in the same step as the next task: `seamOrch` (`seam_test.go:36`) gets `Host{16, 63853}`; `opts()` (`:52`) and the `RunOpts{}` literals in `wsboot_attach_test.go:41,55` get a `Resources` reader returning `Resources{}`; `plant()` (`:41-46`) plants containers at `WantSizing(Resources{}, nil)` so they don't trip the new recreate — verify: `go test ./internal/...` pass count before (recorded first) and after — who: AI — expect: same count plus the new tests — done: `seam_test.go` `plant()`/`opts()`; `wsboot_attach_test.go` needed no edit (its fake never reads Resources); `go test ./internal/... ./cmd/...` 366 → 401 PASS, 0 FAIL
- [x] `RunOpts.Resources` (required), `createSpec.sizing`, disk env from `want`, stopped-container mismatch recreate, `LiveSizing`, setup VM unlimited — verify: orchestrator tests on the extended fake: create carries NanoCPUs/Memory/env; a stopped 0/0 container with disk env 20 (operator default 30) is recreated at host CPU/RAM with disk env 20; a running one is not; a matching stopped one is not; an image-upgrade recreate keeps the old disk env; nil `Resources` refuses the start; a `Resources` read error refuses the start; `LiveSizing` found/absent/error — who: AI — expect: pass
- [x] `runOptsFor` at all four sites — verify: `grep -rn 'orchestrator.RunOpts{' internal/ cmd/ --include='*.go' | grep -v _test` returns only `runOptsFor`'s own line; the four sites are listed in the change entry — who: AI — expect: one hit
- [x] `createProject` records `DiskGB` — verify: httpapi test — who: AI — expect: new record has diskGB = operator default — no unit test: createProject boots a real orchestrator in the background; covered by the Tier 2 check that a new project's GET reports the operator default (Phase 4)
- [ ] GET/PUT `/resources` — verify: httpapi tests on `fakeMode` (extended with `Host`, `LiveSizing`, `WantSizing`): validation 400s per field; `memoryGiB` > 0 on a host with `maxMemoryGiB` 0 → 400; 404 foreign (two `userSpace`s); 409 deleting; running + live == want → persist only, no Stop, rebuilt:false; running + live != want with an equal record → Stop, persist, Ensure, rebuilt:true; stopped → record only; `LiveSizing` error → 500, nothing changed; persist `ErrNotFound` → 404; each failure branch's message and record state; GET `pending` true/false; GET does not wait on a held project lock — who: AI — expect: pass — docs: `.agents/docs/specs/styrman/api.md` — in progress: code and tests done (verifier phase 2 PASS); blocker: none — docs pending the Phase 4 `documentation` dispatch
- [x] Entrypoint grow — verify: `bash -n image/entrypoint.sh`; the Tier 2 disk check (Phase 4) — who: AI — expect: syntax ok
- [x] Checkpoint — expect: `go test -race ./...` exit 0, `go vet ./...` exit 0; verifier log `project-resources-phase-2` PASS; changes entry; commit — evidence: .agents/docs/testing/2026-10-05-verify-project-resources-phase-2.md (PASS: race 16 packages ok; 401 top-level PASS vs 366 at HEAD; RunOpts built only in runOptsFor), 2026-10-05

### Phase 3 — Web
- [ ] api.ts calls + encoding test entries — verify: `(cd web && npm test)` — who: AI — expect: pass
- [ ] ResourcesDialog + Sidebar item + settings label — verify: `(cd web && npm run check && npm run lint && npm test)` — who: AI — expect: exit 0 each; `web/dist/client` rebuilt and committed
- [ ] Checkpoint — expect: verifier log `project-resources-phase-3` PASS; changes entry; commit — evidence: <log>, <date>

### Phase 4 — End to end, docs
- [ ] Tier 2 checks in `integration.sh`: default size = host (`nproc`, `MemTotal` ≥ 97%); PUT 2/4 on a running project → rebuilt:true, `cpu.max`/`memory.max`, `nproc` 3; PUT on a stopped project → rebuilt:false, then start → `cpu.max 200000 100000`; a stop/start cycle keeps `HostConfig.NanoCpus`/`Memory` and the cgroup; disk +1 → `df` ≈ +1G; disk −1 → 400; cpus host+1 → 400 — verify: `bash .agents/docs/testing/styrman/integration.sh` bare, rc read — who: AI — expect: rc 0; count line updated in README in the same commit
- [ ] e2e: Resources dialog golden path at the END of `run.mjs` (it rebuilds); one deliberate-failure run to prove it doesn't cascade (rule e2e-checks-leave-the-suite-as-they-found-it) — verify: `bash web/e2e/run.sh` — who: AI — expect: all pass; forced-fail run shows `failed=1`; count line updated
- [ ] Docs: api.md, deploy.md, guide.md, AGENTS.md Architecture line — `documentation` agent — who: AI
- [ ] Fresh-host path: `deploy/gcp/vm.sh create` + `install.sh` on a new VM, check the wrapper and that a project VM runs cloud-hypervisor at host size — who: user (no gcloud on dave) — expect: done, or "fresh-host path unexercised" in the gate report
- [ ] Post-release host step in the release notes, the gate report and the deploy guides: once each host has auto-updated, re-run `sudo GH_TOKEN=… /opt/styrman/deploy/host-setup.sh` from the verified tree (dave: here; GCP VM: `deploy/gcp/vm.sh ssh` from the Mac). Until then a QEMU host may refuse project starts (D28) — who: AI writes, user runs — docs: `deploy/README.md`, `.agents/docs/deploy/remote.md`, `.agents/docs/deploy/gcp.md`
- [ ] Checkpoint — Definition of Done: verifier, reviewer (diff only), security (input parsing, host root script), documentation, `/review --docs`, changelog, handoff — evidence: <paths>, <date>

## Risks
Assumptions:
- Kata's base memory stays 2048: the constant; host-setup doesn't touch
  `default_memory`. A Kata upgrade that changes it breaks the ceiling → the
  start fails loudly (failure mode above), it doesn't truncate silently.
- QEMU sizing is unmeasured (see Kata sizing facts). A host still on the QEMU
  symlink when the release arrives may refuse every start at unlimited RAM.
  Mitigated by the release order (D17), not by code; checked on the GCP VM by
  the fresh-host task, or reported as unexercised.
- Memory overcommit: measured idle only (10 VMs, D23). Ten projects all
  touching most of their RAM at once can still exhaust the host; that reclaim
  helps is Kata's documented behaviour, not measured here.
- `e2fsck -f` on a FULL large image takes longer than the measured empty one;
  it runs only on the boot that grows the image.
- Already checked (2026-10-05, see Kata sizing facts): the server container
  sees the host's CPUs and RAM (`docker info` on dave too, per the round-2
  cold read); offline `e2fsck`+`truncate`+`resize2fs` works in a Kata guest
  on the virtiofs home, 20 → 500 GB in 0.14 s.

## Decisions
D1 — 2026-10-05 — Owner-only edit; no admin-for-any-project — by: operator "go all" (round 2, item 1)
D2 — 2026-10-05 — Unlimited = all host cores; RAM = floor((host MiB − 2048)/128)×128, i.e. the full host as Kata can size it, no reserve — by: operator "go all" (round 1, item 2), measurements above
D3 — 2026-10-05 — Every change applies by recreate (running: now; stopped: next start); the disk grows offline at boot in the entrypoint; no online grow — by: operator "go all" (round 2, item 3)
D4 — 2026-10-05 — Disk grow-only, 5–500 GB — by: operator "go all" (round 1, item 4)
D5 — 2026-10-05 — `STYRMAN_VCPUS`/`STYRMAN_MEMORY_MB` removed, WARN when set; the operator disk setting stays as the default for new projects — by: operator "go all" (round 1 item 5, round 2 item 8)
D6 — 2026-10-05 — host-setup.sh writes the clh wrapper and sets reclaim; the self-updater is not extended (new root trust surface) — by: operator "go all" (round 1 item 7, round 2 item 6)
D7 — 2026-10-05 — Existing projects recreate once on their next start (sizing mismatch) — by: operator "go all" (round 1 item 6, round 2 item 7)
D8 — 2026-10-05 — Setup VM gets unlimited CPU/RAM, no UI — by: operator "go all" (round 1, item 8)
D9 — 2026-10-05 — No resource fields in the create dialog — by: operator "go all" (round 1, item 9)
D10 — 2026-10-05 — UI home: "Resources…" in the sidebar project menu — by: operator "go all" (round 2, item 2)
D11 — 2026-10-05 — Units: CPU whole cores, RAM whole GiB, disk GB; 0 = unlimited; new projects snapshot the operator disk default at create — by: operator "go all" (round 2 items 4, 5)
D12 — 2026-10-05 — A failed in-guest disk grow is logged in the guest only; the record keeps the request and the next boot retries; UI reporting is a follow-up — by: operator "go all" (round 3, item 3)
D13 — 2026-10-05 — Stored values above the host's size are clamped at create with a WARN, not refused — by: operator "go all" (round 3, item 4)
D14 — 2026-10-05 — A PUT whose values equal the record is a 200 no-op, decided against the record — by: implementer
D15 — 2026-10-05 — PUT decides against the LIVE container: rebuild only when the running container's sizing differs from the request; a record-only difference is persisted without a restart; GET reports `pending` — supersedes D14 — by: cold read rounds 1 and 2 (.agents/docs/reviews/2026-10-05-cold-read-project-resources.md; same shape as network.go, security review 2026-09-09 round 3)
D16 — 2026-10-05 — Records with `DiskGB == 0` are backfilled once at startup — by: cold read round 1, gap 3
D24 — 2026-10-05 — No backfill: `DiskGB == 0` means "keep what the container has" (its `STYRMAN_DOCKER_DISK_GB`, read before any recreate), else the operator default when there is no container — supersedes D16 — by: operator "go all" (round 3, item 2)
D28 — 2026-10-05 — One release. host-setup.sh reaches each host with the signed tag (the updater advances the clone; `host_converge` only manages the firewall), so the root re-run comes AFTER the server update; on the GCP VM (disposable) project starts may fail until it is done; dave already has the clh wrapper — supersedes D17's "before the release" order — by: operator "B sounds smarter." (at the cold-read cap)
D29 — 2026-10-05 — The host-setup test lives beside the code it tests, `deploy/lib/kata-config-test.sh` (as `deploy/firewall/test.sh` does), and `go test ./internal/config/` runs it (`TestKataConfigScript`), instead of a separate script under `testing/styrman/` — by: implementer
D30 — 2026-10-05 — The host size is read lazily (`docker info` on first need, cached only on success), not at `New`: eight httpapi test files call `orchestrator.New` without a daemon, and a slow dockerd after a reboot must not block the listener (`cmd/styrman/main.go:93`). A failed read refuses that start or that GET (503) and is retried next time — supersedes the Interfaces' "fatal at startup" — by: implementer
D17 — 2026-10-05 — No server-side hypervisor probe: the server container cannot see the host's shim. A host is brought to clh + reclaim by re-running `host-setup.sh` before the release; the order goes in the release notes, the deploy guides and the gate report — by: implementer, after cold read round 1 gap 4
D18 — 2026-10-05 — `RunOpts.Resources` is required (nil refuses the start) and built by one constructor, `runOptsFor` — by: cold read round 1, gap 2
D19 — 2026-10-05 — Host size from `docker info` (NCPU, MemTotal) — by: cold read round 1 (macOS dev; the daemon's own validation count)
D20 — 2026-10-05 — GET `/resources` with `limits` and `pending` — by: operator "go all" (round 3, item 1)
D21 — 2026-10-05 — Any project owner may set any size up to the host — by: operator "go all" (D1); cold read round 1 question 13
D22 — 2026-10-05 — The rebuild bound reuses `lanRebuildTimeout`, renamed `rebuildTimeout` — by: cold read round 1 scope note
D23 — 2026-10-05 — REQUIREMENTS.md targets stand unchanged (10+ concurrent, reopen < 5 s). Measured on dave: 10 idle VMs at 16 vCPU / 61696 MiB each raised host memory in use from 4.1 to 15.8 GB (48 GB still available); boot at full size 1.8 s; a 20 → 500 GB grow 0.14 s — by: operator "go all" (round 3 note), measurements 2026-10-05
D25 — 2026-10-05 — A host whose RAM ceiling is ≤ 0 (under 2176 MiB) gets no memory limit (Kata's default) and the dialog hides the RAM row — by: cold read round 2 question 8 (default taken)
D26 — 2026-10-05 — `nproc` in the guest is the setting + 1 (Kata's base vCPU); accepted and explained in the dialog; the workload cap is exact — by: operator "go all" (round 3 note)
D27 — 2026-10-05 — GET reads the container without the project lock (`Orchestrator.LiveSizing`), so opening the dialog never waits behind a boot — by: cold read round 2 question 3 (default taken)
Open: Attach criterion — how to produce a live-sandbox `ContainerUpdate` without a stop — default: flip the policy on a running project via a start call, read the cgroup before any stop (now in Verification)
Open: Tier 2 check that the container ID is unchanged across stop/start — default: yes (now in Verification)
Open: `live` through the `recreateMissingNetwork` recursion (`orchestrator.go:382`) — default: pass `want` into the recursive call so it is never recomputed after a removal
Open: `reclaim_guest_freed_memory` key missing from the toml — default: `kata_clh_reclaim` prints `missing` and returns non-zero; host-setup FATALs; the shell test covers it
Open: `Host` naming, `New`'s context, `Info` in `dockerAPI` — default: unexported field `host`, a 10 s bounded context for the `Info` call, `Info` added to `dockerAPI` and the fake
Open: PUT body decode — default: pointer fields; a missing or non-integer field → 400 naming it
Open: a parsed disk env of 0 — default: treated as absent → operator default
Open: Tier 2 disk tolerance — default: byte delta from `df -B1` within ±2% of 1 GiB; the starting size read from GET
Open: held-lock test of `LiveSizing` — default: an orchestrator test that holds the project lock and calls `LiveSizing` with a 1 s deadline
Open: `wsboot_attach_test.go` edit — default: only if those literals reach a start; pass-count clause run bare
Open: docs for the entrypoint grow — default: guide.md and deploy.md in Phase 4's docs task
Rejected: Kata `default_vcpus = -1` in the toml for "unlimited" CPU — it only covers CPU, needs root on every host, and still needs a server-side number for memory.
Rejected: online grow via `losetup -c` + `resize2fs` in a running guest — unverified in Kata, and a second apply path beside the recreate.
Rejected: resolving resources inside the orchestrator from the store — the orchestrator holds no store today; a required reader in `RunOpts` keeps the existing `Lan` shape.
Rejected: a mismatch recreate for an existing setup VM — setup VMs are reaped and re-created at the new size.
Rejected: passing `cpus − 1` to Kata so `nproc` equals the setting — the workload cap would then be one CPU short of what the user set.
Rejected: PATCH on `/api/projects/{project}` for resources — that route's body is `{name}` (rename); a separate resource keeps the network.go pattern (GET/PUT per concern).
