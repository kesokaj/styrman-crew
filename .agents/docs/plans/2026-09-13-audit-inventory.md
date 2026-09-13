---
owner: explorer
date: 2026-09-13
---
# Recon — audit inventory (2026-09-13)

Repo pinned at commit `e56e775` (clean tree).

## Question
Inventory and structure map of the whole project for an `/audit`: module map,
uniformity (patterns existing in two shapes), orphaned/dead-code candidates,
committed artifacts that shouldn't be — judged against AGENTS.md,
REQUIREMENTS.md, `.agents/docs/changes/` (three newest), specs, and
`.agents/docs/bugs/INDEX.md`.

## Summary
The codebase is well-factored and mostly consistent with its own conventions
(single `writeErr` helper, uniform shell preambles, a deliberate one-poll
dedup pattern in `web/src/lib/projects.ts`). Two real findings stand out:
`internal/files.ContainedPath` (`internal/files/files.go:180`) is an exported,
security-labeled symlink-escape guard for exec-based host tools that nothing
in the shipped binary calls (`deadcode` confirms it's unreachable from
`cmd/styrman`) — its own doc comment claims it exists for "exec-based tools
like git," but `AGENTS.md` says the interactive git client runs in-guest, so
either the guard is stale or a caller is missing. `web/src/components/FileExplorer.tsx`
hand-rolls its own 10s `setInterval` poll with a comment acknowledging it
duplicates the hidden-tab guarantee that `usePolled` (`web/src/lib/hooks.ts`)
and the v1.14.0 poll-dedup (`web/src/lib/projects.ts`) already solved once —
a second, un-deduplicated polling shape survives the refactor that was
supposed to remove exactly this. `web/.claude/agent-memory/` (2 tracked
files) is the same class of stray committed artifact `writing-files.md`
names as a past accident (BUG-006-adjacent), still present. No orphaned
TypeScript exports, no dead entry-point files, and no doc-drift: every
backticked path in README.md/AGENTS.md/specs/deploy docs resolves to a real
file once checked relative to each doc's own directory.

## Relevant files
- `internal/files/files.go` — confined-root file access; `ContainedPath`
  (line 180) is the one exported symbol with no caller outside its own test.
- `web/src/lib/hooks.ts` (`usePolled`, ~line 60-95) and
  `web/src/lib/projects.ts` (`useProjects`, app-wide poll dedup, v1.14.0) —
  the two "correct" polling shapes.
- `web/src/components/FileExplorer.tsx:178-183` — the divergent third shape.
- `web/.claude/agent-memory/{documentation/MEMORY.md,documentation/project_styrman_docs_structure.md}`
  — stray tracked files, same family as the BUG-006 incident.
- `.agents/docs/specs/styrman/*.md`, `.agents/docs/deploy/*.md`, `README.md`,
  `AGENTS.md` — checked for path drift; all backticked file paths resolve.

## Module map

Go (`internal/`, one line each; size = non-test LOC; importers from a
same-repo grep, excluding tests):
- `config` (533 LOC) — env config, paths, operator-tunable settings. Imported by nearly every package (`orchestrator`, `httpapi`, `project`, `terminal`, `conversation`, `cmd/styrman`).
- `identity` (61) — email → username slug, request identity. Imported by `httpapi`, `cmd/styrman`.
- `userstore` (223) — allowlist + admin flags. Imported by `httpapi`, `cmd/styrman`.
- `project` (340, split `store.go`+`uuid.go`) — per-user project store (UUID identity). Imported by `httpapi`, `orchestrator`, `cmd/styrman`.
- `orchestrator` (2244 across `orchestrator.go`, `ports.go`, `exec.go`, `docker.go`, `browserpeer.go`) — Kata microVM lifecycle, ports, legacy cleanup, network isolation (`realBelow`/`projectHomeIsReal`). Imported by `httpapi`, `cmd/styrman`. Largest single-file: `orchestrator.go` at 1647 LOC.
- `terminal` (554, `bridge.go`+`autostart.go`) — exec↔WebSocket terminal bridge. Imported by `httpapi`, `cmd/styrman`.
- `conversation` (533, `conversation.go`+`store.go`+`names.go`) — terminal records + tmux session manager. Imported by `httpapi`, `terminal`, `cmd/styrman`.
- `files` (629) — confined-root file access (`os.Root`-based). Imported by `httpapi`, `cmd/styrman` (pruning), `orchestrator` (transcript janitor pattern per `guest-writable-paths.md`).
- `history` (255) — transcript full-text search. Imported by `httpapi`.
- `gitrepo` (85) — hardened bootstrap-only host-side git (`init`/`rev-parse`). Imported by `httpapi` (`provision.go`, `server.go`), `cmd/styrman`.
- `credstore` (472) — credential broker store. Imported by `credbroker`, `httpapi`.
- `credbroker` (163) — bridge-only HTTP listener (`:8091`) for credential materialization (ADR-030/032). Imported by `cmd/styrman`.
- `links` (366) — public/private preview-link store. Imported by `httpapi`.
- `httpapi` (6928, largest package: `server.go` 1374, `git.go` 1001, `browser.go` 621, `explorer.go` 686) — all REST/WS routing, auth guard, settings, proxy. Imported only by `cmd/styrman`.

