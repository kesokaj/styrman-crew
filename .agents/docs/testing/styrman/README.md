# Testing styrman

Local-first: everything runs on this machine against real Docker + Kata.
No mocks in the orchestrator path.

## Tiers

1. **Unit** — `go build ./...`, `go vet ./...`, `go test -race ./...` (store, validation; fast, no Docker). The vendored Caddy ingress is a SEPARATE Go module and is not covered by the root `go test ./...` run — test it explicitly: `(cd deploy/caddy/ingress && go test ./...)`. Frontend: `cd web && npm run check` (tsc -b), `npm run lint` (oxlint), and `npm test` (`vitest run` — added 2026-09-02; `web` previously had no unit-test framework at all) — 11 tests across two files: `web/src/lib/api.test.ts` (`projectNameFromRepoURL`, `cloneURLError`, `projectNameError` — pure, security- or UX-load-bearing string logic that was the frontend's entire test-coverage story before this) and `web/src/lib/editorPane.test.ts` (`basename`). Deliberately NOT unit-tested: the editor pane store's per-terminal draft behavior (parked on unmount, restored only for the file it belongs to, dropped when that file closes or is evicted) — its state lives in a module-level `Map` that is not exported, so reaching it from a unit test would mean either widening the module's API just for tests or pulling in a React renderer; `editorPane.test.ts`'s own header comment says so explicitly rather than faking coverage with an assertion that would always pass. That behavior is covered by Tier 4 e2e instead. Outside these two files, most TypeScript/React logic is still covered only by type-checking + lint plus, where applicable, Tier 4 e2e — a changed function/component with no unit test and no e2e exercise has no automated coverage at all, note it explicitly when verifying.
2. **Integration (60 checks; 3 auto-SKIP on firewall-hardened hosts)** — `bash .agents/docs/testing/styrman/integration.sh [image]`
   (default `styrman-runtime:test` — see the warning under Prereqs about
   `:dev`). Isolated server (own state dir, port
   18080) through the full lifecycle: legacy-layout migration, async create
   → microVM boot, terminals (records, rename, pause, close-all),
   WS I/O + reattach, transcripts, file explorer (incl. traversal guards),
   ports + all preview URL shapes, the in-guest git client (status + diff),
   sticky stop (close 4002 refusal vs `?boot=1` revive), settings,
   persistence across container destruction, UUID identity + friendly
   rename, wipe. Safe next to a real install. (The old host-side `/diff` +
   checkpoint checks were replaced with `/git/status` + `/git/diff` checks
   when those routes were removed in the git-RCE fix, ADR-039.)
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
   below. NOT automated anywhere (integration.sh and the e2e suite have
   zero harness assertions) — run by hand on every harness sync.
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

## Tier 4 — Browser e2e (Playwright, 62 checks + a final client-error gate; firewall-hardened hosts run 60 + 2 auto-SKIP. A 2026-09-02 measured run — `passed=60 failed=0 skipped=2` against `styrman-runtime:test`, the first session this tier was actually executed rather than counted — matches the current 62-check total; re-measure after any further check additions before citing it as current)

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
workspace listing), plus two mutation-guard checks added 2026-09-02 that
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
cleanup — 62 checks + the client-error gate (60 pass on firewall-hardened
hosts, plus 2 auto-SKIP) plus a final no-client-errors gate + screenshots.
Prereq: `cd web && npm i && npx playwright install chromium` (once).
Self-cleaning: deletes any leftover `e2e-ui` project at start.
To point the Playwright checks at an already-running server instead, call
`node web/e2e/run.mjs [--base http://127.0.0.1:8080] [--shots /tmp/styrman-shots]`
directly.

## Tier 5 — Vendored agent harness (`image/harness/`), manual — KNOWN GAP: not automated

Fires whenever `image/harness/` is re-synced from `~/workspace/devshell_prompt/harness`
and/or `STYRMAN_VERSION` is bumped (ADR-046: the tree is a verbatim mirror,
one delivery path — version bump + image rebuild). No step here is wired
into `integration.sh` or `web/e2e` today; derive it fresh from
`image/entrypoint.sh:110-135` each time until it is automated.

