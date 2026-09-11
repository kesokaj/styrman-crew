---
owner: security
date: 2026-09-11
---
# Security review — styrman-ctl, phase 3 (edit + update)   (2026-09-11)

Range reviewed: `edc4858` (`git show edc4858 --`), the styrman-ctl Phase 3 commit.

Blobs pinned (the working tree has ALREADY moved on to Phase 4 — `deploy/styrman-ctl`
is 807 lines on disk vs 712 in the commit — so every line number below is the
COMMITTED blob, not the file as it sits):

- `deploy/styrman-ctl` — blob `bf4b1f6`, sha256
  `5f25cdc5a958d8a30c06040d234f685128ed2152fb84eaa5300a8554234968b6`, 712 lines
- `internal/config/styrman_ctl_test.go` — +142 lines (harness only, no findings)
- `deploy/README.md` — +7/-4 lines, no findings

Blast radius followed outward from the diff: `deploy/.env.example`,
`deploy/docker-compose.yml`, `deploy/caddy/ingress/{config,cmd,generator,labels}.go`,
`internal/config/config.go`, `internal/userstore/userstore.go`,
`deploy/update/styrman-update`, `deploy/firewall/install.sh`.

## Verdict: FINDINGS

Phase 1's HIGH is genuinely fixed — the value now reaches awk through `ENVIRON`
and a backslash is refused outright, so no escape sequence can smuggle a second
assignment. What is left is smaller but real, and two of it are Phase 1 findings
that were marked fixed while the exact scenario they named still runs: the
control-byte strip landed on the file/label sources but NOT on the `.env` values,
so `BASE_DOMAIN` and the whole new `edit` table still drive the root operator's
terminal (F1); and `validate` still accepts `$`/`"`/`'` in the three secret
values, which `edit` now writes to both `.env` copies and applies through
compose interpolation in the same command (F2). The worst consequence of F2 is
that `edit CLOUDFLARE_API_TOKEN` set to a `${…}` reference ships another
variable's value — the JWT session-signing key among them — to Cloudflare's API
through the internet-facing Caddy. Beyond that, `edit STYRMAN_ALLOWED_EMAILS`
grants three separate privileges and the table names one (F3), and the
env-supplied path overrides now decide which binary root executes (F4).

## Phase 1 findings — resolution

