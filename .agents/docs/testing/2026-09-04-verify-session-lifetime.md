---
owner: verifier
date: 2026-09-04
---
# Verification — session-lifetime (ADR-063)   (2026-09-04)

Range reviewed: working tree vs HEAD (24490c9) —
`deploy/caddy/ingress/generator.go`, `deploy/caddy/ingress/generator_test.go`,
`.agents/docs/architecture/DECISIONS.md`, `.agents/docs/specs/styrman/deploy.md`,
`TODO.md`

## Status: PASS

Scope: complete — all 4 requested checks ran (unit tests, mutation-kill
proof, real Caddyfile validated against vendored `caddy validate`, root
build/vet baseline).

## Commands

1. `(cd deploy/caddy/ingress && gofmt -l . && go vet ./... && go test ./... -count=1 -v -run SessionLifetime)`
   → `gofmt -l .` printed nothing; `go vet ./...` clean;
   `--- PASS: TestGenerateCaddyfile_SessionLifetimeIsExplicit (0.00s)` / `PASS` / `ok`.
   Then `go test ./...` (whole module) → `ok github.com/spgo/devbox/caddy/ingress 2.029s`.

2. Mutation-kill proof: `git stash push -- deploy/caddy/ingress/generator.go`,
   then `go test ./... -run SessionLifetime`:
   ```
   ./generator_test.go:389:5: undefined: SessionLifetimeSeconds
   ./generator_test.go:390:71: undefined: SessionLifetimeSeconds
   FAIL	github.com/spgo/devbox/caddy/ingress [build failed]
   ```
   Counts as the test biting (removing the constant fails the build, which
   fails the test run). `git stash pop` restored it; confirmed with
   `git diff --stat -- deploy/caddy/ingress/generator.go` →
   `1 file changed, 11 insertions(+)`.

3. Rendered a real Caddyfile via a module-external scratch program
   (`.agents/scratch/session-lifetime-verify/`, `testConfig()` shape plus one
   `ModeAuthenticated` app, deleted after the run) and validated it against
   `styrman-caddy:1.19.1` (image present locally):
   ```
   docker run --rm -v <Caddyfile>:/etc/caddy/Caddyfile:ro \
     -e GOOGLE_CLIENT_ID=x -e GOOGLE_CLIENT_SECRET=y \
     -e STYRMAN_JWT_KEY=<64x'a'> -e CLOUDFLARE_API_TOKEN=<40x'a'> \
     styrman-caddy:1.19.1 caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
   ```
   First attempt with `CLOUDFLARE_API_TOKEN=z` failed on Cloudflare's own
   token-format check (unrelated to this change: "API token 'z' appears
   invalid"). With a 40-char dummy token the security app provisioned fully
   — Google OAuth identity provider configured, `crypto default token
   lifetime 28800` inside the `authentication portal myportal` block
   accepted with no complaint — and Caddy printed `Valid configuration`.
   No plugin objection to the new directive or its value.

4. Root repo baseline: `go build ./...` → exit 0, no output.
   `go vet ./...` → exit 0, no output.

## Coverage of changed code

- `deploy/caddy/ingress/generator.go` (`SessionLifetimeSeconds`,
  `writeSecurityBlock` emission) — covered by
  `TestGenerateCaddyfile_SessionLifetimeIsExplicit` in
  `deploy/caddy/ingress/generator_test.go`, confirmed to fail without the
  change (step 2), and independently confirmed against a real `caddy
  validate` run (step 3).
- `.agents/docs/architecture/DECISIONS.md` (ADR-063), `.agents/docs/specs/styrman/deploy.md`,
  `TODO.md` — docs-only, read for consistency; content matches the code
  change (28800s / eight hours, `SessionLifetimeSeconds`, portal block).

## Environment

Linux 6.12.107+deb13-amd64. `go version` per module toolchain directive
(module requires go >= 1.25.1; scratch build auto-selected go1.26.8 via
toolchain download). Docker Engine Community 29.7.2 (plain Docker, no
Colima — Linux box). `styrman-caddy:1.19.1` image present locally
(image id `cbff0a6898a2`).
