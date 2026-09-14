---
owner: bugs
date: 2026-09-14
id: BUG-060
status: fixed
area: internal/httpapi (terminal-WS boot callback) / internal/terminal
---
# BUG-060: first terminal of a stopped project never attached the server to that project's bridge — broker unreachable

## Symptom
Production, 2026-09-14 ~11:38Z, first terminal opened on a stopped project
after the v1.31.0 deploy. Login shell printed the BUG-054 MOTD line
verbatim: "credentials: the broker did not answer (2026-09-14T11:38:46Z
http://172.16.1.254:8091) — brokered tokens, gcloud ADC and the ssh key are
not in this shell; open a new terminal to retry."

## Repro
Server log for that window had only "recreating stopped container for
image upgrade", "creating microVM", "terminal attached" — no self-attach
warning, no "cred broker: served" line. `docker inspect styrman-server`
showed it on `styrman-ingress` only; the project bridge
`sdb-32c61123-…` held only the project container. From inside the guest,
`curl --connect-timeout 2 http://172.16.1.254:8091/env` timed out (rc 28).
Cause: server had just been recreated by the v1.31.0 deploy with zero
running microVMs, so boot-time `Adopt` (`cmd/styrman/main.go` ~99-111)
attached nothing (it only attaches already-running containers).

## Root cause
The terminal WebSocket handler (`internal/terminal/bridge.go` ServeHTTP,
`b.ensure` ~172) boots a stopped project via `httpapi.ensureFor`
(`internal/httpapi/server.go` ~261-279), which called only
`orch.EnsureRunning`. `EnsureRunning` deliberately never self-attaches
(NOTE at `internal/orchestrator/orchestrator.go` ~448) because attaching
reconfigures the server's netns; every OTHER boot path attaches in a
goroutine after its own response (`startProject` ~1112,
`createProject`/`provisionRepo` ~124-130, LAN toggle ~239-242), and the
port-preview proxy attaches lazily. The WS path — the one the UI's own
"Start" terminal button and every project page use — had no attach at all.
Silent before v1.30.1 (`|| true` in the credential snippet, BUG-054),
visible only once that fix added the MOTD.

## Fix
`internal/httpapi/server.go` `ensureFor` now calls `wsBoot(ctx, s.orch, …)`
(~277); new `wsBooter` interface + `wsBoot` (~281-307) call
`EnsureRunning` then `EnsureSelfAttached` synchronously, before the bridge
upgrades the socket. `internal/orchestrator/orchestrator.go` NOTE
(~448-453) extended to name which caller attaches when. Tests:
`internal/httpapi/wsboot_attach_test.go` (red before fix),
`internal/terminal/ensure_before_upgrade_test.go`. Live proof pending next
deploy — not exercised in this session (see plan/review docs dated
2026-09-14).

## Prevention
A mechanism the guest depends on at first login (broker attach) must be
reached by EVERY path that boots a VM — the terminal-WS boot callback is a
boot path. When adding or moving such a step, grep every `EnsureRunning`
caller and list them in the change entry; a "reachability" proof for the
broker must start from a stopped project after a server restart, through
the UI's own terminal, not from a project created/started by the API.
Coverage gaps left open: `EnsureSelfAttached` has no unit test of its own;
`ensureFor`'s record lookup is not covered by the new tests.

## Related
BUG-035 (mechanism reaches every caller — same family, see
`.agents/rules/mechanism-reaches-every-caller.md`), BUG-054 (broker
silently unreachable; its MOTD made this one visible), BUG-040 (a green
run against the wrong path). Three records now share the "mechanism reaches
every caller" prevention rule (BUG-035, BUG-054, BUG-060); the rule already
lives in `.agents/rules/mechanism-reaches-every-caller.md`, written for
BUG-035 on 2026-09-10 — after the attach mechanism (ADR-030, 2026-08-29)
had already been added beside a WS boot path that existed since v1.3
without ever being propagated to it; the rule could only catch the NEXT
such step, which is why its Related section now names this record.
