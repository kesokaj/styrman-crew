---
owner: explorer
date: 2026-09-11
---
# Recon — installer wrapper for deploy/{host-setup,firewall,update,compose,.env} (2026-09-11)

## Question
Survey the existing deploy scripts/docs to inform ONE interactive `deploy/`
entry point (`install`/`update`/`edit`/`uninstall`/`status`, menu or flags)
that WRAPS them rather than replacing them.

## Summary
Everything the new tool needs already exists as separate idempotent pieces
with clear preconditions and root-owned state — the work is orchestration,
not new logic. `host-setup.sh` does OS-level provisioning + scaffolds
`deploy/.env` + calls `firewall/install.sh` + installs (but does not enable)
the updater; bring-up is a manual `docker compose up` the operator runs after
editing `.env`; `styrman-update` is the real "update" path and is enabled
separately, with three explicit prerequisites (deploy clone at `/opt/styrman`,
its own `.env` copy, `allowed_signers`) it refuses to skip. Status information
is scattered across `docker compose ps`, `systemctl status/is-enabled` for
three units, `/api/health`, and three flat state files under
`/etc/styrman/update`. Uninstall has no existing script at any level — the
new tool must derive the "stack / +host-config / +everything" tiers itself
from what `host-setup.sh` and `firewall/install.sh` are known to create.

## Relevant files
- `deploy/host-setup.sh` — OS provisioning, `.env` scaffold, calls firewall install, installs (not enables) updater. Steps below.
- `deploy/firewall/install.sh` — idempotent; installs firewall script+units, converges away ADR-065/066 leftovers, has `--dry-run` and path-override flags for tests.
- `deploy/update/styrman-update` + `.service`/`.timer` — the tag-gated updater; `--check`/`--force`; ADR-052 state-dir discipline.
- `deploy/docker-compose.yml` — the two services (`styrman-caddy`, `styrman-server`); both `image:` lines are `${STYRMAN_VERSION:?...}`-gated, no mutable-tag fallback.
- `deploy/.env.example` — every config key; must-fill vs computed vs optional, documented inline.
- `deploy/README.md`, `.agents/docs/specs/styrman/deploy.md`, `.agents/docs/specs/styrman/versioning.md` — the exact commands for bring-up, manual update, rollback, virgin reinstall.
- `internal/httpapi/server.go:1286` `health()` — `/api/health`, auth-exempt, loopback-only Docker ping.
- `deploy/firewall/test.sh`, `internal/config/firewall_script_test.go` — how root shell is tested today (unshare namespace, `go test` wrapper).
- `.agents/rules/writing-files.md`, `.agents/rules/mechanism-reaches-every-caller.md`, `.agents/rules/guest-writable-paths.md` — direct constraints on any new root script.

## Flows

### 1. `host-setup.sh` (run: `sudo bash host-setup.sh [--user <name>]`)
Flags/env: `--user` (default `$SUDO_USER`); `KATA_VERSION` (3.31.0), `DOCKER_VERSION` (29.7.2), `STYRMAN_STATE_DIR` (`/var/lib/styrman`), `KATA_SHA256` (pin/override), `STYRMAN_DEPLOY_DIR` (`/opt/styrman`), `STYRMAN_IMAGE` (override runtime image tag built in step 7).
Steps (line refs in `deploy/host-setup.sh`):
- 0 (L37-45): sanity-check `/dev/kvm` / nested-virt CPU flags, fails closed if absent on non-virt-capable CPU.
- 1 (L47-54): install Docker CE via `get.docker.com` if absent; always `usermod -aG docker "$OPERATOR"` — **requires operator log-out/log-in afterward** (printed at L379-380).
- 2 (L56-88): download+verify (SHA256, fails closed L69-76 if no digest obtainable) + extract Kata tarball to `/opt/kata`; symlink `containerd-shim-mvm-v2`. Idempotence check: `[[ -x /opt/kata/bin/kata-runtime ]]`.
- 3 (L90-157): merge `/etc/docker/daemon.json` via embedded python3 (runtimes.mvm, time-namespaces off, ipv6/ip6tables off, live-restore, default-address-pools 172.16.0.0/12, log-driver/opts) — atomic write + `.bak-styrman` backup, `systemctl restart docker` ONLY if changed (idempotent).
- 4 (L165-173): KSM host tuning via tmpfiles.d, best-effort (skipped if `/sys/kernel/mm/ksm/run` not writable).
- 5 (L175-187): state dirs `${STATE_DIR}/{homes,projects}` created + chowned to operator, non-recursive; refuses if any of the three roots is a symlink (guest-writable-paths discipline).
- 5b (L189-209): scaffolds `deploy/.env` from `.env.example` ONLY if `.env` absent — fills `STYRMAN_UID/GID`, `DOCKER_GID`, `STYRMAN_USER`, `STYRMAN_INGRESS_TOKEN`, `STYRMAN_JWT_KEY` via sed; `chmod 600`. **Idempotence: presence of `deploy/.env` is the sole gate — it never re-fills or updates an existing file.**
- 5c (L211-228): calls `firewall/install.sh` with `STYRMAN_STATE_DIR` exported; on failure logs a WARN and the exact re-run command but does not abort.
- 6 (L230-234): verifies a Kata microVM actually boots (`docker run --runtime mvm ... uname -r`).
- 7 (L236-261): builds `styrman-runtime:<tag>` image if not already present locally (tag = latest git tag sans `v`, else `setup-<timestamp>`).
- 8c (L267-364): installs `styrman-update` binary + `.service`/`.timer` to `/usr/local/bin` / `/etc/systemd/system`, always; only `systemctl enable --now styrman-update.timer` if BOTH `${STYRMAN_DEPLOY_DIR}/.git` AND `${STYRMAN_DEPLOY_DIR}/deploy/.env` already exist (else prints the full manual heredoc: mint GitHub fine-grained token → `/etc/styrman/gh-token`, clone to `/opt/styrman`, copy `.env`, create `/etc/styrman/allowed_signers` for SSH-signed tags, `systemctl enable --now styrman-update.timer`).
- `install -d -m 700 /etc/styrman` (L372) unconditionally, for the updater's future files.
- 9 (L374-397): prints next steps — log out/in for docker group, edit `.env` (lists must-fill keys), the exact versioned build+compose command.

