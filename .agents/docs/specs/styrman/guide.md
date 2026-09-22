# The styrman guide

Everything about the project in one place: what it is, how it works, how to
install it from nothing, how to operate it, and how to develop it. Detail
specs: `api.md`, `terminal-protocol.md`, `runtime-image.md`, `deploy.md`;
decisions: `../../architecture/DECISIONS.md`; history: `../../changes/`.

## 1. What styrman is

A self-hosted, multi-user web platform for running AI coding agents in
isolated projects. Sign in with Google, create a project, and get browser
terminals inside a dedicated **Kata Containers microVM** — its own kernel,
its own Docker daemon, its own network. Both agents (`claude`, `agy`) are
preinstalled; run whichever you want, in as many terminals as you want.
Every project also has its own browser — a headless Chromium inside the
microVM, shown in the Browser view and shared with the agents (ADR-068).
Live at https://crew.styrman.xyz.

Load-bearing ideas:

1. **1 project = 1 microVM** (ADR-001), identified by a **UUID** (ADR-020) —
   containers are `styrman-<uuid>`, so friendly names are freely renamable
   and reusable. Projects cannot reach each other over the network, ever
   (R12, verified adversarially).
2. **1 project = 1 home** (ADR-031, revising ADR-019; R3). Each project
   mounts its OWN home at `/var/lib/styrman/homes/<user>/p/<uuid>`, seeded
   from the base image (dotfiles; the agent harness as the `sh-agent`
   release binary the image carries, R5, ADR-074 — installed into the
   home on first boot and on image bumps, never over a home whose user
   moved it forward with `sh-agent update` inside the project; mechanics
   in `deploy.md` → "Agent harness in the image (ADR-074)") — no tool
   state leaks between projects. The workspace
   additionally lives in its own
   storage dir (`/var/lib/styrman/projects/<user>/<uuid>`, ADR-027),
   bind-mounted at `~/projects/<dir>`. The ONLY thing shared across a
   user's projects is credentials, via the broker (ADR-030/032): sign in
   once during first-run setup (or in any project) and every project stays
   current automatically, no separate import step (ADR-034). Containers
   stay disposable; users never see each other's worlds. Access is managed
   via `STYRMAN_ALLOWED_EMAILS` (+ admin-only API endpoints — the runtime
   user-management UI was removed in v1.3, operator Q4).
3. **Terminals** (ADR-016): durable records backed by tmux sessions — they
   survive tab closes and disconnects, get a GENERATED friendly name at
   creation (rename any time: header click or the sidebar pencil), carry an
   optional AUTOSTART command (runs when the session is created — first
   open and re-opens after pause/VM restart, never on reconnect — then
   drops to the shell; bolt badge in the sidebar), can be paused (session
   ends, record + transcript stay) and closed, singly or all at once. The shell
   exiting is presented the same way: a **Paused** state with a **Resume**
   action, not a dead terminal. Everything printed in them is transcribed to
   the home (ADR-017): the History tab. A thin **status bar** sits under every
   terminal (`GET /api/projects/{id}/stats`, polled every 4s): CPU %, mem,
   disk, process count, IP, uptime, and a `creds N synced` count with a
   tooltip of what the broker has provisioned into this project (v1.6.0/
   v1.8.1). Plain **Ctrl+C always sends SIGINT** (explicit `\x03`, bypassing
   both the browser's own copy-on-Ctrl+C and xterm's default so a lingering
   copy-on-select highlight can no longer swallow it); Cmd+C or Ctrl+Shift+C
   copies explicitly; paste is Ctrl/Cmd+V or Ctrl+Shift+V via the clipboard API.
   AltGr (Windows/Linux) and Option (macOS) compose non-US symbols (`@ $ {
} [ ] € …`) instead of acting as Meta, and Nordic/Swedish dead keys (`~
\` ^ ´ ¨`) are resolved directly rather than through xterm's buggy
browser-IME path — see terminal-protocol.md's keyboard-handling section.
A sidebar notification bell (badge,
linking to the terminals waiting for input) surfaces terminals whose tmux
window rang its bell while unattended — tmux clears the flag once the
session is viewed. The terminal (`web/src/lib/terminal.ts`) copies on
select: the clipboard is written the instant a selection is highlighted,
not on an explicit Ctrl+C, because TUIs (claude, agy) redraw and wipe
selections before a keypress can land — the runtime image's tmux
`mouse off` default is what lets native browser selection work at all. (The v1.2 link-rescue bar was removed in v1.3 — copy-on-select
   superseded it.) Ctrl+Shift+F opens a scrollback
   search overlay; OSC 52 lets tmux/TUIs write the system clipboard
   directly. It renders via xterm's DOM renderer, not WebGL, so terminal
   text stays real, selectable, assertable DOM. Anything that lands on the
   clipboard is recorded in the Clipboard side-panel tab — session-scoped
   (sessionStorage, 30 entries, 1h TTL, never persisted or synced) since
   copy-on-select could capture a secret typed during setup (ADR-023).
