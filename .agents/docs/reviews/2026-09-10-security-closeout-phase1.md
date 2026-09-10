---
owner: security
date: 2026-09-10
---
# Security review — TODO close-out phase 1 (2026-09-10)

Range reviewed: working tree vs `HEAD` (0378825), i.e. the full uncommitted
`git diff` plus the untracked files it depends on.

**The dispatch pointed at `.agents/scratch/phase1.diff`; that snapshot is
STALE.** It carries 10 files / 346 insertions. The actual working tree carries
29 tracked files (414 insertions) plus 19 untracked ones, and the source of two
of the five nominated surfaces has moved on since the snapshot was taken. Two
surfaces the snapshot does not mention at all are in the blast radius and were
reviewed: `internal/httpapi/server.go` (a new `provisioning sync.Map` and
`deriveState`) and `web/` (`src/lib/sshKey.ts`, `package.json`, a new
`scripts/normalize-shell.mjs` build step, and a regenerated, `go:embed`-ed
`dist/client`). Everything below was read from the LIVE files, not the snapshot.

The most consequential drift: the snapshot's `gitConfigValue` writes an escaped
character literally, so no value could ever contain a control byte. The live one
decodes git's `\n`, `\t` and `\b` into RAW 0x0A / 0x09 / 0x08. Finding 1 exists
only in the live code.

## Verdict: FINDINGS

Nothing here is exploitable across a tenant boundary, and the two nominated
concurrency questions adjudicate CLEAN: `unmarkIfStillRunning`'s
`CompareAndDelete` on the stop's timestamp does correctly refuse to clear a
NEWER stop's marker, and `setProjectNetwork` moved only the response WRITE out
of the project lock — every state-changing step is still inside it. The worst
real defect is a false claim in a comment that a reviewer will act on:
`git.go:81` says "the settings card (toolDetails) uses the same parser so the
two cannot disagree", and `setup.go:224-232` demonstrably uses a different,
laxer, unsanitised parser with a 32x larger read cap. That neighbouring block
still echoes a raw `.gitconfig` line — bidi overrides and all — into
`/api/settings/tools`, which is exactly the hardening the ssh line eight lines
below it just received. Same-user only, hence low, but it is the same weakness
the diff set out to close, left in place behind a comment saying it was closed.

## Findings

- **[low / confidence: high]** `internal/httpapi/setup.go:222-232` — `toolDetails`
  parses the git identity with `strings.HasPrefix(line, "name = ")` over the
  WHOLE `.gitconfig`, ignoring section headers, and appends the raw line to
  `git.Details` with no `stripControls`. Three concrete consequences.
  (i) The comment at `git.go:81` asserts this call site shares
  `gitUserFromConfig`; it does not, so a future reviewer trusting the comment
  will not look. (ii) They disagree in reachable cases: `[github]` + `name = X`
  shows in the settings card but is never the commit author; a quoted
  `name = "Ada"` shows WITH the quotes but commits as `Ada`; `read(".gitconfig")`
  passes `0` for max, which `files.ReadRaw` widens to `MaxReadBytes` (2 MiB),
  while `gitIdentity` caps at 64 KiB — a `.gitconfig` between those sizes makes
  the card show an identity the committer will not use. A user consults that
  card precisely to confirm their commits are attributed correctly. (iii) The
  echoed line is unsanitised: a U+202E in `~/.gitconfig` reaches the JSON and
  the settings row verbatim, the same bidi-spoof the ssh row was just hardened
  against. Remediation: call `gitUserFromConfig` here and render the two parsed
  values, or at minimum wrap the appended line in `stripControls` + `clampBytes`
  and align the read cap; then make the comment true or delete it.

