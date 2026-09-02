# Versioning & image update guide

One signed git tag (`vX.Y.Z`) drives the entire deploy: `STYRMAN_VERSION=X.Y.Z`
stamps the binary/UI, tags all three images, and is what the auto-updater
polls for. This is the "how do I ship/update a thing" companion to
`deploy.md`'s "## Auto-update" section (full mechanics + security model) —
read that for the trust story; this doc is the day-to-day reference.

## The three images

| Image | Dockerfile | Built by | What it runs |
|---|---|---|---|
| `styrman-server` | `deploy/Dockerfile.server` | `docker compose build` (or `styrman-update`) | The Go binary + embedded SPA — the containerized server itself. |
| `styrman-caddy` | `deploy/caddy/Dockerfile` | `docker compose build` (or `styrman-update`) | The vendored ingress: Caddy 2 + caddy-security (Google OAuth) + Cloudflare DNS + the styrman label-discovery module. |
| `styrman-runtime` | `image/Dockerfile` | `docker build ... image/` — NOT a compose service, a separate step | The per-project microVM image every project boots: Kata guest, dotfiles, the agent harness (`claude`/`agy`), toolchains. |

`styrman-runtime` is the odd one out: `docker compose build`/`up --build`
never touches it. Forgetting to (re)build it at the release version is the
most common way a release "ships" while projects keep booting the old guest
image.

## Versioning scheme

One tag, `vX.Y.Z` (semver; no `v` inside the number itself), fans out to:

- `STYRMAN_VERSION=X.Y.Z` — the env var every build/deploy command sets.
- `styrman-server:X.Y.Z`, `styrman-caddy:X.Y.Z` — the two compose image tags
  (`deploy/docker-compose.yml`'s `image:` lines are
  `${STYRMAN_VERSION:?set STYRMAN_VERSION=X.Y.Z — mutable tags are banned}` —
  compose refuses to run at all with it unset).
- `styrman-runtime:X.Y.Z` — the runtime image tag; the server's
  `STYRMAN_IMAGE` env defaults to `styrman-runtime:${STYRMAN_VERSION}`
  unless explicitly overridden (`docker-compose.yml`), and `config.Load`
  (`internal/config/config.go`) refuses to start with `STYRMAN_IMAGE` unset
  — there is no `:latest` fallback anywhere in the code path anymore.
