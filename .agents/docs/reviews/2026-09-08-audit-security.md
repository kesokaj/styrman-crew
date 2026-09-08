---
owner: security
date: 2026-09-08
---
# Security review — whole-project audit (2026-09-08)

Range reviewed: whole tree at HEAD `414be72` (v1.24.0, ADR-066 shipped). Not a diff —
a standing audit of the surfaces named in the dispatch, in priority order.
Threat model applied: a guest escaping its project, reaching another tenant,
reaching the host, or reaching the operator's credentials.

## Verdict: FINDINGS (no high; 6 medium, 6 low, 3 informational)

Nothing found that lets a project guest escape to another tenant or to the host
through the code paths audited. The tenant-isolation core (`internal/files`
`os.Root` confinement, `projectHomeIsReal`/`realBelow` on every Docker bind
path, the per-user project stores, `previewAllowed`) holds up under the
BUG-002/003/004 regression questions, and I could not construct a path that
regresses any of them. The auth guard fails closed on every branch and
`config.Load` refuses to start `STYRMAN_AUTH=header` without an ingress token
(`internal/config/config.go:224`), which is what makes header-based identity
trustworthy at all.

The worst concrete defect found is a **cross-tenant denial of service**: the
`O_NONBLOCK` discipline that `internal/files.openReg` established specifically
to stop "a guest-planted FIFO/device ... wedging the goroutine forever (a
cross-tenant DoS)" is **not applied at five other opens of the same
guest-writable trees** — all three opens in `internal/history`, plus
`files.List`, `files.ListNames` and `files.Save`. A prompt-injected agent can
plant a FIFO in its own project home and wedge a server goroutine *and its OS
thread* on the owner's next routine Files/History request. Details below; it is
availability-only, which is why it is medium and not high, but the server is a
single process shared by every tenant.

The other thing worth putting on the record is a **standing, unfixable
dependency exposure**: `govulncheck` reports both modules importing
`github.com/docker/docker` at versions carrying GO-2026-4887 and GO-2026-4883
with **`Fixed in: N/A`** — no upgrade exists. Both are daemon-side Moby defects
reached only through `init` traces of the client SDK, so the practical risk here
is low, but the two modules are also *pinned at different versions*
(`v28.5.2` root vs `v27.5.1` ingress), and the ingress is the internet-facing,
`network_mode: host`, docker-socket-holding process. That divergence is a real
finding on its own.

The credential-harvest path is the most interesting same-user risk: a
prompt-injected agent in one project can shape what `harvestCredentials` reads
out of its guest, and everything read is then broadcast to *every* project of
that user. That is inside one tenant, not across tenants, which is why it is
medium and not high.

## Findings

### Medium

- **[medium / confidence: high]** `internal/history/history.go:48,144,215` and
  `internal/files/files.go:181,416,450` — **guest-plantable FIFO wedges a server
  goroutine and pins an OS thread (cross-tenant DoS)**. `files.openReg`
  (`files.go:128`) and `openRegWrite` (`files.go:284`) pass
  `syscall.O_NONBLOCK` and say why in their comments — "a guest-planted
  FIFO/device returns immediately instead of wedging the goroutine forever (a
  cross-tenant DoS)". Six sibling opens on the *same* guest-writable trees do
  not:
  - `history.List:48` opens `~/.styrman/logs/<project>` as a directory,
  - `history.searchOne:144` opens a transcript (its `IsRegular` check is
    **after** the blocking open, so it never runs),
  - `history.Read:215` opens `<id>.log` with no regular-file check at all,
  - `files.List:181` opens the caller-supplied `?path=` directory,
  - `files.ListNames:416` (reached from `setup.go:179` against the base home),
  - `files.Save:450` opens `O_WRONLY|O_CREATE|O_TRUNC` (reached from
    `setup.go:127`, writing the setup-done marker into the base home).

  Every one of those paths is inside a tree bind-mounted into a microVM where
  the guest is root (`ProjectHome` / `UserHome`), so the untrusted agent can
  create a FIFO at any of those names. `os.Root.Open` is `OpenFile(name,
  O_RDONLY, 0)` — no `O_NONBLOCK` — and `open(2)` on a FIFO with no peer blocks
  in the kernel indefinitely. In Go that blocks the goroutine *and* pins the OS
  thread it is on. Client disconnect does not unblock it: `readHistory`
  (`explorer.go:399`) sets no timeout, and `searchHistory`'s context is only
  checked *between* files (`history.go:133`), never inside the open. The trigger
  is not an unusual request — the Files panel and History panel make these calls
  routinely, so the agent plants and simply waits. Repeated, this walks the Go
  runtime toward its 10,000-thread limit, at which point the process dies and
  **every tenant's** terminals go with it. Remediation: route all six through
  `openReg`/`openRegWrite`, or add `syscall.O_NONBLOCK` and an `IsRegular` /
  `IsDir` check *before* any blocking use, exactly as `openReg` already does.

