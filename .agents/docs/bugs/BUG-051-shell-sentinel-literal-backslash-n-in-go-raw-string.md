---
owner: bugs
date: 2026-09-13
id: BUG-051
status: fixed
area: internal/orchestrator (merged port-scan script, embedded in Go)
---
# BUG-051: sentinel line never stood on its own line — `printf '%s\\n'` inside a Go raw string

## Symptom
The merged port-scan shell script's sentinel was written as `printf '%s\\n'`
inside a Go raw string literal. Go's raw string does not interpret
backslash escapes, so the shell received a literal `\n` and printed it as
two characters instead of a newline — the sentinel never stood alone on its
own line as the parser expected.

## Repro
`TestPortScanScriptEmitsSentinel` runs the script on the host and asserts
sentinel-on-its-own-line; caught immediately by this test.

## Root cause
Backslash-escape intent (`\n` meaning newline) was written assuming Go's raw
string would pass it through as an escape, but a Go raw string is literal —
the two characters `\` and `n` reached the shell verbatim.

## Fix
Corrected the embedded script text so the newline is a real newline (or the
correct escape for the string type actually in use).

## Prevention
Same family as BUG-037: a shell snippet embedded in a Go raw string must be
tested by RUNNING it, not by reading it — a test that only inspects the Go
source text would have missed this exactly the way this one did not.

## Related
BUG-037 (same prevention rule, different generator language pairing —
recurring pattern; candidate for a permanent project rule on
generated-snippet testing if it recurs again).
