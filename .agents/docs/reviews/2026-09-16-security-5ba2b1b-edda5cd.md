---
owner: security
date: 2026-09-16
---
# Security review — harness channel, gate fix rounds 2 and 3 (5ba2b1b..edda5cd)   (2026-09-16)

Range reviewed: 5ba2b1b..edda5cd

**Scope.** Four commits (2f86816, 5ba2b1b, e3aef79, edda5cd) applying the findings of
`reviews/2026-09-16-security-567b81f-43dc991.md`, `reviews/2026-09-16-security-43dc991-5ba2b1b.md`
and the three code reviews (`-43dc991-5ba2b1b-code.md`, `-5ba2b1b-2f86816-code.md`,
`-2f86816-e3aef79-code.md`). This is a DELTA review — does each security-relevant fix do what
its Outcome claims, is it complete, did it introduce anything new — not a re-review of ADR-073.
Tree state: HEAD is `edda5cd`, and every reviewed path is clean in the worktree
(`git status --short` shows only ` M README.md` plus three untracked docs), so the files read are
the range's head. Nothing was edited but this file.

## Verdict: FINDINGS

Every security-relevant fix in the range is **correct and complete**, and I found nothing new of
medium or higher severity. The two sharpest fixes hold under measurement: the origin gate is now
bash's whole-string `[[ =~ ]]` and refuses all three values the previous per-line `grep` accepted
(measured side-by-side — `ext::sh -c id\nhttps://…`, `https://…/x/y\nhttps://…`, and a plain
trailing newline: `grep` rc=0, `[[ =~ ]]` false for each), which also closes a second door nobody
named — a multi-line origin was being interpolated into the `credential.<host>.helper` **config
key** as well as into argv; and the credential-helper snippet git now receives is a **constant**
(`… "$(cat "${STYRMAN_HARNESS_GH_TOKEN_FILE}")" …`), with the path travelling as one `env` argv
element, so a quote, a `$(`, a space *or a newline* in `ETC_DIR` is inert — strictly better than
the single-quoting it replaced. The recreate rule is genuinely an equality comparison now
(`slices.Equal(got, hbinds)`), it cannot churn (`create` appends the *same* `hbinds` slice, so the
next start compares equal), and it cannot reach the setup VM (`EnsureSetup` is a separate function
that never calls `ensureRunningLocked`). The three findings below are all **low**: the residual
blast radius of the origin gate (the transport is pinned, the *host* is not), a missing regression
test for the helper-snippet fix, and the failure semantics of the strengthened Tier 2 remount probe.

## Findings

- **[low / confidence: high]** `deploy/styrman-ctl:659-660` — the origin gate pins the *transport*
  but not the *host*: `[[ "${HARNESS_ORIGIN}" =~ ^https://[a-z0-9.-]+/[A-Za-z0-9._/-]+$ ]]` accepts
  `https://evil.test/a/b` (measured, `BASH-PASS`). An environment that can set
  `STYRMAN_HARNESS_ORIGIN` therefore redirects root's `git clone --` (`:680`) to an arbitrary host
  **and** points the scoped credential helper at it — `origin_host` is derived from the same value
  (`:665`), so the helper becomes `credential.https://evil.test.helper` and hands
  `/etc/styrman/gh-token-harness`'s fine-grained GitHub token to that host on the first request.
  That is token exfiltration, not code execution: no non-https transport survives the anchor, `--`
  blocks option-shaped values, `to_https` is deliberately not applied first, and nothing the hostile
  origin delivers can become `current` — `harness_main` forces `REQUIRE_SIGNED=1` and the tag must
  verify against `/etc/styrman/allowed_signers`. Reachability is narrow and unchanged from the
  predecessor's framing: the seam guard (`:51-55`) refuses `STYRMAN_HARNESS_ORIGIN` unless
  `STYRMAN_CTL_TEST=1`, and that variable has no gate of its own, so both are settable only in the
  one `sudo -E` poisoned-environment scenario the guard's own comment names. Recorded because the
  fix's comment says it now matches `https_origin()` — it does, and `https_origin()` reads the
  operator's *own* checkout's remote, whereas this value is purely ambient.
  **Remediation, one line and no test breaks:** pin the host too —
  `^https://github\.com/[A-Za-z0-9._/-]+$`. All three must-fail cases in
  `TestStyrmanCtlHarnessOriginIsGated` still fail closed and
  `TestStyrmanCtlUpdateSetupHarnessPlan`'s default origin still passes. (Stdlib/no-dependency note:
  this is a bash conditional, nothing is added.)
