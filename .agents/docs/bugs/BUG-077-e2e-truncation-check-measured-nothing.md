---
owner: bugs
date: 2026-09-22
id: BUG-077
status: fixed
area: web/e2e/run.mjs
---
# BUG-077: e2e truncation-notice check waited on the echoed command, not its output

## Symptom
`web/e2e/run.mjs`'s check "a directory past the listing cap says so" waited
for `document.body.innerText` to contain `BIGDONE` while typing the command
`echo BIGDONE`. The terminal echoes the typed line, so the wait was
satisfied before any of the 5200 files were created — the check measured
nothing. It passed only because a following 30-second `waitForSelector`
absorbed the actual work. `run.mjs` already establishes the opposite
convention twice with a comment saying why (`:190`, `:202-206`: assert the
output sentinel, never the echoed command). Found by the v1.38.0 release
review. That check is also the ONLY coverage for the truncation notice,
which has no unit test — the one test guarding an untested feature was
itself measuring nothing.

## Repro
Run the pre-fix check; it reports pass immediately on typing `echo
BIGDONE`, before the 5200-file directory listing has been created or the
truncation notice has rendered.

## Root cause
The wait's sentinel string could appear in the terminal's echo of the
typed input, before the command executed, so the assertion did not depend
on the behavior it claimed to cover.

## Fix
Wait for `BIG-5200-DONE`, a sentinel the shell computes via `ls big | wc
-l`, asserted against `.xterm-rows`; it cannot appear until the files
exist and proves how many do.

## Prevention
A terminal e2e wait asserts a sentinel the typed line cannot contain —
compute it in the shell. `run.mjs:190` and `:202-206` already document
this convention; it was not consulted when this check was written.

## Related
BUG-078 (the same check's failure mode cascaded into 8 unrelated checks
when made to fail on purpose). Candidate for a shared prevention rule with
BUG-078 under `web/e2e/run.mjs`'s own conventions comment (`:190`,
`:202-206`) — the existing comment already states the rule but was not
followed; recommend strengthening it to name this as a recurrence, or
promoting the pattern into a checked e2e-authoring note the file itself
enforces.
