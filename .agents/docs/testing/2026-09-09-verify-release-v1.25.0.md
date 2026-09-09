---
owner: verifier
date: 2026-09-09
---
# Verification — release gate v1.25.0   (2026-09-09)

Range reviewed: v1.24.1..eccd1a0

> **PARTIAL — SUPERSEDED. No verdict.** The verifier stopped at its turn limit
> after tiers 1-3 and before web/e2e, integration, firewall and the coverage
> trace. Its range (`v1.24.1..eccd1a0`) is also stale: the release candidate
> moved after the preflight reviews found BUG-029/030/031, and the tree that
> ships is verified in `2026-09-09-verify-release-v1.25.0-final.md`. What IS
> established here: `gofmt`/build/vet clean and `go test ./...` 15 packages,
> 196 subtests, 0 failures, on `eccd1a0`. Nothing else in this file counts.


## Status: IN PROGRESS

Scope: running release-gate tiers in order; this file is being updated as each tier completes.

## Commands

1. `go build ./...` → exit 0, clean.
2. `go vet ./...` → exit 0, clean.
3. `go test ./...` → PASS. 15 packages ok (cmd/styrman, internal/config, internal/conversation, internal/credbroker, internal/credstore, internal/files, internal/gitrepo, internal/history, internal/httpapi, internal/identity, internal/links, internal/orchestrator, internal/project, internal/terminal, internal/userstore) + `web` and `cmd/wsprobe` report `[no test files]` (not counted). 196 `--- PASS` subtests (0 `--- FAIL`), matches expected 196 (194 baseline + 2 new in internal/httpapi).
4. `(cd deploy/caddy/ingress && go test ./...)` → `ok github.com/spgo/devbox/caddy/ingress` exit 0.

(remaining tiers pending)

## Coverage of changed code
(NOT DONE — see the superseding log)

## Environment
OS: Linux 6.12.107+deb13-amd64. Go: TBD. Node: TBD.
