# Audit — whole project (2026-09-01)

Scope: HEAD `86e4b9e` (tag `v1.16.0`) plus three uncommitted doc corrections.
Excluded: `image/harness/**` — a verbatim vendored mirror of `devshell_prompt/harness` (ADR-046).

Four scanners ran in parallel. Their full reports:

| Scanner | Verdict | Report |
|---|---|---|
| `explorer` | 3 drift findings | `plans/2026-09-01-recon-v1.16.0-audit-inventory.md` |
| `reviewer` | **REQUEST CHANGES** — 🔴1 🟡16 🟢17 | `reviews/2026-09-01-audit-reviewer.md` |
| `security` | **FINDINGS** — 🔴1 🟡3 🟢8 | `reviews/2026-09-01-audit-security.md` |
| `verifier` | **PASS** (+3 findings) | `testing/2026-09-01-audit-verify.md` |

## Honest assessment

Two independent 🔴s, in code **nobody touched today** — which is the case for
having run this at all. Today's release had four review rounds, three security
passes and a preflight gate, and none of them could see these, because every one
of them was scoped to the diff.

Both are the same shape as the bugs that dominated the day: **a comment asserting
a safety property the code does not have.** The log buffer says `Tail` makes it
safe when `Tail` counts lines, not bytes. The credential sweep says "never let a
crafted path escape the home root" over a purely lexical check that `os.Remove`
then follows through a symlink. That pattern was weighted deliberately in every
scanner's brief, and it produced 9 further instances in the reviewer's pass alone.

What held up is worth stating too, because it is not nothing: the `authGuard`
chain fails closed on unknown modes; `config.Load` refuses to start in header
mode without an ingress token; the ingress strips client-supplied identity
headers before injecting authentic ones; `internal/files` is genuinely
`os.Root`-confined at every entry point; link tokens are 128-bit `crypto/rand`;
nothing hand-rolls auth, crypto or sessions; the username-slug collision that
would collapse two tenants is refused at both load and `Add`; no secrets are
committed; and the API spec matches the code **in both directions** across all 78
registered routes. The test suite was sampled for the "passes while broken" shape
that bit us twice today and no further instances were found.

## Summary

| Severity | Count | Of which cross a trust boundary |
|---|---|---|
| 🔴 High | 2 | 2 |
| 🟡 Medium | 20 | 3 |
| 🟢 Low | 25 | 0 |
| **Total** | **47** | |

## 🔴 High

| Sev | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🔴 | `deploy/caddy/ingress/cmd.go`, `config.go`, `deploy/docker-compose.yml` | 45-50, 67, 8-21 | **Unauthenticated Caddy admin API on host loopback**, in a `network_mode: host` container holding the Docker socket. The generated global options emit no `admin` directive, so no origin restriction; `CADDY_ADMIN` gets no loopback validation (unlike `STYRMAN_DEV_ADDR`, which `config.go:180` refuses when non-loopback). The declared Caddy floor `v2.9.1` predates GO-2026-4537 (cross-origin `/load`, fixed 2.11.1) and GO-2026-5743 (`/config` authz bypass, fixed 2.11.3), both CALLED. One HTTP request to that port replaces the whole ingress config — and the ingress is the sole authority for `X-Caddy-Email`, so that forges any identity including admin, plus host root via the socket. | Emit `admin { origins 127.0.0.1:2019 }` (or `admin off` + unix-socket reload); bump the ingress Caddy floor to ≥2.11.3 and pin what `xcaddy` actually produces; add a loopback guard on `CADDY_ADMIN` mirroring `config.go:180`. Note `deploy/caddy/Dockerfile:24` also `EXPOSE 2019`. |
| 🔴 | `internal/orchestrator/orchestrator.go` | 1229-1234 | **A project VM can OOM the shared server.** Container logs are buffered unbounded into a `strings.Builder` under a comment claiming `Tail` bounds it — `Tail` bounds **lines, not bytes**, and the guest is root and picks the line length (2000 × 10 MiB ≈ 20 GiB). `LogsDialog` re-polls every 30s. `exec.go:26-31` defends the identical threat with `cappedBuffer` and rates it H4/H7: *"without a cap one project could OOM the whole shared server."* | Reuse `cappedBuffer` here. Correct the comment. |

## 🟡 Medium — boundary-crossing

