# Security review — project-create provisioning (credential path)   (2026-09-02)

Scope: 503fc8a..working-tree
Diff: `git diff HEAD -- internal web/src` (uncommitted; `internal/httpapi/provision_test.go` is untracked and included)
Note: the tree was actively edited during this review. Findings are against the
state at 2026-09-02T16:09 CEST, `internal/httpapi/server.go` md5
`ed32a833275e66fefdb197ed26b5e254` (which already contains the `redactSecrets`
rewrite that superseded the positional `redactURL` in `guestErrMessage`).
Line numbers below are from that snapshot; re-locate by symbol if the file moved.

## Verdict: FINDINGS

Nothing in this diff is remotely exploitable, and the parts most likely to be
wrong are right: the shell interpolation is properly quoted and regex-gated, the
`provisionError` channel is strictly owner-scoped, and the hostname change
touches no authz, network, ingress, or broker-resolution decision. The worst
problem is a **credential-lifetime** one, not an injection one: `waitForBroker`'s
login-shell probe is now the **first login shell a new project ever runs**, so it
materializes the user's ssh private key, gcloud ADC and claude/agy tokens into the
per-project home roughly one second after `POST /api/projects` — where previously
nothing landed until the user opened a terminal. A `DELETE` without `wipe=true`
removes the project record but leaves `ProjectHome` on disk, and there is no
orphan sweeper and no remaining UI path to that directory once the record is gone.
The realistic sequence — create a clone project, clone fails, delete it without
ticking wipe — now reliably strands a private key and two OAuth credentials on the
host. Second: the new `redactSecrets` regex splits userinfo at the **first** `@`
while git/curl/`net/url` split at the **last** `@` before the path, so a
credential containing a literal `@` (which `cloneURLRe` permits) is partially
emitted into the API response.

## Findings

- **[medium / confidence: high]** `internal/httpapi/server.go:604` (`waitForBroker`
  call in `provisionRepo`) + `internal/httpapi/server.go:988-992` (wipe-only
  cleanup) — **full credential set is now materialized at create time into a home
  that a normal delete orphans.** The probe runs `bash -lc` (a LOGIN shell), which
  sources `/etc/profile.d/zz-styrman-credentials.sh` and writes the broker's
  file-tier credentials into the project home: `~/.ssh/id_ed25519`,
  `~/.config/gcloud/application_default_credentials.json`,
  `~/.claude/.credentials.json` (`internal/httpapi/credharvest.go:66-105`;
  observed live as three back-to-back `served env/file credentials count:7` pairs
  per create in `docker logs styrman-server`). `image/entrypoint.sh` runs the
  harness with plain `bash script`, never `bash -l` — its own comment at line 131
  says the token "is materialized by a login shell that has not run yet" — so
  before this diff a created-then-deleted project left a clean home. Now the
  window opens at T+1s for every clone/GitHub project. `deleteProject` only
  removes `ProjectHome`/`ProjectWorkspace` under `if wipe`
  (`internal/httpapi/server.go:986`), then removes the record, and nothing else in
  the tree ever deletes a project home (`grep ProjectHome` → only explorer reads
  and the wipe branch). Result: unreferenced secrets at rest with no UI path to
  remove them. **Remediation:** on a non-wipe delete, unconditionally remove the
  credential-bearing paths under `ProjectHome` (`.ssh/`, `.config/gcloud/`,
  `.claude/`, `.gemini/`) even when the workspace is preserved — the broker is the
  single live copy (ADR-030/032), so a materialized copy has no durability value;
  or add an orphan-home sweep keyed on "no project record owns this UUID".

- **[medium / confidence: medium]** `internal/httpapi/server.go:765-769`
  (`userinfoRe` / `redactSecrets`) — **userinfo redaction splits on the first `@`,
  leaking the tail of any credential that contains an `@`.** The pattern is
  `(?i)\b(https?|ssh|git)://[^\s/@]+@`; `[^\s/@]+` stops at the first `@`, but
  RFC 3986 and every real parser take userinfo to the **last** `@` before the
  path. Verified: `net/url.Parse("https://user:AAAA@BBBB...@github.com/o/r.git")`
  yields host `github.com`, password `AAAA@BBBB...` — while `redactSecrets` on the
  same string returns `https://***@BBBBBBBBBBBB@github.com/o/r.git`, emitting the
  post-`@` half of the secret verbatim into `provisionError` and thus into
  `GET /api/projects`. `cloneURLRe` (`server.go:762`) explicitly allows `@` inside
  the URL body, so the user can supply such a URL. Two smaller holes in the same
  regex: the leading `\b` means a guest-controlled prefix (`Xhttps://u:t@h`) defeats
  the match entirely, and a `/` inside the userinfo also defeats it (that one is
  benign — a `/` terminates the authority, so git would not treat it as a
  credential anyway). **Mitigating:** the guest ships git 2.47.3, which strips
  userinfo from its own diagnostics — confirmed empirically, `fatal: unable to
  access 'https://nonexistent.invalid/o/r.git/'` with the userinfo already gone —
  so the leak needs output from another tool or a deliberately hostile guest.
  **Remediation:** stop pattern-matching. Tokenize on whitespace/quotes and run
  each candidate through `net/url.Parse`; if `u.User != nil`, set it to nil and
  re-serialize. That is exact, handles `@`, case, and the `\b` edge in one step.

