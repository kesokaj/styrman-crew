---
owner: security
date: 2026-09-11
---
# Security review — styrman-ctl phase 4 (uninstall + the round-2 credential fixes)   (2026-09-11)

Range reviewed: edc4858..108c4e4 — `deploy/styrman-ctl` only (docs out of scope by the dispatch).
Blob pinned: `deploy/styrman-ctl` at 108c4e4 = md5 `ee3dadce5808a799454a6d7db0b2a29e`, 830 lines.

**The tree MOVED under this review.** `main` was `108c4e4` when the dispatch was written and is
`becbd6b` now (`c397653` at 15:04, `becbd6b` at 15:05 local, both while this review was reading).
The working copy is 860 lines, md5 `79df467d83a6535405377f89ad8ef84a`. Two of the findings below
were independently found and fixed by those two commits; they are still reported against the
reviewed range, each labelled with its state at `becbd6b`, because a signed release could be cut
from either commit. Everything marked **open at HEAD** was re-checked against the working copy.

## Verdict: FINDINGS

The worst of it is S1: at `108c4e4`, `uninstall --level 2` and `--level 3` cannot complete on a
correctly installed host. The teardown deletes the firewall jumps with `iptables -D DOCKER-USER -j
STYRMAN-FWD`, but `deploy/firewall/styrman-firewall.sh:56,62,79` installs them as `-i sdb-+ -j
STYRMAN-FWD`, and iptables deletes only by an exact rule-spec match. The delete returns 1, it is
not `|| true`-guarded, `run` is a plain function, and the script is `set -e` — so the run aborts at
that single line, after level 1 has already destroyed the stack and every project microVM. Nothing
past it executes: `/etc/styrman/gh-token` (the fine-grained GitHub PAT), `/etc/styrman/allowed_signers`
(the updater's entire release-trust root), `/opt/styrman/deploy/.env` (Google client secret,
Cloudflare API token, `STYRMAN_INGRESS_TOKEN`, `STYRMAN_JWT_KEY`), the unit files, the two root
binaries, the images and — at level 3 — all of `/var/lib/styrman` survive an operation the printed
plan promised would remove them, and the operator gets a bare non-zero exit with no `die` message.
The in-range test asserted the *defective* string verbatim (`internal/config/styrman_ctl_test.go:691`
at 108c4e4), which is why it went green: the suite asserts the `--dry-run` plan's TEXT, and `run`
echoes whatever argv it is handed, so no test in this range can distinguish a command that works
from one that always fails. `becbd6b` fixes S1. S2 — `guard_path`'s allow-list is built from the
same two env vars it is meant to constrain — is still open at HEAD and turns `--level 3` into
`rm -rf /var/lib` for anyone who can set `STYRMAN_STATE_DIR` across `sudo`.

## Findings

