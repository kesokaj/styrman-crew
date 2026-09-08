---
owner: reviewer
date: 2026-09-08
---
# Review — gate round 3 cold read (2026-09-08)

Range reviewed: no diff — the four files named by the dispatch, as they stand at HEAD (`dc5bad4`):
`deploy/firewall/styrman-firewall.sh`, `deploy/firewall/install.sh`,
`internal/netpolicy/dir.go`, `internal/netpolicy/status.go`. Read as a cold reader;
history, ADRs and plans deliberately not consulted.

## Verdict: REQUEST CHANGES

The worst problem is not in the reconciler's rule algebra — that part holds — but in the
seam between it and the server. `styrman-firewall.sh` reports a project in `lan=` only
when the rules it *desires* for that project are all present. On the two paths where a
**revocation fails** — a `-D` that returns non-zero (`reconcile`, line 645) or a chain read
that skips the whole reconcile (line 731/735) — the rule stays live in the kernel while the
project drops out of `lan=` entirely, because a revoked project has no `DESIRED` entries
left to count. `netpolicy.Exposed` looks at nothing but `st.Applied`, so with the record
already OFF it answers "not exposed" and the credential broker hands the host-reaching SSH
key to a project that still has local-network reach. That is the exact pairing the
`Exposed` doc comment says it exists to prevent, and root wrote an `error=` line saying so
that the Go side never reads. Second: `WANT_COUNT` counts desired *entries* while `DESIRED`
dedups *keys*, so one repeated CIDR permanently pins every LAN-enabled project to `error`
and permanently defeats the same 60 s disable window. Third: the installer's R12 sshd gate
both aborts silently on the failure it was written to explain, and is skipped on every path
that turns `LAN_ALLOW_HOST=1` on other than one explicit flag.

To the dispatch's headline question directly: **I found no path where rules are INSERTED
without the corresponding stale rules being REMOVED.** The two guards that carry that
promise — `scan_current` returning non-zero skipping tightening *and* loosening together,
and the delete-by-spec pass running before the insert pass inside `reconcile` — both hold,
and the `$(...)`-yields-empty-list hazard the comments describe is genuinely avoided by
building `CHAINS` as a real array. The asymmetry that does exist runs the other way, and is
a *reporting* asymmetry: a rule that could not be removed is reported to the server as
absent.

## Findings

- **[high / confidence: medium]** `internal/netpolicy/status.go:208` (`Exposed`) with
  `deploy/firewall/styrman-firewall.sh:642-647` and `:731-736` — a failed revocation reads
  as "not exposed". Sequence: project P has LAN on, rules applied, `lan=…P…` in the status.
  Operator turns it off; `p.LanAccess` is false immediately. Root's next run either
  (a) fails `ipt -D` and records `ERRORS+=("${key##* }:delete failed")`, or (b) fails
  `ipt -S` on either chain, so `scan_current` returns 1, `reconcile` and `count_applied` are
  both skipped and `lan_list` stays empty. In both cases the RETURN rule is still in
  `STYRMAN-FWD`/`STYRMAN-IN`, and in both cases P is absent from `lan=` — in (a) because
  `count_applied` (line 677) counts only keys present in `DESIRED`, and a revoked project
  has none. `Exposed(record=false, …)` then returns `st.Applied[P]` = false, and
  `internal/httpapi/network.go:325` → `credbroker` serves the host-reaching SSH key to a
  project that can still reach the LAN. The status file carries root's own confession
  (`error=<uuid>:delete failed`, `error=all:chain read failed - no rules added or removed
  this run`) and `Exposed` reads neither. Fix, two halves: (1) in `Exposed`, treat
  `st.Errors[uuid] != ""` as exposed, and treat a status whose warnings/errors say the
  reconcile was skipped as "cannot confirm" (the same rule BUG-014 applied to the UI side);
  (2) better, in the script, make `count_applied` also emit the uuids whose *tagged rules
  are still present in the kernel regardless of `DESIRED`* — e.g. a `present=<uuid>` key —
  and have `Exposed` union `Applied` with it. Then "root could not remove it" and "root
  still has it applied" are the same answer, which is what the doc comment already assumes.

