---
owner: bugs
date: 2026-10-02
id: BUG-084
status: open
area: deploy — production host styrman-update.service (/etc/styrman/gh-token)
---
# BUG-084: the host's auto-updater has been failing unnoticed; the host never took v1.39.0

## Symptom
Journal, 2026-10-02, every tick:
`remote: Invalid username or token. Password authentication is not supported for Git operations.`
`fatal: Authentication failed for 'https://github.com/kesokaj/styrman-crew.git/'`
`[styrman-update] FATAL: git fetch failed — root cannot reach /opt/styrman's origin. For a private repo: mint a FINE-GRAINED GitHub token (this repo only, Contents: Read-only) and install it at /etc/styrman/gh-token (root:root, chmod 600); the origin must be an https:// URL. Expired token? Fine-grained tokens have an expiry — mint a fresh one.`
`docker ps` on the host shows `styrman-server:1.38.0` and `styrman-caddy:1.38.0`, while `v1.39.0` was tagged on 2026-09-22.

## Repro
`systemctl status styrman-update.service` on the host (`Active: failed (Result: exit-code)`); `sudo journalctl -u styrman-update.service -n 30`.

## Root cause
The token in `/etc/styrman/gh-token` is rejected by GitHub (expired or revoked; which one was not established). The updater says so in its own log, but nothing brings a failing updater to the operator's attention, and v1.39.0's records say "auto-deployed" for the GCP VM only; no record says this host was checked after the tag.

## Fix
Not fixed. The operator installs a fresh token at `/etc/styrman/gh-token`; the updater then takes the newest signed tag on its next tick. The agent ran no root step (`deploy/local.md` has no Operator steps record of a standing permission).

## Prevention
After a release tag, read `docker ps` (or `styrman-ctl status`) on EVERY host the tag is meant to reach and name the hosts checked in the release's change entry. A failing `styrman-update.service` is found by `systemctl status styrman-update.service`, never by the absence of complaints.

## Related
`.agents/rules/mechanism-reaches-every-caller.md` (BUG-035: verify with the live-host command), BUG-081 (the same release, tagged from a machine that could not run the gate).