- **[medium / confidence: high]** `deploy/vulncheck.sh:38` — **the repo's own
  vulnerability gate cannot run on a checkout that has no `deploy/.env`.** The
  hardening mount `-v /dev/null:/src/deploy/.env:ro` needs runc to create the
  mountpoint inside a source tree mounted `:ro`, which fails:
  `make mountpoint "/src/deploy/.env": openat .env: read-only file system`. On
  this machine `deploy/.env` does not exist, so both modules fail and the script
  exits 125. It fails **closed** (125 is neither 0 nor 3, and the `case` at line
  46-50 propagates it), which is correct — but any caller that only tests for
  `!= 3` reads a broken scan as clean, and today the documented security-row tool
  simply does not execute. **Remediation:** only add the shadow mount when
  `[ -f "${ROOT}/deploy/.env" ]`, or mount a scratch copy of the tree read-write;
  and have `/preflight` assert exit ∈ {0,3} rather than `!= 3`.

- **[low / confidence: high]** `internal/httpapi/server.go:624,637,643` +
  `:949` — **`provisionErrs` entries are resurrected after a delete and then never
  cleared.** `deleteProject` clears the key once, at request time. `provisionRepo`
  calls `setProvisionError` with no `deletingState` re-check, and a clone can run
  for minutes after the DELETE landed (the only in-flight check is at `:864`,
  before `EnsureRunning`). The write therefore re-creates a `user/projectID` entry
  for a project whose record is gone — no future DELETE will ever clear it, so the
  map grows without bound under repeated authenticated create→delete cycles.
  **Remediation:** re-check `deletingState` inside `setProvisionError` (or at the
  top of each `provisionRepo` branch) and drop the write when a destroy is in
  flight or the record no longer exists.

- **[low / confidence: high]** `internal/httpapi/server.go:690-712`
  (`guestErrMessage`) — **sanitization is narrower than its own comment claims.**
  `strings.Map` drops only C0 (`r < 0x20`) and DEL; C1 controls (U+0080–U+009F),
  bidi overrides (U+202E/U+202D) and zero-width joiners survive in text the comment
  itself describes as guest-controlled, and are rendered into a `title=` tooltip
  (`web/src/components/Sidebar.tsx:283`). No XSS — React escapes the attribute —
  but a hostile guest can spoof what the tooltip appears to say. Separately,
  `msg[:300]` slices **bytes**, not runes, and can split a multi-byte sequence;
  `encoding/json` substitutes U+FFFD so the response stays valid, but the cut is
  not rune-safe. **Remediation:** also drop `unicode.IsControl` and the
  `Cf` (format) category, and truncate with a rune-aware cut
  (`for !utf8.ValidString(msg) { msg = msg[:len(msg)-1] }` or a rune slice).

- **[low / confidence: high]** `internal/httpapi/server.go:762` (`cloneURLRe`) —
  **the API accepts credential-bearing clone URLs it never needs, and git persists
  them.** `cloneURLRe` permits `https://user:token@host/...`, and `git clone`
  writes the remote verbatim into `<workspace>/.git/config` — confirmed locally:
  `url = https://carol:PLACEHOLDER_TOKEN@github.com/o/r.git`. That workspace is
  bind-mounted from the host (`ProjectWorkspace`) and survives a non-wipe delete,
  so a token pasted once lives on in plaintext with no redaction anywhere on that
  path. Since ADR-030's broker + `styrman-git-credential` already supply the
  credential, a userinfo-bearing clone URL is never required. **Remediation:**
  reject userinfo at the API boundary (`net/url.Parse` → `u.User != nil` → 400
  with "credentials are supplied by styrman; use a bare URL"). This deletes the
  whole finding class, including the redaction gap above.

## Checked

