# Project-scoped env variables — SidePanel Env tab (v1.12.0)

Date: 2026-08-30. Operator request: "We have the global ones but I would
also like to have project wide only as well. They should live in the
sidepanel."

## Design (ADR-043)

A new project env tier in the credstore, merged by the broker: the broker
already resolves every `/env` caller's source IP to `(user, project)`
(ADR-032/M17), so project scoping happens entirely broker-side — **zero
guest changes**, the login snippet is untouched. A project variable
overrides a same-named user-wide one for that project only; deleting it
falls back to the global; the whole scoped set dies with the project.

## Changes

- `internal/credstore`: project tier under `<user>/proj-<projectID>/
  env-KEY.json` — `PutProjectEnv` / `ProjectEnv` / `DeleteProjectEnv` /
  `DeleteProjectAll`; shared `validateEnv` + `readEnvDir` extracted from the
  user-wide tier (same identifier/single-line/≤4096-byte rules). `List`
  skips directories, so project vars never surface in Settings.
- `internal/credbroker`: `/env` merges `ProjectEnv(user, project)` over
  `AllEnv(user)` (project wins; a project-tier read error never takes the
  user-wide credentials down).
- `internal/httpapi/projectenv.go` (new): `GET/PUT /api/projects/{id}/env`,
  `GET/DELETE /api/projects/{id}/env/{key}` — list is keys-only, the
  single-value read is audit-logged; 503 when the broker is disabled.
  `deleteProject` now also calls `DeleteProjectAll` so scoped vars can't
  outlive their project.
- web: `SidePanelEnv.tsx` (new Env tab — list with click-to-reveal +
  remove, add form, copy explaining scope/override/new-shells), tab wired
  into `SidePanel.tsx`, api.ts helpers, Settings "Add a variable" hint now
  cross-references the per-project tab.

## Verification

- Unit: `TestProjectEnv` (isolation per project + per user, validation,
  delete/ErrNotFound, DeleteProjectAll leaves other projects intact; List
  never surfaces project vars), `TestBrokerEnvProjectMerge` (project wins
  on shared key, other projects' vars never leak). All suites green;
  tsc/oxlint/web build clean.
- **Live E2E on crew (v1.12.0 deployed):** stored `STYRMAN_E2E_CHECK` on
  project "test" via the API → a fresh login shell in "test" exported it;
  a login shell in "asdasd" did not. Added a same-named GLOBAL var →
  "test" kept the project value, "asdasd" got the global. Deleted the
  project var → "test" fell back to the global. Removed the global →
  clean everywhere. No test residue left.
