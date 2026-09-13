# Runtime image spec (`image/`)

`styrman-runtime` is the per-project microVM image: **pure tooling, no server
binary** (ADR-002). Debian trixie, AMD64 only. Built locally:

```bash
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> image/
```

## Contents

- Base: git, gh, tmux, jq, ripgrep, fd, bat, build-essential, python3, vim/nano
- Docker CE + buildx + compose (per-project DinD, ADR-004)
- gcloud + gke-gcloud-auth-plugin + kubectl. **helm is NOT installed**
  (removed 2026-09-02, operator directive): it was the worst of the image's
  unpinned fetches (`curl … get-helm-3 | bash`) and isn't wanted in the base
  image at all — a project that needs it installs it itself.
- OpenTofu, yq, and hadolint are ALL pinned by exact version + sha256 —
  `ARG TOFU_VERSION`/`TOFU_SHA256`, `ARG YQ_VERSION`/`YQ_SHA256`, `ARG
  HADOLINT_VERSION`/`HADOLINT_SHA256` in `image/Dockerfile` — none is a
  `curl | bash` installer or a `releases/latest/download` fetch. OpenTofu
  used to run the official `get.opentofu.org` installer script
  (`--install-method standalone`) and yq used `releases/latest/download`;
  both were replaced with a pinned download + `sha256sum -c` (2026-09-02
  audit, matching hadolint's existing pin) because an unpinned tool makes the
  tag→image mapping non-deterministic — the auto-updater's rollback would not
  restore the image the old tag actually produced. Every checksum was
  cross-checked against the publisher's own published sums, not just
  computed from whatever the download returned. **Bump procedure** (same for
  all three): bump the `ARG …_VERSION`, fetch the new release, verify its
  sha256 against the publisher's OWN published checksum file/release page
  (never trust a sha256 computed only from the download itself), then update
  the matching `ARG …_SHA256` — version and checksum move together in one
  commit.
- `shellcheck` (apt) and `hadolint` (GitHub release binary, pinned as above)
  — the shell and Dockerfile syntax checkers backing the editor's Check
  feature (`internal/httpapi/check.go`, `POST /api/projects/{id}/check`),
  which runs the language's real checker INSIDE the project VM, the same
  shape as `format` (see api.md's "Format (server-side)" section). Full
  checker table by extension: `gofmt` (go), `shellcheck` falling back to
  `bash -n` on an image predating it (sh/bash), `python3 -m py_compile` (py),
  `node --check` (js/jsx/mjs/cjs), `jq empty` (json), `yq e .` (yaml/yml),
  `tofu fmt -check` (tf/tfvars/hcl), `hadolint -f json`
  (Dockerfile/Containerfile/`*.dockerfile`)
- Go (latest stable at build time), Node 22
- Agents: `claude` (npm `@anthropic-ai/claude-code`), `agy` (official installer → `/usr/local/bin/agy`)
- Browser (ADR-068, 2026-09-11): Debian's `chromium` (headless; pinned by the
  archive like `docker-ce`, so security updates move its version),
  `fonts-liberation`, `socat`, and the npm package `chrome-devtools-mcp`
  pinned by exact version (`ARG CHROME_DEVTOOLS_MCP_VERSION`). The launcher
  `configs/styrman-browser` is installed to `/styrman/bin/styrman-browser`.
  Installed BEFORE the base-package snapshot so apt persistence never
  records chromium as a user install. Image size 3.56 GB → 4.32 GB.
  Details in "Browser" below.
- dbus, gnome-keyring, libsecret-tools — originally installed for agy's Secret
  Service (spike S4, ADR-032a); **removed from the image with ADR-040**:
  agy's real build never touched the keyring, and the broker's keyring tier
  that would have populated it was deleted outright, so `image/Dockerfile`
  no longer installs them (its own comment records the removal). No keyring
  daemon is started at login (no `profile-keyring.sh` step remains)
- e2fsprogs (project docker.img), tini (PID-1 reaper)
- `/etc/styrman-base-packages`: dpkg snapshot — the DIFF BASE for apt persistence (ADR-028: user-installed packages are captured vs this at shutdown and replayed on boot), and still useful for drift debugging

## Browser (`/styrman/bin/styrman-browser`, ADR-068)

One headless Chromium per project, run AS THE PROJECT USER, profile in
`~/.styrman/browser/profile` (per project, R3), started on demand — by the
server when the SPA's Browser view connects (Phase 2) or by the agent's MCP
server — and stopped with the project. Verbs: `start` (idempotent, `flock`ed
so the SPA relay and an MCP start cannot race, `-w 20` so a stuck lock is a
failure not a hang; exit 0 once `/json/version` answers within 15 s, else 1
with the log tail — and a failed launch takes its own process group down),
`stop` (signals the process GROUP, so children outlive no crashed leader —
each group kill is gated by `kill_group_file`'s predicate: signal only when
the group has members AND the leader's `/proc/<pid>/comm` matches the
expected prefix — `chrom*` for the browser, `socat` for the forwarder — OR
the leader is already gone; a stale pid file naming a recycled, unrelated
process is dropped without a signal, never killed), `status`
(`{"running":true,"version":"Chrome/…"}` or `{"running":false}`), `mcp`
(quiet `start`, then `exec chrome-devtools-mcp --browserUrl
http://127.0.0.1:9222 9>&-` on stdio — the `9>&-` closes the `start` lock's
fd before the exec, so the long-lived MCP server never holds it and a
later pane `start` is never blocked behind an MCP session; the `mcp` verb
no longer inherits the start lock at all).

Ports: Chromium's own DevTools listener binds LOOPBACK ONLY on 9229 —
new-headless Chromium ignores `--remote-debugging-address` (verified
2026-09-11: `ss` shows `127.0.0.1:9229` whatever the flag says) — and that
is what in-guest clients use (`mcp` → `--browserUrl http://127.0.0.1:9229`).
For the styrman server, which reaches the guest over the project bridge, a
`socat` forwarder in its own process group (`socat.pid`) publishes it as
`0.0.0.0:9222` with `range=<peer>/32,max-children=16` (≥ 2× the server's
`browserRelayPerUser` cap, so the discovery GET and every concurrent pane
relay fit; the discovery request also sends `Connection: close` on the
dial, so it is never pooled and cannot itself park a socat child): the ONE
admitted source is the server's own address on this bridge, which the
server passes as `STYRMAN_BROWSER_PEER` on every `start` it runs (a
containerized server reads its live address on that network from its own
container; a host-run dev server is the bridge gateway, from the network's
IPAM config — `Orchestrator.BrowserPeer`), falling back to the host of
`STYRMAN_CRED_BROKER` (the same address in a containerized deploy). Only an
IPv4 literal is accepted, since the value lands inside socat's option list;
a peer that is SET but not an IPv4 literal makes `start` fail closed with
its own message ("no usable peer address for :9222…") instead of silently
running loopback-only — the same failure the server's REST `start` and the
relay both report (502 / 4004) when their own bridge address is unknown.
With NO peer at all — a browser the agent's MCP server started, which sets
neither variable — no forwarder runs and the browser stays loopback-only;
the pane's first `start` then arrives with the peer and `ensure_forwarder`
adds the door to the already-running Chromium. `forwarder_up` (the check
`ensure_forwarder` runs before deciding to spawn) trusts only a `socat.pid`
(1–7 digits, no leading zero) naming a LIVE process whose comm is `socat`
AND that holds the `:9222` listening socket itself — the socket's inode
(column 10 of the kernel's `/proc/net/tcp{,6}` rows in state `0A` whose
hex local address ends in `:240A`; no `ss` output is parsed) is among
`/proc/<pid>/fd`'s links —
not "something answers `:9222`", nor "a live socat plus a listener", nor a
parse of process names — so a stranger
already bound to the port fails `start` with its own message instead of a
silent bind attempt, and the peer range is written to `socat.range` only
after the listener is confirmed up. A container the user runs inside the project's
dockerd can reach the guest's bridge IP but is refused by `range=` — a
DevTools client reads `file://` inside the guest, i.e. the brokered
credentials, and an image the user merely runs must not gain that.
Known boundary, accepted: a container the user starts with `--network=host`
inside the project's dockerd shares the guest's network namespace and can
reach Chromium's loopback listener on 9229 directly (and from there
`file://` inside the guest). Host networking is the project user's own
choice inside their own microVM — the same user could read the home
directly — so it is documented, not refused; a pipe transport
(`--remote-debugging-pipe`) would close it and is a follow-up in the plan.
Nothing guesses a peer: an unknown one fails CLOSED (security review
2026-09-11, two rounds; verified under Kata: no env → 9222 not listening;
`STYRMAN_BROWSER_PEER=<gateway>` → forwarder up and the host admitted;
`STYRMAN_BROWSER_PEER=1.2.3.4,range=0.0.0.0/0` → refused, no forwarder;
`stop` takes both groups down). Chromium refuses a `Host` header that is
neither `localhost` nor an IP literal (`Host: example.test` → 500), which
every caller satisfies. The server never lists, proxies, shares or links
9222/9229 (`internal/httpapi`), follows no redirect from the guest's
DevTools server, checks the pane's Origin before any side effect, and caps
concurrent relays at 32.