`cmd/styrman/` — `main.go` (entry + legacy-layout migration), plus
`migrate_test.go`/`prune_test.go`.

`web/src/` (60 non-test `.ts`/`.tsx` files):
- `routes/` — TanStack Start route tree (pages).
- `components/` (23 files) — `TerminalPane`, `BrowserPane`, `FileExplorer`, `SidePanel*` (Git/Env/Clips/History/Ports), dialogs (`NewProjectDialog`, `DeleteProjectDialog`), `ui.tsx` shared primitives.
- `lib/` (16 non-test modules) — `api.ts` (HTTP client), `terminal.ts`, `cdp.ts`/`browserInput.ts` (browser-pane CDP bridge), `projects.ts` (poll dedup), `hooks.ts` (`usePolled` + shared hooks), `lan.ts`, `sshKey.ts`, `toast.ts`, `clips.ts`, `fonts.ts`, `appearance.ts` (referenced by tests), `explorerHistory.ts`, `recentsPanel.ts`, `projectBadge.ts`, `editorPane.ts`, `diffReveal` (test-only, logic likely inline — see Open questions).
- `dist/` — committed build output, embedded via `go:embed` per AGENTS.md; committed **on purpose**, not an artifact finding.
- `.claude/agent-memory/` — 2 tracked files, stray (see Risks).
- `e2e/` — Playwright self-hosted e2e harness (`run.sh`, `run.mjs`).

`image/` — runtime microVM image: `Dockerfile`, `entrypoint.sh`, `configs/*.sh`
(installed under renamed `/etc/profile.d/` names at build time — verified,
not doc-drift), `harness/` (vendored agent-harness tree, out of scope for
this app's own dead-code check).

`deploy/` — `host-setup.sh`, `firewall/` (install.sh, styrman-firewall.sh,
test.sh), `verify-commit.sh`, `vulncheck.sh`, `docker-compose.yml`, vendored
Caddy ingress (separate Go module, its own `go test`).

## Flows / conventions confirmed
- Error responses: single `writeErr(w, status, err)` helper in
  `internal/httpapi/server.go:1314`, used uniformly; `git.go` has one
  git-specific wrapper (`gitCtxErr`) layered on top, not a competing shape.
- Shell script preambles: uniform `#!/usr/bin/env bash` + header comment
  across `deploy/*.sh`, `image/entrypoint.sh`, `image/configs/harness-migrate.sh`,
  `web/e2e/run.sh`, `.agents/docs/testing/styrman/*.sh`. The two
  `image/configs/*.sh` files without a shebang (`profile-motd.sh`,
  `styrman-credentials.sh`) are *sourced*, not executed, by design (`AGENTS.md`
  / `runtime-image.md` describe them as login-shell snippets) — not an
  inconsistency.
- Polling: `web/src/lib/projects.ts` documents a deliberate v1.14.0
  "poll de-amplification" — one shared 5s tick, pauses when `document.hidden`,
  every subscriber shares it. `usePolled` in `hooks.ts` is the generic
  per-hook version of the same pattern (interval + visibilitychange resume).
  `FileExplorer.tsx:178-183` does NOT use either; it opens its own
  `setInterval(..., 10000)` with a comment admitting it re-implements (and
  loses, until manually patched) the same hidden-tab guarantee.

## Risks
- `ContainedPath` (`internal/files/files.go:180`): exported, documented as
  the escape-guard for "exec-based tools like git," zero callers in shipped
  code (confirmed via `deadcode ./cmd/styrman/` → `unreachable func:
  ContainedPath`), only exercised by its own test
  (`internal/files/files_test.go:147-168`). Two readings: (a) genuinely dead
  — `internal/gitrepo` runs `git` directly via `exec.Command` without this
  guard (`internal/gitrepo/gitrepo.go:40` `git()` takes `ws string` raw), so
  if `ws` is ever attacker-influenced this guard's absence is a real gap,
  not just dead code; or (b) intentionally future-provisioned and merely
  unwired. Either reading is worth a maintainer decision, not a silent
  deletion — the `guest-writable-paths.md` rule specifically warns about
  paths that look server-controlled but pass through a mounted home.
- `web/.claude/agent-memory/` tracked in git — same shape as the BUG-006
  incident the project's own rules describe ("a directory looking like junk
  is not evidence... committed by an earlier accident of the same kind").
  Low severity (agent scratch notes, not secrets) but it is the exact
  pattern the rule warns is easy to miss.
- `FileExplorer.tsx`'s standalone poll adds guest-exec-API load the v1.14.0
  fix was written to eliminate; low severity (bounded to one panel, 10s
  interval) but it is a recorded regression pattern (duplicated mechanism
  the codebase already solved once) rather than a new judgment call.

## Open questions
- Whether `ContainedPath` was left over from a since-in-guest-moved git
  client (i.e., truly dead) or is meant to guard a currently-unguarded
  `exec.Command("git", ...)` call somewhere the grep missed — could not
  determine intent from the code alone; needs a maintainer call.
- `diffReveal.test.ts` exists in `web/src/lib/` with no corresponding
  `diffReveal.ts` — logic may be inlined in `DiffView.tsx`/`DiffPane.tsx`;
  not flagged as a finding (test still passes against real code, just not
  a same-named module) but worth a second look if the audit wants full
  test-to-source mapping.
