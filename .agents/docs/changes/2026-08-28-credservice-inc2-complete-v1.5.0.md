# Credential-service Inc 2 complete — all tiers, v1.5.0 (ADR-032)

Date: 2026-08-28. Finishes the credential broker (ADR-030/031): every tool's
credentials are now shared across a user's projects and kept current, via one
unified *pull-on-login* mechanism. Cuts **v1.5.0**.

## What changed

### Broker + store (Go)
- `internal/credstore`: generalized from git-only to three credential kinds —
  `git-<host>.json`, `env-<KEY>.json`, `file-<id>.json` (all 0600, atomic
  temp+fsync+rename). New: `PutEnv`/`AllEnv`, `PutFile`/`AllFiles`, `List`
  (value-free), `Delete`. Env keys validated as shell identifiers; file paths
  validated home-relative (no absolute, no `..`).
- `internal/credbroker`: added `GET /env` (key→value map) and `GET /files`
  (`[{path,mode,data(base64)}]`) beside `GET /git`. All caller-identified by
  bridge source IP; always serve the CURRENT store copy.

### In-guest delivery (image)
- `image/configs/styrman-credentials.sh` → seeded as
  `/etc/profile.d/zz-styrman-credentials.sh` (sorts after the keyring snippet).
  Every login shell pulls `/env` (exports each var) and `/files` (writes each
  file at its mode under `$HOME`). Quiet + fast (5 s timeout); no broker →
  no-op.
- `image/skel/npmrc` → seeded as `~/.npmrc` with
  `//registry.npmjs.org/:_authToken=${NPM_TOKEN}`; npm expands the broker's
  NPM_TOKEN at invocation.

### Onboarding + management (server + UI)
- `POST /api/settings/credentials/import {project}` — one pass over a running
  reference project extracts gh (keyring→GH_TOKEN + git helper), claude
  (`~/.claude/.credentials.json`), gcloud ADC, the first ssh key, and an npm
  authToken; each best-effort, reported per tool.
- `POST /api/settings/credentials/token {key,value}` — store a pasted env
  token. `GET /api/settings/credentials` — value-free list.
  `DELETE /api/settings/credentials/{kind}/{name}` — remove one.
- Settings gets a **Credentials** panel: what's shared, import-from-a-running-
  project, add-a-token, and per-item remove.

## Tiers (honest)
git/docker→registry: secret never stored in the VM (helper tier). gh/npm/
claude/gcloud/ssh: token/file materialized in the VM (env/file tier) — accepted
P1; P2 hardens to secret-never-in-VM. ssh is file-injected because Kata blocks
agent-socket forwarding. **agy** stores OAuth only in gnome-keyring with no
verified mapping (issue #632) — reported "unsupported", the one tool still
needing per-project sign-in.

## Files changed
- `internal/credstore/credstore.go` (+ `credstore_test.go`)
- `internal/credbroker/broker.go` (+ `broker_test.go`)
- `internal/httpapi/credentials.go` (new), `credentials_test.go` (new),
  `server.go` (routes)
- `image/configs/styrman-credentials.sh` (new), `image/skel/npmrc` (new),
  `image/entrypoint.sh` (seed npmrc + install the profile.d snippet)
- `web/src/lib/api.ts`, `web/src/routes/settings.tsx` (Credentials panel)
- Version → 1.5.0 (server ldflags + image build arg)

## Adversarial review fixes (before commit)
A reviewer pass found and I fixed:
- **BLOCKING — cross-tenant leak via the IP→owner cache** (`orchestrator.go`
  `ResolveProjectByIP`): a cache HIT returned the memoized `(user,project)`
  without re-checking the owner, and the cache wasn't invalidated on destroy —
  so a freed bridge subnet/IP reused by a DIFFERENT user's project within the
  30 s TTL could be served the first user's credentials. Fix: a cache hit now
  re-verifies with ONE `ContainerInspect` that the cached container still
  exists, still carries the cached user label, and still holds the source IP
  (`containerHoldsIP`); the scan path also only caches an IP the container
  actually holds. The mapping can't outlive the container that earned it.
- **env value newline injection**: `PutEnv` (and `AllEnv` on read) now reject
  control characters in the value, so a value can't smuggle an extra
  `KEY\tVALUE` line into the login snippet's line-based `export`.
- **delete ≠ revoke**: the Settings panel now states that removing a
  credential stops sharing with new sessions but a token/file already
  materialized in a running project clears on restart.

Confirmed NOT vulnerable by the review: import command-injection (shellQuote +
guest-name regex), login-snippet file-write injection (fully quoted, no eval),
path traversal (validated on write, read, and in the snippet), env-key
validation (triple-enforced), and secret logging (none).

## Verification
- `go test -race ./...` clean (credstore env/file/list/delete, broker
  /env//files/403, httpapi credentials list/token/delete/503).
- `tsc --noEmit` + `vite build` clean.
- Kata integration suite: **62/62** against the v1.5.0 image.
- **Live end-to-end (deployed v1.5.0 server, real Kata project):** broker
  `/env` + `/files` reachable from inside the project by bridge position; a
  fresh login shell exported the env cred and materialized the file cred
  (exact content + mode 640); npm picked up NPM_TOKEN via the skel `.npmrc`;
  import returned an honest per-tool report; value-free list correct;
  synthetic creds + throwaway project cleaned up; the operator's real project
  left untouched.
