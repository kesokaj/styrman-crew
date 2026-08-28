# Credential-service P1, Increment 3 — per-project homes (clean cut)

Date: 2026-08-28. ADR-031. The "truly individual projects" change: each
project gets its own home; only credentials are shared (via the broker,
ADR-030). Clean cut, no migration — operator re-registers creds (prototype).

## What changed

- **Per-project home**: `cfg.ProjectHome(user, project)` =
  `HomesDir/<user>/p/<uuid>`, mounted as `$HOME` in the microVM
  (orchestrator; created at project creation + boot). The per-user
  `HomesDir/<user>` is now just the setup VM's home + the parent.
- **Image seeds the home**: dotfiles moved into the image
  (`image/skel/`, same set as the old `internal/dotfiles`, now removed) and
  the entrypoint seeds them + the standard dirs write-once, alongside the
  harness (Inc 1). A per-project home starts empty and the base image makes
  it a usable, identical shell — no server-side home creation.
- **Server**: dropped `ensureHome` + the `dotfiles` package; `space()` no
  longer touches the home (only the user's metadata dir). `createProject`
  creates the empty per-project home dir (so Files "Home" works pre-boot);
  the orchestrator creates it before the mount; `EnsureSetup` creates the
  base home for the setup VM.
- **Files "Home" scope is per-project**: `/api/home/*` → `/api/projects/
  {project}/home/*`; `scopeFor` roots at `ProjectHome` when the matched
  route pattern contains `/home/` (Go 1.25 `r.Pattern`), else the workspace.
  The managed-path guard keys on the route pattern. UI `Scope` carries the
  project for the home tab.
- **Transcripts** are per-project (`ProjectHome/.styrman/logs/<project>`);
  history List/Search/Read read from `ProjectHome`.
- **Wipe** removes the whole per-project home + workspace (one RemoveAll
  each — docker disk, transcripts, apt, dotfiles, any creds all live in the
  per-project home now).

## Proven live (prod)

Recreated a project (fresh home) + created a second: both homes at
`homes/<user>/p/<uuid>`, each seeded with dotfiles (git-branch prompt) +
harness from the image; a marker written in project A's home was ABSENT in
project B (isolation); the workspace code survived the home swap (ADR-027
separate); the git broker env stayed injected. Truly individual projects.

## Consequences / remaining

- Re-auth per tool until the broker adapters land (git done; ssh, gcloud,
  docker, gh, npm, claude, agy pending) — the operator accepted this
  (prototype).
- No cache sharing between projects (accepted — that was the point).
- Revises R3 again (per-project home).

## Verification

go test -race clean (tests updated for per-project home routes/paths);
e2e; multi-project isolation + fresh-home seeding + git-broker proven live.
