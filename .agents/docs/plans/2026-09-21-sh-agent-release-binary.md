---
owner: main
date: 2026-09-21
---
# Plan — the harness reaches a project as the sh-agent release binary   (2026-09-21)

## Why

The operator rebuilt styrman-harness around one delivery mechanism: `sh-agent` is now
a Go binary with the harness (context, skills, agents, commands, rules) embedded, and
its R8/R9 retire every other channel — no vendored mirror in a consumer, no signed-tag
export on a host. Crew still carries both retired shapes: the mirror `image/harness/`
(ADR-062) and the host harness channel (ADR-073, v1.35.0). Operator, 2026-09-21: "The
sh-agent is the only way to go now … Only add sh-agent binary to the runtime and run
install. That's it, don't complicate things. Browser mcp is ok to run after sh-agent
install." and "Build from the GitHub repo. Not local project." Between crew releases
each project moves itself forward when its user runs `sh-agent update` inside it.
Recon: `.agents/docs/plans/2026-09-21-harness-delivery-recon.md`.

## Scope

In:
- `image/Dockerfile` downloads the sh-agent release binary named by `ARG HARNESS_VERSION`
  from GitHub Releases (private repo; token as a docker build secret), verifies the
  signed checksums, installs it at `/styrman/bin/sh-agent`. The mirror tree and its
  builder stage go.
- `image/entrypoint.sh` runs `sh-agent install --home <home>` (no `--repo`) when the
  stamp differs, keyed on image version + the binary's release; never downgrades a home
  the user moved forward; browser MCP registration after it, as today.
- The host harness channel is deleted end to end (updater verb, unit pair, host-setup
  dir, styrman-ctl setup/status/uninstall parts, `STYRMAN_HARNESS_DIR`, the orchestrator
  mount and its recreate rule, their tests, Tier 2 sections, `make-scratch-harness.sh`).
  The updater's converge removes the unit pair from a host that has it.
- `deploy/verify-commit.sh` and `deploy/vulncheck.sh` lose their mirror sections; the
  image build they run passes the secret.