- **[medium / confidence: medium]** `deploy/update/styrman-update:158-181` —
  the updater asserts that `${DEPLOY_DIR}/.git` is root-owned and not
  group/world-writable (line 166, 171) and that `${DEPLOY_DIR}/deploy/.env` is
  root-owned and mode-restricted (line 176, 180), but it **never asserts the
  worktree root `${DEPLOY_DIR}` itself**. Its own comment at line 163 states the
  threat exactly — "`-c safe.directory` on a tree someone else can write would
  let them choose the code this script builds as root" — and then checks only
  the `.git` directory. `git`'s ownership guard covers the repository, not the
  checked-out files that `docker build` subsequently reads. Two concrete
  residues if `/opt/styrman` is writable by a non-root uid: (a) a window between
  `git checkout --force` / `git clean -d` (line 547-552) and `docker build`
  (line 556) in which tracked files can be modified, and (b) `git clean -d`
  without `-x` deliberately preserves **gitignored** files, so a planted
  `*.key`, `*.pem`, `*.tfvars`, `*.tfstate`, `/styrman`, `wsprobe` or `/screens/`
  survives every checkout into the server build context (`.dockerignore`
  excludes `.git`, `.agents`, `.claude`, `**/node_modules` and `*.env` but none
  of those). Impact of (b) is limited — the builder stage is discarded and only
  `/out/styrman` reaches the final image — so (a) is the real one. `host-setup.sh:311`
  instructs `chown -R root:root ${STYRMAN_DEPLOY_DIR}`, which is why this is
  medium/medium rather than higher: it is an unasserted precondition, not a
  known-broken one. Remediation: add `trusted_dir "${DEPLOY_DIR}"` (the helper
  already exists at line 369) to the checks at line 166, and consider `git clean
  -dx --force` with an explicit `-e deploy/.env`.

- **[medium / confidence: high]** `go.mod:7` + `deploy/caddy/ingress/go.mod:6` —
  the two Go modules pin *different* `github.com/docker/docker` versions
  (`v28.5.2+incompatible` vs `v27.5.1+incompatible`), and the older one is in the
  **internet-facing** component that runs `network_mode: host` with
  `/var/run/docker.sock` mounted (`deploy/docker-compose.yml`, whose own comment
  says "treat any RCE in it as host root"). `deploy/vulncheck.sh` scans both, so
  the divergence is visible, but nothing enforces convergence. Remediation: pin
  both modules to the same, newest `docker/docker`, and add a check to
  `vulncheck.sh` (or `verify-commit.sh`) that fails when the two `go.mod` files
  disagree on a shared dependency's version.

