# Deployment spec (Phase 3)

Two ways to run styrman; both share the same state under `/var/lib/styrman`.

## Dev mode (no auth, localhost/LAN)

```bash
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/          # 127.0.0.1:8080
```

`STYRMAN_AUTH` defaults to `none`; whoever reaches the port acts as the
default user (first allowed email, else the OS user) with admin rights. The
server now **refuses to start** with `auth=none` on a non-loopback `STYRMAN_ADDR`
(and refuses a non-loopback dev listener) — an unauthenticated admin API must
never be exposed beyond loopback. `config.Load` fails closed on every other
misconfiguration too: `auth=header` with an empty `STYRMAN_INGRESS_TOKEN`
refuses to start (an empty token would let any project VM on the shared
bridge spoof `X-Caddy-Email` and impersonate any user, admin included — H2),
and any `STYRMAN_AUTH` value other than `none`/`header` is rejected outright
rather than silently falling through to an unguarded path (M16); `authGuard`
carries the same default-deny as a defense-in-depth backstop. To reach a dev
server from the LAN, front it with the Caddy ingress (`auth=header`), don't
bind `0.0.0.0` with `auth=none`. Never expose this beyond loopback/trusted
networks. The compose deploy runs
**no** auth-bypass listener at all (`STYRMAN_DEV_ADDR` is opt-in, must bind
loopback, and is unset in `docker-compose.yml`). Before the 2026-08-27
security review it bound `0.0.0.0:8081` inside the server container, which
is joined to every project bridge for the port-preview proxy — handing any
project VM unauthenticated admin. Local testing without OAuth instead
launches its own host server on loopback with an isolated state dir
(`web/e2e/run.sh`, `.agents/docs/testing/styrman/integration.sh`), which
project microVMs cannot reach.

## Production mode (Google OAuth, HTTPS, survives reboots)

Two hosts run this mode today: the operator's own Debian 13 box
(`remote.md`) and a disposable GCP Compute Engine VM, same Debian 13 host
path, brought up with `deploy/gcp/vm.sh` + `deploy/gcp/install.sh` instead
of by hand — ADR-075, `.agents/docs/deploy/gcp.md`.

### styrman-ctl (one front door)

`sudo deploy/styrman-ctl` (plan
`.agents/docs/plans/2026-09-11-installer-ctl.md`) wraps every command in
this section instead of replacing any of them — every side effect goes
through the script's `run()`, so `--dry-run` prints the exact plan and
executes nothing.

| Verb | Wraps |
|---|---|
| `install` | `host-setup.sh --user <operator>`; prompts for the five must-fill `.env` keys (`BASE_DOMAIN`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ADMIN_EMAIL`, `STYRMAN_ALLOWED_EMAILS`; validated) with `--env-file` supplying defaults unattended; when the env file carries `CLOUDFLARE_API_TOKEN` it is written too (validated, an invalid value stops the install, never prompted for; v1.39.0 to the release after it dropped it silently and the host used HTTP-01); `resolve_version` (`--version`, else the checkout's newest reachable release tag, never a bare commit sha, ADR-053); `docker build -t styrman-runtime:${V} --secret id=gh_token,… --build-arg STYRMAN_VERSION=${V} image/` (the secret is required since ADR-074; `styrman-ctl` takes it from `--gh-token-file`, else `GH_TOKEN`, else `/etc/styrman/gh-token`, and refuses to build without one); `STYRMAN_VERSION=${V} docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build`; waits up to 120s for `styrman-server` to report healthy; then offers auto-update setup |
| `update --setup` | the "Setup" steps below, as prompts: GitHub token → `/etc/styrman/gh-token` (600); clone-or-fetch `/opt/styrman` under a one-shot `GIT_ASKPASS`; `chown -R root:root` the clone; copy `deploy/.env` into it; write `/etc/styrman/allowed_signers`; `styrman-update --check` with `STYRMAN_UPDATE_STATE_DIR`/`STYRMAN_DEPLOY_DIR`/`STYRMAN_GH_TOKEN_FILE`/`STYRMAN_ALLOWED_SIGNERS` all named explicitly, plus `STYRMAN_UPDATE_STATUS_FILE=${ETC_DIR}/status/updater.json`; `systemctl enable --now styrman-update.timer`; `systemctl start styrman-update.service` for the first tick |
| `update` | `systemctl start styrman-update.service` (one tick now) + `journalctl -u styrman-update.service -n 20` |
| `update --redeploy` | `styrman-update --force` with `STYRMAN_UPDATE_STATUS_FILE=${ETC_DIR}/status/updater.json` (clears a latched failed-version, rebuilds the current tag, records its result in the status file) |
| `edit` | `env_set` on both `.env` copies (checkout's and the clone's, when it exists), then `docker compose -f <dir>/deploy/docker-compose.yml --env-file <dir>/.env up -d` at the currently deployed version — no `--build`, images are immutable per version |
| `uninstall --level 1` | `docker compose … down --remove-orphans`; `docker rm -f` of every container labeled `styrman.managed=true` (project microVMs AND the hidden per-user setup VMs alike); `docker network rm` of every `sdb-*`/`lan-*` network; `systemctl disable --now` the four units in `UNITS[]` (the firewall and updater pairs; the ADR-073 harness-channel pair is retired by the updater, ADR-074) |
| `uninstall --level 2` | level 1, then: the IPv4 AND IPv6 firewall chains (`STYRMAN-FWD`/`STYRMAN-IN` and, when `ip6tables` exists, `STYRMAN-FWD6`/`STYRMAN-IN6`); the unit files + `daemon-reload`; `/usr/local/bin/styrman-update`; `/usr/local/sbin/styrman-firewall.sh`; `/etc/tmpfiles.d/styrman-ksm.conf`; `/etc/styrman`; `/opt/styrman`; the `styrman-runtime`/`styrman-server`/`styrman-caddy` images; `deploy/.env` moved to `deploy/.env.removed-<ts>` |
| `uninstall --level 3` | level 2, then `/var/lib/styrman` |

`status [--json]` reads the same state this section describes
(`/etc/styrman/update/{deployed-version,deployed-sha,failed-version}`, unit
`is-enabled`/`is-active`, the firewall chains, the updater clone's commit)
without changing any of it.

The wrapper deliberately does **not**: run compose against the clone by
hand (the break-glass command below stays a documented command, not a
verb — `versioning.md`); remove Docker or Kata; edit `daemon.json`. "Clean
reinstall (virgin state)" below is `uninstall --level 3` followed by
`install`.

Deploy/upgrade: `STYRMAN_VERSION=<x.y.z> docker compose -f
deploy/docker-compose.yml up -d --build` (the version stamps the binary, the
Settings page, and every image tag). **Compose refuses to run at all with
`STYRMAN_VERSION` unset** — every `image:` line is
`${STYRMAN_VERSION:?...}`-gated (operator rule: no mutable tags). Rebuild the
runtime image FIRST, at the same version — `docker build -t
styrman-runtime:${STYRMAN_VERSION} --build-arg
STYRMAN_VERSION=${STYRMAN_VERSION} image/` — compose never builds it for you.
Full release/rollback procedure and the three-image lineup:
`.agents/docs/specs/styrman/versioning.md`. v1.3 upgrade note: the ADR-027
workspace migration runs on the server's first boot (gracefully stopping
running project VMs it needs to move), and pre-ADR-027 containers are
stop+recreated once on next open to gain the workspace bind.

Composition (`deploy/docker-compose.yml`):
- **styrman-caddy** — the ingress vendored from devbox (`deploy/caddy/`,
  see VENDORED.md): Caddy 2 + caddy-security (Google OAuth portal at
  `auth.<BASE_DOMAIN>`), Cloudflare DNS records + DNS-01 ACME certs (or HTTP-01 ACME when CLOUDFLARE_API_TOKEN is unset), and Docker-label
  app discovery. Host networking; watches the docker socket read-only.
  Container-name-derived subdomains and Caddy policy names are sanitized
  (`sanitizeHostname`/`sanitizePolicyName`, `deploy/caddy/ingress/labels.go`
  and `generator.go`) against malformed/hostile input. The unused
  `picture_extractor` code path and the `caddy-l4` module/dependency have
  been removed from the vendored build (2026-08-30 cleanup).
- **styrman-server** — the Go binary containerized
  (`deploy/Dockerfile.server`), discovered via labels
  (`caddy_hostname=styrman` → `https://styrman.<BASE_DOMAIN>`), running as
  the operator's uid/gid + the host docker group. Since v1.33.0 it asserts
  `net.ipv4.ip_forward=0` at boot and refuses to start otherwise, since it
  joins every project bridge and forwarding-on would violate R12;
  `STYRMAN_ALLOW_IP_FORWARD=1` in `deploy/.env` is the deliberate override —
  a tripped guard fails the health check and the auto-updater rolls back.
  `STYRMAN_AUTH=header`:
  every request must carry the ingress's `X-Caddy-Email` AND pass the
  userstore (`STYRMAN_ALLOWED_EMAILS` + `/var/lib/styrman/users.json`) — R8's
  two layers. The state dir and docker.sock are mounted at host-identical
  paths so microVM bind sources stay valid.

The server's ingress label is `ModeAuthenticated`, not role-gated: **any**
Google account may sign in and reach the server (the `caddy_admins` label was
removed from `styrman-server` in `docker-compose.yml`). The real gate is the
server's own `authGuard` (`internal/httpapi/auth.go`), which 403s every
request whose email isn't in the userstore — **except** a port preview the
project owner shared with that email (ADR-024). Admin rights come only from
`STYRMAN_ALLOWED_EMAILS` (server-side env, above), never from the ingress
role — a guest can never become an admin just by passing the ingress. This
split (ingress lets anyone in, server enforces per-user/per-port permission)
is what makes per-port preview sharing deployable at all.

Required secrets (generated by `host-setup.sh` into `deploy/.env`, no
defaults in compose):
- `STYRMAN_INGRESS_TOKEN` — shared secret the ingress injects as
  `X-Styrman-Ingress` and header-auth mode requires (`config.Load` refuses
  to start without it, H2); blocks a project VM on the shared bridge from
  spoofing `X-Caddy-Email` directly against the server.
- `STYRMAN_JWT_KEY` — pins caddy-security's JWT signing key so browser
  sessions survive a Caddy restart/redeploy instead of logging everyone
  out.
- Session lifetime: the portal issues eight-hour tokens (`crypto default token lifetime 28800`, ADR-063), set in the generator rather than inherited from caddy-security's 900 s default; the styrman server re-checks the userstore on every request, so a removed user is out at once regardless. Expiry shows in the SPA as one toast and a reload (BUG-001).

`STYRMAN_KEYRING_PW` is fully removed: `docker-compose.yml`, `deploy/.env.example`,
and `host-setup.sh` carry zero references to it (the broker keyring tier it
backed was deleted outright, ADR-040). A `deploy/.env` generated by an older
`host-setup.sh` may still carry a leftover `STYRMAN_KEYRING_PW=...` line —
harmless; nothing reads it, compose no longer requires it, and it can be
deleted whenever convenient.

Per-project sizing (ADR-083) is a per-project record, not environment:
`STYRMAN_VCPUS` / `STYRMAN_MEMORY_MB` are removed (set, the server prints a
WARN and ignores them). Default is the whole host: CPUs = `docker info`
NCPU, memory = floor((host MiB − 2048)/128)×128 (read on first need,
cached once it succeeds); the setup VM is unlimited too. Kata adds the
limit to its own base (1 vCPU, 2 GiB), so `nproc` in a terminal shows a
set CPU count + 1, and the host's count when unlimited (vCPUs clamp at the
host). OOM order (ADR-085): project VMs sit at -999. The server and Caddy
run at `oom_score_adj -999` with `mem_limit` 4g / 1g (compose), so on a tie
the kernel kills the largest process (a VM), and a leak in either service is
an in-container OOM kill that the restart policy recovers. dockerd is put at
-999 with no start limit by `host-setup.sh` (`deploy/lib/docker-oom.sh`;
needs the post-release re-run). The host-side git the server runs is set
back to 0. The Docker disk defaults to the operator's "Default Docker
disk for new projects" setting (20 GB), recorded into the project at
creation, and grows offline at boot in `image/entrypoint.sh` (`e2fsck -f`,
`truncate`, `resize2fs`); it never shrinks, and a failure WARNs in the boot
log and is retried at the next boot. A stopped project whose container is
at another size than its record is recreated on its next start, so every
project created before this release rebuilds once (home, workspace and apt
packages survive; the rest of the container filesystem does not). API:
`api.md` "Projects".

