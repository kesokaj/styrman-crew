# 2026-08-27 — Security lockdown (Batch A, part 1)

First remediation batch from the full-project review
(`.agents/code-review/2026-08-27-full-project-review.md`). Closes the
confirmed cross-tenant breaks; the deeper re-architecture items (B1c, B6c,
N22) are marked in-progress in the remediation plan for part 2.

## Summary

- **B1 — removed the auth-bypass dev listener from production.** It bound
  `0.0.0.0:8081` inside a server container joined to every project bridge,
  handing any project VM unauthenticated **admin** (confirmed live in the
  review). Production now has no dev door; local testing without OAuth uses a
  **host** server on `127.0.0.1` (unreachable from project VMs).
  - `config.Load()` now refuses to start the dev listener unless its address
    is loopback (`isLoopbackAddr`) — enforced in code, not a comment.
  - Added an opt-in ingress token (`STYRMAN_INGRESS_TOKEN` /
    `X-Styrman-Ingress`, constant-time compared): when set, header-mode
    requests without it are rejected, blocking a project VM from spoofing
    `X-Caddy-Email` against `:8080`. Disabled by default; enabled together
    with the Caddy ingress in part 2 (B1d).

- **B2–B5 — one fix closes four symlink RCEs: `os.Root` confinement.** Every
  home-relative file operation now goes through `internal/files` helpers
  rooted at the user's home (a real, server-created dir), so a guest-planted
  symlink (guests run as the same uid) is refused atomically at every path
  component — no check-then-use window.
  - `internal/files/files.go` rewritten on `os.OpenRoot`: `List/Read/Write/
    SaveStream/OpenForDownload/ReadRaw/ListNames/Save/Exists`, plus
    `ContainedPath` for exec-based callers (git) that need a real path.
  - `explorer.go`: `rootFor` → `scopeFor` (root is always the user home,
    prefix narrows to the project workspace) — fixes cross-tenant read/write
    via a symlinked `projects/<dir>` (could `PUT users.json` to self-promote).
  - `setup.go`: `toolDetails`/`userSetupStatus` confined reads (was: SSH
    **private** key leak via a symlinked `id_ed25519.pub`); `finishSetup`
    confined marker write (was: arbitrary file overwrite).
  - `history.go`: `List`/`Read` confined (was: arbitrary file read via a
    symlinked `.log`).
  - API error bodies no longer leak host paths (generic message + logged
    detail).
  - New `internal/files/files_test.go`: symlink-leaf / symlink-dir /
    symlink-root escapes, traversal clamping, confined write/read/exists,
    `ContainedPath`.

- **B6 — secrets hygiene.** Added root `.dockerignore` (`.git`, `**/.env`,
  `web/node_modules`, `*.png`, `.agents`, …) so `deploy/.env` (Google secret,
  Cloudflare token, keyring pw) can no longer be baked into the builder
  layer. `host-setup.sh` now generates a real `STYRMAN_KEYRING_PW`
  (`openssl rand`) and the compose requires it (`${STYRMAN_KEYRING_PW:?}`)
  instead of defaulting to the literal `styrman`; also guards an empty
  `DOCKER_GID`.

- **Test harness:** e2e self-hosts via `web/e2e/run.sh` (host server on a
  loopback port, isolated state dir) instead of relying on the production dev
  door — which also fixes the by-name pre-clean self-poisoning (F1, moot
  under fresh state). Retargeted the home-scope check at `.bashrc` (always in
  the skeleton) instead of `.claude` (only after a real sign-in).

## Files changed

`internal/files/files.go` (+`files_test.go`), `internal/httpapi/{explorer,setup,auth,checkpoints}.go`,
`internal/history/history.go`, `internal/config/config.go`,
`deploy/docker-compose.yml`, `deploy/host-setup.sh`, `.dockerignore` (new),
`web/e2e/run.sh` (new), `web/e2e/run.mjs`.

## Verification

`go build`/`go vet`/`go test ./...` green (new `files` containment tests);
integration 56/56; e2e 31/31 (self-hosted); production redeployed and the
host `:8080`/`:8081` dev door confirmed **closed** (`curl` refused), server
health OK via the container. Cross-tenant containment proven by unit tests
(symlink escapes refused). Residual: the `:8080` `X-Caddy-Email` spoof from a
project VM is closed only once the ingress token is enabled on both server
and Caddy (part 2).

## Part 2 — ingress token (B1d), closes the :8080 spoof

The vendored Caddy ingress now injects `X-Styrman-Ingress: <STYRMAN_INGRESS_TOKEN>`
on the authenticated app route (`deploy/caddy/ingress/{config,generator,cmd}.go`),
and the server requires it in header-auth mode (opt-in from Batch A part 1, now
enabled). `host-setup.sh` + `.env.example` generate/carry the token; compose
passes it to both caddy (env_file) and server. A project VM on the shared
bridge can no longer reach `:8080` and spoof `X-Caddy-Email` — it lacks the
token.

Verified live: from inside the server container, a direct request with a
forged `X-Caddy-Email` but no token → **403**; the same with the token → 200;
`/api/health` (exempt) → OK; Caddy's live admin config contains the header.
Rollout was Caddy-first (inject) then server (enforce) to avoid a lockout
window; the server fails **open** if the token is unset, so the rollback is
`STYRMAN_INGRESS_TOKEN=` + redeploy server.

Note: redeploying Caddy rotated caddy-security's session state, so existing
browser sessions must sign in again (one-time re-login, not a lockout).

This completes the P0 security items from the review. Remaining Batch A
follow-ups (B1c server-off-bridges, B6c keyring-pw-via-file, N22 proxy CSP)
are hardening, not open holes.
