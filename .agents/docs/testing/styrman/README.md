# Testing styrman

Local-first: everything runs on this machine against real Docker + Kata.
No mocks in the orchestrator path.

## Tiers

1. **Unit** — `go build ./...`, `go vet ./...`, `go test -race ./...` (store, validation; fast, no Docker). The vendored Caddy ingress is a SEPARATE Go module and is not covered by the root `go test ./...` run — test it explicitly: `(cd deploy/caddy/ingress && go test ./...)`. `internal/orchestrator` now carries its own unit tests (TODO close-out 2026-09-10, item O) through a call-recording fake Docker client (`fakedocker_test.go`, `dockerAPI` interface, following `guestexec_test.go`'s fake style) with a scriptable per-method error and an in-memory container/network table — `ensureRunningLocked` (wrong-bridge recreate, network-inspect refusal, symlinked-home refusal, a running container left alone), the `WithProject`+handler stop/persist/start ordering, `stopMarked`+`unmarkIfStillRunning`'s two-sample marker logic, `Status`, and `currentBridge`, all without a real Docker daemon. New with the 2026-09-13 audit-fix batch (plan `.agents/docs/plans/2026-09-13-audit-fixes.md`): `TestRecreateMissingNetworkRecreatesContainer` (orchestrator — a stopped container whose network vanished is recreated, never reconnected, H1); `internal/credbroker/snippet_test.go` (the `zz-styrman-credentials.sh` snippet has no `/16`-recomputation, references `${STYRMAN_CRED_BROKER}` and the `broker-unreachable` marker path, `bash -n`s clean); `internal/config/styrman_update_test.go` (`__verify-tag` test-hook fixtures — three tags with placeholder armor blocks refused, one honestly SSH-signed tag passed, an empty `allowed_signers` refused; skips without `ssh-keygen`/`gpg.format=ssh`); config's `STYRMAN_DOCKER_DISK_GB` bound (fatal at boot out of range; a persisted-overrides value out of range WARNs and is ignored instead); `internal/terminal/origin_test.go` (`SameOriginOrAbsent` as the first statement of both `ServeHTTP` and `ServeSetup`, a cross-origin `Origin` refused with 403 before any orchestrator call, no panic with a nil-orchestrator bridge); `internal/httpapi/{projectenv,stats}_test.go` (handler round-trips through the guest exec seam); `internal/httpapi/wsboot_attach_test.go` and `internal/terminal/ensure_before_upgrade_test.go` (added 2026-09-14, BUG-060: `wsBoot` calls `EnsureRunning` then `EnsureSelfAttached`, in that order, before the WS bridge's boot callback returns, so the terminal's login shell finds the credential broker reachable on the project's bridge; a failed boot never attaches; the bridge's `ServeHTTP` reaches the callback and reports its error as a plain 500 with no `Upgrade` header, proving the attach happens before the socket is upgraded); `format_test.go` (the beside-the-file placement, `realpath` confinement, `.git/info/exclude` line, `--ignore-path /dev/null`, the `STYRMAN_FMT_DIR` handler validation and its 400 on a climbing path); `internal/config/ipforward_test.go` (added 2026-09-14, v1.33.0 Phase 2: `TestIPForwardOffReadsTheSysctl`, `TestIPForwardOffReportsAMissingFile` — a containerized server reads `/proc/sys/net/ipv4/ip_forward` at boot and refuses to start when it isn't `0`, since it joins every project bridge and forwarding-on would violate R12; `STYRMAN_ALLOW_IP_FORWARD=1` runs anyway with a Warn, and since 2026-09-22 EVERY read error refuses unless the knob is set — a missing sysctl file included, where it previously warned and continued; `TestIPForwardMustRefuse` covers all eight combinations of reading, error and override, including an error arriving alongside `off=true`); `internal/orchestrator/selfattach_test.go` (added 2026-09-14, v1.33.0 Phase 2, against the fake Docker client's subnet-aware network model: `TestEnsureSelfAttachedBindsTheBrokerAddressOnTheBridge` — the broker binds the bridge's real subnet broadcast−1, `TestEnsureSelfAttachedMemoizesAFreshAttach`, `TestEnsureSelfAttachedFallsBackToADynamicAddress`, `TestEnsureSelfAttachedDoesNotMemoizeAFailure`); `internal/httpapi/ensurefor_test.go` (added 2026-09-14, v1.33.0 Phase 2: `TestEnsureForRefusesAnUnknownProjectBeforeBooting`, `TestEnsureForRefusesAProjectBeingDeleted`); `internal/config/gcp_scripts_test.go` (added 2026-09-21, GCP Compute Engine target, ADR-075, 11 tests: `TestGCPVMCreateNeedsYesWithoutTerminal`, `TestGCPVMCreatePlan`, `TestGCPVMDeleteRefusesWithoutConfirm`, `TestGCPVMRejectsBadNames`, `TestGCPInstallPlan`, `TestGCPInstallCloneHelperShape`, `TestGCPInstallRefusesLooseEnvFile`, `TestGCPInstallNeedsRunningVM`, `TestGCPInstallNeedsKVM`, `TestGCPVMStatusExitCodes`, `TestGCPSSHUnreachableIsNamed` — `deploy/gcp/vm.sh`'s and `deploy/gcp/install.sh`'s `--dry-run` plans driven against a fake `gcloud`/`git`/`curl` on `PATH`, asserting a planted secret sentinel never appears in the output); `internal/config/styrman_ctl_test.go` gained `TestStyrmanCtlInstallExportsTokenForHostSetup` (same date: `install --gh-token-file F` exports `GH_TOKEN` from that file into its own process before running `host-setup.sh`, never on a command line). Not covered by any of these: the real `gcloud`/IAP network path and the fresh-host Kata `.tar.zst` download — both need a real GCP project and a real VM; see `.agents/docs/deploy/gcp.md` "Verify" for the hand-run checks. New 2026-10-02 (ADR-077/078/079, BUG ids omitted): `TestEntrypointInstallsHarnessAfterReady` (`internal/config/entrypoint_seed_order_test.go` — pins the entrypoint's TEXT only; the order in a booted microVM is Tier 2's evidence), `TestClassifyControl`, `TestApplyControl` (the read loop's handling of a text frame: a ping is answered and never calls the idle reaper's touch; a resize is activity and resizes) and `TestApplyControlReturnsTheReplyError` (all in `internal/terminal/control_test.go`), `internal/config/styrman_update_status_test.go` (ADR-081, 15 tests, all `TestStyrmanUpdateStatus*`, run in a scratch working directory, failing by name when TMPDIR's own ancestors are writable by others: through the gated `__write-status`/`__exit-status` hooks, the written line's six validated keys, a relative, `..` or symlinked path refused, an ancestor others can write refused unless sticky, a character outside `[A-Za-z0-9._/-]` refused under a UTF-8 locale as under C, the temp-dir helper's own bar checked against owners as well as modes, values outside their sets written as `failing`/`error` and `""`, `since` carried across a failing streak and cleared by ok, the directory trust rules with a symlink and group-writable plants, the exit decision; the real non-root refusal writing nothing; and TEXT pins — the trap above the `DOCKER_CONFIG` case with every global assigned above it, arming below the lock and not under `--check`, each code site, the unit's and the compose file's path — which are NOT evidence of a real root tick), `internal/updatestatus/updatestatus_test.go` and `internal/httpapi/settings_updater_test.go` (the reader's states, strict decode, staleness at its boundary; the `updater` key for admins only), `internal/orchestrator/ports_lifecycle_test.go` (8 tests) and `internal/httpapi/browser_goingaway_test.go` (BUG-088: an exec that fails while a stop or a mode toggle is in flight is no ports / browser not running, not a 500 — the check waits for the project's lock and asks again; the check asks Docker on every tick and answers as soon as the container is down, without waiting for the lock; under a holder that leaves the guest as it was, or with nothing in flight, the failure is still reported; the wait ends with the request, and at 20 s at the latest; requests parked on one project share one wait, so ten of them ask Docker as often as one), `TestStyrmanCtlInstallOptionalKeyAbsent` and `TestStyrmanCtlInstallOptionalKeyInvalidStops` (`internal/config/styrman_ctl_optional_fill_test.go` — the optional `CLOUDFLARE_API_TOKEN` in `install --env-file`); in the ingress module, `TestReloadCaddyFailedLoadBehindA200IsAnError`, `TestReloadCaddyWarningsAloneAreSuccess` and `TestLoadErrorReadsOnlyCaddysErrorObject` (`deploy/caddy/ingress/watcher_masked_load_test.go` — `reloadCaddy` reads `/load`'s response body and treats caddy's `{"error": …}` object as a failed load whatever the status; adapter warnings alone are success). `cmd/wsprobe` (the WS client Tier 2 drives) has a new flag `-control <json>`: it sends one text frame after connecting, before any `-send` input, and prints a text frame it receives as `[text:<data>]`. Not covered by any suite: the downloads of `agy update` and `claude update` — a hand run on 2026-10-02 inside a real booted project of the rebuilt image took agy 1.2.2 to 1.2.14 in `/opt/agy` and claude 2.1.270 to 2.1.287 in `~/.npm-global`; a new login shell ran 2.1.287 and `claude doctor` printed "No installation issues found". New 2026-10-05 (ADR-083 per-project resources, BUG-089, BUG-090, ADR-084): `internal/orchestrator/resources_test.go` (host ceilings, `WantSizing`, the lazy `docker info` probe, `LiveSizing` without the project lock), `resources_start_test.go` (create sizing, the resize recreate of a stopped container, the disk kept across recreates, refusals without a size or a host), `destroy_zombie_test.go` (`forceRemove` waits out a VM still exiting in `Destroy`, `DestroySetup`, the setup reaper and the pre-ADR-027 recreate), `internal/httpapi/resources_test.go` (GET/PUT validation, isolation, live-vs-record decisions, failure branches), `internal/project/store_resources_test.go`, `internal/config/removed_env_test.go`, `internal/config/kata_config_script_test.go` (runs `deploy/lib/kata-config-test.sh`, 39 checks) and `internal/config/compose_oom_test.go` (server and Caddy at `oom_score_adj -999` with `mem_limit` 4g / 1g, ADR-085), `internal/config/docker_oom_script_test.go` (runs `deploy/lib/docker-oom-test.sh`, 20 checks: the dockerd OOM drop-in) and `internal/gitrepo/oom_test.go` (`TestGitResetsItsChildsOOMScore` drives `git()` through a fake `git` on PATH: the child runs at 0 while the caller is at 300). New in v1.42.2: `internal/httpapi/history_search_limit_test.go` (history search past `searchConcurrency` = 4 in flight is 429), `TestReapSetupVMsStopsWhenThePassIsOutOfTime`, and `GOMEMLIMIT=3GiB` pinned in `compose_oom_test.go`. Frontend: `cd web && npm run check` (tsc -b), `npm run lint` (oxlint), and `npm test` (`vitest run` — added 2026-09-02; `web` previously had no unit-test framework at all) — 178 tests across 15 files (2026-09-14; 189 tests across 16 files, measured 2026-10-02, with the new `web/src/lib/termLiveness.test.ts` — 11 tests; 211 tests across 18 files; 230 tests across 19 files from 2026-10-05, with the new `web/src/lib/resources.test.ts` after that, with the new `web/src/lib/updaterNotice.test.ts` (8 tests: no token path named, no notice for none or ok, each failing code's sentence, an empty `latest` or `deployed`, `deploy-failed` never saying "stays on", `stale`, `unknown`); 203 tests across 17 files earlier the same day, with the new `web/src/lib/termOutbox.test.ts` — 8 tests: the typed-while-down queue's order, its 256-entry cap, the 30 s age rule at its boundary with an injected clock, and that one stale key drops the whole queue — and six `CdpClient.alive` tests in `cdp.test.ts`: true on a reply, an error reply or an unrelated event inside the window, false on silence for the whole window, on a close under the probe and on an already-closed client): `web/src/lib/api.timeout.test.ts` (added 2026-09-13, BUG-055: `request()`'s AbortSignal ownership — no caller signal gets a 20 s timeout signal from `request` itself; a caller's own `AbortSignal.timeout(30000)` reaches `fetch` as the SAME object, never wrapped; `checkFile`'s bare `AbortController` signal reaches `fetch` as a composite that aborts when either the controller or the 20 s timeout fires), `web/src/lib/clips.test.ts` (added 2026-09-13), `web/src/lib/diffReveal.test.ts` (now tests `web/src/lib/diffReveal.ts` — the `reveal` function moved out of `DiffView.tsx` into `lib/` so its imports stay one-way), `web/src/lib/cdp.test.ts` (the CDP client
the Browser pane speaks over: request/response correlation, an upstream
close surfacing as `CdpError`/`CDP_CLOSED`), `web/src/lib/browserInput.test.ts`
(the Browser pane's keyboard/pointer mapping, plus `isChord` — gained the
AltGr case 2026-09-13: `Ctrl+Alt` is NOT a chord, so an AltGr-composed
character types into the page instead of being swallowed as a shortcut),
`web/src/lib/lan.test.ts` (8 tests, rewritten for ADR-067's two-state model — `off`/`pending`/`error`/`unavailable` are gone, so this file no longer tests them: `lanChip` renders nothing for a sandboxed or field-absent project and flags "LAN" with warn colours when on; `lanLine` states what LAN mode grants and its limits, and that sandbox is internet-only; three for `preferOverride`, BUG-020 — the optimistic PUT reply is shown until the poll CATCHES UP with it, an ordering comparison on the one `lanAccess` boolean rather than the timestamp-equality test that used to latch the component to a stale reply forever once anything changed the record out of band — the latch is now structurally impossible since there is no clock to compare), `web/src/lib/api.test.ts` (`projectNameFromRepoURL`, `cloneURLError`, `projectNameError` — pure, security- or UX-load-bearing string logic that was the frontend's entire test-coverage story before this), `web/src/lib/api.encoding.test.ts` (added 2026-09-09, closing the security review of 2026-09-03, round 2: every `api.ts` call that interpolates a caller-supplied value into a REST or WS path is exercised with a path-climbing (`../..`) and a query-injecting (`?wipe=true`) value, asserting the RESOLVED path never leaves its intended prefix and never contains a bare `..` segment — written against the call sites rather than the `u` template tag itself, so a new call site that forgets to use it fails the suite instead of a test of the tag passing regardless; also covers `termSocketUrl`'s project/terminal-id encoding and that legitimate query strings built outside the tag still work; its `browserStatus`/`browserStop` rows were deleted 2026-09-13 along with the dead SPA wrappers they covered — the pane calls `browserExec` directly and the REST `api.browserStart` is the only wrapper still live, for Retry), `web/src/lib/editorPane.test.ts` (`basename`; the 8 diffReveal tests, v1.22.0, moved to `diffReveal.test.ts` above — `reveal` makes bidi overrides/isolates, invisible directional marks and C0/C1 controls VISIBLE as `<U+202E>`-style markers instead of passing them through, since the diff pane's job is reviewing a change before committing it and a Trojan-Source line renders differently from how it applies; TAB is deliberately kept), `web/src/lib/recentsPanel.test.ts` (14 tests, v1.22.0: the Recent strip's persisted geometry — height clamped to 48..320 on the way IN and OUT so a hand-edited key cannot collapse the panel to nothing, a stored string parsed since localStorage holds only strings, the collapsed flag REMOVED rather than set to "0" when expanded, and a private window throwing on access), `web/src/lib/explorerHistory.test.ts` (13 tests, v1.22.0: the file explorer's back/forward reducer — the forward tail dropped on a new navigation, object IDENTITY preserved on a redundant push so it cannot re-render or refetch, the 50-entry cap dropping oldest-first while keeping the current entry reachable, and `crumbTrail`'s middle elision keeping each shown crumb's REAL full path rather than an offset into the shortened list), `web/src/lib/appearance.test.ts` (17 tests, v1.22.0: the Settings font store — the two surfaces' differing defaults, per-FIELD fallback so one bad value never discards the others, size clamping to 8..32, a numeric string and a prototype key (`"toString"`) both rejected as font families, corrupt JSON, and a private window whose `localStorage` throws on ACCESS rather than only on write; plus a guard that `FONT_STACKS["google-sans-code"]` is still byte-identical to `terminal.ts`'s historical stack, since a dropped fallback silently changes what renders), `web/src/lib/api.session.test.ts` (6 tests, BUG-001: `request()`/`uploadFiles` against a stubbed browser facing the ingress's cross-origin session-expiry redirect — re-auth once, the reload cap under partial failure, the deferred counter clear, malformed and corrupt counter records), `web/src/lib/sshKey.test.ts` (added 2026-09-10, TODO close-out item H: `\p{C}` control/formatting rejection — RLO, ZWSP, BOM, U+00AD — and `abbreviateKey`'s `\p{Zs}` space-run collapse, plus the full-OpenSSH-type-token enum matching the server's `publicKeyLine`), and `web/src/lib/projectBadge.test.ts` (added 2026-09-10, item 29: the pure `projectBadge(p)` function's precedence over `deleteError`/`provisionError`/`deleting`/`provisioning`, extracted out of `Sidebar.tsx` so the badge choice is unit-tested rather than left to the component). `editorPane.test.ts` also covers the pane store's `discard(req)` (item K9): the parked draft and dirty flag clear and the loaded doc drops on discard. Deliberately NOT unit-tested: the editor pane store's per-terminal draft behavior (parked on unmount, restored only for the file it belongs to, dropped when that file closes or is evicted) — its state lives in a module-level `Map` that is not exported, so reaching it from a unit test would mean either widening the module's API just for tests or pulling in a React renderer; `editorPane.test.ts`'s own header comment says so explicitly rather than faking coverage with an assertion that would always pass. That behavior is covered by Tier 4 e2e instead. Outside these two files, most TypeScript/React logic is still covered only by type-checking + lint plus, where applicable, Tier 4 e2e — a changed function/component with no unit test and no e2e exercise has no automated coverage at all, note it explicitly when verifying.

