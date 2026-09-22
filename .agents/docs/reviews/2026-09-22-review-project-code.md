---
owner: reviewer
date: 2026-09-22
---
# Review — project-code   (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff.

## Status: COMPLETE

## Verdict: APPROVE WITH NITS

The worst problem found is in the release gate, not the product: `deploy/verify-commit.sh`'s
"web/dist freshness" step compares only the SET OF FILE NAMES produced by a rebuild, never
file CONTENT, so a change to the two fixed-name embedded assets (`_shell.html`, `favicon.svg`)
can ship stale inside the binary with the gate printing "committed assets match a fresh build".
Everything else I reached is sound: both Go modules build, vet and test clean; the frontend
typechecks and lints clean; `shellcheck -S warning` is clean on the three shell scripts the
gate omits; the trust boundaries that the bug record says have bitten before (ingress-token
gate, link/shared token routes, credential-broker IP identification, `realBelow`/
`projectHomeIsReal` on both the create and start bind paths, root-owned credential files at
0600) are all in the state their records describe, with the reasoning written down at each
site. No hand-rolled crypto, auth, session or token format exists anywhere in either module.

## Findings

- **[medium / confidence: high]** `deploy/verify-commit.sh:135-144` — the embedded-artifact
  freshness gate asserts NAME-SET equality, not content. It captures
  `(cd web/dist && find client -type f | sort)` before and after `npm run build` and compares
  the two strings. Vite content-hashes everything under `assets/`, so a change there does move
  a name — but `web/dist/client/_shell.html` and `web/dist/client/favicon.svg` have FIXED names
  (`git ls-files web/dist/client | grep -v assets/`). Concrete failure: edit `web/index.html`
  (title, a meta/CSP tag, a preload hint) or `favicon.svg` and commit without rebuilding —
  the asset-name set is unchanged, the gate prints "committed assets match a fresh build", and
  `web/embed.go`'s `//go:embed all:dist/client` ships the OLD shell to every user. This is the
  `review-standards` §2 rule "a test that a generated or embedded artifact matches its source
  asserts BOTH directions" (BUG-066 family). The fix is cheap and the machinery already
  exists: `web/scripts/normalize-shell.mjs` was written specifically to make `_shell.html`
  byte-reproducible, so replace the `find` comparison with a content digest —
  `find client -type f -print0 | sort -z | xargs -0 sha256sum` — on both sides.