| Phase 1 finding | Status |
|---|---|
| **[high]** `env_set` `awk -v` escape processing → smuggled assignment | **RESOLVED.** `:128` passes the value as `V="${value}" awk … ENVIRON["V"]` (no escape processing) and `:117` refuses any backslash outright. Test at `styrman_ctl_test.go:251` plants the exact payload. |
| **[medium]** `env_get` key interpolated into a `sed` program | **RESOLVED.** `:100` applies `is_key`; `:105` matches the key as an awk variable (`index($0, k "=") == 1`). A slash or an `s///e` program can no longer reach a program text. |
| **[medium]** `__` hooks = unconfined root file mutator | **PARTIALLY RESOLVED.** The gate shipped (`:644`, verified: hooks refuse without `STYRMAN_CTL_TEST=1`) and the `--dry-run` side effect is gone (`:121` returns before `mktemp`/`chmod`). The CONFINEMENT to `${ENV_FILE}`/`${CLONE_ENV}` — the other half of the remediation — did **not** ship. See F5. |
| **[medium]** `ask`/`confirm_word` precedence + `[ -r /dev/tty ]` | **RESOLVED.** `has_tty()` at `:60` probes by OPENING (`( : < /dev/tty )`), and `:65` is `[ "${YES}" = 1 ] \|\| ! has_tty` — one condition, correct precedence. |
| **[medium]** `guard_path` not load-bearing enough | **PARTIALLY RESOLVED.** `:216` now requires canonical-as-given (kills the symlinked-parent and `..` cases) and `:217` requires an existing directory (kills `realpath -m`'s nonexistent-path acceptance). Still open: the allowlist is ambient env (F4), and TOCTOU is untested because no delete exists yet in this range. See F10. |
| **[low]** `json_str` with no `python3` → invalid JSON | **RESOLVED.** `:248-255` falls back to `printable` + backslash/quote escaping. Control bytes are stripped and `"`/`\` escaped, so the fallback emits valid JSON. |
| **[low]** `cmd_status` human path echoes raw external values | **PARTIALLY RESOLVED — the finding's own example still runs.** `printable()` (`:247`) was applied to `host`, `health`, `server_img`, `caddy_img`, `dver`, `dsha`, `fver`, `clone_sha`, `clone_tag` — but NOT to `base`/`hn`/`url` (`:277-278`), which is exactly the `BASE_DOMAIN` case Phase 1 verified, nor to `disk`, `dockerv`, `katav`, `operator`. See F1. |
| **[low]** ADR-052: `ENV_FILE` had no scratch override | **RESOLVED.** `:37` `ENV_FILE="${STYRMAN_ENV_FILE:-${REPO}/deploy/.env}"`. |
| **[low]** `validate` admits values the stack cannot use | **PARTIALLY RESOLVED.** `CADDY_ADMIN` is now range-checked (`is_octet`/`is_port`, `:150-164`) — the one clearly fixed item. `BASE_DOMAIN` still accepts `localhost` and an IP-shaped label (no ACME cert). The `$`/quote acceptance in the three secret keys (`:174`) is UNCHANGED and is now live through `edit` — F2. `DEFAULT_RPS` still accepts `0` — F11. |
| **[low]** `is_email_list` validates normalized, `env_set` stores original | **OPEN.** `:148` unchanged; an internal space still validates and is stored verbatim. F7. |
| **[low]** `git -C "${DEPLOY_DIR}"` with no ownership assertion | **OPEN.** `:287-288` unchanged; still relies implicitly on git's dubious-ownership guard and still reports a wrong-owner clone as "not set up". |
| **[low]** global flags extracted from any argv position, no `--` | **OPEN but largely moot for `edit`.** `main:691-698` unchanged. A value can only reach `edit` inside a `KEY=VALUE` token (`:574`), and `--dry-run`/`--yes`/`--confirm` are exact matches, so `X=--yes` survives; a trailing bare `--confirm` dies on `set -u` (fails closed). Downgraded, not closed. |

## Findings

- **[medium / confidence: high]** `deploy/styrman-ctl:277-278` and `:329`, and
  `deploy/styrman-ctl:596` (`cmd_edit`'s table) — **`.env` values still reach the
  root operator's terminal unstripped, including in the new `edit` table.**
  `printable()` was added for "what a file, a git tag or a container label
  contributed to the screen" but `env_get`'s output is neither: `base` and `hn`
  at `:277-278` are raw, `url` is built from them, and `:329` echoes `${url}`.
  The new `masked()` (`:567`) passes `${cur}` through untouched, so `:596`'s
  `printf` renders every one of the 19 `EDIT_KEYS` raw. Scenario, on the trust
  boundary the Phase 1 report already named ("the writer of the checkout `.env`
  is whoever can write the checkout, which on this host includes agents running
  as the operator"): a value carrying cursor-up and erase-line sequences — no
  newline and no backslash needed, so `env_set`'s own refusals are irrelevant
  and `vi .env` is the documented editing path anyway — repaints the line above
  it. The `edit` table is the screen an operator reads to decide what to change,
  and `SUPERADMINS` is on it. Remediation: wrap every `env_get` result on the
  way to a screen in `printable` — `masked` and the `base`/`hn` assignment — not
  only the file, tag and label sources. (`--json` remains safe: `json_str`
  escapes on both branches.)

- **[medium / confidence: high]** `deploy/styrman-ctl:174` reached from
  `cmd_edit:617` and applied at `:635` — **`validate` accepts `$`, `{`, `}`,
  `"` and `'` in `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET`/`CLOUDFLARE_API_TOKEN`
  (`^[^[:space:]]+$` is the whole rule), and `edit` now writes such a value to
  BOTH `.env` copies and recreates the stack in the same command.** Verified
  against the pinned blob: `__validate GOOGLE_CLIENT_SECRET` with the literal
  `${STYRMAN_JWT_KEY}` returns 0, and so does an unbalanced leading double
  quote. That compose interpolates dotenv values is not a guess — it is what
  makes `deploy/.env.example:22` (`SUPERADMINS=${STYRMAN_ALLOWED_EMAILS}`) work,
  and `validate:177` blesses that literal explicitly. Two concrete consequences:
  1. `deploy/docker-compose.yml:37` forwards `CLOUDFLARE_API_TOKEN` into the
     internet-facing Caddy and `deploy/caddy/ingress/generator.go:202` emits
     `acme_dns cloudflare {env.CLOUDFLARE_API_TOKEN}` — so a value that is a
     `${…}` reference ships whatever that variable holds (`STYRMAN_JWT_KEY`, the
     HMAC key that signs every browser session, is in the same file) to
     Cloudflare's API as a bearer token. The old `edit`-less flow put a human
     `docker compose up` between the typo and the effect; `:635` removes it.
  2. A leading `"` hands compose's dotenv parser an unterminated quoted value,
     which swallows the lines below it — in `.env.example` order that is
     `STYRMAN_INGRESS_TOKEN` and `STYRMAN_JWT_KEY`. That half fails CLOSED
     (`internal/config/config.go:226-227` refuses to start in header mode with
     an empty ingress token), but only after `edit` has written both copies.
  Remediation: reject `$ " ' \` and a leading `-` in the three secret values —
  the same defence `deploy/caddy/ingress/labels.go:132`'s `emailRe` already
  applies to the email path, and which its comment argues for in the same words.

- **[medium / confidence: medium]** `deploy/styrman-ctl:566` + `:567` + `:177`
  — **`edit STYRMAN_ALLOWED_EMAILS=…` grants three privileges and the table
  names one.** `internal/userstore/userstore.go:107-110`: every address in
  `STYRMAN_ALLOWED_EMAILS` is a styrman ADMIN, and per `AGENTS.md` an admin
  toggles a project into LAN mode — which by the same document makes that
  project "exactly a normal Docker container", reaching the host itself
  "including its sshd". `deploy/.env.example:22` ships
  `SUPERADMINS=${STYRMAN_ALLOWED_EMAILS}`, so the same list is also the Caddy
  platform-superadmin roster on ALL apps
  (`deploy/caddy/ingress/generator.go:284-287`). In the `edit` table,
  `SUPERADMINS` is not in `SECRET_KEYS`, so `masked` prints it verbatim: the
  operator sees the literal `${STYRMAN_ALLOWED_EMAILS}`, never the effective
  roster, and adding one address to the allowlist silently adds a platform
  superadmin with LAN-toggle rights. Scenario: an operator asked to "give the
  new contractor read access" runs `styrman-ctl edit
  STYRMAN_ALLOWED_EMAILS=me@x,contractor@y`; the tool validates, writes both
  copies, redeploys, prints `OK`, and never says the word admin. Remediation:
  when `SUPERADMINS` holds the reference literal, render the RESOLVED list in
  the table, and have `edit STYRMAN_ALLOWED_EMAILS` state the privileges it
  grants (admin, LAN toggle, platform superadmin) before writing.

- **[medium / confidence: medium]** `deploy/styrman-ctl:23-40`, `:460`, `:554`,
  `:633-635` — **the path overrides are ambient environment, and Phase 3 widened
  what they control from "which tree gets deleted" to "which binary root
  executes".** `BIN_DIR` comes from `STYRMAN_BIN_DIR` (`:28`) and is the path
  `cmd_update` runs as root at `:554`
  (`run env "${UPDATER_ENV[@]}" "${BIN_DIR}/styrman-update" --force`) and checks
  at `:466`; `DEPLOY_DIR`/`REPO` choose the compose file and `--env-file` at
  `:635`; `ETC_DIR` is where `setup_auto_update:502` installs the GitHub token
  and where `deployed_version:240` reads. The Phase 3 verification artifact
  records this as "Accepted: the env-supplied allowlist (root's own
  configuration)". That holds only under sudo's default `env_reset`, which
  nothing in the script asserts, and this repo's own history says the assumption
  has failed before — `.agents/docs/reviews/2026-09-02-security-v1.17.3.md` R1-F3
  is the same `sudo -E` target-injection finding against the updater. Scenario:
  an operator who has learned `sudo -E deploy/styrman-ctl …` from testing (the Go
  harness drives the script entirely through these variables) carries a stale
  `STYRMAN_BIN_DIR` from their shell into a real `update --redeploy`, and root
  executes `${that}/styrman-update`. Remediation: the in-repo precedent is
  `deploy/firewall/install.sh:41`, which takes `--state-dir` and friends as
  explicit FLAGS (ADR-052). Either adopt that, or refuse every `STYRMAN_*_DIR`
  override unless `STYRMAN_CTL_TEST=1` — one gate would then cover this class
  and F5 together.

- **[low / confidence: high]** `deploy/styrman-ctl:643-652` (`cmd_internal`) —
  **the gate shipped, the confinement did not.** Phase 1's remediation was two
  clauses: gate `cmd_internal` on `STYRMAN_CTL_TEST=1` AND confine `__env-set`
  to `${ENV_FILE}`/`${CLONE_ENV}`. Only the first is in the blob. With the gate
  satisfied, `__env-set` is still any-file: no `guard_path`, no `need_root`, no
  confinement — it rewrites the target in place, appends `KEY=VALUE` and forces
  mode `0600` on whatever regular non-symlink path argv names. On the leak
  question the dispatch asks: under sudo's DEFAULT configuration
  `STYRMAN_CTL_TEST` is dropped by `env_reset` (it is in no default `env_keep`),
  and the gate holds — verified, hooks refuse without it. It does NOT hold under
  `sudo -E`, `su` without `-`, a sudoers `env_keep`, or a root shell that
  exported it, which is precisely the same lever as F4, acquired for the same
  reason. Because one env var is now the only thing between argv and an
  arbitrary root `chmod 600`, the second clause is worth its two lines:
  `case "$2" in "${ENV_FILE}"|"${CLONE_ENV}") ;; *) die … ;; esac`.

- **[low / confidence: high]** `deploy/styrman-ctl:613-624` (`cmd_edit`) —
  **"Validate everything before writing anything" is false for a backslash, and
  the failure leaves both `.env` copies half-written with no re-apply.**
  `validate` and `env_set` disagree on the legal value set: verified against the
  pinned blob, `__validate CLOUDFLARE_API_TOKEN` with a value containing a
  backslash returns 0, while `__env-set` with the same value dies "refusing a
  value with a backslash". So in `styrman-ctl edit A=good B=<backslash value>`
  the pre-flight loop at `:615-618` passes both, the write loop at `:620-624`
  writes A into BOTH targets, then dies on B — and because it dies, the re-apply
  at `:635` never runs. The operator sees an error and reasonably concludes
  nothing happened, while the two `.env` copies now disagree with the running
  stack; `status`'s `env_copies_differ` cannot flag it either, since both copies
  received A. Remediation: move the newline/backslash refusal into `validate` so
  the pre-flight loop actually pre-flights, and keep it in `env_set` as the
  second line.

- **[low / confidence: medium]** `deploy/styrman-ctl:147-148` vs
  `deploy/caddy/ingress/labels.go:132` — **two email grammars, and `edit`
  enforces the looser one on the roster that grants admin.** `is_email` bans
  only whitespace, `@` and `,`; `emailRe` additionally bans `{ } " ' #`, with a
  comment explaining that those are "the bytes that would end a `transform user`
  block early". Verified: `__validate SUPERADMINS 'a{b}@c.co'` returns 0.
  Injection fails CLOSED — `parseEmailList` silently DROPS a non-matching
  address rather than emitting it — so the residual is that `styrman-ctl edit`
  reports success for an entry that can never sign in, on the list that decides
  who is an admin. The Phase 1 normalization nit rides along: `:148` strips all
  internal spaces before validating while `env_set` stores the original, and
  `internal/config/config.go:185`'s per-element `TrimSpace` does not repair an
  INTERNAL space. Remediation: adopt `emailRe`'s character class verbatim and
  write the normalized string.