### 2. `firewall/install.sh` (run: `sudo bash deploy/firewall/install.sh [--dry-run]`)
- Flags: `--dry-run` (prints `+ cmd` instead of running, via `run()` wrapper L29); `--bin-dir --unit-dir --tmpfiles-dir --run-dir` (ADR-052 test overrides); several now-obsolete flags accepted-and-ignored for back-compat (`--state-dir --lan-dev --lan-cidr ... --i-accept-sshd-passwords`, L41-46).
- Preconditions: root unless `--dry-run` (L51); source files present + `bash -n` syntax-checked (L52-56).
- Installs: `styrman-firewall.sh` → `/usr/local/sbin`, `.service`/`.timer` → `/etc/systemd/system` (L58-63).
- Removes ADR-065/066 leftovers: `.path` unit, spool tmpfs mount unit, its tmpfiles entry, service drop-in, run dir contents (L65-105) — order matters (disable before delete).
- Leaves `/etc/styrman/firewall.conf` untouched (operator-owned, now inert).
- Activates: `daemon-reload`, `start` + `enable` the `.service` (boot-time unit too), `enable --now` the `.timer` (L107-114); prints current `STYRMAN-FWD`/`STYRMAN-IN` iptables rules.

### 3. `styrman-update` (installed, NOT enabled by host-setup.sh)
- Verbs: no subcommand = normal tick; `--check` = report only, no side effects; `--force` = rebuild/redeploy current tag even if unchanged, and clears a latched failure.
- Required env: `STYRMAN_UPDATE_STATE_DIR` — **no default** (ADR-052); must canonicalize outside `/var/lib/styrman`, be ≥2 path components, not `/`. Production value is `/etc/styrman/update`, pinned only in `styrman-update.service` — any manual/wrapper invocation MUST pass it explicitly.
- `STYRMAN_DEPLOY_DIR` (default `/opt/styrman`), must have `.git` and its own `deploy/.env`.
- Enable: `sudo systemctl enable --now styrman-update.timer` (only after clone+`.env`+`allowed_signers` exist — see host-setup.sh 8c instructions and `deploy.md` "Auto-update": explicitly DO NOT enable before `allowed_signers` exists, since signature check fails closed).
- `host_converge()` (L474-547): on every tick (even `--check`, even after a failed deploy), re-installs `deploy/firewall/{styrman-firewall.sh,.service,.timer}` from the verified deployed commit if they differ from what's on disk, and unconditionally `systemctl enable`s `styrman-firewall.service` every tick (BUG-035 fix — mechanism-reaches-every-caller). Gated on signature enforcement being ON and HEAD matching the recorded verified `deployed-sha`.
- Rollback: on health-check failure (`reports_version`, L606-639, polls `docker inspect styrman-server` label + health + running state up to `HEALTH_TIMEOUT=120s`), latches `${STATE_DIR}/failed-version`, then redeploys the PREVIOUS version — preferring the recorded `deployed-sha` (trusted, skips re-verification) over re-verifying the tag name.
- Status files in `STATE_DIR`: `deployed-version` (tag), `deployed-sha` (commit), `failed-version` (latched broken tag, blocks retry until `--force` or a newer tag).

