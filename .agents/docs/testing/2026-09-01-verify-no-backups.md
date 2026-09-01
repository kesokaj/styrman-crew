# Verification — remove all backups + host GitHub token auth (2026-09-01)

## Status: PASS

Scope: the full uncommitted working tree described in the session context —
removal of the entire backup feature (both layers: per-project archive/
restore in the app, restic→GCS host backups) and the auto-updater's git auth
switch from an SSH deploy key to a fine-grained GitHub token at
`/etc/styrman/gh-token`. `deploy/caddy` (app + ingress module) is unchanged
this round and was not rebuilt or re-tested.

## Commands run

### 1. Main module

```
gofmt -l cmd/styrman deploy/update internal/config internal/gitrepo internal/httpapi
```
→ no output (clean). `internal/backup` no longer exists (deleted), so it was
excluded from the gofmt path list rather than erroring on a missing dir.

```
go vet ./...
```
→ no output (clean). `go version go1.25.0 linux/amd64`.

```
go test -race -count=1 ./...
```
→ all packages `ok`, no failures:
```
ok  	.../cmd/styrman	1.030s
?   	.../cmd/wsprobe	[no test files]
ok  	.../internal/config	1.018s
ok  	.../internal/conversation	1.022s
ok  	.../internal/credbroker	1.028s
ok  	.../internal/credstore	1.019s
ok  	.../internal/files	1.021s
ok  	.../internal/gitrepo	1.035s
ok  	.../internal/history	1.021s
ok  	.../internal/httpapi	1.100s
?   	.../internal/identity	[no test files]
ok  	.../internal/links	1.031s
ok  	.../internal/orchestrator	1.015s
ok  	.../internal/project	1.013s
ok  	.../internal/terminal	1.013s
ok  	.../internal/userstore	1.012s
?   	.../web	[no test files]
```
`internal/backup` is absent from this list (package deleted) and nothing
else references it — confirms the whole tree compiles and every remaining
package's tests pass with the feature fully gone.

### 2. Frontend

```
cd web && npx tsc -b
```
→ exit 0, no output.

```
cd web && npx oxlint src
```
→ exit 0, clean.

Dist-freshness check (a stale `dist` would silently ship the deleted UI —
`web/dist` is `go:embed`'d):
```
grep -rl "From backup" web/dist/client/assets/*.js
```
→ zero matches, both before and after the Tier-4 e2e rebuild (§7 below,
which rebuilds `web/dist` itself). `stat` on `web/src/routes/settings.tsx`
vs. the current `settings-*.js` asset showed the asset's mtime AFTER the
source edit's mtime, confirming the tracked dist in the working tree was
built from the edited source, not stale.

### 3. Shell scripts

```
bash -n deploy/update/styrman-update    → syntax-ok (exit 0)
bash -n deploy/host-setup.sh            → syntax-ok (exit 0)
shellcheck -S error deploy/update/styrman-update deploy/host-setup.sh
```
→ exit 0, no findings at error severity. shellcheck 0.10.0.

### 4. Updater token-path behavior (scratch only — ADR-052 hard constraint honored)

Ran as root via `sudo`. Every path used (`STYRMAN_UPDATE_STATE_DIR`,
`STYRMAN_DEPLOY_DIR`, `STYRMAN_GH_TOKEN_FILE`) was a fresh `mktemp -d`
NESTED scratch path under `/tmp`. `/var/lib/styrman` and `/etc/styrman` were
never read, written, or passed as an override to any of these tests —
confirmed both directories still existed and were unmodified after cleanup
(`/var/lib/styrman` still lists `backups/credentials/homes`; `/etc/styrman`
still root-permission-denied to the operator user, i.e. untouched).

Scratch `DEPLOY_DIR`: a fresh git repo, `.git` and `deploy/.env` chowned
root:root (`deploy/.env` mode 600), origin set to an https URL for a
nonexistent GitHub repo (`https://github.com/kesokaj-nonexistent-org-verify-2026/nonexistent-repo-verify.git`)
— unreachable in the sense that fetching it always fails, deterministically
and fast (no infinite hang), which is the exact shape `--check` needs to
exercise the fetch-failure path.

**a) No token file** (`STYRMAN_GH_TOKEN_FILE` pointed at a scratch path that
does not exist):
```
sudo env -i PATH="$PATH" HOME=/root \
    STYRMAN_UPDATE_STATE_DIR=<scratch>/state \
    STYRMAN_DEPLOY_DIR=<scratch>/deploy_dir \
    STYRMAN_GH_TOKEN_FILE=<scratch>/gh-token-noexist \
    timeout 30 bash deploy/update/styrman-update --check
```
→
```
fatal: could not read Username for 'https://github.com': terminal prompts disabled
[styrman-update] FATAL: git fetch failed — root cannot reach <scratch>/deploy_dir's origin. For a
private repo: mint a FINE-GRAINED GitHub token (this repo only, Contents: Read-only) and install it
at <scratch>/gh-token-noexist (root:root, chmod 600); the origin must be an https:// URL. Expired
token? Fine-grained tokens have an expiry — mint a fresh one.
```
exit 1, returned well inside the 30s `timeout` (no hang — `GIT_TERMINAL_PROMPT=0`
is proven by "terminal prompts disabled" instead of a stalled credential
prompt). **PASS** — the LOUD fetch-failure `fail()` names the token file, no
hang.

