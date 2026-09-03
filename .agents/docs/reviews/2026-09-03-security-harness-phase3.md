---
owner: security
date: 2026-09-03
---
# Security review — harness replacement, Phase 3 (settings roster)   (2026-09-03)

Range reviewed: `c1a6eb9..<working tree>`, restricted to
`internal/httpapi/setup.go`, `web/src/routes/settings.tsx`, and the new
`internal/httpapi/setup_roster_test.go`. Changes under `image/` are Phases 1–2
and were excluded on instruction. Blast radius followed outward from the diff:
`internal/files` (`ListNames`/`Exists`/`openRoot`), `internal/config`
(`UserHome`/`ProjectHome`), `internal/project` (store validation, `List`),
`internal/httpapi` (`who`/`space`/`authGuard`/`scopeFor`),
`internal/orchestrator` (what mounts those paths into a guest), and
`image/entrypoint.sh` (who the guest is inside that mount).

Note: `setup.go` and `setup_roster_test.go` changed under me mid-review — the
`strings.Contains(n, ".bak.")` substring filter I first read had already become
the anchored `oldHarnessBackup` regexp by the time I finished. This review
describes the anchored version (`setup.go:170`), which is the current state and
is the correct one.

## Verdict: FINDINGS

`rosterHome` opens `HomesDir/<user>/p/<uuid>` as an `os.Root` confinement root
(`setup.go:196`). `os.OpenRoot` follows symlinks on the *root path itself* —
`internal/files/files.go:51-56` says so in a comment and justifies it with "the
root is server-controlled and guaranteed real". For `ProjectHome` that
justification does not hold: `HomesDir/<user>` is bind-mounted whole into the
user's setup microVM as `/home/<guestUser>` (`orchestrator.go:571`, `:783`)
where the guest has NOPASSWD sudo (`image/entrypoint.sh:55`) and the host uid of
the owner, so the guest can `rm -rf ~/p/<uuid>` and put a symlink there. The
confinement root then resolves wherever the symlink points, and every relative
path under it is confined to *that* directory instead. I verified `os.OpenRoot`
follows a symlinked root, and separately verified that a symlink at any
*relative* component is still correctly refused ("path escapes from parent"),
so the confinement inside the root is intact — it is the choice of root that is
now attacker-influenced. Within this diff the payoff is bounded (roster entry
names plus an existence oracle, gated on the target carrying
`.config/sh-agent/state.json`); the same unsafe root is however already used by
the file explorer for read *and* write (`explorer.go:48`) and by the project
bind mount (`orchestrator.go:253`, `:783`), which is where it becomes a full
cross-tenant compromise. That part is pre-existing, not introduced here; this
diff adds a fifth consumer and repeats the "server-created, confinement holds"
assumption in its own comments.

Everything else in the diff is sound: identity and tenancy at the API layer are
correct, no new dependency, no secrets, no injection, no hand-rolled crypto,
readiness flags unchanged.

## Findings

- **[high / confidence: high]** `internal/httpapi/setup.go:196` (root cause:
  `internal/config/config.go:355-357` + `internal/orchestrator/orchestrator.go:571`)
  — `rosterHome` uses `s.cfg.ProjectHome(user, p.ID)` =
  `HomesDir/<user>/p/<uuid>` as an `os.Root` root. `openRoot`
  (`internal/files/files.go:54`) resolves that path with ordinary symlink
  following before confining anything; its own comment (`files.go:51-53`) states
  the precondition "the root is server-controlled and guaranteed real", which
  `ProjectHome` does not satisfy. `HomesDir/<user>` is the setup VM's home
  (`orchestrator.go:571` → bind at `:783`), the guest there has NOPASSWD sudo
  (`image/entrypoint.sh:55`) and the same host uid that owns the homes tree
  (ADR-006: "uid/gid stays the host operator's; isolation is at the mount +
  microVM level, not uid"), and `p/<uuid>` is a plain directory whenever that
  project is stopped. A user who replaces one of their own `~/p/<uuid>` entries
  with a symlink therefore re-points the root. **In this handler**, the reachable
  effect is limited: `rosterHome` only accepts a target that carries
  `.config/sh-agent/state.json` (`setup.go:197`), so a full roster read requires
  naming another tenant's *project* home (username slugs are email-derived and
  guessable, project UUIDs are not), and what leaks is entry names under five
  fixed relative paths (`.claude/{skills,agents,commands}`,
  `.gemini/config/{skills,agents}`) — user-authored agent/skill/command names,
  not file contents. The residual capability with no UUID needed is a boolean
  existence oracle for `<any host path>/.config/sh-agent/state.json`. **In the
  blast radius** the same root is passed to `files.List/Read/Write/Remove/
  SaveStream` by `explorer.go:48` (`/api/projects/{id}/home/*`, read *and*
  write) and to the Docker bind at `orchestrator.go:253`/`:783` (a planted
  symlink survives `os.MkdirAll`, which succeeds on an existing directory, so
  booting the project would mount the target as `$HOME`) — that is
  read/write access to another tenant's `.claude/.credentials.json`,
  `.ssh/id_ed25519`, and `.config/gcloud/*`, and it is not introduced by this
  diff. There is no `Lstat`/`O_NOFOLLOW` guard anywhere on these paths (grep for
  `Lstat|EvalSymlinks` in `internal/orchestrator` and `internal/httpapi` returns
  nothing). **Resolution:** (a) diff-scoped — before using it as a root, reject a
  `ProjectHome` that is not a real directory (`os.Lstat` + `Mode()&os.ModeSymlink
  == 0`, ideally an `openat(parent, name, O_NOFOLLOW|O_DIRECTORY)`), and do the
  same in `scopeFor` and in the orchestrator before the bind; (b) durable — take
  the per-project homes out of the setup VM's mounted tree (bind the setup VM at
  `HomesDir/<user>/base`, or move project homes to a sibling root the setup VM
  never sees), which restores the "server-controlled root" invariant the whole
  `internal/files` package documents. (b) is the one that actually closes the
  explorer and bind-mount paths as well; (a) alone leaves a TOCTOU window.

