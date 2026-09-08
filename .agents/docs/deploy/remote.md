---
owner: documentation
date: 2026-09-08
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

**Host firewall half (ADR-065, once per host):** `sudo bash deploy/firewall/install.sh` — run from the deploy clone (`/opt/styrman`) at the deployed tag, never from a working tree. Installs the isolation + per-project-LAN reconciler script, its timer and path units, the bounded spool tmpfs, the marker dir, and — only if absent — `/etc/styrman/firewall.conf` with the LAN prefix discovered from the default route (printed; edit and `sudo systemctl start styrman-firewall.service` to change it). From then on the auto-updater's `host_converge` re-applies the script and units from every verified release tag; `firewall.conf` is never touched automatically. A `styrman-server` container that was already running when the spool tmpfs first appeared keeps seeing the plain directory under it until it is recreated (the next release deploy does that).

## Procedure

Release procedure, the three-image lineup, and the exact commands: `.agents/docs/specs/styrman/versioning.md` "Release procedure". In short: `deploy/verify-commit.sh <commit>` (builds and tests a detached worktree of that exact commit, ADR-061), tag, then on the host

```bash
STYRMAN_VERSION=<x.y.z> docker compose -f deploy/docker-compose.yml up -d --build
```

The tag-gated auto-updater (`deploy/update/`, ADR-050/051) ships disabled; enabling it is a separate operator action with its own runbook in `deploy/README.md`. Once enabled it also converges the host firewall files (`deploy/firewall/*`) on every tick — the host no longer drifts behind the repo the way it did between 2026-09-02 and 2026-09-08.

## Verify

Health through the ingress and a real Google sign-in in a browser (the one step an agent cannot do); `versioning.md` "Release procedure" lists the checks. Firewall half:

```bash
sudo iptables -S STYRMAN-FWD | head          # six DROPs; a LAN-enabled project shows a RETURN at the head
systemctl status styrman-firewall.timer styrman-firewall.path
cat /run/styrman-firewall/status             # version=3, applied_at recent, lan_cidrs=<your prefix>
journalctl -u styrman-firewall -n 20         # one line per change; quiet runs log nothing
```

## Rollback

`versioning.md` "Rollback" and "If the deploy fails". There is no backup layer: durability is git plus `host-setup.sh` (operator decision 2026-09-01, `deploy.md` "Durability").
