# First-login 400: cached per-user stores didn't survive a scrub-without-restart (v1.5.3)

Date: 2026-08-29. Bug found by the operator's actual first "Start setup" on
the freshly-reinstalled v1.5.2 server (the exact scenario the 2026-08-28
handoff was waiting on).

## Symptom

`POST /api/settings/profile` → HTTP 400:
`open /var/lib/styrman/users/ostling-simon/profile.json.tmp: no such file or
directory`. The username picker had loaded fine (GET-only path); only the
write failed.

## Root cause

`Server.space()` (`internal/httpapi/server.go`) `MkdirAll`s
`users/<user>/` and caches an in-memory `userSpace` (profileStore,
project.Store, conversation.Store) per user for the life of the process.
During the 2026-08-28 clean reinstall, the post-restart verification calls
(`GET /api/projects`, `/api/setup/status`, etc. — see
`2026-08-28-clean-reinstall.md`) went through `space()`, which created
`users/ostling-simon/` on disk and cached its stores. The final "byte-virgin"
scrub then `rm -rf`'d `users/` **without restarting the server**, so the
running process kept serving requests through a cached profileStore whose
path pointed at a now-deleted directory. ~14h later the operator's first
profile write hit that stale path → ENOENT.

Only `credstore.atomicWrite` already self-created its directory before
writing; the three per-user atomic writers (profileStore, project.Store,
conversation.Store) all relied on `space()` having made the dir once at
first cache — a correct assumption only as long as nothing removes that dir
out from under a live process.

## Fix

Added `os.MkdirAll(filepath.Dir(path), 0o755)` immediately before the
temp-file open in each of the three per-user writers, mirroring
`credstore.atomicWrite`, so a cached store recreates its directory instead
of failing:
- `internal/httpapi/profile.go` — `profileStore.writeLocked`
- `internal/project/store.go` — save path
- `internal/conversation/store.go` — save path

Each also `slog.Warn`s when the directory was actually missing (a `Stat`
guards the log so normal saves stay quiet). A silent recreate would
otherwise mask the more serious case where `StateDir` itself is unmounted
under the running server — there the recreate lands on an orphaned tree and
the data is lost at restart; the warning is the breadcrumb for that.

Scope is deliberate: the three root-level cached stores (`links.json`,
`users.json`, `settings.json`) write directly under `StateDir`, the
persistent bind-mount root, so a `rm -rf users/` never orphans them and they
were left unchanged — see ADR-033 for the full reasoning.

Regression tests reproduce the incident faithfully — populate the dir, hold
the live store, `RemoveAll` the dir, then write again and assert recovery:
- `TestSetGuestUserRecreatesMissingDir` (`internal/httpapi/profile_test.go`)
- `TestSaveRecreatesMissingDir` (`internal/project/store_test.go`)
- `TestSaveRecreatesMissingDir` (`internal/conversation/store_test.go`)

## Verification

- `go test -race ./...` — all green.
- Rebuilt the styrman-server image stamped **1.5.3**; redeployed live via
  `docker compose up -d styrman`.
- Reproduced the exact incident against the new binary: GET to warm the
  cached space → `rm -rf users/` → `POST` profile → now **200**, directory
  recreated, `profile.json` persisted correctly.
- Reset the live server back to byte-virgin (scrubbed `users/`, then
  **restarted the server** — see operational lesson below); public path
  reverified `https://crew.styrman.xyz` → 302 to Google auth. Server now
  runs 1.5.3; disk holds only the empty `credentials/`, `homes/`, `projects/`
  roots. Awaiting the operator's re-test of first login.

## Operational lesson

Scrubbing `StateDir/users` (or any dir a cached store depends on) under a
**running** server must be followed by a server restart — that's exactly
the step the 2026-08-28 reinstall's final scrub skipped, and it's what
turned an idle cache into a 14-hour-later 400 for the operator's actual
first login. This fix makes stale-cache writes self-heal going forward, but
the "reset to virgin" procedure should still restart after every scrub
(cheap, and doesn't rely on every code path having this fix). Worth folding
into whatever doc governs the clean-reinstall/reset procedure next time it's
touched (`.agents/docs/changes/2026-08-28-clean-reinstall.md` is the
existing writeup of that procedure).

## Release note

Patch release 1.5.3 (image rebuilt and deployed live; not yet git-tagged or
committed as of this writing — the working tree holds the fix).

## Files changed

- `internal/httpapi/profile.go` — `writeLocked` self-creates its dir.
- `internal/project/store.go` — save path self-creates its dir.
- `internal/conversation/store.go` — save path self-creates its dir.
- `internal/httpapi/profile_test.go` — new `TestSetGuestUserRecreatesMissingDir`.
