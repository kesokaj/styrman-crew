# Verification — harness-upgrade-v1.15.0 fix (R1 boot-latency defect)   (2026-09-01, rev2)

## Status: PASS

Re-verification of the fix applied after my earlier PASS
(`.agents/docs/testing/2026-09-01-verify-harness-upgrade-v1.15.0.md`), which
missed a real defect: `antigravity/setup.sh`'s self-check shells out to
`agy -p "/skills"`, guarded only by `command -v agy`; on any home with no
agy OAuth token (every freshly created project) that call blocks a fixed
~60s, and the harness-seeding block sits ahead of `/run/styrman/ready` on
the container boot path — a 4x violation of REQUIREMENTS.md R1. My first
pass measured only a re-seed of a home that already carried a 1.13.1-era
token (fast path), so it never hit the slow path.

The claimed fix: `image/entrypoint.sh` (~line 137) now runs `setup.sh` with
`PATH=/usr/bin:/bin:/usr/sbin:/sbin`, so `command -v agy` fails and the
self-check is skipped. `styrman-runtime:dev` rebuilt at
`STYRMAN_VERSION=1.15.0` (image ID `1a8381a805a1`, created
2026-09-01T09:17:15+02:00). None of the numbers below were taken on trust;
every measurement was re-run fresh by this agent.

## Commands run

### 1. Reproduce the ORIGINAL defect (confirm it was real)

Inside a throwaway container off `styrman-runtime:dev`, created a user,
`mktemp -d`-copied `/styrman/harness`, ran `setup.sh` as that user with the
container's **default PATH** (which includes `/usr/local/bin`, where `agy`
lives):

```
TIMEFORMAT="elapsed=%R"
time runuser -u tuser -- env HOME=/home/tuser bash "$HSRC/setup.sh" >/tmp/out1.log 2>&1
```
→ `elapsed=60.961`, exit 0 (script completes, just slow).

`agy -p "/skills"` alone, same user/env:
```
time runuser -u tuser -- env HOME=/home/tuser agy -p "/skills" >/tmp/agy.log 2>&1
```
→ `elapsed=60.318`.

**Confirms the defect was real** and squarely inside `setup.sh`'s
self-check, not somewhere else in the deploy sequence. PASS (defect
reproduced as described).

### 2. Time the same run under the trimmed PATH

```
time runuser -u tuser -- env HOME=/home/tuser PATH=/usr/bin:/bin:/usr/sbin:/sbin \
    bash "$HSRC/setup.sh" >/tmp/out2.log 2>&1
```
→ `elapsed=0.445`, exit 0, full deploy output (CLAUDE.md, skills, gates,
slash commands) present in the log — not a truncated/failed run.

`command -v agy` under the trimmed PATH → exit 1 (not found), confirming
the guard is what's being defeated. PASS.

### 3. Confirm the deploy is IDENTICAL under the trimmed PATH, not merely fast

Ran two full `setup.sh` deploys inside one container — `userA` with the
default PATH (agy reachable, self-check runs and eventually fails/times
out since there's no token), `userB` with the trimmed PATH (self-check
skipped) — then compared:

| Artifact | userA (agy on PATH) | userB (agy hidden) |
|---|---|---|
| `~/.claude/CLAUDE.md` | 69 lines | 69 lines |
| `~/.gemini/GEMINI.md` | 69 lines | 69 lines |
| `~/.claude/skills/*/` count | 28 | 28 |
| `~/.gemini/config/skills/*/` count | 28 | 28 |
| `~/.claude/agents/` | changelog, documentation, explorer, reviewer, security, verifier (6) | same 6 |
| `~/.gemini/config/plugins/harness/agents/` | same 6 | same 6 |
| `~/.agents/bin/{resume-context.sh,trail-check.sh}` | present, executable, `.bak.*` litter | same |
| `~/.claude/settings.json` | valid JSON | valid JSON |

`diff -rq --exclude="*.bak.*" ~/.claude(A) ~/.claude(B)` → **no
differences**. Same for `~/.agents`.

`diff -rq --exclude="*.bak.*" ~/.gemini(A) ~/.gemini(B)` → **NOT
identical**. userA has extra items userB does not:
`~/.gemini/antigravity-cli/` (brain, cache, conversations,
`conversation_summaries.db`, crashes, `installation_id`,
`jetski_state.pbtxt`, logs), `~/.gemini/config/config.json`,
`~/.gemini/config/mcp_config.json`, `~/.gemini/config/projects/default-cli-project.json`.

