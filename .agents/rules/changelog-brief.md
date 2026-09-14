# Changelog dispatches: a closed fact list, then read the draft (BUG-053 family)

The `changelog` agent invents what it is not told: 2026-09-14 alone it named
the range's HEAD (a handoff) as "the sole substantive commit", credited the
verifier with "integration Tier 1 passing" it never ran, and wrote
"matching the pattern of every other boot path" where the opposite was the
point — the ninth correction in three days (v1.30.0, v1.31.0, v1.32.0
handoffs record the earlier ones).

- The brief is a CLOSED list: every file, test name, agent, verdict and
  SHA the entry may name, with "use ONLY these facts; if a fact is missing,
  leave it out" — and "the release commit" for a SHA that does not exist
  yet, never a placeholder the agent might fill.
- Name what each commit in a range IS (fix / handoff / docs); the agent
  cannot tell them apart from `git log` and will guess.
- Read the whole draft before it is staged: `grep -v '^$' <entry>`. A
  correction is a `sed` on the file, and the correction count goes in the
  handoff's Agent notes so the pattern stays visible.
- The same brief shape applies to the `bugs` and `documentation` agents;
  their drafts have needed fewer corrections but not zero (a split README
  sentence, a chronology sentence, 2026-09-14).

Harness side: a proposal to amend `agents/changelog.md` itself is queued
(retro 2026-09-14); until it lands, this rule is the project's guard.
