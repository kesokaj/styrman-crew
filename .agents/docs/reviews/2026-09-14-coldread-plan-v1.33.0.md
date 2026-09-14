---
owner: reviewer
date: 2026-09-14
---
# Cold-read — plan `2026-09-14-v1.33.0-close-out.md`   (2026-09-14)

Range reviewed: not a diff — the plan file `.agents/docs/plans/2026-09-14-v1.33.0-close-out.md`
plus the code surfaces it names (also read `.agents/docs/plans/2026-09-14-recon-harness-upstream-items.md`,
read-only, for the styrman-harness phase).

## Verdict: APPROVE WITH NITS

The plan is unusually well specified for a cold read — exact function
signatures, line numbers, error-message text, and test cases are given for
almost every item, and every claim I checked against the actual code held
(the `NetworkConnect` fake-format change does not break `callsTo`'s
prefix-based counters; `ensureFor`'s two guards return before any Docker
call and are directly testable with the existing `testRoutes`/`s.deleting`
map access; the compose sysctl and `SelfContainer` guard the plan assumes
already exist). Nothing here is wrong. The gaps are two real but narrow
ambiguities (BUG-038's status, and exactly which stream `verify-commit.sh`
captures) plus one loose end carried from the recon brief into the harness
phase (a bug-record numbering mismatch between the two repos). None of the
three are large enough to call the design broken.

## (1) Gaps — undecided before implementation

- **BUG-038's fate is unstated.** `.agents/docs/bugs/BUG-038-e2e-runner-trap-eof.md`
  is `status: open`, root cause explicitly "Not established" — the format-
  string extraction is the record's own speculative "Consider" prevention,
  not a confirmed fix. The plan's Phase 1 task ("`web/e2e/run.sh`:
  `MOUNT_SOURCES_FMT` variable, trap uses it") does not say whether to flip
  BUG-038 to fixed, downgrade it to "mitigated, unreproduced", or leave it
  open. Two implementers diverge here: one closes the record on an
  unreproduced bug (a false "fixed"), the other leaves it open with no
  update at all. Decide before Phase 1: the honest state is "mitigation
  applied, root cause still unconfirmed" — the record should say that, not
  flip to fixed.
- **`verify-commit.sh` capture: which stream, and does `set -e` still fire
  before the check runs?** (`deploy/verify-commit.sh:184-190`, current form
  `( cd "${WT}" && docker build -q … -t "${IMG}" . >/dev/null )` under
  `set -euo pipefail` at line 15.) Today a build failure aborts the script
  immediately via `set -e`, with docker's own error text going wherever
  docker sends it (uncaptured). The plan's "each `docker build` captures
  its output to a file… on failure, if the output matches `429|…`" requires
  wrapping the build in an `if ! ( … ) >file 2>&1; then` form (the file at
  line ~207, `if ! docker run --rm --entrypoint sh "${RIMG}" …; then`, is
  the existing template) — the Interfaces section states the desired
  behavior but not this mechanical requirement, and does not say whether
  `2>&1` is merged into the same file. If only stdout is captured (matching
  today's `>/dev/null`), a rate-limit message on stderr never matches the
  429 pattern and the whole feature silently never fires. Not calling this
  BLOCKING because the file's own precedent (`if !` form, line ~207) points
  a competent implementer at the fix, and merging both streams is the
  obviously-safe default — but it is worth stating in the plan rather than
  leaving it to be inferred.
- **Harness-side BUG-058 numbering is undefined.** The plan (Surfaces/Phase 5)
  says "A BUG record is written in the harness (none exists there; BUG-058
  is crew's number for it)." styrman-harness's own bug numbering is
  independent of styrman-crew's (its `INDEX.md` is currently at BUG-015)
  — "BUG-058" is not a number that exists, or should exist, in that repo.
  If the harness-side `bugs` skill is dispatched normally it will assign
  the next harness number on its own (probably BUG-016), which resolves
  this on its own — but the plan's phrasing ("BUG-058 is crew's number for
  it") could mislead an implementer into filing `BUG-058-*.md` in the
  harness repo, creating a gap in its own sequence. Worth one clarifying
  line in the plan: "the harness record gets the harness's own next
  number; do not reuse 058."

## (2) Readiness — questions a first-pass implementer would ask

- Does the `ip_forward` guard belong in `main.go` right after `config.Load()`
  (as specified) even though the one existing analog (`STYRMAN_RUNTIME`
  allowlist) lives *inside* `config.Load` itself (`internal/config/config.go:251`)?
  **NON-BLOCKING** — the plan is explicit about the placement and the
  precedent for guards living in `main.go` post-Load also exists (the
  `AuthMode`/`AllowedEmails` check a few lines later); a reasonable
  implementer follows the plan's literal instruction.
- Should `IPForwardOff`'s call site hardcode the literal path
  `/proc/sys/net/ipv4/ip_forward`, or should that string live as a package
  constant? **NON-BLOCKING** — the plan doesn't say; either is fine and a
  test can inject any path via the parameter regardless.
- BUG-038 status on the record (see Gap above). **BLOCKING** in the narrow
  sense that two implementers will write different things into the same
  file — one says "fixed," one leaves it untouched — and that's a factual
  claim about root cause, not a style choice.
- `verify-commit.sh`: capture `2>&1` into the log file, or stdout only?
  **NON-BLOCKING** (existing template + "safe default is merge both"
  makes this decidable alone), but flagged above because getting it wrong
  silently defeats the feature.
- Harness BUG record number. **NON-BLOCKING** — the `bugs` skill assigns
  the number in the normal dispatch path regardless of what the plan calls
  it.
- Phase 5's harness fixes (a)/(b)/(c): all three are specified with exact
  file:line citations, the chosen fix shape, and table-test inputs (recon
  brief §a/b/c, folded into the plan's Interfaces section) — buildable in
  that repo without further questions from the author.

## Not examined

- `.agents/docs/specs/styrman/versioning.md`'s full release procedure text
  (only referenced, not read, by both the plan and the recon brief).
- `web/e2e/run.mjs`'s Playwright assertions themselves (only `run.sh`'s
  trap was checked).
- `internal/config/config_test.go` in full (only grepped for
  `STYRMAN_ALLOW_UNSAFE_RUNTIME` precedent).
- The styrman-harness `apply.go`/`plan.go` code in full (recon brief's
  citations were spot-checked for shape, not re-derived line by line).
- Phase 6's `/preflight` and release mechanics beyond what's stated in the
  plan (out of scope for a cold read of the close-out plan itself).

design ready
implementation not ready

## Round 2

Re-checked the three patches against the plan file directly.

- Phase 1 (line 178): BUG-038 now explicitly resolves to `status: mitigated`,
  root cause still not established, prevention rule cited, a recurrence
  opens a NEW record, INDEX row updated to match. Closes gap 1 — no
  ambiguity left; both implementers now write the same fact.
- Interfaces (line 108): `verify-commit.sh` now specifies
  `if ! ( … ) >"${log}" 2>&1; then` with stderr explicitly included and the
  reason ("the 429 comes from BuildKit's stderr"), pointing at the existing
  `if !` shape already used for the harness step. Closes gap 2.
- Interfaces (line 128): the harness bug record now takes ITS OWN next
  number via its `bugs` dispatch (index at BUG-015), with BUG-058 named
  explicitly as crew's number only. Closes gap 3.

All three round-1 findings are resolved as stated, with no new gap
introduced by the wording used. Nothing else in the plan changed.

design ready
implementation ready
