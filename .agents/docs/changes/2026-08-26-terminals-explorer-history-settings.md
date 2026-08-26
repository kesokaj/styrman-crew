# 2026-08-26 — One concept (terminals), file explorer, transcripts, ports links, settings

## Summary

Operator design review drove a consolidation plus four features. Suites:
unit green, integration **40/40**, browser e2e **27/27**.

## Design fix (ADR-016)

"A conversation is actually a terminal — why do we have terminals on the
side-panel?" Right. Conversations and right-rail shells were the same thing
in two places. Now: a project has **terminals** (durable, titled, in the
sidebar, listed even when stopped); run several for several agents. The
ephemeral shell concept and its endpoints are gone;
`/conversations` → `/terminals`; default title "Terminal N". The side panel
now carries project *context* instead: Files / Ports / History / Diff.

## Features

- **File explorer** (ADR-018, `internal/files`): browse/view/edit/download
  the project workspace and the shared home. Served from the host mount, so
  it works while the microVM is stopped. Traversal and symlink escapes
  refused (tested).
- **Transcripts / history** (ADR-017, `internal/history`): a tmux hook in
  the runtime image pipes every pane to
  `~/.styrman/logs/<project>/<session>.log`; the server serves it
  ANSI-stripped. Everything you and the agents do is recorded and survives
  container rebuilds.
- **Diff**: `git status`/`diff` for the workspace, run host-side (git added
  to the server image) so it works stopped too.
- **Port links**: `/p/<project>/<terminal>/port/<port>/` — the exact URL
  shape the operator asked for — proxying to the app inside the microVM.
  `/proxy/<p>/<port>/` still works; bare forms 301 to the trailing slash.
- **Settings page**: signed-in user, auth mode, allowed accounts (add/revoke
  via userstore), setup status + re-run, theme, runtime + storage facts.

## Failures / gotchas

1. ServeMux rejected `/p/{project}/port/{port}/` alongside
   `/p/{project}/{term}/port/{port}/` — `{term}` can match the literal
   "port", so the prefix patterns overlap. Kept the terminal-scoped form
   (the operator's example) + `/proxy/...` for terminal-less links.
2. The server image lacked `git`, so the diff endpoint returned an empty
   repo silently. Added git; caught by testing the endpoint, not by a build
   error.
3. Default titles still said "Conversation N" after the rename — the e2e
   suite caught it via a selector miss, which is exactly what it is for.

## Verification

- integration.sh now 40 checks: adds terminals-as-records, unknown-terminal
  attach refused, transcripts (incl. survival across container recreate),
  file explorer (list/write/read/download/traversal), ports + all three URL
  shapes, diff, settings, terminals-listed-while-stopped.
- e2e now 27 checks: adds Files/Ports/History/Diff tabs, port link serving a
  real app, transcript content, settings page, both agents installed.
- Production (app.styrman.xyz) still redirects to Google sign-in; API
  unauthenticated is refused.
