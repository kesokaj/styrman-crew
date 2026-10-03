---
owner: bugs
date: 2026-10-02
id: BUG-084
status: fixed
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
Fixed 2026-10-03, run by the agent as root on the operator's word ("Why can't you do 2 and 4 you have my permission", then the operator's choice "Use my gh login token now").
- The rejected token (94 bytes, written 2026-09-02, the length of a fine-grained token) was moved aside to `/etc/styrman/gh-token.rejected-2026-10-03` (root 0600), not deleted.
- The operator's `gh` CLI login token was installed as `/etc/styrman/gh-token`, root:root 0600, through a pipe; it never appeared on a command line or in a log.
- Known deviation (operator's choice): the docs ask for a fine-grained, read-only token. This is the operator's OAuth token with `repo` (write), `admin:public_key`, `gist` and `read:org` scopes. It stops working if the operator logs `gh` out or the token is refreshed. Replacing it with a fine-grained read-only token is the operator's follow-up.
- Verified: `styrman-update --check` printed "v1.41.0 signature OK (allowed signer)" and "update AVAILABLE: v1.38.0 → v1.41.0"; `sudo deploy/styrman-ctl update` printed "OK — v1.41.0 is live" and "updater self-installed from the verified tree"; `styrman-ctl status` verdict OK, deployed v1.41.0; the next tick's `Result=success` wrote `/etc/styrman/status/updater.json` = `{"result":"ok","code":"ok",…,"deployed":"v1.41.0","latest":"v1.41.0"}`, readable inside the server.

## Prevention
After a release tag, read `docker ps` (or `styrman-ctl status`) on EVERY host the tag is meant to reach and name the hosts checked in the release's change entry. A failing `styrman-update.service` is found by `systemctl status styrman-update.service`, never by the absence of complaints.
A fine-grained token expires. Since v1.41.0 an expired or rejected token shows in Settings as `fetch-auth` within one tick, instead of going unnoticed for ten days (ADR-081).

## Related
`.agents/rules/mechanism-reaches-every-caller.md` (BUG-035: verify with the live-host command), BUG-081 (the same release, tagged from a machine that could not run the gate).
