---
owner: reviewer
date: 2026-09-13
---
# Review — close-out fixes (firewall warn, entrypoint stamp, bump-pins)   (2026-09-13)

Range reviewed: 3d50c27..336a5d1 (6 files; single commit 336a5d1, an ancestor of the
current HEAD 441507e — the reviewed blobs were read from the range, not the worktree)

## Verdict: REQUEST CHANGES

The worst problem is not in the changed lines but in what they now do to two callers
that were not traced. `warn()` appends to `WARNS`, and `styrman-firewall.sh` ends with
`if [ "${#WARNS[@]}" -gt 0 ]; then exit 1`. Routing the missing-`xt_physdev` condition
through `warn()` therefore turns a *standing* condition (an absent kernel module, which
`modprobe xt_physdev 2>/dev/null || true` already admits can happen) into a permanent
non-zero exit. Two callers run that script under `set -e` without a guard:
`deploy/firewall/install.sh:109` (`systemctl start` of a `Type=oneshot` unit propagates
the failure, aborting the installer *before* it enables the service and the timer) and
`deploy/firewall/test.sh:61` (the whole firewall suite aborts at section 1). This is
BUG-035's recorded family — a mechanism that did not reach every caller — and the file's
own `warn`/`note` comment says the `note()` variant exists precisely for "a standing
condition such as a missing conf". The rest: `image/bump-pins.sh` reports "applied" for
rewrites that silently matched nothing, and reports "everything is current" on a run
where every digest lookup failed. The entrypoint change is correct and byte-identical to
what it replaced; the integration check's quoting is correct but passes vacuously when
the container is gone.

## Findings

- **[medium / confidence: high]** `deploy/firewall/install.sh:109` — `run systemctl start
  styrman-firewall.service` runs under `set -euo pipefail`. On a host without
  `xt_physdev`, `styrman-firewall.sh` now exits 1, `systemctl start` of the oneshot unit
  returns non-zero, and the installer dies there — never reaching `systemctl enable
  styrman-firewall.service` (line 113) or `systemctl enable --now
  styrman-firewall.timer` (line 114). The base rules land once and then never self-heal
  and never return at boot: the firewall ends up *less* installed than before this
  commit, from a condition that was previously a printed warning. Recorded family:
  BUG-035. Fix: guard it the way the updater already does —
  `deploy/update/styrman-update:578` wraps the same call in `if ! out="$(systemctl start
  … 2>&1)"; then log "WARN: …"; fi` — or move the `start` after both `enable` calls.

- **[medium / confidence: high]** `deploy/firewall/test.sh:61` (also :82, :86, :99, :120)
  — bare `run >/dev/null 2>&1` under `set -euo pipefail`. The same standing condition now
  aborts the entire suite at "=== 1. base rules"; `TestFirewallScriptInNetns`
  (`internal/config/firewall_script_test.go`) then fails with `deploy/firewall/test.sh
  failed: exit status 1` and a log containing no check output at all, instead of one
  honest `FAIL physdev RETURN present` among 80+ passes. test.sh already anticipates this
  exact hazard for the tagged-DROP case at lines 109 and 115 (`run >/dev/null 2>&1 || true`, with
  the comment "the script deliberately exits 1 when it has warnings") — the new warn path
  was not traced back to the other four bare call sites. Verified locally: this host has
  `xt_physdev` loaded (`lsmod | grep xt_physdev` → present), so the suite is green here;
  the breakage is on a host or container without the module, which is the only
  environment where the new warn fires at all. Fix: `|| true` on the bare `run` calls,
  leaving the `check` assertions to carry the verdict.

- **[medium / confidence: high]** `image/bump-pins.sh:76,78-79` — `--apply` prints
  "applied" without proving a single line changed. Concrete case: a `FROM <tag>` line
  with no digest yet. `cur` is empty, `report` prints `<no digest> -> sha256:…` and sets
  `changed=1`, then `sed -i "s#^FROM ${tag}@sha256:[0-9a-f]*#…#"` requires an existing
  `@sha256:` and matches nothing — the base image stays unpinned while the tool says it
  was pinned, and every later run repeats the same claim. The same silent no-op happens
  if an `ARG` is ever renamed (`current` returns empty, the `s/^ARG …=.*/` finds no
  line). A related masking case: when two `FROM` lines share a tag and only one carries a
  digest, `grep -ho … | head -1` reports the pair as "(current)". Fix: after each `sed
  -i`, assert the expected text is now present (`grep -q`) and exit non-zero naming the
  file and pin when it is not.

- **[medium / confidence: high]** `image/bump-pins.sh:68` — a digest that could not be
  read prints one line and is `continue`d past without touching `changed`. If `docker
  manifest inspect` fails for every tag (no daemon, rate limit, offline) while the three
  version pins happen to be current, the script prints "bump-pins: everything is current"
  and exits 0 — a green summary for a run that checked nothing, which is the project's
  own recorded failure mode (BUG-034, BUG-040, BUG-049: silence from your own
  instrumentation read as evidence). Fix: set an `unknown=1` alongside the message and
  exit non-zero with "could not read N base-image digests — this run proves nothing about
  them".

