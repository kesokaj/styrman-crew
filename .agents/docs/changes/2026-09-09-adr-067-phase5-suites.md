---
owner: main
date: 2026-09-09
---
# ADR-067 Phase 5 — the suites meet the rebuild

Phase 5 rewrote `integration.sh` and `web/e2e/run.mjs`/`run.sh` for ADR-067 but
never ran them. Running them is what this entry covers. Both suites are now green
and the run surfaced three defects in shipped code paths, not just in the tests.
**Note:** this covers the first two Phase 5 boxes (integration + e2e rewrites);
the plan's remaining Phase 5 boxes — the live adversarial run and the browser
criteria walked by hand — still need the operator, so the phase is NOT ticked complete.

## Unit tests first

`web/src/lib/api.session.test.ts` was failing. The `u` tagged template
(percent-encoding every interpolated path value, uncommitted from the previous
session, closing the 2026-09-03 security review's path-retargeting finding) was
correct; the test's fixture depended on the flaw it removes — it passed
`getProject("x//y")` so the mux's path-cleaning 301 would fire, and `u` now
encodes that to `x%2F%2Fy`. The case's subject was always the reload counter,
not the crafted id, so the redirecting endpoint is now named directly
(`/api/projects/<id>` redirects, `/api/projects` does not).

New file `web/src/lib/api.encoding.test.ts`: 50 cases, written against the CALL
SITES rather than against `u`, because the failure mode is not "the tag is wrong"
but "a new call site forgot it" — a test of the tag alone would stay green through
exactly that regression. Mutation-checked: reverting the tag at three sites
(revokeLink, scopeBase, the ws path) kills 11 of the 50. It asserts on path
SEGMENTS, not substrings — `encodeURIComponent` escapes the separator, not the
dots, so a correct result still reads `..%2F..%2F..` as one opaque segment; a
substring check fails on a correct result, which is how the file's own first
draft failed 46 of its cases.

It also caught a gap the previous session missed: `termSocketUrl` still built
`/ws/term/${project}/${id}` raw. The terminal `id` is the FIRST value the
2026-09-03 review named, and the /api sweep had not reached the /ws path. Now
encoded through the same tag. vitest 77 (1 failing) → 127 passing.

## Integration: two fixture bugs and a stale image

Briefly: one check re-implemented `config.BridgeNameFor`'s hash input and got it
wrong (it hashed `sdb-`+name; the code hashes the bare network name); one
asserted "no firewall rule names the lan- bridge", which is false by construction
because Docker names every bridge in four of its own rules — including the `-A
DOCKER ! -i <br> -o <br> -j DROP` that IS the cross-project isolation ADR-067
relies on. Replaced with the narrow true claim (no rule jumping into a
STYRMAN-* chain matches a `lan-` interface) plus a new positive check that
Docker's isolation DROP for the lan- bridge exists. Three further failures were
the pinned `styrman-runtime:test` tag having aged past v1.22.0's shell skel.
Result: 78/5/3 → 84/0/3. Recorded as BUG-025 and BUG-026.

## E2E: one root cause behind seventeen failures

The Phase 5 rewrite put the new toggle checks where the old read-only ones sat
— right after the Ports block starts the `:4321` fixture app and ahead of ports,
links, history, git and terminals. Under ADR-067 a toggle rebuilds the container
(measured: id 3415cb5cfff2 → 30b481786c66), so it destroyed the state every
later check needed. Make the point that this is the honest cost of the design
Phase 4 already documented in the dialog copy — the suite simply had not been
reordered to match. Worth calling out: the firewall probe reads only a
TimeoutError as the host firewall's DROP signature, so a destroyed fixture
presented as "firewall probe: fetch failed (NOT the firewall)" and the proxy
checks ran when they should have skipped — a destroyed fixture wearing a firewall
anomaly's clothes. Fix: read-only checks stay; the two toggling ones move to
their own section immediately before Cleanup. Result: 72/17/0 → 86/0/2. Recorded
as BUG-021.

## Three defects in shipped paths, not tests

| finding | issue | fix |
|---------|-------|-----|
| `setProjectLan` timeout | inherited `request()`'s 20 s abort while the PUT rebuilds the project — a user flipping the switch would see a failure for a change that worked | Now 150 s, matching `startProject`/`stopProject`, which carry it for the same reason (BUG-022) |
| `GET /ports` returned 500 during rebuild | `ListeningPorts` checks Status then Execs, and the container can vanish between them | Now re-asks once and reports an empty list when the container demonstrably went away — the shape the git endpoints already use for an absent guest; no unit test because `internal/orchestrator` has no fake Docker client (the BUG-003 residual), so the e2e is its only cover, and Phase 6's reviewer should see it (BUG-023) |
| `run.sh` exited 1 on a green run | `set -e` applies inside an EXIT trap, and `env=$(docker inspect "$c" ...)` failing on a container that vanished between `docker ps -aq` and the loop body aborted the trap before its `rm -rf` | Fixed with `\|\| continue` plus an explicit `exit "${status}"` — also left `/tmp/styrman-e2e-*` dirs behind; both symptoms one cause (BUG-024) |

Also: the client-error gate truncated at 160 chars, cutting the URL mid-UUID and
defeating the recorded-URL fix that TODO item describes; widened to 400, which is
how the failing endpoint (`/ports`) was identified at all.

## Files changed

| file | summary |
|------|---------|
| `.agents/docs/testing/styrman/integration.sh` | Phase 5's LAN-block replacement (`STYRMAN_IT_LIVE_LAN` and the firewall-fixture env var gone, `jq` guard added); the bridge-suffix hash input corrected; the false "no rule names it" check replaced by the STYRMAN-jump claim plus a positive Docker-isolation check |
| `web/e2e/run.mjs` | firewall anomaly check fix, toggle checks moved to end, client-error gate widened to 400 |
| `web/e2e/run.sh` | Phase 5's removal of the firewall-fixture env vars; the EXIT-trap abort guarded with `|| continue`; explicit `exit "${status}"` |
| `web/src/lib/api.ts` | the `u` tag (carried in uncommitted from the previous session) extended to `termSocketUrl`, the one path the /api sweep had missed; `setProjectLan` given the 150 s rebuild budget |
| `web/src/lib/api.session.test.ts` | session reloading redirected endpoint named directly |
| `internal/orchestrator/ports.go` | ListeningPorts re-asks on vanished container, returns empty list |
| new `web/src/lib/api.encoding.test.ts` | 50 cases against call sites, mutation-checked, segments not substrings |
| new `.gitattributes` | forces textual diff for `web/dist/client/_shell.html`, whose NUL comes from TanStack's prerenderer — bytes untouched, diff readable (closes TODO item, closes BUG-007 harm) |

## Verification

Go `build`/`vet` clean, `go test ./...` 15 packages ok both modules (ingress
module run separately); web `tsc -b` and `oxlint` clean, vitest 127 passing
across 9 files; `integration.sh` 84 passed / 0 failed / 3 skipped against real
Kata (the 3 skips are the documented host-firewall host→bridge ones);
`web/e2e/run.sh` 86 passed / 0 failed / 2 skipped, exit 0, no leftover state
dirs. `git diff --numstat` shows every changed file textual (BUG-007 check) and no
unnamed deletions (BUG-005/006 check); `web/dist` is a clean 15-deleted/15-new
content-hash rotation. State: nothing committed.