- **[low / confidence: high]** `internal/config/styrman_ctl_harness_test.go:80-83` — nothing pins
  the credential-helper snippet's new constant form, so the fix can regress silently. The clone-line
  assertions are `-c credential.helper= `, `credential.https://github.com.helper=`,
  `gh-token-harness` and `GIT_TERMINAL_PROMPT=0` — **every one of which was also true of the
  vulnerable pre-fix line**, where `${ETC_DIR}/gh-token-harness` was interpolated *into* the snippet.
  The test therefore cannot tell the fixed snippet from the one it replaced: an edit that puts
  `cat '${ETC_DIR}/gh-token-harness'` back passes the whole suite. This is the same gap the
  predecessor raised about the origin regex (fixed, correctly, by
  `TestStyrmanCtlHarnessOriginIsGated`) and the project's own rule that a check is validated in the
  must-pass *and* must-fail state. **Remediation:** two assertions on the same `clone` line already
  extracted at `:79` — it must contain `cat \"${STYRMAN_HARNESS_GH_TOKEN_FILE}\"` and must **not**
  contain `cat '` (the interpolated shape); the same pair for the crew helper's
  `STYRMAN_GH_TOKEN_FILE` on `styrman_ctl_test.go`'s existing `--setup` plan assertions.
- **[low / confidence: medium]** `.agents/docs/testing/styrman/integration.sh:145-146` — the
  strengthened probe is right in direction (a missing or refusing `mount` can no longer degrade it
  into the plain `! touch` asserted at `:135-136` and still print PASS) and it is validated in its
  must-PASS state: the round-3 verifier log records `guest cannot write the channel even after
  remounting it rw (host-side ro, R12)` as PASS inside `passed=114 failed=0 skipped=3`, so the
  remount really does succeed and `/proc/mounts` really does show `rw` on this host's Kata 3.31.0.
  Two residuals. (a) Its must-FAIL state has not been demonstrated — nobody has shown the check goes
  red when the remount is refused, which is precisely the state the rework exists to catch (the
  `&&` chain makes it certain by inspection, hence high confidence in the mechanism and medium in
  the finding). (b) The assertion is now coupled to a kernel behaviour that may legitimately get
  *stronger*: a guest kernel or Kata release that locks the bind's mount flags makes
  `mount -o remount,rw` return `EPERM`, and this check then turns red at the release gate under a
  name that reads "guest CAN write the channel" — a security improvement reported as a security
  failure. **Remediation:** split it — keep `! touch` as the assertion, and report the remount's own
  rc as a separate line (INFO, or SKIP with the rc named) so a refused remount is *recorded* rather
  than counted as a failed write assertion; validate the must-fail state once by pointing the probe
  at a path that is not a mountpoint.

## Fixes verified, per the predecessors' Outcome sections

- **Per-line `grep` → whole-string `[[ =~ ]]`** (`styrman-ctl:659`). Correct and complete. Measured
  both forms against the exact regex: `grep` rc=0 for every multi-line value, `[[ =~ ]]` false
  (bash calls `regexec` without `REG_NEWLINE`, so `^`/`$` anchor the whole string). A trailing
  newline is refused too. Ordering is right: the check precedes the token staging at `:661`, so a
  refused origin never writes `gh-token-harness`. `[a-z0-9.-]` still refuses userinfo
  (`user:pass@host`) and percent-encoding; an `en_US.UTF-8` locale does **not** broaden `[a-z]` to
  uppercase here (measured). The `die` message is sanitised with `${HARNESS_ORIGIN//[^[:print:]]/?}`
  — the ESC byte is stripped, matching the `printable` behaviour `TestStyrmanCtlDisplayValuesArePrintable`
  already pins. Must-fail tests exist and pass (`TestStyrmanCtlHarnessOriginIsGated`: `ext::sh -c id`,
  the two-line value, `file:///tmp/x`, plus the seam guard for **both** names — the pre-existing
  `TestStyrmanCtlOverridesGatedOutsideHarness` only ever tripped on `STYRMAN_ETC_DIR`, the first
  name in the loop, so this is a real addition). Residual: finding 1.
