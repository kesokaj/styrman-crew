---
owner: bugs
date: 2026-09-04
id: BUG-004
status: fixed
area: internal/files, internal/history
---
# BUG-004: file/history listings below a trusted sub-root came back empty

## Symptom
v1.20.0 release-gate browser e2e (`.agents/docs/testing/2026-09-04-verify-v1.20.0.md`, round 1) failed 21/63 checks: Files panel showed "Empty directory" for a freshly created, non-empty project; file explorer, editor, and clone checks cascaded (no `.git` visible, clone "timeout"). The integration suite had passed minutes earlier — its listing check only grepped for the JSON key `entries`, which an empty list also satisfies.

## Repro
`internal/files/trust_test.go` `TestListWorksBelowATrustedBase`, `internal/history/search_test.go` `TestListBelowATrustedBase`: a `files.Trust` base, a directory with a file below `<base>/<tenant>/…`, `List` returned zero entries before the fix.

## Root cause
BUG-002's fix (`files.Trust`, commit cd11dd0, 2026-09-03) made every root below a tenant base an `os.Root` reached via `Root.OpenRoot(rest)`. Under Go 1.25 a `DirEntry` read from such a sub-root computes `Info()`'s lstat path with the sub-root's prefix repeated, so `Info()` fails with "no such file or directory"; `files.List` and `history.List` treated an `Info()` error as "skip this entry" and silently returned nothing. The transcript janitor hit the same quirk a day earlier (BUG-003) and was fixed locally with `fs.Stat`, but the lesson was not swept over the other `Info()` callers. Untrusted temp-dir-root unit tests never exercised the sub-root path, and the integration check was too weak to notice an empty result.

## Fix
`internal/files/files.go` `List` and `internal/history/history.go` `List` now stat through the root (`rt.Lstat(path.Join(rel, name))`) instead of calling `DirEntry.Info()`; tests above added. `.agents/docs/testing/styrman/integration.sh` now asserts a real entry (`.git` in the workspace, `.styrman` in the home), not just the `entries` key. Commit: the 2026-09-04 release-gate fix commit. Reports: `.agents/docs/testing/2026-09-04-verify-v1.20.0.md` (failing round), `…-round2.md` (re-run, passing).

## Prevention
Never call `DirEntry.Info()` on entries read through an `os.Root` — stat through the root instead (`rt.Lstat(...)` / `fs.Stat(root.FS(), ...)`). Every unit test of a root-reading function needs a case below a `files.Trust` base, not just a bare temp dir. An integration check of a listing must assert a known entry, never just the presence of the `entries` key.

## Related
BUG-002 (introduced the `os.Root`/`files.Trust` sub-root path), BUG-003 (hit the same `Info()`-on-sub-root quirk in the janitor one day earlier and fixed only that one caller — the second attempt this quirk has needed). Three related records now share the "root-reading code must stat through the root, and be tested below a trusted sub-root" lesson; it should graduate into `AGENTS.md` or a `.agents/rules/` file (e.g. a `files.Trust` usage rule) rather than being rediscovered per caller.