Origins: no `--remote-allow-origins`. Chromium's DevTools server admits a
WebSocket upgrade that carries NO Origin header (the styrman relay and
chrome-devtools-mcp) and refuses one that does with 403 — every page rendered
inside the browser sends one, so no page can seize its own browser through
the port (verified 2026-09-11 under Kata: no Origin → 101,
`Origin: http://evil.example` and `Origin: http://localhost:9222` → 403).

Sandbox: ON. Probed 2026-09-11 in a real Kata `mvm` guest (kernel 6.18,
user namespaces on): the launch as an unprivileged user works with
Chromium's own sandbox; `--no-sandbox` is NOT passed and `integration.sh`
fails if it ever appears in the live cmdline. Under plain runc (the default
seccomp profile blocks unprivileged user namespaces) the same launch fails
loudly with "No usable sandbox" — a plain `docker run` of this image is not
the target, the microVM is. Memory: a fresh headless Chromium on
`about:blank` costs ≈250 MB in the guest.

MCP registration (`entrypoint.sh`, third step of the stamped harness block):
`claude mcp remove --scope user styrman-browser || true; claude mcp add
--scope user styrman-browser -- /styrman/bin/styrman-browser mcp` (writes
`~/.claude.json`, refuses a duplicate, hence the remove) and `agy mcp add
styrman-browser /styrman/bin/styrman-browser mcp` (writes
`~/.gemini/config/mcp_config.json`, add-or-update). Graded as one unit
(`HR_OK`) beside `HM_OK`/`HI_OK`: the `~/.styrman/.harness-version` stamp
is written only when all three pass, so a half-registered home is retried
next boot and never reads as done. Neither file is a `settings.json`
(ADR-062 D8 holds); neither is on `harness-migrate.sh`'s move list nor
managed by `sh-agent install`. Both commands work in a fresh, unauthenticated
home (verified 2026-09-11).

