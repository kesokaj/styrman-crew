# Testing styrman

Local-first: everything runs on this machine against real Docker + Kata.
No mocks in the orchestrator path.

## Tiers

1. **Unit** — `go test -race ./...` (store, validation; fast, no Docker).
2. **Integration (60 checks; 3 auto-SKIP on firewall-hardened hosts)** — `bash .agents/docs/testing/styrman/integration.sh [image]`
   (default `styrman-runtime:dev`). Isolated server (own state dir, port
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

## Prereqs

- `styrman-runtime` image built: `docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/`
- Kata runtime `mvm` working: `docker run --runtime mvm --rm debian:trixie uname -r`
  (host-setup.sh verifies this; if it fails with "invalid namespace type" see
  ADR-009 — the daemon.json time-namespaces flag is missing)

## Rebuild-and-retest loop

```bash
go test ./... && bash .agents/docs/testing/styrman/integration.sh
```

Image changes: rebuild the image first; the integration script always creates
fresh containers so it picks the new image up automatically.

## Tier 4 — Browser e2e (Playwright, 49 checks + a final client-error gate; firewall-hardened hosts run 48 + 2 auto-SKIP)

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
on an empty pane, deleting a file that is open in the pane, and syntax
checking — a broken shell script flags a lint-gutter error, a clean file
stays clean, and the check/format temp file leaves no litter in the
workspace listing), and full cleanup — 49 checks + the client-error gate (48 pass on firewall-hardened
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
