# Roadmap clearing marathon: every open item shipped or retired (v1.14.0)

Date: 2026-08-30. Operator: "1-6 just go … I want nothing in todo and
roadmap. This work should be to clean it up so I can start fresh with new
things." One session, ten phases; retirements recorded in ADR-045.

## Phase 0 — gcloud "nothing happened in the projects" (v1.13.2)

Root cause: creds materialize per LOGIN shell; the failing terminals
predated the sign-in. Fix: every harvest/import now execs a no-op login
shell in each RUNNING project — file-tier creds (gcloud/claude/ssh/agy)
land immediately and their tools read files per invocation, so already-open
terminals just start working. Live-verified (mtimes bumped in both VMs).

## Phase 1 — quick wins

Integration suite probes host→bridge reachability and SKIPs the 3
firewall-blocked proxy checks (green suite on the hardened host; skipped
count in the summary). Transcript janitor: `~/.styrman/logs` files older
than STYRMAN_TRANSCRIPT_TTL (720h default) pruned 6-hourly. deploy.md
gained the Clean-reinstall section (restart-after-scrub required, ADR-033).
Vestiges: EnsureSelfAttached's ignored ctx dropped; SetOperator's fsync
moved off the value lock.

## Phase 2 — backups, both layers

Host: bucket `styrman-crew-backups-driven-eft-h4it` (europe-north2
Stockholm, Nearline), bucket-scoped SA, key + restic password root-only in
/etc/styrman (password also in the operator's password manager),
`styrman-host-backup` + systemd timer daily 03:00, retention 7d/4w/6m,
excludes docker disks + StateDir/backups. First snapshot (1.05 GiB) pushed
AND restored back. In-app: doBackup core shared by API/cadence/pre-wipe;
RunBackupCadence (24h default) with keep-newest-5 retention; wipe-delete
takes a FAIL-CLOSED pre-wipe safety backup. Live-verified: cadence pass hit
both projects; a throwaway's pre-wipe backup survived its deletion.

## Phase 3 — ops panel

GET /api/projects/{id}/logs (docker logs tail, demuxed, 256 KiB cap,
owner-scoped). Dashboard cards for running projects: cpu/mem/disk/procs
chips (15s per-card poll) + a Logs dialog with Refresh.

## Phase 4 — auth UX + ingress hygiene

request() detects an expired session (401 or the OAuth-portal redirect an
XHR can't follow) → one toast + page reload. Caddy no longer receives the
whole .env: explicit environment allowlist (live-verified: zero
styrman-only vars in its process env; OAuth redirect intact).

## Phase 5 — poll de-amplification + git badges

lib/projects.ts: ONE shared listProjects poll (4 intervals collapsed;
hidden-tab pause; refreshProjects() fans out). GET /api/git/summary:
branch + dirty/ahead/behind for every running project in one batched call →
sidebar badges (amber when dirty). Auto-titles researched and RETIRED
(ADR-045): claude sessions are per-cwd, not per-terminal.

## Phase 6 — UI polish batch

useModalA11y (topmost-Escape, Tab trap, focus restore) behind the Dialog
shell AND every custom overlay (+role/aria-modal labels: ManageCreds,
CredViewer, NewProject, DeleteProject, file viewer). Keyboard-reachable
primary rows (sidebar project, terminal list, explorer entries).
Poll errors surfaced (Ports/History/Env tabs, bell tooltip).
firstTerminalId() dedupe; SidePanel default follows the 1280px breakpoint
until the user toggles.

## Phase 7 — preview-proxy residuals

EnsureSelfAttached 60s TTL memo (was a Docker round-trip per proxied
request); previewAccess projectID→owner memo (re-validated against the live
record — authz never cached). Proxy path.Cleans the forwarded path and
rewrites absolute Location headers pointing at the guest ip:port back under
the prefix. S10: link/shared exchanges carry a deadline = link expiry
(upgraded WebSockets die with the link). Public-link mint 503s loudly when
STYRMAN_PUBLIC_DOMAIN is unset in header mode. Notifications: concurrent
fan-out + conversation.LiveIn + 5s per-user memo. aliveFromProbe extracted
(+test). S9 verified moot.

## Phase 8 — agent native resume (ADR-011 unparked)

agy verified to ship `--continue`/`--conversation`; bare `claude`/`agy`
autostarts now run `X --continue || X`, so sessions re-created after a
pause/VM restart resume the agent's conversation. The never-read
AgentSession UUID and its helpers removed.

## Phase 9/10 — closure

ADR-045 records every retirement + the standing WON'T DOs; TODO.md is an
empty inbox; README/specs synced; full verification battery (Go suites,
web build, integration incl. new skips, e2e) — results in this file's
Verification section; repo directory renamed to `~/workspace/styrman-crew`
as the final act.

## Verification

- All Go package suites green throughout (incl. new autostart-resume and
  aliveFromProbe cases); tsc/oxlint/web build clean per phase.
- Live checks per phase: credential push (file mtimes), backup cadence +
  pre-wipe survival, /logs and /git/summary responses, caddy env
  minimization, OAuth redirect.
- Integration + e2e final runs: see the summary appended below.

### Final battery results (2026-08-30, post-marathon)

- Go: all 14 package suites green throughout.
- Integration: **57 passed / 0 failed / 3 skipped** (the skips are the
  host-firewall proxy checks, now detected and reported as SKIP — first
  fully green run on the hardened host).
- e2e: **34 passed / 0 failed / 1 skipped** (same firewall skip). Two e2e
  fixes landed during the run: the proxy checks got the firewall probe, and
  the "git tab" selector was stale (predated the v1.7 git-client rewrite —
  the suite hadn't run since; now asserts the shipped surface). The 3
  recoverable hydration warnings are reported as known-benign per ADR-045.
- Live server v1.14.0 healthy; crew.styrman.xyz behind the OAuth gate.
