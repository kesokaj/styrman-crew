# Verification — redfix round 2 (working tree ahead of v1.16.1)   (2026-09-01)

## Status: PASS

Scope: the uncommitted working tree at HEAD `00d1c1831beba6b9705a3caece66670c25e8ca8d`,
focused on the fixes made in response to `.agents/docs/reviews/2026-09-01-review-redfix-round2.md`
(REQUEST CHANGES) and `.agents/docs/reviews/2026-09-01-security-redfix.md` (FINDINGS). All
blocking findings from both reports and from `.agents/docs/testing/2026-09-01-verify-redfix.md`
(round-1 verify, FAIL on missing `config.Load()` coverage) were re-checked against the current
diff, not taken on report.

## Round-1 suites — cited, not re-run, claim independently verified

`integration.sh` (57 passed / 0 failed / 3 skipped) and `web/e2e/run.sh` (48 passed / 0 failed /
2 skipped) are recorded in `.agents/docs/testing/2026-09-01-verify-redfix.md` §7. Per the task
instruction, these were NOT re-run; instead the claim that the server-behavior surface they
exercise is unchanged since that run was checked directly:

- `git diff HEAD -- internal/config/config.go` and `internal/orchestrator/exec.go` /
  `orchestrator.go` show the SAME diffs round-1 already tested against (the `STYRMAN_IMAGE`
  required-value change and the `tailBuffer` exact-boundary fix respectively) — round-1's own
  coverage table explicitly describes both as present-but-untested at that run, which is only
  possible if the code was already in the tree when `go test ./... -race` passed there.
- The only files touched in the main module since round-1's log was written
  (`find deploy internal web -newer .agents/docs/testing/2026-09-01-verify-redfix.md`) are
  `internal/config/config_test.go`, `internal/orchestrator/tailbuffer_test.go`,
  `deploy/Dockerfile.server` (Go builder-image pin only, no app code), and the ingress module
  (`watcher.go`/`watcher_test.go`/`loopback.go`/`loopback_test.go`/`go.mod`/`go.sum` — a
  separate module `integration.sh`/`web/e2e` do not build or exercise).