- **[medium / confidence: high]** `internal/httpapi/credharvest.go:26-142` —
  `harvestCredentials` runs `gh auth token`, `sed … ~/.npmrc`, and
  `base64 -w0 <path>` **inside an untrusted guest** and stores whatever comes
  back as the user's canonical broker credential, which
  `pushCredsToRunningProjects` (`credharvest.go:217`) then materializes into
  *every* running project of that user. An agent that has been prompt-injected
  in project A can write a chosen `~/.npmrc`, a chosen
  `~/.claude/.credentials.json`, or shadow `gh` on `PATH`, and if the operator
  then clicks "Import from project" on A, the attacker's values become the
  user-wide credentials delivered to B, C and D — including
  `STYRMAN_GIT_NAME`/`STYRMAN_GIT_EMAIL`, which the login snippet writes into
  `git config --global` in every project. No tenant boundary is crossed (all
  projects belong to one user), and the operator has to initiate the import, but
  it is a real "one compromised agent poisons every sibling project" path.
  Remediation: at minimum, make the import UI name the project it is importing
  from and warn that its contents become user-wide; better, diff against the
  currently stored value and require confirmation when a harvested credential
  *changes*.

- **[medium / confidence: medium]** `internal/httpapi/server.go:846-895` and
  `internal/httpapi/credentials.go:178-212` — several handlers spawn background
  work bound only by a wall-clock timeout, with **no per-user rate limit**:
  `createProject` starts a 3-minute microVM boot goroutine per POST,
  `importCredentials` runs a full harvest (six-plus guest execs) per POST, and
  `pushCredsToRunningProjects` execs a login shell in every running project. Only
  the credential push is single-flighted (`tryAcquire`, `credharvest.go:192`);
  project creation is not. Any *authorized* user can therefore drive unbounded
  Kata microVM creation and Docker network churn on a 16-thread NUC — the same
  class of concern BUG-015 fixed for the LAN kick path, and the fix note there
  says to rate-limit "based on who can reach it not who is expected to".
  REQUIREMENTS.md puts abuse hardening out of scope, so this is a documented
  accepted risk rather than an oversight — but the concurrency cap is the one
  that protects R12's neighbours, not just the abuser. Remediation: cap
  concurrent in-flight project boots per user, and single-flight
  `importCredentials` the way the push already is.

- **[medium / confidence: medium]** `deploy/caddy/ingress/labels.go:129-144`
  (`parseEmailList`) and `deploy/caddy/ingress/generator.go:328-333`
  (`writeTransform`) — email values taken from container labels
  (`caddy_admins`, `caddy_viewers`, `caddy_superadmins`) and from the
  `SUPERADMINS` env are written **verbatim** into the generated Caddyfile as
  `match email <value>`. `sanitizeHostname` (`labels.go:154`) strips every
  Caddyfile-significant byte from the hostname for exactly this reason and says
  so in its comment; the email path has no equivalent. A value containing a
  newline plus `}` would close the `transform user` block and inject arbitrary
  Caddyfile directives into an internet-facing, docker-socket-holding process.
  Today both sources are operator-controlled (the server sets project labels and
  never sets `caddy_*`; `SUPERADMINS` comes from `deploy/.env`), so I rate this
  medium/medium rather than high — but it is the same asymmetry that the
  hostname path already treats as worth closing. Remediation: validate each
  parsed email against the same `^[^@\s]+@[^@\s]+\.[^@\s]+$` shape
  `internal/httpapi/shares.go:23` already uses, and drop anything that fails.

### Low

- **[low / confidence: high]** `internal/httpapi/git.go:447-450` — the untracked
  branch of `gitDiff` runs `git diff --no-color --no-index -- /dev/null <path>`
  in the guest. `--no-index` takes git *out* of repository mode, so the path is
  no longer confined to the work tree and `?path=../../../etc/shadow` reads an
  arbitrary guest-readable file back through the API. The requester is the
  project owner, who already has a root-capable shell in that same microVM, so
  no privilege boundary is crossed — it is a confused-deputy shape that costs
  nothing to close. Remediation: reject a `path` that is not workspace-relative
  (`path.Clean` + no leading `/` or `..`) before this call, matching what
  `scopeRel` does for the file explorer.

