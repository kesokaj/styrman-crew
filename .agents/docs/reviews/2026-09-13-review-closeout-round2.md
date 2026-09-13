---
owner: reviewer
date: 2026-09-13
---
# Review — close-out fixes, round 2 (firewall callers, styrman-ctl verdict, bump-pins hardening)   (2026-09-13)

Range reviewed: 336a5d1..48d930b
(`git diff 336a5d1..48d930b -- deploy image internal .agents/docs/testing/styrman/integration.sh`;
one commit, 48d930b, plus the README count line from the same commit, read for
closure of the predecessor's finding 7. Blobs read from the range, not the worktree.)

Answering `.agents/docs/reviews/2026-09-13-review-closeout.md` (9 findings) and
`.agents/docs/reviews/2026-09-13-security-closeout.md` (5 findings).

## Verdict: APPROVE WITH NITS

Every blocking finding from both predecessor reports is closed, and the closures are
real rather than cosmetic — I checked the sed regex, the `set -e` semantics and the
verdict glob by running them, not by reading them. The worst thing left is a hole the
fix itself opened: `deploy/firewall/test.sh`'s `run() { … || true; }` discards the
script's exit status everywhere, and line 106's `check "cleanup is idempotent"` quietly
lost the `&&`-gated status assertion it used to carry. The exit status of
`styrman-firewall.sh` is now consumed by three separate callers (this installer, this
suite, `styrman-ctl`'s DEGRADED rule), and after this commit nothing in the repo asserts
it in either direction. Second: `image/bump-pins.sh`'s "atomic" rewrite uses `mktemp`,
which on this host lands on tmpfs while the repo is on nvme — so `mv` is copy+unlink,
not a rename, and it carries mktemp's `0600` onto both Dockerfiles (664 → 600), a change
git does not record. Neither blocks the release; both are one-line fixes.

## Findings

- **[medium / confidence: high]** `deploy/firewall/test.sh:58` and `:106` — `run() { bash
  "${SCRIPT}" || true; }` closes the predecessor's finding 2 but throws the status away
  at every call site, including the one that was using it. Line 106 reads
  `check "cleanup is idempotent" "run >/dev/null 2>&1 && [ \$(tagged_count) -eq 0 ]"`:
  before this commit the `&&` made that check assert *both* "the script exits 0 on a
  clean host" and "the count is 0"; now the left operand is a constant `true` and only
  the count is asserted, under an unchanged check name. Net effect across the suite: no
  test anywhere asserts that `styrman-firewall.sh` exits 0 on a healthy host, and none
  asserts it exits 1 when it has WARNS — while three callers added or changed in this
  very range branch on exactly that status (`install.sh:111` `|| log`, this `run()`'s
  `|| true`, `deploy/styrman-ctl:333`'s `*/failed` rule via systemd). A regression that
  dropped the `WARNS`→`exit 1` block, or one that made the script exit 1 unconditionally,
  passes this suite and `go test ./...` silently. `internal/config/firewall_script_test.go`
  pins only the `warn "xt_physdev unavailable` text, not the status.
  *Fix:* keep the status and assert it — `run() { bash "${SCRIPT}"; RUNRC=$?; return 0; }`,
  then `check "a clean run exits 0" "[ \${RUNRC} -eq 0 ]"` after section 4's clean run and
  `check "the warning run exits 1" "[ \${RUNRC} -eq 1 ]"` after the tagged-DROP run at
  line 112. Two lines, and the contract three callers depend on becomes testable.

- **[low / confidence: high]** `image/bump-pins.sh:95,110` — the comment claims "move both
  into place together — never a half-rewritten tree", and the commit message says
  "rewrites temp copies atomically". `mktemp` with no template writes to `$TMPDIR`/`/tmp`.
  Measured on this host: `/tmp` is `tmpfs`, the repo is `/dev/nvme0n1p2`, so `mv -f` is a
  cross-device copy-then-unlink, not `rename(2)`. Two concrete consequences: (a) the
  window the fix was meant to close is narrowed, not closed — an ENOSPC or an interrupt
  between `mv "${tmp_img}"` and `mv "${tmp_srv}"` still leaves `image/Dockerfile` new and
  `deploy/Dockerfile.server` old, and now with no message at all, since `trap - EXIT`
  has not run yet but the temps are already consumed; (b) the copy carries mktemp's mode:
  verified locally that `mktemp` yields `600` and a cross-device `mv` lands `600` on the
  target, so both Dockerfiles go from `664` to `600`. Git tracks only the exec bit, so
  that permission change is invisible in `git diff` and in every later review — the
  BUG-007 family (a change nobody can see in the normal review path). It also puts
  generated files in `/tmp`, which the global AGENTS.md forbids ("never `/tmp`").
  *Fix:* `tmp_img="$(mktemp "${IMG}.XXXXXX")"` / `tmp_srv="$(mktemp "${SRV}.XXXXXX")"` —
  same directory, so `mv` is a real atomic rename — plus
  `chmod --reference="${IMG}" "${tmp_img}"` before the move.

- **[low / confidence: high]** `.agents/docs/specs/styrman/runtime-image.md:37` — the
  security report's finding 3 asked for the honest trust statement in the script header
  **and** mirrored in the spec. The header now says it plainly (`bump-pins.sh:12-21`:
  "TLS to each publisher plus the publisher's own published sums. Nothing here verifies a
  signature"), but the spec still reads "`--apply` rewrites the ARG lines and the
  `FROM …@sha256` digests in both Dockerfiles, after which `docker build image/` **proves
  the new sums**", three lines below "never trust a sha256 computed only from the
  download itself". A reader of the spec alone still takes the stronger reading the
  finding named. Half-closed. *Fix:* one clause in `runtime-image.md` matching the
  script's own wording (what the build proves is that the archive matches what the
  publisher listed minutes earlier; no signature is checked).

## Predecessor findings, one by one

Review report (`2026-09-13-review-closeout.md`):

| # | Finding | Status |
|---|---------|--------|
| 1 | `install.sh` start-before-enable aborts the installer | **Closed** — `install.sh:110-112`: `run systemctl start … \|\| log "WARN: …"`. Both `enable`s at :116-117 are now unreachable-by-failure. |
| 2 | `test.sh` bare `run` aborts the suite | **Closed**, with a new hole — `test.sh:58`; see finding 1 above. |
| 3 | "applied" without proving a line changed | **Closed** — `bump-pins.sh:104-109`: `grep -qxF` for all five ARG lines, `grep -qsF` per digest, each aborting with "rewrite did not land" *before* any `mv`. The bare-`FROM` case it named is now handled by the second `s` command at :101. |
| 4 | digest failure `continue`d past, green summary | **Closed** — `:69` sets `fail_read=1`, `:74` aborts before the report. A run that could not read a source now exits 1 instead of printing "everything is current". |
| 5 | `npm` missing from the tool loop; `set -e` kills the guard | **Closed** — `:32` adds `npm awk`; every publisher read is `\|\| true`-terminated or `2>/dev/null`-piped into a shape check, so the guard at `:74` is reachable for exactly the failures it names. |
| 6 | partial application possible | **Partly closed** — temp copies + verify + `mv`; residual window is finding 2 above. |
| 7 | README counts stale | **Closed** — `README.md:15` now reads 95 checks / `passed=92`; `grep -c 'check "'` on the script is 95. Matches. |
| 8 | integration check passes vacuously | **Closed** — `integration.sh:131` binds `v=` first. Verified the semantics: for a simple assignment the exit status *is* the command substitution's, so a dead container fails the `&&` chain; `[ -n "$v" ]` catches an empty-but-successful read; and `check(){ if eval "$2"; …}` puts it in an `if`, so `set -e` does not abort. |
| 9 | `styrman-ctl` reports `enabled/failed` as OK | **Closed, with a test** — `deploy/styrman-ctl:333`. Ran the glob matrix: `enabled/failed`→rc=1 (was 0), `enabled/inactive`/`enabled/active`→rc=0, `absent/inactive`→rc=1. `go test -run TestStyrmanCtlStatus ./internal/config` passes. |

Security report (`2026-09-13-security-closeout.md`):

| # | Finding | Status |
|---|---------|--------|
| 1 | publisher/registry strings steer a `sed` program | **Closed** — `:40-42` `is_version`/`is_sha256`/`is_digest` gate *every* interpolated value before it reaches sed, and the delimiter moved to `\|`, which none of the accepted shapes can contain (digits, dots, lowercase hex, `sha256:`). A value that fails the shape sets `fail_read=1` and the run aborts before any rewrite. The `&`-in-replacement class is closed by the same shapes. The only value still unvalidated is `${tag}`, which comes from the repo's own `FROM` lines — the security report already called that cosmetic and it remains so. |
| 2 | installer aborts before `enable` | **Closed** — see review #1. Same shape as `deploy/update/styrman-update:578`. |
| 3 | trust model stated more strongly than it is | **Half-closed** — header fixed, spec not; finding 3 above. The `.asc` signature check is not implemented (not claimed to be, now). |
| 4 | `CLAUDE_CODE_VERSION` adopted with no checksum | **Open, not addressed and not struck in writing** — the suggested `dist.integrity` line is absent, and ADR-070 records the *pre-round* strikes, not this one. Low severity and pre-existing design (the Dockerfile pins only a version), but it is the one predecessor finding this commit neither fixes nor records a reason for. |
| 5 | `npm` not in the tool loop | **Closed** — see review #5. |

## The specific mechanisms the dispatch asked about

Checked by running them, not by reading:

- **`\?` in GNU sed BRE, and the `AS` capture.** Works. GNU sed 4.9 on this host:
  `s|^FROM tag\( AS .*\)\?$|FROM tag@DIGEST\1|` turns `FROM golang:1.25 AS builder` into
  `FROM golang:1.25@sha256:… AS builder` and `FROM debian:trixie` into
  `FROM debian:trixie@sha256:…`; the non-participating group expands to the empty string,
  so no stray text is appended. `debian:trixie` leaves `FROM debian:trixie-slim` untouched
  (the group needs a literal space+`AS`, or EOL). The two `s` commands in one invocation
  do **not** double-apply: after the first rewrites `FROM tag@sha256:…`, the second's
  pattern needs `@`-free text after the tag, which no longer matches.
  *Portability note, not a finding:* `\?` is a GNU extension — on BSD/macOS sed it matches
  a literal `?`. Moot today, because `sed -i` with no suffix argument already fails on
  macOS, so the script is Linux-only either way and fails loudly, not silently.
- **`set -e` + `declare -A` + `continue` in the `while read` loop.** Safe. Ran the exact
  shape under `set -euo pipefail` with and without `inherit_errexit`: `[ -n "${M[$t]:-}" ]
  && continue` does not exit when the test fails, because the failing command is a
  non-final member of an `&&` list; the map is still populated in the parent shell (the
  loop body is not a subshell — the input is a process substitution, not a pipe).
- **`install.sh`'s `run … || log` under `--dry-run`.** Fine. `run()` is
  `if [ "${DRY}" = 1 ]; then echo "+ $*"; else "$@"; fi` — the echo returns 0, so `log` is
  not reached and the printed plan is unchanged. Note the dry-run plan therefore never
  shows the WARN branch, which is correct (it is a runtime outcome, not a step).
- **`test.sh`'s `run(){ … || true; }` against the later `{ run 2>&1 || true; }`.** The
  later guards (`:112`, `:118`) are now redundant but harmless — `|| true` on a command
  that cannot fail, and `:118`'s braces still matter for `pipefail`. The one non-redundant
  loss is `:106`; see finding 1.
- **`styrman-ctl`'s `enabled/failed` rule against a oneshot's states.** `[[ x ==
  enabled/* ]] && [[ x != */failed ]] || rc=1` evaluates left-to-right as intended
  (A false → skip B → rc=1; A true, B false → rc=1). `inactive` (a completed oneshot) is
  OK, `failed` is DEGRADED. **`activating` is handled sensibly**: it matches `enabled/*`
  and is not `*/failed`, so a run in progress reads OK — correct, since the result is not
  yet known and `unit_state` is a snapshot taken at the same instant as everything else in
  the report. `deactivating` likewise. The one residual is pre-existing and unchanged:
  `unit_state` substitutes `${ac:-inactive}`, so a `systemctl` that prints nothing yields
  `enabled/inactive` and reads OK — not reachable via systemd, which always prints a word.
- **The integration check's `v="$(…)" && [ -n "$v" ] && …` inside `eval`.** Correct (see
  #8 above). One note: `check` evals in the *calling* shell, so `v` becomes a suite-global.
  `grep` confirms no other check reads or writes `v` today; a future check that does would
  silently share it. Naming it `_stampv` would remove the hazard; not a finding now.

## Criteria

1. **Dead code** — pass. The bare-`FROM` branch at `:101` currently matches nothing (all
   four `FROM` lines in both Dockerfiles already carry digests), but it is the branch that
   fires the first time someone adds a bare `FROM`, and the `grep -qsF` verify covers it —
   defensive, not dead. `test.sh:112`'s `|| true` and `:118`'s `|| true` are now redundant
   against the new `run()`; harmless, and `:118`'s braces are still load-bearing for
   `pipefail`. No commented-out blocks, no orphaned files, no unreferenced exports.
2. **Flow tracing** — pass for the firewall mechanism, with the test gap of finding 1. I
   traced `styrman-firewall.sh`'s non-zero exit to all four consumers: `install.sh:110`
   (tolerated, then enables — BUG-035's rule satisfied, `enable` is unconditional and
   idempotent), `deploy/update/styrman-update:578,588` (already tolerant, unchanged),
   `test.sh:58` (tolerated), and `styrman-ctl:333` (surfaces it as DEGRADED). The loop
   closes end to end: the installer no longer dies, and the state it now tolerates is the
   exact state the health verb now reports. `bump-pins.sh`'s new abort path was traced
   from each publisher read through `fail_read` to `:74`, and from each `sed` through the
   two verify loops to the `mv`.
3. **Standards conformance** — pass. `set -euo pipefail` throughout; no `$(…)` side
   effects in conditions; the Go test follows the file's existing fake-`systemctl` pattern
   and is appended, not overwritten (`git diff --numstat` on
   `internal/config/styrman_ctl_test.go`: 15 added, 0 deleted — BUG-005's check). All six
   changed files are text in `git diff` (BUG-007's check). `image/bump-pins.sh` is still
   absent from `deploy/verify-commit.sh`'s `bash -n`/`shellcheck` lists — carried over
   from the predecessor's criterion 3, not re-raised as a finding.
4. **Directory layout** — pass. No new files; each change sits in the file that owns the
   behaviour. The one layout smell is finding 2's `/tmp` temps.
5. **Simplicity (DIY-except-security)** — pass. No new dependency (`npm`/`awk` were
   already used, now declared). Nothing hand-rolled in auth, crypto, hashing, sessions or
   tokens. The shape-check helpers are three one-line `[[ =~ ]]` functions — the simplest
   thing that closes the injection class, and fail-closed.
6. **Decoupling** — pass. No module boundary moved; `bump-pins.sh` remains standalone and
   read-only without `--apply`; the `styrman-ctl` change is one line inside the verdict
   block with its test in the package that already owns the script's tests.

## Not examined

- Did not run `image/bump-pins.sh` in either mode — no network calls, no `--apply`. The
  publisher JSON shapes and `docker manifest inspect` output are read from the `jq`
  expressions, as in the predecessor's review. The sed behaviour above was verified
  against a synthetic Dockerfile in the scratch dir, with the real tags.
- Did not run `deploy/firewall/test.sh` (needs root + `unshare -Urnm`), nor
  `.agents/docs/testing/styrman/integration.sh` (needs Kata + a built runtime image).
  Finding 1 is read from the call sites and `set -e` semantics; the check-count claim is
  `grep -c 'check "'`, not a run. The commit's own tier logs
  (`.agents/docs/testing/2026-09-13-*-v1.30.1.log.md`) were **not** audited — they are
  outside the dispatched path list, so I did not confirm the counts in them.
- Did not re-verify the `xt_physdev`-absent case on a live host (this host has the module
  loaded), and did not test `systemctl start` against the real oneshot, so the claim
  "`install.sh` now reaches both `enable`s on a degraded host" is read from `run()` and
  `set -e`, not observed. The live check remains
  `sudo bash deploy/firewall/install.sh` on a host without the module, then
  `systemctl is-enabled styrman-firewall.service styrman-firewall.timer`.
- Did not mutate `deploy/styrman-ctl` to prove the new Go test fails without the fix
  (that would mean editing tracked source). I ran the glob matrix standalone instead: the
  old expression returns 0 for `enabled/failed` and the new returns 1, and the test's fake
  `systemctl` differs from the green `TestStyrmanCtlStatusJSON` fixture only in that
  field — so the test does bite. Confidence high, but it is an inference, not a mutation
  run.
- No live-host or production verification of any kind; no `go test ./...` beyond
  `-run TestStyrmanCtlStatus ./internal/config`; no web tier, no `govulncheck` (no
  manifest changed in the range).
- `.agents/docs/reviews/*` and `.agents/docs/testing/*` files added by the same commit
  were read as *input* (the two predecessor reports) but not reviewed as artifacts.
