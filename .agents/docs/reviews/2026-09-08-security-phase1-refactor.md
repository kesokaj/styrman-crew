---
owner: security
date: 2026-09-08
---
# Security review — phase1-refactor (family table in styrman-firewall.sh)   (2026-09-08)

Range reviewed: 8cbc65d..working tree (unstaged), `deploy/firewall/styrman-firewall.sh`
(the diff supplied as `phase1.diff`, 105 insertions / 41 deletions). Blast radius
followed outward: the whole input surface the script parses — marker FILENAMES under
`$STYRMAN_FW_STATE_DIR/netpolicy/lan/` (owned by the non-root server uid),
`/etc/styrman/firewall.conf` (root-owned), and `iptables -S` output — plus the one
artifact it produces that crosses back to lower privilege, `$RUN/status`, and its
consumer `internal/netpolicy/{status,dir,netpolicy}.go`. Also read
`.agents/docs/plans/2026-09-08-lan-full-parity.md` (changed in the same worktree)
because Phase 2/3 aim this refactor at a second chain.

## Verdict: FINDINGS

Nothing high, and the four defects the refactor was asked to preserve against are
preserved: no rule is deleted by a position captured in an earlier read, deletions
still precede insertions, no indirect dispatch is reachable from a lower-privileged
uid, and the fail-closed paths all hold (verified — `deploy/firewall/test.sh` passes
83/0 unmodified in a `unshare -Urnm` namespace). The worst thing here is the one
place the refactor did reintroduce a BUG-010 shape: `scan_current` and
`count_applied` now obtain their chain list through `for chain in $(chains)`, an
unquoted command substitution whose failure `set -e` + `inherit_errexit` swallow
entirely (empirically confirmed: the loop body silently does not run and the script
continues with rc 0). Because `scan_current` is the *only* producer of `PURGE` and
`MALFORMED`, while `reconcile`'s insert loop iterates `${!DESIRED[@]}` and does not
depend on `chains` at all, a silent failure there withdraws nothing and still adds
everything — tighten skipped, loosen proceeds, the exact inversion of the file's own
stated ordering promise. The trigger is narrow (the `printf | sort -u` fork failing),
but the direction is fail-open on a root firewall and the fix is smaller than the
current code.

## Findings

- **[medium / confidence: high]** `deploy/firewall/styrman-firewall.sh:384` and
  `:460` — `for chain in $(chains)` gates every deletion behind an unquoted command
  substitution. Verified in isolation on bash 5.2.37 with `set -eu` +
  `shopt -s inherit_errexit`: when the called function returns non-zero the `for`
  loop runs zero iterations, no error is raised, and the script proceeds with exit
  status 0. `PURGE`/`MALFORMED`/`SEEN` are populated *only* inside that loop
  (`scan_current`), whereas `reconcile`'s insert pass reads `${!DESIRED[@]}`
  independently — so the failure mode is "no allowance is ever revoked while
  allowances are still installed", i.e. a marker the server deleted keeps its
  head-of-chain RETURN indefinitely. This is the BUG-010 shape (a failure hidden
  inside `$( )`) landing on the one construct that gates all tightening. The same
  expression is unquoted, so it is subject to word splitting and pathname expansion;
  `set -f` is not in effect anywhere in the file. Inert with the current literal
  `STYRMAN-FWD`, live the moment a chain name carries a glob metacharacter.
  **Remediation:** drop `chains()` entirely. Build the list once, forkless, at the
  top of the LAN section — `CHAINS=(); for t in "${FAMILY_TAGS[@]}"; do
  c="${FAM_CHAIN[$t]}"; case " ${CHAINS[*]-} " in *" $c "*) ;; *) CHAINS+=("$c") ;;
  esac; done` — iterate `"${CHAINS[@]}"` quoted, and make `[ ${#CHAINS[@]} -eq 0 ]`
  an explicit hard failure rather than a silent zero-iteration loop.

