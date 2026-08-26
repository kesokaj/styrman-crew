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
