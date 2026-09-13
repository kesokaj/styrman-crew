---
owner: main
date: 2026-09-13
---
# Audit — whole project (2026-09-13)

Range reviewed: working tree at `e56e775` (v1.29.0 + handoff), clean.

## How it was run

A 31-agent workflow: 11 `reviewer` slices (every Go package, the web app in
three slices, deploy/image scripts, the Caddy ingress module), 5 `security`
surfaces (auth/ingress/CSRF, guest-writable paths, credentials/broker,
root-side scripts and the image, frontend and dependencies), one `explorer`
inventory, one `verifier`; then one adversarial refuter per medium/high
finding (13 in the workflow, 5 more by hand for the scans whose structured
return failed). Two security scans and the verifier had to be re-run
outside the workflow (their first runs produced stubs). Every refuter was
told to default to "refuted" unless it could demonstrate the defect from the
code; verdicts and re-graded severities are what the tables below carry.

Artifacts (linked, not pasted):
- Reviews: `2026-09-13-audit-review-{httpapi-core,httpapi-git-files,httpapi-browser-proxy-creds,orchestrator,terminal-conversation-history-files,stores-config-cmd,web-lib,web-components-a,web-components-b-routes,deploy-image-scripts,caddy-ingress}.md`
- Security: `2026-09-13-audit-security-{auth-ingress-csrf,guest-paths-files,credentials-broker,root-scripts-image,web-and-deps}.md`
- Inventory: `.agents/docs/plans/2026-09-13-audit-inventory.md`
- Verifier: `.agents/docs/testing/2026-09-13-audit-verify.md` (every build/vet/fmt/test/lint/shellcheck/vulncheck command PASS; two logic-bearing Go files with zero test coverage → FAIL lines)

## Honest assessment

**One finding is live in production and has been since 2026-09-10:** the
guest's login-shell credential snippet recomputes the broker address for a
`/16` bridge, but `deploy/host-setup.sh` pinned the Docker pool to `/24`
that day (v1.27.1). On this host the container env says
`STYRMAN_CRED_BROKER=http://172.16.1.254:8091` and the snippet rewrites it
to `http://172.16.255.254:8091`, where nothing answers, and its curl is
`|| true`. Every project created since gets no brokered credentials in its
login shells (no `GH_TOKEN`, no gcloud ADC, no ssh key, HTTPS git auth falls
through). Confidence: high — measured on the live host, confirmed by two
independent scans and a refuter. The 2026-09-10 broker reachability test
passed because it used the injected variable directly, not a login shell.

**A second high is user-visible:** `request()` in the SPA combines every
caller's signal with a 20 s timeout, so the deliberately long budgets of
start/stop project and the LAN toggle (150–210 s) are clipped to 20 s —
BUG-022's exact mechanism, re-introduced for two more callers one commit
after BUG-022 was fixed.

Everything else is medium or low: two git-panel verbs that leave a stale
editor buffer, formatter config discovery silently defeated by a temp-file
move, an unbounded boot-time config value, a supply-chain gap in the runtime
image build (four unpinned root fetches under a comment that claims the
opposite), an updater signature gate blind to two of git's four signature
markers (bounded on the timer by `ProtectHome`, open on the documented
hand-run path), an open-redirect suffix match in the auth portal config, and
a setup WebSocket that boots a VM before its Origin check. The codebase as a
whole is in good shape: no cross-tenant path, no header spoofing, no XSS
sink, no hand-rolled crypto, every gate green.

## Summary

| Severity | Confirmed | Refuted / re-graded down | Unverified lows |
|---|---|---|---|
| 🔴 high | 2 | 1 (git Discard → medium) | — |
| 🟡 medium | 14 (all refuted-or-held; the setup-socket boot re-graded low-to-medium) | 2 (orchestrator recreate branch; transcript FIFO hang → low) | — |
| 🟢 low | — | — | 38 |

## 🔴 High