- `config.Load` is called only from `cmd/styrman/main.go`; both `integration.sh` and
  `web/e2e/run.sh` already set `STYRMAN_IMAGE` explicitly (round-2 review, "what I verified
  myself" table) — the new required-value check cannot change either suite's outcome.
- A fresh `go vet ./... && go test -race -count=1 ./...` (below) is clean, confirming nothing
  in the main module regressed since round-1's identical run.

Conclusion: round-1's 57/0/3 and 48/0/2 results still describe the current tree. Not re-run
here (per task instruction); the two log entries above are the citation.

## Commands run (fresh, this round)

### 1. Main module

```
go vet ./...
```
→ clean. `go version go1.25.0 linux/amd64`.

```
go test -race -count=1 ./...
```
→ all packages `ok`:
```
ok  	.../cmd/styrman
?   	.../cmd/wsprobe	[no test files]
ok  	.../internal/backup
ok  	.../internal/config
ok  	.../internal/conversation
ok  	.../internal/credbroker
ok  	.../internal/credstore
ok  	.../internal/files
ok  	.../internal/gitrepo
ok  	.../internal/history
ok  	.../internal/httpapi
?   	.../internal/identity	[no test files]
ok  	.../internal/links
ok  	.../internal/orchestrator
ok  	.../internal/project
ok  	.../internal/terminal
ok  	.../internal/userstore
?   	.../web	[no test files]
```
`go test ./... -race -count=1 -v | grep -c '^--- PASS'` → **107** (round-1 was 105; +2 =
`TestLoadRequiresExplicitImage` and `TestTailBufferExactMaxSingleWriteIsNotTruncation`).

### 2. Ingress module (`deploy/caddy/ingress`, separate `go.mod`)

```
go vet ./...
go test -race -count=1 ./...
```
→ clean; `ok  	github.com/spgo/devbox/caddy/ingress	3.096s`.
`-v | grep -c '^--- PASS'` → **47** (round-1 was 46; +1 =
`TestRejectedReloadArmsRetryAndFatalDoesNot`).

```
gofmt -l internal/orchestrator deploy/caddy/ingress internal/config deploy/update
```
→ no output (clean, exit 0) on both modules.

### 3. `go mod verify` + tidy-clean (scratch copy)

```
cd deploy/caddy/ingress && go mod verify
```
→ `all modules verified`.
```
grep -m1 'caddyserver/caddy/v2' go.mod
```
→ `github.com/caddyserver/caddy/v2 v2.11.4` — floor now matches the Dockerfile's
`caddy:2.11.4-builder`/`caddy:2.11.4` pin (round-2 review finding "go.mod floor is 2.11.3 vs
shipped 2.11.4" — closed).

Scratch-copy tidy check (`cp -r deploy/caddy/ingress/. $SCRATCH/`, `cd $SCRATCH && go mod
tidy`): exit 0; `diff go.mod` and `diff go.sum` against the real files both empty →
**byte-identical, tidy-clean**. (First attempt reported a shell exit 1 from `rm -rf` deleting
the directory the shell itself was still `cd`'d into afterward — re-ran with a `cd` back to the
repo root before cleanup; `go mod tidy`'s own exit code was 0 and the diffs were empty in both
runs.)

### 4. Shell scripts

```
bash -n deploy/update/styrman-update    → syntax-ok
bash -n deploy/host-setup.sh            → syntax-ok
shellcheck -S error deploy/update/styrman-update deploy/host-setup.sh
```
→ exit 0, no findings at error severity. shellcheck 0.10.0.

### 5. Updater guard matrix (root, scratch-only — ADR-052 honored; `/var/lib/styrman`,
   `/etc/styrman`, and the production containers were never touched)

**a) unset `STYRMAN_UPDATE_STATE_DIR`:**
```
sudo env -i PATH="$PATH" bash deploy/update/styrman-update --check
```
→ `FATAL: STYRMAN_UPDATE_STATE_DIR is not set — production: systemctl start
styrman-update.service (or pass /etc/styrman/update explicitly); tests: pass a NESTED scratch
path, e.g. $(mktemp -d)/state (ADR-052)`, exit 1. **PASS.**

**b) `STYRMAN_UPDATE_STATE_DIR=/var/lib/styrman/x`:**
```
sudo env -i PATH="$PATH" STYRMAN_UPDATE_STATE_DIR=/var/lib/styrman/x bash deploy/update/styrman-update --check
```
→ `FATAL: STYRMAN_UPDATE_STATE_DIR=/var/lib/styrman/x is inside /var/lib/styrman — refusing
(ADR-052)`, exit 1. **PASS.**

**c) `STYRMAN_UPDATE_STATE_DIR=//var/lib/styrman/x`:**
```
sudo env -i PATH="$PATH" STYRMAN_UPDATE_STATE_DIR=//var/lib/styrman/x bash deploy/update/styrman-update --check
```
→ same refusal, canonicalized to `/var/lib/styrman/x` first via `realpath -m` before the
`case` match — exit 1. **PASS** (closes the security review's `//var/lib/styrman` evasion
finding).

**d) nested scratch `$(mktemp -d)/state`:**
```
SCRATCH_D=$(mktemp -d); STATE_D="$SCRATCH_D/state"
sudo env -i PATH="$PATH" STYRMAN_UPDATE_STATE_DIR="$STATE_D" bash deploy/update/styrman-update --check
```
→ guard passes; fails at the next check, `FATAL: no deploy clone at /opt/styrman (see
host-setup.sh step 8c)`, exit 1 — matches the documented expected next failure (this host has
no `/opt/styrman` clone). The leaf `$STATE_D` was never created (the script exits before
reaching the `install -d` lines, which sit after the deploy-clone check). **PASS**, guard
ordering correct. Scratch dir removed after.

