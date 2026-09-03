---
owner: security
date: 2026-09-03
---
# Security review — harness phase 2 (entrypoint block + harness-migrate.sh)   (2026-09-03)

Range reviewed: `c1a6eb9..working tree` — `git diff -- image/entrypoint.sh` (the harness block, lines 110-137)
plus the new untracked `image/configs/harness-migrate.sh`. Blast radius followed outward: the installer the
block now invokes (`image/harness/internal/harness/apply.go`, `plan.go`, `repo.go`, `state.go`,
`cmd/sh-agent/main.go`), the container-log pipeline the block writes into
(`internal/orchestrator/orchestrator.go:1189` → `internal/httpapi/stats.go:126` → `web/src/routes/index.tsx:182`),
the host-side consumers of the artifacts it writes (`internal/httpapi/setup.go:189-273` via `internal/files`),
the mounts the guest can persist to (`internal/orchestrator/orchestrator.go:782-789`), the guest-username
validation that feeds the interpolations (`internal/httpapi/profile.go:28`), the runtime image
(`image/Dockerfile`), and `deploy/vulncheck.sh`. Phase 1 (builder stage + vendored tree) not repeated.

## Verdict: FINDINGS

Nothing here crosses a tenant or host boundary — the block drops to the project uid with `runuser`, the
microVM mounts only that project's own home and workspace, and the host-side reads of those artifacts go
through `os.Root`-confined helpers — so the symlink and traversal questions all bottom out inside the user's
own blast radius, where they already have NOPASSWD sudo (`image/entrypoint.sh:55`). The real weakness is
availability: the block is now an **unbounded, user-influenced step sitting ahead of `/run/styrman/ready`**
(`image/entrypoint.sh:268`) with no `timeout` and with a `set -e` helper gated by `&&`, so a home the user
shaped — a `~/.gitconfig` whose `[include] path` is a FIFO, a directory they chmod'ed 0500, a cross-device
`~/.styrman` symlink — stalls or hard-fails the boot before dockerd, on every boot, with no self-recovery.
Everything else is low: log-line spoofing from user-chosen filenames, and an unvalidated manifest path in the
installer that is latent rather than exploitable here.

## Findings

- **[medium / confidence: high]** `image/entrypoint.sh:129-130` — no time bound on either `runuser` call, and
  the whole block precedes `touch /run/styrman/ready` (`:268`). `Install` ends with
  `git -C /styrman/harness rev-parse --short HEAD` (`image/harness/internal/harness/apply.go:75` →
  `repo.go:105`) executed with `HOME` set to the **user-controlled project home**, so git parses the user's
  `~/.gitconfig` before anything else; an `[include] path = <fifo>` there blocks the read indefinitely and the
  entrypoint never reaches the ready marker, so `waitReady` fails, terminal attach fails, and the project is
  bricked for its owner until an operator intervenes — persistently, because the stamp is only written on
  success. Same shape, slower: `~/.styrman` symlinked onto the workspace mount turns every `mv -T` into a
  cross-device copy of whatever tree the user planted at `~/.claude/skills`. This is self-inflicted and
  single-project (per-project lock, own bind mounts only), but it is an unbounded boot-path stall that the
  removed ADR-047 comment used to reason about explicitly. Resolve by wrapping both calls in
  `timeout 60 …` — the old objection to `timeout` no longer holds, since `sh-agent install` is idempotent and
  the stamp is written only after both steps succeed, so a killed attempt simply retries next boot.
- **[low / confidence: high]** `image/configs/harness-migrate.sh:28,81` + `image/entrypoint.sh:129` —
  `set -euo pipefail` plus a bare `mv -T` means the **first** unmovable path aborts the helper mid-inventory,
  and the entrypoint's `&&` then skips `sh-agent install` entirely. Verified in a scratch home: with
  `chmod 500 ~/.claude`, the helper moves `.claude/CLAUDE.md`, dies on `.claude/skills` with exit 1, leaves
  `.gemini/*` unmigrated, and the harness is never installed — repeated identically on every subsequent boot
  (stamp unchanged), while the log says only `WARN: agent harness deploy reported errors`. Resolve by making
  each `move` best-effort (count failures, keep going) and by not gating `install` on the helper's exit status
  (`migrate || log 'WARN: migration incomplete'` on its own line, then `install`).