- **Token path via the git call's `env`, never interpolated** (`styrman-ctl:589,606,609,666,677,680`).
  Correct and complete. The text git receives is now constant — I reconstructed it with a single
  quote planted in `ETC_DIR`: `credential.https://github.com.helper=!f() { echo
  username=x-access-token; printf 'password=%s\n' "$(cat "${STYRMAN_HARNESS_GH_TOKEN_FILE}")"; }; f`
  — and the path is a separate argv element `STYRMAN_HARNESS_GH_TOKEN_FILE=/etc/sty'rman/gh-token-harness`.
  The helper's `sh -c` reads it from git's inherited environment; quote, `$(`, space, `;`, backtick
  and newline in `ETC_DIR` are all inert (the newline case was *not* closed by the single-quoting
  this replaces). `run()` is `"$@"`, never `eval`, so the quoting survives to `execve`. All four
  call sites (crew fetch/clone, harness fetch/clone) carry the assignment; `grep` for `${helper}`
  finds no fifth use and the snippet is never persisted to a config file, so no later git call can
  inherit a helper whose variable is unset. Unset would be fail-closed anyway: `cat ""` errors,
  the password is empty, and `GIT_TERMINAL_PROMPT=0` forbids a prompt — no hang. Residual: finding 2.
- **Presence → equality in the recreate rule** (`orchestrator.go:358-380`, `harness.go:129-144`).
  Correct and complete. `harnessBindsOf` normalises order (repo group then bin group), so a
  reordered `HostConfig.Binds` does not churn; a half-state (repo only) and a duplicated bind both
  land in `default` and rebuild; `slices.Equal(nil, nil)` is true, so a disabled channel with no
  binds starts plainly. The switch arms are exhaustive and in the right order (equal → nothing;
  `len(got)==0` → mount; `len(hbinds)==0` → drop; otherwise re-bind). **No churn loop:** `hbinds` is
  computed once at `:356` and the same slice is handed to `create` at `:483`, where `:1125` appends
  it verbatim, so the next `EnsureRunning` compares equal — and there is no TOCTOU between the
  decision and the create. **The setup VM is unreachable from this rule:** `EnsureSetup` (`:747`) is
  a separate function with its own lock and its own recreate conditions; it never calls
  `ensureRunningLocked`, so the "gain the binds" arm cannot fire on a container `create` will
  deliberately build without them. `err == nil` also excludes the container the network-mode arm
  just removed, so no double remove. `hasHarnessBind` is gone with no callers left
  (`grep` over `internal/`, `cmd/`); `go vet ./...` rc=0. New input surface of `harnessBindsOf` is
  closed: the only other binds a project container carries are `<phome>:/home/<guestUser>` and the
  workspace bind, and `guestNameRe = ^[a-z][a-z0-9_-]{0,31}$` (`httpapi/profile.go:28`, re-validated
  on load from disk) admits no `:`, so neither can be misclassified as a harness bind — and Docker
  would reject a four-field bind anyway.
- **`HARNESS_REPO_DIR` a literal, not derived from `STYRMAN_DEPLOY_DIR`** (`styrman-ctl:30`) with the
  seam in `ctlSetup` (`styrman_ctl_test.go:89`). Correct. The two units
  (`styrman-update.service:21`, `styrman-harness-update.service:15`) and `UPDATER_ENV` (`:520`) all
  agree on `/opt/styrman-harness`. The empty-value case is safe (`[ -z … ]` passes the guard, `:-`
  then takes the default). If the seam were ever dropped from `ctlSetup`, `ctl()`'s production-path
  scan would fail the test loudly on ` /opt/styrman`.
- **`host_converge` harness-unit orphan sweep** (`styrman-update:856`). Correct, and adds no
  exposure. It sits behind the full gate chain I re-walked: both unit-pinned variables set;
  `REQUIRE_SIGNED=1`; the firewall binary's basename pinned; `trusted_dir` on `bindir` *and*
  `unitdir` (exists, root-owned, no `/022`); a 40-hex `deployed-sha` equal to the clone's
  `git rev-parse HEAD`; and, above all of it, `trusted_dir "${DEPLOY_DIR}"` with `.git` root-owned
  and not group/world-writable before `host_converge` is called. `hf` is one of two hard-coded
  literals, the deleted glob is confined to `<unitdir>/styrman-harness-update.{service,timer}.new.*`
  (the quoted prefix is not a pattern; an unexpanded glob is a harmless `rm -f` no-op), and
  `flock -n 9` at `:546` serialises ticks, so the sweep cannot race `atomic_install`'s `.new.$$`.
  `CHECK_ONLY` still changes nothing.
