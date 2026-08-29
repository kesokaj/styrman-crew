# Terminal status bar — live VM resource usage (v1.6.0)

Date: 2026-08-29. Operator request: a thin bar under the terminal (like
argolis-devbox) showing CPU, memory, disk, processes, IP and other info. As a
bonus it gives the terminal pane a hard bottom edge.

## Backend

New `GET /api/projects/{project}/stats` (`internal/httpapi/stats.go`) returns a
live snapshot of the project's microVM:

- CPU % (two `/proc/stat` samples 0.2s apart), vCPU count, 1-min load
- memory used/total (from `/proc/meminfo`)
- disk used/total (`df` of the guest `$HOME`)
- process count (`ps -e`), uptime (`/proc/uptime`), guest hostname
- bridge IP (resolved host-side via `orchestrator.IPAddress`)

Sampled in ONE guest pass with a **non-login** shell (`bash -c`) so it does NOT
trigger the per-login credential pull on every poll. A stopped/absent VM returns
`{state}` with no numbers; an exec failure returns zeros rather than erroring.

## Frontend

`TerminalStatusBar` (`web/src/components/TerminalStatusBar.tsx`) polls the
endpoint every 4s and renders a thin `bg-accent-soft` (blue) bar: a running dot
+ hostname, then CPU, MEM (used/total), DISK (%), PROC, IP, UP, each with a
hover title for detail. It sits directly under the terminal in the terminal
route (`web/src/routes/p.$project.$term.tsx`) as a `shrink-0` row, so the
terminal fills above it and the bar is the pane's hard bottom edge.

Files: `internal/httpapi/stats.go`, `internal/httpapi/server.go` (route),
`web/src/lib/api.ts` (`ProjectStats` + `projectStats`),
`web/src/components/TerminalStatusBar.tsx`,
`web/src/routes/p.$project.$term.tsx`.

## Note

`disk` is `df` of the guest `$HOME`, which is a host-backed mount — so the total
reflects the host volume (shown as a % to stay readable). If a project-scoped
number is wanted later, switch to the DinD image (`~/.dockerd/<project>.img`).

## Release

Built + deployed live as styrman-server 1.6.0. Endpoint verified against the
live "test" project.
