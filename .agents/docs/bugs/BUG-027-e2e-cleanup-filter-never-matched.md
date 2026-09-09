---
owner: bugs
date: 2026-09-09
id: BUG-027
status: fixed
area: web/e2e/run.sh (cleanup trap, container removal)
---
# BUG-027: the e2e cleanup's container filter never matched, so every aborted run leaked a microVM

## Symptom
A project microVM (`styrman-aab3ea11-…`, image `styrman-runtime:test`) was
found running on the host hours after the e2e runs that created it, holding
a Kata guest's resources. It was not a production project — production's
store held only `e441b4b6-…` ("test123"). Nothing in the suite's output
mentioned it; it was noticed only while gathering environment state for a
handoff.

## Repro
Abort `bash web/e2e/run.sh` before its Cleanup section — any failing run
does this, and this session had seven. The project container survives the
EXIT trap that exists to remove it.

## Root cause
`cleanup()` iterated `docker ps -aq --filter label=styrman.managed=true` and
kept a container only if its env contained `STYRMAN_STATE_DIR=${STATE}`:

    env=$(docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}')
    echo "$env" | grep -q "STYRMAN_STATE_DIR=${STATE}" && docker rm -f "$c"

**The orchestrator never sets `STYRMAN_STATE_DIR` on a project container.**
A project container's env is `STYRMAN_USER`, `STYRMAN_UID`, `STYRMAN_GID`,
`STYRMAN_CRED_BROKER`, `STYRMAN_PROJECT`, `STYRMAN_PROJECT_DIR`,
`STYRMAN_DOCKER_DISK_GB` — and nothing else. Verified twice: by inspecting a
real container's env, and by `grep -rn STYRMAN_STATE_DIR internal/orchestrator/
image/` returning nothing. The state dir is the SERVER's configuration; it
was never a property of the guest.

So the condition was unsatisfiable and the loop had never removed anything,
for as long as it had existed. It survived unnoticed because it is only a
SAFETY NET: on a run that reaches its Cleanup section the suite deletes the
project through the UI and the SERVER removes the container. The net was
only ever needed by runs that aborted — exactly the runs nobody inspects
afterwards.

Measured rather than reasoned: with a live project created under an
isolated state dir, the old filter matched **0** containers.

## Fix
Filter on the container's MOUNT SOURCES instead. Every project container
binds its home and workspace from under its server's `StateDir`
(`${STATE}/homes/<user>/p/<uuid>` and `${STATE}/projects/<user>/<uuid>`), so
a mount source under `${STATE}/` is precisely "belongs to THIS run's
isolated server" — the property the env check was reaching for. Production's
containers mount from `/var/lib/styrman` and can never match a `mktemp`'d
state dir.

Proven against a real container before and after: old filter 0 matches, new
filter exactly 1 (this run's project, not production's), and running the
real loop against an aborted run removed both the container and the state
dir with exit 0.

The `|| continue` from BUG-024 is retained on the new command substitution
for the same reason it was added: `set -e` is in force inside an EXIT trap.

## Second half: networks were never removed at all

Fixing the container filter exposed that `cleanup()` removed containers and
the state dir but **no networks**. Each e2e run left one or two `sdb-<uuid>`
networks behind, each holding a /16 from Docker's default pool — and TODO.md
already carries the risk that a 16th project bridge lands in 192.168/16, so
this is not cosmetic.

The fix took **three attempts**, all three failing silently, which is the
reportable part:

1. Guarded on `{{len .Containers}} == 0` and a `date -u -d` parse of
   `{{.Created}}`. Go renders that field as
   `2026-09-09 16:48:14.959530866 +0200 CEST` — an offset AND a zone
   abbreviation — which GNU `date` refuses. Every network hit the `|| continue`
   and the sweep did nothing whatsoever. It looked like a working loop.
2. Switched to `{{.Created.Unix}}`, letting the template convert instead of
   reparsing its prose. Correct, but still leaked one network per run: the
   server deletes projects ASYNCHRONOUSLY, so at trap time a network can still
   have its container attached, and the zero-container guard skipped exactly
   those — which then went orphan a moment later.
3. Retry the sweep (bounded, ~12 s) and drop the zero-container guard: the
   timestamp is what protects production, and `docker network rm` refuses a
   network still in use on its own, so an early attempt is harmless.

Verified by three consecutive full runs: containers none, networks none, state
dirs none, production's container and network untouched.

## Prevention
Three defects in nine lines of cleanup, none of which a green suite could
show: the container filter matched nothing, the network sweep did nothing,
and none of it had a symptom inside the run. A cleanup path that can only
ever run on the failure branch is never
exercised by a green suite — assert it works at least once, by aborting a
run deliberately or by testing the predicate against a real object, rather
than trusting that it reads correctly. And a filter that matches on a
variable must be checked against a real instance of the thing it filters:
`STYRMAN_STATE_DIR` is server configuration and was never in a guest's env,
which one `docker inspect` would have shown at any point in the loop's life.

Related: BUG-024 (the same trap, whose `set -e` abort was a *separate*
defect in the same nine lines — fixing it did not make this loop work, and
the two must not be confused), BUG-003's residual (the same absence of a
seam that lets orchestrator behaviour go unverified).
