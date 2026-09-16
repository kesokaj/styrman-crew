---
owner: explorer
date: 2026-09-16
---
# Recon — host-side signature-gated harness channel (2026-09-16)

## Question
Map every seam the planned replacement of `image/harness/` (image-baked mirror) with a
host-side git-poll + SSH-signature-gated channel touches: the existing `styrman-update`
updater (reusable machinery), the image/entrypoint harness-seed block, the orchestrator's
mount/staleness logic, config path helpers + compose, tests/gates that assert the current
mirror, and the docs/ADRs/requirements that describe it.

## Summary
`deploy/update/styrman-update` already has almost the entire mechanism a harness-only
updater needs — SSH-signature verification with an allowed-signers allowlist
(`verify_tag`, `deploy/update/styrman-update:99-179`), a state-dir contract that refuses
`/var/lib/styrman` (ADR-052, lines 182-213), atomic self-install from verified git objects
(`atomic_install`/`self_install`, lines 415-497), and a converge-on-every-tick pattern
(`host_converge`, lines 499-575) — all directly reusable as a pattern or by sourcing the
verify/atomic-install helpers into a second unit. The harness today is delivered as a
**build-time COPY** (`image/Dockerfile:159-174`) into a versioned image, seeded **write-once
per project** keyed on `~/.styrman/.harness-version` vs `/styrman/VERSION`
(`image/entrypoint.sh:110-200`); moving to a host-side channel breaks that key (there is no
per-image version once the harness ships independently) and needs a new staleness signal.
The server container already bind-mounts `/var/lib/styrman:/var/lib/styrman` 1:1
(`deploy/docker-compose.yml:125`), so a host path like `/var/lib/styrman/harness/current`
is visible inside the server for free — orchestrator mount code just needs to add it to
project-VM `Binds`. Six test/gate surfaces (`integration.sh`, `verify-commit.sh`) and three
ADRs (ADR-046/047, ADR-050/051/052, ADR-062) explicitly assert "no .git in the mirror" /
"plain copy" / "build-time only delivery" and will need rewriting, not just extending.

## Relevant files

