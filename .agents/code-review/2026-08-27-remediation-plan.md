# styrman — remediation plan (fix ALL review findings)

**Companion to:** `2026-08-27-full-project-review.md` (issue IDs B/I/F/N reference it).
**Goal:** fix every finding, in priority order, in reviewable batches. Re-run `reviewer` + both suites (integration 56, e2e 31) after each batch. Commit per batch.
**Status legend:** ☐ todo · ◐ in progress · ☑ done.

---

## Priority ranking (every issue, worst first)

**P0 — blocking security (before any 2nd user or running untrusted code in a VM)**
1. B1 — listeners reachable from project VMs → unauthenticated admin *(confirmed live)*
2. B2 — explorer file API not symlink-contained → cross-tenant read/write → self-promote to admin
3. B3 — setup tool-details raw read → SSH private key leak
4. B4 — transcript history uncontained → arbitrary file read
5. B5 — setup-done marker follows symlinks → arbitrary file overwrite
6. B6 — `.env` in build layer + keyring pw default/exposure
7. N22 — port-preview forwards user creds + same-origin, no CSP *(security-adjacent; fix with B batch)*

**P1 — reliability that bites even a solo operator**
8. I1 — network sweep deletes live nets → reboot bricks idle projects
9. I10 — failed tmux.conf cp → root-owned broken conf → breaks agy/keyring
10. I5 — non-atomic conn swap + blocking old.Close → reopen hang / reconnect spiral
11. I6 — boot self-attach blocks listener → health 502 after reboot
12. I2 — idle reaper races attach → kills live terminal
13. I3 — setup VM never reaped, restarts forever
14. I9 — no SIGTERM → dirty ext4 + dockerd killed mid-write
15. I11 — reserved usernames accepted → broken guest
16. I4 — `profileStore.GuestName` data race
17. I16 — discarded ContainerRemove error → setup recreate wedges
18. I12 — silent vfs fallback looks like data loss
19. I8 — O(fds) port scan piles up *(also I7)*
20. I7 — StdCopy ignores ctx → goroutine/conn leak

**P2 — deployment hardening / reproducibility**
21. I14 — docker.sock not actually restricted for Caddy
22. I13 — unverified installers / Kata tarball (supply chain)
23. I15 — fresh-install paths contradict; only user #1 gets harness (R11/R5)

**P3 — frontend correctness & UX**
24. F1 — e2e deletes by name → self-poisoning suite
25. F6 — usePolled no in-flight/abort guard; request() no timeout
26. F5 — resize not debounced → frame flood
27. F4 — Git tab swallows diff error
28. F2 — file editor discards unsaved edits silently
29. F3 — clips persist tokens to localStorage *(needs decision + ADR)*
30. F7 — inconsistent error handling / unhandled clipboard rejections
31. F10 — e2e agent check is a no-op; no pageerror listener
32. F8 — a11y gaps on overlays/modals

**P4 — docs, nits, optimizations**
33. F9 — docs behind code (clips/link bar/copy/search; changelog; AGENTS.md count)
34. N1–N34 — nits & optimizations (batched cleanup)

---

## Execution plan (batches)

### Batch A — security lockdown (P0) 🔴
**Theme:** close the tenancy boundary. Do this first; nothing else matters if a VM is admin.

