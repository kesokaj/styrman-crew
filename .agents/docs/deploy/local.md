---
owner: documentation
date: 2026-09-03
target: local
---
# Deploy — local

How to build and run styrman on the machine in front of you. Filled at the harness migration (2026-09-03) from `AGENTS.md` Setup Commands; the `documentation` agent keeps it current whenever the build, run, or configuration path changes.

`deploy/styrman-ctl` is for the production host, not this loopback dev run — `STYRMAN_IMAGE=… go run ./cmd/styrman/` below stays the local path.

## Prerequisites

Go (version pinned in `go.mod`), Docker with the Kata `mvm` runtime for real project microVMs (`deploy/host-setup.sh` converges a Debian 13 host; loopback dev mode runs without auth), Node for `web/`. Full dev-mode walkthrough: `.agents/docs/specs/styrman/deploy.md` "Dev mode".

## Build

```bash
go build ./cmd/styrman/
(cd deploy/caddy/ingress && go build ./...)     # separate module, security-critical
GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN \
  -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/   # runtime image (AMD64, local only); the version keys harness re-seeding
```

## Configure

| Variable | Default | Purpose |
|---|---|---|
| `STYRMAN_IMAGE` | — | Runtime image the server boots per project (`styrman-runtime:dev` locally) |
| `STYRMAN_VERSION` | required by compose | Release version; compose refuses to run without it (no mutable tags, ADR-053) |
| `deploy/.env` | `.env.example` | Secrets for production only: OAuth client, Cloudflare token, ingress token, JWT key — never committed |

## Run

```bash
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/     # loopback, no auth
```

## Verify

```bash
go vet ./... && go test ./... && (cd deploy/caddy/ingress && go test ./...)
bash .agents/docs/testing/styrman/integration.sh              # against real Kata; check counts: .agents/docs/testing/styrman/README.md
bash web/e2e/run.sh                                           # Playwright e2e; counts in .agents/docs/testing/styrman/README.md
```

## Container

The runtime image is built above; the server container is `deploy/Dockerfile.server`, exercised through compose in `remote.md`.

## Stop and clean up

Stop the server process; project containers are `styrman-<uuid>` (`docker ps`), per-project dockerd images live under the project home (`~/.dockerd/<uuid>.img`). `integration.sh` cleans its own `styrman-itest-*` state.
