# Security review — PREFLIGHT gate, editor check + tag-gated updater (committed tree)   (2026-09-01)

Scope: `git diff v1.15.0..HEAD` (HEAD = `6b41979`, 85 files). Blast radius followed outward
through `deploy/update/**` → `deploy/host-setup.sh` 8c → `deploy/docker-compose.yml` →
`deploy/Dockerfile.server` / `image/Dockerfile`; `internal/httpapi/{check,format,server}.go` →
`gitContext` → `getProjectFor` → `csrfGuard`/`authGuard` → `internal/project/store.go` →
`internal/config.ContainerName`; `internal/orchestrator/exec.go` → `orchestrator.go` container
resources; `web/src/components/{CodeEditor,FileEditor}.tsx` → `@codemirror/lint`.
Second pass over `.agents/docs/reviews/2026-09-01-security-editor-check-and-autoupdate.md`,
whose findings this commit claims to remediate. Out of scope by ADR-046: `image/harness/**`.

## Verdict: FINDINGS — none block tagging the release; three block *enabling the updater timer*

The remediations mostly hold, and two of them hold completely: `safeSuffix()` closes the
command-injection surface with no residue, and the `styrman.version` label is genuinely
unspoofable by a project VM. But the headline control of this release — "an unverified tag
never becomes root code" — is not airtight in three places, and all three are in code added
by this commit.

Worst first: **`git verify-tag` does not pin the signer.** Verified empirically on this host —
a good signature made by an *untrusted, unknown-provenance* key that merely exists in root's
GPG keyring returns exit 0, and git does not require the signer to match the tagger or any
allowlist. The prior review's finding was "the trust set is anyone who can push a tag, not the
operator"; after remediation the trust set is "anyone who can push a tag **and** get one public
key into root's keyring", which is narrower but is still not "the operator". Right behind it,
`deploy/host-setup.sh` bootstraps the pinned `known_hosts` with `ssh-keyscan` — unauthenticated
TOFU — and the clone that follows still runs `StrictHostKeyChecking=accept-new` writing into
that same file, so the first-contact window the pin was added to close has been *moved to
provisioning time and made permanent*, not closed. And the **rollback path checks out and
builds `${PREVIOUS}` with no integrity check at all**, on a clone whose fetch is `--tags --force`
and therefore accepts a re-pointed old tag.

Everything else asked for checks out. `POST /api/projects/{project}/check` is correctly
authorized, bounded and timed out; the `CloseWrite()` removal leaves a real but
self-inflicted-only stuck-process/temp-file defect inside the caller's own VM; `wc -c` is not
gameable; `git clean -d` (no `-x`) provably preserves `deploy/.env`; `hadolint` is pinned with
a sha256; no secrets; `npm audit` clean; `govulncheck` reports only the 2 pre-existing docker
findings already in TODO.md.

## Findings

- **[medium / confidence: high] NOT ship-blocking; fix before `systemctl enable --now styrman-update.timer`**
  `deploy/update/styrman-update:127` — `git verify-tag "${LATEST}"` proves *"signed by some key
  root's git/gpg will accept"*, not *"signed by the operator"*. Confirmed by experiment on this
  host: a tag whose tagger is `t@t` but whose OpenPGP signature was made by an unrelated key
  imported into the keyring with **no ownertrust** verifies with exit 0 (`Good signature from
  "Attacker Key <evil@example.com>" [unknown]` + `WARNING: This key is not certified…`). git's
  `check_signature` treats good-but-untrusted as success, and nothing compares the signer to the
  tagger or to an allowlist. So the effective trust set is every public key ever imported into
  root's keyring — including keys pulled in for unrelated reasons (`gpg --recv-keys`, verifying
  a download, a colleague's key). The SSH path is materially better — it fails closed when
  unconfigured (verified: `error: gpg.ssh.allowedSignersFile needs to be configured and exist`)
  and enforces a *key* allowlist — but the script does not supply that config: it depends on
  root's ambient `--global` gitconfig, which the script neither sets nor asserts, and which
  `/etc/styrman/update.env` (finding 4) could change. Remediation: pin the signer explicitly in
  the `git()` invocation rather than trusting ambient config —
  `git -c gpg.ssh.allowedSignersFile=/etc/styrman/allowed_signers verify-tag "${LATEST}"` for
  the SSH path, and for the OpenPGP path assert the fingerprint:
  `git verify-tag --raw "${LATEST}" 2>&1 | grep -q "^\[GNUPG:\] VALIDSIG ${STYRMAN_RELEASE_FPR}"`
  with the fingerprint stored root-owned in `/etc/styrman/`. Also correct the comment at
  `:112-117`, which currently claims the check narrows the trust set to "the operator".