- **[medium / confidence: high]** `deploy/firewall/styrman-firewall.sh:451-479` —
  `WANT_COUNT` and `DESIRED` disagree on what a duplicate is. `DESIRED` is an associative
  array, so `desire_lan` writing the same key twice stores one entry; `WANT_COUNT[uuid]` is
  bumped unconditionally on each iteration. One repeated value in `LAN_CIDRS` therefore
  yields WANT=2 against a kernel that correctly holds one rule, `count_applied` (line 686)
  never reaches equality, the project never enters `lan=`, and the status carries
  `error=<uuid>:1 of 2 rules present` on every run forever. Consequences: the UI pins every
  LAN-enabled project to `StateError` (`Derive`, status.go:249) though the firewall is
  exactly right; and — worse — because the uuid is never in `lan=`, the 60 s disable window
  `Exposed` relies on is defeated for every project on that host, giving finding 1 without
  needing any iptables failure at all. Triggers: `--lan-cidr` passed twice, a hand-edited
  conf, or `install.sh:263`'s auto-discovery on an uplink carrying two addresses in the same
  prefix (`ip route show … scope link` lists the connected prefix once per source address).
  Fix: bump `WANT_COUNT` only when the key was not already in `DESIRED`
  (`[ -n "${DESIRED[$k]:-}" ] || WANT_COUNT[...]=$((…+1))`), in all three `desire_*`
  functions — or dedupe `out` in `validate_cidrs` (line 313), which fixes only this family.

- **[medium / confidence: high]** `deploy/firewall/install.sh:102-108` — the R12 sshd gate
  aborts silently on the one failure it was written to report. `set -euo pipefail` is in
  force, so `pw="$("${sshd_bin}" -T 2>/dev/null | sed … | head -1)"` takes the pipeline's
  status through `pipefail` into the assignment; a non-zero `sshd -T` (broken config,
  missing host keys, an `sshd` too old for `-T -C`) exits the installer at that line with
  status 1 and no output. The `:*|*:` branch at line 108, whose whole purpose is to print
  "cannot read sshd's effective config … pass `--i-accept-sshd-passwords`", is therefore
  unreachable. Verified on this box: `bash -c 'set -euo pipefail; v="$(false | sed -n p |
  head -1)"; echo no-exit'` exits 1 without printing. Fails closed, but the operator gets a
  bare exit code on the one command they are told to run. Fix: capture with
  `if ! raw="$("${sshd_bin}" -T 2>/dev/null)"; then die "…"; fi` and parse `raw` afterwards,
  for both `pw` and `probe`.

- **[medium / confidence: high]** `deploy/firewall/install.sh:126` — the R12 sshd
  precondition is bound to the flag, not to the state. `require_sshd_hardened` runs only
  when `--lan-allow-host 1` is on *this* command line. Three ways to reach
  `LAN_ALLOW_HOST=1` without it: (1) the conf this installer writes tells the operator to
  hand-edit and `systemctl start styrman-firewall.service` (the comment block at line 275),
  which never involves the installer; (2) a re-install or the auto-updater's `host_converge`
  over an existing `LAN_ALLOW_HOST=1` conf takes the branch at line 248 and only *logs*;
  (3) sshd's config drifts after a one-time gated install and nothing re-checks. The
  script's own comment at line 82 says "a precondition no code checks is a wish. This checks
  it" — it checks it once, at one entry point. Fix: after the conf is written or read
  (branches at 225/248/253), if the resulting `LAN_ALLOW_HOST` is 1, run
  `require_sshd_hardened` regardless of how it got there.

- **[low / confidence: medium]** `internal/netpolicy/status.go:249` (`Derive`) — the
  `lan && applied` fast path returns `StateOn` without consulting
  `st.Version < MinScriptVersion`, which only `Availability` checks and which `Derive`
  reaches only on the `lan && !applied` path. After a rollback to a v3 script (a documented
  procedure in `specs/styrman/versioning.md`) an already-enabled project keeps rendering
  "on" while the host applies only the v3 family — no `lanhost`, no `lanpub`. That is the
  silent under-apply the `MinScriptVersion` doc comment says must be refused rather than
  under-applied. Fix: check the version before returning `StateOn` and fall through to
  `StateUnavailable` with `Availability`'s reason.

