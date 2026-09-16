---
owner: bugs
date: 2026-09-16
id: BUG-062
status: fixed
area: image/entrypoint.sh (install stdout filter, lines 159-170)
---
# BUG-062: entrypoint's `^settings:` boot-log filter drops the advisory-path lines it was added to surface

## Symptom
image/entrypoint.sh:159-170 keeps the installer's stdout and logs only
lines matching `^settings:`, with a comment saying this makes "a created,
refused or failed ~/.claude/settings.json visible in the boot log". It
does not for every refusal case.

## Repro
In the mirror's `cmd/sh-agent/main.go`, `attributionSet` returns silently
when the existing `~/.claude/settings.json` is not a regular file, cannot
be read, or is not a JSON object; the only report of those cases is
`attributionHint(home, "note: ", out)` (main.go:167), whose lines start
with `note: ` (main.go:654-663; `TestAttributionNote` cases "a directory"
and "a fifo"). The `^settings:` grep in entrypoint.sh drops every `note: `
line. In exactly those cases the key is never written, so that project's
commits carry the Co-Authored-By trailer with nothing shown in the boot
log. Found by the reviewer (finding 3, medium, confidence high) in
`.agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md`; mechanism
confirmed by the main agent reading main.go:167 and :654-663. Not run
live — no boot with a non-regular-file/unreadable/non-JSON
`~/.claude/settings.json` was exercised to observe the empty log.

## Root cause
The filter was added in commit 6a0b39e as the fix to finding 2 of
`.agents/docs/reviews/2026-09-16-security-harness-refresh-4aa19be.md`, and
was written from the `settings:`-prefixed messages of the create/add paths
only — the advisory (`note: `-prefixed) path was not checked against the
filter before it shipped.

## Fix
Fixed 2026-09-16 under the bug skill, main agent, in the working tree (the
fix commit; not yet committed at time of writing). In `image/entrypoint.sh`,
the `printf '%s\n' "${HI_OUT}" | grep '^settings:'` filter became `grep -E
'^settings:|^note: .*attribution'`, still followed by `tr -cd
'[:print:]\n' | cut -c1-200`. The comment above it now says which lines are
logged: the `settings:` line of the create, add and could-not-write paths,
and the `note: … attribution …` advisory printed instead when the file
cannot be judged (not a regular file, unreadable, not a JSON object) — and
that the installer's separate PATH-hint note (`note: <home>/.local/bin is
not on PATH; add it so \`sh-agent\` is available everywhere`, printed on
every seed in this image) is deliberately not logged. Must-fail evidence
before the fix: the installer's stdout for a home with a directory planted
at `~/.claude/settings.json` (captured from `styrman-runtime:test`,
`.agents/scratch/bug-061-062/install-dir-case.txt`; installer exit 0)
contains `note: /tmp/h2/.claude/settings.json is not a regular file, so the
attribution setting could not be checked; Claude Code adds a Co-Authored-By
trailer …` — the old filter yielded 0 lines against it, the new filter
yields that line. Tier 2 checks added in `integration.sh`: `BUG-062 setup:
stamp removed, a directory planted in settings.json's place` (line 404),
`boot log carries the installer's note: line for a settings.json it could
not judge (BUG-062)` (line 415), `the seed still completed around the
refused key (stamp rewritten)` (line 417), `BUG-062 undo: plant removed,
stamp removed for one more seed` (line 419), and `re-seed after the
BUG-062 undo recreates settings.json with the key` (line 460) — all PASS
in the Tier 2 run recorded on BUG-061 (`passed=101 failed=0 skipped=3
aborted=0`).

## Prevention
A log filter added so that a condition is visible is validated once in
the must-pass state and once in the must-fail state — the project's
"silence from your own instrumentation" rule. This fix did exactly that;
the must-pass and must-fail states are now permanent Tier 2 checks
(above), not a one-off manual capture.

## Related
BUG-061 (same commit 6a0b39e, same skipped code-lens review, same
"stdout discarded" stale-comment pair at integration.sh:119-121 and
image/entrypoint.sh:126-128). Review
`.agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md`; security
review `.agents/docs/reviews/2026-09-16-security-harness-refresh-4aa19be.md`
finding 2 (the filter this bug is a defect in).