- **[medium / confidence: medium]** `:390`, `:396-397`, `:415-428` — the
  numbered-delete path is now chain-generic *and* its entry condition was widened
  from `*styrman-lan:*` to `*'--comment "styrman-lan'*`, while nothing in
  `delete_malformed` forbids that path from removing a `-j DROP`. Confirmed
  behaviourally in a namespace against the real script: a
  `--comment "styrman-lanpub:<uuid>"` RETURN planted in `STYRMAN-FWD` — precisely
  what a newer script version will write, and what a rollback would leave behind — is
  removed by number on the next run. The confirm-by-full-text step before `-D` is
  sound (see "Checked"), so this is about the blast radius of the residual race the
  file already documents, not about a missing check. Phase 2 of the plan adds the
  `lanhost` family in `STYRMAN-IN`, whose entire content is the single
  `-A STYRMAN-IN -j DROP` that keeps every guest microVM off the host; from that
  point the read→`-D <n>` window exists in that chain too. Compounding it: the BASE
  insert-if-missing pass runs *first* (`:61-125`), before the LAN section that does
  all the deleting, so a base rule lost to that race is not repaired until the next
  60 s timer tick. **Remediation, in order of value:** (1) re-run the base
  `ensure_chain`/`ensure_rule`/`ensure_jump` pass at the END of the LAN section as
  well — it is idempotent and cheap, and it closes the repair window from 60 s to
  zero; (2) refuse the numbered delete when the confirmed line at position `n` ends
  in `-j DROP`; (3) put the chain and the offending comment's uuid into the warning —
  today `warn "malformed styrman-lan rule removed"` is a constant string naming
  neither, so the status file cannot tell an operator what root removed.

- **[low / confidence: high]** `:360-364` — `rule_args` constructs a function name
  from data (`local tag="${1%% *}"` … `"rule_args_${tag}"`) with no membership
  assertion against the family table. It is *not* reachable from a lower-privileged
  uid: every `DESIRED` key is written by `desire_lan` and every `PURGE` key by
  `fam_key_lan`, both with a hardcoded `lan ` first field, and marker names are
  pinned by the anchored `UUID_RE` plus `-L`/`-f` checks before they can reach any
  key. What blocks an unknown tag today is incidental — the preceding
  `CHAIN="${FAM_CHAIN[${tag}]}"` aborts under `set -u` (verified on bash 5.2:
  `M[$t]: unbound variable`, exit 1). That abort lands mid-`reconcile`, after the
  purge deletes and part-way through the inserts, and `write_status` never runs, so
  the status file retains the previous run's `lan=` list against changed kernel state
  (bounded and fail-closed downstream: `StaleAfter = 3m` in `internal/netpolicy/dir.go`
  and `Derive` then reports `error`/`unavailable`). Make the guard explicit rather
  than incidental: `[ -n "${FAM_CHAIN[${tag}]:-}" ] || { ERRORS+=("${1##* }:unknown
  family ${tag}"); return 1; }`. Related: `ARGS` is a bare global that is never
  reset, so a future `rule_args_<tag>` that returns 0 without assigning would leave
  the *previous* key's `ARGS` and `CHAIN` in place for the subsequent
  `ipt -D "${CHAIN}" "${ARGS[@]}"` — set `ARGS=()` at the top of `rule_args` and
  treat an empty `ARGS` as a hard error.

