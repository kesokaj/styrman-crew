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
removed).

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