- **[medium / confidence: high]** `internal/httpapi/setup.go:262-275` →
  `internal/files/files.go:347` — the roster now reads five directories in a
  *project* home, and `files.ListNames` does an uncapped `ReadDir(-1)` and
  materializes every name; `toolDetails` then serializes all of them into the
  response. The project home is written by the agents themselves (`claude`,
  `agy`, `npm install` lifecycle scripts, anything a prompt injection reaches),
  and `settings.tsx:131` polls this endpoint every 20 s per open settings tab
  with no cache, no per-user limit, and no singleflight. A project that creates a
  large number of entries under `~/.claude/skills` makes the *shared* server
  process allocate all of them and encode them every 20 s — a single tenant can
  degrade the one process that serves every tenant. Before this change the same
  read was rooted at the base home, writable only from the ephemeral setup VM;
  the diff widens the writer set to the always-running project VMs, which is
  where untrusted content actually lives. **Resolution:** cap `listNames` (read
  at most N ≈ 500 entries and set a `truncated` flag), and/or cache the roster
  per user behind a TTL comfortably longer than the 20 s poll.

- **[low / confidence: high]** `internal/httpapi/setup.go:194-199` —
  `rosterHome` walks `sp.projects.List()` and issues an `os.OpenRoot` + `Stat`
  syscall pair per project on every request, worst case (no seeded project) for
  every project the user owns, and there is no per-user project cap anywhere in
  `internal/httpapi`. Linear per-request filesystem work driven by a client-held
  poll on a shared process. Small on its own; it compounds the finding above.
  **Resolution:** memoize the resolved `rosterHome` per user with a short TTL, or
  stop at the first N projects.

- **[low / confidence: medium]** `web/src/routes/settings.tsx:193-199` — the
  "N skills · M subagents" line is computed from a directory the guest can edit,
  so it reports observed content, not an attestation that the shipped harness is
  intact. An operator verifying R5 ("every project gets the identical set") from
  this card would be verifying guest-controlled data. **Resolution:** label it as
  observed, or cross-check the count against the version recorded in
  `.config/sh-agent/state.json` and flag a mismatch.

### Explicitly checked and clean (the questions asked)

1. **Tenancy via the store** — clean. `toolDetails` takes both the username and
   the `userSpace` from one `s.who(r)` (`setup.go:212`); nothing about the caller
   comes from the request body or path. `space()` re-validates the slug against
   `userSlugRe` before joining it into a state path (`server.go:202`). Project
   IDs are UUID-shaped by construction (`project.NewUUID` in `Add`) *and*
   re-validated on every load — `store.go:100` quarantines any record failing
   `uuidRe` (`store.go:132`) into `projects.json.rejected`. So a hand-edited
   `/var/lib/styrman/users/<u>/projects.json` cannot smuggle `../` or an absolute
   path into `filepath.Join`, and cannot name another user's tree. The attacker
   path is the filesystem symlink above, not the store.
2. **`os.Root` confinement** — holds for every relative component, and I verified
   it rather than assuming: a symlinked `skills` leaf (absolute *and* relative
   target) and a symlinked intermediate (`~/.gemini` → another home) are all
   refused with `path escapes from parent`. `files.Exists` (`files.go:462`) and
   `files.ListNames` (`files.go:336`) are both confined the same way. The gap is
   the root path only (finding 1). **What an attacker-controlled entry NAME can
   do once it reaches the payload: nothing.** `de.Name()` cannot contain `/` or
   NUL; `writeJSON` (`server.go:1163`) uses `encoding/json` with default HTML
   escaping; `settings.tsx:196-197` consumes only `.length`; the `skills`/`agents`
   arrays still ship in the payload but every render path is a JSX text child
   (`settings.tsx:319`), which React escapes, and there is no
   `dangerouslySetInnerHTML` on this route (the only one in `web/src` is
   `FileEditor.tsx:394`, unrelated). The residue is size, covered by finding 2.
3. **Resource use** — findings 2 and 3.
4. **Information disclosure in the intended path** — clean. The roster is the
   requesting user's own project home, and per R5 the seeded set is identical for
   every user and every project, so it reveals nothing about the operator or
   another tenant that the user cannot already read by opening a terminal. The
   only disclosure concern is the redirected-root case in finding 1.
5. **Readiness flags** — confirmed unchanged and still rooted at the base home:
   `setup.go:260` (`.claude/.credentials.json`) and `setup.go:272`
   (`.gemini/antigravity-cli/antigravity-oauth-token`) both still use `h =
   s.cfg.UserHome(u.Username)`. Only `Skills`/`Agents`/`Commands` moved to `rh`.

