---
owner: main
date: 2026-09-22
---

# Review — whole project, every lens (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff.

Command: `/review --project --all --high`. Six agents, one per lens, dispatched
in parallel over the same scope; `reviewer` and `security` on the strongest
model (`--high`). No `--fix`: nothing was edited, and no finding below has been
acted on.

## FAIL first

- **Tests lens: FAIL** — on coverage, not on a red test. All six Tier 1
  commands are green at c05219c. `cmd/wsprobe/main.go` has no test file, and
  33 of 47 `web/src` source files are reached by no unit test. Many of those 33
  are named by `testing/styrman/README.md` as Tier 4 (Playwright) territory,
  and Tier 4 was excluded from this pass — so their state is *not examined*,
  not *confirmed absent*.
- **Docs lens: REQUEST CHANGES** — five living-doc copies of the runtime-image
  build command fail as written, and two documents still describe designs the
  code no longer has.

## Verdict per lens

| Lens | Agent | Verdict |
|---|---|---|
| code | `reviewer` (opus) | APPROVE WITH NITS — 3 findings |
| security | `security` (opus) | FINDINGS — 3 + dependency notes |
| simplify | `reviewer` (opus) | APPROVE WITH NITS — 5 findings, all low |
| docs | `reviewer` (opus) | REQUEST CHANGES — 3 + 2 |
| tests | `verifier` | FAIL (coverage; every command green) |
| recon | `explorer` | complete — informational, feeds the lenses above |

## Findings

Severity/confidence as the reporting agent gave them. `M` rows are the main
agent's own, verified in this session.