**After this release, re-run `host-setup.sh` on each host.** It now writes
the `/usr/local/bin/containerd-shim-mvm-v2` wrapper pinning Kata to Cloud
Hypervisor (`configuration-clh.toml`) and sets `reclaim_guest_freed_memory
= true` (`deploy/lib/kata-config.sh`, checked by content each run,
BUG-089). The auto-updater never runs `host-setup.sh`, so the operator runs
`sudo bash /opt/styrman/deploy/host-setup.sh --user <operator>` (the token comes from `/etc/styrman/gh-token`; never on the command line)
as root from the verified tree (on the GCP VM over `deploy/gcp/vm.sh ssh`).
Until then a host without the wrapper (QEMU) may refuse project starts at
full size; memory sizing was measured on Cloud Hypervisor only. Measured on
the NUC 2026-10-05: 10 idle full-size VMs raised host memory in use from
4.1 to 15.8 GB; boot at full size 1.8 s vs 1.1 s; a 20 to 500 GB disk grow
0.14 s.

Both containers are `restart: unless-stopped`: a host reboot brings the whole
stack back via docker.service — no systemd units needed.

**Ingress reload and a failed load at boot.** Caddy's `/load` adapts the
Caddyfile first and, when the adapter has warnings, writes them to the response
before loading — which sends a 200; a load that then fails (at boot: no network
yet for the OIDC metadata fetch) can only say so in the response BODY, as an
`{"error": …}` object. On 2026-10-02 `crew.styrman.xyz` was down about six
hours after a host boot: the watcher checked the status alone, logged "caddy
reloaded successfully", recorded the Caddyfile as applied, and every later
scan said "caddyfile unchanged, skipping reload", so Caddy sat on its
admin-only config with nothing listening on 80/443. `reloadCaddy`
(`deploy/caddy/ingress/watcher.go`) now reads the response body and treats
caddy's error object as a failed load whatever the status; a failed load is
retried every 15 s (the retry the 2026-09-01 fix introduced for a 400).
Diagnosis from the host: `ss -ltn` shows no `:443`, and
`curl -s http://127.0.0.1:2019/config/` prints only the `admin` block. Until a
host runs the release with this fix (the release after v1.39.0),
`docker restart styrman-caddy` brings the ingress back once the network is up.

### Public preview links (optional, ADR-025)

Set `STYRMAN_PUBLIC_DOMAIN` (e.g. `public.crew.${BASE_DOMAIN}`) in
`deploy/.env` to turn on public, expiring preview links. One var drives both
sides of the stack:
- **Ingress** (`PREVIEW_DOMAIN=${STYRMAN_PUBLIC_DOMAIN}` env on the `caddy`
  service, `deploy/docker-compose.yml`): adds a `*.PREVIEW_DOMAIN`
  `ModePublic` wildcard site block (no OAuth — the link token is the
  credential), a wildcard DNS A record (`EnsureDNS`), and an auto-issued
  wildcard TLS cert — the DNS record and cert only with `CLOUDFLARE_API_TOKEN`
  set (DNS-01); without it the wildcard host gets no certificate and no DNS
  record is managed for you
  (`deploy/caddy/ingress/generator.go:25-30`, `deploy/.env.example:30-36`). The `styrman-server` container is the wildcard's target
  via its `caddy_preview_target: 'true'` label.
- **Server** (`STYRMAN_PREVIEW_BASE=${STYRMAN_PUBLIC_DOMAIN}`): each minted
  link is served at its own origin, `https://<token>.<PREVIEW_DOMAIN>/`, via
  `previewHostRewrite` mapping the subdomain to `/link/{token}/…` before
  `authGuard` runs.