- **[low / confidence: high]** `image/configs/harness-migrate.sh:82` + `image/entrypoint.sh:129` — user-chosen
  filenames reach the container log verbatim. The `.bak.*` globs (`:68-72`) enumerate names the user creates,
  `log "moved ${rel}"` prints them unescaped, and unlike the `install` line the helper's stdout is **not**
  redirected to `/dev/null`. Filenames may contain newlines: a file named
  `evil\n[styrman] project <uuid> ready (user=root uid=0).bak.z` produces a forged `[styrman] … ready` line in
  the microVM console — verified in a scratch home — which `GET /api/projects/{id}/logs`
  (`internal/httpapi/stats.go:126`, owner-scoped) returns verbatim to the ops-panel dialog. No XSS (React
  renders it as text in `<pre>`, `web/src/routes/index.tsx:197`) and the guest already owns its own log stream,
  so no boundary is crossed — but it corrupts the one boot-forensics view an operator has, for free. Resolve
  with `>/dev/null` on the helper call (matching the install line) or `printf '%q'` on `${rel}`.
- **[low / confidence: high]** `image/harness/internal/harness/apply.go:55-72` (same pattern
  `Uninstall`, `:199-213`; `pruneEmptyDirs`, `:261-281`) — manifest entries are trusted verbatim.
  `~/.config/sh-agent/state.json` lives in the user-writable home (and per question 3 a user can pre-create it
  precisely to skip the migration), `LoadState` (`state.go:28`) does no validation, and the resync loop calls
  `os.Remove(filepath.Join(home, rel))`: `filepath.Join` *cleans* `../..`, so an entry like
  `../../../etc/whatever` resolves outside the home, after which `pruneEmptyDirs` walks **upward** deleting
  empty directories, bounded only by `len(dir) > len(home)`. Not exploitable across a boundary in this
  deployment — it runs as the project uid inside a VM where that uid already has root, and the only
  host-persistent paths are that project's own home and workspace — but it is a latent arbitrary-unlink
  primitive in a tool the operator also runs on their own machine. Resolve by rejecting any manifest entry that
  is absolute or whose cleaned join is not under `home`, and by bounding `pruneEmptyDirs` with an explicit
  `strings.HasPrefix(dir, home+string(os.PathSeparator))`.
- **[low / confidence: high]** `deploy/vulncheck.sh:35` — `for mod in . deploy/caddy/ingress` no longer covers
  the modules that ship: `image/harness` is a third Go module built into the runtime image, and `AGENTS.md`'s
  "govulncheck for both modules" is stale. Scanned by hand: with the host toolchain the module reports two
  **called** stdlib vulnerabilities (GO-2026-4602 `os` Root escape via `inventory.go:71`, GO-2025-3956
  `os/exec.LookPath` via `repo.go:93`) because `image/harness/go.mod` declares `go 1.24.4`; re-run inside the
  pinned `golang:1.25.13-trixie` builder with `GOTOOLCHAIN=local`, exactly as the shipped binary is built, it
  is `No vulnerabilities found` (exit 0). So the artifact is clean today, but nothing in the repo asserts it,
  and a directive or base-image drift would ship unnoticed. Resolve by adding `image/harness` to the loop.
- **[low / confidence: medium]** `image/configs/harness-migrate.sh:76-84` — intermediate path components are
  followed. The leaf is safe (`mv -T` on a symlink renames the *link*, verified: `~/.claude/skills -> ~/other`
  moved the link, the target was untouched), but a symlinked **parent** is not: with `~/.claude -> /elsewhere`
  the helper moves `/elsewhere/CLAUDE.md` into the backup, and with `~/.styrman -> /elsewhere` both
  `mkdir -p "$(dirname "$dst")"` and, via `image/entrypoint.sh:128,131`, the backup tree and the
  `.harness-version` stamp land at the link target — verified in a scratch home. Worst case, stated precisely:
  everything runs as the project uid, and the only host-persistent paths reachable inside the microVM are that
  project's own home and its own workspace bind (`internal/orchestrator/orchestrator.go:782-789`), so no other
  user's, project's, or host path is touchable; the outcomes are self-harm (own files relocated into the
  backup), a stamp written somewhere ephemeral so the migration repeats every boot, or an `mv` failure into the
  finding above. Resolve, if wanted, by refusing to run when `~/.claude`, `~/.gemini`, `~/.agents` or
  `~/.styrman` is a symlink (`[ -L "${HOME_DIR}/.styrman" ] && { log 'WARN: …'; exit 0; }`).

