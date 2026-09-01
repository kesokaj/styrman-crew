# Security review — PREFLIGHT final, remediation delta `534e0bb..725ca1d`   (2026-09-01)

Scope: ONLY commit `725ca1d` ("security: require SSH-signed tags, roll back by verified
SHA") — three source files (`deploy/update/styrman-update`, `deploy/host-setup.sh`,
`internal/httpapi/check_test.go`) plus docs, agent-memory and a `web/dist` rebuild.
Every claim below was re-derived on this host in a scratch repo with an ephemeral
GPG keyring and throwaway SSH keys (all removed afterwards); the Go negative control
was run with `go test -overlay` so the repository itself was never modified
(`git status` clean before and after).

## Verdict: FINDINGS — NOTHING is ship-blocking for TAGGING. The timer would now RUN if enabled, but the gate it enforces is still bypassable, so do not enable it.

Direct answer to the question asked about finding #1: **no, the new check is not
airtight — it is bypassable exactly the way you suspected.** `git cat-file tag` prints
the whole object including the operator-supplied message, and the message is part of
what a tag author controls. A tag whose message contains a line
`-----BEGIN SSH SIGNATURE-----` at column 0, signed with **PGP**, satisfies the new
`grep` and is then handed to `git verify-tag`, which dispatches on the *last* armor
header in the object — the real PGP block — and exits 0 against any public key present
in root's keyring, allowlist never consulted. Measured here end to end: grep PASSES,
`verify-tag` exit 0, `Good signature … [unknown]` with a freshly imported public key
carrying no ownertrust. The trust set for that path is therefore unchanged from both
previous reviews: "anyone who can push a `vX.Y.Z` tag **and** get one public key into
root's keyring". It is bounded on the low side by the same accident as before — with an
empty keyring the tag fails `Can't check signature: No public key`, exit 1 — and under
the shipped unit `ProtectHome=yes` blanks `/root`, so gpg resolves an empty
`/root/.gnupg` and the timer path refuses (verified with `HOME` pointed at an empty
directory). The documented **manual** path, `sudo styrman-update`, has `HOME=/root` and
root's real keyring, and is not protected by that accident. The remediation is one line
and I verified it binds in both directions: compare the **last** armor header, which is
precisely what git's own `parse_signature` selects.

Second, a defect this delta introduces. The new `trusted` rollback reads `deployed-sha`
from `${STATE_DIR}` and checks that commit out and `docker build`s it as root with
verification **explicitly skipped** — and `${STATE_DIR}` is the one trust-bearing path
in this script whose ownership and mode are never asserted. `/var/lib/styrman` is
`chown`ed to the non-root OPERATOR by `host-setup.sh:279`, and
`/var/lib/styrman/update` is created lazily by the updater's own `mkdir -p`. Before this
delta, writing into that directory bought an attacker a *tag name* that was still
verified; now it buys an arbitrary commit that is not. Fail-closed on first run and on a
wiped state dir — that part of the design is sound — but the file needs the same
ownership/mode assertion the other three trust anchors already get.

Everything else in the delta is a clean improvement. Your prior recheck findings #2
(allowlist path) and #3 (`deploy/.env` mode) are closed, the rollback dead-end is
genuinely fixed, the `check_test.go` replacement is a real test, and no
previously-closed finding regressed. **Tag it.**

## Findings