## Checked

- **Secrets** — clean. No literal credential in the diff or the new test; the
  test's `identity.User{Email: "tester@example.com"}` is a fixture. The only
  secret-bearing file in the deploy tree, `deploy/.env`, is gitignored
  (`.gitignore:2`); the tracked `deploy/.env.example` contains placeholders only
  (`GOOGLE_CLIENT_SECRET=`, `STYRMAN_INGRESS_TOKEN=`, `STYRMAN_JWT_KEY=` all
  empty). Nothing new is logged: `toolDetails` writes no log line, and error
  paths return `writeErr` with the store/identity error, not file contents. The
  pre-existing `ssh` card still refuses to echo a symlinked `.pub`
  (`setup.go:238-241`) — unchanged and still correct.
- **Injection** — clean. No SQL, no shell, no template. The two parsing
  boundaries in the diff are (a) directory entry names, which are filtered by
  prefix and by the anchored `oldHarnessBackup` regexp (`setup.go:170`) — Go's
  `regexp` is RE2, linear time, so no ReDoS even though the input is
  attacker-controlled — and (b) the JSON response, encoded by the stdlib. Path
  construction is `filepath.Join` over a validated slug and a validated UUID (see
  Tenancy above), never over request data.
- **Authz/authn** — clean. `GET /api/settings/tools` (`server.go:340`) sits
  behind `authGuard` with no exemption (`auth.go:26-84`): in `header` mode it
  requires the shared `X-Styrman-Ingress` token compared with
  `subtle.ConstantTimeCompare`, a non-empty `X-Caddy-Email`, and
  `s.users.IsAuthorized(email)`; the exempt paths are `/api/health`,
  `/link/{token}`, and the preview/shared routes, none of which reach this
  handler. Nothing in the diff reorders or bypasses a check, and no identifier is
  trusted from the client.
- **Crypto** — nothing hand-rolled and nothing added. No auth, session, hashing,
  or token code in the diff.
- **Dependencies** — no dependency change: `git diff --stat -- go.mod go.sum
  web/package.json web/package-lock.json` is empty; the only new import is
  stdlib `regexp`, which satisfies stdlib-first. `bash deploy/vulncheck.sh`
  (govulncheck v1.7.0 inside the pinned `golang:1.25.13-trixie` builder, both
  modules) → root module and `deploy/caddy/ingress` each report the same two
  called findings, GO-2026-4887 and GO-2026-4883 in `github.com/docker/docker`
  (v28.5.2 and v27.5.1), both "Fixed in: N/A" — the daemon-side, no-fix pair this
  repo accepts per release; script exit 0, no new vulnerability. The host's own
  `govulncheck ./...` reports 31 findings, all stdlib against the host's
  go1.25.0; that is the known host-toolchain noise `deploy/vulncheck.sh:5-7`
  documents and is not what ships. `npm audit` in `web/` → "found 0
  vulnerabilities".
- **GCP surface** — not applicable and untouched. styrman is self-hosted
  (`.agents/docs/deploy/gcp.md` records this); the diff adds no IAM binding, no
  service account, no `--allow-unauthenticated`, no key file, and no
  environment-variable secret. The only gcloud reference in the blast radius is
  the settings card reading `~/.config/gcloud/application_default_credentials.json`
  for a boolean readiness flag, which predates this change and still reads
  existence only, never contents.

---

## Round 2

Range reviewed: `c1a6eb9..<working tree>` (2026-09-03). Re-reviewed files:
`internal/files/files.go` (new `Trust`/`openRoot`), `internal/files/trust_test.go`,
`internal/httpapi/server.go:126`, `internal/httpapi/setup.go`,
`web/src/routes/settings.tsx`. Blast radius extended past round 1 to every
*other* consumer of the same `<homes>/<u>/p/<uuid>` path shape:
`cmd/styrman/main.go` (`pruneTranscripts`), `internal/history`,
`internal/orchestrator`. Verification: `go vet` clean;
`go test ./internal/files/ ./internal/httpapi/ -race` → both `ok`, no race.

### Verdict: FINDINGS

**The new `files.Trust` code is correct — I checked every edge case asked about
and found no defect in it. The problem is the claim around it.** "FIXED for the
whole class" is true for the `files.*` class and not for the class of *callers
that open the same attacker-influenceable path without going through
`internal/files`*, of which there are three, all unchanged. The worst is
`cmd/styrman/main.go:381`: the transcript janitor does a raw
`os.OpenRoot(<homes>/<u>/p/<entry>)` and then `home.Remove()` on every stale
regular file under `.styrman/logs` — as root, every six hours, unattended — and
it enumerates that directory with a raw `os.ReadDir(<homes>/<u>/p)` at
`main.go:370`, so the `p` directory itself (not just one UUID entry) is the
lever, and no UUID has to be guessed. Its own comment argues the case is safe
because "os.Root confines every path component", which is exactly the
incomplete reasoning round 1 disproved: `os.Root` confines the components
*below* the root, never the root path itself. This is pre-existing, not
introduced by Phase 3 — but it is now material, because an operator reading
"whole class fixed" would believe it is closed.

### Round 1 findings — status

