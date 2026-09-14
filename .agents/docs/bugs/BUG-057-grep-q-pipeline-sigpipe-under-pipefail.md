---
owner: bugs
date: 2026-09-14
id: BUG-057
status: fixed
area: .agents/docs/testing/styrman/integration.sh
---
# BUG-057: new harness check FAILed on 141 (SIGPIPE) though the image printed the exact line grepped for

## Symptom
The new check "sh-agent status labels the image mirror a plain copy"
reported FAIL on its first run (`passed=93 failed=1 skipped=3`) although
`sh-agent status` inside the image printed exactly the line it grepped for:
`repo       /styrman/harness (plain copy, not its own git checkout; update
is refused)` — confirmed by running `sh-agent status` in the image by hand.

## Repro
Inside the image, with `set -o pipefail`:
`sh-agent status … | grep -qF "plain copy"; echo rc=$?` → `rc=141`, five
runs in five. `sh-agent status … | grep -F "plain copy" >/dev/null; echo
rc=$?` → `rc=0`, five in five.

## Root cause
The script runs under `set -euo pipefail` (line 16). The check was
`docker exec … sh-agent status … | grep -qF '…'`. `grep -q` exits as soon
as it finds the first match and closes its read end; `sh-agent status`
then writes its remaining seven lines into a closed pipe and dies with
SIGPIPE (exit 141); pipefail makes the pipeline's reported status 141, not
grep's 0.

## Fix
Same session, same change: both new checks now use `grep -F … >/dev/null`
so grep consumes the whole stream instead of closing early; a comment
above them says why. Second run: `passed=94 failed=0 skipped=3`.

## Prevention
In a script under `set -o pipefail`, a grep that gates a pipeline's exit
status must read the writer to EOF — `grep -F … >/dev/null`, never
`grep -q` (or `-m1`), unless the writer is known to emit exactly one line.

## Related
Same family as BUG-040 and BUG-049 (a pipeline guard measuring something
other than the thing under test).