**Root cause of the discrepancy**: this is not a harness-deploy difference —
it's `agy` itself materializing its own CLI state (installation ID tied to
the container's hostname, a conversation-summary DB, log files) purely as a
side effect of being *invoked at all*, even though the actual `/skills`
query then fails (confirmed in userA's setup.sh log: `agy skills: 0 records
listed; 0/28 deployed skills present by name`, i.e. the self-check was
useless on this home anyway). None of this is part of what `setup.sh`
manages or what the entrypoint's harness-stamp logic checks.

**Judgement on this finding**: the entrypoint.sh comment claims "byte-identical
deploy either way" — that is not literally true (the full `~/.gemini` tree
differs) and should be reworded; the accurate claim is "identical for every
artifact `setup.sh` manages." This is a low-severity doc-accuracy nit, not a
functional defect — if anything it strengthens the case for the fix, since
skipping the self-check also avoids seeding every fresh project with a
useless, host-fingerprinted agy CLI state directory that a real login-shell
`agy` invocation would create fresh anyway once a token exists. Not a FAIL
condition; flagged for a one-line comment correction.

### 4. Confirm nothing `setup.sh` needs lives in `/usr/local/bin`

`/usr/local/bin` in the image holds: `agy`, `go`, `gofmt`, `helm`,
`styrman-git-credential`, `styrman-setup`, `tofu`, `yq` (confirmed via
`ls -la /usr/local/bin` inside the image).

`grep -n "command -v\|which " image/harness/setup.sh image/harness/claude/setup.sh image/harness/antigravity/setup.sh`
→ exactly one hit, `antigravity/setup.sh:237: command -v agy`. No other
tool-presence guards in any of the three scripts.

`grep -nE '\b(go|gofmt|helm|styrman-git-credential|styrman-setup|tofu|yq)\b' <all three scripts>`
→ no matches. Confirmed: `agy` is the only `/usr/local/bin` binary these
scripts reference at all; trimming it off PATH cannot break anything else
setup.sh does (bash, rsync, python3, git, jq — all in `/usr/bin`, present
on the trimmed PATH).

### 5. Static checks

`bash -n image/entrypoint.sh` → exit 0, no output.

`shellcheck image/entrypoint.sh` → 3 pre-existing **SC2015 (info)** notes
(`A && B || C is not if-then-else`) at lines 89, 149, 227 — none touched by
this fix's diff (line 149's `rm -rf "${HSRC}"` cleanup and the PATH= change
are on different lines within the same block; the SC2015 note is on
pre-existing code, unchanged by this fix). No new warnings/errors
introduced. PASS.

### 6. Go regression (smoke check — no Go source changed)

- `go build ./cmd/styrman/` → exit 0, no output.
- `go vet ./...` → exit 0, no output.
- `go test ./...` → all packages `ok` (cmd/styrman, internal/backup, config,
  conversation, credbroker, credstore, files, gitrepo, history, httpapi,
  links, orchestrator, project, terminal, userstore); `cmd/wsprobe`,
  `internal/identity`, `web` `[no test files]` (pre-existing). PASS.

### 7. Live deployment health

- `docker ps` → `styrman-server` `Up 34 minutes (healthy)`, image
  `styrman-server:latest`.
- `docker run --rm --network styrman-ingress curlimages/curl -s -H
  "X-Styrman-Ingress: $TOKEN" -H "X-Caddy-Email: ostling.simon@gmail.com"
  http://styrman-server:8080/api/version` (token read from `deploy/.env`,
  never printed) → `{"version":"1.15.0"}`. PASS.

## Additional independent evidence — live cold-create on MY OWN throwaway project

Per the explicit instruction to prefer a real cold-create measurement over
trusting the requester's numbers, and per the CRITICAL CONSTRAINT to never
touch `test3321` (928e709a-…, running with an attached tmux session) or
`asdasd` (36782cc5-…, deliberately left on 1.13.1) — created and destroyed
my own throwaway project against the live production server, using the same
internal-ingress-header pattern as the `/api/version` check:

- `POST /api/projects {"name":"verify-rev2-throwaway"}` → `id
  786df840-a975-43cd-b020-f54368bf50f3`, `"state":"creating"`.
