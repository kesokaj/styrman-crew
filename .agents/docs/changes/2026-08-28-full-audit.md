# Full-project audit — findings + fixes (v1.5.x)

Date: 2026-08-28. `/audit` over the whole repo: three parallel read-only
audit passes (Go backend · frontend · scripts/deploy/docs) + cross-cutting
verification (route-vs-spec diff, `deadcode`, shellcheck, README coverage,
secrets scan, `go mod tidy`, `npm audit`). Everything below was verified
before being claimed; every 🔴 and the cheap 🟡s were FIXED in this pass,
the structural 🟡s were logged in `TODO.md` ("Deferred from the 2026-08-28
full audit").

## Summary

| Severity | Found | Fixed now | Deferred (TODO.md) |
|---|---|---|---|
| 🔴 bug / broken / contradicts reality | 14 code + 17 docs | **all** | 0 |
| 🟡 should fix | ~44 | 28 | 16 |
| 🟢 nit | ~60 | 24 | rest noted |

Clean (verified, no findings): `internal/identity`; the credential
git-helper + materialization snippet (quoting/paths); TerminalPane's WS
lifecycle; API payload types vs Go handlers (14/14 interfaces);
no `any`/`@ts-ignore` in `web/src`; env-var hygiene (no dead `STYRMAN_*`);
secrets hygiene (nothing embedded, `.env` ignored); `npm audit` prod = 0;
`go mod tidy` = no drift; `go vet` + `go test -race` + `deadcode` = clean
after fixes.

## 🔴 fixed — Go

| File | Issue | Fix |
|---|---|---|
| `internal/httpapi/explorer.go` | **managed-subtree guard missing on write/upload/mkdir**: `PUT home/file?path=.dockerd/<uuid>.img` could O_TRUNC a RUNNING VM's loop-mounted docker disk | guard applied to write, mkdir, and per-part upload; regression tests |
| `internal/orchestrator/orchestrator.go` | `stopProject` ran outside the per-project lock — a stop racing a terminal attach could power the VM off mid-`EnsureRunning` | `Stop` takes the lock; `stopLocked` for the reaper |
| `internal/httpapi/server.go` | createProject's background boot skipped the `deletingState` guard — delete-in-window could resurrect an orphan microVM | re-check before `EnsureRunning` |
| `internal/files/files.go` | `Write`/`SaveStream` could block forever on a guest-planted FIFO (write-only open waits for a reader) | `openRegWrite`: O_NONBLOCK + regular-file check + truncate-after-check; FIFO regression tests |
| `internal/history/history.go` | `Read` used one unchecked `f.Read` — silently truncated transcripts, I/O errors invisible | `io.ReadAll(io.LimitReader)` + error |

## 🔴 fixed — frontend

| File | Issue | Fix |
|---|---|---|
| `routes/settings.tsx`, `routes/index.tsx`, `components/Sidebar.tsx`, `SetupBanner.tsx` | failed loads rendered "Loading…" forever / blank dashboard / lying "No projects yet" / vanished banner | error states surfaced at all four |
| `lib/hooks.ts` | `refresh()` silently dropped while a poll was in flight — post-mutation refreshes lost | queued `pendingRefresh`, re-fired on settle |
| `components/FileExplorer.tsx` | stale listing could repaint the previous directory under the new breadcrumb; failed micromark chunk stuck "Rendering…" forever | generation guard; `.catch` → falls back to editor |
| `components/SidePanel.tsx` | two quick transcript opens could render A's text under B's title | generation guard (mirrors `runSearch`) |

## 🔴 fixed — scripts / deploy / docs

| File | Issue | Fix |
|---|---|---|
| `.agents/docs/testing/styrman/integration.sh` | **the EXIT trap's `[[ FAIL -eq 0 ]]` never became the exit status — failing runs exited 0** (the 62-check gate was unenforceable); eval-quoting broke `${CN}` out of the command strings | explicit `exit` in the trap (semantics proven both ways); quoting fixed |
| `deploy/home-init.sh` + host-setup step 8 | the shared-home initializer actively contradicted ADR-031 (seeded a home no project mounts, printed an unusable ssh key, referenced the deleted `internal/dotfiles`) | **deleted**; host-setup creates the current layout (`homes/`, `projects/`), chowns non-recursively |
| `deploy/host-setup.sh` | runtime image built without `STYRMAN_VERSION` → `/styrman/VERSION` stuck at 0.0.0 → harness never re-seeded into existing homes | version-stamped build (git describe) |
| `image/configs/styrman-setup` | told users sign-ins "cover all projects forever" via a shared home that no longer exists | rewritten: per-project home + Settings → Credentials → Import |
| `image/entrypoint.sh` | MOTD profile.d snippet sorted BEFORE credential materialization → nagged "run styrman-setup" on fully-onboarded projects | MOTD renamed `zzz-` (after `zz-` credentials) |
| `web/README.md` | untouched Vite scaffold boilerplate | rewritten as a real subproject README; `image/` + `deploy/` got pointer READMEs |
| `.agents/docs/testing/styrman/README.md` | pointed at the removed `/dev/` route; claimed shared-home sign-in and a removed link-rescue bar | corrected |
| docs (`guide.md` ×5, `AGENTS.md` ×3, `terminal-protocol.md`, `runtime-image.md`, `api.md`) | still described the pre-ADR-031 world: `homes/<user>` as `$HOME`, wrong docker-disk/transcript paths, `/api/home/*`, stranded table row, missing broker/env rows, R1–R12 | all rewritten to per-project homes + broker; package lists + env tables completed |

## 🟡 fixed (selection)

Go: proxy 502 no longer leaks guest IP:port to link visitors; createProject
no longer returns raw git output (host paths); `redirectSlash` preserves the
query string; profile store persists-first with rollback; userstore save
fsyncs (R8 source of truth); links store validates tokens on load and
`DeleteProject` rolls back on failed save; project UUID-migration save
failure is loud; `CleanupLegacy` logs failures and re-lists before the
network sweep (TOCTOU); `IsRepo`/`EnsureRepo`/`List` take ctx (no more
`context.Background()` in request paths); history `List`/`Search` return
real errors (NotExist = empty); rune-safe truncation (`clampTitle`,
`GitDiff`); `gofmt` whole-tree clean (the `autostart.go` doc comment was
reworded first — `gofmt` would have corrupted `''` into a Unicode quote).

Frontend: `tsconfig` now `strict` + `noUncheckedIndexedAccess` (0 errors);
dead code removed (~120 lines: `sniffChunk`/`extractUrls` URL machinery from
the removed v1.2 link bar, `ExternalLinkIcon`, `archived` field end-to-end,
private `TermTarget`/`Toast`); shared `projectNameError` (rename dialog now
validates like create); `copyToClipboard` helper with toast feedback at the
4 silent sites; terminal rename failures surface via `tryUI`; NewProject
partial success (project created, terminal failed) no longer dead-ends;
setup error phase got a Try-again.

Deploy/image: `.env.example` ships an empty `STYRMAN_KEYRING_PW` (the `:?`
guard now fires instead of deploying the literal); compose documents
`STYRMAN_BROKER_ADDR` + never-publish rationale, drops the phantom
`STYRMAN_PREVIEW_DOMAIN` fallback; caddy Dockerfile's module "verification"
can actually fail now (`|| true` dropped); e2e cleanup also removes stopped
leftovers; skel `grep` alias moved to `bash_aliases`; stale "shared home"
comments swept.

Go dead code removed (deadcode-verified): `config.ProjectsFile`,
`config.Agents`, `LabelAgent`, `history.LogsDir`, `project.SetArchived` +
`Archived`, checkpoints' always-nil `workspacePath` error, exec's hand-rolled
`io.ReadWriter`, gitrepo's no-op `os.Stat`.

## Deferred

See `TODO.md` § "Deferred from the 2026-08-28 full audit" (Dialog
unification + focus trap, keyboard-reachable rows, SidePanel split,
remaining error-surface call sites, central 401 handling, homeDir/tools
semantics post-ADR-031, small idiom/dedupe items, OpenTofu `--skip-verify`,
compose env narrowing, React #418, `AgentSession`).

**REQUIREMENTS.md** (approval-gated, NOT edited): the audit flagged three
wording drifts for the operator to approve — R3 says "agy via keyring
seeding" but agy is reported *unsupported* (issue #632); R3 lists `docker`
as brokered but no docker adapter ships yet; R12 says `sdb-<name>` but
networks are `sdb-<uuid>`.

## Verification
`go vet` + `go test -race ./...` clean · `deadcode` zero · gofmt whole-tree
clean · `tsc --noEmit` (strict) 0 · `vite build` OK · shellcheck: warnings
resolved (one intentional `~/` case-pattern note) · integration 62/62 +
exit-code semantics proven · e2e 35/35 (post-fix runs recorded below).
