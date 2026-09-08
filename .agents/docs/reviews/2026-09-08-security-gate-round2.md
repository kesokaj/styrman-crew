---
owner: security
date: 2026-09-08
---
# Security review — release gate round 2 (verification of the six round-1 fixes)   (2026-09-08)

Range reviewed: 2321b99..3e0bb9c

Scope: the single commit `3e0bb9c` and its blast radius — `deploy/firewall/install.sh`,
`deploy/firewall/styrman-firewall.sh` (its conf grammar and reconcile loop),
`internal/netpolicy/{dir,status}.go`, `internal/httpapi/network.go`,
`internal/credstore/credstore.go`, `internal/credbroker/broker.go`,
`web/src/components/LocalNetwork.tsx`, and the consumers of each — plus
`deploy/docker-compose.yml`, which decides whether the `Installed()` fix reaches production.
Not re-audited: the rest of the v1.24.0 release.

## Verdict: FINDINGS

The commit introduced a regression that breaks the very code path it was fixing.
`deploy/firewall/install.sh:167` now calls `derive_ingress_bridge` seven lines **before**
the function is defined at `:174` — the pre-image had the call at `:158`, after the
definition at `:143`, and this hunk moved it up. Every run with
`--lan-allow-published-ports 1` therefore dies with `derive_ingress_bridge: command not
found` (exit 127, reproduced), *after* the script, the three units, the tmpfiles conf and
the spool mount unit have already been written and *before* the conf is written and before
`systemctl daemon-reload` / start / enable in section 5. The host is left half-converged,
and the well-formed `INGRESS_BRIDGE=` line that defect 2 was about is unreachable on the
only path that produces it. Behind that: the new R12 sshd precondition passes on exactly
the configuration it forbids (`Match Address <lan>` re-enabling password auth is invisible
to `sshd -T` without `-C` — demonstrated), and self-cancels when `sshd` is not on `PATH`;
and the `Installed()` fix does not reach the compose deployment, whose bind mount makes the
run dir exist whether or not the host half was ever installed.

Four of the six fixes are otherwise correct as claimed: the conf grammar (2), the checked
chain reads (4), the enable-time ingress cross-check (5), and the shared host-reaching
predicate (6). Details under "Per-claim verification".

## Findings

- **[high / confidence: high]** `deploy/firewall/install.sh:167` (call) vs `:174`
  (definition) — `derive_ingress_bridge` is invoked before it is defined. Under `set -euo
  pipefail` the unresolved name returns 127 and aborts the installer. Reproduced:
  `bash install.sh --dry-run --state-dir <scratch> --lan-dev lo --lan-cidr 192.168.1.0/24
  --lan-allow-published-ports 1 --ingress-bridge br-0123456789ab …` →
  `install.sh: line 167: derive_ingress_bridge: command not found`, `EXIT=127`. Not
  exploitable by a tenant — it is a self-inflicted, fail-closed denial — but it leaves a
  root-owned host in a half-converged state (new `styrman-firewall.sh` and units on disk,
  never `daemon-reload`ed, timer/path not enabled by this run, conf untouched), which is a
  worse operational position than either the old or the new state. It also makes the
  claimed fix for defect 2 dead code on the flag that reaches it. `bash -n` and
  `shellcheck -S warning` — the only automated coverage (`deploy/firewall/test.sh:361,363`)
  — do not detect call-before-definition, and `test.sh` never executes `install.sh`.
  **Remediation:** move the `if [ "${ALLOW_PUB}" = 1 ]; then derive_ingress_bridge; fi`
  line back below the function body (or hoist the definition above `:167`), and add a check
  that actually runs `install.sh --dry-run --lan-allow-published-ports 1` and greps the
  generated conf against `read_conf`'s own regex — the commit message claims the output is
  "asserted line-by-line against the script's own grammar", and no committed assertion does
  that.