- **[low / confidence: high]** `internal/files/files.go:151-170`
  (`ContainedPath`) — resolves with `filepath.EvalSymlinks` and then does a
  *string prefix* comparison against the evaluated root. That is a classic
  check-then-use window, and the doc comment says so. Its only caller is
  `cmd/styrman/main.go:308` (the legacy ADR-027 workspace migration, run at boot
  against `<home>/projects/<dir>`), and since ADR-064 the tenant dir is
  server-only, so there is no guest able to win the race at the moment it runs.
  Recorded because the helper is exported and the next caller may not be so
  lucky. Remediation: mark it `// Deprecated:` or unexport it, so a new caller
  has to think.

- **[low / confidence: medium]** `internal/httpapi/server.go:439-471` — the SPA
  catch-all serves `_shell.html` for every unmatched non-`/api`, non-`/ws` path
  to any *authorized* user. Combined with `previewHostRewrite`
  (`server.go:486-527`), which only rewrites when the Host matches a configured
  preview base, a request arriving on the main host with an unmatched path is
  answered with the app shell. That is intended, but it means the 404 surface and
  the SPA surface are the same handler, and the `Authorized` check at line 448 is
  the *only* thing keeping a shared-preview guest out of the app shell. It is
  correct today; it is also one deleted `if` away from not being. Remediation:
  none required — recorded so the invariant is on the record with a line number.

- **[low / confidence: medium]** `internal/httpapi/shares.go:27-39`
  (`allUsers`) — enumerates `StateDir/users/*` on every preview-proxy cache miss
  and calls `s.space(user)` for each, which lazily *creates* per-user stores.
  A directory planted under `users/` (server-uid-writable, but so is everything
  there) becomes a user space. `space()` re-validates the slug
  (`server.go:262`), so a traversal name is refused; the residue is only that a
  stray directory costs a store load. Low, informational-adjacent.

- **[low / confidence: medium]** `internal/httpapi/credentials.go:22-66`
  (`credentialContent`) — returns the plaintext of the caller's own stored
  secrets (ssh private key, gcloud ADC, gh token) over the API, guarded only by
  `u.Authorized` and scoped by `u.Username`. It is audit-logged (line 33) and the
  same values already reach the user's guests, so the disclosure is not new — but
  it makes an XSS anywhere in the SPA a full credential exfiltration rather than
  a session compromise. `csrfGuard` does not help (this is a GET). Remediation:
  consider requiring a recent re-auth or at least keeping this endpoint off the
  default settings load path.

- **[low / confidence: low]** `deploy/firewall/styrman-firewall.sh:503` —
  `iface="sdb-$(printf 'sdb-%s' "${name}" | sha1sum | cut -c1-11)"` must agree
  byte-for-byte with `config.BridgeNameFor` (`internal/config/config.go:493`,
  `sha1(netName)` hex, first 11 chars). They do agree as written, and R12's
  whole per-project allowance keys on this. I did **not** find a test that
  asserts the shell and the Go derivation produce the same string for a known
  uuid; `deploy/firewall/test.sh` is a root test I did not execute. A future
  change to either side is a silent mis-targeting of a firewall RETURN rule.
  Remediation: add a cross-language golden-value assertion (one uuid, one
  expected `sdb-<11hex>`) to both `internal/config`'s tests and
  `deploy/firewall/test.sh`.

### Informational

- **[info / confidence: high]** `govulncheck` (both modules, via
  `bash deploy/vulncheck.sh`, exit 3): GO-2026-4887 (Moby AuthZ plugin bypass on
  oversized request bodies) and GO-2026-4883 (off-by-one in plugin privilege
  validation), both `Fixed in: N/A`, both reached only through `client.init` /
  `api.init` traces. These are *daemon*-side defects in a module this project
  uses only as an API **client**; no AuthZ plugin is configured and no plugin
  privileges are granted. Not actionable until Moby ships a fix; recorded so the
  next audit does not re-derive it. `npm audit` in `web/`: **0 vulnerabilities**
  across 301 dependencies.

