---
name: guest-writable-paths
description: Any host path with a component a guest can replace is untrusted as a root, a mount source, or a deletion base — confine it, never open it by name.
---

# Guest-writable paths (BUG-002, BUG-003)

A project home lives at `HomesDir/<user>/p/<uuid>`, INSIDE the base home
the setup VM bind-mounts with a root guest (ADR-031). So `p` and `p/<uuid>`
— and everything the guest writes below any mounted home — can be replaced
by a symlink between two server operations. A path is only as trusted as
its least-trusted parent.

- **Roots:** open through `internal/files` (`files.Trust` registers the tenant
  bases; `openRoot` reaches the root with `os.OpenRoot(<base>/<tenant>)` +
  `Root.OpenRoot(rest)`). Never `os.OpenRoot(path)` on a path below a
  mounted home; never `os.ReadDir`/`os.Stat` it by name either.
- **Mount sources:** on EVERY path that hands the path to Docker — create
  AND start, since Docker stores a bind source unresolved and re-resolves
  it on each start — prove no component below the last server-controlled
  directory is a symlink (`projectHomeIsReal`/`realBelow` at the top of
  `EnsureRunning` in `internal/orchestrator`). `os.MkdirAll` succeeds
  through a symlink-to-dir and the host kernel resolves the bind.
- **Deletion bases:** the transcript janitor is the model — list and open
  through the tenant root; stat with `fs.Stat(root.FS(), rel)`, not
  `DirEntry.Info()`, which fails on a sub-root under Go 1.25.
- **Reading through any confined root:** never `DirEntry.Info()` after
  `ReadDir` — use `rt.Lstat`/`rt.Stat` through the root (BUG-004: the first
  `files.Trust` cut turned every project listing into "Empty directory").
  Every unit test of a root-reading function needs a case below a
  `files.Trust` base; an integration check of a listing asserts a known
  entry, never just the JSON key.
- **New paths:** when a new tree is nested under an existing mount, ask who
  can replace its components before writing the code, and add a test that
  plants the symlink (`internal/files/trust_test.go`,
  `cmd/styrman/prune_test.go`, `internal/orchestrator/realbelow_test.go`).

The residual is the race between the check and the operation; the durable
fix is to stop nesting tenant trees under guest-writable directories
(`TODO.md`, the un-nesting ADR proposal).
