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
