# Verification — harness-upgrade-v1.15.0   (2026-09-01)

## Status: PASS

Independent re-run of every check in the release claim (vendored harness
re-sync from `~/workspace/devshell_prompt/harness` @ `6132b97`, runtime
image `styrman-runtime:dev` rebuilt with `STYRMAN_VERSION=1.15.0`, server
deployed at 1.15.0). No numbers were taken on trust from the requester; all
commands below were re-run fresh by this agent. One real gap is called out
at the bottom (informational, not a FAIL condition on its own — see
judgement section).

## Commands run

1. **Mirror fidelity**
   `diff -rq image/harness /home/kesokaj/workspace/devshell_prompt/harness`
   → `Only in image/harness: VENDORED.md`. Exactly the one allowed
   exception. PASS.

2. **Go regression**
   - `go build ./cmd/styrman/` → exit 0, no output.
   - `go vet ./...` → exit 0, no output.
   - `go test ./... -race` → all packages `ok` (cmd/styrman, internal/backup,
     config, conversation, credbroker, credstore, files, gitrepo, history,
     httpapi, links, orchestrator, project, terminal, userstore); `cmd/wsprobe`,
     `internal/identity`, `web` report `[no test files]` (pre-existing, not
     part of this change). PASS.
   - Separate module `deploy/caddy/ingress` (module
     `github.com/spgo/devbox/caddy/ingress`, NOT covered by the root
     `go test ./...`): `(cd deploy/caddy/ingress && go test ./...)` → `ok`
     (2.028s). PASS.

3. **Frontend** — `web/package.json` scripts are `dev, build, lint, preview,
   check`; no `test` script, no vitest/jest in `devDependencies`. Nothing to
   run. Consistent with the claim that this release touches no web source.

4. **Image contents**
   `docker run --rm --entrypoint bash styrman-runtime:dev -c 'cat
   /styrman/VERSION; wc -l /styrman/harness/shared/AGENTS.md; ls
   /styrman/harness/claude/agents/; ls -d /styrman/harness/shared/skills/*/
   | wc -l'`
   → `1.15.0` / `69 /styrman/harness/shared/AGENTS.md` / 6 agents
   (`changelog.md documentation.md explorer.md reviewer.md security.md
   verifier.md` — no `testing.md`) / `28`. Exact match to spec. PASS.

5. **Harness deploy in a clean home** — mirrored `image/entrypoint.sh:110-135`
   exactly: created a throwaway non-root user (`useradd -m`) inside a
   `styrman-runtime:dev` container, `mktemp -d` a user-owned copy of
   `/styrman/harness`, ran `runuser -u <user> -- env HOME=<home> bash
   <copy>/setup.sh`.
   - Exit code: 0 (script guarded with `set -e`, output printed after —
     confirms non-failure).
   - `~/.claude/CLAUDE.md`: 69 lines. `~/.gemini/GEMINI.md`: 69 lines.
   - Skills: `ls -d ~/.claude/skills/*/ | wc -l` → 28;
     `ls -d ~/.gemini/config/skills/*/ | wc -l` → 28. (First pass used
     `find -iname SKILL.md` recursively and over-counted at 33 for gemini —
     that was a flaw in my own check, not the product; corrected to the same
     top-level-directory-count methodology used for `.claude` and the image
     spec, and both land on 28.)
   - Agents: `~/.claude/agents/` → `changelog.md documentation.md
     explorer.md reviewer.md security.md verifier.md` (6, no testing.md).
   - Gates: `~/.agents/bin/{resume-context.sh,trail-check.sh}` present,
     executable.
   - `~/.claude/settings.json`: parsed with `python3 -c "json.load(...)"` →
     `VALID JSON`.
   All PASS.

6. **Stop-hook safety (guest-session blast radius)**
   `bash /styrman/harness/shared/scripts/trail-check.sh` (no args = hook
   mode), run as a non-root user inside `styrman-runtime:dev`:
   - Case A, `cwd=/tmp` (not a git repo at all): exit 0.
   - Case B, freshly `git init`'d repo with one commit and no `.agents/docs`
     (hence no `changes/` dir): exit 0.
   Both PASS — the hook cannot break a guest session that hasn't opted into
   the harness's DoD trail.

7. **Live deploy**
   - `docker ps` → `styrman-server` `Up 8 minutes (healthy)`.
   - `docker run --rm --network styrman-ingress curlimages/curl -s -H
     "X-Styrman-Ingress: $STYRMAN_INGRESS_TOKEN" -H "X-Caddy-Email:
     ostling.simon@gmail.com" http://styrman-server:8080/api/version`
     (token read from `deploy/.env`, never printed) → `{"version":"1.15.0"}`.
   - `curl -sD - -o /dev/null https://crew.styrman.xyz/` → `HTTP/2 302` to
     `https://auth.styrman.xyz/oauth2/google/?redirect_url=...` — the
     expected unauthenticated redirect, i.e. the site is up and enforcing
     auth, not down. PASS.

## Additional independent corroboration (not in the requested list, done to
avoid taking the "real re-seed" narrative on trust)

