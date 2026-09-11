# Testing styrman

Local-first: everything runs on this machine against real Docker + Kata.
No mocks in the orchestrator path.

## Tiers

1. **Unit** — `go build ./...`, `go vet ./...`, `go test -race ./...` (store, validation; fast, no Docker). The vendored Caddy ingress is a SEPARATE Go module and is not covered by the root `go test ./...` run — test it explicitly: `(cd deploy/caddy/ingress && go test ./...)`. `internal/orchestrator` now carries its own unit tests (TODO close-out 2026-09-10, item O) through a call-recording fake Docker client (`fakedocker_test.go`, `dockerAPI` interface, following `guestexec_test.go`'s fake style) with a scriptable per-method error and an in-memory container/network table — `ensureRunningLocked` (wrong-bridge recreate, network-inspect refusal, symlinked-home refusal, a running container left alone), the `WithProject`+handler stop/persist/start ordering, `stopMarked`+`unmarkIfStillRunning`'s two-sample marker logic, `Status`, and `currentBridge`, all without a real Docker daemon. Frontend: `cd web && npm run check` (tsc -b), `npm run lint` (oxlint), and `npm test` (`vitest run` — added 2026-09-02; `web` previously had no unit-test framework at all) — 147 tests across eleven files: `web/src/lib/lan.test.ts` (8 tests, rewritten for ADR-067's two-state model — `off`/`pending`/`error`/`unavailable` are gone, so this file no longer tests them: `lanChip` renders nothing for a sandboxed or field-absent project and flags "LAN" with warn colours when on; `lanLine` states what LAN mode grants and its limits, and that sandbox is internet-only; three for `preferOverride`, BUG-020 — the optimistic PUT reply is shown until the poll CATCHES UP with it, an ordering comparison on the one `lanAccess` boolean rather than the timestamp-equality test that used to latch the component to a stale reply forever once anything changed the record out of band — the latch is now structurally impossible since there is no clock to compare), `web/src/lib/api.test.ts` (`projectNameFromRepoURL`, `cloneURLError`, `projectNameError` — pure, security- or UX-load-bearing string logic that was the frontend's entire test-coverage story before this), `web/src/lib/api.encoding.test.ts` (added 2026-09-09, closing the security review of 2026-09-03, round 2: every `api.ts` call that interpolates a caller-supplied value into a REST or WS path is exercised with a path-climbing (`../..`) and a query-injecting (`?wipe=true`) value, asserting the RESOLVED path never leaves its intended prefix and never contains a bare `..` segment — written against the call sites rather than the `u` template tag itself, so a new call site that forgets to use it fails the suite instead of a test of the tag passing regardless; also covers `termSocketUrl`'s project/terminal-id encoding and that legitimate query strings built outside the tag still work), `web/src/lib/editorPane.test.ts` (`basename`), `web/src/lib/diffReveal.test.ts` (8 tests, v1.22.0: `DiffView`'s `reveal` makes bidi overrides/isolates, invisible directional marks and C0/C1 controls VISIBLE as `<U+202E>`-style markers instead of passing them through — the diff pane's job is reviewing a change before committing it, and a Trojan-Source line renders differently from how it applies; TAB is deliberately kept), `web/src/lib/recentsPanel.test.ts` (14 tests, v1.22.0: the Recent strip's persisted geometry — height clamped to 48..320 on the way IN and OUT so a hand-edited key cannot collapse the panel to nothing, a stored string parsed since localStorage holds only strings, the collapsed flag REMOVED rather than set to "0" when expanded, and a private window throwing on access), `web/src/lib/explorerHistory.test.ts` (13 tests, v1.22.0: the file explorer's back/forward reducer — the forward tail dropped on a new navigation, object IDENTITY preserved on a redundant push so it cannot re-render or refetch, the 50-entry cap dropping oldest-first while keeping the current entry reachable, and `crumbTrail`'s middle elision keeping each shown crumb's REAL full path rather than an offset into the shortened list), `web/src/lib/appearance.test.ts` (17 tests, v1.22.0: the Settings font store — the two surfaces' differing defaults, per-FIELD fallback so one bad value never discards the others, size clamping to 8..32, a numeric string and a prototype key (`"toString"`) both rejected as font families, corrupt JSON, and a private window whose `localStorage` throws on ACCESS rather than only on write; plus a guard that `FONT_STACKS["google-sans-code"]` is still byte-identical to `terminal.ts`'s historical stack, since a dropped fallback silently changes what renders), `web/src/lib/api.session.test.ts` (6 tests, BUG-001: `request()`/`uploadFiles` against a stubbed browser facing the ingress's cross-origin session-expiry redirect — re-auth once, the reload cap under partial failure, the deferred counter clear, malformed and corrupt counter records), `web/src/lib/sshKey.test.ts` (added 2026-09-10, TODO close-out item H: `\p{C}` control/formatting rejection — RLO, ZWSP, BOM, U+00AD — and `abbreviateKey`'s `\p{Zs}` space-run collapse, plus the full-OpenSSH-type-token enum matching the server's `publicKeyLine`), and `web/src/lib/projectBadge.test.ts` (added 2026-09-10, item 29: the pure `projectBadge(p)` function's precedence over `deleteError`/`provisionError`/`deleting`/`provisioning`, extracted out of `Sidebar.tsx` so the badge choice is unit-tested rather than left to the component). `editorPane.test.ts` also covers the pane store's `discard(req)` (item K9): the parked draft and dirty flag clear and the loaded doc drops on discard. Deliberately NOT unit-tested: the editor pane store's per-terminal draft behavior (parked on unmount, restored only for the file it belongs to, dropped when that file closes or is evicted) — its state lives in a module-level `Map` that is not exported, so reaching it from a unit test would mean either widening the module's API just for tests or pulling in a React renderer; `editorPane.test.ts`'s own header comment says so explicitly rather than faking coverage with an assertion that would always pass. That behavior is covered by Tier 4 e2e instead. Outside these two files, most TypeScript/React logic is still covered only by type-checking + lint plus, where applicable, Tier 4 e2e — a changed function/component with no unit test and no e2e exercise has no automated coverage at all, note it explicitly when verifying.
2. **Integration (93 checks; 3 auto-SKIP on firewall-hardened hosts — a green run against a CURRENT runtime image prints `passed=90 failed=0 skipped=3`. Pass the image explicitly: the default `styrman-runtime:test` goes stale and its pre-v1.22.0 skel fails three GOPATH checks for image reasons, not code reasons)** — `bash .agents/docs/testing/styrman/integration.sh [image]` (port `STYRMAN_ITEST_PORT`, default 18080; the run REFUSES to start when that port is already bound, since its health probe cannot tell its own server from another — BUG-040, 2026-09-11)
   (default `styrman-runtime:test` — see the warning under Prereqs about
   `:dev`). Isolated server (own state dir, port
   18080) through the full lifecycle: legacy-layout migration, async create
   → microVM boot, terminals (records, rename, pause, close-all),
   WS I/O + reattach, transcripts, file explorer (incl. traversal guards),
   ports + all preview URL shapes, the in-guest git client (status + diff),
   sticky stop (close 4002 refusal vs `?boot=1` revive), settings,
   persistence across container destruction, UUID identity + friendly
   rename, wipe, and (since ADR-062) the seeded agent harness: `sh-agent
   status` no drift, the two-line `CLAUDE.md`, 28 skills / 7 agy agents, no
   previous-harness artifacts, and boot→ready within R1's 15 s hard limit
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
     shows "replaced" (4001) and does NOT reconnect
   - type `exit` in a shell conversation → "session exited" (4000)
   - resize the window → TUI reflows (tmux + ContainerExecResize)
4. **Browser e2e (Playwright)** — see the Tier 4 section below.
5. **Vendored agent harness (`image/harness/`)** — see the Tier 5 section
   below: the in-guest assertions are Tier 2 checks since ADR-062; what
   remains manual is the mirror refresh itself and the agent-CLI smoke.
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
  image is missing or `image/` (excluding `image/harness/.git`) has a file
  newer than the image's `Created` timestamp, so a stale `:test` tag can
  never masquerade as a source regression (BUG-026). `STYRMAN_TEST_IMAGE_NO_REBUILD=1`
  skips the check entirely for fast iteration. It refuses to rebuild a
  production tag (`deploy/.env`'s `STYRMAN_IMAGE`, or `styrman-runtime:dev`/
  `*:latest` as a fallback guess) — see the `:dev` warning below — and a
  failed build is FATAL, so no check ever runs against an image the script
  could not refresh. You do not need to build the image by hand before
  running either suite; the line below is for building it standalone.
- `styrman-runtime` image built: `docker build -t styrman-runtime:test --build-arg STYRMAN_VERSION=test-$(date +%s) image/`

  **Do NOT build or tag `styrman-runtime:dev` on this host until v1.16.1
  deploys.** As of v1.16.0, `deploy/.env` on this host still sets
  `STYRMAN_IMAGE=styrman-runtime:dev` — a leftover from before the
  2026-09-01 "no mutable tags" operator rule — so `:dev` is STILL the exact
  tag production boots; rebuilding it re-stamps what's running in prod. This
  happened for real during the 2026-09-01 audit (the marker read
  `dev-1788269597` afterward; restored to `1.16.0`). v1.16.1 closes the
  mutable-tag path structurally (`config.Load` requires `STYRMAN_IMAGE`,
  compose derives `styrman-runtime:${STYRMAN_VERSION}` by default — see
  `.agents/docs/specs/styrman/versioning.md`) — once that release deploys
  and the live `deploy/.env`'s `STYRMAN_IMAGE` override is cleared,
  production boots `styrman-runtime:X.Y.Z` and `:dev` reverts to being a
  purely local, never-deployed dev-loop tag. **Until v1.16.1 is confirmed
  live, treat this warning as fully binding** — re-verify before assuming
  otherwise. An immutable `styrman-runtime:1.16.0` release tag also exists
  if you need a known-good image to compare against. Use `:test` (or any
  tag other than `:dev`) for everything in this guide.
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

## Tier 4 — Browser e2e (Playwright; a green run on this host prints `passed=91 failed=0 skipped=2` plus a final client-error gate. Four Browser-view checks (ADR-068, 2026-09-11) run INSIDE the end-of-run LAN window — between "enabling rebuilds the project" and "disabling returns it to sandbox" — because a host-run server cannot reach a sandbox bridge on this firewall-hardened host and the relay to the in-guest Chromium has the same shape as the auto-SKIPped preview checks: the Browser pill opens the view and a screencast frame arrives (`canvas[data-frames]`), a typed `localhost:4100` navigates with the tab strip carrying the fixture's title, "browser" on the Ports row switches the view and navigates, and the terminal session survives the round-trip. ADR-067 replaced ALL of the old ADR-065/066 LAN checks — the host-half `pending`/`unavailable`/error states, the scratch `STYRMAN_FIREWALL_DIR` stub, the planted fake root status file — with five checks against the two-state model, since there is no host half left to stub: "network block shows the sandbox default" (a new project's Ports-tab "Network" switch reads unchecked/"Sandboxed.", enabled for this admin user), "the API reports sandbox and says the caller may enable" (`GET .../network` is `{lanAccess:false, canEnable:true}`), "no LAN badge on a sandboxed project" (`[data-lan="on"]` renders nowhere — the assertion tsc cannot make, since the badge renders from an optional field and a regression would silently claim every project is on the LAN), and two TOGGLE checks held to the END of the run rather than inline, because applying the switch rebuilds the project's container (ADR-067's accepted cost) and destroys everything a later check in the same project needs (the fixture app, tmux sessions, git working state): "enabling flips the record and shows the LAN badge" and "disabling returns it to sandbox". None of this is stubbed or root-owned any more, so unlike ADR-065/066 there is nothing here that ONLY a hand-walked check against a converged production host could exercise — except the operator's own criteria (LAN reaches the host/LAN/published ports; sandbox does not), which stay a manual walk-through, logged in `testing/`, because Playwright's own host has no real LAN to prove reachability against. v1.22.0 added six appearance checks, two path-navigation checks, three recents-strip checks, three git-diff/fast-commit checks and six git-verb checks (branch create, branch SWITCH — a different argv shape that runs `checkout --end-of-options`, invalid-name rejection, fetch/pull controls reachable, commit history into the pane, stash push -u / pop). The client-error gate records the failing URL alongside the browser's message, which otherwise names neither endpoint nor caller; re-measure after any further check additions before citing a pass count as current)

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
no-client-errors gate + screenshots.
Prereq: `cd web && npm i && npx playwright install chromium` (once).
Self-cleaning: deletes any leftover `e2e-ui` project at start.
To point the Playwright checks at an already-running server instead, call
`node web/e2e/run.mjs [--base http://127.0.0.1:8080] [--shots /tmp/styrman-shots]`
directly.

## Tier 5 — Vendored agent harness (`image/harness/`)

The image carries a verbatim mirror of `styrman-harness` at the commit in
`image/harness/COMMIT` and the `sh-agent` binary built from it; the
entrypoint seeds every project home with `sh-agent install` (write-once,
keyed on `/styrman/VERSION`), after `image/configs/harness-migrate.sh` has
moved a previous harness's artifacts to `~/.styrman/harness-backup-<ts>/`
(ADR-062; plan `plans/2026-09-03-image-harness-replacement.md`). Fires on
every mirror refresh and every `STYRMAN_VERSION` bump.

Automated (Tier 2, `integration.sh`, five checks inside the booted project,
plus six browser checks added 2026-09-11 for ADR-068 — guest side: the
`styrman-browser` MCP server is registered in `~/.claude.json` and
`~/.gemini/config/mcp_config.json` (both asserted with `jq` on the command
field, agy's also on `disabled == false`); `styrman-browser start` as the
project user answers `/json/version` with Chromium's sandbox on — the check
reads the live process's cmdline and fails on `--no-sandbox`; `stop` leaves
`status` reporting not running; server side (Phase 2): the ports list omits
9229 while a bare-exec'd browser listens (loopback-only by design without a
peer), `/proxy/{id}/9222/` and the share PUT answer 400, and the owner's
status/start/stop routes round-trip with the 9222 forwarder up and hidden):
`sh-agent status` exits 0 with no drift; `~/.claude/CLAUDE.md` is the
two-line import stub; 28 skills and 7 agy agents; none of `~/.agents/bin`,
`~/.gemini/GEMINI.md`, `~/.claude/settings.json`; boot→ready under R1's 15 s
hard limit from the container's own timestamps (the value is printed — watch
it against the 5 s soft target; ~2.7 s measured 2026-09-01 on the old
harness, the new install is ~0.02 s on the host).

Manual, on every mirror refresh:

1. Mirror fidelity: `diff -rq --exclude .git --exclude .agents --exclude
   .gitignore ~/workspace/styrman-harness image/harness` → exactly
   `Only in image/harness: COMMIT` and `Only in image/harness: VENDORED.md`;
   `cat image/harness/COMMIT` equals `git -C ~/workspace/styrman-harness
   rev-parse HEAD` (the refresh command is in `image/harness/VENDORED.md`).
2. Image contents: `docker run --rm --entrypoint /styrman/bin/sh-agent
   <image> list --repo /styrman/harness` shows `Skills (28)` and
   `Subagents (7)`; `docker run --rm --entrypoint sh <image> -c 'cat
   /styrman/harness/COMMIT; ls /styrman/configs/harness-migrate.sh'`.
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
   and 28; `agy` reports the rules file `~/.gemini/config/rules/AGENTS.md`.
5. Real re-seed evidence, when a stopped project seeded by ≤ v1.19.x is
   available: start it on the new image and read the entrypoint log line
   `agent harness deployed (version …, styrman-harness <sha>)` plus the
   `[harness-migrate] moved N path(s)` lines; `docker cp` the backup dir
   listing out WITHOUT modifying the container further.

Retired with the old harness: the `diff -rq` against `devshell_prompt`, the
`CLAUDE.md`/`GEMINI.md` line counts, the `~/.agents/bin` gates, the
`settings.json` JSON check, the ADR-047 latency guard (its regression class
— a blocking CLI call on the boot path — is now caught by the Tier 2
boot→ready assertion), and the Stop-hook safety check (no hooks ship).