**e) regression check — the `/tmp` footgun (non-nested `$(mktemp -d)`):**

Two runs, both scratch-only:

1. *Literal task shape* — `STYRMAN_UPDATE_STATE_DIR=$(mktemp -d)` (parent = `/tmp` itself),
   no fake deploy clone: `/tmp` stat before = `1777 root:root`; script exits at the same
   "no deploy clone" check as (d) before ever reaching `install -d`; `/tmp` stat after =
   `1777 root:root` — **unchanged**. This confirms the observable outcome the task asked for,
   but does not by itself prove the `install -d` line was exercised (it wasn't — the
   deploy-clone check gates before it, on this host).

2. *Rigorous re-check* — to actually exercise the fixed `install -d` lines, a fake deploy
   clone was created (`mkdir -p $FAKE/.git $FAKE/deploy && touch $FAKE/deploy/.env`) and the
   run repeated with `STYRMAN_DEPLOY_DIR="$FAKE"` so the script passes the earlier gates:
   ```
   sudo env -i PATH="$PATH" STYRMAN_UPDATE_STATE_DIR="$SCRATCH_E2" STYRMAN_DEPLOY_DIR="$FAKE" bash deploy/update/styrman-update --check
   ```
   `/tmp` stat before/after: `1777 root:root` / `1777 root:root` — **unchanged**. The script
   now reached line 106 (`[ -d "${STATE_PARENT}" ] || install -d …`), found `/tmp` already
   exists, and **skipped** the `install -d` call entirely (the fix) — it did not run
   `install -d -m 700 -o root -g root /tmp`. It then found the pre-existing scratch leaf
   dir (`mktemp -d` created it `700 kesokaj:kesokaj`) already exists too, skipped that
   `install -d` as well, and correctly **failed closed** at the ownership assertion
   (`FATAL: … is not root-owned — refusing to trust the recorded deploy state`) rather than
   silently re-chowning it. This is definitive: the fix does not merely happen to avoid the
   footgun on this host, it structurally skips mutation of any pre-existing directory.
3. *Bonus positive case* — a genuinely fresh nested path (`$(mktemp -d)/nested/state`, parent
   `mktemp -d` pre-exists as `700 kesokaj:kesokaj`, `nested/` and `nested/state` do not exist)
   with the same fake deploy clone: the outer scratch dir stayed `700 kesokaj:kesokaj`
   throughout (never touched — it was never `dirname($STATE_DIR)`); `nested/` and
   `nested/state` were freshly created `700 root:root` by the two `install -d` calls exactly
   as designed. (Run failed later at an unrelated, correct check —
   "`/tmp/.../.git` is not root-owned" — because the fake clone was owned by the test user,
   not root; irrelevant to what this case was checking.)

All scratch dirs (`$SCRATCH_D`, `$SCRATCH_E2`, `$FAKE`, and the bonus-case dirs) were removed
with `sudo rm -rf` immediately after each run. `/var/lib/styrman`, `/etc/styrman`, and the
production containers (`styrman-server`, `styrman-caddy`) were never touched.

### 6. Docker builds — scratch tags only (per ADR-052)

```
docker build -t styrman-caddy:verify-round2-20260901 deploy/caddy
```
→ succeeds. `xcaddy` resolves `caddy/v2 v2.11.4` (the module's own build log:
`v2.11.4 h1:XKxkMTgNSizEvKG6QHue6cAsFOteU2qA61w2tKkCWi0=`) against base image
`caddy:2.11.4-builder`. Verified directly on the built image:
```
docker run --rm styrman-caddy:verify-round2-20260901 caddy version
→ v2.11.4 h1:XKxkMTgNSizEvKG6QHue6cAsFOteU2qA61w2tKkCWi0=
docker run --rm styrman-caddy:verify-round2-20260901 caddy list-modules | grep -E "(security|cloudflare|rate_limit|ingress)"
→ dns.providers.cloudflare
→ http.handlers.rate_limit
→ ingress
→ security
```
All four present. **PASS.**