- **[high] `rosterHome` / `openRoot` symlinked root — PARTIALLY FIXED.** The
  `files.*` half is genuinely fixed and the implementation is sound. Everything
  the author asserts about it holds and I verified each one rather than
  accepting it:
  - **Prefix matching** is correct, and correct for the right reason:
    `filepath.Rel` normalises before comparing, so `Rel("/x/homes",
    "/x/homes2/u/p")` = `"../homes2/u/p"` and `Rel("/x/homes",
    "/x/homesuffix")` = `"../homesuffix"` — both caught by the
    `".."+Separator` guard. This is not the `strings.HasPrefix` bug it would
    have been.
  - **`rel == "."` (root equals the base)** falls through the loop to plain
    `os.OpenRoot(base)`; **`rel == ".."`** likewise. Both correct — no caller
    passes the bare base, and the base is server-created.
  - **Root equal to `<base>/<tenant>`** (`rest == ""`) returns `tr` itself and
    does *not* close it. Correct; that is `UserHome`, the common case for the
    readiness flags.
  - **`tr.Close()` after `tr.OpenRoot(rest)` does not invalidate the child.**
    Verified empirically: `Root.OpenRoot` returns a root holding its own
    `dirfd`, and a read through the child after the parent is closed succeeds.
    No use-after-close.
  - **Concurrency** is fine. `openRoot` takes `RLock` (shared, so requests do
    not serialise) and holds it across the two `openat`s, which is harmless;
    `Trust` takes `Lock` once at `New`. `-race` over both packages is clean.
  - **The tenant is the right anchor.** `os.OpenRoot(<base>/<tenant>)` is still
    a symlink-following open, so the fix only holds if `<base>/<tenant>` cannot
    be replaced by a guest. It cannot: `HomesDir` and `StateDir/projects` are
    never mounted into any guest — the setup VM mounts `<homes>/<u>` and
    project VMs mount `<homes>/<u>/p/<uuid>`, both *at or below* the tenant.
  - **os.Root semantics** are as described: a relative symlink that stays
    inside the root is followed, an absolute one is refused even when its
    target is inside (`path escapes from parent`). Verified.
  - **No caller is left outside a tenant base that should be inside one.**
    `grep 'files\.[A-Z]'` over `internal/httpapi` and `cmd/styrman` gives
    `explorer.go` (10 call sites), `credentials.go:246,272`, `setup.go` (10),
    and `cmd/styrman/main.go:228`. All roots are `UserHome`, `ProjectHome`, or
    `ProjectWorkspace`; all are under a trusted base.

  **What is not fixed** is carried below as R2-1, R2-2 and R2-3.

- **[medium] uncapped `ReadDir(-1)` — FIXED.** `files.go:411` reads at most
  `maxListNames` = 1000. The `err != nil && len(des) == 0` guard is right for
  both stdlib shapes, which I confirmed: `ReadDir(1000)` over 3 entries returns
  `(3, nil)`, over 0 entries returns `(0, io.EOF)` — so an empty directory
  still yields `nil` (unchanged behaviour) and a short read still yields its
  entries. A roster above 1000 entries is silently truncated with no
  `truncated` flag; cosmetic, not a security gap. Accepted.

- **[low] per-request `OpenRoot`+`Stat` per project — REBUTTAL ACCEPTED.**
  Bounded by the per-user project count against a 100+ scale target, and the
  work per project is syscall-cheap. Note the trusted path now costs three
  syscalls per `files.*` call (tenant open, sub open, parent close) rather than
  one; at this bound that changes nothing. No cache needed.

- **[low] `settings.tsx` "N skills · M subagents" is observed, not attested —
  OPEN, unchanged.** `settings.tsx:191-199` still renders bare counts from a
  guest-writable directory with no "observed" label and no cross-check against
  the version in `.config/sh-agent/state.json`. Unchanged severity (low) and
  confidence (medium). Not a vulnerability; an R5 verification that reads
  guest-controlled data as if it were an attestation.

### New findings

- **[high / confidence: high]** `cmd/styrman/main.go:370` and `:381` —
  `pruneTranscripts` reaches the per-project homes with raw stdlib calls that
  `files.Trust` does not intercept: `os.ReadDir(filepath.Join(cfg.HomesDir,
  u.Name(), "p"))` to enumerate projects, then
  `os.OpenRoot(filepath.Join(cfg.HomesDir, u.Name(), "p", p.Name()))` as the
  confinement root, then `fs.WalkDir(home.FS(), ".styrman/logs", …)` and
  `home.Remove(rel)` for every regular file older than `cfg.TranscriptTTL`.
  Both the `p` directory and its entries sit inside `<homes>/<u>`, which is
  bind-mounted whole into the user's setup microVM at `/home/<guestUser>`
  (`orchestrator.go:571` → `:783`) where the guest holds NOPASSWD sudo
  (`image/entrypoint.sh:55`) and the host uid of the owner. `os.ReadDir` and
  `os.OpenRoot` both resolve symlinks on the path they are given, so a symlink
  planted at either level re-points the janitor's root, and every path
  component *below* the re-pointed root is then confined to the wrong tree.
  Because `~/p` itself can be the link, the enumeration at `:370` supplies the
  entry names — no project UUID has to be guessed, only a username slug, which
  is email-derived. The janitor is enabled by default (`TranscriptTTL` 30d,
  `config.go:146`), runs at startup and every 6h (`main.go:332`), is
  unattended, and the code's own comment states it runs as root in production.
  The reachable effect is deletion of another tenant's `.styrman/logs` tree —
  bounded by the fixed `.styrman/logs` prefix and the TTL, so not arbitrary
  file deletion, but it is cross-tenant destruction performed by a root
  process. The comment at `main.go:373-378` asserts safety on the grounds that
  "os.Root confines every path component"; that is the same incomplete
  invariant round 1 disproved. Pre-existing (v1.14.0), not introduced by Phase
  3, and *not* covered by the Phase 3 fix. **Remediation:** route it through
  `files.openRoot` — export a `files.OpenTenantRoot(root) (*os.Root, error)`
  and have the janitor and `internal/history` use it, so `Trust` covers the
  whole class it claims to. Replace the `os.ReadDir` at `:370` with a listing
  taken *through* the tenant root (`tr.Open("p")` + `ReadDir`), and skip
  entries whose `DirEntry.Type()&fs.ModeSymlink != 0` (`u.IsDir()` is checked
  for users at `:365`; nothing equivalent is checked for projects at `:371`).

