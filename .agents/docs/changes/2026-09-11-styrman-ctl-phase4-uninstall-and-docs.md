---
owner: main
date: 2026-09-11
---
# styrman-ctl Phase 4: `uninstall` and documentation     (date: 2026-09-11)

## Summary

Phase 4 of 4 completed: the `uninstall` command with three typed-confirmation levels and full documentation. Level 1 (`stop` word) stops the stack and removes styrman-managed containers and networks, disables the four units. Level 2 (`uninstall` word) adds removal of firewall chains (IPv4 and IPv6), unit files, binaries, tmpfiles entry, `/etc/styrman`, `/opt/styrman`, and `styrman-*` images; `deploy/.env` backed up to `.env.removed-<timestamp>`. Level 3 (hostname confirmation) adds `du -sh` then removal of `/var/lib/styrman`. Every recursive delete is guarded by `guard_path` to prevent accidental deletion outside configured trees. Two new unit tests (`TestStyrmanCtlUninstallPlans`, `TestStyrmanCtlUninstallConfirmations`) verify the per-level presence/absence of commands and refusal of invalid paths. Documentation updated across `deploy/README.md`, `.agents/docs/specs/styrman/deploy.md`, and pointers added to `deploy/{local,remote}.md`. Dry-run on production host verified the complete plan and changed nothing.

## Type

feature, Phase 4 of 4 — uninstall and documentation

## Files changed

- `deploy/styrman-ctl` (enhanced, +150 lines): `uninstall [--level 1|2|3] [--confirm WORD] [--dry-run]` prints the level's plan (subshell run under `DRY=1`), prompts for typed confirmation (`stop` / `uninstall` / hostname; `--confirm WORD` for unattended), executes through `run`; level 1: compose down, remove `styrman.managed=true` containers (project and setup VMs), `sdb-*`/`lan-*` networks, disable four units; level 2 adds IPv4/IPv6 firewall chain teardown, unit files removal, binaries, tmpfiles entry, `/etc/styrman`, `/opt/styrman`, `styrman-*` images, backup `deploy/.env`; level 3 adds `du -sh` then `/var/lib/styrman`; all deletes through `rm_tree` → `guard_path` (canonical, configured dir, real directory, no symlink).
- `internal/config/styrman_ctl_test.go` (2 new tests, 13 total): `TestStyrmanCtlUninstallPlans` asserts per level what commands are present/absent (compose/docker rm/docker network rm/systemctl disable, chains, files, images, state — all inside scratch prefix, never a foreign network or image), `TestStyrmanCtlUninstallConfirmations` asserts right word passes, wrong word aborts, level-2 word for level-3 fails, level-4 fails, no level without tty fails, symlinked `/etc` refused before any plan is printed.
- `deploy/README.md` (new quick start): `sudo deploy/styrman-ctl <verb>` with verb table and link to full docs.
- `.agents/docs/specs/styrman/deploy.md` (new section): `styrman-ctl` mapping each verb to underlying commands and what it deliberately skips.
- `.agents/docs/deploy/remote.md` and `local.md` (pointers to the full guide).

## Agents dispatched

verifier (Phase 3, PASS, edc4858 clean worktree, commit on production read-only), documentation (deploy guide), changelog (this entry). Reviewer and security over Phase 3, and security over Phase 2, to follow.

## Verification

- Shellcheck: clean (SC2030/2031 disabled on the deliberate plan-printing subshell)
- Go vet: `go vet ./...` passes
- Unit tests: `go test -race ./internal/config/` green (13 tests, 2 new for Phase 4; Phase 1 "uninstall is a stub" assertion removed)
- Real, dry-run on production host: `sudo deploy/styrman-ctl uninstall --level 3 --dry-run` printed full plan (compose down from v1.27.1, docker rm one stopped project VM, docker network rm its sdb network, four units disabled, IPv4/IPv6 chain removal, unit file cleanup, binary removal, tmpfiles removal, `/etc/styrman` and `/opt/styrman` removal, ~100 `styrman-*` images removed, 174M `/var/lib/styrman` du output then removal) and displayed "word to type for level 3 is: dave"; afterwards `docker ps`, `systemctl is-enabled styrman-firewall.timer` unchanged (nothing executed)
- Not verified: any real `uninstall` at any level (production host not to be altered), a fresh-host `install`, level-1-then-install round trip (operator's step when wanted)

## Requirements

n/a

## Related

- Plan: `.agents/docs/plans/2026-09-11-installer-ctl.md` (Phase 4 of 4 ticked; whole feature complete)
- Verifier over Phase 3: `.agents/docs/testing/2026-09-11-verifier-styrman-ctl-phase3.md` (PASS, clean worktree edc4858)
- Phase 4 verification: `.agents/docs/testing/2026-09-11-verify-styrman-ctl-phase4.md` (PASS, plan printing and no-change confirmed)
