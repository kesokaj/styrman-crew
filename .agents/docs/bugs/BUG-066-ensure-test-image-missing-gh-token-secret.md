---
owner: bugs
date: 2026-09-21
id: BUG-066
status: fixed
area: .agents/docs/testing/styrman/ensure-test-image.sh (Tier 2 prerequisite)
---
# BUG-066: Tier 2's image rebuild failed on the new build secret the ADR-074 Dockerfile requires

## Symptom
First Tier 2 run after the ADR-074 change (`image/Dockerfile` now downloads
the sh-agent release binary from the private styrman-harness repository at
build with a docker build secret `--secret id=gh_token,…`) aborted before
any check. `ensure-test-image.sh` rebuilds `styrman-runtime:test` with a
plain `docker build` and passed no secret, so the Dockerfile's
`RUN --mount=type=secret,id=gh_token,required=true` failed. (The run was
first attempted against `styrman-runtime:dev`, which the script refuses as a
production tag; the second attempt against the default `:test` tag failed in
the rebuild.)

## Repro
`bash .agents/docs/testing/styrman/integration.sh` (Tier 2) after the
ADR-074 Dockerfile change, before `ensure-test-image.sh` was updated.

## Root cause
The ADR-074 change added the build secret to `image/Dockerfile` and to four
build sites (`deploy/update/styrman-update`, `deploy/host-setup.sh`,
`deploy/styrman-ctl`, `deploy/verify-commit.sh`) but not to the fifth: the
test suite's own image builder. A mechanism did not reach every caller —
the BUG-035 family (`.agents/rules/mechanism-reaches-every-caller.md`).

## Fix
`ensure-test-image.sh` now passes the token by the same rule (`GH_TOKEN`,
else `gh auth token`, else stop), set only on the docker build command. Tier
2 then printed `passed=111 failed=0 skipped=3` and, after the later review
fixes, `passed=112 failed=0 skipped=3` (115 checks total).

## Prevention
When `image/Dockerfile` gains a build-time input (a secret, a build arg
without a default, a build context), grep every `docker build` of `image/`
in the tree — `deploy/`, `.agents/docs/testing/`, `web/e2e/` — and name each
site in the change; the test suite's builder is a caller too. Already
covered by `.agents/rules/mechanism-reaches-every-caller.md` — that rule was
not consulted for this change.

## Related
BUG-035, BUG-060, BUG-064 (same "mechanism reaches every caller" family).
Three-plus occurrences of this family now on record; recommend the rule's
home stays `.agents/rules/mechanism-reaches-every-caller.md` but that a
change touching `image/Dockerfile`'s build inputs specifically be added as
a checklist item there, since the existing rule's examples are all
systemd/network paths, not build-time inputs.
