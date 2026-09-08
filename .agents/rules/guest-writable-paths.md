---
name: guest-writable-paths
description: Any host path with a component a guest can replace is untrusted as a root, a mount source, or a deletion base — confine it, never open it by name.
---

# Guest-writable paths (BUG-002, BUG-003)

The tenant dir `HomesDir/<user>` is server-only since ADR-064; what a guest
can write is a separate bind below it — `base/` (the setup VM) and
`p/<uuid>` (each project). Before ADR-064 the setup VM mounted the tenant
dir itself, with every project home inside it, so `p/<uuid>` could be
replaced by a symlink between two server operations (BUG-002/003). The
guards below stay: a path is only as trusted as its least-trusted parent,
and everything the guest writes below any mounted home can still be a link.

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

- **Server-only, root-read (ADR-065):** `StateDir/netpolicy/lan/<uuid>` is
  written by the server and read by the ROOT firewall script by NAME only. It
  sits beside `users/` and `projects/`, never under a mounted home, so no
  guest can plant a link there; the root side still refuses symlinks, asserts
  the owner, and treats names as the whole grammar (BUG-009: root code below
  a lower-privileged owner's directory never `mkdir -p`s or chowns through a
  path it has not just asserted is a real, correctly-owned directory).

The race between a check and the operation was the residual while tenant
trees sat under a guest-writable directory; ADR-064 (v1.21.0) removed that
layout, and the guards above remain as a second line.