## Checked

- **Secrets (R10)** — clean. The move inventory (`harness-migrate.sh:48-72`) contains no credential path; the
  `.bak.*` globs use `nullglob` only (no `dotglob`), so a leading-dot name such as
  `.claude/.credentials.json.bak.*` cannot match — confirmed by test; `.gemini/antigravity-cli/` (agy's OAuth
  token), `~/.ssh`, `~/.gitconfig`, `~/.config/gh` and `~/.bash_secrets` are all out of scope of both the
  helper and `sh-agent install`. Nothing reads or logs file *contents* — only relative names. `git rev-parse`
  performs no transport, so the system credential helper (`image/Dockerfile:124`,
  `/usr/local/bin/styrman-git-credential`) is never invoked and no brokered credential is fetched or logged at
  boot. The `cut -c1-7 /styrman/harness/COMMIT` in the log line (`entrypoint.sh:132`) reads image content
  baked in by `COPY harness/`, not a bind mount — no user-controlled input. Pattern scan of the new files and
  the vendored tree for key material: only prose about secret-scanning in `skills/preflight/SKILL.md`.
- **Injection** — no shell injection in the block. `USERNAME` (hence `HOME_DIR`, `HARNESS_STAMP` and the
  `sh -c "printf … > '…'"` at `entrypoint.sh:131`) is validated server-side against
  `^[a-z][a-z0-9_-]{0,31}$` (`internal/httpapi/profile.go:28`) before it becomes `STYRMAN_USER`;
  `HARNESS_VER` comes from `/styrman/VERSION` (build arg). Every expansion in `harness-migrate.sh` is quoted;
  the one unquoted expansion (`"${HOME_DIR}"/${g}`, `:91`) is the intended glob. The parsing boundary that
  *is* user-controlled — filenames and `~/.gitconfig` — is covered by the findings above.
- **Authz/authn** — no endpoint, middleware or identity path changed. The artifacts this block writes are read
  host-side by `rosterHome`/`toolDetails` (`internal/httpapi/setup.go:189-273`) through `internal/files`
  helpers built on `os.Root` (`ListNames`/`Exists`/`IsRegular`), which treat escaping symlinks as
  non-existent — so a symlink planted in the home cannot steer the server into reading outside it; the logs
  route is owner-scoped (`stats.go:126`). A user pre-creating `state.json` gains only the ability to skip their
  own migration and to choose which of their own homes the roster is read from — no cross-user effect.
- **Crypto** — none touched; nothing hand-rolled in the diff (no auth, hashing, sessions, or token handling).
- **ADR-047 / PATH (question 4)** — safe. `sh-agent install` executes exactly one external program, `git`
  (`apply.go:75` → `repo.go:105`); `diff` (`apply.go:173`), `go` and `git fetch/pull` (`repo.go:66-102`) are on
  the `diff`/`update` paths, which boot never takes — so nothing network-bound runs. `runuser` hands the child
  a root-owned PATH (container default or `login.defs` `ENV_PATH`); the user's `~/.local/bin` — where
  `install` deposits the `sh-agent` copy (`plan.go:17,124`) — is never on it. Of git's user-controlled config,
  nothing executes for `rev-parse`: aliases cannot shadow builtins, no hook or `core.hooksPath` fires, the
  pager is skipped for a non-tty pipe (`cmd.Output()`), and `core.fsmonitor` is only consulted on an index
  refresh, which `rev-parse` does not do — but note it *would* fire for `git status`, so `sh-agent status`
  must stay off the boot path. `/styrman/harness` ships without a `.git` (verified), so `rev-parse` fails
  silently and `state.json`'s `commit` field is empty — provenance exists only in the log line; consider having
  `sh-agent` fall back to the repo's `COMMIT` file.
- **Failure atomicity (question 3)** — the resulting home is safe (move-never-delete; nothing is destroyed on
  any failure path) and the next boot's predicate is correct: a failed `install` leaves the old stamp, so the
  entrypoint retries, and the helper's predicate (stamp present, manifest absent) re-runs as a no-op because
  the paths are already gone and no backup dir is created when nothing moves. What is *not* correct is
  liveness — see the two findings above: a repeatable helper failure retries forever with the same result.