- **[low / confidence: medium]** `:206-209` vs `:392-395`, `:462-464` —
  `FAMILY_TAGS` and `FAM_CHAIN` are two independent sources of truth and the code
  walks them from opposite ends: `chains()` iterates `FAM_CHAIN`'s *values*, while
  the per-line match iterates `FAMILY_TAGS`. A tag present in `FAM_CHAIN` but absent
  from `FAMILY_TAGS` — the obvious way to disable or roll back a family — leaves the
  script still reading that family's chain while matching none of its live rules, so
  every one of them is classified malformed and removed by number. Table divergence
  lands directly on the riskiest code path. Derive the chain list from
  `FAMILY_TAGS` (as in finding 1's remediation) so an unlisted tag is inert, and add
  a startup assertion that the two tables have identical key sets.

- **[low / confidence: high]** `:217-222` — `CHAINS_CACHE` never persists. Every
  call site is `$(chains)`, so the assignment happens in a subshell and is discarded;
  confirmed empirically (the variable is still empty after the first call). The cache
  is dead code, the comment above it describes behaviour the script does not have,
  and the script forks `printf | sort -u` twice per run to deduplicate a
  one-element hardcoded array. Subsumed by finding 1's remediation.

- **[low / confidence: high]** `:203-205`, `:209` — the family regex hardcodes its
  own chain (`^-A STYRMAN-FWD `) independently of `FAM_CHAIN[lan]`, and the widened
  prefix guard depends on iptables' comment-*quoting* heuristic, not just its render
  order. Probed: a comment containing no character outside `[A-Za-z0-9_./-]` is
  rendered unquoted, so `--comment styrman-lanpolicy` on a DROP is invisible to the
  new guard and was left untouched in my namespace run. Safe for the current family
  (our comments always contain `:` and are therefore always quoted) and safe in
  direction, but it means the guard's reach is defined by an iptables formatting rule
  the script does not state. A render-order or quoting change would turn every rule
  the script itself writes into a "malformed" numbered delete, flapping each
  allowance every 60 s. `test.sh` sections 3/4 catch this on whatever iptables is
  installed on the test host; what is missing is an explicit assertion of the
  iptables variant the regex was written against (the comment claims 1.8.11
  nf_tables "pinned by deploy/firewall/test.sh", but `test.sh` pins nothing).

- **[low / confidence: high]** `:215`, `:326-332`, `:365-369` — the indirect
  dispatch blinds shellcheck to the call graph: `shellcheck -S info` now reports
  SC2317 "command appears to be unreachable" for `fam_key_lan`, `desire_lan` and
  `rule_args_lan`. `test.sh` runs `shellcheck -S warning`, and SC2317 is `info`, so
  the suite stays green — but the linter can no longer catch a misnamed handler, and
  Phase 2/3 add four more. A misnamed `rule_args_lanpub` would surface only as a
  `command not found` abort mid-reconcile (see finding 3 for that path's cost).
  Add a pre-flight assertion after the table: for every `t` in `FAMILY_TAGS`, require
  `declare -F "desire_${t}" "fam_key_${t}" "rule_args_${t}"` — a clean startup
  failure instead of a partial reconcile, and it restores what the linter lost.

## Checked

- **Secrets** — none. `git diff` grepped for credential/token/key patterns: the only
  hits are the script's own `key`/`KEY` rule-identifier variable. Nothing in the diff
  reads, logs or writes a credential. The status file is scrubbed with
  `tr -c 'A-Za-z0-9 ._:/=\n-' '_'` (`:504`) and is `root:<state gid>` 0640; the two
  new strings that reach it (`ERRORS+=("${key##* }:${key%% *} insert failed")`,
  `PENDING+=(…)`) carry only a regex-constrained uuid and a literal tag. The
  malformed-rule warning is a constant, so untrusted `-S` text is never echoed into
  the status file — good, and worth keeping.
- **Injection** — the three parsing boundaries were followed individually.
  (a) Marker filenames, the only input a non-root uid controls: constrained by the
  anchored `UUID_RE` plus `-L`/`-f` at `:345` before anything else touches them, then
  used only as an associative-array subscript (string, not arithmetic, for a
  `declare -A`) and as `printf '%s' | sha1sum` input. `test.sh` section 7 plants a
  symlink, a directory, a `sys-setup-` name and a trailing-character name; all four
  are ignored. (b) `firewall.conf`: whole-line regex allowlist at `:298`, four
  permitted keys, value charset `[A-Za-z0-9./ ,_-]`; `EVIL=$(reboot)` is rejected as
  unparseable (section 12, passing). (c) `iptables -S` output: matched against
  anchored per-family regexes, replayed only from captures, compared by full text.
  No untrusted string reaches `eval`, a glob, or an unquoted expansion — with the one
  exception recorded as finding 1, whose value is a script literal.
- **Authz/authn** — no auth surface in this file. The privilege boundary it enforces
  is root-writes / server-reads: writes go only to `$STYRMAN_FW_RUN_DIR` (asserted
  absolute, outside `$STATE`, not a symlink, root-owned, no group/world write at
  `:163-175`), and `$STATE` is read-only. The marker directory chain is re-asserted
  every run (`:337-341`: not a symlink, a real directory, owned by the state dir's
  owner or root) and yields zero allowances on any failure. Consistent with
  `.agents/rules/guest-writable-paths.md` (ADR-065): names are the whole grammar and
  the tree sits beside `users/`, under no mounted home.
- **Crypto** — none introduced. The only primitive is `sha1sum` used as a *naming*
  function to reproduce `config.BridgeNameFor`, not for authentication or integrity;
  the golden vector (`sdb-e660d58459e`) is pinned in both
  `internal/config/bridgename_test.go` and `deploy/firewall/test.sh` and matched on
  this run. Nothing hand-rolled.
- **Dependencies** — the diff adds none; no `go.mod`/`package.json` change
  (`git diff --stat` = the script plus a plan file). `govulncheck ./...` ran to
  completion here: 31 findings, 29 of them Go stdlib against my *local* `go1.25.0`
  toolchain, all with "Fixed in ≤ go1.25.13" — the release build pins
  `golang:1.25.13-trixie` (`deploy/Dockerfile.server:9`), so that set is a
  local-toolchain artifact, not a shipped one. Run `bash deploy/vulncheck.sh` for the
  authoritative in-image result. The two genuine module findings are pre-existing and
  outside this diff: `github.com/docker/docker@v28.5.2+incompatible` — GO-2026-4887
  (AuthZ plugin bypass on oversized request bodies) and GO-2026-4883 (off-by-one in
  plugin privilege validation), both "Fixed in: N/A", both reached via
  `orchestrator.New` → `client.NewClientWithOpts`. Neither uses an authz plugin, so
  exposure looks nil, but they should be tracked for an upstream fix.
  `npm audit --omit=dev` in `web/`: 0 vulnerabilities.
- **GCP surface** — none. Self-hosted deployment (`.agents/docs/deploy/gcp.md` is
  marked not applicable); no IAM binding, service account, key file, or
  `--allow-unauthenticated` anywhere in the blast radius.
- **Behavioural verification** — `unshare -Urnm bash deploy/firewall/test.sh`:
  **passed=83 failed=0**, `test.sh` unmodified, matching the baseline the plan
  claims. `bash -n` clean; `shellcheck -S warning` clean (`-S info` output analysed,
  see finding 7). `go test -count=1 ./internal/netpolicy/` green, including
  `TestFirewallScriptInNetns` and `TestMinScriptVersionMatchesScript`
  (`STYRMAN_FW_VERSION=3` matches `MinScriptVersion = 3`; correct — the wire format
  did not change, the bump belongs to Phase 2). Three targeted namespace probes were
  run against the real script to answer the widened-guard question; all three are
  read/observe-only and were confined to a private netns with a scratch state dir per
  ADR-052.
- **BUG-007 text check** — `git diff --numstat` on the changed script reports
  `105 41`, and `file` reports "Bourne-Again shell script, Unicode text, UTF-8". The
  diff is genuinely readable source, not an opaque blob.

## Answers to the four questions asked

1. **Delete by captured position — no, BUG-013's remediation survives.**
   `scan_current` records `MALFORMED+=("${chain} ${line}")` — chain and full line
   text, never a number. `delete_malformed` re-derives `n` inside the same fresh
   `ipt -S "${chain}"` read that confirms full-text equality (`:420-427`), and the
   chain is carried on the entry so the re-read targets the right list. Every
   *well-formed* rule is still deleted by spec (`ipt -D "${CHAIN}" "${ARGS[@]}"`,
   `:438`) from regex-validated captures. The residual is the documented
   read→`-D <n>` window under a concurrent root writer; the refactor does not
   introduce it but does widen what travels through it (finding 2).
2. **Tighten-before-loosen — holds.** `reconcile` (`:431-453`) runs the PURGE
   spec-deletes, then `delete_malformed`, then the inserts, in that order. The BASE
   section is unchanged and remains pure insert-if-missing; no base rule can enter
   `MALFORMED` because none carries a `styrman-lan` comment, and `test.sh` section 5
   asserts the six DROPs survive a purge (passing). The one gap is temporal, not
   logical: base repair only runs *before* the deleting section (finding 2).
3. **Indirect dispatch — not reachable from a lower-privileged uid.** Four sites:
   `"desire_${tag}"`, `"fam_key_${tag}"` (×2) and `"rule_args_${tag}"`, plus
   `FAM_CHAIN[…]`/`FAM_RE[…]` lookups. The first three take `tag` straight from the
   `FAMILY_TAGS` literal array. Only `rule_args` derives it from data, and that data
   is a key whose first field is hardcoded by its two writers. Marker names — the
   uid-controlled input — are pinned to `UUID_RE` and never reach a tag position;
   `iptables -S` text reaches only anchored regexes and full-text comparison. Guarded
   incidentally rather than explicitly (finding 3).
4. **`set -eu` + `inherit_errexit` — hold, with one exception.** Both are still set
   at `:42-46` and every new function respects them; no side-effecting `ipt` call
   moved inside a command substitution. The exception is structural, not an `ipt`
   call: `for chain in $(chains)` swallows the helper's failure completely, which is
   the same class of blindness BUG-010 was about (finding 1).
5. **Fail-closed — holds, verified.** Unreadable/invalid conf → `conf_fail` sets
   `CONF_ERROR` and empties `LAN_CIDRS` → `desire_lan` iterates an empty array →
   `DESIRED` empty → `scan_current` marks every existing tagged rule undesired →
   purged (section 12, twelve cases passing). Missing marker directory → `note` +
   `return 0` with `DESIRED` empty → same outcome. Symlinked or wrongly-owned marker
   directory → `warn` + `return 0`, no allowances. Hostile marker names → rejected
   and counted in `ignored` (section 7). A stat racing away mid-check yields an empty
   substitution, which fails the ownership comparison and closes — checked. One
   behavioural change from the refactor is benign: `WANT_COUNT` no longer gets a `0`
   entry when `LAN_CIDRS` is empty (it is simply absent), and `count_applied`'s
   `-gt 0` guard made that entry a no-op anyway, so `lan=` is identical.