- ☑ **B1a** Drop `STYRMAN_DEV_ADDR` from `deploy/docker-compose.yml`. Keep it only for local `go run`.
- ☑ **B1b** In `config.Load()`: refuse to start the dev listener unless `DevAddr` resolves to a loopback IP (fail loudly, not a comment).
- ☐ **B1c** Keep the server off project bridges: dial guests for the port-preview via an explicit `net.Dialer` (or a dedicated attach that is torn down after use) instead of a standing `EnsureSelfAttached`; bind the main listener to the `styrman-ingress` endpoint IP.
- ☑ **B1d** DONE: Caddy ingress injects X-Styrman-Ingress (shared token from .env); server enforces it in header mode (verified live — spoofed X-Caddy-Email without the token → 403, with it → 200). config refuses auth=none on a non-loopback listener; files reject FIFOs/devices (goroutine-wedge DoS). `authGuard` (`header` mode): reject requests whose `RemoteAddr` is not the ingress; optional per-deploy `X-Styrman-Ingress-Token`.
- ☑ **B2–B5 (one change)** `os.Root` for every home-relative I/O: explorer, setup (toolDetails/status/finishSetup), history, plus the DELETE?wipe RemoveAll, createProject MkdirAll, and the R13 boot sweep (verified cross-tenant delete/create refused). SSH pubkey now rejected if it is a symlink (chosen over fingerprint — the pubkey is legitimately shown). `server.go` writeOnce left uncontained: `ensureHome` only runs at first home creation, before any guest content exists.
- ◐ **B2–B5 tests** `internal/files/files_test.go` escape cases DONE (leaf/dir/root symlink, FIFO, cross-tenant RemoveAll, drill-in regression). httpapi handler-level tests still ☐.
- ☑ **B6a** Add root `.dockerignore` (`.git`, `**/.env`, `*.env`, `web/node_modules`, `**/node_modules`, `*.png`, `.agents`, `dist` caches as appropriate).
- ☑ **B6b** `host-setup.sh` generates `STYRMAN_KEYRING_PW` (`openssl rand -base64 24`); drop the `:-styrman` compose default (`${STYRMAN_KEYRING_PW:?}`).
- ☐ **B6c** Deliver the keyring pw to the guest via a mounted tmpfs file/stdin, not `Env`; drop the misleading `unset`.
- ☐ **N22** In `proxy.go` `Rewrite`: strip `Cookie`/`Authorization`/`X-Caddy-*` from `pr.Out`; add a restrictive `Content-Security-Policy`; plan previews on a distinct origin or sandboxed iframe (follow-up if origin split is large).
- **Verify:** re-run security reviewer; from inside a project VM, confirm `:8081` is unreachable and a symlinked project dir 403s; suites green.