- **[medium / confidence: high] NOT ship-blocking on an already-provisioned host; the automation must not be shipped as "pinning"**
  `deploy/host-setup.sh:353-357` — the pinned `known_hosts` that `styrman-update`'s
  `StrictHostKeyChecking=yes` depends on is auto-seeded with `ssh-keyscan "${host}"`, which is
  itself trust-on-first-use with no authentication whatsoever. Because `StrictHostKeyChecking=yes`
  then *never re-prompts*, a host key captured from an on-path attacker at provisioning time is
  pinned permanently and silently, and every subsequent root fetch — whose output is built and
  run as root with the docker socket — trusts it. This is the same window the prior review asked
  to close; it has been narrowed from "every first root fetch" to "the one host-setup run", not
  eliminated, and the prior review explicitly said *"not from an unverified `ssh-keyscan`"*.
  Two aggravations, both in the committed code: (a) it fails open — `2>/dev/null || true` hides
  a keyscan failure, and the `git clone` on the next line still passes
  `StrictHostKeyChecking=accept-new` against the *same* `UserKnownHostsFile`, so an empty
  known_hosts is then populated by TOFU anyway; (b) an `https://` origin produces no `@`, so
  `host` is empty, the seed is skipped, and the file stays empty. Remediation: seed from an
  authenticated source — `curl -fsS https://api.github.com/meta | jq -r '.ssh_keys[]' |
  sed 's|^|github.com |' > /etc/styrman/known_hosts` (TLS-authenticated), fail loudly rather
  than `|| true` if it cannot, and change the clone's `accept-new` to `yes` once seeded.
  The `sed` host extraction is *not* itself an escalation and was considered separately: `host`
  is quoted so there is no argv injection, and `ORIGIN` comes from the operator's own working
  tree — anyone who can rewrite that `remote.origin.url` already controls the code being
  deployed, so it grants nothing new.

- **[medium / confidence: high] NOT ship-blocking; fix before enabling the timer**
  `deploy/update/styrman-update:263` — the rollback calls `deploy "${PREVIOUS}"`, which does
  `git checkout --detach --force "${PREVIOUS}"` and then `docker build` + `docker compose up`
  **as root, with no `git cat-file -t` and no `git verify-tag`**. The integrity block at
  `:122-133` gates `LATEST` only. Combined with `git fetch --tags --force` at `:102`, which
  deliberately accepts a *re-pointed* tag, this is a complete bypass of the release's headline
  control: an attacker with tag-push rights re-points the currently-deployed version tag and
  then waits for any legitimate release to fail its health check or its build — a condition
  they do not even need to cause, since `HEALTH_TIMEOUT=120` and a slow first boot is enough.
  The bypassed control is the one that exists precisely because tag-push rights are the assumed
  attacker capability. Remediation: hoist `:122-133` into `verify_tag_or_die() { … }` and call
  it from `deploy()` for *every* tag, `LATEST` and `PREVIOUS` alike; a rollback target that
  cannot be verified should be a `fail`, not a build.