- **[low / confidence: high]** `deploy/verify-commit.sh:170` — the gate's `shellcheck -S warning`
  covers `deploy/firewall/*.sh deploy/update/styrman-update deploy/gcp/*.sh` but NOT
  `deploy/host-setup.sh`, `deploy/styrman-ctl` or `image/entrypoint.sh`, although line 168's
  `bash -n` does cover `host-setup.sh`. BUG-068's record names `host-setup.sh` and
  `styrman-ctl` as affected files, and `.agents/rules/shared-path-learnings.md` makes
  `host-setup.sh` the canonical shared path that a second target exercises. I ran
  `shellcheck -S warning deploy/host-setup.sh deploy/styrman-ctl image/entrypoint.sh` at HEAD:
  clean (rc=0), so nothing is broken today — the finding is that the gate would not catch a
  regression in the one script that must work on a fresh host. Add the three paths to line 170
  (and `image/entrypoint.sh` to line 168's `bash -n`).

- **[low / confidence: high]** `go.mod:7-8,28` — `go mod tidy` moves
  `github.com/opencontainers/image-spec v1.1.1` out of the indirect block: it is a DIRECT
  import (`internal/orchestrator/docker.go:12`, `internal/orchestrator/fakedocker_test.go:19`).
  `golang.org/x/sys` is likewise a direct import (`internal/files/files.go:29`) and is already
  correctly listed as direct. `REQUIREMENTS.md` → Constraints names the approved dependency set
  as `coder/websocket`, `creack/pty`, `docker/docker` and requires every addition to be
  justified; neither `opencontainers/image-spec` nor `golang.org/x/sys` appears there. Both are
  defensible (ocispec types appear in the Docker SDK's own signatures; `x/sys/unix` supplies
  syscalls the stdlib does not expose) — the deviation is that the requirement's list was never
  updated to say so. Either run `go mod tidy` and record both in `REQUIREMENTS.md` (operator
  approval, per the immutability rule), or note them in the changelog. `creack/pty` is in the
  approved list but is not a dependency at all — the approved list is a ceiling, so that is not
  a defect.

Not findings, recorded so they are not re-discovered:
- **BUG-074** (ingress `AUTHP_ACCESS_TOKEN` cookie name shared across deployments) is open in
  `.agents/docs/bugs/INDEX.md` and `deploy/caddy/ingress/config.go:48` still hard-codes the
  single name. Commit e10df4c records the operator's decision to accept the constraint and not
  fix it unasked, so it is not raised here as a finding.
- `internal/httpapi/git.go`, `check.go`, `format.go`, `stats.go`, `conversation.go`,
  `orchestrator/ports.go` all build `sh -c` / `bash -c` strings, but every one of them executes
  INSIDE the caller's own microVM as the caller's own guest user, with user content delivered
  over stdin to a temp file rather than interpolated (`check.go:373-388`). No privilege boundary
  is crossed, so this is not an injection finding.

## Criteria

1. **Dead code — pass.** `go vet ./...` clean in both modules; no commented-out blocks or debug
   prints found in the files read; `web/dist/server` is gitignored and deliberately excluded
   from the embed. `creack/pty` is an approved-but-unused dependency name in `REQUIREMENTS.md`,
   not code. I did not run a full unused-export analysis across all 29k lines of Go (see Not
   examined).
2. **Flow tracing — pass, with the one gate gap above.** The two paths the bug record says have
   bitten repeatedly are consistent end to end: the symlink guard is applied on BOTH the create
   and the start bind paths (`internal/orchestrator/orchestrator.go:344` `projectHomeIsReal`,
   `:411` and `:758` `realBelow`), closing BUG-003; the ingress-token check fires on both the
   link-path branch and the header-auth branch of `internal/httpapi/auth.go:44,59` with an
   explicit fail-closed `else` for an unknown auth mode; `serveLink`/`serveShared` each refuse
   the other kind's token and both re-check `previewPortAllowed` after the store lookup. Error
   paths propagate with context; the broker distinguishes "not onboarded" from a read fault and
   logs the latter (`internal/credbroker/broker.go:88-95`).
3. **Standards conformance — pass.** Go: `go vet` clean both modules, `go test ./...` rc=0
   (root) and rc=0 (`deploy/caddy/ingress`), run bare with their status read, per
   `.agents/rules/verify-bare.md`. TypeScript: `npm run check` (tsc -b) rc=0 and `npx oxlint`
   rc=0. Shell: `shellcheck -S warning` clean on the gate's own list and on the three scripts
   it omits. Constant-time comparison is used for the ingress token; `crypto/rand` for link
   tokens (`internal/links/store.go:98`) and UUIDs (`internal/project/uuid.go:14`);
   `math/rand/v2` only for cosmetic terminal names (`internal/conversation/names.go`), which is
   correct. Doc-comments at the audited sites still describe the behaviour they guard.
4. **Directory layout — pass.** `cmd/styrman/` entrypoint, private code under `internal/`, no
   `pkg/`, tests beside their code, the SPA fully contained in `web/`, the ingress isolated as
   its own Go module under `deploy/caddy/ingress/`. One observation, not a finding: that
   module's path is still `github.com/spgo/devbox/caddy/ingress`, which is the devbox module
   path rather than a styrman one — it is vendored and never imported by the root module, so
   nothing breaks.
5. **Simplicity (DIY-except-security) — pass.** No hand-rolled primitive or protocol anywhere:
   no custom cipher, MAC, KDF, token format or session format; OIDC lives entirely in
   caddy-security at the ingress, which is the correct place for it. The link capability token
   is 128 bits straight from `crypto/rand` — stdlib use, not DIY. The dependency set is small
   and stdlib-first; see the `go.mod` finding above for the bookkeeping gap.
6. **Decoupling — mostly pass, one structural observation.** Seams are clean: `credbroker`
   consumes a one-method `Resolver` interface declared at the consumer, `files.Trust` confines
   every tenant root in one place, the orchestrator is reached through a narrow `guest.Exec`
   seam. Two god-files force serialized edits and would collide if two engineers worked adjacent
   features: `internal/orchestrator/orchestrator.go` (1704 lines) and
   `internal/httpapi/server.go` (1405 lines, holding routing, the CSRF guard, the body-size
   table and per-user space management). No concrete failure follows from either, so neither is
   a finding under this dispatch's bar — but they are where the next merge conflict will be.

## Not examined

This was a whole-repository review under a 35-call budget; I read a subset and name it honestly.

- **Read in full or in substance:** `REQUIREMENTS.md`, `AGENTS.md`, all eight `.agents/rules/`
  files, `.agents/docs/bugs/INDEX.md` (BUG-001..BUG-074 + the Graduated table),
  `internal/httpapi/auth.go`, `internal/credbroker/broker.go`,
  `internal/httpapi/links_api.go:140-275`, `internal/httpapi/proxy.go:1-120`,
  `internal/httpapi/server.go` (csrfGuard, who, getProjectFor, the embed/serve path),
  `internal/httpapi/check.go:330-400`, `internal/conversation/conversation.go:60-110`,
  `internal/config/config.go:330-380`, `deploy/caddy/ingress/config.go`,
  `deploy/verify-commit.sh:130-215`, `web/embed.go`, `go.mod`.
- **Not read:** `internal/orchestrator/orchestrator.go` beyond the guard/bind sites (grep only),
  `internal/httpapi/git.go` beyond its exec construction (grep only), `internal/files/files.go`,
  `internal/history/`, `internal/explorer.go`, `internal/terminal/bridge.go`,
  `internal/credstore/credstore.go` beyond its permission handling,
  `internal/userstore/`, `internal/identity/`, `internal/gitrepo/`,
  `deploy/caddy/ingress/{generator,watcher,cloudflare,labels,loopback,module,cmd}.go`
  (generator inspected by grep for header/cookie handling only), the entire `web/src/` tree,
  `image/` (Dockerfile, entrypoint.sh, configs — shellchecked but not read),
  `deploy/firewall/`, `deploy/update/styrman-update`, `deploy/gcp/`, `deploy/host-setup.sh`,
  and all of `.agents/docs/` beyond the bug index.
- **Not run:** `web` vitest (`npm test`), Tier 2 `integration.sh`, `web/e2e/run.sh`,
  `deploy/vulncheck.sh` (govulncheck — a dependency-CVE finding is therefore OUT of this
  report's evidence), `deploy/verify-commit.sh` itself, and any image build. The
  `go mod tidy` check was run against copies and both `go.mod`/`go.sum` were restored
  byte-identically (`git status --porcelain go.mod go.sum` empty afterwards); no source,
  test or config file was modified by this review.