**The real uncovered list (measured 2026-09-22, not estimated).** A repo-wide
verifier run reported 33 of 47 `web/src` files as having no unit test and
called it a FAIL. That was a *not examined*, not a *confirmed uncovered*: Tier
4 was not run in that pass. Both suites were then run on this host — Tier 2
`passed=112 failed=0 skipped=3`, Tier 4 `passed=93 failed=0 skipped=2` — and
the answer is: the components Tier 4 drives (the sidebar, the file explorer
and editor pane, the terminal pane and status bar, the side panel's Files /
Ports / History / Git / Clipboard / Env tabs, the browser pane, the new-project
and delete dialogs, the setup page, the LAN badge and network block) are
exercised end to end, while `src/lib/fonts.ts`, `src/lib/projects.ts`,
`src/lib/toast.ts` and `src/router.tsx` are reached by neither tier. Those four
are the honest gap. Standing up a component-testing framework is a dependency
decision and has not been taken; until it is, a change to any of those four
has no automated coverage at all and must say so.
2. **Integration (149 checks; 3 auto-SKIP on firewall-hardened hosts — on 2026-10-05 twenty-four checks were added (a green run on this host prints `passed=146 failed=0 skipped=3`) after the network-mode section, "resources (ADR-083)": a new project records the operator disk default; GET reports unlimited and the host's limits; the guest sees every host CPU and ≥ 97% of host RAM; a PUT of 2 CPUs / 4 GiB / 21 GB rebuilds a running project, after which the guest cgroup reads `cpu.max 200000 100000`, `memory.max` 4 GiB and `nproc` 3, and the Docker disk grew by 1 GiB at boot; a shrink, CPUs above the host and a missing field are 400; a stopped project's PUT only records and its next start applies it; an unchanged project keeps its container across stop/start; a live restart-policy update leaves the size alone; back to unlimited rebuilds; the grow's retry branch (the host truncates the stopped project's image file to 22 GiB over a 21 GiB filesystem, PUTs 22, and the next boot grows the filesystem — 3 checks); the persistence step's `docker rm -f` retries while Docker reports the exiting VM as a zombie (BUG-090) and a check asserts it took effect; later on 2026-10-02 three checks were added after "settings served" for the updater status (ADR-081; the suite's server runs with `STYRMAN_UPDATE_STATUS_FILE=${STATE}/status/updater.json`): "settings: no updater status file, no updater key", "settings: a failing updater status reaches an admin, code and all" (a file planted with the current time) and "settings: an updater status that cannot be read is \"unknown\", never ok"; against the server before the change the suite printed `passed=120 failed=2 skipped=3`, the two checks that expect an `updater` key failing, and after it `passed=122 failed=0 skipped=3`. A green run against a CURRENT runtime image printed `passed=112 failed=0 skipped=3` until 2026-10-02; on 2026-10-02 the final green run on this host against the rebuilt `styrman-runtime:test` printed `passed=119 failed=0 skipped=3` (boot to ready measured 3.10 s on ext4 in that run; one check was added after the earlier `passed=118` run, 121 to 122: "agy's directory is owner-only: the project user's, mode 755, not writable by another uid" (it also asserts `/opt/agy/agy` is executable and nothing under `/opt/agy` is group- or other-writable), whose positive control is `runuser -u nobody -- true`; before it, six checks were added, 115 to 121 — "harness step finished after ready (harness-done marker, ADR-078)", "~/.npm-global/bin and lib exist, user-writable, bin on a login shell's PATH", "claude doctor reports auto-updates usable (no unwritable-prefix warning)", "project user can write agy's directory; /usr/local/bin/agy resolves into it", "ready line precedes the agent harness deployed line (install runs after ready)", "a ping text frame is answered with a pong"; the state dir is now `mktemp -d /var/tmp/styrman-itest-XXXX`, not `/tmp`, which is tmpfs on this host — the boot-budget check read 3.26 s there for an image that measured 16.16 s on disk — and the boot-budget check names the state dir's filesystem type in its label and FAILS, not SKIPs, on tmpfs; `wait_seed` waits, bounded at 150 s, for `/run/styrman/harness-done` before harness assertions that follow a boot) (2026-09-21, ADR-074: the harness channel's fifteen checks went with the channel — the scratch delivery, the server's start-up line, the guest COMMIT/read-only/remount/sha256/no-`.git`/`source channel tag` checks, the plain-copy `status` label, the commit-named boot line, the swap-while-stopped pair — and twelve came in their place: the image's `sh-agent version` is a canonical release, the home's `~/.local/bin/sh-agent` reports the same release, the manifest records `embedded:vX.Y.Z`, the roster equals the binary's `list`, the boot log's release-named line, the stamp as version+release, and the seven-check never-downgrade case in the stop+delete section (stopped again, manifest planted `embedded:v99.0.0` from the host with the stamp cleared, `boot=1`, the `left alone` log line, a `kept` pass line with no new `deployed` line, binary and manifest untouched, stamp rewritten), for 115 total; 2026-09-16, security review of the harness channel: one check added — guest root remounts `/styrman/harness` rw and the write still fails, nothing lands in the host's export (the `:ro` is enforced on the host side of the virtiofs share), for 117 total; 2026-09-16, ADR-073 harness channel Phase 3: ten checks added — the scratch channel delivered before the server starts, the server's start-up line naming its tag and commit, five guest checks in the harness section (the channel's COMMIT, the read-only mount, the binary's sha256, no `.git`, the boot log's `source channel tag v1.0.0`), and three in the stop+delete section (the swap to `v1.0.1` while stopped, the restarted VM mounting the new export, the re-seed from it); the mirror-commit check now reads the channel's `COMMIT` and the two stamp checks compare against version+commit, for 116 total then; 2026-09-16, BUG-061/BUG-062: the harness section's `settings.json` clause dropped from the "no previous-harness artifacts" check and two checks were added in its place — the boot log carries the installer's `settings:` line for the created file (ADR-072), and the guest `settings.json` holds exactly the attribution key with trailers off; the persistence-across-destruction section gained four checks around the recreate — planting a non-regular file at `~/.claude/settings.json` before recreate, the boot log's `note:` line for a settings.json it could not judge, the seed still completing around the refused key, and the plant/stamp undo for one more seed; the stop+delete (wipe) section gained one check confirming the re-seed after that undo recreates `settings.json` with the key; net seven checks added, one replaced, for 104 total; 2026-09-16, BUG-063: the "stop + delete (wipe)" section gained two checks for the Stop-path restart-policy race — "stop left the container exited 0, no restart, policy off (BUG-063)" (`docker inspect` must read exit 0, `RestartCount` 0, policy `no`) and "start switched the restart policy back on (BUG-063)" (policy `unless-stopped` after the `boot=1` start), for 106 total; 2026-09-14: the harness section gained "sh-agent status labels the image mirror a plain copy" and "boot log records the harness deployed at the mirror's commit"; 2026-09-13: the browser section gained "a second start keeps the running forwarder" and the harness section "harness stamp equals the image version after boot"). Pass the image explicitly: the default `styrman-runtime:test` goes stale and its pre-v1.22.0 skel fails three GOPATH checks for image reasons, not code reasons)** — `bash .agents/docs/testing/styrman/integration.sh [image]` (port `STYRMAN_ITEST_PORT`, default 18080; the run REFUSES to start when that port is already bound, since its health probe cannot tell its own server from another — BUG-040, 2026-09-11)
   (default `styrman-runtime:test`). Isolated server (own state dir, port
   18080; the port guard itself now REFUSES to start the run at all when `ss`
is missing — 2026-09-13, was fail-open under `pipefail`, the same shape as
BUG-040's own fix, closed before it could recur) through the full
lifecycle: legacy-layout migration, async create
   → microVM boot, terminals (records, rename, pause, close-all),
   WS I/O + reattach, transcripts, file explorer (incl. traversal guards),
   ports + all preview URL shapes, the in-guest git client (status + diff),
   sticky stop (close 4002 refusal vs `?boot=1` revive), settings,
   persistence across container destruction, UUID identity + friendly
   rename, wipe, and (since ADR-062) the seeded agent harness, which since
   ADR-074 is the image's own `sh-agent` release binary (no scratch channel,
   no mirror: the image under test carries the release its Dockerfile
   pins): in the guest the image's `sh-agent version` is a canonical
   `vX.Y.Z`, `~/.local/bin/sh-agent` reports the same release, the manifest
   records `embedded:vX.Y.Z`, `sh-agent status` from the home's binary
   reports no drift, the seeded skill and agy-agent counts equal the
   binary's own `list`, the two-line `CLAUDE.md`, the boot log's `agent
   harness deployed` line at the channel's commit, the stamp equal to
   version+commit, no previous-harness artifacts; and, in the stop+delete
   section, the channel swapped to `v1.0.1` while the VM is stopped and the
   `boot=1` start mounting the new export (Docker re-resolves the bind
   source) and re-seeding from it — and boot→ready within R1's 15 s hard limit
   (measured and printed; the 5 s soft target is reported, not asserted), and
   (since v1.22.0) the seeded shell skel: `~/.bash_exports` carries
   `GOPATH="$HOME/.go"`, a login shell resolves it, and `~/.go/bin` joins
   PATH once the directory exists. Those three MUST use an INTERACTIVE login
   shell (`bash -lic`): `skel/bashrc:7` returns early for a non-interactive
   shell, so a plain `bash -lc` never reaches `.bash_exports` and reports an
   empty `GOPATH` — a real styrman terminal is interactive under tmux.
   The "network mode" block (ADR-067) replaces the old ADR-065/066 LAN
   checks entirely: there is no server/root split left to test, because the
   mode IS the Docker bridge interface name the server picks at `docker
   network create`. Against a real project: the record starts
   `lanAccess: false` and its bridge (`bridge_of`, reading
   `com.docker.network.bridge.name` off the Docker network) carries the
   `sdb-` prefix, matched by the running container's own interface;
   `PUT .../network {lanAccess:true}` returns 200, the record flips to
   `true`, and the bridge is REBUILT under the `lan-` prefix (same sha1
   suffix of the same network name, asserted byte-for-byte). Where
   `sudo -n iptables -S` is readable, two more checks run live (no opt-in
   flag any more — `STYRMAN_IT_LIVE_LAN` no longer exists): no styrman jump
   matches the `lan-` bridge at all, and Docker's own unconditional
   per-bridge DROP still isolates it from every other bridge. Then
   `PUT {lanAccess:false}` returns 200 with `rebuilt: false` (idempotent —
   already off) and the bridge is back to `sdb-`; a body missing
   `lanAccess` is 400; a nonexistent project id is 404. A project is
   re-enabled to LAN right before its own delete, so the destroy path is
   exercised on a `lan-`-prefixed bridge too, not only a sandboxed one.
   (The old host-side `/diff` + checkpoint checks were replaced with
   `/git/status` + `/git/diff` checks when those routes were removed in
   the git-RCE fix, ADR-039.)