4. **The side panel is project context**: Files (workspace + home; create/
   rename/delete files and folders; view + edit via a lazy-loaded
   CodeMirror 6 editor with syntax highlighting for ~20 languages including
   Java/C/C++/Rust/PHP/SQL/XML/Terraform (v1.7.2) — the editor is a VIEW of
   the terminal's MAIN pane, not a separate modal (2026-09-01, replacing the
   earlier centered file-viewer modal): a `Terminal | Editor` control in the
   terminal header toggles the pane, state is kept per terminal (one
   terminal can sit on the editor while another in the same project shows a
   shell), and the terminal keeps running underneath either way (hidden with
   CSS, never unmounted, so its session survives the toggle). Clicking a
   file in the Files panel opens it in the pane and switches the view there;
   saving no longer closes it, just re-disables Save until the next edit
   (v1.7.2); the explorer row for whatever THIS terminal's pane currently
   holds is highlighted (matched by scope kind + path), and a per-project
   **Recent** strip sits BELOW the file listing (a border-t row, not
   between the header and breadcrumbs — mounting it above shifted every row
   under a stationary pointer, breaking double-click, 2026-09-02) showing
   ALL up to 8 recently opened files: home-scope chips read `~/<full
   path>`; project chips show the basename, or `parent/basename` when
   another recent shares that basename (identical chips used to reopen the
   wrong file) — click a chip to reopen; the currently-open file's chip is
   highlighted too; the list survives reloads via localStorage
   (`styrman.recentFiles.<project>`, one canonicalizing parser shared by
   every read and write — relative-only, no empty/`.`/`..` segments, ≤512
   chars, first-wins dedup — degrading silently to session-only if storage
   is blocked) and is pruned when its file is deleted or renamed
   (2026-09-01). New file / New folder refuses a name that already exists
   in the current listing (creating over one used to silently truncate it
   via the save endpoint, and with no backup layer that was unrecoverable,
   2026-09-02) — client-side only, so a raw API call can still truncate.
   Leaving a terminal — another terminal, a project, Settings — while the
   editor has unsaved edits is blocked with a "Discard and leave" confirm
   dialog, and closing or reloading the browser tab triggers its native
   unsaved-changes warning too (2026-09-02): the per-terminal dirty flag
   does not survive an unmount, so leaving without asking would silently
   discard the buffer. The terminal header's `Terminal | Editor` toggle
   shows the held file's basename (truncated, mono) next to the Editor
   label so the open file is visible even while looking at the terminal view
   (2026-09-01) — the basename, and ", unsaved changes" when dirty, are
   also exposed to screen readers via `aria-describedby` (the toggle's own
   accessible name stays pinned to "Editor"/"Terminal" so exact-name
   locators and screen-reader muscle memory don't break, 2026-09-02); a
   **Wrap** toggle controls soft-wrap (default on, persisted
   across sessions); markdown opens as
   a RENDERED preview with an Edit toggle; a **Format** button reformats with
   the file's real CLI formatter running INSIDE the project VM — `gofmt`,
   `prettier`, `black`/`ruff`, `rustfmt`, `shfmt`, `terraform fmt`/`tofu fmt`
   — except JSON, which still formats locally/offline (v1.7.3); a syntax
   gutter (red squiggles from the same in-VM checker) appears ONLY for file
   types the server actually has a checker for — the SPA's
   `CHECKABLE_EXTENSIONS` mirrors the server's `checkFile` cases one for
   one (TODO close-out 2026-09-10, item K8), so a `.xyz` file never shows a
   gutter that can only ever report "no checker"; each edit debounces one
   in-flight check request, aborting the previous one rather than racing it
   (item K7); upload by
   drag (an upload that overwrites an already-open file evicts that file's
   editor pane, keyed off the server's own reported saved base names rather
   than what was dropped — on a clean success AND on a mid-stream failure
   alike, since the server reports what actually landed on disk before the
   error too, 2026-09-02 — so a stale in-memory copy of overwritten content
   can never linger visibly open; its Recent chip is kept, since the file
   itself still exists), download; destructive operations on
   platform-managed home paths are refused). Closing a file with unsaved
   edits via its "Discard" confirm now actually discards: the parked draft
   and its dirty flag are cleared and the loaded doc is dropped so the next
   open re-reads the file from disk, rather than a close that silently kept
   the buffer around (item K9)., Ports (what's
   listening, with process / inner-docker image, shareable
   `/p/<id>/<t>/port/<port>/` links; per-port sharing — adding a Google
   email mints a stable **private link** `https://<token>.private.…/`
   requiring sign-in + membership in the share list, ADR-026 — and public
   TTL links on `https://<token>.public.…/`, ADR-025), **Env** —
   project-scoped env variables (v1.12.0, ADR-043): like Settings' custom
   variables but delivered ONLY to this project's new login shells; a
   project variable overrides a same-named global one, deleting it falls
   back to the global, and the variables die with the project. Click a name
   to reveal its value (audit-logged); stored via the broker's env tier so
   NO guest change was needed — the broker merges them per calling project
   — History (transcripts under their terminal's friendly name + FULL-TEXT
   SEARCH across all of them — everything you and the agents typed or saw),
   **Git** — a real git client (v1.7.0, see below), Clipboard
   (session-scoped — see above). The HOME surface is a dashboard: project
   cards with live transcript tails per running terminal (15s refresh,
   click to jump in), plus an OPS ROW per running card (v1.14.0):
   cpu/mem/disk/procs chips and a Logs dialog with the microVM console
   tail. The sidebar shows per-project GIT BADGES (branch, amber ±N when
   dirty — one batched /api/git/summary call). One shared listProjects
   poll feeds every surface; polling pauses app-wide in hidden tabs, and
   an expired session triggers one toast + reload into the OAuth flow.
5. **Git is the substrate** (R13, ADR-022): every project workspace IS a git
   repository, no exceptions — `git init -b main` runs synchronously at
   creation, or a clone runs in-guest instead (new project dialog's "Clone a
   repo" option, so the user's ssh key/gh credentials apply; falls back to
   an empty repo on failure), and a boot-time sweep initializes any
   repo-less workspace it finds (adopted pre-R13 projects). The Git side
   panel tab is a real git client (v1.7.0): branch + ahead/behind, Push, a
   commit-message box, "Staged"/"Changes" file groups with per-file stage/
   unstage/discard, and a colored per-file diff viewer. v1.22.0 widened it:
   diffs open in the MAIN pane as a third view beside Terminal and Editor
   (the old in-panel box was 64px tall inside a 320px panel), a Fast commit
   button stages everything and commits a generated UUID behind a confirm
   naming the file count, and the header gained Pull (`--ff-only`), a branch
   dropdown that switches or creates, a Commits list whose entries open that
   commit's diff in the pane, and Stash (`push -u`, so untracked work is
   captured too). A conflicting `pop` leaves conflict markers AND keeps the
   entry — the UI says so rather than unwinding it. Every command runs
   inside the guest VM so the user's identity and the broker's credential
   helper apply, and commits are authored correctly (`git commit` in a
   terminal works too as of v1.7.1 — the broker syncs the git identity, not
   just the auth token). The AUTOMATIC checkpoint mechanism this section
   used to describe (a 10-min loop + snapshots on stop/pause/close) was
   REMOVED in v1.7.0; the MANUAL checkpoint API that survived that change was
   then deleted outright in the 2026-08-30 security review (ADR-039) — it,
   and the standalone host-side `/diff` route, ran host-side `git` against a
   bind-mounted, guest-writable workspace, a guest→server RCE. All that
   remains host-side is a hardened `git init`/`git rev-parse`
   (`internal/gitrepo`), run with config overrides that neutralize a
   guest-controlled `.git/config`. There is no backup layer of any kind
   (operator decision 2026-09-01, superseding the earlier two-layer backup
   feature) — durability is git: commit and push. A wipe-delete is plainly
   irreversible; see deploy.md's "Durability" section.

