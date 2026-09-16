---
owner: bugs
date: 2026-09-16
id: BUG-061
status: open
area: .agents/docs/testing/styrman/integration.sh (Tier 2), image/entrypoint.sh comment, testing README
---
# BUG-061: Tier 2 "no previous-harness artifacts" check must fail against every home seeded at 4aa19be

## Symptom
integration.sh:137-138's check `no previous-harness artifacts (shell gates,
GEMINI.md, settings.json)` runs:
`! docker exec ${CN} sh -c 'test -e ${GH}/.agents/bin || test -e ${GH}/.gemini/GEMINI.md || test -e ${GH}/.claude/settings.json'`
— this must fail against any home seeded at harness commit 4aa19be, since
`sh-agent install` now creates `~/.claude/settings.json` there by design.

## Repro
The reviewer built `image/harness/cmd/sh-agent` from this tree and ran
`install --repo image/harness --home <scratch home>`: it printed
`settings: <home>/.claude/settings.json created with "attribution":
{"commit": "", "pr": ""}` and left a 46-byte file. The main agent read
integration.sh:137-138 and confirmed the check's text. Confirmed live: the
verifier's release-range run of `bash .agents/docs/testing/styrman/integration.sh`
on 2026-09-16 (scratch output
`.agents/scratch/verify-v1.34.0-release/integration.txt`, log
`.agents/docs/testing/2026-09-16-verify-v1.34.0-release.md`) printed
`passed=93 failed=1 skipped=3 aborted=0`; the single FAIL line is
`no previous-harness artifacts (shell gates, GEMINI.md, settings.json)`
(the preceding check, `boot log records the harness deployed at the
mirror's commit`, PASSED). Tier 2 had not been run before the commit
landed (last prior Tier 2 run 2026-09-15 at a81ef4b, before the
attribution write existed; see
`.agents/docs/testing/2026-09-16-verify-harness-refresh-4aa19be.md`,
"Not examined").

## Root cause
Commit 6a0b39e (2026-09-16) changed what `sh-agent install` writes into a
seeded home (adds `~/.claude/settings.json` per ADR-072/R5 amendment), but
the test that asserts the OLD behaviour (settings.json absent) was not
updated with it. The DoD row that reads the diff for exactly this (the
code lens of the reviewer) was skipped for that commit, and Tier 2
integration was not run before the commit landed. The docs lens greps
living docs, not test scripts, so it did not catch the stale assertion
either. Two comments carry the same stale statement: integration.sh:119-121
and image/entrypoint.sh:126-128 both say install's stdout "is discarded" —
no longer true after commit 6a0b39e's stdout filter (see BUG-062).
README.md:406 listed settings.json among files a seeded home must not have
(fixed 2026-09-16 by the documentation agent, uncommitted at time of
writing); README.md:448 still names "the ~/.agents/bin gates" among
retired checks in a sentence needing the same update.

## Fix
Not fixed — pending. Keep the `.agents/bin` and `GEMINI.md` clauses;
replace the settings.json clause with a positive assertion that
`~/.claude/settings.json` holds the attribution key with both values empty
and no other key (e.g. `jq -e '.attribution.commit == "" and .attribution.pr
== "" and (keys == ["attribution"])'`); rename the check; fix the two
stale "stdout discarded" comments and README.md:448; re-measure the Tier 2
count (README's source-of-truth line currently reads
`passed=94 failed=0 skipped=3`; replacing rather than adding a check keeps
the total the same, but must be confirmed by an actual Tier 2 run).

## Prevention
A behaviour change includes the TESTS that describe it — when a
requirement's wording changes, grep the test suites
(`.agents/docs/testing/**/*.sh`, `web/e2e/`) for the old assertion, not
only the living docs. A mirror refresh that changes what `install` writes
into a home runs Tier 2 before its commit, and a mirror refresh of 50+
lines gets the code lens every time, per the project's Definition of Done.

## Related
BUG-062 (same commit, same skipped code lens, same stale-comment family).
Commit 6a0b39e; review `.agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md`
(REQUEST CHANGES); changes entry `.agents/docs/changes/2026-09-16-harness-refresh-4aa19be.md`.
Both records share "code lens skipped on a 50+ line diff" as a
contributing factor — the project's Definition of Done already
states the review-size gate (AGENTS.md step 2); this is a case of the rule
existing and not being followed for this commit, worth flagging to the
release-gate checklist rather than restating the rule.
