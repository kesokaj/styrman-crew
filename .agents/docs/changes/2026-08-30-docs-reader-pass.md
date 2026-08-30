# Fresh-reader docs pass: instructions made executable (post-v1.14.1)

Date: 2026-08-30. Operator: "For the docs and reader and instructions?" —
the audits verified FACTS (docs match code); nobody had read the docs AS
INSTRUCTIONS for a fresh operator. A 3-walk reader workflow (front door /
manual+runbooks / hands-on scripts+env), findings verified, surfaced 7
broken + ~15 confusing + ~14 polish. All fixed.

## Broken (a follower failed or was misled)

- **No restic RESTORE runbook existed** — the one instruction needed in a
  panic. deploy.md now has "Restore on a new host (disaster recovery)":
  numbered, copy-pasteable, including the ownership nuance (restic
  preserves uid/gid — STYRMAN_UID must match the restored trees).
- **Fresh-host quick start failed at step 2**: the docker group added by
  host-setup isn't active in the current shell. README + host-setup's
  printed NEXT steps now say log out/in (or `newgrp docker`) first.
- `STYRMAN_PREVIEW_DOMAIN` (nonexistent) → `STYRMAN_PUBLIC_DOMAIN`.
- guide.md claimed host backups "not yet built" (§1) — they're live.
- host-setup's backup instructions told the operator to write files into
  `/etc/styrman` which nothing created — host-setup now `install -d`s it.
- README Status said v1.14.0 with a v1.13.1 deploy example — now v1.14.1
  with `$(git describe --tags --always)` everywhere a stamp is needed.
- **Release tags had stopped at v1.8.9** while compose documents tag-based
  rollback — v1.9.0…v1.14.1 backfilled as annotated tags (so `git
  describe` and the rollback instructions actually work).

## Decision folded in

`StateDir/backups` is now INCLUDED in the restic snapshot: a deleted
project's in-app archives are the ONLY copy of that project, and excluding
them broke the "backups outlive their project" promise across host loss.
Only the reproducible docker disks stay excluded. Script updated in repo +
on host; a backup ran clean with the new set.

## Confusing/polish (selected)

Quick start now states the :latest-vs-:dev image split (host-setup builds
production's :latest; the reader's :dev build is dev-only); guide §3 lists
the dev-path prereqs (Go 1.25+, Node 22+) and §7's stale roadmap now points
at ADR-045; deploy.md's reinstall wipe list gained users.json / links.json
/ settings.json and its final command is fully qualified; provisioning
steps are ordered with copy-pasteable restic env blocks; secrets how-tos
precede the commands that need them; README self-heal cites fixed
(ADR-036, not ADR-029); testing README tier list/counts coherent;
.env.example documents the five optional ingress knobs and drops jargon;
AGENTS.md build command carries the version arg; REQUIREMENTS gained two
CLARIFYING parentheticals (R9-vs-R11 scope, sdb-uuid network vs sdb-hash
interface — no requirement changed).
