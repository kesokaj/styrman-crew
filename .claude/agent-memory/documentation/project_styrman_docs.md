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
  group: Projects, Git (real client, added v1.7.0), Terminals, Ports &
  preview, Files, Format (server-side, added v1.7.3), History & diff,
  Checkpoints (backend-only since v1.7.0 — automation removed, don't call it
  automatic), Settings/profile/admin, Setup, Misc, Server environment).
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

**2026-08-27 sharing + public links (ADR-024, ADR-025) doc sync:** two
features landed same-day as the security review above (don't confuse the
dates). ADR-024 = per-port preview sharing: owner grants specific Google
emails access to ONE port's preview via `project.Project.SharedPorts
map[int][]string` (JSON `sharedPorts`), enforced by `previewAllowed`
(`internal/httpapi/shares.go`) on the *existing* `/p/.../port/{port}/` and
`/proxy/.../` routes — NOT a new route. Required flipping the ingress label
on `styrman-server` from role-gated (`caddy_admins`) to `ModeAuthenticated`
(`docker-compose.yml`) so any Google account can sign in at all; the
server's own `authGuard` is still the real permission gate. ADR-025 = public
TTL preview links: owner mints a stateful, revocable token
(`internal/links`) for one port, served at `/link/{token}/...` (authGuard-
exempt, public, no oracle on 404) and — when `STYRMAN_PREVIEW_DOMAIN` is set
in `deploy/.env` — also at `https://<token>.<domain>/` via
`previewHostRewrite` + a `*.PREVIEW_DOMAIN` `ModePublic` wildcard block on
the ingress (`deploy/caddy/ingress/generator.go`). One env var
(`STYRMAN_PREVIEW_DOMAIN`) drives both the ingress `PREVIEW_DOMAIN` and the
server's `STYRMAN_PREVIEW_BASE`; empty = feature off. Docs touched: root
`README.md` (new Why bullet, architecture diagram gained a `/link/{token}/`
line, Status paragraph), `api.md` (new rows under "Ports & preview" for
`.../shares` and `.../links` + `/link/{token}`, `STYRMAN_PREVIEW_BASE` added
to the Server environment table), `deploy.md` (new paragraph after the
compose composition list explaining `ModeAuthenticated`, new "### Public
preview links (optional, ADR-025)" section, bring-up fill-in comment grew
`STYRMAN_PREVIEW_DOMAIN`). `guide.md` and `terminal-protocol.md` were NOT
touched — this feature has no terminal/WS surface. Full design rationale
lives in `.agents/docs/architecture/DECISIONS.md` ADR-024/ADR-025 (very
detailed — read those first before re-deriving from code next time).

**2026-08-29 session — v1.5.3 through v1.8.9 doc sync** (git client, network
isolation hardening, credential broker auto-seed, terminal keyboard). Read
per-task changelogs in `.agents/docs/changes/2026-08-29-*.md` (12 files,
v1.5.3–v1.8.9) — that's the authoritative source of what changed; don't
re-derive from git log alone, the changelog prose has the "why" and exact
endpoint names. Key facts for next time:
- **Automatic git checkpoints were REMOVED in v1.7.0** (`RunCheckpointLoop`
  deleted from `cmd/styrman/main.go`; `Server.snapshot` gone). The manual
  list/create/restore endpoints (`internal/httpapi/checkpoints.go`) still
  exist backend-side but nothing in the UI calls them — don't describe them
  as "automatic" or UI-driven anymore. The Git side panel tab became a real
  client instead: `internal/httpapi/git.go`, routes
  `/api/projects/{id}/git/{status,diff,stage,unstage,discard,commit,push}`.
  Backup story is now "microVM volume snapshots of /var/lib/styrman" — host
  infra, not yet built, tracked in TODO.md.
- **Credential broker auto-seeds on setup completion** (ADR-034, v1.5.4):
  `POST /api/setup/done` now calls the same `harvestCredentials` helper
  `POST /api/settings/credentials/import` uses, against the setup VM's
  shared home, before tearing it down. Manual Import is now a
  supplementary/refresh path, not the only way in — don't describe it as
  required in guide.md/deploy.md/README.md.
- **agy moved from the broker's keyring tier to the file tier** (v1.5.5):
  ADR-032a's "agy is a gnome-keyring credential" assumption was wrong for
  the real agy build — it persists `~/.gemini/antigravity-cli/
  antigravity-oauth-token`, a plain JSON file. The keyring tier
  (`credstore.KeyringCred`, `GET /keyring`, the login snippet's
  `secret-tool store` block) is left in place but **dormant** — nothing
  populates it anymore. Say "dormant", not "removed" — it's still wired,
  just unused.