### Batch B — reliability core (P1, Go) 🟠
- ☑ **I1** Rewrite `CleanupLegacy` to build a keep-set of `sdb-` networks referenced by any managed container (all `styrman.project`/`styrman.system` labels) and only remove the rest; add a `ContainerStart` path that recreates a missing project network.
- ☑ **I5** Guard the bridge conn swap with a per-terminal mutex (or `map[string]*session` under one lock); mark the old session replaced via an atomic flag (so it can't emit 4000) and run `old.Close(4001)` in its own goroutine.
- ☑ **I6** Bound the boot `Adopt`+self-attach loop with a single overall deadline (~20 s) or move it to a goroutine that must finish before the first proxy request — never block `ListenAndServe`.
- ☑ **I2** Add `StopIdle` that takes `lockProject`, re-reads `o.activity`, and aborts if touched since the reap decision.
- ☑ **I3** Label setup VMs for the reaper (or a separate activity map) and stop/remove after an idle interval; reconsider `RestartPolicy` for setup VMs.
- ☑ **I16** Return the `ContainerRemove` error (unless `IsNotFound`) in the setup-recreate path.
- ☑ **I4** Add `sync.Mutex` to `profileStore` guarding `GuestName` in `GuestUser`/`setGuestUser`/`usernameChosen`; add `setGuestUser` fsync (**N24**).
- ☑ **I7** Run `stdcopy.StdCopy` in a goroutine and `select` on `ctx.Done()` → `att.Close()`.
- ☑ **I8** Replace the fd-walk port scan with `ss -ltnpH` (one pass); add an in-flight guard to the ports poll (ties to **F6**).
- ☐ **N12** Cap `SetReadLimit` (~16 MiB). **N15** throttle `touch()` ~1/s. **N13/N14/N18/N19/N23** dead-branch/dead-dep/nil-guard cleanups + `containerGuest`/orchestrator/terminal unit tests. **N16** treat `tmux detach` exit distinctly from a real shell exit.
- **Verify:** `go test -race`; reboot drill (stop server with idle projects, restart, confirm they start); reviewer.

### Batch C — image / entrypoint reliability (P1, shell) 🟠
- ☑ **I9** Replace `exec sleep infinity` with a TERM-trapping wrapper: `trap 'kill -TERM $DOCKERD_PID; wait; umount /var/lib/docker-project' TERM; sleep infinity & wait`.
- ☑ **I10** Verify the `.tmux.conf` `cp` succeeded before appending; do the append as the user, guarded on file existence; heal a root-owned broken conf.
- ☑ **I11** Reject reserved names in `setGuestUser` (deny `root` + UID<1000) **and** mirror in `setup.tsx`; entrypoint asserts `id -u "$USERNAME" == UID_` or FATAL-exits.
- ☑ **I12** On vfs fallback, write the chosen driver to a label/file the UI surfaces; log at operator-visible level; `fsck.ext4 -p` before mount.
- ☑ **N30** `df` free-space check before the 20 GB `truncate`. **N32/N33/N34** styrman-setup non-TTY guard + key-comment parse fix; ready-marker/resolv.conf/logdir/tmux-marker-version fixes; `${VAR:?}` guards; scoped `chown`.
- **Verify:** rebuild image; `docker stop` a project and confirm clean dockerd shutdown; suites.

### Batch D — deploy hardening & reproducibility (P2) 🟠
- ◐ **I14** (deferred — needs socket-proxy sidecar + live ingress test, see TODO) Front `docker.sock` with a read-only socket proxy (`CONTAINERS=1,EVENTS=1`) for Caddy; correct the "read-only" docs.
- ◐ **I13** (Kata tarball checksum DONE; OpenTofu skip-verify + version pins deferred, see TODO) Pin + verify the Kata release SHA256; drop OpenTofu `--skip-verify`; pin `yq`/Go/claude/node versions (**N27/N29**).
- ◐ **I15** (deferred, see TODO) Make `host-setup.sh` the single entry point: check `python3`, build the runtime image, invoke `home-init.sh`, print the compose command; give `home-init.sh` a `--user <slug>` flag and have `ensureHome` shell out to it (kill the drifted Go dotfile copy); align `deploy.md`. **N31** healthcheck comment.
- **Verify:** dry-run the fresh-install path end to end (ideally the R11 second machine).

### Batch E — frontend correctness & UX (P3) 🟡
- ☑ **F1** e2e pre-clean: look up the project by name via `GET /api/projects`, delete by UUID (handle duplicates). **F10** assert an agent sentinel (`agents=2`); add `page.on('pageerror')`/console listeners that fail the suite.
- ☑ **F6** `request()` gets `AbortSignal.timeout(15000)`; `usePolled` gets an in-flight guard + generation/ordering guard + unmount guard.
- ☑ **F5** Debounce resize ~100 ms; skip send when `cols<10||rows<3`.
- ☑ **F4** Render the diff error in the Git tab (destructure `error`).
- ☑ **F2** Confirm-on-discard for the file editor backdrop + ✕ when `dirty`.
- ☑ **F3** Decide clips storage: `sessionStorage`/in-memory + short TTL, redact obvious secrets, or gate history off for the setup terminal; write an ADR (R10).
- ☑ **F7** One shared `copy(text)` helper with `.catch` → toast; route all four call sites + `markDone` through consistent error surfacing.
- ☑ **F8** Add `aria-label`s w/ URL to link chips; `role=status`/`aria-live` on the toast; label the search ✕; `role=dialog aria-modal`, Escape, focus-trap/restore on all three modals.
- ☐ **N1–N11** frontend nits: fix stale terminal.ts header; clear `selTimer`/`toastTimer` on dispose; scan only new chunk (**N3**); reset `dismissed`/`urls` on epoch (**N5**); outbound queue during reconnect (**N6**); `onExited` ref (**N7**); `useSyncExternalStore` for clips + stable key (**N8**); lazy clips init (**N9**); setup error retry (**N10**); `.oxlintrc.json` ignoring `dist` (**N11**); prettier `api.ts`.
- **Verify:** `tsc`, e2e, manual copy/link/resize checks.

### Batch F — docs & remaining nits (P4) 🟢
- ☐ **F9** Document clips, link bar, copy-on-select, search overlay in `.agents/docs/specs/`; write the `changes/2026-08-27-*` changelog for the whole terminal/setup/git session; fix `AGENTS.md` check count; ADR for the terminal architecture (`lib/terminal.ts`) and mouse-off decision.
- ☐ **N17/N20/N21/N25/N26/N27/N28** remaining server-side nits (waitReady cost, Content-Disposition escaping, proxy prefix/RawPath, title length cap, git error surfacing, Chmod returns, `Lstat` in `List`).
- **Verify:** docs-in-same-commit; final full reviewer pass.

---

## Suggested order & checkpoints
A → B → C → D → E → F. A is the gate for "multi-user / untrusted-code safe." B+C are the "stop breaking my own projects" batch and can run in parallel with A if needed. Re-run the four-way reviewer after A, after C, and after E. Keep this file's checkboxes current as batches land.