3. **Manual / UX** — things scripts can't judge:
   - `go run ./cmd/styrman/` → http://127.0.0.1:8080/ in a browser
   - create a project each for claude and agy (Antigravity); complete each agent's first-run
     sign-in in the terminal (per project — or once + Settings → Credentials → Import, ADR-032)
   - reload the tab mid-session → same scrollback; open a second tab → first
     shows "Opened in another tab" (4001) with a "Use here" button and does
     NOT reconnect by itself; "Use here", or showing that tab again
     (hidden → visible), takes the terminal back in place (ADR-079)
   - type `exit` in a shell conversation → "session exited" (4000)
   - resize the window → TUI reflows (tmux + ContainerExecResize)
4. **Browser e2e (Playwright)** — see the Tier 4 section below.
5. **Agent harness in the image (`/styrman/bin/sh-agent`)** — see the Tier 5
   section below: the in-guest assertions are Tier 2 checks since ADR-062;
   what remains manual is the guard controls on a Dockerfile change and the
   agent-CLI smoke.
6. **Security (govulncheck)** — `bash deploy/vulncheck.sh`. Runs
   `govulncheck@v1.7.0` for BOTH Go modules (root, and the vendored Caddy
   ingress at `deploy/caddy/ingress`) inside the SAME pinned `golang`
   builder image `deploy/Dockerfile.server` uses, with `GOTOOLCHAIN=local`
   so the scan always runs against that image's OWN Go rather than
   auto-switching to whatever a `go` directive asks for — not the host's
   `go`, which reported 31 stdlib-only findings the shipped binary never
   has (2026-09-02). The repo is mounted read-only, and, WHEN `deploy/.env`
   exists, the one secret-bearing file in it is shadowed with `/dev/null`
   inside the container — a tool this script downloads and runs must never
   see the ingress token. That shadow is now conditional (2026-09-02 audit
   fix): bind-mounting `/dev/null` over a path absent from the tree fails in
   runc with exit 125, which made the script unrunnable on a checkout with no
   `deploy/.env` at all — a fresh clone, or any CI checkout — because a
   release gate that cannot run there is worse than no gate; a checkout with
   no `deploy/.env` has no secret to hide anyway, so it now runs cleanly
   instead. Exit status is govulncheck's own, via `go install ... &&
   govulncheck ./...` (not `go run`, which would collapse a real finding into
   the same exit code as a broken scan): `3` = a called vulnerability in
   either module, `0` = clean, anything else = the scan itself failed
   (network, install, or a broken image) rather than a finding. **Expected
   exit on this repo today is `3`** — the accepted baseline of two
   `docker/docker` findings with no upstream fix, re-checked — not silently
   re-accepted — every release; `0` would mean that baseline shrank (good,
   update the accepted count) and anything else is a broken scan, not a new
   finding.
   Since 2026-09-02 (round 3) the exit across the two modules is WORST-WINS,
   not last-writer-wins: a broken scan (anything but 0/3) always overrides a
   prior finding, and a finding (3) is never masked by a later clean module —
   verified by running the script's own per-module command shape with one
   module path deliberately broken, in both orderings.

## Prereqs

- Both `integration.sh` and `web/e2e/run.sh` now run
  `.agents/docs/testing/styrman/ensure-test-image.sh <tag>` FIRST (TODO
  close-out 2026-09-10, item L): it rebuilds `styrman-runtime:test` when the
  image is missing or `image/` has a file
  newer than the image's `Created` timestamp, so a stale `:test` tag can
  never masquerade as a source regression (BUG-026). `STYRMAN_TEST_IMAGE_NO_REBUILD=1`
  skips the check entirely for fast iteration. It refuses to rebuild a
  production tag (`deploy/.env`'s `STYRMAN_IMAGE`, or `styrman-runtime:dev`/
  `*:latest` as a fallback guess) and a
  failed build is FATAL, so no check ever runs against an image the script
  could not refresh. You do not need to build the image by hand before
  running either suite; the line below is for building it standalone.
- `styrman-runtime` image built: `GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN -t styrman-runtime:test --build-arg STYRMAN_VERSION=test-$(date +%s) image/`
- Kata runtime `mvm` working: `docker run --runtime mvm --rm debian:trixie uname -r`
  (host-setup.sh verifies this; if it fails with "invalid namespace type" see
  ADR-009 — the daemon.json time-namespaces flag is missing)

## Privileged (root) test convention — scratch state only (ADR-052)

The firewall script (`deploy/firewall/styrman-firewall.sh`, ADR-036/067) is
the model for testing a root script WITHOUT root: `deploy/firewall/test.sh`
runs the real script under `unshare -Urnm` — an unprivileged user + network +
mount namespace with its own sysfs remounted, so `/sys/class/net` shows only
the namespace's dummy interfaces and `iptables` starts from an EMPTY ruleset.
Since ADR-067 the script has one job — keep `sdb-*` bridges isolated — so
`test.sh` asserts the base isolation rules, the golden `sdb-`/`lan-` bridge
names `BridgeNameFor` would produce, and that no rule anywhere names a
`lan-` interface. `internal/netpolicy/script_test.go` is gone with
`internal/netpolicy/` itself (~1,049 lines, deleted outright, not just
unused); its drift guard moved to
`internal/config/firewall_script_test.go` (`TestFirewallScriptMatchesBridgePrefixes`,
runs under plain `go test ./...`, no root or namespaces needed): it reads
`deploy/firewall/styrman-firewall.sh`'s source and asserts its `sdb-`
wildcard literally matches what `config.BridgeNameFor` produces, so the two
can never drift apart silently. `test.sh` refuses to run in the host's
network namespace before it mounts or changes anything.
`deploy/firewall/install.sh` is verified by `bash -n`, shellcheck and its
`--dry-run`; its real run is the operator's install on the host, recorded in
the changelog — never a test.


Any root-run exercise of a host-level script (the auto-updater, the backup
script, the firewall script, or any future one) MUST point its state/target
directories at a scratch prefix that cannot alias a real production path —
never a default that resolves inside `/var/lib/styrman`, `/etc/styrman`, or
their equivalents. Cleanup stays confined to that scratch prefix too: never
`rm -rf` a shared directory without first confirming everything that lives
there belongs to the feature under test — a directory that PREDATES the
feature under test (like `/etc/styrman`, which held the restic/GCS DR
secrets before the auto-updater existed) is the highest-risk case, because
it looks self-contained to whoever is testing the newer feature.

This is a direct fix for the 2026-09-01 state-loss incident: while
privileged updater tests and parallel verification agents ran against paths
adjacent to production state, `/var/lib/styrman` was found emptied and
`/etc/styrman` was `rm -rf`'d by a test cleanup step — see
`.agents/docs/changes/2026-09-01-incident-state-loss.md` and ADR-052 in
`.agents/docs/architecture/DECISIONS.md`. `styrman-update` now enforces its
half of this structurally, not just by convention: it has NO in-script
default for `STYRMAN_UPDATE_STATE_DIR` (a hard `fail` if it's unset) and
explicitly refuses any value that is, or is inside, `/var/lib/styrman`
(canonicalized with `realpath -m` first, so `//var/lib/styrman/x` and
relative paths cannot slip past a literal-prefix match). Production always
runs it via `systemctl start styrman-update.service`, which pins
`/etc/styrman/update`; any hand run or test must pass its own value, and
MUST use a NESTED scratch path so the parent directory is one this script
creates rather than an existing shared dir like `/tmp` itself, e.g.
`STYRMAN_UPDATE_STATE_DIR=$(mktemp -d)/state styrman-update --check` (the
script's own `install -d` only creates what is missing — it never re-modes
an existing parent — but a nested path keeps the test unambiguous).

