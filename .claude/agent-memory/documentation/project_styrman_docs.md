---
name: project-styrman-docs
description: Doc structure and conventions for the styrman app inside super-devbox
metadata:
  type: project
---

styrman is one app living in the `super-devbox` repo (repo itself has no
top-level git in this sandbox view, but is treated as the project root).
Its living docs:

- `.agents/docs/specs/styrman/api.md` — REST API spec (tables per resource
  group: Projects, Terminals, Ports & preview, Files, History & diff,
  Settings/profile/admin, Setup, Misc, Server environment).
- `.agents/docs/specs/styrman/runtime-image.md` — the per-project microVM
  image spec; has a numbered "Entrypoint contract" step list (steps 7b etc.
  are sub-steps of the entrypoint script) that must track `image/entrypoint.sh`.
- `.agents/docs/specs/styrman/terminal-protocol.md` — the browser↔server
  WebSocket protocol at `/ws/term/{project}/{conv}` (frames table, close
  codes table, session semantics, transport internals).
- `.agents/docs/specs/styrman/guide.md` — the single "living guide": what it
  is, architecture ASCII diagram, install, operating it, troubleshooting,
  development, roadmap. Numbered "Load-bearing ideas" list in section 1 is
  where cross-cutting UI/product features (terminals, side panel tabs,
  notifications) get a sentence or two.
- `README.md` (repo root) — has a "Why" value-prop section, not a literal
  feature list; don't force feature additions there unless it actually
  enumerates features.

**Style notes observed in these docs:**
- Path placeholders use angle brackets like `<id>`, `<t>`, `<uuid>` in prose
  and inside backticked paths (e.g. `` `/p/<id>/<t>/port/<port>/` `` ), NOT
  curly braces — curly braces are reserved for JSON field lists like
  `{id, title, live}`. Always re-read the exact current text before editing;
  don't assume brace style from memory.
- Terse, dense, table-heavy. New rows/sentences should match existing
  telegraphic tone (fragments over full sentences, em-dashes for asides).
- ADR references like `(ADR-016)`, `(ADR-017)` are inline citations back to
  `.agents/docs/architecture/DECISIONS.md` — keep referencing them when a
  feature ties to an existing decision; don't invent new ADR numbers.

**R13 / ADR-022 (git substrate, added 2026-08-27):** every project workspace
is guaranteed to be a git repo (`internal/gitrepo` package, host-side, works
while the microVM is stopped). Covered in `api.md` (Projects POST row +
new "## Checkpoints (ADR-022)" section) and `guide.md` (load-bearing idea
#5, "Operating it" bullets, dev package list). The create-project API also
grew a `github` field (`"private"`/`"public"`, creates+pushes via `gh`)
mutually exclusive with `cloneUrl` — not just the clone option — check
`internal/httpapi/server.go`'s `createProjectRequest` before describing
project-creation fields, code has grown past what any single feature
request describes. Side panel tab renamed Diff → Git (`web/src/components/
SidePanel.tsx`), holds working-tree diff/branch on top + checkpoints list
below.

**2026-08-27 security review (F9 code review, "Batch A") doc sync:** a
security pass removed the production auth-bypass dev listener (it bound
`0.0.0.0:8081` inside the server container, reachable from every project
VM), added `os.Root` confinement in `internal/files` for all home-relative
file ops, an opt-in `X-Styrman-Ingress` shared-token check
(`internal/httpapi/auth.go`), and pinned caddy-security's JWT signing key
via `STYRMAN_JWT_KEY`. Test suites grew: `.agents/docs/testing/styrman/
integration.sh` is 56 checks (grep `check "` to recount — don't trust a
remembered number, it has grown before); `web/e2e/run.mjs` is 33 Playwright
checks, now run via `web/e2e/run.sh` which self-hosts its own server on
host loopback with an isolated state dir (production has no dev door left
to point it at). Docs touched: `AGENTS.md`, root `README.md` (Status
section counts), `guide.md` (new architecture bullets: auth two-layer,
os.Root confinement, JWT pinning; architecture diagram's server line;
terminal load-bearing idea #3 grew copy-on-select/link-bar/search/OSC52/DOM-
renderer/Clips; idea #4 grew the Clips tab), `api.md` (added
`STYRMAN_INGRESS_TOKEN` to the env table), `deploy.md` (dev-door paragraph
rewritten past tense; new "Required secrets" list under Production mode),
`runtime-image.md` (entrypoint steps 4/7/7b/10 rewritten: reserved-username
rejection, tmux `mouse off`/OSC52, the SIGTERM trap + orchestrator's
`kill -TERM 1` workaround for Kata's `docker stop` not delivering SIGTERM),
`.agents/docs/testing/styrman/README.md` (counts + self-hosting), and
`web/src/lib/terminal.ts`'s header comment (fixed a stale claim of a WebGL
renderer — code has always used DOM here in this snapshot). Terminal
UI features live in `web/src/lib/terminal.ts` (xterm wrapper: copy-on-
select, link-rescue bar sniffing both buffer and raw stream, Ctrl+Shift+F
search, OSC 52) plus `web/src/lib/clips.ts` (session-scoped clipboard
history, ADR-023) and `SidePanel.tsx`'s Clips tab.

See [[feedback-styrman-doc-edits]] for edit-process lessons.