- **[low / confidence: high]** `internal/httpapi/git.go:132-160` (`gitConfigValue`)
  — the live parser decodes git's escapes into RAW control bytes: `\n` becomes
  0x0A, `\t` 0x09, `\b` 0x08. `gitIdentity`'s result is handed to
  `credstore.PutEnv` (`credharvest.go:53-54`), whose value reaches the broker's
  `GET /env` and is consumed by `image/configs/styrman-credentials.sh:39-50` as
  NEWLINE-delimited, TAB-separated records fed to `while read -r __k __v` and
  then `export "${__k}=${__v}"`. An LF inside the value is therefore a RECORD
  SEPARATOR to that loop: `name = "Ada<esc-n>PATH<esc-t>/tmp/x:/usr/bin"` in the
  base home would export an attacker-chosen variable into every login shell of
  every project belonging to that user. **This is currently blocked**, one layer
  down, by `credstore.validateEnv` -> `hasCtrl` (`credstore.go:159, 449`), which
  rejects any rune below 0x20 or 0x7f; the harvest then logs a Warn and reports
  `git-identity: error`. So the finding is that the producer now manufactures
  exactly the byte class the consumer's guard exists to stop, and the only thing
  standing between them is a guard in a third package that nothing in this diff
  references. It is also a silent-identity-loss path: one escaped newline in a
  `.gitconfig` and NO project gets an author identity. Note `hasCtrl` does not
  cover Unicode Cf, so a U+202E in `user.name` still passes, is stored, is
  exported, and is applied by `git config --global user.name`. Remediation:
  reject or `stripControls` the parsed value in `gitIdentity` before returning
  it — the file already has the helper — and keep the escape decoding for
  fidelity only if a caller needs it.

- **[low / confidence: high]** `web/scripts/normalize-shell.mjs:18` +
  `web/dist/client/_shell.html` — the new build step rewrites the prerendered
  router payload's `u:<13 digits>` to the literal `u:0000000000000`, which is a
  legacy OCTAL literal, not a decimal zero. Verified: `node -e` in sloppy mode
  yields `0`; `node --input-type=module` yields
  `SyntaxError: Octal literals are not allowed in strict mode`. It works today
  only because that inline `<script>` has no `type` attribute and is therefore a
  sloppy-mode classic script. Adding `type="module"`, a `"use strict"`, or a
  framework upgrade that emits the payload as a module turns the whole embedded
  app shell into a parse error — a hard availability break in a file that ships
  inside the Go binary via `go:embed` and is regenerated by a build step nobody
  will re-read. Emit `u:0`. Secondary: the script takes the FIRST `u:\d{13}`
  match and exits non-zero only when there is none; it does not assert the match
  is unique or that it sits inside the router payload, so a second such
  substring appearing earlier in future prerender output would silently rewrite
  the wrong bytes of executable JS.