### Updater self-install (`self_install()`, ADR-052/ADR-057 shape)

After a health-checked deploy, `styrman-update` pins the verified commit
ONCE (`SELF_HEAD="$(git rev-parse HEAD)"`) and installs
`deploy/update/{styrman-update,.service,.timer}` **from that git object**
over the host copies (`STYRMAN_UPDATER_BIN` / `STYRMAN_UPDATER_UNIT_DIR` —
no in-script default; the unit alone pins `/usr/local/bin/styrman-update`
and `/etc/systemd/system`). `atomic_install()`/`differs()` source bytes with
`git show "${SELF_HEAD}:${gitpath}"` — **the worktree is never read** — so
**`DEPLOY_DIR` in the harness must be a real git repository**, not a plain
directory of files: a directory holding uncommitted copies of the three
files will make every `differs()`/`git cat-file -e` call fail closed
(nothing installs). Test it in a scratch harness, never against the real
targets:

```bash
sed -n '/^trusted_dir()/,/^}/p;/^atomic_install()/,/^}/p;/^differs()/,/^}/p;/^self_install()/,/^}/p' \
    deploy/update/styrman-update > /tmp/self-install-funcs.sh
# in a scratch script: source /tmp/self-install-funcs.sh, then set
# REQUIRE_SIGNED=1, define log(), and a git() wrapper that replicates the
# real script's cwd-relative one (`git() { command git -C "$DEPLOY_DIR" -c
# safe.directory="$DEPLOY_DIR" "$@"; }` — the real script instead `cd`s into
# DEPLOY_DIR before defining an equivalent cwd-relative git()). Export
# DEPLOY_DIR=<scratch>/deploy_repo — a `git init`'d, root-owned GIT REPO
# whose HEAD commit has deploy/update/{styrman-update,.service,.timer}
# COMMITTED, not a plain directory of copies. Set
# UPDATER_BIN=<scratch>/bin/styrman-update and
# UPDATER_UNIT_DIR=<scratch>/units, both root-owned, mode 0755 scratch
# dirs, put a stub `systemctl` that just logs its argv on PATH, then call
# `self_install`.
```

Run the sourcing script as root (`chown root:root` on the installed files
needs it) and assert: (a) targets absent → all three created, mode 755/644,
root-owned, `daemon-reload` called once, targets logged; (b) run again, HEAD
unchanged → `differs()` reports false for all three → nothing rewritten
(same inode via `stat -c %i`), `daemon-reload` NOT called; (c) **edit the
scratch repo's WORKTREE copy of `styrman-update` WITHOUT committing**, then
rerun → HEAD unchanged, nothing installed, installed bytes do NOT contain
the tamper — a deliberate NO-OP (confirms the round-2 security MEDIUM —
install-from-worktree — stays closed); (d) commit a real change to the
scratch repo, then rerun → new inode, installed bytes equal the new `HEAD`
blob, `daemon-reload` called; (e) `STYRMAN_UPDATE_REQUIRE_SIGNED=0`
(`REQUIRE_SIGNED` in the extracted functions' scope) → skip, nothing
written, checked BEFORE target validation; (f) rename so `UPDATER_BIN`'s
basename isn't `styrman-update` → `WARN`, skip, nothing written; (g) make
`UPDATER_UNIT_DIR` group-writable (`chmod g+w`) → `trusted_dir()` fails,
`WARN`, skip, nothing written; (h) put a **directory** at `<bin>` before a
run → `mv -fT` refuses to overwrite it, `WARN: could not self-install`,
trailing `WARN: self-install incomplete` (the success line is suppressed
whenever anything failed), `self_install` still returns 0, and the units
(a different, non-directory target) still install correctly; (i) plant
`<name>.new.<pid>` orphans for **all three** target names but only commit a
change to one file → the next run removes all three orphans (the sweep is
hoisted to the top of `self_install`, not scoped per-file inside
`atomic_install`).

Separately, hand-run the real script end-to-end against a scratch clone with
an unreachable `origin` (`git remote set-url origin
https://example.invalid/...`) and a scratch, root-owned, non-group-writable
`STYRMAN_UPDATE_STATE_DIR` — the fetch fails before `self_install` is ever
reached, so the self-install log line must NOT appear and nothing must be
written outside scratch (confirmed 2026-09-02, rounds 2 and 3:
`/usr/local/bin/styrman-update`, the real unit files, and
`/etc/styrman/update` all had unchanged mtimes after the run).

## Rebuild-and-retest loop

```bash
go test ./... && bash .agents/docs/testing/styrman/integration.sh
```

Image changes: rebuild the image first; the integration script always creates
fresh containers so it picks the new image up automatically.

## Tier 4 — Browser e2e (Playwright; a green run on this host prints `passed=101 failed=0 skipped=2` plus a final client-error gate (103 checks). 2026-10-05: two checks added at the end, after the network toggles and before cleanup — "Resources dialog: unlimited by default, an out-of-range CPU is refused before saving" and "Resources dialog: 2 CPUs / 4 GiB saves, restarts the project, and reads back" (ADR-083; each closes the dialog in a `finally`). 2026-10-02, last: one check added after "settings shows runtime info" — "settings tells an admin when the host's auto-updater is failing" (`run.sh` exports `STYRMAN_UPDATE_STATUS_FILE=${STATE}/status/updater.json` to the server and to `run.mjs`, which plants a `failing`/`fetch-auth` file with the current time, reloads, reads the notice and removes the file in a `finally`); against the server before the change the suite printed `passed=98 failed=1 skipped=2`, that check alone, which is also its deliberate-failure run (no cascade). Before it a green run printed `passed=98 failed=0 skipped=2` (100 checks). 2026-10-02, later: one check added in the LAN window, after "switching to the terminal and back resumes the screencast" — "a browser view whose socket went silent streams again once its tab is shown" (a throwaway page on terminal 2 opens the Browser view, its one relay socket is frozen by the socket fixture, the tab is hidden and shown, and a new unfrozen relay socket and a ready pane must follow); against the page before its fix the suite printed `passed=97 failed=1 skipped=2`, that check failing on its 20 s wait for the new socket. Earlier the same day a green run printed `passed=97 failed=0 skipped=2`. 2026-10-02 (ADR-079): three checks added after "deep link reattaches terminal 1" — "a terminal taken by another tab is taken back with one click", "a replaced terminal comes back by itself when its tab is shown again", "a terminal whose socket went silent works again once its tab is shown"; the run before them printed `passed=93 failed=0 skipped=2`, and the same suite against the page before the fix printed `passed=93 failed=3 skipped=2`, the three new checks failing. A fourth check followed the same day, "keys typed before a pane gave up are not replayed when it comes back" (a green run printed `passed=97 failed=0 skipped=2`, 99 checks; the run with three of them printed `passed=96`); against the page before its fix the suite printed `passed=96 failed=1 skipped=2`, that check failing with "the line typed before the pane gave up was replayed into the shell". 2026-09-22: one check added, "a directory past the listing cap says so" — it creates 5200 files in the workspace and asserts the explorer's truncation notice appears for that directory and NOT for the workspace root. `internal/files.List` has been capped since the same day and `FileExplorer.tsx` has no unit test, so this is that notice's only coverage; it restores the explorer's path in a `finally`, because when it failed during its own must-fail probe it left the pane misplaced and cascaded into eight later checks. Five Browser-view checks (ADR-068, 2026-09-11; a fifth added 2026-09-13 for the browser review-fix batch) run INSIDE the end-of-run LAN window — between "enabling rebuilds the project" and "disabling returns it to sandbox" — because a host-run server cannot reach a sandbox bridge on this firewall-hardened host and the relay to the in-guest Chromium has the same shape as the auto-SKIPped preview checks: the Browser pill opens the view and a screencast frame arrives (`canvas[data-frames]`), a typed `localhost:4100` navigates with the tab strip carrying the fixture's title, "browser" on the Ports row switches the view and navigates, "switching to the terminal and back resumes the screencast" (2026-09-13: reads the frame count, clicks Terminal, clicks Browser, and waits for count + 1 — `Page.startScreencast` always emits a first frame, so the count rises only if the show path actually restarted it after the hide path stopped it), and the terminal session survives the round-trip. ADR-067 replaced ALL of the old ADR-065/066 LAN checks — the host-half `pending`/`unavailable`/error states, the scratch `STYRMAN_FIREWALL_DIR` stub, the planted fake root status file — with five checks against the two-state model, since there is no host half left to stub: "network block shows the sandbox default" (a new project's Ports-tab "Network" switch reads unchecked/"Sandboxed.", enabled for this admin user), "the API reports sandbox and says the caller may enable" (`GET .../network` is `{lanAccess:false, canEnable:true}`), "no LAN badge on a sandboxed project" (`[data-lan="on"]` renders nowhere — the assertion tsc cannot make, since the badge renders from an optional field and a regression would silently claim every project is on the LAN), and two TOGGLE checks held to the END of the run rather than inline, because applying the switch rebuilds the project's container (ADR-067's accepted cost) and destroys everything a later check in the same project needs (the fixture app, tmux sessions, git working state): "enabling flips the record and shows the LAN badge" and "disabling returns it to sandbox". None of this is stubbed or root-owned any more, so unlike ADR-065/066 there is nothing here that ONLY a hand-walked check against a converged production host could exercise — except the operator's own criteria (LAN reaches the host/LAN/published ports; sandbox does not), which stay a manual walk-through, logged in `testing/`, because Playwright's own host has no real LAN to prove reachability against. v1.22.0 added six appearance checks, two path-navigation checks, three recents-strip checks, three git-diff/fast-commit checks and six git-verb checks (branch create, branch SWITCH — a different argv shape that runs `checkout --end-of-options`, invalid-name rejection, fetch/pull controls reachable, commit history into the pane, stash push -u / pop). The client-error gate records the failing URL alongside the browser's message, which otherwise names neither endpoint nor caller; re-measure after any further check additions before citing a pass count as current)

**Terminal liveness fixtures (ADR-079, 2026-10-02).** A second and a third
page come from the suite's own `browser.newContext()` (Playwright refuses
`context.newPage()` on the implicit context of `browser.newPage()`). A
hidden → visible tab is EMULATED by overriding `document.visibilityState` /
`document.hidden` and dispatching `visibilitychange`, because headless
Chromium does not hide a background page. A dead socket is EMULATED on a
throwaway third page (terminal 2) by an init script that wraps
`window.WebSocket` so one instance can be frozen: its sends dropped, its
message and close events withheld, `readyState` still OPEN (it lists browser relay sockets too, `window.__browserSockets()`, for the browser view's dead-socket check, which uses the same freeze). The two socket checks ("a terminal whose socket went
silent works again once its tab is shown" and the keys-not-replayed check)
share that one init script, installed on the throwaway page before its first
navigation. It can also close the live socket towards the app with a chosen
code (`__dropNow(code)`) and make the next terminal socket the page opens
close with a chosen code before it opens (`window.__failNextTermSocket`). The
keys-not-replayed check stages: live socket dropped as a transient close
(1006), a line typed (queued while reconnecting), the reconnect attempt failed
with a non-transient code (1011, the pane shows "Disconnected"), the tab shown
again (the pane reconnects), and asserts the queued line never ran. The first page is
not navigated by these checks; each closes its extra page and removes its
overrides in a `finally`. NOT covered by any tier: a real sleep/wake, a real
network change, a real hidden tab; the 25 s interval and the `online` trigger
in a browser (their logic is the unit-tested probe; the e2e drives the
visibility trigger); the idle reaper stopping a project under a pinging tab
(only Go unit tests: `TestClassifyControl` proves the classification and
`TestApplyControl` proves the read loop honours it, a ping never calling
the idle reaper's touch; the reaper's timer actually stopping a project
under a pinging tab is not covered).

`bash web/e2e/run.sh [image-tag]` — self-hosts: builds the web SPA
(`cd web && npm run build`) FIRST, then the server, then starts its own
instance on a host loopback port with an isolated state dir, then runs the
Playwright checks against it. The SPA build has to run first because the
server embeds `web/dist/client` via `go:embed` (`web/embed.go`) — building
only the Go binary would silently test whatever assets were already sitting
in `dist` from a previous build, so a real frontend fix could appear to
still be failing (hit for real 2026-09-01). Skip it with
`STYRMAN_E2E_SKIP_WEB=1` when iterating on server-only changes. A run
therefore rewrites the TRACKED `web/dist/client` files with fresh
content-hashed asset names, so `git status` is dirty afterwards — that's
build output, not stray edits; stage it with `git add -A -- web/dist`.
Production has no auth-bypass dev
door to test against anymore (2026-08-27 security review; see
`../../specs/styrman/deploy.md`), and a host-loopback server is unreachable
from project microVMs either way.
Drives the real UI in headless Chromium against that server: project
creation (boots a microVM), agent + shell terminals (xterm's DOM renderer
makes terminal text assertable, plus copy-on-select),
typing into the microVM, the Files/Ports/History/Git/Clipboard side-panel tabs,
rename/delete, deep links, setup page, theme, the editor pane (a file click
opens it there — not a modal — with the correct content, toggling back to
the terminal, the terminal session surviving that round-trip, reopening the
same file, the word-wrap toggle, Dockerfile syntax highlighting,
discard-and-open correctly LOADING the second file rather than dead-ending
on an empty pane, an upload that overwrites the OPEN file evicting its
editor pane while keeping its Recent chip (eviction is keyed off the
server-reported saved names, the same path a mid-stream upload failure
also reports, not the dropped names — code review, 2026-09-02),
deleting a file that is open in the pane, and syntax
checking — a broken shell script flags a lint-gutter error, a clean file
stays clean, and the check/format temp file leaves no litter in the
workspace listing), plus one drift-guard check added 2026-09-10 (TODO
close-out item K8), "checkable extensions match the server's checkers": for
every extension the SPA's `CHECKABLE_EXTENSIONS` lists, asks the check
endpoint for one file of that type and expects a tool name back, and for a
`.xyz` file expects "no checker" — comparing the two lists end to end
through the API rather than cross-parsing either source, plus two
mutation-guard checks added 2026-09-02 that
kill mutants the rest of the suite couldn't catch — "same path in both
scopes: content flips, deletes stay scoped" (a same-relative-path home and
project file, flipping the open pane between them via their chips, then
deleting the project copy while the pane holds the home copy asserts the
project chip is gone, the home chip and its content survive, and the pane
stays put — the scope-flip regression), and the on-disk recents list is capped at 8
on READ, not just on write (seed a 20-entry `styrman.recentFiles.<project>`
blob in localStorage, reload, assert only 8 chips render), plus one more
check added 2026-09-02, "a clone URL carrying a token is refused" (the
server-side `cloneURLHasSecret` 400, api.md's `POST /api/projects` row):
filling the New-project dialog's clone URL with an embedded
`user:token@host` credential must surface "Remove the token from the URL"
and keep the Create button disabled — never a project actually created,
never can auto-SKIP — plus two more checks added 2026-09-02 for the
New-project dialog's clone-name derivation (operator expectation: "clone a
repo = use the same name as the repo"): the
name field derives from the repo URL the moment "Clone a repo" is picked and
a URL is typed, and a name the user types by hand overrides the derived one
even when the URL then changes — both assert against the open dialog and
close it with Escape rather than creating a project, so neither can
auto-SKIP — plus three more checks added 2026-09-02, "A clone that fails
must SAY SO, not present as an empty project" (the 2026-09-02 outage was
silent: provisioning used to run before the server had joined the project's
bridge, so every clone failed for want of credentials and the UI showed a
normal, empty project): these create a real project via the API with a
deliberately unreachable `cloneUrl`
(`https://github.com/kesokaj/e2e-no-such-repo-styrman.git`) and boot a real
microVM to prove the whole failure path — `GET /api/projects` eventually
carries a clone-related `provisionError` for it, the sidebar's red "repo
setup failed" badge appears, and the workspace still has a `.git` entry (R13:
the fallback init ran even though the clone didn't) — then clean up after
themselves (`DELETE .../{id}?wipe=true`) — plus one more check added
2026-09-02, "successful clone fills the workspace": the happy path, which
nothing above exercised. Creates a real project with
`cloneUrl: https://github.com/octocat/Hello-World.git`, boots a real
microVM, and asserts the workspace ends up with entries beyond `.git` (the
R13 fallback init leaves ONLY `.git`, so "more than `.git`" is what
distinguishes a real clone from the fallback) — then cleans up
(`DELETE .../{id}?wipe=true`). SKIPs rather than fails when the guest has no
route to github (its `provisionError` matches `could not resolve host`/
`unable to access`/`connection refused`/`connection timed out`). **The repo
this check clones from MUST be public**: `web/e2e/run.sh` runs the server in
HOST MODE (`STYRMAN_SELF_CONTAINER` unset), where there is no credential
broker by design — `EnsureSelfAttached` no-ops and `waitForBroker` returns
false immediately when `cfg.SelfContainer == ""`
(`internal/httpapi/provision.go`). A first attempt at this check used the private
`kesokaj/styrman-harness` and failed, correctly, with `fatal: could not read
Username for 'https://github.com'`. This is a hard limit of Tier 4, not a bug
to fix here: **the e2e tier can never test credentialed clones — only the
attach-before-provisioning ORDERING** — see `provision.go`'s doc comment and
`guide.md`'s "Project creation is async" bullet for the credential-delivery
path itself, which only Tier 3 (real Kata, real broker) or manual testing
against a containerized server can exercise — and full
cleanup — `passed=87 failed=0 skipped=2` on this host, plus a final
no-client-errors gate + screenshots. The live proof that a project's broker
is reachable (BUG-060) must start from a project already STOPPED across a
server restart, opened through the UI's own terminal — not one just created
or started via the API, which would already have attached in its own
background goroutine and hide the bug the boot-before-upgrade fix exists for.
Prereq: `cd web && npm i && npx playwright install chromium` (once).
Self-cleaning: deletes any leftover `e2e-ui` project at start.
To point the Playwright checks at an already-running server instead, call
`node web/e2e/run.mjs --base <the suite's own server URL> [--shots <dir>]` — `--base` is required since v1.42.3 (the old default was production's own listener on this host); without `--shots` screenshots go to a private `mkdtemp` dir, and `run.sh` deletes its screenshot dir after a green run (printing "screenshots removed") and prints the dir's path after a failed one, keeping it as evidence. `run.mjs` also refuses a `STYRMAN_UPDATE_STATUS_FILE` outside `STYRMAN_E2E_STATE` (the run's state dir), because its Settings check writes and removes that file. `run.sh`'s exit cleanup removes only networks whose project UUID its own server logged.
directly.

## Tier 5 — Agent harness in the image (`/styrman/bin/sh-agent`, ADR-074)

The image carries the `styrman-harness` RELEASE binary named by
`image/Dockerfile`'s `ARG HARNESS_VERSION`, downloaded from GitHub Releases
at build and verified against the harness's signer; the entrypoint seeds
every project home with `sh-agent install` (write-once, keyed on
`/styrman/VERSION` plus the binary's release), after
`image/configs/harness-migrate.sh` has moved a previous harness's artifacts
to `~/.styrman/harness-backup-<ts>/`, and never over a home whose manifest
names a newer release (the user's `sh-agent update` inside the project).
Fires on every `HARNESS_VERSION` bump and every `STYRMAN_VERSION` bump.

Automated (Tier 2, `integration.sh`, inside the booted project, plus the
browser checks added 2026-09-11 for ADR-068 — guest side: the
`styrman-browser` MCP server is registered in `~/.claude.json` and
`~/.gemini/config/mcp_config.json` (both asserted with `jq` on the command
field, agy's also on `disabled == false`); `styrman-browser start` as the
project user answers `/json/version` with Chromium's sandbox on — the check
reads the live process's cmdline and fails on `--no-sandbox`; `stop` leaves
`status` reporting not running; server side (Phase 2): the ports list omits
9229 while a bare-exec'd browser listens (loopback-only by design without a
peer), `/proxy/{id}/9222/` and the share PUT answer 400, and the owner's
status/start/stop routes round-trip with the 9222 forwarder up and hidden):
the image's `sh-agent version` is a canonical `vX.Y.Z`; the home's
`~/.local/bin/sh-agent` reports the same release; `~/.config/sh-agent/state.json`
records `"repo": "embedded:vX.Y.Z"` (the shape the entrypoint's never-downgrade
guard reads); `sh-agent status` from the home's binary exits 0 with no drift;
the seeded skill and agy-agent counts equal the binary's own `sh-agent list`;
the boot log carries `agent harness deployed (version <image VERSION>,
sh-agent vX.Y.Z;` — install's own stdout is kept only for its `settings:`
lines, which the boot log carries; the `agent harness deployed` line remains
the in-guest proof install succeeded; `~/.claude/CLAUDE.md` is the two-line
import stub; none of `~/.agents/bin`, `~/.gemini/GEMINI.md`;
`~/.claude/settings.json` holds exactly the `attribution` key (one
`"attribution"` match) and nothing else the harness wrote — the boot log's
`settings: /home/<user>/.claude/settings.json created with …` line and the
`jq -e` assertion on the guest file are both Tier 2 checks (ADR-072); the
persistence-section checks (BUG-062) separately prove that when a
non-regular file sits at `settings.json` at recreate time, the boot log
instead carries `note: /home/<user>/.claude/settings.json is not a regular
file, so the attribution setting could not be checked` and the seed still
completes around the refused key; the stop+delete section's never-downgrade
case plants `embedded:v99.0.0` into the stopped project's manifest from the
host, clears the stamp, boots, and asserts the `left alone` log line, a
`kept` pass line with no new `deployed` line, the binary and manifest
untouched and the stamp rewritten; boot→ready under
R1's 15 s hard limit from the container's own timestamps (the value is
printed — watch it against the 5 s soft target; ~2.7 s measured 2026-09-01
on the old harness; v2.3.2's `sh-agent install` takes 10.9 s on the host's ext4
and 12.8 s in a microVM on a disk-backed home and runs AFTER ready, ADR-078;
boot to ready measured between 3.01 s and 3.23 s on ext4 on 2026-10-02).

Manual, on every `HARNESS_VERSION` bump:

1. Image contents: `docker run --rm --entrypoint /styrman/bin/sh-agent
   <image> version` prints `sh-agent <HARNESS_VERSION>` (what
   `deploy/verify-commit.sh` asserts on the release path);
   `docker run --rm --entrypoint /styrman/bin/sh-agent <image> list` shows
   the release's `Skills (N)` and `Subagents (N)`; `docker run --rm
   --entrypoint sh <image> -c 'ls /styrman/configs/harness-migrate.sh'`.
2. Guard controls, once per Dockerfile change: a build with
   `--build-arg HARNESS_SIGNER='<a wrong key line>'` fails at `ssh-keygen -Y
   verify` (`Could not verify signature.`); a build with
   `--build-arg HARNESS_VERSION=<an older published release>` builds and its
   `version` prints that release.
3. Old-home migration on the host, no VM (the exact procedure, with the
   asserted outcomes, is `testing/2026-09-03-verify-harness-phase2.md`): seed
   a scratch home with `git archive v1.19.1 image/harness`'s `setup.sh`,
   stamp `~/.styrman/.harness-version`, run `harness-migrate.sh`, then
   `sh-agent install` through the image; every old path is under one
   `~/.styrman/harness-backup-*`, none remains, planted user files are
   byte-identical, a second migrate run is a no-op.
4. Agent-CLI smoke inside a fresh project (needs the operator's sign-in):
   `claude -p "What is the first heading of your global context, and how
   many skills do you have?"` quotes `# AGENTS.md — global agent context`
   and 36 (the harness roster in the image: 36 skills, 8 subagents, v2.3.2); `agy` reports the rules file `~/.gemini/config/rules/AGENTS.md`.
5. Real re-seed evidence, when a stopped project seeded by ≤ v1.19.x is
   available: start it on the new image and read the entrypoint log line
   `agent harness deployed (version …, styrman-harness <sha>)` plus the
   `[harness-migrate] moved N path(s)` lines; `docker cp` the backup dir
   listing out WITHOUT modifying the container further.

Retired with the old harness: the `diff -rq` against `devshell_prompt`, the
`CLAUDE.md`/`GEMINI.md` line counts, the `~/.agents/bin` gates, the old
`settings.json` JSON check (replaced by the ADR-072 attribution-key
assertion, Tier 2), the ADR-047 latency guard (its regression class
— a blocking CLI call on the boot path — is now caught by the Tier 2
boot→ready assertion), and the Stop-hook safety check (no hooks ship).
