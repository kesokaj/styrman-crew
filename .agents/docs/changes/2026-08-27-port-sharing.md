# 2026-08-27 — Shareable port previews

New feature (operator request): the owner can grant specific Google emails
access to a project's **port previews** — and only the previews.

- `project.Project` gains `SharedEmails` (per-project, in the owner's store);
  `Store.SetShares` replaces the list.
- `GET/PUT /api/projects/{project}/shares` (owner-only) read/replace the list;
  emails are normalized (lowercase/trim/dedupe) and validated.
- `authGuard` (header mode): a Google user who is NOT a styrman user is now
  let through **only** for port-preview routes (`/proxy/{p}/{port}/…`,
  `/p/{p}/{term}/port/{port}/…`) — every other route still 403s, so they never
  see the dashboard, terminals, or files.
- `proxyPort` resolves the project across owners (`previewAccess`) and allows
  the owner always, a shared email preview-only, everyone else 403. The
  attach/IP lookups run in the owner's namespace.
- UI: a "Share port previews" box in the Ports side-panel tab — add/remove
  Google emails; owner-only by default.

Combined with N22 (the preview is stripped of the user's session and
sandboxed), a shared guest gets a clean, isolated view of just the exposed
port. Verified live: a non-user is 403'd everywhere until shared, then reaches
the preview (502 with nothing listening, i.e. access granted) while still
403'd on files/dashboard.

Backend tests: emailIn, isPreviewPath, email validation. integration 56/56,
e2e 33/33.