- **[low / confidence: high] NOT ship-blocking**
  `deploy/update/styrman-update:121` — `[ -f /etc/styrman/update.env ] && . /etc/styrman/update.env`
  dot-sources an arbitrary shell file **as root, with no ownership or mode assertion**, three
  lines after the script correctly asserts `stat -c %u` on `${DEPLOY_DIR}/.git` for exactly this
  class of risk. It is sourced *after* `git()`, `STATE`, `FAILED`, `RUNTIME_IMAGE` and
  `HEALTH_TIMEOUT` are established and *before* the integrity check, so it can override the
  runtime image tag, the state paths, the health timeout, redefine the `git()` wrapper, or just
  run commands. This is also the answer to "is there a path that skips verification silently":
  yes — redefining `log() { :; }` in that file suppresses the
  `WARNING: signature checking DISABLED` line entirely, so the opt-out is only *loud* as long as
  the file is honest. In the intended deployment `/etc/styrman` is `install -d -m 700` root-owned
  so this needs root already; it is a missing invariant, not a live hole. Remediation: do not
  source it. Parse the one supported key —
  `REQ="$(sed -n 's/^STYRMAN_UPDATE_REQUIRE_SIGNED=\([01]\)$/\1/p' /etc/styrman/update.env | tail -1)"` —
  or at minimum assert `[ "$(stat -c '%u %a' /etc/styrman/update.env)" = "0 600" ]` first.
  (Checked and clear: the `[ -f … ] && . …` form does *not* trip `set -e` when the file is
  absent — a failing command before `&&` is exempt from errexit.)

- **[low / confidence: high] NOT ship-blocking**
  `deploy/update/styrman-update:79` — the new ownership assertion covers `${DEPLOY_DIR}/.git`
  but not `${DEPLOY_DIR}/deploy/.env`, and that file is *gitignored*, so — as verified below —
  it survives both `git checkout --force` and `git clean -d`. It is the one file in the deploy
  clone that no git operation ever restores or removes, and it supplies
  `STYRMAN_ALLOWED_EMAILS` (the auth allowlist), `STYRMAN_INGRESS_TOKEN`, `STYRMAN_UID/GID` and
  `DOCKER_GID` to the entire stack on the next `docker compose up`. Non-root write access to
  `/opt/styrman/deploy` is therefore a full authentication bypass that nothing in the updater
  catches, and it persists across every subsequent release. Remediation: extend the existing
  assertion — `[ "$(stat -c '%u %a' "${DEPLOY_DIR}/deploy/.env")" = "0 600" ] || fail …` —
  next to the `.git` check, before anything is built.

- **[low / confidence: high] NOT ship-blocking — no boundary crossed**
  `internal/orchestrator/exec.go:105` with `internal/httpapi/check.go:408` — removing
  `CloseWrite()` makes the guest command's *termination* depend on receiving exactly `N` bytes.
  Docker does not kill an exec'd process when the client detaches, so on any short/failed stdin
  write (or a checker that wedges) the handler returns after its 30s deadline while the
  `bash -c` in the guest stays blocked in `head -c N` **forever**: its `trap … EXIT` never fires,
  and the `mktemp -p .` file it created is left in the project workspace root — which is
  persistent and inside the restic backup set. Assessed for blast radius and it does not cross
  one: the stuck process is in the caller's own microVM, which is `NanoCPUs`/`Memory`-capped, and
  the caller can already spawn processes there from their own terminal. The host-side leak is
  bounded — the copy goroutine is abandoned on `ctx.Done()`, `att.Close()` unblocks the reader,
  and the stdin writer unblocks on the closed conn. Remediation (robustness, not a boundary fix):
  give the guest script its own watchdog so it cannot outlive the request —
  `timeout 25 bash -c '…'` in `ExecSpec.Cmd`, or a `head -c` with a read timeout — and consider
  a `PidsLimit` on project containers, which are currently unbounded in process count
  (pre-existing, `orchestrator.go:780`). The `wc -c` assertion itself is **not gameable**: both
  operands derive from the same server-side buffer, it is a truncation detector only, and its
  result feeds a file redirect, never an `eval`.

## Confirmed remediations (each was a prior finding — re-verified in the committed tree)

- **Tag integrity** — `deploy/update/styrman-update:122-133` rejects lightweight tags via
  `git cat-file -t`, runs `git verify-tag`, and `fail`s (exit 1) on either. `--check` mode runs
  *after* the block, so it fails closed too, and the check precedes the "already up to date"
  short-circuit so it runs every tick. The opt-out is root-only and logs. **Holds**, with the
  three qualifications above (no signer pinning, unverified rollback path, sourceable env file).
