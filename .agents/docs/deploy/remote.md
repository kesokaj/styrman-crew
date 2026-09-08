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

### Opting in to full same-network parity (ADR-066, optional, per host)

Since ADR-066 the per-project LAN switch can also include **the host itself
and host-published Docker ports**. Both are off unless the conf says so, so
converging the script to version 4 grants nothing on its own:

```bash
# from /opt/styrman at the deployed tag; updates ONLY the named keys
sudo bash deploy/firewall/install.sh --lan-allow-host 1
sudo bash deploy/firewall/install.sh --lan-allow-published-ports 1   # derives INGRESS_BRIDGE
cat /run/styrman-firewall/status   # lan_allow_host=1, ingress_bridge=br-<12 hex>
```

`--lan-allow-published-ports 1` derives `INGRESS_BRIDGE` from the
`styrman-ingress` network and refuses to proceed if it cannot; that value is
a safety exclusion, so it is never typed by hand unless `--ingress-bridge` is
passed deliberately. A stale value (the network was recreated) names an
interface that no longer exists and the published-port family is skipped
fail-closed. The server cross-checks it and warns in Settings → Host firewall
if the conf disagrees with the live ingress network.

**A LAN-enabled project stops being served the SSH key** while this is on
(the credential broker withholds it, and any materialized copy is removed
from the project home). SSH push from that project will fail; HTTPS with the
GitHub token still works.

### PRECONDITION for `LAN_ALLOW_HOST=1`: harden sshd first

R12 states it as a requirement, not a suggestion: **a deployment that lets
projects reach the host MUST NOT accept password authentication on the
uplink.** With `LAN_ALLOW_HOST=1` an AI agent running inside a project can
open connections to this machine's `sshd`, and `%sudo NOPASSWD:ALL` means a
successful login is root.

Do this **before** setting the key, and never in one step — a mistake here
locks you out of your own machine:

```bash
# 1. Open a SECOND ssh session to the host and leave it open. Do not close it
#    until step 5 succeeds.

# 2. Install your public key (skip if ~/.ssh/authorized_keys already has it).
ssh-copy-id "$USER@<host>"                  # or, locally:
mkdir -p ~/.ssh && chmod 700 ~/.ssh
cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# 3. Prove key auth works in a THIRD, NEW session before changing anything:
ssh -o PreferredAuthentications=publickey -o PasswordAuthentication=no "$USER@<host>" true && echo "key auth OK"

# 4. Only after step 3 printed OK:
printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\n' \
    | sudo tee /etc/ssh/sshd_config.d/10-styrman-no-password.conf
sudo sshd -t && sudo systemctl reload ssh

# 5. Open a FOURTH session to confirm you can still get in. If you cannot,
#    the session from step 1 is still open: remove the file and reload.
sudo sshd -T | grep -E '^(passwordauthentication|permitrootlogin)'
```

Only when `passwordauthentication no` is reported should
`--lan-allow-host 1` be set. As of 2026-09-08 this host has **not** been
hardened and has **not** opted in: `passwordauthentication yes`, no
`authorized_keys` for either account.

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
sudo iptables -S STYRMAN-IN                   # just DROP, unless LAN_ALLOW_HOST=1 and a project is on
cat /run/styrman-firewall/status             # version=4, applied_at recent, lan_cidrs=<your prefix>,
                                             # lan_allow_host / lan_allow_published_ports / ingress_bridge
journalctl -u styrman-firewall -n 20         # one line per change; quiet runs log nothing
```

## Rollback

`versioning.md` "Rollback" and "If the deploy fails". There is no backup layer: durability is git plus `host-setup.sh` (operator decision 2026-09-01, `deploy.md` "Durability").