```
docker build -f deploy/Dockerfile.server --build-arg STYRMAN_VERSION=verify-round2 -t styrman-server:verify-round2-20260901 .
```
→ succeeds (repo-root context). Builder-stage toolchain confirmed separately:
```
docker build -f deploy/Dockerfile.server --target builder --build-arg STYRMAN_VERSION=verify-round2 -t styrman-server-builder:verify-round2 .
docker run --rm styrman-server-builder:verify-round2 go version
→ go version go1.25.13 linux/amd64
```
Matches the Dockerfile's `golang:1.25.13-trixie` pin exactly (newest 1.25.x at pin time).
**PASS.** (Builder helper image removed after the check; the two scratch-tagged final images —
`styrman-caddy:verify-round2-20260901`, `styrman-server:verify-round2-20260901` — left in
place, no `:latest`/`:dev`/production tag touched, no production container rebuilt or
restarted.)

### 7. Compose config rendering

```
STYRMAN_VERSION=9.9.9 docker compose -f deploy/docker-compose.yml config | grep image:
```
→
```
    image: styrman-caddy:9.9.9
    image: styrman-server:9.9.9
```
and `STYRMAN_IMAGE: styrman-runtime:9.9.9` confirmed in the same render (nested
`${VAR:-${OTHER}}` interpolation still resolves). **PASS.**

```
env -i PATH="$PATH" HOME="$HOME" docker compose -f deploy/docker-compose.yml config
```
(unset) → exit 1:
```
error while interpolating services.caddy.image: required variable STYRMAN_VERSION is missing
a value: set STYRMAN_VERSION=X.Y.Z — mutable tags are banned
error while interpolating services.styrman.image: required variable STYRMAN_VERSION is missing
a value: set STYRMAN_VERSION=X.Y.Z — mutable tags are banned
```
**PASS** (refuses as required).

### 8. Coverage — the three new tests, run individually + mutation-verified independently

```
go test ./internal/config/... -run TestLoadRequiresExplicitImage -v
→ PASS. Exercises BOTH paths: STYRMAN_IMAGE="" → Load() errors and the message contains
  "STYRMAN_IMAGE"; STYRMAN_IMAGE="styrman-runtime:9.9.9" → Load() succeeds and
  c.Image == the explicit value (read directly from config_test.go, both assertions present).

go test ./internal/orchestrator/... -run TestTailBufferExactMaxSingleWriteIsNotTruncation -v
→ PASS.

cd deploy/caddy/ingress && go test ./... -run TestRejectedReloadArmsRetryAndFatalDoesNot -v
→ PASS.
```

Not content to trust the changelog claim that these are "mutation-verified" — independently
re-ran both non-trivial mutations in throwaway scratch copies (never touching the tracked
files) and confirmed each test goes red:

- Deleted `w.scheduleRetry(ctx)` from `noteReloadFailure`'s non-fatal branch in a scratch copy
  of `deploy/caddy/ingress` → `TestRejectedReloadArmsRetryAndFatalDoesNot` **FAILED**:
  `watcher_test.go:141: a rejected reload must arm the retry timer`. Scratch copy discarded.
- Reverted `tailBuffer`'s exact-boundary fix to unconditional `t.dropped = true` in a scratch
  copy of the main module → `TestTailBufferExactMaxSingleWriteIsNotTruncation` **FAILED**:
  `tailbuffer_test.go:66: exactly-max write into an empty buffer discards nothing — Dropped()
  must be false` and `:72` (the `StartsAtLineBoundary` assertion also failed). Scratch copy
  discarded.

`git status --porcelain` after both mutation runs shows no change to `deploy/caddy/ingress/`
or `internal/orchestrator/` beyond what was already in the working tree before this
verification started — confirms the mutation runs never touched the real tree.