- **Dependencies** — `image/harness/go.mod` requires nothing (stdlib only, no `go.sum`), which satisfies the
  stdlib-first rule. `govulncheck` run twice, results in the findings section: exit 3 with the host go1.25.0
  against the module's `go 1.24.4` directive, exit 0 (`No vulnerabilities found`) inside the pinned
  `golang:1.25.13-trixie` builder that actually ships it. Root and ingress modules not re-scanned — untouched
  by this diff and covered by `deploy/vulncheck.sh` at the release gate. `npm audit` not run: the diff contains
  no JS/TS change and `web/` is outside its blast radius.
- **GCP surface** — none. Self-hosted deployment; no IAM binding, service account, key file, or Cloud Run
  surface is touched by this diff (`.agents/docs/deploy/gcp.md`: not applicable).

---

## Round 2 (2026-09-03)

Range reviewed: `c1a6eb9..working tree` (HEAD unchanged since round 1) — the rewritten
harness block `image/entrypoint.sh:126-152` and `image/configs/harness-migrate.sh`, plus
the installer paths the new flags reach (`internal/harness/apply.go`, `state.go`,
`repo.go`) and the caller's boot budget (`internal/orchestrator/orchestrator.go:294,587,818`).

### Verdict: FINDINGS (medium closed; 3 lows closed, 1 still open, 1 open-deferred, 2 new lows)

The medium is properly dead, and I verified the mechanism rather than the claim: the
`~/.gitconfig` hang reproduces without the fix and vanishes with it. What replaces it is
smaller and bounded — the bound itself is just set higher than the caller's deadline.

### Per finding

1. **[medium] no time bound; `git rev-parse` honours the user's `~/.gitconfig` — CLOSED.**
   Both mechanisms verified end to end. With a scratch home whose `.gitconfig` has
   `[include] path = <fifo>`, `git -C image/harness rev-parse --short HEAD` hung until
   killed (`timeout 5` → **exit 124**); with
   `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` it returned immediately (exit 0).
   `timeout` reaches the grandchild: coreutils `timeout` without `--foreground` signals
   the whole process group, confirmed locally with a backgrounded child, and both
   binaries exist in the shipped image. The block's new shape (`entrypoint.sh:139-149`)
   is correct — migrate's status captured, install unconditional, stamp only when both
   succeeded, two distinct WARN lines — and `entrypoint.sh:6` is `set -uo pipefail`
   (no `-e`), so the `|| HM_OK=0` guards are belt-and-braces rather than load-bearing.
   - **Half-written home after a timeout: safe, and self-repairing.** `Install`
     (`apply.go:19-89`) writes every file through `writeAtomic` (`:228-256`: temp file in
     the destination directory, `Chmod`, `Close`, `Rename`) so no file is ever visible
     half-written, and it calls `SaveState` **last** (`:74-81`), which is itself a
     `writeAtomic` (`state.go:43-49`). A killed install therefore leaves the *old* or *no*
     manifest, never a truncated one; the next boot's migrate predicate (stamp present,
     manifest absent) is still true, it moves nothing, and install re-runs and writes
     exactly the files that are missing (`bytes.Equal` skips the rest). Nothing is deleted
     on any failure path — the resync loop only removes paths a *previous* manifest listed.
   - **`GIT_CONFIG_GLOBAL=/dev/null` side effects on installed files: none.** Install's
     only git call is the manifest's `commit` (`apply.go:75` → `repo.go:105`), whose error
     is discarded and which is already empty against a `.git`-less mirror; no file content,
     mode, or path depends on git config. `/dev/null` is git's own documented idiom for
     "no global config". `GIT_CONFIG_NOSYSTEM=1` additionally hides
     `/etc/gitconfig`'s `credential.helper` (`image/Dockerfile:124`) from that call, which
     is the right direction: a future transport would fail closed rather than fetch a
     brokered credential at boot. Neither variable persists into the user's shell — they
     are arguments to the one `env` invocation.
   - **NEW [low / confidence: high]** `image/entrypoint.sh:140,142` vs
     `internal/orchestrator/orchestrator.go:294,587` — the bound is above the caller's
     deadline. Two sequential `timeout 60` calls put up to **120 s** of user-influenced
     work ahead of `touch /run/styrman/ready` (`:283`), while `waitReady` gives the whole
     boot **90 s** and R1/ADR-062 D6 assert a 15 s boot SLA. A home shaped to make both
     steps hit their limit (a large planted tree under `~/.claude/skills` turning `mv -T`
     into a cross-device copy, then a slow install into it) still fails the owner's own
     project start — self-inflicted and single-project, as before, but no longer
     *unbounded* only in the sense that it now fails deterministically instead of hanging.
     Resolve: 15–20 s each, which is ~1000× the measured 0.02 s install and keeps the
     worst case inside both budgets.