- **[info / confidence: high]** Secrets: no credential material is committed.
  `git ls-files` finds exactly one secret-shaped tracked path, `deploy/.env.example`
  (a template). `.gitignore` covers `.env`, `*.env`, `.env.*`, `*.pem`, `*.key`,
  `*.tfvars`, `*.tfstate` with `!.env.example` correctly last. A pattern sweep
  (`AKIA`, `ghp_`, `gho_`, `github_pat_`, `sk-ant-`, `xox[baprs]-`, `AIza…`,
  PEM headers) over all 716 tracked non-`dist` files matched only prose in
  `.agents/docs/**`. `deploy/docker-compose.yml` passes secrets as env from a
  gitignored `.env`, which is correct for a self-hosted single-host deploy
  (no Secret Manager — GCP is explicitly not applicable, `docs/deploy/gcp.md`).

- **[info / confidence: high]** GCP surface: **none**. `docs/deploy/gcp.md`
  records the platform as self-hosted; there are no IAM bindings, no service
  accounts, no `--allow-unauthenticated`, and no service-account key files on
  disk in the tree. The only GCP contact is the *user's own* gcloud ADC handled
  as a file-tier broker credential (`credharvest.go:77-100`), which never leaves
  that user's projects.

## Worth a closer look (half-formed — not yet conclusions)

- `internal/httpapi/git.go:89-113` sets `GIT_CEILING_DIRECTORIES=path.Dir(ws)`
  so discovery cannot walk above the workspace. Does that ceiling actually hold
  when `ws` itself is a **bind mount point** (a different filesystem from its
  parent)? `GIT_DISCOVERY_ACROSS_FILESYSTEM=0` is also set, which should stop it
  earlier — but the interaction of the two on a bind boundary is exactly the
  case the comment does not name. Worth one live check in a booted VM.

- `internal/orchestrator/orchestrator.go:436-498` (`ResolveProjectByIP`) gates on
  `containerHoldsIP`, i.e. Docker's record of the container's endpoint address.
  Project containers get `CAP_NET_ADMIN` (`orchestrator.go:856`). A guest can
  therefore add a second address on its own interface. Every address it can add
  is inside *its own* bridge subnet, so resolution still lands on itself — but I
  did not verify that Docker's `NetworkSettings` view can never be made to
  report an address the container no longer holds after a guest-side `ip addr
  del`, which would leave a cached mapping that `ipCacheGet`'s re-verify accepts.
  Probably fine; not proven.

- `deploy/caddy/ingress/generator.go:126-148` (`writePreviewWildcard`) emits the
  `request_header -X-Styrman-Ingress` strip and the `request_header
  X-Styrman-Ingress <token>` inject as **bare directives**, not inside a
  `route {}` block, unlike every other block in the file. Caddy merges repeated
  `request_header` into one handler in written order, so this should be correct —
  but the file's own private/app blocks use `route {}` explicitly "for explicit
  handler ordering" (line 413). Why the asymmetry? If the merge order is not
  guaranteed, a client-supplied `X-Styrman-Ingress` could survive on the public
  preview host. `deploy/caddy/ingress/generator_test.go` exists; I did not read
  it to see whether it asserts the emitted order.

