---
owner: main
date: 2026-09-08
---
# Preflight gate — v1.24.0 (ADR-066, LAN full parity)

Range reviewed: v1.23.0..dc5bad4

## VERDICT: NO-SHIP

Three review rounds produced **16 findings**. All of round 1 and round 2 are
fixed; **six from round 3 are open**. More importantly than the count: in
each of the three rounds, the fixes from the previous round introduced the
next round's worst defect. That pattern, not any single finding, is why this
stops here rather than going to a fourth patching round — `AGENTS.md` caps
review loops at three for exactly this situation.

Nothing is tagged, pushed as a tag, or deployed. Production remains on
v1.23.0 and is unaffected: none of this is released, and both new conf keys
default closed, so even a deploy of this tree would change no behaviour until
a human opts in.

## Open findings (all from round 3)

### 1. [high/med] A failed revocation serves the SSH key to a project that still has LAN reach
`internal/netpolicy/status.go` `Exposed` + `deploy/firewall/styrman-firewall.sh`
(`count_applied`, the skip-both guard).

When `ipt -D` fails, or `scan_current` returns 1 and the whole reconcile is
skipped, the RETURN rule stays live — but `count_applied` counts only
`DESIRED` keys, so the uuid drops out of `lan=`. With the record already off,
`Exposed` reads that as "root confirms it is gone" and the broker serves the
host-reaching SSH key to a project that still has LAN reach. That is exactly
the pairing ADR-066 part (4) exists to prevent.

Root DOES report it — `error=<uuid>:delete failed`, `error=all:chain read
failed…` — and the Go side reads neither. Note the provenance: the
skip-the-reconcile path is a round-1 fix for a fail-open read, so this hole
was created while closing another one.

**Fix direction:** `Exposed` must also return true on `st.Errors[uuid]`, and
the script needs a distinct status key for "the reconcile did not run" that
`Exposed` treats as cannot-confirm. A global `error=all:` line currently lands
in `Warnings`, indistinguishable from any other warning.

### 2. [medium/high] `WANT_COUNT` does not dedup where `DESIRED` does
`styrman-firewall.sh` `desire_lan`/`desire_lanhost`/`desire_lanpub`.

`DESIRED` is an associative array and collapses a repeated key; `WANT_COUNT`
increments per loop iteration. One duplicated CIDR in `LAN_CIDRS` therefore
pins every LAN project at `error=<uuid>:1 of 2 rules present` permanently,
and — with finding 1 — defeats the 60 s disable window `Exposed` depends on.

### 3. [high/high] The sshd gate aborts silently as non-root
`install.sh:102-103`. Under `set -euo pipefail`, `pw="$(sshd -T … | sed …)"`
propagates `sshd -T`'s non-zero exit and `set -e` kills the script at the
assignment, so the "cannot read sshd's effective config" `die` at :108 is
unreachable. `install.sh --lan-allow-host 1 --dry-run` exits 1 with **empty
stdout and stderr**.

**This report must record that the dc5bad4 commit message asserts the
opposite.** It claims "install.sh --dry-run reaches the end on both opt-in
paths". That claim was verified only under `sudo -n`, where sshd is on PATH
and the pipeline succeeds; the non-root path was never exercised and does not
reach the end. A general claim was drawn from a single-path observation. The
commit cannot be amended (pushed), so the correction lives here.

### 4. [medium/high] The Match-block refusal does not refuse
`install.sh:104`. The grep is case-sensitive (`Match[[:space:]]`) while sshd
keywords are case-insensitive, and the `-C` probe address `172.16.0.2` is in
no Docker network on this host (real bridges: 172.19, 172.21; ingress
172.18). Verified by the reviewer on OpenSSH_10.0p2: `match address
172.18.0.0/16` + `passwordauthentication yes` is honoured by sshd, produces
zero grep hits and `probe=no` — so the gate prints "R12 precondition met" on
precisely the configuration it was written to forbid. It also misses `Match`
in `Include`d files outside `/etc/ssh`.

Same line carries a leftover `"${sshd_bin}" -T -d 2>/dev/null >/dev/null;`
subshell whose output is discarded — dead code in a root-run gate.

### 5. [medium/high] The R12 gate is bypassable by the documented path
`install.sh`. The gate runs only for an explicit `--lan-allow-host 1`. A
hand-edited conf — which the conf's own comments document as the way to
change it — and every re-install over an existing `LAN_ALLOW_HOST=1` skip it
entirely. The check should be on the EFFECTIVE resulting state, not the flag.

### 6. [low/high] The ordering test has silent blind spots
`test.sh` §24 works — re-run against 3e0bb9c it catches both round-1 ordering
bugs — but it skips functions written `function name {`, `name () {`,
indented, or containing a digit or uppercase, and misses calls inside `$(…)`
or backticks. An unchecked function produces no output, so the suite still
reports PASS.

## Fixed and verified (rounds 1-2, 10 findings)

Round 1 (`3e0bb9c`): the broker guard was dead code (`Enabled()` can never be
false) and would have withheld the SSH key from every project on any host
without a readable status file; `install.sh` wrote `$'\n'` into a fresh conf,
closing every allowance; R12's sshd precondition was enforced by nothing;
three `ipt -S … || true` reads revoked nothing while still inserting; a
wrong-but-valid `INGRESS_BRIDGE` failed open; the consent dialog claimed the
ingress stays blocked when Caddy is `network_mode: host`; the ssh-path
predicate existed twice and had diverged. Plus `integration.sh` carried three
stale `version=3` fixtures that the `MinScriptVersion` bump invalidated —
8 checks failed and an unguarded `curl` aborted the run, losing 5 more.