- **[medium / confidence: high]** `deploy/firewall/install.sh:85` — `sshd -T` without `-C`
  reports only the **global** effective config, so a `Match` block that re-enables password
  authentication for the LAN is invisible to the new R12 gate. Demonstrated on this host
  with a config of `PasswordAuthentication no` + `Match Address 192.168.0.0/16 /
  PasswordAuthentication yes`: `sshd -T` prints `passwordauthentication no` (the installer
  logs "R12 precondition met" and proceeds), while `sshd -T -C
  addr=192.168.1.5,user=root,host=x` prints `passwordauthentication yes`. The gate
  therefore passes on precisely the configuration R12 forbids, for precisely the network
  the projects are being granted, and a `sudo`-group login on a `NOPASSWD` host is root.
  **Remediation:** evaluate with `-C addr=<a representative address from each configured
  LAN_CIDR>,user=root,host=<x>` and refuse if any of them reports `yes`; additionally refuse
  (or loudly warn) when the config contains any `Match` block that mentions
  `PasswordAuthentication`.

- **[medium / confidence: high]** `deploy/firewall/install.sh:84` —
  `command -v sshd >/dev/null || { log "no sshd binary on this host - precondition not
  applicable"; return 0; }` fails **open**: an sshd that exists but is not on `PATH` reads
  as "no sshd". `sshd` lives in `/usr/sbin`, which is not on `PATH` in every root context
  (`sudo -E`, `su` without `-`, automation with a trimmed environment) — in this very shell
  `/usr/sbin/sshd` exists and `command -v sshd` fails. The repo already knows this:
  `deploy/firewall/test.sh:17` does `export PATH="/usr/sbin:/sbin:${PATH}"`. The one
  enforcement of an R12 requirement thus silently cancels itself on an ordinary
  misconfiguration of the invoking environment. **Remediation:** probe absolute paths
  (`/usr/sbin/sshd`, `/sbin/sshd`, `/usr/local/sbin/sshd`) and treat an active `ssh.service`
  or a listening `:22` (`ss -lnt`) as proof sshd exists — refuse rather than skip when the
  service is up but the binary cannot be located.

- **[medium / confidence: high]** `internal/netpolicy/dir.go:99` +
  `deploy/docker-compose.yml:133` — the claimed HIGH fix does not reach the deployment shape
  the defect described. `Installed()` stats `filepath.Dir(statusPath)`; compose bind-mounts
  `/run/styrman-firewall:/run/styrman-firewall:rslave`, and a missing bind source is created
  by the daemon, so the directory exists inside the server container whether or not
  `deploy/firewall/install.sh` has ever been run on the host. The compose file's own comment
  (`:113-115`) states this outright: "Before deploy/firewall/install.sh has run on the host
  the dir is empty". On such a host `Installed()` is true, `Status()` is nil, and
  `internal/httpapi/network.go:321` still withholds the SSH key from every project — the
  reported symptom, unfixed. The fix only takes effect on a bare dev run on a machine that
  has never had the run dir. Direction is fail-closed (a credential is withheld, never
  leaked), so this is a correctness/availability gap rather than an exposure.
  **Remediation:** key the predicate on evidence the host half actually ran — e.g. a status
  file that has ever been observed (persist the fact), or the presence of `spool/` with the
  expected root-owned parent — or accept and document that a compose deployment withholds
  until the firewall half runs.

- **[medium / confidence: medium]** `deploy/firewall/styrman-firewall.sh:731-737` with
  `internal/netpolicy/status.go:117,209` — a skipped reconcile is indistinguishable, to
  every decision function, from an authoritative "this project has no rule". When
  `scan_current` returns 1 the reconcile and `count_applied` are skipped, but `write_status`
  still runs with a **fresh** `applied_at` and an empty `lan=` list; the only signal is
  `error=all:chain read failed`, whose id is not a uuid, so `ParseStatus` files it under
  `Warnings` — which neither `Availability`, `Derive`, nor `Exposed` reads. Consequence: a
  project whose record was just switched OFF, whose RETURN rule was **not** revoked because
  the reconcile was skipped, evaluates `Exposed(false, uuid, st, now) == false`, and the
  broker resumes serving its host-reaching credentials while the LAN rule is still live —
  the exact pairing ADR-066 part (4) exists to prevent. The outcome is not a regression (the
  old `|| true` produced the same empty current-state and also revoked nothing), but the new
  code makes the skip deterministic and the commit message claims "the status says why"; it
  says why only in a field no consumer consults. **Remediation:** emit a status key the
  parser maps onto a non-authoritative state (a `scan_failed=1` that `Exposed` and
  `Availability` treat like staleness, or reuse `conf_error`), so "could not read the
  chains" reads as "cannot confirm" rather than as "off".