- **Describing text** — `harnessBinds`'s doc-comment, the `harness_test.go` header (now naming the
  moved-channel case and "equal binds start without churn"), `specs/styrman/deploy.md:395-396`
  ("one comparison, the binds a stopped container carries must equal what the server would mount
  now") and the plan's superseded-by-D26 clause all now state what the code does. `deploy.md`'s
  `repo`/`bin` "real directories (never symlinks)" sentence matches `harnessTrusted`'s `os.Lstat` +
  `IsDir()` loop. The Tier 2 count line in `testing/styrman/README.md:15` (117 checks, 3 auto-SKIP →
  `passed=114`) matches the round-3 log exactly — no count drift.

## Checked

- **Secrets:** none introduced. `git diff 5ba2b1b...edda5cd --stat` touches no `.env*`, `*.pem`,
  `*.key`, `*.tfvars`, `*.tfstate` or credential JSON. The change moves a token **path** (not a
  token) from a snippet into an `env` assignment; `/proc/<pid>/environ` of that root `git` is
  root-only and holds a path. `TestStyrmanCtlUpdateSetupHarnessPlan` still asserts neither
  `HARNESSSECRET` nor `SECRETTOKEN` reaches the `--dry-run` plan, and it still discriminates the
  harness token file from the crew one (`gh-token-harness ` does not match its `gh-token ` /
  `gh-token"` probes). No new log line carries a secret; the new `die` sanitises non-printables.
- **Injection:** the range's two injection-relevant boundaries are the ones it fixes — the origin
  string (now whole-string anchored before it reaches argv *and* before it is interpolated into the
  `credential.<host>.helper` config key) and the credential-helper snippet (now constant). The new
  Go parser `harnessBindsOf` consumes only strings the server itself wrote, and its one
  externally-influenced neighbour (the guest username) is regex-constrained without `:`. No `eval`
  added in the shell (the pre-existing `check(){ eval "$2"; }` in `integration.sh` consumes only
  script-local scratch paths), no SQL (no database by design), no template rendering.
- **Authz/authn:** no HTTP surface in the range; the ingress-token / `X-Caddy-Email` guard is
  untouched. The channel's authorization decisions — signature enforcement forced on in
  `harness_main`, the filesystem-ownership checks in `harnessTrusted`/`trusted_dir`, the seam guard
  — are unchanged or strengthened. Nothing here grants a capability.
- **Crypto:** nothing hand-rolled anywhere in the range. No cipher, hash, MAC, signature, KDF, RNG,
  token or session format is implemented or composed. Verification remains `git verify-tag` with
  `gpg.ssh.allowedSignersFile` (a vetted implementation, used as designed); `sha256sum` is a manifest
  checksum. The credential helper is git's own documented mechanism, not an auth protocol built here.
- **Dependencies:** no new module, no new npm package — `go.mod`, `go.sum`,
  `deploy/caddy/ingress/go.mod`, `web/package.json` and `web/package-lock.json` are all absent from
  the range's `--stat`. The one new import is stdlib `slices`. `govulncheck ./...` run **bare**, exit
  status read: **rc=3** on the root module and **rc=3** on `deploy/caddy/ingress` — the accepted,
  unchanged state: GO-2026-4887 and GO-2026-4883, both `github.com/docker/docker@v28.5.2+incompatible`,
  `Fixed in: N/A`, daemon-side, already recorded in `2026-09-16-security-v1.34.0-c26a723.md`
  (`.agents/scratch/sec3-govuln-root.txt`, `.agents/scratch/sec3-govuln-ingress.txt`). `npm audit` in
  `web/` run bare, rc=1: 1 high, `js-yaml` GHSA-2883-xcg3-v3hh (4.0.0-4.3.1) — unchanged, not in this
  range, already a recorded finding.
- **GCP surface:** none — self-hosted; `gcp.md` remains "not applicable". No IAM binding, service
  account, `--allow-unauthenticated`, Secret Manager reference or SA key file anywhere in the range.
- **Static and dynamic checks I ran myself, bare, statuses read:** `go vet ./...` rc=0;
  `go test ./internal/orchestrator/ ./internal/config/` ok; `go test -count=1 -run 'Harness|Channel'
  -v` rc=0 with all 20 matching tests PASS, including the four new/changed ones
  (`TestStyrmanCtlHarnessOriginIsGated`, `TestEnsureRunningRecreatesAStoppedContainerWhenTheChannelMoved`,
  `TestEnsureRunningLeavesAStoppedContainerWhoseHarnessBindsMatch`,
  `TestCreateBindsTheChannelForProjectsNotForTheSetupVM`) — log at `.agents/scratch/sec3-gotest.txt`.
  `bash -n` rc=0 and `shellcheck -S warning -s bash` rc=0 on `deploy/styrman-ctl`,
  `deploy/update/styrman-update` and `.agents/docs/testing/styrman/integration.sh` (worth stating: a
  syntax error in the updater *is* a security failure here — the channel silently stops delivering
  signed releases). Regex behaviour measured in the session scratchpad, not in the repo.

## Read in full

Every path the dispatch named. `deploy/styrman-ctl` — the header and seam guard (`:20-60`), `run()`,
`https_origin`/`to_https`, `setup_auto_update` end-to-end, `setup_harness_channel` end-to-end,
`UPDATER_ENV`, `cmd_update`, the `cmd_status` harness line. `deploy/update/styrman-update` —
`host_converge` in full with every gate above it, `atomic_install`, `differs`, the `flock`, the
`HARNESS_REPO_DIR` gates at `:184-390`, and `:912`. `internal/orchestrator/orchestrator.go` — the
recreate block, the `createSpec`, the `Binds` assembly at `:1125`, `EnsureSetup`, `EnsureRunning`/
`WithProject`/`lockedProject`. `internal/orchestrator/harness.go` — whole file
(`harnessBinds`, `harnessTrusted`, `ownedAndPrivate`, `harnessBindsOf`, `harnessChannelState`).
`internal/orchestrator/harness_test.go` — whole file (393 lines), including `plantChannel`,
`wantHarnessBinds` and `setupSpec`. `internal/config/styrman_ctl_harness_test.go` — whole file
(182 lines). `internal/config/styrman_ctl_test.go` — `ctlSetup` (including the new seam line at `:89`),
`ctl()` and its production-path scan, `TestStyrmanCtlOverridesGatedOutsideHarness`,
`TestStyrmanCtlDisplayValuesArePrintable`. `.agents/docs/testing/styrman/integration.sh` — the
reworked probe and both neighbouring read-only checks, `check()`, `cleanup()`, the `HARNESS_OUT`
definition and every harness/channel line in the file. Also: both predecessor security reports
including their Outcome sections, the three code-review reports' verdicts and findings, the round-3
verifier log, `specs/styrman/deploy.md`'s and the plan's changed sections, `deploy/update/*.service`
Environment lines, and `httpapi/profile.go`'s guest-name validation.

## Not examined

- **`bash deploy/vulncheck.sh`** — the project's authoritative scan (it also covers the
  `image/harness` module inside the pinned builder image) was NOT run: it needs Docker and several
  minutes. `govulncheck ./...` was run directly on the two Go modules instead. **The `image/harness`
  module was therefore not scanned in this review.**
