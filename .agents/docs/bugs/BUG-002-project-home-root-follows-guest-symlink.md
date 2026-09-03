---
owner: bugs
date: 2026-09-03
id: BUG-002
status: fixed
area: internal/files
---
# BUG-002: files.openRoot opened a project home root that a guest could replace with a symlink

## Symptom
Security review of the harness replacement's Phase 3
(`.agents/docs/reviews/2026-09-03-security-harness-phase3.md`, round 1, high)
found `internal/files/openRoot` called `os.OpenRoot(root)` directly. Its
comment claimed the root is "server-controlled and guaranteed real", but a
project home (`HomesDir/<user>/p/<uuid>`) sits inside the base home
(`HomesDir/<user>`) that the setup VM bind-mounts
(`internal/orchestrator/orchestrator.go:571`), where the guest has NOPASSWD
sudo. The guest could replace `p/<uuid>` with a symlink to any host path
(e.g. `/etc`) or to another tenant's home; the server would then open it as
its filesystem root — host-path read/listing via the file explorer, plus a
name leak and existence oracle in the new settings roster. Reviewer verified
this empirically.

## Repro
`internal/files/trust_test.go` `TestTrustedRootRefusesAPlantedSymlink`: a
tenant base with `alice/p/to-host -> <dir outside>` and
`alice/p/to-bob -> bob/p/x`. Before the fix, `files.Exists(to-host, "passwd")`
returned true.

## Root cause
The `files` package assumed every root path handed to it is server-controlled
and cannot be swapped out from under it. ADR-031 nested project homes under a
directory the guest controls (the bind-mounted base home), and nothing
re-examined that assumption when the nesting was introduced.

## Fix
`internal/files/files.go`: `files.Trust(bases...)` registers tenant bases;
`openRoot` now opens any root under `<base>/<tenant>/…` as
`os.OpenRoot(<base>/<tenant>)` then `Root.OpenRoot(rest)`, so every path
component below the tenant boundary is symlink-confined (host paths and other
tenants refused; relative links inside the tenant allowed; absolute links
refused). `httpapi.New` registers `cfg.HomesDir` and `StateDir/projects`.
Also hardened `files.ListNames` to cap `ReadDir` at 1000 (review's medium:
unbounded read of a guest-writable, polled directory). Landed in the Phase 3
commit of the harness replacement (2026-09-03).

## Prevention
A `files.*` root is only as trusted as its least-trusted parent: any root
below a directory a guest can write (e.g. a bind-mounted home) must be opened
through `files.Trust`'s tenant confinement, never `os.OpenRoot(path)`
directly. When a new path gets nested under an existing mount, re-check who
can replace its path components before assuming the root is "server
controlled."

## Related
Reviews: `.agents/docs/reviews/2026-09-03-security-harness-phase3.md`
(rounds 1 and 2). Testing:
`.agents/docs/testing/2026-09-03-verify-harness-phase3.md`. ADR-031 (nested
project homes) is the design decision that introduced the least-trusted
parent.
