---
owner: main
date: 2026-09-11
---
# Plan — `deploy/styrman-ctl`: one interactive entry point to install, update, edit, uninstall and inspect a styrman host   (2026-09-11)

## Why

Operator request, 2026-09-11: "Can we make the installer more interactive.
I [want one] file to install/update/edit/uninstall for the entire project."
Confirmed the same day (scope answers): the file WRAPS the existing scripts
rather than replacing them, and `uninstall` has three levels with typed
confirmations.

Today a fresh host is provisioned by `deploy/host-setup.sh`, which then
prints a page of manual steps: log out and in, edit six `.env` keys by
hand, build the runtime image and run compose at the right version from the
right directory, and — if auto-update is wanted — mint a GitHub token, clone
to `/opt/styrman`, copy `.env`, write `allowed_signers`, enable a timer.
Updating a value later means editing `.env` (twice, once per copy) and
knowing which command re-applies it. There is no uninstall at all, and
"what is deployed right now" lives in four places. Every one of those steps
already exists as an idempotent, fail-closed script or a documented
command (`.agents/docs/plans/2026-09-11-installer-recon-brief.md`); the
missing piece is a single front door that asks the questions, runs the
right script with the right arguments, and says what it did.

## Scope

**In**
- ONE new file, `deploy/styrman-ctl` (bash, `set -euo pipefail`, shellcheck
  clean), run as root (`sudo deploy/styrman-ctl [verb]`). No argument → a
  numbered menu; a verb → that verb; `--yes` and per-verb flags → unattended.
- Verbs: `install`, `update`, `edit`, `uninstall`, `status` (plus `help`).
- `install` = `host-setup.sh --user <operator>` (unchanged, called), then a
  guided `.env`: prompt for the six must-fill keys with validation, then the
  bring-up at a version (runtime image build + compose from THIS checkout —
  the documented first bring-up), then an OFFER to set up auto-update
  (token, clone, `.env` copy, `allowed_signers`, timer) — the existing 8c
  prose turned into prompts.