- **[high / confidence: high]** `internal/orchestrator/orchestrator.go:253` →
  `:783` — carry-forward from round 1's finding 1, still unfixed and still
  outside the reach of `files.Trust`. `os.MkdirAll(phome, 0o700)` on a path
  that is already a symlink-to-directory returns `nil` and leaves the symlink
  in place — I verified this stdlib behaviour rather than assuming it — and the
  raw `phome` string is then handed to Docker as
  `spec.homePath + ":/home/" + spec.guestUser` in `HostConfig.Binds`, where the
  host kernel resolves it, following symlinks, as root. A symlink planted at
  `~/p/<uuid>` from the setup VM therefore causes the *next boot of that
  project* to mount the symlink's target as `$HOME` inside a microVM where the
  user is root: read and write access to another tenant's
  `.claude/.credentials.json`, `.ssh/id_ed25519`, and `.config/gcloud/*`, or to
  a host path. This is the highest-value half of round 1's finding 1, it is
  what makes the class worth closing, and `files.Trust` does not touch it — the
  `files` package cannot, because the resolution happens in Docker.
  **Remediation:** before the bind, open the project home the same way
  `files.openRoot` now does (tenant root + `Root.OpenRoot(rest)`) and pass
  Docker the resolved-and-verified path, or `openat2(RESOLVE_NO_SYMLINKS)` the
  final component; failing that, `os.Lstat` + refuse `os.ModeSymlink` before
  `MkdirAll` (a TOCTOU narrowing, not a close). The durable fix is still round
  1's option (b): move the per-project homes out of the tree the setup VM
  mounts.

- **[low / confidence: medium]** `internal/history/history.go:39`, `:125`,
  `:207` — `List`, `Search` and `Read` each call `os.OpenRoot(homeDir)`
  directly with `homeDir = s.cfg.ProjectHome(...)` (`explorer.go:356`, `:382`,
  `:414`), so the transcript endpoints are the third consumer outside
  `files.Trust`'s reach. A re-pointed root makes them enumerate, grep and
  return the *contents* of `<target>/.styrman/logs/<projectID>/*.log`. The
  practical reach is small — the target must already carry that exact subpath
  under the requester's own project UUID, which another tenant's home will not
  — so this is a weak primitive today, and `List` swallows errors into an empty
  slice so it is barely even an oracle. Recording it at low because the
  *pattern* is the finding: three files now reason about `os.Root` confinement
  independently, and only one of them knows about `Trust`.

- **[low / confidence: high]** `internal/files/files.go:88-92` — the new
  `filepath.Abs` at the top of `openRoot` turns an empty root from a
  fail-closed error into the process working directory. Verified:
  `os.OpenRoot("")` returns `no such file or directory`, while
  `os.OpenRoot(filepath.Abs(""))` opens the CWD. No caller passes `""` today —
  `rosterHome` returns `""` on no seeded project and `setup.go:175-177` guards
  it with an explicit `if home == "" { return nil }` — but that guard is one
  caller deep in `httpapi`, while the behaviour change is in `files`, and
  `rosterHome` is precisely the function that manufactures the empty string.
  Any future `files.Exists("", …)` or `files.Write("", …)` would silently
  operate on the server's working directory instead of erroring.
  **Remediation:** `if root == "" { return nil, fs.ErrInvalid }` at the top of
  `openRoot`, and drop the caller-side guard as redundant.

- **[low / confidence: high]** `internal/files/files.go:57-83` +
  `internal/httpapi/server.go:126` — the tenancy boundary is now process-global
  append-only state established as a side effect of constructing a `Server`.
  Two consequences. (a) Ordering: `main.go` runs `migrateLegacy` (`:44`) and
  `migrateWorkspaces` (`:66`) *before* `httpapi.New` (`:97`), so the
  `files.ContainedPath` at `main.go:228` executes with `trusted.bases` still
  empty and gets the old plain-`os.OpenRoot` behaviour. Harmless today — its
  root is `UserHome`, the tenant root itself, where `Trust` changes nothing —
  but it is a latent trap: any future `files.*` call added before `New` gets
  the weak path silently, with no error and no log. (b) `Trust` has no reset,
  so in `internal/httpapi` every test that constructs a `Server` permanently
  appends its temp dirs to the global slice, which `openRoot` then scans
  linearly on every call for the rest of the test binary. **Remediation:** call
  `files.Trust` from `config.Load` (or from `main` immediately after it) rather
  than from `New`, so the boundary is established before any filesystem work;
  optionally make a second `Trust` with a different base set a hard error
  rather than an append.

