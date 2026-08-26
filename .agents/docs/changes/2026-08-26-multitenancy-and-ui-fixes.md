# 2026-08-26 — Multi-tenancy + the 7-item operator batch

## Summary

Operator batch, all delivered. Verified: unit green, integration **45/45**,
browser e2e **28/28**; production migrated (home → homes/ostling-simon) with
zero data loss.

1. **Typed delete dialog**: deleting a project (= its microVM) now uses an
   in-app dialog requiring the word "delete", with an explicit wipe-files
   checkbox — no browser popups.
2. **Ports show what's behind them**: the in-guest scan now maps socket →
   process (via /proc fd walk) and, for ports published by the project's
   inner docker, the container image (🐳 image tag in the panel).
3. **File explorer controls**: new-file button, upload button, and
   drag-and-drop upload (multipart, binary-safe, 512 MB cap); download
   existed.
4. **Per-user homes (ADR-019, R3 amended)**: every account gets its own
   home disk, projects, terminals, transcripts, and setup VM. Container and
   network names are user-scoped; a startup migration moved the legacy
   layout and removed pre-multitenancy containers.
5. **Admin section**: env-provided accounts are admins; only they see and
   use Settings → Admin (add/revoke users, server-enforced).
6. **Per-tool settings cards**: git (identity), ssh (public key), github
   (accounts), claude (credentials + skills/subagents/commands listed from
   ~/.claude), agy (keyring + ~/.gemini/config skills/agents), each with its
   config dir and a ready dot.
7. **Terminal lifecycle**: pause (kill session, keep record + transcript;
   "paused" state in sidebar), per-terminal delete, and close-all.

## Failures / gotchas

- The formatter (prettier hook) reformatted TS files between my read and
  write several times, silently voiding python-based patches — caught each
  time by grep/build, fixed by editing against on-disk content. Lesson
  recorded: for TS files, Edit against fresh reads beats scripted rewrites.
- First integration run after the change failed its first 4 checks: the
  test server's migration sweep was busy deleting the production legacy
  containers (a one-time global side effect that the prod migration would
  have done anyway) and missed the health-check window.
- First e2e after prod migration hit a cold-start stampede (three microVMs
  racing after the legacy sweep); steady-state creates are ~2.7 s.
- e2e assertions caught two stale selectors (renamed settings row; new
  delete dialog) — updated, plus a new check that the delete button stays
  disarmed until "delete" is typed.

## Verification

integration.sh grew to 45 checks (adds: pause keeps record/loses session,
close-all, migration of legacy home/stores, user-scoped container names,
per-user metadata store). e2e 28 checks (adds typed-delete dialog flow).
