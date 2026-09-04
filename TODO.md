# TODO

Empty by design (operator directive 2026-08-30: "I want nothing in todo and
roadmap — start fresh with new things"). The entire previous roadmap was
either SHIPPED in v1.14.0 or RETIRED with reasons in **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) — including the standing
WON'T-DO decisions. History lives in `.agents/docs/changes/` and git.

Add new items below as they arise.

## Inbox

- [x] Bug (operator, 2026-09-03): after some minutes the bottom stats bar and the sidebar show "fail to fetch"; a browser F5 fixes it until it recurs. **DONE 2026-09-03** — BUG-001: the ingress session token (caddy-security default lifetime 900 s) expires, the `/api/*` redirect to the auth subdomain is cross-origin, and the SPA's `fetch()` rejected before the existing re-auth handling could see it. `request()` now uses `redirect: "manual"` and treats an opaque redirect as an expired session (one toast, one reload). See `.agents/docs/bugs/BUG-001-fail-to-fetch-session-expiry.md` and `.agents/docs/changes/2026-09-03-fail-to-fetch-session-expiry.md`. Not yet released: ships with the next tag.
- [x] Operator decision — session token lifetime. **DONE 2026-09-04** (approved: "fix then commit to the newest version"): `crypto default token lifetime 28800` in the portal block, ADR-063, generator test; ships in v1.20.0. Original note: The ingress uses caddy-security's default `token lifetime` of 900 s with nothing set in `deploy/caddy/ingress/generator.go` `writeSecurityBlock`, so after BUG-001 the SPA now reloads itself every 15 minutes of use (a toast, then `location.reload()`; the terminal reconnects, in-memory UI state is lost). The security review of BUG-001 (`.agents/docs/reviews/2026-09-03-security-fail-to-fetch.md`, observation) reasons that the lifetime is not the revocation control here — `authGuard` re-checks the userstore on every request, so removing a user takes effect at once regardless — and recommends setting it deliberately: `crypto default token lifetime 28800` (a working day) in the `authentication portal` block, recorded in the deploy spec. Trade: a stolen cookie stays valid that long, zone-wide (`cookie domain`). One line plus a generator test; do it with the operator's yes. (added 2026-09-03)
- [ ] 🟢 `web/src/lib/api.ts` interpolates `project`, terminal `id`, and link `token` into API paths without `encodeURIComponent` (`scopeBase` and the `/api/projects/${id}` templates). The weakness is path retargeting, not the reload (security review 2026-09-03, round 2): browsers resolve `..` and `?` before sending, so a crafted link can point a clicked action at another `/api/projects/...` path of the SAME user — `csrfGuard` passes it (genuinely same-origin) and picks its body-size class by path suffix; no tenant crossing. The mux's path-cleaning 301 from such an id now reads as "session expired", bounded to two reloads and an error toast by BUG-001's counter fix. Encode the three. (security review 2026-09-03, low)
- [ ] 🟢 Ingress: `trust login redirect uri domain suffix {BaseDomain}` (`generator.go:260`) trusts the whole DNS zone as a post-login `redirect_url` target, including the public-preview wildcard a project owner controls; the practical ceiling is a trusted-looking landing page (cookies and identity headers are stripped before a guest app sees them). Narrow the suffix to the app host and confirm caddy-security's matcher has a label boundary. Pre-existing, not widened by BUG-001. (security review 2026-09-03, low/low)
- [ ] 🟢 WebSocket sessions are authorized at the handshake only (`internal/terminal/bridge.go`): a removed user's already-open terminal tab keeps streaming until the socket drops. BUG-001's reload used to re-handshake every 15 minutes by accident; with the eight-hour token (ADR-063) that incidental bound is gone. Fix idea: close a user's sockets when the userstore removes them, or re-check authorization on a timer. (security review 2026-09-04, low)
- [ ] 🟢 The ingress generator's Caddyfile is asserted by substring only; nothing in `deploy/verify-commit.sh` runs `caddy validate`/`adapt` on a generated config, and a config the plugin rejects on cold boot serves nothing. The 2026-09-04 lifetime change was validated by hand against `styrman-caddy:1.19.1` (`testing/2026-09-04-verify-session-lifetime.md`); wire that into the gate (build the caddy image, generate for one authenticated app, `caddy validate`). (security review 2026-09-04, low)
- [ ] 🟡 ADR proposal for the operator — un-nest the project homes. ADR-031 put every project home at `HomesDir/<user>/p/<uuid>`, INSIDE the base home the setup VM bind-mounts with a root guest, so `p` and `p/<uuid>` are guest-replaceable. Three code paths trusted that path and were fixed defensively on 2026-09-03 (BUG-002 `files.Trust`, BUG-003 `realBelow` before the bind and the confined janitor), but the orchestrator's check is not atomic with `docker create`: a guest racing it from a running setup VM is a residual window. The durable fix is a layout where no tenant-owned tree sits under a directory a guest can write — e.g. `HomesDir/<user>/base` for the setup VM's home and `HomesDir/<user>/p/<uuid>` for projects with only `base` mounted, or `HomesDir/p/<user>/<uuid>`. Needs a migration of existing homes (rename on the host while nothing runs) and an ADR; the operator's call. (security review 2026-09-03, rounds 2–3)
- [ ] 🟢 Upstream (`styrman-harness`, the operator's repo — styrman never edits the mirror): (1) `sh-agent` turns frontmatter `name:` values and manifest paths into filesystem paths with no `filepath.IsLocal` check (`internal/harness/plan.go`, `apply.go`) — harmless in a project VM (the installer runs as the project user in their own home) but worth closing at the source; (2) `sh-agent version` prints `(devel)` and the manifest's `commit` is empty when the repo has no `.git` (the image's mirror) — a `-ldflags -X` version or a `--commit` flag would let the image stamp `image/harness/COMMIT` into the manifest; (3) `git rev-parse` in `repo.go` ascends out of `--repo` when the mirror sits inside another git checkout (on this host it recorded styrman-crew's own commit as the harness commit — cosmetic, and never in the image, which has no `.git` above `/styrman/harness`); (4) `Install` in `apply.go` returns on the first write error, so a `chmod`'ed directory in the home leaves a partial install repeating every boot — continue and report, as `harness-migrate.sh` now does. Next mirror refresh picks these up. (security review of the harness replacement, 2026-09-03, low)

- [x] Settings harness roster is permanently empty. **DONE 2026-09-03** (ADR-062, Phase 3): `toolDetails` reads the roster from the user's newest project home that carries the sh-agent manifest (`rosterHome`), agy agents from `.gemini/config/agents`, and `listNames` ignores the old `*.bak.<ts>` re-seed copies; the settings cards show "N skills · M subagents". Original note: `internal/httpapi/setup.go:230-252`
  reports the harness roster (`Skills`, `Agents`, `Commands`) from
  `s.cfg.UserHome(u.Username)` — the base/setup home. The entrypoint never
  seeds that home, because the setup VM runs with `STYRMAN_MINIMAL=1`
  (`internal/orchestrator/orchestrator.go:565` → `image/entrypoint.sh`), so
  all three fields are always empty and `omitempty` hides them. Confirmed on
  the host: `/var/lib/styrman/homes/ostling-simon/.claude/agents` does not
  exist. Consequence: the 28 skills / 6 subagents shipped in v1.15.0 can
  never surface in the UI. Pre-existing, deliberately deferred out of the
  v1.15.0 vendoring release; both the reviewer and the agent agree the
  deferral is correct because a real fix needs a product decision (point the
  lookup at a project home, or delete the three fields as dead reporting).
  Secondary: if it is ever repointed at a seeded home, `listNames`
  (`setup.go:167-178`) strips only a `.md` suffix, so harness re-seed
  backups would be listed as agents — a real example exists on the host:
  `~/.claude/agents/testing.md.bak.20260901-065226`. Filter `*.bak.*` if
  repointed. (deferred: needs a product decision, not a code fix)
- [ ] `checkFile`/`formatFile` (`internal/httpapi/check.go`,
  `internal/orchestrator/exec.go`) have no Go handler-level test — only e2e
  coverage plus the script-contract unit test
  (`TestGuestScriptsNeverWaitForStdinEOF`) exercise them. (deferred: flagged
  by the verifier during the 2026-09-01 syntax-check/auto-updater batch,
  not fixed in that pass)
- [x] `deploy/Dockerfile.server` still embeds a committed `web/dist` with
  nothing verifying it matches current `web/src`. **DONE 2026-09-02** — v1.19.0
  `deploy/verify-commit.sh` includes a `web/dist` freshness rebuild-and-compare
  step in the pre-tag gate, catching stale assets before tagging. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md` and ADR-061.

- [x] Unpinned toolchain fetches in `image/Dockerfile` — `yq` uses
  `releases/latest/download`, and helm/OpenTofu install via `curl | bash`, all
  without checksums. **DONE 2026-09-02** — v1.19.0 removes helm entirely (operator
  directive), and adds `deploy/install-tools.sh` pinning OpenTofu and yq with sha256
  cross-checked against publishers' release pages. Caddy base images now digest-pinned.
  See `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.

- [ ] `govulncheck ./...` reports 2 CALLED vulnerabilities, both in
  `github.com/docker/docker@v28.5.2+incompatible` (GO-2026-4887, GO-2026-4883),
  both **Fixed in: N/A** and both daemon-side, flagged only through `init`
  reachability. Pre-existing, no upgrade available — recheck when docker
  publishes a fix. (security review 2026-09-01)

- [ ] Carried-over low findings from the 2026-09-01 re-review of the
  syntax-check/auto-updater remediation
  (`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-rereview.md`),
  none re-argued, all still open: `deploy/update/styrman-update.timer`'s
  `Persistent=true` is inert on a monotonic (`OnBootSec`/`OnUnitActiveSec`)
  timer; `deploy/host-setup.sh` step 8c is still physically ordered before
  step 8b; `deploy()` calls `fail` (which exits) on a checkout error instead
  of `return 1` like the build/health paths, so that one path still bypasses
  the failed-tag latch; `mktemp -p .` in both `check.go` and `format.go`
  writes into the workspace root on every debounce tick instead of a
  dot-dir; the Terraform checker still reports `STYRMAN_TOOL gcc` to the UI
  (mislabelled — it runs `tofu fmt -check`, not gcc, and that's a formatting
  check sold as a syntax check); dead trap globs remain in `check.go`/
  `format.go` (`${F}.err`/`${F}.out` for files never created, plus a
  trailing `rm -f` the `EXIT` trap already covers); `checkFile`
  (`web/src/lib/api.ts`, `CodeEditor.tsx`) takes no `AbortSignal`; the lint
  gutter (`lintGutter()`/`linter()`) is still mounted unconditionally
  instead of only for supported file types; and the close-file confirm
  dialog's "Discard" button (`EditorPane.tsx:171-178`) still discards
  nothing. (deferred: all low-severity, none blocking; batched here so
  they aren't lost)

- [x] Unsaved editor edits are still lost without a prompt when switching
  between terminals. **DONE 2026-09-02** — v1.19.0 moves the draft buffer
  to the per-terminal pane store (keyed by terminal ID), so it survives
  terminal switches and is restored only for the file it belongs to. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.

- [x] `TerminalPane`'s focus/refit schedule sends up to four identical
  tmux resize frames per Editor→Terminal toggle. **DONE 2026-09-02** — v1.19.0
  eliminates duplicate resize frames; the 120ms debounce now works as intended.
  See `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.

- [x] 🟡 Cross-tenant deletion via symlink in `sweepMaterializedFile`
  (`internal/httpapi/credentials.go:217`), fix with an `os.Root`-confined
  `files.Remove`. **DONE 2026-09-02** — v1.19.0 ships `files.Remove` with
  `RootConfinedDelete`, regression test plants the symlink and confirms it's
  rejected. See `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟡 `usernameFromEmail` duplicates `identity.DeriveUsername` on a
  false import-cycle justification, live on two different auth paths —
  divergence resolves one email to two state dirs
  (`internal/config/config.go:392`). **DONE 2026-09-02** — v1.19.0 deletes
  `usernameFromEmail` entirely; auth paths now converge on `identity.DeriveUsername`.
- [x] 🟡 Project egress deny-list omits `100.64.0.0/10` (Tailscale) and
  `198.18.0.0/15` (`deploy/host-setup.sh:149-206`). **DONE 2026-09-02** — v1.19.0
  adds both CIDR blocks to the DOCKER-USER filter rules.
- [x] 🟡 `STYRMAN_RUNTIME` unvalidated while escape-capable caps are
  granted (`internal/orchestrator/orchestrator.go:759`). **DONE 2026-09-02** — v1.19.0
  adds `config.Load` (line 211) allowlist for VM runtimes (`mvm, kata, kata-runtime,
  kata-qemu, kata-clh, kata-fc`), rejecting all others at startup unless
  `STYRMAN_ALLOW_UNSAFE_RUNTIME=1` is set. ADR-060 records the durable decision.
- [x] 🟡 `formatInPlace` has no test coverage, unlike its analog
  `checkerFor`. **DONE 2026-09-02** — v1.19.0 adds Go tests in `internal/httpapi/format_test.go`
  for `TestFormatInPlace`, `TestFormatAndCheckAgreeOnGo`, `TestSafeSuffixStripsEverythingButAlnum`,
  and `TestFormatScriptReadsAnExactByteCount`. (Separate: the frontend gains 11 vitest tests for
  `projectNameFromRepoURL`, `cloneURLError`, `projectNameError`, and `basename`.)
- [x] 🟡 Add `deploy/caddy/ingress` to `/preflight`'s verification — no
  gate scans it today. (2026-09-01 whole-project audit) **PARTIALLY DONE 2026-09-02** via
  v1.17.3's `deploy/vulncheck.sh` for vulnerability scanning. **FULLY DONE 2026-09-02** via
  v1.19.0's `deploy/verify-commit.sh`, which builds `deploy/caddy/ingress` as a separate
  module in the detached-worktree gate (Go build/vet/race all modules). `/preflight` itself
  remains repo-root-only (as a running-gate, not a pre-tag gate), but commit verification
  now covers ingress. See `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md` and ADR-061.
- [x] 🟢 Dead code: `LastActivity`, `Project.Agent`, `Project.GitInit`.
  **DONE 2026-09-02** — v1.19.0 removes all three. `LastActivity` deleted from orchestrator;
  `Project.Agent` and `Project.GitInit` removed from the API (the create request still accepts
  and ignores them for backward compat). Breaking change → MINOR bump.
- [x] 🟢 Coverage: zero frontend unit tests (8,254 LOC);
  `internal/identity` untested despite owning the tenancy key. **DONE 2026-09-02** — v1.19.0
  adds 11 vitest tests for `projectNameFromRepoURL`, `cloneURLError`, `projectNameError`, and `basename`
  in `web/src/lib/api.test.ts` and `web/src/lib/editorPane.test.ts` (note: draft buffer state deliberately
  untested per file comment); adds Go tests in `internal/httpapi/format_test.go` and `internal/identity_test.go`
  with determinism and drift-fix tests confirming `DeriveUsername` behavior after deleting the duplicative
  `usernameFromEmail` path. Frontend LOC remains 8,254; test coverage established.

- [x] 🟡 Caddy base images pinned by tag only, not digest, and the
  builder's own Go toolchain is unasserted (`deploy/caddy/Dockerfile`) —
  `caddy:2.11.4-builder`/`caddy:2.11.4` still float upstream, so
  "tag → binary" stays non-deterministic without a digest pin; add a
  `go version -m` assertion alongside the existing `caddy list-modules`
  check. **DONE 2026-09-02** — v1.19.0 digest-pins both Caddy base images and asserts
  Go toolchain version via `go version -m` in Dockerfile. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟢 `STYRMAN_UPDATE_STATE_DIR=/etc` or `=/` passes the ADR-052 guard
  (root-owned, mode 755 has no group/world-write bits) — a one-component
  typo off the recommended `/etc/styrman/update` silently writes
  deploy-state files into a shared system dir, and the host then looks
  "never deployed" to the timer. **DONE 2026-09-02** — v1.19.0 config.Load
  now validates that `STYRMAN_UPDATE_STATE_DIR` has at least two components
  and is not in the blacklist (`/`, `/etc`, `/var`, `/tmp`, `/opt`, `/home`).
  Prevents system-dir pollution from typos.
- [x] 🟢 Ingress admin `origins` directive is browser-only defense-in-depth,
  not `/load` authentication. **DONE 2026-09-02** — v1.19.1 tightens the loopback
  guard to refuse all names (including `"localhost"`) and accept only IP literals
  verified via `net.ParseIP` and `IsLoopback()`. `DefaultCaddyAdmin` changed from
  `"localhost:2019"` to `"127.0.0.1:2019"` (IP literal). New test `TestDefaultCaddyAdminPassesTheGuard`
  asserts the shipped default passes the guard. See
  `.agents/docs/changes/2026-09-02-v1.19.1-ingress-loopback-guard.md`.

- [x] Editor/file-explorer UX pass (operator, 2026-09-01: "just some
  optimisation" — current behavior works fine, this is polish, not a fix).
  Highlight the currently-open file in the file explorer; show the open
  file's name at the top of the editor pane; a recent-files affordance.
  SHIPPED 2026-09-01 — see
  `.agents/docs/changes/2026-09-01-editor-ux-and-updater-fixes.md`. The
  fourth item scoped here, a per-project "open until closed" notion (the
  active file stays active across explorer navigation until explicitly
  closed, rather than tracking explorer selection), was not part of what
  shipped — re-raise separately if still wanted.
- [x] 🟡 The updater does not reinstall ITSELF: a release that changes
  `deploy/update/*` deploys the stack but leaves the OLD
  `/usr/local/bin/styrman-update` + units installed (bit a THIRD time in the
  v1.17.2 code-review-fix release — the round-2 `/run/*`-only `DOCKER_CONFIG`
  script had to be hand-installed on the host again). Fix idea: `deploy()`
  installs the checkout's updater script+units after a successful health
  check (takes effect next run). SHIPPED 2026-09-02 (v1.17.3) —
  `self_install()`, cmp-gated atomic rename, one-time manual reinstall
  caveat noted; see
  `.agents/docs/changes/2026-09-02-v1.17.3-updater-self-install-and-vulncheck.md`.
- [x] 🟢 The currently-open file's row in the file explorer
  (`bg-accent-soft`, `isOpen` branch, `FileExplorer.tsx:333-337`) has no
  `hover:` class, unlike every other row. **DONE 2026-09-02** — v1.19.0
  adds the missing `hover:` class. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟢 A create-only (`O_EXCL`) file endpoint so a raw API caller can't
  truncate via `PUT .../file` — **DONE 2026-09-02** — v1.19.0 adds `?create=true`
  query param mapping to O_EXCL, refusing overwrites on existing files. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟢 `chipLabel` disambiguates same-basename collisions one parent level
  only — `x/a/n.txt` and `y/a/n.txt` still render two identical `a/n.txt`
  chips. **DONE 2026-09-02** — v1.19.0 walks up directory hierarchy until
  basename becomes unique (not just one level). See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟢 Move the `"recents list is capped on read"` e2e check
  (`web/e2e/run.mjs`) to the end of the editor section, or restore the real
  `styrman.recentFiles.<project>` localStorage key after it. **DONE 2026-09-02** — v1.19.0
  now restores the real localStorage key after the junk-write test, preventing
  ordering landmines for subsequent tests. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟢 `web/src/lib/api.ts`'s `uploadFiles` bypasses `request()`, so it gets
  neither the 401/OAuth-redirect handling nor the abort timeout. **DONE 2026-09-02** — v1.19.0
  routes `uploadFiles` through `request()` like every other API call, giving it
  proper 401/OAuth handling and abort timeout. See
  `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] 🟢 Run `govulncheck` inside the builder image (or pin the host
  toolchain to match) so the security gate measures what actually ships —
  it currently runs against this host's go1.25.0 while
  `deploy/Dockerfile.server` pins `golang:1.25.13-trixie`; the shipped
  artifact is unaffected (every stdlib fix the host scan surfaces is
  ≤ 1.25.13) but the 31-finding noise floor from the toolchain mismatch is
  where a future real advisory would hide. (v1.17.2 round-3 security delta,
  `.agents/docs/reviews/2026-09-02-security-v1.17.2-fixes.md`) SHIPPED
  2026-09-02 (v1.17.3) — `deploy/vulncheck.sh` runs govulncheck@v1.7.0
  inside the Dockerfile-pinned `golang:1.25.13-trixie` image; baseline is
  zero stdlib findings, confirming the 31 host-toolchain findings were
  artifacts. See
  `.agents/docs/changes/2026-09-02-v1.17.3-updater-self-install-and-vulncheck.md`.
- [ ] 🟢 Bearer-token possession-binding trade, accepted risk recorded in
  ADR-056: `/etc/styrman/gh-token` grants full repo-read from any machine
  that reads it once, with no host-key/client-cert/IP anchor the way the
  SSH deploy key it replaced had. Accepted (expiry + one-click GitHub-side
  revocation outweigh it for a repo-scoped read-only credential) — listed
  here so the trade isn't only visible inside the ADR. (security review
  2026-09-01, `.agents/docs/reviews/2026-09-01-security-no-backups.md`)
- [ ] 🟡 Operator actions from the no-backups/token-auth change: (1) mint
  the fine-grained GitHub token (this repo only, Contents: Read-only) and
  install it at `/etc/styrman/gh-token` (root:root, `600`) — **DONE
  2026-09-01**, and the first real timer-driven auto-deploy has since run
  successfully end-to-end (see
  `.agents/docs/changes/2026-09-01-editor-ux-and-updater-fixes.md`); (2) delete the GCS bucket
  `styrman-crew-backups-driven-eft-h4it` and its backup service account in
  project `driven-eft-h4it` (no `gcloud` on this host — operator-run); (3)
  shred `/var/lib/styrman/backups/` on the host — an orphaned, unencrypted
  archive containing at least one project's home (ssh keys, gcloud ADC,
  claude/agy tokens) with no code path left to list or delete it; no agent
  touches `/var/lib/styrman` (ADR-052).
  (`.agents/docs/changes/2026-09-01-remove-backups-and-token-auth.md`)

- [x] `cloneURLRe` (`internal/httpapi/server.go`) credential-bearing URLs handling. **DONE
  2026-09-02** — **OPTION 1 CHOSEN: reject at the boundary**. Clone URLs with embedded credentials
  (`https://user:token@host/repo`) are now refused with a 400 Bad Request at the API boundary
  (`cloneURLHasSecret` in `internal/httpapi/server.go`), removing the entire class rather than
  trying to redact it. Rationale: git persists the token verbatim into `.git/config` on the host,
  the credential broker makes embedded credentials unnecessary (ADR-030/032 deliver ssh keys and
  answer git-credential-helper), and refusing at the boundary is stronger than redaction. See
  ADR-059 and `.agents/docs/changes/2026-09-02-v1.18.1-project-create-and-secure-clone-urls.md`.
  (security review recommendation, operator decision 2026-09-02)
- [x] **Enforce commit-tree verification in the release path** (process gap, not a code bug).
  **DONE 2026-09-02** — v1.19.0 adds `deploy/verify-commit.sh`, a new pre-tag gate that builds
  the commit in a detached worktree (never the mutable checkout), running Go build/vet/race,
  the vendored ingress module, gofmt, frontend check/lint/test, web/dist freshness rebuild-and-compare,
  and a real server image build. Tag and push follow only a passing gate. ADR-061 records the
  durable decision. See `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [x] A project home (`ProjectHome`) survives a `DELETE` without `wipe=true`, stranding
  materialized ssh keys. **DONE 2026-09-02** — v1.19.0 `projectHome.Delete` without `wipe=true`
  now cleans up materialized SSH keys via `files.Remove` (the same symlink-safe boundary used for
  the cross-tenant deletion fix). See `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md`.
- [ ] There is no `provisioning` state in the UI for the window between the 201 and the clone
  finishing, during which the workspace is briefly not yet a git repo (R13). The window grew slightly
  with the broker wait. (review 2026-09-02)
- [ ] `web/src/components/Sidebar.tsx`'s `deleteError` badge has no automated coverage — the sibling
  convention the new `provisionError` badge was copied from. The new badge got e2e coverage in this
  release; the older one did not. (verifier 2026-09-02)