### 4. Compose / bring-up / rollback / virgin reinstall (exact commands)
- Bring-up/update: `cd deploy && STYRMAN_VERSION=<x.y.z> docker compose -f docker-compose.yml up -d --build` — but the runtime image must be built FIRST and separately: `docker build -t styrman-runtime:${V} --build-arg STYRMAN_VERSION=${V} image/` (compose never builds it).
- Compose refuses with `STYRMAN_VERSION` unset (`:?set STYRMAN_VERSION=...` on both `image:` lines, `deploy/docker-compose.yml`).
- Manual/break-glass fallback MUST run from `/opt/styrman` (not a dev checkout) because compose resolves `.env` from cwd (`versioning.md` L165-190) — production outage 2026-09-02 root cause.
- Rollback (manual): from `/opt/styrman/deploy`, `STYRMAN_VERSION=<old> docker compose up -d` (no rebuild — images are immutable once built; confirm the old `styrman-runtime:<old>` tag wasn't pruned).
- Virgin reinstall (`deploy.md` "Clean reinstall"): stop stack, remove every `styrman-*` container + `sdb-*` network, wipe `/var/lib/styrman/{homes,projects,users,credentials}` (keep the dir) AND `users.json`/`links.json`/`settings.json`, then `docker compose up -d --force-recreate styrman` — REQUIRED restart because the server caches per-user state in memory (v1.5.2 bug, ADR-033).
- Real release path is the auto-updater, not manual compose (`versioning.md` L76-85); manual is explicitly a fallback.

### 5. `.env.example` keys
- Must-fill: `BASE_DOMAIN`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `CLOUDFLARE_API_TOKEN`, `ADMIN_EMAIL`, `STYRMAN_ALLOWED_EMAILS` (`SUPERADMINS` defaults to it).
- Computed by host-setup.sh: `STYRMAN_UID`, `STYRMAN_GID`, `DOCKER_GID`, `STYRMAN_USER`, `STYRMAN_INGRESS_TOKEN`, `STYRMAN_JWT_KEY`.
- Optional: `STYRMAN_HOSTNAME`, `STYRMAN_PUBLIC_DOMAIN`/`STYRMAN_PRIVATE_DOMAIN` (preview links), `STYRMAN_IMAGE` (must stay unset in prod — operator rule), `STYRMAN_IDLE_TIMEOUT`, `STYRMAN_TRANSCRIPT_TTL`, `STYRMAN_BROKER_ADDR`, ingress knobs (`AUTH_DOMAIN`, `SHELL_DOMAIN`, `APPS_DOMAIN`, `DEFAULT_RPS`, `CADDY_ADMIN`).
- Validation called out in comments: `CADDY_ADMIN` must be a loopback IP literal+port, not a name (a name makes caddy refuse to start → crash loop under `restart:unless-stopped`); `STYRMAN_VERSION` is explicitly NOT an `.env` key (compose command-line only, no fallback).

### 6. Conventions for any new root script
- `set -euo pipefail` everywhere; `firewall/install.sh` also `shopt -s inherit_errexit`.
- `log()`/`die()`/`fail()` one-liner helpers, `echo "==>"`/`[scriptname]`/`styrman-firewall-install:` prefixes per script.
- Root check pattern: `[[ $(id -u) -eq 0 ]] || { echo ...; exit 1; }`, with a `--dry-run` escape hatch in `firewall/install.sh`.
- ADR-052 (privileged tests/state dirs): **no in-script default** for any state/target dir that could alias production (`STYRMAN_UPDATE_STATE_DIR`, self-install targets) — the systemd unit is the only place the production path is pinned; a hand run or wrapper must pass it explicitly and a canonicalize+prefix-reject guard (`realpath -m` then reject `/var/lib/styrman*`, reject `/` and single-component paths) is mandatory before writing anything.
- Trust checks repeated throughout `styrman-update`: `trusted_dir()` (root-owned, no group/world-write) applied to every directory read as a root input before trusting it.
- Root shell testing today: `deploy/firewall/test.sh` run under `unshare -Urnm bash test.sh` (exercises the real script in an isolated netns/mountns, refuses if it detects the host's netns); wrapped by `internal/config/firewall_script_test.go` under `go test ./...` (skips loudly where userns unavailable). No equivalent test harness exists yet for `host-setup.sh` or `styrman-update` themselves — they are exercised only by `deploy/verify-commit.sh` (build-level) and manual runbooks, not `go test`.
- `.agents/rules/writing-files.md`: never `cat >`/`Write` over an existing path without checking; `.agents/rules/guest-writable-paths.md`: any new path the wrapper touches under a mounted home needs the same symlink-refusal treatment as `host-setup.sh` §5's `[[ -L "${d}" ]]` checks; `.agents/rules/mechanism-reaches-every-caller.md`: any new "enable this unit" step the wrapper adds must be unconditional/idempotent, not gated on "did something change this run" (BUG-035 recurred exactly that way in `host_converge`).

### 7. Status — what exists today, commands
- `docker compose -f deploy/docker-compose.yml ps` (or from `/opt/styrman/deploy` in prod) — container state.
- `curl -s http://<host>:8080/api/health` or from inside the container (`/api/health`, auth-exempt for loopback; `internal/httpapi/server.go:1286`) — pings Docker daemon only when called from loopback, else static `{"status":"ok"}`.
- `systemctl status|is-enabled styrman-update.timer`, `styrman-update.service`, `styrman-firewall.timer`, `styrman-firewall.service` — unit health/enablement (BUG-035 shows `is-enabled` is the trustworthy check, not reading the unit file).
- `cat /etc/styrman/update/{deployed-version,deployed-sha,failed-version}` — what's actually deployed vs. latched-broken (path is whatever `STYRMAN_UPDATE_STATE_DIR` was set to in the unit, i.e. `/etc/styrman/update`).
- `iptables -S STYRMAN-FWD`/`STYRMAN-IN` — firewall rule state, printed by `firewall/install.sh` itself after activation.
- `docker image ls` — to confirm an old runtime tag is still present before a manual rollback.
- No single "styrman status" surface exists yet — the new tool's `status` verb is new synthesis work, not a wrapper around an existing status command.

## Risks
- No existing uninstall script at ANY level — "stack / +host-config / +everything" tiers must be hand-derived by cross-referencing every path/unit each install step creates (state dirs L182-187, `.env` L191-209, updater files L273-278/372, firewall files in `firewall/install.sh` L58-63, docker resources in compose). Missing one leaves orphaned root-owned files/units, exactly the ADR-065/066 leftover pattern `firewall/install.sh` had to clean up after the fact.
- `styrman-update`'s hard ADR-052 refusal of `/var/lib/styrman`-prefixed and single-component state dirs means the wrapper cannot just default `STYRMAN_UPDATE_STATE_DIR` casually — it must either hardcode `/etc/styrman/update` (matching the unit) or fail the same way the script does.
- `host-setup.sh` step 5b only scaffolds `.env` if absent — an `edit` verb that changes `.env` values has no existing "re-apply" mechanism; whether re-applying means `docker compose up -d` (recreate) vs. full `host-setup.sh` re-run vs. targeted `systemctl restart docker` (for daemon.json-affecting keys) is undecided by any current script — different keys need different re-apply strategies (compose env vars vs. daemon.json vs. firewall.conf).
- The manual bring-up and the auto-updater are two independently-maintained deploy paths with real divergent failure history (2026-09-02 outage from running compose from the wrong cwd). A wrapper `update` verb must decide up front whether it means "one-shot manual deploy of a tag" (mirroring the break-glass fallback, cwd-sensitive) or "enable the timer" (mirroring `styrman-update.timer`) — conflating them risks reproducing that outage or bypassing signature verification.
- `host-setup.sh`'s step 8c auto-update block requires PASTING A SECRET at a `read -s` prompt (GitHub token) and creating `allowed_signers` by hand — genuinely interactive steps with no flag today; an unattended/non-interactive mode needs a defined source for these (env var? pre-staged file path?) that doesn't exist in the current scripts.
- Sudo + interactive menu: `host-setup.sh` currently expects to run once, straight through, under `sudo bash ...`; an interactive multi-verb wrapper invoked as `sudo deploy/styrman-ctl` needs to decide how prompts (`.env` edits, confirmation typing for destructive uninstall tiers) interact with a root shell — none of the existing scripts prompt interactively at all today (all are flag/env driven, fail-closed on missing input).

## Open questions
- Should `deploy edit` diff-and-selectively-apply `.env` changes, or just re-run the relevant install step(s) idempotently? No precedent in the codebase either way.
- Does `update` (the new wrapper verb) mean enabling the timer, running one manual `styrman-update --force`-style deploy, or both depending on host state (timer enabled vs not)? Needs an explicit decision before design.
- For unattended/non-interactive `install`, where do the GitHub token and SSH allowed-signer entry come from (flag, env var, pre-staged file, or is auto-update deliberately excluded from non-interactive install)?
- Does "uninstall: stack only" need to also stop/disable `styrman-update.timer`, or is the updater considered separate from "the stack" (it operates on `/opt/styrman`, not the compose project)? The tier boundaries in the dispatch don't obviously map onto host-setup.sh's step ordering (e.g., is the runtime image part of "stack" or "host config"?) — needs explicit design sign-off.