## 2. Architecture

```
Browser ──HTTPS──► styrman-caddy (host net): Google OAuth, TLS, Cloudflare DNS,
                   label discovery (vendored ingress, deploy/caddy)
                      │ X-Caddy-Email + X-Styrman-Ingress
                      ▼
   styrman-server (container; header-auth; no auth-bypass listener in prod)
     ├── embedded TanStack Start SPA (ADR-013)
     ├── identity → per-user space: homes/<user> (server-only parent of
     │   base/ and p/<uuid>, ADR-064), users/<user>/{projects,
     │   conversations,profile}.json  (ADR-019; no database, ADR-007)
     ├── REST (api.md) · WS /ws/term/{id}/{t} (terminal-protocol.md)
     ├── /p/{id}/{t}/port/{port}/ → proxy to apps inside the microVM
     └── Docker SDK ──► per-project Kata microVM  styrman-<uuid>  (mvm)
                          ├── tini → entrypoint → sleep; exec-based terminals
                          ├── dockerd: overlay2 on loop-mounted ~/.dockerd/<id>.img
                          ├── virtiofs: projects/<user>/<uuid> → ~/projects/<dir> (ADR-027)
                          ├── tmux conv-<n> per terminal (bash -l), piped to
                          │   ~/.styrman/logs/<id>/  (transcripts)
                          └── virtiofs: /home/<guestname> = homes/<user>/p/<uuid> (ADR-031)
```

- **Identity**: email → username slug (`ostling.simon@gmail.com` →
  `ostling-simon`); the _terminal username_ inside guests defaults to the
  slug but is user-configurable (Settings → Account; applies to VMs started
  after the change).
- **Guest hostname is the project's own name** (shell prompt, terminal status
  bar) — no `styrman-` prefix; see runtime-image.md's Invariants for the
  exact derivation and why an existing container keeps its old hostname
  until next recreated.
- **State recovery** (ADR-007): containers carry `styrman.*` labels
  (project UUID, user, name) and are re-adopted at boot; legacy layouts are
  migrated automatically (single home → per-user; name-keyed → UUID).
