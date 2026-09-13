---
owner: security
date: 2026-09-13
---
# Security audit — root-side scripts and the runtime image   (2026-09-13)

Range reviewed: working tree at `e56e775708a9062cc6432f5c3278770bed63b685` (clean).
Every `file:line` below is pinned to that commit.

Surface: `deploy/styrman-ctl`, `deploy/update/styrman-update` (+ its two units),
`deploy/firewall/{styrman-firewall.sh,install.sh,test.sh,*.service,*.timer}`,
`deploy/host-setup.sh`, `image/Dockerfile`, `image/entrypoint.sh`,
`image/configs/{styrman-browser,styrman-setup,styrman-credentials.sh,styrman-git-credential}`,
and the ADR-052 scratch-prefix rule on the privileged test paths. Blast radius
followed into `internal/config/config.go` (bridge naming),
`internal/credbroker/broker.go` and `internal/orchestrator/orchestrator.go`
(broker address), plus read-only inspection of the live host's Docker network
state and a throwaway git repo used to measure git's signature parsing.

## Verdict: FINDINGS

Two defects stand out. **First**, the credential broker's address is computed
twice, in two files on this surface, with two different subnet assumptions, and
the two stopped agreeing on 2026-09-10: `deploy/host-setup.sh:128` pins Docker's
`default-address-pools` to `172.16.0.0/12` **size 24**, while
`image/configs/styrman-credentials.sh:29` still derives the broker host as the
guest's first two octets plus `.255.254`, which is only correct for a `/16`. On
the live host the project network is `172.16.1.0/24`, the injected
`STYRMAN_CRED_BROKER` is `http://172.16.1.254:8091`, and the login-shell snippet
overwrites it with `http://172.16.255.254:8091` — an address nothing serves.
R3 ("sign in once, every project stays signed in") is broken outright, silently,
in every project on a /24 bridge. This is BUG-035's recorded mechanism — a change
that did not reach every caller — which makes it more severe, not less.