- **Git identity + claude onboarding flag are now broker-synced too**
  (v1.7.1, v1.5.6): `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` env creds applied
  as `git config --global` on login; claude's `hasCompletedOnboarding` is
  jq-merged into `~/.claude.json` when a token is present. These are
  examples of "per-tool client state the broker doesn't sync automatically
  by default" — watch for the same class of bug with future interactive
  tools (noted as an open question in the v1.5.6 changelog).
- **Network isolation was inert until v1.8.3**: the ADR-029 firewall rule
  matched interface `sdb-+`, but Docker actually named bridge interfaces
  `br-<hash>` unless told otherwise — `config.BridgeNameFor` (sha1, ≤15
  chars) plus `com.docker.network.bridge.name` in both `NetworkCreate` call
  sites (`internal/orchestrator/orchestrator.go`) fixed the naming; a
  DOCKER-USER egress rule was added alongside the existing INPUT rule (INPUT
  alone doesn't cover routed/FORWARD traffic). v1.8.4 made it self-healing:
  `styrman-firewall.timer` re-runs the (delete-then-insert idempotent)
  script every 60s — `iptables-nft -C` was found unreliable against the
  `sdb-+` wildcard, so don't describe the script as using `-C || -I`.
- **Caddy picked a random upstream IP for the multi-homed server** — the
  styrman-server container sits on 3 networks (ingress + 2 project
  bridges); `extractContainerIP` (`deploy/caddy/ingress/watcher.go`) now
  deterministically prefers `styrman-ingress`, then non-`sdb-*`, then
  whatever's there (correct for a preview VM). Fixed v1.8.5, surfaced by the
  v1.8.3 network recreation reshuffling Go's random map iteration.
- **Terminal keyboard handling** (`web/src/lib/terminal.ts`) grew three
  browser-side interceptors across v1.8.3/v1.8.6–v1.8.9: Ctrl+C explicit
  `\x03` + preventDefault (copy-on-select was swallowing it), AltGr(Win/
  Linux)/Option(mac) symbol-compose detection, and Nordic dead-key
  (`~ \` ^ ´ ¨`) resolution bypassing xterm's buggy IME path. Documented in
  a new "Browser-side keyboard handling" section in `terminal-protocol.md`
  — the wire protocol itself didn't change, only what bytes the client
  decides to send.
- **File editor + server-side format** (v1.7.2/v1.7.3): editor is resizable,
  save no longer closes it, ~20 languages added. `POST /api/projects/{id}/
  format` runs the file's REAL CLI formatter (gofmt/prettier/black/rustfmt/
  shfmt/terraform fmt) inside the guest VM — only JSON still formats
  locally in the browser.
- **Settings consolidated** (v1.8.1/v1.8.2): the old separate "Tools &
  credentials" cards were removed; Credentials is now one status-colored
  list (green=synced, grey=not set up, red=signed in but not synced) with a
  click-to-read viewer (`GET /api/settings/credentials/{kind}/{id}/content`).
- **Test count discrepancy found, NOT resolved**: `guide.md`/README.md claim
  "35 self-hosted Playwright browser checks", but grepping
  `web/e2e/run.mjs` for `check(` calls gives 33–34 depending on the regex
  (imprecise — didn't do a careful manual count). Left the guide.md number
  alone rather than risk a wrong "fix"; a future session should actually
  read the file and count precisely before touching that claim.
  `integration.sh`'s "62 checks" (grep `^check `) WAS reverified accurate
  this session — up from 56 at the 2026-08-27 review, so it does grow;
  recount, don't trust memory.

Files touched this session: `api.md` (new "## Git (real client)" and
"## Format (server-side)" sections, `stats` row, rewrote "## Checkpoints" to
mark it backend-only, updated credentials import/setup-status rows, new
credential-content row), `deploy.md` (two new sections: network isolation +
Caddy IP selection; credential-broker paragraph reworded for auto-seed),
`runtime-image.md` (step 6/6c rewritten, two new sections: "First-run
onboarding" and "Credentials materialized per project" table),
`terminal-protocol.md` (new "Browser-side keyboard handling" section),
`guide.md` (load-bearing ideas #2/#3/#4/#5 rewritten, several Architecture
bullets, install/operating-it sections, two troubleshooting rows), root
`README.md` (Why bullets, Status paragraph, quick-start version stamp).

See [[feedback-styrman-doc-edits]] for edit-process lessons.
