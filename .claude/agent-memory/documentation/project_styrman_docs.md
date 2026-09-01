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

Files touched that session: `api.md` (new "## Git (real client)" and
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

**2026-08-30 session — v1.9.0/v1.9.1 security-review-remediation doc sync**
(read `.agents/docs/changes/2026-08-30-security-review-remediation-v1.9.0.md`
+ ADR-039/ADR-040 first — 62 findings, 5 commits, terse per-finding prose;
don't re-derive from git log). Key facts for next time:
- **Host-side git checkpoint machinery is DELETED, not just unused**
  (ADR-039, the C1 critical RCE fix): `internal/httpapi/checkpoints.go` and
  `gitrepo`'s snapshot/restore/commit-tree functions are GONE from the repo
  — this is a step change from the 2026-08-29 state where they were
  "backend-only, dormant." The standalone `GET /api/projects/{id}/diff`
  route is also gone. `internal/gitrepo/gitrepo.go` now only has
  `IsRepo`/`EnsureRepo` (`git rev-parse`/`git init`), hardened with `-c`
  overrides + `GIT_CONFIG_GLOBAL=/dev/null`. Removed the whole "##
  Checkpoints" section from `api.md` (don't re-add it) — folded a short
  removal note into "## Git (real client)" instead.
- **Broker keyring tier DELETED outright** (ADR-040, supersedes the
  2026-08-29 "dormant" state): no `credstore.KeyringCred`/`PutKeyring`, no
  `GET /keyring` on the broker, no `secret-tool store` step in
  `image/configs/styrman-credentials.sh`. `image/configs/profile-keyring.sh`
  no longer exists as a file. agy is a plain `file`-tier credential, full
  stop — say "deleted", not "dormant", from this point forward.
- **STYRMAN_KEYRING_PW is a live discrepancy, not yet cleaned up**:
  `internal/config/config.go` has NO field for it (confirmed by reading the
  whole file) — but `deploy/docker-compose.yml` still has
  `${STYRMAN_KEYRING_PW:?...}` on the server env, `deploy/.env.example`
  still lists it, and `deploy/host-setup.sh` still generates+writes it. This
  is real vestigial deploy-script state (not a doc error) — documented as
  such in `deploy.md`'s "Required secrets" section rather than silently
  dropped or silently kept as if load-bearing. Flagged for a deploy-script
  cleanup in the final report; did not touch the scripts themselves (out of
  a documentation subagent's lane).
- **Firewall reworked to dedicated chains** (H5, applied by the operator
  separately from the 5 remediation commits — needs root, live iptables):
  `STYRMAN-IN`/`STYRMAN-FWD` chains, INSERT-IF-MISSING (not
  delete-then-insert — that had a per-60s window with no isolation), a
  `physdev --physdev-is-bridged RETURN` exemption so a project can still
  reach the broker/proxy on its OWN bridge, plus IPv6
  `disable_ipv6`/mirrored `ip6tables` drops. Rewrote `deploy.md`'s whole
  network-isolation section and the matching `guide.md` Architecture bullet
  — the old "delete-then-insert self-heal" description is now wrong.
- **Test count correction, this time verified precisely**: `web/e2e/run.mjs`
  has exactly 34 `await check(...)` calls (counted every call site, not just
  a regex total) — the docs said 35 in `guide.md`, `AGENTS.md`, and
  `.agents/docs/testing/styrman/README.md` (twice); fixed all four spots.
  `integration.sh` is still 62 (`grep '^check '`), unchanged and reverified.
- **`.agents/docs/testing/styrman/integration.sh` (NOT edited — out of a
  documentation subagent's lane, it's a test script) still contains ~9
  `check "..."` calls against the deleted `/diff` and `/checkpoints[/restore]`
  endpoints — they will 404 at runtime now. Flagged prominently in
  `testing/styrman/README.md` and in the handoff report; belongs to whoever
  owns test-script maintenance (the `testing` subagent) to fix.
- **REQUIREMENTS.md R3 now contradicts the code**: it says "the earlier
  keyring tier is dormant" — should say "deleted" per ADR-040. Did not edit
  REQUIREMENTS.md (approval-gated); reported the exact line in the handoff.
- Frontend `web/src/lib/api.ts`'s `CredItem.kind` TS union still includes
  `"keyring"` even though the backend can never send it anymore — noted as a
  minor stale artifact in the report, not fixed (frontend code, not docs).
- `image/Dockerfile` still installs `dbus gnome-keyring libsecret-tools`
  (comment still attributes them to "agy's Secret Service") — these are now
  genuinely unused (no keyring daemon starts at login, `entrypoint.sh` has
  no keyring step). Documented as "installed, harmless, unused" in
  `runtime-image.md` rather than silently dropped from the "Contents" list.

Files touched this session: `api.md` (Git section rewritten with the RCE-fix
paragraph, Format section stdin/413 fix, History section — dropped `/diff`
row, deleted the whole Checkpoints section, credentials endpoints —
`keyring` stripped from every kind union, setup/status row, server-env table
— dropped `STYRMAN_KEYRING_PW`, added `STYRMAN_VCPUS`/`STYRMAN_MEMORY_MB`,
`STYRMAN_INGRESS_TOKEN`/`STYRMAN_AUTH` rows note fail-closed startup),
`deploy.md` (auth fail-closed paragraph, Required-secrets list + the
STYRMAN_KEYRING_PW vestige note, new VCPUS/MEMORY_MB note, new "What
host-setup.sh pins/verifies" section, ingress composition bullet — sanitize
+ picture_extractor/caddy-l4 removal, full network-isolation section
rewrite), `runtime-image.md` (Contents — keyring packages marked unused,
OpenTofu verification note, entrypoint env table — dropped
STYRMAN_KEYRING_PW, steps 5/6/6b/6c rewritten, Credentials table — agy row
kept + keyring row replaced with a removal paragraph, Invariants — dropped
the STYRMAN_KEYRING_PW bullet, credential-materialization env applies now),
`guide.md` (§1 idea 5 git-substrate rewritten, §2 Architecture — isolation
bullet rewritten for chains/physdev/IPv6, credential-broker bullet — keyring
now "deleted" not "dormant", new "Host-side git is bootstrap-only" bullet,
new "Guest exec output is bounded" bullet, auth bullet gets a fail-closed
sentence, §3 install gets a host-setup.sh pins pointer, §6 test count
35→34), `terminal-protocol.md` (new OSC 52 write-only paragraph after the
Frames section, H8), root `README.md` (Status paragraph rewritten for
v1.9.0/v1.9.1, isolation Why-bullet reworded for chains, version stamps
v1.8.9→v1.9.1 in two places), `AGENTS.md` (test count 35→34 in two spots,
Key Files table — dropped the false "git diff" attribution to
`internal/history`, added an `internal/gitrepo` row), `.agents/docs/testing/
styrman/README.md` (test count 35→34 in two spots, tier-2 description drops
the false "git diff + checkpoints" claim and gains a "Known gap" callout
about the stale checks). REQUIREMENTS.md and `.agents/docs/changes/`/
DECISIONS.md/TODO.md were NOT touched (approval-gated / another subagent's
lane) — see the handoff report for the exact REQUIREMENTS.md R3 wording that
now contradicts the code.

**2026-08-30 session — v1.10.0 setup-terminal robustness + post-setup
credential management** (task-provided facts, already verified against
`image/configs/styrman-setup`, `internal/terminal/bridge.go`,
`internal/httpapi/setup.go`/`server.go`, `web/src/routes/settings.tsx`/
`setup.tsx` before writing). Key facts for next time:
- `styrman-setup` traps SIGINT (`trap 'printf "\n"; exit 0' INT`) so Ctrl+C
  is a clean `exit 0`, not a signal-kill. Previously the reconnecting
  browser terminal's `tmux new-session -A` picked the signal-killed session
  back up and RESTARTED the script from the top; now the pane just ends →
  close 4000, same as a normal finish.
- `styrman-setup <tool>...` (git|ssh|gh|gcloud|claude|agy) runs ONLY that
  step, ensuring git identity quietly first if the target isn't git/ssh.
  Driven by `GET /ws/setup?tool=<tool>` (`internal/terminal/bridge.go`'s
  `ServeSetup`/`setupTool` — strictly allowlisted, interpolated into a shell
  command, unknown values fall back to the full flow), own tmux session
  `styrman-setup-<tool>`.
- New `POST /api/setup/harvest` (`internal/httpapi/setup.go`'s
  `harvestSetup`/`seedBrokerFromSetup`): harvests the running setup VM into
  the broker WITHOUT the done-marker or teardown — `/done` still does both
  plus destroy. This is what Settings → Credentials' grey/red-row
  "sign in"/"re-sync" click opens (`ManageCredsModal` in `settings.tsx`):
  boots the VM once, runs `styrman-setup <tool>` embedded, harvests per
  step, VM stays up for a picker to add more, "Done"/click-outside tears it
  down.
- First-run `/setup` route's `onExited` is now status-aware
  (`web/src/routes/setup.tsx`): finalizes (`POST /api/setup/done`) only if
  `gitIdentity && sshKey` are already true; otherwise discards
  (`DELETE /api/setup`) and leaves the banner — never marks an incomplete
  setup done. This logic is NOT yet documented anywhere in the four spec
  files (out of this task's stated scope — task said touch only the
  Credentials/settings bullet in guide.md, not §3 Install-from-scratch) —
  flag for a future session if `guide.md`'s §3 setup-closing paragraph is
  ever revisited, since it still only describes "typing exit or the script
  finishing," not the Ctrl+C-before-required-creds discard path.

Files touched this session: `runtime-image.md` ("First-run onboarding"
section — two invocation-mode bullets, Ctrl+C paragraph, "Finishing setup"
paragraph split into /done vs /harvest), `api.md` (Terminals section's
`GET /ws/setup` line gained `?tool=`, Setup section intro + new
`POST /api/setup/harvest` row), `terminal-protocol.md` (new
"## Setup terminal (`/ws/setup`)" section, placed between "Browser-side
keyboard handling" and "Close codes" — this doc previously never mentioned
`/ws/setup` at all, only api.md did), `guide.md` (§4 Settings/Credentials
bullet — grey/red rows now clickable, describes the add/re-sync modal and
its Ctrl+C-cancels-current-step behavior). REQUIREMENTS.md, changes/,
DECISIONS.md, TODO.md were NOT touched (approval-gated / changelog
subagent's lane).

**2026-08-30 session — adversarial-audit doc corrections (v1.9.1→v1.13.1
catch-up + 6 stale-fact fixes).** A separate audit pass (not the security
review) found the durable docs had drifted from code in specific, narrow
ways even though `api.md`/`guide.md` already documented v1.10–v1.13 features
in depth — only root `README.md` and `.agents/docs/testing/styrman/README.md`
were stuck citing v1.9.1/56 checks; fixed both (v1.13.1, 60 integration
checks, new Status paragraphs for v1.10/ADR-041, v1.11/ADR-042, v1.12/
ADR-043, v1.13/ADR-044). Six more corrections, each verified against code
before editing (grep, not memory):
- `deploy.md`'s `STYRMAN_KEYRING_PW` paragraph was itself stale — a PRIOR
  session (2026-08-30 security remediation) had documented it as a live
  vestige still required by compose/host-setup.sh; by this session compose,
  `.env.example`, and `host-setup.sh` had actually been cleaned up (zero
  `KEYRING` references — confirmed by grep, only a stray line in the
  gitignored live `deploy/.env` remains). **Lesson: a "vestige pending
  cleanup" note has a shelf life — re-verify it wasn't quietly fixed before
  repeating the claim.**
- `runtime-image.md`'s dbus/gnome-keyring/libsecret-tools bullet said
  "still installed (harmless)" — wrong, `image/Dockerfile` had dropped them
  entirely (its own comment says so). Doc now says "removed... with
  ADR-040", kept as a historical note under Contents rather than deleted
  outright (matches how other removed-feature notes read elsewhere in this
  doc set).
- `api.md`: `GET /api/settings/credentials` response shape gained
  `brokerEnabled:bool` (false+empty items when the broker is disabled —
  not an error, `internal/httpapi/credentials.go`'s `credentialsList`);
  history search softened from "ALL... transcripts" to the real
  `searchMaxFiles=50` newest-first bound (`internal/history/history.go`);
  stopped-project `stats` corrected from "`{state}` only" to "full object,
  State set, every numeric metric zeroed (no `omitempty` on those fields)"
  — verified by reading `projectStatsView`'s struct tags directly.
- `terminal-protocol.md` + `guide.md`: Ctrl+Shift+C copies a selection too,
  not just Cmd+C (`web/src/lib/terminal.ts`'s keydown handler checks
  `(ev.ctrlKey && ev.shiftKey) || (ev.metaKey && !ev.ctrlKey)`) — fixed in
  both the close bullet AND the earlier Frames-section mention in
  terminal-protocol.md (same stale claim appeared twice in that file).
- `TODO.md`: the "Checkpoints incl. ~/.claude" post-v1.3 roadmap item
  closed as substantially shipped by v1.13.0 backups (ADR-044, whole
  per-project home archived); the "Idle-reaper auto-snapshot" closed item's
  rationale (which cited the REMOVED v1.3 checkpoint loop — a stale
  self-reference, since that loop was deleted in v1.7.0 per the earlier
  git-client sync) repointed at ADR-042 (busy-guarded, non-destructive idle
  stop) + ADR-044 (manual backups) instead.
- `guide.md`'s Per-project backups bullet: backup archives do NOT contain
  project-scoped env vars — `seedFromBackup`
  (`internal/httpapi/backups.go`) copies them live from the SOURCE
  project's broker-side credstore at creation time
  (`s.creds.ProjectEnv(user, srcProjectID)`), and deleting a project wipes
  its scoped env set (`DeleteProjectAll`, called from the project-delete
  handler) — so a deleted project's backup restores workspace+home with
  zero scoped vars. This is a real behavioral gotcha (silent env-var loss
  on restore from a deleted project), worth remembering if it resurfaces.
- `deploy/.env.example` gained commented `STYRMAN_BROKER_ADDR` (matches
  compose's `${STYRMAN_BROKER_ADDR:-:8091}`) and a note that
  `STYRMAN_VERSION` is a compose command-line arg, not a `.env` var — the
  task's suggested default value ("0.0.0.0") was wrong, compose's actual
  fallback is `${STYRMAN_VERSION:-0.0.0}`; verified against
  `deploy/docker-compose.yml` before writing the comment rather than
  copying the task's number verbatim.

**Process note**: `TODO.md` is normally the `changelog` subagent's lane, not
documentation's — touched here only because the task explicitly named exact
`TODO.md` line ranges with pre-verified replacement content. Flag this kind
of explicit, narrow instruction as the exception, not a precedent for
reaching into TODO.md unprompted.

**2026-08-30 session — v1.14.0/v1.14.1 fresh-reader fixes to `deploy.md` +
`guide.md` only** (task pre-verified 16 findings against code before
handing them off; still re-verified every claim against source before
writing — grep/read, not trust). Key facts for next time:
- **Host-layer disaster-recovery backups are now real** (R11, v1.14.0/
  v1.14.1): `deploy/backup/styrman-host-backup` + `styrman-backup.
  {service,timer}` (committed in the repo, installed by `host-setup.sh`'s
  step 8b) push a daily whole-state restic snapshot of `/var/lib/styrman`
  to `gs://styrman-crew-backups-driven-eft-h4it` (GCP project
  `driven-eft-h4it`, europe-north2/Stockholm, Nearline). `deploy.md` gained
  a full "Restore on a new host (disaster recovery)" runbook (6 numbered
  steps) plus a "Provisioning order" paragraph and explicit copy-paste env
  blocks — this was previously just a hand-wave ("restic init (env in the
  script)") with a grep-scraping one-liner to list snapshots.
- **Restic excludes changed**: `StateDir/backups` (the in-app per-project
  archives, ADR-044) is now INCLUDED in the host snapshot — a 2026-08-30
  reader review found excluding it broke "backups outlive their project"
  for a DELETED project (its in-app archive is the only surviving copy).
  Only `homes/*/p/*/.dockerd` (reproducible docker disks) stays excluded.
  Verify this against `deploy/backup/styrman-host-backup`'s own comment
  before repeating either the include or exclude claim — it changes.
- **`STYRMAN_PREVIEW_DOMAIN` never existed as a real env var** — that was
  always two separate vars, `STYRMAN_PUBLIC_DOMAIN` (ADR-025, public TTL
  links) and `STYRMAN_PRIVATE_DOMAIN` (ADR-026, email-gated links). One
  stale mention of the old single-var name survived in deploy.md's
  Bring-up `.env` fill-in comment years after the split — fixed. Grep the
  actual code (`deploy/.env.example`, `docker-compose.yml`) rather than
  trusting an inline comment if this resurfaces.
- **`guide.md`'s §7 Roadmap is now a pointer, not a list** (ADR-045,
  operator directive 2026-08-30: "I want nothing in todo and roadmap").
  Don't add a roadmap item list back to guide.md — TODO.md's Inbox is the
  only place open items belong now; guide.md just cites ADR-045 +
  `.agents/docs/changes/`.
- **State-root files** (top-level under `/var/lib/styrman`, sibling to the
  `{homes,projects,users,credentials,backups}` dirs): `users.json`
  (runtime-added accounts), `links.json` (share-link tokens, `internal/
  links`), `settings.json` (admin runtime tunables, `internal/config`).
  All three must be wiped for a truly virgin reinstall — the existing
  clean-reinstall doc/section only wiped the dirs, not these files quoted;
  fixed in `deploy.md`'s Clean reinstall section and added rows for
  `backups/`, `links.json`, `settings.json` to `guide.md`'s "Where things
  live" table (only `users.json` had a row before).
- **Integration check count re-verified at 60** (`grep -c 'check "'
  .agents/docs/testing/styrman/integration.sh` = 60; 3 `skip(...)` call
  sites for firewall-hardened hosts) — `.agents/docs/testing/styrman/
  README.md` already said this, but `guide.md`'s §6 dev-loop comment was
  still stuck at the old "56 checks" from the 2026-08-27 security review
  era. This is the same "recount, don't trust a remembered number" trap
  flagged in the 2026-08-30 v1.9.0 sync entry above — it keeps resurfacing
  because different docs get touched in different sessions.
- **`host-setup.sh` does a lot more than "Docker, Kata, dirs, seeds .env"**:
  it also installs the isolation firewall script + timer, installs the
  backup script + systemd units (8b), and BUILDS `styrman-runtime:latest`
  by default (`RUNTIME_IMAGE="${STYRMAN_IMAGE:-styrman-runtime:latest}"`) —
  the tag production `docker compose` uses. `guide.md`'s §3 install
  one-liner annotation for that command was years stale; expanded it and
  added the dev-path prereqs it does NOT cover (Go 1.25+ toolchain; Node
  22+ only to rebuild `web/`, `web/dist` ships committed so neither runtime
  path needs it). `README.md`'s Quick start section (top-level, not this
  subagent's usual file, but read for reference) had ALREADY been kept
  current with these exact facts including the `STYRMAN_VERSION=$(git
  describe --tags --always)` prefix convention — when guide.md's
  bring-up/install commands drift from README's, README is usually the
  more recently touched one; diff against it before re-deriving from
  scratch.

Files touched this session: `deploy.md` (Bring-up section — OAuth/
Cloudflare prereqs moved before the code block, `STYRMAN_PREVIEW_DOMAIN` →
`STYRMAN_PUBLIC_DOMAIN`/`STYRMAN_PRIVATE_DOMAIN`; Host-layer backups
section — excludes rewritten, new "Provisioning order" paragraph + env
block, new "### Restore on a new host" subsection; Clean reinstall section
— wipe list gained the three state-root files, final command fully
qualified with `-f deploy/docker-compose.yml`), `guide.md` (§1 idea 5
backup sentence, §4 git bullet backup pointer, §7 Roadmap fully rewritten,
§3 install code block + prose expanded, DR line in §4 rewritten to point
at the new runbook, "Where things live" table gained 3 rows, §6 check
count 56→60). Neither file's OTHER content was touched (task scoped to
exactly these two files) — `api.md`, `runtime-image.md`,
`terminal-protocol.md`, root `README.md`, `AGENTS.md`, REQUIREMENTS.md,
TODO.md, DECISIONS.md were read for verification only, not edited.

**2026-09-01 session — v1.15.0 vendored-harness re-sync doc sync.** The
agent harness at `image/harness/` (deployed into every project's guest
home, not styrman's own repo tooling) was re-synced verbatim from its
upstream `devshell_prompt/harness` (commit `6132b97`) and the runtime image
rebuilt at `STYRMAN_VERSION=1.15.0`. Verified against
`image/harness/README.md`, `image/harness/claude/setup.sh`,
`image/entrypoint.sh:110-135`, and `image/Dockerfile:62-75` before writing.
Key facts for next time:
- Harness now ships **6 subagents** (`changelog`, `documentation`,
  `reviewer`, `verifier`, `security`, `explorer`) — up from 4
  (`testing` retired/replaced by `verifier`, which FAILs on changed code
  with no covering tests). `shared/AGENTS.md` grew from 62 to 69 lines
  ("Global Development Standards" → "Global Agent Constitution"), adding a
  Resume Protocol + Definition-of-Done dispatch table. Skills 23 → 28.
- **Harness now deploys to THREE locations per project home**, not two:
  `~/.claude`, `~/.gemini`, AND the new vendor-neutral `~/.agents/bin/`
  (`resume-context.sh`/`trail-check.sh` shell gates — Claude `SessionStart`/
  `Stop` hooks; explicit skill-body steps on Antigravity). Source:
  `image/harness/claude/setup.sh` step 8, confirmed via
  `image/harness/README.md`'s deploy table.
- Delivery mechanism into the runtime image itself is UNCHANGED:
  `image/Dockerfile:71` `COPY harness/ /styrman/harness/`,
  `STYRMAN_VERSION` stamps `/styrman/VERSION`, `entrypoint.sh:110-135`
  write-once-seeds per project home keyed on that version vs
  `~/.styrman/.harness-version`. Re-running setup on a version bump is what
  RE-SEEDS and RETIRES dropped content (e.g. the old `testing.md` agent
  file — `claude/setup.sh`'s `RETIRED_AGENTS` array backs it up then
  removes it) in already-seeded homes.
- This is styrman's OWN `AGENTS.md` (project root) — its Testing/Architecture
  sections describe styrman-crew the app, NOT the vendored guest-side
  harness, and it never names subagents by roster — checked (grep) and
  found nothing stale to fix this session, unlike the runtime-image.md
  spec which explicitly documents the harness's own contents.
- Root `ROADMAP.md` did not exist yet (only the harness's own
  `image/harness/shared/templates/ROADMAP.md` scaffold existed) — created
  it this session, Now/Next/Later, mirroring `TODO.md`'s empty-slate
  framing from the 2026-08-30 operator directive (ADR-045). Next/Later left
  as commented-out placeholders, no invented items.

Files touched this session: `image/README.md` (harness parenthetical +
STYRMAN_VERSION paragraph — third `~/.agents/bin/` location),
`.agents/docs/specs/styrman/runtime-image.md` (step 6 — full harness
contents/deploy-locations/version-bump-retires paragraph), root
`ROADMAP.md` (created). `AGENTS.md` was read and verified but NOT edited —
no stale subagent/harness reference found. `.agents/docs/changes/`,
DECISIONS.md, TODO.md, REQUIREMENTS.md were NOT touched (changelog
subagent's lane / approval-gated).

**2026-09-01 session — review-driven doc sync (editor pane, e2e web build,
e2e counts, hadolint/shellcheck toolchain).** Dispatched off
`.agents/docs/reviews/2026-09-01-review-editor-pane-and-ordering.md`'s
low-severity docs-drift findings, on code already in the working tree
(uncommitted at dispatch time). Key facts for next time:
- **The centered modal file viewer is GONE** (`FileEditor.tsx`'s old modal
  chrome deleted 2026-09-01): the editor is now a VIEW of the terminal's
  MAIN pane (`web/src/components/EditorPane.tsx` + `web/src/lib/
  editorPane.ts`'s per-`${project}/${term}` module store), toggled by a
  `Terminal | Editor` control in the terminal header
  (`web/src/routes/p.$project.$term.tsx`). Both the terminal and the editor
  stay mounted (`hidden`, never unmounted) so switching never drops the
  terminal session or the open file. `guide.md`'s load-bearing idea #4 was
  rewritten for this — don't describe a "resizable editor window" anymore,
  that was the deleted modal's property.
- **e2e count is 45 total** (43 passed + 2 auto-SKIP on this
  firewall-hardened host), up from 36 (34+2) — the 9 new checks are the
  editor-pane suite (open-in-pane not modal, content, toggle back, terminal
  survives the round-trip, reopen same file, word-wrap toggle, Dockerfile
  highlighting, delete-while-open). Fixed in `AGENTS.md` and `.agents/docs/
  testing/styrman/README.md`'s Tier 4 header + body — same "recount, don't
  trust a remembered number" trap as every prior session (see the 2026-08-27
  and 2026-08-30 entries above); it keeps resurfacing because e2e checks get
  added in feature PRs that don't always touch the testing README themselves.
- **`web/e2e/run.sh` now builds the web SPA before the Go server** (already
  in the working tree, well-commented in the script itself) — the server
  `go:embed`s `web/dist/client`, so building only the binary silently tested
  stale assets (a real false failure 2026-09-01). `STYRMAN_E2E_SKIP_WEB=1`
  opts out for server-only iteration; a run leaves `web/dist` dirty
  (fresh content hashes) — stage with `git add -A -- web/dist`. Documented in
  both `AGENTS.md`'s testing bullet and the testing README's Tier 4 section.
- **`image/Dockerfile` gained `shellcheck` (apt) + `hadolint` (GitHub release
  binary, same pattern as `yq`)**, backing `internal/httpapi/check.go`'s
  per-language syntax checker (mirrors `format.go`'s dispatch-by-extension
  shape). Documented in `image/README.md` (one paragraph) and
  `runtime-image.md`'s Contents list (full checker table by extension).
- **IMPORTANT unresolved finding, flagged not fixed (out of documentation's
  lane — this is a code change):** `internal/httpapi/check.go`'s
  `checkFile` handler is written and unit-tested (`check_test.go`) but is
  **never registered** in `server.go`'s mux — grepped every
  `mux.HandleFunc` call site and `/check` isn't among them (unlike
  `/format`, registered right next to where `/check` would go, line ~271).
  So `POST /api/projects/{project}/check` is currently unreachable, and
  nothing in `web/src` calls it either (`api.ts`, `CodeEditor.tsx` have no
  check/lint wiring) — this is backend-complete, unwired dead code, not yet
  a live feature. Documented the toolchain as designed/intended per the
  dispatching task's explicit instruction, but this needs a code fix
  (`mux.HandleFunc("POST /api/projects/{project}/check", s.checkFile)`) and
  frontend wiring before it's actually live — re-verify this is still true
  before describing the Check feature as working in any future doc pass.

Files touched this session: `AGENTS.md` (e2e testing bullet — SPA build +
skip var + count), `.agents/docs/testing/styrman/README.md` (Tier 4 header +
body — count, SPA-build paragraph, editor-pane check list), `guide.md`
(load-bearing idea #4 — editor-pane rewrite, no lint/check-gutter language
added given the unwired-backend finding above), `image/README.md` (new
sentence on shellcheck/hadolint), `runtime-image.md` (Contents — new
shellcheck/hadolint bullet with full checker table). `README.md` (root) has
no editor/modal prose to fix — checked (grep), nothing there. `api.md` was
NOT touched — the `/check` route isn't live yet (see finding above), so
documenting it as a real API surface would itself be inaccurate; revisit
once the route is registered and the frontend calls it.

**2026-09-01 session (second half of the day's cycle) — e2e recount to
49/47+2, auto-updater docs (ADR-050/051), `/check` now live in api.md.**
Third doc-sync pass this same calendar day (after the harness re-sync and
the editor-pane/e2e-web-build pass, both above) — three unrelated features
landed in `.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md`.
Key facts for next time:
- **e2e count is 49 total now** (47 passed + 2 auto-SKIP on this
  firewall-hardened host), up from 45 (43+2) earlier today — 4 more checks
  landed in the SAME session that added the auto-updater: discard-and-open
  loading the second file, no temp-file litter from check/format, syntax
  check flagging a broken shell script, syntax check reporting a clean file
  clean. Fixed in `AGENTS.md` and `.agents/docs/testing/styrman/README.md`'s
  Tier 4 header + body. Static grep of `web/e2e/run.mjs` for `check\(` +
  `skip\(` call sites only totals 48, one short of the reported 49 — the
  task supplied the measured number directly (as a verifier run would) and
  it was trusted over the static recount per instruction; flag this
  discrepancy if a future session needs to reconcile it exactly (don't
  assume 48 is right just because grep says so, and don't assume 49 is
  right either — actually run the suite once to settle it).
- **Auto-updater (ADR-050/051, `deploy/update/styrman-update` +
  `.service`/`.timer`, installed by `host-setup.sh` step 8c)**: root
  systemd timer, every 5 min, deploys the newest tag matching
  `^v[0-9]+\.[0-9]+\.[0-9]+$` (pre-releases excluded by the regex, highest
  by `sort -V` not newest by date). Runs against a DEDICATED clone at
  `/opt/styrman`, never the operator's working tree. **Requires signed
  tags** (`git tag -s`) — rejects lightweight tags and runs
  `git verify-tag`, fails closed; opt-out only via root-owned
  `/etc/styrman/update.env` (`STYRMAN_UPDATE_REQUIRE_SIGNED=0`).
  Consequence: existing v1.14.0–v1.15.0 tags are unsigned/lightweight and
  would be refused as-is. Verifies a deploy by reading `LABEL
  styrman.version` off the running container via `docker inspect` (+
  `.State.Running`/health) — NOT HTTP, since production's
  `STYRMAN_AUTH=header` would 403 an unauthenticated poll (ADR-050's
  same-day correction). Deleting that LABEL from `deploy/Dockerfile.server`
  breaks the updater (rolls back every release — load-bearing cross-file
  contract, held together only by comments in both files). Rollback on
  failed verification redeploys the previous tag and latches the failed one
  at `/var/lib/styrman/update/failed-version`; `--force` clears it.
  `--check` is dry-run/report-only. Ships DISABLED — needs a read-only
  deploy key at `/etc/styrman/deploy-key`, a pinned
  `/etc/styrman/known_hosts` (`ssh-keyscan`), the clone, and the clone's own
  `deploy/.env`; `host-setup.sh` step 8c prints the exact commands — kept
  the new deploy.md section's setup steps consistent with what that script
  actually prints rather than inventing a second version. New "## Auto-
  update (tag-gated, ADR-050/051)" section added to `deploy.md`, placed
  after the Host-layer-backups restore runbook and before Clean reinstall
  (same shape as the backups section: what/where/how bullets, then a
  numbered setup list). One-sentence summary added to root `README.md`'s
  Status bullet list. `guide.md`'s §3 install one-liner comment (which
  enumerated what `host-setup.sh` installs) also got a short addition
  mentioning the auto-update timer, kept minimal since guide.md wasn't in
  the task's explicit scope.
- **`/check` (syntax checking) is NOW live** — `mux.HandleFunc("POST
  /api/projects/{project}/check", ...)` is registered (`server.go:273`) and
  the frontend calls it (`web/src/components/FileEditor.tsx`'s `runCheck`,
  CodeMirror linter on idle) — this REVERSES the 2026-09-01-earlier-session
  finding that it was backend-complete but unwired/dead. Added a new "##
  Check (server-side, syntax)" section to `api.md` right after "## Format
  (server-side)": request `{path,content}`, response
  `{supported,tool?,unavailable?,diagnostics:[{line,col?,severity,message}]}`,
  `supported:false` is HTTP 200 not an error, 409 stopped VM, 413 same cap
  as `/format` (`maxCheckBytes = maxFormatBytes` in `internal/httpapi/
  check.go`). Checker-by-extension table: gofmt/shellcheck-or-bash-n/
  py_compile/node --check/jq empty/yq/tofu fmt -check/hadolint(by
  basename). Before touching this again, re-verify the mux registration and
  frontend call site still exist — this is exactly the kind of "wired vs.
  not" fact that flips between sessions on a fast-moving feature.

Files touched this session: `AGENTS.md` (e2e count 43→47), `.agents/docs/
testing/styrman/README.md` (Tier 4 header 45→49/43→47 + body — 4 new
checks named), `.agents/docs/specs/styrman/deploy.md` (new "## Auto-update"
section), root `README.md` (new Status bullet), `.agents/docs/specs/
styrman/guide.md` (§3 install comment — auto-update timer mention),
`.agents/docs/specs/styrman/api.md` (new "## Check" section). No
changelogs/ADRs/TODO.md touched (out of lane; changelog subagent's territory
— the source changelog entry and ADR-050/051 already existed before this
doc pass, read but not edited).

**2026-09-01 session (third pass, same day) — preflight security fix to
`deploy.md`'s Auto-update section (ssh-keyscan removal, SSH-signing
prerequisites).** A preflight security re-check found the durable spec still
taught REMOVED behavior in two spots. Verified against
`deploy/update/styrman-update` and `deploy/host-setup.sh` step 8c's heredoc
(the authoritative source of the exact operator-facing commands) before
writing. Key facts for next time:
- **`ssh-keyscan` seeding of `/etc/styrman/known_hosts` was deliberately
  REMOVED** (unauthenticated TOFU pinning a trust anchor for a root process
  that builds+runs what it fetches — flagged twice in security review per
  the script's own comment). `host-setup.sh` no longer auto-clones over SSH
  at all; it prints instructions for the operator to seed `known_hosts` from
  GitHub's **published** fingerprints (`curl .../meta | jq ... | tee
  known_hosts`, cross-check against GitHub's docs page) and clone BY HAND
  with `StrictHostKeyChecking=yes`, then `chown -R root:root` the clone.
  Don't reintroduce `ssh-keyscan` language if this resurfaces.
- **Tag verification requires SSH signing + an allowlist, not a bare `git
  verify-tag`.** `git verify-tag` alone exits 0 for a good-but-*untrusted*
  signature (any key ever imported into root's keyring passes with GPG);
  only the SSH-signature path consults an allowlist. `styrman-update`'s
  `verify_tag()` REJECTS any tag object without a `-----BEGIN SSH
  SIGNATURE-----` block — **a PGP-signed tag is refused outright, not merely
  unpinned** — then requires a root-owned, non-group/world-writable
  `/etc/styrman/allowed_signers` read from that FIXED path (never root's
  gitconfig — `styrman-update.service` sets `ProtectHome=yes` and no
  `$HOME`, so `git config --global` reads empty under the timer even though
  it works fine run by hand). Sign with `git config gpg.format ssh &&
  git config user.signingkey ~/.ssh/id_ed25519.pub && git tag -s vX.Y.Z`.
- **Rollback mechanics — a genuine code/comment tension found by reading the
  script closely, documented as observed rather than as the task described
  it.** `styrman-update` writes `/var/lib/styrman/update/deployed-sha` on
  every successful deploy (`git rev-parse <tag>^{commit}`), and the script's
  own comments say this makes rollback "immune to a re-pointed tag and needs
  no re-verification." But `STATE_SHA`/`deployed-sha` is **written and never
  read** — the actual rollback path (`deploy "${PREVIOUS}"`, `PREVIOUS` is
  always a tag string, never a SHA) calls `verify_tag()` on the previous TAG
  again, same as a forward deploy. Net effect: rolling back onto any tag
  made before SSH signing was adopted (v1.14.0–v1.15.0) is refused for the
  same reason a forward deploy of one would be. Documented `deploy.md`'s
  Rollback bullet to match what the code actually does (records the SHA for
  the record, but the automated path still re-verifies the previous tag by
  name) rather than the aspirational "rolls back to the SHA, no
  re-verification" framing the dispatching task described — flag this to
  whoever owns `styrman-update` next, since it may be an unfinished
  refactor (the SHA plumbing exists but nothing consumes it) rather than
  intended behavior.
- **Operational caveat, stated twice on purpose**: the timer must not be
  enabled before `allowed_signers` exists and releases are SSH-signed — the
  gate fails closed and would refuse every tag. Added this both to the
  section's intro paragraph and right after the numbered prerequisites list
  (the task asked for it "wherever enabling the timer is described").

Files touched this session: `.agents/docs/specs/styrman/deploy.md` only
(Auto-update section: intro paragraph, "Tags must be SSH-signed" bullet,
Rollback bullet, "Setup — three prerequisites" list rewritten with the
exact `host-setup.sh` step-8c commands). No changelogs/ADRs/TODO.md touched
(out of lane per this task's explicit instruction — this was a docs-only
preflight fix, not a code change).

**2026-09-01 session (fourth pass, same day) — whole-project `/audit` doc
fixes** (`.agents/docs/reviews/2026-09-01-audit-whole-project.md` +
`2026-09-01-audit-reviewer.md`). Four narrow, task-specified fixes, each
verified against code before writing:
- **`deploy/README.md` had no `update/` bullet** even though `backup/` had
  one — added, matching the `backup/` bullet's shape (what it is, DISABLED
  by default + why, runbook pointer to `deploy.md` → Auto-update).
- **`Project.GitInit` (`internal/project/store.go`) is a dead field** —
  hard-coded `true`, no reader anywhere (R13 made every project
  unconditionally git-init'd). Did NOT delete it from `api.md`'s
  `GET /api/projects` row (the field still round-trips in the JSON — code
  wasn't touched, only docs), instead annotated it as deprecated/dead-weight
  in place. `Project.Agent` and `LastActivity` were checked too (grep) and
  don't appear anywhere in `api.md` — no action needed there.
- **Four `api.md` description-level mismatches from the reviewer's
  API-spec-conformance table**, fixed in place: (1) §Notifications
  undersold the fan-out — code does one goroutine + `ContainerInspect` per
  project RUNNING OR STOPPED (not just running), uncapped, then one exec per
  running project; corrected to describe this accurately as a finding, not
  as acceptable. (2) The credentials-content-viewer row wrongly called it an
  "admin-only Settings surface" — it's `Authorized`-gated (owner reads their
  OWN secrets), not admin-gated; reworded. (3) `GET /api/settings`'s row was
  missing `terminalUsernameChosen` and `setupComplete` (both present in
  `explorer.go`'s response map) — added. (4) The credential broker
  (`internal/credbroker`, a SEPARATE listener on `STYRMAN_BROKER_ADDR`) had
  no route-level documentation anywhere in `api.md`, only the env var — added
  a new "## Credential broker (separate listener)" section (GET
  /git,/env,/files,/healthz with wire shapes) between Setup and Misc,
  pointing to `guide.md` for the prose tier overview.
- **Testing README + `integration.sh` prereq rebuilt `styrman-runtime:dev`**
  — the EXACT tag `deploy/.env`'s `STYRMAN_IMAGE` boots in production on
  this host, so following the documented test loop re-stamps prod (it
  happened for real during this audit). Changed the default/documented tag
  to `styrman-runtime:test` in BOTH `.agents/docs/testing/styrman/README.md`
  (Prereqs bullet, gained an explicit "do NOT build :dev" warning + mentions
  the new immutable `styrman-runtime:1.16.0` release tag) AND
  `integration.sh` itself (its `IMAGE="${1:-styrman-runtime:dev}"` default →
  `:test`, plus a header-comment warning) — `integration.sh` lives inside
  `.agents/docs/testing/styrman/`, so it's treated as part of the guide, not
  application source, and is in this subagent's lane to edit (unlike
  `web/e2e/run.sh`, left alone deliberately — see below).
- **Flagged, not fixed (out of lane / not asked)**: `web/e2e/run.sh` has the
  IDENTICAL `IMAGE="${1:-styrman-runtime:dev}"` default — it doesn't rebuild
  the image (so it can't re-stamp `:dev`), but every e2e run still silently
  boots real project microVMs against production's tag by default. It lives
  under `web/e2e/` (source tree), not `.agents/docs/testing/`, so left
  untouched and flagged in the final report rather than edited unprompted.
  The reviewer's other 46 findings (unbounded log buffer H1, Caddy admin API
  H1, credential-sweep symlink, `usernameFromEmail` false-cycle claim, seven
  duplicated atomic-write blocks, unpinned Caddy/base images, `settings.tsx`
  god-file, zero frontend unit tests, etc.) are CODE fixes, not doc drift —
  intentionally left alone; they belong to whoever picks up the audit's
  "Recommended order" list, not this doc pass.

Files touched this session: `deploy/README.md` (new `update/` bullet),
`.agents/docs/specs/styrman/api.md` (GitInit deprecation note, Notifications
fan-out correction, credentials-content-viewer wording, `GET /api/settings`
row fields, new "## Credential broker" section), `.agents/docs/testing/
styrman/README.md` (Prereqs bullet — `:test` tag + `:dev`-is-prod warning +
`1.16.0` mention, Tier-2 default mention), `.agents/docs/testing/styrman/
integration.sh` (default IMAGE `:dev`→`:test` + header warning). No
changelogs/ADRs/TODO.md touched (changelog subagent's lane, running in
parallel per the dispatching task). REQUIREMENTS.md not touched.

**2026-09-01 session (fifth pass, same day) — new `versioning.md` guide +
no-mutable-tags doc sync (v1.16.1 prep, ADR-052 follow-up, post-reboot
redfix).** Operator asked verbatim for a guide covering the three images,
versioning scheme, and auto-update; dispatched on top of an uncommitted
working tree that closed the redfix review's open findings, hardened the
watcher, finished ADR-052's state-dir enforcement, and introduced a NEW
standing operator rule mid-session: **no mutable image tags** (`:latest`/
`:dev`/floating) may reach a compose `image:` line or production
`STYRMAN_IMAGE` — verified against `internal/config/config.go`,
`deploy/docker-compose.yml`, `deploy/update/styrman-update`,
`deploy/update/styrman-update.service`, `deploy/caddy/Dockerfile`,
`deploy/Dockerfile.server`, and `deploy/host-setup.sh` before writing (not
trusted from the task's prose). Key facts for next time:
- **New file**: `.agents/docs/specs/styrman/versioning.md` — the three
  images/Dockerfiles table, the versioning scheme (one signed tag
  `vX.Y.Z` → `STYRMAN_VERSION` → all three image tags → the
  `styrman.version` LABEL the auto-updater reads), the no-mutable-tags rule
  with its two 2026-09-01 origin incidents (stale `:latest` surviving the
  19:28 reboot; `integration.sh` re-stamping `:dev` which WAS prod's live
  tag), the manual release procedure, updating one image out-of-band
  (caddy pin bumps), pin-bump policy, an auto-updater summary (pointing at
  `deploy.md` for full mechanics), and rollback (manual vs. automatic
  SHA-preferred). Deliberately NOT a duplicate of `deploy.md`'s Auto-update
  section — it's the "how do I ship/update a thing" companion; `deploy.md`
  keeps the full security-model prose and now links back to
  `versioning.md` in two places (a new line right under the "## Auto-update"
  heading, and the Bring-up section).
- **`config.Load` (`internal/config/config.go`) now has NO fallback for
  `STYRMAN_IMAGE`** — an empty value is a hard `Load()` error citing the
  operator rule by name in the error string itself. This is a real, load
  -bearing behavior change from every prior session's assumption of a
  `styrman-runtime:latest` default — re-verify this hasn't regressed before
  citing "no default" again, it's exactly the kind of fact that's easy to
  half-revert.
- **`deploy/docker-compose.yml`'s `image:` lines are now
  `${STYRMAN_VERSION:?set STYRMAN_VERSION=X.Y.Z — mutable tags are
  banned}`** — compose itself refuses to run unset, not just `config.Load`.
  `STYRMAN_IMAGE` defaults to `styrman-runtime:${STYRMAN_VERSION}` in the
  server's `environment:` block (still overridable, e.g. for local dev runs
  that pass `:dev` explicitly).
- **Found and fixed a REAL functional gap, not just stale prose**:
  `deploy.md`'s Bring-up section and both README.md's/`guide.md`'s quick
  starts previously told the operator to `docker build -t
  styrman-runtime:latest image/` then `docker compose ... up -d --build`
  WITHOUT setting `STYRMAN_VERSION` for the build step — under the current
  code this leaves `STYRMAN_IMAGE` resolving to a `styrman-runtime:X.Y.Z`
  tag that was NEVER BUILT (only `:latest` was), so projects could not
  boot. Rewrote all three quick-start blocks to build
  `styrman-runtime:${STYRMAN_VERSION}` explicitly before `compose up`, with
  `STYRMAN_VERSION` set consistently across both commands. This is the kind
  of drift-become-breakage this doc-sync pattern exists to catch — don't
  assume a mechanical "fix the tag string" edit is enough, trace whether
  the resulting command sequence actually still works end to end.
- **`deploy/host-setup.sh` step 7 STILL builds `styrman-runtime:latest` by
  default** (`RUNTIME_IMAGE="${STYRMAN_IMAGE:-styrman-runtime:latest}"`,
  confirmed by reading the script — this was NOT changed by the session's
  working-tree diff, it's a pre-existing gap the no-mutable-tags rule
  hasn't reached yet). This is now DEAD WEIGHT once a real `vX.Y.Z` deploy
  runs (production references `styrman-runtime:${STYRMAN_VERSION}`, never
  `:latest`) — documented honestly as a one-time SMOKE-TEST build (confirms
  Kata can boot the image) rather than claiming it's what production uses,
  in README.md/guide.md's quick-start comments. Did NOT edit
  `host-setup.sh` itself (shell script, code not docs) — flag this for
  whoever next touches that script: the `:latest` default there is
  inconsistent with the rule enforced everywhere else and is a legitimate
  future cleanup target.
- **ADR-052 follow-up landed in code this session**: `styrman-update` has
  NO in-script default for `STYRMAN_UPDATE_STATE_DIR` at all (a bare unset
  value is now a hard `fail`, where before it likely had a
  production-aliasing default — this is the actual code fix for the
  2026-09-01 state-loss incident) and explicitly refuses any value that IS
  or is INSIDE `/var/lib/styrman`. The production path
  (`/etc/styrman/update`) is pinned in exactly one place,
  `styrman-update.service`'s `Environment=` line — documented this as the
  single source of truth in `deploy.md`, `versioning.md`, and a NEW
  "## Privileged (root) test convention — scratch state only (ADR-052)"
  section in `.agents/docs/testing/styrman/README.md` (generalizes beyond
  just the updater — any future root-run host script inherits the same
  rule).
- **The `:dev`-is-prod testing warning from an earlier 2026-09-01 pass
  needed a two-phase honesty fix, not a flat correction**: it previously
  said unconditionally "do NOT build `:dev`, it's what prod boots." That
  was true when written (this host's live, gitignored `deploy/.env` really
  did have `STYRMAN_IMAGE=styrman-runtime:dev`) but will stop being true
  once v1.16.1 deploys and that override is presumably cleared. Rewrote to
  state BOTH phases explicitly ("binding until v1.16.1 deploys... re-verify
  before assuming otherwise") rather than either leaving a warning that
  will silently go stale, or removing a warning that's still live right
  now. **Do not assume this override has been cleared in a future
  session** — re-check the live host's `deploy/.env` (not `.env.example`,
  which never had it) or ask, since it's gitignored and this repo view
  can't see it directly.
- **`api.md`'s Server-environment table had the exact same stale
  `styrman-runtime:latest` default claim** for `STYRMAN_IMAGE` — found via
  a repo-wide grep for `runtime:latest`/`0.0.0` while checking for
  collateral damage, not called out in the dispatching task's explicit file
  list. Fixed anyway (squarely api.md's job to match `config.go`), same
  pattern as prior sessions: a task's named file list is a floor, not a
  ceiling — grep for the exact stale string across `.agents/docs/**/*.md`
  and root `*.md` before considering a mutable-tag/versioning doc pass
  done, changelogs and pre-existing review/audit reports excluded (their
  own lane, historical record).

Files touched this session: `.agents/docs/specs/styrman/versioning.md`
(NEW), `deploy.md` (Production-mode paragraph, Bring-up section, Auto-update
section — intro link + "What it does" bullet + run-state paragraph),
`README.md` (quick start rewritten, new versioning.md pointer),
`.agents/docs/specs/styrman/guide.md` (§3 install code block, same fix
shape as README), `.agents/docs/testing/styrman/README.md` (`:dev` warning
rewritten two-phase, new "Privileged (root) test convention" section),
`AGENTS.md` (Setup Commands — versioning.md pointer added, deploy line
itself unchanged since it already matched; Testing — new ADR-052 bullet),
`api.md` (`STYRMAN_IMAGE` table row). No changelogs/ADRs/TODO.md touched
(changelog subagent's lane, running in parallel per the dispatching task —
ADR-050/051/052 already existed, read but not edited).

See [[feedback-styrman-doc-edits]] for edit-process lessons.
