---
owner: explorer
date: 2026-09-22
---
# Recon — project   (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff.

## Status: COMPLETE

## Question
Inventory and structure map of the repository: uniformity, orphaned and dead-code candidates, across all four build units — root Go module, `deploy/caddy/ingress` Go module, `web/` SPA, `image/`+`deploy/` shell tooling.

## Summary
Four build units confirmed: root Go module (`cmd/styrman`, `internal/*`, 14 packages, ~19k Go LOC incl. tests), the separate `deploy/caddy/ingress` module (vendored/forked Caddy, `module github.com/spgo/devbox/caddy/ingress` — name still says the old repo, see Risks), `web/` (TanStack Start SPA, ~13.4k TS/TSX LOC across `components/`, `routes/`, `lib/`), and `image/`+`deploy/` (shell tooling: image build, host setup, firewall, GCP, updater). No orphaned Go package found so far — every `internal/*` package has at least one importer outside itself. `internal/netpolicy` (1,049 lines) and `image/harness/` were confirmed physically deleted, matching ADR-067 and ADR-074's supersession claims — no dead code left behind for either. `internal/httpapi` (12,669 lines incl. tests, 33 files) and `internal/orchestrator` (3,734 lines, 15 files) are large single-package concentrations; `server.go` (1,405 lines) and `git.go` (1,001 lines) in httpapi, `orchestrator.go` (1,704 lines) are the size outliers worth a closer pass by a later lens.

## Relevant files
- `go.mod` (root) — module `github.com/kesokaj/styrman-crew`, go1.25.0, Docker SDK + websocket + x/sys.
- `deploy/caddy/ingress/go.mod` — separate module, `github.com/spgo/devbox/caddy/ingress` (name mismatch — see Risks), pulls in full Caddy v2 + zap/quic-go/prometheus dependency tree.
- `internal/httpapi/` (12,669 total incl. tests, 33 files) — REST/WS routing; `server.go` 1,405, `git.go` 1,001, `explorer.go` 690, `browser.go` 615, `network_test.go` 793 (test file bigger than several source files — flagged, not concerning on its own).
- `internal/orchestrator/` (3,734 total, 15 files) — `orchestrator.go` 1,704 lines is by far the largest single Go source file in the tree.
- `internal/config/`, `internal/conversation/`, `internal/project/`, `internal/terminal/` — each split into multiple files unlike some single-file packages (`credbroker/broker.go`, `credstore/credstore.go`, `files/files.go`, `identity/identity.go`, `links/store.go`, `userstore/userstore.go`, `gitrepo/gitrepo.go`, `history/history.go` — one file each). The one-file-per-package convention is the norm; multi-file packages are the ones doing more (httpapi, orchestrator, conversation, config, terminal, project).
- `cmd/wsprobe/main.go` — small WS test probe; only consumer is `.agents/docs/testing/styrman/integration.sh` (built and invoked there, not from any Go code or the main binary). Not dead, but it's test tooling living under `cmd/` rather than under the test script's own dir or `internal/` — a naming/location convention break worth a reviewer glance, low confidence this is wrong (could be deliberate so `go build ./...` covers it).
- `web/src/lib/` — 31 files, notably paired `X.ts` + `X.test.ts` for most modules (`clips`, `lan`, `sshKey`, `cdp`, `diffReveal`, `browserInput`, `projectBadge`, `explorerHistory`, `recentsPanel`, `appearance`, `editorPane`) plus `api.ts` split into three test files (`api.test.ts`, `api.timeout.test.ts`, `api.session.test.ts`, `api.encoding.test.ts` — four test files for one source file, uniform-ish naming but worth confirming none is a stale duplicate).
- `web/src/components/` — 26 components, flat directory, PascalCase filenames — uniform. No subdirectories despite the count; not obviously wrong given TanStack Start conventions but a size flag for a future lens. Checked: a name-grep for each of the 26 component names against the rest of `web/src` found at least one reference outside its own file for every one — no zero-reference component (grep-based, not a real import-graph tool; a component referenced only in a comment or a similarly-named sibling would false-positive as "used").
- `image/configs/` — mix of `.sh`-suffixed (`harness-migrate.sh`, `profile-motd.sh`, `styrman-credentials.sh`) and no-suffix executables (`styrman-browser`, `styrman-git-credential`, `styrman-setup`) — inconsistent naming convention within one directory; low confidence on whether this is intentional (no-suffix ones may be deliberately made to look like normal binaries on PATH).
- `deploy/` — `host-setup.sh`, `styrman-ctl`, `verify-commit.sh`, `vulncheck.sh` at top level (mixed with/without `.sh`), plus `firewall/`, `gcp/`, `update/`, `caddy/` subdirs each holding a coherent cluster (install/lib/service/timer pattern repeats in `firewall/` and `update/` — consistent sibling convention).