- **[low / confidence: high]** `internal/httpapi/network.go:151` — `lanView` still calls
  `netpolicy.Exposed(...)` **without** the `Installed()` guard the broker now has, so the two
  disagree exactly where the fix applies. With no host half and no status,
  `Exposed(false, …, nil, …)` returns true (`status.go:209`) and the UI reports
  `credentialImpact: "ssh-key-withheld"` for every project while the broker actually serves
  the key. The comment immediately above (`:148-150`) asserts "the UI cannot disagree with
  what is actually served" — no longer true. A security indicator claiming a control that is
  not in force is worse than no indicator. **Remediation:** extract one
  `s.lanExposedFor(p)` helper containing the `Installed()` guard and call it from both
  `LanExposed` and `lanView`.

- **[low / confidence: high]** `deploy/firewall/install.sh:85,88` — under `set -euo
  pipefail`, `pw="$(sshd -T 2>/dev/null | sed … | head -1)"` aborts the whole installer with
  a bare exit status when `sshd -T` itself fails, so the explanatory `""` → `die` branch at
  `:88` is all but unreachable and the operator gets no message (stderr was discarded).
  Demonstrated: `/usr/sbin/sshd -T -f <cfg>` → `sshd: no hostkeys available -- exiting.`,
  rc 1. Fail-closed (nothing is installed), but undiagnosable. **Remediation:** capture the
  status explicitly (`pw="$(… || true)"`, or `if ! pw="$(…)"; then die "<message>"; fi`) and
  keep the helpful text.

- **[low / confidence: medium]** `deploy/firewall/install.sh:98` — the R12 precondition runs
  only when `--lan-allow-host 1` is passed on **that** invocation. A conf already carrying
  `LAN_ALLOW_HOST=1`, a hand-edit of `/etc/styrman/firewall.conf`, or sshd being reconfigured
  to accept passwords afterwards is never re-checked, and `styrman-firewall.sh` — the
  component that re-installs the allowance every 60 s — has no notion of sshd at all. The
  check is a one-time opt-in gate, not an invariant. **Remediation:** also run it in the
  "keeping existing `${CONF}`" branch, which already greps for `^LAN_ALLOW_HOST=1` at `:223`,
  and consider a periodic advisory from the reconciler.

- **[low / confidence: medium]** `internal/httpapi/network.go:109-113` — a failed or
  timed-out `NetworkInspect` caches the empty answer together with `ingressBrAt`, so the new
  enable-time cross-check is silently disabled for a full minute after any Docker hiccup.
  "Empty means no opinion" is the right rule, but it should not be sticky. Enabling is
  admin-gated, so exploitation needs an already-privileged actor. **Remediation:** do not
  advance `ingressBrAt` when the answer is empty, or give the negative result a much shorter
  TTL.

Carried forward, not new: `web/dist/client/_shell.html` still carries a raw NUL at offset
7417 (`__root__\x00`), so git renders it as binary and this commit's change to it
(`Bin 7606 -> 7606`) was unreviewable by diff — the project's own BUG-007 rule
(`.agents/rules/writing-files.md`) calls that a stop sign. Already recorded in
`2026-09-08-security-release-v1.23.0.md:127` and three sibling reviews; it predates the
range (present at 2321b99 and at HEAD) and is TanStack generator output. Verified
out-of-band that the shipped bundle carries the new consent string
(`web/dist/client/assets/p._project._term-BkI-OXN_.js`) and no longer carries the old one,
so the SPA does match `web/src` for this change.

## Per-claim verification

1. **`Installed()` vs `Enabled()`** — the three states are distinguished in code
   (`dir.go:82` configured, `:99` exists, `Status()` readable), and an ON record still
   withholds with the half absent: `Exposed` short-circuits on `record` at
   `status.go:209`, and `LanExposed`'s fallback returns `p.LanAccess`
   (`network.go:322`), so a `true` record yields `true`. The new tests pin both halves.
   **Incomplete** for the compose deployment (finding above), and the sibling UI path was
   not updated (finding above).

