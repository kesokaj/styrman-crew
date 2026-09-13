---
owner: bugs
date: 2026-09-13
id: BUG-056
status: open
area: process — release gate (deploy/verify-commit.sh)
---
# BUG-056: v1.30.1 verify-commit failed on Docker Hub 429, not a code defect

## Symptom
`deploy/verify-commit.sh 5a4dcde` (13:33) failed in the runtime-image build
step: `failed to resolve source metadata for docker.io/library/debian:trixie@sha256:6788062a…:
unexpected status from HEAD request … 429 Too Many Requests`, rc=1. Log:
`.agents/scratch/verify-commit-v1.30.1.log`. Every other gate for the commit
had already passed: verifier PASS, integration 92/0/3, e2e 92/0/2, firewall
suite 26/0.

## Repro
Run `deploy/verify-commit.sh` on a host whose Docker Hub anonymous pull
budget (100/h per IP) is already spent for the hour. A direct registry probe
confirmed `x-ratelimit-limit: 100;w=3600` exhausted for
`docker-ratelimit-source` = this host's IPv4.

## Root cause
The host pulls from Docker Hub anonymously. The day's activity — six
`ensure-test-image` builds, four `verify-commit` runs, two updater deploys
(each runtime-image build issues a HEAD per FROM line, three per build), plus
`image/bump-pins.sh`'s `docker manifest inspect` (3 calls/run, run ~5 times,
including by verifier agents) — consumed the entire 100/h anonymous budget
before the release verify ran.

## Fix
Not fixed. Decision taken: wait for the rate-limit window to refill (monitor
polls `ratelimit-remaining`, release proceeds when ≥ 30 remain); the tag is
not pushed on a failed verify.

## Prevention
- The release path depends on Docker Hub availability. `docker login` with a
  Docker Hub account (authenticated: 40/h free, more paid) or a pull-through
  cache removes this failure mode — record the operator's choice in
  `.agents/docs/deploy/remote.md` once decided.
- Registry-info-only tools (`image/bump-pins.sh`'s `manifest inspect`) must be
  run sparingly and never repeated inside verifier loops — cite a check-mode
  run instead of re-running it.
- `verify-commit.sh`'s image-build step should detect a 429 and say "Docker
  Hub rate limit — retry after the window" instead of a generic build
  failure (follow-up, not done today).
- A tag is never pushed on a failed `verify-commit`, whatever the cause —
  applies here as it does to every prior verify-then-tag gate.

## Related
Same family as BUG-034/BUG-053 (claims/records must cite real evidence — here
the evidence is the rate-limit header, not a guess); process rule to add to
`.agents/docs/deploy/remote.md` once the operator picks auth-vs-cache.
