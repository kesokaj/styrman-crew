---
owner: bugs
date: 2026-09-03
id: BUG-003
status: fixed
area: internal/orchestrator, cmd/styrman (transcript janitor)
---
# BUG-003: project home path trusted as a Docker bind source and as a root-run janitor's root

## Symptom
Phase 3 security review round 2 (`.agents/docs/reviews/2026-09-03-security-harness-phase3.md`)
found two more consequences of the invariant BUG-002 already disproved — that
`HomesDir/<user>/p/<uuid>` is server-controlled. It is not: the setup VM
bind-mounts `HomesDir/<user>` with a NOPASSWD-sudo guest, so the guest can
replace `p` or `p/<uuid>` with a symlink.
1. `internal/orchestrator/orchestrator.go` created the project home with
   `os.MkdirAll(phome)` (returns nil through a symlink-to-dir) and passed the
   raw path to Docker as the bind SOURCE — the host kernel resolves it, so a
   guest-planted link would mount any host path (e.g. `/etc`, another
   tenant's home) into the project VM, root guest inside.
2. `cmd/styrman/main.go`'s `pruneTranscripts` (root, every six hours) did
   `os.ReadDir`/`os.OpenRoot` on the raw `<homes>/<u>/p/<entry>` path and
   `Remove`d under `.styrman/logs` — a symlinked entry would delete under a
   host path. Its comment asserted safety on the same broken invariant.

## Repro
`internal/orchestrator/realbelow_test.go` `TestRealBelowRefusesSymlinkComponents`;
`cmd/styrman/prune_test.go` `TestPruneTranscriptsRefusesASymlinkedProjectHome`
(a real project's old transcript is pruned; one behind a symlinked
`p/<uuid>` is untouched).

## Root cause
ADR-031 nested per-project homes INSIDE the base home that the setup VM
mounts. Three code paths that turn that path into a mount source or a
root — the orchestrator's Docker bind, the root janitor, and `internal/files`
(BUG-002) — all assumed the path was real, but a guest with NOPASSWD sudo in
the mounted base home can replant any component below it as a symlink.

## Fix
Round 2 (incomplete — see "also went wrong" below):
`internal/orchestrator/orchestrator.go`: new `realBelow(base, target)` lstats
every component below the base home; only the CREATE branch of
`EnsureRunning` (`docker create`) was gated before `docker create`.
`cmd/styrman/main.go`: janitor opens `os.OpenRoot(<homes>/<u>)` and reaches
`p/<uuid>` via `Root.OpenRoot` (symlink-confined), stats via
`fs.Stat(home.FS(), rel)` — a sub-root's `DirEntry.Info()` lstats a path with
the sub-root prefix repeated and fails under Go 1.25, which would have
silently pruned nothing (caught by the new test on first attempt; fixed with
`fs.Stat`). `internal/files` got the same class fix as BUG-002
(`files.Trust`).

Round 3 fix (2026-09-03, after security review round 3 found the START
branch unguarded): the check is hoisted to the TOP of `EnsureRunning`, before
the create/start switch, as `projectHomeIsReal(base, phome)` (a `realBelow`
variant that permits a not-yet-existing home but refuses a dangling or live
symlink), plus the existing re-check after `MkdirAll` on the create path.
Test: `internal/orchestrator/realbelow_test.go`
`TestProjectHomeIsRealAllowsMissingRefusesLinks`.

Residual, now stated precisely: the window between the check and Docker's
own path resolution at create/start is milliseconds — a guest would have to
race it from a running setup VM; `EnsureRunning` itself still has no unit
test because the orchestrator has no fake Docker client (a known gap). The
durable fix remains un-nesting project homes out of the mounted base home
(ADR proposal, TODO.md). Reports:
`.agents/docs/testing/2026-09-03-verify-harness-phase3-round2.md`,
`.agents/docs/reviews/2026-09-03-security-harness-phase3.md` (round 3). The
review did not converge in three rounds; the operator decides whether a
fourth targeted round runs before release.

## Also went wrong (second attempt required)
Round 2's fix gated only the CREATE branch of `EnsureRunning`. The START
branch — `ContainerStart` of an existing, stopped container — had no check.
Docker (verified on this host's Engine 29.7.2) stores a bind mount's source
unresolved and re-resolves it fresh on every start, so a guest could plant
the symlink at `p/<uuid>` from the setup VM, then simply Stop/Start the
project (a routine action; the idle reaper also stops projects) to get the
target mounted as `$HOME` with root — no race needed. The round-2 record's
"not atomic with `docker create`" language understated this: the exposure
was a whole unguarded code path, not a narrow timing window. Root cause of
the miss: the fix was reasoned about only against the code path exercised by
the reported repro (create), not against every path that hands a value to
Docker.

## Prevention
A path that becomes a mount source, a root, or a deletion base is only as
trusted as its least-trusted parent: before handing it to Docker,
`os.OpenRoot`, or a remove, prove no component below the last
server-controlled directory is a symlink (`realBelow`/`projectHomeIsReal` for
bind sources, `files.Trust`/`Root.OpenRoot` for roots), on EVERY path that
reaches that sink — not just the one in the repro. For Docker bind sources
specifically: check both CREATE and START, since Docker re-resolves a stored
bind source on each start, not just at create. Never nest a tenant-owned
tree inside a directory a guest can write. When walking a sub-root, stat
through `Root.FS()`, not `DirEntry.Info()`.

## Related
BUG-002 (same broken invariant, `internal/files`). Third occurrence of the
"trusted-path-below-guest-writable-mount" class across BUG-002/BUG-003 — the
`realBelow`/`files.Trust` confinement rule should graduate into this
project's `AGENTS.md` or a `.agents/rules/` file so new code touching
`HomesDir/<user>/p/*` inherits it instead of re-deriving it.