**b) World-readable fake token file** (`chmod 644`, `chown root:root`):
```
sudo env -i PATH="$PATH" HOME=/root \
    STYRMAN_UPDATE_STATE_DIR=<scratch>/state2 \
    STYRMAN_DEPLOY_DIR=<scratch>/deploy_dir \
    STYRMAN_GH_TOKEN_FILE=<scratch>/gh-token-644 \
    timeout 30 bash deploy/update/styrman-update --check
```
→
```
[styrman-update] FATAL: <scratch>/gh-token-644 is readable or writable beyond root (chmod 600 it)
```
exit 1. **PASS** — the perms refusal fires before any fetch attempt.

**c) Root-600 fake token file** (contains a fake, obviously-invalid token
value):
```
sudo env -i PATH="$PATH" HOME=/root \
    STYRMAN_UPDATE_STATE_DIR=<scratch>/state3 \
    STYRMAN_DEPLOY_DIR=<scratch>/deploy_dir \
    STYRMAN_GH_TOKEN_FILE=<scratch>/gh-token-600 \
    timeout 30 bash deploy/update/styrman-update --check
```
→
```
remote: Invalid username or token. Password authentication is not supported for Git operations.
fatal: Authentication failed for 'https://github.com/kesokaj-nonexistent-org-verify-2026/nonexistent-repo-verify.git/'
[styrman-update] FATAL: git fetch failed — root cannot reach ... (same message as §a, now naming
gh-token-600)
```
exit 1. **PASS** — this proves the credential helper is wired: it got PAST
the perms guard (no "chmod 600 it" refusal) and made GitHub reject the fake
token specifically (`remote: Invalid username or token`), i.e. it failed on
auth, not on the guard.

Scratch dirs (`<scratch>`, its three `state*` siblings) removed with
`sudo rm -rf` immediately after all three cases. No production path was
touched at any point.

### 5. Compose config rendering

```
cd deploy && STYRMAN_VERSION=9.9.9 docker compose -f docker-compose.yml config
```
→ exit 0, renders both services with `image: styrman-caddy:9.9.9` /
`image: styrman-server:9.9.9`. Rendered output (piped to a scratch file,
grepped, then deleted — it contains this host's real deploy secrets from
`deploy/.env`, not reproduced here) had **zero** case-insensitive matches
for `backup` anywhere in the config (`grep -ic backup` → `0`). **PASS.**