Leave `STYRMAN_PUBLIC_DOMAIN` empty to keep the feature fully off — no
wildcard block, no DNS record, no cert. Enabling it **creates a public,
unauthenticated wildcard host** (`*.PREVIEW_DOMAIN`); every request there is
gated only by an unguessable 128-bit token with a 1h/24h/7d TTL, not by
Google auth. See `.agents/docs/specs/styrman/api.md` for the mint/list/revoke
endpoints and `internal/links` for the token store.

### Private share links (optional, ADR-026)

Set `STYRMAN_PRIVATE_DOMAIN` (e.g. `private.crew.${BASE_DOMAIN}`) to serve
per-port email shares on their own subdomains too. Same two-sided wiring:
- **Ingress** (`PRIVATE_DOMAIN=${STYRMAN_PRIVATE_DOMAIN}`): adds a
  `*.PRIVATE_DOMAIN` wildcard site block that **requires Google OAuth**
  (`writePrivateWildcard` reuses the server app's authorization policy — the
  JWT cookie is domain-scoped, so an existing session works), injects the
  verified identity headers + the ingress token, preserves Host, plus the
  wildcard DNS record and cert (both need `CLOUDFLARE_API_TOKEN`/DNS-01;
  without it the wildcard host gets no certificate and no DNS record).
- **Server** (`STYRMAN_PRIVATE_BASE=${STYRMAN_PRIVATE_DOMAIN}`): sharing a
  port mints a stable private link served at
  `https://<token>.<PRIVATE_DOMAIN>/` via the same host rewrite →
  `/shared/{token}/…`; `serveShared` then checks the ingress-verified email
  against the port's share list on EVERY request.

Unlike the public wildcard this host is never anonymous: an unauthenticated
visitor is 302'd to Google sign-in first, and an authenticated one sees the
app only if the owner shared that port with their email. Because each share
gets its own origin, the CSP sandbox that the same-origin `/p/` preview needs
does not apply — stateful apps work for email guests exactly as they do on
public links. The kinds are locked to their routes: a private token 404s on
`/link` (no-login) and a public token 404s on `/shared`.

### Bring-up

Prerequisites: a Google OAuth client (console.cloud.google.com → APIs &
Services → Credentials): Web application, redirect URI
`https://auth.<BASE_DOMAIN>/oauth2/google/authorization-code-callback`;
and an optional Cloudflare API token scoped Zone:DNS:Edit on the zone (when
omitted, Caddy uses HTTP-01 ACME challenges, which cover the base app and the auth portal only; the wildcard preview/private-link hostnames need DNS-01 and get no certificate, and DNS records are created manually).

```bash
# host-setup.sh's step 7 builds the runtime image, which downloads the sh-agent
# release from the private styrman-harness repo (ADR-074): on a fresh host no
# /etc/styrman/gh-token exists yet. That file is ALSO the auto-updater's root
# token, so it holds a FINE-GRAINED, read-only token (Contents: Read on
# kesokaj/styrman-crew and kesokaj/styrman-harness, an expiry), never your
# full-scope `gh` login token. Save it to a 600 file, then hand sudo the FILE:
# `sudo GH_TOKEN=…` and `sudo --preserve-env=GH_TOKEN` both log the value
# (`ENV=GH_TOKEN=…`, checked on dave 2026-10-05). Or
# `sudo deploy/styrman-ctl install --gh-token-file <600 file>`.
# Paste the token into a file only you can read (umask 077), end with Ctrl-D:
(umask 077 && cat > ~/fine-grained-token)
# Install it only when the host has none yet (never overwrite the updater's),
# never an empty one; remove the plaintext copy once installed.
if sudo test -e /etc/styrman/gh-token; then echo "a token is already installed; left as is"
elif ! test -s ~/fine-grained-token; then echo "~/fine-grained-token is missing or empty — nothing installed" >&2
elif ! sudo install -D -m 600 -o root -g root ~/fine-grained-token /etc/styrman/gh-token; then echo "install failed — nothing installed" >&2
elif ! shred -u ~/fine-grained-token; then echo "installed, but removing ~/fine-grained-token failed — delete it by hand" >&2
fi
sudo bash deploy/host-setup.sh --user $USER   # also seeds deploy/.env identity values
vi deploy/.env                                # fill: BASE_DOMAIN, GOOGLE_CLIENT_ID/SECRET,
                                              #       ADMIN_EMAIL, STYRMAN_ALLOWED_EMAILS
                                              #       (+ optional CLOUDFLARE_API_TOKEN,
                                              #       STYRMAN_HOSTNAME, STYRMAN_PUBLIC_DOMAIN /
                                              #       STYRMAN_PRIVATE_DOMAIN)
# Build + deploy at the release version — the exact commands (and why
# `git describe --tags --always` must NOT be used here: its SHA-suffixed
# fallback is a mutable-tag violation, ADR-053, precisely the mistake the
# compose `image:` lines' `${STYRMAN_VERSION:?...}` guard exists to catch)
# live in versioning.md's "Release procedure" / "Manual / break-glass
# fallback" — this first bring-up IS the break-glass case, since no
# auto-updater clone exists yet to do it for you. If this checkout has no
# reachable vX.Y.Z tag at all, pass STYRMAN_VERSION explicitly
# (e.g. STYRMAN_VERSION=0.1.0) rather than deriving one — a derived value
# must always be a real version, never a bare commit sha.
# → https://styrman.<BASE_DOMAIN>  (Google sign-in via auth.<BASE_DOMAIN>)
```

No mutable tags (operator rule) — see `.agents/docs/specs/styrman/
versioning.md` for the full release procedure, the three-image lineup, and
why this rule exists (two concrete 2026-09-01 incidents).

### What `host-setup.sh` pins/verifies