- **Project creation is async**: the API returns instantly
  (`state:"creating"`); the microVM boots in the background (~3 s) and the
  first terminal attach waits on readiness. A `cloneUrl`/`github` create needs
  credentials, so once the microVM is up the background boot calls
  `provisionRepo` (`internal/httpapi/provision.go` — split out of
  `server.go` so the sequence lives next to `provision_test.go`, which pins
  the step ORDER; attach and exec are injected as funcs so that order is
  unit-testable without a Docker daemon), which joins the project's own
  bridge to the credential broker (`EnsureSelfAttached`) and polls
  `$STYRMAN_CRED_BROKER/healthz` from inside the guest over a PLAIN shell
  (`waitForBroker` — a login shell would re-materialize credentials into the
  project home on every probe, just to ask "is the broker up?") BEFORE
  cloning or running `gh repo create` — attach used to run LAST, after
  provisioning, so every credentialed create ran against an unreachable
  broker and silently degraded to an empty repo (no ssh key, no `GH_TOKEN`,
  no git identity). The terminal-WS boot path has the same shape of fix
  (BUG-060, 2026-09-14): `wsBoot` (`internal/httpapi/server.go`) now calls
  `EnsureSelfAttached` itself, synchronously, right after the microVM boots
  and before the socket is upgraded, because that path has no HTTP response
  to protect and its login shell needs the broker reachable immediately —
  before this, the first terminal of a project left stopped across a server
  restart came up with the broker unreachable. The OTHER way provisioning can fail — the microVM never
  boots at all — is handled by a sibling function, `bootFailed`, built from
  the SAME `provisionSpec` so the two outcomes can never disagree about what
  was asked for: before v1.18.1 a boot failure on the GitHub-create arm
  reported nothing at all (the clone arm's R13 fallback masked it), the same
  silent-failure defect class the attach-ordering fix above exists to
  prevent. Provisioning itself (the clone / commit / `gh` commands)
  still runs in a login shell so
  `/etc/profile.d/zz-styrman-credentials.sh` materializes those credentials
  first. Neither step does anything in host-mode dev (no self-container, no
  bridge, no broker — `EnsureSelfAttached` no-ops and `waitForBroker`
  returns false immediately), so that mode can only prove the clone
  ORDERING, never real credential delivery — see the Tier 4 e2e note in
  `.agents/docs/testing/styrman/README.md`. A provisioning failure surfaces as
  `provisionError` in the project
  list/get views (mirrors `deleteError`) — the sidebar shows a red "repo
  setup failed" badge with the message as its title; the guest output behind
  that message is redacted (`redactSecrets`) before it reaches the message
  or a server log. While the clone/init/`gh` step is still running — before
  it can fail or succeed — the sidebar shows a "provisioning…" chip in the
  same slot (`state:"provisioning"`, TODO close-out 2026-09-10, item N; the
  chip choice is a pure `projectBadge(p)` function, item 29, unit-tested for
  every state/error combination rather than left to the component to get
  right silently).
- **Isolation** (R12): per-project bridges (`sdb-<uuid>`, named
  `sdb-<11hex>` at the Linux interface level via `config.BridgeNameFor` —
  Docker's default `br-<hash>` interface name would slip past the firewall
  match, which is exactly what happened until v1.8.3, see deploy.md) shared
  only with the server, which runs `ip_forward=0` (asserted at boot since
  v1.33.0 — `STYRMAN_ALLOW_IP_FORWARD` in `api.md`'s env table is the
  deliberate override); the server joins project
  networks in the background (a live join can drop in-flight connections —
  the preview proxy 307s once instead when it must join on demand). A
  project reaches the **internet and nothing else**: not another project
  (separate bridges + Docker's inter-bridge DROP + a DOCKER-USER rule
  dropping private-range FORWARD traffic — LAN, other bridges, cloud
  metadata), and not the host itself (an INPUT drop) — plus a `physdev
--physdev-is-bridged` exemption so a project can still reach the
  credential broker and port-preview proxy on its OWN bridge (same-bridge L2
  traffic otherwise also traverses the isolation chain). Installed by
  `host-setup.sh`'s `styrman-firewall.sh` into dedicated `STYRMAN-IN`/
  `STYRMAN-FWD` chains, kept applied by a self-healing `styrman-firewall.timer`
  (re-runs every 60s; INSERT-IF-MISSING, never delete-then-insert, so there is
  no window where isolation is briefly absent) (ADR-029, reworked 2026-08-30
  — see deploy.md). The script (`deploy/firewall/`) drops six ranges
  (RFC1918, 169.254/16, and the CGNAT/benchmark ranges 100.64/10 and
  198.18/15) from every bridge whose interface name starts `sdb-`.
  Since ADR-067, per-project LAN access is not a firewall rule at all: it
  is which prefix the server gave that project's bridge interface at
  `docker network create` — `sdb-<hash>` (sandboxed, matched by the rules
  above) or `lan-<hash>` (matched by none of them, so it behaves exactly
  like a normal Docker container on this host — NAT'd out to the LAN, the
  host itself, and the host's published ports). Cross-project isolation
  does not depend on this script at all: Docker writes its own
  unconditional `-A DOCKER ! -i <br> -o <br> -j DROP` per bridge, which is
  what blocks a LAN project from reaching another project whatever either
  one's prefix is. Toggling a project's mode (§4 "Network") rebuilds its
  Docker network and container — the home and workspace are bind mounts and
  survive, packages are replayed from the ADR-028 apt manifest, and anything
  living only in the container's own filesystem is lost. There is no
  allowlist, no conf, no root-side per-project state, no spool and no
  version handshake to keep current: root parses nothing the server writes.
  A LAN project is served credentials exactly like any other — the
  ADR-065/066 SSH-key withholding latch is deleted, not merely unused (see
  §4).
- **Credential broker** (ADR-030/032): a credential-only listener (`:8091`,
  bridge-only, never published to the host) hands each project the CURRENT
  credential for a tool, identified by its bridge source IP — so a refresh in
  the one broker store propagates to every project. Tiers: `GET /git`
  (a system git helper pulls per-op, nothing stored in the project);
  `GET /env` (shell vars — `GH_TOKEN`, `NPM_TOKEN`, `STYRMAN_GIT_NAME`/
  `STYRMAN_GIT_EMAIL` — exported/applied by a login-shell snippet,
  `/etc/profile.d/zz-styrman-credentials.sh`, which also jq-merges claude's
  `hasCompletedOnboarding` flag so a materialized token doesn't re-trigger
  first-run login, v1.5.6); and `GET /files` (files — gcloud ADC, ssh key,
  claude's `.credentials.json`, and agy's OAuth token file — written into
  the home by the same snippet). When the broker does not answer (`/env` or
  `/files` fails), the snippet writes a marker instead of failing the login
  shell, and the next INTERACTIVE shell's MOTD prints one line naming the
  broker URL and the time credentials last failed to arrive (BUG-054); the
  marker clears once both calls succeed again. That's all three tiers: the fourth
  `GET /keyring` tier ADR-032a added for agy (Secret Service) turned out to
  not match agy's real build (a plain file, ADR-038) and was left dormant,
  then deleted outright in the 2026-08-30 security review (ADR-040) — no
  broker code, store kind, or shell step references it anymore. Finishing
  first-run setup now seeds the broker automatically from whatever the user
  just signed into (ADR-034, v1.5.4) — no separate import step required;
  `POST /api/settings/credentials/import {project}` remains as a
  supplementary/refresh path. Manage from the Settings **Credentials**
  panel (consolidated status-colored view, v1.8.2 — see below).
- **Idle lifecycle** (v1.11.0): terminal traffic (keystrokes + output on
  ATTACHED terminals) timestamps the project; candidates older than
  `STYRMAN_IDLE_TIMEOUT` (default 8h — a stop only powers the VM off,
  everything persists and the next attach resumes exactly where it left
  off) get a final in-guest busy probe before the plug is pulled: any
  RUNNING DinD container (`docker ps`), or any tmux pane whose foreground
  process isn't a plain shell (a detached build, claude still working),
  blocks the stop and resets the idle clock. Reopening a terminal boots on
  connect.
- **There is no backup layer** (operator decision 2026-09-01, removing the
  v1.13.0/v1.14.0 in-app + host-layer backup features entirely — code, UI,
  and host secrets alike): "if a user needs backup they should commit to
  git." A **wipe-delete** (`DELETE /api/projects/{id}?wipe=true`) removes
  the workspace, home, and docker disk with no safety net and no way back —
  the UI's "type delete to confirm" dialog is the only gate. Durability for
  anything that matters is: commit and push. See deploy.md's "Durability"
  section for rebuilding a lost host.
- **Stop is sticky** (v1.11.0): the sidebar Stop button (and the idle
  reaper) marks the project deliberately stopped; open terminal tabs show
  "Project stopped" instead of auto-reconnecting — an automatic reconnect
  used to boot the VM right back up, making Stop un-stick whenever a tab
  was open. Only explicit user intent boots it again: the Start button,
  opening/resuming a terminal (its first connect carries `?boot=1`), or
  any API start. See terminal-protocol.md's close 4002. Since v1.34.1, Stop
  also switches the container's restart policy to `no` before the guest's
  own shutdown, and the next Start or terminal attach switches it back to
  `unless-stopped`, so Docker never restarts the clean exit (BUG-063).
- **Caddy's ingress upstream is chosen deterministically**: the server
  container is multi-homed (the ingress network plus each project's `sdb-*`
  bridge, for the broker + preview proxy), so a naive "first network" pick
  is random-order and can resolve to a dead project-bridge IP — this caused
  a live 502 (v1.8.5). `extractContainerIP` now prefers `styrman-ingress`,
  then any non-`sdb-*` network, falling back to whatever's there (correct
  for a preview VM, which has only its own `sdb-*` network) — see deploy.md.
- **Auth is two-layer** (R8): the ingress proves identity via Google OAuth
  (`X-Caddy-Email`) and a shared, constant-time-compared `X-Styrman-Ingress`
  token proves the request actually came through Caddy — a project VM on
  the server's shared bridge can't reach `:8080` and spoof either header;
  the server then re-checks the email against its own userstore. Production
  runs no auth-bypass dev listener (`STYRMAN_DEV_ADDR` is opt-in, must bind
  loopback, and is unset in the compose deploy — 2026-08-27 security
  review). Every auth misconfiguration fails CLOSED at startup, not just at
  request time: `config.Load` refuses to start with `STYRMAN_AUTH=header` and
  an empty `STYRMAN_INGRESS_TOKEN`, and refuses any `STYRMAN_AUTH` value
  other than `none`/`header` outright (2026-08-30 security review, H2/M16).
- **Host-side git is bootstrap-only** (ADR-039): the server process itself
  only ever runs `git init`/`git rev-parse` (`internal/gitrepo`), both
  hardened against a guest-controlled `.git/config` (fsmonitor/hooksPath/ext
  transport all neutralized). Everything else git — status, diff, stage,
  commit, push — runs INSIDE the project's own guest VM (ADR-035); the
  former host-side checkpoint machinery, which ran unhardened host-side git
  against the same bind-mounted, guest-writable trees, is deleted outright,
  not merely unused.
- **Guest exec output is bounded** (H4/H7): `Orchestrator.Exec` (readiness
  probes, tmux bookkeeping, git status/diff, stats, format) caps a guest
  command's combined stdout/stderr at 2 MiB in memory — the guest is
  untrusted root in its own VM and otherwise controls how much the shared
  server holds — and guarantees a 30s deadline when the caller passes none,
  so a wedged guest command can't pin a goroutine forever.
- **File ops are confined** (`internal/files`, `os.Root`): every
  home-relative read/write is rooted at the user's home and refuses any
  symlink that would leave it, atomically at every path component — the
  tenancy boundary for guests that can otherwise symlink freely inside
  their own home.
- **Sessions survive redeploys**: caddy-security's JWT signing key is
  pinned via `STYRMAN_JWT_KEY` (`host-setup.sh` generates it) instead of
  rotating per boot, so restarting Caddy doesn't log everyone out.
- **Browser view** (ADR-068): one headless Chromium per project, started on
  demand as the project user by `/styrman/bin/styrman-browser` and stopped
  with the project. The SPA's Browser pane draws a CDP screencast over an
  owner-only relay, `GET /ws/browser/{project}` (never `previewAccess`, so a
  share-list guest gets the same 404 as any other project route); the agents
  drive the SAME browser through the `styrman-browser` MCP server, registered
  for both `claude` and `agy` at harness seeding (the narrow ADR-062
  exception: each tool's own `mcp add` command writes its own MCP registry,
  no `settings.json`, no hooks). Trust boundary: Chromium's own DevTools
  listener is loopback-only inside the guest (port 9229); a `socat` forwarder
  republishes it on the project bridge (port 9222) but admits only the
  styrman server's own bridge address as a peer, so a container the user
  merely runs inside the project cannot reach it. Port 9222 is never listed,
  previewed, shared, or linked. No Origin is admitted on the DevTools socket
  (no `--remote-allow-origins`), so a page rendered inside the browser can
  never open a DevTools connection to seize its own browser. This is the
  first long-running in-guest daemon since ADR-002 — Chromium is a tool the
  project user runs, not a server the styrman server depends on: the server
  still only reaches the guest through `docker exec` (start/stop/status) and
  the project bridge (the CDP relay), exactly as the port-preview proxy
  reaches a dev server. The pane is mounted per terminal, keyed on
  `${project}/${terminal}`: a terminal that never opened the Browser view
  holds no relay and no launcher exec at all. Switching to another view
  (Terminal, Editor, Diff) keeps the relay open but hides the pane — the
  screencast STOPS and no viewport is pushed to the guest, so a hidden 0×0
  pane can neither resize the agents' shared browser nor keep streaming
  frames nobody sees; switching back resumes the screencast. AltGr
  (Ctrl+Alt) characters type into the page rather than triggering a
  shortcut — the pane treats only Ctrl/Cmd-without-Alt as a chord. A
  **Retry** button (shown only after a failed launch) re-runs the start
  through the REST endpoint (`POST .../browser/start`) first, so it can
  show the launcher's FULL log tail instead of the 120-byte close-reason
  budget the relay itself carries. Full detail: runtime-image.md's
  "Browser" section, api.md's "Browser (ADR-068)",
  terminal-protocol.md's "Browser relay".

## 3. Install from scratch (R11)

Baremetal Debian 13 or a GCP VM with nested virtualization.

```bash
git clone <repo> styrman && cd styrman
sudo bash deploy/host-setup.sh --user $USER   # Docker, Kata "mvm", daemon cfg, dirs, the
                                              # isolation firewall + auto-update systemd
                                              # units (auto-update installed but disabled
                                              # until its clone + GitHub token exist,
                                              # see deploy.md), seeds deploy/.env, and does
                                              # a one-time SMOKE-TEST build of
                                              # styrman-runtime:latest to confirm Kata can
                                              # boot it — production never runs this tag
                                              # (no mutable tags; versioning.md)
GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN \
  -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/  # separate tag for the loopback dev run; the version keys harness re-seeding

# Dev (no auth, loopback; needs a Go 1.25+ toolchain — host-setup.sh does NOT install one):
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/   # → http://127.0.0.1:8080

# Production (Google OAuth + HTTPS + reboot-safe; needs neither Go nor Node):
vi deploy/.env    # BASE_DOMAIN, GOOGLE_CLIENT_ID/SECRET, CLOUDFLARE_API_TOKEN,
                  # ADMIN_EMAIL, STYRMAN_ALLOWED_EMAILS, STYRMAN_HOSTNAME (default
                  # styrman; the live deploy uses crew) (redirect URI:
                  # https://auth.<domain>/oauth2/google/authorization-code-callback)
# The version derivation + build/deploy commands are NOT restated here — see
# versioning.md's "Release procedure" for the exact steps (commit, verify the
# COMMIT not the working tree, sign + push a vX.Y.Z tag, let the auto-updater
# deploy it) and its "Manual / break-glass fallback" for the one-time,
# no-updater-yet path a from-scratch install actually needs. Do NOT derive
# the version with `git describe --tags --always` — versioning.md explains
# why that command's SHA-suffixed fallback breaks the no-mutable-tags rule
# (ADR-053).
# DNS records + certs are automatic → https://<STYRMAN_HOSTNAME>.<domain>
```

`host-setup.sh` installs a pinned Docker CE version and Kata release with a
fail-closed SHA256 check before extracting it as root — see deploy.md for
the exact pins and the `daemon.json` keys it sets. Node 22+ is needed only
to rebuild the frontend (`cd web && npm run build`; `web/dist` ships
committed) — neither path above requires it to just run.

First use per user: the sidebar shows a **First-time setup** banner →
"Open setup" boots a hidden minimal microVM running the guided
`styrman-setup`. Only git identity and an ssh key are REQUIRED; GitHub,
Google Cloud (account login + separately Application Default Credentials),
Claude, and agy are each OPTIONAL — `[y/N]` prompts, skip any and rerun
`styrman-setup` anytime, in the setup VM or any project terminal, to pick up
where you left off (v1.8.0). Already-set credentials just show ✔ with a
`renew? [y/N]` prompt — Enter keeps and moves to the next step, `y` re-runs
that sign-in (renew/switch account; a renewed ssh key keeps the old pair as
`id_ed25519.replaced`) (v1.10.1). The generated SSH key's comment is the user's
git email (fallback `user@styrman` if none is set), not "styrman". Closing
the setup terminal (typing `exit`, letting the script finish, or **Ctrl+C** —
a clean exit as of v1.10.0, no longer a restart) returns to the dashboard and
seeds the credential broker from whatever was just signed into — every
project, current and future, is current with zero re-auth, no manual import
step (ADR-034, v1.5.4). The exit is **status-aware** (v1.10.0): setup is only
marked done (`POST /api/setup/done`, VM destroyed) once the required creds
(git identity + ssh key) are present; a Ctrl+C before that just discards the
VM (`DELETE /api/setup`) and leaves the banner up so you can resume — an
incomplete setup is never marked done. Settings → Credentials →
**Import from a project** still exists as an optional refresh path (a
rotated token, or a tool signed into inside a project instead of at setup).

## 4. Operating it

- **New project**: name + repo choice (empty git repo, clone an existing
  one, or create+push a fresh GitHub repo via `gh`) → ~3 s to a live
  terminal; type `claude` or `agy`. Cloning auto-fills the name field from
  the repo URL's own basename (slugged to the project-name rules, `.git`
  stripped) until you type your own name, which then wins. A clone URL
  carrying an embedded credential (`https://user:token@host/...`) is refused
  inline — the Create button stays disabled with an explanation until the
  token is removed (server-side: `cloneURLHasSecret`, api.md); nothing needs
  one, styrman supplies git credentials automatically. Every workspace
  is a git repo from creation, no exceptions (R13). Rename any time (⋯ →
  Rename); delete via the type-"delete" dialog (wipe checkbox controls
  files).
