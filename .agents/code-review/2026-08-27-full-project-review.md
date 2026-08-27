# styrman — full-project code review

**Date:** 2026-08-27
**Scope:** entire project, including uncommitted work (post-commit `12e42d8`).
**Method:** four parallel read-only reviewers — Go core (orchestrator/terminal/conversation/main), Go API+security (httpapi/files/gitrepo/history/project/identity/userstore/config), frontend (`web/`), image+deploy. No code was changed.
**Headline confirmed live:** the server container is joined to a project bridge (`172.19.0.3` on `sdb-<uuid>`) with the no-auth admin listener bound to `0.0.0.0:8081` — three reviewers found this independently.

Severity: 🔴 blocking · 🟠 important · 🟡 frontend/UX · 🟢 nit/optimization.
IDs (B/I/F/N) are stable references used by the remediation plan.

---

## 🔴 Blocking — security (cross-tenant / host-root)

### B1 — API listeners reachable from project microVMs → unauthenticated admin
- **Where:** `deploy/docker-compose.yml:46,54` (`STYRMAN_ADDR=0.0.0.0:8080`, `STYRMAN_DEV_ADDR=0.0.0.0:8081`); `cmd/styrman/main.go:82-102`; `internal/orchestrator/orchestrator.go` `EnsureSelfAttached`; `internal/httpapi/auth.go:43-46`.
- **Issue:** both listeners bind all interfaces *inside* the server container, which joins every project's `sdb-<uuid>` bridge for the port-preview proxy. Any code in a project VM reaches `http://<server-bridge-ip>:8081/…` → `auth=none` → identity = `DefaultUser()` = first allowed email with `Admin:true`. Port 8080 alternatively trusts a spoofed `X-Caddy-Email` (ingress overwrites the header, but this path never touches the ingress). The `127.0.0.1:8080:8081` publish only constrains the host path; `ip_forward=0` is irrelevant (connection *to* the server, not routed *through*).
- **Exploit:** verified live: server is `172.19.0.3` on the project bridge; reviewers ran from inside a project VM: `curl …:8081/api/projects` → full list; `…/api/home/file?path=.ssh/id_ed25519` → admin's private key; `…/api/settings` → `isAdmin:true`; can add itself to userstore, delete other users' projects.
- **Who:** any code running in any project VM (every agent / cloned repo build script).
- **Fix:** remove `STYRMAN_DEV_ADDR` from prod compose; enforce loopback-only for the dev listener in `config.Load()` (not a comment); bind the main listener to the ingress network IP (keep the server off project bridges, or dial guests via an explicit bound `net.Dialer`); reject `header`-mode requests whose `RemoteAddr` is not the ingress; consider a per-deploy ingress shared-secret header.

### B2 — Project file explorer not symlink-contained → cross-tenant read/write
- **Where:** `internal/httpapi/explorer.go:23-36` (`rootFor`) feeding `/files`, `/file`, `/upload`, `/download`.
- **Issue:** `rootFor` returns `filepath.Join(UserHome, "projects", p.Dir)` raw. `files.Resolve` then treats whatever the root resolves to as `realRoot`, so a symlinked root escapes containment. Guest runs as the same uid; `~` is bind-mounted; the server resolves symlinks in the host namespace where all of `/var/lib/styrman` is mounted.
- **Exploit:** in own VM `mv ~/projects/foo ~/projects/foo.real && ln -s /var/lib/styrman/homes/victim ~/projects/foo` → read/write victim's home (SSH keys, creds). Point at `/var/lib/styrman` → `PUT users.json` to self-promote to admin, or rewrite any tenant's stores.
- **Fix:** `rootFor` must return `files.Resolve(UserHome, "projects/"+p.Dir)` (the guard `checkpoints.go` already uses).

### B3 — Setup tool-details raw file read → SSH private key leak
- **Where:** `internal/httpapi/setup.go:142-209` (`toolDetails`), `:28-44` (`userSetupStatus`).
- **Issue:** `read(path)=os.ReadFile(filepath.Join(home,path))` with no containment; `.ssh/id_ed25519.pub` returned verbatim as `ssh.Details`.
- **Exploit:** `ln -sf /var/lib/styrman/homes/victim/.ssh/id_ed25519 ~/.ssh/id_ed25519.pub` → `GET /api/settings/tools` returns the victim's **private** key. Same for `~/.bash_secrets`, `homes/victim/.claude/.credentials.json`, `users.json`; `listNames` leaks arbitrary directory listings.
- **Fix:** route all home-relative reads through `files.Read`/`os.OpenRoot`; return a fingerprint instead of the raw pubkey.

