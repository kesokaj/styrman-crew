---
owner: bugs
date: 2026-10-05
id: BUG-093
status: fixed
area: process — release tag pre-push signature check (deploy/styrman-update trust list)
---
# BUG-093: a release tag signed by a key the hosts do not trust was pushed; "No principal matched." was read as a pass

## Symptom
v1.42.4 was tagged at 20:05 (`git tag -s v1.42.4 -m "styrman v1.42.4"`, commit 38d1a0c) and pushed.
dave's auto-updater refused it at 20:10:19 with status
`{"result":"failing","code":"signature","deployed":"v1.42.3","latest":"v1.42.4"}`.

## Repro
Sign a tag with a key absent from `/etc/styrman/allowed_signers`, then run the updater.
Pre-push check as run:
`git -c gpg.ssh.allowedSignersFile=.agents/scratch/allowed_signers_check tag -v v1.42.4`
printed `Good "git" signature with ED25519 key SHA256:2bCW…` then `No principal matched.`

## Root cause
`~/.ssh/id_ed25519` on dave (git `user.signingkey`) was replaced at 17:19 the same day, after v1.42.3
shipped. v1.42.3 was signed by `SHA256:5HpYQJjpG+…`; v1.42.4 by
`SHA256:2bCWnLoMXNQ3NNSBL6F0TXb4p7aqjHR9DLWIYhca548`. `/etc/styrman/allowed_signers` still listed only
the old key. The main agent filtered the pre-push output with `grep -E "Good|BAD|error"`, which showed
the first line and hid `No principal matched.`, and reported the tag verified. A matching signer prints
`Good "git" signature for <principal> with …` (with "for"); this one had no "for". The scratch allowlist
used for the check was a stale copy, not the host's installed list, so the check could not speak for
what the hosts trust.

## Fix
2026-10-05, operator: replaced dave's `/etc/styrman/allowed_signers` with the new key's line
(`ostling.simon@gmail.com ssh-ed25519 <new key>`); the old private key no longer exists on dave. The
main agent then confirmed `Good "git" signature for ostling.simon@gmail.com with ED25519 key SHA256:2bCW…`
against the same line. The GCP VM's list does not yet hold the new key (operator step, open).

## Prevention
Before pushing a release tag, verify it against the signer line the hosts actually trust (not a scratch
copy) and require the principal in the output: `grep 'Good "git" signature for '`, never a grep for
"Good". A filter that can hide "No principal matched." is the BUG-033/041 shape (a verification read
through a filter); rule already exists in `.agents/rules/verify-bare.md` and was not followed.

## Related
BUG-033, BUG-041 (`.agents/rules/verify-bare.md`).
