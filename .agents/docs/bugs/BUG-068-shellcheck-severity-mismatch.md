---
owner: bugs
date: 2026-09-21
id: BUG-068
status: fixed
area: deploy/verify-commit.sh (release gate's `shellcheck -S warning` step) / deploy/update/styrman-update, deploy/host-setup.sh, deploy/styrman-ctl
---
# BUG-068: release gate's `shellcheck -S warning` failed SC2054 on an unquoted `--secret id=...,src=...` array element the change's own verification had missed

## Symptom
`deploy/verify-commit.sh b60d891` (the ADR-074 commit) failed at its "host
scripts: bash -n, shellcheck, systemd-analyze verify" step:
`shellcheck -S warning deploy/update/styrman-update` reported SC2054 ("Use
spaces, not commas, to separate array elements") on line 682,
`secret_args=(--secret id=gh_token,src="${GH_TOKEN_FILE}")`.

## Repro
`bash deploy/verify-commit.sh b60d891` on the ADR-074 commit; shellcheck
step fails. The comma is part of the docker `--secret` argument's value, so
the array was correct at runtime — the image had built and Tier 2 had
passed on that tree — but the gate treats a warning as a failure.

## Root cause
The change's own verification ran `shellcheck -S error` (the verifier's
dispatch named that severity), while the release gate (`verify-commit.sh`)
runs `shellcheck -S warning`. The same new `--secret id=gh_token,…` pattern
was written unquoted at five separate docker-build sites; only one branch of
`styrman-ctl` happened to be quoted already, so the mismatch surfaced only
at the stricter gate, on the release commit, not during development.

## Fix
Commit 327da6b: every `--secret id=gh_token,…` array element quoted as one
word (e.g. `(--secret "id=gh_token,src=${GH_TOKEN_FILE}")`) at all five
build sites. `shellcheck -S warning` clean on all of them;
`TestStyrmanCtlInstallPlan` still passes.

## Prevention
A verifier dispatch must run shellcheck at the severity the RELEASE GATE
uses (`-S warning`, read from `deploy/verify-commit.sh`), never a looser
severity picked independently. A new bash array element that contains a
comma is quoted as one word on the day it is written, not left for the gate
to catch.

## Related
BUG-066, BUG-067 (same ADR-074 change, same day). `deploy/verify-commit.sh`
is the authority for gate-level check severity.
