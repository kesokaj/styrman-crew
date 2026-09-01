# Security review — PREFLIGHT recheck, remediation delta `6b41979..534e0bb`   (2026-09-01)

Scope: ONLY the follow-up commit `534e0bb` ("security: pin the tag signer, verify on
rollback, drop the keyscan TOFU seed") — 4 source files
(`deploy/update/styrman-update`, `deploy/host-setup.sh`, `internal/httpapi/server.go`,
`internal/httpapi/check_test.go`) plus three docs files and a `web/dist` rebuild.
Each finding of `2026-09-01-security-preflight-v1.15.1.md` re-derived on this host
rather than taken on report; blast radius followed into
`deploy/update/styrman-update.service` (which the delta did not touch but now
depends on) and into `.agents/docs/specs/styrman/deploy.md` (the durable operator
instructions the changed comments contradict).

## Verdict: FINDINGS — NOTHING is ship-blocking for TAGGING v1.15.1. Two items block enabling the timer, and one of them means the timer cannot work at all.

Four of the six items are genuinely closed: the TOFU seed is gone with no surviving
path, the rollback is verified, `update.env` is read instead of sourced and the
"signature checking DISABLED" warning can no longer be silenced, and the 5 MiB
`maxCheckBytes` guard is now reachable without widening any DoS surface.

The headline remediation is not. **`git -c gpg.format=ssh … verify-tag` does not pin
anything for a PGP-signed tag** — verified empirically on this host: git dispatches
verification on the *signature payload type*, not on `gpg.format`, so a tag carrying
`-----BEGIN PGP SIGNATURE-----` is still handed to gpg, still exits 0 for a
good-but-untrusted key sitting in root's keyring, and the allowed-signers file is
never consulted. The SSH path *does* enforce the allowlist ("No principal matched",
exit 1) and does so regardless of `gpg.format`, so the option is simply inert.
Directly answering the question asked: no, this does **not** break a GPG-signing
operator — GPG-signed tags still verify fine. That is the problem. Prior finding #1
is narrowed (unconfigured hosts now refuse everything, SSH signers are allowlisted)
but not closed.

Immediately behind it, a defect the delta *introduced*: `verify_tag` now hard-requires
`git config --global --get gpg.ssh.allowedSignersFile`, and under the shipped
systemd unit that read can never succeed — systemd does not set `$HOME` for a system
service with no `User=`/`SetLoginEnvironment=` (`git config --global` then dies with
`fatal: $HOME not set`, exit 128, swallowed by `2>/dev/null || true`), and
`ProtectHome=yes` in `styrman-update.service:19` blanks `/root` anyway. The updater
therefore `fail`s on **every timer tick** with "tag signing is not verifiable",
forever. It fails CLOSED, so it is not exploitable — but the release's headline
feature is inert in its shipped configuration, and the manual `sudo styrman-update`
test cannot reveal it, because sudo has a `$HOME` and no `ProtectHome`.

Because the timer is disabled and uninstalled on this host, neither item can be
reached by an attacker today. Tag the release.

## Findings

- **[medium / confidence: high] not ship-blocking; blocks `systemctl enable --now styrman-update.timer`**
  `deploy/update/styrman-update:165` — `command git -c gpg.format=ssh -c gpg.ssh.allowedSignersFile="${signers}" verify-tag "${tag}"`
  does not force SSH verification. Re-derived here, four experiments in a scratch repo:
  (a) PGP-signed tag, signing key present in the keyring, `-c gpg.format=ssh` +
  allowlist containing an unrelated SSH key → **exit 0**, allowlist never consulted;
  (b) same tag with the *public key only*, freshly imported, **no ownertrust** →
  `Good signature … [unknown]` + `WARNING: This key is not certified…`, **exit 0**;
  (c) same tag with `gpg.ssh.allowedSignersFile=/nonexistent` → still **exit 0**;
  (d) SSH-signed tag, signer absent from the allowlist → `No principal matched`,
  **exit 1**, and present → exit 0 *even with `-c gpg.format=openpgp`*. git's
  `check_signature()` selects the verifier from the armor header, so `gpg.format`
  is a signing-side knob only. Net trust set for the PGP path is unchanged from the
  prior review: "anyone who can push a tag **and** get one public key into root's
  keyring". Bounded on the low side by experiment (e): with an empty root keyring the
  same tag fails `Can't check signature: No public key`, exit 1 — so a host whose root
  never imported a key is safe by accident, not by control. Remediation: make the
  format an assertion instead of a preference — reject the tag unless its signature
  block is `-----BEGIN SSH SIGNATURE-----`
  (`git cat-file tag "${tag}" | grep -q '^-----BEGIN SSH SIGNATURE-----'`) before
  verifying, or for a PGP release key pin the fingerprint:
  `git verify-tag --raw "${tag}" 2>&1 | grep -q "^\[GNUPG:\] VALIDSIG ${FPR}"`.
  Also fix the comment at `:148-153` and the operator text at
  `deploy/host-setup.sh:403-410`, both of which state the allowlist is what the
  updater enforces.

- **[medium / confidence: high] not exploitable (fails closed); makes the shipped timer a permanent no-op**
  `deploy/update/styrman-update:161` with `deploy/update/styrman-update.service:19` —
  the trust anchor's *location* is read from root's ambient `--global` gitconfig, which
  the unit cannot see. Verified two ways: `man systemd.exec` — "$HOME, $LOGNAME and
  $SHELL are only set for units that have `User=` set and `SetLoginEnvironment=` unset
  or true" (the unit sets neither), and `ProtectHome=` — "/home/, /root and /run/user
  are made inaccessible and empty for processes invoked by this unit"; and empirically,
  `env -u HOME git config --global --get gpg.ssh.allowedSignersFile` → `fatal: $HOME
  not set`, exit 128. `2>/dev/null || true` turns that into an empty `signers`, so
  `verify_tag` takes the `fail` branch on every invocation and tells the operator to
  run the `git config --global` command they already ran. Asserting the *file* exists
  does not remove the ambient-trust problem; it relocates it to a config path that is
  environment-dependent. Remediation: drop the gitconfig lookup and read a fixed
  root-owned path — `signers=/etc/styrman/allowed_signers` — asserting `stat -c %u` = 0
  and `find … -perm /022` empty on it, exactly as `update.env` is now handled; that is
  also what the printed instructions create. (Security-relevant beyond availability:
  an operator who hits this loop is one step from `STYRMAN_UPDATE_REQUIRE_SIGNED=0`.)

- **[low / confidence: high] not ship-blocking**
  `deploy/update/styrman-update:84` — the new `deploy/.env` guard checks ownership only
  (`stat -c %u` = 0) while its own failure message says "refusing to deploy with
  credentials another user can write". A root-owned file with mode 0666 or 0664 passes
  and is exactly the case the message describes; a root-owned 0644 leaks
  `STYRMAN_INGRESS_TOKEN` and `STYRMAN_ALLOWED_EMAILS` to every local user and also
  passes. The delta already implements the right pattern nine lines away for
  `update.env` (`find … -perm /022`), so this is an inconsistency, not a missing idea.
  Remediation: add the same `-perm /022` test (ideally `/077`, since this file is a
  secret) next to the ownership check. The same gap exists on the `.git` assertion at
  `:79` (pre-existing, unchanged by this delta).

- **[low / confidence: high] not ship-blocking; documentation owner**
  `.agents/docs/specs/styrman/deploy.md:343` and `:308` — the durable spec is now the
  only place that still teaches the two behaviours this commit removed. `:343` tells
  the operator to pin `/etc/styrman/known_hosts` "via `ssh-keyscan github.com`" — the
  unauthenticated TOFU seed the commit deleted from `host-setup.sh` precisely because
  it moved the window rather than closing it; an operator following the spec
  re-introduces it by hand. `:308` still describes the gate as "`git verify-tag` must
  succeed" and lists three prerequisites that do **not** include an
  `allowed_signers` file — following it produces a host where the updater refuses
  every tag (see finding 2). `DECISIONS.md:1867` is stale in the same way but ADRs are
  append-only, so it needs a superseding note, not an edit. The script comments
  themselves are correct and do not overclaim — checked line by line at
  `styrman-update:93-98`, `:119-132`, `:146-153`, `:217-220` and
  `host-setup.sh:349-358`; each now states what the code does, including the
  deliberate "does NOT auto-seed" wording.

- **[info / confidence: high]** `internal/httpapi/check_test.go:255` — the new test
  does not exercise `csrfGuard`. Its loop asserts its own string literals, and the two
  real assertions compare `maxCheckBytes` against hard-coded bounds. Deleting
  `"/check"` from the `csrfGuard` switch would not fail any test, so the limit that was
  just made load-bearing has no regression guard. A table test through
  `csrfGuard(http.HandlerFunc(...))` with a 2 MiB body would.

- **[info / confidence: high]** `deploy/update/styrman-update:133-144` — `update.env`
  is now parsed for exactly one key. Any other assignment that used to take effect by
  sourcing (`STYRMAN_UPDATE_HEALTH_TIMEOUT`, `STYRMAN_IMAGE`, `STYRMAN_DEPLOY_DIR`) is
  now silently ignored. That is the correct security trade, but it is a silent
  behaviour change and nothing warns about unknown keys.

## Confirmed closed (each re-derived, not taken on report)

- **Prior #3, rollback bypass — CLOSED.** `verify_tag "${tag}"` is the first statement
  of `deploy()` (`:221`), before `git checkout`. `deploy()` has exactly two call sites
  (`:293` LATEST, `:310` PREVIOUS) and both are covered; `verify_tag "${LATEST}"` at
  `:175` additionally runs before the failed-latch, the "already up to date"
  short-circuit and the `--check` exit, so `--check` fails closed too. Traced every
  other path that reaches `git checkout`/`docker build`: there is none. Operational
  note, not a finding: a rollback target signed by a since-retired key now `fail`s
  rather than deploying — correct, but it turns a bad release into a manual repair.
- **Prior #2, `ssh-keyscan` TOFU — CLOSED.** No `ssh-keyscan` and no `accept-new`
  survives in any shipped script (grepped the whole tree excluding `.agents/docs` and
  `web/dist`): `host-setup.sh` no longer clones at all, prints the
  `api.github.com/meta` seed over TLS plus a cross-check against GitHub's published
  fingerprints page, and the manual clone now uses `StrictHostKeyChecking=yes`. The
  empty-`host`/`https://`-origin and `|| true` fail-open sub-cases die with the code
  that had them. The updater comment at `:93-98` no longer claims the window is closed
  by keyscan and instead names it as TOFU. The only surviving keyscan instruction is
  the spec file above (finding 4). `DECISIONS.md:463` (`accept-new`) is the *guest-side*
  clone, out of scope and unchanged.
- **Prior #4, `update.env` sourced as root — CLOSED, ordering correct.** The file is
  never sourced; `REQUIRE_SIGNED` defaults to 1; ownership (`stat -c %u`) and
  group/world-writability (`find … -perm /022`) are asserted **before** the `grep`, and
  both take the `fail` path. Since nothing from the file is executed, `log()` cannot be
  redefined and the `WARNING: signature checking DISABLED` line at `:173` is
  unsilenceable — it is emitted from the main script body before any deploy. Two
  bonuses checked: a *symlinked* `update.env` is refused (find uses lstat, so the
  link's own 0777 mode trips `-perm /022`), and non-`0` values, `export …=0` and
  `# …=0` all leave `REQUIRE_SIGNED=1`, i.e. every mis-parse fails toward signing.
- **Prior #5, `deploy/.env` ownership — CLOSED as specified**, modulo the mode gap in
  finding 3. Assertion sits with the `.git` check, before `git()` is defined with
  `-c safe.directory` and before anything is fetched or built.
- **Prior closing note, `maxCheckBytes` dead code — CLOSED, no DoS widening.**
  `csrfGuard` now grants `/format` and `/check` the same 8 MiB `MaxBytesReader` as
  `/file`, so `check.go:334` and `format.go:73` reach their 413 for a 5–8 MiB body.
  Surface assessed rather than assumed: `authGuard` wraps `csrfGuard`
  (`server.go:409`), so an unauthenticated request never reaches the reader; the
  routes are `POST /api/projects/{project}/{check,format}` resolved through
  `gitContext` → `getProjectFor` → the caller's own store; the 413 fires **before**
  any exec; the exec keeps its 30s context and 2 MiB output cap. Peak per-request
  memory rises from ~1 MiB to ~8 MiB of body plus the decoded string, against an
  existing `/file` at 8 MiB and `/upload` at 512 MiB for the same authenticated
  caller — no new capability, no new concurrency (unbounded before and after). The
  suffix match cannot capture another handler: only these two routes end in
  `/check`/`/format`, and a non-matching path 404s. One knock-on worth recording: the
  unremediated stuck-`head -c` defect (prior finding 6, accepted) can now be driven
  with a 5 MiB payload instead of ~1 MiB — still inside the caller's own
  CPU/memory-capped VM, still self-inflicted only.

## Checked

**Secrets** — Clean. The delta adds no credential material and no `.env`. The new
`fail` strings name paths only (`/etc/styrman/update.env`, `${DEPLOY_DIR}/deploy/.env`)
and never their contents; `grep -qE` on `update.env` is quiet, so no line of that file
can reach journald. `host-setup.sh` prints commands, not values. `verify-tag` output
goes to the journal and carries a key fingerprint/principal — public data.

**Injection** — No new sink. The one interpolation asked about, `-c
gpg.ssh.allowedSignersFile="${signers}"`, is not abusable: it is a single quoted argv
element, and I tested that a newline inside a `-c` value cannot inject a second config
key (`git -c gpg.ssh.allowedSignersFile=$'/tmp/x\n[core]\npager=echo INJECTED' config
--get core.pager` → exit 1, unset; the newline stays inside the value). The value's
source is root's own gitconfig, so writing it already implies root. `verify_tag`'s
`${tag}` comes from the same anchored `^v[0-9]+\.[0-9]+\.[0-9]+$` filter as before, and
`git cat-file -t` runs before `verify-tag`. `shellcheck -S warning` clean on both
scripts. Note in passing (pre-existing, unchanged): `CURRENT` derived from
`git describe --tags --exact-match` on first run skips the tag-shape regex that the
state-file path applies.

**Authz/authn** — Unchanged. `internal/httpapi/auth.go` is untouched by the delta; the
only server change is a body-size limit inside `csrfGuard`, which sits *inside*
`authGuard`, so ordering still rejects unauthenticated and cross-origin requests before
a byte is read. No route added, moved, or exempted.

**Crypto** — Nothing hand-rolled; the delta adds no primitive. It changes *policy* over
vetted tooling (git/gpg/ssh-keygen signature verification, OpenSSH host-key pinning),
and finding 1 is precisely that the policy does not bind for one of the two signature
formats git accepts. A third format exists and is untested here: an x509/gpgsm-signed
tag would dispatch to `gpg.x509.program`, also outside the allowlist — another reason
to assert the signature format rather than configure a preference.

**Dependencies** — No dependency change in this delta: `go.mod`, `go.sum`,
`web/package.json` and `web/package-lock.json` are untouched (`git diff --stat` empty
for all four). Re-ran both scanners anyway on the committed tree: `npm audit` in
`web/` → **found 0 vulnerabilities**; `govulncheck ./...` (via
`go run golang.org/x/vuln/cmd/govulncheck@latest`, not installed locally) → **2 called
vulnerabilities from 1 module**, the pre-existing
`github.com/docker/docker@v28.5.2+incompatible` pair already recorded in `TODO.md:53`,
plus 3 uncalled — identical to the `6b41979` run, nothing new. `go build ./...` clean,
`go test ./internal/httpapi/` passes.