## Entrypoint contract (`entrypoint.sh`, runs as root)

Inputs (env, set by the orchestrator):

| Env                                            | Meaning                                                                                                                             |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `STYRMAN_USER` / `STYRMAN_UID` / `STYRMAN_GID` | In-guest account (the user's configurable "terminal username"); uid/gid MUST match the host owner of the homes (spike S2)           |
| `STYRMAN_PROJECT`                              | Project UUID — keys the docker disk and transcript dir                                                                              |
| `STYRMAN_PROJECT_DIR`                          | Workspace dir name under ~/projects                                                                                                 |
| `STYRMAN_MINIMAL`                              | System VMs (onboarding): skip workspace + DinD                                                                                      |
| `STYRMAN_DOCKER_DISK_GB`                       | Size for a newly created docker.img                                                                                                 |
| `STYRMAN_CRED_BROKER`                          | Credential broker URL on this project's bridge (ADR-030/032); consumed by the git helper + login-shell snippet. Empty = broker off. |

Steps, in order:

1. Remount `/proc/sys` rw, restore `/etc/resolv.conf` (Kata has no Docker DNS).
2. cgroup-v2 nesting: remount `/sys/fs/cgroup` rw, create `init` group, enable
   subtree controllers (without this dockerd dies — spike S3).
3. mknod loop devices 0–7.
4. Create the user (matching uid/gid), passwordless sudo, docker group. A
   reserved name (root, daemon, …) that already exists at a different uid is
   rejected — the boot fails loudly instead of every `docker exec` silently
   running as the wrong uid on a home it doesn't own (I11).
5. `~/projects/<dir>`, `~/.dockerd`, `~/.config`, `~/.cache`, `~/.local/bin`
   (as the user; skipped in minimal mode). `~/.ssh` (mode 700) is created
   unconditionally, even in minimal mode. (The `~/.local/share/keyrings` mkdir
   and the dbus/gnome-keyring packages were dropped with the keyring tier,
   ADR-040.)
6. Seed the per-project home (ADR-031) write-once from the image: shell
   dotfiles from `image/skel/` (`~/.bashrc`, `~/.profile`, `~/.bash_*`, and
   `~/.npmrc` — the last referencing `${NPM_TOKEN}` from the broker) and the
   agent harness from `image/harness/` — a verbatim mirror of the
   `styrman-harness` repository at the commit in `image/harness/COMMIT`
   (refresh procedure in `image/harness/VENDORED.md`; ADR-062, superseding
   ADR-046's source and ADR-047's PATH trim). The image builds `sh-agent`
   from that mirror in a builder stage pinned to the same Go toolchain as
   `deploy/Dockerfile.server` and ships it as `/styrman/bin/sh-agent`. The
   harness is 28 skills and 7 subagents (`bugs`, `changelog`,
   `documentation`, `explorer`, `reviewer`, `security`, `verifier`) plus the
   global context (`AGENTS.md`); `sh-agent install --repo /styrman/harness
   --home <home>` copies it into `~/.claude` (`AGENTS.md`, a two-line
   `CLAUDE.md` stub importing it, `skills/`, `agents/`) and
   `~/.gemini/config` (`rules/AGENTS.md`, `skills/`, `agents/<name>/agent.md`
   derived for `agy`), writes its manifest to `~/.config/sh-agent/state.json`,
   and drops a copy of itself at `~/.local/bin/sh-agent` (`sh-agent status`
   inside a project is the drift check; `sh-agent update` is not used in
   VMs — the image is the delivery path). It ships NO hooks, NO scripts, and
   NO `settings.json` for either tool (operator directive 2026-09-03), so
   Claude Code runs in its default permission mode inside a project. `install`
   is a file copier — no network, no `agy`/`claude` call — so the ADR-047
   PATH trim is gone (0.02 s into an empty home on the host). Write-once,
   keyed on `/styrman/VERSION` versus the per-home `~/.styrman/.harness-version`
   stamp: bumping `STYRMAN_VERSION` and rebuilding the image is what RE-SEEDS
   an already-onboarded project's home. A home seeded by the previous harness
   (≤ v1.19.x: stamp present, no sh-agent manifest) is migrated first by
   `image/configs/harness-migrate.sh`, which MOVES every path the old
   `setup.sh` wrote — `~/.claude/{CLAUDE.md,settings.json,skills,agents,
   commands,statusline.sh,format-hook.sh,feedback-capture.sh}`,
   `~/.gemini/{GEMINI.md,settings.json,format-hook.sh}`,
   `~/.gemini/config/{skills,plugins/harness,global_workflows}`,
   `~/.agents/bin`, and the `.bak.<ts>` copies beside them — into
   `~/.styrman/harness-backup-<ts>/`, never deletes, and touches nothing else
   (credentials, `~/.gemini/antigravity-cli`, `~/projects`, user dotfiles).
   The stamp is written only when migration AND install both succeed.
   6b. Login-shell `/etc/profile.d` snippets: `zzz-styrman-motd.sh` (zzz- so it
   runs LAST, after credentials are materialized — it probes
   `~/.claude/.credentials.json` etc. and would otherwise nag "run
   styrman-setup" on a fully-onboarded project) and
   `zz-styrman-credentials.sh` (ADR-032, credential materialization, below).
   **There is no keyring snippet anymore** — the former
   `styrman-keyring.sh`/`profile-keyring.sh` step (session dbus + gnome-keyring
   unlock) is deleted outright (ADR-040); no keyring daemon is started at
   login.
   6c. Credential materialization (`zz-styrman-credentials.sh`, ADR-032). Skipped
   entirely if `STYRMAN_SKIP_CRED_PULL` is set (the harvest/import exec sets
   this so it reads the guest's REAL current files instead of the broker's
   own copy — H6). Otherwise, on every login shell:
   - Derives the broker's address from the VM's OWN current IP (broadcast−1
     of its `/16`) rather than trusting `$STYRMAN_CRED_BROKER` baked into the
     container env at creation time — a network recreation can hand the
     bridge a new subnet, which would otherwise strand the project on a
     stale broker IP (M17). Falls back to the injected value if its own
     address can't be read.
   - `GET /env` → `export` each var. The broker merges any PROJECT-SCOPED
     variables (SidePanel → Env, ADR-043) over the user-wide set for the
     calling project — the guest snippet is unchanged; scoping happens
     entirely broker-side via the caller's IP→(user,project) resolution.
     `GET /files` → writes each file under
     `$HOME` at its mode, via an `umask 077` temp file + atomic rename (never
     a torn or briefly-world-readable credential file).
   - applies `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` (env creds) as
     `git config --global user.name/user.email` — per-project homes carry no
     git identity by default (only the auth token is a broker tier), so
     without this a fresh project's `git commit` fails with "who are you"
     even though push/pull already authenticate (v1.7.1). Set as global
     config, not `GIT_AUTHOR_*` env, so a per-repo `user.name` still wins.
   - when `~/.claude/.credentials.json` is present, jq-merges
     `hasCompletedOnboarding: true` into `~/.claude.json` (creating a
     minimal file if absent), writing atomically via a temp file + rename so
     a concurrent `claude` never reads a torn file — a fresh per-project home
     otherwise has the flag unset even with a valid token, so interactive
     `claude` re-runs first-run onboarding and prompts for login despite
     already being signed in (`claude -p` is unaffected — it skips
     onboarding) (v1.5.6).
   - There is no `GET /keyring` step — that broker tier and this snippet's
     former `secret-tool store` block are both deleted (ADR-040).
   - **The skel dotfiles are write-once PER FILE, not version-keyed.** The
     copy loop is guarded by `[ ! -e "${dst}" ]` (`entrypoint.sh:84-92`), so
     unlike the harness above — which the `/styrman/VERSION` vs
     `~/.styrman/.harness-version` stamp deliberately RE-seeds on a version
     bump — a dotfile that already exists is never touched again, at any
     version. A change to `image/skel/` therefore reaches **new project homes
     only**. Concretely, v1.22.0 moved `GOPATH` to `$HOME/.go`
     (`skel/bash_exports`, `skel/bashrc`): homes created before it keep
     `$HOME/go` and anything `go install`-ed there stays on the old path
     until the user edits their own `~/.bash_exports` or the project is
     recreated. This is intended (operator decision 2026-09-04) — the files
     are the user's after first boot and provisioning must not clobber them.
7. Default `~/.tmux.conf` if the user has none (status off, login shells,
   `mouse off` — mouse mode steals selection from the browser terminal, so
   it's off so xterm.js can select natively and copy-on-select works).
   7b. tmux transcript hook: pipe-pane every session to `~/.styrman/logs/<uuid>/`
   (ADR-017). The generated `/etc/styrman/tmux-project.conf` also sets
   `monitor-bell on`, `bell-action any`, `visual-bell off` (unattended-bell
   notifications) and `set-clipboard on` (OSC 52, so tmux/TUIs can write the
   browser's clipboard through the terminal's OSC 52 handler).
8. DinD (ADR-008): create sparse ext4 `~/.dockerd/<uuid>.img` if absent,
   loop-mount at `/var/lib/docker-project`, start dockerd with **overlay2**;
   fall back to **vfs** directly on the home if the loop mount fails.
9. `touch /run/styrman/ready` — the orchestrator's readiness probe target.
10. Install a SIGTERM trap, then `sleep infinity &` and `wait` (not `exec`,
    so the shell stays alive to run the trap; tini stays PID 1 and still
    reaps exec'd session zombies). The trap stops dockerd cleanly and
    unmounts the loop-mounted ext4, so its journal is clean on the next boot
    (I9). Kata's `docker stop` powers the guest off without delivering
    SIGTERM to init, so the orchestrator's `Stop` triggers this itself first
    — `docker exec kill -TERM 1` — and waits briefly before falling through
    to `ContainerStop`.

## First-run onboarding (`image/configs/styrman-setup`)

Idempotent, interactive script run in the hidden setup VM (also re-runnable
in any project to check status or sign in to more tools). Split into
REQUIRED and OPTIONAL steps (v1.8.0), with two invocation modes (v1.10.0):

- `styrman-setup` — the full flow: required git identity + ssh key, then
  each optional tool in turn. Unchanged behavior.
- `styrman-setup <tool>...` — run ONLY the named step(s) (`git|ssh|gh|
gcloud|claude|agy`). When the target isn't git/ssh itself, the git
  identity is ensured quietly first (it's the prerequisite for the ssh key
  comment / a usable env). This is what the "add one credential later" flow
  in Settings → Credentials runs, via `GET /ws/setup?tool=<tool>` — see
  terminal-protocol.md and api.md's Setup section.

**Ctrl+C exits cleanly** (`trap 'printf "\n"; exit 0' INT`, v1.10.0).
Previously SIGINT killed the script by signal, and the browser terminal's
reconnect (`tmux new-session -A`) picked the ended session back up, which
RESTARTED `styrman-setup` from the top. The trap's `exit 0` now ends the
pane cleanly instead — and belt-and-braces, the client no longer
auto-reconnects a setup pane that has been open once (v1.10.1, see
terminal-protocol.md), so a restart is impossible even when the bridge's
1 s exit-code poll loses the race and reports a transient close.

**Keep / renew** (v1.10.1): every step is idempotent AND renewable. A
credential that already exists shows ✔ and offers `renew? [y/N]` — Enter
keeps it and moves on to the next step, `y` re-runs that sign-in (for the
ssh key, renewal generates a NEW key and keeps the old pair as
`id_ed25519.replaced` — the broker only harvests `id_ed25519` itself, and
the prompt warns the old key must be replaced wherever it's registered).
All status PROBES (`gh auth status`, `gcloud auth list`, `gcloud auth
application-default print-access-token`, `claude auth status`) run with
stdin `</dev/null` so none can swallow the user's typed-ahead answers to
later prompts (found by a pty test: `claude auth status` ate buffered
input, stranding the flow at its prompt).

- **Required** — git identity (`git config --global user.name/user.email`,
  prompted if unset) and an ssh key (generates `~/.ssh/id_ed25519` if
  absent; comments it with the user's git email, falling back to
  `user@styrman`, never the literal "styrman"). Neither can be skipped
  when absent — when present, the keep/renew prompt above applies.
- **Optional**, each gated behind a `[y/N]` prompt (`ask` — a
  non-interactive run, e.g. exec'd from a harvest, always answers no, so it
  never blocks): GitHub (`gh auth login --hostname github.com --git-protocol
ssh --web`); Google Cloud, guarded by `command -v gcloud` (`gcloud auth
login` for the account, then SEPARATELY `gcloud auth
application-default login` for ADC — the ADC step is what terraform and
  the client libraries use, and the one the broker syncs); Claude
  (`claude`); Antigravity (`agy`). Skipping any of them is fine — rerun
  `styrman-setup` anytime to pick up where it left off; the completion
  message says credentials are shared with every project automatically
  (rewritten v1.5.5 — used to send the operator to a now-obsolete manual
  Settings → Credentials → Import step).

Finishing setup harvests whatever was just signed into straight into the
credential broker (ADR-034, v1.5.4), via one of two routes (v1.10.0):
`POST /api/setup/done` — the full first-run completion: harvest, write
`~/.styrman/setup-done`, and destroy the VM — or `POST /api/setup/harvest`
— the "add a credential" path: harvest only, no done-marker, VM left
running so the next tool can be signed into without rebooting. See
deploy.md's Credential broker section and api.md's Setup section.

## Credentials materialized per project (broker tiers)

| Tool                 | Broker tier                                                                                  | Source in the shared/setup home       | Notes                                                                                                                                                                                                                                                                                                                             |
| -------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| git (push/pull auth) | git helper (per-op; nothing stored in the project)                                           | `gh auth token --hostname github.com` | A system git credential helper (`image/configs/styrman-git-credential`) calls the broker per operation; parses the JSON response with `jq` (base image has it), falling back to a `sed` extraction only right for simple unescaped tokens                                                                                         |
| gh (`GH_TOKEN`)      | env                                                                                          | `gh auth token`                       | Same token as git, exported as a shell var                                                                                                                                                                                                                                                                                        |
| git identity         | env (`STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL`)                                                 | shared-home `.gitconfig`              | Applied as `git config --global user.name/user.email` on login (v1.7.1) — per-repo identity still overrides it                                                                                                                                                                                                                    |
| claude               | file (`~/.claude/.credentials.json`)                                                         | same path                             | Carries a refresh_token; login-shell snippet also jq-merges the onboarding flag (v1.5.6, see entrypoint step 6c)                                                                                                                                                                                                                  |
| gcloud ADC           | file (`~/.config/gcloud/application_default_credentials.json`)                               | same path                             | Application-default creds — terraform + client libraries                                                                                                                                                                                                                                                                          |
| gcloud CLI (account) | file (`~/.config/gcloud/credentials.db` + `active_config` + `configurations/config_default`) | same paths                            | `gcloud auth login` account + active config, so `gcloud <command>` is authenticated in every project (v1.9.3). Only the DEFAULT config is synced; `access_tokens.db` is a cache gcloud regenerates and is intentionally not synced                                                                                                |
| ssh                  | file — first of `id_ed25519` / `id_rsa` / `id_ecdsa`                                         | `~/.ssh/`                             | Kata blocks agent-socket forwarding, so the private key itself is materialized into each project                                                                                                                                                                                                                                  |
| npm                  | env (`NPM_TOKEN`)                                                                            | `~/.npmrc`'s `_authToken`             | `image/skel/npmrc` (no leading dot in the image; `entrypoint.sh` copies it to `~/.npmrc` at seed time) references `${NPM_TOKEN}`                                                                                                                                                                                                 |
| agy                  | file (`~/.gemini/antigravity-cli/antigravity-oauth-token`, 0600)                             | same path                             | **v1.5.5**: ADR-032a originally modelled this as a keyring (Secret Service) credential — wrong for this agy build, which persists a portable `{access_token, token_type, refresh_token, expiry}` JSON file, not a keyring item. Moved to the file tier, which also works from the minimal setup VM (no keyring daemon runs there) |

There is no keyring tier: the broker's keyring tier (`credstore.KeyringCred`/
`PutKeyring`, `GET /keyring`, the login snippet's `secret-tool store` block)
was left in place as dormant code after the v1.5.5 agy move, then **deleted
outright** in the 2026-08-30 security review (ADR-040) — no store kind,
broker endpoint, or shell step references it anymore.

## Invariants

- Everything user-precious lives in `/home/<user>` (the virtiofs mount);
  the container itself is disposable at all times.
- **Image upgrades apply on the next stop/start**: a stopped container whose
  rootfs came from an older build of the runtime image is recreated at boot
  (v1.5 review fix). Running containers keep their rootfs until stopped. The
  apt manifest (ADR-028) replays what the user installed.
- **Guest hostname is the project's own name, never platform-prefixed**:
  `internal/orchestrator.guestHostname(name)` trims a trailing dash
  (`ValidateName` permits one; a hostname must not end in one) and falls back
  to `"project"` if that empties the string. It shows on the shell prompt
  (`\h` in `image/skel/bashrc`) and as `host` in `GET /api/projects/{id}/stats`
  and the terminal status bar. It used to be `"styrman-" + name`, which read
  as `styrman-styrman-harness` for a project named after its own repo — the
  platform's own name is not information the operator needs on every prompt
  line (operator directive 2026-09-02). The hostname is set at container
  creation, not baked into the image, so it only takes effect on the next
  recreate: an existing project keeps its OLD hostname until it is recreated,
  which happens automatically the next time the "image upgrades apply on
  stop/start" rule above fires. The hidden onboarding VM keeps the hostname
  `styrman-setup` deliberately — it is not a project.
- Agent conversations are exec'd as `bash -lc 'exec <agent>'` so
  `/etc/profile.d` (the credential-materialization snippet) always applies.
- The image never contains secrets; credentials arrive over the broker on
  the project's own bridge at each login (ADR-030/032) — never baked into
  the image, never passed to the guest via container env.
