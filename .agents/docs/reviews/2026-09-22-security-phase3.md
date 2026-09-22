---
owner: security
date: 2026-09-22
---
# Security review — phase3   (2026-09-22)

Range reviewed: commits 49eb863..0024de7 (git log 49eb863..0024de7); the diff is git diff 49eb863...0024de7

## Status: COMPLETE

## Verdict: FINDINGS (all low; the diff is a net improvement)

All four questions in the brief come back clean, one of them only after I had
written the opposite down and disproved it against Caddy's own source (see
"Disproved" below). `private_file` cannot be made weaker than either original
by any caller that exists; `env_get`'s `is_key` + awk-variable pair is
injection-free for every character class named, newline included; the
`X-Caddy-Role` strip is correctly positioned and the identity headers beside
it really are unconditionally overwritten; and the `credMu` split removes no
invariant `deletingMu` was carrying. What remains is three low items, the
sharpest being that the change entry's and `lib.sh`'s claim that a test
"asserts the two copies still agree" is true for `env_get` and false for
`private_file` — the function whose drift in "two of five checks" motivated
the whole merge is the one left unguarded.

## Findings

- **[low / confidence: high]** `deploy/gcp/lib.sh:37-45` vs `deploy/styrman-ctl:525-537`
  — the new header comment ("the duplication is deliberate, and
  `internal/config/gcp_scripts_test.go` asserts the two copies still agree
  rather than trusting them to") and the change entry's "Learnings for the
  local host" both overstate what is guarded. `TestGCPLibEnvGetMatchesStyrmanCtl`
  compares only `env_get`'s awk line and `is_key`. `private_file` remains two
  independent bodies: `styrman-ctl`'s unchanged one-argument `root-or-self` /
  `022` form, and `lib.sh`'s new parameterised one. A future hardening of
  either copy will silently not travel — the exact failure the phase was
  written to end, and the shape `.agents/rules/shared-path-learnings.md` names
  (what a second target teaches about the shared path is fixed in the shared
  path). Remediation, cheapest first: extend the drift test to compare the
  `private_file` body the way it compares the awk line, or have `styrman-ctl`
  call the parameterised form (`private_file "$f" root-or-self 022`). At
  minimum, correct both descriptions to say `env_get` and `is_key` only —
  a guarantee that names more than it covers is worse than none.
- **[low / confidence: high]** `deploy/gcp/lib.sh:47` (`parent_mask="${3:-077}"`,
  used at `8#${parent_mask}`) — `PARENT_MASK` is the one new parameter with no
  validation, and `0`, `00` or `000` makes the parent-directory check a
  tautology (`pmode & 0 == 0` always), i.e. weaker than either original, with
  no error. Verified by probe: empty string and unset both fall back to `077`
  (safe, `:-` treats empty as unset); `0`/`00`/`000` accept a 755 parent
  silently; a non-octal digit (`8`, `9`) makes bash abort the arithmetic and,
  under `set -euo pipefail`, kills the script — fail-closed. No caller passes
  a mask today (`install.sh:61` passes none), so this is not currently
  reachable; it is recorded because the function has just become a
  general-purpose parameterised helper that invites a caller with a computed
  mask, and because a value interpolated into `$(( ))` is parsed as an
  arithmetic *expression*, not as digits — bash's evaluator recursively
  evaluates variable values, so a mask sourced from outside the script would
  be an evaluation surface, not merely a wrong number. Remediation: one guard
  beside `is_key`, e.g. `[[ "${parent_mask}" =~ ^[0-7]{3}$ ]] || die` (which
  also rejects `000`), mirroring the `case`-with-`*) die` the owner policy
  already has.
- **[low / confidence: high]** `internal/httpapi/credharvest.go:192-206` —
  `tryAcquire(m map[string]bool, …)` still takes the map as a parameter but now
  hardcodes `s.credMu`, where before the parameter and the lock were at least
  consistent for every map in the file. A future caller passing `s.deleting`
  (guarded by `deletingMu`) compiles, type-checks, and silently protects the
  map with the wrong mutex — a data race the race detector only finds if a
  test exercises that exact interleaving. The split itself is right; the
  generic signature is the residue. Remediation: drop the parameter
  (`tryAcquireCredPush(user, projectID string)` closing over `s.credPushing`),
  which is also shorter than defending it with a comment.

### Dependencies (pre-existing, not introduced by this range)
`govulncheck` reports 2 vulnerabilities your code calls, in BOTH modules, from
one dependency: `github.com/docker/docker@v28.5.2+incompatible` —
**GO-2026-4887** and **GO-2026-4883**, both `Fixed in: N/A`. No upstream fix
exists, nothing in this diff changed a dependency, and no action is available
beyond tracking; recorded so the release gate does not read `rc=3` as new.

## Disproved (written down, then checked)

I first recorded a **medium** finding: that `request_header X-Caddy-Email
"{http.request.header.X-Token-User-Email}"` in `writeAppBlock`'s non-public
branch and in `writePrivateWildcard` is preceded by no strip of a
CLIENT-supplied `X-Caddy-Email`/`-User`/`-Subject`, so if the placeholder
resolved empty and Caddy skipped an empty `Set`, the client's own identity
header would survive to a server that derives all authorization from it
(`internal/httpapi/auth.go:64`). Caddy v2.11.4's
`modules/caddyhttp/headers/headers.go:243-252` settles it: the `set` loop
calls `hdr.Set(fieldName, strings.Join(newVals, ","))` **unconditionally** —
no empty-value skip — so the client's value is overwritten either way, and an
empty or literal-placeholder result is refused by the server (401 on empty,
403 on a non-authorized string). The role header genuinely was the only gap on
that path, and the fix closes it. Ordering is sound for the same reason: each
`request_header` line becomes its own handler inside `route {}`, executed in
written order, so the unmatched strip runs before the matcher-scoped sets;
within a single `HeaderOps`, `Delete` would run after `Set`, but that case does
not arise here (different field names, different matchers).

## Checked

- **Secrets** — no credential, token or key added. The diff moves the secret
  FILE gate (`~/.config/styrman/gcp/{env,gh-token,allowed_signers}`) without
  loosening it: defaults `self`/`077` reproduce `install.sh`'s prior behaviour
  exactly. `die` messages print modes and owners, never file contents; the
  probe scripts in `gcp_scripts_test.go` write a literal `"x"`, no real secret.
- **Injection** — `env_get` probed directly: `is_key` refuses `A/B`, `A"B`,
  `A$B`, `A B`, `*`, `''`, tab, `A.B`, `A\rB`, and both `BASE_DOMAIN\nEVIL`
  and a trailing `\n` (bash `=~` anchors to string ends; `[A-Z0-9_]*` cannot
  span a newline). The key then travels as an awk `-v` variable matched with
  `index($0, k "=") == 1` — never interpolated into a program — and the
  removed fork's `grep -E "^$2="` regex interpolation is now asserted gone by
  test. `styrman-ctl __env-get` (argv-supplied key, the only externally-fed
  caller) is guarded by the same `is_key`. No new shell/SQL/path sink.
- **Authz/authn** — see "Disproved": the `X-Caddy-Role` strip is correct,
  ordered correctly, and the remaining identity headers on that path are
  genuinely overwritten. `HeaderIngress` is stripped unconditionally in all
  four blocks before being re-set. The server-side guard
  (`internal/httpapi/auth.go`) rejects an empty `X-Caddy-Email` and
  constant-time compares the ingress token — unchanged by this diff.
- **Crypto** — nothing hand-rolled, nothing touched. No primitive, MAC, token
  format, session or auth flow implemented or composed in this range.
- **Dependencies** — no dependency added or bumped (`go.mod`/`go.sum`/
  `package.json` untouched in the range). `govulncheck ./...` run bare in both
  modules, exit status read: root `rc=3`, ingress `rc=3`, same two
  unfixed `docker/docker` advisories in each (above). `npm audit` not run — no
  JS/TS file is in the diff or its blast radius.
- **GCP surface** — no IAM binding, service account, `--allow-unauthenticated`
  or key file changed. The change is to the operator-side secret gate for
  `deploy/gcp/install.sh`; secrets stay in mode-600 files under a 700
  directory on the operator's machine and are handed to the build as a
  `--secret`, not an env var. `R3` (per-project home, credentials only via the
  broker) is untouched by the `credMu` split: the push path, its single-flight
  semantics and its rerun coalescing are byte-for-byte the same logic on a
  different mutex.
- **The lock question specifically** — `deletingMu` is held only around map
  mutations, never across `Destroy`, `Adopt` or `Exec`; `tryAcquire` had
  exactly one caller (`s.credPushing`); no code reads a cred map under
  `deletingMu` or a lifecycle map under `credMu`. The shared lock therefore
  never serialised a push against a delete over any window that mattered — it
  was incidental, not load-bearing. The push-into-a-dying-project race
  (`Adopt` lists it, `Destroy` removes it, `Exec` fails) existed identically
  before, and `sweepProjectCredentials` still runs *after* `Destroy` returns,
  so no credential can be re-materialised into a swept home.
  `go test -race -run 'Cred|Push|Delete' ./internal/httpapi/` → exit 0;
  `go test ./internal/config/ ./internal/httpapi/` → exit 0;
  `(cd deploy/caddy/ingress && go test ./...)` → ok.

## Not examined

- The **fresh-host path was not exercised** by this review or by the change
  entry, which records "Not run: Tier 2, Tier 4, container builds — Phase 5".
  `deploy/gcp/install.sh` IS the fresh-host caller of the merged helpers;
  per `.agents/rules/shared-path-learnings.md` the release carrying this must
  either run `vm.sh create` + `install.sh` against a new VM or say
  "fresh-host path unexercised" in the gate report. I read the scripts, I did
  not run them against a VM.
- Caddy behaviour was verified against the **vendored v2.11.4 source in the
  module cache**, not against a running ingress: no generated Caddyfile was
  fed to `caddy validate` or to a live server in this review, and the new
  test asserts byte order in the generated text, not Caddy's applied order.
- `npm audit` / `web/` — not run, no JS in the blast radius.
- `internal/httpapi/server.go` outside the `credMu` declaration and the
  delete path (lines ~985-1125), and `deploy/styrman-ctl` outside `env_get`,
  `is_key` and `private_file`, were not read in full.
- `GO-2026-4887` / `GO-2026-4883` were recorded by ID and "Fixed in: N/A";
  I did not read the advisory text to judge whether the called symbols are
  reachable from styrman's own use of the Docker client.

## Outcome

Appended by the main agent, in the commit that carries the fixes. All three
findings accepted; none rebutted.

- **Finding 1 — the drift guard covered only `env_get` while the comment and
  the change entry claimed it covered `private_file`: fixed**, and the finding
  is exactly right that this was the worst of the three, because the
  overclaim was about the one function whose drift motivated the merge.
  `TestGCPLibEnvGetMatchesStyrmanCtl` now also compares `private_file` across
  both copies by the five checks it makes (symlink refusal, owner read, file
  mode read, the `&077` file-mode check, parent mode read) and pins the one
  value that differs on purpose — `styrman-ctl`'s `022` against `lib.sh`'s
  `077` default — so the reasoning recorded for that difference cannot rot
  without a test failing. Proved by mutating `styrman-ctl`'s parent mask from
  `022` to `077`: valid bash, and the test names it
  ("deploy/styrman-ctl's private_file no longer uses parent mask 022").
  `deploy/styrman-ctl` was restored from a checksummed copy and verified
  byte-identical. Both descriptions corrected: `deploy/gcp/lib.sh`'s comment
  and `changes/2026-09-22-review-remediation-phase-3.md`, the latter carrying
  a note saying what it used to claim.
- **Finding 2 — `PARENT_MASK` unvalidated: fixed** at `deploy/gcp/lib.sh`.
  The mask must match `^[0-7]{3}$`, and `000` is refused by name as the
  tautology it would be. Unreachable today, as the finding says; the guard is
  there so no future caller can get it wrong silently.
- **Finding 3 — `tryAcquire` took a map parameter while hardcoding `credMu`:
  fixed** at `internal/httpapi/credharvest.go`. The parameter is gone and the
  function operates on `credPushing` directly, with the trap recorded in its
  doc comment. Its one caller and its four test call sites updated;
  `go test -race ./internal/httpapi/` → 0.
- **The disproved medium finding is noted with thanks.** Writing down an
  `X-Caddy-Email` empty-placeholder hazard, then checking Caddy's own source
  and disproving it, is the behaviour the review standards ask for — the
  report's "Disproved" section is more useful than a finding would have been.
- **The two `docker/docker` advisories** (GO-2026-4887, GO-2026-4883, both
  `Fixed in: N/A`) are pre-existing, outside this range, and already accepted
  at `deploy/vulncheck.sh:9-13`. Left.
- **The fresh-host path was not exercised against a VM**, and the review is
  right that `.agents/rules/shared-path-learnings.md` wants that stated rather
  than implied. It is stated in this phase's change entry and will be stated
  again in the release gate report.

Verification after the fixes, bare: `go vet ./...` → 0; `go test ./...` → 0;
`go test -race ./internal/httpapi/` → 0; `(cd deploy/caddy/ingress && go test
./...)` → 0; `bash -n` and `shellcheck -S warning` over `deploy/gcp/*.sh` → 0.