### B4 — Transcript history not contained → arbitrary file read
- **Where:** `internal/history/history.go:20-99` via `internal/httpapi/explorer.go:153-180`.
- **Issue:** `LogsDir` uncontained; `id` regex validates the name, not the target; `~/.styrman/logs/` is guest-writable.
- **Exploit:** `ln -s /var/lib/styrman/homes/victim/.bash_secrets ~/.styrman/logs/<own-uuid>/leak.log` → `GET /api/projects/<own>/history/leak` returns contents.
- **Fix:** resolve `LogsDir` through the guard; root-confined open.

### B5 — Setup-done marker follows symlinks → arbitrary file overwrite
- **Where:** `internal/httpapi/setup.go:79-87` (`finishSetup`); `internal/httpapi/server.go:160-171` (`writeOnce`).
- **Issue:** `os.WriteFile` follows a guest-planted symlink.
- **Exploit:** symlink the marker at `homes/victim/.ssh/id_ed25519` (or `users.json`) → `POST /api/setup/done` truncates it to a timestamp. Destructive cross-tenant write.
- **Fix:** `O_NOFOLLOW` / root-confined open.

> **B2–B5 share one fix:** Go 1.25 `os.OpenRoot(root)` + `root.Open/Create/OpenFile` for all home-relative I/O; also closes the `files.Resolve` TOCTOU (check-then-use, winnable by a guest looping `rename()`). `internal/files/` has **no tests** — add escape-case tests (`../`, absolute, symlinked leaf/intermediate/root, non-existent leaf under symlinked parent).

### B6 — Secrets in build layer + weak keyring defaults
- **Where:** no root `.dockerignore`; `deploy/Dockerfile.server:9` `COPY . .`; `deploy/.env.example:33`; `image/entrypoint.sh:74`; `internal/orchestrator/orchestrator.go:340`.
- **Issue:** `deploy/.env` (Google secret, Cloudflare token, keyring pw) is baked into the **builder layer** (final image is clean, but leaks via `--target builder`, `docker save`, BuildKit cache export). `STYRMAN_KEYRING_PW` defaults to literal `styrman`. The entrypoint `unset` only clears the shell copy — the password stays in `container.Config.Env`, visible in `docker inspect` and every `docker exec` (every user terminal).
- **Fix:** root `.dockerignore` (`.git`, `**/.env`, `*.env`, `web/node_modules`, `*.png`, `.agents`); generate keyring pw in `host-setup.sh` (`openssl rand`), drop the `:-styrman` default; pass keyring pw via file/stdin, not env.

---

## 🟠 Important — reliability / correctness

