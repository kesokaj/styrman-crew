---
owner: main
date: 2026-10-05
---
# Per-project resources, Phase 3: the Resources dialog   (2026-10-05)

Plan: `.agents/docs/plans/2026-10-05-project-resources.md` (Phase 3).

- **Sidebar:** the project menu has "Resources…" after Rename, which opens
  `ResourcesDialog`.
- **`ResourcesDialog`** (`web/src/components/ResourcesDialog.tsx`, new):
  - CPU and Memory each have an "Unlimited" checkbox and a number, limited to
    the host (`limits` from the GET). The CPU row notes that the terminal shows
    one CPU more than the setting.
  - The Docker disk is a number with "Grow only, min–max GB". The Memory row is
    replaced by a note on a host too small to set memory.
  - A "Running at its previous size" line appears when `pending`.
  - The text above Save says a running project restarts, and that only home,
    workspace and apt packages survive.
  - Save is disabled while nothing would change, unless `pending`.
- **Form logic:** `web/src/lib/resources.ts` (new): `formFrom`, `bodyFrom`
  (whole numbers within the limits, the first error in field order) and
  `unchanged`. Tests are in `resources.test.ts` (17 tests).
- **API client** (`web/src/lib/api.ts`): `ProjectResources`,
  `projectResources`, and `setProjectResources` with a 210 s timeout paired
  with the server's `rebuildTimeout` (BUG-022). The network PUT's comment now
  names `rebuildTimeout`. `api.encoding.test.ts` gains both calls.
- **Settings:** "Docker disk / project" is now "Default Docker disk for new
  projects".
- `web/dist/client` rebuilt.

Verification: `verifier`, `.agents/docs/testing/2026-10-05-verify-project-resources-phase-3.md`
(PASS: `npm run check` and `npm run lint` exit 0; `npm test` 19 files / 230
tests vs 18 / 211 at HEAD; the bundle rebuilds to the same assets; `go build`
ok). Not covered: the dialog in a real browser (Phase 4 e2e). The component
files have no unit tests of their own, only the form logic. Diff read by me.

## Learnings for the local host
none
