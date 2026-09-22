---
owner: security
date: 2026-09-22
---
# Security review — files   (2026-09-22)

Range reviewed: the named files as they are in the working tree, read whole — not a diff: internal/files

## Status: COMPLETE

## Verdict: FINDINGS

Both claims the brief asked to verify hold. `openRoot` really does reach a root below
a trusted base with `os.OpenRoot(<base>/<tenant>)` followed by `Root.OpenRoot(rest)`
(files.go:126-135), with no `os.OpenRoot(path)` on a path below a mounted home and no
`os.ReadDir`/`os.Stat` by name anywhere in the package; and no `DirEntry.Info()`
survives in the package or in any caller — `files.List`, `history.List` and
`cmd/styrman`'s prune walker each stat through the root (`rt.Lstat` /
`fs.Stat(root.FS(), rel)`), each with the BUG-004 reason written beside it. The
confinement boundary itself is sound and I found no path escape.

What is not sound is what the confinement lets one tenant *spend*. `files.List` reads
a guest-controlled directory with an uncapped `ReadDir(-1)` and then sorts the result
with a hand-rolled O(n²) insertion sort — measured at 9.8 s of single-core CPU for
50 000 entries on this machine, quadratic above that — on the single Go process that
serves every tenant's terminals. That is the exact lever `ListNames` twelve lines
below caps at 1000 entries, with a comment explaining why; `List` never got the cap.
Second, `ReadRawNoFollow` opens the parent directory with `rt.Open`, the one open in
the package missing the `O_NONBLOCK` every other open carries specifically so a
guest-planted FIFO cannot wedge a request goroutine and its OS thread forever.

## Findings

