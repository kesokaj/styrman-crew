---
owner: reviewer
date: 2026-09-21
---
# Review — sh-agent-release-binary round 2   (2026-09-21)

Range reviewed: `git diff HEAD` on the fix hunks named by the dispatch:
`image/entrypoint.sh` (`harness_home_newer` + `HOME_REL=""` block through the
`kept`/`deployed` lines), `deploy/update/styrman-update` (`retire_harness_units`,
`__retire-harness-units` hook, `TRUST_UID=0`), `deploy/host-setup.sh` (step-7
token block), `deploy/styrman-ctl` (`secret_args` in `cmd_install`),
`deploy/verify-commit.sh` (`BUILD_TOKEN` block + runtime build line),
`.agents/docs/testing/styrman/ensure-test-image.sh` (token block),
`.agents/docs/testing/styrman/integration.sh` (never-downgrade case),
`internal/config/entrypoint_guard_test.go`, `internal/config/styrman_update_test.go`
(`TestStyrmanUpdateRetiresHarnessUnits`), `.agents/docs/specs/styrman/api.md`,
`.agents/docs/specs/styrman/guide.md:24-36`. Verifying the `## Outcome` claims
in round 1 (`.agents/docs/reviews/2026-09-21-review-sh-agent-release-binary.md`)
and the security review (`.agents/docs/reviews/2026-09-21-security-sh-agent-release-binary.md`).

## Status: COMPLETE

## Verdict: REQUEST CHANGES

