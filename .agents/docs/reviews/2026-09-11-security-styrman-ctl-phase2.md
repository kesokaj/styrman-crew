---
owner: security
date: 2026-09-11
---
# Security review — styrman-ctl phase 2 (install + update --setup)   (2026-09-11)

Range reviewed: 8344f06..c334efd (`git show c334efd --`), i.e. the newest commit on `main`.
Blob pinned: `deploy/styrman-ctl` at c334efd = `bf80942`.

NOTE — the working tree MOVED under this review. `deploy/styrman-ctl` on disk is
`fb16dce` (phase 3 work in progress: `env_set` gained a `--dry-run` early return,
`cmd_update` gained `--redeploy`, `cmd_edit` appeared). Findings below are against the
COMMITTED blob and were re-checked against the working copy; where a line number differs
between the two, both are given. The drift fixes exactly one half of one finding (F7's
dry-run disk write); `cmd_edit` (uncommitted) WIDENS F1, taking `KEY=VALUE` from argv into
the same `env_set` and writing both copies of `.env`.

## Verdict: FINDINGS

The worst of it is F1: `env_set` refuses a value containing a newline — an explicit,
commented guard against smuggling a second assignment into `deploy/.env` — and then hands
the value to `awk -v v=…`, which expands backslash escapes on assignment. A value whose
two characters are `\` `n` therefore becomes a real newline inside awk, and the guard is
bypassed. Verified on this host's awk (GNU Awk 5.2.1, and mawk is also present; both do
POSIX escape processing on `-v`): a `GOOGLE_CLIENT_SECRET` value of
`x\nSTYRMAN_IMAGE=…` produces two assignment lines in the file. The injected line is
never validated, lands in a root-deployed `.env`, and is handed straight to
`docker compose --env-file`, where `STYRMAN_IMAGE` picks the image every project microVM
boots and `BASE_DOMAIN` picks the hostname Caddy serves. Two more findings sit around the
GitHub token: the one-shot `GIT_ASKPASS` helper is host-agnostic where both the prose it
replaces (host-setup.sh 8c) and the updater it configures are deliberately scoped to
`credential.https://github.com.helper`, and the clone URL reaches `git clone` as a bare
first positional with no `--`.

## Findings

- **F1 [medium / confidence: high]** `deploy/styrman-ctl:115-118` (working copy `:118`) —
  the newline guard on line 109 is defeated by `awk -v`. `awk -v v="${value}"` performs
  escape-sequence expansion on the assignment (POSIX; verified on this host for gawk 5.2.1, `gawk --posix` and mawk — all three), so
  the literal two-character sequence `\n` in a value becomes a newline in the output line.
  `validate` for `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` and `CLOUDFLARE_API_TOKEN` is
  only `^[^[:space:]]+$`, and a backslash and an `n` are not whitespace, so the value
  passes validation and `env_set` writes two lines. Scenario: the operator is handed a
  "client secret" (or an `--env-file` for the unattended path, `cmd_install`'s
  `--env-file` at `:376`/`:404`) whose value is
  `<real-secret>\nSTYRMAN_IMAGE=ghcr.io/attacker/img:1`. `STYRMAN_IMAGE` is absent from
  `.env.example`, so nothing later in the file overrides it, and every project microVM
  then boots the attacker's image; `BASE_DOMAIN` (line 8 of `.env.example`) is likewise
  overridable from any key below it, since dotenv takes the last assignment. The
  uncommitted `cmd_edit` takes `KEY=VALUE` straight from argv into the same path, and
  writes BOTH the checkout's and the clone's `.env`. Remediation: never pass data through
  `awk -v`. Pass it in the environment and read `ENVIRON["v"]` inside the program, or
  drop awk for a bash read/print loop. Additionally reject `\` in a value, or validate the
  file after writing (`grep -c '^KEY='` must be 1 and the line count must be unchanged).
- **F2 [medium / confidence: medium]** `deploy/styrman-ctl:460,468,471` — the one-shot
  askpass answers ANY credential prompt with the GitHub token. `GIT_ASKPASS` is not
  host-scoped: git invokes it for whatever host the URL names, including after an HTTP
  redirect to another host, and `https_origin` (`:362-368`) never asserts the scheme is
  https — it only rewrites the two ssh forms, so an `http://`, `git://` or arbitrary-host
  origin is passed through unchanged and the token would go out in cleartext or to a
  third party. This is a deliberate departure from the mechanism it replaces:
  host-setup.sh 8c's prose (`deploy/host-setup.sh:317-319`) and the updater itself
  (`deploy/update/styrman-update:215,227`) both use `-c credential.helper=` plus
  `-c 'credential.https://github.com.helper=…'`, and the updater's comment says in so
  many words that clearing is there "so an ambient root credential store can never
  silently answer for a fetch". `setup_auto_update`'s `git clone`/`git fetch` clear
  nothing, so an ambient helper in `/root/.gitconfig` or `/etc/gitconfig` can also answer
  first. Scenario: the operator's checkout has `origin =
  http://internal-mirror.example/styrman.git` (or a host that 302s elsewhere); one
  `styrman-ctl update --setup` hands the fine-grained token to that host. Remediation:
  reject any origin that is not `https://`, and use the updater's exact host-scoped
  credential-helper form instead of `GIT_ASKPASS`, so the token is offered only to the
  host the operator pinned.
