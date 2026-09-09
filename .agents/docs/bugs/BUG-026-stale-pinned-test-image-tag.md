---
owner: bugs
date: 2026-09-09
id: BUG-026
status: fixed
area: image/ + testing harness (pinned styrman-runtime:test tag)
---
# BUG-026: stale pinned styrman-runtime:test image made a shell-skel change from three days earlier read as a regression

## Symptom
Three integration checks failed — "seeded .bash_exports carries the new
GOPATH", "login shell resolves GOPATH to $HOME/.go", "$HOME/.go/bin joins
PATH once it exists" — reading exactly like a regression in the v1.22.0
shell skel.

## Repro
Run the integration suite against `styrman-runtime:test` without rebuilding
it after a skel change lands.

## Root cause
Not a code defect. `integration.sh` and `web/e2e/run.sh` both default to
the pinned tag `styrman-runtime:test`, deliberately, so a test run cannot
re-stamp `styrman-runtime:dev`, which `deploy/.env` points production at
(a real 2026-09-01 audit finding). But nothing keeps `:test` current. The
image on this host was built 2026-09-04 08:22, before v1.22.0 (built
14:59 the same day) introduced `GOPATH="$HOME/.go"` in
`image/skel/bash_exports`; it still carried the old `$HOME/go`. Verified
by reading the skel out of three images: `:test` had `$HOME/go`, `:dev`
and `:1.24.1` had `$HOME/.go`. Rebuilding `:test` from current `image/`
made all three pass (84/0/3).

## Fix
Rebuilt `styrman-runtime:test`. The default tag is unchanged and should
stay unchanged — the isolation it buys production from re-stamping is
correct and not the problem.

## Prevention
A pinned image tag that no build step refreshes will silently age into
false test failures that read as source regressions. Before treating an
image-dependent check failure as a code regression, compare the tag's
build date against the commit that introduced the behaviour under test —
and rebuild the test tag as a prerequisite of the suite, or have the suite
assert the tag is not older than the source it exercises.

## Related
BUG-021 (same ADR-067 Phase 5 suite run). The deliberate `:test`/`:dev`
split is documented at the 2026-09-01 audit; this bug is about its upkeep,
not its existence.
