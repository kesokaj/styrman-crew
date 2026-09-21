---
owner: bugs
date: 2026-09-21
id: BUG-067
status: fixed
area: image/entrypoint.sh (ADR-074 never-downgrade guard)
---
# BUG-067: harness never-downgrade guard misreported skips and let "unknown" outrank a real version

## Symptom
Found by code review (`.agents/docs/reviews/2026-09-21-review-sh-agent-release-binary.md`,
finding 3) and security review
(`.agents/docs/reviews/2026-09-21-security-sh-agent-release-binary.md`,
finding 3) before release. Two defects in the same guard:
(a) when the guard skipped `sh-agent install` because the home's manifest
named a newer release, the block still logged the pass line
"agent harness deployed (…)" for that boot, misstating what happened.
(b) when `/styrman/bin/sh-agent version` failed, `HARNESS_REL=unknown`, and
`sort -V` ranks any `vX.Y.Z` above `unknown`, so the guard skipped install
for any home with an embedded manifest, wrote the stamp
`<version>+unknown`, and logged "deployed … sh-agent unknown" —
permanently, since the stamp suppressed retries.

## Repro
Not reproduced live; found by review reading the entrypoint's comparison
logic and its `sort -V` use against an `unknown` release string.

## Root cause
The comparison trusted its inputs' shape (a release string on each side)
without asserting it, so a sentinel value (`unknown`) participated in a
version sort as if it were a version. Separately, the pass-line log did not
distinguish "installed" from "skipped", so a skip read as a deploy.

## Fix
The comparison is now a pure function `harness_home_newer <home> <image>`,
true only when BOTH are canonical `vX.Y.Z` and the home's is strictly
newer; an `unknown` image release takes a WARN path with `HI_OK=0`, no
install and no stamp (retried next boot). A skipped install logs
"agent harness kept (…)" and never "deployed". Tests:
`TestEntrypointHarnessHomeNewer` (`internal/config/entrypoint_guard_test.go`,
extracts the function; the `(v9.9.9, unknown)` case fails against a mutant
without the image-side canonical check) and Tier 2's check that a
left-alone boot logs "kept", not a new "deployed" line.

## Prevention
A guard comparing two values whose shape comes from a command's output
asserts the shape of BOTH sides before comparing — a sentinel like
`unknown` must lose, never win, a version comparison — and a pass line
names what the boot did (`deployed` vs `kept`) so a skipped step cannot read
as a done one. A boot-time step that keys a stamp on a value it could not
read writes no stamp.

## Related
Same "silence from your own instrumentation"/misleading-green family as
BUG-034 and the global hard rules on verified claims. No existing project
rule covered this shape (guard on unvalidated sentinel input); worth adding
to a rules file if a similar `sort -V`/version-comparison pattern recurs.