| ID | Where | Issue | Hits solo operator? |
|----|-------|-------|---------------------|
| I1 | `orchestrator.go` `CleanupLegacy` (`~:298-307`) | `strings.HasPrefix(n.Name,"sdb-")` sweep also deletes **current** project/setup networks; a stopped project's network is removed on restart → `EnsureRunning`/`EnsureSetup` fail "network not found" forever, no recreate path (verified: `docker network rm` of a stopped container's net succeeds, then `start` fails). Narrow race: sweep can delete a net between `create()`'s NetworkCreate and ContainerCreate. Violates R7. | **Yes** — reboot bricks idle-stopped projects |
| I2 | `orchestrator.go` idle reaper (`~:401,477-497`) | `Stop` without the per-project lock or re-checking activity → serial reaper (30 s each) can kill a terminal that attached minutes after `Adopt`; tmux state lost | **Yes** |
| I3 | `orchestrator.go:214-233,363` | Setup VM has no `styrman.project` label (reaper never sees it) and `RestartPolicy: unless-stopped` → abandoned onboarding leaks a Kata VM that returns every reboot (one running now) | **Yes** |
| I4 | `internal/httpapi/profile.go:16-63,120` | Data race on `profileStore.GuestName`: `setGuestUser` writes while `GuestUser`/`usernameChosen` read (WS attach + `GET /api/settings` + `POST profile`). Torn string = memory-unsafe. `go test -race` doesn't exercise it | Yes (rare) |
| I5 | `internal/terminal/bridge.go:181-185` | "One active connection" is `LoadAndDelete`+`Store` (non-atomic) → two near-simultaneous attaches both attach, `-D` kicks one → StatusNormalClosure → frontend auto-reconnect spiral. Also `old.Close(4001)` runs synchronously on the new handler and blocks ~5-10 s on a dead peer (coder/websocket close handshake waits on `readMu`) | **Yes** — reopen hang |
| I6 | `cmd/styrman/main.go:68-77` | Boot `Adopt` + serial `EnsureSelfAttached` (30 s timeout, 500 ms sleep each) runs **before** `ListenAndServe` despite "non-blocking" comment (only `CleanupLegacy` is) → `/api/health` delayed minutes after a reboot → ingress 502 | **Yes** |
| I7 | `internal/orchestrator/exec.go:43` | `stdcopy.StdCopy` ignores `ctx`; Docker SDK hijack has no ctx watchdog → a hung guest command leaks a goroutine + hijacked conn permanently → fd exhaustion (polled paths) | Yes (slow) |
| I8 | `internal/orchestrator/ports.go:50-61` | Port scan is O(sockets × fds) with a `readlink` subprocess per fd (~190 ms/port measured), polled every 7 s with no in-flight guard → busy VM → 15-20 s scans pile up, each holding a hijacked exec conn | **Yes** — use `ss -ltnpH` |
| I9 | `image/entrypoint.sh:187` | `exec sleep infinity` = no SIGTERM handling; on `docker stop` dockerd is killed mid-write and the loop-mounted ext4 never unmounted → dirty journal every restart; nested containers SIGKILL'd with no grace | **Yes** |
| I10 | `image/entrypoint.sh:79-81,105-107` | If the `.tmux.conf` `cp` fails (home not yet writable), the later `echo >>` **creates it as root** with only the source line → user loses `mouse off` + `default-command bash -l` (keyring/agy break) and can't edit without sudo; re-run won't heal (`[ ! -f ]` now false) | **Yes** (latent) |
| I11 | `internal/httpapi/profile.go:51`, `web/src/routes/setup.tsx:39`, `image/entrypoint.sh:44-47` | Reserved names (`root`,`daemon`,`bin`,`nobody`,`docker`) pass `guestNameRe`; `useradd` skipped, guest runs as the reserved uid on a 1000-owned home; the line-56 self-heal chowns to 1000 while shell is uid 1 → every write fails, no diagnostic; `usermod -aG docker root` | **Yes** if picked |
| I12 | `image/entrypoint.sh:137-146` | vfs fallback silently swaps `data-root` → all prior images/containers/volumes vanish from view; only a swallowed WARN; ready marker still set; abandoned 20 GB `.img` keeps consuming home | Yes (on failure) |
| I13 | `deploy/host-setup.sh:53-55,45`, `image/Dockerfile:37-51` | Kata tarball extracted into `/` as root with no checksum/signature; `get-helm-3`, OpenTofu (`--skip-verify`), nodesource, `antigravity` installer, `get.docker.com|sh`, `yq …/latest` all unverified — supply chain | Build-time |
| I14 | `deploy/docker-compose.yml:17` | `docker.sock:ro` does **not** restrict the API (RO applies to files, not socket `connect()`); Caddy (internet-facing, host-network) has full read-write Docker = host root. Docs repeat the false "read-only" claim | Yes |
| I15 | `deploy/host-setup.sh:141` vs `deploy.md:39-47`; `home-init.sh:32-47` vs `server.go:144-172` | Two fresh-install paths contradict; only user #1 gets the harness (`ensureHome` duplicates dotfiles in Go, omits harness/SSH/gitconfig; `.bash_secrets` already drifted). R11/R5 don't converge; `host-setup.sh` uses `python3` unguarded | On expand |
| I16 | `internal/orchestrator/orchestrator.go:203` | `ContainerRemove` error discarded when recreating a setup VM after a username change → if force-remove fails, `create` then fails "name already in use", misleading 500, no self-heal | Yes (rare) |

---

## 🟡 Frontend / UX