- **Secrets** — No credentials, tokens, or keys added in the diff; no `.env` or
  credential-shaped file is tracked (`find . -name .env` → none; `deploy/.env` is
  absent and gitignored). The diff **improves** logging: `"output", out` was
  removed from the `clone failed` / `initial commit failed` / `github repo create
  failed` warn lines, so raw guest output no longer reaches `docker logs
  styrman-server`; the surviving `"url", redactURL(ps.cloneURL)` is redacted.
  Remaining secret exposure is lifecycle and redaction-completeness, findings 1, 2
  and 6 above.
- **Injection** — Clean, and this was the main thing to get right in a move from
  bare exec to `bash -lc`. `req.CloneURL` is gated by
  `cloneURLRe = ^(https://|ssh://|git@)[\w.@:/~+-]+$` (no quote, space, `;`, `$`,
  backtick, backslash, newline) *and* passed through `shellQuote`, *and* still
  preceded by `--`, so neither shell metacharacters nor a leading-dash option
  reach git. `p.Name` is `^[a-z][a-z0-9-]{0,30}$` via `project.ValidateName`
  (enforced in `Store.Add`, with `Dir = Name` re-validated on load) and also
  `shellQuote`d for `gh repo create`. `ps.github` is interpolated as `"--"+ps.github`
  but is unreachable unless `wantsGitHub()` already proved it is exactly
  `private` or `public`. `core.sshCommand='ssh -o StrictHostKeyChecking=accept-new'`
  is a source literal in single quotes with no interpolation point. The guest
  working directory reaches Docker as `ExecSpec.WorkDir`, never through a shell.
  `brokerProbeCmd` is a constant. Request bodies are bounded by the csrf guard's
  1 MiB `MaxBytesReader` (`server.go:503`).
- **Authz/authn** — Clean, no cross-tenant reach. `provisionErrs` is keyed
  `user + "/" + projectID`; the only two readers (`listProjects:736-742`,
  `getProject:921-926`) pass `u.Username` from `identity.FromContext` via
  `who`/`getProjectFor`, and iterate the caller's own `sp.projects` store. The
  only writers pass the `u.Username` captured in the create request's closure.
  `projectView` (the sole carrier of `ProvisionError`) is constructed nowhere else
  — the share/preview path (`shares.go`, `proxy.go`) proxies ports and never emits
  it. No new endpoint, no middleware change, no client-supplied ID trusted.
- **Crypto** — Nothing hand-rolled; no auth, session, JWT/OIDC, password-hashing
  or crypto primitive is added or modified by this diff. TOFU is unchanged and not
  widened: `StrictHostKeyChecking=accept-new` appears exactly once in the tree
  (`server.go:618`, plus its assertion in `provision_test.go:146`), scoped by
  `git -c core.sshCommand` to that single clone invocation; no global
  `GIT_SSH_COMMAND`, `~/.ssh/config`, or `UserKnownHostsFile` override exists.
- **Hostname change** — No security decision keys on the `styrman-` guest-hostname
  prefix. `ResolveProjectByIP` resolves by bridge subnet (`sdb-<uuid>`), container
  name (`config.ContainerName` = `"styrman-"+projectID`, untouched) and the
  `styrman.user` label. The ingress reserved-hostname guard
  (`deploy/caddy/ingress/labels.go:71`) tests `strings.HasPrefix(containerName,
  "styrman-")` — the Docker **container** name, not the guest UTS hostname.
  `Adopt` filters on the managed label. `guestHostname` trims the trailing dash
  `ValidateName` would otherwise allow, keeping the value a legal DNS label, and
  its empty fallback is unreachable given `ValidateName`.
- **Dependencies** — No dependency change: `git diff HEAD -- go.mod go.sum
  web/package.json web/package-lock.json` is empty; no new import in the diff.
  `npm audit` (web, with and without dev) → **0 vulnerabilities**.
  `deploy/vulncheck.sh` → **exit 125, scan did not run** (finding 3). Run manually
  in the same pinned builder image (`golang:1.25.13-trixie`, govulncheck v1.7.0,
  `GOTOOLCHAIN=local`): **2 called vulnerabilities, both
  `github.com/docker/docker@v28.5.2+incompatible` — GO-2026-4887 and GO-2026-4883,
  no fix available**, the documented accept-per-release pair; standard library
  clean. For contrast, the host's `go1.25.0` govulncheck reports 31 — 29 of them
  stdlib issues the shipped `1.25.13` binary does not have; the pinned-image number
  is the honest one.
- **Build/test state** — `go build ./...` and `go vet` on the changed packages
  pass; `go test ./internal/httpapi ./internal/orchestrator ./internal/project`
  passes. Verification of this change is the verifier's row, not this one.
