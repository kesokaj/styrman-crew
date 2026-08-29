# Git side panel becomes a real git client; auto-checkpoints removed (v1.7.0)

Date: 2026-08-29. Operator direction: drop the automatic git checkpoints
(backups will come from microVM volume snapshots instead) and make the Git side
panel work like a real git UI — status, stage/add, commit, push, diffs, what's
uncommitted.

## Removed: automatic checkpoints

- The periodic 10-min checkpoint loop (`RunCheckpointLoop`) and the event-driven
  snapshots (on project stop, terminal pause/close, all-terminals-closed) are
  gone, along with the now-unused `Server.snapshot` helper. Files:
  `cmd/styrman/main.go`, `internal/httpapi/server.go`,
  `internal/httpapi/checkpoints.go`. The manual checkpoint list/create/restore
  endpoints remain in the backend for now but are no longer used by the UI.

Backups are now expected to come from **microVM volume snapshots** of
`/var/lib/styrman` (operator's chosen story) — tracked in TODO, since it's host
infra (LVM/ZFS/restic — the host FS must support it).

## New: git side panel (real client)

Backend (`internal/httpapi/git.go`) — every git command runs INSIDE the guest
VM, where the user's git identity and the broker credential helper live, so
commits carry the right author and push authenticates. Direct argv (no shell),
so client paths can't be interpreted as shell:

- `GET  /git/status` — branch, upstream, ahead/behind, and every changed path
  parsed from `git status --porcelain=v2` (staged / unstaged / untracked / renames).
- `GET  /git/diff?path=&staged=&untracked=` — per-file diff (untracked shown via
  `--no-index` against /dev/null).
- `POST /git/stage` · `/git/unstage` — `git add` / `git reset` (a path or all).
- `POST /git/discard` — `git checkout` (or `clean` for untracked); confirmed in UI.
- `POST /git/commit` — commits staged changes. The author is injected from the
  user's SHARED-home `.gitconfig` via `-c user.name/user.email` (per-project
  homes carry no identity — the broker syncs the auth token, not the identity).
- `POST /git/push` — pushes the branch, sets upstream on first push; clean 409
  for "no commits yet".

Frontend (`web/src/components/SidePanel.tsx` GitTab rewrite + `web/src/lib/api.ts`):
branch bar with Push, a commit message box + Commit, "Staged" and "Changes"
groups with per-file stage/unstage/discard, and a per-file diff viewer with
colored +/- lines. `usePolled(..., [project])` so it tracks project switches.

## Known gap (TODO)

Git commits IN A PROJECT TERMINAL still fail with no identity — the broker
syncs the git token but not `user.name`/`user.email`, and per-project homes
start without them. The git UI works around this by injecting the identity
host-side, but `git commit` typed in a terminal needs the identity propagated
too (e.g. pull-on-login setting it, or GIT_AUTHOR_* env — same class as the
v1.5.6 claude onboarding flag).

## Verification (live, styrman-server 1.7.0)

Against the live "test" project: status listed the untracked files; stage → the
file showed `A` (staged); unstage reverted it; untracked diff rendered; push
returned a clean 409 ("no commits yet"). Commit with `-c` identity injection
validated in an isolated repo — author came through as
`kesokaj <ostling.simon@gmail.com>`. `go test -race`, `go vet`, `tsc -b` green.

## Release

Built + deployed live as styrman-server 1.7.0.
