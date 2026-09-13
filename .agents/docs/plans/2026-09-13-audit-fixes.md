---
owner: main
date: 2026-09-13
---
# Plan — Audit fixes (v1.30.0)   (2026-09-13)

## Why

The whole-project audit `.agents/docs/reviews/2026-09-13-audit-project.md`
(31-agent workflow + refuters) confirmed 2 high and 14 medium findings and
listed 38 lows. The operator's instruction: "Now a full /audit of there are
plenty of fixes create a minor and use /preflight then commit and push."
There are. One high is live in production since 2026-09-10 (BUG-054: no
brokered credentials in login shells on /24 bridges).

## Scope

In: every confirmed high and medium; the lows marked *do* below (cheap,
mechanical, testable). Out: lows marked *record* (need a migration, an
operator approval, or a move rather than a fix) — they stay in the audit
report and TODO.md.

Release: MINOR (v1.30.0) — user-visible behaviour changes (credentials
work again in login shells; long operations no longer time out at 20 s;
Discard/pop keep the editor honest; formatter config discovery restored;
the image build is deterministic) and nothing an existing deployment must
react to.

## Findings → fixes

### High
| # | Where | Fix |
|---|---|---|
| H1 | `image/configs/styrman-credentials.sh:25-31` | Delete the `/16` recomputation; the injected `STYRMAN_CRED_BROKER` is authoritative (Docker refuses to remove a network with an attached container; every orchestrator recreate path recreates the container with a fresh env). Keep the `[ -n ]`/`command -v curl` guards. When `/env` does not answer, write one line to `$HOME/.styrman/broker-unreachable` (timestamp + URL) that `profile-motd.sh` shows as "credentials: broker did not answer at <url> (<time>)"; delete the marker on the next success. `runtime-image.md:249-250` rewritten. Test: `internal/credbroker/snippet_test.go` reads the snippet, asserts it contains no `255.254`/`cut -d. -f1,2`, references `${STYRMAN_CRED_BROKER}` and the marker path; and `bash -n` it. |
| H2 | `web/src/lib/api.ts:370` | `signal: init?.signal ?? AbortSignal.timeout(20000)`; comment says a caller that passes a signal owns its bound. Audit of callers: `checkFile` combines its own 20 s (kept), start/stop 150 s, LAN 210 s, upload 120 s. Vitest `api.timeout.test.ts` (stubbed fetch reading `init.signal`): default bound present when no signal; a caller's `AbortSignal.timeout(30000)` is passed through unchanged (not an `any` composite — assert `signal === given`). |