- `docker ps -a` shows both project containers `Exited` (neither started nor
  modified by this verification — read via `docker cp` on the stopped
  filesystem only, which does not start a container):
  - `styrman-928e709a-…`: `.styrman/.harness-version` = `1.15.0`;
    `~/.claude/CLAUDE.md` = 69 lines; `~/.claude/agents/` = 6 live agents
    plus `testing.md.bak.20260901-065226` (retired copy, matches the claimed
    `claude/setup.sh` retirement behavior); 28 skills; `~/.agents/bin/`
    holds both live gates plus one `.bak` set each (matches the documented
    "litter" side effect). Confirms the claimed real re-seed independently.
  - `styrman-36782cc5-…`: `.styrman/.harness-version` = `1.13.1`;
    `~/.claude/agents/` = `changelog.md documentation.md reviewer.md
    testing.md` (old 4-agent set, no explorer/security/verifier). Confirms
    it is genuinely untouched, as instructed.

- Browser e2e (`bash web/e2e/run.sh styrman-runtime:dev`) — prereq
  (`npx playwright install chromium`) was already satisfied
  (`~/.cache/ms-playwright/chromium-1234` present), so it was run per
  instructions: **35 passed / 0 failed / 2 skipped** (skips are the two
  known host-firewall-blocked host→bridge port checks, both pre-existing and
  documented in the suite's own output, not new). No regression.

- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:dev` —
  NOT re-run. The task explicitly permits trusting the requester's own run
  ("re-run only if you doubt it") and it is a multi-minute, self-cleaning
  suite; I have no reason to doubt the reported 57/0/3 given the independent
  corroboration above (image contents, clean-home deploy, and the real
  stopped-container inspection all line up with what integration.sh would
  be exercising). Recorded here as accepted-on-trust, explicitly flagged as
  such rather than silently re-stated as directly verified.

## Coverage of changed code

Change set is `image/harness/**` (19 modified, 19 added, 25 deleted) plus
docs (`image/README.md`, `.agents/docs/specs/styrman/runtime-image.md`,
`.agents/docs/architecture/DECISIONS.md` ADR-046, `ROADMAP.md`, changelog
entry, agent-memory notes). Confirmed via `git diff --stat HEAD`: zero Go,
zero `web/` source changes.

- No Go/web/deploy source changed → the "changed code without covering
  test = FAIL" rule has no packages to apply to here. Not a FAIL on that
  basis.
- However, the judgement call requested explicitly: **is the absence of new
  automated tests acceptable, and does the platform have ANY automated
  assertion that a seeded harness is correct?**
  - `grep -i harness .agents/docs/testing/styrman/integration.sh` → no
    match (exit 1).
  - `grep -in "CLAUDE.md\|skills\|agents/\|trail-check\|resume-context\|VENDORED" .agents/docs/testing/styrman/integration.sh`
    → one incidental hit, the `Usage:` comment path
    `.agents/docs/testing/styrman/integration.sh`, not a real check.
  - `grep -i "harness\|CLAUDE.md\|skills\|trail-check" web/e2e/*.mjs` → no
    match.
  - **Confirmed: the platform has zero automated assertions, anywhere in
    `integration.sh` or the Playwright e2e suite, that a harness deployed
    into a home is correct** (line counts, agent roster, skill count, gate
    presence, valid JSON, hook safety). Every check in items 4–6 above
    exists only because this verification pass built it ad hoc from
    `image/entrypoint.sh:110-135`; none of it is wired into the repo's
    regression suite. This is a real, standing gap — a future harness sync
    could silently ship a broken `setup.sh`, a missing skill, or a hook that
    hangs a guest session, and neither `go test`, `integration.sh`, nor
    `web/e2e` would catch it.
  - Given (a) this release is a verbatim vendor mirror with an explicit
    ADR (046) constraining it to "no hand-edits," (b) the content itself was
    exercised end-to-end by hand in this verification with matching results
    across a clean container AND a real re-seeded project home, and (c) the
    non-regression evidence (go suite, ingress module, e2e, live
    `/api/version`) is all green — I judge the ABSENCE of a new automated
    test **acceptable for shipping this specific release**, but the GAP
    itself is real and should be tracked: a lightweight `harness-check.sh`
    (mirroring exactly the clean-home steps in item 5 + item 6 above) belongs
    in `integration.sh` or as its own step so the next vendor sync doesn't
    depend on a human re-deriving these assertions from the entrypoint
    script by hand.

## Environment

- Tier: local only (all commands ran directly on this host, no remote box
  requested or used).
- OS: Linux 6.12.101+deb13-amd64 (Debian trixie), Docker plain (no Colima —
  Linux host).
- `styrman-runtime:dev` image ID `f8e986b1c571`, built 2026-09-01 08:49:52
  CEST (pre-existing at start of this verification; not rebuilt by this
  agent).
- `styrman-server` container: `Up`, health status `healthy`, image
  `styrman-server:latest`.
- Project homes `styrman-928e709a-…` and `styrman-36782cc5-…`: both
  `Exited`, left untouched (inspected only via `docker cp` against the
  stopped filesystem layer — no start, no exec, no modification).