- **[medium / confidence: high] not ship-blocking for tagging; blocks `systemctl enable --now styrman-update.timer`**
  `deploy/update/styrman-update:187` — `git cat-file tag "${tag}" | grep -q '^-----BEGIN SSH SIGNATURE-----'`
  is satisfiable from the tag **MESSAGE**, so it does not establish that the signature
  git will actually check is an SSH one. Re-derived in a scratch repo: a tag created
  with a message whose second line is the marker, signed with PGP, produced an object
  whose `cat-file` output carries the marker at column 0 *above* a real
  `-----BEGIN PGP SIGNATURE-----` block; the grep passed and
  `git -c gpg.ssh.allowedSignersFile=<allowlist-containing-an-unrelated-key> verify-tag`
  exited **0**. Repeated against a keyring holding only the freshly imported *public*
  key with no ownertrust: `Good signature … [unknown]` + "not certified with a trusted
  signature", exit **0** — the allowlist is never consulted on the PGP path. The reason
  is git's `parse_signature`, which scans every line and keeps the **last** armor header
  it sees, so a genuine signature block appended at the end always wins over a decoy in
  the message. Two controls confirm the check still holds where it can: an *unsigned*
  annotated tag carrying the marker is refused (`sshsig_armor: incomplete message`, exit
  1), and an SSH-signed tag by a non-allowlisted signer is refused (`No principal
  matched`, exit 1). Exploitable in principle by whoever can push a `vX.Y.Z` tag —
  precisely the population this control exists to exclude — provided root's keyring
  holds any public key; blocked today on the timer path only because `ProtectHome=yes`
  hands gpg an empty `/root/.gnupg`, and **not** blocked on the documented
  `sudo styrman-update` path. Remediation, verified correct in both directions here
  (SSH tag with a decoy PGP header in its message → still `-----BEGIN SSH SIGNATURE-----`;
  PGP tag with the decoy SSH header → `-----BEGIN PGP SIGNATURE-----`):
  ```sh
  [ "$(git cat-file tag "${tag}" | grep -E '^-----BEGIN (SSH|PGP) SIGNATURE-----' | tail -1)" \
      = '-----BEGIN SSH SIGNATURE-----' ] || { log "REFUSED: …"; return 1; }
  ```
  Belt-and-braces alternative for the same cost: also require that
  `git verify-tag --raw` emits no `[GNUPG:]` lines. The comment at `:181-186` and the
  spec claim at `.agents/docs/specs/styrman/deploy.md:310` ("**a PGP-signed tag is
  REFUSED, not merely unpinned**") are both false until this is fixed and must move
  with the code.

- **[medium / confidence: high] not ship-blocking for tagging; blocks enabling the timer**
  `deploy/update/styrman-update:35,344,347,368-372` — the new `trusted` rollback path
  skips verification for a ref read out of `${STATE_DIR}/deployed-sha`, and `STATE_DIR`
  is never checked for ownership or mode, unlike `${DEPLOY_DIR}/.git` (`:82`),
  `deploy/.env` (`:88,92`), `/etc/styrman/update.env` (`:144,147`) and
  `${ALLOWED_SIGNERS}` (`:194`). `host-setup.sh:278-279` creates `/var/lib/styrman` and
  `chown`s it to the **operator**, not root; `/var/lib/styrman/update` does not exist
  until the updater's `mkdir -p` (`:73`) creates it. Two consequences: a process running
  as the operator uid can pre-create `update/` before the first tick and own every state
  file outright, and even afterwards it owns the parent directory, so the root-owned
  `update/` can be renamed aside and replaced. Writing a 40-hex SHA there then selects
  any commit in the deploy clone's object store — the fetch refspec brings down branch
  refs, not only tags — and the rollback checks it out, `git clean -d`s, and
  `docker build`s it **as root** with `verify_tag` never called. The 40-hex regex bounds
  the *shape* of the ref, not its provenance. Blast radius is bounded in practice
  because the main resident of that uid is `styrman-server`, which already holds
  `/var/run/docker.sock` (`deploy/docker-compose.yml:114`) and is therefore already
  root-equivalent — but "a non-root shell as the operator can choose what root builds"
  is exactly the outcome the surrounding ownership assertions exist to prevent, and this
  delta re-opens it for a ref that is by construction unverified. Remediation: keep the
  SHA in `/etc/styrman` (root-owned `0700`, already created at `host-setup.sh:427`), or
  assert `stat -c %u` = 0 and empty `find "${STATE_DIR}" -perm /022` before reading
  `deployed-sha`, alongside the existing anchors.

  On the specific questions asked about the trust argument: "we verified this SHA when
  we deployed it" is sound **for the git tree** — `git rev-parse "${LATEST}^{commit}"`
  runs after a `verify_tag`-gated, health-confirmed deploy, no fetch intervenes, and a
  re-pointed tag cannot move a recorded SHA. It says nothing about the working
  directory: `git clean -d` (no `-x`) deliberately keeps ignored files, so a planted
  ignored file survives into the build context — but that is equally true of the
  verified path and is pre-existing, not a regression. First run after adopting this and
  a wiped state dir both behave correctly: `PREVIOUS_SHA` is empty, the code logs the
  fallback and rolls back to the **tag**, which must verify. A truncated or short write
  is likewise safe (`> "${STATE_SHA}"` truncates before `rev-parse` can fail, and the
  regex then rejects the empty value). The one way `trusted` reaches a genuinely
  unverified tree without touching the state dir: if `STYRMAN_UPDATE_REQUIRE_SIGNED=0`
  was set when the SHA was recorded, `verify_tag` returned 0 without checking anything,
  and the recorded SHA stays "trusted" after the operator re-enables signing. Low on its
  own — it needs a root opt-out first — but it argues for recording the *verification
  outcome* next to the SHA rather than inferring it.

- **[low / confidence: high] not ship-blocking**
  `deploy/update/styrman-update:174` — `ALLOWED_SIGNERS="${STYRMAN_ALLOWED_SIGNERS:-…}"`
  makes the trust anchor's location relocatable from the environment. Not a weakness as
  shipped: the script exits unless `id -u` is 0 (`:63`), the unit sets no `Environment=`,
  and sudo's default `env_reset` strips it, so reaching the override already implies
  root. Recording it because it is the one override whose effect is "choose a different
  allowlist" rather than "choose a different directory", and because a future sudoers
  rule granting `styrman-update` to a non-root user with `SETENV` would turn it into
  root code execution. The checks on the file itself are sound and correctly ordered —
  I confirmed a root-owned symlink to an attacker-owned file is caught by
  `stat -c %u` (follows) and an attacker-owned symlink to a root file is caught by
  `find -perm /022` (lstat, mode 0777), and both bad cases plus "missing/unreadable"
  take the refuse branch before `verify-tag` runs.

- **[low / confidence: high] not ship-blocking; documentation owner**
  `deploy/host-setup.sh:405` still prints
  `sudo git config --global gpg.ssh.allowedSignersFile /etc/styrman/allowed_signers`,
  which this delta made a no-op — the updater pins the path and never reads that config
  key. Harmless in itself, but the printed block is now the only operator-facing
  instruction for creating the allowlist and it omits the two properties the script
  hard-requires (root-owned, not group/world-writable). `sudo tee` happens to produce
  `0644 root:root`, which passes, so the operator will not notice until it does not.
  `deploy.md:310` needs the correction from finding 1 in the same pass. Cosmetic in the
  same block, pre-existing: `host-setup.sh:392` duplicates "(gitignored, so a fresh
  clone has none)", and the numbered prerequisites read 1, 2, "Then enable it", 3.