2. **`INGRESS_BRIDGE` conf line** — correct as written. `INGRESS_LINE` is built before the
   `printf` (`install.sh:245-246`); the value `br-<12 hex>` matches `read_conf`'s grammar
   `^(LAN_DEV|LAN_CIDRS|LAN_ALLOW_HOST|LAN_ALLOW_PUBLISHED_PORTS|INGRESS_BRIDGE)=([A-Za-z0-9./\ ,_-]*)$`
   (`styrman-firewall.sh:419`), where the old literal `\n` would have hit `conf_fail` at
   `:430` and closed every allowance. An empty `INGRESS_LINE` leaves no stray blank line,
   because `$( )` strips the trailing newlines the format adds. The path is nevertheless
   unreachable via `--lan-allow-published-ports 1` (finding 1), and no automated assertion
   backs the "line-by-line against the grammar" claim.

3. **sshd precondition** — the refusal genuinely precedes every install step (`:98`, before
   section 1 at `:100`; only read-only `stat` checks run earlier), and the override is not
   reachable by accident: `--i-accept-sshd-passwords` is an explicit long flag, defaults to
   `0`, is settable by no environment variable, and any unknown flag exits 2 (`:56`). The
   `[ … ] && { …; return 0; }` and `command -v … || { …; return 0; }` forms are the
   errexit-safe shapes. The **parsing** is the weak part: three findings above (Match blocks,
   `PATH`, pipefail abort). "Spoofable" in the narrow sense — no, `sshd -T` reads root-owned
   config as root; an attacker able to edit `sshd_config` is already root-equivalent.

4. **Checked chain reads** — correct. All three sites now test the read
   (`styrman-firewall.sh:569, 612, 668`); a failed scan skips `reconcile` **and**
   `count_applied` together (`:731-736`); a failed re-read in `delete_malformed` `continue`s
   rather than deleting by an unconfirmed position (`:612-615`). The `<<<"${dump}"`
   substitution is safe: a here-string on an empty variable yields one empty line, which
   every loop's leading `case "${line}" in "-A ${chain} "*)` discards. No `ipt -S … || true`
   remains in the LAN section. The one unchecked read left, `ensure_jump`
   (`styrman-firewall.sh:70`), can only cause a redundant `-I` of a base jump — tightening,
   never loosening. The residual is finding 5 above: the skip is invisible to the consumers
   of the status.

5. **Enable refused on ingress-bridge mismatch** — correct. A Docker lookup failure returns
   `""` from `BridgeNameForNetwork` (`orchestrator.go:386-387`) and
   `ingressBridgeMismatch` returns `""` at `network.go:115`, so an empty answer never blocks
   an enable; both the availability check and the mismatch check sit inside `if on`
   (`network.go:199-215`), so DISABLE is never refused; the check is also a no-op when the
   conf did not opt into published ports (`:104`). The production shapes agree: the compose
   ingress network is a plain `driver: bridge` with no `com.docker.network.bridge.name`, so
   the server's `br-<12 hex>` derivation matches root's. The refusal text names operator
   bridge names, but the enable path is admin-gated (`:190`), so nothing leaks to a tenant.
   Residual: the sticky negative cache (finding above).

6. **Shared `IsHostReaching`** — correct; the callers can no longer disagree. Exactly two
   remain (`credbroker/broker.go:189,197` via the one-line delegate at `:50`, and
   `httpapi/network.go:282`), both through `credstore.IsHostReaching`. No over-match:
   `~/.sshfoo/key`, `~/notes/.ssh-notes.md`, `~/.config/gcloud/application_default_credentials.json`
   → false. No under-match on any storable spelling: `~/.ssh`, `~/.ssh/`, `.ssh/config`,
   `.ssh//key`, `./.ssh/key`, `.//.ssh/key`, `~/./.ssh/key`, `$HOME/.ssh/id_rsa` → true.
   `..` segments cannot be stored at all (`credstore.go:449 validRelPath`), which closes the
   `~/a/../.ssh/k` spelling. The prune's separately-derived `rel` (`network.go:285`, trimming
   only `~/` and `$HOME/`) is normalised downstream by `files.cleanRel`
   (`internal/files/files.go:45`, `path.Clean("/"+strings.TrimSpace(rel))`), so a `./`-spelled
   credential is deleted from the same file the predicate matched — the divergence is
   genuinely gone. The leading-`/` trim over-matches `/.ssh/key`, which is unstorable and
   would only withhold more. The UI copy change (`LocalNetwork.tsx:186-193`) is accurate:
   Caddy is `network_mode: host`, the `lanhost` family RETURNs `-d <host uplink addr>` in
   `STYRMAN-IN`, so with `LAN_ALLOW_HOST=1` the ingress on that address is reachable.

