---
owner: security
date: 2026-09-04
---
# Security review — un-nest project homes, Phase 1   (2026-09-04)

Range reviewed: `0da855e..<working tree>` — the uncommitted Phase 1 diff of
`.agents/docs/plans/2026-09-04-unnest-project-homes.md`:
`internal/config/config.go`, `cmd/styrman/main.go`,
`internal/orchestrator/orchestrator.go`, `internal/httpapi/server.go`,
`cmd/styrman/migrate_test.go`, `internal/config/config_test.go`,
`internal/orchestrator/realbelow_test.go`, `.agents/rules/guest-writable-paths.md`.
Blast radius followed outward: every `HomesDir`/`UserHome`/`TenantDir`/`ProjectHome`
consumer in `internal/` and `cmd/` (`files.Trust`+`openRoot`, the transcript janitor,
`credentials.go:262-263`, `credharvest.go`, `setup.go:34,127,214`, `git.go:73`,
`explorer.go:48,443`, `config.go:372 LegacyWorkspaceExists`,
`main.go:232 migrateWorkspaces`), both orchestrator `create` bind specs, the compose
mount layout, and ADR-064.

## Verdict: FINDINGS

**Blocking: yes.**

The layout change is the right fix and, once it has taken effect, it does close the
BUG-002/003 class by construction: `base/` and `p/<uuid>` become siblings under a
server-only tenant dir, so no guest mount contains another guest's home or any
server-trusted path outside its own tenant. The problem is the transition. `migrateBaseHomes`
runs at server start, **before** the orchestrator exists and therefore before anything can
stop a pre-ADR-064 setup VM — which is still running with the *tenant dir* bind-mounted
read-write at `/home/<guest>`, as the same uid the server runs as. The migration `Lstat`s
`base` exactly once and then calls `os.Rename(tenant/<name>, base/<name>)` in a loop; POSIX
`rename(2)` **follows** a symlink that appears as an intermediate component of *newpath*
(verified locally on go1.25.0: the entry lands outside), so a guest that swaps `base`
between a real directory and a symlink in a loop redirects the server's renames — with
guest-chosen filenames and guest-chosen content — into any directory on the
`/var/lib/styrman` filesystem the server's uid can write. That includes another tenant's
`homes/<victim>/base/.bashrc` (code execution in the victim's setup VM, where the broker
materializes their credentials) and `StateDir/users.json` (the allowlist, read moments
later in the same boot; a malformed one is a hard `os.Exit(1)` startup DoS). The change
that exists to remove a TOCTOU introduces a wider one for the duration of the upgrade it
performs. A second, quieter gap: nothing removes the stale setup containers at startup, so
until each user next opens onboarding, BUG-003's original residual is not closed at all —
it is *narrowed by intent only*, and still live on the exact hosts this ships to.

## Findings