| ID | Where | Issue |
|----|-------|-------|
| F1 | `web/e2e/run.mjs:38` | Pre-clean `DELETE …/e2e-ui?wipe=true` deletes by name but store is UUID-keyed → 404 no-op → aborted run leaves the project → next Create 409s → whole suite cascades to FAIL (self-poisoning) |
| F2 | `web/src/components/FileExplorer.tsx:256` | Backdrop `onClick={onClose}` discards unsaved CodeMirror edits with no guard (`dirty` computed, unused for this); ✕ button same |
| F3 | `web/src/lib/clips.ts:37` | Copy-on-select persists **OAuth codes/tokens** to `localStorage` (30 entries, no TTL, survives sign-out) during gh/claude/keyring flows. Needs an explicit decision (recommend `sessionStorage`/in-memory + short TTL) recorded in DECISIONS.md (R10). No XSS vector found |
| F4 | `web/src/components/SidePanel.tsx:240` | Git tab `if(!data) "Loading…"` ignores the diff error `usePolled` returns (contrast `cpError` rendered at :269) → stopped project / git failure → "Loading…" forever |
| F5 | `web/src/components/TerminalPane.tsx:171-172` | `ResizeObserver`→`fit.fit()`→resize frame every layout tick, no throttle → window-drag floods `{resize}` frames, each a full in-VM TUI redraw (against <50 ms echo SLA); also send-guard for `cols<10||rows<3` |
| F6 | `web/src/lib/hooks.ts:16`, `api.ts:132` | `usePolled` fires every interval regardless of in-flight, no unmount/ordering guard; `request()` has no timeout/abort → slow VM boot stacks polls; stale response overwrites fresh |
| F7 | across | Four inconsistent error styles (`tryUI`→alert, inline, raw alert, silent `.catch(()=>{})`); `SidePanel.tsx:314,115`, `TerminalPane.tsx:198`, `terminal.ts:93` write clipboard with no catch → silent failures on plain-http LAN; `SetupBanner.markDone` no catch |
| F8 | new overlays/modals | a11y: link chips repeat "Open"/"Copy"/"Dismiss" names (need `aria-label` w/ URL); "Copied" toast no `role=status`/`aria-live`; search-close ✕ no label; `FileExplorer`/`NewProjectDialog`/`DeleteProjectDialog` lack `role=dialog aria-modal`, Escape, focus-trap/restore |
| F9 | docs | Clips, link bar, copy-on-select, search overlay undocumented in `.agents/docs/`; no `changes/2026-08-27-*` entry for this session; `AGENTS.md` says "29 Playwright checks" (now 31) |
| F10 | `web/e2e/run.mjs:135-143` | "both agents installed" waits only for the typed command text, never asserts output `2` → passes even if neither agent exists; no `pageerror`/console listener so uncaught errors/rejections don't fail the suite |

---

## 🟢 Nits & optimizations