- `deploy/update/styrman-update` (760 lines) — the crew-tag updater; sections below are the
  reusable machinery.
  - `verify_tag()` lines 99-179: requires an annotated+SSH-signed tag (rejects lightweight
    tags and non-SSH armor via a last-marker grep, lines 118-134), consults
    `ALLOWED_SIGNERS="${STYRMAN_ALLOWED_SIGNERS:-/etc/styrman/allowed_signers}"` (line 96),
    asserts root ownership + non-group/world-writable on that file (lines 149-159), runs
    `git -c gpg.ssh.allowedSignersFile=... verify-tag` inside a scratch `GNUPGHOME`
    (lines 161-174). Callable standalone via the `__verify-tag` test hook (lines 181-190,
    gated on `STYRMAN_UPDATE_TEST=1`, used only by `internal/config/styrman_update_test.go`).
  - Tag selection (lines 425-433 area / "Find the newest STABLE tag"): `git fetch --tags
    --force --prune --prune-tags`, then `git tag -l 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$'
    | sort -V | tail -1` — picks the highest VERSION by `sort -V`, not most-recent-created.
    The planned channel needs the opposite shape: walk DOWN the sorted list trying each tag
    until one verifies (current script tries only the single latest and fails closed with no
    fallback) — this is new logic, not reusable as-is.
  - State-dir contract (ADR-052), lines 182-213 and 249-296: `STYRMAN_UPDATE_STATE_DIR` has
    NO in-script default, refuses any path under/equal to `/var/lib/styrman`, refuses `/` and
    single-component paths, requires the resolved dir to be root-owned/non-writable. Three
    state files: `deployed-version`, `deployed-sha` (rollback trusts a recorded SHA without
    re-verification, comment lines ~50-56), `failed-version` (retry latch).
  - `self_install()` / `atomic_install()` lines 415-497: content is read via `git show
    "${SELF_HEAD}:${gitpath}"` (never the mutable worktree) into a temp file beside the
    target, chmod/chown, then `mv -fT` — atomic rename so a running process keeps its old
    inode. `differs()` (line ~438) does a `git show | cmp` diff before touching disk. Directly
    reusable as a helper for installing a harness binary/tree from a verified checkout.
  - `host_converge()` lines 499-575 — the closest existing pattern to "converge a
    non-container host artifact from a verified tag on every tick, gated on `HEAD` matching
    `deployed-sha`" (BUG-035 lesson embedded: `systemctl enable` runs unconditionally every
    tick, never only when something changed that tick — see
    `.agents/rules/mechanism-reaches-every-caller.md`).
  - Health/rollback: `reports_version()` (~line 655) polls `docker inspect styrman-server`
    label, not the HTTP API (auth-exempt reasoning documented); `deploy()` (~line 605)
    checks out, `git clean -d` (no `-x`, preserves `deploy/.env`), builds+composes.
  - Test hook: `internal/config/styrman_update_test.go` (`TestStyrmanUpdateVerifyTagMarkers`)
    is the only Go-side test exercising the script, via `__verify-tag`.
  - `deploy/update/styrman-update.service` / `.timer`: `OnBootSec=5min`,
    `OnUnitActiveSec=5min`, `RandomizedDelaySec=60`; env vars
    `STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update`, `STYRMAN_UPDATER_BIN`,
    `STYRMAN_UPDATER_UNIT_DIR`, `STYRMAN_FW_INSTALL_BIN`; `RuntimeDirectory=styrman-update`
    (per-run tmpfs `DOCKER_CONFIG`); hardening (`NoNewPrivileges`, `ProtectHome=yes` — note:
    this makes `/root/.gitconfig` unreadable under the timer, a documented past bug source).
  - `deploy/styrman-ctl` — `setup_auto_update()` (~line 510-599): writes
    `${ETC_DIR}/allowed_signers` (root:root 644, line 595), `${ETC_DIR}/gh-token`, runs
    `styrman-update --check` then enables the timer; `UPDATER_ENV` array (line 492) is the
    canonical env-var list a second updater unit would need its own version of.
- `image/Dockerfile:8-13` — `harness-builder` stage, `FROM golang:1.25.13-trixie@sha256:...`
  (pinned digest, matches `deploy/Dockerfile.server`'s pin per ADR-062); `COPY harness/ /src/`
  then `go build ... -o /out/sh-agent`. Lines 159-174: `COPY harness/ /styrman/harness/`,
  `COPY --from=harness-builder /out/sh-agent /styrman/bin/sh-agent`, `RUN echo
  "${STYRMAN_VERSION}" > /styrman/VERSION && chmod -R go-w /styrman/harness`.
- `image/.dockerignore` — excludes `.git`, `**/.git`, `*.env`/`**/.env`, `**/*.pem`,
  `**/*.key` from the harness build context (this is WHY the image-baked mirror has no
  `.git` — a property several tests assert literally).
- `image/entrypoint.sh:110-200` — the harness-seed block: gated on `[ -z "${MINIMAL}" ] &&
  [ -x /styrman/bin/sh-agent ]`; `HARNESS_VER="$(cat /styrman/VERSION)"`; stamp
  `${HOME_DIR}/.styrman/.harness-version` compared against `HARNESS_VER` (line ~124) — this
  is the exact coupling to replace: today "changed" = "image version changed"; a host-side
  channel needs the stamp keyed on the harness's own commit/version instead. Three capped
  (`timeout 20`) steps as the project user: `harness-migrate.sh`, `sh-agent install --repo
  /styrman/harness --home "$HOME_DIR"`, two `mcp add` calls; stamp written only if all three
  succeed (line ~195-200); boot log line `"agent harness deployed (version ${HARNESS_VER},
  styrman-harness $(cut -c1-7 /styrman/harness/COMMIT)...)"` is what
  `integration.sh:136-137` greps for.
- `image/harness/VENDORED.md` — today's manual refresh recipe (`rsync --exclude .git ...`,
  writes `COMMIT`, `SHA256SUMS`); documents the exact "plain copy, no .git" invariant the new
  channel must either preserve (VM side) or explicitly change (host side, which WOULD have
  `.git` under the plan, contradicting `image/harness/`'s historical no-git rule at the new
  location — worth flagging explicitly to the planner).