- Tier 2 checks for the new seed; docs; R5 + ADR-074 (operator approval required for R5).
Out: any crew-side automation of `sh-agent update` (none: the user runs it in the
project); a pin file, build wrapper, `bump-pins.sh` change (the version is the Dockerfile
ARG); `harness-migrate.sh` (unchanged); the Settings roster (reads what install wrote);
the release itself (`/preflight` is the operator's call).
Follow-ups noted, not built: none.

## Surfaces touched

`image/Dockerfile`, `image/harness/` (deleted), `image/entrypoint.sh`, `image/README.md`,
`deploy/verify-commit.sh`, `deploy/vulncheck.sh`, `deploy/update/styrman-update`,
`deploy/update/styrman-harness-update.{service,timer}` (deleted), `deploy/host-setup.sh`,
`deploy/styrman-ctl`, `deploy/docker-compose.yml`, `deploy/README.md`,
`internal/config/config.go` (+ `harnessdir_test.go`, `styrman_harness_test.go`,
`styrman_ctl_harness_test.go` deleted; `styrman_ctl_test.go`, `styrman_update_test.go`
adjusted), `internal/orchestrator/{harness.go,harness_test.go}` (deleted),
`internal/orchestrator/orchestrator.go` (+ `orchestrator_test.go`),
`.agents/docs/testing/styrman/{integration.sh,README.md,make-scratch-harness.sh}`,
`README.md`, `AGENTS.md`, `REQUIREMENTS.md` (R5), `.agents/docs/architecture/DECISIONS.md`
(ADR-074), `.agents/docs/specs/styrman/{deploy,versioning,runtime-image}.md`,
`.agents/docs/deploy/remote.md`. Recon brief: `plans/2026-09-21-harness-delivery-recon.md`.

## Interfaces

**Dockerfile (runtime stage, AFTER the `mkdir -p /styrman` step — today line 142 — so the
destination exists; `gh`, `ssh-keygen` and `sha256sum` are installed by the apt step above it):**
```
ARG HARNESS_VERSION=v1.6.0
ARG HARNESS_SIGNER='ostling.simon@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBkx0BICH1xG18f0a0F/02Vw0OvBTfSOVNXkPkO62NCR'
RUN --mount=type=secret,id=gh_token,required=true \
    set -eu; mkdir -p /styrman/bin; d=$(mktemp -d); cd "$d"; \
    GH_TOKEN="$(cat /run/secrets/gh_token)" gh release download "${HARNESS_VERSION}" --repo kesokaj/styrman-harness \
        --pattern sh-agent-linux-amd64 --pattern SHA256SUMS --pattern SHA256SUMS.sig --dir "$d"; \
    printf '%s\n' "${HARNESS_SIGNER}" > allowed_signers; \
    ssh-keygen -Y verify -f allowed_signers -I "${HARNESS_SIGNER%% *}" -n sh-agent-release -s SHA256SUMS.sig < SHA256SUMS; \
    grep -qxF "# styrman-harness ${HARNESS_VERSION}" SHA256SUMS; \
    grep -v '^#' SHA256SUMS | sha256sum -c - --ignore-missing; \
    install -m 755 sh-agent-linux-amd64 /styrman/bin/sh-agent; \
    test "$(/styrman/bin/sh-agent version)" = "sh-agent ${HARNESS_VERSION}"; \
    cd /; rm -r "$d"
```
`HARNESS_VERSION` must be a canonical `vX.Y.Z` (the binary installs its embedded harness
only then); the `test` line enforces that the binary reports exactly it. The
`harness-builder` stage, `COPY harness/`, `chmod -R go-w /styrman/harness` and the
harness lines of the header comment go. `image/harness/` is deleted (`git rm -r`).

**Build command (AGENTS.md Setup Commands, versioning.md, deploy/README.md):**
```
GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN \
  -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/
```
The token must read `kesokaj/styrman-harness`. Four scripts build the image; every one
applies the same rule, in this order: `GH_TOKEN` set in the environment → `--secret id=gh_token,env=GH_TOKEN`;
else `/etc/styrman/gh-token` present (the updater's crew token, `${GH_TOKEN_FILE}` /
`${ETC_DIR}/gh-token`) → `--secret id=gh_token,src=<that file>`; else the script stops before
the build with `FATAL: no GitHub token for the styrman-harness download — export GH_TOKEN=$(gh auth token) or install /etc/styrman/gh-token`.
The operator grants the crew token read on styrman-harness — one token, no new file.
Sites: `deploy/update/styrman-update` `deploy()` (root; the token file), `deploy/host-setup.sh`
step 7 (its header comment "no secrets needed" changes), `deploy/styrman-ctl install`
(the `run docker build` line; `--gh-token-file F` when given is the `src=` form, else the
rule above), `deploy/verify-commit.sh` (`GH_TOKEN="${GH_TOKEN:-$(gh auth token)}"`, `env=`).

**Entrypoint block (replaces the ADR-073 stamp logic):**
```
HARNESS_VER="$(cat /styrman/VERSION …)"
HARNESS_REL="$(/styrman/bin/sh-agent version 2>/dev/null | awk '{print $2}')"   # vX.Y.Z
[ -n "${HARNESS_REL}" ] || HARNESS_REL=unknown
HARNESS_ID="${HARNESS_VER}+${HARNESS_REL}"
HARNESS_STAMP="${HOME_DIR}/.styrman/.harness-version"          # unchanged path
if stamp != HARNESS_ID:
    migrate (unchanged)
    HOME_REL = the `embedded:vX.Y.Z` release in ~/.config/sh-agent/state.json, "" if none/other
    if HOME_REL is a release newer than HARNESS_REL (sort -V):    # the user ran sh-agent update
        log "agent harness: home has sh-agent ${HOME_REL} (sh-agent update), image carries ${HARNESS_REL}; left alone"
    else:
        sh-agent install --home "${HOME_DIR}"   (as the user, timeout 20, stdout filtered as today)
    MCP registration (unchanged)
    stamp written only when every step succeeded (unchanged)
    log "agent harness deployed (version ${HARNESS_VER}, sh-agent ${HARNESS_REL}; browser MCP registered for claude and agy)"
```
`HOME_REL` is read with `sed -n 's/^ *"repo": *"embedded:\(v[0-9][0-9.]*\)".*/\1/p'` — a
path (a mirror/channel install) or no file yields "" and installs. The shape is the release
binary's own: styrman-harness `internal/harness/source.go:44` (`EmbeddedPrefix = "embedded:"`)
and `apply.go:117` (`repo, commit := EmbeddedPrefix+src.Version, ""` for an embedded install),
written by `SaveState` as `"repo": "embedded:vX.Y.Z"` with `json.MarshalIndent` (two-space
indent, so the key sits at the line start after spaces) — checked in the harness checkout
(`~/workspace/styrman-harness`, commit 914dc90, three commits after the v1.6.0 tag with these files unchanged since it), NOT in `image/harness/`, which mirrors
the pre-rebuild source and has no such prefix. The Phase 1 entrypoint smoke asserts the real
binary's seeded `state.json` matches that `sed` (the pattern is validated against real output
before Tier 2 plants the same shape).

