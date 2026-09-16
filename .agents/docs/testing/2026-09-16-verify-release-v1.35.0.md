---
owner: main
date: 2026-09-16
---
# v1.35.0 verified live (2026-09-16)

Range reviewed: v1.34.1..08d8fa9 (the tagged commit; the release commit is 6e2b0d8, the tag's commit adds the mirror's regenerated `SHA256SUMS`, BUG-065)

Every command below was run bare by the main agent on the host `dave` (the production host), its status read.

## Before the tag

- `deploy/verify-commit.sh 6e2b0d8` → rc 1: `VENDORED.md: FAILED` at the mirror-integrity step (BUG-065). Fixed in 08d8fa9; `deploy/verify-commit.sh 08d8fa9` → rc 0, "OK: commit 08d8fa914785 builds and tests clean in isolation — safe to tag" (its first attempt was killed by the session's memory watchdog during `go test -race`; the re-run, alone, completed).
- `git -c gpg.format=ssh -c user.signingkey=~/.ssh/id_ed25519.pub tag -s v1.35.0 -m "styrman v1.35.0" 08d8fa9`, then `git -c gpg.format=ssh -c gpg.ssh.allowedSignersFile=<scratch copy of /etc/styrman/allowed_signers> tag -v v1.35.0` → `Good "git" signature for ostling.simon@gmail.com with ED25519 key SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU`. Pushed 2026-09-16T21:18:56Z (`refs/tags/v1.35.0` = 5dc1fbd7).

## The deploy tick (styrman-update.service, journal)

`v1.35.0 signature OK (allowed signer)` → `deploying v1.35.0 as 1.35.0` → both images built (`styrman-caddy:1.35.0`, `styrman-server:1.35.0`, the runtime image's harness-builder stage from `image/harness/`) → `self-install targets: /usr/local/bin/styrman-update, /etc/systemd/system/styrman-update.{service,timer}` (the PREVIOUS updater's list — no harness units, as BUG-064 predicted) → `updater self-installed from the verified tree (takes effect next run)` → `OK — v1.35.0 is live`. No rollback, no latch. `deployed-version` v1.35.0, `deployed-sha` 08d8fa91478522c791a087ea9e78f6fd3b129405; `/opt/styrman` at 08d8fa9.

## The server

`styrman-server:1.35.0` Up (healthy), `styrman-caddy:1.35.0` Up; the public URL answers 302 (the OAuth redirect); the server log at start, 21:20:16Z: `WARN harness channel absent; the image's baked harness seeds projects path=/var/lib/styrman/harness/current`, then `styrman listening … image=styrman-runtime:1.35.0`.

## A throwaway project's first boot (`verify-v1-35-0`, c02a0c7f)

Created through the API from inside the server container; ready after 6 s. Boot log: `settings: /home/ostling-simon/.claude/settings.json created with "attribution": …` and `agent harness deployed (version 1.35.0, styrman-harness 4aa19be, source image; browser MCP registered for claude and agy)`. In the guest: no `/styrman/harness/TAG`, `COMMIT` 4aa19be, no bind at `/styrman/harness` (`/proc/mounts`), the container's `HostConfig.Binds` carry no harness bind; `sh-agent status` as the project user in an interactive login shell: `repo /styrman/harness (plain copy, not its own git checkout; update is refused)`, `27 skills, 7 agents`. The operator's two projects (`f8e77d67` on 1.34.1, `fae465b5` on 1.34.0) were not touched; they upgrade on their next stop/start.

Deleted afterwards — with `?wipe=1` by mistake (the parameter is `wipe=true`): the record and the microVM went (202, the container and network gone within 30 s), the home (141 MB, its `.dockerd` image) and the workspace (124 KB) stayed by design; both removed by hand as their owner (uid 1000) after `grep <uuid> /proc/mounts` showed nothing mounted: `rm -r /var/lib/styrman/homes/ostling-simon/p/<uuid> /var/lib/styrman/projects/ostling-simon/<uuid>`. Not a product defect; the memory note for the next release carries the right parameter.

## The second tick (the new updater's host_converge, BUG-064)

The tick at 23:25 CEST (the first under the v1.35.0 updater): `host-converge: harness-channel units installed from 08d8fa914785 (enabled below once the clone exists)`, then `already up to date`. `/etc/systemd/system/styrman-harness-update.{service,timer}` present (root, 644) and byte-equal to the tree's (`cmp`); `systemctl is-enabled styrman-harness-update.timer` → `disabled`, `is-active` → `inactive` — as designed: no clone at `/opt/styrman-harness` until the operator's setup; `/etc/systemd/system/styrman-update.service` carries `STYRMAN_HARNESS_REPO_DIR` (self-installed by the deploy tick). `sudo /opt/styrman/deploy/styrman-ctl status` (the v1.35.0 ctl) → rc 0, `verdict OK`, with the new line `harness    not set up — styrman-ctl update --setup --harness-token-file <file>   disabled/inactive (timer), static/inactive (service)`; `server healthy styrman-server:1.35.0 (styrman-caddy:1.35.0)`, `deployed v1.35.0 sha 08d8fa9…`, `updater enabled/active (timer) … clone /opt/styrman at 08d8fa9 (v1.35.0)`.

## What this release does NOT yet show

The channel delivering: it needs the operator's fine-grained token (`sudo styrman-ctl update --setup --harness-token-file <file>`) and a signed `vX.Y.Z` tag on styrman-harness. Until then every project seeds from the image's mirror (`source image`), which is what this record shows.