- **[medium / confidence: high]** `internal/files/files.go:191` and `:232-246` —
  **uncapped listing plus an O(n²) sort on guest-controlled input.** `List` calls
  `d.ReadDir(-1)` on a directory the guest owns (its own project home / workspace,
  where it is root inside the microVM), builds one `Entry` per dirent, does one
  `rt.Lstat` syscall per entry (plus a second `rt.Stat` per symlink), then sorts with
  `sortEntries`, an insertion sort. `(*os.File).ReadDir` returns dirents in directory
  order, not sorted, so the guest chooses the order; `less` puts directories first,
  so an order of files-then-directories forces every directory to bubble the whole
  prefix. Measured on a copy of `sortEntries`/`less` (evidence:
  `.agents/scratch/sortcost/main.go`): n=5 000 → 102 ms, n=20 000 → 1.57 s,
  n=50 000 → 9.83 s — clean quadratic, so ~160 s at 200 000 entries, per request,
  repeatable by one authenticated tenant against a process shared by all of them. The
  memory side is the same lever: `ReadDir(-1)`'s `[]DirEntry` plus a `[]Entry` of
  five strings each, unbounded, in a server whose OOM takes every tenant's terminals
  with it. No caller bounds it — `httpapi.listFiles`
  (`internal/httpapi/explorer.go:54-68`) passes `?path=` straight in and serializes
  whatever comes back. The package already states the principle at :489-491 ("the
  directory is guest-writable and this is polled — an unbounded ReadDir is a
  per-tenant lever on the shared server") and applies it only to `ListNames`.
  Remediation: bound the read the way `history.List` does (`ReadDir` in batches to a
  cap, e.g. 5000, and return a `truncated` flag the UI renders), and replace
  `sortEntries`/`less` with `slices.SortFunc` + `cmp.Or`/`strings.Compare` — stdlib,
  O(n log n), no new dependency, and the hand-rolled sort then disappears.

- **[medium / confidence: medium]** `internal/files/files.go:446` —
  **`ReadRawNoFollow` opens the parent directory without `O_NONBLOCK`.** `rt.Open(dirRel)`
  is `OpenFile(name, O_RDONLY, 0)` (verified in Go 1.27.1's `os/root.go:102-104`),
  while every other open in this package carries `syscall.O_NONBLOCK` with an
  explicit comment: `openReg` :158, `List` :186, `write` :353, `openRegWrite` :299,
  `Save` :518, `ListNames` :484 — and `internal/history` :54-58 repeats the reasoning
  after the same gap was found there in the 2026-09-08 audit. `open(2)` on a FIFO
  without `O_NONBLOCK` blocks until a peer appears, wedging the request goroutine and
  the OS thread it is on; enough of them walks the runtime into its thread limit. The
  caller today is `internal/httpapi/setup.go:247` reading `.ssh/id_ed25519.pub` from
  a project home, and a guest owns that home: `rm -rf ~/.ssh && mkfifo ~/.ssh` makes
  the open hang with no timeout. Confidence is medium on *reach* only — it depends on
  how often a tenant can drive that handler — not on the mechanism, which is certain.
  Remediation: `rt.OpenFile(dirRel, os.O_RDONLY|syscall.O_NONBLOCK|unix.O_DIRECTORY, 0)`,
  matching the rest of the package; `O_DIRECTORY` also moves the not-a-directory case
  to the open instead of the later `openat`.

- **[low / confidence: high]** `internal/httpapi/explorer.go:99-111` (blast radius) —
  **`managedHomeTarget` is bypassable by a guest-planted symlink.** The guard refuses
  delete/rename/upload against `projects`, `.dockerd`, `.styrman`, `.ssh` in the home
  scope by matching the *first string component* of the cleaned relative path, but
  `os.Root` follows symlinks that stay inside the root. Probed against stdlib
  semantics (evidence: `.agents/scratch/rootsem/main.go`): with `x -> .dockerd`
  planted in the root, `Root.RemoveAll("x/disk.img")` returned `nil` and
  `.dockerd/disk.img` was gone. So `DELETE ?path=x/<uuid>.img` destroys the
  loop-mounted docker disk of a possibly-running microVM, and `k -> .ssh` reaches the
  set-up-once-ever R3 identity, each through a name the guest chooses. This is **not**
  a tenancy break — the guest and the API caller are the same principal, and the guest
  can delete those files from inside its own VM regardless — which is why it is low.
  It is recorded because the comment above the function reads like a boundary
  ("refuses destructive operations on platform-managed subtrees") and a future reader
  may lean on it as one. Remediation: either resolve the first component through the
  root before the check (`rt.Lstat(first)`, refuse a symlink there) or amend the
  comment to say the guard is an accident rail, not a boundary.

- **[low / confidence: high]** `internal/files/rmcheck_test.go:14-25` and the package
  test set — **the cross-tenant wipe guard is tested only on the untrusted root
  path.** `TestRemoveAllRefusesSymlinkedProjects`, the test that exists because a
  reviewer found the `DELETE ?wipe=true` cross-tenant destruction, never calls
  `Trust`, so it exercises bare `os.OpenRoot` — not the sub-root shape production
  always has. Across the package only four functions have a below-a-Trust-base case:
  `Exists`/`ListNames` (trust_test.go:14), `List` (trust_test.go:110) and
  `ReadRawNoFollow` (files_test.go:304). `Read`, `ReadRaw`, `Write`, `Create`, `Save`,
  `SaveStream`, `OpenForDownload`, `Remove`, `RemoveAll`, `Rename` and
  `MkdirAll`/`MkdirAllPerm` have none — and the whole write/delete family is what
  BUG-002/003 was about. BUG-004 is the precedent for why this matters: a sub-root
  quirk that silently changed behaviour got through a green suite because no test ran
  below a Trust base. The rule file requires exactly this ("Every unit test of a
  root-reading function needs a case below a `files.Trust` base"). Remediation: add
  one table-driven test that registers a base and re-runs the existing hostile-link
  cases with the root at `<base>/<tenant>/p/<uuid>`, asserting both directions —
  refused for the escaping link, succeeds for the real path — the way
  `TestReadRawNoFollowBelowATrustBase` already does.

- **[low / confidence: high]** `internal/files/files.go:96-138` — **the untrusted-root
  fallback and two silent fail-to-fallback paths.** When no `Trust` base matches,
  `openRoot` warns once per root and falls back to bare `os.OpenRoot(root)`, which
  follows symlinks on the root path itself — the BUG-002 shape. The trade-off is
  knowingly taken and documented at :96-107 (audit 2026-09-08), and the once-per-root
  warning is the mitigation, so this is recorded rather than re-litigated. The
  residual worth naming: `trusted.bases` is process-global mutable state installed as
  a side effect of `httpapi.New` (`internal/httpapi/server.go:206`), so every other
  caller's safety is a property of startup order, and two paths reach the fallback
  with no signal distinct from "no base matches" — `Trust` ignores a `filepath.Abs`
  error (:81-83) and keeps a relative base, and `openRoot` ignores the same error
  (:114) and keeps a relative root; either makes `filepath.Rel` return an error and
  the base be skipped. Remediation: have `Trust` reject a base it cannot make
  absolute — it runs once at startup, where failing loudly is free.

- **[low / confidence: medium]** `internal/files/files.go:269` — `Read` allocates a
  full `MaxReadBytes` (2 MiB) buffer per call regardless of the file's size, before
  `io.ReadFull`. The `/file` GET route is tenant-reachable and unthrottled, so
  concurrency multiplies it (1000 in-flight reads ≈ 2 GB transient) in the same
  shared process the first finding is about. Remediation: size the buffer to
  `min(st.Size()+1, MaxReadBytes)` — `st` is already in hand from `openReg`, and the
  `+1` preserves the `Truncated` computation.

## Checked

- **Secrets** — no credential, token or key material in the package; nothing secret
  reaches a log. `warnUntrustedRoot` logs a root path only, once per distinct root.
  `ReadRawNoFollow`'s doc-comment (:419-429) correctly bounds what `O_NOFOLLOW` does
  and does not guarantee for the `.ssh/*.pub` read it exists for, including that a
  hard link is still read — and `internal/httpapi/setup.go` validates the public-key
  shape on top, as that comment requires.
- **Injection** — no SQL, shell, template or path-concatenation-into-a-command
  anywhere in the package. The only external input is the caller-supplied relative
  path: `cleanRel` (:48-54) normalizes with `path.Clean("/"+strings.TrimSpace(rel))`
  then strips the leading `/`, which collapses `..`, absolute paths, `.` and the
  empty string to a root-relative form before `os.Root` re-validates every component
  itself. `ReadRawNoFollow` additionally rejects a final component of `""`, `.` or
  `..` (:443). The upload path's filename handling
  (`internal/httpapi/explorer.go:294-300`) uses `filepath.Base` on the
  client-supplied part name and re-cleans the join. I found no traversal.
- **Authz/authn** — none in this package by design; the scope root comes from
  `httpapi.scopeFor` (`internal/httpapi/explorer.go:33-51`), derived from the
  authenticated identity plus a project the per-user store must return, never from a
  client-supplied path. Request bodies are capped upstream by `http.MaxBytesReader`
  (`internal/httpapi/server.go:650-663`: 512 MiB for `/upload`, 8 MiB for
  `/file`|`/format`|`/check`), which is what bounds `SaveStream`'s otherwise
  unbounded `io.Copy` — a caller-side bound, correct today, worth keeping in mind if
  `SaveStream` gains a caller.
- **Crypto** — none in the package. Nothing hand-rolled: no primitive, protocol,
  token or session format is implemented or composed here.
- **Dependencies** — one non-stdlib import, `golang.org/x/sys/unix`, for
  `unix.Openat`, justified in the code (`syscall.Openat` is Linux-only, dev machines
  are macOS) and correct. `govulncheck ./...` (v1.7.0 on PATH, Go 1.27.1) ran bare:
  **2 called vulnerabilities**, GO-2026-4887 and GO-2026-4883, both in
  `github.com/docker/docker@v28.5.2+incompatible`, both **`Fixed in: N/A`** — no
  upstream fix exists, and both IDs are already recorded in five files under
  `.agents/docs/testing/`. Neither is in this package's blast radius (they reach
  `internal/orchestrator`'s Docker client), and no action is available beyond
  continuing to track them. `govulncheck ./internal/files/...` alone: no
  vulnerabilities. `go test ./internal/files/` passed (bare, exit 0). `npm audit` not
  run: no JavaScript in the reviewed scope or its Go blast radius.
- **GCP surface** — none: no IAM binding, service account, Secret Manager reference
  or `--allow-unauthenticated` in the package or its callers.

## Not examined

- `internal/httpapi` was read only where the scope required it —
  `explorer.go:1-145` and `250-320`, `server.go:200-210` and `650-672`,
  plus the grep hits in `credentials.go` and `setup.go`. The rest of the file
  explorer and editor handlers (format/check, download, the CSRF guard's full
  construction, `shares.go`) were not read.
- `internal/history/history.go` was read at its three `files.OpenRoot` call sites and
  `List`; `searchOne` and `Read`'s tail were not read in full.
- `cmd/styrman/main.go` was read at `pruneTranscripts` and `migrateBaseHomes`
  (:340-470); the legacy-layout migration above :340 was not.
- `internal/files/files_test.go` and `remove_test.go` were read by function index and
  hostile-setup grep plus the two Trust-base cases in full, not line by line;
  `trust_test.go` and `rmcheck_test.go` were read whole.
- The two Docker CVEs' actual call traces were not inspected (`-show verbose` not
  run); `deploy/vulncheck.sh`'s containerized run, which also covers the vendored
  Caddy ingress module, was not executed — the host `govulncheck` was used instead.
- No dynamic test of either DoS finding against a running styrman server: the two
  measurements are a copied `sortEntries` and a stdlib `os.Root` semantics probe in
  `.agents/scratch/`, deliberately not exploits against the platform.
