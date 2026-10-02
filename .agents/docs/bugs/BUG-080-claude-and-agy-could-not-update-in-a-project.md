---
owner: bugs
date: 2026-10-02
id: BUG-080
status: fixed
area: image/Dockerfile, image/entrypoint.sh (runtime image)
---
# BUG-080: claude and agy could not be updated inside a project

## Symptom
Operator: "I got a feeling I cant update software (claude/agy) in the project in crew."
- `agy update` -> `Error: directory /usr/local/bin is not fully accessible (readable: true, writable: false)`
- `claude doctor` -> `Can't auto-update: npm global folder isn't writable`
- A manual `claude update` printed `Successfully updated from 2.1.270 to version 2.1.287`, and `claude --version` in the same shell still printed `2.1.270`.

## Repro
In `styrman-runtime:1.38.0`, as the project user in an interactive login shell (`bash -lic`): `agy update`; `claude doctor`.

## Root cause
1. agy replaces its own binary in the directory it really lives in (it resolves symlinks, so a symlink to it from `~/.local/bin` fails the same way). The image put it in root's `/usr/local/bin`.
2. `image/skel/bash_exports` sets `NPM_CONFIG_PREFIX=$HOME/.npm-global`, and a per-project home (ADR-031) starts without that directory. Claude then reports the prefix unwritable and turns its updater off; `image/skel/bashrc` adds `~/.npm-global/bin` to `PATH` only when it exists at shell start.
3. `image/Dockerfile`'s ADR-069 note said agy "self-updates at run time anyway". In the image it could not, and nobody had run it.

## Fix
- `image/Dockerfile`: agy at `/opt/agy/agy`, `/opt/agy` mode 0777, `/usr/local/bin/agy` a symlink to it.
- `image/entrypoint.sh`: `~/.npm-global/bin` and `~/.npm-global/lib` created as the user on every non-minimal boot.
- Decision: ADR-077. Ships in the release after v1.39.0.
- Tests: Tier 2 (`.agents/docs/testing/styrman/integration.sh`) checks "~/.npm-global/bin and lib exist, user-writable, bin on a login shell's PATH", "claude doctor reports auto-updates usable (no unwritable-prefix warning)", "project user can write agy's directory; /usr/local/bin/agy resolves into it". All three FAILED against the image before the fix and pass after.
- Hand run in a real booted project of the rebuilt image: agy 1.2.2 -> 1.2.14, claude 2.1.270 -> 2.1.287. The downloads themselves are in no suite.

## Prevention
A tool the image installs for the project user is run AS that user in an interactive login shell, its own `update` and `doctor` commands included, before the image or a comment claims it works; "it updates itself" is a claim to execute, not to write down.

## Related
ADR-069 (the note that was false), ADR-031 (per-project homes start empty), BUG-034 family (a claim nobody ran), `changes/2026-09-22-review-remediation-phase-4.md:73`, `plans/2026-10-02-tool-updates-freeze-harness-v2.md`.
