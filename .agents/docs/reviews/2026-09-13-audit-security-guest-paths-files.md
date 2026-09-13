---
owner: security
date: 2026-09-13
---
# Security audit — guest-writable paths & file access (2026-09-13)

Range reviewed: working tree at commit `e56e775708a9062cc6432f5c3278770bed63b685` (clean).
Every citation below is `path:line` as of that commit.

Surface: `internal/files` (os.Root confinement, `Trust`, `ReadRawNoFollow`),
`internal/orchestrator` mount sources (`projectHomeIsReal`/`realBelow`/`mkdirAllConfined`),
`internal/history` transcripts, `internal/httpapi` explorer / git / check / format,
`cmd/styrman` migration + transcript janitor, credential sweeps.
Judged against `AGENTS.md`, `REQUIREMENTS.md` R1–R13, `.agents/rules/guest-writable-paths.md`,
and BUG-002/003/004/009 in `.agents/docs/bugs/INDEX.md`.

## Verdict: FINDINGS (1 medium, 3 low — no cross-tenant escape found)

The tenancy boundary itself holds. I found **no path below a mounted home that is
opened by name**: every home- and workspace-relative operation goes through
`files.openRoot` (`internal/files/files.go:108`), the janitor and the ADR-064 base-home
migration walk through `os.Root` handles (`cmd/styrman/main.go:336`, `:400`), and every
Docker bind source is re-proved non-symlink on both the create and the start path
(`internal/orchestrator/orchestrator.go:341`, `:389`). The worst thing on the record is
a missing regular-file guard in `history.Read` — the one transcript reader of three that
never got the 2026-09-08 audit's fix, so a guest-planted FIFO at
`~/.styrman/logs/<project>/<id>.log` parks the request goroutine of whoever opens that
transcript, on a process shared by every tenant.

## Findings