- **[high / confidence: high]** `cmd/styrman/main.go:452-469` (`migrateBaseHomes`) —
  **symlink race on `base` lets a guest redirect the migration's renames out of the tenant
  dir, as the server's uid.** The guard is `os.Lstat(base)` once at :459, outside the loop;
  each `os.Rename(filepath.Join(tenant, name), filepath.Join(base, name))` at :466
  re-resolves `base` in the kernel. `base` is a non-final component of *newpath*, and
  `rename(2)` follows symlinks on every component but the last — I confirmed the three
  relevant semantics on this toolchain: (a) rename through a symlinked *intermediate*
  succeeds and lands outside; (b) rename onto a *trailing* symlink replaces the link, it
  does **not** write through (so a symlink farm *inside* `base` is harmless — that half of
  the question is clean); (c) `os.Root.Rename` refuses (a), `path escapes from parent`.
  The attacker is any allowlisted tenant whose setup VM was created before this change and
  is still running when the server restarts: that VM binds `TenantDir` (`homes/<u>`) at
  `/home/<guest>` read-write (`orchestrator.go:620` at the old `UserHome`, `create` →
  `Binds` at `:832`), it runs as `${STYRMAN_UID}` — the server's own uid
  (`deploy/docker-compose.yml:75`) — and `/var/lib/styrman` is a single bind
  (`:120`), so homes and state are one filesystem and cross-directory renames succeed.
  The attacker controls both the entry *names* (anything in the tenant dir that is not
  `base` or `p` is moved) and their *content*, and can spin `mkdir base; rmdir base;
  ln -s <target> base; rm base` indefinitely from a backgrounded process; the loop gives
  one chance per entry per boot, re-armed every boot, and the upgrade restart is exactly
  when it fires. Worst case, in decreasing certainty: overwrite `homes/<victim>/base/.bashrc`
  or `.profile` → command execution in the victim's setup VM with their brokered
  credentials; overwrite `StateDir/users.json`, which `userstore.New` reads *after* the
  migration in the same boot (`main.go:47`) → arbitrary emails on the allowlist, or invalid
  JSON → `parse users.json` error → `os.Exit(1)`, permanent startup DoS; overwrite a
  victim's `StateDir/users/<v>/projects.json` or a `StateDir/credentials/` entry. The
  `os.Mkdir` branch at :464 is *not* affected (mkdir does not follow a final symlink; it
  fails `EEXIST` and the user is skipped) — the loop is the whole exposure.
  **Remediation, two parts, both cheap:** (1) do the renames confined —
  `rt, _ := os.OpenRoot(tenant); rt.MkdirAll("base", 0o700); rt.Rename(name, "base/"+name)`.
  `os.Root.Rename` exists in the pinned go1.25 toolchain (`go.mod: go 1.25.0`;
  `deploy/Dockerfile.server: golang:1.25.13-trixie`) and resolves both operands with
  `openat`-style confinement, so no swap of `base` can redirect it. (2) Better still,
  remove the adversary first: move `migrateBaseHomes` to run *after* `orchestrator.New`
  (like `migrateWorkspaces`, which already stops a live VM before renaming under it,
  `main.go:66` and `:253-260`) and force-remove every container labelled
  `system=setup` whose `/home/<guest>` bind is not the new `UserHome` — `setupHomeStale`
  is already the predicate. Part (2) also fixes the next finding. Keep the `Lstat` refusal
  as the third layer.

- **[medium / confidence: high]** `internal/orchestrator/orchestrator.go:584-592`
  (`setupHomeStale` recreate) + `cmd/styrman/main.go:44-45` — **the layout change is lazy,
  so BUG-003's residual is narrowed, not closed, for an unbounded window after the
  upgrade.** A pre-ADR-064 setup container is only recreated when that user's next
  `EnsureSetup` call happens (Settings → onboarding). Until then it keeps the tenant dir
  mounted, its `$HOME` still contains `p/` — every project home the user owns, writable —
  and the exact medium accepted in `reviews/2026-09-04-security-v1.20.0.md` still applies
  verbatim: `projectHomeIsReal` at `:226` is a sequence of `Lstat`s and Docker re-resolves
  the bind source at `ContainerStart` (`:234`, `:239`) and `ContainerCreate` (`:832`), so a
  guest swapping `~/p/<uuid>` in a loop while stopping/starting a project can still mount an
  arbitrary host path as a root guest's `$HOME`. Nothing in this diff, in `CleanupLegacy`,
  or in `Adopt` sweeps those containers proactively; the plan's Failure-modes section treats
  a setup VM running through the deploy purely as a UX interruption ("nothing is lost"),
  which is true for credentials and false for the threat model. ADR-064's "the accepted
  TOCTOU medium is closed by construction" is therefore accurate only *eventually*.
  **Remediation:** the same startup sweep as above — enumerate `label=system=setup`,
  test with `setupHomeStale`, force-remove the stale ones before the migration runs; log
  one line per removal. That makes the ADR's claim true at the moment of upgrade, and the
  lazy `EnsureSetup` check becomes the backstop it reads as.

