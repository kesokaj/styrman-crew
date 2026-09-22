---
owner: reviewer
date: 2026-09-22
---
# Review — phase0b-code   (2026-09-22)

Range reviewed: working tree — changed files: internal/files/files.go, internal/files/files_test.go, internal/files/rmcheck_test.go, internal/files/trust_test.go, internal/httpapi/explorer.go; untracked: internal/files/listbounds_test.go, internal/httpapi/managedtarget_test.go (git diff HEAD -- <files>, untracked files read whole)

## Status: COMPLETE

## Verdict: REQUEST CHANGES

The bounded-listing work (cap, `ReadDir(n>0)` handling, `SortStableFunc`) is correct and well tested. The managed-subtree symlink guard (`managedHomeTarget`/`linkedFirstComponent`) is not: it only inspects whether the FIRST path component is a symlink, so a guest can still reach `.dockerd`/`.ssh`/etc. through a symlink planted at any DEEPER component — the exact class of attack the fix's own doc comment describes, just moved down one level. Confirmed by direct reproduction against `os.Root` (below): a nested symlink is silently followed and the guarded delete/write/rename succeeds through it. This is the same vulnerability the diff was written to close, still open.

## Findings

- **[high / confidence: high]** `internal/httpapi/explorer.go:117-151` (`managedHomeTarget` / `linkedFirstComponent`) — the guard checks only `first`, the path's first `/`-separated component, for being a symlink. A symlink nested below a real top-level directory (e.g. `work/sub/link -> ../../.dockerd`) is invisible to it: `first="work"` is a real directory, so `linkedFirstComponent(root, "work")` reports "not a symlink" and the guard returns `false` (not managed) regardless of what `rest` (`sub/link/disk.img`) resolves through. `files.RemoveAll`/`Rename`/`Write`/`SaveStream` then run through `os.Root`, which follows symlinks that stay inside the root at every component (per the package doc comment) — so the delete/write/rename reaches the managed subtree anyway. Reproduced directly against `os.Root` (not the project's test files, a scratch verification at `.agents/scratch/nestedsymlink_check.go`, discarded after use): planting `root/work/sub/link -> ../../.dockerd` and calling `rt.RemoveAll("work/sub/link/disk.img")` deletes `root/.dockerd/disk.img` — the loop-mounted docker disk the guard exists to protect. `TestManagedHomeTargetRefusesASymlinkedFirstComponent` only plants the symlink directly under root (`x/disk.img`), so this gap has no test either. Fix: walk every component of `rel` up to (not including) the final element and refuse if any is a symlink, or resolve the path through `os.Root` component-by-component and reject on the first symlink found below the scope root — the guard needs the same at-every-component confinement the package's own read paths already have.
- **[low / confidence: medium]** `internal/httpapi/explorer.go:137-151` — `linkedFirstComponent` does its own `Lstat` via a freshly opened root, separate from the `os.Root` handle the actual delete/rename/write later opens and traverses. Between the guard's check and the operation there is a TOCTOU window in which the guest (who fully controls their own home) could replace a real top-level directory with a symlink. Secondary to the finding above — closing the component-walk gap removes most of the incentive to race this, but the check-then-act pattern is still worth collapsing into a single resolution once the primary fix lands.

## Criteria

1. **Dead code** — pass. The old `less` insertion-sort comparator has no remaining references (`grep` confirms); `Listing`/`compareEntries` are the only new exports/helpers and both are used.
2. **Flow tracing** — fail on the guard (finding above); the `Listing`/`Truncated` plumbing traces cleanly from `files.List` through `listFiles` to the JSON response, and `List`'s `ReadDir(maxListEntries+1)` correctly treats `io.EOF` as "empty, not an error" while still propagating any other read error (verified against the stdlib contract: for `n>0`, a non-empty partial read returns `nil` error, an empty read at EOF returns `io.EOF`, and any other error is returned even when entries were already collected — the code discards those and errors out, which is the safe choice). `go vet` and `go test ./internal/files/... ./internal/httpapi/...` both pass clean on the current tree.
3. **Standards conformance** — pass except the guard's completeness (covered above); `O_NONBLOCK` added to `ReadRawNoFollow`'s parent-directory open matches the pattern already used elsewhere in the package (openReg, List) and closes the one open the package doc comment says was missing it.
4. **Directory layout** — pass. New tests (`listbounds_test.go`, `managedtarget_test.go`) sit beside the code they cover, in the same packages.
5. **Simplicity / DIY-except-security** — pass. `slices.SortStableFunc` replaces a hand-rolled O(n²) insertion sort with the stdlib equivalent, preserving the same total order (dirs first, then case-insensitive name, verified by tracing both comparators and by the new stability test); no new dependencies; nothing here touches auth/crypto.
6. **Decoupling** — pass. `managedHomeTarget`/`linkedFirstComponent` and `List`'s cap are small, locally-scoped changes; no new cross-package reach-ins.

## Not examined

Nothing in the named scope was skipped — every changed and untracked file was read in full, `go vet` and `go test` were run against both packages, and the nested-symlink claim was independently reproduced against `os.Root` outside the project's own test files (a repo-wide `grep -rn "files\.List("` confirmed `internal/httpapi/explorer.go` is the only non-test caller of `files.List`, so the `[]Entry` → `*Listing` return-shape change has no other caller to check). Outside this scope entirely: the rest of the repository, and whether `internal/history` or any other package that calls into `internal/files` (e.g. its own `OpenRoot`/`Lstat` use) has a symlink-confinement gap of the same shape as the one found here — not investigated, since it is outside the named diff.

## Outcome

Appended by the main agent, in the commit that carries the fixes.

- **Finding 1 — `managedHomeTarget` inspects only the first component; a
  symlink nested deeper bypasses it: fixed** at `internal/httpapi/explorer.go`
  (`traversesALink`), which now lstats every component of the path EXCEPT the
  last through the confined root, with the same fail-closed error handling.
  The finding was reproduced before it was fixed:
  `TestManagedHomeTargetRefusesASymlinkNestedDeeper`
  (`internal/httpapi/managedtarget_test.go`) was written first and failed
  against the pre-fix code with "a managed subtree was reachable through a
  symlink below the first component", then passed after. The last component is
  deliberately still not inspected — acting on a link itself removes only the
  link, which `TestFileDeleteSymlinkEscape` requires; that carve-out has its
  own assertions at both depths.
- **Finding 2 — lstat-then-act race: rebutted, and the comment corrected.**
  Both sides of the window are the same principal acting inside the same
  confined home: a guest can swap a directory for a link between the check and
  the operation, and what it gains is the ability to delete its own project's
  managed files. The tenancy boundary is `os.Root`'s and is atomic, unaffected
  by this guard. Closing the race would mean carrying one root handle from
  check to operation through every `files.*` verb. The trade-off is now
  recorded where a reader meets it, at `internal/httpapi/explorer.go`'s
  `traversesALink` doc comment ("What this is NOT: a tenancy boundary"), which
  is the part of the finding that was worth acting on.

Verification after the fixes, every command run bare and its status read:
`go vet ./...` rc 0; `go test ./...` rc 0 (324 pass, 0 fail, 1 skip — 311 was
the pre-phase baseline, no test removed); `(cd deploy/caddy/ingress && go test
./...)` rc 0; `(cd web && npm run check)` rc 0; `(cd web && npm run lint)`
rc 0; `(cd web && npm test)` rc 0 (15 files, 178 tests).
