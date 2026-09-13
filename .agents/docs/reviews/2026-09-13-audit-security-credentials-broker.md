---
owner: security
date: 2026-09-13
---
# Security audit — credential store, broker, and in-guest materialization (2026-09-13)

Range reviewed: working tree at `e56e775708a9062cc6432f5c3278770bed63b685` (clean tree, `main`).
Every path and line number below is pinned to that commit.

Surface: `internal/credstore/`, `internal/credbroker/`, the broker's caller
resolver in `internal/orchestrator/orchestrator.go`, the credential routes and
setup/import harvest in `internal/httpapi/` (`credentials.go`, `credharvest.go`,
`projectenv.go`, `setup.go`, `explorer.go`, `provision.go`),
`image/configs/styrman-credentials.sh`, `image/configs/styrman-git-credential`,
`deploy/.env` handling (R10), secret logging, and error bodies that could echo a
token. Judged against AGENTS.md, REQUIREMENTS.md R1–R13, the three newest
`.agents/docs/changes/` entries (v1.29.0 release, browser-review-fixes, v1.28.0
release), `.agents/docs/specs/styrman/{runtime-image,api,guide}.md` and
`.agents/docs/bugs/INDEX.md`.

## Verdict: FINDINGS

The credential broker's in-guest client computes the broker's address itself,
from a hard-coded `/16` assumption, and **overwrites the correct address the
server injected**. Since `deploy/host-setup.sh:128` pinned Docker's address pool
to `/24` subnets (commit `46f3ec2`, 2026-09-10), that computation is wrong for
every project bridge: the live host's project network is `172.16.1.0/24` with the
broker at `172.16.1.254`, and the guest asks `172.16.255.254`. Every login shell
in every project created since that daemon restart therefore fetches nothing —
no ssh key, no `GH_TOKEN`/`NPM_TOKEN`, no git identity, no claude/agy/gcloud
credential — silently, because the snippet swallows every curl failure. R3's
"sign in once, every project stays signed in" does not hold, and the same
clobbered variable mis-points the git credential helper for every child process
of that shell. Nothing in the test suites can see it: the only broker probe
(`waitForBroker`) deliberately uses a *plain* shell and the *injected* URL, so it
measures the address that still works. The second-order half of the same defect
is a security one: for a LAN-mode project the misdirected, unauthenticated,
plaintext request leaves the host, and whatever answers it gets to write files
into the project home.

Everything else in this surface held up: the store's path/keys/values are
validated, secrets are never logged or echoed in error bodies, `deploy/.env` is
gitignored and `.env.example` carries no values, the resolver re-verifies a
cached IP→project mapping against the live container, and the HTTP credential
routes are behind the ingress-token + userstore guard and scoped to the caller's
own username.

## Findings

