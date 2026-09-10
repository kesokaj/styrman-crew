---
owner: bugs
date: 2026-09-10
id: BUG-037
status: fixed
area: build normaliser — web/ generated script token (TODO close-out)
---
# BUG-037: octal-looking placeholder written into JS source was a syntax hazard

## Symptom
The first build normaliser wrote the literal token `u:0000000000000` into the
page's inline script, chosen for stable byte-length so replacement would not
shift offsets. Found by security review in Phase 2: a leading-zero numeric
token is octal-literal-shaped and a hazard in strict-mode JS parsing.

## Repro
Run the build normaliser; inspect the emitted script for the `u:0...0`
placeholder; `node --check` (or strict-mode parse) on the surrounding context
flags the leading-zero digit sequence.

## Root cause
The placeholder was designed only against one constraint (fixed byte length
for safe in-place replacement) without checking that the chosen bytes were
valid/safe in the TARGET language the generated file is written in.

## Fix
Placeholder changed to `u:0`, found by security review Phase 2 (see plan
`.agents/docs/plans/2026-09-10-todo-closeout.md`).

## Prevention
A generated token that lands inside another language's source must be
checked by that language's own parser (here: `node --check` on the build
output), not just validated against the generator's own constraints
(length, charset chosen in isolation).

## Related
Same TODO close-out as BUG-034/035/036; same underlying pattern of "verify
with the target's own tool, not by inspection."
