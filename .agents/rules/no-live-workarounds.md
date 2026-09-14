# No hand-applied workarounds on the live host (operator, 2026-09-14)

When a live defect leaves a project broken, do not offer or apply host-side
or UI workarounds — no `docker network connect` on the production host, no
"stop then start from the sidebar", no "open the port preview once".

- Say whether deleting and recreating the project clears it for that
  project, and until when (the next server restart, the next release).
- The lasting fix is the release; offer `/preflight` in one line.
- A workaround never goes into a handoff's Next Steps either: the
  2026-09-14 handoff carried one and was superseded the same afternoon.

Why: "I dont want custom fixes. THis is still not a production project. I
can remove it if it solves it." (feedback log 2026-09-14T15:00:42+02:00,
after BUG-060). Projects on crew are disposable while the platform is
pre-production; the operator wants the platform fixed, not the instance
patched, and a patched instance hides whether the fix works.
