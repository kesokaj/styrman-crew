---
owner: reviewer
date: 2026-09-16
---
# Review — worktree-bug-061-062-code   (2026-09-16)

Range reviewed: working tree vs 8ada624 — .agents/docs/testing/styrman/integration.sh, image/entrypoint.sh

The diff fixes both BUG-061 and BUG-062 correctly and matches the mirror's actual
`attributionSet`/`attributionHint` behaviour (`image/harness/cmd/sh-agent/main.go`)
byte for byte. Nothing is broken. I traced every new `check()` against the real
message formats in `main.go` and against the two evidence files named in the
dispatch, both of which corroborate the fix: `install-dir-case.txt` shows the
installer emitting exactly the `note: <path> is not a regular file, so the
attribution setting could not be checked; …` line the new integration check
greps for, and `integration-run1.txt` shows a full Tier 2 run at
`passed=101 failed=0 skipped=3` with every new/changed check (BUG-061's three
harness-section checks, BUG-062's five persistence-section checks) PASSing.

## Findings

None rising to correctness/security/rule-violation severity.

## Criteria

1. Dead code — pass. No new dead code; the stale check and the two stale
   "stdout is discarded" comments BUG-061 named are replaced, not left beside
   the new text.
2. Flow tracing — pass. `entrypoint.sh`'s new filter `^settings:|^note: .*attribution`
   covers every stdout line `attributionSet`/`attributionHint` can emit for
   `install`: the create/add/symlink/oversize/could-not-write paths (all
   `settings:`-prefixed) and both `note: `-prefixed advisories (`… could not
   be checked …attribution…` and `… does not set "attribution"…`), while
   correctly excluding the unrelated `pathHint` PATH note (no "attribution"
   substring) — matches the intent stated in the updated comment. The
   integration checks' expected strings are exact substrings of the real
   `fmt.Fprintf` formats in `main.go:529` and `:659`, and both the "settings:
   line" and "note: line" checks are exercised must-pass (BUG-061's check,
   `integration-run1.txt`) and must-fail-then-pass (BUG-062's setup/note/undo/
   re-seed sequence around the planted directory), satisfying the project's
   "validate a log filter in both states" rule the bug record cites. `check()`
   (`if eval … then ok else bad fi`) never trips `set -euo pipefail` on a
   failing assertion, consistent with existing checks in the file; `mkdir`/`rm`
   setup/undo steps are themselves wrapped in `check()` so a failed plant
   reads as one FAIL rather than aborting the suite, per the added comment.
3. Standards conformance — pass. Grepped the repo for the two stale-comment
   phrases BUG-061 named ("stdout is discarded", "no previous-harness
   artifacts … settings.json)"); both live-code instances are fixed, and the
   only remaining hits are append-only trail (`.agents/docs/testing/*.md`,
   `reviews/`, `changes/`, `bugs/INDEX.md`), which this diff correctly leaves
   alone.
4. Directory layout — n/a, no new files.
5. Simplicity — pass. No new dependency; the fix is a one-line regex change
   in the entrypoint and a same-shape `check()`/`jq` addition in the test
   script, following existing patterns in both files.
6. Decoupling — n/a for a diff this small; both files stay independently
   editable, no new coupling introduced.

## Not examined

- `.agents/docs/testing/styrman/README.md` (per the dispatch, out of scope —
  the documentation agent's parallel edit; I read it only to sanity-check its
  check-count line against `integration-run1.txt`'s `101/0/3`, which it
  matches).
- The bug record files' own `status: open` / "Not fixed — pending" text
  (BUG-061-*.md, BUG-062-*.md) versus `INDEX.md`'s `fixed` — a trail
  inconsistency outside this diff's two files, for the `bugs` agent to close,
  not a code defect.
- `attributionOff`'s "does not set attribution" (off==false, non-empty
  explicit value) note path is covered by the regex by inspection but was
  not exercised live by either evidence file — only the "not a regular file"
  case was.
- No `docker` or live boot was run by me directly; conclusions rest on
  reading `main.go` plus the two provided evidence files.