- `internal/orchestrator/orchestrator.go`:
  - `ContainerCreate` HostConfig at lines 1056-1080: `Binds` built inline (closure at
    line 1065) from `spec.homePath` and optional `spec.workspacePath` — currently exactly
    two binds, both read-write, no read-only mount anywhere in this function. A harness
    mount would be a **third** bind, and would want `:ro` (Docker bind syntax
    `src:dst:ro`) — no existing precedent for a read-only bind in this file to mirror
    exactly; nearest analog is the workspace bind's construction pattern.
  - `hasWorkspaceBind()` (line 615) / staleness check at line 280: `o.cfg.Image` vs
    `insp.Image` (`cur.ID != insp.Image`, line ~300) triggers image-upgrade recreate — this
    is the ONLY existing "does the running container's inputs still match config" trigger;
    a harness-mount-source change (e.g. `current` symlink flip) does NOT change the
    container's `Image` or recorded `Binds` source path (bind source is the same host path,
    just its target changes) — **Docker will not recreate on a harness update at all** under
    this model; needs its own trigger or relies on the VM being long-lived and the harness
    only mattering at next home-write, which conflicts with "read-only mount reflects
    latest" framing. Flagged as an open question below.
  - `projectHomeIsReal`/`realBelow` (lines 341, 408, 1610-1636) — the mount-source symlink
    guard (`.agents/rules/guest-writable-paths.md`); ANY new bind source under server
    control needs the same "prove no symlink below the trust boundary" treatment before
    being added to `Binds` — currently only applied to home/workspace paths.
  - Setup VM spec: `noRestart: true` at line 773 (ephemeral onboarding VM, "reap don't
    resurrect"); its `Binds` come from the same `ContainerCreate` code path (no separate
    mount list), so it WOULD also get a harness mount automatically if added there — the
    entrypoint already gates harness-seed on `[ -z "${MINIMAL}" ]`, unclear if the setup VM
    runs MINIMAL=1 (not confirmed in this recon — open question).
- `internal/config/config.go`: path helpers `StateDir` (line 98, default
  `/var/lib/styrman`, env `STYRMAN_STATE_DIR`), `HomesDir`/`TenantDir`/`ProjectHome`/`UserHome`
  (lines 393-410). No `HarnessDir` helper exists yet; natural analog is a sibling of
  `StateDir` like `filepath.Join(c.StateDir, "harness", "current")`, env-overridable the same
  way (`envOr("STYRMAN_HARNESS_DIR", ...)`, matching every other path helper's shape at
  line 192-193).
- `deploy/docker-compose.yml`: server service already bind-mounts
  `/var/lib/styrman:/var/lib/styrman` (line 125) — **1:1 host:container path**, so
  `/var/lib/styrman/harness/current` on the host is `/var/lib/styrman/harness/current`
  inside the server container automatically; no new compose volume entry needed for the
  server to read the channel's output, only for the orchestrator to bind it further into
  each project VM (a host path handed straight to Docker's `ContainerCreate`, since the
  server's Docker client talks to the HOST daemon via the mounted socket, not a container
  path — confirm via `STYRMAN_DOCKER_HOST`/socket bind, not fully traced in this pass).