- **[medium / confidence: high]** `internal/history/history.go:230` — `Read` opens the
  transcript with `O_RDONLY|O_NONBLOCK` and then `f.Stat()`s it (`:235`) **without ever
  checking `st.Mode().IsRegular()`**. Its sibling `searchOne` does exactly that check,
  twice, with the comment naming the hazard ("a guest-planted FIFO blocked in open(2)
  and the check never ran (audit 2026-09-08)", `:152-167`), and `internal/files/openReg`
  (`internal/files/files.go:167`) refuses non-regular files for the same reason. The
  transcript directory is inside the project home, which is bind-mounted into the microVM
  where untrusted agent code runs as root, so its contents are attacker-chosen. `O_NONBLOCK`
  only keeps `open(2)` from blocking; the subsequent `io.ReadAll` on a FIFO with no writer
  parks the goroutine in the poller indefinitely, and `history.Read` takes no `context`
  (`:216`) so neither the handler's 
  request cancellation (`internal/httpapi/explorer.go:414`) nor a client disconnect frees it.
  `cmd/styrman/main.go:139` sets only `ReadHeaderTimeout`, no `WriteTimeout`, so nothing
  reaps the stuck handler: each `GET /api/projects/{id}/history/{id}` against a planted
  FIFO leaks one goroutine plus one fd on the shared server. A device node planted the
  same way is read as a file (bounded by `maxBytes`, but still not what the endpoint means).
  *Fix:* mirror `searchOne` — `rt.Lstat` the relative path first and return unless
  `IsRegular()`, and re-assert `IsRegular()` on the opened fd; pass `r.Context()` down so a
  disconnect ends the read. A unit test that plants a FIFO under a `files.Trust` base and
  asserts `Read` returns promptly (the shape of `internal/files/files_test.go:215`) keeps it.

- **[low / confidence: high]** `internal/files/files.go:180` — `ContainedPath` has **no
  production caller** (only `internal/files/files_test.go:147`); it is dead code on the
  security-critical path. Worse, it is a trap for the next caller: it validates
  `filepath.EvalSymlinks(full)` but returns the **unresolved** `full` (`:198`), so the
  path handed back is re-resolved by the OS at use time — precisely the check-then-use
  window BUG-003 was about, in a helper whose doc comment invites `git -C <path>` use.
  The interactive git client now runs in-guest (`internal/httpapi/git.go:171`), which is
  why the last caller went away. *Fix:* delete the function and its test; if a path-taking
  helper is ever needed again, return the resolved path and re-prove it with
  `realBelow`-style component checks at the call site.

- **[low / confidence: medium]** `internal/httpapi/explorer.go:102` — `managedHomeTarget`
  refuses destructive explorer operations on `projects`, `.dockerd`, `.styrman`, `.ssh`
  by matching the **first component of the requested relative path**, but `files.*`
  deliberately follows symlinks that stay inside the root
  (`internal/files/files.go:242`, `Root` semantics). A guest that plants
  `~/x -> ~/.dockerd` inside its own project home turns
  `PUT /api/projects/{id}/home/file?path=x/<uuid>.img` back into the O_TRUNC of a
  running microVM's loop-mounted docker disk that the guard was added to prevent —
  the guard's own comment at `:188` names that outcome. It is not a tenancy escape
  (the link and the target are both inside the caller's own home, and the guest could
  corrupt its own disk from inside anyway), so the loss is integrity of platform-managed
  state, self-inflicted but reachable through a normal-looking UI action. *Fix:* decide
  the guard on what the path **resolves to** rather than how it is spelled — e.g. after
  `openRoot`, `Root.Lstat`/`Root.Stat` the first component and refuse when it is a symlink,
  or keep the managed subtrees out of reach by construction. Cheapest honest option:
  refuse a symlinked first component in the home scope.

- **[low / confidence: high]** `internal/httpapi/format.go:84` vs `:120` — the comment
  above the exec says "Temp file in the workspace (WorkDir=ws) so formatters discover the
  project's own config", while `formatScript` writes to `$HOME/.styrman/tmp` and its own
  comment says "Temp file outside the workspace, as in checkScript". One of the two is
  wrong about where the formatter runs, and the stated *reason* for the design (config
  discovery) no longer holds for the code as written: `prettier`/`gofmt` are invoked on a
  file outside the repo, so repo-local config is found only via `WorkDir`, not via the
  file's own directory. No security impact — the script runs in the guest, as the guest
  user, on guest-owned files — but a reader trusting the first comment will reason about
  the wrong path. *Fix:* delete the stale half of the comment and state the actual
  contract (temp file under `~/.styrman/tmp`, cwd = workspace) in one place.

## Cleared on inspection

- **Root opening (BUG-002).** `files.openRoot` (`internal/files/files.go:108`) reaches
  every root below a `Trust` base through `os.OpenRoot(<base>/<tenant>)` +
  `Root.OpenRoot(rest)`; `httpapi.New` registers both bases before any route exists
  (`internal/httpapi/server.go:205`), and the untrusted-root fallback now warns once per
  root (`:145-151`) instead of failing silently.
- **Mount sources (BUG-003).** `ensureRunningLocked` proves the home real on the start
  path (`internal/orchestrator/orchestrator.go:341`) *and* re-proves it after creating it
  (`:389`), creates it through `mkdirAllConfined` (`:382`), and pre-creates the workspace
  mount target through a root at the verified home (`:402`). `realBelow`
  (`:1559`) walks every component with `Lstat`. `projectHomeIsReal` treats only
  `fs.ErrNotExist` as benign (`:1581`).
- **Listing through a sub-root (BUG-004).** `files.List` uses `rt.Lstat`, not
  `DirEntry.Info()` (`internal/files/files.go:233`); `history.List` the same
  (`internal/history/history.go:81`); `pruneTranscripts` stats with `fs.Stat(home.FS(), rel)`
  (`cmd/styrman/main.go:352`).
- **Janitor and migrations.** `pruneTranscripts` opens the tenant dir — server-only since
  ADR-064 — and reaches `p/<uuid>` only via `tenant.OpenRoot` (`cmd/styrman/main.go:336-345`);
  `migrateBaseHomes`/`migrateTenantIntoBase` operate entirely inside an `os.Root` at the
  tenant dir (`:400`, `:429`); `migrateLegacy` refuses to rename onto a non-empty tenant
  (`:201-203`).
- **Credential sweeps.** `sweepProjectCredentials` and `sweepMaterializedFile`
  (`internal/httpapi/credentials.go:226`, `:256`) delete only through `files.Remove`
  (os.Root-confined); the `filepath.Glob` at `:262` enumerates `homes/<u>/p/*`, a
  server-only directory.
- **`ReadRawNoFollow`.** Opens the parent through the root and `openat`s the final name
  with `O_NOFOLLOW|O_NONBLOCK|O_CLOEXEC` (`internal/files/files.go:471`), asserts
  `IsRegular`, and its only caller validates the content shape as a public key
  (`internal/httpapi/setup.go:247`). The hard-link/copy limitation is documented, not
  claimed away.
- **FIFO/device hardening elsewhere.** `openReg`, `openRegWrite`, `List`, `ListNames`,
  `Save`, `write` all carry `O_NONBLOCK` and (where they open a file) an `IsRegular`
  check (`internal/files/files.go:157`, `:316`, `:213`, `:502`, `:536`, `:371`).
- **Write-side guards.** `Create` uses `O_EXCL` so the kernel settles the race
  (`internal/files/files.go:371`); `Rename` refuses to clobber and documents its
  non-atomicity (`:555`).
- **Root-assisted wipe (BUG-009 family).** `RootRemoveContents` refuses paths outside
  `StateDir`/`HomesDir` (`internal/orchestrator/orchestrator.go:1281-1305`) and uses
  `find <dir> -mindepth 1 -delete`, which is physical (`-P`) and unlinks symlinks rather
  than descending them; the path itself is server-computed, never client-supplied. A
  partial wipe keeps the record so secrets are never reported gone while on disk
  (`internal/httpapi/server.go:1040`).
- **Host-side git.** Only `init`/`rev-parse` remain host-side, with exec-capable config
  keys neutralised and host global/system config ignored (`internal/gitrepo/gitrepo.go:31-55`);
  the interactive client runs in-guest with `GIT_CEILING_DIRECTORIES` set to the workspace
  parent (`internal/httpapi/git.go:186`) and `--` before every client-supplied path
  (`:300`, `:310`, `:331`, `:334`, `:547`, `:693`).
- **Guest exec templates.** `safeSuffix` reduces the extension to `[A-Za-z0-9]{1,16}`
  before it reaches `mktemp --suffix=` (`internal/httpapi/check.go:59`) and `format.go:88`
  routes through the same helper; checker commands come from a closed switch
  (`check.go:83-126`). Content travels on stdin with an exact byte count, never argv
  (`format.go:121-127`).
- **Project `Dir`.** Frozen from a validated name at creation and re-validated on load
  (`internal/project/store.go:106`, `:156`), so `projects/<dir>` cannot carry `..`.
- **Authorization on the file routes.** `scopeFor` derives the root from the session
  identity and the project record, never from a client-supplied path
  (`internal/httpapi/explorer.go:34-51`); the unauthenticated/unauthorized-identity
  escapes in `authGuard` are limited to `/link/`, `/proxy/` and `/shared/` port previews
  (`internal/httpapi/auth.go:37`, `:73`; `internal/httpapi/shares.go:101`,
  `internal/httpapi/links_api.go:274`), none of which reach a file endpoint.
- **Secrets.** No credential material in the reviewed code or in the paths it logs; the
  sweeps log `rel` and `home` paths, not contents.

## Not examined

- `internal/terminal`, `internal/conversation` (tmux session/transcript *writer* side,
  including the in-guest `pipe-pane` command construction) — only the reader side is in
  this dispatch's surface.
- `internal/httpapi/browser.go`, `proxy.go`, `links_api.go`, `shares.go` beyond the
  auth-path classification above — covered by `2026-09-13-audit-review-httpapi-browser-proxy-creds.md`.
- `internal/credbroker` (bridge-only listener) and `internal/credstore` beyond confirming
  their paths live under `StateDir`, never below a mounted home.
- `image/` (entrypoint, `styrman-setup`, launcher) and `deploy/` (firewall, compose,
  `styrman-ctl`) — separate audit slices this cycle.
- `deploy/caddy/ingress` (separate Go module).
- Dependency scanning: `govulncheck` was **not** run for this slice — it needs the pinned
  builder image (`bash deploy/vulncheck.sh`, a Docker build) and this dispatch is
  read-only source review; no new dependency appears in the reviewed files
  (`golang.org/x/sys/unix` in `internal/files` predates this range and is justified in
  `files.go:469`). `npm audit` is not applicable to this surface (no `web/` code reviewed).
- No test was executed and no exploit was written; every claim above is a reading of the
  cited lines plus documented Go/POSIX open semantics.