- **[low / confidence: medium]** `deploy/firewall/styrman-firewall.sh:83-84` and `:113-125`
  — two base-rule failures that never reach the server. The `xt_physdev` fallback writes
  "WARN xt_physdev unavailable — broker may be blocked" straight to stderr instead of
  calling `warn`, so it never enters `WARNS` and never reaches the status file, which is the
  server's only view of root; the operator sees a healthy status while the same-bridge
  RETURN (broker + port-preview) is missing for every project. The whole IPv6 block is
  `|| true` end to end: if `ip6tables` fails, no `STYRMAN-IN6`/`STYRMAN-FWD6` chain exists,
  nothing is recorded anywhere, and R12's isolation then rests solely on the per-bridge
  `disable_ipv6` sysctl at line 111 — which is also `|| true` and only covers bridges that
  exist at that tick, leaving up to 60 s for a newly created bridge. Fix: route both through
  `warn`/`note` so they land in `warn=`.

- **[low / confidence: medium]** `deploy/firewall/styrman-firewall.sh:163-175` —
  `prepare_run_dir` is called as `prepare_run_dir || exit 0`, so errexit is disabled for its
  entire body, and the function ends in an unconditional `return 0`. If
  `install -d … "${RUN}/spool"` (line 173) fails — the likeliest cause being `spool`
  existing as a non-directory — the failure is swallowed, the run continues, `drain_spool`
  returns early on `[ -d ]`, `spool_seen` reads 0, and the spool never exists: every server
  `Kick` fails (warned at most hourly, `dir.go:243`), toggles apply only on the 60 s timer,
  and the status file says nothing. Fix: `install -d … || { warn "cannot create ${RUN}/spool
  — LAN section skipped"; return 1; }`.

- **[low / confidence: high]** Dead code. `deploy/firewall/styrman-firewall.sh:427` — the
  `*) note "conf: ${key} is not acted on"` arm is unreachable: the regex at line 419 admits
  exactly the five keys that all have their own arm. `deploy/firewall/install.sh:104` —
  `"${sshd_bin}" -T -d 2>/dev/null >/dev/null;` discards both its output and its status
  inside the command substitution; it is a no-op preceding the `grep`. Delete both.

## Answers to the dispatch's five questions

1. **Insert without remove.** None found. `scan_current` returning 1 skips `reconcile` and
   `count_applied` together (line 731), so tightening and loosening are never split;
   `reconcile` runs the purge pass and `delete_malformed` before the insert pass (642-660);
   a purge whose `-D` fails leaves the rule but the following `ipt -C` in the insert pass
   then short-circuits, so no duplicate is created; the one-time migration deletes
   (101-104) were correctly moved *after* the replacement chains and jumps. The `-S`-read
   failure modes all return rather than reading as "chain empty". The residual is the
   reporting asymmetry in findings 1 and 2.
2. **`${tag}` values.** Exactly `lan`, `lanhost`, `lanpub`. Every construction site iterates
   `FAMILY_TAGS` (497, 580, 673) except `rule_args`, where `tag="${1%% *}"` comes from a key
   built by `desire_*` or `fam_key_*` and is checked against `FAMILY_TAGS` before any
   `"rule_args_${tag}"` expansion (507-521). No marker name, conf value or `-S` line can
   reach a function name. Pass.
3. **Swallowed failures.** Findings 6 and 7 above are the two that matter.
   Also noted, not findings: `spool_seen` (line 186) is a pipeline in a script without
   `pipefail`, so a `find` failure reads as 0 — cosmetic, the value is status-only;
   `scan_current` is called in an `if`, disabling errexit throughout its body, but every
   command there whose failure matters is explicitly tested; `flock` being absent would be
   indistinguishable from a busy lock (line 181) and exits 0 without a status write, which
   ages out to `stale` in 3 minutes and reads as exposed — the safe direction. The
   `[ … ] && cmd` and `[ … ] || for …; done` constructs in `install.sh` (86, 90-92, 221-223,
   274) were checked against bash 5.2 empirically and do **not** trip errexit.
