---
owner: security
date: 2026-09-08
---
# Security review — gate round 3 (2026-09-08)

Range reviewed: 3e0bb9c..dc5bad4

Blast radius followed outward from the diff: `deploy/firewall/install.sh` (the whole
argument surface, both opt-in flags, and the sshd/`/etc/ssh` config it parses),
`deploy/firewall/styrman-firewall.sh` (root's consumer of the conf it writes),
`internal/netpolicy/dir.go` → `internal/httpapi/network.go:307 LanExposed` → the
credential broker's key-withholding decision, `deploy/docker-compose.yml:75,133`
(the container uid and the `/run/styrman-firewall` bind), and `deploy/firewall/test.sh`
§24 as the guard the commit relies on to stop round 1's bug class recurring.

## Verdict: FINDINGS

Round 2 did it again, in the same function it was fixing. `require_sshd_hardened`
now resolves sshd off PATH (correct, and needed — `sshd` really is off a non-root
PATH on this host), but `install.sh` runs under `set -euo pipefail`, and
`pw="$(sshd -T … | sed … | head -1)"` at line 102 aborts the whole script the moment
`sshd -T` exits non-zero. Reproduced: `install.sh --lan-allow-host 1 --dry-run` exits
**1 with completely empty stdout and stderr** — the `:*|*:` "cannot read sshd's
effective config" `die` added by this very commit at line 108 is unreachable in
exactly the case it was written for. At 3e0bb9c the `command -v sshd` guard
short-circuited before that pipeline on this same host, so round 2's PATH fix is what
routed a supported invocation into a silent abort, and the commit message's claim
that "install.sh --dry-run reaches the end on both opt-in paths" is false for the
`--lan-allow-host` path unless you are root with loadable host keys. It fails closed
at the machine level; the security cost is that an operator who gets a bare exit 1
with no diagnostics reaches for `--i-accept-sshd-passwords`, which turns the R12 gate
off entirely.

Second: the new Match-block refusal is case-sensitive and sshd's keywords are not, and
the `-C` probe address is in no Docker network that exists. Verified against
OpenSSH_10.0p2 on this host — a config of `PasswordAuthentication no` plus
`match address 172.18.0.0/16` / `passwordauthentication yes` is honoured by sshd
(probe from 172.18.0.5 returns `yes`) while `grep -E '^[[:space:]]*Match[[:space:]]'`
returns zero hits and the gate's fixed probe at 172.16.0.2 returns `no`. The gate
prints "R12 precondition met" on precisely the configuration the round-2 commit
message says it now forbids.

The `Installed()` change (claim 3) is sound and is the one unambiguous improvement in
the range. The `sshd -T -d` in the `cfg=` line (claim 2) is dead code but harmless.
The §24 structural test (claim 4) does catch both round-1 bugs — I re-ran it against
3e0bb9c's file and it reports them — but its enumeration is narrow enough that a
function written in any of four ordinary forms is silently not checked at all.

## Findings

- **[high / confidence: high]** `deploy/firewall/install.sh:102-103` — under
  `set -euo pipefail` (line 16), `pw="$("${sshd_bin}" -T 2>/dev/null | sed … | head -1)"`
  propagates `sshd -T`'s non-zero exit through `pipefail` into the assignment, and
  `set -e` kills the script there. Same for `probe=` on line 103. Proven with
  `bash -x`: execution stops immediately after `pw=`, before the `case`, printing
  nothing at all. `sshd -T` exits non-zero whenever host keys are unreadable (any
  non-root run, i.e. every `--dry-run` preview on a host with sshd installed) or the
  sshd config has a syntax error — the two situations the `:*|*:` branch exists to
  report. Exploitable in principle because the fail-closed abort is indistinguishable
  from a broken script: the operator is not told to harden sshd, is not told about the
  documented override's real purpose, and the shortest path out of a silent exit 1 is
  `--i-accept-sshd-passwords`, which skips R12 enforcement entirely (line 86).
  Remediation: capture the exit status instead of letting it kill the script — e.g.
  `if ! raw="$("${sshd_bin}" -T 2>/dev/null)"; then die "cannot read sshd's effective
  config …"; fi` and then `pw="$(printf '%s\n' "${raw}" | sed -n …)"`, same for the
  `-C` probe; do not wrap a pipeline whose first stage may legitimately fail in a
  bare `set -e` assignment.

- **[medium / confidence: high]** `deploy/firewall/install.sh:104` — the Match-block
  refusal greps `-E '^[[:space:]]*Match[[:space:]]'`, case-sensitively. sshd_config
  keywords are case-insensitive (sshd(8): "keywords are case-insensitive and arguments
  are case-sensitive"). Verified on OpenSSH_10.0p2: a file containing
  `match address 172.18.0.0/16` + `passwordauthentication yes` is honoured by sshd
  (`sshd -T -C addr=172.18.0.5,user=root,host=…` → `passwordauthentication yes`) and
  produces zero grep hits, so `cfg` is empty and the gate logs "no Match blocks — R12
  precondition met". Two further gaps in the same expression: the scan covers only
  `/etc/ssh/sshd_config` and `/etc/ssh/sshd_config.d`, so a `Match` in any file pulled
  in by an `Include` pointing elsewhere is invisible; and `sshd -T` reads the default
  config path regardless of what the running daemon was started with (`-f`).
  Remediation: `grep -riE '^[[:space:]]*match[[:space:]]'`, and resolve `Include`
  targets (or refuse when an `Include` names a path outside `/etc/ssh`).

- **[medium / confidence: high]** `deploy/firewall/install.sh:103` — the `-C` probe is
  hardcoded to `addr=172.16.0.2`, which is in no Docker network. Docker's default
  local-scope pool starts at 172.17.0.0/16; on this host the real project bridges are
  172.19.0.0/16 and 172.21.0.0/16 and `styrman-ingress` is 172.18.0.0/16 —
  172.16.0.0/16 is used by nothing. Verified: with an *uppercase*
  `Match Address 172.18.0.0/16` → `PasswordAuthentication yes`, the gate's probe at
  172.16.0.2 returns `no` while a probe at 172.18.0.5 returns `yes`. So the `-C` probe
  contributes no coverage for any network actually granted, and the entire weight of
  the Match defence rests on the case-sensitive grep above. The probe would also miss
  a `Match Address <LAN_CIDRS>` block, which is the shape an operator granting LAN
  access is most likely to have written. Remediation: derive probe addresses from the
  real bridge subnets (the script already runs `docker network inspect` for the ingress
  in `derive_ingress_bridge`) and from each `LAN_CIDRS` entry, and probe each in a
  loop; the syntax itself is fine — `addr`/`user`/`host` are accepted keys and the
  spec is well-formed (confirmed against OpenSSH_10.0p2).

- **[low / confidence: high]** `deploy/firewall/test.sh:365-379` — §24's structural
  check has silent false negatives, and an unchecked function produces no output, so
  the suite still reports PASS. Enumeration is `grep -oE '^[a-z_]+\(\)'`, so a
  definition written `function name {`, `name () {` (space before the parens), indented,
  or with a digit or uppercase letter in the name is never enumerated and therefore
  never checked — all four verified by construction. The call regex's leading class
  `(^|[;&|[:space:]])` excludes `(` and backtick, so `x="$(fn …)"` and `` x=`fn` `` are
  missed — also verified. Comment-stripping only removes whole-line comments, so a
  trailing `# … run …` can produce a spurious hit; that direction fails loudly and is
  safe. The check does work today (I re-ran it against 3e0bb9c and it correctly
  reports `die(23<40)` and `derive_ingress_bridge(116<118)`), and passes on the current
  file; the risk is that it quietly stops covering whatever is written next.
  Remediation: enumerate with
  `^[[:space:]]*(function[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*[[:space:]]*(\(\))?[[:space:]]*\{`,
  add `(` and a backtick to the before-class, and make an enumerated name with no
  resolvable definition line a hard failure rather than a skipped iteration.

- **[low / confidence: high]** `deploy/firewall/install.sh:104` — `"${sshd_bin}" -T -d
  2>/dev/null >/dev/null;` inside the `cfg=` substitution is dead code: both streams go
  to `/dev/null` and it is followed by `;`, so `cfg` is entirely the grep's output. It
  is harmless today for two reasons that are both accidents rather than intent —
  `inherit_errexit` is off and `install.sh` sets no `shopt` (verified: with
  `inherit_errexit` on, the failing `sshd -T -d` would abort the substitution and empty
  `cfg`, silently disabling the Match-block refusal), and `-T` forces test mode so the
  `-d` never starts a listening debug daemon. A future edit that adds
  `shopt -s inherit_errexit` or drops the `-T` turns a leftover into a real defect.
  Remediation: delete the subshell.

- **[low / confidence: high]** `deploy/firewall/install.sh:106-116` — the
  `case "${pw}:${probe}"` list cannot fail open (only the literal `no:no` passes; I
  enumerated all nine combinations of `no`/`yes`/empty and every other one dies), but it
  mis-attributes two of them: `pw=yes, probe=""` and `pw="", probe=yes` both hit the
  `:*|*:` arm and report "cannot read sshd's effective config" when the truth is
  "password authentication is on". Cosmetic in effect, misleading in the exact spot
  where the operator decides whether to reach for the override. Remediation: test the
  empty case first (`""|*:""`), then `no:no`, then refuse.

- **[low / confidence: medium]** `internal/netpolicy/dir.go:114` — the change to stat
  the status file is correct and is a real improvement (the compose bind at
  `deploy/docker-compose.yml:133` does make the directory exist unconditionally, and the
  container runs as `${STYRMAN_UID}:${STYRMAN_GID}` per line 75 against a root-owned
  0755 run dir, so the server's uid cannot forge the file — `Installed()` is now
  root-attested). All three cases asked about hold: never-installed compose host →
  no file → record-only → key issued, which is right because no rule can exist;
  installed-then-uninstalled without a reboot → the file lingers on the /run tmpfs →
  `Installed()` true and `Status()` stale → `Exposed` (status.go:209) returns true for
  every project → keys withheld, conservative and safe; after a reboot the tmpfs is
  clear and the reading is correct again. The residual is that `Installed()` and
  `Status() != nil` now share one input, so BUG-014's "rules may be live, root's word
  unreadable" state survives only when the file exists but is unparseable/oversized —
  which the new test does pin. If the status file is *absent* while rules are still
  live, `LanExposed` (network.go:321) falls back to the record and would issue the SSH
  key to a project whose record is off while a stale rule stands. Reaching that needs
  root to delete the file, or `reconcile` to apply rules and then die before
  `write_status` (styrman-firewall.sh:730-737, where `write_status` runs after
  `reconcile`) on a boot with no prior status — and since the rule set is derived from
  the records, I could not construct a realistic instance. Recorded as theoretical.
  Remediation: none required; if the ordering is ever revisited, write a
  `schema=1`-only status file before `reconcile` so "root's half is present" is
  asserted before any rule is applied.

## Verification of the four claims

1. **Ordering.** Confirmed. No function in `install.sh` is now called before its
   definition: `log`/`die`/`run` at 35-37 precede the loop at 39-65,
   `require_sshd_hardened` (84) precedes its call (126), `derive_ingress_bridge` (200)
   precedes its call (216). The `run` hoist is behaviour-neutral because `DRY` is read
   at call time, is initialised at line 30 before the definition, and every `run` call
   is after the loop (first at 130) — confirmed by the dry-run emitting `+ …` lines.
   The `die` hoist is a real fix: `--lan-allow-host 2 --dry-run` gives
   `FATAL: --lan-allow-host takes 0 or 1` / exit 1 at dc5bad4 versus
   `die: command not found` / exit 127 at 3e0bb9c. End to end: the
   `--lan-allow-published-ports 1` dry-run reaches "dry run complete", derives
   `br-02a045873a1d`, and writes `INGRESS_BRIDGE=` into the conf body, so round 1's
   conf fix is reachable. The `--lan-allow-host 1` dry-run does **not** reach the end —
   see the high finding; with `--i-accept-sshd-passwords` it does.
2. **sshd gate.** `-C` spec well-formed and accepted; `case` list cannot fail open;
   `cfg=` leftover is dead code — but the Match refusal is case-blind and the probe
   address is fictional. See the two medium findings and two lows.
3. **`Installed()`.** Reasoning holds; one theoretical residual recorded as low.
4. **§24 test.** Real and effective against the bug it was written for; false negatives
   recorded as low.

## Checked

- **Secrets** — clean. Every added line in the range grepped for
  `password|secret|token|api[-_]key|BEGIN … PRIVATE KEY|AKIA|ghp_|xox[baprs]-`; every hit
  is prose or the literal sshd option name `passwordauthentication`. No credential
  material, no committed `.env`, nothing echoed into a log. `deploy/vulncheck.sh`
  already shadows `deploy/.env` before handing the tree to a container.
- **Injection** — the new external-input surfaces are `sshd -T` output (consumed by a
  fixed `sed -n 's/^passwordauthentication //p'` into a `case` with literal patterns, so
  no expansion path) and `grep -rl` over `/etc/ssh` (root-owned; the resulting filename
  is only interpolated into a `die` message). The argument loop's `die` calls are now
  reachable, which is a strict improvement. `${fn}` in test.sh §24 is interpolated into
  `grep -E` patterns, but the names come from `^[a-z_]+\(\)` in a repo-controlled file,
  and `ORDER_BAD` is single-quoted into the `check` expression. No new shell injection.
- **Authz/authn** — the only authorization-relevant change is `Installed()`, which
  gates the credential broker's SSH-key withholding via `LanExposed`. Analysed above:
  no fail-open path I could construct; the server's uid cannot forge the signal.
  `require_sshd_hardened` is a deploy-time precondition, not a request-path check.
- **Crypto** — nothing hand-rolled. No auth, hashing, session, or JWT/OIDC code in the
  range. The commit reads OpenSSH's own effective config rather than parsing sshd_config
  itself, which is the right instinct; the defects are in the reading, not in a
  home-grown construction.
- **Dependencies** — no dependency change in the range (`go.mod`, `go.sum`,
  `web/package.json`, `web/package-lock.json`, and the ingress module's are all
  untouched). `bash deploy/vulncheck.sh` (the authoritative scan, inside the pinned
  `golang:1.25.13-trixie`): root module and `deploy/caddy/ingress` each affected by
  GO-2026-4887 and GO-2026-4883 in `github.com/docker/docker` (v28.5.2 / v27.5.1), both
  with no fix available and both already documented as accepted-per-release in
  `deploy/vulncheck.sh`'s header; `image/harness` clean; exit 3. A host-toolchain run
  (`govulncheck ./...` on go1.25.0) additionally reports 29 stdlib findings — that is
  the artefact the project documents, not a property of the shipped binary, which
  builds on 1.25.13. `npm audit --omit=dev` in `web/`: 0 vulnerabilities.
- **GCP surface** — not applicable and not touched. This is a self-hosted deployment
  (`.agents/docs/deploy/gcp.md` says so); no IAM binding, service account, Cloud Run
  service, `--allow-unauthenticated`, or service-account key file appears anywhere in
  the range.
- **Suites run** — `unshare -Urnm bash deploy/firewall/test.sh` → passed=121 failed=0
  (matches the commit's claim, including §24 and shellcheck at `-S warning`);
  `go vet ./...` clean; `go test ./internal/netpolicy/... ./internal/httpapi/...` ok.
  All files in the range are UTF-8 text (`file` + `git diff --numstat`), so nothing in
  this review was signed off unread (BUG-007).
