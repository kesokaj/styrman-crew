---
owner: security
date: 2026-09-22
---
# Security review — project   (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff.

## Status: COMPLETE

## Verdict: FINDINGS

The worst thing on the record is a supply-chain hole in the one artifact every
tenant boots: `image/Dockerfile` pins and checksum-verifies *every* download it
makes — Go, Node, OpenTofu, yq, hadolint by publisher sha256, the `sh-agent`
harness by an ssh signature — except the Antigravity CLI, which is fetched from
`https://antigravity.google/cli/install.sh` and executed as root with no
version pin and no integrity check at all. Whatever that URL serves at build
time becomes root-level code in the runtime image, and the runtime image is
what every project microVM runs with its owner's brokered credentials
materialized into it. Everything else found is defense-in-depth or dependency
hygiene. The boundaries that actually carry identity — the ingress header
contract, the auth guard, the credential broker's bridge-position
identification, the per-tenant slug, the VM-runtime and `ip_forward`
assertions — are in unusually good shape, and each already carries a
fail-closed guard plus a test. Note the "Not examined" section: the
guest-writable-path guards were traced to their call sites but
`internal/files/` itself was not read line by line.

## Findings

- **[medium / confidence: high]** `image/Dockerfile:106-112` — the Antigravity
  CLI is installed by `curl -fsSL -o /tmp/agy-install.sh
  https://antigravity.google/cli/install.sh; bash /tmp/agy-install.sh`, with no
  version pin and no integrity check. Every other fetch in this file is pinned
  and verified (`TOFU_SHA256`, `YQ_SHA256`, `HADOLINT_SHA256`, `GO_SHA256`,
  `NODE_SHA256`, and `ssh-keygen -Y verify` against `HARNESS_SIGNER` for
  `sh-agent`); here TLS is the only control. A compromised publisher, a
  compromised CDN object, or any party who can present a certificate for that
  host lands arbitrary build-time root code in the image every tenant's microVM
  boots — beside the credential broker's login-shell snippet, the git
  credential helper, and the materialized gcloud ADC / ssh key. The file's own
  comment ("the installer downloads to a file first so a failed fetch cannot
  run a partial script") addresses truncation, not authenticity, and the
  "floats by upstream design" note explains the version float but not the
  missing digest. Remediation: pin the agy release by version and verify a
  publisher-published digest the same way `HARNESS_VERSION`/`HARNESS_SIGNER`
  already is. If the publisher offers neither a versioned artifact nor a
  digest, pin the installer script's own sha256 as an `ARG` and have
  `image/bump-pins.sh` refresh it, so a change in the served bytes breaks the
  build instead of shipping silently; record the residual in an ADR. No new
  dependency is involved — the fix is `sha256sum -c`, already used five times
  in this file. `.agents/rules/shared-path-learnings.md` (BUG-070) states the
  rule this violates: "a download's checksum comes from something the publisher
  still publishes".

- **[low / confidence: medium]** `deploy/caddy/ingress/generator.go`,
  `writeAppBlock` ModeRoleGated branch — `X-Caddy-Role` is only ever *set*
  inside three `header_regexp` matchers, and a client-supplied `X-Caddy-Role`
  is never stripped on this path. `X-Caddy-Email`/`-User`/`-Subject` are set
  unconditionally from the authorizer's placeholders, the `X-Token-User-*`
  family is stripped before *and* after `authorize`, and `X-Styrman-Ingress` is
  stripped unconditionally before injection — this one header is the exception.
  If no matcher fires, the client's own value reaches the upstream. In practice
  the policy admits only holders of one of those three roles so a matcher
  should always fire, and styrman's own server ignores `X-Caddy-Role` entirely
  (admin comes from `userstore.IsAdmin`, i.e. `STYRMAN_ALLOWED_EMAILS`), so no
  app in this deployment is affected — the exposure is any future role-gated
  app behind this generic ingress. Remediation: emit `request_header
  -X-Caddy-Role` immediately before the matchers, exactly as the `ModePublic`
  branch already strips the whole identity set.

- **[low / confidence: high]** `deploy/caddy/ingress/generator.go`, same block —
  `app.Hostname` is interpolated unescaped into `header_regexp
  X-Token-User-Roles "app/<hostname>/<role>"`. `sanitizeHostname` permits `.`
  and `-`, and `.` is a regexp metacharacter, so a hostname containing a dot
  also matches a sibling app's role string differing in that position.
  Widening only, and both the hostname and the role vocabulary are
  server/operator-controlled, so it is not reachable today. Remediation:
  `regexp.QuoteMeta` the hostname before it enters the matcher string.

- **[low / confidence: high]** `web/package-lock.json` — `npm audit --omit=dev`
  reports one high advisory: `js-yaml@4.3.1`, GHSA-2883-xcg3-v3hh
  (`maxTotalMergeKeys` does not bound CPU for empty merge sources). Path:
  `@tanstack/react-start@1.168.49 → @tanstack/start-plugin-core@1.171.39 →
  xmlbuilder2@4.0.3 → js-yaml@4.3.1`. Build-time only: the shipped artifact is
  the static export embedded via `go:embed`, and js-yaml reaches neither the
  browser bundle nor the Go server, so the exposure is CPU exhaustion on a
  build host parsing hostile YAML, which this build does not do. Remediation:
  `npm audit fix` (or bump `@tanstack/react-start`) at the next frontend touch.

- **[low / confidence: medium]** `cmd/styrman/main.go:50-51` — the R12
  no-routing assertion fails **open** on one error class: when
  `/proc/sys/net/ipv4/ip_forward` returns `fs.ErrNotExist`, the server logs a
  Warn and continues, while every other read error and a forwarding-on reading
  both exit 1 unless `STYRMAN_ALLOW_IP_FORWARD=1`. The comment reasons it
  deliberately ("an exotic kernel must not take the service down over it"), so
  this is a judgement call rather than an oversight — but it is exactly the
  shape this project has recorded four times (BUG-014, BUG-028, BUG-029,
  BUG-031: a status check erroring and being read as the safe answer), and the
  global rule is that a guard's error path keeps the safe outcome. A container
  spec with `/proc` masked or that path bind-mounted away silently takes the
  fail-open branch, and R12's guarantee then rests on a Warn nobody reads. The
  actor who can set that spec is the operator, which is why this is low.
  Remediation (if the trade is to be reversed): treat `ErrNotExist` like every
  other error and require the existing `STYRMAN_ALLOW_IP_FORWARD=1` override —
  the escape hatch for the exotic kernel already exists and is already named in
  the message.

- **[informational / confidence: high]** `govulncheck` reports GO-2026-4887 and
  GO-2026-4883 as *called* in both modules, from
  `github.com/docker/docker@v28.5.2+incompatible`, both "Fixed in: N/A". Not a
  new finding: `deploy/vulncheck.sh:9-13` names both IDs and records them as
  accepted per release with no fix available. Recorded so the gate's
  expectation stays explicit — a third ID in that scan is a new finding, not
  more of the same. Weight by blast radius: the Caddy container is
  internet-facing and holds the Docker socket
  (`deploy/docker-compose.yml:22-26` says to treat any RCE in it as host root),
  so both `go.mod` files should be bumped the day a fixed release exists.

- **[informational / confidence: high]** `image/Dockerfile:30-35` — the apt
  keyrings for Docker, the GitHub CLI and Google Cloud are fetched over plain
  TLS with no pin (trust-on-first-use at build time). Ecosystem-normal, and
  every subsequent package is then apt-signature-checked against that key;
  noted only so it sits beside the finding above, not proposed for change.

## Checked

- **Secrets** — no tracked `.env`, `*.pem`, `*.key`, `*.tfvars` or credential
  JSON (`git ls-files` + pattern scan over the whole tree). Every high-entropy
  hit (`ghp_SECRETTOKEN`, `-----BEGIN OPENSSH PRIVATE KEY-----`, `sk-ant-…`) is
  a test fixture or a *redaction* test asserting the value never reaches a log
  or an argv (`internal/config/styrman_ctl_test.go:924-948`,
  `internal/httpapi/server_test.go:148-291`). `deploy/.env.example` carries
  empty placeholders only. The GitHub token in the image build is a BuildKit
  `--mount=type=secret` (`image/Dockerfile:150`), never an `ARG` or a layer.
  `deploy/vulncheck.sh:25-34` shadows `deploy/.env` with `/dev/null` so a
  container running a downloaded tool never sees the ingress token. Broker
  handlers log counts and key names, never values (`credbroker/broker.go`).
- **Injection** — every guest command crosses `orchestrator.ExecSpec` as an
  argv slice. The four `bash -c <script>` sites (`httpapi/check.go:383`,
  `httpapi/format.go:111`, `httpapi/stats.go:77`, `orchestrator/ports.go:94`)
  build the script from allowlisted constants (`checkerFor`, `formatInPlace`,
  `safeSuffix`), pass user content on **stdin** and variable data through env
  (`STYRMAN_FMT_DIR`), never interpolated — and in any case run as the
  project's own guest user inside that user's own microVM, where the user
  already has a shell, so no boundary is crossed. Exactly one host-side
  `exec.Command` exists (`internal/gitrepo/gitrepo.go:42`), bootstrap-only
  (`init`/`rev-parse`), hardened with `-c core.hooksPath=/dev/null`, `-c
  protocol.ext.allow=never`, `-c uploadpack.packObjectsHook=`, `-c
  core.fsmonitor=`. Caddyfile generation sanitizes hostnames
  (`sanitizeHostname`, DNS-label charset, plus a reject for a name that
  sanitizes to nothing) and emails (`emailRe`, rejecting whitespace, braces,
  quotes and `#` — the bytes that would close a `transform user` block early)
  before either is written verbatim into config. Ports are `Atoi`-parsed and
  range-checked at every entry (`sharePort`, `ParseLabels`). The two low
  findings above are the residue of this otherwise complete treatment.
- **Authz/authn** — `config.Load` fails closed four ways: `STYRMAN_AUTH=none`
  requires a loopback `STYRMAN_ADDR` (`config.go:215`); an unknown mode is
  rejected (`:220`); `STYRMAN_AUTH=header` **refuses to start** without
  `STYRMAN_INGRESS_TOKEN` (`:228`), closing the spoof path a project VM on a
  shared bridge would otherwise have against `:8080`; and a non-VM
  `STYRMAN_RUNTIME` is refused (`:252`) because project containers get
  `CAP_SYS_ADMIN`/`NET_ADMIN`/`NET_RAW` and `apparmor=unconfined`
  (`orchestrator.go:1062-1067`), which is containment only inside a Kata
  microVM. `authGuard` repeats the unknown-mode refusal as a backstop and
  compares the ingress token with `crypto/subtle.ConstantTimeCompare`. A Google
  account that is not a styrman user gets `Authorized:false` and an **empty**
  `Username`, and `previewAllowed` requires `u.Authorized && ownerUser ==
  u.Username` for the owner branch, so a slug collision can never be an
  ownership claim. The isolation key is guarded at its source: `userstore.New`
  refuses to start when two authorized emails slug to the same
  `identity.DeriveUsername`, and `slugCollision` repeats the check on add.
  `serveLink` refuses a private token on the unauthenticated `/link/` route
  (same 404 as unknown — no oracle) and bounds a public exchange by the link's
  expiry; `serveShared` re-reads the live share list so revoking an email cuts
  access without rotating the token. Every WebSocket entry point applies
  `terminal.SameOriginOrAbsent` (`bridge.go:117,199`, `browser.go:332`) and
  mutations get an Origin check (`server.go:625-639`). Broker callers are
  identified by bridge position with a verify-on-cache-hit re-inspect
  (`orchestrator.go:557-571`) that re-proves the owner label *and* that the
  container still holds that IP, so a reused subnet cannot serve the previous
  tenant's credentials.
- **Crypto** — nothing hand-rolled; no finding. Link tokens are 128 bits from
  `crypto/rand`, hex-encoded (`internal/links/store.go:96-102`); project IDs are
  `crypto/rand` v4 UUIDs (`internal/project/uuid.go`). `crypto/sha1` appears
  once (`internal/config/config.go:502`) purely to derive a 15-character Docker
  bridge *interface* name — a naming function, not a security control, and its
  collision resistance is irrelevant to the `sdb-`/`lan-` prefix match the
  firewall performs. Sessions, JWTs and the whole OIDC flow are delegated to
  caddy-security + Google; the generator only pins the signing key (`crypto key
  sign-verify {env.STYRMAN_JWT_KEY}`), the 8 h lifetime (ADR-063), `cookie
  samesite lax`, and the label-bounded `.` -prefixed redirect-suffix trust that
  closed the open redirect. No `math/rand` on any security path (its one use is
  `internal/conversation/names.go`, friendly terminal names).
- **Dependencies** — `govulncheck ./...` ran bare on both modules with the
  host's go1.27.1, exit 0 from the wrapper shell, output read in full: root
  module 2 called vulnerabilities (GO-2026-4887, GO-2026-4883) from
  `docker/docker@v28.5.2+incompatible`, 0 in imported packages, 3 in
  required-but-uncalled modules; the ingress module the same 2 called plus 7
  uncalled. Both IDs are the ones `deploy/vulncheck.sh` already documents as
  accepted with no fix available. `npm audit --omit=dev` in `web/`: 1 high
  (js-yaml, above). Caveat on method: `deploy/vulncheck.sh` runs the scan inside
  the *pinned builder image*, which is the stronger measurement; this review
  used the host toolchain and did **not** run the wrapper (its Docker
  build/pull side effects are out of scope for a read-only review). The finding
  set matches what the wrapper documents. No new dependency is introduced by
  anything proposed here.
- **GCP surface** — `deploy/gcp/vm.sh:77` creates the VM with
  `--no-service-account --no-scopes`: no attached identity, so no IAM blast
  radius from the instance, and no service-account key file anywhere in the
  tree (`gcloud iam service-accounts keys create` appears nowhere). ssh is
  IAP-tunnelled by default with the public path an explicit opt-in
  (`vm.conf:23`, and `.agents/rules/remote-check-names-transport-first.md`
  records why), and `lib.sh:102` carries a `PUBLIC = {"0.0.0.0/0", "::/0"}`
  check. No Cloud Run and no `--allow-unauthenticated` anywhere. Install-time
  secrets reach the VM as files under `$HOME/.styrman-install/` passed to
  `styrman-ctl install` (`install.sh:165`) rather than as instance metadata or
  process env — appropriate for a Compute Engine target with no Secret Manager
  dependency. Read by targeted grep, not in full: see below.

## Not examined

- `internal/files/` was **not** read. The guest-writable-path guards were
  traced only to their call sites: `files.Trust(cfg.HomesDir,
  StateDir/projects)` at `httpapi/server.go:206`, and
  `projectHomeIsReal`/`realBelow` at `orchestrator.go:344` (before the
  create-or-start switch, so it covers both paths), `:411` (re-check after
  `mkdirAllConfined`, since `MkdirAll` succeeds through a symlink-to-dir),
  `:708` and `:758` (the setup VM's home). Those four are the only call sites
  and they sit on the paths the rule names. What was **not** verified:
  `openRoot`'s own confinement logic, the `DirEntry.Info()` prohibition
  (BUG-004), and whether any reader in `internal/history/` or
  `internal/httpapi/explorer.go` reaches a home path outside a confined root.
  Highest-value next target.
- `deploy/gcp/install.sh` and `lib.sh` beyond the IAM/firewall/secret greps —
  the bring-up logic, its ssh handling, and `styrman-ctl install`'s own
  argument handling were not read line by line.
- `internal/httpapi/` handlers not opened: `explorer.go`, `git.go` (beyond the
  argv grep), `credentials.go`, `credharvest.go`, `setup.go`, `network.go`,
  `provision.go`, `browser.go` (beyond the origin check), `profile.go`,
  `projectenv.go`, and the route table itself — so which paths
  `isPreviewPath`/`isSharedPath` actually admit for an *unauthorized* Google
  account was reasoned from the two prefix functions, not from the mux.
- `internal/credstore/`, `internal/conversation/`, `internal/terminal/` bodies;
  `cmd/styrman/main.go` beyond the `ip_forward` block and the legacy-layout
  migration.
- `deploy/firewall/styrman-firewall.sh` read to line ~90 of 9 kB (the base
  isolation rules and the `xt_physdev` warn path); its cleanup/migration tail,
  `install.sh`'s tail, and `test.sh` were not read. `deploy/host-setup.sh`,
  `deploy/styrman-ctl`, and `deploy/Dockerfile.server` not opened.
- `deploy/caddy/ingress/watcher.go`, `cloudflare.go`, `cmd.go` — the admin
  `/load` path, the Docker event loop, and the Cloudflare token handling. Only
  `generator.go`, `labels.go`, `loopback.go`, `config.go`, `module.go` were
  read, and `generator.go` lines 1-79 were skipped.
- `web/` application code, and `image/entrypoint.sh` / `image/configs/*` (the
  in-guest credential materialization the broker feeds, including
  `styrman-credentials.sh` and `styrman-git-credential`) — not read.
- No test suite was run and `deploy/vulncheck.sh` was read but not executed.