- **[info / confidence: medium]** `deploy/update/styrman-update:341-347` — `deployed-version`
  and `deployed-sha` can desync (delete the former, keep the latter, and `CURRENT` falls
  back to `git describe`). The rollback would then check out the recorded SHA as trusted
  but stamp it with an unrelated version, `reports_version` would fail, and the run ends
  at "rollback ALSO failed". Availability only, fails closed, no trust consequence.

## Confirmed closed by this delta (each re-derived, not taken on report)

- **Recheck #2, allowlist path unreadable under `ProtectHome=yes` — CLOSED, and the
  timer would now work.** The `git config --global` lookup is gone; `ALLOWED_SIGNERS` is
  a pinned literal. I verified the SSH verify path needs no `$HOME` at all:
  `env -u HOME GIT_CONFIG_SYSTEM=/dev/null git -c gpg.ssh.allowedSignersFile=… verify-tag`
  on an allowlisted SSH-signed tag → `Good "git" signature …`, exit **0**; the same
  command on a non-allowlisted signer → `No principal matched`, exit **1**. So the
  permanent-no-op failure mode is gone: with `/etc/styrman/allowed_signers` in place the
  timer verifies and deploys, and with it absent it refuses loudly with a `create it`
  instruction instead of a config command the operator already ran.
- **Recheck #3, `deploy/.env` ownership-only — CLOSED.** `:91-93` adds
  `find … -perm /077`, stricter than the `/022` used for `update.env`, which is the right
  call for a file holding `STYRMAN_INGRESS_TOKEN`; it sits with the ownership check,
  before `git()` is defined and before anything is fetched or built.
- **Rollback dead-end — CLOSED.** `deployed-sha` is now read at `:342` and used at
  `:368-372`; `deploy()` returns 1 instead of calling `fail` on a checkout error
  (`:274`) and on both build failures, so the `if deploy … && reports_version …`
  condition can actually fall through to the rollback branch. `verify_tag` likewise
  returns instead of exiting, and every call site handles the return: `:214`
  (`|| fail`), `:271` (`|| return 1`). No path reaches `git checkout` or `docker build`
  without either `verify_tag` or the explicit `trusted` argument.
- **Recheck #4, spec taught `ssh-keyscan` — CLOSED.** `deploy.md:377` now says "never
  `ssh-keyscan`, which is unauthenticated trust-on-first-use" and explains why; the
  prerequisites list at `:303-320` documents the SSH-signing requirement. Grepped the
  whole tree excluding `.agents/docs` and `web/dist`: **no** `ssh-keyscan` and **no**
  `accept-new` survives in any shipped script. `DECISIONS.md:463` is the guest-side
  clone, out of scope and unchanged.
