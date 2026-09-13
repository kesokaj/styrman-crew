---
owner: main
date: 2026-09-13
---
# Integration tier — v1.30.0 gate (final tree e6ce8c4, pinned image)

Run by the main thread: `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` (image rebuilt from the audit-fix tree: Go 1.27.1, Node 22.23.2, claude-code 2.1.270, agy 1.2.2). An earlier run at e18d62c gave the same result (`.agents/scratch/integration-audit.log`). Verbatim output, colour codes stripped:

```
ensure-test-image: styrman-runtime:test is current (built 2026-09-13T12:26:20)

=== build ===

=== start server (state=/tmp/styrman-itest-Viie, port=18080) ===
PASS health endpoint
PASS embedded SPA served
PASS SPA deep-link fallback
PASS unknown API path is a JSON 404

=== create project (boots microVM, git init) ===
created in 0s: {"id":"15f68377-0f20-44c5-b1f5-392ee2b52816","name":"itest","dir":"itest","createdAt":"2026-09-13T10:38:16.400356951Z","state":"creating"}
PASS create returns immediately (async boot)
PASS project got a UUID
PASS microVM became ready in the background
PASS microVM kernel differs from host
PASS workspace dir on host
PASS git repo initialized
PASS in-guest dockerd runs overlay2

=== agent harness (ADR-062: sh-agent seeded write-once from the image) ===
PASS sh-agent reports no drift in the project home
PASS CLAUDE.md is the two-line import stub
PASS 28 skills and 7 agy agents seeded
PASS no previous-harness artifacts (shell gates, GEMINI.md, settings.json)

=== browser (ADR-068: one headless Chromium per project, MCP-registered for both agents) ===
PASS browser MCP server registered for claude and agy
no peer address for :9222 (STYRMAN_BROWSER_PEER unset): browser stays loopback-only
PASS styrman-browser start answers /json/version with the sandbox on
no peer address for :9222 (STYRMAN_BROWSER_PEER unset): browser stays loopback-only
PASS ports list omits the browser's 9229 while it listens
PASS preview proxy refuses the browser port with a 400
PASS browser status/start/stop routes answer for the owner (9222 up, hidden)
PASS a second start keeps the running forwarder (socat pid unchanged)
PASS styrman-browser stop ends it (status reports not running)

=== shell skel (v1.22.0: GOPATH is $HOME/.go, seeded write-once per file) ===
PASS seeded .bash_exports carries the new GOPATH
PASS login shell resolves GOPATH to $HOME/.go
PASS $HOME/.go/bin joins PATH once it exists
PASS boot to ready within R1's 15 s hard limit (measured 2.63s; soft target 5 s)

=== terminals (durable records, ADR-016) ===
PASS record created with title
PASS rename persisted
PASS attach to unknown terminal is refused

=== terminal I/O over WebSocket ===
PASS shell echoes marker
PASS cwd is the workspace
PASS terminal reported live

=== tmux session survives disconnect ===
PASS reattach sees same shell state

=== transcripts (history) ===
PASS transcript listed
PASS transcript captured the command
PASS invalid transcript id refused

=== file explorer ===
PASS project listing works (shows .git)
PASS home listing works (per-project, shows .styrman)
PASS path traversal refused
PASS file written and read back
PASS file visible on the host
PASS download serves the file

=== ports + preview proxy ===
PASS port detected
SKIP pretty URL proxies to the app (host firewall blocks host→bridge; prod unaffected)
SKIP legacy proxy URL still works (host firewall)
PASS bare URL redirects to trailing slash

=== public TTL preview links (1.1 foundation) ===
PASS link minted
SKIP public link serves the app (host firewall blocks host→bridge; prod unaffected)
PASS link listed for the project
PASS bad token 404s
PASS revoke deletes the link
PASS revoked link no longer serves

=== network mode (ADR-067) ===
PASS a new project defaults to sandbox
PASS its bridge carries the sdb- prefix (matched by the firewall drops)
PASS the running container is on that bridge
PASS admin may enable
PASS the record now says LAN
PASS the bridge was rebuilt with the lan- prefix
PASS the suffix is unchanged (same project, same identity in ip link)
PASS no styrman jump matches the lan- bridge
PASS docker still isolates the lan- bridge from other bridges
PASS the project still runs after the rebuild
PASS its home survived the rebuild
PASS disable returns 200
PASS the bridge is back to sdb-
PASS a no-op PUT reports rebuilt=false
PASS malformed body is 400
PASS unknown project is 404

=== git (in-guest client) ===
PASS git status reports the running repo
PASS git status sees the new file (untracked)
PASS git diff of the untracked file shows its content

=== settings ===
PASS settings served

=== persistence across container destruction (R3/R5) ===
PASS file survives container recreate
PASS docker.img persisted
PASS transcript survives recreate

=== terminal pause + close-all ===
PASS paused terminal keeps record, loses session
PASS close-all removed every terminal

=== multi-tenancy + uuid identity ===
PASS legacy home migrated to per-user layout
PASS container named styrman-<uuid>
PASS per-user metadata store
PASS friendly rename sticks, dir unchanged
PASS live duplicate name refused

=== stop + delete (wipe) ===
PASS container stopped
PASS terminals still listed while stopped
PASS stopped project refuses auto-reconnect (close 4002)
PASS VM stayed stopped after refused reconnect
PASS boot=1 reconnect boots the stopped VM
PASS VM running again after boot=1
PASS re-enable accepted before the delete
PASS the record says LAN before the delete
PASS delete left no netpolicy tree behind
PASS container gone (async delete)
PASS workspace wiped
PASS project gone from API

=== cleanup ===
passed=91 failed=0 skipped=3 aborted=0
rc=0
```