- **Root SSH** — `styrman-update:93` uses `StrictHostKeyChecking=yes` +
  `UserKnownHostsFile=/etc/styrman/known_hosts`, and `accept-new` is gone from the updater
  entirely. **Holds on the updater side**; the *seeding* of that file is finding 2.
- **Clone ownership** — `styrman-update:79` asserts `stat -c %u` on `.git` = 0 *before* `git()`
  is defined with `-c safe.directory`. Ordering is correct. **Holds**, modulo finding 5.
- **`git clean -d --force` (no `-x`)** — verified empirically in a scratch repo with this
  project's `.gitignore` semantics: `deploy/.env` **survives**, an untracked `planted.go` is
  **removed**. Nothing secret is destroyed and nothing secret is exposed — `.dockerignore`
  excludes `.git`, `**/.env`, `*.env`, `deploy/.env`, `**/node_modules` from the `context: ..`
  server build. The residual persistence gap is now only files matching a `.gitignore` pattern:
  `wsprobe`, `/styrman`, `/screens/` survive `clean` and are *not* in `.dockerignore` — but
  neither Dockerfile executes them, so this is informational. Moving `STATE_DIR` out of the
  clone to `/var/lib/styrman/update` correctly fixes the latch-wipe.
- **State validation** — `styrman-update:138` anchors `CURRENT` to `^v[0-9]+\.[0-9]+\.[0-9]+$`
  before it can reach `git log`/`git checkout` argv, and blanks it with a WARN otherwise;
  `PREVIOUS` derives from `CURRENT`. Argument injection via a leading `-` is closed. **Holds.**
- **`safeSuffix()`** — **complete**, re-verified by exhausting the sinks rather than reading the
  fix. `checkScript` interpolates exactly three things: the sanitized suffix, two `%d` ints from
  `len(body.Content)`, and `c.cmd`; every `c.cmd` in `checkerFor` is a compile-time constant,
  including both arms of the shellcheck/`bash -n` branch and both arms of the `-s bash` shebang
  branch. `formatScript` is identical in shape and applies the same sanitizer at `format.go:85`
  even though `formatInPlace` already allowlists `ext` — correct anti-drift call. The Dockerfile
  basename bypass stays closed because the sanitizer is applied at the *call site*
  (`check.go:353`), after `checkerFor`, so `Dockerfile.$(id)` yields suffix `id`. `body.Path`
  reaches only `filepath.Base`/`filepath.Ext` and is never opened, written, or used as a name.
- **`hadolint` pinned** — `image/Dockerfile:44` pins `2.15.1` and verifies
  `sha256sum -c` **before** `chmod +x`. `shellcheck` comes from apt (distro-signed). The
  unpinned `yq`/`helm`/`tofu` fetches are recorded in `TODO.md:43-51` as accepted risk with the
  rollback-determinism consequence spelled out. **Holds.**

## Checked

**Secrets** — Clean. Scanned the full `v1.15.0..HEAD` diff (excluding `web/dist` and
`package-lock.json`) for credential patterns: every hit is prose in `.agents/docs/**` review
text discussing tokens, or the identifier `focusToken`. No committed `.env`, no keys, no
tokens. `styrman-update` touches `deploy/.env` only in an existence test (`:62`) and never reads
its contents; nothing it writes to journald carries a secret. `web/e2e/run.sh` is dev-only, uses
`mktemp -d` state and a `127.0.0.1` bind, and its `sudo rm -rf "${STATE}"` targets only that
mktemp path. `.gitignore`/`.dockerignore` unchanged and still correct.

**Injection** — See "safeSuffix" above: complete, no residual sink. In the updater, `LATEST` is
constrained by the anchored `grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$'` and `CURRENT`/`PREVIOUS` are
now re-validated against the same anchor. `git()`'s `-c safe.directory` value is a script
constant. The one unvalidated root-side input left is `/etc/styrman/update.env`, filed as
finding 4. `ssh-keyscan "${host}"` is quoted; the `sed` extraction is not an independent
escalation (finding 2). Guest-side, content never touches argv — it goes over stdin to a
`head -c N` file redirect.