Seven of the eight security-review fixes and all three round-1 review fixes
verify cleanly against the code, with two tests that genuinely fail on the
old code (`TestEntrypointHarnessHomeNewer`, `TestStyrmanUpdateRetiresHarnessUnits`
run and pass; both compile and exercise real must-fail cases per the
in-file comments). The one problem: the Outcome for security finding 5
("token-file readers skip the bar the updater sets for itself") claims FIXED
but only two of the three sites the original finding named were touched.
`deploy/verify-commit.sh`'s new `/etc/styrman/gh-token` branch (added in
this same round, wholly new code) still does nothing but `[ -r
/etc/styrman/gh-token ]` — no ownership or group/world-writable check, the
exact gap the finding raised for this file (line 68 in the current file,
`deploy/verify-commit.sh:15-16` in the finding's numbering pre-move). The
Outcome text for item 5 names `host-setup.sh`, `styrman-ctl install`, and
"styrman-update deploy()" as fixed and simply omits `verify-commit.sh` —
the third of the three sites the finding itself named — so the record
overclaims completeness on a finding the code shows is still open there.

## Findings

- **[low / confidence: high]** `deploy/verify-commit.sh:68-69` — security
  finding 5 ("the new token-file readers skip the bar the updater sets for
  itself") explicitly named three sites: `host-setup.sh`, `styrman-ctl:475`,
  and `deploy/verify-commit.sh:15-16`. `host-setup.sh` now checks `stat -c %u
  = 0` and `find -perm /077` before using `/etc/styrman/gh-token` (confirmed,
  `deploy/host-setup.sh:41-42`); `styrman-ctl` calls `private_file` on it
  (confirmed, `deploy/styrman-ctl:475` → `private_file()` at line 507, which
  asserts owner, mode, and parent-dir mode). `verify-commit.sh`'s equivalent
  branch — entirely new code added in this same fix round (`git diff HEAD`
  shows the whole `BUILD_TOKEN` block, lines 55-76, as added) — is only
  `elif [ -r /etc/styrman/gh-token ]; then SECRET_ARGS=(--secret
  id=gh_token,src=/etc/styrman/gh-token)`: no ownership check, no
  group/world-writable check. The Outcome entry for this finding names
  `host-setup.sh`, `styrman-ctl install`, and "`styrman-update deploy()`,
  already checked in its preamble" as the fix and never mentions
  `verify-commit.sh` at all — the third named site is left both unfixed and
  unacknowledged. Concrete scenario: on a shared or misconfigured dev box
  running `deploy/verify-commit.sh` as root (the release-verification step
  named in `versioning.md`'s release procedure) with a world-writable or
  non-root-owned `/etc/styrman/gh-token`, the build authenticates with
  whatever token is at that path without the check that exists specifically
  to catch that case one file over. Fix: lift the same two-line check (or
  call the shared logic `styrman-ctl` factored into `private_file`) into
  this branch before using `src=/etc/styrman/gh-token`.

## Verified fixes (round 1 + security review Outcomes)

- Round 1 finding 1 (`api.md` `STYRMAN_HARNESS_DIR` row) — VERIFIED: no
  `STYRMAN_HARNESS_DIR`/`HARNESS_DIR` hit in `.agents/docs/specs/styrman/api.md`.
- Round 1 finding 2 (`guide.md:28-32` stale ADR-073 description) — VERIFIED:
  `guide.md:24-36` now describes the ADR-074 release-binary seeding and
  never-downgrade behaviour and points at `deploy.md`'s "Agent harness in
  the image (ADR-074)" section, which exists (`deploy.md:276`).
- Round 1 finding 3 ("deployed" logged after a skipped install) —
  VERIFIED: `image/entrypoint.sh:195-249`. `HOME_REL=""; HI_SKIPPED=0` is set
  unconditionally at the top of the block (the script runs under `set -uo
  pipefail`, so an unguarded read would abort — both are set before any read).
  The `unknown`-image-release, `harness_home_newer` (skip), and normal-install
  branches each set `HI_SKIPPED`/`HI_OK` correctly, and the final log picks
  "kept" vs "deployed" on `HI_SKIPPED`. `integration.sh`'s never-downgrade
  case (`.agents/docs/testing/styrman/integration.sh:487-506`) computes
  `DEPLOYED_BEFORE` from `docker logs "${CN}"` on the same (stopped, not
  removed) container before the `sed`-planted newer manifest and the
  `boot=1` restart, then re-reads the same container's log stream after the
  boot and asserts the count is unchanged and a `kept` line is present —
  genuinely fails on the pre-fix code, which always logged a fresh
  `deployed` line.
- `harness_home_newer` (entrypoint) — VERIFIED correct under `sh`/`bash`
  (the file's shebang is `#!/usr/bin/env bash`, so `grep -E` and the
  backslash-dot literals are ordinary bash/grep, not a POSIX-sh concern).
  Hand-tested the extracted function: `(v9.9.9, unknown)` → false,
  `(v1.7.0, v1.6.0)` → true, `(v1.6.0, v1.6.0)` → false, matching
  `TestEntrypointHarnessHomeNewer`'s table; ran the Go test — PASS.
  The `unknown`-image-release case is intercepted *before* the function is
  called (`if [ "${HARNESS_REL}" = unknown ]`), so the function itself never
  has to special-case it — consistent with the test and the security
  finding's requirement that an unreadable image binary never wins.
  Note: the implemented fix differs slightly from finding 3's literal
  remediation text ("an unreadable binary always takes the install path,"
  i.e. attempt install anyway) — the actual fix skips the install attempt
  entirely when `HARNESS_REL=unknown` and goes straight to the WARN/no-stamp
  path. The end state (no stamp written, WARN logged, retried next boot) is
  the same either way, and skipping an install attempt against a binary
  that just failed `version` avoids invoking a broken/absent binary a
  second time — not a defect, just a deviation from the literal remediation
  wording, noted for completeness rather than as a finding.
- `TRUST_UID` from the ambient environment (security finding 4) — VERIFIED:
  `deploy/update/styrman-update:229`, `TRUST_UID=0` sits after both
  test-hook blocks (`__verify-tag` at 205-217, `__retire-harness-units` at
  219-224, both `exit` before reaching 229) and before every production
  call site of `retire_harness_units` (752, 793, 822). The only other
  assignment is inside the gated test hook (line 222). A plain `VAR=0`
  overrides an inherited exported value (bash does not treat prior `export`
  as `readonly`), so an ambient `TRUST_UID` from the caller's environment no
  longer reaches the guard on the production path.
- `TestStyrmanUpdateRetiresHarnessUnits` `--check` and untrusted-unit-dir
  cases (security finding 7) — VERIFIED: the test (lines 133-263) plants
  units and four state files, exercises the refused-without-
  `STYRMAN_UPDATE_TEST` case, the normal retire (asserts `systemctl` call
  order and idempotency), a `--check` run (asserts units, state files, and
  `systemctl.calls` all untouched, "would remove" in the output), and a
  `0777` unit dir (asserts WARN and nothing removed). Ran: PASS.
- `export GH_TOKEN` widening (security finding 6) — VERIFIED for both named
  sites. `deploy/verify-commit.sh:65-76`: `BUILD_TOKEN` copies `GH_TOKEN`
  (or `gh auth token`) before `unset GH_TOKEN` (line 75); the build call at
  line 208 is `GH_TOKEN="${BUILD_TOKEN}" build_image …`, a prefix assignment
  to a shell *function* call. Confirmed empirically (`bash -c` scratch test)
  that a prefix assignment on a function call *does* export the variable
  into the environment of any external command the function subsequently
  runs (bash's temporary-environment mechanism applies to functions, not
  only external commands), and that the assignment does not leak past that
  one call — so `--secret id=gh_token,env=GH_TOKEN` inside `build_image`'s
  `docker build` sees the token, and nothing later in the script does.
  `.agents/docs/testing/styrman/ensure-test-image.sh:53-59` uses the same
  pattern directly on `docker build` (an external command, where the export
  behaviour is unambiguous). Both scripts run under `set -euo pipefail`;
  `${GH_TOKEN:-}` guards every read of the possibly-unset variable, so `set
  -u` does not abort on the `unset`.
- Uncapped `sed` on the guest-writable manifest (security finding 8) —
  VERIFIED: `image/entrypoint.sh:207-210`, the manifest is read only when
  `[ -f "${HS}" ] && [ ! -L "${HS}" ]`, under `timeout 5 sed -n …`, matching
  the neighbouring capped steps.

## Criteria

1. **Dead code** — pass on the reviewed hunks; nothing left dangling by the
   `harness_home_newer`/never-downgrade rewrite or the `TRUST_UID`/retire
   changes.
2. **Flow tracing** — fail on one hop: `deploy/verify-commit.sh`'s new
   token-file branch does not carry the ownership/perm guard the sibling
   sites (`host-setup.sh`, `styrman-ctl`) now enforce for the same
   `/etc/styrman/gh-token` path — see the finding above. Everything else
   traced cleanly: `HI_SKIPPED`/`HOME_REL` initialized on every path that
   reads them under `set -u`; `TRUST_UID` reset on every production call
   site; both new/extended Go tests build and pass and are wired to fail on
   the pre-fix code (confirmed by reading the extraction/test-hook
   mechanics, not by mutation testing here — the file's own comments and
   the round-1/security Outcome text describe the must-fail runs that were
   done).
3. **Standards conformance** — the `verify-bare.md` rule (run verification
   bare, read status, then commit) was followed for my own checks (`go
   build ./...`, `go test ./internal/config/... `, the two targeted `go
   test -run` invocations all run as bare statements, read before use).
   `guest-writable-paths.md` and `mechanism-reaches-every-caller.md` are
   both respected by the `TRUST_UID=0`/unconditional-retire design. The one
   standards gap is the finding above: a rule ("root must not authenticate
   with a token it hasn't vetted") stated and enforced at two of three
   named sites is not enforced, and not flagged as un-enforced, at the
   third.
4. **Directory layout** — pass; no new files, nothing misplaced.
5. **Simplicity (DIY-except-security)** — pass; `ssh-keygen -Y verify`,
   `sha256sum -c`, `stat`/`find`-based ownership checks are all the
   project's existing stdlib-first pattern, not new hand-rolled primitives.
6. **Decoupling** — the repeated (and now-inconsistent) token-file guard
   across four sites (`host-setup.sh`, `styrman-ctl`, `styrman-update`,
   `verify-commit.sh`) is the same trade round 1 already noted as
   reasonable (no shared shell library exists); the finding here is that
   one copy was simply dropped, not that the duplication itself is wrong.

## Not examined

- The full bodies of `deploy/host-setup.sh`, `deploy/styrman-ctl`, and
  `deploy/update/styrman-update` outside the named blocks — only the
  diff hunks and the functions referenced from them (`private_file`,
  `retire_harness_units`, the two test hooks) were read.
- `deploy/verify-commit.sh` beyond the `BUILD_TOKEN` block and the runtime
  build line — the rest of the file (worktree setup, the harness-mirror
  removal hunk, `go test -race`, the ingress module block) was skimmed via
  `git diff` output but not re-verified line by line.
- Did not run `integration.sh` (Tier 2), `web/e2e/run.sh`, or
  `deploy/vulncheck.sh` live — relied on reading the never-downgrade test
  block and the round-1/security records of prior Tier 2 runs.
- Did not re-verify security findings 1 and 2 (mutable-tag pin, harness
  content outside the review boundary) beyond confirming they are recorded
  as REBUTTED/ACCEPTED with an operator quote in the Outcome — did not
  independently re-litigate whether that acceptance is sound; that is a
  judgment call for the operator, not a code-verification question.
- `.agents/docs/architecture/DECISIONS.md` has no `ADR-074` entry yet
  (grepped, zero hits) even though every spec file already cites it; per
  the plan file (`.agents/docs/plans/2026-09-21-sh-agent-release-binary.md`
  D6) this is pending operator approval and out of this dispatch's named
  file list, so not scored as a finding here — flagged only so it is not
  mistaken for an oversight of this review.

## Outcome

Written by the author, 2026-09-21.

1. `deploy/verify-commit.sh` token-file branch without the ownership/permission bar — FIXED: the branch now asserts root ownership and no group/world access (the same two-line check as `host-setup.sh`) before the `src=` form; the security report's Outcome item 5 corrected to name the site. Two-line change, read by the author (`git diff deploy/verify-commit.sh`), `bash -n` clean; no further review round.
