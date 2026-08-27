# 2026-08-27 — Targeted cleanup (post-review, operator-selected)

Four review findings the operator picked after the main batches.

- **N22 — port-preview no longer leaks the user's session into a project's
  app.** The reverse proxy strips `Cookie`/`Authorization`/`X-Caddy-*`/
  `X-Styrman-Ingress` from the outbound request (a project's app never sees the
  caddy-security cookie or the ingress token), and responses carry
  `Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups
  allow-modals allow-downloads` — the previewed page runs in a unique opaque
  origin, so it is not same-origin with the SPA and cannot `fetch('/api/…')`
  as the user or read styrman's cookies. Normal dev apps still work.
- **N16 — `tmux detach` no longer strands a live terminal.** A detach exits
  the attach exec cleanly, which previously sent close 4000 ("shell exited")
  and the client refused to reconnect. The bridge now checks `tmux
  has-session` after the exec ends: session alive → normal close (client
  reconnects); session gone → 4000.
- **N26 — a broken git repo no longer renders as an empty (clean) diff.**
  `GitDiff` checks `git status` (works on an unborn HEAD) and returns an error
  if it fails, instead of swallowing it.
- **N2 — terminal timers cleared on teardown** (`selTimer` in the terminal
  module, `toastTimer` in TerminalPane) so a copy/toast can't fire after the
  component unmounts.

Verified: go test/vet green, integration 56/56 (live port-preview), e2e 33/33,
prod redeployed.
