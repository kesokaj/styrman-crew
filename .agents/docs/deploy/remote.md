---
owner: documentation
date: 2026-09-09
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

**Host firewall half (ADR-036/067, once per host):** `sudo bash deploy/firewall/install.sh` — run from the deploy clone (`/opt/styrman`) at the deployed tag, never from a working tree. Installs the isolation script and its timer unit only. Since ADR-067 the per-project LAN switch is not a firewall rule at all — see "Per-project network mode" below — so this script has one job: keep every `sdb-*` (sandbox) bridge isolated from the host, the LAN, other projects, cloud metadata and the tailnet, while leaving the internet and the same-bridge broker/port-preview proxy reachable. On a host upgrading from v1.23/v1.24, `install.sh` also REMOVES the ADR-065/066 machinery it left behind — the `.path` unit, the spool tmpfs mount and its tmpfiles entry, the run dir, the service drop-in — because a leftover `.path` unit pointing at a deleted spool fails on every boot; `/etc/styrman/firewall.conf` is left in place untouched (its keys are no longer read by anything, and this script does not edit an operator-owned file). From then on the auto-updater's `host_converge` re-applies the script and unit from every verified release tag.

### Post-upgrade cleanup for hosts crossing the ADR-067 release

This is cleanup, not a requirement: a host that skips it keeps working — LAN
mode is purely server-side since ADR-067, so nothing is blocked by leaving
old units in place. The auto-updater deliberately does not do this step: it
only ever installs the files the verified tree carries, and removing a unit
the tree no longer has is `deploy/firewall/install.sh`'s job, not
`deploy/update/styrman-update`'s (see that script around line 506). So a host
that only auto-updates is left with two enabled-but-inert systemd units from
the deleted ADR-065/066 machinery: `styrman-firewall.path` (watching a spool
directory nothing writes to any more) and the escaped spool tmpfs mount unit
(`run-styrman\x2dfirewall-spool.mount`). Neither ever triggers, but they are
still enabled clutter.

Run once, after the first ADR-067-or-later deploy, to remove them:

```bash
sudo bash deploy/firewall/install.sh   # from /opt/styrman at the deployed tag
```

This disables and deletes the `.path` unit, the spool mount unit, its
tmpfiles entry, the service drop-in, and the (by-then-empty) run dir. It
leaves `/etc/styrman/firewall.conf` in place on purpose: **nothing reads that
file any more** (the `LAN_*` keys it may still carry are inert), and
`install.sh` does not edit an operator-owned file — an operator who
rediscovers it later should not be misled into thinking it still configures
anything.

### Post-upgrade steps for the next tag (TODO close-out 2026-09-10)

Run both once, after deploying the next tag:

```bash
sudo bash deploy/firewall/install.sh    # from /opt/styrman at the deployed tag
sudo bash deploy/host-setup.sh --user $USER
```

`install.sh` now also ENABLES `styrman-firewall.service` at boot (`WantedBy=
multi-user.target`), not just the timer: the base rules land once right
after `docker.service` instead of waiting for the timer's first tick, and
that first tick itself now fires at `OnBootSec=5s` (down from 15s) as the
belt to that brace — closing the boot-ordering window ADR-067's follow-up
(f) measured. Re-running `install.sh` on an already-converged host is a
no-op beyond that.

