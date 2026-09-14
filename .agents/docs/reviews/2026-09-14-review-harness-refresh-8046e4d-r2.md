---
owner: reviewer
date: 2026-09-14
---
# Review — harness-refresh-8046e4d-r2   (2026-09-14)

Range reviewed: v1.30.1..worktree (round 2)

Round-1 report: `.agents/docs/reviews/2026-09-14-review-harness-refresh-8046e4d.md`
(not amended). This round re-reviews the two round-1 findings and the delta
since that report. The delta is exactly four files — by mtime, everything in
the change set written after round 1 finished (12:13:37) is
`.agents/docs/testing/styrman/integration.sh`, `image/Dockerfile`,
`image/harness/VENDORED.md` and `image/harness/SHA256SUMS`, all at 12:16:50 —
plus the two bug records, the change entry and the security report, which
predate it and are reviewed here for whether what they state matches the code.

## Verdict: APPROVE WITH NITS

Both round-1 findings are closed, and closed properly rather than papered
over: the boot-log check now holds the commit on the HOST and fails closed in
every degradation mode I could simulate, and `VENDORED.md` names BUG-058 by id
with `SHA256SUMS` correctly regenerated over the edit. Nothing in the delta is
broken. The one thing worth the operator's attention is not a code defect but a
trail one: `integration.sh` and `image/Dockerfile` both changed AFTER the last
recorded green integration run, so the `passed=94 failed=0 skipped=3` that the
change entry and the testing README cite describes a script and an image build
that no longer exist in this tree. That is a re-run before tagging, not a fix.

## Round-1 findings

