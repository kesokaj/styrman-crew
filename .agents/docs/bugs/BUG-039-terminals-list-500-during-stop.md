---
owner: bugs
date: 2026-09-11
id: BUG-039
status: fixed
area: internal/httpapi (server.go listTerminals)
---
# BUG-039: GET /api/projects/{id}/terminals answered 500 while a container was going down

## Symptom
During a project stop or an ADR-067 network-mode rebuild (30-50s on this
host), the terminals listing returned 500, making the sidebar's terminal
list vanish behind an error for the whole window. Observed twice on
2026-09-11: the Browser-view Playwright hand-walk
(`.agents/docs/testing/2026-09-11-verify-browser-phase3.md`, "client
errors" row) and the first e2e run of the new Browser checks, where the
suite's "no uncaught client errors" gate failed on exactly this 500.

## Repro
Open a project page, stop the project (or PUT /network to flip mode),
poll GET /api/projects/{id}/terminals during the stop.

## Root cause
The handler treated a failed liveness scan (`s.tmux.Live`, a docker exec
of `tmux ls` in the guest) as a failure of the whole listing, although the
terminal RECORDS (R6: durable records over tmux sessions) are the truth
and liveness is a best-effort annotation on top of them.

## Fix
2026-09-11, Phase 4 commit of the browser-view plan: on a scan error the
handler logs "terminals: live scan failed, reporting none live" and
answers 200 with every record marked not live, instead of 500.

## Prevention
A listing whose rows come from a durable store must never 500 because a
best-effort annotation of those rows failed — degrade the annotation,
keep the rows, log why. Same shape as the existing ListeningPorts handling
for a vanished container (see BUG-023, same class of "vanished guest
container fails both transport and exit-code paths").

## Related
BUG-023 (same class: vanished guest container during a mode rebuild
breaks an annotation call, not the underlying data).