- **S1 [high / confidence: high]** `deploy/styrman-ctl:698-705` @108c4e4 — **FIXED at `becbd6b`**
  (`drop_chain`, which derives the rule from `iptables -S` and makes every step best-effort).
  The level-2 chain teardown can never match the installed rule, and its failure kills the run.
  Measured, not reasoned:
  (a) this host's live rules are `-A INPUT -i sdb-+ -j STYRMAN-IN`, `-A DOCKER-USER -i sdb-+ -j
  STYRMAN-FWD`, `-A INPUT -i sdb-+ -j STYRMAN-IN6`, `-A FORWARD -i sdb-+ -j STYRMAN-FWD6`
  (`iptables -S` / `ip6tables -S`, read-only);
  (b) in a throwaway `unshare -n` namespace, `iptables -D DOCKER-USER -j STYRMAN-FWD` against that
  exact rule returns 1 ("Bad rule"), the subsequent `-X` returns 4 ("Device or resource busy"), and
  the jump is still present afterwards;
  (c) `bash -c 'set -euo pipefail; run(){ "$@"; }; f(){ echo A; run false; echo B; }; f; echo after'`
  prints only `A` and exits 1 — a failing `run` inside a function aborts the whole script.
  Scenario: an operator decommissioning the NUC runs `uninstall --level 3 --confirm <hostname>`,
  reads a plan listing `rm -rf -- /etc/styrman`, `rm -rf -- /opt/styrman`, `rm -rf -- /var/lib/styrman`,
  types the hostname, watches the stack and every microVM go away, and is left with a host that still
  holds a live GitHub PAT, the release-signing trust root, and every production secret in two copies
  of `deploy/.env` — while believing the machine is wiped. Remediation (as landed in `becbd6b`):
  enumerate the jumps with `iptables -S <parent> | grep -- "-j <chain>$"` and delete each one by its
  own text; make `-D`/`-F`/`-X` best-effort; see S8 for the residual in that fix.
- **S2 [medium / confidence: high]** `deploy/styrman-ctl:210-226` (`guard_path`) with `:23-24` —
  **open at HEAD.** `guard_path` proves a delete target is canonical, absolute, at least two
  components and *one of the configured styrman trees* — but the "configured trees" are
  `${STATE_DIR}` and `${DEPLOY_DIR}`, read straight from `STYRMAN_STATE_DIR` / `STYRMAN_DEPLOY_DIR`
  with no grammar. The guard therefore validates the path against a variable the same actor
  supplies. `c397653` added exactly this gate for the other seven path overrides
  (`:38-42`, "test seam, not a production knob"), and deliberately left these two ungated as
  "documented knobs" — they are the only two that reach `rm -rf`. Measured: with
  `STYRMAN_STATE_DIR=/var/lib`, `guard_path /var/lib` returns 0 (probed through the `__guard-path`
  hook; the hook needs `STYRMAN_CTL_TEST=1` but the production path
  `uninstall --level 3` → `rm_tree "${STATE_DIR}"` → `guard_path` does not). Scenario: any sudoers
  entry with `SETENV`, any `sudo -E`, or a root cron/systemd wrapper that exports
  `STYRMAN_STATE_DIR=/var/lib`, plus `uninstall --level 3 --confirm <hostname>`, is `rm -rf /var/lib`
  — Docker's image store, the systemd journal, dpkg's database. The two-component rule only excludes
  `/etc`, `/var`, `/opt`; it accepts `/var/lib`, `/usr/local`, `/home/<user>`, `/etc/systemd`.
  Remediation: give the two surviving knobs the same grammar the rest got — require
  `^/[A-Za-z0-9._/-]+$`, refuse a path whose last component is not `styrman`-shaped, and keep a
  hard deny-list of FHS directories (`/usr`, `/var/lib`, `/etc/systemd`, `/home`, `/root`, `/boot`).
- **S3 [medium / confidence: medium]** `deploy/styrman-ctl:486-493` @108c4e4, unchanged at HEAD —
  **open.** `--allowed-signers-file` gets no integrity check at all: not `private_file`, not an
  owner check, not a symlink check, not a check on the containing directory. Its one line is then
  installed as `/etc/styrman/allowed_signers`, mode 644, and is the *entire* trust root
  `deploy/update/styrman-update` uses to decide which signed release tag root will deploy. The
  matching secret, `--gh-token-file`, did get `private_file` (`:469-472`), so the asymmetry is
  visible in the diff itself — and for a trust root, integrity matters more than confidentiality.
  Scenario: the operator prepares `/tmp/signers` (a world-writable, sticky directory) and runs
  `sudo styrman-ctl update --setup --allowed-signers-file /tmp/signers`; another local account
  creates `/tmp/signers` first, or swaps it in the window before root reads it, and every later
  updater tick accepts release tags signed by that key and deploys them as root. Remediation: apply
  the same bar the updater's own `trusted_dir` applies — refuse a symlink, refuse a file not owned
  by root or `SUDO_USER`, refuse group/world-*writable* file or parent — and echo the installed
  fingerprint (`ssh-keygen -lf`) so the operator can eyeball what they just trusted.
- **S4 [medium / confidence: high]** `deploy/styrman-ctl:488` @108c4e4, unchanged at HEAD —
  **open; this is F8 only half-fixed.** The new guard is
  `[ "$(wc -l < "${signers_file}")" -le 1 ] || die`, and `wc -l` counts NEWLINES, not lines. A
  two-signer file with no trailing newline counts 1, passes the guard, and `head -1` silently drops
  the second signer exactly as before. Measured: a file containing
  `a@x.com ssh-ed25519 AAAA\nb@x.com ssh-ed25519 BBBB` (no final newline) gives `wc -l` = 1 and
  `head -1` = the first line. Scenario: an operator adds a colleague's key as a second line with
  `printf '%s'`, sees the setup succeed, and discovers months later — during an incident, when the
  colleague cuts the release — that the updater refuses the tag. Remediation: stop reducing the file
  to one line. Validate every non-blank line with `is_signers_line` and install the whole file;
  `head -1` is the bug, the counter is a patch over it.
- **S5 [low / confidence: high]** `deploy/styrman-ctl:683-687` (level 1's `docker compose down`) —
  **open at HEAD.** `down` without `-v` keeps named volumes, and `deploy/docker-compose.yml`
  declares `caddy_data` and `caddy_config`. `caddy_data` is Caddy's certificate store: the ACME
  account private key and the private keys of every issued certificate for `*.crew.styrman.xyz`.
  The level-3 menu text says "+ everything … IRREVERSIBLE", yet those keys survive in
  `/var/lib/docker/volumes/` after a level-3 uninstall. Scenario: a decommissioned NUC is sold or
  re-imaged loosely; the TLS private keys for a live public hostname go with it. Remediation: at
  level 3, `down --volumes` (or `docker volume rm` the two by name after the `down`), and say so in
  the level-3 plan line; at levels 1-2 keeping them is right and should be stated.
- **S6 [low / confidence: medium]** `deploy/styrman-ctl:739` with `confirm_word` at `:84-94` —
  **open at HEAD.** Level 3's confirmation word is
  `printable "$(hostname 2>/dev/null || uname -n)"`. The `||` fires only when `hostname` *fails*,
  not when it succeeds and prints nothing, so a host with an empty kernel hostname yields
  `word=""` — and `confirm_word` then compares the typed line against the empty string, so pressing
  Enter at `Type  to continue:` executes the irreversible delete of `/var/lib/styrman`. The same
  empty word is printed in the dry-run hint ("the word to type for level 3 is: "). `--confirm ''`
  does *not* short-circuit (the `[ -n "${CONFIRM_GIVEN}" ]` test), so this is the interactive path
  only. Remediation: `[ -n "${word}" ] || word=delete-all-styrman-data`, or `die` when the host has
  no name — a confirmation that an empty line satisfies is not a confirmation.
- **S7 [low / confidence: high]** `deploy/styrman-ctl:714` — **open at HEAD.** Two problems in one
  line. (a) `mv -- "${ENV_FILE}" "${ENV_FILE}.removed-$(date +%Y%m%d-%H%M%S)"` has one-second
  resolution and no `-n`/`--backup`, so two uninstalls in the same second silently clobber the
  first backup; the plan also prints a timestamp computed in the plan subshell, which is never the
  one the execution uses. (b) More to the point, the backup is the full production `.env` — Google
  client secret, Cloudflare API token, `STYRMAN_INGRESS_TOKEN`, `STYRMAN_JWT_KEY` — left
  indefinitely in the operator's git checkout by an operation whose menu text says it removes the
  host configuration. It is mode-600 and `.gitignore:7` (`.env.*`) does cover
  `deploy/.env.removed-*` (verified with `git check-ignore -v`), so there is no commit risk; the
  finding is secrets-at-rest after a teardown, and an operator who is not told is an operator who
  will not shred it. Remediation: `mv -n` (or `mktemp`-suffixed), and log the backup's absolute
  path with an explicit "contains every production secret — `shred -u` it when you are done".
- **S8 [low / confidence: medium]** `deploy/styrman-ctl:694-704` at `becbd6b` (the S1 fix; outside
  the reviewed range, inside its blast radius) — **open at HEAD.** `drop_chain` now runs
  `run "${ipt}" -D … || true` and then unconditionally `-F "${chain}" || true`. If the unhook fails
  for any reason the fix did not anticipate (an iptables-nft/legacy split rendering the rule
  differently under `-S`, a second jump inserted by hand, `grep` finding nothing because the parent
  chain does not exist), the flush still runs and leaves an **empty STYRMAN-FWD still referenced
  from DOCKER-USER** — an empty user chain returns, so every private-range DROP that confines a
  sandbox project is gone while the hook remains. Level 2 also removes
  `styrman-firewall.{service,timer}`, so nothing re-establishes them. Scenario: a level-2 uninstall
  is done, then reversed by bringing the stack back up by hand (`docker compose up -d`) instead of
  re-running `install`; sandbox projects come back with an empty isolation chain and reach the LAN,
  the host and each other's bridges. Remediation: flush and delete only after confirming no jump
  remains — `"${ipt}" -S "${parent}" | grep -q -- "-j ${chain}\$" && { warn "…still hooked, leaving
  it intact"; return 0; }` — and never flush a referenced chain.
- **S9 [low / confidence: medium]** `deploy/styrman-ctl:511-513` @108c4e4 — **mitigated at HEAD by
  `c397653`'s gate, not by this line.** The inline credential helper is built by raw string
  interpolation of `${ETC_DIR}`:
  `helper="credential.${origin_host}.helper=!f() { … \"\$(cat ${ETC_DIR}/gh-token)\"; }; f"`.
  Git executes everything after `!` through `sh -c`, so the path lands unquoted in a root shell
  snippet: a space breaks the helper (git runs `cat /etc/my styrman/gh-token` and the fetch fails
  with no credential), and a `;`, a backtick or a `$(…)` in `ETC_DIR` is a command root's git runs.
  `${origin_host}` is safe — `https_origin`'s new `^https://[a-z0-9.-]+/[A-Za-z0-9._/-]+$` anchor
  admits no metacharacter. At `becbd6b`, `STYRMAN_ETC_DIR` is refused outside the test harness, so
  the only remaining writer of `ETC_DIR` is the script's own literal default; the finding stands as
  a latent quoting defect that the gate, not the construction, is holding shut. Remediation:
  single-quote the path inside the snippet and validate `ETC_DIR` against `^/[A-Za-z0-9._/-]+$`
  before it is interpolated, so the guarantee lives where the string is built.
- **S10 [info]** `deploy/styrman-ctl` teardown generally, and
  `internal/config/styrman_ctl_test.go:668-740` — the `--dry-run` plan is produced by `run`
  echoing its own argv, so it proves the *words* a step would use and nothing about whether the
  step works; S1 is exactly that gap, and the in-range test pinned the broken words. Outside the
  iptables lines (now best-effort), every teardown step still aborts the whole run on failure:
  `docker rm -f ${ids}` on a wedged Kata container ends level 1 before the units are disabled;
  `docker image rm` on an image another container still references ends level 2 before the `.env`
  backup and before level 3 touches `/var/lib/styrman`. Fail-closed is the right direction for a
  destructive verb, but silent is not: give every teardown step `|| warn`, count the failures, and
  end with "N of M steps failed — the host is PARTIALLY uninstalled" instead of a bare non-zero exit.

## Disposition of the Phase 2 findings (F1-F10)

| # | was | now | evidence |
|---|-----|-----|----------|
| F1 | `awk -v` escape expansion defeats the newline guard | **resolved** | `:117` refuses a backslash in any value outright; `:128` passes the value through `V="${value}" … ENVIRON["V"]`, and the surviving `-v k=` side is constrained to `^[A-Z][A-Z0-9_]*$` by `is_key` |
| F2 | host-agnostic `GIT_ASKPASS` offers the token to any host | **resolved** | `:513,527,530` — inline helper scoped to `credential.https://<origin-host>.helper`, `-c credential.helper=` clears inherited helpers, and `https_origin` now admits only plain `https`. Residual: S9 (quoting of the path inside it) |
| F3 | clone URL as a bare positional, any transport | **resolved** | `:398` anchors `^https://[a-z0-9.-]+/[A-Za-z0-9._/-]+$` (no userinfo, no port, no `ext::`/`file://`, nothing option-shaped); `:530` uses `clone -- "${origin}"` |
| F4 | existing clone fetched with the token, origin unchecked | **partial** | `:521-525` compares the clone's origin to `https_origin` and dies on a mismatch — but the updater's own `trusted_dir` bar (root ownership, not group/world-writable `.git`) is still not applied, and `install -m 600 … ${ETC_DIR}/gh-token` at `:517` writes the token *before* the origin check runs |
| F5 | `__*` hooks reachable in the shipped script | **resolved** | `:761-762` — `cmd_internal` dies unless `STYRMAN_CTL_TEST=1`. `__env-set` is still path-unguarded, but unreachable without the flag |
| F6 | source secret files unchecked, left on disk | **partial** | `private_file` (`:469-472`) checks the *mode* of `--gh-token-file` only. No owner check, no symlink check, no check on the containing directory; nothing for `--allowed-signers-file` (S3) or `--env-file`; the source file is still left where it was |
| F7 | `EXIT`-only trap around the token temp dir | **resolved** | `:503` — `trap 'rm -rf "${tmpd}"' EXIT INT TERM HUP`, and the dry-run branch writes nothing at all |
| F8 | unanchored signers regex; silent `head -1` | **partial** | `:467` anchors the pattern with an optional trailing comment field; the new line-count guard at `:488` is defeated by a missing trailing newline (S4) and the file is still reduced to one line rather than validated whole |
| F9 | `$ { } \` accepted in secrets that compose interpolates | **resolved** | `:176` — measured: `a$b`, `a{b`, `a}b`, `a\b`, `a b` all rejected; `abc` accepted. (`;`, backticks and quotes are still accepted, correctly: the value never reaches a shell, only `--env-file`) |
| F10 | `docker logs --tail 30` to stderr on a failed bring-up | **open (info)** | unchanged at `:356`; still an info item, no secret is known to be logged by the server |

## Checked

- **Secrets** — no credential is committed in the range. The GitHub token still never reaches argv
  or the plan: read into a variable, written under `mktemp -d` 700 with `umask 077`, installed
  `-m 600 -o root -g root`, and the helper reads the file at call time rather than embedding it
  (`--dry-run` prints the helper string, which carries a path, not a token). The new secrets issues
  are all at rest *after* the new verb runs: S1 (nothing is deleted at all), S5 (Caddy's TLS private
  keys), S7 (the `.env` backup). `.gitignore:7` (`.env.*`) covers `deploy/.env.removed-*` — verified
  with `git check-ignore -v`, not assumed.
- **Injection** — `docker ps -aq --filter label=styrman.managed=true | tr '\n' ' '` into an
  unquoted `docker rm -f ${ids}` is **clear**: `-q` emits 12-hex-character IDs, which carry no IFS
  character and no glob metacharacter, and the `[ -z "${ids// /}" ]` guard stops an empty expansion
  from turning into `docker rm -f` with no operand. A leading `-` cannot reach docker on any of the
  four paths: containers come from `-aq` (hex), networks are `grep -E '^(sdb|lan)-'`-anchored,
  images are `grep -E '^styrman-(runtime|server|caddy):'`-anchored, and the fallback branch uses two
  literals; all four are quoted at the call site anyway. `drop_chain`'s unquoted `${rule}` at
  `becbd6b` is word-split from `iptables -S` output — intended, and iptables renders no glob
  character in these rules, but it runs without `set -f`. `guard_path`'s `case`/`[[` patterns are
  quoted. `ver` is forced to `^[0-9]+\.[0-9]+\.[0-9]+$` or `0.0.0` before it becomes a compose
  variable. The one real interpolation-into-a-shell is S9.
- **`guard_path` under the plan/execution TOCTOU** — **clear on the race, broken on the allow-list
  (S2).** The plan subshell and the execution each call `guard_path` independently, so the
  execution's check is the one immediately preceding its own `rm -rf`; the window is the usual
  check-then-act microseconds, and winning it means replacing `/etc/styrman`, `/opt/styrman` or
  `/var/lib/styrman` *itself*, whose parents are root-owned. Below the tree it does not matter:
  GNU `rm -rf` does not follow symlinks during traversal, so the guest-writable homes under
  `/var/lib/styrman/homes/<u>/p/<uuid>` (the BUG-002/003 shape) cannot redirect the delete outside
  the tree — the links are unlinked, not followed. `findmnt` shows no host mount under the state
  dir, so no `rm -rf` crosses into another filesystem today; that is a property of the current
  layout, not an invariant the code asserts, and `--one-file-system` would make it one.
- **`--dry-run --confirm WORD` cannot execute** — **clear, verified by reading both gates.**
  `:747` returns early on `DRY=1` with no `--confirm`; when `--confirm` *is* given the flow reaches
  `confirm_word` (which dies on a mismatch) and then `:753`, `[ "${DRY}" = 1 ] && { log …; return 0; }`,
  before `uninstall_level` is ever called for real. The `&&`-list form does not trip `set -e` when
  `DRY=0` (probed: a failing `[ … ] && cmd` inside a function and inside a `for` body both let the
  script continue), so the non-dry path falls through correctly.
- **Level 3 without the hostname** — reachable in exactly one way, S6 (empty hostname ⇒ empty word
  ⇒ Enter confirms). Otherwise the level-3 word is neither guessable from the script nor satisfiable
  by the level-1/level-2 words (`TestStyrmanCtlUninstallConfirmations` covers the cross-level case),
  `--confirm ''` does not short-circuit, and a leading `--confirm` is consumed as the *verb* by
  `main`, so the interactive menu cannot inherit one.
- **Authz/authn** — nothing here implements authentication. `need_root` gates every verb and is
  bypassed only under `--dry-run`, which is now genuinely read-only (`becbd6b` moved `du` behind
  `run`; the plan subshell otherwise only runs `docker ps/inspect/images/network ls`, `iptables -S`,
  `realpath`, `stat`). An unprivileged local user can therefore still run
  `uninstall --level 3 --dry-run` and learn the deployed version, the managed container IDs and
  whether each styrman tree exists — an info leak of no consequence on a single-operator host.
- **Crypto** — none hand-rolled in the range. The release-trust decision belongs to
  `deploy/update/styrman-update` (`git verify-tag` + `gpg.ssh.allowedSignersFile`); this diff only
  installs the file that decision reads, which is why S3 and S4 are rated where they are.
- **Dependencies** — no new dependency; the script shells out to tools `host-setup.sh` already
  requires. `govulncheck` was **not run**: the reviewed change is bash, and `deploy/vulncheck.sh`
  builds a pinned builder image (a minutes-long docker build) for a diff with no Go source in it.
  `npm audit` is not applicable — no `web/` file is in range. The one Go file in the commit
  (`internal/config/styrman_ctl_test.go`) is a test that shells out to the script; it was run
  (`go test ./internal/config/ -run StyrmanCtlUninstall -v` — both tests PASS at HEAD) and is
  discussed under S1/S10, not as a dependency.
- **GCP surface** — none. styrman is self-hosted; `.agents/docs/deploy/gcp.md` records "not
  applicable". No IAM binding, service account, `--allow-unauthenticated` or Secret Manager use
  anywhere in the range.
- **BUG-035 (a mechanism reaches every caller)** — the inverse shape is present and named: S1 is a
  teardown step that could not reach its target on the only path it actually runs, and the test that
  vouched for it asserted the step's *text*. The rule's own remedy applies — verify with the exact
  live-host command (`iptables -S DOCKER-USER`), never by reading the script's words.
- **ADR-052 (privileged tests point at a scratch prefix)** — honoured and, at `c397653`, enforced:
  the path overrides the Go test needs are now refused outside `STYRMAN_CTL_TEST=1`. S2 is the two
  knobs deliberately left outside that gate.
- **ADR-055 (no backup layer; durability is git, R13)** — the level-3 text says so, correctly. S5
  and S7 are the two things level 3 leaves behind despite saying "everything".

## Not examined

- `deploy/host-setup.sh`, `deploy/firewall/install.sh` and `deploy/update/styrman-update` as wholes.
  Read only for the comparisons this review needed: the installed jump rules
  (`styrman-firewall.sh:53-62,79,99-110`), the chain names, and the updater's credential/`trusted_dir`
  shape referenced by F4 and S3.
- `deploy/docker-compose.yml` beyond its `volumes:` block and its interpolated variable list (S5).
- The docs in the range (`deploy/README.md`, `specs/styrman/deploy.md`, `deploy/local.md`,
  `deploy/remote.md`) — excluded by the dispatch. **Note for whoever owns them:** if they state that
  level 3 removes everything, S5 makes that untrue, and S1 made it untrue of level 2 as well at
  `108c4e4`.
- `cmd_install`, `cmd_status` and `cmd_edit` except where the new code reaches them
  (`deployed_version`, `compose_dir`, `validate`, `env_set`, `guard_path`).
- Commits `c397653` and `becbd6b` were read only for the regions that bear on findings in the
  reviewed range (the override gate at `:38-42`, `drop_chain` at `:690-704`, `run du`). They have
  not had a review of their own, and S8 is a finding *in* `becbd6b` — it needs one.
- No destructive command was run. The probes were: read-only `iptables -S`/`ip6tables -S`/`findmnt`
  on this host; an `unshare -n` throwaway namespace for the iptables delete semantics; `bash -c`
  one-liners for `set -e` and the secret regex; scratchpad files for `wc -l`; and the script's own
  read-only `__guard-path` hook. Nothing in the repository was modified and no exploit was built.

## Verdict line

FINDINGS — 1 high (S1: at `108c4e4` the level-2/3 uninstall aborts at its first `iptables -D` and
leaves the GitHub PAT, the release-trust root and every production secret on a host the operator
believes is wiped; fixed at `becbd6b`), 3 medium (S2 `guard_path`'s allow-list is populated by the
ungated `STYRMAN_STATE_DIR`, making `--level 3` an `rm -rf /var/lib` — open at HEAD; S3 the
allowed_signers source file gets no integrity check at all; S4 the new one-signer guard is defeated
by a missing trailing newline), 5 low and 1 informational. Of the Phase 2 findings F1-F10: six
resolved (F1, F2, F3, F5, F7, F9), three partial (F4, F6, F8), one open as an info item (F10).