## Checked

- **Secrets** — clean. Every added line in the range grepped for
  `api_key|secret|token|password|passwd|BEGIN … PRIVATE KEY|AKIA|ghp_|xox[baprs]-`; every hit
  is prose in the added review/deploy docs or the literal string `passwordauthentication` in
  the new sshd check. No credential material, no committed `.env`, no secret echoed into a
  log or an error path. The new `die` text quotes only sshd's own `passwordauthentication`
  value; the 503 body from the ingress-bridge refusal names bridge names and is admin-only.
- **Injection** — no SQL, no template, no shell interpolation of tenant input added. The new
  shell code interpolates only operator-supplied flags and `sshd -T` output (root-owned) and
  only into `log`/`die` messages, never into a command. `INGRESS_LINE` is concatenated into a
  `printf '%s'` argument, not into a format string. `read_conf`'s grammar
  (`styrman-firewall.sh:419`) still bounds the conf's value charset, and the new conf line
  matches it. `<<<"${dump}"` replaces a process substitution with no change in how lines are
  parsed. The one path-injection surface, `pruneHostReachingCreds`, still routes through
  `internal/files`' confined roots (BUG-002/003 guard intact).
- **Authz/authn** — unchanged and intact. Enable is still admin-gated before any state change
  (`network.go:190`), the 404-before-authorization ordering at `:182-188` is untouched, and
  both new refusals sit inside `if on`, so a tenant's disable is never blocked. The
  admin-only `host` block in `lanView` is unchanged, so root's LAN prefixes still do not reach
  a tenant.
- **Crypto** — nothing hand-rolled, nothing added. No auth, hashing, session, or JWT/OIDC code
  in the range; the only crypto-adjacent line is the new `sshd -T` policy read, which
  delegates to OpenSSH.
- **Dependencies** — no dependency changed: `git diff 2321b99..3e0bb9c -- go.mod go.sum
  web/package.json web/package-lock.json deploy/caddy/ingress/go.{mod,sum}` is empty.
  `govulncheck ./...` (local, go1.25.0) reports 31 findings, 29 of them stdlib
  `crypto/x509`/`net/http`-class issues fixed in go1.25.3 — an artifact of the **local**
  toolchain, not of the shipped binary, which builds on `golang:1.25.13-trixie`
  (`deploy/Dockerfile.server:9`, the image `deploy/vulncheck.sh` pins). The two module-level
  findings are pre-existing and unrelated to this commit:
  GO-2026-4887 (Moby AuthZ plugin bypass on oversized request bodies) and GO-2026-4883
  (Moby off-by-one in plugin privilege validation), both in
  `github.com/docker/docker@v28.5.2+incompatible`. `npm audit --omit=dev` in `web/`: **found
  0 vulnerabilities**. `deploy/vulncheck.sh` (the authoritative in-image run) was not executed
  in this pass — it needs the Docker builder image and this commit touches no dependency.
- **GCP surface** — not applicable and unchanged: self-hosted only. No IAM binding, no service
  account, no Cloud Run, no `--allow-unauthenticated`, no service-account key file, no secret
  passed as an env var in the range. `.agents/docs/deploy/gcp.md` remains "not applicable".
- **Suites run for this review** — `go build ./...` and `go vet ./...` clean;
  `go test ./internal/netpolicy/... ./internal/httpapi/... ./internal/credstore/...
  ./internal/credbroker/...` all pass (the four new tests included);
  `unshare -Urnm bash deploy/firewall/test.sh` → **passed=120 failed=0**, confirming the
  commit's claim for the reconciler. `deploy/firewall/install.sh` is not covered by any suite
  beyond `bash -n` and `shellcheck`, which is why finding 1 shipped green.
