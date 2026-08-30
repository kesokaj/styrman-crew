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
credential broker's bridge-only listener and what to back up).

`backup/` — the host-layer restic→GCS disaster-recovery kit: `styrman-host-backup` (run by the `styrman-backup.service`/`.timer` units, daily 03:00 + up to 15m; installed and enabled by `host-setup.sh` once the secrets exist). Runbook: `../.agents/docs/specs/styrman/deploy.md` → Host-layer backups / Restore on a new host.
