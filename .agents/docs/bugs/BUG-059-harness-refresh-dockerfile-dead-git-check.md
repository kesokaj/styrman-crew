---
owner: bugs
date: 2026-09-14
id: BUG-059
status: fixed
area: image/Dockerfile
---
# BUG-059: round-1 security fix added a build-time `.git` check that can never fire

## Symptom
Round-1 security review of the harness-mirror refresh (8046e4d) asked that
the "no `.git` in /styrman/harness" property be asserted at build time. The
fix added `test ! -e /styrman/harness/.git && \` to the runtime stage's RUN
in `image/Dockerfile`, with a comment claiming a refresh shipping a `.git`
"fails here, not at every project boot". Round-2 security review
(`.agents/docs/reviews/2026-09-14-security-harness-refresh-8046e4d-r2.md`)
found the assertion dead: `image/.dockerignore` already lists `.git` and
`**/.git`, so a `.git` under `image/harness/` never enters the build
context. Round-2 verifier
(`.agents/docs/testing/2026-09-14-verify-harness-refresh-8046e4d-r2.md`,
step 4b) proved it — a scratch copy of `image/` with a planted
`harness/.git` built successfully, rc 0, both cached and `--no-cache`.

## Repro
Copy `image/` to scratch, `mkdir image/harness/.git`, `docker build`; the
`test ! -e` RUN line still passes because `.dockerignore` strips `.git`
before the build context is even sent.

## Root cause
A new guard was added without checking whether another mechanism already
enforced the same property. `.dockerignore` already made `.git` unreachable
inside the build context, so the RUN-line assertion had no input that could
ever trigger it. The fix looked correct because the positive case (image
builds, suite green) passed — nobody made the guard fail once on purpose.

## Fix
Reverted the dead RUN-line check; `image/Dockerfile`'s comment now names
`.dockerignore` as the actual guarantee, and `image/entrypoint.sh`'s comment
says the same. Round-3 security review
(`.agents/docs/reviews/2026-09-14-security-harness-refresh-8046e4d-r3.md`)
closed the finding and asked instead for a one-line assertion in
`deploy/verify-commit.sh`'s runtime-image step (the gate container that
already runs); applied.

## Prevention
A new assertion, guard, or check is trusted only after it has been made to
FAIL once on purpose (plant the condition, watch it go red) — same rule as
the global "silence from your own instrumentation is not evidence" and
BUG-034. Before adding a guard, grep for the mechanism that may already
enforce the property (`.dockerignore`, a firewall rule, a Go gate); a
second guard is either dead or a contradiction.

## Related
BUG-034, BUG-035 (same family — same-project rule already exists; this is
its third instance and should graduate to a project rule if it recurs
again).