- **[high / confidence: high]** `image/configs/styrman-credentials.sh:25-31` —
  **the login-shell credential snippet computes the wrong broker address on
  every project bridge, so no credential is ever materialized.** Line 29 rebuilds
  `STYRMAN_CRED_BROKER` as `<octet1>.<octet2>.255.254:<port>` — broadcast−1 of a
  `/16`. The server binds the broker at broadcast−1 of the bridge's *actual*
  subnet (`brokerIPForSubnet`, `internal/orchestrator/orchestrator.go:475-488`,
  used by `EnsureSelfAttached`), and since `deploy/host-setup.sh:128` pins
  `default-address-pools` to `[{"base":"172.16.0.0/12","size":24}]` every new
  bridge is a **/24**. Verified read-only on this host: `docker network inspect
  sdb-32c61123-…` reports `172.16.1.0/24`, so the broker is at `172.16.1.254`
  while a guest on `172.16.1.2` derives `172.16.255.254`. Both `/env` and
  `/files` fetches then fail (`curl … || true`, lines 40 and 54) and the shell
  continues in silence: no `~/.ssh/id_ed25519`, no gcloud ADC/`credentials.db`,
  no `~/.claude/.credentials.json`, no agy OAuth token, no `GH_TOKEN`,
  `NPM_TOKEN`, `STYRMAN_GIT_NAME`/`_EMAIL`. Because the variable arrives
  exported from the container env, the reassignment stays exported, so every
  child of that login shell — including `image/configs/styrman-git-credential:29`
  — asks the wrong address too, and `git push`/`clone` lose their token as well.
  Two extra seconds of connect timeout are paid per login shell and one per git
  operation. This is a repeat of the mechanism in **BUG-035** (a change made in
  one place — the daemon's address pool — not propagated to the consumer that
  depends on it) and of **BUG-025** (a consumer re-deriving an algorithm instead
  of reading the constant the producer used), so it is more severe, not less.
  A second fragility in the same expression: line 25 takes `NR==1` of *all*
  global-scope IPv4 addresses, which after R2's per-project dockerd starts also
  includes the guest's own `docker0` (`172.17.0.1/16`) — an interface-ordering
  accident away from a differently wrong answer.
  **Fix:** delete the derivation and use the injected `STYRMAN_CRED_BROKER`
  (the orchestrator writes it at every container create,
  `internal/orchestrator/orchestrator.go:1019`, and both the ADR-067 mode toggle
  and any network recreation rebuild the container, so the M17 "stale IP" case
  it was added for no longer exists). If a self-heal must stay, read the prefix
  *length* the kernel already reports (`ip -o -4 addr show dev eth0` yields
  `172.16.1.2/24`) and compute broadcast−1 of that CIDR — never a fixed `/16`.
  Either way, add one integration check that a **login** shell in a real project
  materializes a known credential (see the low finding on coverage below).

- **[medium / confidence: medium]** `image/configs/styrman-credentials.sh:29,54-70`
  — **the broker peer is chosen by arithmetic and trusted without
  authentication, and its answer writes files into the project home.** The
  broker has no token, no TLS and no server identity by design (network position
  *is* the identity, `internal/credbroker/broker.go:1-12`), which is sound while
  the peer is provably on the project's own bridge. With the wrong address above,
  it is not. For a SANDBOX project the request fails closed — the live rule set
  (`sudo -n iptables -S STYRMAN-FWD`) returns early only for
  `--physdev-is-bridged` traffic and otherwise drops `-d 172.16.0.0/12` — but a
  **LAN** project's bridge is `lan-<hash>`, which `DOCKER-USER -i sdb-+` does not
  match (ADR-067, R12), so the request is NAT'd onto the operator's network. Any
  host that holds `172.16–172.31.255.254` there (a LAN or VPN route inside
  172.16/12) can answer `/files` with attacker-chosen `path`, `mode` and `data`
  and `/env` with attacker-chosen variables; the snippet writes each under
  `$HOME` (only absolute paths and `..` are refused — `~/.bashrc`, `~/.profile`,
  `~/.ssh/authorized_keys` are all reachable) and exports each into the shell.
  That is guest code execution supplied by an unauthenticated stranger. It is
  gated on the deployment's LAN using 172.16/12, hence medium.
  **Fix:** the high finding's fix removes the wrong address; on top of it, make
  the snippet refuse any broker address outside the guest's own on-link subnet
  (compare against the interface CIDR it already reads), so "a random host
  answered" is structurally impossible rather than merely unlikely.

