# Architecture Decision Records

Append-only. Newest at the bottom.

---

## ADR-001: One project = one Kata microVM

**Date:** 2026-08-26 · **Status:** accepted

**Context:** styrman runs AI agents with broad permissions plus Docker-in-Docker per
project. The predecessor `devbox` used Kata (runtime `mvm`) per *user*; an initial
proposal to downgrade to plain runc was rejected by the operator — isolation is the point.

**Decision:** Every project container runs under Kata (`mvm`): own guest kernel,
`CapAdd: SYS_ADMIN, NET_ADMIN, NET_RAW` (not `--privileged`, which breaks Kata device
hotplug), `apparmor=unconfined`, explicit DNS (Kata lacks Docker's embedded DNS),
per-project bridge network. Config ported from devbox `internal/orchestrator/manager.go`.

**Consequences:** Hard blast-radius containment between projects; safe nested dockerd.
Costs: microVM cold-start latency (spike S5), virtiofs for the shared home (S2/S3).

---

## ADR-002: Terminal transport via Docker exec, no in-container server

**Date:** 2026-08-26 · **Status:** accepted (spike S1 passed 2026-08-26)

**Context:** devbox ran a shell-server binary inside each container (HTTP+WS on :38917),
which couples server updates to image rebuilds.

**Decision:** Project microVMs run no server. The styrman server uses Docker
`ContainerExecCreate` (TTY) to run `tmux new -A -s conv-<id> <agent>` and bridges the
stream to the browser WebSocket, reusing devbox's wire protocol (binary = PTY bytes,
JSON text = resize, close 4000 = shell exited, 4001 = replaced by newer connection).

**Consequences:** Runtime image is pure tooling; orchestration logic updates centrally.
Fallback if exec streaming under Kata disappoints (S1): port devbox's shell-server.

---

## ADR-003: Single shared persistent home across all microVMs

**Date:** 2026-08-26 · **Status:** accepted (spikes S2/S3 passed 2026-08-26; see ADR-008 for the S3 storage design)

**Context:** Core operator requirement: configure ssh/agent/cloud credentials once, ever.
devbox proved host-bind-mounted homes (its ADR-012), but 1:1 home:container.

**Decision:** `/var/lib/styrman/home` is virtiofs-mounted as `/home/<user>` in every
project microVM. Workspaces live at `~/projects/<name>`; per-daemon state is
project-scoped (`~/.dockerd/<name>`). Write discipline: a project writes only its own
workspace + dockerd dir; shared-home writes are human-driven.

**Consequences:** Credentials and the agent harness deploy once to the home, not per
image/container. Risk: concurrent virtiofs writers (S2); dockerd overlay2 on virtiofs
(S3) — fallback is per-project named volumes for `~/.dockerd`.

---

## ADR-004: Per-project Docker-in-Docker

**Date:** 2026-08-26 · **Status:** accepted

**Context:** Sharing the host `docker.sock` would let any project see/kill any other
project's containers — and styrman's own. Rejected by the operator: everything separated.

**Decision:** Each microVM runs its own `dockerd` (devbox entrypoint pattern: cgroup v2
nesting, loop devices, `data-root` in the home under a per-project path).

**Consequences:** Full Docker isolation per project; image caches are per-project
(accepted disk cost). Safe only because of ADR-001.

---

## ADR-005: Terminal-first — no embedded IDE

**Date:** 2026-08-26 · **Status:** accepted

**Context:** Both predecessors embedded code-server; the operator explicitly wants it
gone. The Antigravity 2.0 conversation-centric layout is the UI model.

**Decision:** The agent CLI in a terminal is the interface. UI = project/conversation
sidebar + terminal panes. Ceiling for file UX is a lightweight viewer/editor pane
(Phase 4). REQUIREMENTS.md R4 makes this non-negotiable to resist scope creep.

---

## ADR-006: Frontend is a TanStack Start SPA embedded in the Go binary

**Date:** 2026-08-26 · **Status:** accepted

**Context:** Predecessor UIs were no-build vanilla JS (devbox's a 1,187-line monolith
whose own TODO called for a refactor). Global stack standard is TanStack Start + TS +
Tailwind.

**Decision:** `web/` contains a TanStack Start SPA (static export, no SSR), embedded via
`go:embed`; xterm.js vendored locally. The server stays a single deployable binary.

---

## ADR-007: State from labels + filesystem, no database

**Date:** 2026-08-26 · **Status:** accepted

**Context:** devbox proved label-based adoption (`RecoverState`) across restarts.

**Decision:** Containers carry `styrman.*` labels; conversations are discovered from
tmux; the only file is `projects.json` (atomic tmp+rename, devbox userstore pattern)
for metadata that must outlive containers (agent choice, created, archived).

**Consequences:** Restart/reboot recovery is adoption, not replay. No migrations. If
requirements ever exceed this (search, history), revisit with SQLite.

---

## ADR-008: Per-project dockerd uses overlay2 on a loop-mounted ext4 image

**Date:** 2026-08-26 · **Status:** accepted (spike S3)

**Context:** overlay2 cannot mount on virtiofs (`invalid argument`); devbox worked
around this with `storage-driver: vfs`, which copies every layer fully (slow, bloated).

**Decision:** Each project's dockerd data-root is a sparse ext4 image at
`~/.dockerd/<project>.img` (default 20 G, configurable), loop-mounted inside the guest
by the entrypoint, with overlay2 on top. Prereqs (verified): remount `/sys/fs/cgroup`
rw + cgroup-v2 nesting, loop-device mknod. `vfs` stays available as a config fallback.

**Consequences:** Real overlay2 performance and bounded per-project Docker disk; the
image file persists on the shared home and travels with backups. Growing an image later
= `truncate` + `resize2fs`.

---

## ADR-009: Host prerequisite — Docker time-namespaces disabled

**Date:** 2026-08-26 · **Status:** accepted (spike S0)

