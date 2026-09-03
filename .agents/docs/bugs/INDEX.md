# Bugs

<!-- The project's memory of what went wrong. One row per BUG-NNN record in this directory; newest last. Maintained by the `bugs` agent. Read at resume; consult before touching an area listed here. -->

| ID | Date | Area | Status | Symptom | Prevention |
|---|---|---|---|---|---|
| BUG-001 | 2026-09-03 | web/src/lib/api.ts (session/auth) | fixed | sidebar/stats bar show raw "Failed to fetch" after ~5-15 min, F5 fixes it | fetch() can't follow the ingress's cross-origin auth redirect; use `redirect: "manual"` and check `res.type === "opaqueredirect"` as session-expired |
| BUG-002 | 2026-09-03 | internal/files | fixed | `os.OpenRoot(root)` followed a guest-plantable symlink for project home roots, escaping to host paths/other tenants | a `files.*` root below a guest-writable mount must go through `files.Trust`'s tenant confinement, never `os.OpenRoot(path)` directly |
| BUG-003 | 2026-09-03 | internal/orchestrator, cmd/styrman | fixed (residual: check-to-Docker-resolution window; no fake-Docker-client unit test) | project home path used raw as a Docker bind source (create AND start) and as the root janitor's root, both guest-symlink-plantable; round-2 fix missed the start path, letting a routine Stop/Start remount the guest's target as root | prove no path component below the last server-controlled dir is a symlink on EVERY path that hands the path to Docker (create AND start: Docker re-resolves a stored bind source on each start); never nest tenant trees in a guest-writable mount |