4. **Associative arrays.** `DESIRED`/`SEEN`/`PURGE` share one key space and cannot collide
   across families: the tag is the first field and no field can contain a space (every
   component is produced by a validating regex — `UUID_RE`, `CIDR_RE`, the `LAN_DEV` and
   bridge-name patterns), so differing arity between `lan` (6 fields) and the other two
   (5) is harmless. `fam_key_*` round-trips exactly what `desire_*` builds, in the order
   `iptables -S` renders. `WANT_COUNT`/`APPLIED_COUNT` are keyed by uuid across all
   families, which is the intended all-or-nothing semantics — but their counting bases
   disagree (finding 2). One further quirk, safe direction: `APPLIED_COUNT` counts
   occurrences, so an undeleted duplicate can make the total reach `WANT_COUNT` while a
   different rule is missing — the project reads "on" while some traffic is still dropped.
5. **`dir.go`/`status.go`.** Yes, in one direction: the server can conclude "root applied
   nothing" while root still has rules live — findings 1, 2 and 5. I could not construct the
   opposite: `lan=` is written only from a post-reconcile read of the kernel
   (`count_applied`), `Installed()` correctly tests the status *file* rather than its
   Docker-created parent directory, `stale()` covers clock skew in both directions, and
   `readStatus` rejects an oversized or unparseable file into `nil`, which `Derive` and
   `Exposed` both treat as "cannot confirm".

## Criteria

1. **Dead code** — fail (low): `styrman-firewall.sh:427` unreachable `case` arm;
   `install.sh:104` no-op `sshd -T -d`; `install.sh:108`'s `die` unreachable under
   `pipefail` (finding 3).
2. **Flow tracing** — fail: the script→status→`Exposed` path drops root's own error
   reporting on the floor (finding 1); `WANT_COUNT`→`lan=`→`Derive` breaks on a duplicate
   (finding 2); `install.sh`'s gate does not follow the state it guards (finding 4).
3. **Standards conformance** — pass with a note. `set -eu` + `inherit_errexit` is in force
   and the `$(...)`-swallowing hazard (BUG-010) is genuinely avoided, including in the
   `CHAINS` construction. `pipefail` is deliberately absent and only bites cosmetically
   (Q3). Go side: errors wrapped with context, no discarded `err`, no floating goroutines;
   `Sync`/`SetMarker` share `d.mu` and `Kick`/`KickThrottled` share `d.kickMu` consistently.
   The cached `*Status` is handed to concurrent handlers by pointer with live maps inside —
   no current caller mutates it, so not a finding, but it is one careless `st.Applied[x]=…`
   away from a data race.
4. **Directory layout** — pass. Root-run shell under `deploy/firewall/`, server half under
   `internal/netpolicy/`, tests beside both (`script_test.go`, `dir_test.go`,
   `status_test.go`, `deploy/firewall/test.sh`).
5. **Simplicity (DIY-except-security)** — pass. No new dependency, stdlib-only Go, no
   hand-rolled auth/crypto/session code (the sha1 bridge-name derivation is an identifier,
   not a security primitive, and is pinned to the Go side by a golden vector). The shell
   metaprogramming is the one place complexity is bought, and it buys a real second and
   third caller (three families over one reconcile) with the function-name dispatch
   validated against the table first.
6. **Decoupling** — pass. The rule-family table is a genuine seam: a new family is a row
   plus two small functions. The shell↔Go contract is pinned in three places
   (`MinScriptVersion` vs `STYRMAN_FW_VERSION`, the bridge-name golden vector, the status
   grammar tests). The one gap is semantic rather than structural: `lan=`'s meaning
   ("desired and present") is assumed by `Exposed` to also mean "root has nothing of this
   project's left", and nothing machine-checks that — which is finding 1.

## Tests I would want before this ships

None of the three highest findings is covered. Specifically: a `deploy/firewall/test.sh`
case where a purge `-D` fails and the status must still name the project; a case with a
repeated `LAN_CIDRS` value asserting the project reaches `lan=`; and a Go test that
`Exposed(false, uuid, st, now)` is true when `st.Errors[uuid]` is set.