- `docker inspect --format '{{.State.StartedAt}}'` → `2026-09-01T07:29:55.820784240Z`.
- `docker logs -t` on the new container (mine, not an operator project —
  read-only log inspection, no exec into anyone else's container):
  - `07:29:57.437997501Z [styrman] agent harness deployed (version 1.15.0)`
    → **+1.617s** from container start.
  - `07:29:58.511626699Z [styrman] project ... ready (user=ostling-simon
    uid=1000)` → **+2.691s** from container start.
- This independently re-derives the operator's own claimed numbers
  (container start → ready = 2.69s, harness deployed 1.63s in) almost
  exactly (2.691s vs 2.69s; 1.617s vs 1.63s), on a genuinely fresh project
  with no pre-existing agy token — i.e. the exact scenario the original
  defect hit and my first-pass verification missed. R1 (< 5s target, < 15s
  hard limit) is met with large margin (2.69s, ~1.85x the target, ~5.6x
  under the hard limit — nowhere near the prior 4x-over-hard-limit
  regression).
- Cleaned up: `DELETE /api/projects/786df840-...?wipe=true` → `202`;
  polled (`docker ps -a`, up to 60s) until the container was gone (13
  iterations × 2s); confirmed removed from `GET /api/projects` listing and
  from `/var/lib/styrman/projects/ostling-simon/`. Note: the project's
  entry under `/var/lib/styrman/backups/ostling-simon/` was NOT removed by
  `wipe=true` — appears to be separate backup-retention behavior, unrelated
  to this fix and out of scope for this verification; not flagged as a
  defect here, only noted for completeness.
- At no point was `test3321` (928e709a-…) or `asdasd` (36782cc5-…) started,
  stopped, exec'd into, or otherwise modified. `docker ps` was used
  read-only to enumerate containers; the stopped `36782cc5` filesystem was
  not touched at all this pass (no `docker cp`, unlike the first pass).

## Judgement — is shipping without a latency assertion in `integration.sh` acceptable?

`integration.sh`'s readiness poll (`for i in $(seq 1 60); do docker exec
"${CN}" test -f /run/styrman/ready ... ; sleep 1; done`) and its delete-poll
(`seq 1 60`, `sleep 2`) both assert only eventual success within a 60-100s
window, with no elapsed-time capture or upper-bound assertion tighter than
"didn't time out entirely." Confirmed by direct inspection
(`.agents/docs/testing/styrman/integration.sh` lines ~71-75, ~250-256) —
this is exactly why the original 60-61s regression was invisible to it: the
original defect's ~61s hang sat right at the edge of (and could
intermittently exceed) the 60-iteration/1s-sleep = 60s window, making its
detection a coin-flip on timing jitter rather than a designed assertion.
Already recorded honestly as debt in `ROADMAP.md` under "Next."

**Judgement: acceptable to ship this specific fix without adding the
assertion now**, for three reasons: (a) I have independently, manually
reproduced both the defect (60.961s) and the fix (0.445s) at the container
level, plus re-derived the operator's real-world cold-create numbers on a
fresh project against production — this is at least as rigorous as an
automated assertion would be, just not durable; (b) the fix is a one-line,
narrowly-scoped PATH change with an obvious, well-understood mechanism, not
a change large enough to warrant blocking release on test-infrastructure
work; (c) the gap is already tracked with an accurate, actionable
description on `ROADMAP.md`, not silently dropped. **However**, this
exact gap already let one real R1-violating regression reach a "verified
PASS" state once — it should not be treated as low-priority debt. I
recommend it be pulled into the very next work cycle (a `docker exec ...
date` timestamp captured before the create call and after the ready-poll
loop breaks, asserted against a hard ceiling well under R1's 15s limit,
e.g. 10s, would have caught this outright) rather than left to accumulate
on `ROADMAP.md` indefinitely. Not a FAIL condition for this verification.

## Coverage of changed code

Same change set as the original release (`image/harness/**` vendor sync)
plus the one-line `PATH=` fix and comment in `image/entrypoint.sh`. No
Go/web/deploy source changed — the "changed code without covering test =
FAIL" rule has no packages to apply to. The standing gap (no automated
harness-correctness or boot-latency assertion anywhere in the repo) is
already the subject of the `ROADMAP.md` "Next" entry above; not re-flagged
as a fresh FAIL here since it predates and is orthogonal to this specific
fix, and is already tracked.

## Environment

- Tier: local only.
- OS: Linux 6.12.101+deb13-amd64 (Debian trixie), Docker plain (no Colima).
- `styrman-runtime:dev` image ID `1a8381a805a1`, created
  2026-09-01T09:17:15+02:00 (pre-existing at start of this verification,
  already rebuilt with the fix; not rebuilt by this agent).
- `styrman-server`: `Up`, healthy, serving `1.15.0`.
- Untouched throughout: `test3321` (928e709a-…, running, attached tmux) and
  `asdasd` (36782cc5-…, stopped, 1.13.1 harness by design) — no start,
  stop, exec, or filesystem access against either this pass.
- One throwaway project (`verify-rev2-throwaway`,
  786df840-a975-43cd-b020-f54368bf50f3) created and fully deleted by this
  agent via the live API for the cold-create timing measurement.
