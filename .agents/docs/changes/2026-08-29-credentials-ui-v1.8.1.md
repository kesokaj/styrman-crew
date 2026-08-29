# Credentials: click-to-read, clearer custom vars, synced count in project (v1.8.1)

Date: 2026-08-29. Operator asks on the Settings → Credentials page and the
project view.

## Click a credential to read it

New `GET /api/settings/credentials/{kind}/{id}/content`
(`internal/httpapi/credentials.go`) returns a stored credential's readable
content — the operator's own secret on the admin-only settings surface, the
exact value the broker materializes into each project. Files return their text
(binary flagged), env/git return the value/token. The "Shared now" list rows are
now clickable and open a read-only `CredViewer` modal (reuses the syntax-
highlighted editor); binary secrets are described, not dumped. Files:
`internal/httpapi/credentials.go`, `internal/httpapi/server.go` (route),
`web/src/lib/api.ts`, `web/src/routes/settings.tsx`. The "Shared now" label now
shows the count.

## Custom variables clarified

The "Add a token" row is now "Add a variable" with copy making clear it stores
ANY custom env variable (NPM_TOKEN, DATABASE_URL, …), exported as a shell var in
every project, and that re-storing a name updates it. Same endpoint
(`putTokenCredential` → `PutEnv`); this was always a general env-var store — the
UI just says so now. The key input upper-cases as you type.

## "Import from a project" clarified

Its hint now says it is OPTIONAL — setup already auto-syncs (ADR-034); Import is
for refreshing or adding creds from a project you've signed into (a rotated
token, or a tool you signed into inside a project rather than setup).

## Synced-credential count in the project view

The terminal status bar now shows `creds N synced`, with a tooltip listing the
names — so each project shows what the broker has provisioned into it. Polled
slowly (30s); `web/src/components/TerminalStatusBar.tsx`.

## Verification (live, 1.8.1)

Content endpoint returns: env `STYRMAN_GIT_NAME` → "kesokaj"; file
`ssh-id_ed25519` → the private key text at `~/.ssh/id_ed25519`; git `github.com`
→ the 40-char token. `tsc -b` + `go build` green.

## Release

Built + deployed live as styrman-server 1.8.1.
