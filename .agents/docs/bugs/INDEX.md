# Bugs

<!-- The project's memory of what went wrong. One row per BUG-NNN record in this directory; newest last. Maintained by the `bugs` agent. Read at resume; consult before touching an area listed here. -->

| ID | Date | Area | Status | Symptom | Prevention |
|---|---|---|---|---|---|
| BUG-001 | 2026-09-03 | web/src/lib/api.ts (session/auth) | fixed | sidebar/stats bar show raw "Failed to fetch" after ~5-15 min, F5 fixes it | fetch() can't follow the ingress's cross-origin auth redirect; use `redirect: "manual"` and check `res.type === "opaqueredirect"` as session-expired |
