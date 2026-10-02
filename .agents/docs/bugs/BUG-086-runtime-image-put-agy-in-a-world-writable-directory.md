---
owner: bugs
date: 2026-10-02
id: BUG-086
status: fixed
area: image/Dockerfile, image/entrypoint.sh (runtime image)
---
# BUG-086: the runtime image put agy in a world-writable directory

## Symptom
`image/Dockerfile` created `/opt/agy` with `install -d -m 0777`, and `/usr/local/bin/agy` resolves into it. In a microVM of that image, `runuser -u nobody -- test -w /opt/agy` succeeded: any uid in the VM could replace the binary the project user runs next, which would then run with that user's passwordless sudo and brokered credentials. Found by the `security` review of `1fe28a0..4460e1b`. Committed and pushed in `11e4f9a`, never released.

## Repro
`docker run --rm --runtime mvm --entrypoint sh <image> -c 'ls -ldn /opt/agy; runuser -u nobody -- test -w /opt/agy && echo writable'` printed `drwxrwxrwx 2 0 0 ... /opt/agy` and `writable`.

## Root cause
The fix for BUG-080 (agy could not update itself) needed the directory writable by the project user, whose uid is not known at image build. The author chose mode 0777 and wrote, in four places (the Dockerfile comment, ADR-077, the plan, the docs), that "the mode opens no boundary" because the project user is root in its own VM already. That reasoning considered one principal and missed every other uid in the VM (a daemon's worker, `_apt`). The plan's cold reader had asked whether root ever executes agy; nobody asked who else could write the directory.

## Fix
- `image/Dockerfile`: `install -d -m 0755 /opt/agy`.
- `image/entrypoint.sh`: `chown "${UID_}:${GID_}" /opt/agy` on every boot, before the minimal-mode branch. The directory only; the binary stays root-owned (replacing a file needs its directory, and this keeps the boot-time change a metadata one).
- Decision: ADR-080. Fixed in the release after v1.39.0.

## Tests
Tier 2 (`.agents/docs/testing/styrman/integration.sh`) check "agy's directory is owner-only: the project user's, mode 755, not writable by another uid", with `runuser -u nobody -- true` as its positive control; a green run printed `passed=119 failed=0 skipped=3`. Must-fail: the repro probe against the 0777 image. Hand run in a real booted project of the rebuilt image: `/opt/agy` is the project user's, `agy update` went 1.2.2 to 1.2.14.

## Prevention
"No boundary, the user is root here anyway" names ONE principal. Before making a path writable, list every uid that exists where it lives and what each could plant there for someone else to run; write access goes to the one uid that needs it (ownership), never to everyone (mode).

## Related
BUG-080 (the fix that introduced it); BUG-009 (root code below a lower-privileged owner's directory, the same family from the other side); `.agents/rules/guest-writable-paths.md`; ADR-077; ADR-080.
