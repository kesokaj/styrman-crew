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

Full walkthrough: `../.agents/docs/specs/styrman/deploy.md` (including the
credential broker's bridge-only listener). There is no backup layer —
durability is git: commit and push, and rebuild a lost host from this
script + the repo.

`update/` — the tag-gated auto-updater (ADR-050/051): `styrman-update` +
`.service`/`.timer` units, installed by `host-setup.sh` step 8c but shipped
**DISABLED**. Enabling it is a deliberate, separate operator action — it
requires a dedicated clone at `/opt/styrman`, its own `deploy/.env`, and a
root-owned `/etc/styrman/allowed_signers` for SSH-signed releases before the
timer is turned on. Runbook (prerequisites, signing, rollback):
`../.agents/docs/specs/styrman/deploy.md` → Auto-update.
