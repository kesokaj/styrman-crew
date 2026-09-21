---
owner: explorer
date: 2026-09-21
---
# Recon — harness delivery inventory (2026-09-21)

## Status: COMPLETE

## Question
Complete inventory of everything participating in delivering the agent
harness (skills/agents/context into ~/.claude and ~/.gemini/config, plus the
sh-agent binary) into a project VM's home, across the two existing
mechanisms (image mirror `image/harness/`, host harness channel ADR-073),
plus PATH composition, installed tools, entrypoint home-seeding, credential
broker gh auth, living docs, recent changes, and risks for replacing both
with an embedded sh-agent release binary approach.

## Summary
Two independent delivery paths converge on one seeding call in
`image/entrypoint.sh` (`sh-agent install --repo /styrman/harness --home
"$HOME"`), gated by `HARNESS_STAMP=~/.styrman/.harness-version` keyed on
`image-version+harness-commit`. **Mirror** (ADR-062): `image/harness/` is a
`.git`-less copy of the private `styrman-harness` repo baked into the
runtime image at build time, with `sh-agent` compiled from it in a builder
stage; refreshed by hand via `VENDORED.md`'s rsync recipe, checked by
`deploy/verify-commit.sh`. **Channel** (ADR-073, v1.35.0): a root systemd
timer (`styrman-harness-update.{service,timer}`) runs `styrman-update
harness`, which walks signed `styrman-harness` git tags down to the newest
that verifies against `/etc/styrman/allowed_signers`, exports it (git
archive + offline `go build`) and smoke-tests it, then atomically swaps
`/var/lib/styrman/harness/<commit>` under `/var/lib/styrman/harness/current`.
The server (`internal/orchestrator/harness.go`, config key `HarnessDir` /
`STYRMAN_HARNESS_DIR`) bind-mounts `current/repo` and `current/bin/sh-agent`
read-only into every non-minimal project container at the SAME guest paths
the image mirror occupies (`/styrman/harness`, `/styrman/bin/sh-agent`), so
the entrypoint's one seed line transparently prefers the channel and falls
back to the image mirror when nothing is mounted or the mount is untrusted.
Both paths are file-copiers only — `sh-agent install` never runs `git` or
`go build` on a plain copy (harness's own BUG-015 rule) — so an embedded
sh-agent release binary approach (image ships an `sh-agent` release binary
with the harness data embedded in it, or the entrypoint runs `sh-agent
install` from the binary's own embedded copy, and the user runs `sh-agent
update` per project themselves) would delete the mirror tree, the channel's
whole export/swap/bind-mount machinery, and the corresponding Tier 2
sections — but must replace: the `HARNESS_STAMP` re-seed keying (an
embedded-binary version replaces `image-version+harness-commit`), the
`harness-migrate.sh` one-time migration step (still needed once more, from
the CURRENT two mechanisms into whatever a bundled binary leaves), the MCP
browser registration block (currently gated on the same stamp), and R5's
wording (`REQUIREMENTS.md` is immutable without operator approval — any
change to "how the harness is delivered" needs an ADR + R5 edit in one
commit, per AGENTS.md's Hard rules).

## Relevant files

### Mirror mechanism (ADR-062)
| Path | Role |
|---|---|
| `image/harness/` (agents, skills, commands, context, rules, cmd/sh-agent, COMMIT, SHA256SUMS, VENDORED.md, AGENTS.md, CLAUDE.md, REQUIREMENTS.md, ROADMAP.md, TODO.md, go.mod) | `.git`-less verbatim copy of the private `styrman-harness` repo at the commit named in `COMMIT`; refreshed by hand (rsync recipe in `VENDORED.md`). |
| `image/harness/VENDORED.md` | States the mirror is now the FALLBACK (post-ADR-073), gives the refresh recipe, and the note that it is itself listed in its own `SHA256SUMS` (BUG-065). |
| `image/Dockerfile` (lines 8–13, 159–177) | Builder stage `harness-builder` (`golang:1.25.13-trixie`) compiles `/styrman/harness/cmd/sh-agent` → `/out/sh-agent` with `GOPROXY=off`; runtime stage `COPY harness/ /styrman/harness/`, `COPY --from=harness-builder /out/sh-agent /styrman/bin/sh-agent`, writes `/styrman/VERSION` from `ARG STYRMAN_VERSION`, `chmod -R go-w /styrman/harness`. |
| `image/bump-pins.sh` | Refreshes OTHER pinned tool versions (Go, Node, Claude Code, base-image digests) — does NOT touch the harness mirror itself; the harness Go toolchain pin (`harness-builder`'s `FROM golang:...`) is bumped together with `deploy/Dockerfile.server`'s. |
| `deploy/verify-commit.sh` (lines 69, 111–235) | Vets the mirror: `sha256sum -c SHA256SUMS`, `go vet ./...` inside it, that the runtime image's Go toolchain matches, that the built image's `sh-agent list --repo /styrman/harness` matches the commit's tree's skill/agent counts, and that `/styrman/harness/.git` is absent (plain-copy proof). Does NOT run the mirror's own `go test ./...`. |

### Channel mechanism (ADR-073)
| Path | Role |
|---|---|
| `deploy/update/styrman-update` (harness verbs, lines ~198–455) | `harness_alarm`, `harness_test_guard`, `harness_export` (git-archive + offline docker-run `go build`), `harness_smoke` (dry-run install + list inside the runtime image as uid 1000), `harness_swap` (atomic rename, `current` symlink swap, prunes old exports, clears the failed-tag latch), `harness_main` (walks `git tag -l 'v*' | sort -Vr` down from newest, verifies signature, refuses/alarms/latches on failure). Dispatched via `harness) MODE=harness` (line 455). |
| `deploy/update/styrman-harness-update.service` / `.timer` | Root systemd unit pair running `styrman-update harness` on its own timer, independent of the crew release timer (`styrman-update.timer`). |
| `deploy/styrman-ctl` (lines 25–31, 64, 322–385, 456–702) | `install`/`update --setup --harness-token-file` provisioning: clones `/opt/styrman-harness` from `https://github.com/kesokaj/styrman-harness.git`, writes the read-only token to `/etc/styrman/gh-token-harness`, creates the root-owned `STATE_DIR/harness` content dir, does a dry `styrman-update harness --check`, enables the timer; `status` reports the `harness` line (version, sha, FAILED latch, last refusal) and a `harness` JSON block. |
| `deploy/host-setup.sh` (lines 188–194, 260–287) | Creates the root-owned `${STATE_DIR}/harness` content dir (refuses if it's a symlink) and installs the two channel unit files (not yet enabled — that's `styrman-ctl`'s job). |
| `internal/orchestrator/harness.go` (175 lines) | Server-side trust + bind logic: `harnessBinds(dir)` (nil,nil if disabled; fs.ErrNotExist if absent; else the two `:ro` bind strings on the UNRESOLVED `current` path so Docker re-resolves on every start); `harnessTrusted` (dir not a symlink, root/server-owned, not group/world-writable, `current` a symlink resolving inside the dir, target owned+private, `repo/COMMIT` and `bin/sh-agent` present as regular files); `harnessBindsOf` (extracts a container's existing harness binds for the recreate-rule comparison, D26); `harnessChannelState`/`logHarnessChannel` (one server-start log line naming tag/commit/path, or why the channel is disabled/absent/refused). |
| `internal/orchestrator/orchestrator.go`, `harness_test.go` | Callers/tests of the above — the recreate rule compares a stopped container's existing harness binds against what `harnessBinds` returns now, so a moved/refused channel forces a recreate. |
| `internal/config/config.go` (line ~101–105, 173) | `HarnessDir string` field, `envOr("STYRMAN_HARNESS_DIR", "")` (empty = channel disabled). |
| `internal/config/harnessdir_test.go`, `styrman_harness_test.go`, `styrman_ctl_harness_test.go` | Unit tests: env wiring, `TestStyrmanHarnessChannel` (drives `styrman-update harness` in `STYRMAN_UPDATE_TEST=1` mode against a scratch origin/keys — walk-down, export shape, sha256, `--check`, downgrade refusal, `--force`, latch), `TestStyrmanCtlUpdateSetupHarnessPlan` (dry-run plan of `update --setup --harness-token-file`). |
| `.agents/docs/testing/styrman/make-scratch-harness.sh` | Builds a scratch `styrman-harness`-shaped git origin (signed `v1.0.0` tag) FROM `image/harness/`, for Tier 2's channel setup. |
| `.agents/docs/testing/styrman/integration.sh` | Tier 2: delivers the scratch channel before the server starts, asserts the server's start-up log line (tag/commit), in-guest checks (COMMIT match, read-only mount incl. a remount-rw-and-write-fails probe, binary sha256, no `.git`, boot log's `source channel tag vX.Y.Z` and `agent harness deployed` lines), stop+delete/wipe re-seed checks, and the swap-while-stopped → restart-picks-up-new-export case. |

### Shared seed call (both mechanisms funnel here)
| Path | Role |
|---|---|
| `image/entrypoint.sh` lines 110–228 | The ONE seed block: `HARNESS_VER=$(cat /styrman/VERSION)`, `HARNESS_COMMIT=$(head -c40 /styrman/harness/COMMIT)`, `HARNESS_ID="${HARNESS_VER}+${HARNESS_COMMIT}"`, `HARNESS_SRC` = `"channel tag $(cat TAG)"` if `/styrman/harness/TAG` is non-empty else `"image"`. Stamp `~/.styrman/.harness-version`; if it differs: (1) `timeout 20 … harness-migrate.sh` (migrates ≤v1.19.x artifacts), (2) `timeout 20 … sh-agent install --repo /styrman/harness --home "$HOME"` (stdout filtered to only the `settings:`/`note:` lines about the ADR-072 attribution key), (3) `claude mcp add`/`agy mcp add` for the `styrman-browser` MCP server (ADR-068 D4). Stamp written only if all three succeed; each step is non-fatal to boot (20s timeout, WARN + retry-next-boot). Log line: `agent harness deployed (version X, styrman-harness <sha7>, source <channel tag vX.Y.Z|image>; browser MCP registered for claude and agy)`. |
| `image/configs/harness-migrate.sh` | Moves a PRE-ADR-062 harness's artifacts (including both vendors' old `settings.json`) into `~/.styrman/harness-backup-<ts>/`; never deletes. |

## Flows
1. Boot → `image/entrypoint.sh` runs as root inside the Kata VM → user
   creation/home ownership → skel dotfiles → `/etc/profile.d` snippets
   copied → **harness block**: reads `/styrman/harness/{COMMIT,TAG}` and
   `/styrman/VERSION` (these paths are either the image's baked-in mirror OR
   the server's read-only bind-mount of the channel's `current/repo` — the
   entrypoint cannot tell and doesn't need to) → stamp check → migrate →
   `sh-agent install` → MCP registration → stamp write.
2. Which of `/styrman/harness` and `/styrman/bin/sh-agent` the guest sees is
   decided OUTSIDE the entrypoint, at container create/start time, by
   `internal/orchestrator` calling `harnessBinds(cfg.HarnessDir)`: if the
   channel is trusted, two extra `:ro` binds are added to the container's
   bind list (on the unresolved `.../current` path, so a stopped-then-
   restarted container re-resolves to whatever the channel currently points
   at); otherwise the image's own `COPY`-baked `/styrman/harness` and
   `/styrman/bin/sh-agent` are what's there.
3. The channel itself is populated independently, on its own timer, by
   `styrman-update harness` on the HOST (never inside any VM): clone →
   verify signed tag → export (git archive, strip `.agents`/`.git`, stamp
   COMMIT/TAG/SHA256SUMS) → offline `go build` in the pinned builder image →
   smoke-test inside the deployed runtime image → atomic swap of `current`.

## Conventions
- Every seed/install step in the entrypoint is capped with `timeout 20` and
  is non-fatal — a harness hiccup must never block a project boot.
- Idempotent stamp files (`~/.styrman/.harness-version`) gate re-seeding;
  written only after ALL constituent steps succeed, so a partial failure is
  retried whole on the next boot (BUG-035 discipline — see
  `.agents/rules/mechanism-reaches-every-caller.md`).
- Trust checks (`harnessTrusted`) are defense-in-depth against a
  guest-writable or symlink-swapped bind source — same family as
  `.agents/rules/guest-writable-paths.md`.
- `sh-agent install` on a plain copy (no `.git`) never runs `git` or
  `go build` — this is load-bearing for BOTH mechanisms and is asserted by
  `deploy/verify-commit.sh` (`test ! -e /styrman/harness/.git`) and Tier 2.
- Docs discipline: R5 in `REQUIREMENTS.md` is the canonical wording and is
  immutable without operator approval; every harness-mechanism change so far
  paired an ADR with an R5 edit in the same commit (ADR-062, ADR-062
  amendment, ADR-073).

## PATH composition (guest)
- `image/skel/bashrc` (seeded write-once as `~/.bashrc`) builds PATH in this
  order, de-duplicated: `$HOME/.local/bin` (if present) **first**, then
  `$HOME/.go/bin`, then `$HOME/.npm-global/bin`, then whatever PATH already
  was (which includes `/usr/local/bin` from the image's Go/Node install via
  symlinks, and apt's `/usr/bin` etc.). So `~/.local/bin` DOES come before
  `/usr/local/bin`.
- `image/skel/profile` (seeded as `~/.profile`) just sources `~/.bashrc` for
  bash login shells — this is what tmux/login shells (and the WS terminal
  bridge, which spawns a login shell) get.
- `sh-agent install` itself does NOT modify PATH; `image/entrypoint.sh`'s
  large comment block (lines ~128–130) notes the old ADR-047 PATH trim
  (needed for the OLD harness's `agy -p` self-check) is gone since the
  current `install` makes no CLI/network call.

## Installed tools (image/Dockerfile)
- **Go**: yes, `ARG GO_VERSION=1.27.1`, installed to `/usr/local/go`,
  symlinked `/usr/local/bin/go` and `/usr/local/bin/gofmt` (Dockerfile lines
  96–104) — on PATH for every terminal user (via the trailing `/usr/local/bin`
  in the inherited PATH, after `~/.local/bin` etc.).
- **gh**: yes, official APT repo, `docker-ce docker-ce-cli … gh
  google-cloud-cli … kubectl` (Dockerfile lines 37–48).
- **ssh-keygen (openssh-client)**: yes, in the base `apt-get install` list
  (Dockerfile line 27: `openssh-client rsync make build-essential …`).
- **git**: yes, base apt list (Dockerfile line 26); also configured with a
  SYSTEM `credential.helper = /usr/local/bin/styrman-git-credential` and
  `credential.useHttpPath false` (Dockerfile lines 179–183, ADR-030).
- The harness-builder stage uses its OWN pinned `golang:1.25.13-trixie` — a
  DIFFERENT Go version than the runtime image's `1.27.1` — used only to
  compile `sh-agent`, never shipped itself.

## Home seeding at boot (guest, `~/`)
Per `image/entrypoint.sh` (non-MINIMAL mode), the entrypoint writes/creates,
as the project user, in this order: `~/projects/<dir>` (workspace bind
mount, no-op if already mounted), `~/.dockerd`, `~/.config`, `~/.cache`,
`~/.local/bin`, `~/.ssh` (0700); skel dotfiles `.bashrc .profile
.bash_aliases .bash_exports .bash_secrets(0600) .npmrc` (write-once, never
clobbers); `/etc/profile.d/zzz-styrman-motd.sh` and
`/etc/profile.d/zz-styrman-credentials.sh` (root-owned, not per-home);
`~/.tmux.conf` (write-once); the harness via `sh-agent install` into
`~/.claude`, `~/.gemini/config`, plus its manifest at `~/.config/sh-agent/`
and the one `attribution` key in `~/.claude/settings.json` (ADR-072); MCP
registry entries in `~/.claude.json` and `~/.gemini/config/mcp_config.json`;
`~/.styrman/logs/<project>/` (transcripts); `~/.styrman/apt/<project>.list`
(apt-manifest replay, background, after ready). Re-seed decision: the
harness stamp `~/.styrman/.harness-version` holds
`"${HARNESS_VER}+${HARNESS_COMMIT}"` — a plain string compare against the
freshly computed value; differs → full re-seed (migrate + install + MCP,
stamp updated only on all-success). Everything else (dotfiles, tmux.conf) is
separately write-once (file-existence check), not version-keyed.

## Credential broker gh auth
`image/configs/styrman-credentials.sh` (sourced by every login shell via
`/etc/profile.d/zz-styrman-credentials.sh`) pulls `GET
${STYRMAN_CRED_BROKER}/env` and `export`s each key, including `GH_TOKEN` —
confirmed by `internal/credstore`/`internal/credbroker` tests
(`cs.PutEnv("alice","GH_TOKEN",...)`). There is NO `gh auth login` or `gh
config` call anywhere in the snippet or the entrypoint; `gh` is authenticated
purely because it natively honors the `GH_TOKEN` env var once exported into
the shell. The same snippet also sets `git config --global user.name/email`
from broker-served identity, and patches `~/.claude.json`'s
`hasCompletedOnboarding` when `~/.claude/.credentials.json` is present.

## Living docs naming the mirror/channel
- `README.md`, `AGENTS.md` (Setup Commands: `image/bump-pins.sh`,
  `deploy/verify-commit.sh`, `styrman-ctl update --setup
  --harness-token-file`; Architecture: full ADR-073 paragraph on the harness
  channel).
- `REQUIREMENTS.md` R5 (verbatim, `REQUIREMENTS.md` lines ~48–58): agents
  `claude`/`agy`; harness = `styrman-harness`, installed by `sh-agent`, no
  hooks/scripts, only the `attribution` settings key (ADR-062/ADR-072);
  carried by "the **host's verified harness channel** … (ADR-073 …), the
  base image's pinned mirror at `image/harness/COMMIT` remaining the
  fallback until the channel has delivered"; default shell config carried
  by the base image (ADR-030); both seeded identically into every project
  home.
- `.agents/docs/architecture/DECISIONS.md`: **ADR-062** "The seeded agent
  harness is styrman-harness, installed by sh-agent from a pinned mirror —
  no hooks, no scripts, no settings files" (2026-09-03) + its 2026-09-04
  amendment updating R5's wording; **ADR-073** "The harness reaches every
  project through a host-side, signature-gated channel; the image's mirror
  is the fallback" (2026-09-16, Accepted) — full D1–D7 decision list and
  consequences (see Summary above for the gist).
- `.agents/docs/specs/styrman/deploy.md` § "Harness channel (ADR-073)"
  (line 276) — the full host-side mechanics.
- `.agents/docs/specs/styrman/versioning.md` line 91 — the channel as the
  post-release delivery path, distinct from a crew release.
- `.agents/docs/deploy/remote.md` — "Harness channel token (ADR-073)" setup
  note (line 25) and a full setup section (line 199) pointing at
  `deploy.md`.
- `.agents/docs/testing/styrman/README.md` — Tier 2 description (line 15)
  with the FULL history of check-count changes; current line: **117 checks
  total, 3 auto-SKIP on firewall-hardened hosts, `passed=114 failed=0
  skipped=3`**; Tier 5 section (line 390, "Vendored agent harness
  (`image/harness/`)") describing the manual mirror-refresh verification
  recipe (rsync diff, `cat COMMIT` match, `sh-agent list`/`install --dry-run`
  checks).
- `image/harness/VENDORED.md` — states the mirror is now the FALLBACK and is
  itself under `SHA256SUMS`.

## Recent changes (three newest `.agents/docs/changes/` entries, by filename date)
1. `2026-09-16-v1.35.0-release.md` — newest; the v1.35.0 release entry.
2. `2026-09-16-retro-proposal-1-applied.md`
3. `2026-09-16-v1.34.1-release.md`
(Also present same-day: `v1.34.0-release.md`, `preflight-v1.34.0-no-ship.md`,
`harness-refresh-a81ef4b.md`, and the three `harness-channel-phase-{1,2,3}.md`
entries that built ADR-073 — all from 2026-09-16, the day the channel
shipped. `2026-09-21` git log shows the v1.35.0 handoff/release/harness-pin
commits described in gitStatus, which are the harness channel's first live
proof (BUG-064 "proven live" per the commit message) but I did not find a
separate `.agents/docs/changes/` entry dated 2026-09-21 in this listing —
see Not examined.)

## Risks / open questions for replacing both with an embedded sh-agent binary
- **R5 is immutable without operator approval.** Any redesign of "how the
  harness reaches a project" needs an ADR + an R5 edit approved by the
  operator in the same commit — do not start the migration without that
  approval in hand (AGENTS.md Hard rules, and the ADR-062/073 precedent of
  exactly this).
- **Two independent trust models collapse into one.** The channel's
  signature verification (`verify_tag`, SSH-signed tags,
  `/etc/styrman/allowed_signers`) and the mirror's build-time baking are
  BOTH currently defenses against a compromised/stale harness; an
  embedded-binary approach needs to say explicitly what (if anything)
  replaces channel-side signature verification, since "the binary is in the
  image, trust the image" is a materially different trust boundary than
  "the binary is signed and verified on each delivery."
- **The stamp/re-seed keying changes shape.** `HARNESS_ID =
  "${HARNESS_VER}+${HARNESS_COMMIT}"` currently distinguishes image bumps
  from harness bumps. An embedded-binary scheme needs an equivalent
  version identity (probably just the binary's own version) and must still
  answer: what happens when the OPERATOR runs `sh-agent update` inside one
  project (per-project, user-initiated, per the proposed design) vs. the
  image being rebuilt with a newer embedded binary — two update paths
  landing in the same stamp file is exactly the shape of bug family
  BUG-035 (a mechanism must reach/agree with every caller).
- **The MCP registration block (`claude mcp add` / `agy mcp add`) is
  currently gated on the SAME stamp as harness install.** If harness
  delivery moves to "user runs `sh-agent update` per project," MCP
  registration either needs to move into `sh-agent install`/`update`
  itself, or the entrypoint's stamp logic needs to split into two
  independently-keyed gates — currently a single `HARNESS_STAMP` covers
  both.
- **`harness-migrate.sh` would need one more one-time migration**: from
  "mirror OR channel, seeded via `sh-agent install --repo`" to "embedded
  binary, seeded via `sh-agent install` with no `--repo`" — the artifact
  shapes (`~/.config/sh-agent/` manifest, `~/.claude/settings.json`
  attribution key) likely carry over, but this needs confirming against the
  harness's own install code (not examined here — it lives in the private
  `styrman-harness` repo, only mirrored into `image/harness/`).
- **Tier 2's ~15 harness-specific checks and Tier 5's whole section**
  (`.agents/docs/testing/styrman/integration.sh`,
  `make-scratch-harness.sh`) are built entirely around "does the channel
  deliver / does the mirror match `image/harness/COMMIT`" — replacing the
  delivery mechanism means most of these checks get deleted or rewritten,
  not just extended; per `.agents/rules/tests-describe-behaviour.md`
  (BUG-061 family) this is exactly the kind of change where a stale test
  assertion (e.g. "no `/styrman/harness/.git`", "source channel tag" log
  line) would silently keep passing against removed behavior if not swept.
- **`deploy/verify-commit.sh`'s mirror section (lines 69, 111–235)** would
  need deletion/rewrite in step with removing `image/harness/`; it currently
  gates every release.
- **Per-project `sh-agent update` (user-initiated) is a new attack/trust
  surface** not present in either current mechanism: today NEITHER
  mechanism lets an unprivileged project user pull and run arbitrary new
  harness code — the channel is root-only on the host, the mirror is
  baked at image-build time. A per-project self-update by the terminal user
  (who is root INSIDE their own VM but not on the host) changes what "R5:
  every project seeded identically" means, and needs an explicit decision
  on whether projects can now diverge in harness version, and whether that
  is acceptable given R5's current "seeded identically into every project
  home" language.

## Not examined
- The private `styrman-harness` repository itself (its `sh-agent
  install`/`update` implementation, its own ADR-026 tag-signing design) —
  only its mirror inside `image/harness/` was read; anything about how an
  "embedded binary" would actually be built there is out of this repo's
  reach.
- `deploy/Dockerfile.server` (the SERVER's own Go toolchain pin, referenced
  as matching the harness-builder stage) was not opened — only grepped
  incidentally via `verify-commit.sh`'s reference to it.
- `internal/httpapi/setup.go` and `web/src/routes/settings.tsx` (named in
  ADR-062's Source line as reading the harness/settings roster) were not
  read — relevant to "does the Settings page need updating" for a delivery
  mechanism change but not requested in the recon scope.
- Full text of `.agents/docs/specs/styrman/deploy.md` § "Harness channel
  (ADR-073)" (only located at line 276, not read in full) and
  `.agents/docs/deploy/remote.md`'s full setup section (line 199) — grepped
  for headings only, not read end-to-end.
- Whether a `.agents/docs/changes/` entry exists dated 2026-09-21 for the
  work described in the gitStatus commit log (v1.35.0 verified live,
  harness pin regeneration) — `ls .agents/docs/changes/` sorted only showed
  entries through 2026-09-16 in my listing; did not re-check with today's
  date explicitly.
- `image/harness/agents/`, `/skills/`, `/commands/`, `/context/`, `/rules/`
  contents themselves (what actually gets installed) — only counted/listed
  at the top level, not read.
- `internal/config`'s full config.go (only the `HarnessDir` field region was
  read).