1. Mirror fidelity: `diff -rq image/harness ~/workspace/devshell_prompt/harness`
   → must report ONLY `VENDORED.md` differing.
2. Image contents match `image/harness/VENDORED.md`'s stated roster:
   `docker run --rm --entrypoint bash <image> -c 'cat /styrman/VERSION;
   wc -l /styrman/harness/shared/AGENTS.md; ls /styrman/harness/claude/agents/;
   ls -d /styrman/harness/shared/skills/*/ | wc -l'`.
3. Clean-home deploy — mirrors `image/entrypoint.sh:110-135` exactly
   (write-once seed, keyed on `/styrman/VERSION`): inside the image,
   `useradd -m` a throwaway user, `mktemp -d` a user-owned copy of
   `/styrman/harness` (setup.sh needs a writable tree), then
   `runuser -u <user> -- env HOME=<home> PATH=/usr/bin:/bin:/usr/sbin:/sbin
   bash <copy>/setup.sh`. Assert exit 0, matching `CLAUDE.md`/`GEMINI.md`
   line counts, matching skill count under BOTH `~/.claude/skills/*/` and
   `~/.gemini/config/skills/*/` (top-level directory count — recursive
   `SKILL.md` search over-counts nested reference files with the same
   name), the agent roster under `~/.claude/agents/`, both gates under
   `~/.agents/bin/`, and valid JSON in `~/.claude/settings.json`.

   **3a. Latency assertion (ADR-047 regression guard) — time the same
   command:**

   ```bash
   time runuser -u <user> -- env HOME=<home> PATH=/usr/bin:/bin:/usr/sbin:/sbin \
       bash <copy>/setup.sh >/dev/null 2>&1
   ```

   Must complete in **~1s, not ~60s** (measured 2026-09-01: **456ms**
   trimmed PATH vs **60,900ms** default PATH, byte-identical deploy either
   way — see ADR-047 and `runtime-image.md` step 6). This guards against
   the exact regression class Tier 5 exists for: `agy`'s
   `antigravity/setup.sh` self-check (`agy -p "/skills"`, gated only by
   `command -v agy`) blocks for a fixed 60s on a home with no agy OAuth
   token — i.e. every fresh project — and that call sits ahead of
   `/run/styrman/ready` on the container boot path. The trim only works
   because `agy` isn't reachable under `/usr/bin:/bin:/usr/sbin:/sbin`; if
   a future `image/Dockerfile` change or an upstream `setup.sh` edit moves
   `agy` (or adds another blocking call) onto that trimmed PATH, this is
   the only check that will catch it — `go test`, `integration.sh`, and
   `web/e2e` are all blind to harness timing. Also assert the guard
   condition directly: `runuser -u <user> -- env
   PATH=/usr/bin:/bin:/usr/sbin:/sbin command -v agy` must find nothing.

   **3b. End-to-end, optional but closer to what R1 actually gates:**
   create a throwaway project via the API, then compute the gap between
   `docker inspect -f '{{.State.StartedAt}}' <container>` and the
   timestamp on the `project <uuid> ready` line from
   `docker logs -t <container>`. Must be under R1's 5s soft target and
   well under its 15s hard limit (measured 2026-09-01: **2.69s** cold
   create end-to-end). Delete the throwaway project afterwards.
4. Stop-hook guest-session safety: `bash
   /styrman/harness/shared/scripts/trail-check.sh` (no args = hook mode,
   as Claude Code's Stop hook invokes it) must exit 0 both outside any git
   repo and inside a git repo with no `.agents/docs/changes/` dir — a repo
   that never opted into the harness contract must never be blocked.
5. Real re-seed evidence, if a stopped project container seeded from an
   older version is available: `docker cp <container>:<home>/.styrman/.harness-version -`
   plus the same CLAUDE.md/agents/skills checks as step 3, WITHOUT
   starting or otherwise modifying the container (`docker cp` reads the
   stopped filesystem layer directly).

**Known gap (2026-09-01):** none of the above is scripted. A future
harness sync that breaks `setup.sh`, drops a skill, or ships a hook that
hangs a guest session would pass `go test`, `integration.sh`, and
`web/e2e` cleanly. A `harness-check.sh` implementing steps 2–4 above
belongs in this directory or as an `integration.sh` step.