## Coverage of changed code (this round's scope)

| Changed behavior | Covering test | Verdict |
|---|---|---|
| `config.Load()` — `STYRMAN_IMAGE` required, no fallback | `TestLoadRequiresExplicitImage` (refusal + success paths) | covered, mutation-independent-of-scratch-check not needed (assertion is direct on `Load()`'s own return) |
| `tailBuffer` exact-boundary fix (single write of exactly `max` into an empty buffer must not set `dropped`) | `TestTailBufferExactMaxSingleWriteIsNotTruncation` | covered, mutation-verified red (re-run, this log) |
| watcher retry-vs-fatal decision (`noteReloadFailure`) | `TestRejectedReloadArmsRetryAndFatalDoesNot` | covered, mutation-verified red (re-run, this log) |
| `deploy/update/styrman-update` ADR-052 guard (unset/prefix/canonicalization refusals; parent/leaf `install -d` only-if-missing) | no automated test (bash, no harness in this repo) — verified live, §5, including a rigorous re-check that reaches past the earlier gates to actually exercise the fixed `install -d` lines | verified this run |
| `deploy/host-setup.sh` versioned-tag build + printed next-steps recipe | no automated test — verified by reading the diff against the version-derivation shape (`git describe --tags --abbrev=0 | sed 's/^v//'`, matching `styrman-update`'s own `${LATEST#v}`) | verified by inspection, consistent with `styrman-update` |
| `deploy/caddy/ingress/go.mod` caddy floor 2.11.3 → 2.11.4 | `go mod verify` + tidy-clean scratch check; live image `caddy version` = v2.11.4 | verified this run |
| `deploy/Dockerfile.server` golang 1.25-trixie → 1.25.13-trixie | builder-stage `go version` on the built image | verified this run |
| `deploy/caddy/ingress/loopback.go` numeric-port validation | pre-existing `loopback_test.go` cases (not re-audited line-by-line this round; `go test` green, gofmt clean) | covered (per round-1/round-2 review record) |

No changed code in this round's scope is without a covering test or an equivalent live
verification. The bash/compose surfaces remain without an automated harness (a standing,
previously-noted gap in this repo, not new) — covered here by direct, repeatable manual
verification per the task's explicit instruction to test them that way.

## Documentation updated as part of this run

`.agents/docs/testing/styrman/README.md` — the `STYRMAN_UPDATE_STATE_DIR` example was updated
from the flat `$(mktemp -d)` shape to the nested `$(mktemp -d)/state` shape, matching the
script's own current `fail` message (ADR-052) and the rigorous regression check in §5e above.
The flat shape is not unsafe under the current fix (proven in §5e run 2 — an existing parent is
never re-moded), but the nested shape is what the script itself now recommends, and keeping the
testing guide's example in sync avoids it drifting back into a footgun-shaped recommendation on
a future revert.

## Environment

Tier: local only (no remote/cloud requested). OS: Linux 6.12.107+deb13-amd64. Docker Engine
29.7.2 (plain Docker, no Colima — Linux host). `go version go1.25.0 linux/amd64` (local
toolchain; `golang:1.25.13-trixie` and `caddy:2.11.4-builder`/`caddy:2.11.4` are the pinned
build/runtime images, verified in §6). shellcheck 0.10.0. `sudo` available non-interactively
for the root-required updater guard checks.

Scratch artifacts created and left in place (non-production tags only):
`styrman-caddy:verify-round2-20260901`, `styrman-server:verify-round2-20260901`. Removed:
`styrman-server-builder:verify-round2` (helper image, deleted after use), all `/tmp/styrman-
verify-*` scratch directories (removed with `sudo rm -rf` immediately after each check). No
production container (`styrman-server`, `styrman-caddy`) was started, stopped, rebuilt, or
restarted. `/var/lib/styrman` and `/etc/styrman` were never touched.

## Blocking findings

None.