- **[low / confidence: high]** `cmd/styrman/main.go` / `internal/config/config.go`
  — deleting `migrateWorkspaces` and `LegacyWorkspaceExists` also deletes the
  last SIGNAL. For the deployment `AGENTS.md` describes (single host, migrated
  at v1.19.x) there is no residual: `homes/<u>/base/projects/<dir>` does not
  exist, so `ensureRepos`/`ensureRunningLocked` cannot shadow anything.
  For any OTHER host the residual is concrete: `ensureRepos` now
  `os.MkdirAll`s `projects/<u>/<uuid>` and `git init`s it, and
  `ensureRunningLocked` binds it at `~/projects/<dir>`, so the project boots with
  an empty tree while the real one sits untouched at
  `homes/<u>/base/projects/<dir>`. `.agents/docs/deploy/remote.md` documents the
  manual move, but its sentence "the server has logged a benign WARN at every
  start for the missing path ever since" describes the PRE-change behaviour —
  I grepped `cmd/` and `internal/` and after this change no warning, error, or
  guard mentions the legacy workspace path at all. A host in that state now gets
  silence. Mitigation that should be stated in the doc: the legacy tree stays
  reachable from the SETUP VM (which mounts `base/`), so it is recoverable
  without host access. Remediation: either keep a single boot-time WARN when
  `homes/<u>/base/projects/<dir>` is a non-empty directory, or correct the doc
  sentence to say there is no longer any signal.

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:1143-1178`
  — adjudicating the nominated question: a NEWER stop's marker CANNOT be cleared
  by an older goroutine. The marker value is that stop's `time.Now()`, `time.Time`
  is `==`-comparable (so `sync.Map.CompareAndDelete` neither panics nor
  mis-matches), two `time.Now()` calls are never equal, and a Start in between
  deletes the key so the CAD finds nothing. I also checked the type change from
  `bool` to `time.Time` against every reader: `Stopped()` (line 1183) tests key
  PRESENCE only and never type-asserts, and lines 442 and 1373 only `Delete`.
  No panic path. The residual is the one the comment already names and does not
  fully close: two samples 2 s apart is not proof, so a `ContainerStop` still in
  flight more than ~4 s past the error returns "Running" twice, the marker is
  cleared, the container then goes down, and an open tab's terminal reconnect
  boots it back — BUG-030's resurrection, narrowed but not eliminated. This is
  an availability/UX property, not an authorization boundary: I traced the
  network-mode toggle and a resurrection there brings the project up on the mode
  its RECORD still names (`persist()` runs after `pr.Stop`, so a failed stop
  leaves the record unchanged), never on a mode an admin has revoked. Second
  residual, low: each failed stop spawns one unbounded-in-count goroutine living
  ~12 s and issuing 2 `ContainerInspect` calls; `stopMarked` serialises per
  project, so amplification needs many projects. Remediation, if it is worth
  anything: sample until the container is observed NOT running or the budget
  expires, and keep the marker on expiry.

- **[informational / confidence: high]** `internal/httpapi/server.go:154, 849, 933`
  — `provisioning sync.Map` is keyed by project ID ALONE, while its two
  neighbours (`deletingState`, `provisionError`) are keyed by user+ID. IDs are
  server-generated UUIDs, so there is no practical collision and the value
  exposed is a boolean the owner already knows; the ownership check still
  happens in `getProjectFor` before `isProvisioning` is consulted. Flagged only
  because the asymmetry invites a future caller to assume the map is
  tenant-scoped. Lifecycle is correct: `Store` happens before the `go`
  statement and `defer s.provisioning.Delete` is registered first, so it runs
  LAST (after the recover defer) on every return including a panic.

## Checked

- **Secrets** — no credential, token, or key added in the diff; no `.env`,
  `*.pem`, `*.key`, or credential JSON among the 29 tracked + 19 untracked
  paths. The removal of `exec.Command("git", "config", "--file", …)` from
  `gitIdentity` removes a path that could put a home path into a process
  table. `credharvest.go:59` logs only `user` and the two error values, never
  the identity. Secret VALUES are not logged on any changed line.
- **Injection** — commit author reaches the guest as direct argv
  (`gitExec` -> `orchestrator.Exec` -> `ContainerExecCreate` with a `Cmd`
  slice), no shell, so `-c user.name=<value>` cannot break out of its argv
  element; git's own ident formatter strips control bytes from the commit
  object. The one live injection MECHANISM is the broker-env-to-login-shell TSV
  loop in finding 2, blocked today by `credstore.hasCtrl`. `publicKeyLine` I
  tried to bypass and could not: the type is an anchored enum regex (so a bare
  `ssh-` prefix and an `sk-ant-…` API token are both rejected), the body is
  restricted to the base64 alphabet, and the comment goes through
  `stripControls` (which removes Cc, Cf including RLO U+202E and ZWSP U+200B,
  and U+2028/U+2029) and then `strings.Fields`/`Join`, whose `unicode.IsSpace`
  covers NBSP, NEL, U+2000-U+200A, U+202F, U+205F, U+3000 and U+1680 — so the
  output can hold only single ASCII spaces. Leading `\t` is normalised to a
  space BEFORE the `Cut`, so a tab-separated `.pub` parses. The enum omits
  OpenSSH's `*-cert-v01@openssh.com` types, which is correct here because the
  only file read is the fixed `.ssh/id_ed25519.pub`. The `web/src/lib/sshKey.ts`
  regex now genuinely matches the Go enum (I diffed both), and the regenerated
  `dist/client/assets/settings-DfdwTlfF.js` contains the TIGHT regex, so the
  embedded bundle is not stale against its source. No SQL (no database), no
  template rendering, no path built from client input on any changed line.
- **Authz/authn** — no auth check added, removed, or reordered. In
  `setProjectNetwork` the 404-before-authorization ordering, the admin gate on
  ENABLE only, and the delete-in-flight conflict all still precede
  `WithProject`; converting the writes to a `reply` value moved only the
  response write. `deriveState` cannot expose another tenant's project because
  both call sites resolve ownership first. `unmarkIfStillRunning` touches no
  identity.
- **Crypto** — nothing hand-rolled and nothing cryptographic added. The
  `.gitconfig` and `.pub` parsers are text parsers, not primitives; the ingress
  token / `X-Caddy-Email` trust path is untouched.
- **Dependencies** — no new Go module and no new npm package; the one new
  runtime dependency is `web/scripts/normalize-shell.mjs`, ~20 lines using only
  `node:fs`, which satisfies stdlib-first. `govulncheck ./...` RAN on both
  modules (root and `deploy/caddy/ingress`), exit 3 on each, symbol hits only in
  the STANDARD LIBRARY at the local toolchain `go1.25.0`: GO-2026-6218
  (`net/url`), GO-2026-6090 (`crypto/tls`), GO-2026-6089 (`net/http`),
  GO-2026-5972 (`encoding/asn1`), all "Fixed in go1.25.13". The SHIPPED binary
  is built in the pinned `golang:1.25.13` builder that `deploy/vulncheck.sh`
  extracts from `deploy/Dockerfile.server`, where these are fixed; I did not run
  the containerised script (docker not exercised in this review), so treat the
  authoritative number as `bash deploy/vulncheck.sh`. `go.mod` still declares
  `go 1.25.0`. `npm audit` in `web/` RAN: 1 high, js-yaml 4.0.0-4.3.1
  GHSA-2883-xcg3-v3hh, transitive via
  `@tanstack/react-start -> @tanstack/start-plugin-core -> xmlbuilder2`,
  i.e. build-time only and not in the shipped bundle. Both results are
  PRE-EXISTING; nothing in this diff introduced or worsened them.
- **GCP surface** — none. This project is self-hosted (`docs/deploy/gcp.md`
  records it as not applicable); no IAM binding, service account, Cloud Run
  flag, or Secret Manager reference appears in the diff.
- **File integrity (BUG-005/006/007)** — `git diff --numstat` on every changed
  path returns real line counts, no `-`/`-` binary pair. `cmd/styrman/migrate_test.go`
  shows `0 / 88`: that is the legitimate removal of `TestMigrateWorkspaces`
  alongside the function it tested, not an overwrite — the file is ` M`, the
  three other tests in it survive, and two new test files were added under
  non-colliding names (`gitconfig_test.go`, `state_test.go`). It does leave a
  dangling trailer comment ("restored from v1.20.0 …") with nothing after it.
  `web/dist/client/_shell.html` contains a NUL at offset 7417 inside
  `{i:"__root__\0"}`; it is the framework's, pre-existing, and `.gitattributes`
  already forces a textual diff for it, so it was readable in review. `go build`,
  `go vet` and `go test ./...` are all green.

## Not examined

- `deploy/caddy/ingress` beyond `govulncheck` — the diff does not touch it and
  its tests were not run (`(cd deploy/caddy/ingress && go test ./...)`).
- The web test and lint tiers: `npm run check`, `npm run lint`, `npm test` were
  NOT run, so `web/src/lib/sshKey.test.ts`'s 21 added lines are unverified as
  passing; I read them, I did not execute them.
- `bash deploy/vulncheck.sh` (the pinned-builder govulncheck) — not run; the
  govulncheck numbers above are from the host toolchain.
- The runtime image was not rebuilt and `image/configs/styrman-credentials.sh`
  was read, not executed. Finding 2's shell mechanism is traced by reading, and
  its blocker (`hasCtrl`) is traced by reading; neither was demonstrated at
  runtime, and by policy no exploit was constructed.
- Integration and e2e suites (`.agents/docs/testing/styrman/integration.sh`,
  `web/e2e/run.sh`) — not run; no Kata VM was booted.
- The 19 untracked regenerated `web/dist/client/assets/*.js` bundles were spot
  checked for the ssh-key regex only. They are minified build output and were
  not otherwise read.
- The three untracked `.agents/docs/` artifacts (changes, review, testing notes)
  were not read, deliberately: a reviewer reads the diff, not the author's
  account of it.

## Response (main, 2026-09-10)

- **Finding 1 — fixed.** `toolDetails`'s git card now calls `gitIdentity`
  (the same `gitUserFromConfig` parse, 64 KiB cap, control characters
  stripped) and renders `name = …` / `email = …` from the parsed values;
  the prefix scan and the raw echo are gone; `Ready` = email present.
- **Finding 2 — fixed.** `gitUserFromConfig` strips control characters from
  both values on return, so a decoded `\n`/`\t`/`\b` never reaches `git -c`
  argv or the broker's TSV. The escape-decoding test now expects the
  stripped result and says why.
- **Finding 3 — fixed.** `normalize-shell.mjs` emits `u:0`; the dist is
  rebuilt and the page's script is re-checked by the e2e run.
- The stale-snapshot note is right: the diff file predated Phase 2's edits on
  the same tree; the live tree is what was reviewed and what is committed.
