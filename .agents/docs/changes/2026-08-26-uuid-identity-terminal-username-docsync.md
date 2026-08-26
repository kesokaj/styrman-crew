# 2026-08-26 — UUID project identity, terminal username, async create, full doc sync

## Summary

Operator batch: (1) projects identified by UUID with friendly, renamable,
reusable names — "this concept all over"; (2) the in-guest terminal username
became a per-user setting. Chasing an e2e flake also produced a real
architectural fix (ADR-021). Then a full documentation sync pass. Verified:
unit green, integration **49/49**, browser e2e **29/29** (twice, for flake
confidence), production redeployed with automatic migration.

## UUID identity (ADR-020)

`Project{ID, Name, Dir}`: containers `styrman-<uuid>`, networks
`sdb-<uuid>`, docker disks and transcripts keyed by UUID; API paths and UI
routes use the UUID; names are labels — renamable (new ⋯ → Rename +
`PATCH /api/projects/{id}`), unique only among current projects (the
workspace dir is name-based, frozen at creation), reusable after deletion.
Stores migrate name-keyed records on load; previous-scheme containers are
swept at boot via the Docker SDK (`CleanupLegacy` — the containerized
server has no docker CLI, which the first CLI-based attempt missed).

## Terminal username (per-user profile)

`users/<u>/profile.json` holds `guestUser`; Settings → Account → "Terminal
username" edits it (validated `^[a-z][a-z0-9_-]{0,31}$`). Applies to
microVMs started after the change; the home path on disk stays keyed by the
identity slug.

## The flake that became ADR-021

Browser createProject POSTs failed intermittently with connection-refused
while curl always worked. Root cause: `EnsureSelfAttached` — joining the
server's own container to the just-created project network — reconfigures
the server's netns and kills in-flight connections, sometimes including the
triggering request. Fixes: async creation (201 `state:"creating"`, boot in
background; also better UX and an old TODO), self-attach only in background
paths, and a one-time 307 when the preview proxy must attach on demand.
Verified stable across repeated e2e runs.

## Failures / gotchas (honest log)

- CLI-based legacy sweep silently no-oped in the container (no docker CLI)
  → rewritten on the SDK.
- Synchronous CleanupLegacy force-removing a running Kata VM blocked the
  listener past the health window → made async.
- A `curl -fsS` returning 409 tripped `set -e` and silently truncated the
  integration suite (42 "all passing" out of 48 defined — caught by
  counting defined vs executed checks). Lesson: assert expected non-2xx
  with `curl -s -w`.
- Formatter/patch races on TS files struck again (NewProjectDialog kept the
  name-based call); caught by e2e.

## Documentation sync (operator request: "on par")

README, AGENTS.md, REQUIREMENTS (R6/R7 wording), guide.md and api.md
rewritten to current reality; terminal-protocol/runtime-image/deploy specs
patched; testing README updated to 49/29 suites; TODO pruned+extended;
ADR-020/021 added; the two phase-plan documents marked HISTORICAL with a
pointer to the living guide.