- **[medium / confidence: high]** `internal/credbroker/broker.go:61-75` with
  `internal/orchestrator/orchestrator.go:89-105` — **every container the project
  runs inside its own dockerd is handed the user's entire credential set, for
  all of that user's projects.** The caller is resolved from the source IP, and
  the project's inner containers SNAT behind the microVM's bridge address (the
  code says so at `containerHoldsIP`'s comment), so `curl http://<broker>/files`
  from any `docker run` image — which R2 exists to make easy — returns the ssh
  private key, gcloud ADC and `credentials.db`, the claude and agy OAuth refresh
  tokens, and `/env` returns `GH_TOKEN`/`NPM_TOKEN`. ADR-068 (D3) records exactly
  this reasoning for the browser's port 9222 ("one trust domain with the project
  user and the project's own inner containers"); nothing records it for the
  broker, whose contents are worth far more and are not scoped to the calling
  project. The risk is inherent to the metadata-server pattern the ADR chose, so
  this is a recorded-intent gap rather than a code defect — but an unrecorded
  one is how a future change (e.g. "let agents run untrusted images") silently
  inherits it.
  **Fix:** state it in ADR-030/032 and `specs/styrman/guide.md` as an accepted
  risk, naming the blast radius (all tiers, all of that user's projects,
  including credentials harvested after the container started). If it is not
  acceptable, stop relying on network position alone: deliver a per-project
  secret into the guest root filesystem (e.g. `/run/styrman/broker-token`, mode
  0400, root-owned, outside the home bind) and require it as a request header —
  an inner container has to be given it deliberately.

- **[low / confidence: high]** `internal/credstore/credstore.go:459-471`
  (`safe`), used by `PutGit:121`, `GetGit:132` and `Delete:400-405` —
  **distinct git hosts collide onto one store file.** `safe` maps `.`, `:`, `/`
  and `\` all to `_`, so `github.com`, `github_com`, `github:com` and
  `github/com` all address `git-github_com.json`. Storing a credential for one
  silently replaces another's token, deleting one deletes the other, and the
  JSON's `Host` field then disagrees with the filename that `List` and the
  Settings view key off. The broker's `?host=` is URL-decoded and passed straight
  in (`broker.go:82-87`), so a guest can request the collided name; within one
  user's own store that discloses nothing new (all tiers are already served to
  all of that user's projects), but the silent overwrite is a real correctness
  defect for anyone with more than one forge.
  **Fix:** validate the host against the same `hostnameRe` the HTTP layer already
  applies (`internal/httpapi/explorer.go:537`) in `PutGit`/`GetGit`/`Delete`, and
  make the filename injective (`url.PathEscape` or hex of the host).

- **[low / confidence: high]** `.agents/docs/testing/styrman/integration.sh`
  (no broker checks at all) and `web/e2e/run.mjs:1462-1466` (host-mode server,
  "no credential broker by design") — **nothing in any suite exercises credential
  delivery through a login shell**, which is why the high finding shipped
  unnoticed. The one probe that touches the broker, `waitForBroker`
  (`internal/httpapi/provision.go:185-203`), uses a plain shell and the injected
  URL *on purpose*, so it can never fail for the derived URL — an instrumentation
  blind spot of the BUG-034/BUG-040 family: the green signal measures a different
  address than the one production uses.
  **Fix:** add one integration check against real Kata — store a marker env
  credential, then assert `bash -lc 'echo $MARKER'` in a project prints it (and
  that `ls -l ~/.ssh/id_ed25519` exists when an ssh credential is stored). It
  fails today and would have failed on 2026-09-10.

- **[low / confidence: medium]** `internal/credstore/credstore.go:86-92` (`New`)
  — `os.MkdirAll(baseDir, 0o700)` sets the mode only when it *creates* the
  directory. `StateDir/credentials` lives on the host bind (`/var/lib/styrman`,
  currently `drwx------`, verified), but a restore, an `rsync -a` without
  `--perms`, a `tar` extraction under a 022 umask, or a hand-made directory
  leaves a looser mode in place forever, and every user's plaintext secrets
  underneath become readable to any uid on the host. The per-user subdirectory
  created by `atomicWrite` has the same property.
  **Fix:** follow the `MkdirAll` with an explicit `os.Chmod(baseDir, 0o700)` (and
  the same for the user dir), so the mode is asserted on every start rather than
  only at first creation — the same "never trust a pre-existing directory's
  mode/owner" rule BUG-009 wrote for the root firewall installer.

- **[low / confidence: medium]** `internal/credbroker/broker.go:87-92` — a
  genuine store failure is reported to the caller and the operator as "not
  onboarded". Every `GetGit` error (unreadable file, corrupt JSON, a filename too
  long) becomes `404 no credential` with nothing logged, so git falls through to
  "no credential" and an operator debugging a failed push sees an empty log where
  the env and files tiers would both have logged. The 404 is right for git;
  the silence is not.
  **Fix:** `if !errors.Is(err, credstore.ErrNotFound) { b.logger.Warn(...) }`
  before writing the 404, matching the `/env` and `/files` handlers' treatment of
  store errors.

## Cleared on inspection

- **Secrets in the repo (R10).** `.gitignore:1-18` covers `.env`, `*.env`,
  `.env.*` with `!.env.example`; `git ls-files` shows no `.env` and
  `deploy/.env.example` carries names and comments only, no values. Nothing in
  `image/`, `deploy/` or the store code embeds a token.
- **Secret logging.** The broker logs user/project/host/count and never a value
  (`broker.go:94,124,145`); `putTokenCredential`, `deleteCredential`,
  `credentialContent`, `projectenv.go` and `credharvest.go` log keys, ids and
  per-tool status strings only. `validateEnv`'s errors name the key, never the
  value. The harvest report contains "stored (GH_TOKEN)"-style labels only.
- **Error bodies.** `guestErrMessage` (`server.go:684-707`) strips control bytes
  *before* `redactSecrets` (`server.go:791-793`) so a planted byte cannot break
  the userinfo regex, and clamps length; `cloneURLHasSecret` refuses a
  credential-bearing clone URL at creation with a message that does not echo it.
- **Caller resolution.** `ResolveProjectByIP`
  (`orchestrator.go:528-590`) re-verifies a cache hit with a live
  `ContainerInspect` (owner label + `containerHoldsIP`) and refuses an IP that
  merely falls inside a project's subnet, so a freed/reused address cannot be
  served another user's credentials; `sdb-sys-setup-*` is excluded, so the setup
  VM (where the user signs in) is never served the broker's copy.
- **HTTP authz.** Every credential route is registered on the guarded mux
  (`server.go:462-467`, `RoutesWithAuth` → `authGuard`), which requires the
  ingress token in header mode and refuses an unauthorized email for anything
  that is not a preview/share path (`auth.go:26-95`, `shares.go:101-107`,
  `links_api.go:274-279`). All store access is keyed by `u.Username`; there is no
  client-supplied user or project id on any credential path, and
  `getProjectFor` resolves the project through the caller's own store.
- **No CORS anywhere in the server or the broker** — a page in the project's own
  in-guest Chromium (ADR-068) can issue `fetch('http://<broker>/files')` but
  cannot read the response, and the launcher passes no `--disable-web-security`
  (`image/configs/styrman-browser:237-244`).
- **Store input validation.** `envKeyRe` bounds env keys to identifiers,
  `hasCtrl` rejects control characters (the `KEY\tVALUE` line protocol), a 4 KiB
  value cap and a 256 KiB file cap are enforced, `validRelPath` refuses absolute
  paths and `..`, the guest snippet re-checks both, `atomicWrite` is 0600
  temp+rename, and `.json.tmp` cannot be read back as a credential.
  `importGuestFile` quotes the guest path and `Exec` caps output at 2 MiB
  (`orchestrator/exec.go:32`), so a hostile guest file cannot exhaust the server.
- **Revocation actually revokes.** `deleteCredential` captures the path before
  the delete and `sweepMaterializedFile`/`sweepProjectCredentials` remove already
  materialized copies through `files.Remove`'s `os.Root` confinement — the
  guest-symlink lesson of BUG-002/003 is applied on both paths.
- **Broker exposure.** `deploy/docker-compose.yml:102-105` publishes no port for
  the server; the broker binds inside the container only, and the only other
  network it sits on (`styrman-ingress`, 172.18.0.0/16) is reachable from the
  host and host-networked Caddy, both of which fail the IP→project resolution.
  Docker's unconditional per-bridge DROP keeps another project's bridge out.
- **Dependencies.** No new dependency in this surface; `go.mod` is the three
  approved modules plus `golang.org/x/sys`.

## Checked

- **Secrets** — none in code, config, tests or git history for this surface;
  `deploy/.env` is gitignored and absent from `git ls-files`; no secret is
  logged or returned in an error body (see Cleared).
- **Injection** — guest paths are single-quoted (`shellQuote`), the `gh
  --hostname` interpolation is `hostnameRe`-validated, store filenames are
  separator-free (though not injective — low finding), the broker's `?host=`
  cannot escape the user directory, and the guest snippets reject non-identifier
  env keys and absolute/`..` file paths.
- **Authz/authn** — all credential routes sit behind `authGuard` (ingress token
  + userstore) and are scoped to the caller's own username; the broker's
  authorization is network position, re-verified against the live container per
  request (medium finding records what that means for inner containers).
- **Crypto** — nothing hand-rolled here: no hashing, signing, session or token
  minting in the credential path; the ingress token comparison elsewhere uses
  `crypto/subtle`. The broker is deliberately unauthenticated plaintext on a
  private bridge, which is only sound while the peer is provably on that bridge
  — the medium finding is where that stops holding.
- **Dependencies** — `govulncheck ./...` (local toolchain go1.25.0) exit 3, 31
  findings: 29 are stdlib issues fixed in go1.25.8–1.25.13 and **do not apply to
  the shipped artifact**, which is built with `golang:1.25.13-trixie`
  (`deploy/Dockerfile.server:9`, `image/Dockerfile:8`) — that is what
  `deploy/vulncheck.sh` exists to measure, and it was not re-run here because it
  needs the builder image. The two module findings are
  `github.com/docker/docker@v28.5.2+incompatible` GO-2026-4887 (AuthZ plugin
  bypass on oversized bodies) and GO-2026-4883 (off-by-one in plugin privilege
  validation), both "Fixed in: N/A": they are daemon-side defects in code this
  repo imports only as a client, and this deployment runs no AuthZ plugin and
  installs no Docker plugins — no action beyond tracking. `npm audit` was **not**
  run: the `web/` surface is out of scope for this audit and unchanged by it.
- **GCP surface** — not applicable: self-hosted, `docs/deploy/gcp.md` says so.
  The only GCP contact is the harvested gcloud ADC/`credentials.db` treated as
  opaque file credentials; no IAM binding, service account key file on disk
  (beyond the user's own harvested ADC), or Cloud Run surface exists here.

## Not examined

- `deploy/caddy/ingress` (separate Go module) — the ingress's OAuth/JWT handling,
  `X-Caddy-Email` injection and the `X-Styrman-Ingress` token generation. Only
  the server's *consumption* of those headers was read.
- `deploy/firewall/` beyond reading the live `DOCKER-USER`/`STYRMAN-FWD` rules to
  settle whether a misdirected broker request escapes (it does for LAN, not for
  sandbox). The firewall's own installer, updater and self-healing loop were not
  audited.
- The `web/` Settings and SidePanel credential UI (how values are rendered,
  cached or copied to the clipboard) and `npm audit`.
- `internal/files`, `internal/terminal`, `internal/links`, `internal/history` —
  touched only where the credential paths call into them (`files.Remove`,
  `files.ReadRawNoFollow`).
- **Nothing was built, and no test suite was run.** No `go build`/`go test`, no
  `integration.sh`, no `web/e2e/run.sh`, no image build, no Kata guest. The high
  finding is established from the pinned source plus three read-only host
  observations (`docker network inspect` → `172.16.1.0/24`, `docker inspect
  styrman-server`, `iptables -S STYRMAN-FWD`), not from watching a login shell
  fail; the failing-shell demonstration belongs to the integration check the
  finding asks for.