| Sev | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🟡 | `internal/httpapi/credentials.go` | 217-233 | **Cross-tenant file deletion via guest-planted symlink.** `sweepMaterializedFile` guards with a lexical `filepath.Rel` under the comment "Never let a crafted stored path escape the home root", then calls `os.Remove`, which follows symlinks in every parent component. A project user (root in their own VM) symlinks `~/.ssh` at another user's home subtree and deletes their own credential; the server, owning every home, unlinks the victim's private key or agent token. `setup.go:123-126` fixed this exact class by moving to `files.Save`; this call site was missed. | Add a `files.Remove(root, rel)` helper using `os.Root` and pass the home as root. Fix the comment. |
| 🟡 | `deploy/host-setup.sh` | 149-206 | **Egress isolation is a deny-list missing the operator's own network.** The header promises a project VM reaches "the INTERNET but nothing private". The chain DROPs `10/8, 172.16/12, 192.168/16, 169.254/16` — **`100.64.0.0/10` is absent**, which is Tailscale's entire space, on a documented bare-metal homeserver deployment. Host-destined traffic IS covered; the gap is other machines on the tailnet. | Invert to fail-closed (RETURN only for proven-public destinations). Stopgap: add `100.64.0.0/10` and `198.18.0.0/15`, and reword the comment to name what it actually covers. |
| 🟡 | `internal/orchestrator/orchestrator.go` | 759-767 | **The containment model assumes Kata without asserting it.** Every project container gets `CAP_SYS_ADMIN`, `NET_ADMIN`, `NET_RAW`, `apparmor=unconfined`. `Runtime` is `envOr("STYRMAN_RUNTIME","mvm")` — a free string with no validation, in a `config.Load` that otherwise fails closed on auth mode, dev addr and ingress token. Under `runc` (typo, dev override, silently reverted runtime) the same path hands every project user host root. Default holds today. | Allowlist `Runtime` in `config.Load` and/or probe the daemon's registered runtimes at startup, refusing escape-capable containers on a non-VM runtime. |

## 🟡 Medium — correctness and process

| Sev | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🟡 | `internal/config/config.go` | 392 | `usernameFromEmail` duplicates `identity.DeriveUsername`, justified as avoiding an import cycle — **`go list -deps` shows no cycle exists**. The copies are live on *different* auth paths (`auth.go:81` header vs `:86` none), so divergence resolves one email to two different state dirs. No test pins them. | Delete the copy, call `identity.DeriveUsername`. If a cycle ever appears, add a test asserting equality. |
| 🟡 | `deploy/backup/styrman-host-backup` | 17 vs 24 | The DR layer's own doc says `backup.env` MUST redefine `GOOGLE_PROJECT`; the code exports `GOOGLE_PROJECT_ID`. Nothing enforces the "MUST", so a fresh host silently targets the **original** deployment's restic bucket. | Fix the comment, and fail closed when `RESTIC_REPOSITORY` is still the baked-in default on a host that never set `backup.env`. |
| 🟡 | `deploy/.env`, `.agents/docs/testing/styrman/README.md` | — | **Production boots a mutable image tag.** `STYRMAN_IMAGE=styrman-runtime:dev`, and the testing guide's prereq rebuilds that same tag with `dev-$(date +%s)`. Following the documented test procedure re-stamps what production boots — it happened during this audit (marker read `dev-1788269597`). Restored to `1.16.0`; an immutable `styrman-runtime:1.16.0` tag now also exists. | Point `STYRMAN_IMAGE` at a pinned release tag; have the testing docs build `styrman-runtime:test`. |
| 🟡 | `internal/httpapi/format.go` | `formatInPlace` | Zero test coverage, unlike its direct analog `checkerFor` (`TestCheckerFor`). | Add the mirror-image table test. |
| 🟡 | `internal/httpapi/server.go` | notifications | Spec says "one docker exec per running project"; code fans out a goroutine + `ContainerInspect` per project **running or stopped**, uncapped — 100+ concurrent Docker calls per user every 5s at the `REQUIREMENTS.md` scale target. | Bound the fan-out; correct the spec. |