- **F3 [low / confidence: high]** `deploy/styrman-ctl:471` (working copy `:508`) — the clone
  URL reaches a root `git clone` as a bare first positional: no `--` end-of-options marker
  and no scheme allow-list. `https_origin` (`:362-368`) rewrites the two ssh forms and
  returns EVERYTHING ELSE verbatim, so whatever sits in the checkout's `.git/config` is
  what root runs. Two grammars get through: an argument beginning with `-` is parsed by
  git as an OPTION (probed non-destructively — `git clone --upload-pack=/bin/false <repo>
  <dir>` reports the NEXT argument as the repository, i.e. the dashed argument was
  consumed as a flag, and git keeps parsing flags after positionals too), and git's
  non-http transports (`ext::`, `file://`) are accepted as URLs, with `protocol.ext.allow`
  defaulting to `user` — i.e. allowed for a direct clone like this one. The operator
  account is not a privilege boundary on this host (`.agents/docs/deploy/remote.md:101`
  records `%sudo NOPASSWD:ALL`), so this is defence in depth, not a live escalation — but
  `.git/config` is writable by anything running as the operator (an editor plugin, a
  package postinstall inside the checkout), and the payoff is the next
  `sudo styrman-ctl install`. Remediation: one line —
  `[[ "${o}" =~ ^https://[A-Za-z0-9.-]+/ ]] || die "origin must be an https URL"` in
  `https_origin`, plus `git clone -- "${origin}" "${DEPLOY_DIR}"`.
- **F4 [low / confidence: high]** `deploy/styrman-ctl:466-468` — the `fetch --tags` branch
  fires when `${DEPLOY_DIR}/.git` merely exists, and fetches from whatever remote THAT
  clone's config names, with the token offered (F2). Nothing checks that the existing
  clone's origin matches the checkout's origin, that its `.git` is root-owned, or that it
  is not group/world-writable — the three things `trusted_dir` in
  `deploy/update/styrman-update:73,177-185` insists on before it will trust the same
  directory. Remediation: run the updater's `trusted_dir` bar and compare
  `git -C "${DEPLOY_DIR}" remote get-url origin` to `https_origin` before fetching;
  refuse otherwise.
- **F5 [low / confidence: high]** `deploy/styrman-ctl:605-613` (committed `:513-521`) — the
  `__*` internal test hooks are reachable in the shipped script via `main`'s
  `__*) cmd_internal …` arm, and `__env-set FILE KEY VALUE` performs an arbitrary-file
  mutation as root with no `need_root`, no path guard and no restriction to the styrman
  trees: it replaces or appends one `KEY=VALUE` line in any existing non-symlink file.
  On THIS host the operator already has `%sudo NOPASSWD:ALL`
  (`.agents/docs/deploy/remote.md:101`), so nothing is gained today; on any host where the
  operator's sudo is restricted to `styrman-ctl` — the natural way to hand out a front
  door — it is a root escalation — e.g. appending an uppercase
  assignment to a file that is sourced or read as an environment. Remediation: gate
  `cmd_internal` on an env var the Go test sets (`[ -n "${STYRMAN_CTL_TEST:-}" ] || die`),
  and route `__env-set`'s FILE through `guard_path`-style checking.
- **F6 [low / confidence: high]** `deploy/styrman-ctl:436-448` — neither `--gh-token-file`
  nor `--env-file` nor `--allowed-signers-file` is checked for ownership or mode before
  being read, and the token file is left on disk afterwards. `install -m 600` is used for
  the destination, so the destination is fine; the SOURCE may be a world-readable file in
  the operator's home or in `/tmp`, and the unattended path (`--yes --gh-token-file`) is
  the one most likely to leave it there. Remediation: refuse a source that is a symlink,
  not owned by root or the invoking operator, or group/world-readable, and tell the
  operator to delete it (or `shred`) when the run succeeds — the 8c prose avoided a file
  entirely by piping through `read -rs`.
