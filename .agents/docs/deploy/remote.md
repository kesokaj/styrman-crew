---
owner: documentation
date: 2026-09-03
target: remote
---
# Deploy — remote machine (production host)

Production is one self-hosted Debian 13 host (baremetal on the operator's network, serving `crew.styrman.xyz`). Access details are the operator's and are not recorded here. This guide points at the procedures that already exist; it does not repeat them.

## Target

| Item | Value |
|---|---|
| Host | operator's Debian 13 box (`deploy/host-setup.sh` converges it, R11) |
| Services | vendored Caddy ingress (Google OAuth, TLS, Cloudflare DNS) + containerized server (`deploy/Dockerfile.server`) via `deploy/docker-compose.yml` |
| Secrets | `deploy/.env` on the host only |

## Prerequisites on the target

`sudo bash deploy/host-setup.sh --user $USER` (Docker, Kata `mvm`, tuning, state dirs, runtime image, `.env` scaffold — idempotent). Bring-up and production mode: `.agents/docs/specs/styrman/deploy.md` "Production mode" and "Bring-up".

## Procedure

Release procedure, the three-image lineup, and the exact commands: `.agents/docs/specs/styrman/versioning.md` "Release procedure". In short: `deploy/verify-commit.sh <commit>` (builds and tests a detached worktree of that exact commit, ADR-061), tag, then on the host

```bash
STYRMAN_VERSION=<x.y.z> docker compose -f deploy/docker-compose.yml up -d --build
```

The tag-gated auto-updater (`deploy/update/`, ADR-050/051) ships disabled; enabling it is a separate operator action with its own runbook in `deploy/README.md`.

## Verify

Health through the ingress and a real Google sign-in in a browser (the one step an agent cannot do); `versioning.md` "Release procedure" lists the checks.

## Rollback

`versioning.md` "Rollback" and "If the deploy fails". There is no backup layer: durability is git plus `host-setup.sh` (operator decision 2026-09-01, `deploy.md` "Durability").