| # | Sev / conf | Where | Finding | Lens |
|---|---|---|---|---|
| 1 | med / high | `deploy/verify-commit.sh:135-144` | The web/dist freshness gate compares the rebuild's **file-name set** (`find client -type f \| sort`), never content. `_shell.html` and `favicon.svg` have fixed names, so an edit to `web/index.html` or the favicon committed without a rebuild ships stale inside `//go:embed all:dist/client` while the gate prints "committed assets match a fresh build". `web/scripts/normalize-shell.mjs` already makes `_shell.html` byte-reproducible, so a `sha256sum` set is a drop-in. Confirmed by reading the lines. | code |
| 2 | high / high | `README.md:271`, `.agents/docs/deploy/local.md:21`, `.agents/docs/specs/styrman/guide.md:463`, `.agents/docs/specs/styrman/runtime-image.md:7`, `.agents/docs/testing/styrman/README.md:128` | ADR-074 made the GH token a **required** build secret (`image/Dockerfile:150`, `required=true`), but all five living-doc copies still give the bare `docker build … image/` without `--secret id=gh_token`. Each fails as written. `AGENTS.md`'s own Setup block is correct, so the docs disagree with each other. Confirmed by grep. | docs |
| 3 | high / high | `.agents/docs/deploy/local.md:31,36`; `README.md:50-61` | `local.md` documents and passes `STYRMAN_FIREWALL_DIR`, deleted with the ADR-067 spool; `README.md` still describes the ADR-065/066 root-mediated LAN model including SSH-key withholding, whose removal `internal/credbroker/broker_test.go:249` asserts. | docs |
| M1 | high / high | `deploy/.env.example:63` | Same family as #3, missed by the docs lens: the line still says "`STYRMAN_FIREWALL_DIR` is set by docker-compose.yml (/run/styrman-firewall, bind-mounted)". `deploy/docker-compose.yml` contains no firewall reference at all, and `git grep STYRMAN_FIREWALL_DIR` outside `.agents/docs/` returns this line and nothing else — no code reads it. Verified in this session. | main |
| 4 | low / high | `deploy/gcp/install.sh` | Forks `styrman-ctl`'s `env_get` as `grep -E "^$2=" \| tail -n1`, dropping the `is_key` guard and the awk-variable literal match a past security review put there ("a key with a slash changed a sed program"). No live exploit — every caller passes a literal — but the hardening does not travel. It already sources `deploy/gcp/lib.sh`; the hardened copy belongs there. | simplify |
| 5 | low / high | `deploy/gcp/install.sh` | `private_file` is an 11-line near-copy of `styrman-ctl`'s and has **already diverged** in 2 of 5 checks (owner policy; parent dir `&022` vs `&077`). Two copies of a secret-file gate that drifted once will drift again — one `lib.sh` version taking an owner-policy argument. | simplify |
| 6 | low / med | `deploy/caddy/ingress/generator.go` (`writeAppBlock`, ModeRoleGated) | `X-Caddy-Role` is set only inside three `header_regexp` matchers, and a client-supplied copy is never stripped — unlike every other identity header on that path. Not reachable today (styrman ignores the header; admin comes from the userstore). Fix is one line: `request_header -X-Caddy-Role` before the matchers. | security |
| 7 | low / med | `cmd/styrman/main.go:50-51` | The R12 `ip_forward` assertion fails **open** on `fs.ErrNotExist` (Warn + continue) while every other error path exits 1. Deliberately reasoned in place, but it is the shape of BUG-014/028/029/031. | security |
| 8 | low / high | `deploy/verify-commit.sh:170` | The gate's `shellcheck -S warning` omits `deploy/host-setup.sh`, `deploy/styrman-ctl` and `image/entrypoint.sh`. BUG-068 names the first two, and `shared-path-learnings.md` makes `host-setup.sh` *the* shared fresh-host path. All three are clean at HEAD — the gap is coverage, not a live defect. | code |
| 9 | low / high | `internal/httpapi/server.go` (1,405 lines) | `deletingMu` guards the project-lifecycle maps **and** `credharvest.go`'s `credPushing`/`credPushRerun` — two unrelated features sharing one lock inside a 30-field struct. A 3-line `credMu` separates them; the terminal handlers want `terminals.go`. | simplify |
| 10 | low / med | converge paths for ADR-065/066/073 | ~125 lines of never-retiring converge code whose consumers are gone with the superseded designs. Related to #3: the same three retirements left both stale prose and stale code. | simplify |
| 11 | low / high | `go.mod` | `opencontainers/image-spec` is a direct import listed as indirect (`go mod tidy` promotes it); it and `golang.org/x/sys` are direct dependencies absent from `REQUIREMENTS.md`'s approved list. Both defensible — the list was never updated. | code |
| 12 | info | `.agents/docs/testing/styrman/README.md:8`, `:130-135` | Names two deleted test files; carries a v1.16.x caveat. | docs |
| 13 | info | `README.md:112` | Says "v1.34.1, deployed" while v1.37.0 is live, and the release list stops at v1.35.0 — leaving the retired ADR-073 harness channel and `styrman-ctl update --setup --harness-token-file` reading as live instructions. | docs |
| 14 | info | `deploy/caddy/ingress/go.mod` | Module path is still `github.com/spgo/devbox/caddy/ingress`, a leftover from the predecessor repo. Not published, so functionally harmless. | recon |

### Reported and rebutted here

**Security's top finding — the unpinned Antigravity CLI install**
(`image/Dockerfile:106-112`, reported medium/high: `curl … install.sh` then
`bash`, as root, no version pin, no digest, while every other fetch in the file
is sha256- or signature-verified). The substance of the observation is correct,
but the agent did not read the audit note 50 lines above it. `image/Dockerfile:47-56`
records this exact float as a **reasoned, accepted exception** under ADR-069:
Google publishes `agy` latest-only behind a manifest whose sha512 the installer
verifies, offers no versioned artifact, and the CLI self-updates at run time, so
"a build-time pin would be theatre". The proposed fix — pin plus `sha256sum -c`
via `bump-pins.sh` — is therefore not available as written. What remains is the
narrow residual the note does not claim to close: the fetch of `install.sh`
itself is TLS-only, and the manifest verification happens inside a script that
was not itself verified. That residual is real and unrecorded; the finding as
filed is not. Downgraded to **low / med, residual only**.

### Dependency notes (security, recorded not raised)

- `js-yaml@4.3.1` high (GHSA-2883-xcg3-v3hh) — build-time only, transitive under
  `@tanstack/react-start`.
