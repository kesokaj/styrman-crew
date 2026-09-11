---
owner: security
date: 2026-09-11
---
# Security review — styrman-ctl, phase 1 (2026-09-11)

Range reviewed: `092c7ef` (`git show HEAD --`), the styrman-ctl Phase 1 commit.
Blobs pinned (the working tree has since moved on to Phase 2, so line numbers
below are the COMMITTED blobs, not the file on disk):

- `deploy/styrman-ctl` — blob `f2693c4`, sha256 `e92386b84e5dfa8deab5643a09bb1edae0d12d88c8888a83a27b8f0f955b4f49`, 359 lines
- `internal/config/styrman_ctl_test.go` — blob `8d70fb3`
- `deploy/README.md` — 7 added lines, no findings

Blast radius: a root-run entry point shipped into the deploy clone that (from
Phase 2) writes `deploy/.env` — the file holding `SUPERADMINS`,
`STYRMAN_ALLOWED_EMAILS`, `STYRMAN_INGRESS_TOKEN`, `STYRMAN_JWT_KEY`,
`GOOGLE_CLIENT_SECRET`, `CLOUDFLARE_API_TOKEN` — and (from Phase 4) deletes
trees. Phase 1 ships `status` plus all the helpers those phases will call, so
the helpers are reviewed as if already live.

## Verdict: FINDINGS

`env_set`'s stated guarantee — "refuses … a value with a newline (it would
smuggle a second key)" — does not hold. The replace branch passes the value
through `awk -v`, which processes escape sequences, so the two characters `\n`
in a value become a real newline and a second `KEY=VALUE` line lands in the
file the platform reads its admin roster and its ingress shared secret from.
The newline guard on the line above is the control written for exactly this
case, and it is bypassed by a form no operator would read as suspicious.
Everything else is smaller: an unvalidated key interpolated into a `sed`
program, `__env-set` accepting any file on the host, `--yes` not actually
skipping prompts, and a path guard that is not yet strong enough for the
deletes Phase 4 will put behind it.

## Findings

- **[high / confidence: high]** `deploy/styrman-ctl:105` (`env_set`, replace branch) —
  `awk -v v="${value}"` performs escape-sequence processing on the assignment,
  so the newline refusal at `:99` (which only matches a real `$'\n'`) is
  bypassed by a literal backslash-n. Verified on a scratch file: setting
  `B` to `x\nSUPERADMINS=attacker@example.test` produced two lines —
  `B=x` and a fresh `SUPERADMINS=` assignment. Since compose's dotenv takes
  the LAST assignment, a smuggled line silently rewrites the superadmin
  roster, the allowed-email list, or `STYRMAN_INGRESS_TOKEN`. The same call
  also mangles `\t`, `\\`, `\0…` in any legitimate secret. Reachability today
  is operator argv only (root already), but Phase 3's `edit` is specified to
  feed operator- and file-supplied values straight into this function, which
  is where a mistyped or pasted value becomes an authz change. Note the
  asymmetry that hides it: the append branch (`:108`, `printf '%s=%s\n'`) is
  literal, so the SAME value behaves differently depending on whether the key
  already exists — the test at `styrman_ctl_test.go:248` only covers the real
  newline, on the replace branch, and passes.
  Remediation: keep the value out of `awk -v` — `v="${value}" awk '… ENVIRON["v"] …'`
  (ENVIRON does no escape processing), or write the value with `printf` in both
  branches; and extend the refusal from `$'\n'` to any backslash.

- **[medium / confidence: high]** `deploy/styrman-ctl:90` (`env_get`) — the key is
  interpolated unquoted into a `sed` substitution program with NO validation,
  while `env_set:100` validates its key with `^[A-Z][A-Z0-9_]*$`. A key
  containing `/` changes the program (verified: `sed: -e expression #1, char 8:
  unknown option to 's'`), and GNU sed's `s///e` flag executes the pattern
  space as a shell command — so any key that is not a trusted literal is
  command execution under the root the script already runs as. Today the
  callers are two literals plus the `__env-get` hook, so this is a latent
  parsing-boundary defect, not a live escalation; it becomes live the moment a
  key comes from a file, a menu, or a loop over `.env.example`.
  Remediation: apply the same one-line key regex in `env_get`, or replace the
  `sed` with `awk -v k=… 'index($0,k"=")==1{sub(/^[^=]*=/,"");v=$0} END{print v}'`.