- **N1** `terminal.ts:6` — module header claims WebGL/GPU renderer; code deliberately uses DOM renderer (stale doc).
- **N2** `terminal.ts:105,248` — `selTimer` not cleared in `dispose()` → copy fires ~250 ms after teardown; same class: `TerminalPane.tsx` `toastTimer`, `SidePanel.tsx:316` setTimeouts.
- **N3** `terminal.ts:182` — every WS chunk re-scans the full 8 KB tail through 3 regexes + 2 allocs → CPU per frame under `cat`; scan new chunk + small overlap or debounce.
- **N4** `terminal.ts:133` — with copy-on-select, Ctrl+C on a live selection copies instead of SIGINT (2nd press works); consider gating on Ctrl+Shift+C.
- **N5** `TerminalPane.tsx:243` — `dismissed`/`urls`/`copied` never reset on `targetKey`/epoch change → re-printed auth URL never re-surfaces.
- **N6** `TerminalPane.tsx:156` — keystrokes during a reconnect window dropped silently; a small outbound queue flushed on `onopen` fixes it.
- **N7** `TerminalPane.tsx:139` — `onExited` captured at mount (deps `[targetKey]`); latent stale closure (use the `CodeEditor.tsx:110` ref pattern).
- **N8** `SidePanel.tsx:308` — `useState` force-render over a module store risks tearing under React 19; use `useSyncExternalStore`. `key={c.at}` collides on same-ms clips.
- **N9** `clips.ts:16` — `load()` touches `localStorage` at module import → build-time landmine for a prerendered SPA; lazy-init.
- **N10** `setup.tsx:13,115` — `loaded` is dead state; `phase==="error"` is a dead end with no retry.
- **N11** `web/package.json` lint — `oxlint` lints `dist/**`, burying ~10 real `src` warnings (one real: `FileExplorer.tsx:36` set-state-in-effect → `key={project}` remount).
- **N12** `bridge.go:176` — `SetReadLimit(-1)` unbounded frame → OOM on huge paste; cap ~16 MiB.
- **N13** `conversation.go:70,76` — script ends in `:` so exec code always 0; `if code != 0` branch is dead.
- **N14** `conversation.go:45`, `bridge.go:35` — unused `cfg` fields (dead deps).
- **N15** `bridge.go:272` — `touch()` boxes a `time.Time` into `sync.Map` per 32 KB chunk; throttle ~1/s.
- **N16** `exec.go:120`/`bridge.go:318` — `tmux detach` inside guest exits the exec → browser gets 4000 and refuses to reconnect to a live session.
- **N17** `orchestrator.go:384-398` — `waitReady` ~700 docker calls per cold boot (exec+attach+inspect per 500 ms); use one long-lived exec on the marker, or backoff.
- **N18** `orchestrator.go:200` — `insp.Config.Env` deref without nil guard (low-confidence reachable).
- **N19** No tests for `internal/orchestrator` or `internal/terminal` (most concurrency-heavy); `containerGuest` is pure and untested.
- **N20** `explorer.go:9,148` — `Content-Disposition` filename interpolated unescaped (cosmetic; Go neutralizes CRLF); use `mime.FormatMediaType`.
- **N21** `proxy.go:70-76,99` — `proxyPrefix` uses `LastIndex("/<port>/")` (mis-trims paths repeating the port segment); `RawPath` not set (percent-encoded segments re-encoded).
- **N22** `proxy.go:94-104` — reverse proxy forwards the user's `Cookie`/`X-Caddy-*` into the guest and serves previews same-origin with the SPA, no CSP → a previewed page can script the styrman API as the user. (Security-adjacent; grouped with B-fixes.)
- **N23** `server.go:551` — `sp2,_ := s.space(...)` ignored error → nil deref if it ever fails; reuse `sp` from `getProjectFor`.
- **N24** `profile.go:59` — `setGuestUser` write lacks fsync while other stores fsync (inconsistent durability).
- **N25** `server.go:625,643`, `conversation/store.go:132` — terminal titles unbounded (1 MiB body cap only).
- **N26** `history.go:121` — `out,_ := cmd.Output()` swallows git errors → broken repo renders as empty diff.
- **N27** `server.go:158-159` — `os.Chmod` returns ignored on the 0700 home/.ssh hardening.
- **N28** `files.go:84` — `List` `os.Stat`s (follows) symlinks → leaks existence/dir-ness of arbitrary paths; use `Lstat`.
- **N29** Image bloat (`Dockerfile`): build-essential, pip, full gcloud+GKE, Go, Node, helm, kubectl, opentofu, btop+htop, vim+nano; npm cache never cleaned; PEP-668 breaks `pip install`. Unpinned Go/claude/yq → non-reproducible builds.
- **N30** `entrypoint.sh:129` — no free-space check before a 20 GB sparse `.img` → ENOSPC mid-write corrupts overlay2.
- **N31** `Dockerfile.server:21` — healthcheck hits the auth-required :8080 (works only via the `/api/health` exemption; fragile coupling).
- **N32** `styrman-setup:20-25,33-34` — empty git identity accepted as success under non-TTY exec; key-comment migration misparses (`awk $3` → "no" for comment-less keys) and is dead for home-init keys.
- **N33** `entrypoint.sh:21-22,26,89,112` — ready-marker cleared after `:?` checks; resolv.conf truncate-before-read can empty DNS; transcript logdir mkdir unguarded; tmux migration marker unversioned.
- **N34** `docker-compose.yml:36,44` / `host-setup.sh:117,128` — no `${VAR:?}` guards on required env; `DOCKER_GID` can be empty; `chown -R` whole state dir every re-run.

---

## Cleared (verified sound — do not re-litigate)

`go vet` + `go test -race ./...` clean. Confirmed good: `files.Resolve` containment logic itself (for a trusted root); `checkpoints.go`/`gitrepo.go` (guarded path, regex-validated IDs, argv-not-shell, generic errors); `cloneURLRe` + `redactURL`; `gh repo create` name pinned by `ValidateName`; per-user store isolation (`space()`/`getProjectFor`); container binds only the requester's own home; Caddy ingress sets/strips `X-Caddy-Email` (spoofing blocked *through the ingress*); WS same-origin default + record-required attach; atomic+fsync store writes with load-time re-validation; slog JSON (no log injection); WS lifecycle/heartbeat no goroutine leak; `activeConns` CompareAndDelete; XSS in link chips impossible (anchored `https?://`, `rel=noopener`); sniffer decoder/regex correctness + bounded accumulators; CodeMirror lazy-grammar race safe; close-code handling matches spec; no StrictMode spiral; `tsc --noEmit` 0 errors; no committed secrets in reviewed packages.

---

## Verdict

**REQUEST CHANGES.** Six blocking security items (one network-reachable auth bypass confirmed live; four symlink-containment gaps the `workspacePath` guard was written for but never applied to explorer/tool-details/history/marker; secrets in the build layer). These are pre-existing architectural gaps surfaced by this review, **not** regressions from the current uncommitted terminal work — the reviewers cleared that work in the ways it was built. Context: for a solo operator on a home network running trusted code, nothing here is actively exploited today; the cross-tenant items go live the moment a second user is added **or** untrusted code runs inside a project VM (which is the product's core purpose — running agents on arbitrary repos).
