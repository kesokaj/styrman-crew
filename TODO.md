# TODO

Empty by design (operator directive 2026-08-30: "I want nothing in todo and
roadmap — start fresh with new things"). The entire previous roadmap was
either SHIPPED in v1.14.0 or RETIRED with reasons in **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) — including the standing
WON'T-DO decisions. History lives in `.agents/docs/changes/` and git.

Add new items below as they arise.

## Inbox

- [x] Show the ssh PUBLIC key in Settings → Credentials, not just the path `~/.ssh/id_ed25519` (operator, 2026-09-10: "I like the ssh.pub key to be visible in the settings menu. That is handy."). The setup page's tool status already reads and echoes it (`internal/httpapi/setup.go`, the `ssh` toolInfo's `Details`, symlink-refusing); the settings row's `detail` is a constant string. Small frontend task once the same detail is exposed on the settings side. (added 2026-09-10) **DONE 2026-09-10** — frontend-only, see this entry.

- [ ] Converge the eight shared dependencies that still diverge between the root module and `deploy/caddy/ingress` (`go-connections`, `httpsnoop`, `go-logr/logr`, the three OpenTelemetry packages, and two more — `deploy/verify-commit.sh` prints the full list on every run). `docker/docker` was converged on 2026-09-08 and is the one the gate now BLOCKS on; the rest are warnings because turning them all into a release blocker would force an untested eight-package bump. The ingress is the internet-facing component (`network_mode: host`, docker socket mounted), so it is the last place that should sit on older versions. (added 2026-09-08, from the project audit)

- [x] `web/e2e/run.sh`'s exit code is unreliable: observed `1` twice while the suite itself reported `passed=89 failed=0`, and not reproducible on demand. **DONE 2026-09-09** — root-caused and fixed, and the two symptoms turned out to be one cause. `set -e` applies inside an EXIT trap, so `env=$(docker inspect "$c" ...)` failing on a container that vanished between `docker ps -aq` and the loop body aborted `cleanup` mid-loop, before its `rm -rf` — hence both the leftover `/tmp/styrman-e2e-*` dirs AND an exit status of 1 after a green run. The earlier note ruled the trap out because `cleanup`'s last command returns 0; true, but the trap never reached it. Fixed with `|| continue` on the assignment plus an explicit `exit "${status}"`. See BUG-024. (added 2026-09-08)

- [ ] `web/dist/client/_shell.html` churns on every SPA build: TanStack's prerenderer embeds a wall-clock timestamp in the router payload (`u:1788958111974` -> `u:1788965694748`), so any `web/e2e/run.sh` run leaves the tree dirty with a one-line diff carrying no information — an easy thing to sweep into an unrelated commit. Visible at all only because `.gitattributes` now forces a textual diff. Decide whether to strip it post-build or gitignore the churn. (added 2026-09-09)

- [x] 🟡 The network-mode PUT's rollback should not exist for the direction that matters. The v1.25.0 preflight security review (`reviews/2026-09-09-preflight-security-v1.25.0-round2.md`) found the last residual fail-open: on a LAN→sandbox revocation the record is written FIRST and rolled back on failure, so a store write failing between the two writes leaves the record saying sandbox with LAN in force (ERROR-logged; needs a local JSON write to fail right after one succeeded). Three review rounds landed on this mechanism in one day (BUG-028/029/031), which AGENTS.md says is the signal to delete it rather than defend it: on TIGHTENING, persist only after the rebuild succeeds; keep write-first only for loosening, where a failure over-claims against a container that has less. Shorter code, no rollback, class closed. Security round 3 (`...-round3.md`, medium) adds the consequence: because the PUT's no-op short-circuit trusts the record, a wrong record cannot be re-applied by re-issuing the PUT — the self-service recovery today is **Stop, then Start**, which applies the record through the stopped-container mismatch heal; the same restructure should make PUT compare against the LIVE bridge name rather than the record so a re-PUT is itself the recovery. Not done on release eve because it restructures the handler and needs its own review. (added 2026-09-09) **DONE 2026-09-10** — restructured as described (`.agents/docs/changes/2026-09-10-network-mode-put-locked.md`); rollback deleted, live-bridge no-op, whole decision under the project lock.