**Authz/authn** — `internal/httpapi/auth.go` is unchanged in this diff. The new
`POST /api/projects/{project}/check` is registered inside `RoutesWithAuth` (`server.go:272`),
behind the full `authGuard(authMode, csrfGuard(mux))` chain (`server.go:409`), and resolves via
`gitContext` → `getProjectFor` → `s.who(r)` → the caller's **own** per-user project store — so
it enforces ownership exactly as `/format` does; another user's project ID 404s. Resource bounds
are real: `csrfGuard` clamps the body at **1 MiB** via `MaxBytesReader` (`/check` matches
neither the `/upload` nor `/file` exception), a 30s `context.WithTimeout` derived from
`r.Context()` bounds the exec, `maxExecOutput` caps the reply at 2 MiB, and the UI's linter is
debounced to 1s idle for a single open file. Note for the record: `maxCheckBytes = 5 << 20` at
`check.go:31` is **dead** — the 1 MiB middleware cap always fires first, turning what should be
a 413 into a 400 "path and content required". Cosmetic, not security. A hostile *file* cannot
make a checker consume the VM beyond what its author already can from their own terminal; a
hostile file that wedges a checker is finding 6. **`styrman.version` cannot be influenced by a
project VM** — verified three ways: `deploy/docker-compose.yml` binds `/var/run/docker.sock`
into `styrman-caddy` (ro) and `styrman-server` only, never into project containers;
`config.ContainerName(projectID) = "styrman-" + projectID` where `projectID` is a
**server-generated UUIDv4** validated by `uuidRe` on both `Add` and load (`project/store.go:102,151`),
so the string `styrman-server` is unreachable by construction and does not depend on Docker's
name-uniqueness as a backstop; and `image/Dockerfile` carries no `styrman.version` LABEL at all,
so even a hypothetically renamed container would present an empty label, which
`reports_version()` explicitly rejects (`[ -n "${got}" ]`). The label build-arg flows only from
the root-owned deploy clone.

**Crypto** — Nothing hand-rolled. No auth, session, password-hashing, JWT or OIDC code in the
diff. Cryptographic material in scope is handled by vetted tooling: stock OpenSSH via
`GIT_SSH_COMMAND`, git/gpg/ssh-keygen for tag signature verification, `sha256sum -c` for the
hadolint pin, and the pre-existing untouched `openssl rand -hex 32` in host-setup. The gaps are
in *policy* (which signer is trusted — finding 1) and *bootstrap* (which host key is trusted —
finding 2), not in the primitives.

**Dependencies** — `govulncheck ./...` (via `go run golang.org/x/vuln/cmd/govulncheck@latest`;
not installed locally): **2 called vulnerabilities from 1 module**, both the pre-existing
`github.com/docker/docker@v28.5.2+incompatible` findings (GO-2026-4887, GO-2026-4883) already
recorded in `TODO.md:53` — reachable only through package-`init` edges, `Fixed in: N/A` on the
`+incompatible` module path, **not re-reported as new**. Plus 3 vulnerabilities in required-but-
uncalled modules. `npm audit` in `web/`: **found 0 vulnerabilities**. One new npm dependency,
`@codemirror/lint ^6.9.7` — justified: first-party CodeMirror, the only source of
`linter`/`lintGutter`, no stdlib or existing-dep equivalent. Two new runtime-image tools:
`shellcheck` (apt, distro-signed, no new trust) and `hadolint` (version-pinned + sha256-verified
— the correct pattern, and the one the surrounding `yq`/`helm`/`tofu` lines still lack). No new
Go modules. `go build ./...` clean, `go test ./internal/httpapi/...` passes.

**Frontend** — No new XSS sink. Checker output reaches `@codemirror/lint` as
`Diagnostic.message`, a plain string appended as a text node; `renderMessage` is unused.
`d.line`/`d.col` are clamped to the document before becoming positions (`CodeEditor.tsx:225-231`),
so a checker reporting past EOF cannot throw inside CodeMirror. `checkNote` is a React-escaped
JSX text child. `api.checkFile` sends JSON via the shared `request()` helper; the project id is
a UUID from the router. No `eval`, `new Function`, `innerHTML`, `srcdoc` or `postMessage` in the
changed components; the single `dangerouslySetInnerHTML` (markdown preview) is unchanged in
substance and still fed by `micromark(text, { allowDangerousHtml: false })`.