- **[medium / confidence: high]** `deploy/styrman-ctl:294-302` (`cmd_internal`) — the
  `__` hooks are documented "not for operators" but are ordinary verbs reachable
  from argv (`main:355` routes any `__*`), with no test-only gate. `__env-set`
  in particular is an unconfined root file mutator: it accepts ANY existing
  non-symlink path, rewrites it in place, appends `KEY=VALUE`, and forces mode
  `0600` — it never consults `guard_path`, never asks, and (`:110`, `:114`)
  does its `chmod`/temp write even under `--dry-run`. Verified on a scratch
  file with `/etc/passwd`-shaped content: content preserved, `NOTAKEY=1`
  appended, mode changed 644 → 600. Operator-typo scenario: a mistyped path
  such as a unit file under `/etc/systemd/system` silently becomes root-only
  readable and gains a junk line; a file that must stay world-readable breaks
  with no message. Remediation: confine `__env-set` to `${ENV_FILE}` and
  `${CLONE_ENV}`, and require an explicit marker (`[ -n "${STYRMAN_CTL_TEST:-}" ]`)
  before `cmd_internal` runs at all — the Go harness can set it, an operator's
  fingers cannot.

- **[medium / confidence: high]** `deploy/styrman-ctl:55` (`ask`) and `:80`
  (`confirm_word`) — two defects in the same condition:
  1. `[ "${YES}" = 1 ] || [ ! -t 0 ] && [ ! -r /dev/tty ]` parses as
     `([ YES ] || [ ! -t 0 ]) && [ ! -r /dev/tty ]` (bash `&&`/`||` are equal
     precedence, left-associative). With `--yes` and a terminal present the
     branch is FALSE, so the function prompts anyway. Verified. `--yes`
     therefore does not make the Phase 2 installer unattended — it hangs on
     the first prompt, in a root-run installer an operator may have launched
     under `nohup`/systemd expecting it not to.
  2. `[ -r /dev/tty ]` is TRUE even with no controlling terminal — `access(2)`
     on the 0666 device node succeeds; only `open(2)` fails with ENXIO.
     Verified under `setsid`: `[ -r /dev/tty ]` true, `read < /dev/tty` fails.
     So the "no terminal" fallback never fires; instead `read … < /dev/tty`
     fails and `set -e` kills the script. `confirm_word:80`'s guard is dead code
     for the same reason (it fails closed, which is the right direction, with
     the wrong message). Remediation: parenthesize the condition, and probe the
     terminal by opening it — `if [ "${YES}" = 1 ] || ! { exec 3</dev/tty; } 2>/dev/null; then`.

- **[medium / confidence: medium]** `deploy/styrman-ctl:173-185` (`guard_path`) — this
  is the only thing that will stand between Phase 4 and `rm -rf` on a
  production host, and it is not yet load-bearing enough. Verified behaviours:
  - the allowlist is read from ambient env (`STYRMAN_STATE_DIR`/`_DEPLOY_DIR`/`_ETC_DIR`,
    `:23-25`), so under `sudo -E`, a root shell, or a sudoers `env_keep`, the
    guard authorises whatever those name — an arbitrary scratch tree was
    accepted as "one of the configured styrman trees";
  - `realpath -m` accepts a path that does not exist, so the guard cannot tell
    the tree from a typo near it (a nonexistent `.../does/not/exist` was accepted
    when named as `STYRMAN_STATE_DIR`);
  - the symlink refusal (`:178`) tests the LEAF of the given path only; a target
    reached through a symlinked PARENT is accepted. The residual is small here
    (it still resolves into the allowed tree), but it contradicts the project's
    own rule — "a path is only as trusted as its least-trusted parent",
    `.agents/rules/guest-writable-paths.md`;
  - nothing pins the inode between the guard and a later delete (TOCTOU). `rm -rf`
    is FTS_PHYSICAL so it will not follow a link out of the tree, but the tree it
    walks (`${STATE_DIR}/projects/<u>/<uuid>`) is guest-writable.
  Remediation before any delete lands: require the target to exist and be a
  directory; compare `stat -c %d:%i` (device+inode) against the configured tree
  rather than a string; refuse when `realpath` differs from the given path; and
  take the test prefix from an explicit flag (`--state-dir`, the
  `deploy/firewall/install.sh` + ADR-052 pattern) instead of ambient env.

- **[low / confidence: high]** `deploy/styrman-ctl:205` (`json_str`) — no availability
  check for `python3`. With python3 missing or failing, each `$(json_str …)`
  expands to the empty string and `--json` prints syntactically INVALID JSON
  (verified: `{"host":,"installed":true,…}`) while exiting 1, i.e. a monitoring
  caller sees "degraded" plus unparseable output rather than an error.
  `deploy/host-setup.sh:32` already sets the precedent (`command -v python3 ||
  FATAL`). Remediation: `have python3 || die "python3 is required for --json"`.