Twelve further 🟡 standards findings (`err.Error()` substring matching ×2, sentinel
`==`, swallowed writer `Close`, hand-rolled sort, 7 copies of atomic-write, a
1,013-line `settings.tsx`, ~235 lines of migration logic in `main.go`, unpinned
`caddy:2-builder`, `Dockerfile.server` never building the SPA) are itemised in
`reviews/2026-09-01-audit-reviewer.md`.

## 🟢 Notable low findings

| Sev | File | Issue |
|---|---|---|
| 🟢 | `changes/2026-09-01-editor-pane-and-ordering.md` §3 | Asserts the modal editor is "still used by Settings" — false; it was deleted later the same day. Never corrected in place, so a reader of `changes/` alone is misled. |
| 🟢 | `changes/2026-09-01-release-v1.16.0.md` | Says the updater "ships disabled **and uninstalled**". `host-setup.sh` step 8c installs the binary and units unconditionally; only `systemctl enable` is conditional. |
| 🟢 | `deploy/README.md` | Documents `backup/` but has no entry for the new `deploy/update/` — the highest-risk component shipped today is the one missing from the index. |
| 🟢 | `internal/orchestrator/orchestrator.go:367` | Comment says the IP→(user,project) map "is identity/audit, not a security boundary". ADR-043 made it one: `credbroker` serves project-scoped env keyed on it. Implementation is correct; the comment would license a future relaxation. |
| 🟢 | `internal/backup/backup.go:115-187` | `Extract`'s "must not depend on who wrote the archive" overclaims: `rejectSymlinkParents` skips the leaf, and no `Skip` list applies on extract. Unreachable today (archives are only server-written, no upload path). |
| 🟢 | Dead code | `LastActivity`, `Project.Agent`, `Project.GitInit` (constant `true`, never read, yet documented in `api.md`). |
| 🟢 | Harness re-seed | Every fresh project accumulates two identical `.bak.*` files in `~/.agents/bin/` — both vendor setup scripts write the same target with no `cmp` guard. |

## Verification

`go build` · `go vet` · `go test -race -count=1 ./...` · `gofmt -l` · `go mod tidy`
(no-op) · the separate `deploy/caddy/ingress` module · `tsc -b` · `oxlint` ·
`shellcheck -S error` ×3 — all clean. `integration.sh` **57/0/3**. e2e **48/0/2**
in isolation (one concurrent run flaked at 47/1/2 under simultaneous image build
+ integration; both events reported).

**Live fresh-install check on the running server:** 0 projects; a throwaway
project created and deleted; harness seeded (6 agents, 28 skills, both vendors,
gates present); `/check` returned real shellcheck and Python diagnostics with no
`__pycache__` litter; **`/format` returned real gofmt'd content, not empty** —
the release's data-loss fix holds in production.

**Test-theatre hunt:** both of today's known-fixed examples negative-controlled
(each guard confirmed to go red with its fix reverted, then restored); ~20 further
test functions sampled across seven packages; **no additional theatre found**.

## Coverage gaps

`internal/identity` untested despite owning the tenancy key · `orchestrator` 3.0%
· `terminal` 8.5% · **zero frontend unit tests** (8,254 LOC, no runner) ·
no handler-level test for `checkFile`/`formatFile` · `formatInPlace` untested ·
`deploy/update/styrman-update` has no automated tests at all.

## What was NOT checked

- **ADR-048 and ADR-049 were inventoried by title only**, not content-checked
  against the code. ADR-050 and ADR-051 both needed same-day corrections, so
  these two deserve the same treatment.
- Not every `internal/httpapi` handler was traced to confirm ownership checks
  resolve through the same `who()`/`gitContext()` path — only that those are the
  two entry points that exist.
- The auto-updater and `internal/httpapi/check.go` were deliberately not
  re-litigated (three security passes today); the security scanner confirmed it
  found nothing new in them, but did not repeat the full analysis.
- `image/harness/**` was inventoried, not reviewed — it is upstream's code.
- No fix in this report has been applied. Nothing here has been re-verified after
  a change, because nothing has been changed.

## Recommended order

1. The two 🔴 — both cross a boundary, both are a few lines, both have a
   correct sibling implementation in the same codebase to copy.
2. `credentials.go` symlink removal (🟡, cross-tenant, same one-line shape).
3. `usernameFromEmail` divergence — a tenancy-key correctness risk, not a nit.
4. Pin the production image tag; it silently changed during this very audit.
5. Everything else triaged into `TODO.md`.