**Second**, `verify_tag`'s armor-header check (`deploy/update/styrman-update:313-315`)
recognises two of the four signature markers git actually dispatches on.
Measured against git 2.47.3: a tag whose last git-recognised marker is
`-----BEGIN PGP MESSAGE-----` or `-----BEGIN SIGNED MESSAGE-----` makes git run
**gpg**/**gpgsm**, while the script's grep reports an earlier decoy
`-----BEGIN SSH SIGNATURE-----` as the last header and the check passes — landing
on exactly the keyring-only, allowlist-free path the function exists to refuse.

## Findings

- **[high / confidence: high]** `image/configs/styrman-credentials.sh:25-31` (with
  `deploy/host-setup.sh:122-131`) — **the guest recomputes the broker address for a
  `/16` bridge; bridges have been `/24` since 2026-09-10, so credential
  materialization is dead.** The snippet takes the guest's own IPv4, keeps
  `cut -d. -f1,2` (two octets) and appends `.255.254`. The host side
  (`internal/orchestrator/orchestrator.go:475` `brokerIPForSubnet`) computes
  broadcast−1 of the **actual** mask and attaches the server there. With the pool
  `{"base":"172.16.0.0/12","size":24}` those differ. Measured on this host:
  network `sdb-32c61123-…` is `172.16.1.0/24`; container env
  `STYRMAN_CRED_BROKER=http://172.16.1.254:8091`; the snippet's own expansions,
  replayed verbatim, produce `http://172.16.255.254:8091`. The override is
  unconditional for any global IPv4 address, and the failure path is
  `curl -fsS --connect-timeout 1 … || true` — silent.
  Consequences: no `GH_TOKEN`/`NPM_TOKEN`/`CLAUDE_*` env creds, no gcloud ADC, no
  SSH key, no `git config user.name/email`; and because the snippet assigns to an
  already-exported variable, `styrman-git-credential` inherits the wrong URL too,
  so HTTPS git auth falls through in every login shell. Users routed around this
  will paste tokens into project files, against R10.
  Second-order: `172.16.255.254` is an address the operator does not control. For a
  SANDBOX project the firewall's `172.16.0.0/12` DROP
  (`deploy/firewall/styrman-firewall.sh:75-78`) means nothing answers; for a **LAN**
  project (ADR-067 — no styrman rule matches `lan-<hash>`) whatever answers there
  gets to drive `GET /files`, whose response the snippet writes into `$HOME` at
  caller-chosen relative paths and modes (`:56-72`) — `~/.bashrc`, `~/.gitconfig`,
  `~/.claude.json` are all in range. The request is an unauthenticated GET, so
  nothing is leaked outbound; the response is trusted.
  **Fix:** delete the octet arithmetic and use the guest's own prefix length —
  `ip -o -4 addr show scope global` already prints `addr/len`, so compute
  broadcast−1 of that CIDR. Simplest and least duplicated: drop the recomputation
  entirely and trust the injected `STYRMAN_CRED_BROKER`, re-injecting it on the
  network-recreation path the M17 comment worries about (the orchestrator already
  knows the new subnet). Add a test that feeds the snippet a `/24` address, and
  make the MOTD or the boot log say "broker unreachable" instead of staying quiet.
  Update `.agents/docs/specs/styrman/runtime-image.md:250`, which still states
  "broadcast−1 of its `/16`".

- **[medium / confidence: high]** `deploy/update/styrman-update:313-315` — **the
  "must be SSH-signed" gate misses two of git's four signature markers, so a tag
  can be routed to the keyring-only verifier while passing the check.** The grep is
  `^-----BEGIN (SSH|PGP) SIGNATURE-----` and the last match must be the SSH one.
  git dispatches on four markers (`gpg_formats` in `gpg-interface.c`):
  `PGP SIGNATURE`, `PGP MESSAGE`, `SIGNED MESSAGE` (x509/gpgsm) and `SSH SIGNATURE`.
  Measured with git 2.47.3 on a throwaway repo, using placeholder armor blocks
  only (no real signature, no bypass constructed):

  | tag object contains (in order)          | verifier git launched | script's `tail -1` |
  |---|---|---|
  | SSH SIGNATURE, then PGP SIGNATURE       | `gpg`                 | PGP SIGNATURE → refused ✔ |
  | PGP SIGNATURE, then SSH SIGNATURE       | ssh (`sig_find_principals`) | SSH SIGNATURE → allowed ✔ |
  | SSH SIGNATURE, then **PGP MESSAGE**     | `gpg`                 | **SSH SIGNATURE → allowed ✘** |
  | SSH SIGNATURE, then **SIGNED MESSAGE**  | `gpgsm`               | **SSH SIGNATURE → allowed ✘** |

  The last two rows are the gap: the check passes, `gpg.ssh.allowedSignersFile` is
  then irrelevant, and the script's own comment (`:277-281`) records why that path
  is fatal — "with GPG it exits 0 for a good-but-untrusted signature, so any key
  ever imported into root's keyring passes, and git never compares signer to
  tagger". The tag message is attacker-controlled, so planting the decoy costs
  nothing. **Bounding condition, stated honestly:** completing the bypass still
  needs a key in root's GnuPG keyring, and the production timer runs with
  `ProtectHome=yes` (`styrman-update.service:81`), which blanks `/root` and makes
  gpg fail. So the production path currently fails closed for a reason nobody
  designed, and the documented hand-run recovery (`sudo styrman-update --force`,
  `host-setup.sh:333`, and the updater's own "re-run with --force" messages) does
  not. That is a control resting on an accident.
  **Fix (two lines, both cheap):** widen the regex to git's whole marker set —
  `^-----BEGIN (SSH SIGNATURE|PGP SIGNATURE|PGP MESSAGE|SIGNED MESSAGE)-----` — and
  keep requiring the last match to be `-----BEGIN SSH SIGNATURE-----` (verified
  against all four fixtures plus an honestly SSH-signed tag: the honest tag still
  passes, and an empty allowlist still refuses it). Then belt-and-braces: run the
  verification with `GNUPGHOME` pointed at an empty directory so the openpgp and
  x509 paths can never succeed whatever the parse decides. Add the four fixtures as
  a test — the check has now been wrong once in a way no green run would show.

- **[medium / confidence: high]** `image/Dockerfile:89-91` — **the Go toolchain is
  fetched unpinned and unverified into `/usr/local` as root at build time**, in a
  file whose own comment at `:50-62` states the opposite rule ("Every remaining
  tool is pinned by version AND verified by sha256 (audit 2026-09-02)") and
  explains why it matters ("an unpinned tool makes the tag→image mapping
  non-deterministic, so the auto-updater's rollback would not restore the image the
  old tag produced"). `GO_VERSION` is whatever `go.dev/dl/?mode=json` returns at
  build time and the tarball is piped straight into `tar -C /usr/local -xz` with no
  checksum — the largest root-extracted archive in the image. Rolling back to
  `styrman-runtime:X.Y.Z` after a failed release (`deploy/update/styrman-update:577`)
  rebuilds from this Dockerfile, so the rolled-back runtime is not the one the old
  tag produced. **Fix:** `ARG GO_VERSION` + `ARG GO_SHA256` alongside
  `TOFU_*`/`YQ_*`/`HADOLINT_*`, verified with `sha256sum -c -` (the go.dev JSON
  publishes each file's `sha256`), bumped deliberately per `runtime-image.md`.

- **[medium / confidence: high]** `image/Dockerfile:92` and `:97` — **two
  `curl … | bash` installers still run as root at image-build time.**
  `https://deb.nodesource.com/setup_22.x | bash -` and
  `https://antigravity.google/cli/install.sh | bash`. The same file removed exactly
  this pattern for OpenTofu (`:56-58`: "an unauthenticated remote shell script
  running as root at build time, fetching whatever version was current that day")
  and for Helm (`:72-74`: "the worst of the unpinned fetches"). Both survivors are
  unversioned and unverified, and both are trusted to edit apt sources or install
  binaries the agents then run. **Fix:** for NodeSource, do what the Dockerfile
  already does three times at `:38-44` — fetch the key into `/etc/apt/keyrings` and
  write the `sources.list.d` entry inline, executing no script. For `agy`, pin a
  release artifact URL + sha256. If neither is possible, record the exception in
  `runtime-image.md` beside the ones that were closed, so it is a decision rather
  than an oversight.

- **[medium / confidence: high]** `image/Dockerfile:96` — **`npm install -g
  @anthropic-ai/claude-code` is unpinned and runs lifecycle scripts as root**, 27
  lines above the pinned, `--ignore-scripts` install of `chrome-devtools-mcp` whose
  comment (`:114-119`) states the rule ("no transitive package runs a lifecycle
  script as root at build time (security review 2026-09-11)") and then asserts,
  incorrectly, "only the top-level versions are pinned" — claude-code's is not. Any
  release of claude-code or of any transitive dependency executes arbitrary code as
  root during the image build, and the tag→image mapping is again non-deterministic
  (same rollback consequence as the Go finding). **Fix:** `ARG
  CLAUDE_CODE_VERSION=<x.y.z>` and `npm install -g --ignore-scripts
  "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"`. If claude-code genuinely
  needs a postinstall, say so in the comment rather than leaving the blanket claim.

- **[low / confidence: high]** `deploy/host-setup.sh:199-207` — **`deploy/.env` is
  created with the ambient umask and only `chmod 600` afterwards**, so the file
  holding the freshly generated `STYRMAN_INGRESS_TOKEN` and `STYRMAN_JWT_KEY`
  exists world-readable for the duration of the `sed`. Root's default umask on
  Debian is 022 → 0644. The window is short and the directory is the operator's
  checkout, but the rest of the system is careful about exactly this
  (`deploy/update/styrman-update:193` refuses a `deploy/.env` readable beyond root).
  **Fix:** `(umask 077; sed … > "${SCRIPT_DIR}/.env")`, or write via a `mktemp` file
  created 0600 and `mv` it into place.

- **[low / confidence: high]** `deploy/host-setup.sh:50` — **Docker is installed by
  piping `https://get.docker.com` to `sh` as root**, while the Kata install 20 lines
  below refuses to proceed without a checksum and says why ("extracting an
  unverified tarball as root into `/` is a code-execution vector on a
  MITM'd/tampered asset. Refuse rather than warn-and-continue"). `VERSION=` pins the
  packages, not the script, and R11 asks for a reproducible installation from this
  repo. **Fix:** install `docker-ce` from the official apt repo with a pinned
  keyring — the pattern `image/Dockerfile:38-40` already uses — or at minimum fetch
  the script to a file, check it against a pinned digest, then run it.

- **[low / confidence: medium]** `deploy/host-setup.sh:65-68` — **the Kata checksum
  is fetched from the same origin as the artifact it checks**, so with `KATA_SHA256`
  unset (the documented default path) it catches corruption but not a substituted or
  compromised release asset. The `FATAL` at `:73` reads as a stronger control than it
  is. **Fix:** ship the expected digest for the pinned `KATA_VERSION` in-repo as the
  default for `KATA_SHA256` and treat the network fetch as the fallback — the
  version is already pinned at `:15`, so the digest can be too.

- **[low / confidence: high]** `deploy/firewall/styrman-firewall.sh:69-70` — **the
  one rule whose absence breaks the credential broker warns with a bare `echo`, not
  `warn()`**, so it never enters `WARNS`, the script still exits 0 (`:160-162`), and
  the systemd unit still reports success. Without `xt_physdev` the same-bridge RETURN
  is missing and the `172.16.0.0/12` DROP at `:75-78` swallows broker and
  port-preview traffic for every sandbox project — a total credential outage that
  monitoring reads as green. BUG-034's family: silence from your own
  instrumentation. **Fix:** call `warn` so the condition reaches the exit path like
  every other degraded state in this script.

- **[low / confidence: high]** `deploy/styrman-ctl:555` — **`${ETC_DIR}` is
  interpolated unquoted into the shell string git evaluates as a credential
  helper**, which is the precise construction `deploy/update/styrman-update:207-211`
  documents removing after a security review ("the path is never interpolated into
  the git-evaluated shell string … an unquoted interpolation made a path with spaces
  yield an empty password, and operator-env paths are a root shell-injection
  surface"). Production is protected by the test-seam guard at `styrman-ctl:44-48`,
  which refuses `STYRMAN_ETC_DIR` unless `STYRMAN_CTL_TEST=1` — so this is
  defence-in-depth, not a live hole, hence low. **Fix:** mirror the updater — export
  `GH_TOKEN_FILE` and let the single-quoted helper read it from its own environment.

- **[low / confidence: high]** `deploy/styrman-ctl:646` — **`edit KEY=VALUE` accepts
  secrets on the command line**, so `styrman-ctl edit GOOGLE_CLIENT_SECRET=…` puts
  the OAuth secret (or `CLOUDFLARE_API_TOKEN`) into root's shell history and into
  every `ps` listing for the life of the process. The same file states the opposite
  rule 100 lines below — "Secrets travel through root-only temp files, never argv or
  the plan" (`:546`) — and `--gh-token-file` + `private_file` already implement it
  for the updater token. **Fix:** for keys in `SECRET_KEYS`, refuse an inline value
  and fall back to the masked `read -rs` prompt, or accept `KEY=@/path/to/file` and
  route it through `private_file`.

- **[low / confidence: high]** `deploy/host-setup.sh:224` and
  `deploy/firewall/install.sh:36,41-46` — **dead configuration surface left by
  ADR-067.** host-setup passes `STYRMAN_STATE_DIR` into `firewall/install.sh`, which
  has read no such variable since ADR-067; install.sh still accepts and silently
  consumes `--state-dir --lan-dev --lan-cidr --lan-allow-host
  --lan-allow-published-ports --ingress-bridge` (each `shift 2`) and
  `--i-accept-sshd-passwords`. Keeping the arg-eating aliases is defensible for one
  release; the env var is pure noise and `--run-dir` now exists only to locate things
  to delete. **Fix:** drop the `STYRMAN_STATE_DIR=` prefix at `host-setup.sh:224`;
  put a release number in the obsolete-flag comment so the block has an expiry.

- **[low / confidence: medium]** `image/entrypoint.sh:165` — **a build-time string is
  interpolated into an `sh -c` command line.**
  `runuser -u "${USERNAME}" -- sh -c "printf '%s' '${HARNESS_VER}' > '${HARNESS_STAMP}'"`
  single-quotes `${HARNESS_VER}` inside a double-quoted string, so a value containing
  `'` breaks out and the remainder executes as the project user. `HARNESS_VER` comes
  from `/styrman/VERSION` (the `STYRMAN_VERSION` build arg), which the updater
  constrains to `vX.Y.Z` (`styrman-update:244`) — hence medium confidence and low
  severity — but `host-setup.sh:248`'s `setup-$(date …)` fallback and any hand
  `docker build --build-arg` are unconstrained. **Fix:** pass the value as an
  argument, as the launcher already does (`styrman-browser:188`):
  `sh -c 'printf "%s" "$1" > "$2"' _ "${HARNESS_VER}" "${HARNESS_STAMP}"`.

## Cleared on inspection

- **Tag verification, everything but the marker set** (`styrman-update:293-339`) —
  refuses lightweight tags; asserts `allowed_signers` exists, is root-owned and is
  not group/world-writable *before* `verify-tag`; passes
  `gpg.ssh.allowedSignersFile` explicitly rather than trusting root's gitconfig
  under `ProtectHome=yes`; re-verifies inside `deploy()` (`:562-566`). Measured: an
  honestly SSH-signed tag verifies against a matching allowlist and is refused
  against an empty one. The one gap is the marker set, above.
- **Rollback trust** (`:669-682`, `:700-714`) — rolls back by recorded 40-hex SHA,
  refuses a malformed value, and records the SHA **only** when `REQUIRE_SIGNED=1`,
  deleting it otherwise, so an opt-out window cannot mint a permanently trusted ref.
- **Clone and credential handling** (`:177-229`) — asserts the worktree root *and*
  `.git` are root-owned and not group/world-writable before `-c safe.directory`;
  asserts `deploy/.env` is root-owned and `-perm /077` clean; always resets inherited
  credential helpers; scopes the token helper to `https://github.com`; reads
  `GH_TOKEN_FILE` from the exported environment rather than interpolating it;
  `GIT_TERMINAL_PROMPT=0`. No token in a remote URL, none in a process listing.
- **ADR-052 handling** (`styrman-update:77-107`; `styrman-update.service:47-57`;
  `firewall/test.sh:23-33`; `internal/config/styrman_ctl_test.go:87-112`) — no
  in-script state default; refusal of `/var/lib/styrman*`, `/` and bare top-level
  dirs after `realpath -m`; `install -d` only when the dir is absent (BUG-009 and the
  `/tmp` re-mode finding); `DOCKER_CONFIG` honoured only under `/run` and only when
  root-owned and not group/world-writable. The firewall test refuses to run in the
  host's network namespace by comparing `/proc/self/ns/net` with `/proc/1/ns/net`
  before it mounts or changes anything; the ctl test runs every path under a scratch
  root and asserts no production path appears in the printed plan.
- **`host_converge` ordering** (`:641-656`) — runs before the FAILED latch and the
  up-to-date exit (BUG-008), changes nothing under `--check` (`:513-521`), and calls
  `systemctl enable styrman-firewall.service` **unconditionally** (`:544`) — BUG-035's
  recurrence closed, and `firewall/install.sh:113` carries the same call.
- **`deploy/firewall/styrman-firewall.sh`** — insert-if-missing only; the one-time
  migration deletes superseded direct rules *after* the replacements exist (`:81-90`);
  the ADR-065/066 cleanup matches whole lines against a strict regex, refuses to
  delete any tagged `DROP`, and rebuilds argv from constrained capture groups instead
  of re-splitting text (BUG-013); `shopt -s inherit_errexit` (BUG-010); `-w 5` on
  every `iptables` call; any warning forces exit 1.
- **R12 bridge naming** — `internal/config/config.go:487-494` derives `sdb-`/`lan-`
  plus 11 hex of sha1(network name), 15 chars exactly;
  `createBridgeNetwork` (`internal/orchestrator/orchestrator.go:891-898`) is the
  single creation site and always passes the mode; `bridgename_test.go` and
  `firewall_script_test.go` pin the prefix the `-i sdb-+` wildcard matches. A project
  cannot influence its own interface name.
- **`styrman-ctl` uninstall** (`:719-834`) — every recursive delete passes
  `guard_path` (canonical, ≥2 components, not a symlink, exactly one configured tree)
  *and* `looks_like_ours` (basename must be `styrman`, system prefixes refused by
  name, structural markers checked with `real_dir`/`real_file` so a planted link does
  not count); a failed marker check skips rather than deletes and raises
  `TEARDOWN_FAILED`; `drop_chain` never flushes a chain that is still referenced;
  the plan is printed by a `DRY=1` subshell before the confirmation word is asked.
- **`styrman-ctl` env handling** — `env_get`/`env_set` pass the key as an `awk`
  variable and the value through `ENVIRON` (never `-v`, which decodes escapes),
  refuse newlines and backslashes, refuse a symlinked target, write 0600 via temp +
  rename; `validate` is allowlist-per-key with control bytes refused everywhere;
  `printable` strips control bytes from anything echoed back; `private_file` asserts
  owner, mode and parent mode before reading a secret; `https_origin` refuses any
  origin that is not a plain `https://` URL, so `ext::`/`file://` can never reach a
  root `git clone`.
- **`image/configs/styrman-git-credential`** — validates the `host` field against
  `[A-Za-z0-9.-]` before putting it in a URL, and the broker keys the lookup by host
  (`internal/credbroker/broker.go:83-96`), returning 404 for any host without a
  stored credential. `credential.useHttpPath false` plus a system-wide helper does
  **not** hand the GitHub token to an arbitrary clone target.
- **`internal/credbroker/broker.go`** — every route resolves the caller by source IP
  and 403s an unresolved one; no token in a URL; no route serves another user's
  material.
- **`image/configs/styrman-browser`** — reads listening-socket inodes from
  `/proc/net/tcp{,6}` rather than parsing `ss`; confirms the recorded pid is alive,
  is `socat`, and holds the socket; pid files constrained to 1–7 digits, no leading
  zero, `>= 2`; `range=<ip>/32` accepts only a whole-value IPv4 match; the lock fd is
  closed on every `exec` (BUG-042); `refuse_links` runs before anything is created.
  Everything here is same-uid inside a microVM the project user already owns through
  `sudo`.
- **`image/configs/styrman-setup`** — prints only the **public** key; no credential
  reaches stdout or a log; non-interactive runs never prompt.
- **`image/entrypoint.sh` root phase** — the `sudoers.d` write and `docker` group
  membership are unreachable unless `id -u "${USERNAME}"` already equals the host uid
  (`:51-54`), so an odd or reserved name fails the boot instead; the apt replay uses
  `--` so a guest-written manifest cannot become flags; every path it touches is
  inside that project's own binds.
- **Secrets in the tree** — no credential material in any audited file;
  `deploy/.env` is gitignored and `deploy/.env.example` carries placeholders only.
- **Mechanical checks** — `bash -n` clean on all twelve audited shell files;
  `shellcheck -S warning` clean apart from one false positive (SC2088 on the literal
  `~/` *pattern* at `styrman-credentials.sh:60`, which is a `case` glob, not an
  expansion).
- **GCP surface** — none. The project is self-hosted; `docs/deploy/gcp.md` records
  "not applicable". No IAM binding, service-account key or `--allow-unauthenticated`
  anywhere on this surface.

## Not examined

- **`image/configs/harness-migrate.sh`** and `image/harness/**` — the vendored
  `styrman-harness` mirror, including the `sh-agent` source built into the image at
  `Dockerfile:8-13` and run against each guest-writable home at
  `entrypoint.sh:144-146`. Not read. This is the largest unreviewed code path on the
  surface.
- **`deploy/styrman-ctl:876-961`** — the `cmd_internal` test hooks, `cmd_help`,
  `cmd_menu` and `main`'s argument parsing were skimmed for the gate at `:880`
  (`STYRMAN_CTL_TEST=1` required) but not audited line by line.
- **`deploy/docker-compose.yml`, `deploy/Dockerfile.server`, the vendored Caddy
  ingress module** — outside the named surface. R12's `ip_forward=0` claim and the
  ingress-token guard were taken from the documents, not verified in the compose
  file or the Go handler.
- **Runtime verification of the firewall rules on the live host** — no `iptables -S`
  was run. The rule analysis is from the script's text, `deploy/firewall/test.sh`'s
  assertions, and one read-only `docker network inspect`.
- **`govulncheck` / `npm audit`** — not run. This audit changed no dependency;
  `deploy/vulncheck.sh` (both Go modules inside the pinned builder image) and
  `cd web && npm audit` are the project's own commands and belong to the verifier's
  tier, not to a read-only audit of shell scripts and Dockerfiles.
- **IPv6 isolation timing** — `styrman-firewall.sh:95-111` disables IPv6 only on
  bridges that exist at tick time and wraps every `ip6tables` call in `|| true`, so a
  bridge created just after a tick is unprotected for up to 60 s if `ip6tables` also
  fails silently. Not pursued because `host-setup.sh:114-117` forces `ipv6` and
  `ip6tables` to `false` at the daemon, which should mean no project bridge ever
  carries an IPv6 address — that assumption was not verified on the live host.
