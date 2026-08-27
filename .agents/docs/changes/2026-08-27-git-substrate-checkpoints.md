# 2026-08-27 — Git as the substrate: checkpoints, clone-on-create, ADR-022/R13

## Summary

Operator directive: every project workspace must always be a git
repository, providing real history/diffs plus a git-native
checkpoint/restore (undo) system (ADR-022, new REQUIREMENTS.md R13,
operator-approved). New `internal/gitrepo` package backs both. Verified:
unit/vet green, integration suite **54/54**, Playwright e2e suite **30/30**,
plus a live end-to-end check against a real public repo clone. Ops
side-effect: 13 empty orphaned UUID-named workspace dirs (pre-ADR-020
leftovers) were removed from the operator's projects dir during
verification.

## Changes

1. **New `internal/gitrepo` package**: the checkpoint/restore engine
   described in ADR-022 — `Init` (`git init -b main`), `Checkpoint` (private
   index file `.git/styrman-index` via `GIT_INDEX_FILE`, whole-tree snapshot
   of tracked+untracked/non-ignored files, `commit-tree` under a neutral
   `styrman` identity, ref `refs/styrman/checkpoints/<ns-stamp>-<sha8>`,
   prune-to-40), `List`, and `Restore` (auto-snapshots current state first,
   then `read-tree --reset` + `checkout-index -f -a` + `clean -fd`). New
   test file covers 6 cases including a restore-undo roundtrip (checkpoint →
   modify → restore → modify again → restore to the pre-restore
   auto-snapshot → original modification reappears).

2. **`createProject` reworked** (`internal/httpapi/...`): project creation
   now always runs `git init` server-side and synchronously, before the
   `201` response, for empty projects (cheap/local, doesn't need the
   async path from ADR-021). Added a `cloneUrl` field on the create request,
   validated by regex before use and passed to the in-guest git invocation
   as an argument **after `--`** (prevents flag-injection via a crafted
   URL like `--upload-pack=...`). The GitHub-specific create flow is
   unchanged. The old in-guest `git init` step for the plain-empty-project
   path was removed as redundant now that the server does it directly.

3. **Checkpoints HTTP routes**: `GET /api/projects/{p}/checkpoints`,
   `POST /api/projects/{p}/checkpoints`, `POST
   /api/projects/{p}/checkpoints/{id}/restore`.

4. **Auto-snapshot hooks**: a checkpoint is now taken automatically on
   project stop, terminal pause, terminal close, and close-all, in addition
   to the manual button and the pre-restore auto-snapshot from
   `gitrepo.Restore` itself.

5. **Boot-time R13 sweep**: `ensureRepos` in `cmd/styrman/main.go` runs at
   startup and `git init`s any existing workspace found without a `.git`
   directory (covers projects adopted before this change).

6. **Frontend**: the side panel's Diff tab is renamed **Git** and now shows
   a checkpoints list with a "Checkpoint now" button and per-entry Restore
   (guarded by a typed-confirm `confirm()` dialog, not a plain OK/Cancel).
   `NewProjectDialog`'s old "No git" option is replaced with "Clone a repo",
   which takes a URL input wired to the new `cloneUrl` field.

7. **Test suites grew**: Playwright e2e 29 → **30** (new Git tab checks +
   manual checkpoint flow; along the way, fixed a strict-mode locator
   ambiguity where the empty-checkpoints-state text itself contained the
   string "Checkpoint now", which collided with the button's own locator).
   Integration suite 49 → **54** (checkpoint create, list, dedup, restore,
   and a bogus-checkpoint-id error case).

## Ops note

While verifying against real container state, found and removed 13 empty,
orphaned UUID-named workspace directories in the operator's projects dir —
leftovers from before ADR-020 introduced UUID-keyed project identity. No
project records referenced them; deletion was directory-only cleanup, not a
data-affecting change.

## Verification

- `go build`, `go vet`, `go test` green, including the new 6-case
  `internal/gitrepo` test file (restore-undo roundtrip included).
- Integration suite: 54/54.
- Playwright e2e suite: 30/30.
- Live checks against a real public repo: cloned `octocat/Hello-World`
  (branch `master`, tracking `origin`) via the new clone-on-create flow;
  took a manual checkpoint and restored on the clone, confirming the user's
  own branch/HEAD was untouched by the restore.

## Known limitation (see TODO.md)

Idle-reaper stops do not yet trigger an auto-snapshot — the reaper doesn't
currently have the project's workspace `Dir` available to call into
`gitrepo.Checkpoint`.

## Review round (applied before commit)

The `reviewer` subagent audited the diff; all findings were fixed:

- **BLOCKING — cross-user workspace escape.** Checkpoint/restore/diff used the
  raw `<home>/projects/<dir>` path; every guest runs as the same uid on the
  shared mount, so a project could symlink its workspace at another user's
  home and drive `checkout-index`/`clean` there. Now resolved through
  `files.Resolve` (the same symlink-containment guard the file explorer uses)
  in a shared `workspacePath` helper; `projectDiff` routed through it too.
  Verified live: a workspace symlinked to `/etc` returns "workspace
  unavailable", reads nothing.
- **BLOCKING — shared private index.** All snapshots wrote one
  `.git/styrman-index`; a killed process left a stale `.lock` that wedged
  checkpoints forever, and concurrent snapshots raced. Now a unique
  `os.CreateTemp` index per operation (auto-removed) plus a per-workspace
  mutex serializing snapshot/restore. New `TestConcurrentSnapshots`.
- Restore no longer touches the user's real index — a throwaway index drives
  `read-tree`/`checkout-index`, and `clean` reads that same index (fixing a
  bug where clean, seeing the empty real index, deleted the just-restored
  files). `clean -ffd` also clears nested clones; new `TestRestoreClearsNestedClone`.
- `git` calls take a `context.Context` (timeouts); the stop-hook snapshot moved
  to after `Stop` returns and off the request path (quiescent tree, no blocking).
- Handler errors: malformed id → 400, absent id → 404 (sentinel `ErrBadID`/
  `ErrNotFound`); host paths no longer leak in API error bodies (logged only).
- Clone URLs with embedded credentials are redacted before logging (`redactURL`,
  tested); client clone-URL regex mirrors the server's.
- Boot-failure + clone now still leaves an empty repo (R13 holds); `ensureRepos`
  logs a genuinely unreadable users dir; `gc --auto` after prune; rune-safe
  label truncation; `IsRepo`/git-dir detection handles `.git`-as-a-file
  (worktrees) via `rev-parse --absolute-git-dir`.
- UI: Git tab refreshes the diff (not just the list) after checkpoint/restore,
  shows a real "not a repo yet" state and checkpoint-list errors.

Verification after fixes: unit/vet/build green (gitrepo + httpapi tests incl.
concurrency, nested-repo, redaction, clone-URL validation), integration 56/56,
e2e 30/30, live symlink-escape refusal + clone/checkpoint/restore checks.
