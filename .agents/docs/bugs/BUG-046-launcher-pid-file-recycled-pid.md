---
owner: bugs
date: 2026-09-13
id: BUG-046
status: fixed
area: image/configs/styrman-browser (launcher pid handling)
---
# BUG-046: launcher trusted a recycled pid from a pid file surviving VM restart

## Symptom
Launcher pid files under the bind-mounted home survive VM restarts.
`kill_group_file` signalled `-<pid>` unconditionally — potentially a recycled
pid belonging to an unrelated process after restart — and `forwarder_up`
counted ANY listener on :9222 as "our" forwarder.

## Repro
Not reproduced as a live incident; found in review of restart-survival
behavior of pid files.

## Root cause
A pid file was treated as an authority ("this pid is still our process")
rather than a hint, with no identity check against what the OS says the
process actually is after a restart cycle recycles pids.

## Fix
`/proc/<pid>/comm` predicate: signal when the process group has members AND
the leader matches by comm, or the leader is altogether gone (still signal
the group in that case — a first fix design that required the leader to
match, else drop, would have defeated `do_start`'s orphan cleanup; caught in
cold-read round 2). Forwarder ownership now checked via `socat.pid`, not by
"something is listening on :9222".

## Prevention
A pid file is a hint, never an authority — always confirm identity (comm)
before acting on a recovered pid. A "still our process" check must
preserve the dead-leader-live-children case, not just add a match
requirement blindly.

## Related
Cold-read round 2, .agents/docs/reviews/2026-09-13-coldread-browser-review-fixes*.md.
