---
owner: verifier
date: 2026-09-21
---
# Testing guide — styrman-crew

One guide per project (ADR-032). Commands are those in `AGENTS.md` under
"Setup Commands"; check counts for every tiered suite live in
`.agents/docs/testing/styrman/README.md` — the source of truth, not
restated here.

## Tier 1 — unit (every verification run)

```bash
go build ./cmd/styrman/
go vet ./...
go test ./...
(cd deploy/caddy/ingress && go test ./...)   # separate module, not covered by root go test
cd web && npm run check && npm run lint && npm test && cd -
```

## Tier 2+ — integration / e2e / security (see AGENTS.md + README.md)

```bash
bash deploy/vulncheck.sh
bash .agents/docs/testing/styrman/integration.sh   # check counts: styrman/README.md
bash web/e2e/run.sh                                # check counts: styrman/README.md
deploy/verify-commit.sh [commit-ish]               # release gate, detached worktree
```

## Notes

- Run every command bare, on its own; never pipe a verification command
  into a filter inside a chain that leads to a commit (`verify-bare.md`).
- `AGENTS.md` "Setup Commands" is authoritative for exact invocations and
  env vars (`STYRMAN_IMAGE`, `SKIP_WEB_FRESHNESS`, etc).