**Server:** `config.HarnessDir` and `STYRMAN_HARNESS_DIR` removed; `internal/orchestrator/harness.go`
removed; `EnsureRunning`'s recreate rule loses the harness-binds comparison (back to the
rule before 28212ac: image version and mode); `containerSpec.harnessBinds` removed;
compose line `STYRMAN_HARNESS_DIR=…` removed.

**Host:** `styrman-update`: `harness` mode and every `harness_*` function removed, and
`host_converge`'s two harness blocks (the every-tick unit install, the timer enable) and
`self_install`'s optional harness units removed. A NEW function `retire_harness_units`,
OUTSIDE `host_converge` and independent of its guards (it must run on a host with
`REQUIRE_SIGNED=0` or an untrusted firewall dir too — the BUG-064/BUG-035 rule; cold read G2):
its only guards are `UPDATER_UNIT_DIR` set (the unit pins it; unset → one log line, return 0)
and `trusted_dir` on it. For each of `styrman-harness-update.timer`, `.service` present under
that dir: `systemctl disable --now <unit>` (timer first; a failure is a WARN, the file is still
removed), `rm -f`; `systemctl daemon-reload` once when anything was removed; then
`rmdir "${STATE_DIR}/harness"` when it exists, is not a symlink and is empty (never `rm -r`).
Under `--check` it reports what it would remove and changes nothing. Called immediately BEFORE
each of the three `host_converge` calls in the tick (today lines ~1015, ~1055, ~1083), so it
runs every tick. Test hook, beside `__verify-tag`: `__retire-harness-units <unitdir> <statedir>`
under `STYRMAN_UPDATE_TEST=1`, with `systemctl` resolved from `PATH` (the test puts a fake first). `host-setup.sh`: the harness dir and unit
install removed. `styrman-ctl`: `--harness-token-file`, `HARNESS_REPO_DIR`, `HARNESS_ORIGIN`,
the setup steps, the `harness` status line and JSON block, the uninstall lines for the
token and clone — removed. The unit files deleted from `deploy/update/`.

**verify-commit.sh:** the mirror section (sums, vet, toolchain pin, list-count match,
`.git` absence) replaced by one check on the built image:
`docker run --rm --entrypoint /styrman/bin/sh-agent "$RIMG" version` equals
`sh-agent <HARNESS_VERSION default read from the Dockerfile ARG line>`; `list` printed as
information. `vulncheck.sh`: `image/harness` dropped from the module list.

## Failure modes

- No token / token cannot read styrman-harness: `docker build` fails in the download RUN
  with gh's error (`HTTP 404` / `authentication required`); nothing is built. The updater's
  deploy tick fails as any image-build failure does today (latched, `styrman-ctl status`).
- Signature, version line, checksum or `version` mismatch: the RUN fails, no image.
- `sh-agent install` fails or times out in the guest: WARN in the boot log, stamp not
  written, retried next boot, boot continues (unchanged).
- Home newer than the image: install skipped, logged, stamp written (the MCP step still
  runs); the home's `~/.local/bin/sh-agent` and harness stay.
- `sh-agent update` in a project without a gh token that reads the repo: gh's own error
  in the terminal; the project stays on the image's release. Not crew's to handle.
- A host still carrying the unit pair: converge disables and removes it on its first tick
  running the new updater (BUG-064: the tick that installs the new updater runs the OLD one,
  so the removal lands one tick later — idempotent, every tick).

## Verification

