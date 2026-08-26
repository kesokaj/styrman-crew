# 2026-08-26 — Phase 1 review fixes, styrman-setup onboarding, conversation-model redesign

## Summary

The reviewer subagent's pass over Phase 1 returned 5 blocking findings and ~15
should-fixes; all blockers and the high-value should-fixes are now fixed and
re-verified (integration 18/18). Operator feedback after first live use added:
a guided per-user login prestep (`styrman-setup`, baked into the image),
conversation delete in the dev console, and a redesign of the conversation
concept (ADR-011) for Phase 2.

## Review findings fixed

- Terminal bridge: exec/goroutine/ghost-tmux-client leak on browser disconnect
  (handler-scoped ctx + ex.Close watcher; tmux -D kicks stale clients);
  heartbeat ping now has a 10 s deadline (was: never detected dead clients);
  wg.Wait no longer stalls up to 30 s; exit-code check polls (4000 vs normal
  close race); unknown agent → clear 500 instead of empty pane command.
- Entrypoint: stale /run/styrman/ready cleared at boot (was: instant false
  readiness after docker stop→start); ready only touched after dockerd is
  confirmed up (else exit 1); docker.img built at .tmp then mv'd (no
  half-made images silently diverting to vfs); dropped chmod 666 on socket.
- Orchestrator: per-project locks (a slow cold start no longer blocks every
  other project); idle reaper seeds adopted containers' clocks instead of
  stopping them all on the first tick after a server restart; machine-readable
  container State; network cleanup on failed create; activity map cleanup.
- API: WS Origin verification ON (was InsecureSkipVerify); cross-origin
  mutating requests rejected + 1 MB body cap; failed creation rolls back the
  registration; wipe also removes the vfs fallback dir and logs failures.
- Store: entries validated on load (invariant holds even for hand-edited
  files); fsync before rename (power-cut-safe projects.json).
- host-setup.sh: docker restarted only when daemon.json actually changed
  (re-run no longer kills running microVMs); backup copied not moved; atomic
  config write. home-init: shared home chmod 700.

## Deferred to TODO (from review)

systemd unit for the server (Phase 3 scope), Kata tarball checksum pinning,
keyring password visible in container env (docker inspect), conversation-id
allocation race (documented as best-effort), archived flag unused.

## Operator feedback (first live use)

- "Agents aren't logged in — need a prestep per user for git/claude/agy":
  added `styrman-setup` (guided, idempotent: git identity, SSH key, gh,
  claude, agy) baked into the image + a login-shell MOTD hint. UI onboarding
  banner is Phase 2.
- "Can't delete conversations": ✕ button added to the dev console (API existed).
- "Missing the Antigravity-style sidebars": that is Phase 2 (the real UI);
  the dev console is a throwaway.
- "Conversation is the wrong concept — I want history per conversation":
  ADR-011 — conversation = durable agent session with native resume; tmux
  demoted to transport; plain shells become "terminals". Phase 2 implements.

## Verification

go test green, go vet clean, bash -n on all scripts, image rebuilt (cached),
integration.sh 18/18, demo projects recreated on the new image with
styrman-setup present, server live on 0.0.0.0:8080.