- **F7 [low / confidence: medium]** `deploy/styrman-ctl:452-455,493` — the token's temp copy
  is cleaned only by an `EXIT` trap. There is no `INT`/`TERM`/`HUP` trap, and `git clone`
  of this repo is exactly the multi-minute step an operator interrupts. The exposure is
  small (mode 600 under a `mktemp -d` 700 directory, and the same token is installed at
  `/etc/styrman/gh-token` anyway), but a leftover under `$TMPDIR` outlives the run. The
  committed blob also wrote the real token to disk under `--dry-run`, which does NOT need
  root — i.e. under an unprivileged `$TMPDIR`; the working copy has already fixed that half
  (`:483-498` sets `tmpd="<tmpdir>"` and writes nothing), though it still PROMPTS for the
  token before the dry-run branch. The EXIT-only trap stands in both. Remediation:
  `trap 'rm -rf "${tmpd}"' EXIT INT TERM HUP`.
- **F8 [low / confidence: high]** `deploy/styrman-ctl:430,445` — `is_signers_line` is
  unanchored at the end (`[A-Za-z0-9+/=]+` with no `$`), so anything may follow the key,
  and `--allowed-signers-file` silently uses only `head -1`: an operator supplying a file
  with two signers gets one, with no warning, and only discovers it when a release signed
  by the second key is refused. The file is then installed 644 as the updater's entire
  trust root. Remediation: anchor the pattern (allow an optional trailing comment only),
  and copy the whole file, validating every non-blank line.
- **F9 [low / confidence: medium]** `deploy/styrman-ctl:163` (validate) — secrets are
  accepted with `$`, `{`, `}` and backslashes in them, and are then written into a file
  that `docker compose --env-file` interpolates: `${…}` in a value expands against the
  other keys (this is exactly how `SUPERADMINS=${STYRMAN_ALLOWED_EMAILS}` works, line 134)
  and a bare `$` is mangled. A Google client secret containing `$` is therefore silently
  corrupted at deploy time (a confusing auth outage), and a value containing
  `${STYRMAN_JWT_KEY}` would copy a secret into a variable that is logged or exposed
  elsewhere. Remediation: reject `$` in the four secret/opaque keys, or document that the
  value must be single-quoted in `.env`.
- **F10 [info]** `deploy/styrman-ctl:328` — on a failed bring-up, `docker logs --tail 30
  styrman-server` is dumped to stderr. Fine on a terminal; if `install` is ever run from
  a unit or a CI wrapper, those lines land in the journal. No secret is known to be
  logged by the server today (slog JSON), so this is a note, not a finding.

## Checked

- **Secrets** — the GitHub token never reaches argv: it is read into a shell variable,
  written under a `mktemp -d` 700 with `umask 077`, and moved with `install -m 600 -o root
  -g root`; every `run …` line that the `--dry-run` plan prints carries paths only
  (confirmed by reading each `run` call in `setup_auto_update`, and by the plan assertions
  in `internal/config/styrman_ctl_test.go`). The askpass helper reads
  `/etc/styrman/gh-token` at call time rather than embedding the token. `read -rs` keeps
  the interactive path off the terminal and out of history. No credential is committed in
  the diff. Residual: F7 (temp-copy lifetime), F6 (source-file hygiene).
- **Injection** — F1 (`awk -v` escape expansion defeating the newline guard) is the real
  one. `env_set`'s key is regex-anchored; `env_get` interpolates its KEY into a `sed`
  expression un-validated but is only ever called with literals from `MUST_FILL`/
  `EDIT_KEYS` (and the `__env-get` hook). `resolve_version` enforces
  `^[0-9]+\.[0-9]+\.[0-9]+$` on the `git describe` output before it becomes a docker tag —
  a tag named `--foo` or `; rm -rf /` cannot survive that, and the same grammar guards
  `cmd_edit`'s `deployed_version`. The compose invocation is `--env-file` with an absolute
  path and `env STYRMAN_VERSION=…` rather than a shell string, so no compose flag can be
  smuggled through a `.env` LINE; what a hostile `.env` line CAN do is set any interpolated
  variable (`STYRMAN_IMAGE`, `BASE_DOMAIN`, `STYRMAN_INGRESS_TOKEN`, …), which is precisely
  why F1 matters. `case` patterns that embed `${DEPLOY_DIR}` are quoted, so no glob.
  `is_email_list` iterates `$1` unquoted (word-split on `IFS=,`) without `set -f`, so a
  value containing `*` is glob-expanded against the CWD — a curiosity, not a finding.