| Sev | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🔴 | `image/configs/styrman-credentials.sh` | 29 | Login-shell snippet rewrites the injected broker URL to `<o1>.<o2>.255.254` (a `/16` assumption); bridges are `/24` since 2026-09-10 → no credentials materialize; silent (`curl … \|\| true`). Companion (medium, :54): on a LAN project the misdirected `GET /files` trusts whatever answers at that address and writes into `$HOME`. | Delete the recomputation: trust `STYRMAN_CRED_BROKER` (a network cannot change under a running container — Docker refuses to remove an attached network, and every recreate path in the orchestrator recreates the container with a fresh env). Fix `runtime-image.md:249-250`. Pin with a test that the snippet contains no octet arithmetic. Make the MOTD say when the broker did not answer. |
| 🔴 | `web/src/lib/api.ts` | 370 | `AbortSignal.any([init.signal, timeout(20000)])` clips every caller's longer budget (start/stop 150 s, LAN toggle 210 s) to 20 s. | `signal: init?.signal ?? AbortSignal.timeout(20000)`; callers that pass a bare controller signal combine their own bound (checkFile already does). Vitest: a caller signal with a 30 s timeout must not abort at 20 s. |

## 🟡 Medium (confirmed)

| Sev | File | Line | Issue | Suggested fix |
|---|---|---|---|---|
| 🟡 | `web/src/components/SidePanelGit.tsx` | 528 | Discard rewrites the tree server-side but never evicts the path from open editor panes; the next Save undoes the discard (re-graded from high: needs an open, unsaved-then-saved buffer). | Call `treeChanged()` after `gitDiscard` like checkout/pull/stash. |
| 🟡 | `web/src/components/SidePanelGit.tsx` | 429 | A conflicting stash pop (server answers 409 but the tree is rewritten with markers) skips `treeChanged()` because it runs only on success. | Run `treeChanged()` in `finally` for pop. |
| 🟡 | `internal/httpapi/format.go` | 119 | Format temp file moved to `$HOME/.styrman/tmp` (46f3ec2) — formatters resolve config from the file's own path, so the project's prettier/black/ruff/rustfmt/shfmt config is no longer found; header comment and api.md still claim the opposite. (`check.go:427` same shape, low.) | Format's temp file back in the workspace as a `.styrman-fmt-*` dotfile (lifetime ~100 ms, an explicit user action, unlike check's debounce tick); fix the comments and api.md. |
| 🟡 | `internal/config/config.go` | 170 | `STYRMAN_DOCKER_DISK_GB` unbounded at boot while every other path bounds it 5–500. | Validate in `Load`, fail closed like the sibling checks. |
| 🟡 | `internal/files/files.go` | 180 | `ContainedPath` has no production caller (`deadcode` confirms; last caller deleted with `migrateWorkspaces`) and is the package's only check-then-use path. | Delete it and its test. |
| 🟡 | `internal/files/files.go` | 297 | `Read` discards `io.ReadFull`'s error: a real I/O fault is served as 200 + `Truncated`. | Accept `io.EOF`/`io.ErrUnexpectedEOF`, return anything else. |
| 🟡 | `internal/credbroker/broker.go` | 61 | Any container inside a project's dockerd reaches the broker with the guest's IP and gets the user's whole credential set; no ADR records that. | Record the decision (the user's own workloads, R3's shared-credentials model; the boundary is the project) in ADR-030's amendment; no code change. |
| 🟡 | `deploy/update/styrman-update` | 313 | The "must be SSH-signed" grep knows two of git's four signature markers; an object with an SSH block followed by `PGP MESSAGE`/`SIGNED MESSAGE` passes the gate while git dispatches to gpg/gpgsm. Bounded on the timer (`ProtectHome=yes`), open on `sudo styrman-update --force`. | Widen the regex to all four markers, keep "last must be SSH"; run verification with `GNUPGHOME` at an empty dir; add the four fixtures to the updater's test. |
| 🟡 | `image/Dockerfile` | 89–97 | Go toolchain fetched as "newest today" with no checksum; NodeSource and Antigravity `curl \| bash` as root; `npm install -g @anthropic-ai/claude-code` unpinned and without `--ignore-scripts`; the comment at :55 claims everything is pinned and verified. The auto-updater rebuilds the image on every tag, so rollback determinism depends on this. | Pin Go by version + sha256; install Node from the official tarball with `SHASUMS256.txt`; pin claude-code's version + `--ignore-scripts`; for the Antigravity installer record the accepted risk (no upstream pinning mechanism) in the comment and ADR instead of a hash that would break every release on an upstream edit; correct the comment. |
| 🟡 | `deploy/caddy/ingress/generator.go` | 264 | `trust login redirect uri domain suffix <BASE_DOMAIN>` is a bare `HasSuffix` in go-authcrunch: `evilstyrman.xyz` is trusted and `?redirect_url=` survives sign-in — open redirect on the auth portal (refuter: HOLDS — `go-authcrunch v1.1.41` `redirect_match.go:59-60` is a bare `HasSuffix`; `?redirect_url=` survives Google sign-in via the referer cookie, `handle_http_login.go:377-405`; a leading dot is accepted by the directive and `crew.<BASE>` still matches; no token leaks). | Emit `.` + BASE_DOMAIN; a generator test asserting the leading dot (there is none for this line today). |
| 🟡 | `internal/terminal/bridge.go` | 196 | `ServeSetup` boots the setup microVM before the Origin check (which only runs inside `websocket.Accept`); csrfGuard exempts GET; a page can force-boot a VM (refuter: HOLDS at low-to-medium — the cookie carries no SameSite so Chromium treats it Lax and drops it on a cross-site WS handshake, Firefox/Safari do not; and from a SAME-SITE page — another tenant's opt-in preview host `<token>.public.crew.<BASE>` — every browser sends it. Bounded: `EnsureSetup` is per-user idempotent, one VM per victim, reaped by the idle reaper; no guest code runs before `Accept`). | Check `sameOriginOrAbsent` first in `ServeSetup` (and `ServeTerminal`), as `browserSocket` does; `cookie samesite lax` in the generator closes the cross-site half. |
| 🟡 | `.agents/docs/specs/styrman/api.md` | 15, 200, 266 | Documents the deleted ADR-065 design (lanState/lanMessage, hostFirewall, `STYRMAN_FIREWALL_DIR`) as current. | `documentation` rewrites those sections to ADR-067. |
| 🟡 | `internal/httpapi/projectenv.go`, `stats.go` | — | Reachable routes with zero test coverage (verifier FAIL lines). | Handler tests over the guest seam. |

## 🟢 Low (unverified; the cheap ones are in the fix batch, the rest recorded)

| File | Line | Issue | Suggested fix |
|---|---|---|---|
| `internal/history/history.go` | 230 | `Read` lacks the `IsRegular` check its sibling has (refuter: a FIFO with no writer reads as EOF, a guest cannot hold the host inode — residual is parity). | Lstat + `IsRegular` before open, as `searchOne`. |
| `internal/httpapi/server.go` | 842 | `createProject` leaks the workspace dir when tenant-home creation fails after the workdir exists. | Remove the workdir on that path. |
| `internal/httpapi/server.go` | 168 | `Server.provisioning` keyed unlike its sibling maps. | One key shape. |
| `internal/httpapi/credentials.go` | 36 | Store read failures masked as 404. | 500 + log for non-NotFound errors. |
| `internal/httpapi/browser.go` | 456 | Relay pumps are not joined before the slot is released. | Wait for both pumps after the closes. |
| `internal/httpapi/check.go` | 427 | Same temp-file/doc gap as format, lower impact. | Comment says why check stays out of the workspace. |
| `internal/httpapi/explorer.go` | 1 | Grab-bag file (explorer + settings + admin + credential import + notifications). | Not in this batch (a move, not a fix). |
| `internal/orchestrator/orchestrator.go` | 222 | Bridge-mode-mismatch logic duplicated. | One helper. |
| `internal/config/config.go` | 503 | `envIntOr`/`envDurationOr` swallow parse errors silently. | WARN line. |
| `internal/project/uuid.go` | 12 | `crypto/rand` failure falls back to an id the loader later quarantines. | Fail hard (panic-free: return an error to the caller). |
| `internal/credstore/credstore.go` | 459 | `safe()` not injective (hosts collide onto one file). | Follow-up: needs a store migration; recorded. |
| `internal/credstore/credstore.go` | 86 | 0700 only on create. | `Chmod(0700)` always. |
| `internal/credbroker/broker.go` | 87 | Every GetGit error is a silent 404. | Log non-NotFound. |
| `internal/terminal/bridge.go` | 469 | Project-stopped close can overwrite the revoked code in the LOG only. | Cosmetic; recorded. |
| `internal/terminal/bridge.go`, `httpapi/browser.go` | 369, 428 | The 60 s re-check tests the allowlist, not session validity (a socket outlives the 8 h JWT). | Follow-up: needs the ingress to expose expiry; recorded. |
| `deploy/caddy/ingress/config.go` | 47 | `AuthCookieName` has no caller. | Delete. |
| `deploy/caddy/ingress/labels.go` | 64 | `sanitizeHostname` can yield empty/colliding names. | Validate, refuse empty. |
| `deploy/caddy/ingress` (public block) | — | `Cookie` not stripped on the public block; prefix exemptions on an unnormalized path (no bypass found). | Strip; `path.Clean` before prefix checks. |
| `deploy/host-setup.sh` | 372, 199, 50, 65, 224 | `install -d /etc/styrman` unguarded; `.env` perms; Docker from distro; Kata checksum; two more. | The guard + perms in the batch; rest recorded. |
| `deploy/firewall/styrman-firewall.sh` | 69 | (see the security report) | Recorded. |
| `deploy/styrman-ctl` | 555, 646 | `${ETC_DIR}` handling; `edit KEY=VALUE` accepts loose keys. | Validate the key. |
| `deploy/Dockerfile.server`, `image/Dockerfile` | 9/19, 8/15 | Base images tag-pinned, not digest-pinned (the caddy Dockerfile digest-pins and says why). | Digest-pin. |
| `image/entrypoint.sh` | 165 | A build-time string is … (see report). | Recorded. |
| `image/configs/styrman-credentials.sh` | 60 | shellcheck SC2088 false positive. | Disable-comment. |
| `web/src/components/BrowserPane.tsx` | 45, 327 | Unused `TargetInfo.attached`; `upsert` type-checks `targetId` only. | Delete; check `url`/`title` are strings. |
| `web/src/components/FileExplorer.tsx` | 178 | Ad-hoc polling beside `usePolled`. | Use `usePolled`. |
| `web/src/components/icons.tsx` | 159 | `ExternalLinkIcon` unused. | Delete. |
| `web/src/routes/settings.tsx` | 963 | Orphan JSDoc for a removed ADR-065 view. | Delete. |
| `web/src/lib/api.ts` | 867 | `portUrl` skips the `u` tag. | Use it. |
| `web/src/lib/diffReveal.test.ts` | 2 | Tests a components module from lib/. | Move next to what it tests. |
| `web/src/lib/clips.ts` | 71 | Documented invariants, no unit test. | Vitest. |
| `web/src/components/DiffView.tsx` | 10 | Zero-width Trojan-Source variant not revealed. | Add U+200B/U+200C/U+200D/U+2060/U+FEFF. |
| `web/.claude/agent-memory/` | — | Tracked in git (agent scratch). | `git rm --cached`, gitignore. |
| `web/package.json` | — | `js-yaml@4.3.1` high in `npm audit` (dev/build-time). | Bump when upstream ships. |
| `REQUIREMENTS.md` | — | `golang.org/x/sys/unix` not in the approved-deps list. | Needs operator approval; recorded. |
| `.agents/docs/testing/styrman/integration.sh` | — | No suite exercises credential delivery through a login shell. | The host-run test server has no broker (SelfContainer empty); pin the snippet by a unit test instead. |

## Refuted

- `internal/orchestrator/orchestrator.go:280` — the workspace-bind recreate branch lacks a running guard, but the scenario is unreachable for any container the current server creates (single create site sets the bind).
- `internal/history/history.go:230` — as stated (goroutine leak on a planted FIFO): read on a writer-less FIFO returns EOF; `fifo_test.go` pins it; a guest cannot hold the host inode open (virtiofs). Residual parity fix is low.
- `image/Dockerfile` chrome-devtools-mcp — pinned (`ARG …=1.9.0`, `--ignore-scripts`); dropped from the finding.

## What was not checked

- Web components' coverage map is approximate (verifier: filename grep against e2e checks, ~20 files with no unit test and no traced e2e driver).
- Kata guest kernel behaviour for FIFOs on virtiofs was reasoned, not measured.