- **Finding 1** (boot-log check self-referential and failing open) — **closed.**
  `integration.sh:135-136` now reads the commit from the host file
  (`c="$(cut -c1-7 '${REPO}/image/harness/COMMIT')"`, `REPO` set at line 40 from
  `dirname "$0"/../../../..`, which resolves to the repo root) and only the
  version from the container, then requires `[ ${#c} = 7 ]` and `[ -n "$v" ]`
  before the grep. The comment above it (lines 119-131) states exactly this and
  cites the round-1 review. Verified by simulation rather than by reading:
  I rebuilt the `check()`/`eval` wrapper verbatim in the scratchpad with a
  `docker` stub on `PATH` and ran four cases — matching log → PASS; `docker exec`
  failing so `v` is empty → FAIL; log naming a different commit (`6893e22`,
  the previous mirror) → FAIL; host `COMMIT` missing so `cut` errors → FAIL.
  The fail-open hole is gone and the commit half is now a real assertion:
  it proves the running image was built from THIS tree's `COMMIT`, which
  `ensure-test-image.sh` makes true by rebuilding whenever any file under
  `image/` is newer than the tag (so the check cannot go red merely because the
  image is stale, except under `STYRMAN_TEST_IMAGE_NO_REBUILD=1`, where it
  fails closed — the right direction). `bash -n integration.sh` clean;
  `grep -c 'check "'` = 97, matching the testing README.
  Residual, not a finding: it still cannot prove the mirror TREE is upstream's
  `8046e4d` — only that the image and the tree agree on the label. `VENDORED.md`
  says so explicitly ("`COMMIT` remains an attestation by whoever refreshed —
  the `diff -rq` above … is its only check"), which is the honest framing.

- **Finding 2** (`VENDORED.md` silent on the mirror's red suite) — **closed.**
  § Refresh gained the paragraph, with the BUG-058 id, the failing test's name
  (`TestDryRunNeverBuilds` in `cmd/sh-agent`), the reason (asserts the
  checkout-shaped dry-run output), the fact that `deploy/verify-commit.sh` vets
  and never tests the mirror, and the sentence round-1 asked for so a second,
  unrelated failure stays visible: "A different failure there is a different
  problem." Verified the integrity side, since `VENDORED.md` is itself a listed
  entry in the manifest and an edit without regeneration would break the release
  gate: `(cd image/harness && sha256sum -c --quiet SHA256SUMS)` → **rc 0**, and
  the gate's own list comparison from `deploy/verify-commit.sh:93`
  (`diff <(find . -type f ! -name SHA256SUMS | …) <(cut -c67- SHA256SUMS)`) →
  **empty, rc 0**, 154 files listed and 154 on disk, `find . -type l` empty,
  `find .agents -type f` empty. All five untracked paths under `image/harness/`
  are non-ignored (`git check-ignore` says so) and are in the manifest
  (`skills/review/SKILL.md`, the `retros/.gitkeep`), so the gate will pass in
  the commit's worktree once they are staged — it will fail if they are not.

## Findings

- **[low / confidence: high]** `.agents/docs/testing/styrman/integration.sh`,
  `image/Dockerfile` — both were modified at 12:16:50, after the verifier log
  (`.agents/docs/testing/2026-09-14-verify-harness-refresh-8046e4d.md`, 12:13:47)
  and the change entry (12:09:57) were written. Those artifacts record
  `passed=94 failed=0 skipped=3` and the testing README repeats it at
  `README.md:15`, naming "boot log records the harness deployed at the mirror's
  commit" among the checks that were green — but that check's body was rewritten
  afterwards, and the image that run booted was built from a Dockerfile without
  the new `test ! -e /styrman/harness/.git` step. Failure scenario: the release
  gate or the next operator run is the first execution of the current script and
  the current image build, and the trail says both were already observed green —
  the BUG-040/BUG-034 family (a green signal that measured something else). My
  simulation above exercises the shell logic but not real `docker exec`/`docker
  logs`, and no image was built in this review. Concrete fix: re-run
  `bash .agents/docs/testing/styrman/integration.sh` (which rebuilds the image
  via `ensure-test-image.sh`, so it covers the Dockerfile guard too), read the
  status bare per `.agents/rules/verify-bare.md`, and update the verifier log and
  the change entry's Verification paragraph with the observed counts before the
  release tag. No code change is implied.

Checked in the delta and NOT findings:

- **`image/Dockerfile:166-173` — the `.git` guard is correct and cannot false-
  positive.** `test ! -e /styrman/harness/.git` sits inside the runtime stage's
  `&&` chain, before `chmod -R go-w`, so a mirror carrying `.git` fails the build
  with a non-zero RUN rather than shipping a tree `sh-agent` would treat as a
  checkout (`harness.IsCheckout` is a bare `os.Lstat(repo/.git)`,
  `internal/harness/repo.go:75-78`) — which is the premise the entrypoint's
  dropped git guards rest on. It asserts the IMAGE's state, which is the right
  side of the seam: `image/.dockerignore` already excludes `.git` and `**/.git`
  from this build context, so today the guard is belt-and-braces, and it stays
  correct if that line is ever removed or the context changes. It cannot fail
  spuriously (a `.git` under `image/harness/` would have to exist and be copied).
  The RUN prints no message on failure, but BuildKit echoes the failing step's
  full command, so the operator sees the `test` line — and the four-line comment
  above says why. Placement after the `COPY` is the only place it can go.
- **The bug records match the code.** BUG-057: both new checks do use
  `grep -F … >/dev/null` (`integration.sh:134,136`), and the root-cause account
  of `grep -q` + SIGPIPE + `pipefail` matches `set -euo pipefail` at line 16.
  BUG-058: `TestDryRunNeverBuilds` is at `image/harness/cmd/sh-agent/main_test.go:198`
  and does assert `"[dry-run] the binary would be rebuilt from"` (line 209); the
  plain-copy branch it actually hits prints
  `[dry-run] plain copy, not its own git checkout: the running binary would be
  installed as is` (`main.go:143`). The record's line citation is 211 rather than
  209 — inside the same assertion loop, not worth a change. Both are in
  `INDEX.md` with the prevention rule in the row.
- **`VENDORED.md`'s new "A plain copy, by design" section quotes the binary
  accurately.** `plain copy, not its own git checkout: the running binary is
  installed as is` = `main.go:145`; `repo /styrman/harness (plain copy, not its
  own git checkout; update is refused)` = `main.go:234`, which is also the exact
  substring `integration.sh:134` greps. The replaced "Known cosmetic gap"
  section's claim (empty `commit` field because there is no `.git`) survives
  intact in the new text, so nothing was lost in the rewrite.
- **The change entry's numbers check out.** 154 files in `SHA256SUMS`; 56 tracked
  paths changed under `image/harness/` of which 2 deleted → "54 modified, 2
  deleted, 5 untracked" is right; 27 skills / 7 agents matches the tree; check
  count 97 matches `grep -c 'check "'` and the testing README. `VENDORED.md`'s
  new release sentence points at `.agents/docs/changes/` for the full SHA, and
  `specs/styrman/versioning.md:89-92` does require the release entry to name
  `image/harness/COMMIT` — the cross-reference is real, not aspirational.

## Criteria

1. **Dead code — pass.** The delta adds four lines of shell/Dockerfile and one
   documentation paragraph; nothing is left orphaned, nothing commented out. The
   round-1 pass on the two deleted skill directories is unchanged (no new
   references appeared).
2. **Flow tracing — pass.** The boot-log check's two ends now sit on opposite
   sides of the seam (host tree → `ensure-test-image.sh` → image →
   `image/entrypoint.sh:131,177` → `docker logs`), and every hop was traced;
   the check's four behaviours were mutation-tested rather than reasoned about.
   The Dockerfile guard's caller chain is the single `docker build` in
   `ensure-test-image.sh` and `deploy/verify-commit.sh`'s runtime-image step —
   both build from `image/` with the same context, so the guard reaches every
   caller (the BUG-035 rule); it is unconditional, not gated on "did something
   change", so no first-run-after-ship gap exists. No Go changed in this delta.
3. **Standards conformance — pass.** `bash -n` clean; the new check keeps the
   `grep -F … >/dev/null` form BUG-057 requires and the capture-then-test order
   the port guard already uses; my own verification was run bare with its status
   read (`.agents/rules/verify-bare.md`). The `&&` chain in the Dockerfile RUN is
   a build step, not a verification-piped-into-a-commit, so that rule does not
   apply. `.agents/rules/writing-files.md`: `git diff --numstat` on all four
   delta files shows real line counts, no binary — nothing was written through a
   generator into a control byte.
4. **Directory layout — pass.** No new files in the delta; the bug records,
   change entry and reviews are in their conventional `.agents/docs/` homes, and
   the scratch I used for the simulation is the session scratchpad, not the repo.
5. **Simplicity, DIY-except-security — pass.** The finding-1 fix moved one value
   from one side of a comparison to the other and added two length guards; the
   finding-2 fix is five lines of prose. The Dockerfile guard is a single
   `test`. No dependency, no abstraction, nothing security-sensitive hand-rolled.
6. **Decoupling — pass, and improved.** Round 1's observation that the check
   "reached back through the container for both sides of its comparison" is what
   the fix addresses: one side is now held on the host, so the assertion spans
   the crew↔image seam instead of collapsing inside it. The crew↔harness seam is
   still exactly the `sh-agent` CLI contract plus `/styrman/harness/COMMIT`, and
   the `.git` guard is enforced on the crew side of it without editing the
   mirror.

## Not examined

- **No image was built and no microVM booted.** The boot-log check was verified
  by simulating `check()`'s `eval` against a `docker` stub, which exercises the
  quoting, the length guards and the fail-closed paths but not real `docker
  exec`/`docker logs` output; the Dockerfile guard was read and traced, never
  built. This is the substance of the finding above.
- **`deploy/verify-commit.sh` was not executed** (BUG-056's Docker Hub rate-limit
  window). Its mirror-integrity step was replicated by hand instead: the
  `sha256sum -c --quiet` and the line-93 file-list `diff`, both against the
  working tree rather than a detached worktree of the commit.
- **Everything round 1 already covered was not re-reviewed**: the vendored Go
  (`cmd/sh-agent`, `internal/harness`), `image/entrypoint.sh`, `README.md`,
  `image/README.md`, `specs/styrman/runtime-image.md`, the testing README, and
  the ~3,700 lines of vendored markdown prose are unchanged since 12:13:37 and
  carry round 1's verdict unaltered.
- **The security review** (`.agents/docs/reviews/2026-09-14-security-harness-refresh-8046e4d.md`,
  12:15:04) was read only for the finding that produced the Dockerfile guard; its
  other conclusions are its own artifact and were not re-litigated here.
- **`web/`** is untouched by this range.
