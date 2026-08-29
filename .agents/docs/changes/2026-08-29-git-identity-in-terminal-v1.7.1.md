# Git identity propagates to project terminals (v1.7.1)

Date: 2026-08-29. Closes the gap noted in v1.7.0: `git commit` in a project
TERMINAL failed with no `user.name`/`user.email`. The broker synced the git
auth token but not the identity, and per-project homes (ADR-031) start without
it. The git UI already worked (it injects the identity host-side via `-c`); this
makes the terminal work too.

## Fix

- Harvest (`internal/httpapi/credentials.go`): `harvestCredentials` now also
  stores the user's git identity from their shared-home `.gitconfig`
  (`s.gitIdentity`) as env creds `STYRMAN_GIT_NAME` / `STYRMAN_GIT_EMAIL`.
- Pull-on-login (`image/configs/styrman-credentials.sh`): applies them as
  `git config --global user.name/user.email` on each login shell. Set as
  config (not `GIT_AUTHOR_*` env), so a per-repo `user.name` still overrides it.

## Verification (live, styrman-server 1.7.1 + rebuilt styrman-runtime:dev)

Re-ran the setup harvest → broker gained `env-STYRMAN_GIT_NAME/EMAIL`. Restarted
the "test" project (recreated on the new image) → after a login shell,
`git config --global user.name/email` returned `kesokaj` /
`ostling.simon@gmail.com`, and a fresh `git commit` in the terminal (isolated
repo, no per-repo identity) authored as `kesokaj <ostling.simon@gmail.com>`.

## Release

Built + deployed live as styrman-server 1.7.1; styrman-runtime:dev rebuilt.