### Medium
| # | Where | Fix |
|---|---|---|
| M1 | `SidePanelGit.tsx:528` | Discard → `run(async () => { try { await api.gitDiscard(...) } finally { treeChanged() } })`. |
| M2 | `SidePanelGit.tsx:429` | Pop → `treeChanged()` in `finally` (the 409 path rewrote the tree). |
| M3 | `format.go:119` (+ comments, `api.md`) | `formatScript`: temp file `mktemp -p "$WS" .styrman-fmt-XXXXXX --suffix=.ext` where `$WS` is the exec's WorkDir (`ws`), so the formatter resolves the project's config; `check.go` stays in `~/.styrman/tmp` with its comment saying why (a debounce tick, and checkers take the path). `format.go:84-86` comment and `api.md`'s format section corrected. `format_test.go` asserts the script's mktemp targets `$PWD`-relative workspace and the dot-prefix. |
| M4 | `config.go:170` | `Load`: after `envIntOr`, `if c.dockerDisk < 5 \|\| c.dockerDisk > 500 { return nil, fmt.Errorf("STYRMAN_DOCKER_DISK_GB must be 5-500, got %d") }`. Test in `config_test.go`. |
| M5 | `files.go:180` | Delete `ContainedPath` and `TestContainedPath` (files_test.go:147-168). |
| M6 | `files.go:297` | `n, err := io.ReadFull(f, buf); if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) { return nil, err }`. Test: a reader that fails mid-read is hard to plant through os.Root; assert the EOF cases still return content (existing tests) and add a comment. |
| M7 | `broker.go:61` | No code change: ADR-030 amendment records that any workload inside a project's dockerd shares the project's identity at the broker (the boundary is the project, R3), with the LAN-mode note. `changelog` writes it. |
| M8 | `styrman-update:313-315` | Regex `^-----BEGIN (SSH SIGNATURE\|PGP SIGNATURE\|PGP MESSAGE\|SIGNED MESSAGE)-----`, last must be `-----BEGIN SSH SIGNATURE-----`; `GNUPGHOME` pointed at a fresh empty temp dir for the `git verify-tag` call (removed after). Test: `internal/config/styrman_update_test.go` (new; the ctl test's harness shape) builds tag objects with placeholder armor blocks via `git hash-object -t tag -w` in a scratch repo and runs the script's `verify_tag` function (sourced) against the four fixtures + an honestly SSH-signed tag with a scratch allowed_signers → three refused, honest passes; and with an EMPTY allowed_signers the honest tag is refused. |
| M9 | `image/Dockerfile:88-99` | `ARG GO_VERSION`/`GO_SHA256` (go.dev's published sum), Node from `nodejs.org/dist/vX/node-vX-linux-x64.tar.xz` verified against `SHASUMS256.txt`'s line (`NODE_VERSION`/`NODE_SHA256`), `npm install -g --ignore-scripts @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}`; Antigravity: download `install.sh` to a file, run it, and RECORD the accepted floating version in the comment (no upstream pin; a hash would break every release on an upstream edit — decision in the ADR); the `:55` comment rewritten to name exactly what is pinned and what floats and why. `runtime-image.md` "Go (latest stable at build time)" → the pinned version and the bump procedure. Base images digest-pinned (`debian:trixie`, `debian:trixie-slim`, `golang:1.25.13-trixie`) as `deploy/caddy/Dockerfile` does. Verify: `docker build` of the image locally (the e2e's ensure-test-image), `deploy/verify-commit.sh` at release. |
| M10 | `generator.go:264` | `domain suffix .` + BASE_DOMAIN (label-bounded) — pending the refuter; if it holds, plus a generator test asserting the leading dot. |
| M11 | `bridge.go:196` | `ServeSetup`: `sameOriginOrAbsent`-style check BEFORE `ensure` (move the check the bridge has for terminals, or add one); refuse 403 before any side effect; test. Pending the refuter. |
| M12 | `api.md:15,200,266` | `documentation` rewrites the network section to ADR-067 (two-state `lanAccess`, `canEnable`, no host firewall spool). |
| M13 | `projectenv.go`, `stats.go` | Handler tests through the guest seam (`guestexec_test.go` style): projectenv GET/PUT round-trip + validation refusals; stats shape + a failing guest. |

### Low — *do*
history.go:230 Lstat+IsRegular parity; server.go:842 workspace-dir cleanup on the failing path; server.go:168 key shape; credentials.go:36 500+log on non-NotFound; browser.go:456 join the pumps; check.go:427 comment; orchestrator.go:222 one helper; config.go:503 WARN on unparsable env; uuid.go:12 return an error instead of a fallback id (callers: createProject → 500); credstore.go:86 chmod 0700 always; broker.go:87 log non-NotFound; caddy `AuthCookieName` deleted; `sanitizeHostname` refuses empty; host-setup.sh:372 guard; styrman-ctl:646 key validation `^[A-Z][A-Z0-9_]*$`; BrowserPane `attached` field deleted + `upsert` checks url/title strings; icons `ExternalLinkIcon` deleted; settings.tsx orphan JSDoc deleted; `portUrl` through `u`; diffReveal.test.ts moved beside DiffView (or the function into lib — whichever keeps imports one-way); clips.ts vitest; DiffView zero-width chars; `web/.claude/agent-memory/` untracked (`git rm -r --cached`, .gitignore) — BUG-006 rule: `git ls-files` first, remove by name, `git status` after; SC2088 disable comment; public-block Cookie strip + `path.Clean` before prefix exemptions (ingress).

### Low — *record* (TODO.md / audit report, not this batch)
explorer.go grab-bag split; credstore `safe()` injectivity (store migration); session-validity re-check (needs the ingress to expose expiry); REQUIREMENTS.md deps list (operator approval); host-setup Docker-from-distro / Kata checksum / .env perms; firewall :69; entrypoint :165; js-yaml (upstream); FileExplorer polling (behavioural change, own review).

## Surfaces touched
`image/configs/{styrman-credentials.sh,profile-motd.sh}`, `image/Dockerfile`, `deploy/Dockerfile.server`, `deploy/update/styrman-update`, `deploy/caddy/ingress/{generator.go,config.go,labels.go,+tests}`, `deploy/host-setup.sh`, `deploy/styrman-ctl`, `internal/{files,history,config,credstore,credbroker,project,orchestrator,terminal,httpapi}` (+ new tests: `credbroker/snippet_test.go`, `config/styrman_update_test.go`, `httpapi/{projectenv,stats}_test.go`), `web/src/lib/{api.ts,+api.timeout.test.ts,clips.test.ts}`, `web/src/components/{SidePanelGit,BrowserPane,DiffView,icons}.tsx`, `web/src/routes/settings.tsx`, `.gitignore`, docs (`api.md`, `runtime-image.md`, testing README counts), ADR-030 amendment + ADR for the image pins.

## Failure modes
- H1: broker unreachable → MOTD line names the URL and time; nothing else changes (still `|| true` for the login shell's sake).
- M4: a bad `STYRMAN_DOCKER_DISK_GB` stops the server at boot with the message (same as the other fail-closed checks).
- M8: a tag whose trailing block is not an SSH signature is refused with the existing message; a mis-dispatch to gpg cannot succeed (empty keyring).
- M9: a changed upstream Go/Node tarball fails the image build loudly (`sha256sum -c`); the release fails and rolls back; the fix is bumping the ARG pair.
- uuid: `crypto/rand` failure → createProject 500 "cannot generate an id", nothing persisted.

## Verification
- Unit: `go test -race ./...` (new: snippet pin, config bound, update fixtures, projectenv/stats handlers, files Read error path), ingress module tests, `cd web && npm run check && npm run lint && npm test` (new: api timeout, clips).
- Image: `docker build image/` (ensure-test-image) proves the pins; `deploy/verify-commit.sh` at release.
- Integration + e2e on the rebuilt image (credentials snippet, format temp file: e2e has a format check? — the editor's Format button check if present; else the integration tier's `format` route check).
- Live after deploy: a login shell in a NEW project shows `GH_TOKEN` set (operator or a `docker exec … bash -lc 'env | grep -c GH_TOKEN'` against a project of the test server — the host-run server has no broker, so this is production-only: verified after the updater deploys, recorded in the handoff).

## Phases
### Phase 1 — Production defects (H1, H2, M1, M2, M4, M5, M6, low history/uuid/credstore/broker)
- [ ] snippet + MOTD + pin test — verify: go test, bash -n (AI)
- [ ] api.ts signal + test — verify: vitest (AI)
- [ ] git panel finally/treeChanged — verify: check/lint (AI)
- [ ] config bound + test; files Read/ContainedPath; history parity; uuid; credstore/broker logs — verify: go test -race (AI)
### Phase 2 — Server + ingress (M3, M8, M10, M11, M13, lows)
- [ ] format temp back in the workspace + comments + test — verify: format_test, integration format check (AI)
- [ ] updater marker regex + GNUPGHOME + fixture test — verify: new Go test (AI)
- [ ] ingress suffix dot + AuthCookieName + sanitizeHostname + Cookie strip + path.Clean; tests — verify: ingress module tests (AI)
- [ ] ServeSetup origin-first + test — verify: terminal/httpapi tests (AI)
- [ ] projectenv/stats tests; server.go lows; browser.go join; orchestrator helper; config WARN — verify: go test -race (AI)
### Phase 3 — Image + scripts (M9, host-setup, styrman-ctl, digests)
- [ ] Dockerfile pins + comment + base digests; Dockerfile.server digest — verify: local image build (ensure-test-image), styrman-runtime:test boots in integration (AI)
- [ ] host-setup guard; styrman-ctl key validation + test — verify: bash -n, shellcheck, ctl test (AI)
### Phase 4 — Web lows, docs, records, release
- [ ] BrowserPane/icons/settings/portUrl/diffReveal move/clips test/DiffView/agent-memory untracked — verify: check/lint/test, git status (AI)
- [ ] documentation (api.md ADR-067, runtime-image.md, testing README), changelog (audit + fixes + ADR-030 amendment + image-pin ADR), bugs (BUG-054/055 done; any new), TODO records — then `/preflight v1.30.0`

## Risks
- M9 makes every future release depend on pinned upstream artifacts still being downloadable at those sums; the bump procedure is documented and the failure is loud.
- H1 removes a "resilience" hack; the design argument (network cannot change under a container) is recorded in the ADR so nobody re-adds it.