- **Docker CE**: installed at a pinned `DOCKER_VERSION` (currently `29.7.2`,
  override via env) via `get.docker.com`, not whatever is currently latest —
  reproducible installs (#11).
- **Kata Containers**: downloads `kata-static-${KATA_VERSION}-amd64.tar.zst`
  (Kata's releases are zstd tarballs with no `.sha256sum` sidecar any more —
  BUG-070) and extracts with `tar --zstd` (installing `zstd` first when it's
  absent). The tarball is SHA256-verified before extraction as root into `/`,
  checked in order against `KATA_SHA256` (an env pin), the pinned constant
  `KATA_SHA256_3_31_0` for the default `3.31.0`, or GitHub's release-asset
  digest for any other version; if none of those resolve (network issue, or
  the release has no digest published) the script **fails closed** rather
  than warning and continuing — pin `KATA_SHA256` to proceed offline (M15).
- **`/etc/docker/daemon.json`** merge (idempotent, only restarts Docker if it
  actually changed): the `mvm` runtime + `time-namespaces: false` (ADR-009,
  Docker ≥29.5 vs Kata ≤3.31), `ipv6`/`ip6tables: false` (the project-isolation
  firewall is IPv4-only, so an IPv6-enabled Docker bridge could otherwise
  bypass it), `live-restore: true` (running project microVMs survive a daemon
  restart), and `log-driver: json-file` with `log-opts {max-size: 50m,
  max-file: 3}` (bounds container log growth) — a `.bak-styrman` copy of any
  prior `daemon.json` is kept.
- **Host firewall (ADR-065)**: §5c runs `deploy/firewall/install.sh` after
  the state dir exists — the isolation + per-project-LAN script into
  `/usr/local/sbin`, its `.service`/`.timer`/`.path` units, a tmpfiles entry
  and a bounded tmpfs mount unit for `/run/styrman-firewall/spool`, the
  server-owned marker dir `netpolicy/lan`, and — only if absent —
  `/etc/styrman/firewall.conf` with the LAN prefix discovered from the
  default route (printed). Idempotent; the updater's `host_converge`
  re-applies the script and units from every verified tag afterwards.

### Verified without credentials

Images build; compose config validates; the containerized server passed the
header-auth matrix (401 no header / 403 unauthorized / 200 authorized /
health open) against the live state dir. The remaining untested step is the
Google OAuth handshake itself, which needs the real client + domain.

## Durability (no backup layer, operator decision 2026-09-01)

styrman does **not** back up anything, at either layer. The in-app
per-project backup feature (ADR-044, v1.13.0/v1.14.0) and the host-layer
restic→GCS disaster-recovery snapshot (v1.14.0) are both **removed
entirely** — code, routes, UI, systemd units, and the `/etc/styrman/
{backup-sa.json,restic-pw}` secrets they used. There is deliberately no
replacement. The operator's reasoning: "if a user needs backup they should
commit to git."

- **Durability is git.** Commit and push; whatever's in the remote survives
  a lost or wiped host.
- **A wipe-delete (`DELETE /api/projects/{id}?wipe=true`) is now plainly
  irreversible** — it removes the workspace, the project home, and the
  docker disk with no safety net. The UI's "type delete to confirm" dialog
  is the only gate; there is nothing to restore from afterward.
- **Rebuilding a host** is: clone the repo, `sudo deploy/styrman-ctl install
  --gh-token-file <600 file>` (the token travels as a file — a
  fine-grained, read-only one, since it becomes the updater's token; never
  through sudo's argv or environment — `sudo GH_TOKEN=…` and
  `--preserve-env` both land in sudo's log — see "Bring-up" above), fill in a fresh `deploy/.env`, bring the stack up. A
  GCP host: `deploy/gcp/vm.sh create` + `deploy/gcp/install.sh` (`gcp.md`). There is no state to restore — only project workspaces that
  were actually committed and pushed come back; anything left uncommitted
  in a project's workspace or home is gone with the old host.

## Auto-update (tag-gated, ADR-050/051)

### Agent harness in the image (ADR-074)

The agent harness (styrman-harness) reaches every project as the `sh-agent`
RELEASE binary with the harness embedded, baked into the runtime image at
build time — superseding both ADR-062's image mirror and ADR-073's host
harness channel (deleted: the two harness units, `styrman-update`'s
`harness` verb, `styrman-ctl`'s `--harness-token-file`, the
`/opt/styrman-harness` clone, `/etc/styrman/gh-token-harness`,
`STYRMAN_HARNESS_DIR` and the server's read-only harness binds).

**Image build.** `image/Dockerfile` downloads the release named by
`ARG HARNESS_VERSION` (default `v2.3.2`; was `v1.6.0` until 2026-10-02) with `gh release download` from
`kesokaj/styrman-harness` (assets `sh-agent-linux-amd64`, `SHA256SUMS`,
`SHA256SUMS.sig`), verifies `SHA256SUMS.sig` with `ssh-keygen -Y verify`
against the signer line in `ARG HARNESS_SIGNER`, checks the `SHA256SUMS`
header names that version, checks the binary's checksum, and checks
`sh-agent version` prints `sh-agent <HARNESS_VERSION>`. Bumping the harness
a crew release ships is editing that `ARG` line.

**Token.** The repository is private, so every image build takes a GitHub
token as a docker build SECRET (`--secret id=gh_token,…`, never a layer):
`GH_TOKEN` in the environment when set, else the updater's token file
`/etc/styrman/gh-token`, else the build stops before it starts. Sites:
`deploy/update/styrman-update` (deploy), `deploy/host-setup.sh` step 7,
`deploy/styrman-ctl install` (`--gh-token-file F` when given; a `--dry-run`
without a token plans the `src=` form and says a real run needs one),
`deploy/verify-commit.sh` and `.agents/docs/testing/styrman/ensure-test-image.sh`
(both fall back to `gh auth token`). The crew token at
`/etc/styrman/gh-token` must therefore also read `kesokaj/styrman-harness`
— the operator grants that; there is no separate harness token any more.

**Boot.** `image/entrypoint.sh` compares
`~/.styrman/.harness-version` (`<image VERSION>+<sh-agent release>`)
against the image's own; when it differs it runs `harness-migrate.sh`
(unchanged), then the browser MCP registration, and — since ADR-078, as a
background job AFTER `/run/styrman/ready` — `sh-agent install --home <home>`
(no `--repo`, since the harness is embedded in the binary) as the project
user under a 120 s cap, then the stamp, and logs `agent harness deployed
(version <VERSION>, sh-agent vX.Y.Z; browser MCP registered for claude and
agy)` (or a WARN line). Before ready: migrate (20 s cap), the
never-downgrade decision and the two MCP registrations (20 s each), worst
case 60 s. `/run/styrman/harness-done` is cleared at the top of every boot
and written when the harness step ends, whatever the outcome, on every
non-minimal boot. A `claude` or `agy` started before the install finishes
sees no skills until restarted. (Before ADR-078 the install ran before the
ready line, and the MCP registrations after it.)
`install` copies the binary to `~/.local/bin/sh-agent` (first on `PATH` via
skel/bashrc) and writes `~/.config/sh-agent/state.json` with
`"repo": "embedded:vX.Y.Z"`. Never-downgrade: when that manifest already
names a release NEWER than the image's, install is skipped and the log says
`agent harness: home has sh-agent vA (sh-agent update), image carries vB;
left alone`.

**Per-project update.** The user runs `sh-agent update` in a project
terminal; it needs `gh` (authenticated through the credential broker's
`GH_TOKEN`, so the user's own token must read the private harness repo too)
and `ssh-keygen` (both in the image); it downloads the newest release,
verifies it against the signer compiled into the binary, and replaces
`~/.local/bin/sh-agent`. Crew does nothing else for this: no timer, no
mount, no mirror. Projects may run different harness releases from each
other and from the image's baked-in one.

**Host cleanup.** The updater's `retire_harness_units` step runs on EVERY
tick, before `host_converge`, independent of its firewall guards:
`systemctl disable --now` the ADR-073 timer then service, removes both unit
files, `daemon-reload`, and removes `harness-version`, `harness-sha`,
`harness-failed-version`, `harness-refused` from `/etc/styrman/update`.
Under `--check` it only reports. The now-empty content dir
`/var/lib/styrman/harness` is an operator `sudo rmdir /var/lib/styrman/harness`
— it is not removed automatically. A host that ever ran the channel's setup
(`styrman-ctl update --setup --harness-token-file`, v1.35.0) also holds its
read-only token at `/etc/styrman/gh-token-harness` and its clone at
`/opt/styrman-harness`: revoke that fine-grained token at GitHub, then
`sudo rm -f /etc/styrman/gh-token-harness` and `sudo rm -rf /opt/styrman-harness`
— the updater names no production path of its own (ADR-052), so neither is
removed automatically; the production host never ran that setup.

Day-to-day "how do I ship a release / update one image" reference:
`.agents/docs/specs/styrman/versioning.md`.

Installed by `host-setup.sh` step 8c but ships **DISABLED** — enabling it is
a deliberate, separate action, not a side effect of a normal deploy, since
the timer's prerequisites (a dedicated clone, its own secrets, and a
root-owned `/etc/styrman/allowed_signers` for SSH-signed releases) don't
exist on a fresh host. **Do not enable the timer until `allowed_signers`
exists and releases are SSH-signed** — the signature gate fails CLOSED and
would refuse every tag it sees otherwise; `host-setup.sh` ships the timer
disabled for exactly this reason.

- **What it does**: `styrman-update` (+ `.service`/`.timer`) polls the
  origin every 5 min (`styrman-update.timer`, `OnUnitActiveSec=5min`,
  `RandomizedDelaySec=60`) and deploys the newest STABLE version tag —
  strictly `^v[0-9]+\.[0-9]+\.[0-9]+$` — so pre-release tags (`-rc1`,
  `-beta`) are ignored BY DESIGN and can be pushed to the remote for
  inspection without being auto-deployed. It picks the highest tag by
  `sort -V`, not the most recently pushed one. On every deploy it builds
  `styrman-runtime:X.Y.Z` (never `:dev`/`:latest`) plus BOTH compose images
  (server + caddy) at that same version, and passes `STYRMAN_VERSION`/
  `STYRMAN_IMAGE` explicitly to `docker compose` — shell env outranks the
  clone's own `deploy/.env` in compose precedence, so a stale line in that
  file can never silently re-point projects at a mutable tag.
- **The docker CLI's config dir is a per-run tmpfs, never a persistent
  path.** `docker build`/`docker compose` insist on creating `$DOCKER_CONFIG`
  (`/root/.docker` by default) — and `ProtectHome=yes` (below) makes `/root`
  inaccessible and empty, which killed the first real end-to-end deploy at
  `docker build`, and its rollback with it (2026-09-01 incident). The fix is
  NOT to park it under the trust-asserted state dir (that re-pins an
  unasserted path there, a review finding on the first attempt at this fix):
  `styrman-update.service` sets `RuntimeDirectory=styrman-update` +
  `RuntimeDirectoryMode=0700` and `Environment=DOCKER_CONFIG=%t/styrman-update/docker`
  — systemd creates and wipes that directory around every oneshot run, so no
  root-executed docker config, cli-plugins, or buildx state ever accretes
  anywhere. A hand run of `styrman-update` gets the same shape from the
  script itself: an inherited `DOCKER_CONFIG` is honoured ONLY when it
  already sits under `/run/` (i.e. the systemd `RuntimeDirectory` case
  above) — and even then only after its nearest EXISTING ancestor directory
  (the `RuntimeDirectory`'s `docker/` leaf may not exist yet) passes the
  same trust bar as any other root input: root-owned (uid 0) and not
  group/world-writable, since a `/run/*` path shape alone proves nothing
  (`/run/lock` is `1777`, `/run/user/<uid>` is uid-owned) — the script hard-
  fails otherwise. Any other inherited value, including one a `sudo -E`
  invocation might try to inject, is ignored, and the script `mktemp -d`s a
  fresh directory instead and registers an `EXIT` trap to remove it. Neither
  path is `/root/.docker`, `/etc/styrman/update`, or any other persistent
  location.
- **Where it runs**: a DEDICATED clone at `/opt/styrman`
  (`STYRMAN_DEPLOY_DIR`), never the operator's own working tree — the
  compose build reads `context: ..` from the repo root, so an automatic
  `git checkout <tag>` against a tree the operator is actively editing
  would stomp in-progress work.
- **Run-state lives under `/etc/styrman/update`**
  (`STYRMAN_UPDATE_STATE_DIR`), **not** `/var/lib/styrman`: `host-setup.sh`
  chowns the latter to the OPERATOR uid, and the rollback path (below)
  trusts a recorded commit SHA enough to SKIP re-verifying it — so a
  `/var/lib/styrman`-resident state dir would let the operator uid plant a
  `deployed-sha` and have root build an arbitrary commit down that trusted
  path (security review). **This is enforced by the script itself, not just
  by convention (ADR-052):** `styrman-update` has NO in-script default for
  `STYRMAN_UPDATE_STATE_DIR` at all — an unset value is a hard `fail`, not a
  fallback — and it explicitly REFUSES any value that is `/var/lib/styrman`
  or lives under it (canonicalized with `realpath -m` first, so
  `//var/lib/styrman/x` and relative paths can't slip past a literal-prefix
  match). It ALSO refuses the filesystem root (`/`) and any other bare
  top-level directory (`/etc`, a typo'd single-component path off the
  recommended `/etc/styrman/update`) — the original ADR-052 guard only
  checked the `/var/lib/styrman` prefix, so a one-component path passed it
  cleanly (root-owned, `0755`, no group/world-write bits) and scattered
  `deployed-version`/`deployed-sha`/`failed-version` into a shared system
  directory where nothing would ever find them (2026-09-02 audit). The state
  dir must resolve to at least two path components under `/`. The ONLY place
  the production path
  (`/etc/styrman/update`) is pinned is `Environment=` in
  `styrman-update.service`; a hand run or a test MUST pass its own value
  (production: `systemctl start styrman-update.service`; tests: a scratch
  prefix, e.g. `$(mktemp -d)`). This closed the 2026-09-01 state-loss
  incident, where a privileged test run against a production-aliasing
  default state path preceded `/var/lib/styrman` being emptied and
  `/etc/styrman` being `rm -rf`'d — see
  `.agents/docs/changes/2026-09-01-incident-state-loss.md` and the ADR-052
  testing convention in `.agents/docs/testing/styrman/README.md`.
  `/etc/styrman` is root-owned `0700`; the script
  creates `${STATE_DIR}` (and its parent) `root:root 0700` and, on every
  run, asserts it is still root-owned and not group/world-writable before
  trusting anything read from it. Three files live there: `deployed-version`
  (the tag last successfully deployed), `deployed-sha` (that tag's exact
  commit — see Rollback below), and `failed-version` (the tag most recently
  rolled back, so the timer stops retrying it). State was moved out of the
  deploy clone for a second reason too: `git clean -d` inside `deploy()`
  used to delete both dotfiles on every run when they lived at the clone
  root, wiping the failed-tag latch and reinstating the retry storm it
  exists to prevent (review finding, reproduced: both files gone after one
  clean).
- **Status file (ADR-081): `/etc/styrman/status/updater.json`.** The one
  part of `/etc/styrman` the server sees. `styrman-update.service` sets
  `STYRMAN_UPDATE_STATUS_FILE` to it (the script has no default: unset, it
  writes nothing) and the updater writes one JSON line at the end of every
  real tick (keys and codes: `versioning.md`'s auto-updater section).
  The status file's path must be absolute and canonical: no symlink, `..`
  or `//` anywhere on it. It must also consist of plain characters only,
  `A-Z a-z 0-9 . _ / -`; any other character (a newline, a space) is refused
  the same way. Every directory above the status directory, from
  `/` down, must be a real directory owned by root (or by the updater's own
  uid) that no one else can write; a directory others can write is accepted
  only with the sticky bit, which stops them renaming entries they do not
  own. `/etc/styrman/status` itself must be a real directory owned by root,
  not group/world-writable; the updater creates it (0755) when missing and
  its parent passes the same check. Otherwise it logs `WARN: status not
  written: …`, writes nothing, and the tick's own result is unchanged. `--check` and a tick
  that found the lock busy or refused before taking it write nothing.
  `styrman-ctl`'s hand runs (`update --setup`'s `--check`,
  `update --redeploy`'s `--force`) pass
  `STYRMAN_UPDATE_STATUS_FILE=${ETC_DIR}/status/updater.json`, so a forced
  redeploy records its result at once. `docker-compose.yml` gives the
  server the same variable and the read-only mount
  `/etc/styrman/status:/etc/styrman/status:ro`; Docker creates a missing
  directory as an empty root-owned 0755 one, so a host without auto-update
  starts fine and shows nothing. The server (`internal/updatestatus`,
  config `UpdateStatusFile`, no default; unset turns the feature off) reads
  at most 4 KiB, decodes strictly and re-validates every field: `ok`,
  `failing`, `stale` (a valid file whose `checkedAt` is more than 60
  minutes old; the timer runs every 5 minutes and one tick may take 45), or
  `unknown` (present but unreadable, malformed, or a value outside its set;
  never shown as ok). No file: nothing. Admins see it in Settings
  (`api.md`). The first status appears one tick (about 5 minutes) after a
  host takes this release, because the deploying tick runs the previous
  updater, which then installs the new one (`self_install`).
- **Tags must be SSH-signed, against an allowed-signers allowlist.** The
  updater checks out, `docker build`s, and `docker compose up`s the tag AS
  ROOT, and `docker-compose.yml` itself lives inside that checkout — so it
  fails CLOSED rather than trusting "anyone who can push a tag": a
  lightweight tag (`git cat-file -t` ≠ `tag`) is rejected outright. A bare
  `git verify-tag` is deliberately NOT what's used — it exits 0 for a
  good-but-**untrusted** signature, so with PGP any key ever imported into
  root's keyring would pass and git never compares signer to tagger. Only
  the SSH signature path consults an allowlist, so the updater requires
  one — and the check is stricter than a plain grep for the marker line: it
  takes the **LAST** `-----BEGIN (SSH|PGP) SIGNATURE-----` header out of
  `git cat-file tag <tag>`, matching git's own `parse_signature`, and
  refuses unless that last header is exactly
  `-----BEGIN SSH SIGNATURE-----` — **a PGP-signed tag is REFUSED, not
  merely unpinned**; signing with GPG will not satisfy this check. A plain
  first-match grep was bypassable: `git cat-file tag` also prints the
  author-controlled tag MESSAGE, so a decoy `-----BEGIN SSH SIGNATURE-----`
  line planted in the message satisfied a naive grep while a real PGP
  signature block appended after it still won verification — `verify-tag`
  then exited 0 with the SSH allowlist never consulted (security review,
  reproduced). Once the SSH-signature shape is confirmed, it verifies the
  tag against a root-owned, non-group/world-writable
  `/etc/styrman/allowed_signers`, read from that FIXED path rather than
  root's gitconfig: `styrman-update.service` sets `ProtectHome=yes` and
  never sets `$HOME`, so `git config --global` is unreadable under the
  timer even though it works fine when the operator runs the same command
  by hand. Sign every release with:
  ```bash
  git config gpg.format ssh && git config user.signingkey ~/.ssh/id_ed25519.pub
  git tag -s vX.Y.Z -m "styrman vX.Y.Z"
  ```
  The only opt-out is a root-owned, non-group/world-writable
  `/etc/styrman/update.env` setting `STYRMAN_UPDATE_REQUIRE_SIGNED=0` —
  READ with `grep` against an anchored pattern (tolerating `export` and
  quoting), never sourced: `. /etc/styrman/update.env` as root would
  execute whatever the file contains, and a redefined `log(){ :; }` in it
  would silence the "signature checking DISABLED" warning below, giving a
  silent skip path (security review). Ownership and mode are asserted
  before the file is read at all. The warning logs on every run so the
  opt-out can't be silently disabled from anywhere the operator doesn't
  already have root. **Operator-visible consequence**: existing tags
  v1.14.0–v1.15.0 are annotated-but-unsigned or lightweight, so the
  updater would refuse every one of them as-is — all releases from this
  point forward must be SSH-signed as above (plain `-s` with a GPG key
  does NOT satisfy this). See ADR-051.
- **`deploy/.env` is itself an asserted trust input**, not just a file
  whose presence is checked. Before touching anything, the script confirms
  `${DEPLOY_DIR}/.git` and `${DEPLOY_DIR}/deploy/.env` are root-owned, and
  that `deploy/.env`'s mode is not readable/writable beyond root (`chmod
  600`) — it holds `STYRMAN_INGRESS_TOKEN`/`STYRMAN_ALLOWED_EMAILS`. It is
  gitignored, so it survives both `git checkout --force` and `git clean -d`
  across every deploy.
- **How it verifies a deploy**: reads `LABEL styrman.version` off the
  running `styrman-server` container with `docker inspect`, plus
  `.State.Running` and `.State.Health.Status` — **no HTTP request and no
  auth** (ADR-050's correction: the original design polled `/api/version`,
  but production runs `STYRMAN_AUTH=header` and the updater has no ingress
  identity, so every healthy release would 403, time out, and roll itself
  back). **Deleting the `styrman.version` LABEL from
  `deploy/Dockerfile.server` breaks the updater**: an absent label reads as
  an empty string, which never matches the wanted version, so EVERY future
  release fails verification and gets rolled back. This is a load-bearing
  cross-file contract between that Dockerfile and `styrman-update`, not an
  implementation detail — currently held together only by comments in both
  files.
- **Rollback**: if the new tag doesn't report itself healthy within
  `STYRMAN_UPDATE_HEALTH_TIMEOUT` (default 120s), the updater latches the
  failed tag at `${STATE_DIR}/failed-version` (so the 5-minute timer does
  not retry it forever) and redeploys the PREVIOUS version. **It prefers
  the exact commit over the tag name.** On every successful deploy made
  while `STYRMAN_UPDATE_REQUIRE_SIGNED=1` (the default), the script records
  `git rev-parse <tag>^{commit}` at `${STATE_DIR}/deployed-sha`; a deploy
  made during an opt-out window (`REQUIRE_SIGNED=0`) deliberately does
  **not** record one, so a "trusted" SHA can never survive an opt-out
  window into signing being re-enabled. When a usable (full 40-hex) SHA is
  on record, the rollback calls `deploy()` with a third `trusted` argument
  that **skips `verify_tag` entirely** — that exact tree was already
  verified when it was first deployed, and rolling back by SHA is also
  immune to `git fetch --tags --force` having since re-pointed the tag
  name. Only when no SHA was recorded (state dir doesn't have one yet, or
  the previous deploy happened during an opt-out window) does it fall back
  to rolling back onto the previous TAG, which then must verify exactly
  like a forward deploy — that fallback would refuse a rollback onto any
  release made before SSH signing was adopted (v1.14.0–v1.15.0), which is
  why the SHA-based path, not the tag fallback, is what makes rollback
  actually work across the signing-adoption boundary. `--force` clears the
  failed latch (and also forces a redeploy of the current tag even if
  nothing changed).
- **It installs itself too**, so a release that only touches
  `deploy/update/*` no longer needs a manual step. The script and its units
  live OUTSIDE the stack they deploy — `/usr/local/bin/styrman-update` plus
  `styrman-update.{service,timer}` in `/etc/systemd/system`, all placed
  there once by `host-setup.sh` step 8c — so a prior release that changed
  only those files deployed every container while leaving the OLD updater
  running (bit the operator 2026-09-01/02, tracked as a TODO before this
  closed it). After EITHER a forward deploy or a rollback reports itself
  healthy, `self_install()` pins the verified commit ONCE
  (`SELF_HEAD="$(git rev-parse HEAD)"`), so every read below resolves
  against that exact SHA even if the checkout is mutated mid-run, then for
  each of `deploy/update/{styrman-update,styrman-update.service,
  styrman-update.timer}` checks it's present in that commit with
  `git cat-file -e "${SELF_HEAD}:<path>"` (a missing file skips the whole
  install with a `WARN`) and, for anything `differs()` reports changed
  (`git show "${SELF_HEAD}:<path>" | cmp -s - <installed>`), writes
  `git show "${SELF_HEAD}:<path>" > "<target>.new.$$"`, `chmod`s and
  `chown root:root`s that temp file, then `mv -fT`s it over the live
  target. **The worktree is never read** — only the verified git object at
  `SELF_HEAD`: the checkout stays a writable directory after verification,
  so reading from it (rather than the object store) would let a local user
  with write access swap a file in post-verification and have it installed
  anyway. `-T` makes a directory sitting at the target path FAIL the
  install rather than silently absorb the file into itself (a plain
  `mv -f` onto a directory "succeeds" that way). Orphaned
  `<target>.new.<pid>` files from an earlier interrupted attempt are swept
  for **all three** targets up front, every run, not just the ones that
  changed this time. The atomic rename means the currently-running script
  keeps its own already-open (old) inode and the swap is safe mid-run; the
  NEW script/units take effect on the NEXT tick (`systemctl daemon-reload`
  re-reads the units immediately when anything changed — nothing already
  running is restarted, and `systemctl enable` of the timer stays
  `host-setup.sh`'s job: self-install never enables or disables it). Since
  reading `deploy/update/*` now means reading arbitrary paths out of the
  object store, `${DEPLOY_DIR}/.git` must be root-owned **and** not
  group/world-writable, not just root-owned — the general trust-input check
  above only asserted ownership; this is the extra bar self-install added.
  **It only ever fires when signature enforcement is ON**
  (`REQUIRE_SIGNED=1`, the default): the `/etc/styrman/update.env` opt-out
  that accepts unsigned tags NEVER self-installs, logging a skip instead —
  an unverified tree must not become the thing that decides whether
  signatures are checked next time (the rollback path's own SHA-trust
  exception doesn't extend here; it only skips re-verifying a SHA this host
  already verified when first deploying it). The two install targets are
  also validated before use, the same trust bar as every other root input
  in this script: the basename of `STYRMAN_UPDATER_BIN` must be exactly
  `styrman-update` (a mismatch logs a `WARN` and skips), and both
  destination directories must exist, be root-owned, and not be
  group/world-writable (`trusted_dir()`; otherwise `WARN` and skip). A
  self-install failure — including a partial one where the binary installs
  but a unit file doesn't, or vice versa — is logged as a `WARN`, never
  fails an already-verified, already-healthy deploy, and suppresses the
  "updater self-installed from the verified tree" success line: that line
  prints only when something changed AND nothing failed. The two targets
  are pinned the same ADR-052 shape as the state dir above — **no
  in-script default**: `STYRMAN_UPDATER_BIN`/`STYRMAN_UPDATER_UNIT_DIR`,
  set only by `Environment=` in `styrman-update.service`
  (`/usr/local/bin/styrman-update`, `/etc/systemd/system`); a hand run or a
  test that leaves them unset logs `self-install skipped: ... unset (the
  unit sets them)` and installs nothing.
- **One-time exception**: v1.17.3, the release that introduces self-install,
  is itself deployed by the OLD `styrman-update` already running on the
  host — the one without this logic — so it still needs one FINAL manual
  reinstall right after that one deploy completes:
  ```bash
  cd /opt/styrman
  sudo install -m 755 -o root -g root deploy/update/styrman-update /usr/local/bin/styrman-update
  sudo install -m 644 -o root -g root deploy/update/styrman-update.service deploy/update/styrman-update.timer /etc/systemd/system/
  sudo systemctl daemon-reload
  ```
  Every release after v1.17.3 self-installs from its own verified git object;
  this manual step is never needed again.
- **Flags**: `styrman-update --check` (report what would happen, changes
  nothing) and `styrman-update --force` (see above).

Setup — three prerequisites, since the timer stays disabled until all
exist (`host-setup.sh` step 8c prints these exact commands). `host-setup.sh`
does **not** auto-clone — it prints these steps and leaves them to the
operator:

1. A deploy CLONE at `/opt/styrman`. **The host holds NO SSH key at all**
   (operator decision 2026-09-01): auth is instead a **FINE-GRAINED GitHub
   token** with `Contents: Read-only` on this repository AND
   `kesokaj/styrman-harness` (the image build downloads `sh-agent` from it,
   ADR-074) and nothing else, root-owned
   at `/etc/styrman/gh-token`, fed to git over HTTPS via an inline
   credential helper — never embedded in the remote URL (that would land in
   `.git/config` and every process listing). TLS to `github.com` replaces
   the SSH `known_hosts` trust anchor the old deploy-key flow needed, so
   there is nothing to seed or pin here. Mint the token in the GitHub UI
   (Settings → Developer settings → Personal access tokens → Fine-grained
   tokens: Repository access = only those two; Permissions = Contents:
   Read-only; pick an expiry you will honor — a lapsed token makes the
   updater fail LOUDLY at fetch, not silently):
   ```bash
   sudo install -d -m 700 /etc/styrman
   # paste the token when prompted — never into shell history:
   sudo bash -c 'umask 077; read -rs t; printf "%s" "$t" > /etc/styrman/gh-token'
   # HTTPS origin; the token never goes in the URL, only through the helper
   # reading that root-only file:
   sudo git -c credential.helper= \
        -c 'credential.helper=!f() { echo username=x-access-token; printf "password=%s\n" "$(cat /etc/styrman/gh-token)"; }; f' \
        clone https://github.com/<owner>/<repo>.git /opt/styrman
   sudo chown -R root:root /opt/styrman
   ```
2. That clone's OWN `deploy/.env` (gitignored, so a fresh clone has none)
   — copy the operator's filled-in `deploy/.env` into it:
   ```bash
   sudo cp deploy/.env /opt/styrman/deploy/.env
   sudo chmod 600 /opt/styrman/deploy/.env
   ```
3. A root-owned `/etc/styrman/allowed_signers` allowlist, and releases
   signed with SSH (a PGP-signed tag is refused, see "Tags must be
   SSH-signed" above):
   ```bash
   printf '%s %s\n' "<your-git-email>" "$(cat ~/.ssh/id_ed25519.pub)" \
     | sudo tee /etc/styrman/allowed_signers
   sudo chmod 644 /etc/styrman/allowed_signers   # root-owned, not group/world-writable
   # signing side:
   git config gpg.format ssh && git config user.signingkey ~/.ssh/id_ed25519.pub
   git tag -s vX.Y.Z -m "styrman vX.Y.Z"
   ```

Then `sudo styrman-update --check` (dry run) and
`sudo systemctl enable --now styrman-update.timer`. Pushing a signed
`vX.Y.Z` tag afterward deploys it within ~5 minutes. **Do not enable the
timer before all three prerequisites exist** — the signature gate fails
CLOSED, so a timer enabled early would simply refuse every tag it polls.

## Clean reinstall (virgin state)

Long form: `.agents/docs/changes/2026-08-28-clean-reinstall.md`. Short form:
stop the stack, remove every `styrman-*` container and `sdb-*` network, wipe
`/var/lib/styrman/{homes,projects,users,credentials}` (keep the dir)
AND the state-root files `users.json`, `links.json`, `settings.json` —
otherwise stale accounts, share links, and admin tunables survive a
"virgin" reinstall — then bring the stack back up. **REQUIRED: restart the
styrman server AFTER scrubbing `StateDir/users`** — the server caches
per-user spaces in memory, and skipping the restart leaves stale user state
that 400s the next first login (this exact omission caused the v1.5.2
first-login failure, fixed v1.5.3 / ADR-033).
`docker compose -f deploy/docker-compose.yml up -d --force-recreate
styrman` after the scrub satisfies it.

## Project→host/LAN network isolation (R12, ADR-029, chain-based self-heal since 2026-08-30; per-project LAN exception since ADR-065)

Every project's Docker bridge is named `sdb-<projectID>` (`config.NetworkName`),
but Docker's actual Linux bridge INTERFACE would default to `br-<hash>` unless
told otherwise — and the isolation firewall matches on the interface name, not
the Docker network name. `config.BridgeNameFor` derives a short (≤15 char,
interface names are capped), sha1-based `sdb-<11hex>` interface name from the
network name; both `NetworkCreate` call sites (`internal/orchestrator/
orchestrator.go` — initial project creation and the missing-network recreate
path) pass it as `Options: {"com.docker.network.bridge.name": ...}`. Until
this was fixed (v1.8.3) the isolation rules matched nothing and a project VM
could reach the host, the LAN, and cloud metadata — confirmed live before the
fix landed; existing projects' networks had to be recreated once to pick up
the named bridge.

`deploy/host-setup.sh` installs `/usr/local/sbin/styrman-firewall.sh`, which
uses two DEDICATED chains rather than direct rules on `INPUT`/`DOCKER-USER`:
- `STYRMAN-IN` (jumped to from `INPUT` for `-i sdb-+`) — a single `DROP`:
  a project bridge may send the host nothing (its gateway IS the host, so
  this is the only path that matters for host access).
- `STYRMAN-FWD` (jumped to from `DOCKER-USER` for `-i sdb-+`, which runs
  first in FORWARD and survives Docker restarts) — first a `physdev
  --physdev-is-bridged` `RETURN` exemption, then a `DROP` for each of
  `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`,
  `100.64.0.0/10` (CGNAT — where Tailscale puts a tailnet) and
  `198.18.0.0/15` (the benchmark range some VPNs route real traffic on; both
  added 2026-09-02, audit finding). The physdev exemption lets a project reach the credential broker and the
  port-preview proxy on its OWN bridge (both are the server's IPs there):
  with `bridge-nf-call-iptables=1`, genuinely same-bridge L2 traffic still
  traverses `DOCKER-USER`, so without this RETURN the broker was silently
  dropped alongside everything else (the regression this fixes, H5).

Public internet, return traffic (`-o sdb-+`), and the same-bridge broker +
preview proxy (via the physdev exemption) are all unaffected; another project
or system bridge, the LAN, and cloud metadata are all in the dropped ranges,
so cross-project traffic stays blocked.

IPv6: since this isolation is IPv4-only, the script also sets
`net.ipv6.conf.<bridge>.disable_ipv6=1` on every `sdb-*` interface and
best-effort mirrors the drop rules into `ip6tables` (`STYRMAN-IN6`/
`STYRMAN-FWD6` for `fc00::/7`/`fe80::/10`) — defense in depth against a future
IPv6-capable bridge bypassing an IPv4-only firewall. `host-setup.sh`'s
`daemon.json` merge also turns `ipv6`/`ip6tables` off at the Docker daemon
level for the same reason.

Rules are applied idempotently via **INSERT-IF-MISSING**, not
delete-then-insert: each chain's own rules use `iptables -C` (reliable once
the wildcard match lives only on the chain JUMP, tested textually via `-S`
rather than `-C`) to check before inserting, so the self-heal timer never
opens a window where isolation is briefly absent — the old delete-then-insert
approach had one every 60s. A one-time migration step deletes any pre-chain
direct rules from earlier versions on first run.

Self-healing: `styrman-firewall.service` (a plain oneshot, NOT
`RemainAfterExit` — so it can be re-triggered) is re-run by
`styrman-firewall.timer` (`OnBootSec=15s`, `OnUnitActiveSec=60s`) every
minute, so a Docker daemon restart, a firewall-manager reload, or a manual
flush is undone within ~1 min rather than surviving until the next reboot.
The `sdb-+` wildcard means new project bridges are covered automatically, no
per-network rule needed.

**Where it lives and how it ships (ADR-065).** The script is
`deploy/firewall/styrman-firewall.sh` (it was a heredoc in `host-setup.sh`
until 2026-09-08), installed by `deploy/firewall/install.sh` — which
`host-setup.sh` §5c calls — and re-applied, together with its three units,
by the auto-updater's `host_converge` from every verified release tag. That
closes the gap that left this host running a 2026-08-30 script with four
DROPs for six days after the repo had six: nothing used to deliver a
host-setup change. `install.sh` is idempotent; run it by hand once per host
(from `/opt/styrman` at the deployed tag) and after that only when you want
to re-discover the LAN prefix (delete `/etc/styrman/firewall.conf` first —
the installer writes it ONLY IF ABSENT). Confirm with
`iptables -S STYRMAN-FWD`, `systemctl status styrman-firewall.timer
styrman-firewall.path` and `cat /run/styrman-firewall/status`.

**The per-project LAN exception (ADR-065).** An admin may allow individual
projects, each toggled separately, to reach the operator's local network —
exactly the prefixes in root-owned `/etc/styrman/firewall.conf`
(`LAN_DEV=`, `LAN_CIDRS=`; validated on every run: `/16`..`/30`, host bits
zero, no overlap with 172.16/12, 169.254/16, 100.64/10, 198.18/15, 127/8,
0/8, 224/3 or any Docker bridge's connected route — an invalid value closes
EVERY allowance and reports `conf_error=`). The server records intent in
`projects.json` and projects it as an empty marker file
`/var/lib/styrman/netpolicy/lan/<uuid>` whose NAME is the whole grammar
(root reads names only, regex-validated; contents are never parsed), then
drops a trigger into the root-parented spool `/run/styrman-firewall/spool`
(a 1 MiB / 1024-inode tmpfs owned by the server's uid). The script's LAN
section runs after the base rules: per marker and per prefix it keeps one
rule at the HEAD of `STYRMAN-FWD`,
`-i sdb-<hash> -o <LAN_DEV> -s <bridge subnet> -d <cidr> -m comment
--comment "styrman-lan:<uuid>" -j RETURN` — RETURN, not ACCEPT, so Docker's
own cross-bridge DROP still runs; `-s`/`-o` pinned because `rp_filter` is
loose and MASQUERADE matches only the bridge subnet. These tagged RETURNs
are the ONE class of rule the script deletes on demand (marker gone = rule
gone, tighten before loosen, always by full spec — never by a rule number
another root writer could shift); the base rules keep the never-delete
promise. Root writes `/run/styrman-firewall/status` (root 0640, the state
dir's group) — `version=`, `applied_at=`, `lan_cidrs=`, `lan=<uuids>`,
`pending=`, `error=`, `conf_error=`, `warn=` — and the server derives each
project's `off / pending / on / error / unavailable` from it, refusing an
enable with 503 whenever that file is absent, stale, below
`netpolicy.MinScriptVersion`, or without a prefix. `styrman-firewall.path`
(`DirectoryNotEmpty` on the spool) applies a toggle in about 0.2 s; the 60 s
timer remains the backstop; the path unit carries no `After=docker.service`
(an ordering cycle) and is revived by the script at most once per ten
minutes. The allowed traffic is masqueraded like any other bridge egress, so
it reaches the LAN as the HOST's address, not as a per-project one: a LAN
ACL keyed on this machine's IP admits every LAN-enabled project, and the LAN
cannot tell them apart. That is inherent to the NAT topology, not something
the allow rule chooses, and it is the reason enabling is admin-only.
Still dropped for a LAN-enabled project: the host itself
(`STYRMAN-IN`), host-published Docker ports (the drop sees the post-DNAT
container address), every other project and system bridge, the ingress
bridge, cloud metadata, the tailnet; LAN hosts cannot initiate connections
in; the setup VM can never be allowed (its network name is not a uuid).
Reserved conf keys, parsed as 0 until their own ADR: `LAN_ALLOW_HOST`,
`LAN_ALLOW_PUBLISHED_PORTS`.

## Caddy ingress upstream selection (multi-homed server)

The `styrman-server` container is attached to **three** Docker networks in a
typical deploy: `styrman-ingress` (Caddy → server), plus two project bridges
(`sdb-*`, joined for the credential broker and the port-preview proxy). Go
map iteration over a container's networks is random-order, so naively picking
"the first IP" (`deploy/caddy/ingress/watcher.go`'s `extractContainerIP`) can
return a dead project-bridge IP instead of the reachable ingress IP — this
happened live (v1.8.5): Caddy proxied `crew.<domain>` to a `sdb-*` bridge
address, which timed out, producing a 502 the browser saved as a bare
`document.txt` (no content-type on the error body). It surfaced after the
v1.8.3 network recreation reshuffled the map; the bug was latent since the
server first became multi-homed.

Fixed to select deterministically, in order:
1. The `styrman-ingress` network's IP, if the container is on it (the normal
   case for `styrman-server`).
2. Otherwise any network NOT named `sdb-*` (avoids project bridges).
3. Otherwise whatever's there — a **preview VM** legitimately has only its own
   `sdb-*` network, and that IS the correct target for it.

Host-network containers still resolve to `127.0.0.1` (a prior, unrelated
branch). Locked in by `TestExtractContainerIP`
(`deploy/caddy/ingress/watcher_ip_test.go`).

## Credential broker (ADR-030/032)

The containerized server runs a second, credential-only listener
(`STYRMAN_BROKER_ADDR`, default `:8091`). It is **bridge-only**: the server
binds a fixed IP (subnet broadcast−1) on every project bridge it attaches
to, and each project receives that URL as `STYRMAN_CRED_BROKER`. Caller
identity is network position (the source IP maps to the owning project) —
so the port must NEVER be published to the host or the ingress; there is
nothing to configure for it beyond leaving it unpublished (the compose file
does not expose it).

When the server attaches (`EnsureSelfAttached`) varies by path: a terminal
boot attaches SYNCHRONOUSLY, before the WebSocket upgrade (`wsBoot` in
`internal/httpapi/server.go`), because the login shell the terminal is
about to spawn needs the broker reachable immediately and there is no HTTP
response left to protect by then; project create, start, and a LAN-mode
toggle attach in a background goroutine AFTER their response; the
port-preview proxy attaches lazily, on the first proxied request; and a
container already running at server start is adopted at boot time. Before
`wsBoot` existed, the terminal-boot path only booted the VM and relied on
one of the other paths to have attached already, so the first terminal of a
project that was stopped across a server restart came up with the broker
unreachable ("credentials: the broker did not answer" MOTD) — BUG-060.

On-disk: the broker's store lives at `<state>/credentials/<user>/` (0600
JSON files — git tokens, env tokens, injected files). There is no backup
layer (see "Durability" above) — losing the host loses this store too;
re-run `styrman-setup` (or Settings → Credentials) on the rebuilt host to
re-onboard. Deleting a file revokes that credential for new sessions.
Onboarding is now automatic (ADR-034, v1.5.4): finishing first-run
`styrman-setup` harvests every credential the user just signed into — from
the setup VM's home (`homes/<user>/base`) — straight into the broker, so every project
created afterwards is current with zero re-auth. `Settings → Credentials →
Import from a project` (`POST /api/settings/credentials/import`) still
exists as a supplementary/refresh path — for rotated tokens, or a tool the
user signed into inside a project instead of at setup time.

Per-project homes (ADR-031) live at `<state>/homes/<user>/p/<uuid>` —
each mounted as that project's `$HOME`; the setup VM's home is
`homes/<user>/base` (ADR-064, v1.21.0 — it was `homes/<user>` itself, which
put every project home inside the setup VM's mount; the server moves an
old base home into `base/` once at startup). `homes/<user>` is server-only
and mounted into no guest. The old shared-home model (and
`deploy/home-init.sh`) is gone: the base image seeds every new home.