- **Authz/authn** — nothing in the diff implements authentication. `need_root` gates every
  verb except `--dry-run` (by design, for the Go test) and `cmd_internal` (F5). Indirect
  authz impact is via `.env`: `STYRMAN_ALLOWED_EMAILS`/`SUPERADMINS`/`STYRMAN_INGRESS_TOKEN`
  all live there, which is what makes F1 more than a config-hygiene bug. `--yes` takes
  defaults but never invents a secret: `ask` dies when a needed default is empty, and
  `cmd_install:418` refuses to set up auto-update unattended unless both files were passed.
  `--yes` DOES silently overwrite the clone's `.env` (`:476`) — documented behaviour,
  acceptable.
- **Crypto** — none hand-rolled here. Tag trust is `git verify-tag` with
  `gpg.ssh.allowedSignersFile`, owned by `deploy/update/styrman-update`; this diff only
  installs the allowed_signers file it reads (F8 on the validation of that line).
- **Dependencies** — no new dependency; the script shells out to tools host-setup.sh
  already requires (`git`, `docker`, `systemctl`, `install`, `awk`, `sed`, `python3` for
  `json_str`). `python3`, `curl`, `git` are asserted present at `:389`. govulncheck/npm
  audit are not applicable to a bash change and were not run for this diff; the Go file in
  the commit is a test that only shells out to the script.
- **GCP surface** — none. styrman is self-hosted; `.agents/docs/deploy/gcp.md` records
  "not applicable". No IAM, no service account, no Secret Manager in scope.
- **`chown -R root:root ${DEPLOY_DIR}` (`:473`)** — CLEAR, measured rather than reasoned:
  GNU `chown -R` defaults to `-P` (no traversal of symlinks) and on this host
  (coreutils, Debian 13) a symlink inside the tree pointing at a file outside it left the
  target's ownership untouched while the link itself was re-owned. A hostile repo content
  therefore cannot use the clone's `chown -R` to re-own host files. Note separately that
  the clone checks out the DEFAULT BRANCH unverified — that is by design, the signature
  gate is in the updater before any deploy, and the tree is root-owned so
  `styrman-update`'s `trusted_dir` bar still holds.
- **`wait_healthy` (`:319-330`)** — bounded loop, returns 1, caller `die`s; no secret in
  the failure path beyond the container log dump (info item).
- **BUG-035 (a mechanism must reach every caller)** — `cmd_install` delegates the firewall
  to `deploy/host-setup.sh`, which runs `deploy/firewall/install.sh` at `:224` and warns
  on failure, so the new front door does not bypass it. `setup_auto_update` runs
  `systemctl enable --now styrman-update.timer` unconditionally (not inside a "changed"
  guard), which is the shape BUG-035's rule asks for.
- **BUG-009 / ADR-052 (root code under a lower-privileged owner's tree)** — the paths this
  diff writes (`/etc/styrman`, `/opt/styrman`) are root's own; `install -d -m 700 -o root
  -g root` is used rather than `mkdir -p`, and no `chown` walks below an operator-owned
  directory. `env_set` refuses a symlinked target. The residual is that `install -d` on a
  pre-existing `/etc/styrman` SYMLINK would chmod/chown its referent — planting that needs
  root already, so it is noted, not filed.

## Not examined

- `cmd_edit` and the `--redeploy` path beyond noting that the uncommitted `cmd_edit`
  reaches `env_set` from argv (F1): they are NOT in the reviewed commit and the working
  tree is still moving.
- `deploy/host-setup.sh` as a whole (only §8c, the prose this replaces, and the firewall
  call at `:224`).
- `deploy/update/styrman-update` as a whole (only its credential handling, `trusted_dir`,
  and the tag-verification gate, as the comparison this review needed).
- `deploy/firewall/install.sh`, `deploy/docker-compose.yml` beyond its interpolated
  variable list, and the web/e2e and integration suites.
- No test was executed: this review read code and ran two contained probes in the
  scratchpad (awk `-v` escape expansion; `chown -R` symlink semantics). Neither touched the
  repository.

## Verdict line

FINDINGS — 1 medium-impact injection (F1, the `.env` newline guard is bypassable through `awk -v`), 1 medium credential-scope regression against the mechanism it replaces (F2, an unscoped `GIT_ASKPASS` where host-setup.sh 8c and the updater both pin `credential.https://github.com.helper`), and 7 low findings plus 1 informational note (F3-F10). Nothing here is a working exploit and none was built.