`host-setup.sh`'s `daemon.json` merge adds `default-address-pools:
[{"base":"172.16.0.0/12","size":24}]` when absent, so a 16th-or-later
project bridge draws from a range the firewall's drop list actually names
instead of colliding with `192.168.0.0/16` (the LAN those drops exist to
keep sandbox projects out of). This takes effect for NEW Docker networks
only — an already-created project's bridge keeps its existing subnet — and
needs a Docker daemon restart to apply at all: `host-setup.sh` detects the
`daemon.json` change and runs `systemctl restart docker` itself.
`live-restore` (already on) keeps every running project microVM up across
that restart, so nobody's session is interrupted; only project creations
AFTER the restart draw from the new pool.

**Pre-ADR-027 workspaces (removed with the next tag):** the automatic
migration of a legacy workspace
(`homes/<user>/base/projects/<dir>` → `projects/<user>/<uuid>`) is deleted
from the server (plan `.agents/docs/plans/2026-09-10-todo-closeout.md`,
item F). The only known deployment migrated at v1.19.x, where the server
has logged a benign WARN at every start for the missing path ever since. A
host that still holds a tree under `homes/<user>/base/projects/` must move
it by hand into `/var/lib/styrman/projects/<user>/<uuid>` (the UUID is the
project's id in `users/<user>/projects.json`) BEFORE upgrading; otherwise
the project boots with an empty workspace while the old tree stays on disk
untouched.

### Per-project network mode (ADR-067)

A project's network mode is not a firewall rule the host applies — it is which Docker bridge INTERFACE name the server chose when it created the project's network: `sdb-<hash>` (sandbox, the default) or `lan-<hash>` (LAN), same sha1 suffix of the same network name either way. The `sdb-*` jumps in `styrman-firewall.sh` match the sandbox prefix; a `lan-*` bridge is matched by no styrman rule at all, so it is exactly a normal Docker container on this host: it NATs out as the host and reaches whatever the host reaches (the local network, the host's own addresses including `sshd`, and host-published Docker ports). It gains no reach into other projects — that guarantee is Docker's own, unconditional per-bridge `DROP` rule, not something this script grants or withholds.

There is no allowlist, no conf opt-in and no root-side per-project state to install or verify here. Toggling a project's mode (Settings → the project, admin only) rebuilds that project's Docker network and container — the home and workspace are bind mounts and survive, anything living only in the container filesystem does not. The credential broker no longer withholds the SSH key from LAN projects; a LAN project is served credentials like any other.

### Advice, not a precondition: harden sshd before using LAN mode

Unlike the deleted ADR-065/066 design, nothing in the code gates LAN mode on this. But the LAN switch means the whole LAN, sshd included, and `%sudo NOPASSWD:ALL` means a successful password guess against this host's `sshd` from inside a LAN project is root. **Hardening sshd is still strongly advised before enabling LAN mode on any project** — do it, but nothing will stop you if you skip it:

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

Once `passwordauthentication no` is reported, LAN mode on any project no
longer risks a password-guessed root login through this host's `sshd`. As of
2026-09-08 this host has **not** been hardened:
`passwordauthentication yes`, no `authorized_keys` for either account.

**Operator decision, 2026-09-10: password login stays.** LAN mode is used
for specific projects only, and the operator accepts the exposure above for
them. The procedure stays here for whoever changes their mind; nothing
re-raises it.

## Procedure

Release procedure, the three-image lineup, and the exact commands: `.agents/docs/specs/styrman/versioning.md` "Release procedure". In short: `deploy/verify-commit.sh <commit>` (builds and tests a detached worktree of that exact commit, ADR-061), tag, then on the host

```bash
STYRMAN_VERSION=<x.y.z> docker compose -f deploy/docker-compose.yml up -d --build
```

The tag-gated auto-updater (`deploy/update/`, ADR-050/051) ships disabled; enabling it is a separate operator action with its own runbook in `deploy/README.md`. Once enabled it also converges the host firewall files (`deploy/firewall/*`) on every tick — the host no longer drifts behind the repo the way it did between 2026-09-02 and 2026-09-08.

## Verify

Health through the ingress and a real Google sign-in in a browser (the one step an agent cannot do); `versioning.md` "Release procedure" lists the checks. Firewall half:

```bash
sudo iptables -S STYRMAN-FWD | head          # six DROPs; no per-project RETURN — the mode is the bridge name now
systemctl status styrman-firewall.timer
sudo iptables -S STYRMAN-IN                   # just DROP, unconditionally
ip -o link show | grep -E ' (sdb|lan)-'      # each project's live mode
journalctl -u styrman-firewall -n 20         # one line per change; quiet runs log nothing
```

## Rollback

`versioning.md` "Rollback" and "If the deploy fails". There is no backup layer: durability is git plus `host-setup.sh` (operator decision 2026-09-01, `deploy.md` "Durability").