- Tests/gates:
  - `.agents/docs/testing/styrman/integration.sh` lines 112-157 and ~405-420: asserts (a)
    `sh-agent status ... | grep 'plain copy, not its own git checkout'` (line 135) — this
    literally fails if the mount becomes a real checkout; (b) boot log's
    `"agent harness deployed (version $v, styrman-harness $c..."` line keyed on
    `/styrman/VERSION` and `image/harness/COMMIT` (lines 136-137) — both paths go away under
    the plan; (c) harness stamp write-once behavior re-tested at lines ~405-420 by deleting
    the stamp and a settings.json and re-checking equality against `/styrman/VERSION`.
  - `deploy/verify-commit.sh` lines 69-136, 212-235: mirror integrity via `sha256sum -c
    SHA256SUMS` + `go vet` (line 115), file-list diff against `SHA256SUMS` + symlink check
    (lines 116-119), builder Go pin extracted from `image/Dockerfile`'s `FROM ... AS
    harness-builder` line (line 124), "runtime image seeds N skills, M agents" check
    comparing `find image/harness/skills` counts against `sh-agent list --repo
    /styrman/harness` run inside the built image (lines 216-225), and the explicit "no
    `/styrman/harness/.git`" assertion (lines 226-235) — every one of these assumes the
    build-time-baked mirror and will need rewriting for a host-fetched channel (there is no
    `image/harness/` tree in the repo to sha256-check once the mirror moves off image-build).
  - No `ensure-test-image.sh` found by that literal name in this pass (not located; treat as
    open/needs a second look — recon budget did not reach it).

## Conventions to mirror

- Signature verification: SSH-signed annotated tags only, allowed-signers file, fail-closed,
  opt-out only via a root-owned `update.env` that is READ not sourced (never `. file`) —
  `verify_tag()` is the template for any second verifier.
- State dirs: ADR-052's "no in-script default, refuse anything under `/var/lib/styrman`,
  refuse top-level paths" pattern is load-bearing precedent from a real incident; a harness
  updater's own state dir (deployed-commit, failed-version equivalents) should get the same
  treatment, likely its own env var and its own `/etc/styrman/harness-update` -style path.
- Self-install / atomic swap: `git show <verified-sha>:<path>` → temp file → chmod/chown →
  `mv -fT` is the house pattern for "root-trusted binary replaced from a verified checkout,
  never the mutable worktree."
- Every root-owned trust input gets an explicit ownership+mode assertion before being
  trusted (`trusted_dir`, repeated stat/find checks throughout).
- BUG-035 lesson (already codified in `.agents/rules/mechanism-reaches-every-caller.md`):
  any "enable/converge" step for a new host mechanism must run unconditionally every tick,
  never gated on "something changed this tick," and must grep every caller.

## Risks

1. **No existing "recreate on harness change" trigger.** The orchestrator's only staleness
   check compares `o.cfg.Image` (the whole runtime image) to the running container's
   `insp.Image`; a harness update via a separately-swapped host mount changes neither. Either
   the plan accepts that a running project VM does NOT get harness updates until it is
   recreated for some other reason (contradicts "every project VM" framing), or a new
   staleness signal (e.g. compare `current` symlink target against a label stamped at
   container create) must be added to `EnsureRunning`.
2. **Tag-walk-downward-until-verified is new logic, not present in `styrman-update`.** The
   existing script tries only the single latest tag and fails closed; the harness channel's
   "walk down `sort -V` until one verifies, alarm on every refusal, never install below the
   currently-installed version" is unbuilt and needs its own care around the
   already-hardened `verify_tag` (reusable) but a new loop + "installed version" comparison
   (not reusable).
3. **`no .git` invariant flips location.** Six tests/ADRs assert the IMAGE's mirror has no
   `.git` (a deliberate build-context exclusion). The new host-side checkout at
   `/var/lib/styrman/harness/<commit>/` WOULD have `.git` (it's a real clone), which is fine
   if the VM-visible `current` symlink point is still a plain copy/rsync without `.git`, but
   this must be built explicitly — nothing today produces a "git checkout → strip .git →
   ready for guest mount" step; `VENDORED.md`'s manual rsync recipe is the only precedent and
   it is a human-run script, not automation.
4. **Guest-writable-paths guard is not free.** `guest-writable-paths.md` requires
   `realBelow`/`projectHomeIsReal`-style symlink proofs on every new Docker bind source;
   the `current` symlink itself, sitting under `/var/lib/styrman/harness/`, is server-only
   (not guest-writable) so the specific race BUG-002/003 addressed may not apply — but the
   rule says "ask who can replace its components before writing the code," and this recon did
   not trace who else can write under `/var/lib/styrman/harness/`.
5. **verify-commit.sh and integration.sh checks assume co-location in this repo.** Both
   scripts check `image/harness/` paths that live IN this repo (`sha256sum -c SHA256SUMS`,
   `find image/harness/skills`). Once the harness is fetched by the HOST from the separate
   `styrman-harness` repo at runtime, this repo's own commit no longer pins what's running —
   `verify-commit.sh`'s "verify a COMMIT before tagging" story changes fundamentally: crew's
   release no longer determines the harness version at all, which contradicts
   `versioning.md:89-92`'s statement that a harness refresh is versioned via `image/harness/
   COMMIT` bump in a crew release.
6. **ADR-046/047/062 decision sentences are directly contradicted by the plan** and will need
   a new ADR that explicitly supersedes: ADR-046 ("STYRMAN_VERSION bump is the only delivery
   path"), ADR-062 D1/D2/D4 ("verbatim mirror... refreshed by hand," "sh-agent update is not
   used inside VMs; the image remains the delivery path"). This is a full reversal of a
   standing, twice-reaffirmed decision, not an incremental change — the planner should treat
   it as ADR-severity, not a routine feature.

## Open questions

1. Does the setup (onboarding) VM run with `MINIMAL=1`? Not confirmed in this pass — decides
   whether it also needs the harness mount (`noRestart: true` spec, orchestrator.go:773).
2. How does the server's Docker client reach the host daemon (socket bind path in
   `docker-compose.yml`) — needed to confirm a host path like
   `/var/lib/styrman/harness/current` handed to `ContainerCreate`'s `Binds` really resolves
   on the HOST filesystem and not inside the server's own container. Not fully traced.
3. `ensure-test-image.sh` (named in the dispatch) was not located by that name in this pass —
   needs a follow-up grep/find; not reached.
4. `.agents/docs/specs/styrman/{runtime-image,guide}.md` sections were not read (only
   `versioning.md`/`deploy.md`/`remote.md` were grepped) — likely contain more mirror
   references not yet inventoried.
5. `REQUIREMENTS.md` R1 (boot budget) text was not located by that literal heading in this
   pass (grep for `^R1\b` returned nothing) — the boot-budget number and its relationship to
   the harness-seed timeout block (image/entrypoint.sh's 80s-worst-case comment) was not
   confirmed against the current requirement text.
6. Whether Go is expected on the production host at all (item 7 of the dispatch) — not
   confirmed; the updater builds images via `docker build`/`docker compose build` only, and
   `deploy/vulncheck.sh` uses a pinned BUILDER IMAGE rather than host Go, suggesting a
   harness-only updater could similarly avoid needing host Go by building `sh-agent` inside a
   pinned builder container the same way `image/Dockerfile`'s `harness-builder` stage does —
   but this was not verified by reading `deploy/vulncheck.sh` in full.
7. `deploy/firewall` interactions with a new host path were not examined at all in this pass.

## Not examined

- `deploy/vulncheck.sh` full contents (only referenced, not read).
- `.agents/docs/specs/styrman/runtime-image.md` and `guide.md` mirror-related sections.
- `deploy/firewall/` for any interaction with a new host state path.
- `ensure-test-image.sh` (name given in dispatch, not located).
- `internal/orchestrator` unit tests that touch mount construction (not opened; only
  production code was read).
- REQUIREMENTS.md's exact R1 wording (grep for the literal heading came back empty; may use
  different numbering/formatting than assumed).
- The Docker-socket bind in `docker-compose.yml` (server→host daemon reachability) — not
  traced to confirm host-path semantics for a new orchestrator bind.
