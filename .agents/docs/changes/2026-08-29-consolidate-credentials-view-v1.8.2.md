# Consolidate Settings into one status-colored Credentials view (v1.8.2)

Date: 2026-08-29. Operator: the "Tools & credentials" cards and the separate
"Credentials" list overlapped — collapse them into one view, with a status
colour per credential.

## Change

Removed the per-tool `ToolCard`s and the "Tools & credentials" heading. The
`Credentials` section is now the single view:

- **Standard credentials** — git, ssh, github, gcloud, claude, agy — each a row
  with a **status dot**: GREEN = signed in AND synced to the broker, GREY = not
  set up, RED = signed in locally but NOT synced (a gap to fix by re-running
  setup / Import). Each row shows a one-line detail (git identity, gh user,
  paths) and is **clickable to read** its stored value.
- **Custom variables** — broker env creds beyond the standard ones — listed,
  clickable, removable.
- **Add a variable** and **Import from a project** (marked optional) unchanged.

The status is computed client-side from the existing endpoints (`setup/status`
for "set up", `credentials` for "synced", `tools` for the detail line) — no new
backend. `web/src/routes/settings.tsx`.

The page is now: Account · First-time setup · Credentials · Appearance ·
Runtime.

## Verification (live, 1.8.2)

Setup status drives the colours: git/ssh/github/claude/agy are set + synced
(green); gcloud is not set (grey); no red (everything signed in is synced).
`tsc -b` + `go build` green.

## Release

Built + deployed live as styrman-server 1.8.2.