- **Prior `update.env` sourcing — NOT regressed.** Still `grep`ped, never sourced;
  ownership and `-perm /022` still asserted first; the "signature checking DISABLED"
  warning at `:216` is still emitted from the main body and unsilenceable. This delta
  additionally *tightened* the opt-out regex (`:151`) — it is now anchored at `$`, so
  `…=0abc` and `…=0 # comment` no longer disable signing, while `export …=0` and `…="0"`
  do. Every mis-parse still resolves toward signing. Fail-closed direction confirmed.
- **`internal/httpapi/check_test.go` — genuinely strengthened, weakens nothing.** It
  drives the real `csrfGuard` with a 2 MiB body over four routes and adds a negative
  control (`/terminals`, expected 413). I reproduced the negative control independently
  without touching the repo: `go test -overlay` against a copy of `server.go` with the
  `/check` case deleted from the switch fails with
  `/api/projects/p1/check: read-2MiB allowed=false want=true (status 413)`. It is
  test-only, adds no route, no fixture credential and no production code path; it sends
  no `Origin` header, so it exercises the size limit rather than the cross-origin
  branch, which remains covered elsewhere. The `maxCheckBytes` bounds assertion is
  retained and tightened into one condition.

## Checked

**Secrets** — Clean. No credential material, no `.env`, nothing new echoed to the
journal. The new refuse messages name paths (`${ALLOWED_SIGNERS}`,
`${DEPLOY_DIR}/deploy/.env`) and print a `printf … | sudo tee` template, never file
contents; `grep -qE` on `update.env` stays quiet. The `deployed-sha` value that now
reaches the log via `log "rolling back to a previously verified tree (${ref})"` is a
public commit id.

**Injection** — No new sink. `${tag}` still comes from the anchored
`^v[0-9]+\.[0-9]+\.[0-9]+$` filter with `git cat-file -t` ahead of it; the rollback ref
is constrained to `^[0-9a-f]{40}$` before it is passed to `git checkout`, which closes
the leading-`-`-as-option class for the new argument. `-c gpg.ssh.allowedSignersFile=`
takes a single quoted argv element and a newline inside a `-c` value cannot inject a
second config key (re-tested previously). `bash -n` and `shellcheck -S warning` clean on
both scripts.

**Authz/authn** — Unchanged. No Go production file is touched by this delta
(`internal/httpapi/server.go` is not in it); `check_test.go` is a test. No route added,
moved or exempted; `authGuard` still wraps `csrfGuard` at `server.go:409`.

**Crypto** — Nothing hand-rolled; no primitive added. The delta changes *policy* over
vetted tooling (git / gpg / `ssh-keygen -Y verify`). Finding 1 is that the policy check
is a string match on an attacker-influenced buffer rather than an assertion about the
block git will verify — the fix stays inside the vetted tooling. A third format remains
untested and unaddressed by the current check: an x509/gpgsm-signed tag dispatches to
`gpg.x509.program`, also outside the allowlist; the `tail -1` remediation handles it by
construction because its header is neither of the two accepted forms.

**Dependencies** — No dependency change: `git diff --stat 534e0bb..HEAD` is empty for
`go.mod`, `go.sum`, `web/package.json` and `web/package-lock.json`. Both scanners re-run
on the committed tree anyway: `npm audit` in `web/` → **found 0 vulnerabilities**;
`govulncheck ./...` (via `go run golang.org/x/vuln/cmd/govulncheck@latest`; not installed
locally) → **2 called vulnerabilities from 1 module**, the pre-existing
`github.com/docker/docker@v28.5.2+incompatible` pair already recorded in `TODO.md:53`,
plus 3 uncalled — byte-identical in shape to the `534e0bb` run, nothing new.
`go build ./...` clean and `go test ./internal/httpapi/` passes.

**Host state** — Re-confirmed that none of this is live: `systemctl is-enabled
styrman-update.timer` → `not-found`, and `/opt/styrman`, `/etc/styrman`,
`/var/lib/styrman` and `/usr/local/bin/styrman-update` do not exist on this host. That
is what keeps both findings off the tagging path. Test artifacts (scratch repo,
ephemeral GPG homes, throwaway SSH keys, and a `~/.gnupg` that gpg created as a side
effect of the no-`$HOME` experiment) were all removed; `git status` is clean.