Tiers: unit (`go test ./...`, `go vet ./...`, the caddy module untouched), script
(updater converge test via `STYRMAN_UPDATE_TEST=1`), image (`docker build` with the
secret, `sh-agent version` in the image), Tier 2 integration (`integration.sh` against
real Kata, updated checks), web Tier 1 (`npm run check && npm run lint && npm test`,
untouched but run: the roster endpoint is unchanged).
E2E criteria (Tier 2 unless marked):
- A new project boots; its boot log says `agent harness deployed (version <img>, sh-agent v1.6.0; …)`;
  in the guest `~/.local/bin/sh-agent version` prints `sh-agent v1.6.0`, `sh-agent status`
  exits 0, `~/.config/sh-agent/state.json` records `embedded:v1.6.0`, `~/.claude/skills`
  holds the skills `sh-agent list` names, `~/.claude/settings.json` has the attribution key.
- Edge: a home whose manifest records a newer release than the image (planted from the
  host as the owner, stamp cleared) boots with `left alone` in the log and its manifest
  untouched.
- Edge: a host with the unit pair installed and disabled loses both files and the empty
  content dir after one converge (script test).
- Golden path, who: user, on the live host after the release: in a project terminal
  `sh-agent update` prints `up to date (v1.6.0)` (or installs a newer release).

## Phases

- [x] Cold-reader check — round 1 `reviews/2026-09-21-review-plan-sh-agent-release-binary.md` (REQUEST CHANGES: G1 RUN placement, G2 unit-removal placement, G3 manifest format — all fixed in the plan), round 2 `reviews/2026-09-21-review-plan-sh-agent-release-binary-round-2.md` (APPROVE, design ready, implementation ready; one wording nit applied), 2026-09-21