- **[low / confidence: medium]** `deploy/styrman-ctl:277-289` (`cmd_status`, human
  output) — values sourced from outside the script are echoed raw to the root
  operator's terminal: `.env` values via `env_get` (`:227`), the
  `deployed-version`/`deployed-sha`/`failed-version` files, `git describe`
  output from the clone, and container image names from `docker inspect`.
  Verified: a `BASE_DOMAIN` containing ANSI/OSC bytes rendered as
  `https://crew.ex.test␛[2J␛]0;pwned␇` — screen-clearing and title-setting
  sequences on the operator's terminal, which is how a hostile value hides the
  line below it. The `--json` path is safe (python3 `json.dumps` escapes
  control bytes and non-ASCII). The writer of the checkout `.env` is whoever can
  write the checkout, which on this host includes agents running as the operator.
  Remediation: sanitize externally sourced fields in the human path too
  (strip `[:cntrl:]`), not only in JSON.

- **[low / confidence: high]** `deploy/styrman-ctl:35` + `internal/config/styrman_ctl_test.go:103`
  (ADR-052 scratch-prefix claim) — `ENV_FILE="${REPO}/deploy/.env"` is the ONE
  path in the committed script with no env override, and `cmd_status:226` prefers
  it over the scratch clone copy. On any host set up per `deploy/README.md`
  (which instructs `vi .env` in `deploy/`), `go test ./...` therefore reads the
  operator's REAL secrets file, the URL assertion at test:158 fails, and the real
  `BASE_DOMAIN` is printed into the test log. The harness's leak guard only greps
  the OUTPUT for literal production path strings, so it cannot catch a read of a
  path it never prints. The test comment's "nothing here may touch
  /var/lib/styrman or /etc/styrman" is true; "every path pointed at a scratch
  prefix" is not. Already being fixed in the working tree (`STYRMAN_ENV_FILE`);
  recorded because it is in the reviewed range.

- **[low / confidence: high]** `deploy/styrman-ctl:124-147` (`validate`) — rules that
  admit values the stack cannot use, so the early signal the validator exists to
  give is not given (all verified):
  - `BASE_DOMAIN` accepts `1.2.3.4` and `localhost` — neither can get an ACME
    certificate, so the host comes up with no working TLS;
  - `CADDY_ADMIN` accepts `127.999.999.999:99999` and port `0` (octet and port
    ranges unchecked). The security-relevant property — loopback only, never a
    name, never `0.0.0.0` — IS correctly enforced;
  - `GOOGLE_CLIENT_ID|GOOGLE_CLIENT_SECRET|CLOUDFLARE_API_TOKEN` accept
    `${STYRMAN_JWT_KEY}`, `$(id)` and quoted forms. Compose's dotenv interpolates
    `${VAR}` in unquoted values — that is exactly why the `SUPERADMINS` literal
    at `:123` works — so a secret containing `${…}` expands another variable into
    it: `GOOGLE_CLIENT_SECRET='${STYRMAN_JWT_KEY}'` would ship the JWT signing key
    to Google as the OAuth secret, and a legitimate token containing `$` is
    silently corrupted. Remediation: reject `$`, `"`, `'` and a leading `-` in
    secret values, or require and emit single-quoted form.

- **[low / confidence: medium]** `deploy/styrman-ctl:127` (`is_email_list`) — validation
  normalizes (`e="${e// /}"` strips ALL spaces inside each element) but `env_set`
  writes the ORIGINAL string, so what was validated is not what is stored.
  `internal/config/config.go:185` does `TrimSpace` per element, so the test's own
  `"a@b.co, c@d.org"` survives; an INTERNAL space (`a @b.co`) validates and is
  stored verbatim and then never matches an identity. Fails closed (no extra
  access), but silently drops an admin from the roster. Remediation: validate the
  normalized string and WRITE the normalized string.