### Checked (round 2)

- **Secrets** — clean, unchanged. No credential in `files.go`, `trust_test.go`,
  `server.go:126`, or the settings diff; the new test uses `t.TempDir()` and
  literal `"x"` file contents. `Trust` logs nothing, and `openRoot` returns
  stdlib path errors that name only server-controlled paths.
- **Injection** — clean. The one new parsing boundary is `filepath.Rel` over
  two server-derived absolute paths; no request data reaches `Trust` or
  `openRoot`. `oldHarnessBackup` (`setup.go:168`) is unchanged RE2, linear time.
- **Authz/authn** — clean and unchanged. `GET /api/settings/tools` still sits
  behind `authGuard` with no exemption; nothing in this round reorders a check
  or trusts a client-supplied identifier. The tenancy boundary *moved into*
  `files.openRoot`, which strengthens `explorer.go` and `credentials.go`
  without changing their handlers.
- **Crypto** — nothing hand-rolled, nothing added, no auth/session/hash/token
  code in this round.
- **Dependencies** — no change: `git diff --stat -- go.mod go.sum
  web/package.json web/package-lock.json` is empty; the only new imports are
  stdlib `slices` and `sync`, which satisfies stdlib-first.
  `bash deploy/vulncheck.sh` (govulncheck inside the pinned
  `golang:1.25.13-trixie` builder) → exit 0; root module and
  `deploy/caddy/ingress` each report the same two called findings,
  GO-2026-4887 / GO-2026-4883 in `github.com/docker/docker`, both "Fixed in:
  N/A" — the accepted no-fix daemon-side pair, identical to round 1, nothing
  new. The script now also scans the new `image/harness` module: "No
  vulnerabilities found." `npm audit` in `web/` → "found 0 vulnerabilities".
- **GCP surface** — not applicable and untouched; styrman is self-hosted. No
  IAM binding, service account, `--allow-unauthenticated`, key file, or
  env-var secret in this round.
- **Test coverage of the fix** — `trust_test.go` is a good test and covers the
  five cases it claims. Gap worth noting, not a finding: nothing asserts
  end-to-end that `httpapi.New` trusts the *right* bases — a planted symlink is
  never driven through the roster or explorer handler, so a future edit to
  `server.go:126` (wrong base, or the call deleted) would not fail any test.

---

## Round 3

Range reviewed: `c1a6eb9..<working tree>` (2026-09-03), final round under the
three-round rule. Re-reviewed: `cmd/styrman/main.go:365-415`
(`pruneTranscripts`), `cmd/styrman/prune_test.go`,
`internal/orchestrator/orchestrator.go:170-300`, `:1229-1249` (`realBelow`),
`internal/orchestrator/realbelow_test.go`, `internal/files/files.go:92-97`.
Blast radius re-followed to the whole `EnsureRunning` lifecycle (create *and*
start), the idle reaper, and Docker's own bind-source resolution.

### Verdict: FINDINGS — not converged

**The janitor fix is complete and correct; the orchestrator fix is not, and
the gap is larger than the residual the code comment and BUG-003 record.**
`realBelow` is called in exactly one place — `EnsureRunning`'s
`errdefs.IsNotFound` branch (`orchestrator.go:265`), i.e. only when the
container is *created*. The `case err == nil` branch (`:217-221`) starts an
existing stopped container with no path check at all, and Docker stores a
bind source **unresolved** and re-resolves it on **every start**. So the
exposure is not "a guest racing `docker create`": it is the ordinary
lifecycle, with no race. Create a project, later replace `~/p/<uuid>` with a
symlink from the setup VM, stop and start the project (Stop button or idle
reaper, both routine), and the next boot mounts the link's target as `$HOME`
in a microVM where the guest is root. BUG-003's `status: fixed` and its
"residual" paragraph are both wrong on this point.

### Round 2 findings — status

