# deploy/ — production deployment

`host-setup.sh` converges a fresh Debian 13 host (Docker, Kata `mvm`,
tuning, state dirs, runtime image, `.env` scaffold) — idempotent, R11.
`docker-compose.yml` runs the vendored Caddy ingress (Google OAuth, TLS,
Cloudflare DNS — `caddy/`) in front of the containerized server
(`Dockerfile.server`).

```bash
sudo bash host-setup.sh --user $USER
vi .env   # secrets: OAuth client, Cloudflare token, allowed emails
STYRMAN_VERSION=<x.y.z> docker compose up -d --build
```

`styrman-ctl` — one front door over the scripts above (plan
`../.agents/docs/plans/2026-09-11-installer-ctl.md`): `sudo deploy/styrman-ctl
status [--json]` reports what is installed, deployed and running (exit 0 ok,
1 degraded, 2 nothing installed); `sudo deploy/styrman-ctl install` runs
`host-setup.sh`, asks for the six must-fill `.env` keys (validated), builds
the runtime image and brings the stack up at the checkout's release tag (or
`--version`), then offers to set up auto-update; `sudo deploy/styrman-ctl
update --setup` does that setup on its own (token, clone, `.env` copy,
`allowed_signers`, timer) — the 8c prose above as prompts; `sudo
deploy/styrman-ctl update` runs one updater tick now (`--redeploy` forces
the current tag and clears a latched failure); `sudo deploy/styrman-ctl edit
[KEY=VALUE …]` edits `.env` (both copies, validated) and recreates the stack
at the deployed version. `uninstall` arrives with the plan's last phase. `--dry-run`
prints every command a verb would run and runs nothing; `--yes` with
`--env-file`, `--gh-token-file` and `--allowed-signers-file` runs unattended.

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
timer is turned on. Runbook (prerequisites, signing, rollback):
`../.agents/docs/specs/styrman/deploy.md` → Auto-update.
