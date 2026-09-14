---
owner: security
date: 2026-09-14
---
# Security review — harness-refresh-8046e4d-r2   (2026-09-14)

Range reviewed: v1.30.1..worktree (round 2)

Round 1: `.agents/docs/reviews/2026-09-14-security-harness-refresh-8046e4d.md`
(not amended). This round judges the six dispositions and reviews the delta
that carries them: `image/Dockerfile`, `.agents/docs/testing/styrman/integration.sh`,
`image/harness/VENDORED.md`, `image/harness/SHA256SUMS`, plus the comment
rewrites in `image/entrypoint.sh` and `deploy/verify-commit.sh`.

## Verdict: FINDINGS

All six dispositions hold on the facts — every claim in the two rebuttals I
was asked to check is true against the source, and the strongest leg of the
`dev-setup.md` rebuttal is one the main agent did not lean on: the project
user is in the `docker` group with an in-guest dockerd
(`image/entrypoint.sh:57`), which is already root-equivalent inside the
guest, so `sudo` grants nothing new and the round-1 medium is fairly a low
here. Nothing reopens. The one real defect in the delta is that the new
build-time guard does not catch the mistake its own comment says it catches:
`image/.dockerignore` strips `.git` and `**/.git` from the build context, so
`test ! -e /styrman/harness/.git` (`image/Dockerfile:172`) can never fire on
a hand refresh that leaves a `.git` in `image/harness/` — I verified that
empirically with a minimal build. The guard is still worth keeping (it
catches a future `.dockerignore` regression, which is the property the
entrypoint's dropped git guards actually rest on), but the comment at
`image/Dockerfile:169-170` is a description that outlives its behaviour.

## Round-1 findings — disposition per item

1. **[was medium] `image/harness/context/dev-setup.md:8` sudo sentence — REBUTTED.
   Disposition holds; severity is fairly LOW in the crew context.** Every
   claim checked and true: the sudoers line is written verbatim
   (`image/entrypoint.sh:55`, `${USERNAME} ALL=(ALL) NOPASSWD: ALL` into
   `/etc/sudoers.d/${USERNAME}`, mode 440); one microVM per project is
   *enforced*, not assumed — `HostConfig.Runtime = o.cfg.Runtime`
   (`internal/orchestrator/orchestrator.go:1038`), default `mvm`
   (`internal/config/config.go:167`), and `config.Load` refuses any non-VM
   runtime unless `STYRMAN_ALLOW_UNSAFE_RUNTIME=1`
   (`internal/config/config.go:250-263`, with the reason spelled out: the
   containers carry `CAP_SYS_ADMIN` + `apparmor=unconfined`); R12 is as
   quoted (`REQUIREMENTS.md:73-81`); the never-edit-the-mirror rule is real
   (ADR-046 decision + ADR-062 D1, VENDORED.md the single exception); the
   credential broker identifies the caller by source IP / bridge position and
   never by uid (`internal/credbroker/broker.go:2-4,64-66`), so root and the
   project user receive identical credentials. **Root inside the VM reaches
   nothing the project user cannot** — and the decisive reason is the one the
   rebuttal only mentions in passing: `usermod -aG docker "${USERNAME}"`
   (`image/entrypoint.sh:57`) over the per-project dockerd is already
   root-inside-the-guest by construction. So the sentence changes an agent's
   *manners*, not its reach, and the indirect-prompt-injection delta it buys
   is close to zero. Carrying it to the operator for the harness repo is the
   right call, and ADR-046/062 correctly forbid patching the mirror.
   One overstated supporting claim, recorded so it is not reused as a
   premise: "the host firewall isolates the VM from the host and LAN
   regardless of what runs as root inside it" is true only in the default
   sandbox mode (`sdb-<hash>`); in the admin-toggled LAN mode (`lan-<hash>`)
   the project is matched by no styrman rule at all and reaches the LAN and
   the host including its sshd (AGENTS.md, ADR-067). The conclusion survives
   — network reach is not uid-dependent — but the sentence as written is
   wrong for half the modes.

2. **[low] entrypoint dropped the git-config guard — FIXED, with a caveat.
   Closed for the risk that was reported; the fix's comment is inaccurate.**
   The guard is inside the `&&` chain (`image/Dockerfile:171-172`), so a
   failure does fail the build, and `deploy/verify-commit.sh:192` builds the
   runtime image from the commit, so the gate does execute it. But it cannot
   catch "a hand refresh that ships a `.git`": `image/.dockerignore` lists
   `.git` and `**/.git`, so such a directory is never in the build context
   and `COPY harness/ /styrman/harness/` never brings it in. Verified, not
   reasoned: a minimal `busybox` build with the same two ignore lines,
   `harness/.git/config` present on disk, `COPY harness/ /h/` and
   `RUN test ! -e /h/.git` — the guard printed PASS. What the line really
   asserts is *no `.git` in the image*, i.e. it catches a future removal or
   typo of the `**/.git` line; that is the property the entrypoint's dropped
   `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` actually depend on, so
   the boot-time hazard (a `git rev-parse` against a tenant `~/.gitconfig`,
   and `shouldRebuild` → `go build` in a root-owned tree) is closed.
   **Does `verify-commit.sh` therefore cover a hand refresh that ships a
   `.git`? Yes — but through two other checks, not through this one, and it
   would in fact have covered it before today's change.** (a) Its mirror list
   diff and symlink refusal (`deploy/verify-commit.sh:92-99`) run over the
   commit's own worktree, where every `.git/*` file would appear as unlisted
   against `SHA256SUMS` → FAIL. (b) git cannot track a nested `.git` at all,
   so the committed tree cannot carry one; only the *working tree* can, and
   there `.dockerignore` neutralises it. Net: keep the line as second-line
   defence, fix the comment (see Findings).

3. **[low] `apply.go` manifest paths without `filepath.IsLocal` — REBUTTED.
   Disposition holds.** Re-checked: no `IsLocal` anywhere in
   `image/harness/internal/harness/` (no code change, as a rebuttal implies).
   The trust argument is correct for crew — `~/.config/sh-agent/state.json`
   is the tenant's own file, and every `sh-agent` invocation in `image/` and
   `deploy/` runs as the tenant (`image/entrypoint.sh:153-154`,
   `runuser -u "${USERNAME}"`; the only other one is
   `deploy/verify-commit.sh:195`, a throwaway `docker run --rm … list`). No
   privilege or tenant boundary is crossed. Carrying it upstream is right.
   Record the expiry condition: **this rebuttal is void the day `sh-agent`
   runs as root in the image** — that single change turns a
   same-user file removal into a root-side path traversal.

4. **[low/med] frontmatter `name:` → path (`inventory.go`) — REBUTTED. Same
   disposition, and it holds for the same reasons**, plus one that is
   specific to it: the mirror is root-owned and `chmod -R go-w`
   (`image/Dockerfile:173`), so no tenant can plant a `SKILL.md` there. Same
   expiry condition as 3, and additionally: it becomes live in-repo only if
   styrman ever installs from a tenant-writable tree.

5. **[low/low] predictable `Reinstall` build dir (`repo.go:143`) — REBUTTED.
   Disposition holds; the supporting sentence is half right.** Reachability
   is correctly argued and confirmed: `shouldRebuild(goOnPath, child,
   ownCheckout)` requires `ownCheckout`
   (`image/harness/cmd/sh-agent/main.go:390-392`), `IsCheckout` is an
   `os.Lstat` of `<repo>/.git` (`repo.go:75-78`), false in the image ⇒
   `Reinstall` never runs there. "Now asserted at build by item 2" is the
   half that is not right: per finding 2 the assertion holds over the *image*
   and is made by `.dockerignore`, with the new `test` as backstop — the
   conclusion is unchanged, the mechanism is not what the sentence says.

6. **[low/low] `eval` of container-sourced strings — FIXED. Disposition holds
   under `check()`'s `eval` (`integration.sh:52`).** The command
   substitutions are now escaped (`\$(...)`), so they execute inside the
   single `eval` pass instead of being interpolated into the string before
   it; their results live in `c` and `v` and are used only as `"$c"` / `"$v"`
   inside double quotes. `eval` parses once and expands after parsing, and
   expansion results are not re-scanned for shell syntax, so a hostile
   `/styrman/VERSION` can contribute pattern text to `grep -F` but no longer
   shell text. `[ ${#c} = 7 ]` and `[ -n "$v" ]` fail closed on a truncated
   read, `grep -F … >/dev/null` (never `-q`) is right under this script's
   `set -o pipefail` (BUG-057), and sourcing the commit from
   `${REPO}/image/harness/COMMIT` ends the compare-a-constant-with-itself
   problem for the half that matters. Two residuals below, both low/low,
   neither reopening the item.

## Findings (new, in the delta)

- **[low / confidence: high]** `image/Dockerfile:169-170` — **the guard's
  comment claims a check the guard cannot perform.** "A refresh that ships a
  `.git` fails here, not at every project boot" is false:
  `image/.dockerignore`'s `.git` / `**/.git` keep such a directory out of the
  build context entirely, so `test ! -e /styrman/harness/.git`
  (`image/Dockerfile:172`) passes in exactly that case (verified with a
  minimal `busybox` build reproducing the same ignore lines and a planted
  `harness/.git/config`). Exploitable in principle only as trust misplaced: a
  future session reads the comment, believes the tree-level invariant is
  enforced at build, and skips the check that really enforces it
  (`deploy/verify-commit.sh:92-99`'s unlisted-file diff over the commit's
  worktree). Remediation: restate it as what it is — "keeps a `.git` out of
  the IMAGE; a second line behind `.dockerignore`'s `**/.git`, so a
  `.dockerignore` regression fails the build. The source tree is policed by
  `deploy/verify-commit.sh`'s SHA256SUMS list diff."

- **[low / confidence: medium]** `image/Dockerfile:172` vs
  `image/harness/internal/harness/repo.go:76` — **the two predicates disagree
  on a dangling symlink.** `test ! -e` follows links, so a `.git` symlink
  pointing nowhere passes the build guard; `IsCheckout` uses `os.Lstat`, so
  the same entry makes `sh-agent` treat the mirror as a checkout. Unreachable
  today (the `.dockerignore` exclusion, plus `deploy/verify-commit.sh:94-98`
  refusing any symlink in the mirror), but a guard that does not match the
  predicate it guards is worth one token: `test ! -e … && test ! -L …`.

- **[low / confidence: low]** `.agents/docs/testing/styrman/integration.sh:132`
  — **`${REPO}` is still interpolated into the `eval`'d string**, inside
  single quotes (`'${REPO}/image/harness/COMMIT'`). A single quote anywhere
  in the checkout path breaks out of that quoting and the remainder is
  executed by `eval` (`integration.sh:52`). `REPO` is
  `$(cd "$(dirname "$0")/../../../.." && pwd)` — operator-controlled, so no
  boundary is crossed; it is the same class the fix removed for `v`, left
  behind. Remediation: export it and let the eval'd text reference `"$REPO"`.

- **[low / confidence: low]** `.agents/docs/testing/styrman/integration.sh:132`
  — **`[ -n "$v" ]` does not cover an embedded newline.** If
  `/styrman/VERSION` ever held a multi-line value, `grep -F` treats the
  pattern as several fixed strings, and an empty line among them matches
  every log line — a false PASS, the precise failure mode the length check
  was added to prevent. `VERSION` is root-written at build time from
  `STYRMAN_VERSION`, so this is a pattern note. Remediation: `case "$v" in
  *[!A-Za-z0-9._-]*) false;; esac` or compare `${#v}` against a
  single-line read.

- **[info]** `.agents/docs/testing/styrman/integration.sh:132` — the version
  half of the new check is still self-referential: `$v` is the container's
  `/styrman/VERSION`, and the boot line it matches prints `HARNESS_VER`,
  which is `cat /styrman/VERSION` (`image/entrypoint.sh:131,177`). Only the
  commit half cross-checks the image against the tree. The comment is honest
  about the commit and silent about the version; one clause would close it.

- **[info]** `image/harness/SHA256SUMS` lists five paths that are **not yet in
  git**: `cmd/sh-agent/main_test.go`, `skills/golang/references/golang-samber.md`,
  `skills/golang/references/golang-structure.md`, `skills/review/SKILL.md`,
  `skills/init-project/assets/agents-tree/docs/retros/.gitkeep`. The manifest
  therefore describes the working tree, not a commit. This fails CLOSED —
  `deploy/verify-commit.sh:92-99` would report them as missing entries — so
  it is a staging note, not a hole: all five must be staged in the same
  commit as `SHA256SUMS`, or the release gate blocks.

## Checked (delta only; round 1 covered the full range)

- **Mirror integrity after the `VENDORED.md` edit** — re-run, because that
  file is inside the hashed set: `sha256sum -c --quiet SHA256SUMS` in
  `image/harness` exits **0**; the list diff (`find . -type f ! -name
  SHA256SUMS` vs `cut -c67- SHA256SUMS`) is **empty**, exit 0; `find . -type
  l` is **empty**. `SHA256SUMS` was regenerated with the edit, and
  `VENDORED.md` is ADR-046's one permitted hand-edit. The edit itself is
  documentation (the BUG-058 note about the mirror's red `TestDryRunNeverBuilds`,
  and the plain-copy description) — no instruction, no URL, no command.
- **Secrets** — nothing added in the delta; no credential, token, or key in
  the four changed files. `image/.dockerignore` still excludes `*.env`,
  `**/*.pem`, `**/*.key`. The new boot-log check reads `/styrman/VERSION`
  and `COMMIT` only; `install`'s stdout stays discarded
  (`image/entrypoint.sh:154`), so no home content reaches the log.
- **Injection** — the one `eval` is finding 6 above, now materially safer;
  two residual interpolations recorded as low/low. No new `exec.Command`, no
  `sh -c` with interpolated data. The Dockerfile `RUN` interpolates only
  `${STYRMAN_VERSION}`, an `ARG` with the same exposure as before.
- **Authz/authn** — untouched in the delta. `sh-agent` still runs only as the
  project user (`runuser -u "${USERNAME}"`, `image/entrypoint.sh:153-154`);
  grep over `image/` and `deploy/` finds no root-side invocation. The
  runtime-is-a-VM assertion (`internal/config/config.go:250-263`) is intact.
- **Crypto** — nothing hand-rolled, nothing cryptographic changed.
  `SHA256SUMS` remains an integrity manifest checked with `sha256sum -c`.
- **Dependencies** — no dependency change in the delta; `image/harness/go.mod`
  still has no `require` block, and the builder stage keeps `GOPROXY=off
  GOTOOLCHAIN=local`. `govulncheck` was **not re-run this round**: nothing in
  the delta touches Go source (`git diff HEAD --stat` on the delta files is
  Dockerfile, shell, and markdown only), so round 1's result inside the
  pinned builder image — `No vulnerabilities found.`, exit 0 — still
  describes the code. `npm audit` not applicable; nothing under `web/` is in
  this range.
- **GCP surface** — none touched in the delta.
- **Empirical check run** — one throwaway `docker build` in the session
  scratchpad against a `busybox` context, to settle finding 2 by observation
  rather than by reading Docker's matching rules. No repo file created or
  modified; image `dockerignore-probe:tmp` is disposable.

## Not examined

- The upstream `styrman-harness` checkout — `COMMIT` remains an attestation,
  exactly as `VENDORED.md` says.
- No live run: the runtime image was not rebuilt from `image/`, and
  `.agents/docs/testing/styrman/integration.sh` was not executed (needs the
  Kata runtime and a free port). The two new checks were read and reasoned
  about; their *passing* is still unverified, and the boot-log string was
  matched against `image/entrypoint.sh:177` by eye. `deploy/verify-commit.sh`
  was read, not executed.
- The bulk prose of the harness refresh (skills/agents markdown) — reviewed in
  round 1, unchanged since; only `VENDORED.md` moved in this delta.
- `image/configs/harness-migrate.sh`, the MCP registration block, and the
  ingress/server modules — unchanged in this delta.