2. **[low] `set -e` aborts the inventory on one unmovable path, install then skipped — CLOSED.**
   Re-ran the round-1 scenario in a scratch home (`chmod 500 ~/.claude`): the helper now
   emits four `WARN: could not move … — left in place` lines, moves the three reachable
   `.gemini`/`.agents` paths, prints `moved 3 path(s) …; 4 left in place`, and exits 1 —
   and the entrypoint installs anyway and withholds the stamp. Exactly the shape I asked for.
   - **NEW [low / confidence: medium]** `image/harness/internal/harness/apply.go:41-43` —
     `Install` returns on the **first** write error, so in that same scenario every file
     planned after the unwritable `~/.claude/…` entry is skipped, and each boot repeats the
     identical partial result with the same two WARNs. Nothing is destroyed
     (move-never-delete holds) and the stamp is correctly withheld, so this is liveness and
     operator-signal, not integrity — but "retried forever with the same result" survives
     the fix in a narrower form. Upstream: collect write errors and continue, so a single
     unwritable directory cannot withhold the other 150 files.
3. **[low] unsanitized names reach the container log — CLOSED.** `plain()`
   (`harness-migrate.sh:76-78`) strips `\000-\037` and `\177` from every logged name.
   Verified with a file named `evil\n[styrman] project deadbeef ready (user=root uid=0)\033[31mX.bak.z`:
   the newline and the ESC are gone and the output is a single `[harness-migrate] moved …`
   line, so neither a forged `[styrman] … ready` line nor an ANSI sequence reaches the
   ops-panel dialog. Residual `[31m` is literal text in a `<pre>` — no capability.
4. **[low] manifest entries trusted verbatim in `apply.go` — rebuttal ACCEPTED (upstream),
   STILL OPEN as a record.** Correct that styrman does not edit the mirror. But the item is
   in neither `TODO.md` (whose only change this session ticks the settings-roster entry) nor
   `image/harness/TODO.md` / the upstream checkout's `TODO.md` at `COMMIT` — so "recorded
   for upstream" is an intention, not a fact in any tree I can read. Same for phase 1's
   findings 3, 4 and 10.
5. **[low] `deploy/vulncheck.sh` module list — CLOSED.** `:35` now reads
   `for mod in . deploy/caddy/ingress image/harness`. Ran the script in full:
   `image/harness` → **No vulnerabilities found**; `.` and `deploy/caddy/ingress` → the
   accepted docker/docker GO-2026-4887 / GO-2026-4883; exit 3 per its documented semantics.
6. **[low/medium] symlinked parents in the migrate inventory — STILL OPEN, unchanged.**
   No guard was added: `harness-migrate.sh:84`'s `[ -L "${src}" ]` is the leaf-existence
   test, not a refusal, and nothing checks whether `~/.claude`, `~/.gemini`, `~/.agents` or
   `~/.styrman` is itself a symlink. Round-1 reasoning stands verbatim — self-harm only,
   bounded by the project's own two bind mounts — and it is the least important item in
   either report. The one-line refusal from round 1 still applies if wanted.

### Checked (round 2)

- **Secrets** — the new flags reduce exposure rather than add it: `GIT_CONFIG_NOSYSTEM=1`
  keeps the image's git credential helper out of the install call. No credential path
  entered the inventory, nothing logs file contents, and `plain()` now bounds what a
  filename can put on the console.
- **Injection** — no new shell string reaches an interpreter. `HARNESS_VER` is still
  interpolated into the single-quoted `sh -c "printf … > '…'"` stamp write
  (`entrypoint.sh:144`), but its source is `/styrman/VERSION`, a build-time arg, not
  tenant input; `USERNAME` remains server-validated (`internal/httpapi/profile.go:28`).
  Every expansion in the helper stays quoted apart from the intended glob.
- **Authz/authn** — unchanged; no endpoint, middleware, or identity path in this delta.
- **Crypto** — none touched; nothing hand-rolled.
- **Dependencies** — `deploy/vulncheck.sh` output above; `image/harness/go.mod` still
  requires nothing. `npm audit` not applicable — no JS/TS in this delta.
- **GCP surface** — none (self-hosted).
