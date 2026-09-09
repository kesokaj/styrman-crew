---
owner: bugs
date: 2026-09-09
id: BUG-031
status: fixed
area: internal/orchestrator/orchestrator.go (bridgeModeMismatch, EnsureRunning)
---
# BUG-031: an inspect error in the mode-mismatch check read as "no mismatch", restarting a revoked project on its lan- bridge

## Symptom
Found by the `/preflight` security review of the release candidate
(`reviews/2026-09-09-preflight-security-v1.25.0.md`, medium/medium), the
third fail-open in the same revocation path in one day. Not caught by any
suite: it needs `docker network inspect` to fail at exactly the moment a
project is being rebuilt for a mode change.

## Repro
Project in LAN mode. PUT `lanAccess:false`. The handler stops the container
(success) and calls `EnsureRunning(Lan:false)`. Inside, for a stopped
container, `bridgeModeMismatch` inspects the network to decide whether the
bridge must be rebuilt. If that inspect fails, it returned `false` — "no
mismatch" — so `EnsureRunning` simply STARTED the existing container on its
existing `lan-` bridge. The record says sandbox; the container has LAN.

## Root cause
    insp, err := o.docker.NetworkInspect(...)
    if err != nil {
        return false
    }

The doc comment above it said so in words: "an inspect error is not a
mismatch". Written for the ordinary start path, where a transient inspect
failure should not cost a container rebuild. Correct there; on the mode-change
path the same line turns "I could not look" into "nothing to do" — the exact
shape of BUG-029 in the handler above it, one layer down.

## Fix
`bridgeModeMismatch` returns `(bool, error)`. A NotFound is still "not a
mismatch" (`recreateMissingNetwork` owns it and builds the bridge under the
right name). Any other inspect error is returned, and `EnsureRunning` refuses
to start: "cannot inspect the project network, refusing to start on an
unverified bridge". The container stays DOWN — the safe state, since nothing
is in force — and in the handler that is the `EnsureRunning`-error branch,
which keeps the new record so the next start retries the check. **No unit
test**: `internal/orchestrator` has no fake Docker client (BUG-003's
residual). Covered by reading and by the handler tests above it.

## Prevention
The same rule as BUG-029, one layer lower: an `err != nil` that returns a
DEFAULT answer (`false`, "no", "stopped") is only safe when the default is the
fail-closed one. Trace what the caller does with that default. Three
fail-opens in one path in one day — BUG-028, BUG-029, BUG-031 — each one line
from the last, each closed by a review rather than a test, is the strongest
argument yet for BUG-003's missing orchestrator seam.

Related: BUG-028, BUG-029 (the handler), BUG-030 (found beside it), BUG-003.