- **[low / confidence: high]** `cmd/styrman/main.go:465-470` — **a colliding regular file in
  `base/` is silently overwritten, contradicting ADR-064 and the plan.** ADR-064 says the
  migration leaves "a colliding entry in place"; `os.Rename` only does that for a
  *non-empty directory* (`ENOTEMPTY`, logged as "move failed"). A top-level regular file —
  `.gitconfig`, `.bashrc`, `.bash_history` — is replaced atomically with no error and no log
  line, and the counter reports it as a success. This is reachable on the second boot after a
  partial migration, and on any boot where the still-running old setup VM re-created a file
  in the tenant dir after the first pass. `migrateWorkspaces` is the house precedent: it
  detects the both-exist case and logs `workspace migration CONFLICT … resolve by hand`
  (`main.go:240-248`). **Remediation:** `os.Lstat(filepath.Join(base, name))` (or
  `rt.Lstat`) before each rename; on any existing target, skip and log at Warn with the
  entry name, matching the ADR's stated behaviour.

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:594-600` —
  **`EnsureSetup`'s reuse path starts an existing setup container without re-checking its
  bind source, unlike `EnsureRunning`.** `EnsureRunning` deliberately calls
  `projectHomeIsReal` *before* the switch precisely because "START re-resolves the stored,
  unresolved source on every start (verified on Engine 29.7.2)" (`:216-226`). The
  `realBelow(TenantDir, UserHome)` added here is create-only (`:612`); the `exists` branch
  calls `ContainerStart` at `:598` with no check. Unreachable today once the tenant dir is
  genuinely server-only — which is the point of the change — but this is the exact
  create-only-guard asymmetry that BUG-003 recorded, and it silently depends on a property
  enforced nowhere in this function. **Remediation:** hoist `realBelow(o.cfg.TenantDir(user),
  o.cfg.UserHome(user))` above the `if exists` branch so both paths are covered, matching
  `EnsureRunning:226`.

- **[low / confidence: medium]** `internal/httpapi/git.go:73` — **the one server read below
  `HomesDir` that is not `os.Root`-confined, and its target is now guest-writable in
  `base/`.** `gitIdentity` builds `filepath.Join(s.cfg.UserHome(user), ".gitconfig")` by
  path and hands it to `exec.Command("git", "config", "--file", cfg, key)`; git follows a
  symlinked `--file`. A guest can point `base/.gitconfig` at any file the server's uid can
  read and have `user.name` / `user.email` lines lifted out of it — into the settings UI
  (`setup.go:238-245`), into `git -c user.name=…` on commit (`git.go:282`), and into the
  broker as `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL` (`credharvest.go:52-61`). Exfiltration is
  limited to lines that parse as those two keys, and `--file` disables `include.*` expansion
  by default, which is why this is low and not medium; no argv injection (values are single
  argv elements). Pre-existing — `UserHome` was equally guest-writable before this diff, so
  the change neither creates nor worsens it — but it is inside the blast radius and it is the
  odd one out: every sibling read (`setup.go` readiness, `explorer.go`, `credentials.go`
  sweeps, the janitor) goes through `files.openRoot`. **Remediation:** `files.IsRegular(h,
  ".gitconfig")` + `files.ReadRaw(h, ".gitconfig", 0)` and parse the two keys in Go — the
  pattern already used one function away for `.ssh/id_ed25519.pub`, whose comment explains
  exactly this reasoning.

- **[low / confidence: medium]** `cmd/styrman/main.go:182-192` (`migrateLegacy` step 1) —
  **the legacy guard now tests the wrong path on a half-migrated host.** The
  `os.Stat(target)` precondition moved from `homes/<user>` to `homes/<user>/base`. On a host
  that has *both* `StateDir/home` and an already-populated `homes/<user>` without `base` —
  an inconsistent state, but the state a failed earlier migration leaves — the old code
  correctly skipped, and the new code renames the legacy home into `base/` and then hands
  `migrateBaseHomes` a `base` that is already full, so the tenant's own entries collide:
  directories fail `ENOTEMPTY` and are stranded in the tenant dir, regular files are silently
  overwritten per the finding above. No such host is expected (the operator states the
  multi-tenant cut is long past), which is why confidence is medium and severity low.
  **Remediation:** keep both preconditions — skip step 1 when *either* `TenantDir(user)` is
  non-empty or `UserHome(user)` exists.

- **[low / confidence: high]** `internal/orchestrator/orchestrator.go:588-590` —
  **the stale-bind path force-removes a possibly-running setup VM.** `ContainerRemove(…,
  Force: true)` skips the entrypoint's `TERM` trap. For a setup VM this is much less
  dangerous than the file itself warns about elsewhere ("a force-remove would skip the
  entrypoint's SIGTERM trap and tear down the loop-mounted ext4 dirty", `:186-189`), because
  `STYRMAN_MINIMAL=1` takes the `trap 'exit 0' TERM INT; sleep infinity` branch with no
  dockerd and no loop-mounted image (`image/entrypoint.sh:199-205`). What can still be lost
  is unflushed virtiofs data written into `base/` in the seconds before the kill — precisely
  a just-pasted OAuth token — plus the live tmux/terminal session and any in-flight
  interactive sign-in prompt. Consistent with the adjacent guest-changed branch, so this is
  a nit, not a regression. **Remediation:** `ContainerStop` with a short timeout, then
  remove, as the ADR-027 recreate path does at `:189-194`.

## Answers to the questions asked

1. **Does any guest mount still contain a path a different guest or the server treats as
   trusted?** No — *after* the layout takes effect. There are exactly two bind sites, both in
   `create` → `HostConfig.Binds` (`orchestrator.go:831-837`): the project VM gets
   `ProjectHome = homes/<u>/p/<uuid>` → `/home/<guest>` plus
   `ProjectWorkspace = StateDir/projects/<u>/<uuid>` → `/home/<guest>/projects/<dir>`
   (`:298-305`); the setup VM gets `UserHome = homes/<u>/base` → `/home/<guest>`, no
   workspace (`:615-620`). `base/` and `p/<uuid>` are now siblings, so the only remaining
   nesting is a project's own workspace inside its own home — same guest, same project.
   The server's reads/writes below `HomesDir` are all same-tenant and all confined except
   one: `files.Trust(HomesDir, StateDir/projects)` (`server.go:126`) makes `openRoot` anchor
   at `<base>/<tenant>` and then `Root.OpenRoot(rest)`, so an explorer/history/credential
   root at `base` or at `p/<uuid>` refuses a symlink at any component *and* confines the
   sub-root to that project (cross-project links within one tenant are refused too); the
   janitor (`main.go:384-414`) opens the tenant root and reaches `p` and `p/<uuid>` only
   through it; `credentials.go:262-263` globs `HomesDir/<u>/p/*` — `filepath.Glob` `Lstat`s,
   so a symlinked `p` yields no matches — and each home is then handed to the confined
   `files.Remove`; `setup.go:34,127,214` are `files.Exists`/`ReadRaw`/`Save` on `UserHome`;
   `credharvest.go` reads the guest, not the host. The exception is `git.go:73`, the raw
   `--file` path (finding above). **BUG-003's TOCTOU residual: narrowed, not closed** — it is
   closed by construction only for tenants with no surviving pre-ADR-064 setup container, and
   nothing removes those at upgrade (finding 2). Fix finding 2 and the answer becomes
   "closed".
2. **Can the guest race the migration?** Yes — see the high finding. Renamed entries are
   redirected through a swapped `base` because `rename(2)` follows intermediate symlinks in
   *newpath*; the single pre-loop `Lstat` does not protect the N renames that follow. The
   other half of the question is clean: a guest that pre-creates `base` as a real directory
   full of symlinks gains nothing, because the *final* component of newpath is never followed
   — the rename replaces the link itself (verified). Worst case as the server's uid:
   arbitrary-file placement with guest-controlled content anywhere on the
   `/var/lib/styrman` filesystem, i.e. cross-tenant home poisoning → code execution in
   another tenant's VM, allowlist rewrite, or a startup-DoS `users.json`.
3. **`setupHomeStale`.** `insp.Mounts` is server-set at create and cannot be influenced by
   the guest: the only binds are `spec.homePath` and `spec.workspacePath`, both composed
   server-side from `config`, no host Docker socket reaches any guest, and a guest has no
   API to its own container's metadata. `filepath.Clean` on both sides handles the trailing
   slash; a missing `/home/<guest>` mount reads as stale, which is fail-safe (recreate) but
   means a transient empty `Mounts` from Docker would cause an unnecessary recreate — and
   note that if `mkdirAllConfined`/`realBelow` then fails, the user's setup VM has already
   been removed and `EnsureSetup` returns an error, i.e. fail-closed but destructive.
   Force-removing during sign-in loses **nothing the broker holds**: harvest only ever
   `Put`s what it finds and never deletes on absence (`credharvest.go:32-96`), the home is a
   bind so the files survive on the host, and after the migration they are exactly where the
   recreated VM will mount them. What is lost is the live shell/tmux session, an in-flight
   interactive OAuth prompt, and any unflushed write at kill time (low finding above). One
   transient wrinkle: while an old setup VM is still bound to the tenant dir, a git identity
   configured in it lands at `homes/<u>/.gitconfig`, so `gitIdentity` (host-side, now reading
   `base/`) reports "not found" until the next boot's migration moves it.
4. **`migrateLegacy`.** Correct in the normal cases and correctly ordered: it runs before
   `migrateBaseHomes` (`main.go:44-45`), which in turn runs before `migrateWorkspaces`
   (`:66`), so a pre-ADR-027 `projects/<dir>` reaches `base/projects/<dir>` before
   `LegacyWorkspaceExists` and `ContainedPath(UserHome, "projects/…")` look for it — the new
   test asserts exactly that. `os.MkdirAll(TenantDir, 0o700)` is a no-op on an existing dir
   and does not re-chmod, and the error is logged without aborting startup. The one
   regression is the changed guard on a half-migrated host (low finding above). Unrelated
   pre-existing note, unchanged by this diff: `migrateWorkspaces` resolves the old workspace
   with `files.ContainedPath` (`EvalSymlinks`, not `os.Root`) and then renames by path, so an
   intermediate `base/projects` swapped between the check and the rename could move another
   tenant's directory — same class, one-time path, guarded by the containment check; worth
   folding into the same `os.Root` sweep if the high finding is fixed with `Root.Rename`.
5. **R10.** No credential value is read, logged, or moved across a trust boundary by the
   change *as designed*: the migration moves secret-bearing files (`.ssh/id_ed25519`,
   `.claude/.credentials.json`, `.config/gcloud/*`) only within one tenant's own tree,
   host-side, without opening them. The stated claim "logs entry counts, not names" is
   **not exactly right**: the success line at `main.go:470` logs `user`, `to`, `entries`,
   `failed` — counts — but the failure line at `:467` logs `"entry", name`, a guest-chosen
   filename. That is metadata, not a secret, and `slog`'s JSON handler escapes it, so there
   is no log-injection or disclosure issue; the plan/ADR wording should just be corrected.
   R10 does hold — **conditionally on the high finding being fixed**, since the race turns
   the migration into a cross-tenant secret *mover*.

## Checked

- **Secrets** — no credential, token, or key added in the range; nothing secret-shaped is
  read, echoed, or written to a log. `git diff` over the eight files shows no `.env`, key
  material, or hardcoded value. Only caveat: the migration's error branch logs a
  guest-chosen filename (question 5).
- **Injection** — no SQL, no shell string interpolation, no template. The only `exec` in
  the blast radius is `git config --file <server-built path> <constant key>`
  (`git.go:73-81`) and the values it returns are passed as single argv elements
  (`git.go:282`) — no argv splitting. Path handling is the real parsing boundary here and it
  is covered by the findings: `os.ReadDir` entry names cannot contain `/`, `.`, or `..`, so
  the only untrusted input is the *existence and type* of `base`, which is the race.
- **Authz/authn** — untouched. No route, middleware, ingress header, or `who()` path is in
  the range. Indirect exposure only: `users.json` (the allowlist re-checked at
  `userstore.New`, `main.go:47`) becomes an overwrite target under the high finding, which
  is why that finding is an authz issue and not merely a filesystem one.
- **Crypto** — nothing hand-rolled, nothing added; no auth, hashing, session, or JWT code in
  the range.
- **Dependencies** — no new dependency; `go.mod`, `go.sum`, `web/package.json`, and
  `web/package-lock.json` are unmodified (`git status --porcelain` over all four: empty).
  `bash deploy/vulncheck.sh` in the pinned `golang:1.25.13-trixie` builder: root module —
  2 affected (`GO-2026-4887`, `GO-2026-4883`, both `github.com/docker/docker@v28.5.2`, both
  "Fixed in: N/A"); `deploy/caddy/ingress` — the same two at `v27.5.1`; `image/harness` —
  no vulnerabilities. Identical to the v1.20.0 baseline, unchanged by this diff; both are
  daemon-side plugin-privilege/AuthZ issues in Moby with no fixed version and no authz
  plugin in this deployment. A bare local `govulncheck ./...` additionally reports stdlib
  findings only because the host toolchain is go1.25.0; the shipped binary is built at
  1.25.13, which is why the pinned script is the authoritative run. `npm audit` in `web/`:
  **found 0 vulnerabilities**.
- **GCP surface** — not applicable: self-hosted (`.agents/docs/deploy/gcp.md` records this).
  No IAM binding, service account, Cloud Run flag, or key file anywhere in the range. The
  only credential-at-rest concern is `StateDir/credentials`, which is local and is a target
  of the high finding, not a GCP surface.
- **Build/tests** — `go vet ./...` clean; `go test ./cmd/... ./internal/config/...
  ./internal/orchestrator/... ./internal/files/...` all `ok`, including the new
  `TestMigrateBaseHomes`, `TestMigrateLegacyHomeLandsInBase`, `TestSetupHomeStale`, and
  `TestHomeLayoutKeepsGuestMountsApart`. Note the coverage gap this review turns on: the new
  migration test exercises the *static* planted-symlink case (carol) but no concurrent
  swap — the high finding is invisible to it by construction. A regression test that renames
  through a swapped intermediate is the natural companion to the `Root.Rename` fix.

---

## Round 2 (2026-09-04)

Range reviewed: `0da855e..<working tree>` — re-review of
`cmd/styrman/main.go`, `internal/orchestrator/orchestrator.go`,
`internal/config/config.go`, `internal/httpapi/server.go`,
`cmd/styrman/migrate_test.go`, `internal/orchestrator/realbelow_test.go`,
plus ADR-064 and `TODO.md` for the recorded acceptances. Blast radius
re-walked: the new startup order in `main()` (`migrateLegacy` →
`userstore.New` → `orchestrator.New` → `RemoveStaleSetupVMs` →
`migrateBaseHomes` → `migrateWorkspaces` → background adopt/sweep →
listeners), every `UserHome`/`TenantDir` consumer, and the Docker list/inspect
path the sweep adds.

### Verdict: FINDINGS (non-blocking)

**Blocking: no.**

The high finding is closed, and closed at the right layer: every step of the
migration now runs through `os.OpenRoot(TenantDir)`, so the kernel — not a
pre-loop `Lstat` — is what refuses a redirect. I re-verified the four
semantics this turns on against the pinned language version (go1.25, the
module's `go 1.25.0` directive selects the 1.25.0 toolchain; the bare `go` on
PATH is 1.24.4, where `os.Root.Rename` does not exist — worth knowing before
anyone builds outside the pinned image):

| case | result |
|---|---|
| `rt.Rename("f", "base/f")`, `base` → a dir outside the root | `renameat f base/f: path escapes from parent` — **refused**, nothing lands outside |
| `rt.Lstat("base")` on a planted symlink | `IsDir()` false → the up-front refusal fires |
| `rt.Rename` of a **symlink entry** | the link itself moves; its target is untouched and is *not* dereferenced |
| `rt.Lstat("base/x")` where `base/x` is a **dangling** symlink | `nil` → the collision check counts it as `leftInPlace`, never replaces it |

Cross-tenant reach is gone, so the `users.json` / victim-`base/.bashrc`
scenarios are gone with it. What is left is same-tenant only: a guest that
wins the race between `rt.Lstat("base")` and the renames can point `base` at
its own `p/<uuid>` (case 3, confirmed: the rename succeeds and the entry lands
in the user's own project home), and the `rt.Lstat(dst)` → `rt.Rename` gap can
still let a racing guest have its own file replaced. Both are one user's own
files landing in one user's own project home, which the broker already fills
with that same user's credentials — no boundary crossed. The one new weakness
the fix itself introduces is availability, not confidentiality: the sweep is
now synchronous pre-listener Docker work on an unbounded `context.Background()`.

### Per finding

- **[high] `migrateBaseHomes` rename through a swapped `base` — CLOSED.**
  `main.go:453-518`. `os.OpenRoot(cfg.TenantDir(...))` per tenant;
  `fs.ReadDir(rt.FS(), ".")`, `rt.Lstat("base")` (non-dir → hard refusal, no
  entry moved), `rt.Mkdir`, `rt.Lstat(dst)`, `rt.Rename(name, "base/"+name)` —
  no raw path operation survives in the loop. A symlinked tenant *entry* is
  skipped by `u.IsDir()` (`os.ReadDir` types come from the directory entry, not
  a stat, so a link reports as non-dir). Logging is counts only —
  `user`/`to`/`entries`/`leftInPlace`/`failed`, with the guest-chosen `"entry",
  name` of round 1 gone; the remaining error strings are `os.Root` errors whose
  paths are the constants `"."`/`"base"`. R10's "counts, not names" claim in
  ADR-064 is now literally true. Ordering also helps twice over: the migration
  runs *after* `userstore.New`, so `users.json` is already loaded, and *after*
  `orchestrator.New` + the sweep, so in the normal case there is no live guest
  at all.

- **[medium] recreate is lazy, BUG-003's residual stays open after upgrade —
  CLOSED.** `orchestrator.go:1329-1365` (`RemoveStaleSetupVMs`) called at
  `main.go:64`, before `migrateBaseHomes` at `:70` and before
  `migrateWorkspaces` at `:73` — which incidentally hardens
  `migrateWorkspaces` too, since its `files.ContainedPath` + by-path rename
  (round 1, answer 4) now runs at a moment when nothing has `base/` mounted.
  The filter is exact (`label=styrman.system=setup`, constants only, house
  precedent `ReapSetupVMs:1223`), `All: true` catches stopped ones, and
  `setupHomeStale` fails safe: a missing `/home/<guest>` mount, or a
  `containerGuest(Env)` that comes back `""` because `STYRMAN_USER` is absent,
  both read as stale → removed → `EnsureSetup` recreates. Two residuals, both
  low and noted below: the sweep is best-effort on a Docker error, and it has
  no unit test (only `setupHomeStale` does — a fake-Docker test for the
  list/inspect/remove path is the natural companion, since this function is now
  the *precondition* the migration's "no guest holds the old mount" claim rests
  on).

- **[low] colliding entry silently overwritten — CLOSED.** `main.go:508-511`,
  `rt.Lstat(dst)` before every rename; `TestMigrateBaseHomesLeavesCollisionsInPlace`
  asserts the colliding entry stays in the tenant dir, the existing `base/`
  entry is untouched, and the non-colliding one still moves. Verified the check
  also catches a *dangling* symlink at the destination (table above), which is
  the case a plain `os.Stat` would have missed.

- **[low] `EnsureSetup` reuse path never re-checks the bind source — CLOSED.**
  `orchestrator.go:565-570`: `projectHomeIsReal(TenantDir, UserHome)` is now the
  first thing after the per-user lock, so the `exists` → `ContainerStart` path is
  covered as well as create, matching `EnsureRunning:226`. `ErrNotExist` is
  tolerated, so a first-time user still reaches the create path, which then
  re-checks with `realBelow` after `mkdirAllConfined` (`:616-621`).

- **[low] `migrateLegacy` guard on a half-migrated host — CLOSED.**
  `main.go:196-199`: step 1 requires `UserHome` absent **and**
  `len(tenantEntries) == 0`; `TestMigrateLegacySkipsAHalfMigratedTenant` covers
  it. This also closes the ordering question the sweep would otherwise raise —
  `migrateLegacy` still runs before `orchestrator.New` and therefore before the
  sweep, but any host with a live pre-ADR-064 setup VM has a non-empty tenant
  dir (its dotfiles), so step 1 is a no-op exactly where a live guest exists.

- **[low] `git.go:73` unconfined `git config --file` — REBUTTAL ACCEPTED.**
  The file is the user's own and the guest can plant that symlink in `base/`
  directly whether or not the migration moves one there, so the change neither
  creates nor widens it; recorded in `TODO.md:19` as a green low with the right
  remediation (route it through the confined `files.*` read). One factual
  addition for the record: the migration *does* now move a pre-existing
  `homes/<u>/.gitconfig` symlink into `base/` as a link (case 1 above) — same
  blast radius, no new exposure, no reason to change the disposition.

- **[low] `Force: true` teardown — ACCEPTED.** Recorded in ADR-064's Decision
  ("force-removes") and Consequences ("interrupted once, nothing is lost — the
  broker holds what was harvested"). Residual unchanged and accepted: a secret
  pasted into the setup VM seconds before the process stopped, never harvested,
  is lost. Note ADR-064's Consequences paragraph still says a setup VM running
  through the upgrade "sees its files under `~/base/` until `EnsureSetup` next
  runs and recreates it" — the startup sweep now removes it at boot, so that
  sentence describes the pre-round-2 behaviour and should be corrected (docs
  only, non-blocking).

### New in round 2

- **[low / confidence: medium]** `cmd/styrman/main.go:64` — **the sweep is
  synchronous, pre-listener Docker work on an unbounded context.**
  `orch.RemoveStaleSetupVMs(context.Background())` does one `ContainerList` plus
  one `ContainerInspect` per setup container with no deadline, before the HTTP
  listener starts. `main.go:82-85` records exactly this failure mode as a past
  incident — "Boot recovery MUST NOT delay the listener: after a host reboot a
  slow dockerd (10 Kata VMs restarting) made /api/health hang for minutes, which
  the ingress reads as 502" — which is why `Adopt` was moved to a goroutine with
  a 60s timeout. The sweep genuinely *must* stay synchronous and before the
  migration (making it async reopens the medium), so the fix is a deadline, not
  a goroutine: `ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)`.
  A hung dockerd then degrades to the already-handled `logger.Warn("stale setup
  VM sweep failed")` path plus a confined migration, instead of a boot that never
  reaches `ListenAndServe`. Availability only; no confidentiality impact.
- **[low / confidence: low]** `internal/orchestrator/orchestrator.go:1353-1354` —
  **`insp.Config.Labels` / `insp.Config.Env` are dereferenced without a nil
  check, in code that now runs before the listener.** If the daemon ever returns
  an `InspectResponse` with a nil `Config`, this panics and the server never
  boots. Not guest-influenceable (a container removed between list and inspect
  gives `NotFound`, which is already skipped) and the same unguarded pattern
  exists at `:429`, `:460`, `:581`, so this is a pre-existing house habit — but
  the blast radius is worse here because a panic in `main()` is a hard startup
  failure rather than one failed request. `if insp.Config == nil { continue }`.
- **Not findings, for the record.** (a) The `filters` import moved into the
  first import group; `gofmt -l` is clean on all six files and the identifier is
  used exactly once with constant arguments — cosmetic only. (b) The sweep
  matches `styrman.system=setup` daemon-wide, so two styrman instances sharing a
  Docker daemon would remove each other's setup VMs; identical to the existing
  `ReapSetupVMs` scope and out of scope for a single-host deployment. (c)
  `setupHomeStale` reading a missing/renamed guest env as stale can cause one
  unnecessary recreate — fail-safe and self-healing.

### Checked (round 2)

- **Secrets** — no credential, token, or key added or moved across a boundary by
  the new code; the migration still moves secret-bearing files only within one
  tenant's own tree without opening them, and round 1's one log-metadata caveat
  (`"entry", name`) is gone. `TODO.md` and ADR-064 carry no secret material.
- **Injection** — no SQL, shell, or template construction added. The new Docker
  filter is built from constants (`config.LabelSystem+"=setup"`); the only
  untrusted input in the range remains the *existence and type* of `base` and
  the tenant-dir entry names, and both are now handled inside an `os.Root`,
  where names cannot contain `/` and cannot escape.
- **Authz/authn** — untouched; no route, middleware, ingress header, or `who()`
  path in the range. The indirect authz exposure of round 1 (the migration able
  to rewrite `StateDir/users.json`) is closed by the confinement *and* by the
  reordering that puts the migration after `userstore.New`.
- **Crypto** — nothing hand-rolled, nothing added; no auth, hashing, session, or
  JWT code in the range.
- **Dependencies** — none added or changed: `git status --porcelain` over
  `go.mod`, `go.sum`, `web/package.json`, `web/package-lock.json`,
  `deploy/caddy/ingress/go.{mod,sum}` is empty, so round 1's authoritative
  `bash deploy/vulncheck.sh` run inside the pinned `golang:1.25.13-trixie`
  builder stands unchanged (2 affected in each Go module, both
  `github.com/docker/docker` plugin-privilege/AuthZ issues with no fixed
  version and no authz plugin deployed; `image/harness` clean; `npm audit` in
  `web/`: 0 vulnerabilities). Not re-run: nothing in the range can change the
  result.
- **GCP surface** — not applicable, self-hosted (`.agents/docs/deploy/gcp.md`).
  No IAM binding, service account, Cloud Run flag, or key file in the range.
- **Build/tests** — `go vet ./...` clean; `gofmt -l` clean on all six changed
  files; `go test ./cmd/styrman/ ./internal/orchestrator/ -race` → both `ok`
  (1.02s each), including `TestMigrateBaseHomes`,
  `TestMigrateBaseHomesLeavesCollisionsInPlace`,
  `TestMigrateLegacySkipsAHalfMigratedTenant`, `TestMigrateLegacyHomeLandsInBase`,
  `TestSetupHomeStale`; `go test ./internal/config/ ./internal/httpapi/
  ./internal/files/` also `ok`. Coverage gaps that remain, neither blocking:
  no test drives a *concurrent* `base` swap (the confinement makes the static
  cases sufficient, but a `rt.Rename`-through-a-swapped-intermediate regression
  test would pin the property), and `RemoveStaleSetupVMs` has no fake-Docker
  test despite now being the migration's stated precondition.
