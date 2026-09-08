---
owner: bugs
date: 2026-09-08
id: BUG-016
status: fixed
area: deploy/docker-compose.yml (per-project LAN access, ADR-065)
---
# BUG-016: rw bind of the LAN spool used default propagation, so a container started before the tmpfs mount wrote into an unbounded, undrained directory

## Symptom
Security review: `deploy/docker-compose.yml:130` binds
`/run/styrman-firewall` into the server container with the default
`rprivate` propagation. A `styrman-server` container started before the
spool's tmpfs `.mount` unit first mounted keeps seeing the underlying,
operator-owned plain-`/run` directory — none of the `size=1M,nr_inodes=1024`
bound the install unit imposes, and nothing on the host drains it. Combined
with BUG-015 (unthrottled no-op kick), a tenant's PUT loop accumulates files
in host RAM indefinitely. `deploy/remote.md` documented this state and
called it harmless, which was true for correctness (kicks fall back to the
60s timer) but false for availability.

## Repro
Not reproduced live; found by code reading in
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` (finding 3,
`deploy/docker-compose.yml:130`).

## Root cause
The compose bind was written for the common case (mount already present at
container start) without considering start-order: Docker's default
`rprivate` bind does not pick up a host mount that appears under the bind
source after the container starts, so the container and the host can
disagree about which directory "the spool" is.

## Fix
Bind changed to `:rslave` so a later host mount of the tmpfs propagates into
the container. In the working tree per the review; lands with the Phase 2
checkpoint commit that follows this record; `deploy/remote.md`'s "harmless"
claim to be corrected alongside.

## Prevention
When root mounts something under a path a container binds, the bind must be
`rslave` (or the container recreated after the host mount appears) — never
assume start order, and a doc that calls a stale-bind state "harmless" must
justify that for both correctness AND resource-exhaustion/availability, not
just correctness.

## Related
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` finding 3;
BUG-015 (the unthrottled kick that turns this into an accumulation, not just
a theoretical gap); BUG-012 (unbounded spool sizing, prior finding, same
mechanism family).
