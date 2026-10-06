---
owner: bugs
date: 2026-10-06
id: BUG-094
status: open
area: release signing (deploy/gcp allowed_signers, ADR-089)
---
# BUG-094: exposed original release-signing key still listed as trusted on the GCP VM

## Symptom
Crew's original release-tag signing key `SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU` (dave's passphrase-less `~/.ssh/id_ed25519`, also the styrman-harness release key and dave's GitHub login key) was exposed on 2026-10-05 per styrman-harness ADR-053. Crew recorded it only as "replaced" (ADR-089: "The original key can no longer sign"), and `.agents/docs/deploy/gcp.md` still listed it as a trusted signer in the GCP VM's `/etc/styrman/allowed_signers`. An exposed key CAN still sign: any host listing it would install a crew tag signed by whoever holds the copy.

## Repro
Not a runtime failure. Read harness ADR-053 (`gh api`) against ADR-089 and `gcp.md`.

## Root cause
ADR-089 recorded the rotation as a replacement. The exposure, recorded in the harness repository, never reached crew's records, so the per-host signer lists were never told to drop the key.

## Fix
Crew records (this change): ADR-089 amendment 1 in DECISIONS.md (the key is exposed and must be listed by no host); `.agents/docs/deploy/gcp.md` and `.agents/docs/specs/styrman/versioning.md` say the VM list must drop it.
Not yet done: remove the `5HpY...` line from the GCP VM's `/etc/styrman/allowed_signers` (operator step).
State checked 2026-10-06: dave's list holds only `SHA256:2bCWnLoMXNQ3NNSBL6F0TXb4p7aqjHR9DLWIYhca548` (`sudo -n cat`, each line via `ssh-keygen -lf`); the GitHub SSH authentication keys (`gh api user/keys`) no longer include `5HpY...`; GitHub SSH signing keys could not be listed (404 for the token). The GCP VM's list was NOT checked (no gcloud on dave).

## Prevention
When a signing key is rotated, record WHY (lost, replaced, exposed) and, if exposed, name every list that trusts it (each host's `allowed_signers`, the image's pins, GitHub) and its removal. A key retired in one repository (the harness) is checked for use in the other (crew).

## Related
Found 2026-10-06 by the `security` agent reviewing the harness v2.4.1 bump (`.agents/docs/reviews/2026-10-06-security-harness-sha256-pin.md`, finding 1); the main agent confirmed it from harness ADR-053. BUG-093 (a tag signed by an untrusted key was pushed, 2026-10-05), ADR-082, ADR-089, ADR-090. Open until the operator confirms the GCP VM's list.
