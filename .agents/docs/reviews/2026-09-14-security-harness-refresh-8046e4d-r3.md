---
owner: security
date: 2026-09-14
---
# Security review — harness-refresh-8046e4d-r3   (2026-09-14)

Range reviewed: v1.30.1..worktree (round 3)

Narrow round, scoped by the dispatch to `git diff HEAD -- image/Dockerfile
image/entrypoint.sh` and its blast radius (`image/.dockerignore`, the build
call sites, the `sh-agent install` path, the boot block's timeouts). Round 2:
`.agents/docs/reviews/2026-09-14-security-harness-refresh-8046e4d-r2.md`
(not amended). Round 1: `…-security-harness-refresh-8046e4d.md`.

## Verdict: FINDINGS (three low)

The deletion is clean and the round-2 finding is **closed**: `git diff v1.30.1
-- image/Dockerfile` is **+7 / −0**, i.e. the seven comment lines and nothing
else — the `RUN` block is byte-identical to v1.30.1, so removing the dead
`test ! -e` changed no build behaviour at all, and the false sentence it
carried is gone. Both new comments are true statements about the code. What
is broken is smaller and one level out: the invariant those comments now rest
on — no `.git` in `/styrman/harness` — is enforced solely by
`image/.dockerignore`, and **nothing asserts `.dockerignore`'s contents on the
release-gate path**. `deploy/verify-commit.sh` already builds the runtime
image (`:192`) and runs a throwaway container against it (`:195`); it checks
skill/agent counts and not this. The only automated assertion left lives in
`.agents/docs/testing/styrman/integration.sh:134`, which needs Kata and a
hand run. Meanwhile the *cost* of a `.dockerignore` regression grew in this
same worktree: Go is on PATH in the runtime image (`image/Dockerfile:96-104`),
so a `.git` in the mirror flips `shouldRebuild` to true and every project boot
would `go build` inside that root-owned tree, and the install line no longer
carries `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1`, so the boot's git
calls read the tenant's `~/.gitconfig` again. No tenant boundary is crossed in
any of this — it takes an operator edit to `.dockerignore` first — hence low.

## Round-2 items

- **[low] `image/Dockerfile:169-170`, comment claiming a check the guard could
  not perform — CLOSED by deletion.** Verified as deletion, not repair: the
  runtime `RUN` no longer contains `test ! -e /styrman/harness/.git`
  (`git diff v1.30.1 -- image/Dockerfile` shows additions only), the new
  comment (`image/Dockerfile:166-172`) names `image/.dockerignore` as the
  mechanism, and the entrypoint's clause (`image/entrypoint.sh:139-143`) says
  the same in one sentence. Both are accurate against the source:
  `image/.dockerignore` does list `.git` **and** `**/.git` (so every depth,
  not just the context root); `IsCheckout` is exactly `os.Lstat(repo/.git)`
  (`image/harness/internal/harness/repo.go:75-78`); and on a plain copy the
  install path makes **no** git call — the one `Inspect` it reaches
  (`apply.go:79`) returns early through `IsCheckout`, and `knownCheckout` is
  skipped because `--repo` is passed. "The timeout is the whole defence" is a
  true description of the code as it now stands (see finding 3 for what that
  sentence does not bound). The round-2 sibling finding about `test ! -e`
  vs `os.Lstat` disagreeing on a dangling symlink is **moot** — the predicate
  it compared no longer exists.
- **Behaviour delta, stated plainly:** `image/Dockerfile` has none. The
  worktree's `image/entrypoint.sh` still carries one executable change against
  v1.30.1 — dropping `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` from
  the install invocation (`image/entrypoint.sh:155`), dispositioned in round 2
  — and it is now the load-bearing one, since the build-time backstop that
  round 2 credited is gone. Everything else in that file's ±34/−12 is comment.
  No live doc still claims the env guard exists (grep over `*.md`/`*.sh`: only
  historical `changes/`, `plans/`, and `testing/` entries, which are records).

- **eval-check residuals — the reasoning HOLDS, with one scope correction.**
  `REPO` is `$(cd "$(dirname "$0")/../../../.." && pwd)`
  (`integration.sh:40`) — the operator's own checkout path on the operator's
  own machine, no tenant input, so the single-quote break-out stays a
  self-inflicted breakage class, not a boundary. `/styrman/VERSION` is
  root-written at build from `ARG STYRMAN_VERSION` — but it is **not**
  unreachable by a tenant in general: every project user has passwordless
  `sudo` (`image/entrypoint.sh:55`) and the docker group (`:57`), so the guest's
  root can rewrite `/styrman/VERSION` inside *its own* container. The residual
  is safe for a different reason than the one given: the container the suite
  reads is the suite's own, created and destroyed by the run, and — since the
  round-2 escaping fix — a hostile `$v` can only contribute `grep -F` pattern
  text, so even then the worst case is a **false PASS of a test check, never
  execution**. Conclusion unchanged (leave both residuals); premise corrected.

## Findings (new)

- **[low / confidence: high]** `deploy/verify-commit.sh:192-195` (invariant
  owned by `image/.dockerignore:2-3`, relied on by `image/Dockerfile:166-172`
  and `image/entrypoint.sh:121-123,139-143`) — **the no-`.git` invariant is now
  asserted nowhere on the release-gate path.** A one-line regression to
  `image/.dockerignore` (plausible: the mirror refresh is a git clone, and
  "why is `.git` excluded" is a natural edit) ships a checkout-shaped mirror.
  Consequence, in this worktree specifically: Go is installed at
  `/usr/local/bin/go` in the runtime stage (`image/Dockerfile:96-104`), so
  `shouldRebuild(goOnPath, child, ownCheckout)`
  (`image/harness/cmd/sh-agent/main.go:128-131`,
  `main.go:389-392`) becomes true, every project boot enters `Reinstall`
  (`repo.go:143-158`: `go build` into `<repo>/.agents/scratch/…` in a
  root-owned, `go-w` tree) under a 20 s cap, and `Inspect`'s `git rev-parse`
  now runs **without** the removed `GIT_CONFIG_GLOBAL`/`NOSYSTEM` env, i.e.
  against the tenant's own `~/.gitconfig` — the precise 2026-09-03 stall.
  Not attacker-initiated (it needs the operator edit first), so low; but the
  gate already builds this image and runs a container against it, so the fix is
  one line beside `deploy/verify-commit.sh:195`:
  `docker run --rm --entrypoint sh "${RIMG}" -c 'test ! -e /styrman/harness/.git'`,
  or run `status --repo /styrman/harness` instead of `list` and grep the
  `plain copy, not its own git checkout` line the suite already greps at
  `integration.sh:134`. Assert the consequence where the gate can see it.
- **[low / confidence: medium]** `image/Dockerfile:166-168` and
  `image/entrypoint.sh:121-122,140-142` — **a build-time property is written
  as a runtime one.** "The mirror in the image is always a plain copy", "no
  refresh can ship one here", "/styrman/harness is always one" are true of the
  image as built; they are not true of the running container. The project user
  is root in the guest (`image/entrypoint.sh:55`, NOPASSWD sudo) and
  `/styrman/harness` lives in the container filesystem, which survives
  stop/start, so a tenant can create `/styrman/harness/.git` and reinstate both
  the `go build` and the `~/.gitconfig`-reading git call on their next boot.
  Nothing escalates — that install step runs as the same tenant under
  `runuser` and the tenant is already root in their own VM, so the effect is a
  ≤20 s self-inflicted boot delay plus a `WARN` — which is exactly why the
  timeout sentence is the right defence. Record it as the expiry condition,
  alongside round 2's items 3–5: **void the day any part of this block runs as
  root, or the mirror is read by anything privileged.** Remediation: add "at
  build" to those three sentences so a later reader does not inherit a runtime
  guarantee that was never made.
- **[low / confidence: medium]** `image/entrypoint.sh:149-153` — **"in any
  case" overstates what four sequential caps bound.** The rewritten comment
  keeps the claim that the 20 s cap "must stay inside R1's 15 s soft budget as
  far as possible and the server's 90 s ready wait in any case", but this block
  now holds **four** `timeout 20` steps (`:154` migrate, `:155` install, `:168`
  claude MCP, `:171` agy MCP) = **80 s** worst case, ahead of
  `/run/styrman/ready`, against `o.waitReady(ctx, id, 90*time.Second)`
  (`internal/orchestrator/orchestrator.go:445` and `:760`) — which also has to
  cover everything else in the boot. The four steps pre-date this diff, but the
  diff rewrites this very sentence and (per the finding above) reopens a stall
  path that was previously closed by the env guard, so the arithmetic is now
  load-bearing. Self-inflicted and bounded (a failed boot is retried, the stamp
  stays unwritten), hence low. Remediation: state the cumulative bound
  (4 × 20 s) in the comment, or give the whole block one shared deadline.

## `.dockerignore` — could a normal build open a hole? (asked explicitly)

No hole found; the honest gap is the missing assertion above, not the
matching. Checked:

- **Every build call site uses context = `image/`**: `deploy/verify-commit.sh:192`
  (`docker build … image/` inside a detached worktree), `deploy/host-setup.sh:255`
  and `:392`, `.agents/docs/testing/styrman/ensure-test-image.sh:48`,
  `image/README.md:29` (`docker build … .` from inside `image/`), AGENTS.md's
  documented command. `.dockerignore` is resolved at the **context root**, so
  `image/.dockerignore` applies to all of them.
- **Context = repo root fails closed**, twice over: `COPY harness/
  /styrman/harness/` has no source at the repo root (build error), and the root
  `./.dockerignore` lists `.git` as well.
- **Legacy builder vs BuildKit**: both apply the context-root `.dockerignore`
  and both support `**`, so `**/.git` covers `harness/.git` either way; the
  exclusion happens when the context is assembled, so `COPY` cannot see the
  directory at all. One BuildKit-only sharp edge worth knowing: a file named
  `image/Dockerfile.dockerignore` would take **precedence** over
  `image/.dockerignore` and silently replace the whole list — including
  `**/*.pem` and `**/*.key`. No such file exists (`find -name '*.dockerignore'`
  → `./.dockerignore`, `./image/.dockerignore`).
- **`--build-context`**: cannot replace the default context, so it cannot
  smuggle a `.git` into `COPY harness/`. It *can* override a stage by name —
  `--build-context harness-builder=…` would replace the compiled `sh-agent`
  (`image/Dockerfile:8-13`) — a supply-chain note, not this invariant, and no
  call site in the repo passes the flag.
- **Git-URL context**: `.dockerignore` still applies, and
  `BUILDKIT_CONTEXT_KEEP_GIT_DIR=1` would keep only the *context root's*
  `.git`, never `harness/.git`. Not a path anyone here uses.
- **Committed tree**: git cannot track a nested `.git`, so only a working tree
  can carry one, and `deploy/verify-commit.sh:92-99` (SHA256SUMS list diff +
  symlink refusal) still runs over the commit's worktree.

## Checked (this round only)

- **Secrets** — none in the delta; the two files add comments and remove two
  env assignments. `image/.dockerignore` still excludes `*.env`, `**/*.pem`,
  `**/*.key` (see the `Dockerfile.dockerignore` note above for the one way that
  list could be bypassed). Install's stdout is still discarded
  (`image/entrypoint.sh:156`), so no home content reaches the boot log; the
  `agent harness deployed (version …, styrman-harness …)` line (`:180`) prints
  only the build version and the mirror's short commit, and matches the string
  `integration.sh:136` greps.
- **Injection** — no new interpolation. The stamp write still passes the
  version as an argument, never into an `sh -c` string
  (`image/entrypoint.sh:177-179`); the MCP block's `sh -c` carries a constant.
  The `eval` residuals are `integration.sh`, addressed above.
- **Authz/authn** — unchanged. Both boot steps still run as the project user
  (`runuser -u "${USERNAME}"`), never root; no new root-side invocation of
  `sh-agent` in `image/` or `deploy/`.
- **Crypto** — nothing cryptographic in the delta.
- **Dependencies** — none added or changed; the harness module still has no
  `require` block and the builder keeps `GOPROXY=off GOTOOLCHAIN=local`
  (`image/Dockerfile:13`). `govulncheck` **not re-run this round**: the delta is
  comments plus two env assignments in a shell script, no Go source, so round
  1's run inside the pinned builder image still describes the code (and the
  change entry records `deploy/vulncheck.sh` rc 3 = the two accepted
  docker-daemon findings, `image/harness` clean). `npm audit` not applicable —
  nothing under `web/` in this range.
- **GCP surface** — none; self-hosted project.
- **No commands run against the image** — I ran no build and no container this
  round (see Not examined). All build behaviour above is read from the
  Dockerfile, `.dockerignore`, and the call sites; round 2's empirical
  `.dockerignore` probe is the observation this rests on.

## Not examined

- **The dispatch's own evidence was taken on trust, not reproduced**: the 12:33
  rebuild, `passed=94 failed=0 skipped=3`, and
  `docker run --rm --entrypoint sh styrman-runtime:test -c 'test ! -e
  /styrman/harness/.git'`. I neither rebuilt the image nor ran the suite; the
  first finding is precisely that this check is a hand run, not a gate step.
- Everything outside the two named files: `integration.sh`'s other 93 checks,
  `deploy/verify-commit.sh` beyond lines 92-99 and 175-205, the harness mirror's
  prose, `image/configs/harness-migrate.sh`, the MCP block's own behaviour,
  and the server/ingress modules — rounds 1 and 2 cover them, and nothing in
  this delta moved them.
- The upstream `styrman-harness` checkout; `COMMIT` remains an attestation.
- Round-2's `SHA256SUMS`-not-yet-in-git staging note was not re-checked.
