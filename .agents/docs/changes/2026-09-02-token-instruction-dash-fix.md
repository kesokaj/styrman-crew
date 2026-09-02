# Fix dash-incompatible `read -s` in the printed token-setup instructions (date: 2026-09-02)

## Summary
`deploy/host-setup.sh`'s step 8c auto-update instructions told the operator to run
`sudo sh -c 'umask 077; read -rs t; printf "%s" "$t" > /etc/styrman/gh-token'`. On Debian
`/bin/sh` is dash, whose `read` has no `-s`, so the command died with "read: Illegal option
-s" — but the `printf` still ran with an empty `$t`, TRUNCATING `/etc/styrman/gh-token` to 0
bytes. The operator hit exactly this on 2026-09-02 while rotating the token; the updater then
fails at every fetch until the token file is re-written. Fixed to `sudo bash -c '...'` (bash
supports `read -s`), so the read either succeeds silently or fails without touching the file.

## Type
bugfix

## Files changed
- `deploy/host-setup.sh` — step 8c token snippet, `sudo sh -c` → `sudo bash -c`.
- The `deploy.md` copy of this snippet, if present, fixed alongside.

## Verification
Manual re-read of the printed instructions; `sudo bash -c 'umask 077; read -rs t; ...'` runs
`read -s` under bash where the flag exists. Operator re-wrote `/etc/styrman/gh-token` by hand
to recover from the truncation before this fix landed.

## Requirements
n/a — instruction-text bugfix, not a `REQUIREMENTS.md` item.