`deploy/.env.example` and `docker-compose.yml` were also grepped directly
for `STYRMAN_BACKUP`: zero matches in either.

### 6. Server image build (scratch tag) — embedded dist changed

```
docker build -f deploy/Dockerfile.server --build-arg STYRMAN_VERSION=verify-20260901-nobackup \
    -t styrman-server:verify-20260901-nobackup .
```
→ succeeds (`golang:1.25.13-trixie` builder, `debian:trixie-slim` runtime).
Image tagged `styrman-server:verify-20260901-nobackup` only —
`styrman-server:latest`/`:dev` untouched, no production container touched.

`deploy/caddy` (app + the separate-go.mod `deploy/caddy/ingress` module) is
**unchanged this round** (`git diff --stat -- deploy/caddy` and
`deploy/caddy/ingress` both empty) — skipped per the task, no AMD64/ARM64
Caddy build run.

### 7. Self-hosted suites

```
bash .agents/docs/testing/styrman/integration.sh
```
(default image `styrman-runtime:test`, isolated state dir under `/tmp`, port
18080) → **passed=57 failed=0 skipped=3** (the 3 skips are the documented
host-firewall preview-proxy cases, "prod unaffected"). Matches the expected
57/0/3 exactly. Full pass list includes stop/delete/wipe, persistence,
multi-tenancy, settings, git client, ports/preview — nothing in the output
references backups (the feature's routes are gone from `RoutesWithAuth`, so
there was nothing left to test here even before this run).

```
bash web/e2e/run.sh
```
(builds `web/` SPA, then the server, self-hosts on loopback port 18090,
isolated state dir under `/tmp`) → **passed=48 failed=0 skipped=2**, plus
the final "no uncaught client errors" gate PASS. Matches the expected
48/0/2 exactly — the same 49+2/48+2-shape suite as before, with no new/
removed check names, confirming no e2e case named or exercised the deleted
backup UI. This run rewrote tracked `web/dist/client/*` with fresh
content-hashed asset names (documented, expected build-output dirt); a
post-run `grep -rl "From backup" web/dist/client/assets/*.js` still found
zero matches.

Kata prereq verified first: `docker run --runtime mvm --rm debian:trixie
uname -r` → `6.18.28`.

## Coverage of changed code

