---
owner: bugs
date: 2026-09-16
id: BUG-062
status: open
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
Not fixed — pending. Match `^settings:` and `^note: ` (or log every line
the installer prints on stdout, bounded as now); update the comment to say
precisely what is and is not visible in the boot log.

## Prevention
A log filter added so that a condition is visible is validated once in
the must-pass state and once in the must-fail state — the project's
"silence from your own instrumentation" rule. Here, one boot with a
directory at `~/.claude/settings.json` would have shown the empty log
before the fix shipped.

## Related
BUG-061 (same commit 6a0b39e, same skipped code-lens review, same
"stdout discarded" stale-comment pair at integration.sh:119-121 and
image/entrypoint.sh:126-128). Review
`.agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md`; security
review `.agents/docs/reviews/2026-09-16-security-harness-refresh-4aa19be.md`
finding 2 (the filter this bug is a defect in).