- The `styrman.version` LABEL baked into `styrman-server` at build time
  (`deploy/Dockerfile.server`'s final `ARG`/`LABEL`) — this is what the
  auto-updater reads with `docker inspect` to confirm a deploy came up
  healthy. **Never remove this label**: an absent label reads as an empty
  string, which never matches a wanted version, so every future release
  would fail verification and roll itself back.

**Operator rule: no mutable tags.** No `:latest`, `:dev`, or any other
floating tag reaches a compose `image:` line or a production `STYRMAN_IMAGE`.
Two concrete incidents on 2026-09-01 are why this is a hard rule, not a
style preference:

1. **The reboot incident.** The host rebooted at 19:28 CEST mid-fix. Because
   images were tagged `:latest` at the time, the restarted container picked
   up the STALE pre-fix binary — the actual fix sat built but unbooted under
   the same mutable tag, so `docker ps` gave no signal anything was wrong.
   Versioned tags make "what is actually running" readable straight off
   `docker ps`/`docker inspect`, and a rollback becomes
   `STYRMAN_VERSION=<old> docker compose up -d` — no rebuild, no guessing
   which binary a tag currently points at.
2. **The testing-guide incident.** The same day's whole-project audit found
   `.agents/docs/testing/styrman/integration.sh` defaulted to rebuilding
   `styrman-runtime:dev` — which happened to be the EXACT tag `deploy/.env`'s
   `STYRMAN_IMAGE` was pointing production at on this host. Running the
   documented test loop silently re-stamped the image every live project was
   about to boot from next. Fixed by moving the test default to `:test`
   (`.agents/docs/testing/styrman/README.md`); the underlying lesson — any
   mutable tag is a shared, ambient variable, not a private one — is now
   enforced structurally rather than by convention: `STYRMAN_IMAGE` has no
   default at all in `config.Load`, and every compose `image:` line has a
   hard `${STYRMAN_VERSION:?...}` guard.

`styrman-runtime:dev` and `styrman-runtime:test` remain legitimate — they are
LOCAL-ONLY, explicitly-passed tags for the dev loop and the test loop
respectively (`STYRMAN_IMAGE` is always set explicitly for both, never left
to a default). The rule is about what a compose/production deploy is allowed
to run, not a ban on mutable tags everywhere.

## Manual release procedure

1. Land the change (commit; the review/security/verifier gates and
   `/preflight` are a separate concern from this doc — see `AGENTS.md`'s
   Definition of Done).
2. Sign and push the tag — SSH signing isn't cosmetic here, it's the exact
   signature shape the auto-updater's `verify_tag` requires (below), so an
   unsigned or PGP-signed tag can only ever be deployed manually:
   ```bash
   git config gpg.format ssh && git config user.signingkey ~/.ssh/id_ed25519.pub
   git tag -s vX.Y.Z -m "styrman vX.Y.Z"
   git push origin vX.Y.Z
   ```
3. Build all three images and bring the stack up at that version:
   ```bash
   STYRMAN_VERSION=X.Y.Z
   docker build -t styrman-runtime:${STYRMAN_VERSION} \
     --build-arg STYRMAN_VERSION=${STYRMAN_VERSION} image/
   STYRMAN_VERSION=${STYRMAN_VERSION} \
     docker compose -f deploy/docker-compose.yml up -d --build
   ```
   (compose's `build:` blocks point at `deploy/Dockerfile.server` /
   `deploy/caddy/Dockerfile` — `styrman-runtime` is never built by compose,
   the first `docker build` line above is not optional.)
4. Verify: `curl -fsS https://styrman.<domain>/api/health`, confirm the
   Settings page shows `X.Y.Z`, sign out/in once to confirm the Google OAuth
   portal still redirects correctly (caddy-security surviving a version bump
   is the highest-risk step — an existing session staying logged in is NOT
   proof, test a fresh login), and open/close a project to confirm it boots
   on the new runtime tag.

## Updating a single image out-of-band

Bumping just the Caddy build (a plugin pin, a CVE fix) doesn't need a full
feature release of the server — but it still ships through the normal
version tag, not a side channel:

1. Edit `deploy/caddy/Dockerfile` — pins only, never a floating base
   (e.g. `caddy:2.11.4-builder`/`caddy:2.11.4`,
   `--with github.com/greenpau/caddy-security@v1.1.64`).
2. Rebuild it as part of the **next** release version — `docker-compose.yml`'s
   `image:` line is `styrman-caddy:${STYRMAN_VERSION}`, so a caddy-only
   change still goes out through the manual release procedure above; there
   is no separate versioning axis for one image.
3. Smoke test before calling it done — not optional for a Caddy bump
   specifically: `docker compose build caddy`, `caddy version` inside the
   built image, `caddy list-modules | grep -E
   "(security|cloudflare|rate_limit|ingress)"` (the Dockerfile's own `RUN`
   step already asserts this at build time), restart, confirm the site
   serves AND a fresh Google OAuth login round-trips — caddy-security
   across a Caddy version bump is the real risk (see
   `.agents/docs/reviews/2026-09-01-review-redfix.md`'s 5-step smoke test
   for the exact sequence this was verified against on 2026-09-01).

The same shape applies to a server-only or runtime-only bump: edit that one
Dockerfile, ship it inside the next `vX.Y.Z` release, verify before calling
it done.

## Pin-bump policy

Every `FROM`/`--with`/`ARG`-pinned dependency across all three Dockerfiles is
an EXACT version, never a floating tag or branch — the same "no mutable
tags" rule applied one layer down, since a floating **builder** breaks the
same guarantee a floating **image tag** does (the binary depends on the day
it was built, not the pin, and a rollback can't reproduce it). Bump pins
deliberately and together, not piecemeal under time pressure:

- `deploy/caddy/Dockerfile`: `caddy:X.Y.Z-builder`/`caddy:X.Y.Z`,
  `caddy-security@vX.Y.Z`, `caddy-dns/cloudflare@vX.Y.Z`,
  `mholt/caddy-ratelimit@vX.Y.Z`.
- `deploy/Dockerfile.server`: `golang:X.Y.Z-trixie`.
- `image/Dockerfile`: base image + tool versions — see `runtime-image.md`.

A bump that changes Caddy's major/minor needs the full smoke test above
before it lands — an untested `xcaddy build` against a new Caddy floor has
never been assumed safe here; it's what the redfix review's smoke test
existed to close out.

## The auto-updater, end to end

Ships **disabled**; enabling it is a deliberate, separate operator action —
full mechanics, security model, and the three setup prerequisites live in
`deploy.md`'s "## Auto-update (tag-gated, ADR-050/051)" section. Short
version:

- `styrman-update.timer` polls every 5 min for the newest tag matching
  `^v[0-9]+\.[0-9]+\.[0-9]+$` (`sort -V`, so pre-release tags like `-rc1`
  are ignored by construction and it's the highest version, not the most
  recently pushed).
- Every tag must be **SSH-signed** and verified against a root-owned
  `/etc/styrman/allowed_signers` allowlist — a PGP-signed tag is refused
  outright, not merely unpinned (ADR-051).
- It runs against a dedicated clone at `/opt/styrman`, never the operator's
  own working tree, so an automatic `git checkout <tag>` can never stomp
  in-progress work.
- On every tag it builds `styrman-runtime:X.Y.Z` AND both compose images
  (server + caddy) — never a floating tag, same rule as the manual
  procedure above.
- **Run-state (`deployed-version`, `deployed-sha`, `failed-version`) lives
  ONLY at `/etc/styrman/update`, pinned by `Environment=` in
  `styrman-update.service` — that unit is the ONLY place the production
  path is set.** The script itself has **no default at all**
  (`STYRMAN_UPDATE_STATE_DIR` is required, and the script `fail`s
  immediately if it's unset) and explicitly REFUSES any value inside or
  under `/var/lib/styrman` (ADR-052). This is a direct fix for the
  2026-09-01 state-loss incident: a privileged test run against a
  production-aliasing default state path preceded `/var/lib/styrman` being
  emptied and `/etc/styrman` being `rm -rf`'d. Anything that invokes
  `styrman-update` outside the systemd unit — including tests — MUST pass
  its own scratch prefix (e.g. `$(mktemp -d)`); see
  `.agents/docs/testing/styrman/README.md`'s ADR-052 section for the
  convention this generalizes to.
- It verifies a deploy by reading the `styrman.version` LABEL off the
  running `styrman-server` container (`docker inspect`) — not HTTP, since
  production's `STYRMAN_AUTH=header` would 403 an unauthenticated poll and
  make every healthy release look like a failure.
- **It also keeps itself current — but only when signature enforcement is
  ON** (`REQUIRE_SIGNED=1`, the default; the `/etc/styrman/update.env`
  unsigned-tag opt-out never self-installs). After a health-checked deploy
  (forward OR rollback) made under enforcement, it installs its own
  `deploy/update/{styrman-update,
  styrman-update.service,styrman-update.timer}` from the verified git
  object at the deployed commit — never the checkout's worktree — over the
  live `/usr/local/bin`/`systemd` copies, atomically (temp file + rename),
  taking effect on the next tick — see `deploy.md`'s Auto-update section
  for the exact mechanics. Targets are pinned the same ADR-052 way
  as the state dir (`STYRMAN_UPDATER_BIN`/`STYRMAN_UPDATER_UNIT_DIR`, no
  in-script default). **One-time exception**: v1.17.3, the release that
  introduces this, is deployed by the OLD script and still needs one manual
  reinstall right after that single deploy — every release after it
  self-installs.

## Rollback

- **Manual**: images are immutable once built, so rollback needs no
  rebuild — `STYRMAN_VERSION=<old-x.y.z> docker compose -f
  deploy/docker-compose.yml up -d` re-points every `image:` line at the
  already-built tag. Confirm the old `styrman-runtime:<old-x.y.z>` tag
  wasn't pruned (`docker image ls`) before relying on this.
- **Automatic**: if the newly-deployed tag doesn't report itself healthy
  within `STYRMAN_UPDATE_HEALTH_TIMEOUT` (default 120s), `styrman-update`
  latches the tag at `${STATE_DIR}/failed-version` (so the timer stops
  retrying it) and redeploys the previous version — preferring the exact
  recorded commit SHA over the tag name when one was recorded, since that
  tree was already verified once and re-verifying by tag name would refuse
  any release made before SSH signing was adopted (v1.14.0–v1.15.0). The SHA
  path IS live: `styrman-update` records `deployed-sha` on every verified
  deploy and its rollback prefers that SHA as a trusted ref, falling back to
  tag-name re-verification only when no SHA was recorded. See `deploy.md`'s
  Auto-update section for the exact mechanics.