- `internal/httpapi/server.go:555-558` — `csrfGuard` exempts preview/link/shared
  paths from *both* the Origin check and the body-size cap. The cap exemption is
  justified in the comment (an app's uploads); the Origin exemption is too
  (`Origin: null` from a sandboxed frame). But `isLinkPath` is a bare
  `strings.HasPrefix(p, "/link/")` on a path that `http.ServeMux` has not yet
  cleaned. I convinced myself this is safe (an uncleaned `..` path reaches
  ServeMux and gets a 301, not data), but it deserves a written test rather than
  my reasoning.

## Checked

- **Secrets** — clean. Full pattern sweep over 716 tracked files (excluding
  `web/dist/`); only `deploy/.env.example` is secret-shaped and it is a template.
  `.gitignore` ordering verified (the `!.env.example` negation is last).
  No secret is echoed into an error path: `guestErrMessage`
  (`server.go:645`) strips control bytes and `redactSecrets` (`server.go:757`)
  removes URL userinfo before any guest-controlled text reaches a response;
  `cloneURLHasSecret` (`server.go:732`) refuses a clone URL carrying a password
  at the boundary rather than redacting it later.
- **Injection** — no shell is used for guest commands: every git verb builds a
  direct argv (`gitExec`, `git.go:89`), file paths always follow `--`, and the
  one interpolated value in the setup path is allowlisted (`setupTool`,
  `bridge.go:198`). `shellQuote` (`credharvest.go:184`) is correct
  single-quoting. Path injection is closed by `os.Root` confinement throughout
  `internal/files` and by `scopeRel`/`cleanRel`. `credstore` validates env keys
  against `^[A-Za-z_][A-Za-z0-9_]*$`, rejects control characters in values
  (`hasCtrl`), caps values at 4 KiB and file credentials at 256 KiB, and
  `validRelPath` rejects absolute paths and any `..` segment. The one gap found
  is `parseEmailList` in the ingress (finding above). The firewall script's
  `read_conf` (`styrman-firewall.sh:419`) accepts only five keys against a
  character-class-bounded regex and fails **closed** on any unparseable line.
- **Authz/authn** — `authGuard` (`internal/httpapi/auth.go:26`) fails closed on
  an unknown mode, requires the constant-time-compared ingress token in header
  mode, and `config.Load` refuses to boot header mode with an empty token
  (`config.go:224`) and refuses `auth=none` on a non-loopback bind
  (`config.go:211`) and a non-loopback dev listener (`config.go:206`).
  Cross-tenant reach: every project handler resolves through
  `getProjectFor` → the **requester's own** `project.Store`, so a UUID belonging
  to another user simply is not found. The two deliberate cross-owner paths are
  `previewAccess`/`previewAllowed` (`shares.go:44,79`) — the owner branch
  requires `u.Authorized && ownerUser == u.Username`, so a non-styrman Google
  guest whose derived slug collides cannot match — and `serveShared`
  (`links_api.go:199`), which re-reads the live share list on every request so
  revocation is immediate. Public and private link kinds are refused on each
  other's route. `requireAdmin` gates the admin endpoints; the settings payload
  hides `hostFirewall` from non-admins (`explorer.go:456`, the BUG-017 fix).
- **Crypto** — nothing hand-rolled. No password hashing, no session
  implementation, no JWT/OIDC code in this repo: authentication is Google OAuth
  terminated by `caddy-security` in the vendored ingress, and the server only
  reads a header. The one crypto primitive in server code is
  `crypto/subtle.ConstantTimeCompare` for the ingress token (`auth.go:40,55`) —
  the correct choice. `sha1` (`config.go:494`) is used only to derive a bridge
  *interface name* from a network name, not as a security primitive.
  Link tokens go through `links.ValidToken`; I read the call sites but not the
  generator (see Not examined).
- **Dependencies** — `bash deploy/vulncheck.sh` run, exit 3: two Moby
  vulnerabilities with no fix available, both daemon-side, in both modules (see
  Informational). `npm audit` in `web/`: 0 vulnerabilities, 301 deps. The Go
  dependency set is genuinely stdlib-first and matches the REQUIREMENTS.md
  allowlist exactly (`coder/websocket`, `docker/docker`; `creack/pty` is no
  longer required at root) — no unjustified additions.
- **XSS / browser surface (`web/`)** — one `dangerouslySetInnerHTML` in the
  whole SPA (`web/src/components/FileEditor.tsx:395`), fed by
  `micromark(text, { allowDangerousHtml: false })` (line 155). Raw HTML in a
  guest-authored `README.md` is escaped, and micromark's own `sanitizeUri`
  restricts `<a href>` to http/https/irc/mailto/xmpp, so `javascript:` is
  neutralized; the `.replaceAll("<a href=", …)` at line 156 can only match
  micromark-generated markup because `<` in content is already escaped. No
  `innerHTML`, `document.write`, `eval`, `new Function`, `srcdoc` or
  `javascript:` anywhere in `web/src`. `window.open` (`lib/terminal.ts:152`)
  passes `noopener,noreferrer`. **No secret is persisted client-side**: a setup
  token goes to `sessionStorage` with an explicit rationale
  (`web/src/lib/clips.ts:7-9`); `localStorage` holds only editor/layout
  preferences and a recents list.
- **Ingress admin API** — `isLoopbackAddr` (`deploy/caddy/ingress/loopback.go:14`)
  is the guard that keeps the config-replacement endpoint off the network, and it
  fails closed correctly: `host:port` required, port must be canonical decimal,
  IP **literal** only (names — including `localhost` — are refused because NSS
  resolution at bind time is not what the check inspected), and `:2019` is
  rejected. `cmd.go:149` refuses to start on failure, and `writeGlobalOptions`
  (`generator.go:217`) re-emits the pin into every generated config so a `/load`
  cannot move the listener. This closes the 2026-09-01 incident described in the
  comment at `generator.go:205-216`.
- **GCP surface** — not applicable; see Informational. No IAM, no service
  accounts, no key files, no Cloud Run.

## Not examined

Named honestly, because this is a whole-project audit and the record should say
where its edges are.

- `deploy/update/styrman-update` (695 lines, root via systemd timer) — **now
  read in full**, along with `styrman-update.service`. It is unusually well
  hardened: `verify_tag` (line 280) requires an *annotated* tag, requires an
  **SSH** signature specifically (because the GPG path exits 0 for
  good-but-untrusted keys and never consults an allowlist), takes the **last**
  armor header to defeat a decoy header planted in the author-controlled tag
  message, and asserts `allowed_signers` is root-owned before trusting it —
  every one of those is a real, previously-exploited bypass, closed. The
  `/etc/styrman/update.env` opt-out is `grep`ed, never sourced. `self_install`
  and `host_converge` install from `git show <verified-sha>:<path>`, never from
  the mutable worktree, and both refuse to run when signature enforcement is
  off. The state-dir guards (ADR-052, lines 79-98) reject `/var/lib/styrman`,
  `/`, and bare top-level dirs after canonicalization. One gap found (above).
  It still has **no test script**, which for a root-run, self-replacing binary
  is the single largest untested surface in the repo.
- `deploy/firewall/styrman-firewall.sh` — read in full, and its structure
  (delete-by-spec, fail-closed conf parsing, tighten-before-loosen, the
  `inherit_errexit` and `CHAINS` array guards from BUG-010/013) reviewed. **Not**
  executed; `deploy/firewall/test.sh` (412 lines) was **not run or read**, and no
  live `iptables -S` state was inspected.
- `deploy/firewall/install.sh` — read; the sshd password-auth gate
  (`require_sshd_hardened`, line 112) and the conf-writing logic were reviewed,
  but the `Include` glob expansion at line 104 (`for g in ${inc}` — deliberate
  word splitting) was not adversarially tested.
- `internal/conversation/` (store + tmux manager, 493 lines) — **not read**.
- `internal/history/` — **now read in full**; one finding above.
- `internal/links/store.go` — token generation **now verified**: `newToken`
  (line 96) uses `crypto/rand` for 16 bytes / 128 bits, hex-encoded, and
  `ValidToken` (line 356) is an exact 32-lowercase-hex shape check. Correct.
  The rest of the store (expiry sweep, `MaxLivePerProject`, `EnsurePrivate`,
  `DeletePrivate`, the persistence path) was **not** read.
- `internal/userstore/` — **now read** for the authorization core.
  `IsAdmin` (line 110) is env-only (`STYRMAN_ALLOWED_EMAILS`), so a runtime-added
  user can never become admin; `IsAuthorized` (line 117) checks env ∪ stored.
  The H3 slug-collision guard is enforced in **both** places it must be —
  `New` (line 67, refuses to *start* if two authorized emails collide) and
  `Add` (line 141) — and `identity.DeriveUsername` is confirmed to be the single
  derivation. `Add`/`Remove`/`save` were read only in outline.
- `internal/identity/` — **now read**; `DeriveUsername` and the `Authorized`
  flag's contract check out.
- `internal/gitrepo/` — **not read**.
- `internal/orchestrator/` — read `EnsureRunning`, `create`, `Stop`,
  `ResolveProjectByIP`, `EnsureSelfAttached`. **Not read**: `Destroy`,
  `CleanupLegacy`, `RemoveStaleSetupVMs`, `Logs`, `ListeningPorts`, `IPAddress`,
  the idle reaper, `exec.go` in full, and `realbelow.go`/`projectHomeIsReal`'s
  own implementation (I read its *callers* and its contract, not its body).
- `internal/httpapi/`: **not read** — `setup.go`, `provision.go`, `check.go`,
  `network.go`, `lanprune`, the second half of `git.go` (checkout/log/stash/
  `validBranchName`/`validCommitHash`/`gitRun`), `deleteProject`/wipe,
  `listNotifications`, project env handlers.
- `internal/netpolicy/status.go` (`ParseStatus`, `Exposed`, staleness) — **not
  read**; only `dir.go`.
- `deploy/caddy/ingress/`: read `labels.go`, `generator.go`, `loopback.go` and
  the admin-binding path in `cmd.go`. **Not read** — `watcher.go` (415 lines,
  the Docker event loop that supplies `ContainerIP`/`Port` straight into
  `reverse_proxy` targets), `cloudflare.go` (341 lines, DNS record management
  holding a Cloudflare API token), `module.go`, `config.go`, and **all of its
  tests**. Its `go test ./...` was **not run** — and the root `go test ./...`
  does not cover this module, so it is the least-tested-in-practice Go code in
  the repo.
- `web/` — swept for XSS sinks and client-side secret storage (see Checked); the
  one `dangerouslySetInnerHTML` was read in context. **Not** read: the rest of
  the SPA, notably `lib/api.ts`'s session/redirect handling (BUG-001's area),
  the terminal component's handling of raw PTY bytes, and how git/guest error
  strings are rendered in the side panels. `npm audit` was run.
- `internal/conversation/` — surveyed by symbol only, not read. Confirmed that
  ids are validated on load as well as on create (`store.go:65`, because they
  flow into tmux session names and the autostart command into a shell string)
  and that `liveSessions` (`conversation.go:87`) is the one place a `sh -c`
  string is built — from constants, with no interpolation. The store's
  Create/Rename/SetCommand/Delete bodies were **not** read.
- `image/` — read `configs/styrman-credentials.sh` only. **Not read**:
  `Dockerfile`, `entrypoint`, `styrman-setup`, `styrman-git-credential`,
  the harness under `image/harness/`.
- `cmd/styrman/main.go` (589 lines) — **not read** beyond line 308's
  `ContainedPath` call.
- No test suite was executed (`go test ./...`, the ingress module's tests,
  `web/e2e/run.sh`, `integration.sh`, `deploy/firewall/test.sh`). Verification
  of behaviour is a `verifier` job; this review is static.