- `update` = the auto-updater's path only: `update` runs one updater tick
  now (`systemctl start styrman-update.service`, or `styrman-update --force`
  for a redeploy of the current tag when asked); `update --setup` performs
  the auto-update setup (same code as install's offer); `update --status`
  is `status`. The wrapper never runs compose against `/opt/styrman` by
  hand: the break-glass command stays a documented command, not a verb
  (the 2026-09-02 outage came from that path — `versioning.md`).
- `edit` = a key-by-key editor of `deploy/.env`: lists every key of
  `.env.example` with its current value (secrets masked), prompts for a
  change, validates, writes BOTH copies when the deploy clone exists
  (`/opt/styrman/deploy/.env` is what the updater deploys with), then
  re-applies by recreating the stack at the currently deployed version
  (`docker compose up -d` from the clone if it exists, else this checkout).
- `uninstall` = three levels, each a typed confirmation, each printing its
  exact plan before asking:
  1. **stack** — `docker compose down` (both services), stop and remove every
     styrman-managed container — project microVMs AND the hidden per-user
     setup VMs alike, selected by the label `styrman.managed=true`
     (`internal/config/config.go` `LabelManaged`; the setup VM carries
     `styrman.system=setup` and NO project label, so a project-label filter
     would leave it running and its `sdb-sys-setup-<user>` network
     un-removable) — then every network named `sdb-*` or `lan-*`, then stop
     and disable the four styrman units. Keeps data, host config, images,
     Docker and Kata. Confirmation word: `stop`.
  2. **stack + host config** — level 1, then remove the unit files, the
     installed binaries (`/usr/local/bin/styrman-update`,
     `/usr/local/sbin/styrman-firewall.sh`), the firewall chains, the KSM
     tmpfiles entry `/etc/tmpfiles.d/styrman-ksm.conf` (host-setup.sh §4;
     KSM itself is left as it is until reboot — a host-wide tunable, not
     styrman state), `/etc/styrman` (updater state, token, signers), the
     deploy clone `/opt/styrman`, the `styrman-runtime`/`styrman-server`/
     `styrman-caddy` images, `deploy/.env` in this checkout (backed up to
     `deploy/.env.removed-<ts>` first). Leaves `/var/lib/styrman`, Docker,
     Kata, `daemon.json` (the runtime entry is inert without the images) and
     the operator's docker-group membership. Confirmation word: `uninstall`.
  3. **everything** — level 2, then `/var/lib/styrman` (every home,
     workspace, user store, credential store, transcript). Confirmation:
     the host's name as printed. Irreversible (R13: durability is git).
- `status` = one screen: server health (compose ps + the `styrman-server`
  health state + `/api/health` through the container's loopback), deployed
  version (compose image tags, `/etc/styrman/update/deployed-version`,
  `failed-version` if latched), the four units (`is-enabled`, `is-active`),
  firewall chains present, updater clone present and at which commit, disk
  use of `/var/lib/styrman`, running project microVMs, Docker and Kata
  versions. Exit 0 healthy, 1 degraded, 2 not installed.
- `--dry-run` on every verb prints each command it would run (`+ cmd`) and
  runs nothing — the test seam, same as `firewall/install.sh`.
- Unattended mode: `--yes` skips confirmations that are not destructive;
  destructive levels also need `--confirm <word>`; `install --env-file
  <path>` supplies the must-fill keys; `--gh-token-file <path>` and
  `--allowed-signers-file <path>` supply the auto-update secrets; without
  them the auto-update setup is skipped with a printed reason.
- Docs: `deploy/README.md` (the quick start becomes `sudo deploy/styrman-ctl
  install`), `.agents/docs/deploy/remote.md` and `local.md` (pointers),
  `.agents/docs/specs/styrman/deploy.md` (a "styrman-ctl" section mapping
  each verb to the underlying commands, and the uninstall levels).
- Tests: shellcheck; a Go test (`internal/config/styrman_ctl_test.go`,
  beside the firewall script test) that runs the script with `--dry-run`
  under a scratch prefix (ADR-052) for every verb and asserts the printed
  plan — the exact commands, their order, the refusals; a real `status` and
  a real `install --dry-run` on this host as the manual check.

**Out**
- Rewriting or changing the behaviour of `host-setup.sh`,
  `firewall/install.sh`, `styrman-update`, the compose file or `.env.example`
  (bug fixes found on the way are separate tasks).
- Uninstalling Docker, Kata or the operator's docker-group membership;
  editing `daemon.json` beyond what `host-setup.sh` already does.
- A manual compose deploy verb (break-glass stays documented, not wrapped).
- A curses/TUI; colour beyond what the existing scripts use; any
  non-Debian host; remote execution (the tool runs ON the host).
- Editing `.env` keys the wrapper does not understand: unknown keys are
  shown read-only with "edit the file by hand".

**Follow-ups noted, not built**
- `styrman-ctl backup`/`restore` — deliberately not (ADR-055: no backup
  layer; durability is git).
- A `doctor` verb (deeper diagnostics: Kata boot test, broker reachability).
- Shipping `styrman-ctl` to `/usr/local/bin` by the updater's `host_converge`
  (today it runs from the checkout or the clone).

## Surfaces touched

| Path | Change |
|---|---|
| `deploy/styrman-ctl` (new, ~600 lines bash) | the tool |
| `internal/config/styrman_ctl_test.go` (new) | `--dry-run` plan tests per verb under a scratch prefix |
| `deploy/README.md`, `.agents/docs/deploy/{remote,local}.md`, `.agents/docs/specs/styrman/deploy.md` | docs |
| `ROADMAP.md`, this plan | trail |

Nothing in `image/`, `internal/` (beyond the test), `web/` or the ingress.

## Interfaces

### Invocation

```
sudo deploy/styrman-ctl                      # menu: 1 install  2 update  3 edit  4 uninstall  5 status  q
sudo deploy/styrman-ctl install [--user U] [--version X.Y.Z] [--env-file F] [--gh-token-file F] [--allowed-signers-file F] [--skip-auto-update] [--yes] [--dry-run]
sudo deploy/styrman-ctl update [--redeploy] [--setup [--gh-token-file F] [--allowed-signers-file F]] [--yes] [--dry-run]
sudo deploy/styrman-ctl edit [KEY=VALUE ...] [--no-apply] [--yes] [--dry-run]
sudo deploy/styrman-ctl uninstall [--level 1|2|3] [--confirm WORD] [--dry-run]
sudo deploy/styrman-ctl status [--json]
deploy/styrman-ctl help
```

- Root required for every verb but `help`: `status` reads root-owned
  `/etc/styrman/update` and `iptables`; `--dry-run` runs as any user
  (prints only).
- `OPERATOR` (the account that owns the state tree and runs the server):
  `--user`, else `$SUDO_USER`, else die (`pass --user`) — the same rule
  `host-setup.sh` applies, decided here once and forwarded as `--user`.
- Paths (all overridable by env for the tests, ADR-052; the production
  values are the defaults ONLY because they are what `host-setup.sh` and
  the units already pin — the test passes a scratch prefix for every one):
  `STYRMAN_STATE_DIR` (`/var/lib/styrman`), `STYRMAN_DEPLOY_DIR`
  (`/opt/styrman`), `STYRMAN_ETC_DIR` (`/etc/styrman`), `STYRMAN_UNIT_DIR`
  (`/etc/systemd/system`), `STYRMAN_BIN_DIR` (`/usr/local/bin`),
  `STYRMAN_SBIN_DIR` (`/usr/local/sbin`), `STYRMAN_TMPFILES_DIR`
  (`/etc/tmpfiles.d`). The updater is always invoked with
  `STYRMAN_UPDATE_STATE_DIR="${STYRMAN_ETC_DIR}/update"` explicitly (it
  has no default by design).
- The checkout: `REPO="$(cd "$(dirname "$0")/.." && pwd)"`; the script
  refuses to run from a path under `${STYRMAN_DEPLOY_DIR}` for `install`
  (the clone is the updater's, never the operator's working tree —
  `host-setup.sh` 8c) and refuses `edit`/`update` when neither this
  checkout's `deploy/.env` nor the clone's exists ("run install first").

### Helpers (one file, sections in this order)

```bash
log()  { echo "==> $*"; }                # same voice as host-setup.sh
warn() { echo "==> WARNING: $*" >&2; }
die()  { echo "styrman-ctl: ERROR $*" >&2; exit 1; }
run()  { if [ "${DRY}" = 1 ]; then echo "+ $*"; else "$@"; fi; }   # every side effect goes through run
ask()      { # ask VAR "prompt" "default" [secret]  — prompt on /dev/tty; in --yes mode: default or die if none
confirm_word() { # confirm_word WORD "why" — reads the exact word; --confirm WORD satisfies it unattended
env_get()  { # env_get FILE KEY → value ('' if absent)
env_set()  { # env_set FILE KEY VALUE — replaces the line or appends; atomic (tmp + mv), mode 600, never follows a symlink (refuses)
validate() { # validate KEY VALUE → 0/1 with a message — one rule per key of .env.example (unknown keys are refused by `edit`):
             #   BASE_DOMAIN, STYRMAN_PUBLIC_DOMAIN, STYRMAN_PRIVATE_DOMAIN: hostname grammar (labels of [a-z0-9-], dots, ≤253), the two preview domains may be empty
             #   STYRMAN_HOSTNAME, AUTH_DOMAIN, SHELL_DOMAIN, APPS_DOMAIN: a single label [a-z0-9-]{1,63} (the last three may be empty)
             #   GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, CLOUDFLARE_API_TOKEN: non-empty, no whitespace
             #   ADMIN_EMAIL: one address with one @; STYRMAN_ALLOWED_EMAILS: comma list of such; SUPERADMINS: same, or the literal ${STYRMAN_ALLOWED_EMAILS}
             #   STYRMAN_IDLE_TIMEOUT, STYRMAN_TRANSCRIPT_TTL: a Go duration ^([0-9]+(ns|us|ms|s|m|h))+$ (TTL may be 0)
             #   STYRMAN_BROKER_ADDR: ^:[0-9]{1,5}$ or host:port, or empty; DEFAULT_RPS: integer or empty
             #   CADDY_ADMIN: empty, or a loopback IP LITERAL with a port (127.0.0.0/8 or ::1) — a name is refused (the caddy container would crash-loop)
             #   STYRMAN_IMAGE: must be empty in production (a mutable tag would re-pin every project) — refused unless empty
             #   STYRMAN_UID, STYRMAN_GID, DOCKER_GID, STYRMAN_USER, STYRMAN_INGRESS_TOKEN, STYRMAN_JWT_KEY: computed/generated by host-setup.sh — `edit` refuses them ("re-run install")
deployed_version() { # from the clone's deployed-version file, else the running styrman-server image tag, else ''
units_state() { # prints is-enabled/is-active for styrman-firewall.{service,timer} styrman-update.{service,timer}
```

### `install` flow (exact)

1. Preconditions: root; not inside the clone; Debian (`/etc/debian_version`);
   `python3`, `curl`, `git` present (host-setup needs them); print the plan.
2. `run bash "${REPO}/deploy/host-setup.sh" --user "${OPERATOR}"` (env
   passed through: `STYRMAN_STATE_DIR`, `STYRMAN_DEPLOY_DIR`, `KATA_VERSION`,
   `DOCKER_VERSION`, `STYRMAN_IMAGE`).
3. `.env`: for each must-fill key (`BASE_DOMAIN`, `GOOGLE_CLIENT_ID`,
   `GOOGLE_CLIENT_SECRET`, `CLOUDFLARE_API_TOKEN`, `ADMIN_EMAIL`,
   `STYRMAN_ALLOWED_EMAILS`): current value from `deploy/.env` (scaffolded
   by step 2) → `ask` (secrets hidden) → `validate` → `env_set`. Unattended:
   `--env-file` values, missing key → die naming it. `SUPERADMINS` left as
   the scaffold's `${STYRMAN_ALLOWED_EMAILS}` unless asked.
4. Version: `--version`, else `git -C "${REPO}" describe --tags --abbrev=0`
   stripped of `v`, else die ("no release tag; pass --version") — a bare
   commit sha is never a version (ADR-053).
5. Bring-up: `run docker build -t styrman-runtime:${V} --build-arg
   STYRMAN_VERSION=${V} "${REPO}/image"`, then `run env STYRMAN_VERSION=${V}
   docker compose -f "${REPO}/deploy/docker-compose.yml" up -d --build`
   (compose reads `deploy/.env` next to the file — cwd-independent because
   `-f` is absolute and `--env-file` is passed explicitly:
   `--env-file "${REPO}/deploy/.env"`). Wait for
   `docker inspect -f '{{.State.Health.Status}}' styrman-server` = healthy
   (≤ 120 s) and print the URL `https://${STYRMAN_HOSTNAME:-styrman}.${BASE_DOMAIN}`.
6. Auto-update offer (skipped with `--skip-auto-update` or unattended
   without both secret files): see `update --setup`.
7. Summary: what ran, what was skipped and why, the log-out/log-in reminder
   for the docker group (still needed for the operator's OWN shell; the
   tool ran as root so nothing above depended on it).

### `update --setup` flow (the 8c prose as code)

1. Token: `ask` (secret) or `--gh-token-file` → `install -m 600 -o root`
   to `${STYRMAN_ETC_DIR}/gh-token` (never echoed, never logged).
2. Clone: `run git clone <https origin of this checkout> "${STYRMAN_DEPLOY_DIR}"`,
   or `git -C "${STYRMAN_DEPLOY_DIR}" fetch --tags` when the clone exists —
   BOTH run with the same one-shot `GIT_ASKPASS` helper (a temp script under
   `mktemp -d`, mode 700, removed on exit, that prints `x-access-token` for
   the username prompt and the token file's content for the password
   prompt), so no credential is ever stored in the clone's config; the
   updater later uses its own helper reading `${STYRMAN_ETC_DIR}/gh-token`
   (`host-setup.sh` 8c). NO checkout here: the updater's first tick fetches
   the tags and checks out the newest signed release itself
   (`deploy/update/styrman-update:231-238,568`), and that tick is step 5 of
   this same flow — a manual checkout would be redundant at best and, with
   no defined version under `update --setup`, a guess at worst.
3. `.env`: `install -m 600 -o root -g root` this checkout's `deploy/.env`
   to `${STYRMAN_DEPLOY_DIR}/deploy/.env`, and the whole clone stays
   `root:root` (`chown -R root:root "${STYRMAN_DEPLOY_DIR}"` after the
   clone/fetch) — exactly what `host-setup.sh`'s 8c instructions say and
   what the updater, which runs as root and treats the clone as a trusted
   input, expects; the checkout's own `deploy/.env` stays operator-owned
   as `host-setup.sh` 5b leaves it. Overwrite the clone's copy only with
   confirmation when both exist and differ; `edit` keeps them in sync.
4. `allowed_signers`: `ask` for the line (`<email> <key type> <key>`) or
   `--allowed-signers-file`; validate the shape (three fields, key type in
   the OpenSSH set); `install -m 644 -o root` to
   `${STYRMAN_ETC_DIR}/allowed_signers`.
5. `run systemctl enable --now styrman-update.timer` (the unit was installed
   by `host-setup.sh` 8c), then `run systemctl start styrman-update.service`
   for the first tick and print `status`.

### `update` (no flag)

`systemctl is-enabled styrman-update.timer` must be `enabled` (else: "auto-
update is not set up — run `styrman-ctl update --setup`"). Then
`run systemctl start styrman-update.service`, wait for it to finish (≤ 15
min), print the journal's last 20 lines and `status`. `--redeploy` runs
`run env STYRMAN_UPDATE_STATE_DIR=${STYRMAN_ETC_DIR}/update STYRMAN_DEPLOY_DIR=${STYRMAN_DEPLOY_DIR} styrman-update --force`
instead (clears a latched failure, rebuilds the current tag).

### `edit`

Interactive: a table of every key in `.env.example` order with the current
value (`GOOGLE_CLIENT_SECRET`, `CLOUDFLARE_API_TOKEN`, `STYRMAN_INGRESS_TOKEN`,
`STYRMAN_JWT_KEY` shown as `********`), numbered; pick a number, `ask` the
new value (empty = keep), `validate`, repeat until `done`. Unattended:
`KEY=VALUE` arguments, each validated. Writes: `env_set` on this checkout's
`deploy/.env` and, when it exists, the clone's. Re-apply (unless
`--no-apply`): `deployed_version` → `run env STYRMAN_VERSION=${V} docker
compose -f <clone or checkout>/deploy/docker-compose.yml --env-file <same
dir>/.env up -d` (no `--build`: images are immutable per version; a changed
env only needs a recreate), wait for healthy, print `status`. A changed
`STYRMAN_UID/GID/DOCKER_GID/STYRMAN_USER` is refused ("computed by
host-setup.sh; re-run install").

### `uninstall`

Prints the level's plan (every command, from `run` in dry-run form), asks
for the confirmation word, then executes through `run`. Level 1 commands, in
order: `docker compose ... down --remove-orphans`; for each container with
the label `styrman.managed=true` (`docker ps -aq --filter
label=styrman.managed=true` — project microVMs AND the hidden setup VMs,
which carry `styrman.system=setup` and no project label): `docker rm -f`;
for each network named `sdb-*`/`lan-*` (`docker network ls --format
'{{.Name}}'`): `docker network rm`; `systemctl disable --now` the four
units (absent units skipped). Level 2 adds: the firewall chains, IPv4 AND IPv6 — `iptables -F STYRMAN-FWD`,
`-D DOCKER-USER -j STYRMAN-FWD`, `-X STYRMAN-FWD` (same for `STYRMAN-IN` in
`INPUT`), and when `ip6tables` exists the same for `STYRMAN-FWD6` (jump
from `FORWARD`) and `STYRMAN-IN6` (jump from `INPUT`), which
`deploy/firewall/styrman-firewall.sh:97-110` creates whenever `ip6tables`
is present; each `-D`/`-X` is best-effort (`|| true`) since a chain may
already be gone — then `rm -f` of the unit files + `daemon-reload`, the two binaries, the
tmpfiles entry, `rm -rf "${STYRMAN_ETC_DIR}"` and `"${STYRMAN_DEPLOY_DIR}"`
(each refused if it is a symlink or not root-owned — the BUG-009 rule),
`docker image rm` of `styrman-runtime:*`, `styrman-server:*`,
`styrman-caddy:*`, then `mv deploy/.env deploy/.env.removed-<ts>`. Level 3
adds `rm -rf "${STYRMAN_STATE_DIR}"` after a `du -sh` is printed and the
host name typed. Every `rm -rf` target is `realpath -m`'d and must equal one
of the configured dirs (never `/`, never a single component, never a
symlink) — the ADR-052 guard, restated here because this is the first
script that DELETES those trees.

### `status` output (human) and `--json`

```
styrman-ctl status  (host <name>, 2026-09-11 14:02)
  server     healthy   styrman-server 1.27.1 (styrman-caddy 1.27.1)   https://crew.styrman.xyz
  deployed   1.27.1    /etc/styrman/update/deployed-version, sha 4686792 ; no failed-version
  updater    enabled/active (timer)   clone /opt/styrman at 4686792 (v1.27.1)
  firewall   enabled/active (service, timer)   chains STYRMAN-FWD (8 rules) STYRMAN-IN (1)
  projects   3 running microVMs, 12 project dirs, /var/lib/styrman 41G
  host       Docker 29.7.2, Kata 3.31.0 (mvm), operator kesokaj
```
The host name shown, and the word level-3 `uninstall` asks the operator to
type, is `hostname` (the kernel's node name, `uname -n`); `--json` carries
it as `"host"`.
`--json`: the same as one object. Exit 0 all green, 1 anything degraded
(failed-version latched, a unit disabled, server unhealthy), 2 nothing
installed (no compose project, no units).

## Failure modes

| Failure | What the operator sees | Exit |
|---|---|---|
| Not root | `styrman-ctl: ERROR run as root (sudo)` | 1 |
| `install` from inside `/opt/styrman` | refused with the reason (the clone is the updater's) | 1 |
| `host-setup.sh` fails | its own output, then `styrman-ctl: ERROR host-setup.sh failed — fix and re-run install (idempotent)` | its code |
| Validation fails (bad email, hostname, CADDY_ADMIN name) | the rule, re-prompt; unattended: die naming the key | 1 |
| No release tag and no `--version` | die with the ADR-053 reason | 1 |
| Compose refuses (`STYRMAN_VERSION` unset) | cannot happen: the wrapper always passes it; if compose fails anyway, its output + "the stack is not up; `styrman-ctl status`" | its code |
| Server not healthy in 120 s | the last 30 lines of `docker logs styrman-server`, exit 1, stack left as is | 1 |
| `update` without the timer set up | the `--setup` hint | 1 |
| Updater tick fails | journal tail; `failed-version` explained; `--redeploy` hint | 1 |
| `edit` of a computed key | refused with "re-run install" | 1 |
| Confirmation word wrong | "aborted, nothing changed" | 1 |
| An `rm -rf` target fails the path guard | `ERROR refusing to delete <path>: <reason>`; nothing deleted; exit 1 | 1 |
| `--dry-run` | every `+ cmd` line, no side effect, exit 0 | 0 |

## Verification

Tiers: unit (Go test over `--dry-run` plans) · shellcheck · manual on this
host (`status`, `install --dry-run`, `edit --dry-run`, `uninstall --dry-run
--level 3`) · a real `install` on a fresh Debian 13 host is R11's own
criterion and stays the operator's (or a VM's) step — recorded when run.

- **Unit** (`internal/config/styrman_ctl_test.go`): runs `deploy/styrman-ctl
  <verb> --dry-run ...` with every path env pointed at a `t.TempDir()`
  scratch prefix and a fake `docker`/`systemctl`/`iptables` on `PATH` that
  records calls (the script's `run` prints them anyway); asserts, per verb:
  the ordered command plan, that no `+ rm -rf` names anything outside the
  scratch prefix, that `install` refuses under the clone dir, that `edit`
  refuses computed keys and validates the six must-fill keys (table of
  good/bad values), that `uninstall --level N` needs the right word, that
  `env_set` refuses a symlinked `.env` and keeps mode 600, that the updater
  is always invoked with `STYRMAN_UPDATE_STATE_DIR` set. Never touches
  `/var/lib/styrman` or `/etc/styrman` (ADR-052; the test fails if any
  printed path starts with them).
- **shellcheck**: clean at the default level.
- **Manual (AI, this host)**: `sudo deploy/styrman-ctl status` prints the
  live production state; `install --dry-run`, `edit --dry-run FOO=1`
  (refused: unknown key), `uninstall --level 3 --dry-run` print plans and
  change nothing (`git status`, `systemctl is-enabled`, `docker ps` before
  and after are identical). NO real install/uninstall on the production
  host in this plan.
- **E2E criteria**: golden path = on a fresh Debian 13 VM, `sudo
  deploy/styrman-ctl install` walks from bare host to a healthy stack with
  only the prompts answered; edge case = `uninstall --level 1` then
  `install` again brings the stack back with data intact. Both are the
  operator's to run (or a nested-virt VM when one is available); the plan
  is done without them but says so.

## Phases

### Phase 1 — Skeleton, `status`, dry-run harness, tests
- [x] `deploy/styrman-ctl`: arg parsing, menu, helpers (`run`, `ask`, `confirm_word`, `env_get/set`, `validate`, `guard_path`), `help`, `status` (human + `--json`, exit 0/1/2), the `__` test hooks — shellcheck clean; `sudo deploy/styrman-ctl status` on this host 2026-09-11: server healthy 1.27.1, deployed v1.27.1 sha 4686792, updater timer enabled with the clone at v1.27.1, firewall enabled with 7+1 rules, 0 running microVMs / 1 project dir / 174M, Docker 29.7.2, Kata 3.31.0 — all matching `docker inspect`, `systemctl is-enabled` and the state files; verdict OK. Two `set -e` traps found by the tests and fixed (a bare `a || b && c` list; pipelines under pipefail); who: AI
- [x] `internal/config/styrman_ctl_test.go`: harness (scratch prefix for every path, fake docker/systemctl/iptables/git/du/hostname/kata-runtime on PATH, a production path in any output fails the test); 6 tests: help + unknown/unimplemented verbs, `status --json` fields + degraded (unhealthy, latched failed-version) + nothing-installed exit 2, the validate table (19 good / 16 bad), `env_set` (replace-all-then-one, append, mode 600, symlink/newline/non-key refusals, dry-run leaves the file), `guard_path` (/, single component, outside, a child, a symlink at the root) — `go test -race ./internal/config/` green 2026-09-11; who: AI
- [x] Docs stub in `deploy/README.md` — who: AI
- [x] Phase checkpoint — committed 2026-09-11 (`changes/2026-09-11-styrman-ctl-phase1-skeleton-status.md`, `testing/2026-09-11-verify-styrman-ctl-phase1.md`); who: AI

### Phase 2 — `install` and `update --setup`
- [ ] `install`: preconditions, host-setup call, the `.env` prompts and validation, version resolution, bring-up with `--env-file` and healthy wait, summary — verify: dry-run plan test asserts the exact ordered commands and the refusals; `sudo deploy/styrman-ctl install --dry-run` on this host prints a plan and changes nothing; who: AI
- [ ] `update --setup`: token file, clone with a one-shot askpass, `.env` copy, `allowed_signers` validation, timer enable, first tick — verify: dry-run plan test; the askpass helper never lands in git config (test greps the dry-run plan and a fake `git` records its argv/env); who: AI
- [ ] Phase checkpoint — who: AI

### Phase 3 — `update` and `edit`
- [ ] `update` / `--redeploy`: timer precondition, service start + wait, journal tail — verify: dry-run plan test (`STYRMAN_UPDATE_STATE_DIR` present in every updater invocation); who: AI
- [ ] `edit`: table, prompts, `KEY=VALUE` mode, both copies, computed-key refusal, re-apply at the deployed version — verify: dry-run plan test; `sudo deploy/styrman-ctl edit --dry-run STYRMAN_IDLE_TIMEOUT=9h` on this host prints the two writes and the recreate without doing them; who: AI
- [ ] Phase checkpoint — who: AI

### Phase 4 — `uninstall`, docs, done
- [ ] `uninstall` levels 1–3 with the plan printout, confirmation words, path guard on every delete — verify: dry-run plan test per level asserts every command and that no delete leaves the scratch prefix; `uninstall --level 3 --dry-run` on this host prints the plan and touches nothing; who: AI
- [ ] Docs: `deploy/README.md` quick start, `deploy.md` "styrman-ctl" section, `remote.md`/`local.md` pointers — verify: `documentation` agent; who: AI
- [ ] Definition of Done: `verifier` (Go test, shellcheck, the four dry-runs on this host), `reviewer` over the diff, `security` (root script that deletes trees, handles a token and a signers file, prompts for secrets), `changelog`, `bugs` — who: AI
- [ ] Fresh-host e2e (`install` on a Debian 13 VM; `uninstall --level 1` + `install` again) — who: user, or AI when a nested-virt VM is provided
- [ ] Release: PATCH or MINOR? MINOR (a new operator surface) — who: user says ship

## Risks

- **A root script that deletes trees**: the path guard is the whole
  safety story; it is tested first (Phase 1) and every delete goes through
  it. `--dry-run` is printed before any confirmation so the operator reads
  the plan they are approving.
- **Two `.env` copies**: `edit` keeps them in sync; `install` writes the
  checkout's and copies it into the clone; the updater only ever reads the
  clone's. `status` warns when they differ.
- **Prompts under sudo**: `ask` reads from `/dev/tty`, so piping or a
  missing TTY falls to `--yes` semantics (default or die) — never a hang.
- **Secrets**: the GitHub token is written with `install -m 600` and never
  echoed; `set -x` is never enabled; the dry-run plan prints
  `gh-token: <redacted>`; `.env` secrets are masked in `edit`'s table and
  in `status --json` (absent, not masked).
- **Testing root logic without root**: the Go test exercises `--dry-run`
  plans with fake tools — it proves the commands and their order, not their
  effect. The effect is the wrapped scripts', which have their own history;
  the truly new effectful code is `env_set` (tested for real in a temp dir)
  and the deletes (dry-run + the guard's unit table).
- **This host is production**: the manual checks are read-only or dry-run;
  no real `install`/`uninstall` here in this plan.
