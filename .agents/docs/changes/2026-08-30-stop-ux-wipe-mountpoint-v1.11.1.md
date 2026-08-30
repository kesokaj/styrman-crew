# Stop UX polish + non-root wipe-delete fix (v1.11.1)

Date: 2026-08-30. Follow-up to v1.11.0 from live operator testing ("terminal
says paused but the light is still green… really slow to start after
stopped") plus the diagnosis of the integration suite's pre-existing
"project gone from API" failure.

## 1. Deterministic "Project stopped" on every pane

On Stop, the dying execs raced `ExitCodeWait`: some panes landed on 4000
("Paused — the shell exited") and others on a transient close, so the same
Stop looked different per pane. The bridge now overrides the close decision
whenever the project carries the deliberately-stopped marker — every pane
gets **4002 "project stopped"** (`internal/terminal/bridge.go`).

## 2. Optimistic stopping…/starting… in the sidebar

Stop takes 10–40 s server-side (graceful guest shutdown → container stop)
and the list poll is 5 s, so the project dot stayed green long after the
click. `Sidebar.tsx` now tracks a per-project pending state: the dot pulses
amber with a "stopping…"/"starting…" label the moment the action is clicked,
and the menu's Stop/Start button is disabled until the request settles.

## 3. Wipe-delete stranded the record under a non-root server

Root cause of the suite's pre-existing "project gone from API" failure:
dockerd creates a missing bind-mount TARGET as root — the workspace mounts
at `~/projects/<dir>` INSIDE the home bind, so a root-owned
`<phome>/projects/<dir>` appeared in every per-project home. A non-root
server's wipe (`os.RemoveAll`) then failed with EACCES and M18 correctly
kept the record listed ("wipe incomplete") forever. The orchestrator now
pre-creates the mount target itself (0755, exactly what dockerd would
create) before `ContainerCreate` — a root server keeps today's layout
bit-for-bit; a non-root server can now wipe cleanly
(`internal/orchestrator/orchestrator.go`).

Production (root server) was never affected; this mattered for the
integration suite and any future non-root deployment.

## Why "slow to start after stop" (no code change)

The operator's stop/start recreated the container: its rootfs predated the
v1.10.1 runtime-image rebuild, and image upgrades apply on the next
stop/start by design (recreate + full first boot). Subsequent stop/starts
are a normal Kata cold boot (~5–10 s). Verified: the container's CreatedAt
jumped to the moment of the start, right after the image rebuild.

## Verification

- Repro harness: before the fix, wipe-delete left root-owned
  `homes/<u>/p/<id>/projects/<dir>` and the record listed with "wipe
  incomplete"; after, the mountpoint is server-owned, the record is gone in
  4 s, the home wipes fully, no log errors.
- go build/vet/tests green; web tsc/oxlint/build clean.
- Deployed v1.11.1; crew behind the OAuth gate (302), server healthy.