- **[high] `cmd/styrman/main.go` transcript janitor — FIXED. Verified, not
  accepted.** Every leg of the question asked holds, checked empirically
  against go1.25.0 on this host:
  - **`fs.ReadDir(tenant.FS(), "p")` does not follow a symlinked `p` out of
    the tenant.** `p -> /abs/outside` → `openat p: path escapes from parent`;
    `p -> ../outside` (relative escape) → the same. A relative symlink whose
    target stays *inside* the tenant root **is** followed — but that is the
    tenant's own base home, which the guest already owns outright, so it
    crosses no boundary: the worst case is the janitor walking
    `<base>/.ssh/.styrman/logs`, which does not exist. Not a finding.
  - **`tenant.OpenRoot(filepath.Join("p", name))` (`main.go:389`)** refuses an
    escaping symlink at the last *and* at an intermediate component
    (`path escapes from parent`).
  - **The walk inside the project home is confined.** `fs.Stat(home.FS(), rel)`
    (`:401`) on `.styrman/logs/x.log -> /etc/passwd` returns
    `statat …: path escapes from parent`, so the entry is skipped, not
    deleted. `fs.WalkDir` does not descend a symlinked directory (`d.IsDir()`
    is false, type `L---------`), so there is no walk amplification.
  - **The `d.Info()` → `fs.Stat` change is required and the stated reason is
    exactly right.** On a root obtained from `Root.OpenRoot`, `d.Info()`
    returns `lstat p/uuid/.styrman/logs/a.log: no such file or directory` —
    the sub-root prefix is repeated. Without this change the janitor would
    have pruned nothing, silently. I reproduced both the failure and the fix.
  - **`fs.Stat` follows symlinks where `Info()` lstats, and that does not
    create a deletion primitive.** `Root.Remove` unlinks the final component
    without following it: removing an in-root relative symlink to a file left
    the target byte-identical. The only new effect is that the janitor now
    also reaps stale *symlinks* a guest planted in its own `.styrman/logs`.
    The stat→`Remove` pair is not atomic, but since `Remove` never follows,
    the worst outcome of losing that race stays inside the tenant's own
    `.styrman/logs`. Informational, not a finding.
  - **Tenant enumeration** is safe: `os.ReadDir(cfg.HomesDir)` + `u.IsDir()`
    (`:366`) — a symlink at `<homes>/<user>` reports `IsDir()==false` and is
    skipped; `HomesDir` is in any case never mounted into a guest.
  - **Descriptor hygiene:** `tenant.Close()` on both exits (`:385`, `:414`),
    `home.Close()` per project (`:410`); the sub-root holds its own dirfd, so
    there is no use-after-close and no leak.
  - `prune_test.go` is a real regression test: it asserts both directions (the
    genuine home is pruned, the symlinked one is untouched).

- **[high] `internal/orchestrator/orchestrator.go` bind source — PARTIALLY
  FIXED.** The predicate is right; the call site is incomplete. Carried below
  as R3-1.
  - **`realBelow` itself is correct.** `filepath.Rel` normalises before
    comparing, so a sibling-prefix base (`…/homes` vs `…/homes2`) yields
    `../homes2/…` and is refused — this is not the `strings.HasPrefix` bug it
    could have been. `rel == "."` and `rel == ".."` are both refused. The
    per-component `os.Lstat` walk outward from `base` is the right shape and
    fails closed on a missing component. `base` itself is unchecked, which is
    correct: see the setup-VM answer below.
  - **The setup VM's own bind (`orchestrator.go:582`,
    `homePath: o.cfg.UserHome(user)`) does not need the check — server control
    of the parent suffices, and I checked why rather than assuming.**
    `HomesDir` is never mounted into any guest: the setup VM receives
    `<homes>/<u>`, project VMs receive `<homes>/<u>/p/<uuid>`, both *at or
    below* `<u>`, so no guest can see, rename, or replace `<homes>/<u>`
    itself. The same argument clears `workspacePath`
    (`StateDir/projects/<u>/<uuid>`, `:284`): the guest sees only its contents
    at `~/projects/<dir>`, never the parent directory that holds the name. A
    one-line `realBelow(cfg.HomesDir, cfg.UserHome(user))` would be free
    defence-in-depth against an operator or migration mistake, but it closes
    no reachable path and I am not recording it as a finding.

- **[low] `filepath.Abs("")` → cwd — FIXED.** `files.go:92-97` returns
  `errors.New("empty root")` before `filepath.Abs`, which is the fail-closed
  behaviour asked for. The caller-side guard in `setup.go` is now redundant
  but harmless. Closed.

- **[low] settings counts not attested — REBUTTAL ACCEPTED, closed.** The
  counts are display-only and describe what is on disk; `sh-agent status` is
  the attestation and is what the integration test asserts. Reasonable.

- **[low] `internal/history` `os.OpenRoot(homeDir)` (`history.go:39`, `:125`,
  `:207`) — OPEN, unchanged.** Still three raw roots outside `files.Trust`'s
  reach. Severity unchanged (low): reaching them requires the target to
  already carry `.styrman/logs/<the requester's own project UUID>`.

- **[low] `files.Trust` established from `httpapi.New` — OPEN, unchanged.**
  `main.go:44`/`:66` still run `migrateLegacy`/`migrateWorkspaces` before
  `httpapi.New` at `:97`, so `files.ContainedPath` at `main.go:228` still
  executes with an empty base list. Harmless today (its root is the tenant
  root itself); still a latent trap for any future `files.*` call added
  before `New`.

### New / re-graded finding