- **[low / confidence: medium]** `deploy/styrman-ctl:633-635` (`cmd_edit`'s
  re-apply) — **root `docker compose` against a compose file in the operator's
  checkout whenever the clone has no `.env`.** `if [ -f "${CLONE_ENV}" ]; then
  dir="${DEPLOY_DIR}"; else dir="${REPO}"; fi` — the else branch is every host
  before `update --setup` and every host that took `--skip-auto-update`. A
  compose file is code: `volumes: ["/:/host"]`, `privileged: true`, a `build:`
  context that runs a Dockerfile. The checkout is writable by the operator
  account, which on this host is also what coding agents run as. `cmd_install`
  has the identical property but announces it (`:435`, "from this checkout — the
  documented first bring-up") and the operator is consciously installing; `edit`
  reads as "change one setting". Remediation: prefer the clone whenever
  `${DEPLOY_DIR}/.git` exists, and print the compose file path in the `:634` log
  line so the directory root is about to execute is on screen before it runs.

- **[low / confidence: medium]** `deploy/styrman-ctl:238-243` (`deployed_version`)
  — **the trusted half is read without the assertion its own sibling makes, and
  the untrusted half is bounded only by a regex.** The file
  `${ETC_DIR}/update/deployed-version` is root-owned (created under
  `install -d -m 700 -o root -g root`, `:501`) and is the right source, but it is
  `cat`'d with no symlink refusal and no owner check, where
  `deploy/update/styrman-update:169-170` makes exactly that assertion before
  trusting its clone. The fallback takes `{{.Config.Image}}`'s tag off the
  running `styrman-server` and hands it to `STYRMAN_VERSION=` at `:635`. What
  saves that path is `:632`'s `^[0-9]+\.[0-9]+\.[0-9]+$`, which bounds the worst
  case to a DOWNGRADE to some older released tag chosen by whoever could
  re-create the container — and that requires the docker socket, which on this
  host is root-equivalent already, so the trust delta today is nil. Recorded
  because the regex is the entire guard and Phase 4 adds more readers of the
  same file. Remediation: assert regular-file + not-a-symlink + `stat -c %u` = 0
  before `cat`, and log which of the two sources the version came from, so
  "re-applying at 1.27.1" is attributable.

- **[low / confidence: medium]** `deploy/styrman-ctl:208-224` (`guard_path`) —
  **hardened as asked, still ambient, still unexercised by a delete.** The two
  concrete Phase 1 weaknesses are closed: `:216` refuses anything whose
  `realpath` differs from the literal path (so a symlinked PARENT, a symlinked
  leaf and a `..` are all out, satisfying the "least-trusted parent" rule in
  `.agents/rules/guest-writable-paths.md`), and `:217` requires an existing
  directory (so `realpath -m`'s tolerance of a typo near the tree is gone).
  Residuals: the three allowed trees are still whatever `STYRMAN_STATE_DIR` /
  `_DEPLOY_DIR` / `_ETC_DIR` say (F4), the comparison is string equality against
  an un-canonicalized `${allowed}` (fails closed on a trailing slash, which is
  the right direction), and nothing pins device+inode between the guard and a
  later `rm -rf`. No delete exists in this range — Phase 4's `uninstall` is in
  the working tree and out of scope. Re-review this function against the first
  `rm -rf` that lands behind it.

- **[low / confidence: medium]** `deploy/styrman-ctl:181` (`DEFAULT_RPS`) —
  `^[0-9]+$` with no range check accepts `0`, which
  `deploy/caddy/ingress/cmd.go:159-165` parses successfully (only a PARSE failure
  falls back to the default 100) and `generator.go:129-132` emits verbatim as
  `events 0` inside the internet-facing ingress's `rate_limit` block. Whether
  that means "block everything" or "no limit" is decided by the rate-limit
  module, not by anything styrman-ctl or the generator checks — either reading
  is a bad outcome an operator reached by typing a plausible number.
  Remediation: require `>= 1` (and an upper bound), the same treatment
  `CADDY_ADMIN` just received.

## Checked

- **Secrets** — no credential, token or key is added by the diff; `deploy/.env`
  is not committed and does not exist in this checkout. Masking:
  `SECRET_KEYS` = `GOOGLE_CLIENT_SECRET CLOUDFLARE_API_TOKEN
  STYRMAN_INGRESS_TOKEN STYRMAN_JWT_KEY`, and `masked:567` covers all four in
  the `edit` table — including the two COMPUTED keys, which the table lists
  read-only. `GOOGLE_CLIENT_ID` is deliberately unmasked (it is public);
  `ADMIN_EMAIL`/`STYRMAN_ALLOWED_EMAILS`/`SUPERADMINS` are unmasked, which is
  PII on the root operator's own terminal and correct for a roster editor. The
  prompt path is sound: `ask:71-73` shows `********` rather than the value and
  reads with `read -rs`, and an empty answer restores the REAL default
  (`:77`), not the asterisks. `setup_auto_update:483-498` is a genuine
  improvement — under `--dry-run` the token and signers never touch the disk at
  all, and in a real run they travel through a `700` temp dir and
  `GIT_ASKPASS`, never argv, never a git config. No secret is echoed by
  `log`/`die` on any path I read.
- **Injection** — Phase 1's two live cases are closed (see the table above);
  re-verified by reading the pinned blob, not by trusting the changelog. `edit`'s
  `KEY=VALUE` split (`:616`) is `${kv%%=*}` / `${kv#*=}`, so a value containing
  `=` is preserved and the key still passes `is_key` inside `env_set`. The awk
  key match is anchored and includes the `=`, so `A` cannot clobber `AB`, and a
  commented `# KEY=` line is correctly not treated as an assignment. `grep -q
  "^${key}="` is safe because `is_key` bans regex metacharacters. No `eval`, no
  unquoted expansion into a command. The two parsing boundaries that are NOT
  clean are compose's dotenv (F2) and the ingress's Caddyfile via the email
  grammar (F7, fails closed). `shellcheck -s bash` on the pinned blob: clean, 0
  findings.
- **Authz/authn** — no auth code in the diff; the authz surface is the `.env`
  file, and it is the reason F2 and F3 are rated where they are. `need_root` is
  called by `cmd_status`, `cmd_install`, `cmd_update` and `cmd_edit`;
  `cmd_internal` deliberately does not (it is a test hook) — see F5. `validate`
  correctly REFUSES operator edits to all six `COMPUTED_KEYS`, so
  `edit STYRMAN_INGRESS_TOKEN=…` and `edit STYRMAN_JWT_KEY=…` are rejected at
  `:168` and the menu refuses them again at `:602`. `STYRMAN_IMAGE` accepts only
  the empty value, preserving the no-mutable-tags rule.
- **`cmd_update` specifically** — `systemctl start styrman-update.service` takes
  no operator input at all; `:550` refuses to run unless the timer is already
  enabled. On `--redeploy`: `--force` in `deploy/update/styrman-update` clears
  only the "nothing new" and "previously failed" gates — line 344
  (`verify_tag "${LATEST}" || fail`) runs unconditionally, and `verify_tag`
  requires an SSH signature from `allowed_signers` and explicitly refuses a PGP
  signature. So `update --redeploy` does NOT deploy unverified code. Phase 2's
  fix is present and correct: `UPDATER_ENV:460` names all four updater inputs so
  the updater's own `/etc/styrman` defaults cannot silently apply when `ETC_DIR`
  is elsewhere. The residual is F4 — `${BIN_DIR}` itself.
- **`STYRMAN_BROKER_ADDR`** — `validate:180` accepts `0.0.0.0:8091`, which looks
  alarming against `.env.example:62` ("never publish this to the host") but is
  not an exposure: the server has no `ports:` mapping and sits on
  `networks: [styrman-ingress]`, so `:8091` (the default) already binds all
  interfaces INSIDE the container's netns and `0.0.0.0` says the same thing.
  Broker caller identity is bridge position, unchanged by this diff. Not a
  finding.
- **Crypto** — nothing hand-rolled anywhere in the diff: no auth, session,
  hashing, JWT or OIDC code. `STYRMAN_JWT_KEY` and `STYRMAN_INGRESS_TOKEN` are
  generated by `host-setup.sh` and are refused by `validate` as computed keys.
  The only crypto-adjacent path is `is_signers_line:461`, which shape-checks an
  `allowed_signers` line before it is installed; the real verification is git's,
  in the updater, and is unchanged.
- **Dependencies** — no Go module change, no npm change, no new runtime
  dependency; `python3` is now optional (`json_str` has a fallback) and is still
  hard-required by `cmd_install:415` alongside `curl` and `git`, which matches
  `deploy/host-setup.sh:32`. `govulncheck` NOT run: it is not on PATH here and
  the project's `deploy/vulncheck.sh` runs it inside a pinned builder image via
  `docker build`, which this diff does not warrant since it changes no
  dependency — stated rather than faked. `npm audit` not applicable (no `web/`
  change). `shellcheck` WAS run on the pinned blob: clean.
- **GCP surface** — none. Self-hosted deployment;
  `.agents/docs/deploy/gcp.md` is "not applicable". No IAM binding, no service
  account, no `--allow-unauthenticated`, no key file, in the diff or its blast
  radius.
- **Bounded root work** — `cmd_edit` and `cmd_update` add no `mkdir -p`, no
  `chown` and no recursive operation below a lower-privileged owner's directory,
  so no BUG-009-class issue. `env_set`'s atomicity is intact (`mktemp` in the
  same directory, `chmod 600` BEFORE the rename, `mv -f`); its `-L`/`-f` checks
  are TOCTOU-able in the operator-writable checkout, but the outcome is bounded
  — `mv` replaces a swapped symlink by `rename(2)` rather than writing through
  it, and `chown --reference` dereferences to the victim file's owner, so the
  content copied into `deploy/.env` stays unreadable to the swapper. Noted, not
  filed.
- **Set -e correctness** — the `[ -f X ] && arr+=(…)` pattern at `:580-581` and
  the `[ -n "${base}" ] && url=…` at `:278` do NOT abort under
  `set -euo pipefail` + `inherit_errexit`; confirmed empirically in a scratch
  script. `cmd_edit` therefore works when only one `.env` copy exists.

## Not examined

- **Phase 4 (`uninstall`) and the rest of the working tree.** `deploy/styrman-ctl`
  is 807 lines on disk against the 712 reviewed here, and
  `.agents/docs/changes/2026-09-11-styrman-ctl-phase4-uninstall-and-docs.md` is
  untracked — none of it is in range. `guard_path` (F10) must be re-reviewed
  against the first `rm -rf` placed behind it; that is the single most important
  follow-up.
- **`internal/config/styrman_ctl_test.go`'s +142 lines** were read only for what
  they assert about the fixes (the backslash payload at `:251`, the gate at
  `:265`, `STYRMAN_CTL_TEST=1` at `:79`); the harness itself was not reviewed as
  code, and no test was executed — the working tree has moved past the reviewed
  commit, so a run would not describe this range.
- **`deploy/host-setup.sh`, `deploy/firewall/install.sh`,
  `deploy/update/styrman-update`** beyond the specific lines cited — unchanged
  in this range; the updater was read only for `--force`'s signature behaviour
  and its input defaults.
- **Compose's dotenv parser was reasoned about, not executed.** F2's
  interpolation claim rests on `.env.example:22` being a shipped, working
  construct; the unterminated-quote sub-case (F2 part 2) is the weaker half and
  is why F2 leads with the interpolation consequence rather than the parser one.
- **Plans, handoffs and the conversation** — not read, per the dispatch. The
  Phase 3 verification artifact was read only for its claimed resolution of the
  Phase 1 findings, which this report checks against the blob rather than
  accepting.
