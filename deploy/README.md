# deploy/ — production deployment

## Quick start

```bash
sudo deploy/styrman-ctl install
```

`styrman-ctl` (plan `../.agents/docs/plans/2026-09-11-installer-ctl.md`) is
the one front door: it wraps the scripts below and never reimplements them.
Underneath, `install` runs exactly this:

```bash
sudo bash host-setup.sh --user $USER
vi .env   # secrets: OAuth client, Cloudflare token, allowed emails
STYRMAN_VERSION=<x.y.z> docker compose up -d --build
```

`host-setup.sh` converges a fresh Debian 13 host (Docker, Kata `mvm`,
tuning, state dirs, runtime image, `.env` scaffold) — idempotent, R11.
The `mvm` runtime is a wrapper at `/usr/local/bin/containerd-shim-mvm-v2`
that pins Kata to Cloud Hypervisor, and `configuration-clh.toml` gets
`reclaim_guest_freed_memory = true` (`lib/kata-config.sh`, BUG-089, ADR-083).
Both are checked by content on every run, so re-running `host-setup.sh`
repairs a host that drifted; tested without root by
`lib/kata-config-test.sh` (run by `go test ./internal/config/`).
`docker-compose.yml` runs the vendored Caddy ingress (Google OAuth, TLS,
Cloudflare DNS — `caddy/`) in front of the containerized server
(`Dockerfile.server`).

### Verbs