### Phase 1 — the image carries the release binary; the entrypoint installs it
- [x] Dockerfile: download + verify RUN after `mkdir -p /styrman`, ARGs, stage and mirror COPY removed; the four build sites pass the secret by the one token rule; `git rm -r image/harness` — verify: image, `GH_TOKEN=$(gh auth token) docker build --secret … -t styrman-runtime:dev … image/` exit 0 and `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:dev version` — expect: `sh-agent v1.6.0`; positive control for the guard: a build with `--build-arg HARNESS_VERSION=v1.5.0` on an image whose SHA256SUMS names v1.5.0 passes, one with a wrong signer line (`--build-arg HARNESS_SIGNER=…`) fails at `ssh-keygen -Y verify` — docs: `AGENTS.md`, `.agents/docs/specs/styrman/versioning.md`, `image/README.md` — done 2026-09-21: build exit 0 (`.agents/scratch/image-build.log`), `version` → `sh-agent v1.6.0`, `list` → Skills (27) / Subagents (7); the wrong-signer build failed with `Could not verify signature.` (exit 255); `HARNESS_VERSION=v1.5.0` built and reported `sh-agent v1.5.0`
- [x] Entrypoint: new stamp keying, `install --home` without `--repo`, the never-downgrade guard, the new log line — verify: `bash -n image/entrypoint.sh`; a `docker run` of the image with a scratch home (the existing entrypoint smoke recipe in `testing/styrman/README.md`) — expect: boot log `agent harness deployed (version dev-…, sh-agent v1.6.0; browser MCP registered for claude and agy)`, `~/.local/bin/sh-agent` present, and the plan's exact `sed` over the seeded `~/.config/sh-agent/state.json` prints `v1.6.0` (the real binary's output validates the pattern; empty output is a FAIL — positive control: the same `sed` over a line `"repo": "/styrman/harness"` prints nothing) — done 2026-09-21: a privileged `docker run` of `styrman-runtime:dev` with a scratch home logged `agent harness deployed (version dev-1790011500, sh-agent v1.6.0; browser MCP registered for claude and agy)`, the settings: line, `~/.local/bin/sh-agent` present, the `sed` over the real `state.json` printed `v1.6.0` (the negative control printed nothing), stamp `dev-1790011500+v1.6.0`; the never-downgrade walk (manifest set to `embedded:v9.9.9`, stamp removed, second run) logged `left alone`, binary and manifest sha256 unchanged, stamp rewritten
- [ ] `deploy/verify-commit.sh`: secret passed, mirror section replaced by the `version` check; `deploy/vulncheck.sh`: module list — verify: `bash -n` both; `SKIP_WEB_FRESHNESS=1 deploy/verify-commit.sh HEAD` after the phase commit (it verifies a COMMIT) — expect: `runtime image carries sh-agent v1.6.0` line, exit 0 — docs: `.agents/docs/specs/styrman/versioning.md` § Release procedure
- [x] Checkpoint — expect: the image builds from GitHub with no mirror in the tree; a scratch-home boot seeds the harness from the binary — evidence: phases 1–3 were verified together (the mirror's deletion turned two channel tests red until Phase 2 removed them, so no per-phase commit): `testing/2026-09-21-verify-sh-agent-release-binary.md` PASS (image build with the secret, `sh-agent v1.6.0`, 27 skills / 7 subagents), 2026-09-21

### Phase 2 — the channel is deleted: server and host
- [x] Server: `HarnessDir`/`STYRMAN_HARNESS_DIR`, `harness.go`, `harness_test.go`, `harnessdir_test.go`, the recreate-rule comparison, `containerSpec.harnessBinds`, compose line — verify: `go build ./... && go vet ./... && go test ./...` bare — expect: exit 0; `grep -rn "harnessBinds\|HarnessDir\|HARNESS_DIR" internal cmd deploy` empty — docs: `.agents/docs/specs/styrman/deploy.md` — done 2026-09-21: `go build`/`go vet` exit 0, `go test ./...` exit 0 (`.agents/scratch/test2.log`), the grep empty
- [x] Host: `styrman-update` harness verbs and mode removed, `retire_harness_units` added and called before every `host_converge`; unit files deleted; `host-setup.sh`, `styrman-ctl` harness parts removed; `styrman_harness_test.go`, `styrman_ctl_harness_test.go` deleted; `styrman_ctl_test.go` adjusted — verify: new Go test `TestStyrmanUpdateRetiresHarnessUnits` (plants both unit files under a scratch unit dir and an empty scratch `<state>/harness`, runs `__retire-harness-units` under `STYRMAN_UPDATE_TEST=1` with a fake `systemctl` first on PATH recording its calls) — expect: both files gone, the dir gone, the fake saw `disable --now` for the timer, then the service, then one `daemon-reload`; a second run changes nothing and calls no `systemctl`; positive control: the hook is refused without `STYRMAN_UPDATE_TEST=1`, and a scratch dir that is NOT empty stays — docs: `deploy/README.md`, `.agents/docs/deploy/remote.md` — done 2026-09-21: `TestStyrmanUpdateRetiresHarnessUnits` PASS (the content-dir `rmdir` moved to the operator per D9; the four `harness-*` state files are removed too); the old script answers the hook with `unknown flag: __retire-harness-units` (control)
- [x] `bash -n` on `styrman-update`, `styrman-ctl`, `host-setup.sh`; `go test ./internal/config/ -run 'StyrmanCtl|StyrmanUpdate'` — expect: exit 0 — done 2026-09-21: all exit 0 (one leftover `u_hs_s` reference in styrman-ctl's verdict found by `TestStyrmanCtlStatus…` and removed)
- [x] Checkpoint — expect: no file in the tree names the channel except history (changes/, ADRs, reviews, bugs) — evidence: `testing/2026-09-21-verify-sh-agent-release-binary.md` PASS (go build/vet/test -race, the retire test by name, the pre-change control); the docs lens `reviews/2026-09-21-review-worktree-docs.md` found no living restatement (the grep's remaining hits are past-tense history in AGENTS.md/deploy.md/README and the updater's own retire comments), 2026-09-21

### Phase 3 — Tier 2 describes the new seed
- [x] `integration.sh`: channel sections removed (scratch delivery, server log line, mount and remount probes, swap-while-stopped); `make-scratch-harness.sh` deleted; new checks: boot log line with `sh-agent vX.Y.Z`, guest `~/.local/bin/sh-agent version` equals the image's, state.json `embedded:`, `sh-agent status` exit 0 in the guest, skill count equals `sh-agent list`, the never-downgrade case (manifest planted newer, stamp cleared, stop/start, `left alone` logged, manifest unchanged, then restored) — verify: `bash .agents/docs/testing/styrman/integration.sh` bare, exit status read — expect: `failed=0`, the new count line — docs: `.agents/docs/testing/styrman/README.md` (Tier 2 count line in the same commit; Tier 5 mirror section removed) — done 2026-09-21: `bash integration.sh` (default `styrman-runtime:test`, rebuilt by `ensure-test-image.sh` with the build secret) printed `passed=111 failed=0 skipped=3 aborted=0` (114 checks; the three skips are the host-firewall proxy checks); log `.agents/scratch/tier2.log`; the first run aborted before any check because `ensure-test-image.sh` had no secret — fixed in the same phase; re-run after the review fixes (D11): `passed=112 failed=0 skipped=3 aborted=0`, 115 checks (`.agents/scratch/tier2b.log`)
- [x] Web Tier 1: `(cd web && npm run check && npm run lint && npm test)` — expect: exit 0 — done 2026-09-21: after `npm ci` (node_modules was absent), check/lint/test exit 0, 178 tests in 15 files
- [x] Checkpoint — expect: Tier 2 green with the new checks — evidence: Tier 2 `passed=111 failed=0 skipped=3` (114 checks) then, after the review fixes, `passed=112 failed=0 skipped=3` (115), recorded in `testing/2026-09-21-verify-sh-agent-release-binary{,-round-2}.md`, 2026-09-21

### Phase 4 — docs, requirements, Done
- [x] (who: user) R5 rewritten (text under D6) and ADR-074 appended in one commit; ADR-062 and ADR-073 marked superseded — approved 2026-09-21 ("Go", in reply to "Say yes to the R5 text"); done 2026-09-21 and ADR-074 appended in one commit; ADR-062 and ADR-073 marked superseded — verify: `git diff --stat` shows REQUIREMENTS.md and DECISIONS.md in one commit — expect: the approval quoted in ADR-074 — docs: `REQUIREMENTS.md`
- [x] Living docs: `README.md`, `AGENTS.md` (Setup Commands, Architecture, Key Files), `image/README.md`, `deploy/README.md`, `specs/styrman/{deploy,versioning,runtime-image}.md`, `deploy/remote.md`, `testing/styrman/README.md` — verify: `/review --docs` over the range; `grep -rn "harness channel\|image/harness\|harness-token-file\|STYRMAN_HARNESS_DIR" README.md AGENTS.md image deploy .agents/docs/specs .agents/docs/deploy .agents/docs/testing/styrman/README.md` — expect: empty
- [x] Definition of Done: verifier over the whole range, reviewer (diff only), security (Dockerfile secret, host script, entrypoint), documentation, changelog entry, bugs for anything that went wrong; plan closed; commit, push, `/handoff` — expect: every finding fixed or rebutted in its report's `## Outcome`
- [x] Checkpoint — expect: tree clean, handoff on disk naming v1.36.0 as the next release for `/preflight` (operator) and the live `sh-agent update` check as the operator's e2e — evidence: `handoffs/2026-09-21-sh-agent-release-binary.md` and `handoffs/2026-09-21-adr-074-close.md`, 2026-09-21

## Risks

Assumptions:
- The release binary records an embedded install as `"repo": "embedded:vX.Y.Z"` in `state.json` — read in the harness source at v1.6.0 (Interfaces); checked against the real binary's output by the Phase 1 entrypoint smoke (its `sed` must print `v1.6.0`).
- `gh release download` with `GH_TOKEN` works non-interactively inside the build (no `gh auth login`) — checked by the Phase 1 build itself, first task.
- Docker on the host supports `RUN --mount=type=secret` and `--secret id=…,src=` without a `# syntax=` directive (Docker 29.7.2 here, BuildKit default) — checked by the Phase 1 build; the updater's build runs on this same host.
- The crew token at `/etc/styrman/gh-token` can read styrman-harness — operator action (grant it) before the release deploy; the deploy tick fails visibly otherwise (Failure modes).
- `sh-agent install` from a release binary resyncs a home installed by `--repo` (path manifest): removes what the old manifest listed and the embedded harness lacks — checked in Tier 2 by seeding a home twice? No: checked by the harness's own tests (`Install` resync by manifest, README § Daily commands: "removes files a previous install wrote that the repo no longer has"); the Tier 2 never-downgrade case covers the guard, not the resync.
- The credential broker's `GH_TOKEN` reaches `gh` in a project shell (recon: `styrman-credentials.sh` exports it) — the operator's live check.

## Decisions

D1 — 2026-09-21 — The image carries the PUBLISHED release binary, downloaded from GitHub Releases at build time with a build secret, verified against the harness's signer line; no Go build, no checkout, no mirror — by: "Build from the GitHub repo. Not local project."
D2 — 2026-09-21 — The version is the Dockerfile `ARG HARNESS_VERSION` default; bumping the harness a crew release ships is editing that line — by: "don't complicate things" (a pin file and a bump-pins change were offered and dropped)
D3 — 2026-09-21 — One token: the updater passes its crew token (`/etc/styrman/gh-token`) as the build secret; the operator grants it read on styrman-harness; the channel's `gh-token-harness` file and the styrman-ctl `--harness-token-file` path are deleted — by: implementer, offered 2026-09-21 ("grant that same token read access … nothing new is installed") and not objected to at "Start"
D4 — 2026-09-21 — Boot seed: one stamp as today, keyed `<image version>+<binary release>`; install runs when it differs; a home whose manifest records a NEWER release than the image is left alone (the user's `sh-agent update` is never reverted by an image bump) — by: implementer; the guard is the one addition beyond "run install" and exists because an image carrying an older harness than the home would otherwise downgrade it silently
D5 — 2026-09-21 — Browser MCP registration stays in the entrypoint after install, under the same stamp — by: "Browser mcp is ok to run after sh-agent install."
D6 — 2026-09-21 — R5 text (approved 2026-09-21: "Go"; the edit and ADR-074 landed in one commit): replace from "is carried by the **host's verified harness channel**" through "(ADR-030, revised 2026-08-28; was "deployed into the shared home")." with: "is carried by the **base image** as the `sh-agent` release binary with the harness embedded — downloaded at image build from the styrman-harness GitHub release named by the Dockerfile's `HARNESS_VERSION`, its signed checksums verified against the harness's signer (ADR-074, operator approval 2026-09-21; was "the host's verified harness channel … the image's pinned mirror … the fallback", ADR-073). The entrypoint installs it into a project home whose harness differs and never downgrades one the user moved forward; from there each project moves itself forward when its user runs `sh-agent update` in it (the harness's R8/R9), so projects may run different harness releases. The default shell config is carried by the base image and seeded write-once into every project home (ADR-030, revised 2026-08-28; was "deployed into the shared home")." — by: implementer, from the operator's words in ## Why
D7 — 2026-09-21 — Version for the release: v1.36.0, MINOR (projects gain self-update; an existing deployment reacts to nothing — converge removes the units) — by: implementer
D9 — 2026-09-21 — The retire step removes the unit pair and the four `harness-*` state files under the updater's state dir; the channel's content dir (`/var/lib/styrman/harness`, pinned only by the retired unit) is the operator's `rmdir` — the updater names no production path of its own (ADR-052) — supersedes the plan's `rmdir` in Interfaces — by: implementer
D10 — 2026-09-21 — Every image build site (`styrman-update`, `host-setup.sh`, `styrman-ctl install`, `verify-commit.sh`, `ensure-test-image.sh`) takes the token by one rule: `GH_TOKEN` env, else the installed token file (or `gh auth token` for the developer-side scripts), else stop before the build; a `styrman-ctl install --dry-run` without a token plans the `src=` form and says a real run needs one — by: implementer
D11 — 2026-09-21 — Review fixes: the never-downgrade comparison is a pure function `harness_home_newer` (both sides canonical, an unreadable image release takes the WARN path with no stamp); a skipped install logs `agent harness kept …`, never `deployed`; the manifest is read only as a regular file under `timeout 5`; `TRUST_UID=0` pinned on the production path; the token-file readers assert root/0600; the developer scripts never export the token; the retire hook takes `--check` and the test covers it and the untrusted-dir refusal — by: code review `reviews/2026-09-21-review-sh-agent-release-binary.md` (finding 3) and security `reviews/2026-09-21-security-sh-agent-release-binary.md` (findings 3–8; 1–2 rebutted/accepted in its Outcome)
D8 — 2026-09-21 — `harness-migrate.sh` unchanged: its predicate (old stamp present, no sh-agent manifest) is independent of the delivery mechanism — by: "don't complicate things"
Open: keep the never-downgrade guard (D4)? — the cold read (round 1, Scope) notes it is the one addition beyond "run install" and attributed to the implementer — default: keep (six lines; without it an image whose ARG lags a release the user already updated to would reinstall the older harness over the newer one on the next stop/start, silently)
Rejected: building sh-agent from a local checkout passed as a build context — "Build from the GitHub repo. Not local project."
Rejected: a wrapper `image/build.sh`, a `image/HARNESS_VERSION` pin, a `bump-pins.sh` refresh, deleting `harness-migrate.sh` and the vulncheck line as separate work — "This sounds excessive."
Rejected: keeping the channel's token file for the build — one token is fewer moving parts (D3).