- **[low / confidence: high]** `image/bump-pins.sh:25,42,45` — `npm` is invoked at line
  42 but is absent from the `for tool in curl jq docker sed` prerequisite loop, and
  `npm view … 2>/dev/null` swallows bash's own "command not found". Under `set -e` the
  assignment then terminates the script with status 127 and **no output whatsoever** (the
  report `echo`s all come later). The friendly guard at line 45 ("could not read every
  publisher source — nothing changed") is unreachable for the likeliest failures: a
  failing `curl -fsSL` in a `VAR="$(…)"` assignment also exits before it. Fix: add `npm`
  to the tool loop; drop the `2>/dev/null` or capture with `|| true` so the guard runs.

- **[low / confidence: high]** `image/bump-pins.sh:76-80` — partial application is
  possible: the ARG rewrite is one `sed -i` on `image/Dockerfile`, then the digest loop
  does one `sed -i` per tag across both Dockerfiles. A failure anywhere in the loop leaves
  the tree half-rewritten, with only a bare non-zero exit and no statement of what was
  already written. Low because the operator's next step is a build. Fix: rewrite into
  temp copies and `mv` both files at the end, or print the files already modified on the
  failure path.

- **[low / confidence: high]** `.agents/docs/testing/styrman/README.md:15` — the new
  integration check is not reflected in the documented counts. The file still reads
  "Integration (94 checks; … `passed=91 failed=0 skipped=3`)" while the script now holds
  95 `check` invocations, so a correct green run prints `passed=92` and reads as a
  mismatch against the document `AGENTS.md` names as the source of truth. The immediately
  preceding commit (25bdae8) updated these counts for exactly this reason. Fix: bump both
  numbers in the same commit as the check.

- **[low / confidence: medium]** `.agents/docs/testing/styrman/integration.sh:130` — the
  new check compares two `docker exec … cat` command substitutions. If the container is
  gone, stopped, or `docker exec` fails for any reason, both substitutions are empty and
  `[ "" = "" ]` **passes**. The check can therefore report green precisely when it could
  not run — the shape of BUG-040 and BUG-049. (Confidence on "does it matter here" is
  medium only because earlier checks in the same section would usually have failed first;
  the vacuous-pass mechanism itself is certain.) Fix: bind the stamp first and require it
  non-empty, e.g. `v="$(docker exec ${CN} cat ${GH}/.styrman/.harness-version)"; [ -n
  "$v" ] && [ "$v" = "$(docker exec ${CN} cat /styrman/VERSION)" ]`.

- **[low / confidence: high]** `deploy/styrman-ctl:313,330-335` — the commit's stated goal
  is that the degraded state stops "reading as green", but the operator-facing health
  verb does not act on it. `unit_state` returns `enabled/failed` for a permanently failing
  oneshot, and the verdict only globs `enabled/*`, so `styrman-ctl status` still prints
  "verdict OK" while every sandbox project's credential broker and port preview are
  blocked. The oneshot nuance matters for the fix: a *successful* run also reads
  `enabled/inactive`, so the correct change is to treat a `/failed` suffix specifically as
  degraded — not to require `/active`.

## What is correct (checked, not assumed)

- `image/entrypoint.sh:168` writes exactly what the old form wrote. `sh -c 'printf "%s"
  "$1" > "$2"' _ "${HARNESS_VER}" "${HARNESS_STAMP}"` puts `_` in `$0`, so no option
  parsing applies to the values; `$1`/`$2` are inside single quotes and are expanded by
  the inner `sh`, both quoted, so no splitting or globbing. `HARNESS_VER` comes from
  `"$(cat /styrman/VERSION …)"` (line 127), whose trailing newline is already stripped by
  the command substitution, so `printf "%s"` emits the same bytes as the old
  `printf '%s' '<ver>'`. The injection path the comment describes is genuinely closed.
- The integration check's quoting is right for `check(){ if eval "$2"; …}`: the `\"\$( … )\"`
  escapes survive the outer double-quoted string and the substitutions run inside `eval`,
  while `${CN}` and `${GH}` are expanded at build time and are both set (lines 97, 113).
  Trailing-newline differences between the stamp and `/styrman/VERSION` cannot cause a
  false mismatch, since `$( )` strips them on both sides.
- The `s#…#` delimiter cannot be broken by a `#` in a digest or tag: `sha256:<hex>` has
  no `#`, and neither do the four `FROM` refs in the two Dockerfiles. The regex `.` in a
  tag like `golang:1.25.13-trixie` over-matches harmlessly. `debian:trixie` does not
  match the `debian:trixie-slim` line, because the pattern requires `@` immediately after
  the tag — checked against both files.
- `declare -A DIGEST` is safe here: the `while IFS= read -r ref … done < <(…)` loop body
  runs in the parent shell, so the map survives; `${DIGEST[${tag}]:-}` is `set -u`-safe,
  and bash 5.2 tolerates `"${!DIGEST[@]}"` on an empty map.
- `[ "${1:-}" = "--apply" ] && APPLY=1` does **not** trip `set -e` — tested directly on
  bash 5.2.37: the no-argument run reaches the next line and the script exits 0. The
  documented dry-run mode works.
- `current()` parses every ARG it is asked for: all five (`GO_VERSION`, `GO_SHA256`,
  `NODE_VERSION`, `NODE_SHA256`, `CLAUDE_CODE_VERSION`) exist exactly once in
  `image/Dockerfile` with a plain unquoted default. The Node filename the helper sums
  (`node-v…-linux-x64.tar.xz`) matches the one the Dockerfile downloads, and the Go
  `kind=="archive"` selection matches `go${VER}.linux-amd64.tar.gz`.
- `shellcheck -S warning image/bump-pins.sh` and `bash -n image/bump-pins.sh` are both
  clean (run locally, bare, exit 0).
- `deploy/update/styrman-update`'s `host_converge` is *not* affected by the new exit
  status: every `systemctl` call there is already wrapped in `if ! out="$(…)"` and only
  logs a WARN, and the unconditional `systemctl enable styrman-firewall.service` at line
  588 is outside the `changed` guard (BUG-035's second lesson is still honoured).

## Criteria

1. **Dead code** — one instance. `image/bump-pins.sh:45`'s "could not read every
   publisher source" guard is unreachable for the failures it names, because `set -e`
   kills the assignments above it first (finding 5). Nothing else orphaned; no
   commented-out blocks; the new script has no unused paths.
2. **Flow tracing** — FAIL. `warn()`'s new caller was not traced to
   `deploy/firewall/install.sh`, `deploy/firewall/test.sh`, or `deploy/styrman-ctl`'s
   health verdict (findings 1, 2, 9). The entrypoint and integration-check hops trace
   cleanly.
3. **Standards conformance** — mostly pass. Bash style matches the repo's root scripts
   (`set -euo pipefail`, no `$(…)` side effects). One gap: `deploy/verify-commit.sh:151-153`
   `bash -n`/`shellcheck`s only `deploy/firewall/*.sh` and `deploy/update/styrman-update`,
   so `image/bump-pins.sh` — a new script that rewrites two Dockerfiles with `sed` — has
   no lint or syntax gate and no test of any kind. Adding it to those two lists is a
   one-line change and would make the Definition of Done's "changed code without a
   covering test" clause defensible for it.
4. **Directory layout** — pass. `image/bump-pins.sh` sits beside the `image/Dockerfile`
   it maintains; the Go text-pin test is in the package that owns `BridgeNameFor`, as its
   own comment explains; the integration check is in the existing suite.
5. **Simplicity (DIY-except-security)** — pass. No new dependency; `curl`/`jq`/`sed`/
   `docker` only, and the prerequisite loop (modulo the missing `npm`) is the right shape.
   No hand-rolled auth, crypto, hashing, sessions or tokens anywhere in the range. The
   sha256 verification remains the publishers' own published sums, checked at build time
   by `sha256sum -c`, which is the correct arrangement.
6. **Decoupling** — pass. The helper is standalone and read-only without `--apply`; it
   does not import or duplicate any server code, and no module boundary moved.

## Not examined

- Did not run `.agents/docs/testing/styrman/integration.sh` (needs Kata + a built runtime
  image); the new check is reviewed by reading only.
- Did not execute `image/bump-pins.sh` at all, in either mode — no network calls made, so
  the publisher-JSON shapes (`go.dev/dl?mode=json`, `nodejs.org/dist/index.json`) are
  reviewed from the jq expressions, not from real responses. `--apply` was deliberately
  never run.
- Did not reproduce the `xt_physdev`-absent case: this host has the module loaded. Findings
  1 and 2 are read from `set -e` semantics and the call sites, not observed. The exact
  live check is `modprobe -n xt_physdev; sudo bash deploy/firewall/install.sh --dry-run`
  on a host without it.
- Did not determine how systemd's `OnUnitActiveSec=60s` timer behaves against a oneshot
  unit that now fails on every tick (whether `ActiveEnterTimestamp` stops advancing and
  the timer re-fires immediately into `StartLimitBurst=20`). If it does, the self-heal
  stops entirely rather than merely warning — worth one experiment on a scratch unit
  before shipping finding 1's fix.
- Did not review `.agents/docs/specs/styrman/runtime-image.md` beyond the five changed
  lines (they accurately describe the helper's scope: Go, Node, Claude Code, digests).
- Did not build the runtime image or run `go test ./...` / `web` tiers; no Go or
  TypeScript behaviour changed in this range apart from the text-pin assertion.