| Verb | Does |
|---|---|
| `status [--json]` | what is installed, deployed and running; exit 0 ok, 1 degraded, 2 nothing installed |
| `install [--user U] [--version X.Y.Z] [--env-file F] [--gh-token-file F] [--allowed-signers-file F] [--skip-auto-update]` | `host-setup.sh`, the five must-fill `.env` keys (`BASE_DOMAIN`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ADMIN_EMAIL`, `STYRMAN_ALLOWED_EMAILS`; validated; `CLOUDFLARE_API_TOKEN` is optional and written too when `--env-file` carries it — an invalid value stops the install, never prompted for), build the runtime image (the `--gh-token-file` token is also the docker build secret for the image's `sh-agent` download, ADR-074 — it must read `kesokaj/styrman-harness` too; an empty or whitespace-only token file is refused by name, as `styrman-update` refuses one), bring the stack up at the checkout's release tag (or `--version`), offer auto-update setup. `--gh-token-file` is the preferred way to hand `install` a token: it never appears on a command line, unlike the bare `GH_TOKEN=$(gh auth token) bash host-setup.sh` form (`install` exports it for `host-setup.sh` in its own process) |
| `update` / `update --redeploy` / `update --setup […]` | run one updater tick now; `--redeploy` forces the current tag and clears a latched failure; `--setup` performs the auto-update setup on its own (token, clone, `.env` copy, `allowed_signers`, timer) — the 8c prose below, as prompts |
| `edit [KEY=VALUE …] [--no-apply]` | edits `.env` (both copies when the clone exists, validated — a `KEY` not matching `[A-Z][A-Z0-9_]*` is refused outright) and recreates the stack at the deployed version |
| `uninstall [--level 1|2|3] [--confirm WORD]` | tears down at one of three levels (below); prints the exact plan before asking |

Global flags: `--dry-run` prints every command a verb would run, as `+ cmd`,
and runs nothing; `--yes` takes defaults and skips non-destructive
confirmations; `--confirm WORD` satisfies a destructive confirmation
unattended. Any of the three may come BEFORE the verb as well as after
(`styrman-ctl --dry-run status` ≡ `styrman-ctl status --dry-run`, 2026-09-13)
— flags are collected from the whole argv; the first non-flag token is
still the verb.

### Uninstall levels

| Level | Confirmation word | Removes | Keeps |
|---|---|---|---|
| 1 — stack | `stop` | the stack, every project microVM + network, disables the units | everything else (data, host config, images) |
| 2 — + host config | `uninstall` | level 1, plus firewall chains, unit files, binaries, `/etc/styrman`, the deploy clone, the images, `deploy/.env` (backed up first) | `/var/lib/styrman`, Docker, Kata |
| 3 — everything | the host's name | level 2, plus `/var/lib/styrman` (every home, workspace, user store, transcript) — irreversible | nothing |

### Path env overrides (test seams, ADR-052)

`STYRMAN_STATE_DIR`, `STYRMAN_DEPLOY_DIR`, `STYRMAN_ETC_DIR`,
`STYRMAN_UNIT_DIR`, `STYRMAN_BIN_DIR`, `STYRMAN_SBIN_DIR`,
`STYRMAN_TMPFILES_DIR`, `STYRMAN_KATA_DIR`, `STYRMAN_ENV_FILE` — the
defaults are the production locations `host-setup.sh` and the units already
pin; a test (or a dry run against a scratch tree) overrides every one of
them instead of aliasing `/var/lib/styrman`/`/etc/styrman`.

Full walkthrough: `../.agents/docs/specs/styrman/deploy.md` (including the
credential broker's bridge-only listener). There is no backup layer —
durability is git: commit and push, and rebuild a lost host from this
script + the repo.

`firewall/` — the host firewall half (ADR-065): `styrman-firewall.sh` (R12
isolation + the per-project LAN reconciler), its `.service`/`.timer`/`.path`
units, `install.sh` (idempotent; discovers the LAN prefix once into
`/etc/styrman/firewall.conf`) and `test.sh` (the real script under an
unprivileged namespace, run by `go test`). `host-setup.sh` §5c calls
`install.sh`; the updater's `host_converge` re-applies the script and units
from every verified tag.

`update/` — the tag-gated auto-updater (ADR-050/051): `styrman-update` +
`.service`/`.timer` units, installed by `host-setup.sh` step 8c but shipped
**DISABLED**. Enabling it is a deliberate, separate operator action — it
requires a dedicated clone at `/opt/styrman`, its own `deploy/.env`, and a
root-owned `/etc/styrman/allowed_signers` for SSH-signed releases before the
timer is turned on. The signature gate (`verify_tag`) recognises all four
git tag-signature markers (`SSH SIGNATURE`, `PGP SIGNATURE`, `PGP MESSAGE`,
`SIGNED MESSAGE`) so a mis-armored tag is refused with a clear message
instead of a mis-dispatch, but only the trailing `SSH SIGNATURE` block ever
passes — `git verify-tag` itself runs with `GNUPGHOME` pointed at a fresh
empty directory (removed after) so a PGP-signed tag can never succeed by
accident. `styrman-update __verify-tag <tag> <allowed_signers> <repo>` is a
test-only internal verb exercising that same `verify_tag` function directly
(`internal/config/styrman_update_test.go`); it refuses to run at all unless
`STYRMAN_UPDATE_TEST=1` is set, which only the Go test sets. Runbook
(prerequisites, signing, rollback): `../.agents/docs/specs/styrman/deploy.md`
→ Auto-update.

At the end of every real tick the updater writes its outcome (`result`,
`code`, `checkedAt`, `since`, `deployed`, `latest`) as one JSON line to
`/etc/styrman/status/updater.json` (ADR-081; `styrman-update.service` sets
`STYRMAN_UPDATE_STATUS_FILE`, the script has no default). The compose file
mounts `/etc/styrman/status` read-only into the server, the only part of
`/etc/styrman` it sees, and Settings shows admins a notice when the updater
is failing, stale or unreadable. Codes and mechanics:
`../.agents/docs/specs/styrman/versioning.md` and `deploy.md`.

## GCP

`gcp/` — a disposable GCP Compute Engine target, same host path as above
(one Debian 13 VM with nested virtualization, `host-setup.sh` →
`styrman-ctl install`, unchanged): `vm.sh` (create/status/ssh/stop/start/
delete over IAP, no public port 22) and `install.sh` (clones the repo at a
release tag, verifies its signature, stages secrets, runs `styrman-ctl
install` on the VM). Full procedure, defaults and prerequisites:
`../.agents/docs/deploy/gcp.md` (ADR-075).

The agent harness (`styrman-harness`) no longer has a host channel or
updater verb (ADR-073 deleted, superseded by ADR-074): the `sh-agent`
release binary is downloaded and verified straight into the runtime image
at build time (`image/Dockerfile`), and a project pulls a newer release
itself with `sh-agent update`. The updater's `retire_harness_units` step
runs on every tick to remove any leftover ADR-073 units and state on an
upgrading host. Full mechanics: `../.agents/docs/specs/styrman/deploy.md`
→ Auto-update → "Agent harness in the image (ADR-074)".