- [x] 🟡 A mode toggle races a concurrent start (same review, medium): `RunOpts.Lan` is a caller snapshot of the record read outside the orchestrator lock, so a `POST /start` that read the OLD record can bring the container up on the old bridge after the toggle's `Status` said "stopped" — and running containers are never re-checked. Needs a very tight race. Fix sketch: have `EnsureRunning` (or the start handler, under `netMu`) re-read the record at the moment it decides the bridge name, so the name always reflects the record as of the start. (added 2026-09-09) **DONE 2026-09-10** — `RunOpts.Lan` is a read under the project lock and the PUT runs under the same lock; security round 3 confirms every interleaving closed.

- [x] 🟢 `waitReady` timeout after a successful `ContainerStart` inside `ensureRunningLocked` leaves the "deliberately stopped" marker set on a container that may be running (reviewer round 2, low, pre-existing since BUG-030's fix) — clear the marker after `ContainerStart` succeeds, or re-inspect on that branch. (added 2026-09-10) **DONE 2026-09-10** — the marker is lifted right after the start, before the readiness wait; see this entry.

- [ ] 🟢 The network-mode handler writes its HTTP reply while holding the project lock (security round 3, low/low, latent: bodies are tiny) — collect the reply inside the closure and write after `WithProject` returns. (added 2026-09-10)

- [ ] 🟢 `publicKeyLine` in `internal/httpapi/setup.go` is untested at its call site — mutating the gate to `line != ""` leaves `go test ./internal/httpapi/` green; `TestPublicKeyLine` covers the function, not `toolDetails`. Add a `toolDetails` test under a `files.Trust` base with a `.pub` that is a symlink, a hard link, and a copy of the private key, asserting `Details` is empty. (release review `reviews/2026-09-10-review-v1.26.0-delta.md` round 3, low) (added 2026-09-10)

- [ ] 🟢 `publicKeyLine` key type validation needs tightening — tests only match prefixes like `ssh-`, `sk-`, so `ssh- AAAA`, `sk- A`, or an API-token-shaped `sk-ant-…` line would be accepted and rendered as the user's public key (their own file, so cosmetic; the frontend applies the same shape). Require a full type token: `ssh-(ed25519|rsa|dss)`, `ecdsa-sha2-<curve>`, `sk-<type>@openssh.com`. (same review, low) (added 2026-09-10)

- [ ] 🟢 `web/src/lib/sshKey.ts` rejects `\p{Cc}` (plus U+2028/2029) but not the rest of `\p{C}`: U+202E RLO, ZWSP, BOM, U+00AD and NBSP still reach the row's title attribute and the clipboard, and `abbreviateKey` does not collapse RLO — display spoofing of the user's OWN row only. Widen to `\p{C}` (keeping TAB) and collapse Unicode spaces in `abbreviateKey`. (security review `reviews/2026-09-10-security-ssh-pubkey-settings.md` round 3, low; added 2026-09-10)

- [ ] 🟢 `publicKeyLine` in `internal/httpapi/setup.go` validates the type token and the base64 body but not the comment field, so control bytes in a `.pub` comment are serialised into the `/api/settings/tools` JSON; the browser is the only filter for any other API consumer. Strip or reject control characters in the comment server-side (`stripControls` in `server.go` exists for titles). (security review `reviews/2026-09-10-security-ssh-pubkey-settings.md` round 3, low; added 2026-09-10)

- [ ] 🟢 `unmarkIfStillRunning` (`internal/orchestrator/orchestrator.go`, BUG-030's guard), two lows from the v1.25.0 preflight round 4 (`reviews/2026-09-09-preflight-review-v1.25.0-round4.md`): (a) one `State.Running` sample is not proof the stop is not in flight — a deadline firing mid-`ContainerStop` leaves the daemon stopping, the inspect sees Running, the marker clears, and the tenant's own next reconnect can resurrect the VM (no access gain; narrow). Require two Running samples ~2 s apart. (b) the inspect-error and not-running branches are silent, so when the marker is kept an operator sees BUG-030's 4002 symptom with nothing in the log — one `Warn`. Both post-freeze on purpose: four review rounds on this path in one day. (added 2026-09-09)

- [x] 🟢 Vulnerability scan status at v1.25.0: the authoritative `bash deploy/vulncheck.sh` (both modules, pinned builder) exits 3 on the two known `github.com/docker/docker` advisories GO-2026-4887 / GO-2026-4883 — `Fixed in: N/A`, daemon-side defects reached through the client SDK's init, unchanged since v1.24.1. A security review's ad-hoc `govulncheck ./...` on the host reported "31 called Go-1.25 stdlib vulns"; the final release verifier could NOT reproduce that (zero stdlib entries in the authoritative scan), so that figure is unconfirmed — re-run on the next builder bump and settle it rather than cite it. (added 2026-09-09, from the v1.25.0 preflight) **STRUCK 2026-09-10** — deploy/vulncheck.sh runs at every release and reports the two docker/docker advisories itself; no fix exists upstream (v28.5.2 is the newest); a TODO adds nothing the gate does not already say. Merged with the older govulncheck item below.

- [ ] The pinned `styrman-runtime:test` tag that both suites default to has no refresh step and silently ages into false failures that read as source regressions — rebuilt by hand on 2026-09-09 after it cost three integration checks; decide whether the suites should assert the tag's age or rebuild it as a prereq. See BUG-026. (added 2026-09-09)

- [x] Re-run the firewall's base insert-if-missing pass at the END of the LAN section when that section changed the chain, so a base rule removed by a concurrent root writer is repaired within the same run rather than within 60 s. Suggested by the Phase 1 security review (`reviews/2026-09-08-security-phase1-refactor.md`, finding 2) and deliberately not done there: it is independent of ADR-066 and wants the base block extracted into a function first. (added 2026-09-08) **STRUCK 2026-09-10** — ADR-067 deleted the LAN section; the script has no per-project section left to re-run after.

- [x] The opt-in live LAN block in `integration.sh` (`STYRMAN_IT_LIVE_LAN=1`) races the production server's 2-minute marker sweep. **MOOT/REMOVED 2026-09-09** — ADR-067 deleted that block and the marker machinery it raced entirely. (added 2026-09-08)
- [x] Bump `STYRMAN_FW_VERSION` in `deploy/firewall/styrman-firewall.sh` whenever the script changes materially. **MOOT 2026-09-09** — ADR-067 deleted the version handshake entirely. (added 2026-09-08)
- [x] `web/dist/client/_shell.html` carries a raw NUL at offset 7417 and git treats it as binary, so a reviewer cannot read it; pre-existing generated SPA build output, not BUG-007. **DONE 2026-09-09** — resolved by `.gitattributes` forcing a textual diff; the generator output bytes are left alone deliberately, but a reviewer can now read the file. (added 2026-09-08)

- [x] 🟢 ADR-065 follow-ups (recorded, not built — plan
  `.agents/docs/plans/2026-09-08-per-project-lan-access.md`): (a) a
  `lanPolicy = admins|owners|off` operator knob via `operatorOverrides` /
  `PATCH /api/settings/runtime` (~30 lines) if a tenant ever needs to
  self-serve; (b) `LAN_ALLOW_HOST` / `LAN_ALLOW_PUBLISHED_PORTS` conf keys
  (reserved, parsed as 0) each need their own ADR-029 amendment; **RESOLVED 2026-09-08 in ADR-066 Phase 3** — a published-
  port rule was recorded here as needing `! -o sdb-+ ! -o <ingress br>` — **that shape is invalid and cannot be written**: iptables 1.8.11 (nf_tables) refuses a second `--out-interface` (`multiple --out-interface options not allowed`, probed live 2026-09-08). The replacement is a positive destination allowlist, specified in `plans/2026-09-08-lan-full-parity.md`; (c) per-project LAN
  DNS (guest resolvers are pinned to 8.8.8.8/1.1.1.1 — access is by IP);
  (d) pin `default-address-pools` to 172.16.0.0/12 in `daemon.json` so a
  16th project bridge can never land in 192.168/16 (today the conf
  validator's overlap refusal fails closed); (e) verify Docker 29.7 +
  live-restore keeps `DOCKER-USER`/`STYRMAN-*` across a daemon restart
  (exposure today: ≤65 s LAN-closed, never open); (f) run the base section
  `Before=docker.service` to close the pre-existing ≤15 s boot window
  (unverified whether Docker keeps a pre-created `DOCKER-USER`); (g) **RESOLVED 2026-09-08 by ADR-066's latch amendment** — the
  transitive path "LAN-enabled project → a LAN host that trusts its key →
  back to this host" is closed for the SSH key: a project that has ever
  had LAN access is permanently no longer served it. **STRUCK 2026-09-10** — strike the WHOLE item but state per letter in the reason: (a) self-serve `lanPolicy` knob — not requested, enable stays admin-only; (b) resolved 2026-09-08; (c) LAN hostname resolution inside a LAN project — operator declined (access is by IP); (d) and (f) are FIXED in plan `.agents/docs/plans/2026-09-10-todo-closeout.md` phase 3 (address-pool pin, firewall unit ordering); (e) daemon-restart live-restore check — declined: it would restart production's Docker; (g) resolved 2026-09-08.

- [x] 🟢 Coverage tracing for v1.22.0 is incomplete (release gate, 2026-09-04). The verifier ran every command green but did not finish tracing each changed file to a covering test; the largest untraced surface is `web/src/components/SidePanelGit.tsx` (351 changed lines), plus `deploy/verify-commit.sh`'s new CSS-origin gate step, `image/skel/*`, and `integration.sh`'s own additions. These are UNKNOWN, not established failures — `SidePanelGit.tsx` is exercised by the e2e git checks but not line-for-line. Per AGENTS.md a changed file with no covering test is a FAIL row, so this needs closing out rather than assuming. **STRUCK 2026-09-10** — four release gates (v1.23–v1.26) have since verified the whole tree with per-file tracing where it mattered; the v1.22.0 surfaces are exercised by the e2e git checks; line-for-line tracing of a shipped release is not a standard the project holds.
- [x] The six git HTTP handlers added in v1.22.0 have NO Go unit test. **DONE 2026-09-04**: `internal/httpapi` gained a narrow `guestRuntime` interface (`Status` + `Exec`), set from the orchestrator in `New()`, with 15 call sites routed through it and 26 handler tests in `guestexec_test.go`. The argv IS the security decision in these handlers (`add -A` not `commit -a`, `--end-of-options` on the switch path, `-u` on stash push, no `--prune` on fetch) and none of it could be asserted before — a comment claiming an invariant is not a test. **Correction to this item's own claim:** the Exec seam closes this and the `checkFile`/`formatFile` item, but NOT BUG-003's residual, which needs a fake DOCKER client — a different seam entirely. Two of three, not three.

- [ ] 🟡 BUG-003's residual, restated at its real size and NOT closed by the `guestRuntime` seam: `EnsureRunning` has no unit test because `internal/orchestrator` has no fake Docker client. `o.docker` is a concrete `*client.Client` reached through 19 distinct methods (ContainerCreate/Start/Inspect/List/Remove/Stop, four Exec calls, six Network calls, ImageInspect, Ping, ContainerLogs), and `EnsureRunning` alone is 166 lines. Closing it means a `dockerAPI` interface over at least the subset EnsureRunning touches, plus a fake — its own task, roughly the size of the seam just done. Worth doing: what becomes testable is the symlink check on the bind source (BUG-002/003), the guard that keeps one tenant's home out of another's mount. (sized 2026-09-04)
- [x] 🟢 A transient 404 tripped the e2e client-error gate once during teardown (a request outliving a deleted project) and did NOT reproduce; never root-caused. The gate now records the failing URL alongside the browser's message, so a recurrence is diagnosable. (v1.22.0, 2026-09-04) **STRUCK 2026-09-10** — never reproduced across every e2e run since 2026-09-04 (86 checks green on 2026-09-10); the gate now records the URL, so a recurrence is diagnosable; nothing to build until then.

- [x] Bug (operator, 2026-09-03): after some minutes the bottom stats bar and the sidebar show "fail to fetch"; a browser F5 fixes it until it recurs. **DONE 2026-09-03** — BUG-001: the ingress session token (caddy-security default lifetime 900 s) expires, the `/api/*` redirect to the auth subdomain is cross-origin, and the SPA's `fetch()` rejected before the existing re-auth handling could see it. `request()` now uses `redirect: "manual"` and treats an opaque redirect as an expired session (one toast, one reload). See `.agents/docs/bugs/BUG-001-fail-to-fetch-session-expiry.md` and `.agents/docs/changes/2026-09-03-fail-to-fetch-session-expiry.md`. Not yet released: ships with the next tag.
- [x] Operator decision — session token lifetime. **DONE 2026-09-04** (approved: "fix then commit to the newest version"): `crypto default token lifetime 28800` in the portal block, ADR-063, generator test; ships in v1.20.0. Original note: The ingress uses caddy-security's default `token lifetime` of 900 s with nothing set in `deploy/caddy/ingress/generator.go` `writeSecurityBlock`, so after BUG-001 the SPA now reloads itself every 15 minutes of use (a toast, then `location.reload()`; the terminal reconnects, in-memory UI state is lost). The security review of BUG-001 (`.agents/docs/reviews/2026-09-03-security-fail-to-fetch.md`, observation) reasons that the lifetime is not the revocation control here — `authGuard` re-checks the userstore on every request, so removing a user takes effect at once regardless — and recommends setting it deliberately: `crypto default token lifetime 28800` (a working day) in the `authentication portal` block, recorded in the deploy spec. Trade: a stolen cookie stays valid that long, zone-wide (`cookie domain`). One line plus a generator test; do it with the operator's yes. (added 2026-09-03)
- [x] 🟢 `web/src/lib/api.ts` interpolates terminal `id` and link `token` into API paths without `encodeURIComponent` (**partially addressed 2026-09-04, v1.22.0**: the GIT endpoints now encode `project`, new and pre-existing methods alike, because phase 6 rewrote that block anyway). **DONE 2026-09-09** — the `u` tagged template now encodes every interpolated value including the /ws terminal path. Covered by `web/src/lib/api.encoding.test.ts`, 50 cases written against call sites (not the tag alone), mutation-checked, asserts on path segments not substrings. (security review 2026-09-03, low)
- [x] 🟢 Ingress: `trust login redirect uri domain suffix {BaseDomain}` (`generator.go:260`) trusts the whole DNS zone as a post-login `redirect_url` target, including the public-preview wildcard a project owner controls; the practical ceiling is a trusted-looking landing page (cookies and identity headers are stripped before a guest app sees them). Narrow the suffix to the app host and confirm caddy-security's matcher has a label boundary. Pre-existing, not widened by BUG-001. (security review 2026-09-03, low/low) **STRUCK 2026-09-10** — operator decision after the plan's cold-reader check (`reviews/2026-09-10-coldread-todo-closeout.md`, rounds 2–3): with `APPS_DOMAIN` unset, `AppsDomain == BaseDomain` and the shell itself is served as an app under it, so no redirect-trust list can keep login working and exclude the public-preview wildcard; the "landing-page ceiling" (cookies and identity headers stripped before a guest app) is the accepted residual.
- [ ] 🟢 WebSocket sessions are authorized at the handshake only (`internal/terminal/bridge.go`): a removed user's already-open terminal tab keeps streaming until the socket drops. BUG-001's reload used to re-handshake every 15 minutes by accident; with the eight-hour token (ADR-063) that incidental bound is gone. Fix idea: close a user's sockets when the userstore removes them, or re-check authorization on a timer. (security review 2026-09-04, low)
- [x] 🟢 The ingress generator's Caddyfile is asserted by substring only; nothing in `deploy/verify-commit.sh` runs `caddy validate`/`adapt` on a generated config, and a config the plugin rejects on cold boot serves nothing. The 2026-09-04 lifetime change was validated by hand against `styrman-caddy:1.19.1` (`testing/2026-09-04-verify-session-lifetime.md`); wire that into the gate (build the caddy image, generate for one authenticated app, `caddy validate`). (security review 2026-09-04, low) **STRUCK 2026-09-10** — operator decision, same review: the generated config resolves `{env.*}` secrets at load and the ingress module has no generator entry point, so a `caddy validate` gate step would validate a config filled with dummy values at the cost of a Caddy image build per gate run; the manual validation precedent (`testing/2026-09-04-verify-session-lifetime.md`) stands.
- [ ] 🟢 `cmd/styrman/main.go` `migrateWorkspaces` (the pre-ADR-027 workspace move, `UserHome/projects/<dir>` → `StateDir/projects/<user>/<uuid>`) resolves the source with `EvalSymlinks` and then renames by path; since ADR-064 that source sits under the setup VM's live mount (`base/`), so the same check-then-rename shape BUG-003 fixed elsewhere remains here. It only fires for a legacy workspace, of which this host has none (all migrated at v1.19.x). Confine it (rename through an `os.Root` at the tenant dir, or refuse when the source has a symlink component) or delete the legacy path once no deployment can carry one. (security review of v1.21.0, 2026-09-04, medium/medium, not a regression)
- [ ] 🟢 `internal/httpapi/git.go:73` reads the user's `.gitconfig` from the setup VM's home with `git config --file <base>/.gitconfig` — a plain path read, not confined like the `files.*` reads around it; the file is the user's own, so a planted symlink there leaks nothing the user cannot already read, but it is the one base-home read outside `internal/files`. Route it through a confined read. (security review of ADR-064 Phase 1, 2026-09-04, low)
- [x] 🟢 Accepted lows from the harness-replacement security reviews (2026-09-03), listed so they are not lost: `files.Trust` is registered in `httpapi.New`, so a `files.*` caller that runs before `New` (none today) would get plain `os.OpenRoot` semantics — register at config load if one appears; `harness-migrate.sh` moves through a symlinked `~/.styrman` if the user planted one (their own blast radius, as the project user); `sh-agent update` ships in the tenant binary but is inert against the `.git`-less mirror (upstream could gate it). (`reviews/2026-09-04-security-v1.20.0.md`) **STRUCK 2026-09-10** — accepted and recorded in `reviews/2026-09-04-security-v1.20.0.md`; nothing to build.
- [x] 🟡 ADR proposal for the operator — un-nest the project homes. ADR-031 put every project home at `HomesDir/<user>/p/<uuid>`, INSIDE the base home the setup VM bind-mounts with a root guest, so `p` and `p/<uuid>` are guest-replaceable. Three code paths trusted that path and were fixed defensively on 2026-09-03 (BUG-002 `files.Trust`, BUG-003 `realBelow` before the bind and the confined janitor), but the orchestrator's check is not atomic with `docker create`: a guest racing it from a running setup VM is a residual window. The durable fix is a layout where no tenant-owned tree sits under a directory a guest can write — e.g. `HomesDir/<user>/base` for the setup VM's home and `HomesDir/<user>/p/<uuid>` for projects with only `base` mounted, or `HomesDir/p/<user>/<uuid>`. Needs a migration of existing homes (rename on the host while nothing runs) and an ADR; the operator's call. (security review 2026-09-03, rounds 2–3) **STRUCK 2026-09-10** — DONE by ADR-064 (v1.21.0): the setup VM mounts `homes/<u>/base`, the tenant dir is mounted into no guest; the layout this item asked for.
- [x] 🟢 Upstream (`styrman-harness`, the operator's repo — styrman never edits the mirror): (1) `sh-agent` turns frontmatter `name:` values and manifest paths into filesystem paths with no `filepath.IsLocal` check (`internal/harness/plan.go`, `apply.go`) — harmless in a project VM (the installer runs as the project user in their own home) but worth closing at the source; (2) `sh-agent version` prints `(devel)` and the manifest's `commit` is empty when the repo has no `.git` (the image's mirror) — a `-ldflags -X` version or a `--commit` flag would let the image stamp `image/harness/COMMIT` into the manifest; (3) `git rev-parse` in `repo.go` ascends out of `--repo` when the mirror sits inside another git checkout (on this host it recorded styrman-crew's own commit as the harness commit — cosmetic, and never in the image, which has no `.git` above `/styrman/harness`); (4) `Install` in `apply.go` returns on the first write error, so a `chmod`'ed directory in the home leaves a partial install repeating every boot — continue and report, as `harness-migrate.sh` now does. Next mirror refresh picks these up. (security review of the harness replacement, 2026-09-03, low) **STRUCK 2026-09-10** — belongs to the `styrman-harness` repo; the operator carries the four items there; styrman never edits the mirror.

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
- [x] `checkFile`/`formatFile` have no Go handler-level test. **DONE
  2026-09-04** via the same `guestRuntime` seam: `guestexec_test.go` covers
  the format round trip (content reaches the guest on STDIN, never
  interpolated into the shell script), the `STYRMAN_OK`/`STYRMAN_ERR`
  sentinel that separates formatted output from a formatter's error message
  (422 carrying the formatter's own line), an unsupported extension refused
  without touching the guest, and check's `supported:false` path. Original
  note: only e2e plus `TestGuestScriptsNeverWaitForStdinEOF` exercised them.
  (flagged by the verifier 2026-09-01)
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

- [x] `govulncheck ./...` reports 2 CALLED vulnerabilities, both in
  `github.com/docker/docker@v28.5.2+incompatible` (GO-2026-4887, GO-2026-4883),
  both **Fixed in: N/A** and both daemon-side, flagged only through `init`
  reachability. Pre-existing, no upgrade available — recheck when docker
  publishes a fix. (security review 2026-09-01) **STRUCK 2026-09-10** — same two advisories as the v1.25.0 scan item; tracked by `deploy/vulncheck.sh` at every release; no upstream fix (checked 2026-09-10: v28.5.2 newest).

- [ ] Carried-over low findings from the 2026-09-01 re-review of the
  syntax-check/auto-updater remediation
  (`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-rereview.md`),
  none re-argued, all still open: `deploy/update/styrman-update.timer`'s
  `Persistent=true` is inert on a monotonic (`OnBootSec`/`OnUnitActiveSec`)
  timer; `deploy/host-setup.sh` step 8c is still physically ordered before
  step 8b (**already closed** — step 8b was removed entirely, verified 2026-09-10); `deploy()` calls `fail` (which exits) on a checkout error instead
  of `return 1` like the build/health paths, so that one path still bypasses
  the failed-tag latch (**already closed** — it does `log …; return 1`, `styrman-update:560`, verified 2026-09-10); `mktemp -p .` in both `check.go` and `format.go`
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
  adds both CIDR blocks to the DOCKER-USER filter rules. **Correction
  2026-09-08:** the REPO change shipped; production never received it —
  nothing delivered host-setup changes, and the live host ran the four-DROP
  script of 2026-08-30 until the ADR-065 work found it. Converged by the
  operator's `deploy/firewall/install.sh` run; `host_converge` in the updater
  closes the delivery gap for good.
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
- [x] 🟢 Bearer-token possession-binding trade, accepted risk recorded in
  ADR-056: `/etc/styrman/gh-token` grants full repo-read from any machine
  that reads it once, with no host-key/client-cert/IP anchor the way the
  SSH deploy key it replaced had. Accepted (expiry + one-click GitHub-side
  revocation outweigh it for a repo-scoped read-only credential) — listed
  here so the trade isn't only visible inside the ADR. (security review
  2026-09-01, `.agents/docs/reviews/2026-09-01-security-no-backups.md`) **STRUCK 2026-09-10** — recorded in ADR-056; a TODO line adds nothing.
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
