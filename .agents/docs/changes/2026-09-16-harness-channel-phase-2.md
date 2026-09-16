---
owner: main
date: 2026-09-16
---
# Harness channel Phase 2: the server mounts the channel (2026-09-16)

## Summary

Phase 2 of `.agents/docs/plans/2026-09-16-harness-channel.md` (ADR-073): the server reads `STYRMAN_HARNESS_DIR`, and every project container mounts `<dir>/current/repo` at `/styrman/harness` and `<dir>/current/bin/sh-agent` at `/styrman/bin/sh-agent`, both read-only, when the host's channel is present and trusted. Absent or refused, the image's baked mirror seeds the project as before (R5's fallback). A stopped container is rebuilt on its next start to gain the binds once the channel has delivered, or to drop them once it is absent. The guest still boots the image's copy until Phase 3 teaches the entrypoint the channel's stamp and source line.

## Type

Feature (phase checkpoint)

## Changed

- `internal/config/config.go` — `Config.HarnessDir` from `STYRMAN_HARNESS_DIR` (default empty = disabled).
- `internal/orchestrator/harness.go` — new: `harnessBinds(dir)` (nil, nil when disabled; nil and an error wrapping `fs.ErrNotExist` when nothing is at `current`; nil and another error when untrusted), `harnessTrusted` (the content dir not a symlink, owned by root or the server, no group/world write; `current` a symlink resolving inside the dir to a directory under the same rules holding `repo/COMMIT` and `bin/sh-agent` as regular files), `ownedAndPrivate`, `hasHarnessBind`, `harnessChannelState`, `logHarnessChannel` (one INFO or WARN per server start naming tag, commit and path, or absent/refused/disabled).
- `internal/orchestrator/orchestrator.go` — `createSpec.harnessBinds`, appended to `Binds` in `create`; `ensureRunningLocked` computes the binds once, logs a WARN for a refused channel, and gains a third stopped-only recreate condition beside the workspace-bind and image-upgrade ones (present-and-trusted without the bind → recreate with it; absent with the bind → recreate without it; refused → start as is); the project create spec carries the binds, the setup VM's never does; `New` logs the channel state and tolerates a nil logger (the httpapi tests pass one).
- `deploy/docker-compose.yml` — server env `STYRMAN_HARNESS_DIR=/var/lib/styrman/harness` with the comment (same path inside as on the host through the existing `/var/lib/styrman` volume).
- `internal/config/harnessdir_test.go` — new: `TestHarnessDirFromEnv`.
- `internal/orchestrator/harness_test.go` — new: the binds on the unresolved `current` path; absent is `fs.ErrNotExist`; disabled is nil, nil; seven untrusted shapes refused (world-writable export, world-writable content dir, `current` escaping the dir, `current` a real directory, no `sh-agent`, no `COMMIT`, the content dir a symlink); the channel state; the three recreate outcomes and the running-container case against the fake Docker client; the setup VM's create carries no binds. Red before the code (7 FAIL on behaviour, no compile error), green after.
- Docs (main agent): `.agents/docs/specs/styrman/deploy.md` — the Phase-1 scope note replaced by the "Server side" paragraph of the harness-channel subsection; `.agents/docs/specs/styrman/api.md` — `STYRMAN_HARNESS_DIR` row in the server env table.
- `.agents/docs/plans/2026-09-16-harness-channel.md` — Phase 2 boxes and Checkpoint ticked with the evidence; D23 (empty `STYRMAN_HARNESS_DIR` is a third state, disabled: no binds, no recreate, no WARN).

## Verification

`verifier` agent, `.agents/docs/testing/2026-09-16-verify-harness-channel-phase-2.md`: PASS — `go build`/`go vet`/`gofmt` clean; the config and orchestrator harness tests all PASS (8 top-level, 7 subtests); `go test -race ./... -count=1` all ok; the ingress module ok; compose renders the new env line; by reading: the recreate rule is stopped-only, the create passes the same binds, the setup VM's create passes none; positive control: the mechanism is absent from `518c5c0`. The compose line has no test tier beyond `docker compose config`.

Fix on the way (no bug record: a first-attempt test correction): `t.TempDir()` is 0775 under this host's umask and the server rightly refuses a group-writable content dir; the test helper now creates the dir 0755 as `host-setup.sh` does. One design fix caught by the whole-module run before the verifier: `orchestrator.New` is called with a nil logger by the httpapi tests, so the new start-up log line panicked there; `New` now substitutes a discard logger.

## Agents dispatched

`verifier`. Review and security rows are owed once over the whole feature at Done (Phase 4).
