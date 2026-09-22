---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 5 — the coverage unknown, measured   (2026-09-22)

Phase 5 of `plans/2026-09-22-project-review-remediation.md`, closing finding 17
of the whole-project review and the debt Phase 0b knowingly took on.

## What the verifier's FAIL actually was

The repo-wide verifier reported `cmd/wsprobe/main.go` and 33 of 47 `web/src`
files as having no unit test, and returned FAIL. Tier 4 was excluded from that
pass, so those 33 were *not examined*, not *confirmed uncovered* — writing 33
unit tests before knowing which is which would have been the expensive answer
to a question nobody had asked. Both suites have now been run on this host:

- **Tier 2** (`integration.sh`): `passed=112 failed=0 skipped=3` — exactly the
  count `testing/styrman/README.md` names, the 3 being the firewall-hardened
  host's expected auto-SKIPs.
- **Tier 4** (`web/e2e/run.sh`): `passed=93 failed=0 skipped=2`, one more than
  the previous baseline of 92 — the check added below.

**The honest gap is four files, not 33.** Everything Tier 4 drives is
exercised end to end; `src/lib/fonts.ts`, `src/lib/projects.ts`,
`src/lib/toast.ts` and `src/router.tsx` are reached by neither tier. That list
is now in `testing/styrman/README.md`, measured rather than estimated, with
the note that a change to any of the four has no automated coverage at all.

`cmd/wsprobe` stays struck, for the reason the plan recorded: it is a WS dev
probe that `integration.sh` builds and invokes seven times, and a unit test for
it would test the harness rather than the product.

## The debt from Phase 0b is paid

Phase 0b capped `files.List` and added an explorer notice for a truncated
listing, and shipped that notice with **no test** — `FileExplorer.tsx` has no
unit test and the project has no component-testing framework, so standing one
up is a dependency decision rather than a side effect of a security fix. That
was recorded as a Phase 5 task rather than claimed as covered.

`web/e2e/run.mjs` now carries "a directory past the listing cap says so": it
creates 5200 files in the workspace, asserts the notice appears for that
directory, and asserts it does **not** appear for the workspace root — so a
notice stuck permanently on would fail too.

**Proved, not assumed.** With the notice's render suppressed
(`{false && !error && truncated && …}`, which type-checks), the check fails:
`page.waitForSelector: Timeout 30000ms exceeded`. Restored from a checksummed
copy, verified byte-identical, and the suite is green again at 93.

**That probe also found a defect in the check itself.** When it failed, it
left the explorer at the wrong path and cascaded into eight later checks —
`passed=84 failed=9`, which reads as nine defects instead of one. The check
now restores the explorer's position in a `finally`, so its own failure stays
one failure. A test that corrupts the run when it fails is a bad test even
when the thing it guards is correct, and this is only visible if you make the
check fail on purpose.

## Verification

Every command bare, status read:

- `GH_TOKEN=… bash .agents/docs/testing/styrman/integration.sh` → 0,
  `passed=112 failed=0 skipped=3`
- `bash web/e2e/run.sh` → 0, `passed=93 failed=0 skipped=2`
- The must-fail run above → 1, the new check named

Tier 2 rebuilt `styrman-runtime:test` from this tree first, so both suites ran
against this session's code, not a stale image.

## Learnings for the local host

None for the deploy path. One for the test suite, recorded above: an e2e check
that navigates must restore its position in a `finally`, or its failure is
reported as many.
