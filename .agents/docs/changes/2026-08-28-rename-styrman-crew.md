# 2026-08-28 — Rename project to styrman-crew; first push to private remote

## Summary

The project (repo) is now **styrman-crew** — the platform itself keeps the
name *styrman*. Chosen because `kesokaj/styrman` on GitHub is already taken
by an unrelated project (headscale network dashboard), and because the bare
name didn't say what the platform is; "crew" = the agents working under the
helmsman.

Deliberately unchanged: binary name (`styrman`), `STYRMAN_*` env vars,
container names (`styrman-server`, `styrman-caddy`), and the `styrman.xyz`
domain — no functional change, no redeploy needed.

## Files changed

- `go.mod` — module path `github.com/kesokaj/styrman` → `github.com/kesokaj/styrman-crew`
- 18 `.go` files — import paths updated accordingly
- `README.md` — title → styrman-crew; name-origin line mentions the crew
- `AGENTS.md` — project overview names the repo styrman-crew

## Remote

- Created private repo `kesokaj/styrman-crew`; `origin` set to it.
- Pushed `main` plus tags `v1.0.0`, `v1.1.0` (first-ever push of this repo).

## Verification

- `go build ./...`, `go vet ./...` clean.
- `go test -race ./...` — all 8 test packages pass.