**Context:** Docker ≥ 29.5.0 enables private time namespaces by default
(moby/moby#52326); the Kata ≤ 3.31 guest agent rejects the `time` namespace type →
every `--runtime mvm` container fails with `invalid namespace type`
(kata-containers#13080).

**Decision:** `/etc/docker/daemon.json` MUST carry
`"features": {"time-namespaces": false}` on styrman hosts. Owned by
`deploy/host-setup.sh`; applied manually on this host 2026-08-26 (backup:
`daemon.json.bak-20260826`). Revisit when Kata supports time namespaces.

---

## ADR-010: Reproducible host installation is a first-class deliverable

**Date:** 2026-08-26 · **Status:** accepted (operator requirement)

**Context:** Operator directive: the entire project must be easy to reproduce on
another baremetal machine first, or on a GCP VM. The current host accumulated its
Docker/Kata/gVisor setup from devbox scripts plus manual fixes — that history must not
be load-bearing.

**Decision:** `deploy/host-setup.sh` provisions a styrman host from a fresh Debian 13
(baremetal or GCP VM with nested virt): Docker CE, Kata (runtime `mvm`), daemon.json
(incl. ADR-009 flag), host tuning (KSM/THP, sysctls, nested-KVM modprobe), state dirs,
compose + systemd unit. Idempotent, no reliance on devbox leftovers. This machine gets
re-run through it as validation. GCP VM path requires a machine type with nested
virtualization enabled.

**Consequences:** REQUIREMENTS.md R11. The manual S0 fix becomes code. Disaster
recovery = fresh install + restore `/var/lib/styrman`.

---

## ADR-011: Conversations are durable agent sessions; tmux is only the live view

**Date:** 2026-08-26 · **Status:** accepted (design direction for Phase 2; operator feedback)

**Context:** Phase 1 equated "conversation" with a tmux session. Operator
feedback after first use: that model is wrong — you can quit claude/agy inside
one and the "conversation" lives on as a shell; killing tmux loses nothing the
user cares about; and the thing they actually want per conversation is its
**history**, like Antigravity's conversation list.

**Decision:** Redefine for Phase 2:
- A **conversation** is a durable record bound to the agent's own session
  (Claude Code session id under `~/.claude/projects/<dir>/`; agy's equivalent
  under `~/.gemini`), with a title and timestamps, stored per project
  (`conversations.json`, userstore pattern — the one addition to ADR-007).
- Opening a conversation attaches (or recreates) its tmux session; if the
  pane's agent exited or the container was restarted, styrman relaunches the
  agent with its native resume (`claude --resume <id>`; agy equivalent) — so
  history survives everything, which is the actual requirement.
- Plain shells are **terminals**, not conversations (Antigravity's right
  rail); they are ephemeral and unlisted in history.
- Deleting a conversation removes the record + tmux session; the agent's
  transcript files remain in the shared home unless the user purges them.

**Consequences:** Phase 2 UI lists conversation history per project even for
stopped projects. Requires per-agent session-id discovery (claude: parse
`~/.claude/projects`; agy: investigate its session store). Phase 1's
tmux-only conversations remain the transport layer underneath.

---

## ADR-012: UI built on TanStack Router + Vite (static SPA), not full TanStack Start

**Date:** 2026-08-26 · **Status:** accepted

**Context:** The global standard says TanStack Start for every frontend.
styrman's UI is a static SPA embedded in the Go binary (ADR-006): the Go
server is the only backend, there is no SSR, no server functions, no Node at
runtime — Start's entire value-add is unusable here, while its Nitro/vinxi
build machinery adds weight and a second server model.

**Decision:** `web/` is Vite + React + TypeScript + Tailwind v4 +
**@tanstack/react-router** (code-based routes: `/`, `/p/$project/$conv`,
`/setup`), built to `dist/` and embedded via `go:embed` with an index.html
fallback for deep links. `dist/` is committed so `go build` needs no Node.
Documented deviation #2: dark theme is the default (terminal product),
light supported via `data-theme`.

**Consequences:** Single deployable binary stays true. If styrman ever needs
SSR or server functions, migrating Router → Start is the designed path.

---

## ADR-013: UI migrated to full TanStack Start (SPA mode) — supersedes ADR-012

**Date:** 2026-08-26 · **Status:** accepted (operator decision)

**Context:** ADR-012 deviated from the REQUIREMENTS.md "TanStack Start"
constraint with Router + Vite. The reviewer flagged that REQUIREMENTS.md
changes need explicit operator approval; offered the choice, the operator
chose migration over amendment.

**Decision:** `web/` is TanStack Start (v1.168+, Vite plugin) in **SPA mode**
(`spa.enabled`): file-based routes under `src/routes/` (`__root.tsx` renders
the document, index / `p.$project.$conv` / `setup`), generated route tree,
prerendered static shell. The build emits `dist/client/` (committed, embedded
via go:embed; the Go server falls back to `_shell.html` for deep links) and
`dist/server/` (Start's Node handler — unused, gitignored). Still no Node at
runtime; the single-binary deploy is unchanged.

**Consequences:** REQUIREMENTS.md holds as written. SSR/server functions are
now one config flip away if ever wanted. Build order matters: `vite build`
generates `routeTree.gen.ts`, so typecheck runs after it.

---

## ADR-014: Conversations are login shells; agents launched manually

**Date:** 2026-08-26 · **Status:** accepted (operator decision)

**Context:** Operator simplification: both agents ship in the image, so
choosing one per project at creation adds friction without value, and a
conversation auto-running an agent surprises more than it helps.

**Decision:** No agent picker; `Project.Agent` is optional/unused (kept for
compat). Every conversation pane is `bash -l` — run `claude` or `agy` (or
anything) inside it; the MOTD hints at styrman-setup until signed in.
Conversations remain durable records (titles, sidebar, listed when stopped);
terminals remain the ephemeral right rail. The claude session-pinning
machinery (ADR-011 resume) is parked: records still carry a UUID, and
re-enabling auto-launch/resume is a one-function change in the bridge
(`git show 87541d8:internal/terminal/bridge.go` has the last version).

**Consequences:** Simpler mental model and UI; "history per conversation"
now means tmux scrollback + agent-native history (`claude --resume` by hand)
until a future decision revives pinning.

---

## ADR-015: Project network isolation — per-project bridges, server cannot route

**Date:** 2026-08-26 · **Status:** accepted (operator directive; R12)

**Context:** Operator requirement: project microVMs must not access each
other at all. Projects carry NET_ADMIN/NET_RAW (for their DinD), and the
containerized server is attached to every project's bridge (for the
port-preview proxy) — so "could a project route to another via the multi-homed
server?" is a real question.

**Decision:** 1 project = 1 bridge network (`sdb-<name>`), containing only
that project's microVM and the styrman server. Docker's inter-bridge
isolation blocks project↔project directly. Defense in depth: the server runs
with `net.ipv4.ip_forward=0` (compose sysctl) — it originates connections to
each project (all the proxy needs) but is structurally incapable of forwarding
between the networks it bridges. Host networking is reserved for the Caddy
ingress (which needs to see real client IPs and reach every bridge); projects
never use it.

**Consequences:** Verified empirically: two projects cannot ping/connect to
each other even when one adds an explicit route through the server's IP. The
cost is the server must `NetworkConnect` itself to each project network on
start/attach and `NetworkDisconnect` before removal.

**Verification:** `.agents/docs/testing/styrman/` — the isolation checks in
integration.sh (added 2026-08-26).

---

## ADR-016: One concept — terminals. The side panel is context, not sessions

**Date:** 2026-08-26 · **Status:** accepted (operator design review)

**Context:** Operator: "a conversation is actually a terminal correct? Why do
we have terminals on the side-panel?" Correct — since ADR-014 removed agent
auto-launch, a "conversation" and a right-rail "terminal" were the same
thing (a login shell in the project's microVM) presented in two places, with
one durable and one ephemeral for no reason.

**Decision:** A project has **terminals**: durable records (title, id,
sidebar entry, listed even when the project is stopped), each backed by one
tmux session. Run several to work with several agents at once — that is what
the operator wanted the right-rail for. The ephemeral shell concept and its
endpoints are removed; `/api/projects/{p}/conversations` becomes
`/api/projects/{p}/terminals`; the WS path parameter is `{term}`.

The **side panel** now holds project *context*, not sessions: Files (project
workspace + shared home), Ports, History (transcripts), Diff.

Internally the Go package stays `conversation` and the tmux prefix stays
`conv-` so existing sessions and stored records keep working; the doc
comments carry the terminal vocabulary.

**Consequences:** One mental model, one place to look. Transcripts are per
terminal (`conv-<id>.log`), so "history per terminal" is naturally scoped.

---

## ADR-017: Terminal transcripts via tmux pipe-pane into the shared home

**Date:** 2026-08-26 · **Status:** accepted (operator request)

**Context:** Operator wants "a file that logs everything claude or agy or my
self is doing in the terminals" — a durable record of what happened in a
project, independent of scrollback.

**Decision:** The runtime image sets a tmux hook (`after-new-session` /
`after-new-window`) that pipes every pane to
`~/.styrman/logs/<project>/<session>.log`. The path is project-specific but
`~/.tmux.conf` is shared across projects, so the entrypoint writes
`/etc/styrman/tmux-project.conf` (per container) which the shared config
sources. The server reads these host-side and strips ANSI escapes for the
History tab; `GET /api/projects/{p}/history[/{id}]`.

**Consequences:** Transcripts live in the shared home, so they survive
container rebuilds, idle stops, and project deletion (deliberately — history
outlives the terminal). They capture *everything*, including anything typed
into a terminal: treat the home's `0700` permissions as the boundary and
avoid pasting secrets you would not want recorded. No rotation yet
(TODO).

---

## ADR-018: File explorer reads the host mount, not the guest

**Date:** 2026-08-26 · **Status:** accepted

**Context:** The UI needs to browse project workspaces and the shared home.

**Decision:** The server (which already mounts `/var/lib/styrman` at the
host path) reads and writes these files directly via `internal/files`, with
traversal and symlink-escape checks against a fixed root. No exec, no
running container required.

**Consequences:** Browsing and editing work while a project is stopped, and
cost nothing per keystroke. The explorer's two scopes are the project
workspace and the shared home (R3 — where every credential lives).

---

## ADR-019: Multi-tenancy — per-user homes, projects, and setup

**Date:** 2026-08-26 · **Status:** accepted (operator directive; amends R3)

**Context:** Operator: another account with access "should have its own
disk", and "not my projects" — the admin adds users, each isolated.

**Decision:** Identity flows from the ingress header (or the configured
default on the loopback dev listener) through `internal/identity`
(email → username slug, devbox's derivation). Everything is namespaced by
user: home `/var/lib/styrman/homes/<user>` (created with a skeleton on first
sight; credentials via that user's own styrman-setup), metadata stores
`/var/lib/styrman/users/<user>/{projects,conversations}.json`, containers
`styrman-<user>-<project>`, networks `sdb-<user>-<project>`, per-user hidden
setup VMs, per-user transcripts. In-guest username = the slug; uid/gid stays
the host operator's (isolation is at the mount + microVM level, not uid).
Admins = env-provided emails (`STYRMAN_ALLOWED_EMAILS`); only they manage
the user list (Settings → Admin). A startup migration moves the legacy
single-home layout into the default user's namespace and removes
pre-multitenancy containers (data persists; containers are disposable).

**Consequences:** A second user signs in and finds an empty home, their own
setup banner, their own projects — and no path to anyone else's. The state
dir (all homes + metadata) is the single backup target. Legacy migration ran
2026-08-26 (admin home → homes/ostling-simon).

---

## ADR-020: Project identity is a UUID; names are friendly labels

**Date:** 2026-08-26 · **Status:** accepted (operator directive)

**Context:** Operator: "we need to start using uuids, the same name can be
used many times… all projects will get a uuid and a friendly name."

**Decision:** `Project{ID(uuid), Name, Dir}`; everything runtime keys on the
UUID: containers `styrman-<uuid>`, networks `sdb-<uuid>`, docker disks
`~/.dockerd/<uuid>.img`, transcripts `~/.styrman/logs/<uuid>/`, API paths
and UI routes. Names are renamable (`PATCH /api/projects/{id}`) and reusable
after deletion; among *current* projects a name must be unique because the
workspace dir (`Dir`, frozen at creation) is name-based. Stores migrate
name-keyed records by assigning UUIDs on load; previous-scheme containers
are swept via the Docker SDK at boot (CleanupLegacy — the containerized
server has no docker CLI).

**Consequences:** No naming collisions ever again across users or history;
renames are free. URLs show UUIDs (stable, shareable) with names in the UI.

---

## ADR-021: Async project creation; the server never reconfigures its own
network on a request path

**Date:** 2026-08-26 · **Status:** accepted (bug-driven)

**Context:** Browser POSTs to create projects failed intermittently with
connection-refused while curl succeeded. Root cause: `EnsureSelfAttached`
(joining the server's own container to the new project network for the
port-preview proxy) reconfigures the server's network namespace, killing
in-flight HTTP connections — including the very request that triggered it.

**Decision:** (a) `POST /api/projects` returns 201 `state:"creating"`
immediately and boots the microVM in a background goroutine (terminal
attach waits on the per-project lock + readiness). (b) Self-attach happens
only in the background (post-create, post-start, at boot for adopted
projects); when the preview proxy must attach on demand it responds 307 to
the same URL so the browser retries on a fresh connection instead of
dialing through a doomed one.

**Consequences:** Creation UX is instant; the flake class is gone (verified
by repeated e2e runs). First-ever preview of a project may redirect once.

---

## ADR-022: Git as the substrate (R13)

**Date:** 2026-08-27 · **Status:** accepted (operator directive)

**Context:** Operator: "You always need git, at least the local one. This
can power history and diffs." Project workspaces up to this point were git
repos only incidentally (if the user happened to `git init`, or if the
project was created by cloning). There was no guaranteed local history, and
no undo mechanism for in-progress work beyond the file explorer's raw reads
(ADR-018) and terminal transcripts (ADR-017), neither of which is a diffable
snapshot.

**Decision:** Every project workspace IS a git repository, no exceptions.

- **Creation:** empty projects get server-side `git init -b main` run
  synchronously at creation time, before the `201` is returned (unlike the
  rest of project creation, which is async per ADR-021 — this step is cheap
  and local, no network, no microVM boot dependency). Clone-on-create runs
  in-guest instead (needs the user's ssh key/agent and gitconfig, which only
  exist inside the guest per ADR-003), with `StrictHostKeyChecking=accept-new`,
  and falls back to an empty `git init` repo if the clone fails so the
  no-exceptions invariant still holds.
- **Boot-time sweep:** `ensureRepos` in `cmd/styrman/main.go` walks existing
  workspaces at startup and `git init`s any that are repo-less (adopted
  pre-R13 projects, or projects whose init failed for some reason).
- **Checkpoints** are whole-tree snapshots (tracked + untracked files;
  `.gitignore`d files excluded, matching normal git semantics) taken via a
  **private index file** (`.git/styrman-index`, `GIT_INDEX_FILE` override) so
  they never touch the user's own staging area. The snapshot is committed
  with `commit-tree` under a neutral `styrman` author/committer identity, to
  `refs/styrman/checkpoints/<nanosecond-utc-stamp>-<sha8>` — never a branch.
  These refs are never pushed: `git push` only sends `refs/heads/*` and
  `refs/tags/*` by default, so the checkpoint namespace is invisible to the
  user's own git remotes without extra effort. Pruned to the newest 40.
- **Triggers:** manual button, project stop, terminal pause/close/close-all,
  and pre-restore (see below).
- **Restore** = `read-tree --reset <ref>` + `checkout-index -f -a` + `clean
  -fd` (so files not tracked by the checkpoint but present as ignored
  survive — restore only touches what git tracked at snapshot time). Every
  restore first snapshots the *current* state (via the same checkpoint path)
  before restoring, so a restore is itself undoable. Documented side effect:
  after a restore the user's own index equals the checkpoint tree, so `git
  diff --cached` against `HEAD` will show the restored files as staged —
  this is a consequence of using git's index/worktree primitives for the
  restore and is called out in the UI copy rather than worked around.
- **Alternatives considered and rejected:** a shadow branch (pollutes
  `git log`/`git push` for the user's real branches); `git stash` (stack
  semantics don't fit "N labeled restore points", and stash is user-visible
  and can be popped/dropped by the user's own git usage, making it fragile
  as an internal mechanism); a separate backup directory outside `.git`
  (duplicates what git already does, no content-addressed dedup, doubles
  disk cost).
- The pre-existing `gitInit` config flag is now dead code — every project
  always gets git regardless of its value. Accepted as-is (ignored, not
  removed) rather than spending a cleanup pass on it now.

**Consequences:** Every project has real git history from creation, usable
for diffs and manual commits by the user as normal, plus a parallel,
git-native undo/checkpoint system that never interferes with the user's own
branches, stash, or remotes. REQUIREMENTS.md gained R13 (operator-approved)
to make this non-negotiable. Follow-up gap: the idle-reaper does not yet
auto-snapshot before stopping a project (see TODO.md) because the reaper
doesn't currently have the workspace `Dir` plumbed into it.

## ADR-023: Clipboard history is session-scoped, not persisted (2026-08-27)

**Context.** The terminal copies on select (mouse-up), and a Clips side-panel
tab shows what styrman copied — genuinely useful ("what was that URL from ten
minutes ago"). But copy-on-select means selecting an OAuth code or token
during first-run setup lands it in that history. The initial implementation
used `localStorage` (persists indefinitely, survives sign-out).

**Decision.** Clipboard history lives in `sessionStorage` with a 1-hour TTL,
capped at 30 entries, lazily initialized (never touched at module import — the
SPA is prerendered). It clears when the tab closes, never syncs across
tabs/devices, and is never sent to the server. It is a short-lived
convenience, deliberately not durable (R10).

**Alternatives.** `localStorage` (rejected: durable secret exposure);
in-memory only (rejected: lost on reload, too lossy to be useful); redacting
token-shaped strings (rejected: unreliable, and the link-bar already gives a
safe path for auth URLs). Session scope + TTL is the balance.

---

## ADR-024: Per-port preview sharing, not project-wide (2026-08-27)

**Context.** Sharing a project's live preview with a guest (a client, a
teammate without a styrman account) needs a real access-control decision:
what exactly does the shared link expose? A project VM can have several
listening ports at once (dev server, API, admin panel, ad hoc debug tools).

**Decision.** A project owner grants specific Google emails access to ONE
exposed port's preview: `project.Project.SharedPorts map[int][]string`,
keyed per-port, not project-wide. `previewAllowed` (internal/httpapi/shares.go)
gates the owner branch on an *authorized* user whose slug matches the
owner's store (`u.Authorized && ownerUser == u.Username`) — never a guest,
whose derived username could otherwise collide with an owner's slug and
grant them the owner's projects (tracked as finding B2 in the pre-1.0.0
review). The guest branch is a case-insensitive email match against
`SharedPorts[port]`. Deploying this required flipping the styrman ingress
host from admin-only to `ModeAuthenticated` (`deploy/docker-compose.yml`) so
any Google account can sign in and reach the server at all; the server's own
`authGuard` (internal/httpapi/auth.go) is the real gate — an authenticated
but non-authorized email is 403'd everywhere except `isPreviewPath` routes
(`/proxy/…`, `/p/{project}/{term}/port/{port}/…`).

**Consequences.** Least privilege: a shared guest reaches exactly the one
port shared with them, never a port-scan surface across the whole VM. Admin
rights still come only from `STYRMAN_ALLOWED_EMAILS` server-side, never from
merely passing the ingress. Cost: any Google account can now reach the
login page (though it gets 403 on the SPA shell and every non-preview route
unless it's an authorized styrman user).

Source: internal/httpapi/shares.go, internal/httpapi/auth.go,
internal/project/store.go, deploy/docker-compose.yml.

---

## ADR-025: Public TTL preview links via a wildcard subdomain (2026-08-27)

**Context.** Email-based sharing (ADR-024) still requires the guest to have
a Google account and sign in. The pre-1.0.0 backlog wanted a lower-friction
option: a link anyone holding it can open, no login, that expires on its
own. The design also had to solve the SPA's cross-origin isolation problem
for previewed apps without a CSP-sandbox workaround.

**Decision.** Public, expiring capability links to one port, served at
`https://<token>.<PREVIEW_DOMAIN>/`, backed by a stateful token store
(`internal/links`) rather than a signed blob — stateful means a link can be
revoked and listed, which a signed blob can't (no revocation before its own
expiry). The 128-bit hex token is the capability: `GET /link/{token}/…`
(`internal/httpapi/links_api.go`) is exempt from `authGuard` and fully
public; a bogus or expired token 404s with no distinguishing error (no
oracle). TTL is bounded to one of 1h/24h/7d and every mint/revoke is
audit-logged. Giving each token its own subdomain (rather than a path under
the main host) means each preview gets a clean origin/root — the previewed
app's root-relative asset paths work unchanged, and it's naturally
cross-origin-isolated from the SPA.

Wiring: the ingress adds a `*.PREVIEW_DOMAIN` `ModePublic` wildcard site
block (Host preserved so the server can read the token subdomain,
client-supplied identity headers stripped, ingress token injected) plus
wildcard DNS (`EnsureDNS`) and an auto-issued wildcard cert
(`deploy/caddy/ingress/generator.go`, `cloudflare.go`). The server's
`previewHostRewrite` (internal/httpapi/server.go), run outermost before
`authGuard`, maps `<token>.PREVIEW_BASE` → `/link/{token}/…`. One compose
var, `STYRMAN_PREVIEW_DOMAIN`, drives both the ingress `PREVIEW_DOMAIN` and
the server's `STYRMAN_PREVIEW_BASE`.

**Consequences.** A public wildcard host now exists on the ingress whenever
`STYRMAN_PREVIEW_DOMAIN` is set (empty = feature fully off, the default).
Revocation and expiry deny immediately (`links.Store.Get` filters expired
entries). Shipped and verified live 2026-08-27 against
`preview.styrman.xyz`.

Source: internal/links/store.go, internal/httpapi/links_api.go,
internal/httpapi/server.go (previewHostRewrite), deploy/docker-compose.yml,
deploy/caddy/ingress/generator.go, deploy/caddy/ingress/cloudflare.go.

## ADR-026: Private share links on an OAuth-gated wildcard (2026-08-28)

**Context.** Per-port email shares (ADR-024) were served same-origin with the
SPA at `/p/…`, which forces a CSP sandbox (N22) that breaks stateful apps for
guests — the exact limitation public links (ADR-025) don't have, since each
public token gets its own subdomain origin. The operator wanted email shares
to get the same own-origin treatment, with self-describing hostnames:
`*.public.<app domain>` for anyone-with-the-URL links, `*.private.<app
domain>` for email-gated ones.

**Decision.** Two sibling wildcards, one per auth model — a single wildcard
cannot serve both because the ingress decides authentication per host block,
before the server can inspect the token. `*.PRIVATE_DOMAIN` is a second
generated site block (`writePrivateWildcard`) that REQUIRES Google OAuth: it
reuses the target app's authorization policy (the caddy-security JWT cookie
is scoped to the base domain, so a session from any styrman host is valid on
the wildcard — SSO for free) and injects the verified identity headers plus
the ingress token, preserving Host.

Sharing a port mints a stable, NON-EXPIRING `kind=private` token in the same
`internal/links` store (idempotent — editing the email list never rotates the
URL; emptying it deletes the link). `previewHostRewrite` maps
`<token>.PRIVATE_BASE` → `/shared/{token}/…`, where `authGuard` resolves the
ingress identity (non-styrman Google users are allowed through to this route
only) and `serveShared` re-checks the email against the port's CURRENT share
list on every request — revoking an email cuts access instantly, and the
token itself never grants anything. Kinds are locked to their routes: a
private token 404s on the no-login `/link` route (else sharing would mint a
public capability) and a public token 404s on `/shared`. The per-project link
cap counts each kind separately. No CSP sandbox on the private origin — same
isolation rationale as ADR-025.

**Consequences.** Email-share guests now get full-fidelity previews (cookies,
localStorage, same-origin fetches) instead of the sandboxed `/p/` view, which
remains for the owner's in-SPA preview. A second wildcard DNS record + cert
exist when `STYRMAN_PRIVATE_DOMAIN` is set (empty = off). The share list
stays the single source of authorization; the private link is only an
address. Shipped and verified live 2026-08-28 against
`private.crew.styrman.xyz`.

Source: internal/links/store.go (kinds, EnsurePrivate), internal/httpapi/
links_api.go (serveShared, kind locks), internal/httpapi/shares.go (mint/
revoke lifecycle), internal/httpapi/server.go (previewHostRewrite),
deploy/caddy/ingress/generator.go (writePrivateWildcard), deploy/
docker-compose.yml, web/src/components/SidePanel.tsx.

---

## ADR-027: Per-project workspace storage outside the shared home

**Date:** 2026-08-28 · **Status:** accepted (operator Q5, v1.3 plan §4b)

**Context:** ADR-003 put workspaces at `~/projects/<name>` inside the one
shared home, separated by convention only ("write discipline"). The operator
asked for real per-project storage; the review of the files API showed the
convention also weakened confinement (project scope could reach the whole
home via same-uid symlinks).

**Decision:** Every project's working tree lives in its own host dir
`StateDir/projects/<user>/<uuid>`, bind-mounted into the microVM at
`~/projects/<dir>` (in-guest paths unchanged). The home stays shared (R3 —
credentials once, ever). The server addresses workspaces ONLY via
`cfg.ProjectWorkspace`; the files API's project scope roots its os.Root at
the workspace itself (strictly better confinement); git/checkpoints use the
path directly (its parent is never mounted into any guest, so no
containment guard is needed). A boot sweep migrates pre-ADR-027 workspaces
by same-filesystem rename; `~/.dockerd/<uuid>.img` stays in the home
(ADR-008 unchanged).

**Consequences:** Clean per-project deletion (one RemoveAll), per-project
quotas/backup become possible, project scope can no longer see the home.
Home-scope file browsing shows `~/projects/<dir>` as empty mount points —
workspaces are browsed via Project scope.

---

## ADR-028: apt persistence via manifest replay

**Date:** 2026-08-28 · **Status:** accepted (operator Q7, v1.3 plan §4b)

**Context:** `apt install` writes the container rootfs: survives stop/start,
dies on container removal (delete, guest-username change, image upgrade).
The operator wants per-project packages that stick until project deletion.

**Decision:** Manifest + replay, not image mutation: at graceful shutdown
the entrypoint records the delta between `dpkg-query` and the image's
`/etc/styrman-base-packages` snapshot to `~/.styrman/apt/<uuid>.list`; on
boot, a non-empty manifest is replayed by `apt-get install` IN THE
BACKGROUND after the ready marker (the boot SLA never waits on apt), logged
to `~/.styrman/apt/<uuid>.log`. Wiped with the project. Alternatives
rejected: overlay images over /usr (complex, ADR-008-style loop devices for
little gain) and per-project derived images (couples projects to image
builds).

**Consequences:** Packages reappear minutes after a recreate rather than
instantly; a hard kill loses deltas since the last graceful stop. Both
accepted — the manifest is also human-readable and agent-editable.

---

## ADR-029: Project→host network isolation

**Date:** 2026-08-28 · **Status:** accepted (operator directive, v1.3.1)

**Context:** R12 required project microVMs not to reach one another, enforced
by per-project bridges + Docker's inter-bridge DROP (the FORWARD path) and
the server container's `ip_forward=0`. But a project's default gateway is the
host, and nothing guarded the INPUT path — traffic addressed to the host
itself. Live verification from a project VM reached the host's SSH (:22) and
Caddy (:443). The operator's model is explicit: **a project may reach the
internet and nothing else — not another project, not the host ("Dave").**

**Decision:** Drop traffic arriving on any project bridge (`sdb-+`) that is
addressed to the host, via `iptables -I INPUT -i sdb-+ -j DROP`. Installed by
`deploy/host-setup.sh` as a systemd oneshot (`styrman-firewall.service`,
After=docker.service) so it survives reboots and Docker restarts (Docker
rebuilds its own chains but never touches INPUT). Internet egress is
unaffected (routed/NAT'd in FORWARD, never INPUT); the port-preview proxy is
unaffected (server↔project is same-bridge L2, not host INPUT).

**Consequences:** Projects are now egress-internet-only at the network layer.
The project can still TCP-connect to the styrman **server container** on its
own bridge (same-bridge L2), but every API request requires the secret
`X-Styrman-Ingress` header it does not have (→403), so that residual path is
already neutralized at L7 (defense-in-depth, not a new hole). Blocking it
too would need fragile ebtables/physdev L2 filtering for no real gain —
declined. Note the wildcard `sdb-+` covers all present and future project
bridges with one rule.

---

## ADR-030: Per-project homes + credential broker (share only credentials)

**Date:** 2026-08-28 · **Status:** accepted (operator approval, "plan then
do → Approve. Build."). Full design:
`.agents/docs/plans/2026-08-28-credential-service.md`.

**Context:** ADR-003's single shared home made credentials trivial but also
shared every tool's `$HOME` state (Go/npm/pip caches, global installs,
history) across a user's projects — "separate but not separate." The
operator wants projects TRULY individual, with ONLY credentials shared and
kept current (no re-auth), via a credential SERVICE, not a shared mount.

**Decision:** Three layers.
1. **Base image carries identical config + harness** for every project
   (seeded into each home from an image skeleton), replacing the
   shared-home/host-side harness deploy (**supersedes R5's mechanism**).
2. **Per-project home** volume — individual by construction; nothing shared
   at the FS layer (**supersedes R3's "one shared home"**).
3. **Credential broker** owns the single live copy of each credential and
   delivers it per-tool, refresh propagating to all. Research (plan §4): 7/8
   tools (ssh agent-socket; git/docker/gcloud credential helpers; gh/npm env
   tokens; claude apiKeyHelper) keep the secret OUT of the project; agy is
   keyring-seeded (gnome-keyring, no working env path — issue #632).
   Broker shape: a credential-only endpoint on each project's OWN bridge
   (identity = network position; composes with ADR-029), plus push-by-exec
   for bootstrap + agy keyring seeding.

**Phasing:** P1 = image config/harness + per-project homes + broker
delivering current credentials (the operator's ask: individual + shared +
no re-auth). P2 = harden to short-lived/dynamic tokens (secret never
readable in the VM). Build order (safe→risky): (Inc1) harness into image;
(Inc2) broker + adapters; (Inc3) per-project homes + the guarded, backed-up
migration of the existing home (highest care — credentials must never be
lost).

**Consequences:** True per-project isolation; auth once, current everywhere.
Costs: no cache sharing (accepted); a real credential subsystem; a one-time
home migration. agy remains the weakest link until upstream adds an env path.

---

## ADR-031: Per-project homes (clean cut from the shared home)

**Date:** 2026-08-28 · **Status:** accepted (operator: "prototype... don't
need to save old data, I can reregister creds").

**Context:** ADR-003's single shared home (revised by ADR-030) meant every
tool's $HOME state (Go/npm caches, global installs, history, agent auth)
was shared across a user's projects — "separate but not separate." The goal:
TRULY individual projects.

**Decision:** Each project gets its OWN home at
`HomesDir/<user>/p/<uuid>`, mounted as `$HOME` in its microVM. Homes start
empty and the base image's entrypoint seeds them (dotfiles from
`image/skel/`, harness from `image/harness/` — R5/ADR-030), so every project
gets identical defaults with nothing shared at the FS layer. The per-user
`HomesDir/<user>` becomes just the setup VM's home + the parent of the
per-project homes. The workspace stays separate (ADR-027). The ONLY thing
shared across projects is credentials, via the broker (ADR-030). The server
no longer creates or seeds homes (dropped `ensureHome` + the `dotfiles`
package); it creates the empty per-project home dir at project creation, the
image seeds it. Files "Home" scope and transcripts are now per-project
(`/api/projects/{id}/home/*`, `ProjectHome/.styrman/logs`). Clean cut — no
migration; the operator re-registers credentials (prototype).

**Consequences:** Projects are genuinely individual; no cache/state/auth
leak between them. Cost: re-auth per tool until the broker adapters cover it
(git done; ssh/gcloud/docker/gh/npm/claude/agy pending), and no cache
sharing (accepted). Revises R3 further (per-project home, not shared).

---

## ADR-032: Credential broker P1 complete — env + file tiers, pull-on-login

**Date:** 2026-08-28 · **Status:** accepted (operator: "finish everything...
test everything"). Completes the ADR-030 broker beyond the git slice.

**Context:** ADR-031 made homes per-project, so nothing is shared at the FS
layer — the ONE thing that must still be shared and current across a user's
projects is credentials. ADR-030 shipped the git tier (a credential helper
that pulls per-op from the broker on the project's bridge). The remaining
tools (gh, npm, claude, gcloud, ssh, agy) needed delivery too.

**Decision:** One unified *pull-on-login* mechanism, three tiers, all served
by the same bridge-only broker (caller identified by network position, ADR-030):

- **git** (helper tier, ADR-030): `GET /git?host=` → git credential helper.
  The secret is never stored in the project.
- **env-token tier**: `GET /env` returns a `{KEY: value}` map (GH_TOKEN,
  NPM_TOKEN, CLAUDE_*, any). A login-shell snippet
  (`/etc/profile.d/zz-styrman-credentials.sh`) exports each into every shell.
- **file-injection tier**: `GET /files` returns `[{path, mode, data}]`
  (gcloud ADC, ssh private key). The same snippet writes each into the home
  at its mode. Paths are validated home-relative (no absolute, no `..`), both
  in the store and in the snippet.

The store (`credstore`) generalizes from git-only to three kinds
(`git-<host>.json`, `env-<KEY>.json`, `file-<id>.json`, all 0600, atomic
write). The broker (`credbroker`) serves the CURRENT copy, so a refresh in
the store propagates to every project on its next login shell — sign in
once, no re-auth, new projects included. Onboarding:
`POST /api/settings/credentials/import {project}` extracts every credential
it can from a reference RUNNING project in one pass (gh token from the
keyring; claude `~/.claude/.credentials.json`; gcloud ADC; the first ssh
private key; an npm authToken from `~/.npmrc`), each best-effort and reported
per tool. `POST .../token {key,value}` stores a pasted env token;
`GET .../credentials` lists value-free; `DELETE .../{kind}/{name}` removes one.
A Settings "Credentials" panel drives all of it.

**Tiers, honestly:** git/docker→registry never store the secret in the VM.
gh/npm/claude/gcloud/ssh materialize a token or file inside the VM — accepted
for P1 (the operator's ask is "shared + current + no re-auth", which all
tiers meet; "secret never readable in the VM" is the P2 goal). ssh cannot use
the agent-socket tier because Kata blocks unix-socket forwarding across the
microVM boundary (the reason the broker is network-based at all), so the key
is file-injected — P2 could add a network ssh-agent proxy. agy stores its
OAuth only in the gnome-keyring with no verified service/account mapping
(issue #632); it is reported "unsupported" rather than half-wired — the one
tool still needing per-project sign-in.

**Consequences:** Every tool except agy is shared and kept current across a
user's projects via one mechanism; onboarding is one import from a project
you signed into. Cost: the env/file tiers place a live token/file in the VM
(P2 hardens this); a login shell makes two fast broker calls (bridge-local,
5 s timeout, quiet on failure). Proven live: a synthetic env cred exported
and a file cred materialized (content + mode) in a fresh login shell of a
real Kata project, authed purely by bridge position; import returns an
honest per-tool report; store round-trips and value-free listing verified by
unit tests + the live server. An adversarial review fixed one cross-tenant
leak (the IP→owner cache now re-verifies the container holds the source IP on
every hit, so a reused subnet/IP after a destroy can't serve another user's
credentials) and hardened env values against control-character injection.

---

## ADR-032a: agy joins the broker — the keyring tier (pull-on-login)

**Date:** 2026-08-28 · **Status:** accepted. Amends ADR-032, which shipped
agy as "unsupported — keyring mapping unverified".

**Context:** The operator asked "agy pull on login?". Re-investigation
settled the two unknowns: (1) `strings` on the agy binary shows it vendors
**zalando/go-keyring**, whose Secret Service items always carry the
attribute pair `{service, username}`; (2) our own `styrman-setup` probe
already keys on `service antigravity` — so the item shape is fully known.
Nothing needs installing at login; agy ships in the image — only the TOKEN
moves.

**Decision:** a fourth broker tier, `keyring`:
- Store: `keyring-<id>.json` (`{id, service, username, label, secret}`,
  0600; service/username/label validated single-line — they ride the TSV
  delivery; secret = arbitrary bytes, base64 on the wire).
- Broker: `GET /keyring` → `[{service, username, label, secret(b64)}]`,
  caller-identified by bridge IP like every other endpoint.
- Delivery: `zz-styrman-credentials.sh` (after the keyring daemon is up —
  `styrman-keyring.sh` sorts first) pipes each item into
  `secret-tool store --label=… service … username …`. Identical attributes
  replace-or-create, so a seeded item is indistinguishable from a native
  sign-in to go-keyring.
- Import: from a signed-in reference project — `secret-tool search --all
  service antigravity` learns the username, `secret-tool lookup` returns
  the raw secret. **Gotcha (live-verified):** `secret-tool search` prints
  the item to stdout but the `attribute.*` lines to STDERR — the import
  merges streams; `2>/dev/null` silently loses the attributes.

**Proven live** (synthetic token, then scrubbed back to virgin state):
seeded a fake `{service: antigravity, username: synth@test}` item in
project A → import reported `agy: stored (keyring)` → a brand-new project
B's first login shell held the item (`secret-tool lookup` returned the
exact secret; the styrman-setup probe reported "agy: keyring credentials
present") with zero manual steps.

**Consequences:** every tool the platform ships is now broker-delivered;
"sign in once, import, all projects current" has no exceptions in
mechanism. Remaining assumption to confirm with a REAL token (operator's
next sign-in): agy stores exactly one item under `service antigravity` and
accepts a seeded refresh token — go-keyring semantics say yes; if agy ever
adds a second service name, extend the import's search. Same
stale-overwrite caveat as the file tier: re-importing after a re-auth
keeps the broker copy current.

---

## ADR-033: Per-entity atomic writers self-create their directory

**Date:** 2026-08-29 · **Status:** accepted (bug-driven — first real-login
400 on v1.5.2)

**Context:** `Server.space()` (`internal/httpapi/server.go`) `MkdirAll`s
`users/<user>/` once and caches the resulting `userSpace` (profileStore,
project.Store, conversation.Store) for the process lifetime. The
2026-08-28 clean-reinstall's post-restart verification calls warmed that
cache for `ostling-simon`, and the final byte-virgin scrub then `rm -rf`'d
`users/` **without restarting the server** — leaving the running process
holding stores whose path pointed at a deleted directory. The operator's
first real `POST /api/settings/profile` (v1.5.2, ~14h later) hit that stale
path and got a 400 (`open .../profile.json.tmp: no such file or
directory`). Only `credstore.atomicWrite` already self-created its
directory before writing; the other three per-user atomic writers assumed
`space()`'s one-time `MkdirAll` was durable for the life of the cache — an
assumption any external scrub of `StateDir` breaks.

**Decision:** Every atomic writer that persists a cached, per-entity store
(profileStore, project.Store, conversation.Store — matching the existing
credstore.atomicWrite pattern) now `os.MkdirAll(filepath.Dir(path), 0o755)`
immediately before opening its `.tmp` file, so a write recovers a missing
directory instead of failing. This is the write-time complement to the
existing tmp+fsync+rename atomicity: directory presence is no longer an
invariant the cache's creator (`space()`) has to guarantee for the whole
process lifetime — each writer guarantees it for itself, on every write.
Going forward, any new cached per-entity store in this codebase should
follow the same self-creating-directory pattern rather than relying on a
one-time `MkdirAll` elsewhere.

**Scope (deliberate):** this covers the writers under
`StateDir/users/<user>/` — profileStore, project.Store, conversation.Store —
because that per-user subtree's parent directory is exactly what the
documented reset removes (`rm -rf StateDir/users`). The other three
process-lifetime cached stores — `links.Store` (`StateDir/links.json`),
`userstore.Store` (`StateDir/users.json`), and the operator overrides in
`config.SetOperator` (`StateDir/settings.json`) — write directly under the
`StateDir` root, which is the container's bind-mount source
(`deploy/docker-compose.yml`) and is never removed by any reset short of
unmounting the volume itself. Deleting those individual files leaves their
parent intact, so their next save recreates the file with no ENOENT; they
were intentionally left unchanged. If `StateDir` itself is ever unmounted
under a running server, `MkdirAll` is the wrong recovery anyway (it would
create an orphaned tree on the container's own layer, masking the loss) —
the `slog.Warn` each writer now emits when it has to recreate a missing
directory is the breadcrumb for that case.

**Consequences:** A stale in-memory cache pointed at a deleted directory
now self-heals on next write rather than 400ing. This does not replace
correct operational procedure: scrubbing `StateDir/users` (or any dir a
cache depends on) under a running server should still be followed by a
restart — cheap, and doesn't rely on every code path having this fix
(noted operationally in
`.agents/docs/changes/2026-08-29-first-login-profile-write-enoent-v1.5.3.md`).
Shipped as v1.5.3.

---

## ADR-034: Setup completion seeds the credential broker (2026-08-29)

**Context:** ADR-032/032a made the broker pull-on-login from every project,
but only after the operator ran Settings → Credentials → Import from some
project. `styrman-setup` runs in the hidden onboarding VM, which mounts the
user's SHARED base home (`orchestrator.EnsureSetup` → `cfg.UserHome(user)`),
so every credential the operator signed into during setup landed there
correctly. But ADR-031 gives projects their OWN per-project homes seeded
from the image — they do not inherit the shared home. Import reads the
PROJECT's guest home, not the shared home, so importing from a brand-new
project found nothing to harvest, the broker stayed empty, and every new
project still needed a full manual sign-in — silently defeating the whole
point of ADR-030/032. Nothing in the system bridged shared home → broker;
this was a real onboarding gap, not user error, and it reproduced live on
the operator's actual first setup run.

**Decision:** `finishSetup` (`POST /api/setup/done`,
`internal/httpapi/setup.go`) now harvests the broker from the setup VM
itself, before tearing it down, using the same extraction logic Import
already used — pulled out of `importCredentials`
(`internal/httpapi/credentials.go`) into a shared
`(*Server).harvestCredentials(ctx, user, containerID, guest) map[string]string`,
called against `config.SetupContainer(u.Username)`. `guestOut` and
`importGuestFile` were changed to take a `context.Context` instead of
reading off `*http.Request`, since `finishSetup` runs the harvest on its own
background context (90s timeout) rather than the request's. The harvest is
best-effort by design: a disabled broker (`s.creds == nil`) or a harvest
that finds nothing must never block completing setup — it only logs a
report. A companion UX change lands in the same deploy: the setup terminal's
`paneCmd` dropped its trailing `exec bash -l` (`internal/terminal/bridge.go`
`ServeSetup`), so the pane ends — and the existing close-4000 signal fires —
the moment `styrman-setup` exits, instead of dropping the operator into an
idle shell; `web/src/routes/setup.tsx` wires `<TerminalPane onExited>` to
call `api.setupDone()` (which now also seeds the broker) and navigate to the
dashboard, so finishing setup is a single, automatic action with no manual
"Done" click.

**Consequences:** the broker's "sign in once, every project current" promise
now holds starting from the very first project — no separate in-project
Import step is required, closing the exact gap the operator hit live. The
Import panel remains as a supplementary path for a tool the operator missed
or re-authed after setup. One known gap remains open: agy did NOT harvest
live, because the minimal-mode setup VM (`STYRMAN_MINIMAL=1`) doesn't run
`gnome-keyring-daemon`, so `secret-tool search service antigravity` finds
nothing even though `~/.local/share/keyrings/login.keyring` exists on disk —
ADR-032a's keyring tier needs a running/unlocked Secret Service to extract
from, and the setup VM in minimal mode doesn't provide one. This is the same
"agy real-token check" the ADR-032a handoff flagged as open, now pinned to
this specific cause rather than left as a general unknown; fixing it (start
+ unlock the keyring daemon in minimal mode, or harvest agy from a full
project VM instead) is deferred. The `styrman-setup` completion message
still tells the operator to manually run Import — now redundant advice — but
correcting it requires a runtime-image rebuild and is deferred rather than
rolled into this patch deploy. Shipped as v1.5.4 (`docker compose up -d
styrman`, in-place restart, state preserved — not a virgin reset); live-
verified against the operator's real account: an existing project restarted
after `finishSetup` pulled git/gh/claude/ssh with zero re-sign-in
(`gh api user` → `kesokaj`, authenticated `git ls-remote` succeeded). As of
this writing the change (including the rebuilt `web/dist/**`) is deployed
live but **not yet committed to git** — prod (1.5.4) is ahead of the
working tree's git history.

Source: internal/httpapi/setup.go, internal/httpapi/credentials.go,
internal/terminal/bridge.go, web/src/routes/setup.tsx. Verification:
.agents/docs/changes/2026-08-29-setup-seeds-broker-v1.5.4.md

---

## ADR-035: Volume-snapshot backups replace git auto-checkpoints (2026-08-29)

**Context:** ADR-022 made checkpoints (periodic 10-min loop + event-driven
snapshots on project stop / terminal pause / close / close-all, via a private
index and `commit-tree` refs) the project's backup/undo mechanism. The
operator decided project backups should instead come from **microVM volume
snapshots** of `/var/lib/styrman` (host-infra: LVM/ZFS/btrfs, or scheduled
restic/borg to GCS — still TODO, filesystem-dependent), and separately wanted
the Git side panel to become a genuinely usable git client (status, stage,
commit, push, diff) rather than existing only to power the checkpoint
machinery.

**Decision:** Removed `RunCheckpointLoop` and the event-driven snapshot
triggers, plus the now-unused `Server.snapshot` helper
(`cmd/styrman/main.go`, `internal/httpapi/server.go`,
`internal/httpapi/checkpoints.go`); the manual checkpoint list/create/restore
endpoints from ADR-022 remain in the backend but are no longer called by the
UI. In their place, `internal/httpapi/git.go` became a real git client whose
commands run INSIDE the guest VM (not host-side against the bind-mounted
workspace) via direct argv, no shell — running in-guest is what lets a commit
carry the user's real git identity and a push authenticate through the
broker's credential helper (ADR-030). Commit author is injected host-side
from the shared-home `.gitconfig` via `-c user.name/user.email`, since
per-project homes (ADR-031) carry no identity by default.

**Consequences:** Until microVM volume snapshots land, there is no automatic
backup/undo mechanism at all for project state — only whatever git history
the user commits themselves. `git commit` typed directly in a project
terminal still lacked identity at ship time (only the git UI injected it
host-side) — closed two releases later by v1.7.1
(`STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` env creds applied via
`git config --global` on each login shell).

Source: internal/httpapi/git.go, internal/httpapi/checkpoints.go,
internal/httpapi/server.go, cmd/styrman/main.go,
web/src/components/SidePanel.tsx, web/src/lib/api.ts. Verification:
.agents/docs/changes/2026-08-29-git-ui-remove-autocheckpoints-v1.7.0.md,
.agents/docs/changes/2026-08-29-git-identity-in-terminal-v1.7.1.md

---

## ADR-036: Project network isolation made effective — bridge naming + egress firewall + self-heal (2026-08-29)

**Context:** ADR-029 shipped a host-INPUT `DROP` rule matching bridge
interface `sdb-+`, verified live at the time. Live re-verification (operator
report) found it inert: Docker names the bridge NETWORK `sdb-<uuid>` but the
bridge INTERFACE itself `br-<hash>` — `-i sdb-+` matched nothing, so a
project VM could in fact reach the host's SSH (:22), the LAN, and cloud
metadata, confirmed live. ADR-029 also only ever guarded the host's own
INPUT chain, never the FORWARD path for routed traffic to other private
ranges (LAN, other projects' subnets, 169.254/16 metadata) — a real gap
beyond what it addressed. Whatever fix followed also needed to survive a
Docker daemon restart, a firewall-manager reload, or a manual flush, not
just a reboot.

**Decision:** Three parts. (1) Name bridge interfaces so the isolation rule
can match them: `config.BridgeNameFor` derives a `sdb-<hash>` interface name
(sha1-derived, ≤15 chars — the kernel's `IFNAMSIZ` cap), set via
`com.docker.network.bridge.name` at both `NetworkCreate` call sites
(`internal/config/config.go`, `internal/orchestrator/orchestrator.go`). (2)
Add real egress filtering: `deploy/host-setup.sh`'s `styrman-firewall.sh`
keeps the existing `INPUT -i sdb-+ DROP` (host) and adds `DOCKER-USER -i
sdb-+ -d <range> DROP` for each RFC1918 range plus 169.254/16, covering the
FORWARD path Docker's own chains never guarded. (3) Self-healing:
`styrman-firewall.service` became a plain (non-`RemainAfterExit`) oneshot,
re-triggered every minute by a new `styrman-firewall.timer`
(`OnBootSec=15s`, `OnUnitActiveSec=60s`); the script switched from `-C ||
-I` to unconditional delete-then-insert per rule, because `iptables-nft -C`
false-positives against the `sdb-+` wildcard (reports a missing rule as
present, silently skipping the insert).

**Consequences:** This completes/hardens ADR-029's stated invariant ("a
project may reach the internet and nothing else") rather than replacing
it — the intent was always full isolation; the bridge-naming mismatch meant
it was never actually enforced. Networks created before this fix needed
manual recreation to pick up the new bridge name (done for the two live
projects at the time); new projects get it automatically. Any firewall
flush now self-heals within about a minute instead of requiring a reboot or
manual re-run.

Source: internal/config/config.go, internal/orchestrator/orchestrator.go,
deploy/host-setup.sh. Verification:
.agents/docs/changes/2026-08-29-network-isolation-fix-ctrlc-v1.8.3.md,
.agents/docs/changes/2026-08-29-firewall-selfheal-timer-v1.8.4.md

---

## ADR-037: Deterministic Caddy upstream selection for the multi-homed server (2026-08-29)

**Context:** The styrman server container is multi-homed by design —
attached to `styrman-ingress` plus every active project's bridge (self-attach
for the port-preview proxy and the same-bridge credential broker, ADR-015 /
ADR-030 / ADR-032). Caddy's ingress plugin (`extractContainerIP` in
`deploy/caddy/ingress/watcher.go`) discovered the upstream IP by iterating
the container's Docker networks map and returning the first entry — Go map
iteration order is randomized per process, so which IP came out was
effectively arbitrary. It happened to return the ingress IP until the
ADR-036 firewall fix recreated both project networks and reattached the
server, at which point re-discovery picked a project-bridge broadcast
address instead. Result: Caddy proxied `crew.styrman.xyz` to an unreachable
project-bridge IP, producing a 502 whose plain-text error body the browser
saved as `document.txt` instead of rendering — live-reported by the operator
as a broken/blank site.

**Decision:** `extractContainerIP` now selects deterministically instead of
taking the map's first entry: (1) prefer the `styrman-ingress` network's IP
if present; (2) else prefer any network that is not a project bridge
(`sdb-*`); (3) else fall back to whatever network exists — the case for a
single-homed preview VM, correctly reached via its own `sdb-*` IP. Locked in
by a regression test (`watcher_ip_test.go`, `TestExtractContainerIP`): a
server on ingress + two `sdb-*` bridges resolves to the ingress IP across
100 iterations (ruling out map-order flakiness); a preview VM on a single
`sdb-*` bridge resolves to its own IP; host-network resolves to `127.0.0.1`;
no networks resolves empty.

**Consequences:** Caddy's upstream selection for any multi-homed container
is now stable and independent of Go map iteration order or
network-(re)attachment order — this class of bug (silent, order-dependent
upstream drift after a network recreate) cannot recur for the
ingress/project-bridge split as currently shaped. Caddy-image-only fix; no
styrman-server rebuild required.

Source: deploy/caddy/ingress/watcher.go,
deploy/caddy/ingress/watcher_ip_test.go. Verification:
.agents/docs/changes/2026-08-29-caddy-ingress-ip-selection-v1.8.5.md

---

## ADR-038: agy credential storage is file-based, not keyring — amends ADR-032a (2026-08-29)

**Context:** ADR-032a modelled agy's OAuth credential as a gnome-keyring
(Secret Service) item — inferred from `strings` on the agy binary showing it
vendors `zalando/go-keyring`, and from `styrman-setup`'s own probe keying on
`service antigravity` — and built a whole broker tier around that
assumption (`keyring-<id>.json` store, `GET /keyring`, a `secret-tool store`
step in the login snippet). It left one explicit open item: confirm against
a real (not synthetic) token that agy stores exactly one item under
`service antigravity`. Investigating that gap live — the v1.5.4 harvest had
reported agy "not found," blamed at the time on the minimal-mode setup VM
not running a keyring daemon — found the real cause instead: this agy build
does not put its token in the keyring at all. A signed-in home's
`~/.local/share/keyrings/login.keyring` is only 719 bytes, far too small to
hold a token, while `~/.gemini/antigravity-cli/antigravity-oauth-token` is a
498-byte, mode-0600 JSON file (`{token: {access_token, token_type,
refresh_token, expiry}, auth_method: "consumer"}`) — a standard, portable
OAuth2 credential with a refresh token. `secret-tool search service
antigravity` finds nothing on this build regardless of whether a keyring
daemon is running: ADR-032a's premise was simply wrong for this agy build.

**Decision:** agy moves to the broker's existing FILE tier (ADR-032),
exactly like claude's credentials file: `harvestCredentials`
(`internal/httpapi/credentials.go`) now reads
`~/.gemini/antigravity-cli/antigravity-oauth-token` as a 0600 file
credential instead of running the two-step `secret-tool search`/`lookup`
keyring path. This also fixes harvesting from the minimal-mode setup VM for
free — a plain file read needs no Secret Service/keyring daemon, where the
keyring path structurally could never have worked there. The broker's
keyring tier (`credstore.KeyringCred`/`PutKeyring`, `GET /keyring`, the
login snippet's `secret-tool store` block) is left in place, unused by any
tool, rather than removed in this change — tracked in TODO.md as a
dedicated cleanup.

**Consequences:** ADR-032a's open verification (real-token check) is
closed, but its answer inverts the ADR's premise — agy was never a keyring
credential for this build, so **ADR-032a is superseded on the
agy-storage point specifically** (the four-tier broker shape it generalized
— env / file / git-helper / keyring — remains valid; the keyring tier
itself just currently has zero consumers and is dormant/dead code pending
removal). Every tool the platform ships is now confirmed broker-delivered
end to end, live-verified with a real account: git/gh/ssh/claude/agy all
materialized in a fresh project's first login shell with zero manual
sign-in.

Source: internal/httpapi/credentials.go, image/configs/styrman-setup.
Verification: .agents/docs/changes/2026-08-29-agy-file-tier-v1.5.5.md

---

## ADR-039: Git RCE closed — host-side git is bootstrap-only and hardened; the checkpoint machinery is deleted (2026-08-30)

**Context.** A multi-agent security review (finding C1, critical) found that
host-side git execution against project workspaces — bind-mounted,
guest-writable trees (ADR-027) — was a guest→server remote-code-execution
vector: a project VM (root in its own microVM, ADR-001) could plant a
`.git/config` with `core.fsmonitor`, `core.hooksPath`, or the `ext::`
transport pointing at an arbitrary program, and any host-side `git`
invocation against that tree would execute it as the styrman server
process. ADR-035 had already moved the interactive git client
(status/stage/commit/push/diff) to run inside the guest, for identity and
credential reasons — but left ADR-022's checkpoint machinery
(`internal/httpapi/checkpoints.go`, `gitrepo`'s snapshot/restore/
`commit-tree` flow) in the backend as dead-but-present code that still ran
host-side `git` against the same guest-writable trees: the exact RCE shape,
just unreachable from the current UI. The residual `gitrepo.IsRepo`/
`EnsureRepo` (`git rev-parse`, `git init`), used at project creation and the
boot-time repo sweep (ADR-022), also ran host-side and unhardened against a
workspace that could already hold an attacker's `.git/config` by the time
`rev-parse` runs (e.g. after an in-guest `git clone` of a hostile repo).

**Decision.** (1) Deleted the checkpoint machinery outright —
`internal/httpapi/checkpoints.go` and `gitrepo`'s snapshot/restore/
commit-tree functions are gone, not merely unused; ADR-022's checkpoint
refs, private index, and 40-snapshot pruning no longer exist in any form.
(2) The two host-side git calls that remain by necessity — `git init -b
main` (new/adopted workspaces) and `git rev-parse --absolute-git-dir`
(`IsRepo`) — are hardened in `internal/gitrepo/gitrepo.go`: every
invocation passes `-c core.fsmonitor= -c core.hooksPath=/dev/null -c
core.pager=cat -c core.editor=false -c protocol.ext.allow=never -c
uploadpack.packObjectsHook=`, plus `GIT_CONFIG_GLOBAL=/dev/null`/
`GIT_CONFIG_SYSTEM=/dev/null` so no ambient host config layers in either;
`init` only ever runs when no repository exists yet, so it can never
observe an attacker-controlled config. (3) The interactive git client
(`internal/httpapi/git.go`) stays guest-side per ADR-035, confirmed
unchanged by this review — direct argv (no shell), executed via
`orchestrator.Exec` inside the project's own microVM, where a hostile
config can only exec as that project's own already-untrusted root.

**Consequences.** No git command in the styrman server process ever again
executes against a workspace whose `.git/config` a guest could control,
closing C1. This completes ADR-035's move (which addressed the interactive
client but left the checkpoint code path standing) and supersedes ADR-022's
checkpoint design entirely — checkpoints are not merely "not called by the
UI" (ADR-035's phrasing) but deleted; the project's only backup/undo
mechanism is, as ADR-035 already stated, the still-deferred microVM
volume-snapshot work (TODO.md) plus whatever the user commits via the
guest-side git client. The `gitInit` config flag (already noted dead in
ADR-022) is untouched by this change.

Source: internal/gitrepo/gitrepo.go, internal/httpapi/git.go. Verification:
.agents/docs/changes/2026-08-30-security-review-remediation-v1.9.0.md
(commit 0192afd).

---

## ADR-040: Broker keyring tier removed — agy credential delivery is file-tier only (2026-08-30)

**Context.** ADR-032a added a fourth broker tier (`keyring`) to deliver
agy's OAuth token via a seeded gnome-keyring/Secret Service item
(`credstore.KeyringCred`/`PutKeyring`, `GET /keyring`, a `secret-tool store`
step in the login snippet), on the premise that this agy build stores its
token in the keyring. ADR-038 (2026-08-29) found that premise wrong — this
agy build never touches the keyring at all; its token is a plain 0600 JSON
file at `~/.gemini/antigravity-cli/antigravity-oauth-token` — and moved
agy's harvest/delivery to the broker's existing file tier, but explicitly
left the now-unused keyring tier's code in place "rather than removed in
this change... tracked in TODO.md as a dedicated cleanup," since nothing in
the system had used it since v1.5.5. The security review's low-severity
backend/script pass picked up that TODO item.

**Decision.** The broker keyring tier is deleted outright:
`credstore.KeyringCred`/`PutKeyring` and the store's `keyring-<id>.json`
kind, the broker's `GET /keyring` endpoint, and the `secret-tool store`
block in the login-shell credential snippet
(`image/configs/styrman-credentials.sh`) are removed. agy is now,
unambiguously, a file-tier credential like claude's — there is no dormant
fourth tier left in the broker.

**Consequences.** ADR-032a is superseded in full: ADR-038 had already
superseded it "on the agy-storage point specifically," and this ADR removes
the remaining dormant code that decision deliberately left behind. The
broker's shape reverts to three tiers in practice — git-helper, env-token,
file-injection — with no capability regression, since no shipped tool ever
used the keyring tier. TODO.md's "Remove the now-dormant broker keyring
tier" item is done and struck.

Source: internal/credstore/, internal/credbroker/,
image/configs/styrman-credentials.sh. Verification:
.agents/docs/changes/2026-08-30-security-review-remediation-v1.9.0.md
(commit 8ca6368).

---

## ADR-041: Setup terminal clean-exit trap + allowlisted per-tool targeting + harvest-without-teardown (2026-08-30)

**Context.** Two first-run-setup problems. (1) Pressing Ctrl+C in the setup
terminal restarted `styrman-setup` instead of exiting: SIGINT killed the bash
script as a signaled exit, the tmux session ended, the browser terminal
treated the socket close as transient and reconnected, and
`tmux new-session -A` (ADR-002's attach-or-create pattern) recreated the
already-ended session — so the script ran again from the top, looking like a
restart loop. (2) Adding a single credential after setup (e.g. signing into
gcloud later) required re-running the entire onboarding flow; the greyed-out
credential rows in Settings weren't actionable, so there was no path to fix
or add one tool without redoing everything.

**Decision.** `image/configs/styrman-setup` adds
`trap 'printf "\n"; exit 0' INT`, so Ctrl+C now always exits 0 instead of
dying to a signal — the pane ends, the bridge's close-4000 signal fires, and
the UI finalizes cleanly with no reconnect-into-recreated-session loop. The
script is also refactored into per-tool functions behind a dispatch:
`styrman-setup` (no args) still runs the full flow unchanged;
`styrman-setup <tool>...` (`git|ssh|gh|gcloud|claude|agy`) runs only that
tool's step (ensuring git identity first when the target isn't `git`/`ssh`).
`internal/terminal/bridge.go`'s `ServeSetup` (`GET /ws/setup`) accepts an
optional `?tool=` param to select a targeted run, given its own tmux session
(`styrman-setup-<tool>`, distinct from the main `styrman-setup` session).
Because that value is interpolated directly into the shell command, it is
strictly allowlisted through a new `setupTool()` helper — only the six known
tool names pass; anything else (shell metacharacters, spaces, command
substitution) resolves to `""` (full flow), covered by
`internal/terminal/setuptool_test.go`'s `TestSetupToolAllowlist`. A new
`POST /api/setup/harvest` (`internal/httpapi/setup.go`) seeds the credential
broker from the RUNNING setup VM without marking setup done and without
tearing the VM down, sharing its extraction logic with `finishSetup`
(ADR-034) via a common `seedBrokerFromSetup(user)` helper. On the frontend, a
new `ManageCredsModal` (`web/src/routes/settings.tsx`) makes the grey/red
credential rows clickable: it boots the setup VM once, runs
`styrman-setup <tool>` in a lazy-loaded embedded terminal
(`TerminalPane`, carrying the tool in its reconnect key), harvests on each
tool's pane exit while keeping the VM up so several tools can be added in one
sitting, and only tears the VM down on "Done". `web/src/routes/setup.tsx`'s
`onExited` is now status-aware — it finalizes via `POST /api/setup/done` only
when git and ssh are both present, otherwise it `DELETE`s the setup session
and leaves the banner up, so an incomplete run (e.g. Ctrl+C before git/ssh)
is never marked done.

**Consequences.** Ctrl+C in the setup terminal now returns control to the
user cleanly and can never trigger the restart loop again. A single missing
or stale credential can be added from Settings without rerunning full
onboarding, and the setup VM stays up across several such adds in one
session rather than being torn down and reprovisioned per tool. The
`?tool=` value being interpolated into a shell command makes the allowlist a
hard security boundary, not a UX nicety — `setupTool()` must stay exhaustive
if a seventh tool is ever added.

Source: image/configs/styrman-setup, internal/terminal/bridge.go,
internal/terminal/setuptool_test.go, internal/httpapi/setup.go,
internal/httpapi/server.go, web/src/lib/api.ts,
web/src/components/TerminalPane.tsx, web/src/routes/settings.tsx,
web/src/routes/setup.tsx. Verification:
.agents/docs/changes/2026-08-30-setup-terminal-robustness-v1.10.0.md

## ADR-042: Sticky stop via a deliberately-stopped marker + boot-intent connects; busy-aware idle reaper (2026-08-30)

**Context.** Every terminal WebSocket connect called `EnsureRunning`, and a
stopped VM's dying execs surfaced as TRANSIENT closes (the aliveness probe on
a stopped container fails → "detached"), so open panes auto-reconnected and
booted the VM straight back. The Stop button could not stick while any
terminal tab was open, and the idle reaper was defeated the same way. The
reaper's idle signal was also incomplete: the activity map only records
attached-terminal traffic, so a detached build, a `docker run -d`, or an
agent still working in a closed tab all looked idle and were killed at the
timeout. Separately, the operator judged the 24h default TTL too long given
that a stop is fully non-destructive.

**Decision.** The orchestrator keeps a `stopped` marker per project
(`sync.Map`), set under the project lock by `Stop()` (BEFORE the container
stop, so the dying execs' reconnects already see it) and by the reaper's
`stopIfIdle`; cleared by any successful `EnsureRunning` and by `Destroy`.
The terminal bridge answers a connect to a marked project with close **4002**
(`config.CloseProjectStopped`) unless the URL carries `?boot=1` — boot
intent, which only user actions produce: `TerminalPane` sends it on the
FIRST connect of an effect run (opening/switching to a pane, pressing
Start/Resume), never on automatic retries. On 4002 the client renders
"Project stopped" with a Start action and does not reconnect.
`stopIfIdle` additionally probes the guest before stopping: any running DinD
container (`docker ps -q`) or any tmux pane whose foreground process is not
a plain shell (`tmux -S /tmp/tmux-*/default list-panes -a -F
'#{pane_current_command}'`, run as root across all user sockets) blocks the
stop and refreshes the idle clock; a probe failure reads as idle so the
reaper still converges on wedged containers. Default
`STYRMAN_IDLE_TIMEOUT` drops 24h → 8h (config default, compose fallback,
.env.example). `cmd/wsprobe` prints `[close:<code>]` so integration checks
can assert the protocol.

**Consequences.** Stop means stopped: terminals become inert views with an
explicit Start affordance, and only intent boots a VM — the reaper and the
Stop button can no longer be undone by an open tab. Real work (containers,
detached/silent commands, running agents) survives any TTL; the timer only
fells genuinely idle VMs, which lose nothing by stopping. The busy probe
adds one short exec per reap CANDIDATE (already past the TTL), not per tick.
Opening a stopped project's terminal still boots it — that click is intent
by definition. `boot=1` is not a security boundary (any authorized client
may boot its own project); the marker is purely a UX-correctness latch.

Source: internal/orchestrator/orchestrator.go, internal/terminal/bridge.go,
internal/config/config.go, web/src/components/TerminalPane.tsx,
web/src/lib/api.ts, cmd/wsprobe/main.go,
.agents/docs/testing/styrman/integration.sh. Verification:
.agents/docs/changes/2026-08-30-sticky-stop-idle-probe-v1.11.0.md

## ADR-043: Project-scoped env variables as a broker-merged credstore tier (2026-08-30)

**Context.** The broker's env tier (ADR-032) is user-wide: every variable
reaches every project's login shells. The operator wants variables scoped to
one project (a DATABASE_URL that belongs to one app), managed from that
project's side panel. Candidate designs: per-project files in the guest home
(survives outside the broker's lifecycle, invisible to the credentials
model), container env at create time (requires a VM recreate per change), or
a broker-side tier (no guest change, live on the next shell).

**Decision.** A project env tier in the credstore, merged broker-side. The
broker already resolves every caller's source IP to `(user, project)` for
logging and the git tier — `/env` now merges `ProjectEnv(user, project)`
over `AllEnv(user)`, project value winning on a shared key. The guest login
snippet is UNTOUCHED: scoping is entirely a broker concern. Storage is
`<user>/proj-<projectID>/env-KEY.json`, sharing the user-wide tier's
validation (identifier key, single-line value ≤4096 bytes — the line-based
guest delivery must never be injectable); `List` skips directories so the
Settings view never shows another surface's rows. CRUD lives on the project
(`/api/projects/{id}/env[/{key}]`, owner-scoped by `getProjectFor`): the
list returns keys only, a single-value read is audit-logged like the
Settings credential viewer, and `deleteProject` removes the whole scoped dir
so variables cannot outlive their project or leak into an id reuse. The UI
is a SidePanel "Env" tab; copy states the contract (new login shells only;
overrides a same-named global; falls back on delete).

**Consequences.** Changing a project variable needs no VM restart — the
next login shell picks it up, same as the user-wide tier. A project variable
shadows a global one by design; the fallback-on-delete keeps the mental
model simple (most-specific wins). The merge happens on every `/env` pull
(one extra directory read; negligible). Verified live end-to-end: export in
the owning project, absent in another, shadow + fallback + project-delete
cleanup all observed on crew.

Source: internal/credstore/credstore.go, internal/credbroker/broker.go,
internal/httpapi/projectenv.go, internal/httpapi/server.go,
web/src/components/SidePanelEnv.tsx, web/src/components/SidePanel.tsx,
web/src/lib/api.ts, web/src/routes/settings.tsx. Verification:
.agents/docs/changes/2026-08-30-project-env-vars-v1.12.0.md

## ADR-044: Per-project backup/restore as server-side tar.gz + root-assisted file ops via the server's own container (2026-08-30)

**Context.** ADR-035 removed git auto-checkpoints; R13 deferred durable
backup to host-level volume snapshots. The operator now wants a per-project
lifecycle: a Backup button, restore, and creating a new project from a
backup. The host FS is plain ext4 without LVM (verified), so native
snapshots are unavailable; the docker disk (`~/.dockerd`, up to tens of GB,
reproducible from the apt manifest + registry) dominates a naive archive.
Complication discovered live: the hardened server runs UNPRIVILEGED
(uid 1000), while homes accumulate ROOT-owned entries — dockerd's
bind-mount target stubs (pre-v1.11.1) and anything the guest creates via
passwordless sudo (virtiofs maps guest root to host root). An unprivileged
RemoveAll cannot delete those, and a plain rename cannot even MOVE a
root-owned directory to a new parent (the ".." rewrite needs write on the
directory itself).

**Decision.** A `internal/backup` package archives `workspace/` + `home/`
prefixes into tar.gz, excluding `~/.dockerd`; extraction is defensive
(no absolute/`..` paths, only files/dirs/symlinks, best-effort Lchown).
Archives live under `StateDir/backups/<user>/<projectID>/` with a meta
sidecar, survive project deletion, and back three flows: restore-in-place
(stopped projects only, guarded by a `restoring` map every boot path
checks), create-from-backup (seeds workspace + home + the source's scoped
env vars pre-boot), and deletion by raw ids under /api/settings. Restore is
TRANSACTIONAL: current state is quarantined by rename and renamed back on
extraction failure — never deleted first. For root-owned entries the
quarantine, its cleanup, and wipe-delete escalate through the server's own
container: `docker exec -u 0` on SelfContainer running `mv` /
`find -delete`, path-guarded to StateDir. This grants nothing the server
lacks — it already holds the docker socket (root-equivalent); it makes the
existing power explicit, auditable, and confined.

**Consequences.** Backups are crash-consistent (file-level, allowed while
running) and small (docker disk excluded; a rebuilt project repopulates its
cache). A failed restore leaves the project exactly as it was — proven live
when the first two attempts failed on the root-ownership traps and rolled
back intact. Restore requires the project stopped, composing with ADR-042's
sticky-stop. Backups of deleted projects remain restorable into new
projects. The host-level whole-state snapshot plan (TODO) is unchanged —
this is the user layer, that is disaster recovery. Root-assisted ops exist
ONLY for StateDir paths via the two guarded helpers; any new caller must
keep that boundary.

Source: internal/backup/, internal/httpapi/backups.go,
internal/httpapi/server.go, internal/orchestrator/orchestrator.go,
web/src/components/Sidebar.tsx, web/src/components/NewProjectDialog.tsx,
web/src/routes/settings.tsx, web/src/lib/api.ts. Verification:
.agents/docs/changes/2026-08-30-project-backups-v1.13.0.md

## ADR-045: Roadmap reset — everything shipped or explicitly retired (2026-08-30)

**Context.** After the v1.9–v1.13 remediation-and-feature burst the operator
directed: "I want nothing in todo and roadmap. This work should be to clean
it up so I can start fresh with new things." Every open item was therefore
either BUILT (v1.14.0, one marathon session) or RETIRED here, append-only,
so TODO.md can be an empty inbox without losing any decision.

**Shipped in v1.14.0** (details in the changelog): credential push to
running projects; firewall-aware integration skips; transcript janitor
(STYRMAN_TRANSCRIPT_TTL); clean-reinstall doc; vestige sweep; host-layer
restic→GCS backups (europe-north2/Nearline, daily systemd timer, restore
round-trip verified) + in-app backup cadence/retention/pre-wipe safety net;
ops panel (dashboard CPU/mem/disk chips + microVM console logs endpoint);
central session-expiry reload + Caddy explicit env allowlist; poll
de-amplification (one shared listProjects store) + batched git badges;
modal a11y core (focus trap/restore, topmost-Escape) across every overlay;
keyboard-reachable primary rows; poll-error surfacing; nav-helper dedupe;
responsive side-panel default; preview-proxy residuals (attach + owner
memos, path.Clean + Location rewrite, S10 expiry-bounded streams, fail-loud
public-link mint, concurrent LiveIn notifications scan, aliveFromProbe
extraction); agent NATIVE RESUME (bare `claude`/`agy` autostarts run
`--continue || fresh`, unparking ADR-011; the never-read AgentSession UUID
removed). *(Revised same day in v1.14.1: auto-resume REVERTED to
per-terminal opt-in — all terminals of a project share one cwd, so the
automatic rewrite made several terminals resume the SAME conversation;
users now set `claude --continue` / `agy --continue` as the autostart
themselves. See 2026-08-30-review-remediation-v1.14.1.md.)*

**Retired, with reasons** (operator-ratified 2026-08-30):

- **Warm VM pool of 1** — the runtime image is prebuilt; a pool would only
  hide the ~5–10s Kata cold boot, which the operator judged fast enough.
  If boot latency ever matters, optimize the boot path, don't juggle
  standby VMs.
- **Platform API as an MCP server** — would let in-project agents manage
  styrman itself (ports/backups/restarts); no current need, real new attack
  surface. Reintroduce as a first-class feature if wanted.
- **Auto-titles from claude transcripts** — structurally unreliable: claude
  sessions key on the project CWD, not the terminal, and several terminals
  share one slug dir, so terminal↔session mapping cannot be made truthful.
  The dashboard's live transcript tails already answer "what is this
  terminal doing" better than a stale title would.
- **moby SDK migration** — pinned docker/docker v27 works; churn with zero
  user value. Revisit only when an upstream break forces it.
- **Artifact Registry Makefile target** — conditional on styrman ever
  leaving this host; resurrect then.
- **Pruning the tiny in-memory maps** (projLocks/ipCache/wsLocks) — bounded
  in practice at single-operator scale; documented at the declarations.
- **Exercising the entrypoint's vfs fallback in a test** — the fallback has
  never fired in production; revisit if it ever does.
- **SPA hydration warning (React #418)** — carried from the 2026-08-27 e2e:
  recoverable by design (React re-renders), all client-only state already
  flows through useSyncExternalStore with server snapshots, and the e2e
  harness reports it if it ever regresses into something real. Closed as
  known-benign rather than tracked forever.

**Standing WON'T DO** (operator decisions 2026-08-27, moved from TODO.md so
they stay on record): I14 docker-socket read-only proxy for Caddy —
declined; I13 version-pinning Go/Node/claude/yq in the runtime image —
declined (the terminal is a dev environment and intentionally ships current
tools; the Kata tarball checksum in host-setup.sh stays — that is
supply-chain verification, not version pinning).

**Consequences.** TODO.md is an empty inbox: anything that matters next is
a NEW decision, not archaeology. Retired items are not forgotten — they are
here, with the reasoning to revisit against.

Source: the v1.14.0 commits (e043765..HEAD). Verification:
.agents/docs/changes/2026-08-30-roadmap-clearing-v1.14.0.md

---

## ADR-046: Vendored harness stays a verbatim mirror; STYRMAN_VERSION bump is the only delivery path (2026-09-01)

**Context.** `image/harness/` is a copy of `devshell_prompt/harness` baked
into the runtime image so every project home gets identical agent-harness
defaults from the base image, no shared home or host-side deploy dependency
(R5/ADR-030). The image entrypoint deploys it into each project home
write-once, gated by a marker (`~/.styrman/.harness-version`, keyed on
`/styrman/VERSION`) — so a home only ever re-runs `setup.sh` when the image
version it was seeded from differs from the current one. The v1.15.0 sync
(upstream commit `6132b97`) pulled in a substantial harness rehaul —
Constitution rewrite, 4→6 subagents, 2 new shell gates, 23→28 skills — and
raised the question of how such vendor updates should be applied and shipped
going forward.

**Decision.** `image/harness/` is treated as a verbatim mirror of
`devshell_prompt/harness` and is never hand-edited in place; the single
exception is `image/harness/VENDORED.md`, a styrman-only file preserved
across every sync to record the sync date, upstream commit hash, and a
summary of what the harness currently contains. Any change upstream is
brought in only by a full re-copy from `devshell_prompt/harness`. Because
the deploy is write-once and keyed on `/styrman/VERSION`, a harness change
has exactly one delivery path: bump `STYRMAN_VERSION` and rebuild the
runtime image. There is no mechanism to push a harness change to
already-running project homes without that version bump — matching the
existing credential-broker precedent (ADR-030) that the image, not a
runtime patch, is the unit of harness delivery.

**Consequences.** Dropped or renamed agents and skills (e.g. `testing.md`
retired in favor of `verifier`, `skill-creator` and `code-review` removed in
v1.15.0) are retired from an already-seeded home only when that home
re-seeds against the new version — `claude/setup.sh`'s explicit
retired-agent removal plus `rsync --delete` on the skills directory do the
actual cleanup, but only run once per version bump, never proactively. A
project that never restarts its container never gets a harness upgrade.
Re-seeding also leaves timestamped `.bak` litter behind in the guest home
(one set per bump) and, on `rsync --delete`, silently removes any skill a
project user hand-added to `~/.claude/skills/` — both accepted as the cost
of write-once, image-driven delivery rather than a live sync channel.

Source: image/harness/VENDORED.md, image/harness/claude/setup.sh.
Verification: .agents/docs/changes/2026-09-01-harness-upgrade-v1.15.0.md

## ADR-047: Styrman-side workarounds for upstream harness behavior live in the entrypoint invocation, never inside image/harness/ (2026-09-01)

**Context.** An independent review of the v1.15.0 harness sync (ADR-046)
found that `antigravity/setup.sh` — the vendored, verbatim-mirrored script —
ends with a self-check, `agy -p "/skills"`, guarded only by `command -v
agy`. On any home with no Antigravity OAuth token (every newly created
project, since the token is materialized by a login shell that has not run
at entrypoint time) that call hard-blocks the boot path for a fixed 60s,
violating REQUIREMENTS.md R1 and ADR-045's stated cold-boot latency. ADR-046
already commits `image/harness/` to being a verbatim mirror, never
hand-edited — so patching `setup.sh` directly was not an option without
breaking that rule and losing the ability to `diff -rq` clean against
upstream on every future sync.

**Decision.** The fix runs the vendored `setup.sh` with a trimmed PATH
(`/usr/bin:/bin:/usr/sbin:/sbin`, excluding `/usr/local/bin` where `agy`
lives) from `image/entrypoint.sh`'s invocation of the harness-seeding block,
so `command -v agy` fails and the self-check is skipped — without touching
a single byte inside `image/harness/`. This sets the precedent for how
future upstream-harness quirks should be handled: a styrman-side workaround
for behavior baked into the vendored tree belongs in the *invocation* of
that tree (`image/entrypoint.sh`, or equivalent call-site code outside
`image/harness/`), never inside `image/harness/` itself. If a workaround
cannot be expressed as an invocation-side change (e.g. it requires altering
control flow inside a vendored script), that is a signal to raise the issue
upstream in `devshell_prompt/harness` rather than to special-case the
mirror.

**Consequences.** The mirror stays verbatim and `diff -rq` against upstream
remains the fidelity check ADR-046 relies on. The trade-off is that
invocation-side workarounds are easy to lose track of over time (they live
away from the code they're compensating for, discoverable only via
entrypoint comments and this ADR) — future re-syncs must re-check whether
each workaround is still needed, since an upstream fix could make it
obsolete or, worse, an upstream change could shift the assumption the
workaround depends on (e.g. if `agy` moves to a different PATH location).
No workarounds are known to be currently stale as of this writing.

Source: image/entrypoint.sh (harness-seeding invocation), the review at
.agents/docs/reviews/2026-09-01-review-harness-vendor-sync-v1.15.0.md.
Verification: .agents/docs/changes/2026-09-01-harness-upgrade-v1.15.0.md
(Post-review finding and fix section).

---

## ADR-048: File editor is a view of the main pane, not a modal — one pane per terminal, terminal never unmounts (2026-09-01)

**Context.** The file editor was a centered modal launched from the 320px
side panel, stacked on top of the terminal and blocking it while open. The
operator asked for the editor and the terminal to coexist as two views of
the same space, switchable per terminal, so one terminal in a project can
show an editor while another shows a live shell — and for the switch to be
instant and lossless, not a page-like transition.

**Decision.** The terminal's main pane now renders both the terminal and the
editor at all times; only one is visible, controlled by a `Terminal |
Editor` toggle in the terminal header (`web/src/components/EditorPane.tsx`,
`web/src/lib/editorPane.ts`). View state is per-terminal, keyed
`${project}/${term}`, held in a **module-level** store
(`useSyncExternalStore`, not React Router route state or component state) —
because `TerminalPane` is keyed by project/term and remounts on every
terminal switch, route-local state would forget which view each terminal was
on. The pane-open file is deliberately not persisted to localStorage; it's
session context, not a durable preference (contrast with the `Wrap` toggle,
which is a preference and is persisted). The terminal DOM/component is
hidden with CSS when the editor view is active and is **never unmounted**:
`TerminalPane`'s cleanup effect (`TerminalPane.tsx:272-281`) closes the
terminal's WebSocket on unmount, so unmounting on every toggle would kill the
live session each time the operator switched away from the terminal. Shared
editor logic (`useFileEditorState`, toolbar, body) was extracted into
`web/src/components/FileEditor.tsx` so the new pane and the pre-existing
modal (kept for Settings, which has no terminal in context) run one
implementation, not two.

**Consequences.** Any future component that reads or writes per-terminal UI
state that must survive a terminal-route remount should follow the same
module-level-store-keyed-by-`${project}/${term}` pattern rather than route
state — this is now the precedent, not a one-off. The never-unmount
constraint on `TerminalPane` is now load-bearing for more than the terminal
itself: any future work that changes how/when the main pane swaps views must
preserve it, or every view toggle will silently drop the operator's shell
session.

**Correction (same-day review remediation).** Two claims in the first draft of
this ADR were wrong and are corrected here rather than left to be copied:
(1) the module-level store is NOT needed because the route remounts on a
terminal switch — it does not remount (TanStack Router keys a match only when
`remountDeps` is configured, and none is). The real reason is the opposite:
plain `useState` would PERSIST across the switch and leak one terminal's view
and open file onto the next, so keying by `${project}/${term}` is what makes
the state per-terminal. (2) The modal was NOT retained for Settings — Settings
renders `CodeEditor` directly and never mounts `FileExplorer`, so the modal
had no remaining entry point and was deleted. There is now exactly ONE editor
surface.

Source: web/src/lib/editorPane.ts, web/src/components/EditorPane.tsx,
web/src/components/FileEditor.tsx, web/src/components/TerminalPane.tsx:272-281.
Verification: .agents/docs/changes/2026-09-01-editor-pane-and-ordering.md

---

## ADR-049: A hijacked Docker exec stream is one bidirectional connection — never `CloseWrite()` it, guests must read an exact byte count (2026-09-01)

**Context.** `internal/orchestrator/exec.go`'s `Exec` helper runs
guest-side scripts (format, and now check) by writing a payload to the
hijacked exec stream's stdin and then calling `CloseWrite()` to signal EOF,
the normal pattern for a plain pipe. A Docker hijacked exec connection is
not a plain pipe: it is a single bidirectional TCP/unix connection carrying
stdin on the write side and the multiplexed stdout/stderr frames on the read
side. Half-closing it with `CloseWrite()` tears down the whole connection,
not just the write half — so anything the guest process printed *after* it
finished reading stdin was silently discarded. This shipped `/format`
returning `{"content":""}` for every call, and since the editor pipes that
response straight into the buffer, clicking Format blanked the open
document — a real data-loss bug, live in production since `/format`
shipped, caught only when instrumented guest-side logging showed the
formatting script completing normally with no corresponding output ever
reaching the server. Unit tests never caught it because the defect was in
the transport, not in anything the tests exercised (script logic, argument
parsing) — end-to-end/live verification was the only path that could have
caught it, and initially didn't.

**Decision.** Guest-side scripts driven over exec never rely on stdin EOF.
The caller sends an exact byte count up front (or the script otherwise knows
its expected input length) and reads with `head -c <len>`, so the
connection is never half-closed and stays fully open for the guest to write
its result back. This is now the required contract for any future
stdin-carrying exec endpoint in this codebase — `CloseWrite()` on a
hijacked Docker exec stream is a bug, not a valid way to signal EOF.
Guarded by `TestGuestScriptsNeverWaitForStdinEOF`, negative-control tested
(fails if `cat >`/stdin-EOF-waiting is reintroduced into a guest script).

**Consequences.** `/format` and the new `/check` (ADR item, same day —
`.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md`) both read
exact byte counts and are verified live to return their guest's full
output. Any new endpoint added later that pipes stdin to a guest process
over Docker exec must follow the same pattern; reviewers should treat a
`CloseWrite()` call on an exec stream as a red flag pending confirmation
it isn't the hijacked bidirectional kind.

Source: internal/orchestrator/exec.go, internal/httpapi/check.go.
Verification: .agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md

---

## ADR-050: Tag-gated auto-update from a dedicated deploy clone, with rollback and a failed-tag latch (2026-09-01)

**Context.** The operator asked for the platform to update itself
automatically (operator request item 1), porting the update mechanism from
argolis-devbox's `devbox update`. The argolis original acted on new commits
directly in whatever tree it ran from; the operator explicitly wanted
deploys gated on stable version tags instead, so release candidates could
be pushed to the remote and inspected without being auto-deployed.

**Decision.** `deploy/update/styrman-update` (+ `.service`/`.timer`,
installed by `deploy/host-setup.sh` step 8c) polls the origin, filters tags
to `^v[0-9]+\.[0-9]+\.[0-9]+$` (excluding any `-rc`/`-beta`/etc. suffix by
construction), and picks the highest by `sort -V` rather than the most
recently pushed. Three departures from the argolis original: (a) it
operates on a **dedicated clone at `/opt/styrman`**, never the operator's
own working tree, because styrman's compose build reads from the repo root
and an automatic `git checkout <tag>` against a tree the operator is
actively editing would stomp uncommitted work; (b) it **rolls back** to the
previous tag if the newly deployed version fails to report itself healthy
via `/api/version`, and records the failed tag so the 5-minute timer does
not rebuild a known-broken release on every subsequent tick; (c) `flock`
serializes runs, since the argolis timer could stack overlapping
invocations if one run outlasted the timer interval. Both the server and
runtime images rebuild on every matching tag, by the operator's explicit
choice — the runtime rebuild is also how a harness version bump
(ADR-046/047) reaches already-running project homes on their next boot.

**Consequences.** RCs and betas are structurally invisible to the updater —
no separate "ignore pre-release" flag to keep in sync, the tag regex is the
whole mechanism. The updater is not yet live on this host: the timer is
gated on the deploy clone having its own `deploy/.env` (gitignored), which
a fresh `/opt/styrman` clone does not have until `host-setup.sh`'s
credential step is run there — enabling it is a deliberate, separate,
follow-up action, not a side effect of this change landing. Tag selection,
ordering, rollback-on-unhealthy, failed-tag latch, missing-clone, and
concurrent-lock paths were all exercised against a real origin before
shipping.

**Correction (same-day review remediation).** The original decision text above
says the updater "rolls back to the previous tag if the newly deployed
version fails to report itself healthy via `/api/version`". That approach
was abandoned before shipping and never went live: production runs
`STYRMAN_AUTH=header`, `authGuard` exempts only `/api/health`, and the
updater has no ingress token or session — its synthetic HTTP identity got
403 on every call, so **every** healthy release would have timed out,
rolled back, and latched its own tag as failed. What actually shipped
instead: `deploy/Dockerfile.server` stamps `LABEL
styrman.version="${STYRMAN_VERSION}"` (the `ARG`/`LABEL` pair is
re-declared in the final stage and placed **last**, after `COPY`/
`HEALTHCHECK`, so a version bump alone doesn't bust the apt-layer cache),
and `styrman-update`'s `reports_version()` reads that label straight off
the running container with `docker inspect`, together with
`.State.Running` and `.State.Health.Status`. No HTTP request, no auth, no
ingress token, and `authGuard` was deliberately **not** widened to exempt
`/api/version` — the label read needs no auth surface at all.

This is a load-bearing cross-file contract, not an implementation detail:
`deploy/Dockerfile.server`'s `LABEL` and `styrman-update`'s
`reports_version()` must move together. **Deleting or renaming that LABEL
makes the updater roll back every future release**, because an absent
label reads as an empty string and fails the health check closed. That
coupling currently exists only as comments in the two files — anyone
touching either must know the other exists.

Source: deploy/update/styrman-update, deploy/update/*.service,
deploy/update/*.timer, deploy/host-setup.sh (step 8c),
deploy/Dockerfile.server.
Verification: .agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md,
.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-rereview.md

---

## ADR-051: Auto-update requires signed tags — narrows the trust boundary to the operator, not "anyone who can push a tag" (2026-09-01)

**Context.** A `security` review of ADR-050's updater
(`.agents/docs/reviews/2026-09-01-security-editor-check-and-autoupdate.md`)
found the worst problem was not the shell-injection surface (that one is
genuinely fixed) but the trust model of the auto-update path itself: the
updater checks out any `vX.Y.Z` tag that appears on the remote and, as
**root**, `docker build`s and `docker compose up`s it within ~5 minutes —
and `docker-compose.yml` itself lives inside that checkout, so a hostile
revision does not need a code-execution bug, it only needs to add a bind
mount (e.g. `/:/host`) to own the host. Before this decision the updater
verified nothing about where a tag came from: no `git verify-tag`, no
allowed-signers file, and the accepted trust set was implicitly "anyone
who can push a tag" — a GitHub account compromise, a future collaborator,
or a write-capable CI token — rather than "the operator". The read-only
deploy key used to fetch constrains the *host*, not the *pusher*.

**Decision.** Signed tags are now **required**. Before checkout, the
updater runs `git cat-file -t "${LATEST}"` and rejects lightweight tags
outright (a lightweight tag cannot carry a signature), then runs
`git verify-tag` and fails closed if it does not succeed. This can only be
turned off via root-owned `/etc/styrman/update.env`
(`STYRMAN_UPDATE_REQUIRE_SIGNED=0`), which logs a warning every time it
fires, so silently disabling the check is not possible from a
non-root-writable location. Three related hardening changes landed
alongside it, all forced by the same "root builds and runs this checkout"
threat model: (a) root's `git fetch` now pins the GitHub host key
(`StrictHostKeyChecking=yes` + `/etc/styrman/known_hosts`) instead of
`accept-new`, because a root-privileged fetch must not trust-on-first-use
even for one window; (b) the deploy clone's ownership is asserted with
`stat -c %u` before `-c safe.directory` disables git's ownership guard,
so the override only ever applies to a clone that is actually root-owned;
(c) `git clean -d --force` runs after checkout so untracked files planted
in the clone cannot survive into the build context (`context: ..`) —
`-x` is deliberately **not** passed, because `deploy/.env` is gitignored
and must not be deleted by the clean.

**Consequences.** The operator-visible cost is immediate: tags
`v1.14.0`–`v1.15.0` are annotated-but-unsigned or lightweight, so the
updater would refuse every one of them as-is. All releases from this
point forward must be created with `git tag -s`. Also decided in the same
review pass, recorded here rather than as a separate ADR because they are
smaller-scoped hardening of the same trust boundary rather than new
decisions: `hadolint` is now pinned to `2.15.1` with a sha256 checksum
(an unpinned tool made the tag→image mapping non-deterministic, so a
rollback would not reliably reproduce the image the old tag originally
produced), and the updater's systemd unit gained `NoNewPrivileges`,
`ProtectHome`, and similar hardening — with the explicit caveat that this
buys little on its own: a hostile tag's payload runs inside the Docker
daemon, not inside this unit's cgroup, so the signature check above is
the control that actually matters, not the sandboxing.

**Correction (2026-09-01 preflight remediation, commit `534e0bb`).** Two
security properties this decision claimed did not actually hold as shipped.

First, "runs `git verify-tag` and fails closed" did not narrow the trust
boundary to the operator as the Context/Decision text above asserts:
verified empirically, GPG's `git verify-tag` exits 0 for a
good-but-**untrusted** signature, and git never compares the signer to the
tagger, so the real accepted trust set was "any key ever imported into
root's keyring" — the same "anyone who can push a tag" problem this ADR
set out to close, just moved one layer down. The SSH signing path does not
have this gap (it enforces an allowlist and fails closed when
unconfigured), so it is now the supported path: `verify_tag()` asserts
root's `gpg.ssh.allowedSignersFile` is set and readable, failing closed
with setup instructions otherwise. Verification was also missing from the
rollback path — `deploy "${PREVIOUS}"` built the previous tag with no
check, and `git fetch --tags --force` accepts a re-pointed old tag — so
`verify_tag()` is now called for every tag `deploy()` builds, rollback
included, not only the forward path.

Second, the "root's `git fetch` now pins the GitHub host key... instead of
`accept-new`" line in the Decision above was undermined by a change made
later the same day: `deploy/host-setup.sh` started auto-seeding
`/etc/styrman/known_hosts` via `ssh-keyscan` during its auto-clone step,
paired with `accept-new` against that same file — the exact unauthenticated
trust-on-first-use this ADR rejected, just relocated from fetch-time to
provisioning-time rather than closed, and a failed scan left an empty file
that the clone then TOFU-populated regardless. This is a case of code
asserting a security property (fixed host-key pinning) that it did not
have. `host-setup.sh` no longer auto-clones over SSH; it prints commands to
seed `/etc/styrman/known_hosts` from GitHub's **published** host-key
fingerprints, and the comment claiming the TOFU window was closed has been
corrected.

Two smaller hardening changes landed in the same remediation, forced by
the same "root builds and runs this checkout" threat model: root now reads
`/etc/styrman/update.env` with `grep` instead of sourcing it (sourcing ran
its contents as root; a redefined `log(){ :; }` there silently swallowed
the "signature checking DISABLED" warning), with root ownership and
non-group/world-writable mode asserted first, and the same ownership
assertion was added for `deploy/.env`, which supplies
`STYRMAN_ALLOWED_EMAILS` and survives `checkout --force`/`git clean -d`.

Source: deploy/update/styrman-update, deploy/update/*.service,
deploy/host-setup.sh, image/Dockerfile.
Verification: .agents/docs/reviews/2026-09-01-security-editor-check-and-autoupdate.md,
.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md (§6)

---

## ADR-052: Privileged tests never touch a production-aliasing path — mandatory scratch prefix (2026-09-01)

**Context.** On 2026-09-01, while exercising the tag-gated auto-updater
(ADR-050/ADR-051) before it was tagged into v1.16.0, `/var/lib/styrman` —
the entire production state directory (every project home, workspace,
per-user JSON store, and in-app backup) — was found empty and root-owned,
and `/etc/styrman` (home to the restic/GCS disaster-recovery secrets, which
predate the updater entirely) was deleted outright by a `sudo rm -rf`
issued as test cleanup. The exact command that emptied `/var/lib/styrman`
was never identified; it happened while privileged updater tests and
parallel verification agents ran against paths adjacent to production
state. Full incident record:
`.agents/docs/changes/2026-09-01-incident-state-loss.md`. Two compounding
causes: the updater's default state directory pointed at
`/var/lib/styrman` with no override consistently applied when tests ran as
root, and the cleanup step deleted `/etc/styrman` without establishing
that anything other than the feature under test lived there.

**Decision.** Any privileged (root-run) test, script exercise, or
verification pass MUST run against a scratch prefix that cannot alias a
real production path — never a default that resolves inside
`/var/lib/styrman`, `/etc/styrman`, or their equivalents. Concretely: (a)
host-level scripts and services (the updater and any future ones) take an
explicit, test-overridable state-directory argument/env var with **no
default that is itself a production path** when invoked outside its own
systemd unit; (b) any harness or agent that must run privileged tests sets
that override unconditionally as part of test setup, rather than relying
on a human or agent to remember it per invocation; (c) a cleanup step
(`rm -rf` or equivalent) run against a shared directory must first
establish everything that lives there and confine deletion to what the
feature under test actually created — a directory that predates the
feature (like `/etc/styrman`, which held DR secrets before the updater
existed) is the highest-risk case precisely because it looks
self-contained to whoever is testing the newer feature.

**Consequences.** This ADR states the convention; it does not itself
perform the fix. Two follow-ups are tracked in `TODO.md`: moving the
updater's default state path off `/var/lib/styrman` in its docs and tests,
and re-provisioning `/etc/styrman/backup-sa.json` and `restic-pw` (the
daily `styrman-backup.timer` fails until then). Separately, this incident
is also a case for treating out-of-band credential storage as load-bearing
policy rather than incidental: the restic password's copy in the
operator's password manager — outside `/etc/styrman` — is the only reason
the GCS backup stayed recoverable in principle once the on-host copy was
destroyed alongside the data it protects.

Source: .agents/docs/changes/2026-09-01-incident-state-loss.md,
.agents/docs/reviews/2026-09-01-audit-whole-project.md,
deploy/update/styrman-update, deploy/backup/styrman-host-backup

---

## ADR-053: Immutable versioned image tags — one release version drives all three images (2026-09-01)

**Context.** The operator re-stated a standing rule the same day: "never
`latest`, always versions." Two same-day incidents made the cost of not
having this concrete. First,
`.agents/docs/changes/2026-09-01-incident-reboot-outage.md`: a `sudo
reboot` restarted `styrman-caddy` on the mutable `:latest` tag, which
still pointed at the 2026-08-30 image — a fixed image built at 16:49 that
same day sat unbooted under a different tag, and nothing forced the
running container onto it. Second, the same day's whole-project audit
caught production's `deploy/.env` pointing `STYRMAN_IMAGE` at the mutable
`styrman-runtime:dev` tag, and the redfix round-2 review separately caught
the testing guide recommending that exact tag be re-stamped by hand — a
tag that ordinary test/dev iteration silently mutates underneath whatever
happened to be booted from it last.

**Decision.** A single `STYRMAN_VERSION` drives every image this project
ships. Release flow: `git tag -s vX.Y.Z` (ADR-051's signed-tag requirement
already covers provenance) → `STYRMAN_VERSION=X.Y.Z` → `styrman-server:X.Y.Z`,
`styrman-caddy:X.Y.Z`, `styrman-runtime:X.Y.Z`. Enforcement is structural,
not conventional: `deploy/docker-compose.yml` interpolates
`${STYRMAN_VERSION:?set STYRMAN_VERSION=X.Y.Z — mutable tags are banned}`
for the server and caddy images (compose refuses to render at all without
it) and derives `STYRMAN_IMAGE` from the same variable;
`internal/config/config.go`'s `Load()` independently refuses an empty
`STYRMAN_IMAGE` (previously a silent fallback baked into the default —
closed the same day this ADR was written); `deploy/update/styrman-update`
and `deploy/host-setup.sh` both build `styrman-runtime:${ver}` from the
same tag-derivation shape (`${LATEST#v}` / `git describe --tags --abbrev=0
| sed 's/^v//'`) rather than defaulting to `:latest`; the two Dockerfiles
pin their base images and, for the Caddy builder, every `xcaddy` plugin
version by exact tag (`caddy:2.11.4-builder`/`caddy:2.11.4`,
`caddy-security@v1.1.64`, `caddy-dns/cloudflare@v0.2.4`,
`caddy-ratelimit@v0.1.0`, `golang:1.25.13-trixie`) instead of floating
majors. `:dev` and `:test` remain legitimate tags for local iteration —
they are never produced or consumed by any production path, only passed
explicitly by a human or a test harness that sets `STYRMAN_IMAGE` itself.

**Consequences.** "What's running is readable off `docker ps`" — a
version tag now means one specific, reproducible build, and a rollback is
just re-pointing compose at an old immutable tag rather than hoping the
right bytes are still behind `:latest`. Cost: every deploy path (manual,
`host-setup.sh`'s fresh install, the auto-updater) now needs its own
version-tagged runtime-image build step, and the redfix round-2 review
caught exactly this gap on the manual/fresh-install path before it shipped
(closed in the same commit as this ADR). Deferred, tracked in `TODO.md`:
digest-pinning the base images (a tag pin is still mutable upstream — the
security review's point) and asserting the Caddy builder's own Go
toolchain version, the way `deploy/caddy/Dockerfile` already asserts its
plugin set via `caddy list-modules`.

Source: deploy/docker-compose.yml, internal/config/config.go,
deploy/update/styrman-update, deploy/host-setup.sh,
deploy/Dockerfile.server, deploy/caddy/Dockerfile, deploy/.env.
Verification: .agents/docs/changes/2026-09-01-redfix-hardening-and-versioning.md,
.agents/docs/reviews/2026-09-01-review-redfix-round2.md (round 3),
.agents/docs/testing/2026-09-01-verify-redfix-round2.md.

---

## ADR-054: Ingress reload self-heal — retry rejected loads, exit on admin loss (2026-09-01)

**Context.** `.agents/docs/changes/2026-09-01-incident-reboot-outage.md`:
on a host reboot, the Caddy ingress's first `/load` attempt failed (the
network wasn't up yet for `caddy-security`'s OIDC metadata fetch), and the
watcher running at the time logged "caddy reloaded successfully" anyway
and latched the failed config as applied — every later scan then saw
"caddyfile unchanged, skipping reload" and did nothing, for the rest of
the outage. Separately, the failed load itself moved Caddy's admin
listener off loopback (`0.0.0.0:26293`, observed live via `ss`), because
Caddy replaces the running admin server from the *new* config before that
config's apps are provisioned — a property of Caddy's own reload sequence
that a rejected load does not, by itself, protect against.

**Decision.** Two independent mechanisms, addressing the two distinct
failure shapes above. First, a failed `/load` is never latched as applied:
rejected loads (the admin process answered, the config itself was
refused) now retry on a fixed 15s timer, indefinitely — verified safe
because Caddy ≥2.11 keeps both the admin server and the previously-running
config intact across a *rejected* load (empirically confirmed against
2.11.4: bad `/load` → 400, admin still 200, site still 302 — the 19:28
admin hijack was specific to the old watcher/config combination, not an
inherent Caddy behavior). Second, ≥4 consecutive **transport**-level
failures — admin unreachable from this process at all, a materially
different failure than a rejected-but-answered load — make the watcher
report fatal and exit 1; the container's `restart: unless-stopped` policy
then recovers with a fresh minimal admin config rather than a process
stuck retrying against an admin it can no longer reach. The generated
Caddyfile also now pins an explicit `admin <loopback-addr>` block on every
load (ADR-053's sibling change, `deploy/caddy/ingress/generator.go`), so a
loaded config — rejected or not — can never move the listener off
loopback again; this is the control that actually closes the
`0.0.0.0:26293` shape, independent of the retry logic.

**Consequences.** A boot-order race (network not yet up, a container
started before its dependencies) now self-heals within 15 seconds instead
of wedging until the next container event forces a rescan — closing the
exact mechanism of the 2026-09-01 reboot outage. The retry-forever design
has a known limit, recorded rather than hidden: a *persistent* rejection
(bad OAuth credentials, an operator-authored `Caddyfile.base` that
collides with the new `admin` block) becomes a silent site-down that
retries every 15s indefinitely with no backoff or escalation past a
`log.Error` line — deferred as a follow-up, not fixed here. The retry
tests were mutation-proven against exactly the class of test-theatre a
review round caught first (deleting the retry call, or the fatal path's
early return, left the original test suite green; the corrected tests go
red under both mutations).

Source: deploy/caddy/ingress/watcher.go, deploy/caddy/ingress/generator.go,
deploy/caddy/ingress/watcher_test.go.
Verification: .agents/docs/changes/2026-09-01-redfix-hardening-and-versioning.md,
.agents/docs/reviews/2026-09-01-review-redfix-round2.md (round 3, mutation-proven),
.agents/docs/testing/2026-09-01-verify-redfix-round2.md.

---

## ADR-055: No backup layer of any kind — durability is git (2026-09-01)

**Context.** Operator decision, same evening: "No backup at all. If a user
needs backup they should commit to git." R13 (as amended 2026-08-30) had
mandated two layers — ADR-044's per-project in-app archive/restore and a
planned restic→GCS whole-state DR layer — and the platform is still
prototyping. The same-day state-loss incident
(`.agents/docs/changes/2026-09-01-incident-state-loss.md`) sharpened the
call: the DR layer's own credentials (`/etc/styrman/{backup-sa.json,
restic-pw}`) lived only under `/etc/styrman`, so when that directory was
deleted alongside `/var/lib/styrman`, the backup meant to protect the data
shared its fate with the data — the only reason the GCS snapshot stayed
recoverable in principle was an out-of-band copy of the restic password in
the operator's password manager, not anything the DR layer itself
guaranteed.

**Decision.** Both ADR-044 layers are removed, in-app and host: the
`internal/backup` package, its five `/api` routes, `FromBackup`
project-seeding, the backup cadence goroutine, the config knobs, and every
UI surface (Sidebar action, Settings section, New-project option); and
`deploy/backup/*` plus host-setup's restic install/unit/provisioning step.
A host is rebuilt from `deploy/host-setup.sh` plus the repo, not restored
from a snapshot. Wipe-delete is now plainly irreversible — the safety
snapshot it used to take before destroying a project's files is gone, so
the `DeleteProjectDialog` wipe checkbox defaults **unchecked** rather than
pre-checked, making the destructive path opt-in. This supersedes ADR-044
outright and amends R13's backup mandate, with the operator's approval
recorded in the same commit as the code change (REQUIREMENTS.md changes
require explicit approval on record; the reviewer treated shipping code
against the old R13 text as a blocking violation until the amendment
landed).

**Consequences.** The only durability guarantee left is git — commit and
push. Two things fall out of this that are NOT code follow-ups but operator
actions: the orphaned archive directory `/var/lib/styrman/backups/` (found
live by security review — unencrypted tar.gz files containing per-project
homes, i.e. materialized ssh keys, gcloud ADC and agent tokens, now
unreachable by any code path) needs shredding on the host, and the GCS
bucket + its service account need deleting in the GCP project — neither of
which any agent performs (`/var/lib/styrman` is off-limits per ADR-052, and
no `gcloud` runs on this host). The "create project from backup" seeding
path is gone entirely, not replaced.

Source: internal/orchestrator/orchestrator.go, internal/httpapi/
credharvest.go, deploy/host-setup.sh, REQUIREMENTS.md (R13),
web/src/components/DeleteProjectDialog.tsx.
Verification: .agents/docs/changes/2026-09-01-remove-backups-and-token-auth.md,
.agents/docs/reviews/2026-09-01-review-no-backups.md,
.agents/docs/reviews/2026-09-01-security-no-backups.md,
.agents/docs/testing/2026-09-01-verify-no-backups.md.

---

## ADR-056: Auto-updater authenticates with a fine-grained GitHub token, not an SSH deploy key (2026-09-01)

**Context.** Operator decision, same evening: "the host should not have a
key." ADR-051 hardened the SSH deploy-key path (host-key pinning, signed-tag
verification) but never questioned the key's presence itself — and that
path had already cost two TOFU (trust-on-first-use) incidents in the same
day's review cycles (ADR-051's own correction paragraph records one; the
other was `host-setup.sh` re-introducing `accept-new` at provisioning time
after the fetch-time TOFU was closed). A private key that lives on the host
forever is a standing liability independent of how carefully the fetch that
uses it is pinned.

**Decision.** The updater's git auth is now a fine-grained GitHub token,
scoped to this one repository at `Contents: Read-only`, stored at
`/etc/styrman/gh-token` (root-owned, mode `600` asserted before every use,
refused otherwise). The origin is HTTPS, replacing the pinned-`known_hosts`
SSH trust anchor with TLS's public CA set. A URL-scoped, per-invocation git
credential helper answers only for `github.com` (an unscoped form was shown
in review to hand the token to a probe for any host); the token's file path
is passed via the helper's own exported environment rather than
interpolated into the git-evaluated string; inherited credential helpers
are always cleared, with or without a token file present; and
`GIT_TERMINAL_PROMPT=0` turns a missing or expired token into a loud,
immediate failure rather than a hang — fine-grained tokens expire by design,
and that expiry is treated as a feature (forces rotation) rather than
worked around.

**Consequences, stated honestly.** A bearer token has no possession
binding: reading `/etc/styrman/gh-token` once yields full repo-read from
any machine on the internet, with no host-key, client-certificate, or IP
anchor requiring the attacker to also reach GitHub over the original
channel — an SSH private key at least required that. Accepted as a
roughly-neutral trade, tilting positive: both mechanisms are repo-scoped
and read-only, the token expires and is one-click revocable with an audit
trail from the GitHub UI (an SSH key is neither), and the SSH path's own
TOFU history in this project outweighs the possession-binding loss. This
revises ADR-051's *prerequisites* (the transport and trust anchor the
updater's `git fetch` relies on) — ADR-051's signed-tag requirement
(`git verify-tag` against a pinned SSH allowed-signers file, checked before
every checkout including rollback) is unchanged and remains the control
that actually gates what gets built and run as root. On the live host: the
deploy key was revoked, its key files and `known_hosts` deleted, and
`/opt/styrman` re-pointed at the HTTPS origin — the 5-minute update timer
now fails loudly at every fetch until the operator mints and installs the
token, which is deliberate, not a regression.

Source: deploy/update/styrman-update, deploy/host-setup.sh (step 8c).
Verification: .agents/docs/changes/2026-09-01-remove-backups-and-token-auth.md,
.agents/docs/reviews/2026-09-01-review-no-backups.md,
.agents/docs/reviews/2026-09-01-security-no-backups.md,
.agents/docs/testing/2026-09-01-verify-no-backups.md.

---

## ADR-057: The updater installs itself from the verified release — the host's updater and its systemd units are release-controlled state (2026-09-02)

**Context.** Three manual reinstalls of `/usr/local/bin/styrman-update` and its two systemd units
in two days (2026-09-01/02). The updater deploys the container stack from a signature-verified
checkout (ADR-051), but its own script and units were `host-setup.sh` provisioning artifacts,
never touched again after initial install — so any release that changed `deploy/update/*` shipped
everything except itself, silently reverting to the old updater behavior until an operator noticed
and hand-copied the new files onto the host.

**Decision.** After a health-checked deploy — both the main path and a successful rollback —
`self_install()` installs `deploy/update/styrman-update`, `styrman-update.service`, and
`styrman-update.timer` over the host's copies, installed from the verified git object at HEAD,
never the worktree: `differs()`/`atomic_install()` read each file via
`git show "HEAD:deploy/update/<f>"` (`HEAD` being the tag or trusted SHA `deploy()` just checked
out and verified), not from the checkout directory's on-disk bytes — the checkout stays a mutable,
writable directory after verification, so reading from the worktree would let a local user with
write access to it swap a file post-verification and have it installed anyway. A byte-identical
object is left alone; a changed one is written to a temp file and moved into place with `mv -fT`
(atomic, new inode, and a directory at the target path fails loudly instead of silently absorbing
the file), so the currently running script finishes this run on its old inode and the replacement
takes effect on the next tick; `systemctl daemon-reload` runs only when a unit actually changed.
Targets are pinned by the systemd unit with no in-script default
(`Environment=STYRMAN_UPDATER_BIN=/usr/local/bin/styrman-update`,
`Environment=STYRMAN_UPDATER_UNIT_DIR=/etc/systemd/system`), the same shape ADR-052 already
established for `STYRMAN_UPDATE_STATE_DIR`. Critically, this only runs when signature enforcement
is on (`REQUIRE_SIGNED=1`): with the opt-out engaged the deployed tree is by definition
unverified, and an unverified tree must never become the mechanism that decides whether
signatures are checked next time.

**Consequences.** No new capability is created — a signed release already runs arbitrary root
code via `docker build`/`docker compose` on this host, so a hostile signed tag was already a
total compromise before this change. What does change: a signed release can now also rewrite its
own hardening and its own replacement mechanism on the very next run, so the signer allowlist
(`/etc/styrman/allowed_signers`) is now the single trust root for the host's updater
*configuration*, not just for what gets deployed. Accepted by design (security review). The
`REQUIRE_SIGNED` opt-out path never self-installs, so an operator who has deliberately disabled
signing keeps manual control of the updater itself as a deliberate consequence, not an oversight.

Source: deploy/update/styrman-update, deploy/update/styrman-update.service.
Verification: .agents/docs/changes/2026-09-02-v1.17.3-updater-self-install-and-vulncheck.md,
.agents/docs/reviews/2026-09-02-review-v1.17.3.md (round 3),
.agents/docs/reviews/2026-09-02-security-v1.17.3.md (round 3),
.agents/docs/testing/2026-09-02-verify-v1.17.3-round3.md (final record).

---

## ADR-058: Project repo provisioning must run after the credential-broker attach and inside a login shell (2026-09-02)

**Context.** Live incident: in `createProject`'s background goroutine, `EnsureSelfAttached` —
which joins the styrman-server container to the project's bridge, the ONLY route to the credential
broker (ADR-030/032) — ran AFTER repo provisioning. Every `cloneUrl` clone and every
`github: private|public` repo create ran against an unreachable broker: no ssh key materialized,
no git-credential-helper answer, no GH_TOKEN. Confirmed in the live server log: `13:42:04.888
clone failed … Permission denied (publickey)` followed 35ms LATER by `13:42:04.923 cred broker:
served env credentials`, and `13:44:20.168 clone failed … could not read Username` followed 32ms
later by the broker's first successful serve. The failure was logged at `logger.Warn` only,
silently degrading to an empty repo with nothing shown in the UI (no provisioning error reported
to the user, no fallback message, no diagnostic).

**Decision.** Project repo provisioning is split into an ordered sequence that MUST be
testable without a Docker daemon: (1) pause briefly so the 201 response lands before the
network reconfigures, (2) attach to the credential-broker bridge, (3) wait from INSIDE the guest
until the broker answers via `curl $STYRMAN_CRED_BROKER/healthz` in a plain (not login) shell,
(4) run the clone or GitHub repo create in a LOGIN shell so `/etc/profile.d/zz-styrman-credentials.sh`
materializes the ssh key and git identity. Step 1 is a deployment artifact (provisionDelay tuned at
runtime); steps 2–4 are correctness constraints. The regression test `TestProvisionRepoAttachesBeforeAnyCredentialedCommand`
directly asserts the attach→broker-wait→provision order on every build; if the order regresses,
the test fails immediately (no live incident, no operator discovery path).

**Consequences.** Extraction of `provisionSpec` and `provisionRepo` into `internal/httpapi/provision.go`
and a new test file `provision_test.go` makes the step order explicit and testable. Tests run the
real provisioning logic with injected attach/run funcs, so they exercise every condition and edge case
(clone failure, GitHub repo create, plain init) without a working Docker daemon. A provisioning failure
now surfaces as a `provisionError` field on the project API and a red sidebar badge so the user sees
WHY a project came up empty instead of guessing. Accepted: every clone failure is now visible; the
fallback `git init` still leaves a usable workspace (R13); the code is testable and its correctness
cannot regress.

Source: internal/httpapi/provision.go, internal/httpapi/provision_test.go.
Verification: .agents/docs/changes/2026-09-02-v1.18.0-project-create-provisioning.md,
.agents/docs/reviews/2026-09-02-review-project-create-provisioning.md,
.agents/docs/reviews/2026-09-02-review-project-create-provisioning-rereview.md,
.agents/docs/reviews/2026-09-02-security-project-create-provisioning.md,
.agents/docs/testing/2026-09-02-verify-project-create-provisioning.md,
.agents/docs/testing/2026-09-02-verify-project-create-provisioning-rerun.md.

---

## ADR-059: Credential-bearing URLs are refused at the API boundary, not accepted and redacted (2026-09-02)

**Context:** Clone URLs that embed a password or token (`https://user:token@host/repo`) are accepted by the clone endpoint and then redacted from logs. The security review discovered that `git clone` persists the entire URL — including the userinfo — verbatim into the cloned project's `.git/config` on the host, where nothing redacts it. A later push to that host keeps the token as git history. The credential broker (ADR-030/032) makes embedded credentials unnecessary: it delivers the user's ssh key per request and answers git-credential-helper queries per HOST, so `PutGit(user, GitCred{Host})` onboards any host via `gh auth token --hostname HOST` without the user typing anything into a URL.

**Decision:** Clone URLs that contain userinfo with a password character (`:`) are REFUSED at the `POST /api/projects` API boundary with a 400 Bad Request — the entire class of vulnerability is deleted rather than mitigated with redaction. The check is dual-sided: `cloneURLHasSecret` in `internal/httpapi/server.go` (authority parsing via `net/url.Parse`, userinfo split at the LAST `@` of the authority like git/curl do) at the server, and `cloneURLError` in `web/src/lib/api.ts` client-side for immediate feedback before the round trip. A bare USERNAME is still allowed (`ssh://git@host/o/r` is the standard SSH form, and usernames without `:` are not credentials). The check is specific to credential-bearing USERINFO, not to arbitrary URLs containing the character `:` — `http://host:8080/repo` and `https://host:443/repo` are accepted.

**Consequences:** No project can clone a URL with embedded credentials, closing the `.git/config` persistence class entirely. The credential broker is now the ONLY credential path (other than manual git config or SSH key discovery on the agent runner). The operator's test case: SSH clone of `git@github.com:kesokaj/styrman-harness.git` succeeds (populated the workspace, no provisioning error) after the v1.18.0 provision-order fix, demonstrating that the credential broker is sufficient for the tested scenario. An SSH clone of a repo on an unknown host (where the broker has never seen a credential) would fail at git-permission time, not at clone-acceptance time — the test case does not cover that edge, which remains a constraint recorded in the operational docs: any host needs a seeded credential (`gh auth token --hostname HOST`) before first clone.

Source: internal/httpapi/server.go, web/src/lib/api.ts.
Verification: .agents/docs/changes/2026-09-02-v1.18.1-project-create-and-secure-clone-urls.md,
.agents/docs/reviews/2026-09-02-security-project-create-provisioning.md (section on credential URL handling).

---

## ADR-060: A microVM runtime is a precondition of the containment model, asserted at load (2026-09-02)

**Context:** ADR-001 built the security model around Kata microVMs as the isolation boundary — projects run with `CapAdd: SYS_ADMIN, NET_ADMIN, NET_RAW` and `apparmor=unconfined` (`internal/orchestrator/orchestrator.go:759`), which are escape vectors unless confined to a guest kernel. The runtime was an unvalidated free string (`STYRMAN_RUNTIME` env var, defaulting to `mvm`), and the system assumed it was a Kata variant only through code review discipline, never through validation at startup. A typo or misconfiguration could silently set `STYRMAN_RUNTIME=runc`, keeping all the caps but losing the microVM boundary — a critical containment failure.

**Decision:** `config.Load()` (line 211 onward) now allowlists VM runtimes at system startup: `mvm, kata, kata-runtime, kata-qemu, kata-clh, kata-fc`. `mvm` is this deployment's Kata runtime name (registered in daemon.json by `deploy/host-setup.sh`); the `kata-*` names are upstream's defaults. Any other value is rejected with a fatal error before any project containers are created, unless `STYRMAN_ALLOW_UNSAFE_RUNTIME=1` is explicitly set (which logs a loud warning to stderr and proceeds). This is not optional for normal deployment: a microVM is the ONLY safe runtime for this architecture.

**Consequences:** The containment model is now asserted, not assumed — a misconfigured host cannot boot styrman with a non-VM runtime unless the operator explicitly overrides and accepts the risk. The cost is one allowed-values list that must stay in sync with the vendors' actual microVM runtime names. A future runtime change (e.g., gVisor, firecracker) requires this list to be updated, not inferred from environment state. The escape hatch (`STYRMAN_ALLOW_UNSAFE_RUNTIME=1`) exists for development scenarios (e.g., testing on runc locally) and is intentionally noisy to prevent accidental production misconfigurations. Enforced in v1.19.0.

Source: internal/config/config.go, internal/orchestrator/orchestrator.go, cmd/styrman/main.go.
Verification: .agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md (STYRMAN_RUNTIME allowlist section).

---

## ADR-061: A release tag is verified against the COMMIT, never the working tree (2026-09-02)

**Context:** v1.18.0 was tagged on 2026-09-02 at 14:38. The commit `b321f35` staged `server.go` (which called a new method) but NOT `provision.go` (which defined it). Every local `/preflight` gate passed because they all ran against the complete WORKING TREE, which was correct. The commit itself was a subset — this commit/tree divergence was silent at tag time and only surfaced when the auto-updater fetched the tag and attempted to build the committed tree, which failed with a compile error. Every released artifact since then has been built and verified against working trees that happened to be correct, never against the commits that were actually tagged.

**Decision:** A new `deploy/verify-commit.sh` script, run as a pre-tag/pre-push gate in the release path, builds the commit in a detached worktree (never the mutable checkout). The gate runs: Go build/vet/race, the separate vendored ingress module, gofmt, frontend check/lint/test, a `web/dist` freshness rebuild-and-compare, and a real server image build against the committed tree. If any step fails, the commit is rejected and will not be tagged. This replaces the assumption "the working tree is correct" with a hard requirement "the commit being tagged IS the thing that was verified." Tag and push only follow a passing gate.

**Consequences:** Commits that are syntactically broken or whose `web/dist` is stale are caught before tagging, not during operator rollback or after the failed deploy is already public. The cost is a second build pass (working tree build for the gate, commit build for the release); the benefit is that tag → deployed artifact becomes deterministic and idempotent. Commits are no longer publishable before they have been verified to build on their own. Enforced in v1.19.0.

Source: deploy/verify-commit.sh, .agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md (Verification section).
Verification: v1.18.0 incident (root cause: commit not verified before tag); v1.19.0 pre-tag gate confirms `deploy/verify-commit.sh` runs successfully on the three bundled commits.

---

## ADR-062: The seeded agent harness is styrman-harness, installed by sh-agent from a pinned mirror — no hooks, no scripts, no settings files (2026-09-03)

**Context:** ADR-046 made `image/harness/` a verbatim mirror of `devshell_prompt/harness` with `STYRMAN_VERSION` as the only delivery path, and ADR-047 trimmed `/usr/local/bin` from the seeding invocation's PATH to dodge that harness's 60 s `agy -p` self-check. The operator rebuilt their harness as `styrman-harness` (28 skills, 7 subagents, a Go installer `sh-agent`, no hooks or scripts, the agy rules at `~/.gemini/config/rules/AGENTS.md`) and directed on 2026-09-03 that the image seed it and that every old occurrence go — "even settings.json files for gemini and claude". The old harness had also written a `settings.json` that put Claude Code in auto permission mode; the settings page's harness roster had been empty since v1.15.0 because it read the never-seeded base home.

**Decision:** (D1) `image/harness/` is a verbatim mirror of the `styrman-harness` checkout at the commit recorded in `image/harness/COMMIT`, refreshed by hand (`VENDORED.md`); `image/Dockerfile` builds `sh-agent` from it in a builder stage pinned to `deploy/Dockerfile.server`'s Go toolchain and ships `/styrman/bin/sh-agent`. (D2) `image/entrypoint.sh` seeds every non-MINIMAL project home with `sh-agent install --repo /styrman/harness --home $HOME` as the project user, write-once on the existing `~/.styrman/.harness-version` stamp; the ADR-047 PATH trim is retired because `install` makes no CLI or network call. (D3) A home seeded by the previous harness — stamp present, no `~/.config/sh-agent/state.json` — is migrated first by `image/configs/harness-migrate.sh`, which MOVES every path the old setup scripts wrote (both vendors' `settings.json` included, as whole files) into `~/.styrman/harness-backup-<ts>/` and never deletes; the stamp is written only when migration and install both succeed. (D4) `sh-agent update` is not used inside VMs; the image remains the delivery path. (D5) The settings roster is read from the user's newest project home that carries the manifest; agy agents from `.gemini/config/agents`; the old `*.bak.<ts>` copies are ignored. (D6) `integration.sh` asserts the seeded harness and a boot→ready bound (R1's 15 s hard limit). (D8) Nothing in `image/` seeds a `settings.json` for either tool.

**Consequences:** Claude Code inside a project runs in its default permission mode (the operator's own machine runs the same way); `agy` reads its rules from `~/.gemini/config/rules/AGENTS.md` and has no `GEMINI.md`. Migrated homes keep the old artifacts under `~/.styrman/harness-backup-<ts>/` — recoverable, at a disk cost of roughly the old harness's size — and a file a user had dropped into a harness-owned tree moves with it. `sh-agent status` inside a project prints a blank commit (the mirror has no `.git`); `image/harness/COMMIT` is the record and the release changelog names it (`versioning.md`). The R5 sentence "vendored from `devshell_prompt/harness`" in `REQUIREMENTS.md` is out of date; the one-line edit awaits the operator's approval (REQUIREMENTS.md is immutable without it). Supersedes ADR-046's mirror source and ADR-047 entirely; ADR-046's delivery model (version bump = re-seed) stands.

Source: image/Dockerfile, image/harness/{COMMIT,VENDORED.md}, image/entrypoint.sh, image/configs/harness-migrate.sh, internal/httpapi/setup.go, web/src/routes/settings.tsx, .agents/docs/testing/styrman/integration.sh.
Verification: .agents/docs/plans/2026-09-03-image-harness-replacement.md (phases and e2e criteria), .agents/docs/testing/2026-09-03-verify-harness-phase{1,2,3}.md, .agents/docs/reviews/2026-09-03-{review,security}-harness-phase*.md, .agents/docs/changes/2026-09-03-harness-replacement-phase*.md.

---

## ADR-062 — amendment (2026-09-04): R5 wording updated with the operator's approval

**Context:** ADR-062 left `REQUIREMENTS.md` R5 reading "vendored from `devshell_prompt/harness`" pending the operator's explicit approval (REQUIREMENTS.md is immutable without it). The operator approved on 2026-09-04: "change requirements, this harness we are adding is the new one now. Full rip and replace."

**Decision:** R5 now names the harness as a mirror of `styrman-harness` at `image/harness/COMMIT`, installed by `sh-agent`, with no hooks, scripts, or settings files; the previous wording is kept as history in the same sentence. Nothing else in R5 changes.

**Consequences:** The requirement and the implementation agree; ADR-062's last open consequence is closed.

Source: REQUIREMENTS.md R5, ROADMAP.md "Now".
Verification: .agents/docs/changes/2026-09-03-harness-replacement-phase4.md (the proposal), this session's release entry.

---

## ADR-063: The ingress session lifetime is set deliberately to a working day (2026-09-04)

**Context:** The authentication portal never set `crypto default token lifetime`, so caddy-security's 900 s default governed every session. Before BUG-001 that expiry surfaced as "Failed to fetch" on every REST-backed view until F5; after it, as a toast and a full page reload every 15 minutes of use (in-memory UI state lost, terminals reconnect). The security review of BUG-001 (`reviews/2026-09-03-security-fail-to-fetch.md`, observation) noted that the lifetime is not the revocation control here — `authGuard` re-checks the userstore on every request, so removing a user takes effect immediately regardless of the JWT's remaining life — and recommended choosing the value deliberately. The operator approved a fix on 2026-09-04 ("fix then commit to the newest version").

**Decision:** `deploy/caddy/ingress/generator.go` writes `crypto default token lifetime 28800` (eight hours, `SessionLifetimeSeconds`) into the `authentication portal` block. A generator test asserts the directive is present.

**Consequences:** A signed-in browser reloads to re-authenticate at most once per working day instead of every quarter hour. The trade: a stolen or replayed cookie stays valid up to eight hours, zone-wide (`cookie domain` is the base domain), where it was fifteen minutes; authorization is still re-checked per request, so this widens exposure of an already-compromised browser session, not of a removed user. Changing the value is one constant plus the test. Because `STYRMAN_JWT_KEY` pins the signing key, sessions SURVIVE a Caddy redeploy: tokens issued before this release keep their old 900 s expiry, tokens issued after it live eight hours, and there is no deny list — rotating `STYRMAN_JWT_KEY` is the only way to force every session out at once (security review 2026-09-04).

Source: deploy/caddy/ingress/generator.go (`SessionLifetimeSeconds`, `writeSecurityBlock`), deploy/caddy/ingress/generator_test.go.
Verification: this release's changelog entry; `(cd deploy/caddy/ingress && go test ./...)`.

---

## ADR-064: The setup VM's home is homes/<user>/base; the tenant directory is never mounted into any guest (2026-09-04)

**Context:** ADR-031 gave every project its own home at `HomesDir/<user>/p/<uuid>` and left the setup VM (the hidden onboarding microVM that runs `styrman-setup`) mounting `HomesDir/<user>` itself. From inside a project VM nothing is nested; from inside the setup VM, `~/p/` is every project home the user owns, writable as root. A symlink planted there was followed by the server's roots (BUG-002), by the transcript janitor and by Docker's bind-source resolution on the next project start (BUG-003); v1.20.0 guarded all three, and the consolidated release review recorded the remaining check-to-mount window as an accepted medium (`reviews/2026-09-04-security-v1.20.0.md`). The operator chose to remove the layout rather than keep defending it (2026-09-04: "Go with your recommendation: un-nest").

**Decision:** `Config.UserHome(user)` — the setup VM's home — is `HomesDir/<user>/base`. `Config.TenantDir(user)` = `HomesDir/<user>` is the server-only parent of `base/` and `p/`; no guest mounts it. At startup the server first force-removes every setup container whose home bind is not the new base (`RemoveStaleSetupVMs`), so no guest holds the old mount, then `migrateBaseHomes` moves every tenant's pre-ADR-064 contents (everything except `base/` and `p/`) into `base/` — every step through an `os.Root` at the tenant dir, so a `base` swapped for a symlink cannot redirect a rename; a `base` that is not a real directory is refused and a colliding entry is left in place; the log carries counts, never names. `migrateLegacy` creates the tenant dir before renaming the pre-multitenancy home into `base/` and skips a half-migrated tenant. `EnsureSetup` checks on every path that `base/` has no symlink component, recreates a setup container whose home bind is stale, and creates `base/` through the same confined root. The v1.20.0 guards (`files.Trust`, `projectHomeIsReal`/`realBelow`, the confined janitor and `MkdirAll`s) stay as a second line. Plan: `plans/2026-09-04-unnest-project-homes.md`.

**Consequences:** No guest mount contains another guest's home, so the class behind BUG-002/003 has no place to occur and the accepted TOCTOU medium is closed by construction. The migration is one-way: a downgrade to v1.20.x would mount the tenant dir again and find the sign-in state under `base/` (readiness shows not-set until moved back by hand). A setup VM running through the upgrade is force-removed by the startup sweep before the migration touches its mount; an in-progress sign-in is interrupted once, nothing is lost (the broker holds what was harvested). Ships in v1.21.0.

Source: internal/config/config.go (`TenantDir`, `UserHome`), cmd/styrman/main.go (`migrateBaseHomes`, `migrateLegacy`), internal/orchestrator/orchestrator.go (`EnsureSetup`, `setupHomeStale`), internal/httpapi/server.go (`createProject`).
Verification: .agents/docs/changes/2026-09-04-unnest-project-homes-phase1.md and -phase2.md, .agents/docs/testing/2026-09-04-verify-unnest-phase1.md, the Phase 2 upgrade fixture and release verification.

---

## ADR-065: Per-project local-network exception — intent from the server, enforcement by the root firewall (2026-09-08)

**Context:** R12 with ADR-029 and ADR-036 made every project internet-only: the root-owned host firewall drops all routed traffic from any `sdb-*` bridge to the private ranges, re-applied every 60 s by `styrman-firewall.timer`. The operator asked on 2026-09-08 for "a toggle to enable local network traffic per project. It should be able to change during runtime on or off." Two constraints shaped the answer. The styrman server runs as the operator's uid in its own network namespace with no `NET_ADMIN` and no iptables binary (`deploy/docker-compose.yml`, `deploy/Dockerfile.server`), so it cannot change a rule; only root on the host can. And nothing delivers host-setup changes to a running host: the auto-updater self-installs only `deploy/update/*`, the release procedure has no host step, and on 2026-09-08 the live script (mtime 2026-08-30) was found to carry four DROPs where the repo's heredoc has carried six since 2026-09-02 (100.64/10 and 198.18/15 missing). Research: `plans/2026-09-08-per-project-lan-access.md` (seven subsystem readers, three independent designs, three judges, a synthesis and an adversarial critic whose seven verified refutations are folded in; cold-reader check passed on round two).

**Decision:** Six parts. (1) **Intent lives in the store, projected as a name.** `Project.LanAccess` in `projects.json`; the server mirrors it as an empty marker file `<StateDir>/netpolicy/lan/<uuid>` — server-only, never a bind source — that root reads by NAME only (regular file, not a symlink, lowercase UUIDv4 regex); contents are never parsed by anyone. (2) **Root decides what "local" means and applies it.** The firewall script moves from a heredoc in `host-setup.sh` to `deploy/firewall/styrman-firewall.sh`, carries `STYRMAN_FW_VERSION`, takes every path from the unit's environment (ADR-052), runs today's base rules verbatim first, then reconciles: for each marker and each LAN prefix in root-owned `/etc/styrman/firewall.conf` (discovered once by `deploy/firewall/install.sh` from the default-route uplink, validated on every run — prefix /16..30, host bits zero, no overlap with 172.16/12, 169.254/16, 100.64/10, 198.18/15, 127/8, 0/8, 224/3 or any Docker bridge's connected route) one rule at the head of `STYRMAN-FWD`: `-i sdb-<hash> -o <LAN_DEV> -s <bridge subnet> -d <cidr> -m comment --comment "styrman-lan:<uuid>" -j RETURN`. RETURN, not ACCEPT, so Docker's own cross-bridge DROP still runs; `-s` and `-o` because `rp_filter` is loose and MASQUERADE matches only the bridge subnet, so a VM with `NET_ADMIN` could otherwise spoof a LAN source. Stale, malformed and misplaced tagged rules are deleted by rule NUMBER, tighten-before-loosen; a no-op run performs reads only. This is the one class of rule the self-healing script deletes on demand — ADR-036's "never delete a live rule" holds for base rules. (3) **Sub-second application without root code in the server.** The server drops an empty trigger file into the root-parented spool `/run/styrman-firewall/spool` (operator-owned 0700 under a root 0755 parent); `styrman-firewall.path` (`DirectoryNotEmpty`) starts the same oneshot the timer starts; the script drains the spool first and again on exit so no exit path can leave the path unit re-triggering; the 60 s timer stays the backstop. Root writes `/run/styrman-firewall/status` atomically; the server derives `off / pending / on / error / unavailable` from root's word, never from the request lifecycle, and refuses ENABLE with 503 whenever root's support is absent, stale, outdated (`version` below `netpolicy.MinScriptVersion`) or unconfigured; DISABLE is always accepted. (4) **Authorization.** Enable requires an admin acting on a project in their own store (no admin path into another user's project exists and none is added); the owner may always disable. (5) **What stays closed, for a LAN-enabled project too:** the host itself (`STYRMAN-IN` untouched — it runs sshd and Caddy on the LAN address, the operator has passwordless sudo, and the project holds its owner's SSH key through the broker), host-published Docker ports (the drop sees the post-DNAT container address), every other project and system bridge, the ingress bridge and broker, cloud metadata, the tailnet; LAN hosts cannot initiate connections in; the setup VM is structurally excluded because its network name is not a uuid. Two conf keys, `LAN_ALLOW_HOST` and `LAN_ALLOW_PUBLISHED_PORTS`, are reserved (parsed as 0) for a later, separate ADR. (6) **Delivery.** `deploy/firewall/install.sh` is the narrow idempotent installer `host-setup.sh` §3b delegates to; `styrman-update` gains a tag-gated `host_converge()` that installs `deploy/firewall/*` from the verified git object on every tick, so firewall changes ship with releases from now on. Supersedes, in place, ADR-029's "nothing else" and the unconditional reading of ADR-036's egress drops; both stay as history.

**Consequences:** R12 is amended (entry below) with the exception and with the corrected list of dropped ranges. The design fails closed by construction: base = block, per-project = allow, so an external flush, a dead timer, an invalid conf or a missing host half can only remove access, never grant it. New root-parsed input surfaces exist but are bounded to directory names and a root-owned conf; the security agent reviews the script and a netns test carries the hostile cases. Root code now ships by signed tag; the updater already builds and runs images as root from the same object, so no new trust edge, and the allowed-signers file remains the control. LAN access is by IP only (guest resolvers stay 8.8.8.8/1.1.1.1). Turning access off drops the next outbound packet of every open flow (`DOCKER-USER` runs before any conntrack accept); clients see timeouts. A UI chip flags the standing exception. The pre-existing four-versus-six drift on this host is converged in Phase 0, and the delivery gap that produced it is closed by part (6). Ships across five phases; see the plan.

Source: .agents/docs/plans/2026-09-08-per-project-lan-access.md; from Phase 1 on, deploy/firewall/, internal/netpolicy/, internal/httpapi/network.go.
Verification: the per-phase entries in .agents/docs/changes/ and the Phase 5 log in .agents/docs/testing/ (to be written); .agents/docs/reviews/2026-09-08-cold-reader-lan-access-plan.md.

---

## ADR-065 — amendment (2026-09-08): R12 wording updated with the operator's approval

**Context:** The plan proposed the exact R12 text under its decision D1, recommending approval verbatim, and presented all seven decisions with the recommendations the text was written under. The operator's reply on 2026-09-08 was "Go do work". That reply is recorded here verbatim as the approval, with its scope stated: it answered the seven-decision list as presented, including D1's "approve the text under R12 amendment verbatim". REQUIREMENTS.md is immutable without explicit approval (REQUIREMENTS.md:4); ADR-055 treated code shipped against the old text as blocking, so this entry lands with the R12 edit and before any implementation.

**Decision:** R12 gains the local-network exception paragraph as proposed, and its enforcement clause now names the six dropped ranges (the script has dropped 100.64/10 and 198.18/15 since 2026-09-02; the text said "RFC1918 + metadata"); the previous wording is kept as history in the same sentence. Nothing else in R12 changes: the cross-project sentence, the per-project bridge, `ip_forward=0` and the host-networking clause are unconditional.

**Consequences:** The requirement and the design agree before code exists. If the operator later judges "Go do work" too thin an approval for a requirements change, the edit is one commit to revert and this entry says so.

Source: REQUIREMENTS.md R12, ROADMAP.md "Now".
Verification: .agents/docs/plans/2026-09-08-per-project-lan-access.md (D1), this session's Phase 0 changelog entry.

---

## ADR-066: A LAN-enabled project gets full same-network parity, and gives up the SSH key for it (2026-09-08)

**Context:** ADR-065 shipped the per-project LAN switch in v1.23.0 with ON meaning "the configured prefixes, minus the host, minus host-published ports". The operator flipped it in a browser for the first time on 2026-09-08 — the one step that feature deliberately left unautomated — found `192.168.86.1` reachable and `192.168.86.58` not, and restated the intent: *"If lan on it is like they are on the exactly same network with full access."* The OFF state was confirmed correct in the same run and does not change. Two factual corrections came out of the design work and are recorded here because both were wrong in the project's own records. **First:** the reasoning in ADR-065 part (5) and in R12 held that the host must stay closed partly because "the project holds its owner's credentials" — i.e. the brokered SSH key would make a LAN-enabled project root on the host. That is false on this host: neither `/home/kesokaj/.ssh/authorized_keys` nor `/root/.ssh/authorized_keys` exists, so the key opens other hosts that trust it, not this one. The exposure `LAN_ALLOW_HOST` actually opens is `sshd` on `0.0.0.0:22` with `passwordauthentication yes` combined with `/etc/sudoers:50 %sudo ALL=(ALL:ALL) NOPASSWD:ALL` — a credential-guessing surface whose success is root, not instant root. **Second:** the rule shape `TODO.md` had carried since the v1.23.0 security review for published ports — `! -o sdb-+ ! -o <ingress br>` — cannot be written at all: iptables 1.8.11 (nf_tables) refuses a second `--out-interface` (`multiple --out-interface options not allowed`, probed live in a throwaway chain on 2026-09-08). Alternatives considered and rejected: true L2 membership (macvlan/ipvlan or a bridged uplink giving each LAN-enabled project its own DHCP lease and inbound reachability), rejected because it breaks the `sdb-*` interface naming every R12 rule keys on, breaks the credential broker's bridge-only listener and the port-preview proxy, and would make two LAN-on projects L2 peers of each other — contradicting R12's unconditional cross-project clause; and carving port 22 out of the host allowance, rejected as an exclusion list that loses to a host that keeps gaining services.

**Decision:** Five parts. (1) **Parity is outbound, not membership.** A LAN-enabled project reaches everything a peer on `LAN_CIDRS` reaches, host and host-published ports included; it still NATs out as the host and there is still no inbound path into the VM. (2) **Both new reaches are conf-gated and default to closed.** `LAN_ALLOW_HOST` and `LAN_ALLOW_PUBLISHED_PORTS` move from reserved-parsed-as-0 to real, with **absent still meaning 0**, so an existing deployment does not silently gain host access when the script converges; opting in is a human running `install.sh`. (3) **The exclusion is a positive destination allowlist, not a negation.** The host family is one RETURN per host address on `LAN_DEV` in `STYRMAN-IN`; the published-port family is one RETURN per *allowed* bridge subnet in `STYRMAN-FWD`, carrying `-m conntrack --ctstate DNAT`, where allowed means every Docker bridge that is neither `sdb-*` nor the configured `INGRESS_BRIDGE`. Unset `INGRESS_BRIDGE` skips the family entirely. This is forced by the `--out-interface` finding above and is the better shape anyway: it fails closed by construction. (4) **A LAN-enabled project is not served the operator's SSH key.** The credential broker withholds `~/.ssh/`-shaped file credentials from it, gated on `LanExposed` = the union of the project record, root's `Applied` set, and cannot-confirm — never the record alone, because a disable would otherwise re-serve the key while the firewall rule was still live for up to 60 s. This closes ADR-065 follow-up (g) (the transitive path: LAN-enabled project → a LAN host that trusts the key → back here), which is the risk it actually addresses; it is NOT what keeps the project off this host. (5) **Hardening the host's sshd is a deployment precondition, documented and operator-run, never automated.** A deployment that sets `LAN_ALLOW_HOST=1` must not accept password authentication on the uplink. The sequence — install the pubkey, prove a new session logs in with it while the current one stays open, only then set `PasswordAuthentication no` — is in `docs/deploy/remote.md`; doing it in one step risks locking the operator out of their own machine. Amends ADR-065 parts (5) and D3 in place; both stay as history.

**Consequences:** The switch becomes a larger privilege grant and the UI says so at the point of enabling, naming both consequences. `STYRMAN_FW_VERSION` goes to 4 and `netpolicy.MinScriptVersion` with it, so a host on the old script refuses to enable rather than silently under-applying. The reconcile machinery in `styrman-firewall.sh` is generalized over `(tag, chain, regex, args-builder)` before either family is added — three near-copies would put the next BUG-013-class bug in whichever copy was missed — and that refactor is behaviour-preserving by construction, its pass condition being `deploy/firewall/test.sh` passing unmodified. A LAN-enabled project loses ssh push; `GH_TOKEN` over HTTPS still covers github.com. Enabling LAN on a project that has already run means the key was materialized in that project's home: the server prunes the standard path, a copy the guest made elsewhere survives, and the honest statement is that the key was exposed to it.

Source: .agents/docs/plans/2026-09-08-lan-full-parity.md; deploy/firewall/, internal/netpolicy/, internal/credbroker/, internal/httpapi/, cmd/styrman/main.go.
Verification: .agents/docs/reviews/2026-09-08-review-lan-full-parity-plan.md (cold reader, round 1) and the round-2 report; per-phase entries in .agents/docs/changes/ (to be written).

---

## ADR-066 — amendment (2026-09-08): R12 wording updated with the operator's approval

**Context:** The plan proposed the exact replacement text for R12's exclusion sentence under "Proposed R12 amendment (NOT applied — needs approval)", after the two corrections above had been put to the operator and both recommendations accepted. The operator's reply on 2026-09-08 was "approve the R12 text and start phase 1". That is recorded here verbatim as the approval, with its scope stated: it approves the exact text as drafted in the plan, nothing wider. REQUIREMENTS.md is immutable without explicit approval (REQUIREMENTS.md:4), and ADR-055 treats code shipped against stale requirement text as blocking, so this entry lands with the R12 edit and before any implementation.

**Decision:** R12's exclusion sentence is replaced by the same-network-parity paragraph: conf-gated host and published-port reach, the sshd password-authentication prohibition, the narrowed exclusion list (now naming the ingress bridge explicitly), the NAT-only-outbound restatement, and the broker's withholding of the SSH key. The superseded wording is kept inline as history in the same sentence, as ADR-065's amendment did. Nothing else in R12 changes: the cross-project guarantee, the per-project bridge, `ip_forward=0` and the host-networking clause remain unconditional.

**Consequences:** The requirement and the design agree before code exists. If the operator later judges the approval too narrow for how the implementation lands, the edit is one commit to revert and this entry says so.

Source: REQUIREMENTS.md R12, ROADMAP.md "Now".
Verification: .agents/docs/plans/2026-09-08-lan-full-parity.md ("Proposed R12 amendment"), this session's changelog entry.

---

## ADR-066 — amendment (2026-09-08): the credential withholding LATCHES; the live-state question is removed, not answered

**Context:** ADR-066 part (4) withheld host-reaching credentials from a project that could *currently* reach the LAN, which the server answered with `netpolicy.Exposed` — the union of the project record, root's applied set, and "cannot confirm". Three review rounds found that this question has no reliable answer. Root's rule can outlive the record by up to a minute; a failed `ipt -D`, or a skipped reconcile after an unreadable chain, leaves the RETURN live while `count_applied` drops the project out of `lan=`, so the server reads "root confirms it is gone" and serves the key to a project that still has LAN reach — precisely the pairing part (4) exists to prevent (release review round 3, `reviews/2026-09-08-review-gate-round3-coldread.md`, high). Two earlier defects came from the same apparatus: the guard written against `netpol.Enabled()`, which a production default makes permanently true (so every host without a readable status file withheld from every project), and its replacement `Installed()`, which stat'd a directory that `docker-compose` bind-mounts into the server and therefore also could not be false. The operator's observation ended it: *"A project does not need to have its ssh key copied on the host. If enough if we have full network access."* The live state was being tracked so the key could come BACK; nobody needed it back.

**Decision:** The withholding latches. `Project.LanEverEnabled` is set the first time `LanAccess` is turned on and is never cleared — including by `RestoreLanAccess`, whose rollback means the marker write failed *after* the record saved, so the broker may already have stopped serving and un-latching is the one direction that can hand a key back by accident. `Server.LanExposed` reads that boolean and consults root's firewall status not at all. `netpolicy.Exposed` and `Dir.Installed` are DELETED; nothing in the credential path reads the status file any more. The UI says the withholding is permanent and that a new project is the way to get SSH push back. Supersedes part (4)'s live-state wording in place; the reasoning for withholding at all is unchanged (the transitive path, ADR-065 follow-up (g)).

**Consequences:** The highest open finding of the release gate is deleted rather than fixed, along with the whole class that produced three defects across three rounds — there is no window, no staleness, no dependency on a file root may not have written. A project that has ever had LAN access cannot push over SSH again without being recreated; `GH_TOKEN` over HTTPS still covers GitHub. This is strictly more honest than the previous design, which took on that complexity to restore access to a key the project had already held. R12's sentence ("A LAN-enabled project is not served the operator's SSH key by the credential broker") remains true as written and is not amended; a clarification that it also covers previously-enabled projects is OFFERED to the operator, not applied.

Source: internal/project/store.go, internal/httpapi/network.go, internal/netpolicy/{status,dir}.go, web/src/components/LocalNetwork.tsx.
Verification: TestServerLanExposedLatches, TestLanLatchSurvivesRollback; the gate report reviews/2026-09-08-preflight-v2.0.0.md records the finding this removes.

---

## ADR-067: The bridge interface prefix IS the network mode; ADR-065/066's root-mediated machinery is deleted (2026-09-09)

**Context:** The operator asked on 2026-09-08 for "a toggle to enable local network traffic per project" and on 2026-09-09 restated what it was meant to be: *"I just wanted you to give the option to enable local lan access as normal docker container have. Not copying a lot of keys and setting up scripts etc. If lan on, give NETWORK access from the project to the networks accessible in the docker host. If sandbox on. Only allow internet and nothing else."* ADR-065 (v1.23.0) and ADR-066 (v1.24.0) delivered instead ~3,500 lines across 15 commits: a marker directory, a `/run` spool, a systemd path unit, a root reconciler with three rule families and a status file, a version handshake, a rate limiter, two conf opt-ins, an sshd hardening precondition and a permanent SSH-key withholding latch. Thirteen of the project's twenty bug records (BUG-008 … BUG-020) come from that machinery, and **the feature was never once applied on real hardware** — `iptables -S` on this host on 2026-09-09 showed zero `styrman-lan*` rules, with the v4 script installed, the conf written and the status file current. It was blocked behind `install.sh --lan-allow-host 1`, which correctly refuses while the host accepts sshd password authentication.

**The design error was one step, taken in ADR-065's context.** That the server has no `NET_ADMIN` and no iptables binary — true — was read as *therefore the server must ask root to change a rule*. Everything above follows from that. The step not taken: the server does not need to change a rule, it needs the project **not to be matched by one**. Every styrman rule keys on the bridge INTERFACE name (`-i sdb-+` on the `INPUT` and `DOCKER-USER` jumps), and that name is chosen by the server at `docker network create` via `com.docker.network.bridge.name` (`internal/config/config.go:493`) — authority it already holds, on an operation it already performs.

**Two facts were verified before this decision, both previously assumed wrong or not asked.** (1) R12's cross-project guarantee is enforced by *Docker*, not by styrman: Docker 29.7.2 writes `-A DOCKER ! -i <br> -o <br> -j DROP` per bridge, so a new flow entering a bridge from anywhere but that bridge is dropped whatever the source bridge is called. (2) The prefix alone decides LAN reach. Both measured on this host with two throwaway Docker networks differing only in the four-character prefix, no styrman code involved: a `lan-` bridge reached the gateway, the host and the internet; an `sdb-` bridge reached only the internet; and from the `lan-` bridge the other bridge's container was unreachable by ICMP and TCP, including with `NET_ADMIN` and a manual route added — the attack R12 names by name. Recorded in `testing/2026-09-09-lan-prefix-probe.md`.

**Decision:** Four parts. (1) **The mode is the prefix.** A sandboxed project's bridge interface stays `sdb-<11hex>`; a LAN project's is `lan-<11hex>`, same sha1 suffix of the same network name, so no existing project's bridge is renamed by this release and one project keeps one identity in `ip link` across a toggle. The Docker NETWORK name stays `sdb-<uuid>` in both modes, which is what leaves project listing, GC, the credential broker, `ports.go` and the ingress watcher untouched. A `lan-` bridge is matched by no styrman rule and is therefore exactly a normal Docker container on this host. (2) **Applying it rebuilds, it does not reconfigure.** The mode is fixed at network create, so `EnsureRunning` gains a third force-recreate condition beside the two it already has (the pre-ADR-027 workspace bind and the image upgrade, `orchestrator.go:187-216`): a stopped container whose network's bridge name disagrees with the project record is removed, and `create()` rebuilds the network under the wanted name. Stopped containers only, like the image upgrade. This also self-heals a project left in the wrong mode however it got there. (3) **Everything root-mediated is deleted**, not shrunk: `internal/netpolicy/` (1,049 lines), the reconcile families and conf `LAN_*` handling in `styrman-firewall.sh`, `styrman-firewall.path`, the `/run` spool and its compose bind, the version handshake, the rate limiter, the sshd precondition in `install.sh`, and both withholding mechanisms — the `LanEverEnabled` latch AND `pruneHostReachingCreds`, a second, independent path that deleted the SSH key out of the project home on every enable. (4) **The operator's two decisions, recorded verbatim.** Asked whether a restart-on-toggle was an acceptable price for deleting the root path, and whether the SSH-key latch should stay: "Yes — restart is fine" and "Delete it". A LAN project is served credentials like any other; the transitive path ADR-066 named (LAN project → a host trusting the key → back here) is reopened and accepted, not overlooked. Supersedes ADR-065 and ADR-066 and all three of their amendments in place; all stay as history.

**Consequences:** Roughly 2,600 lines are deleted and ~150 added, and the feature stops depending on root entirely — no marker, no spool, no status file, no version handshake, so none of BUG-014 through BUG-020's mechanisms exist to fail. R12 is amended (the entry below) and gets *shorter*. Three costs are real and stated rather than mitigated with new machinery: a toggle rebuilds the project container (the home and workspace are bind mounts and survive; the ADR-028 apt manifest replays installed packages; the rootfs layer is discarded — the image-upgrade path's already accepted cost); LAN mode means the whole LAN including the host's sshd, with hardening now advice in `deploy/remote.md` rather than an enforced gate, because the gate is precisely what stopped the previous design from ever being used; and the sandbox now has one belt rather than two, so if the root firewall script never runs, sandboxed projects are unconfined — pre-existing for the base rules, mitigated by ADR-036's timer and `host_converge`, and deliberately NOT mitigated with server-side state, since deriving policy from root's word is the mechanism this ADR deletes. Cross-project isolation resting on Docker alone is asserted and attacked live before release, and a Docker major upgrade must re-run that probe.

**A process note, because it cost more than the code.** Three review rounds landed on the same mechanism and each fixed it instead of questioning it; `AGENTS.md` carries that rule verbatim and it was not applied. The one time it was — ADR-066's latch amendment, "the highest open finding of the release gate is deleted rather than fixed" — produced the best outcome of that effort. This plan's own cold-reader check then found eleven items across three rounds, nine of them the same class: a consumer of a deleted identifier missing from a hand-written table. Hand-curation was the defect; the list was regenerated by grepping every deleted identifier across the tree, which found eight more surfaces that three review rounds had not, including `deploy/verify-commit.sh`'s `systemd-analyze verify` of a unit file this ADR deletes — a release-gate failure nobody had caught. Re-running that sweep, expecting zero hits, is the completion check for each deletion phase.

Source: .agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md; .agents/docs/testing/2026-09-09-lan-prefix-probe.md; internal/config/, internal/orchestrator/, internal/httpapi/, internal/credbroker/, deploy/firewall/, web/src/.
Verification: the cold-reader reports reviews/2026-09-09-cold-reader-lan-mode-plan{,-round2,-round3}.md; the per-phase entries in .agents/docs/changes/ and the live adversarial log testing/2026-09-09-lan-mode-live.md (to be written, Phase 5).

---

## ADR-067 — amendment (2026-09-09): R12 wording updated with the operator's approval

**Context:** The exact replacement text for R12 was put to the operator in chat before any code, together with the scope statement and two flagged consequences (cross-project isolation would rest on Docker alone for a LAN project; a stale-rule cleanup would be needed on hosts carrying the v4 script). REQUIREMENTS.md is immutable without explicit approval (REQUIREMENTS.md:4), and ADR-055 treats code shipped against stale requirement text as blocking, so this entry lands with the R12 edit and before any implementation.

**Decision:** The operator's reply on 2026-09-09 was "aspprove" [sic]. Recorded here verbatim as the approval, with its scope stated: it approves the exact text as presented, which replaced the whole of numbered item 12 (REQUIREMENTS.md lines 70-113) and is reproduced in Appendix A of the plan. R12 now states the two enforcement layers separately — Docker's unconditional cross-project DROP, and styrman's `sdb-`-prefix sandbox — and describes the mode as the interface name. It gets shorter: the allowlist, the conf opt-ins, the enforced sshd precondition and the SSH-key withholding sentence are gone, with the superseded wording kept inline as history exactly as ADR-065's and ADR-066's amendments did.

**Consequences:** The requirement and the design agree before code exists. If the operator later judges the approval too narrow for how the implementation lands, the edit is one commit to revert and this entry says so.

Source: REQUIREMENTS.md R12, ROADMAP.md "Now".
Verification: .agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md Appendix A; this session's Phase 0 changelog entry.

---

## ADR-067 — amendment (2026-09-09): what the LAN mode widens, stated rather than implied

**Context:** The live adversarial run (`testing/2026-09-09-lan-mode-live.md`)
and the security review (`reviews/2026-09-09-security-adr-067.md`) each found
the same thing from different directions. ADR-067 says a LAN project reaches
"this server including its SSH service". Measured: it reaches the host at
*every* address the host holds — including the host's gateway address on
OTHER projects' bridge interfaces (`172.21.0.1` from a project on `lan-…`),
because the `-i sdb-+` wildcard that fed `STYRMAN-IN` simply no longer
matches. The other project's *container* stays unreachable; R12's
cross-project clause holds, and it held under the `cap_net_admin` manual-route
attack R12 names. Two consequences follow that the ADR did not write down:

1. **The credential broker's "bridge-only" binding now rests on one layer, not
   two.** Before, a project could not reach any broker address but its own
   bridge's, because the host firewall dropped guest→host on every `sdb-+`
   interface. Now a LAN project can reach the broker on any bridge address.
   What keeps that harmless is the broker's caller check — source IP resolved
   to (user, project) through `containerHoldsIP`, which requires the address
   to be one the resolved container actually holds — plus the fact that the
   broker serves per-USER credentials, so a project reaching a "wrong" address
   is still served its own user's secrets. Cross-tenant would need a forged
   source address whose TCP handshake also completes, and the SYN-ACK routes
   out the victim's bridge. **Not measured**: the broker only starts when the
   server is containerized and the live run used a host process. The test
   that settles it is specified in the security review (compose deploy, two
   users, four probes) and is owed before this is called closed.

2. **The host's sshd is reachable from a LAN project with whatever
   authentication the host allows.** ADR-066 enforced password-auth-off as a
   precondition; ADR-067 deleted the precondition (operator decision). On the
   production host at the time of writing: `passwordauthentication yes`,
   listening on `0.0.0.0:22`. That is a real exposure the moment any project
   is put in LAN mode, and it is the operator's to close — `deploy/remote.md`
   now says so as advice rather than as a gate.

**Decision:** Record both as the cost of the design rather than reopen it.
The broker question gets its test against the compose deployment before the
next release that touches `internal/credbroker`. sshd hardening stays advice,
stated loudly, because ADR-067's whole point was that root-mediated gates cost
more than they protected; the operator chose that trade with the sshd case in
front of them.

**Consequences:** A reviewer reading ADR-067 alone no longer has to discover
either point by measurement. And a failed revocation now FAILS CLOSED
(`internal/httpapi/network.go`, security review finding 1): the record is
rolled back when `Stop` fails, so no surface reports "sandboxed" while a
container still runs on a `lan-` bridge — the mismatch heal fires for stopped
containers only, and would never have corrected it.

Source: testing/2026-09-09-lan-mode-live.md; reviews/2026-09-09-security-adr-067.md.
Verification: `TestNetworkFailedRevocationRollsBackTheRecord` (mutation-checked); the broker test above is OWED, not done.

---

## ADR-067 — correction to the third amendment (2026-09-09, same day)

The amendment above says "a failed revocation now FAILS CLOSED". When it was
written that was true of the `Stop` branch only; the `/preflight` reviews the
same evening found the same fail-open on the `Status`-error branch above it
(BUG-029) and in `bridgeModeMismatch` below it (BUG-031). Both are now
closed, so the sentence holds for every branch that can leave a container
running on the wrong bridge. The one it cannot hold for is a failed rollback
WRITE to the record store — that branch logs at ERROR and needs a human.
Recorded here rather than edited in place: this file is append-only, and a
claim that was briefly wrong should stay visible as such.

Source: reviews/2026-09-09-preflight-review-v1.25.0.md, reviews/2026-09-09-preflight-security-v1.25.0.md.
Verification: TestNetworkStatusErrorOnRevocationRollsBack (mutation-checked); BUG-031 has no unit test — see its record.

---

## ADR-067 — amendment (2026-09-10): the base rules land at boot right after docker.service

The close-out plan (2026-09-10) left the timing of `styrman-firewall.service` to be decided
by measurement: run the base rules BEFORE `docker.service`, or keep `After=` and shrink the
window. Two facts decided it without a netns experiment. Project containers carry restart
policy `unless-stopped` (`orchestrator.go`, `restartPolicy`), so they come back at daemon
start on their own and the window between that and the timer's first tick (15 s) was real.
And the script inserts its jump into `DOCKER-USER` (`ensure_jump DOCKER-USER sdb-+
STYRMAN-FWD`) without creating that chain — Docker creates it — so the unit cannot usefully
run before the daemon at all. Whether Docker would preserve a pre-created chain was NOT
measured and does not matter for this ordering.

**Decision:** `styrman-firewall.service` gains `[Install] WantedBy=multi-user.target`. Both
installers (`deploy/firewall/install.sh` and the updater's `host_converge`) now `systemctl enable`
the service. This schedules the base rules to run once right after `docker.service`, closing the
≤15 s boot window where neither layer was in place (project containers restart on their own unless-stopped policy,
so the window between daemon start and the timer's first `OnBootSec` tick was real). The timer's
`OnBootSec` drops from 15 s → 5 s as supporting margin. Changes take effect on the operator's next
`sudo bash deploy/firewall/install.sh` (root); until then only the 5 s tick applies.

**Consequences:** once the operator has re-run `install.sh`, the base rules land right after
`docker.service` at every boot and the timer's 5 s first tick is a margin, not the only run.
Residual: the few hundred milliseconds between the daemon restarting its containers and the
unit's ExecStart. Verification is the operator's: `systemctl is-enabled styrman-firewall.service`
→ `enabled` after `install.sh` (it reads `static` until then), and `systemctl show -p After
styrman-firewall.service` naming `docker.service`. Not yet verified on this host at the time
of writing.

Source: deploy/firewall/styrman-firewall.service, deploy/firewall/install.sh, deploy/update/styrman-update.
Verification: `.agents/docs/changes/2026-09-10-closeout-phase3-deploy.md`.

---

## ADR-068: Browser view — one headless Chromium per project microVM, driven by the operator through a CDP screencast and by the agents through an MCP server (2026-09-11)

**Date:** 2026-09-11 · **Status:** accepted (operator approval 2026-09-11: architecture, agent access, the R4 sentence)

**Context:** The operator asked for a browser inside the app — a third main-pane view beside Terminal and Editor — to open sites served inside a project (a dev server on the guest's `localhost`) and public sites, and, above all, so that `claude` and `agy` running in the same microVM can drive that browser to test what they build while the operator watches. Share links and the port-preview proxy stay what they are; this is not a replacement. Two standing decisions pushed back: R4 / ADR-005 ("terminal-first, a file viewer is the ceiling") and ADR-002 ("project microVMs run no server"). An iframe through the existing preview proxy would have needed no image change but the agents could never see or drive it, and most public sites refuse to be framed; Xvfb + VNC would show Chrome's own chrome at a higher image, CPU and relay cost.

**Decision:** (D1) The runtime image carries Debian's `chromium` (headless) and the pinned npm package `chrome-devtools-mcp`; a launcher `/styrman/bin/styrman-browser` (`start|stop|status|mcp`) runs ONE Chromium per project AS THE PROJECT USER, profile in `~/.styrman/browser/profile` (per project, per R3), DevTools protocol on the guest's bridge address, port 9222. It starts on demand — the first Browser-pane connection or the agent's MCP server — and stops with the project. (D2) The styrman server keeps ADR-002's shape: it reaches the guest only through `docker exec` (to start / stop / probe the launcher) and through the project bridge (to relay the CDP WebSocket), exactly as the port-preview proxy reaches a dev server. Chromium is a tool the project user runs, not a server the styrman server depends on; the image's "pure tooling" line is amended, not abandoned. (D3) Port 9222 is one trust domain with the project user and the project's own inner containers, which can reach it; it is never published, never listed in the Ports panel, refused by the preview proxy, the share list and the link mint, and the browser routes (`/api/projects/{id}/browser*`, `/ws/browser/{id}`) are owner-only — a shared guest gets 403. (D4) Agent access is an MCP server named `styrman-browser` registered at harness seeding with each tool's OWN command — `claude mcp add --scope user` (writes `~/.claude.json`) and `agy mcp add` (writes `~/.gemini/config/mcp_config.json`) — as a third, stamp-gated step of `image/entrypoint.sh`'s seeding block. This is the one narrow exception to ADR-062's "no settings files": the tools' own MCP registries, still no `settings.json`, no hooks, no scripts for either tool. (D5) The SPA shows a CDP `Page.startScreencast` stream on a canvas and sends `Input.*` events; the server relays text frames between the SPA and the guest's browser-level DevTools endpoint with no protocol knowledge. (D6) R4 gains one sentence: "A Browser view showing the project's own in-guest Chromium, shared with the agents for testing what they build, is a testing surface and not an IDE (ADR-068, operator approval 2026-09-11)." (D7) Accepted costs: ≈ +400 MB runtime image (chromium is 371 MB installed); ≈ 250 MB guest memory per project while its browser runs (measured 2026-09-11 in a Kata probe on `about:blank`), more on heavy pages; project VMs are not held to Kata's 2048 MB base (`default_maxmemory = 0`).

**Consequences:** Chromium runs with its own sandbox on — probed 2026-09-11 inside a real Kata `mvm` container (guest kernel 6.18.28, user namespaces on); `--no-sandbox` is not used and a future breakage fails loudly in the pane rather than silently weakening it. The agents see the same browser the operator sees; a tab the agent opens appears in the pane's tab strip. No idle stop, no "follow the agent" mode, no clipboard / IME / downloads in v1 (follow-ups in the plan). Every image release rebuilds Chromium into `styrman-runtime:X.Y.Z` like every other tool. Supersedes nothing; amends ADR-002 (the image now runs one user-owned daemon on demand), ADR-005 / R4 (the sentence above) and ADR-062 (D4).

Source: `.agents/docs/plans/2026-09-11-browser-view.md` (interfaces, failure modes, phases); `REQUIREMENTS.md` R4; `image/Dockerfile`, `image/configs/styrman-browser`, `image/entrypoint.sh`; `internal/orchestrator/browser.go`, `internal/httpapi/browser.go`; `web/src/components/BrowserPane.tsx`.
Verification: `.agents/docs/reviews/2026-09-11-coldread-browser-view-r{1,2}.md`; the plan's per-phase verification and change entries (to be written as the phases land).

---

## ADR-068 — amendment (2026-09-11): the DevTools port admits one peer, the styrman server; no page can open it

Phase 2's security review (`.agents/docs/reviews/2026-09-11-security-browser-phase1.md` FINDINGS: 2 high) and the reviewer's feedback (`REQUEST CHANGES: 1 high`) raised the need to clarify port 9222's access control and defend against two attack vectors: (1) a page rendered in the browser opening its own DevTools socket to read the home via `file://` URLs, and (2) an inner container the user runs on the bridge reaching 9222 and doing the same.

**Decision:** (D1) Chromium's `--remote-allow-origins=*` flag removed entirely; Chromium now accepts a WebSocket upgrade only from clients that send NO Origin header and refuses every Origin (`http://localhost:9222`, `http://evil.example`, etc.) with 403 Forbidden. The Go relay is such a headerless client; any browser page that tries to open the socket fails the Origin check. (D2) socat's bridge listener now runs with `range=<peer>/32,max-children=4` where the peer is the server's own bridge address, parsed from `STYRMAN_CRED_BROKER` in a containerized deploy (where the broker runs on a specific bridge IP), else the default gateway (a host-run dev server); only when NEITHER exists does the forwarder stay off and the browser loopback-only. A container the user runs inside the project's dockerd has a source address outside that /32 and is refused; the MCP verb uses Chromium's loopback port 9229 directly.

**Consequences:** Port 9222 remains unreachable by a malicious inner container or a page in the browser; the only upgrade peer is the styrman server's relay. The peer restriction is verified under Kata by two probes of the launcher as an unprivileged user (2026-09-11): with no `STYRMAN_CRED_BROKER` the host, which is the bridge gateway there, gets `/json/version` 200; with `STYRMAN_CRED_BROKER=http://10.99.99.99:8091` the same host is refused (connection closed); loopback 9229 answers either way. No inner-container probe was run — the claim about inner containers rests on `range=` admitting one source address, not on a measurement. Regressions are caught by the relay test's fake DevTools server, which refuses any Origin exactly as Chromium does (so a relay that started sending one would fail the test), and by the three new integration checks (ports list omits 9222/9229 while the browser listens; `/proxy/{id}/9222/` and the share PUT answer 400; the owner's status/start/stop round-trip). `STYRMAN_CRED_BROKER` is set by the orchestrator whenever the server is containerized (ADR-030/032), i.e. in every production deploy.

**Source:** `internal/httpapi/browser.go` (relay with no upstream Origin), `image/configs/styrman-browser` (socat range + max-children), `.agents/docs/reviews/2026-09-11-security-browser-phase1.md` (MEDIUM#3, security HIGH#2, reviewer HIGH), `.agents/docs/changes/2026-09-11-browser-phase2-server-and-review-fixes.md` (all findings and fixes).
Verification: `internal/httpapi/browser_test.go` (`TestBrowserRelayPumpsBothWays` with the Origin-refusing fake, `TestBrowserPortRefusedOnPreviewSharesLinks`), the three integration checks above, the two Kata probes, all recorded in `.agents/docs/testing/2026-09-11-verify-browser-phase2.md` (integration passed=90 failed=0 skipped=3 twice).

---

## ADR-068 — amendment (2026-09-13): implementation and review-fix details clarified, deferred decision recorded

**Context:** A `/code-review` of v1.27.1..v1.28.0 returned 15 findings; three-round cold-read during planning reached the round cap with only implementation details remaining. All findings were resolved, and one scope question arose: whether to add a Stop control to the pane.

**Findings and clarifications:**

1. **Revocation re-check and close code (finding 2).** The relay watch goroutine signals a buffered `revoked` channel exactly once; `relayWebSockets`' select favours the 4003 close code on the `revoked` branch by placement. In `AuthMode=none` (dev loopback listener), the check is nil and no watch starts. `Server.stillAuthorized` is assigned once in `New`, reusing the terminal bridge's existing `stillAuthorized` closure, so both the bridge and relay share the same identity check — they are kept in sync by construction, not by repeated reads.

2. **Pane hides screencast and viewport on hide (finding 4).** A hidden pane (`visible=false`) does not write a viewport (gate on `visible` before `applyViewport`) and does not hold an active screencast session (the `screencastRef` latch stores the currently-streaming sessionId or null; a mount-while-hidden attaches without a screencast until shown; the visible effect guards `setScreencast` on `visible` and a session existing). Since Chromium is shared with the agents, pushing a 0×0 viewport from a hidden pane would misrepresent the viewport the agents see.

3. **Launcher pid file predicates (finding 8).** `kill_group_file` requires pid ≥ 2 (rejects 0 and 1 by numeric check before any operation) and signals the group ONLY when (the leader's `/proc/<pid>/comm` starts with the given prefix — `chrom*` for browser, `socat` for forwarder — AND members exist via `pgrep -g`, OR the leader's process is gone and members still exist — the `do_start` failure path with zygote/renderer children). Both callers (`do_stop` ×2, `ensure_forwarder` ×1) pass the correct prefix. `forwarder_up` confirms pid in file alive with matching comm and confirms a listener on :9222 — two independent checks, not one. When the leader is gone, `[ -d /proc/<pid> ]` false, the predicate still holds because the group check is independent.

4. **Server-run start with unusable peer (finding 11).** When `STYRMAN_BROWSER_PEER` is SET but not an IPv4 literal (an unusable peer), `peer_range` returns exit code 1 and `ensure_forwarder` exits 1 with the reason. The REST `POST /browser/start` sees this exit and returns 502 with the launcher tail; the relay does not start and the pane shows `unavailable` phase. Before: an unusable peer was accepted and the browser started but the forwarder loop failed, leaving the pane with "endpoint unreachable". Now: fail closed, reason visible to the operator immediately. `peer_range` still prints 0 (the range) when the peer is unset (loopback-only, MCP server) or when it is a valid IPv4 literal; it prints nothing and returns 1 only on set-but-invalid.

5. **`max-children=16` cap (finding 5).** A Go test `TestLauncherPinsPortsAndCaps` in `internal/httpapi/browser_launcher_test.go` reads the launcher script and asserts `max-children` value ≥ 2 × `browserRelayPerUser` (8 per the server's config), tied by code and comment. The cap accommodates concurrent relay connections (discovery GETs and WebSocket upgrades) without touching the MCP server (which uses loopback 9229, never socat).

6. **Stop control deferred (scope decision).** A control to stop the browser and drop the relay was struck from this release. Reason: `do_stop` signals the Chromium process GROUP (all renderer/zygote children and GPU process), which affects EVERY pane connected to that browser (all terminals in the project share one Chromium). A Stop button on one pane that kills the shared browser and severs all other panes' sessions is a destructive user-facing verb needing (1) scope confirmation, (2) an explicit confirmation dialog naming the agents, (3) a distinct close code to distinguish intentional stop from disconnection, and (4) no Reconnect on the client. This is a feature, not a review fix, and was deferred to a future scope gate. The REST operator verb `POST /browser/stop` remains (exists today for integration tier), and the agent (operator API) can call it directly.

**Decision:** Clarify ADR-068 with the above implementation details. Record the Stop control decision as deferred: it is load-bearing for the design (a shared headless browser cannot have an unqualified stop button), remains open, and the follow-up is noted in the plan.

**Consequences:** The relay re-checks ownership deterministically and closes 4003 when revoked. Hidden panes do not affect the shared browser's viewport or screencast. Pid files are validated before use, closing a loophole where recycled pids or zero could cause harm. Peer unusability is fail-closed, giving the operator immediate visibility. The launcher's child count is bound by a test. The Stop control awaits a future scope decision that names the confirmation UI and semantics.

Source: `.agents/docs/plans/2026-09-13-browser-review-fixes.md` (findings table, scope section, follow-ups); `.agents/docs/reviews/2026-09-13-review-browser-review-fixes.md` (findings 2, 8, 11, 5, heartbeat ping); `.agents/docs/reviews/2026-09-13-security-browser-review-fixes.md` (findings 1, 3, 5–6 on pid validation and UTF-8).
Verification: `internal/httpapi/browser_test.go` (`TestBrowserRelayClosesOnRevocation` client sees 4003); `internal/httpapi/browser_launcher_test.go` (`TestLauncherPinsPortsAndCaps`, `TestLauncherKillGroupFile`); `.agents/docs/changes/2026-09-13-browser-review-fixes.md` (integration 90/0/3, e2e 92/0/2).

## ADR-069: Runtime image supply chain — pinned versions and the broker-address contract (2026-09-13)

**Context:** The whole-project audit (2026-09-13) examined 31 surfaces and confirmed 2 highs and 14 mediums. One high was live in production: the guest's login-shell credential snippet recomputed the broker IP for a `/16` network against real `/24` bridges (since v1.27.1), silently unset all brokered credentials in every login shell, but passed the 2026-09-10 broker reachability test because that test read the injected variable directly. A second independent defect surfaced in the supply chain: `image/Dockerfile` had unpinned upstream fetches (`yq` latest release, `agy` manifest fetch, npm install without `--ignore-scripts`, Go/Node from distro without checksums) under a comment claiming the opposite.

**Decision:** Three parts. (1) **Pinned supply chain.** Go 1.27.1 + `sha256:63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445` (go.dev published sum), Node 22.23.2 + `sha256:d60acfe00a2932254bb0ad20e01b0d74397a0875595de719654b214f4b03f307` (nodejs.org/dist), Claude Code 2.1.270 + `npm install -g @anthropic-ai/claude-code@${VERSION}` with `--ignore-scripts` OFF (postinstall: node install.cjs materializes the CLI). Chrome DevTools MCP, hadolint, OpenTofu, yq are pinned identically. Antigravity (agy) is the ONE exception: the upstream installer fetches a "latest" manifest from Google Cloud Run and verifies sha512 against that manifest; no versioned artifact URL is offered and the CLI self-updates at runtime, so a build-time pin would be theatre — documented exactly that. Base images digest-pinned for both Dockerfiles (`deploy/caddy/Dockerfile` is the precedent). (2) **Bump procedure.** When a pinned version must move (Go security patch, Node LTS rotation, Claude Code feature/fix), re-read the publisher's checksum, update the ARG pair and its sha256/sha512, rebuild `docker build image/`, and commit. The auto-updater rebuilds the image on every tag, so a mismatched checksum fails the build loudly and the release rolls back — no silent divergence, by construction. (3) **Broker-address contract (the production defect):** `STYRMAN_CRED_BROKER` is computed and injected by the server at container create time, derived from the project's network's subnet and the bridge's address, and is **authoritative inside the guest** — it is never re-derived, recomputed, or questioned. The contract holds precisely because a network cannot be changed under a RUNNING container (Docker refuses to remove a network with live endpoints), and the one path that could give a container a new network without a new env is closed: if a network is missing on container start, `recreateMissingNetwork` **recreates the container** (not reconnects), so its env always names the bridge it runs on. Production proof (2026-09-13): a new project's login shell materializes `GH_TOKEN` from the broker (credentials flow again on real hardware).

**Consequences:** Every future release has reproducible, byte-identical behavior up to the pinned tools' versions — the release is deterministic. A changed upstream artifact (Go tarball recomputed, agy manifest redeployed differently, Node mirror sync) fails the image build with a checksum mismatch before the image is tagged and deployed; the mechanism is fail-loud, not fail-silent. Antigravity remaining unpinned is accepted risk and recorded: the upstream design offers no versioning mechanism and the CLI self-updates in the field anyway. The broker-address mistake (computing `/16` against real `/24` bridges) is closed by deleting the computation entirely and trusting injection. The mistake's root cause — that a network COULD change under a running container without invalidating the env — was incorrect; the network cannot change, by Docker's own enforcement, so injection alone is durable. No mitigation mechanism, no marker file, no re-check — just deletion of the unnecessary computation and documentation of the contract.

Source: `image/Dockerfile` (ARG pairs, comment), `deploy/Dockerfile.server` (base-image digests, comment), `image/configs/styrman-credentials.sh` (snippet references `${STYRMAN_CRED_BROKER}` only), `internal/orchestrator/orchestrator.go` (recreateMissingNetwork recreates the container), `.agents/docs/specs/styrman/runtime-image.md` (version list). 
Verification: `docker build image/` produces identical binaries on any build run with the same ARG versions; `docker run` of the image proves the CLIs' `--version` outputs (go, node, claude, agy); login shells in new projects show `GH_TOKEN` set (2026-09-13 production post-deploy verification, owed before closing).

---

## ADR-030 — amendment (2026-09-13): workloads inside a project's dockerd share the project's identity at the broker

**Context:** The audit examined the credential broker's access control and found that workloads a user runs inside their project's dockerd (a nested container on the project bridge) reach the broker with the same source IP as the guest microVM and therefore receive the same user-wide credential set. This is by design (ADR-030 Part 2: "caller identified by network position"), but it was never recorded and the design lived in code only. No code path needs to change; the decision is policy clarification.

**Decision:** Record that the boundary of credential sharing is the project (not the VM). Any workload inside a project's dockerd — whether it's the guest's own containers or the styrman server's own orchestration traffic for that project — reaches the broker with the project-bridge source IP and is served that user's credentials. This composes cleanly with R3 (per-project homes) and the secure credential tiers (git-credential-helper, env export, file injection, keyring seeding): the workload is running INSIDE the user's project namespace and has access to everything the user can access on the project's bridge and host already (by design, both the user and the workload are the project user, or the workload is the operator's own debugging). The broker's IP→(user, project) resolution and the per-user credential store handle the boundary; no per-container identity check is needed or added.

**Consequences:** The broker remains bridge-only (projects on `sdb-*` bridges in v1.29.0+, or earlier with the input guard `containerHoldsIP` in place) and per-user, with the boundary moved one level up to the project. The design is simpler and more transparent when stated this way, and it makes clear why LAN mode (ADR-067) does not require witholding credentials — a LAN-enabled project already had the ability to do everything the credentials would enable within its own network reach. A nested workload reading the broker output is working within the same trust domain as the guest and is intended to benefit from the same credentials (e.g., an agent running in a container should be able to push to the same repos the terminal can).

Source: ADR-030 Part 2 (the bridge-only design), ADR-030/032a (the broker's IP→(user, project) resolution), internal/credbroker/broker.go (the containerHoldsIP check and per-user store).
Verification: The contract is implicit in every test of the broker's authorization path; a failing test would catch any future narrowing. No new test needed; the decision is documentation of existing behavior.

---

## ADR-070: TODO close-out 2026-09-13 — the recorded audit lows and the v1.29.0 follow-ups are struck (2026-09-13)

**Context:** The whole-project audit (2026-09-13, `.agents/docs/reviews/2026-09-13-audit-security-root-scripts-image.md`) ran as 31 agents over every package, the web app, the deploy scripts and five security surfaces; the 2026-09-10 close-out plan (`.agents/docs/plans/2026-09-10-todo-closeout.md`) reduced the then-open TODO to twelve follow-up items the audit re-audited and the operator approved for strike or fix. Two items are security-relevant and fixed in v1.30.1 PATCH. The rest are cosmetic or low-severity and deferred for reasons recorded here.

**Decision:** Operator approval (2026-09-13, in-call: "I approve all your recommendations … Do not overcommit, it works"). FIXED in v1.30.1 PATCH: (1) `deploy/firewall/styrman-firewall.sh` line 69 — the missing-`xt_physdev` condition now goes through `warn()` so the degraded state reaches the exit code and the report, rather than reading as green. (2) `image/entrypoint.sh` line 165 — the harness version and stamp path are passed as arguments to `sh -c`, never interpolated into the shell. NEW in v1.30.1: `image/bump-pins.sh` (operator phrase: "if we pin stuff make it easy to upgrade") — a standalone script that refreshes the ADR-069 pins from the publishers' sources (`go.dev`, `nodejs.org/dist`, etc.) and rewrites the ARG lines and base-image digests in both Dockerfiles when run with `--apply`.

STRUCK (reason per item): The Stop control in the browser pane (would kill the Chromium the agents share; the REST `POST /browser/stop` stays for operators). `liveUnknown` flag on terminals listing (BUG-039's degrade-not-500 shape is the accepted behaviour). Harness re-seed cap (once per boot under a 20 s timeout is already bounded). Split `explorer.go` (a move, no behaviour). Credential store's non-injective host naming (needs a store migration for a case nobody has hit in production). Session-expiry re-check on open sockets (needs the ingress to expose JWT expiry; the existing 60 s allowlist re-check stands). `golang.org/x/sys/unix` in REQUIREMENTS.md's approved-deps list (REQUIREMENTS.md is immutable without explicit operator approval; the dependency is justified in the 2026-09-10 plan's Phase 1 for `unix.Openat` O_NOFOLLOW — recorded here instead). Host-setup nits (Docker from the distro vs pinned upstream, Kata checksum verification, `.env` permissions — none blocks anything). js-yaml build-time advisory (caught by `npm audit` at the next release; upstream fix awaited). FileExplorer polling beside `usePolled` (a behaviour change for no user gain). Provisioning-map key shape and orchestrator mismatch helper (cosmetic duplications).

**Consequences:** The open TODO is now empty (ADR-045's standing rule). Two security lows from the audit are fixed; the rest are deferred to a future release with reasons transparent in this record. `image/bump-pins.sh` becomes the operator's tool for keeping the supply chain current; future bump procedures are documented in `.agents/docs/specs/styrman/runtime-image.md`. The broker-address injection (ADR-069) is live and verified in production. History of the close-out lives in `changes/2026-09-13-todo-closeout.md`.

Source: `.agents/docs/plans/2026-09-10-todo-closeout.md` (the twelve items), `.agents/docs/reviews/2026-09-13-audit-security-root-scripts-image.md` (the two security findings), in-call approval 2026-09-13.
Verification: Recorded in the release gate report; the two fixes are in v1.30.1 PATCH.

---

## ADR-071: The terminal-WebSocket boot path self-attaches on the request path, before the upgrade (amends ADR-021) (2026-09-14) — Accepted

**Context:** ADR-021 (2026-08-26) kept `EnsureSelfAttached` off request paths because joining the server container to a project bridge reconfigures its network namespace and, on the published dev port of that time, killed in-flight connections including the triggering request. The terminal-WebSocket handler (`internal/terminal/bridge.go` ServeHTTP → the `ensure` callback = `httpapi.ensureFor`) is the path every project page boots a stopped project through (the TerminalPane's own "Start" is a WebSocket reconnect, not POST /start), and it had NO attach at all — after every server restart the first terminal of a stopped project ran its login shell against the broker's fixed bridge address (STYRMAN_CRED_BROKER, subnet broadcast−1) that nobody held, so git/gh/ssh/gcloud/claude credentials were missing ("credentials: the broker did not answer" MOTD; measured live 2026-09-14: server on the ingress network only, guest curl timed out, no `sbJoin` for the server in dockerd's journal). A background attach cannot serve this path: the login shell runs the moment the socket is up, and the attach must precede it.

**Decision:** `wsBoot` (`internal/httpapi/server.go`, called from `ensureFor`) boots via `EnsureRunning` and then calls `EnsureSelfAttached` synchronously, BEFORE the bridge upgrades the socket. The "fresh" return is ignored there: nothing has been written to the response yet, and a client whose handshake a fresh attach disturbs reconnects (the TerminalPane's retry loop) into the 60 s memoized attach — the client's retry plays the role ADR-021's 307 plays for the proxy. The other paths are unchanged: create/start/LAN-toggle attach in a goroutine after their response; the port-preview proxy lazily with a 307; boot-time adoption for running VMs. `EnsureRunning` itself still never attaches (its NOTE, now naming which caller attaches when).

**Rejected:** (1) attaching inside `EnsureRunning` — reinstates ADR-021's original defect for every caller (other in-flight requests, e.g. the SPA's polling, on a published port); (2) an attach-then-close-and-retry handshake (a 4xxx close code telling the client to reconnect) — one more round trip on every cold boot and one more protocol mechanism for a case the retry loop already covers; (3) leaving the WS path to the proxy's lazy attach — the broker is needed before any preview is opened.

**Consequences:** a stopped project opened from the UI has the broker reachable from its first login shell. On a deployment with a published server port (ADR-021's dev topology) the WS handshake that triggers a fresh attach may fail once and be retried by the client; production (Caddy on the host network reaching the server over the `styrman-ingress` bridge, no published port) has no such port. Verification: unit tests `internal/httpapi/wsboot_attach_test.go` (boot then attach; no attach on a failed boot) and `internal/terminal/ensure_before_upgrade_test.go` (the callback runs before any upgrade); the live proof is the operator's after the release that ships it (open a project stopped across a server restart from the UI: no broker MOTD line, `gh auth status` succeeds). Related: BUG-060, BUG-054, BUG-035; rule `.agents/rules/mechanism-reaches-every-caller.md`.

---