- **Git**: the Git tab is a real client (v1.7.0) — branch bar with Push, a
  commit-message box, "Staged"/"Changes" file lists with per-file stage/
  unstage/discard, and a colored per-file diff viewer. v1.22.0 added Pull,
  a branch dropdown (switch / create), a Commits list, Stash push/pop, a
  Fast commit button, and moved diffs into the MAIN pane. (Automatic
  checkpoint snapshots were REMOVED in v1.7.0 — see §1 idea 5. There is no
  backup feature to replace them with: durability is git — commit and
  push. See deploy.md's "Durability" section.)
- **Network mode** (ADR-067, per project): the top of the **Ports** tab
  holds a "Network" switch (Sandbox / LAN). Sandbox is the default and means
  internet only. LAN means the project becomes an ordinary machine on the
  operator's network, outbound: it reaches everything the styrman host
  itself reaches — other devices on the LAN, the host itself including its
  SSH service, and the host's published Docker ports — exactly like a normal
  Docker container, because that is now literally what the mode is: the
  server names the project's Docker bridge interface `lan-<hash>` instead of
  `sdb-<hash>`, and no styrman firewall rule matches that prefix. It remains
  NAT-only outbound: nothing on the network can open a connection INTO the
  project, and it still cannot reach any other project (Docker's own
  unconditional per-bridge DROP, unaffected by either prefix). An **admin**
  turns it on; the owner may always turn it off. The confirmation dialog
  names both consequences: the blast radius (everything above), and that
  **applying the switch rebuilds the project's container** — the home
  directory and workspace are untouched (bind mounts) and installed packages
  are replayed from the ADR-028 apt manifest, but anything living only in
  the container's own filesystem is lost. A running project's toggle applies
  by rebuilding immediately; a stopped project's applies the next time it
  starts. There is no intermediate state — no `pending`/`error`/
  `unavailable`, no host firewall to be outdated or misconfigured, because
  the switch is the interface name the server itself picks at network
  create, not a request to anything root-owned. The switch compares the
  request against the project's LIVE bridge, not the stored record, so a
  project whose badge has drifted from its actual reach is repaired by
  simply toggling the switch again — no separate Stop/Start needed. A
  change that could not be applied (the container couldn't be inspected,
  stopped, or restarted) reports that the mode was **NOT changed**, and in
  that case nothing changed. A small `LAN` chip flags the
  project in the sidebar and on the dashboard while the mode is LAN. The
  credential broker no longer withholds the SSH key from LAN projects —
  see "Credential broker" above — so unlike ADR-065/066 there is nothing to
  show under Settings for this feature; that panel is gone.
- **Using the browser** (ADR-068): a **Browser** pill sits next to
  Terminal/Editor/Diff in the terminal header. The address bar accepts a
  full URL, a bare `:3000` or `localhost:3000` shortcut, or a click of
  "browser" on a Ports row (opens that port) — `localhost` inside this
  browser IS the project's own microVM, no proxy or share link involved.
  The tab strip shows every open page, including tabs the agent opened.
  Ctrl/Cmd+L focuses the address bar; Ctrl/Cmd+R reloads; AltGr (Ctrl+Alt)
  characters type into the page rather than triggering a shortcut. States:
  "Project is stopped" (start it from the terminal, then Reconnect);
  "Browser unavailable" with a Retry button — Retry re-runs the launch
  through `POST .../browser/start` first, so the pane can show the
  launcher's FULL log tail instead of the shorter reason a failed relay
  handshake carries; "Browser disconnected" with a Reconnect button (the
  guest's Chromium went away — the project stopped, or it crashed, or the
  frame budget was exceeded); "Access revoked" (a user removed from the
  project while the pane was open — checked on the same 60 s interval as a
  terminal's own re-authorization). The pane is mounted only for a terminal
  that has opened the Browser view at least once — switching to Terminal,
  Editor or Diff keeps the connection but pauses the screencast (no frames,
  no viewport pushed to the guest) until the Browser view is shown again.
  The browser stops when the project does; its profile — cookies, logins,
  everything Chromium would normally remember — lives in the project home
  at `~/.styrman/browser/profile` and is per project, like everything else
  under ADR-031.

  The agents drive the same browser and the same profile. Do not sign in to
  anything in the Browser pane you would not hand the agent — a page the
  agent is asked to test can instruct it, and it can then read every
  signed-in session in that profile.

  From a project terminal, `claude` or `agy` already has the
  `styrman-browser` MCP server registered (tools to navigate, click, type,
  and screenshot the SAME browser the pane shows) — no setup step, it was
  seeded into the project home with the rest of the agent harness. A tab the
  agent opens appears in the pane's tab strip.

  Known limits (v1): no clipboard between the page and your machine, no
  IME/dead-key composition, no downloads, file chooser, or print;
  JavaScript dialogs (`alert`/`confirm`/`prompt`) are accepted automatically
  with a one-line notice rather than shown; the browser is not stopped for
  idleness — it runs until the project does.
- **Terminals**: sidebar, per project; generated friendly names; ⏸ pause /
  ✕ delete / close all; per-terminal autostart command (pencil → dialog;
  set the autostart to `claude --continue` / `agy --continue` and the
  terminal RESUMES its conversation after a pause/VM restart (both CLIs
  start fresh when there is nothing to resume). Deliberately OPT-IN per
  terminal, not automatic: all terminals of a project share one cwd, so an
  automatic rewrite made several terminals resume the SAME conversation
  (2026-08-30 review);
  runs at session creation, then drops to the shell); transcripts land in
  History regardless; a status bar under the terminal shows live CPU/mem/
  disk/IP/uptime and synced-credential count (v1.6.0/v1.8.1).
- **Settings**: account + terminal username, a single status-colored
  **Credentials** view (v1.8.2, replacing the old separate "Tools &
  credentials" cards) — GREEN = signed in and synced to the broker (click a
  green row to read its stored value, v1.8.1), GREY = not set up, RED =
  signed in locally but not yet synced. The ssh row's stored value is the
  user's public key itself (abbreviated, with the full key on hover) and a
  copy button, ready to paste into a git host or an `authorized_keys` file —
  read via the same tool-status check the setup page uses, which refuses to
  echo a `.pub` that is a symlink. Grey and red rows — or their "sign
  in"/"re-sync" button — are clickable too (v1.10.0): clicking opens a modal
  that boots the hidden setup VM once and runs `styrman-setup <tool>` in an
  embedded terminal, harvesting that tool into the broker as soon as the
  step finishes; the VM stays up so a picker lets you add several
  credentials in one sitting without rebooting between them, and Ctrl+C in
  that terminal cancels just the current step and returns to the picker
  (the script's SIGINT trap makes this a clean exit, not a restart — see
  terminal-protocol.md). The picker's green rows are clickable too: an
  already-signed-in tool re-runs its step with a `renew? [y/N]` prompt —
  Enter keeps it, `y` re-authenticates (renew / switch account) (v1.10.1).
  "Done" (or clicking outside the modal) tears the VM down. An "Add a variable" row stores any custom env var (NPM_TOKEN,
  DATABASE_URL, …); "Import from a project" is marked optional (setup
  already auto-syncs, ADR-034) — theme (Light / Dark / System), and —
  admins only — LIVE runtime tunables: idle-stop timeout (applies to the
  reaper immediately) and docker disk size for new project VMs, persisted
  in `StateDir/settings.json` over the env defaults. (The user-access card
  was removed in v1.3 — access = env + admin API.) An admin's `DELETE
  /api/settings/users/{email}` is TERMINAL revocation only (TODO close-out
  2026-09-10, item I): every open terminal of the removed user closes
  within a minute (close 4003, see api.md/terminal-protocol.md), but their
  projects, tmux sessions, and brokered credentials keep running until an
  admin separately stops the projects — removing a user does not stop or
  delete anything of theirs by itself. Env-provided admins cannot be
  removed through this API at all; taking one off the list needs an env
  edit and a server restart.
- **Where things live** on the host:

| Path                                               | What                                                                                                                           |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `/var/lib/styrman/homes/<user>/p/<uuid>`           | A project's OWN home (ADR-031): dotfiles, tool state, docker disk, transcripts — nothing backs this up, see "Durability" below |
| `/var/lib/styrman/homes/<user>/base`               | The setup VM's home (ADR-064); `homes/<user>` itself is server-only, mounted into no guest |
| `/var/lib/styrman/projects/<user>/<uuid>`          | Workspaces (ADR-027, per-project storage) — source code; commit and push it, nothing else preserves it                         |
| `…/homes/<user>/p/<uuid>/.dockerd/<uuid>.img`      | Per-project Docker disk (sparse)                                                                                               |
| `…/homes/<user>/p/<uuid>/.styrman/logs/<uuid>/`    | Terminal transcripts                                                                                                           |
| `/var/lib/styrman/credentials/<user>/`             | The broker's credential store (ADR-032, 0600) — re-onboard via setup on a new host, nothing preserves this either              |
| `/var/lib/styrman/users/<user>/*.json`             | Project/terminal/profile metadata                                                                                              |
| `/var/lib/styrman/users.json`                      | Runtime-added accounts (admins are env-only)                                                                                   |
| `/var/lib/styrman/links.json`                      | Public/private preview share-link tokens (ADR-025/026)                                                                         |
| `/var/lib/styrman/settings.json`                   | Admin-set runtime tunables (idle timeout, docker disk size) overriding env defaults                                            |
| containers `styrman-<uuid>`, networks `sdb-<uuid>` | Disposable runtime state                                                                                                       |

Deleting a project WITHOUT "wipe files" keeps its workspace dir under
`/var/lib/styrman/projects/<user>/` (that's the point), but nothing in the
UI lists it anymore — reclaim disk by removing it manually if you never
re-import it.

- **Reboots**: containers restart themselves (`unless-stopped`; a project
  stopped from the UI or by the idle reaper stays stopped, its policy being
  `no` while stopped — v1.34.1, BUG-063); the server re-adopts from labels. **A lost host is not recoverable**: there is no
  backup layer (operator decision 2026-09-01) — durability is git, commit
  and push. Rebuilding a host is `host-setup.sh` + a fresh `deploy/.env`
  (see deploy.md's "Durability" section); anything not committed to a
  remote, and every onboarded credential, is gone with the old host.

## 5. Troubleshooting

| Symptom                                               | Cause / fix                                                                                                                                                                                                                                                                                  |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `invalid namespace type` starting any microVM         | Docker ≥29.5 time-namespaces vs Kata ≤3.31 — host-setup sets the daemon.json feature flag (ADR-009).                                                                                                                                                                                         |
| Site unreachable but server fine                      | Check DNS first — a "newly registered domain" blocklist (NextDNS et al.) sinkholed styrman.xyz once (`dig` for 0.0.0.0 answers).                                                                                                                                                             |
| dockerd inside a project uses `vfs`                   | Loop mount of `~/.dockerd/<uuid>.img` failed; see container logs (ADR-008 fallback).                                                                                                                                                                                                         |
| agy/claude ask to sign in despite a synced credential | A tool's TOKEN can sync while its per-project CLIENT STATE doesn't (e.g. claude's `hasCompletedOnboarding` — fixed for new logins since v1.5.6). If it's an OLDER project VM, recreate it to pick up a runtime-image fix; a running container keeps the rootfs it booted with until stopped. |
| `git commit` in a terminal fails "who are you"        | The user's git identity hasn't reached the broker yet — rerun `styrman-setup` (or `Settings → Credentials → Import`) so `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` get harvested, then open a fresh login shell (v1.7.1).                                                                        |
| Terminal shows "replaced"                             | Same terminal opened in another tab (close 4001) — one attach each, by design.                                                                                                                                                                                                               |
| First port-preview 307s or drops once                 | The server just joined that project's network; the redirect/retry lands.                                                                                                                                                                                                                     |
| Project won't start                                   | `docker logs styrman-<uuid>` — the entrypoint logs every step.                                                                                                                                                                                                                               |
| Local network switch disabled: "Not available on this server" | The root half is not installed, outdated (script below version 4 — see ADR-066), or has no prefix: on the host, `sudo bash deploy/firewall/install.sh` (from `/opt/styrman` at the deployed tag), then check `cat /run/styrman-firewall/status`, `systemctl status styrman-firewall.timer styrman-firewall.path` and `journalctl -u styrman-firewall`. "Applying…" that turns into an error after 90 s means the reconciler never ran: same three commands. |
| A project in LAN mode can't reach the host or its published ports | Not expected since ADR-067: LAN mode is full same-network parity by design, no opt-in required. Check the project's actual bridge — `ip -o link show \| grep lan-` — and that Docker's own inter-bridge DROP isn't the thing being hit by mistake; `sudo iptables -S STYRMAN-FWD`/`STYRMAN-IN` should show no rule matching a `lan-*` interface at all. |
| Toggling a project's network mode restarted it / lost something | Expected (ADR-067): applying the switch rebuilds the project's container — home and workspace (bind mounts) survive, packages replay from the apt manifest, but anything living only in the container's own filesystem is gone. |
| A project's network badge disagrees with its actual reach | Re-issue the same PUT (toggle the switch again in the UI): it compares the request against the container's LIVE bridge, not the stored record, so a drifted record repairs itself. If the reply says the mode was NOT changed, the container could not be inspected/stopped/started and nothing changed — check `docker logs styrman-<uuid>`. |

## 6. Development

```bash
go test ./...                                        # unit (stores, guards, routing)
bash .agents/docs/testing/styrman/integration.sh     # vs real Kata; check counts (and the auto-skip count on firewall-hardened hosts): .agents/docs/testing/styrman/README.md
bash web/e2e/run.sh                                  # browser e2e (Playwright, self-hosted; check counts: .agents/docs/testing/styrman/README.md)
cd web && npm run build                              # UI → dist/client (committed)
```

Layout: `cmd/styrman` (server) · `cmd/wsprobe` (WS test client) ·
`internal/{config, identity, userstore, project, conversation, orchestrator,
terminal, httpapi, files, history, gitrepo, links, credstore, credbroker}` · `image/` (runtime microVM
image) ·
`deploy/` (host-setup, compose, vendored caddy ingress, server Dockerfile) ·
`web/` (TanStack Start SPA). Conventions: `AGENTS.md`; non-negotiables:
`REQUIREMENTS.md` (R1–R13).

## 7. Roadmap

Deliberately EMPTY as of v1.14.0 (operator directive, 2026-08-30: "I want
nothing in todo and roadmap — start fresh with new things"). Everything the
roadmap used to list here — transcript rotation, an ops panel, sidebar git
badges, agy native session resume, backup automation — SHIPPED that release;
a few older ideas (a warm VM pool, a platform MCP server, auto-titles from
transcripts, a moby SDK migration) were explicitly RETIRED with reasons
instead. Full shipped/retired history: **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) and `.agents/docs/changes/`.
`TODO.md` is the inbox for new items going forward — add there, not here.

Lineage: combines `devbox` (Kata multi-tenant shell platform; ingress,
terminal protocol) and `argolis-devbox` (single-VM cloud shell). Full
history: `.agents/docs/plans/` and `.agents/docs/changes/`.