| Changed behavior | Covering test | Verdict |
|---|---|---|
| `internal/backup` package deletion (whole feature) | N/A — package removed entirely; nothing in the repo imports or calls it (`grep -rn "internal/backup" --include=*.go .` → 0 hits) | covered by absence — deletion is complete, nothing dangling |
| `internal/httpapi/backups.go` + `backups_guard_test.go` deletion (5 routes, `createProject`'s `FromBackup` path, wipe-delete's pre-wipe safety backup) | N/A — deleted along with their only test file; `grep -rln "STYRMAN_BACKUP" .` and a targeted grep for `FromBackup\|restoreBackup\|listAllBackups\|doBackup\|seedFromBackup` across all `*_test.go` both return 0 hits | covered by absence |
| `tryAcquire` moved to `credharvest.go` (single-flight for credential push, its one remaining caller) | `internal/httpapi/inflight_guard_test.go`: `TestTryAcquireSingleFlights` — first acquire succeeds, second acquire on the same key is refused while held, a different key is not blocked, release makes the key acquirable again | covered |
| `cmd/styrman/main.go`: `RunBackupCadence` goroutine removed; `ensureRepos` doc comment updated | exercised transitively — `go test ./cmd/styrman` passes, and `ensureRepos`'s actual behavior (git-repo invariant on boot) is unchanged by this diff (comment-only there); `integration.sh`'s "git repo initialized" + legacy-layout-migration checks exercise the boot path this function runs in | covered (functional) |
| `internal/config/config.go`: `backupCadence`/`backupKeep` fields, env vars, and `BackupCadence()`/`BackupKeep()`/`ProjectBackups()`/`UserBackups()` accessors all removed | N/A — removed, nothing left to test; `go vet` + `go build` (via `go test ./...`) confirm no remaining caller | covered by absence |
| `deploy/update/styrman-update`: SSH deploy-key auth replaced with fine-grained-token auth (`GIT_TERMINAL_PROMPT=0`, `GH_TOKEN_FILE`, perms assertions, inline credential helper) | no Go/bash unit-test harness exists in this repo for shell scripts (same as every prior release); verified LIVE in this run, exactly as the task specified (§4, all three required scenarios: no token / world-readable token / valid-shape root-600 token) | verified this run; remains **unautomated** — same caveat every prior verification log for this script has recorded |
| `deploy/host-setup.sh` §8c (token-mint instructions) and §8b (backup section replaced with a removal note) | `bash -n` + `shellcheck -S error` clean; the actual printed instructions were read and diffed by hand in this session and match the new token flow described in the task | verified this run (static + manual read); no automated test, consistent with every prior host-setup.sh verification |
| `deploy/docker-compose.yml` / `deploy/.env.example`: `STYRMAN_BACKUP_*` lines removed | verified live via `docker compose config` (§5) rendering cleanly with zero `backup` occurrences, plus a direct grep of both files | verified this run |
| `deploy/backup/` directory deletion (script + 2 systemd units) | N/A — deleted; no test referenced these files even before removal (they were host-installed artifacts, not Go-tested) | covered by absence |
| `web/src` UI removal (Sidebar backup action, NewProjectDialog "from backup", `settings.tsx` `BackupsSection`, `api.ts` backup functions/types) | `tsc -b` + `oxlint` clean (no dangling references/imports); `web/e2e/run.sh`'s 48-check suite exercises Sidebar, NewProjectDialog (project creation), and the settings page directly and none references backups; dist-freshness grep (§2, §7) confirms the built assets don't contain the removed UI's strings | covered (compile-clean + functional e2e, no unit test framework exists for `web/src` components beyond tsc/oxlint in this repo) |
| `internal/gitrepo/gitrepo.go` (doc-comment-only change, no logic touched) | `go test ./internal/gitrepo` passes (unchanged behavior) | covered — trivially, comment-only |

No removed-feature test remains anywhere in the tree: a repo-wide grep for
`internal/backup`, `STYRMAN_BACKUP`, and the specific removed backup
handler/helper names across all `*_test.go` files returns zero hits in
every case. Had any such reference survived, `go build`/`go vet` would have
failed to compile — it didn't, which is itself confirmation.

## Environment

Tier: local only (no remote/cloud requested, none of ADR-052's forbidden
paths touched). OS: Linux 6.12.107+deb13-amd64 (host), Docker Engine
29.7.2 (plain Docker, no Colima — Linux host). Kata `mvm` runtime confirmed
working (`uname -r` → `6.18.28` inside a `--runtime mvm` container).
`go version go1.25.0 linux/amd64` (toolchain); `golang:1.25.13-trixie` is
what the Dockerfile.server build actually pulled and used. Node v20.19.2 /
npm 9.2.0 for the web/e2e tier. shellcheck 0.10.0.

Scratch artifacts created and left in place (all clearly non-production
tags; two — `styrman-caddy:verify-20260901`, `styrman-server:verify-20260901`
— predate this run from a prior verification session, left as they were; one
new — `styrman-server:verify-20260901-nobackup` — added by this run). No
production container (`styrman-server`, `styrman-caddy`) was started,
stopped, rebuilt, or otherwise touched. `/var/lib/styrman` and `/etc/styrman`
were never written to, and were confirmed still present/unmodified after
every root-run scratch test in §4.

No blocking findings. No non-blocking findings beyond the pre-existing,
already-tracked lack of automated coverage for the shell/compose layers
(§4/§5/§6 verified by hand, as they always have been in this repo — not a
regression introduced by this change set).