Round 2 (`dc5bad4`): `derive_ingress_bridge` was called above its definition
(exit 127 on the only path that reaches it); `sshd -T` ignored Match blocks;
`Installed()` stat'd the run dir, which docker-compose bind-mounts, so it was
still always true in production. The new ordering test then found a second
instance of the round-1 class: `die` called from the argument loop above its
own definition.

## What passed

| Row | Result |
|---|---|
| `deploy/verify-commit.sh` @ `dc5bad4` | PASS — builds and tests clean in a detached worktree |
| `go vet`, `go test ./...`, ingress module | clean |
| `deploy/firewall/test.sh` | 121 / 0 |
| Browser e2e | 89 / 0, 2 skipped |
| `integration.sh` (vs `styrman-runtime:1.23.0`) | 79 / 0, 4 skipped |
| `vulncheck.sh` | only the two accepted no-fix docker/docker findings |
| `npm audit` | 0 |
| Trail freshness (Step 3) | pass — append-only verified by numstat, 0 deletions |
| Docs freshness (Step 4) | one finding, remediated (`README.md:45` claimed projects can never reach the host) |
| Version bump (Step 6.2) | no-op — nothing in-repo tracks a version; the tag is the version |

The cold read cleared the core isolation property explicitly: **no path
inserts rules without removing stale ones**, `${tag}` is provably only
`lan|lanhost|lanpub`, and no key can collide across families.

## What was NOT checked

- **The live published-port path has never been exercised.** The
  `integration.sh` assertions exist but need `LAN_ALLOW_PUBLISHED_PORTS=1` on
  a real host; this host has not opted in. The v1.23.0 handoff recorded the
  same gap and it is still open.
- **No LAN allowance has ever been applied by the v4 script on real
  hardware.** Every firewall test is either the unshare namespace or a
  no-marker run against live chains.
- The sshd gate has been exercised only against this host's configuration
  (`passwordauthentication yes`, no Match blocks). The Match-block failure in
  finding 4 was demonstrated by the reviewer, not by this suite.
- Coverage tracing from the verifier log flags four changed files with no
  direct test: `install.sh` (structural checks only), `LocalNetwork.tsx`
  (e2e only), `BridgeNameForNetwork`'s Docker-success path, and
  `config.IngressNetwork`.
- One unexplained one-off: an e2e run exited 1 while reporting 89/0/2 and
  did not reproduce. Leftover `/tmp/styrman-e2e-*` dirs suggest the cleanup
  trap does not always complete.

## Recommendation

Fix the six open findings in a fresh session, with an independent review
afterwards, before tagging. The mechanical gate has passed at every point in
this release and caught none of the 16 findings; it confirms the tree builds,
not that it is correct.

---

## Post-gate update (2026-09-08): finding 1 removed by design change, not by a fix

The operator, reading the NO-SHIP report, made the observation that ends
finding 1: *"A project does not need to have its ssh key copied on the host.
If enough if we have full network access."*

The live-state machinery existed so the SSH key could come BACK when LAN
access was turned off. Nobody needed it back — and turning LAN access off
cannot un-expose a key the project already held. So the withholding now
**latches**: `Project.LanEverEnabled` is set once and never cleared, and
`LanExposed` reads that boolean and consults root's firewall status not at
all.

**Finding 1 is deleted rather than fixed.** So is the class that produced
three defects across three rounds — the dead `Enabled()` guard, its
`Installed()` replacement (which `docker-compose`'s bind mount also made
permanently true), and the failed-revocation hole. `netpolicy.Exposed` and
`Dir.Installed` are gone; nothing in the credential path reads the status
file any more. Recorded as an ADR-066 amendment.

**Still open: findings 2-6** (`WANT_COUNT` dedup, the three `install.sh` sshd
gate defects, and the ordering test's blind spots). The verdict stays
NO-SHIP.

Suites after the change: `go vet`/`go test ./...` clean,
`deploy/firewall/test.sh` 121/0, browser e2e 89/0/2-skipped with all five
ADR-066 checks passing including the new latch assertion, vitest 85.

---

## Version corrected to v1.24.0 (2026-09-08)

Originally opened as v2.0.0. The operator corrected it on the grounds that a
release which fixes its own unreleased work is not a major: **nothing breaks
for an existing host.** Both new conf keys default closed, so converging the
script to version 4 changes no behaviour until a human runs `install.sh` with
an opt-in flag; the `MinScriptVersion` 3→4 bump only refuses an ENABLE on a
host whose script has not converged yet, and `host_converge` converges it
from the tag; and the one user-visible behaviour change — a project that was
LAN-enabled and then disabled does not get its SSH key back — cannot be a
regression, because no released version withheld it at all.

The version lives only in the git tag: nothing in the repo tracks one.

## All findings closed (2026-09-08)

Sixteen findings were raised across three rounds. Final disposition: eleven
fixed, one DELETED by design (the latch amendment removed the question rather
than answering it), four subsumed by those changes. Commits `3e0bb9c`,
`dc5bad4`, `4b237a2`, `301d603`.

The last round's five were fixed in `301d603`: `WANT_COUNT` now dedups with
`DESIRED`; the sshd gate was rewritten rather than patched (its three defects
all came from trying to evaluate sshd's config cleverly, so the replacement
does not try — it refuses on "cannot tell"), and now runs on the effective
resulting state rather than the flag; the ordering check recognises every
bash spelling and matches command position.

**This does not by itself make the verdict SHIP.** Every fix this round was
verified by deliberately re-introducing the defect and watching the test
fail, and the suites are green — but the same was true before each of the
three rounds. The standing recommendation is unchanged: an independent review
of the cumulative result before tagging, because the author of these fixes is
the author of most of the findings.