- **[high / confidence: high]** `internal/orchestrator/orchestrator.go:217-221`
  (guard present only at `:265`) — **`realBelow` gates container creation and
  nothing else, and Docker resolves a bind source at every start.** I verified
  the Docker half on this host (Engine 29.7.2, default runtime) rather than
  reasoning about it: a container created with a symlink as its bind source
  keeps the *unresolved* path in both `HostConfig.Binds` and `Mounts[].Source`
  (`…/link:/mnt`); starting it printed `content-A`, and after repointing the
  symlink to a second directory the **same, unmodified container** started
  again and printed `content-B`. The daemon does not canonicalise at create,
  so the resolution happens when the mount is set up — which is runtime
  independent and therefore applies to the `mvm`/Kata path (virtiofsd is
  handed the same source string) as well as to runc. Consequence for styrman:
  `EnsureRunning`'s `case err == nil` branch (`:217`) calls `ContainerStart`
  (`:221`, and again at `:226` after a network recreate) on an existing
  container with no `realBelow` and no comparison against the recorded bind.
  Projects are stopped and restarted as a matter of course — the Stop button
  (`Stop`, `:850`) and the idle reaper (`stopIfIdle` `:913`, `RunIdleReaper`
  `:1085`) both stop them, and every subsequent open goes through
  `EnsureRunning`. So a user who replaces their own `~/p/<uuid>` with a
  symlink from the setup VM (bind-mounted `<homes>/<u>`, NOPASSWD sudo,
  `image/entrypoint.sh:55`) and then stops and reopens the project gets the
  link's target mounted as `$HOME` in a microVM where they are root: read and
  write on another tenant's `.claude/.credentials.json`, `.ssh/id_ed25519`,
  `.config/gcloud/*`, or on a host path such as `/var/lib/styrman`. No race is
  involved; the create-time check is simply not on this path. The exposure is
  pre-existing, but it is now *recorded as closed* — the comment at `:259-264`
  and BUG-003 both describe the residual as non-atomicity with `docker
  create` — and a mis-stated fix is worse than a known-open one.
  **Remediation:** hoist the check above the `switch` so it gates both
  branches, e.g. compute `phome := o.cfg.ProjectHome(user, project)` once and
  call `realBelow(o.cfg.UserHome(user), phome)` before `case err == nil` as
  well as before `o.create` — two `Lstat`s per boot. Additionally compare
  `insp.HostConfig.Binds`/`insp.Mounts[0].Source` against the expected `phome`
  before starting an adopted container, so a container created out of band
  cannot carry a foreign source. Correct BUG-003 to `status: partially fixed`
  and restate its residual. The durable fix is still de-nesting project homes
  out of the setup VM's mounted tree (the ADR already in TODO.md), which
  removes the need for `realBelow` entirely.

- **Test-coverage gap (not a finding, but it is why this slipped):**
  `realbelow_test.go` unit-tests the predicate in isolation. Nothing exercises
  `EnsureRunning`, so no test distinguishes "the create path is guarded" from
  "every path is guarded" — a fake-Docker test that stops and restarts a
  project with a planted symlink would have failed and caught this.
  `prune_test.go`, by contrast, tests the janitor at the function that
  actually runs in production, which is why the janitor fix is trustworthy.

### Checked (round 3)

- **Secrets** — clean. No credential in `main.go`, `orchestrator.go`,
  `files.go`, or either new test; `prune_test.go` writes literal `"x"` into
  `t.TempDir()`, `realbelow_test.go` only creates directories. One note, not a
  finding: `realBelow`'s error text is wrapped by `EnsureRunning` and reaches
  the client verbatim through `writeErr` (`server.go:1180`), disclosing the
  absolute host path `/var/lib/styrman/homes/<slug>/p/<uuid>` to the
  authenticated owner of that project. Not new in kind — the pre-existing
  `create project home: %w` on the line above already returns the same path —
  and it names only the caller's own tree.
- **Injection** — clean. No SQL, shell, or template. The two parsing
  boundaries in this round are directory entry names (used only as path
  components through an `os.Root`, never concatenated into a command) and
  `filepath.Rel` over two server-derived absolute paths. No request data
  reaches `realBelow` or `pruneTranscripts`.
- **Authz/authn** — clean and unchanged; nothing in this round touches a
  handler, a guard, or an identity. `authGuard` coverage of
  `GET /api/settings/tools` is as verified in round 1.
- **Crypto** — nothing hand-rolled, nothing added; no auth, session, hashing,
  or token code in this round.
- **Dependencies** — no change: `git diff --stat -- go.mod go.sum
  web/package.json web/package-lock.json` is empty; the round's only new
  import is stdlib `io/fs` in `cmd/styrman`. `bash deploy/vulncheck.sh` (both
  Go modules plus `image/harness`, govulncheck inside the pinned
  `golang:1.25.13-trixie` builder) → exit 0; the same two called findings in
  `github.com/docker/docker` (GO-2026-4887 / GO-2026-4883, both "Fixed in:
  N/A") that rounds 1 and 2 recorded and the project accepts, `image/harness`
  "No vulnerabilities found". `npm audit` in `web/` → "found 0
  vulnerabilities".
- **GCP surface** — not applicable and untouched; styrman is self-hosted. No
  IAM binding, service account, `--allow-unauthenticated`, key file, or
  env-var secret in this round.
- **Tests** — `go test ./cmd/styrman/ ./internal/orchestrator/
  ./internal/files/ ./internal/httpapi/ -race` (go1.25.0) → all four `ok`, no
  race, no failure. `go vet ./...` clean.

### Convergence

**Not converged.** Two of the three fixes land (janitor: complete and
verified; `openRoot("")`: complete) and one rebuttal is accepted, but one
high-severity path is open — and, unlike the previous rounds, it is
documented as closed in the code comment, in BUG-003, and in the change
record. This is round 3 of three: per the project's rule, the remaining item
goes to the operator rather than another review round. The fix is a
one-statement hoist plus a correction to BUG-003; it does not need a redesign.
