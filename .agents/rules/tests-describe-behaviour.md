# A behaviour change includes the tests that describe it (BUG-061)

The 4aa19be harness refresh made `sh-agent install` write
`~/.claude/settings.json`; R5 and the living docs were updated, and the Tier 2
check that asserted the file ABSENT stayed, red, until the release gate found
it — the docs lens greps living docs, not test scripts, and the refresh
commit ran no code lens and no Tier 2.

- When a requirement's wording or a boot-time behaviour changes, grep the
  test suites for the old assertion, not only the docs:
  `.agents/docs/testing/**/*.sh`, `web/e2e/`, and `*_test.go` beside the code.
  A hit that still asserts the old behaviour is part of the change.
- A mirror refresh that changes what the image writes into a home runs
  Tier 2 (`integration.sh`) before its commit and gets the code lens every
  time — the Definition of Done's 50-line threshold applies to a refresh as
  to any diff.
- The Tier 2 count line in `testing/styrman/README.md` changes in the same
  commit as the checks it counts.

Same family as BUG-035 (a mechanism must reach every caller) and the
describing-text rule in the global context: a test is the sharpest
description of a behaviour, and a stale one reads as a guarantee.