- **[low / confidence: medium]** `deploy/styrman-ctl:236-238` — `git -C "${DEPLOY_DIR}"`
  runs as root with no ownership assertion, where the sibling
  `deploy/update/styrman-update:169-170` makes exactly that assertion ("Assert the
  clone is root-owned BEFORE disabling git's ownership guard"). styrman-ctl relies
  implicitly on git's dubious-ownership guard and reports its failure as "no clone
  at … — auto-update not set up", which reads as "not installed" rather than "this
  clone is owned by someone else". If root's gitconfig ever gains
  `safe.directory=*` (the common workaround), root would then read config and
  attributes from a tree it does not own. `/opt/styrman` is `root:root` on the
  live host today. Remediation: match the updater — `stat -c %u` assert, and
  distinguish "wrong owner" from "absent" in the status line.

- **[low / confidence: medium]** `deploy/styrman-ctl:341-349` (`main`) — global flags are
  extracted from ANY argv position, including positions a verb treats as data, and
  there is no `--` end-of-flags terminator. The test relies on this
  (`__env-set f A 9 --dry-run`). For Phase 3's `edit KEY VALUE`, a value that
  begins with `--yes`/`--dry-run`/`--confirm` is silently eaten and changes the
  run's mode instead of being stored; `--confirm` as the last argument dies on
  `set -u` (fails closed). Remediation: stop flag parsing at `--`, and parse
  globals only before the verb.

## Checked

- **Secrets** — no credential, token or key added by the diff; `.env` itself is
  not committed and `deploy/.env` does not exist in this checkout. `ask`'s secret
  path masks the default as `********` and uses `read -rs`, so a typed secret is
  not echoed; `SECRET_KEYS` is declared for Phase 3 masking. Secrets DO reach the
  terminal indirectly in two places worth noting: `confirm_word:77` echoes
  `--confirm`'s argument back in its error, and `run mv` under `--dry-run` prints
  the temp path (not the value). The real secret-adjacent issue is the ADR-052
  finding above (a test reading the operator's real `.env`).
- **Injection** — the two live ones are recorded above: `awk -v` escape processing
  in `env_set` (value → extra assignment) and the unvalidated key in `env_get`'s
  `sed` program. Checked and CLEAN: key `A` does NOT clobber `AB`
  (`index($0,k"=")==1` is anchored and includes the `=`; verified with a file
  holding both); `grep -q "^${key}="` is safe because the key regex bans regex
  metacharacters; `mktemp`+`mv -f` in the same directory is an atomic rename with
  `chmod 600` applied BEFORE the rename; no `eval`, no unquoted expansion into a
  command, shellcheck clean (`shellcheck -s bash`, 0 findings).
- **Authz/authn** — no auth code in the diff. The authz surface is indirect and is
  the reason the first finding is rated high: `.env` carries `SUPERADMINS`,
  `STYRMAN_ALLOWED_EMAILS` and `STYRMAN_INGRESS_TOKEN`, so any write primitive
  into that file is an authz primitive. `need_root:47` gates `status` correctly
  and is intentionally waived under `--dry-run` (which performs no side effect
  except `env_set`'s temp file, noted above).
- **Crypto** — nothing hand-rolled; no auth, session, hashing, JWT or OIDC code in
  the diff. `STYRMAN_JWT_KEY`/`STYRMAN_INGRESS_TOKEN` are listed as
  host-setup-computed (`COMPUTED_KEYS:118`) and `validate` correctly REFUSES
  operator edits to them.
- **Dependencies** — no Go module, no npm change; the test adds one `_test.go`
  file importing stdlib only. The one new runtime dependency is `python3` for
  `json_str`, which `deploy/host-setup.sh:32` already requires and fails fast on —
  justified, but styrman-ctl does not make the same check (finding above).
  `govulncheck` was NOT run: it is not on PATH here and the project's
  `deploy/vulncheck.sh` runs it inside a pinned builder image (docker build), which
  this diff does not warrant since it changes no Go dependency. `npm audit` not
  applicable — no `web/` change. `shellcheck` WAS run on the pinned blob: clean.
- **GCP surface** — none. Self-hosted deployment (`.agents/docs/deploy/gcp.md` is
  "not applicable"); no IAM, no service account, no Cloud Run flag in the diff.
- **Bounded root work in `status`** — `du` is `timeout 20`-bounded and does not
  follow symlinks; `find … -mindepth 2 -maxdepth 2 -type d` is depth-limited and
  does not follow symlinks (a guest planting symlinks only miscounts);
  `docker inspect`/`iptables -S`/`git rev-parse` are read-only. No BUG-009-class
  issue found: `status` never `mkdir -p`s, chowns or writes below a
  lower-privileged owner's directory.

## Not examined

- Phases 2-4 (`install`/`update`/`edit`/`uninstall`) — stubs in this range; the
  helper findings above are written against the callers those phases will add,
  and `guard_path` should be re-reviewed when the first `rm -rf` lands behind it.
- The working tree's in-progress Phase 2 changes to `deploy/styrman-ctl` and
  `internal/config/styrman_ctl_test.go` (`STYRMAN_ENV_FILE`, `OPERATOR`,
  `MUST_FILL`, the answers-file test) — out of the reviewed range; only the
  `STYRMAN_ENV_FILE` fix was noted, not reviewed.
- `deploy/host-setup.sh`, `deploy/firewall/install.sh`, `deploy/update/styrman-update`
  beyond the conventions this script is measured against — unchanged in this range.
- The plan and verification artifacts in the commit
  (`.agents/docs/plans/…`, `.agents/docs/changes/…`, `.agents/docs/testing/…`) were
  not read, per the dispatch.
- No test run: `go test ./internal/config/` was not executed (the working tree has
  moved past the reviewed commit, so a run would not describe this range).