- **Tier 2 (`integration.sh` against real Kata), Tier 4 (`web/e2e/run.sh`), the full race suite and
  the ingress module's tests** — not RUN by me; running them is the verifier's row. My judgement of
  the reworked remount probe is static plus the round-3 verifier log's recorded PASS line and
  `passed=114 failed=0 skipped=3`, which is evidence of the must-pass state only (finding 3).
- **The three code-review reports read only in part** — I read each one's verdict, findings and the
  previews of their Outcome sections to establish what each commit claims, not their full text.
- **Out-of-range files in the working tree** — the untracked
  `.agents/docs/changes/2026-09-16-v1.35.0-release.md` and
  `.agents/docs/reviews/2026-09-16-preflight-v1.35.0.md`, and the modified `README.md`, are not in
  `5ba2b1b..edda5cd` and were not reviewed.
- **Documentation-only paths in the range beyond the two I read** — the four review reports and three
  verifier logs the range commits are prose artifacts outside the executable blast radius; I read
  them as evidence, not as a diff of their own. `specs/styrman/deploy.md` and
  `plans/2026-09-16-harness-channel.md` WERE read for the sections the code touches.
- **Whether a hostile https origin's server can do anything beyond receiving the token** — finding 1
  is stated against the check and the token flow; probing a remote git server's behaviour is
  exploit work and out of scope.

## Outcome

Written by the main agent, 2026-09-16, in the commit that applies the findings (the ctl tests green after the edits; Tier 2 re-run by the verifier for the release gate).

- `styrman-ctl:659` the transport pinned but not the host — fixed: `^https://github\.com/[A-Za-z0-9._/-]+$` (the token is a GitHub token; the production origin is the literal default anyway); `TestStyrmanCtlHarnessOriginIsGated` refuses `https://evil.test/…`.
- `styrman_ctl_harness_test.go:80-83` assertions also true of the pre-fix snippet — fixed: the clone line must contain `cat "${STYRMAN_HARNESS_GH_TOKEN_FILE}"` and the env word `STYRMAN_HARNESS_GH_TOKEN_FILE=<etc>/gh-token-harness git`, and must not contain `cat '`.
- `integration.sh:145` the remount's outcome folded into the verdict — fixed: the remount's outcome (`remounted-rw` / `remount-refused`) is reported in the check's name; the verdict is the failed write plus nothing on the host; the must-fail state is the rw control recorded in `deploy.md` (the same touch on a `:rw` bind succeeds, measured by hand 2026-09-16).