- `GO-2026-4887` / `GO-2026-4883` called in both modules from `docker/docker
  v28.5.2`, no fix available — already accepted at `deploy/vulncheck.sh:9-13`.
  Recorded so a third ID reads as new rather than as more of the same.

### Clear

No committed secrets. No hand-rolled crypto, auth, sessions or JWT/OIDC anywhere
in either module: `crypto/rand` tokens and UUIDs, `subtle.ConstantTimeCompare`,
sessions and OIDC delegated to caddy-security. Config fails closed on auth mode,
ingress token, non-VM runtime and slug collisions. The GCP VM runs
`--no-service-account --no-scopes` with IAP-only ssh. No exported identifier
without a production caller in either Go module (21 lowest-reference candidates
hand-checked); no unreferenced `web/src` export. `internal/netpolicy/` (ADR-067)
and `image/harness/` (ADR-074) are genuinely deleted — the supersessions left no
orphaned package behind. `cmd/wsprobe` looks orphaned but `integration.sh` builds
and invokes it seven times. No stale test found by the docs lens. BUG-074 is
still live at `ingress/config.go:48` and was correctly not raised: the operator
accepted the constraint in e10df4c.

## Verification evidence (tests lens)

Every command run bare, exit status read before judging, per
`.agents/rules/verify-bare.md`:

| Command | Exit | Counts |
|---|---|---|
| `go build ./cmd/styrman/` | 0 | — |
| `go vet ./...` | 0 | — |
| `go test -v -count=1 ./...` | 0 | 311 pass, 0 fail, 1 skip |
| `(cd deploy/caddy/ingress && go test -v -count=1 ./...)` | 0 | 53 pass, 0 fail, 0 skip |
| `(cd web && npm run check)` | 0 | `tsc -b`, silent |
| `(cd web && npm run lint)` | 0 | `oxlint`, silent |
| `(cd web && npm test)` | 0 | 15 files, 178 tests — matches the README baseline |

The one Go skip is `internal/config`'s expected environment-gated skip.

## Not checked

This was a repository-wide pass by six agents under turn budgets; none of them
read everything, and the gaps below are where a finding could still be hiding.

- **`internal/files/` was never read by the security lens** — its own named
  biggest gap. The symlink guards were traced to their call sites
  (`orchestrator.go:344/411/708/758`, `files.Trust` at `httpapi/server.go:206`)
  and those cover create and start, but `openRoot`'s own confinement and the
  BUG-004 `DirEntry.Info()` prohibition are **unverified**. Given
  `guest-writable-paths.md`, this is the most valuable follow-up in this record.
- Roughly 60% of Go files unread by the code lens; most `internal/httpapi`
  handlers, the ingress watcher/cloudflare/cmd, the firewall script's tail,
  `deploy/host-setup.sh`, `styrman-ctl`, `image/entrypoint.sh`, `image/configs/*`
  and all of `web/` unread by the security lens.
- `orchestrator.go` (1,704 lines), `httpapi/server.go` (1,405) and `git.go`
  (1,001) were flagged as size outliers by recon but never read for internal
  cohesion by any lens.
- Unexported dead code was not swept: the installed `deadcode` binary is go1.26
  against a go1.27.1 toolchain and errors out. Rebuilding it would close this
  cheaply.
- Tier 2 (`integration.sh`, 115 checks), Tier 4 (`web/e2e/run.sh`), `vulncheck.sh`,
  any container build and `deploy/verify-commit.sh` were all excluded by
  dispatch — they need the live host. Finding #1 is *in* `verify-commit.sh`,
  which was read but not run.
- Docs lens did not examine `terminal-protocol.md`, `api.md` response shapes, or
  `deploy.md`/`guide.md` end to end.

## Artifacts

- `.agents/docs/reviews/2026-09-22-review-project-code.md`
- `.agents/docs/reviews/2026-09-22-security-project.md`
- `.agents/docs/reviews/2026-09-22-review-project-simplify.md`
- `.agents/docs/reviews/2026-09-22-review-project-docs.md` — ends with 8 `git grep`
  commands that re-check every stale reference it names
- `.agents/docs/testing/2026-09-22-verify-project.md`
- `.agents/docs/plans/2026-09-22-recon-project.md`