## Flows
- Root module → `cmd/styrman/main.go` wires `internal/gitrepo`, `internal/credbroker`, `internal/credstore`, `internal/userstore`, then hands off to `internal/httpapi` (routing) which calls into `internal/orchestrator` (Kata/Docker lifecycle), `internal/terminal`, `internal/files`, `internal/conversation`, `internal/project`, `internal/links`, `internal/history`.
- `deploy/caddy/ingress` is fully separate (own go.mod/go.sum), built/tested independently (`(cd deploy/caddy/ingress && go test ./...)` per AGENTS.md) and consumed only as a container image (`deploy/caddy/Dockerfile`) — no Go import edge from the root module into it, or vice versa; connection is at the deploy layer only (`deploy/docker-compose.yml`, header contract `X-Styrman-Ingress`/`X-Caddy-Email`).
- `image/` is consumed by `deploy/Dockerfile.server`... actually image/Dockerfile builds the runtime microVM image separately, independent of deploy/Dockerfile.server (server image) — two different Dockerfiles for two different artifacts (server container vs. per-project runtime image). Not yet fully traced — see Open questions.
- `web/` builds to `web/dist/client`, embedded into the Go binary via `web/embed.go` + `go:embed`, per AGENTS.md/README — the SPA and Go server are one deployable artifact at runtime despite being separate toolchains at build time.

## Conventions
- One-file-per-package is the norm for small/focused `internal/*` packages; multi-file packages correlate with genuinely larger surface area (httpapi, orchestrator) rather than arbitrary splitting.
- Go test files sit beside their source (`foo.go` + `foo_test.go`), consistent throughout `internal/` and `cmd/`.
- `web/src/lib/*.ts` pairs 1:1 with `*.test.ts` (with `api.ts` as the one many-to-one exception, four test files).
- Shell scripts in `deploy/` mix `.sh`-suffixed and bare-named files; `image/configs/` does the same — plausibly deliberate (bare names = things installed onto PATH inside the guest) but not verified against a written rule.
- ADR-driven supersession is real and cleanly executed in this repo: both `internal/netpolicy` and `image/harness/` were fully removed at the commits their ADRs (ADR-067, ADR-074) claim, not just marked deprecated in comments.

## Risks
- `deploy/caddy/ingress/go.mod`'s module path `github.com/spgo/devbox/caddy/ingress` does not match the current repo (`kesokaj/styrman-crew`) or its own directory — confirmed from the old `devbox` lineage AGENTS.md describes as this project's predecessor. Low risk functionally (it's a vendored fork, not published), but a naming-uniformity finding worth flagging explicitly since AGENTS.md calls this file "vendored Caddy ingress" and `VENDORED.md` says it's "now a fork, not a mirror" — the module path never got updated to match. Confidence: high on the fact, low on whether it's worth fixing (may be intentional to ease upstream diffing).
- `internal/httpapi/server.go` (1,405 lines) and `internal/orchestrator/orchestrator.go` (1,704 lines) are large single files carrying central logic (routing table, VM lifecycle) — classic hot-file risk for merge conflicts and for a change touching many unrelated concerns at once. Not inspected for internal cohesion in this pass.
- `cmd/wsprobe` living under `cmd/` (peer of the actual product binary `cmd/styrman`) while its only consumer is a Tier-2 shell test script is a structural oddity: a reader scanning `cmd/` for "the binaries this repo ships" gets one real product and one test helper with no docstring marking the difference. Did not check for a package comment stating its purpose.

## Open questions
- Whether `cmd/wsprobe` has any other consumer (e.g. `web/e2e/`, `deploy/`) — only checked `.go`, `.sh`, `Makefile` files via grep; did not check `.mjs`/`.ts` e2e harness or CI-equivalent scripts.
- No exhaustive exported-identifier-with-no-caller sweep was run (would need `go vet`/`staticcheck`/`deadcode` per package or a scripted `grep` per exported symbol — not attempted given the 35-call budget; spot checks on 6 packages (`gitrepo`, `identity`, `credbroker`, `credstore`, `links`, `userstore`) all showed live external callers, but this is a package-level check, not a symbol-level one — an unused exported function *inside* a package with other live exports would not surface this way).
- `image/Dockerfile` vs `deploy/Dockerfile.server` relationship (server image vs. per-project runtime image) was inferred from names/AGENTS.md, not traced through the actual compose/build files.
- `web/src/components/` (26 files) and `web/src/routes/` (5 files) were listed but not opened — no check yet for components with zero import sites (would need a grep per component name across `.tsx`/`.ts`).
- `.agents/docs/architecture/DECISIONS.md` was scanned only for `superseded`/ADR-header lines, not read in full; other ADRs may mark additional dead-code candidates not surfaced by this grep (e.g. an ADR that changes a mechanism without using the word "superseded").
- `cmd/wsprobe` confirmed to have no consumer under `web/` (grep for "wsprobe" across `web/` returned nothing) — its only caller remains `.agents/docs/testing/styrman/integration.sh`, strengthening (not resolving) the structural-oddity flag in Risks.
- `image/Dockerfile` (builds `styrman-runtime`, the per-project microVM image, ADR-001/002/074) and `deploy/Dockerfile.server` (builds the server binary + embedded UI container) confirmed to be two genuinely separate artifacts with no build-time overlap — not a duplication, just two Dockerfiles for two deployables, matching AGENTS.md's description.

## Not examined
- Full read of any individual large file (`server.go`, `git.go`, `orchestrator.go`) — only sizes and importers checked.
- `web/src/routes/` and `web/src/components/` internal cross-reference graph (which component/route imports which).
- `deploy/gcp/`, `deploy/firewall/`, `deploy/update/` script contents — only listed, not read, so no dead-function-inside-shell-script check was done.
- `image/configs/*` script contents.
- Symbol-level unused-export sweep (see Open questions).
- `.agents/docs/architecture/DECISIONS.md` body text beyond ADR headers and "supersed*" grep hits.
