---
owner: main
date: 2026-09-08
---

# Preflight — v1.23.0 (per-project local-network access, ADR-065)

Range reviewed: v1.22.0..bdc15b7 (the release commit that carries this
report, the two release reviews, the release changelog entry and the
gate-driven fixes sits on top of bdc15b7; `deploy/verify-commit.sh` is run
again on that commit before the tag is signed).

**VERDICT: SHIP**

No blocking finding. Eight findings across the two whole-range reviews (0
high, 2 medium, 6 low): five fixed in the release commit, two rebutted in
writing, one accepted as pre-existing. The rebuttals and the acceptance are
recorded below with their reasoning, not merely asserted.

## Findings and disposition

Ordered by severity. Two independent agents reviewed the same range; where
both raised the same thing it is marked.

- **[medium / confidence: high] FIXED — the LAN allowance is masqueraded and
  that was never disclosed.** (security) `REQUIREMENTS.md:82`, ADR-065 (5),
  `web/src/components/LocalNetwork.tsx`. Traffic from a LAN-enabled project
  reaches the LAN as *this host's own address*, so any device whose access
  list trusts the styrman host by IP — a NAS export, a printer, a local model
  server — silently extends that trust to an AI agent inside the project.
  True of the NAT topology, not a choice the allow rule makes, and absent
  from every operator-facing surface. Now stated in the three places an
  operator actually reads before acting: the enable confirm dialog, the
  guide's "Local network" bullet, and `deploy.md`'s exception section.
  **Not** added to `REQUIREMENTS.md` R12: that file is immutable without the
  operator's explicit approval, and R12 was already amended once today. The
  exact sentence is offered for approval in "Deferred to the operator" below.
- **[medium / confidence: high] FIXED — the testing README contradicted
  itself on the Tier 4 count.** (reviewer)
  `.agents/docs/testing/styrman/README.md:240` said 86 checks (84 + 2
  auto-SKIP) while `:328` still carried the pre-release 82 (80 + 2). Both
  were wrong. Measured rather than argued: `web/e2e/run.mjs` holds 85
  `await check(` call sites; a green run on this firewall-hardened host
  prints `passed=84 failed=0 skipped=2`, which is 83 checks plus the final
  client-error gate, with 2 auto-SKIP. The README counts that gate
  separately, so 86 double-counted it. Both locations now read 85 checks +
  the gate, 83 + 2 auto-SKIP, with the arithmetic spelled out so the next
  reader can check it against a run. Same defect class `2ef1e51` already
  fixed once inside this range.
- **[low / confidence: medium] FIXED — root created a directory inside the
  server uid's own tree.** (security) `deploy/firewall/install.sh` created
  and chowned `<StateDir>/netpolicy/lan` as root. `chown`/`chmod`
  dereference and the loop did not re-validate the parent between
  iterations, and the server's uid owns `StateDir`, so it could swap a
  component between root's check and root's write. BUG-009 again, narrowed
  rather than closed. Fixed by removing root from the path entirely: the
  server already creates that directory itself at boot, as its own uid, with
  the right mode by construction, and the reconciler already treats an absent
  marker dir as "no allowances" and says so in its status. Nothing is lost.
- **[low / confidence: medium] FIXED — the one-time migration deleted before
  the replacement existed.** (security) `deploy/firewall/styrman-firewall.sh`
  removed the pre-chain rules of earlier versions *before* installing the
  `STYRMAN-IN`/`STYRMAN-FWD` chains and jumps, contradicting the file's own
  never-delete-then-insert promise and opening one window per host on a first
  run from a pre-chain version. Moved after the chains are in place. This
  host has no pre-chain rules left, so the practical exposure here was nil;
  the file's claim is now true for a fresh host as well.
- **[low / confidence: high] FIXED — dead accessor.** (reviewer)
  `internal/netpolicy.Dir.MarkerDir()` had zero callers including tests.
  Removed.
- **[low / confidence: high/medium] REBUTTED — `STYRMAN_FIREWALL_DIR`
  defaults to the production path.** (raised independently by BOTH agents,
  `internal/config/config.go`.) The argument: the root script and the
  updater unit in this same range take every path from their unit with no
  in-code default (ADR-052), so the server should too, and a host-process
  dev run on this machine otherwise reads root's status file and can drop a
  trigger in its spool. Declined, for two reasons. First, ADR-052's rule is
  about *root-run* programs, where a bare invocation must never alias
  production; the server is not one, and every other path in the same struct
  carries a production default (`StateDir` → `/var/lib/styrman`,
  `BrokerAddr` → `:8091`), so singling this field out would be inconsistent
  inside its own file. Second, and decisively, removing the default inverts
  a failure mode: an operator whose compose env lost the key would tear down
  every live allowance at the next boot, because an empty value now means
  "disabled, remove every marker" (the fix for BUG-014). That is a silent
  availability loss for a feature meant to be off by default, not off by
  accident. The residual cost is real but small and bounded: a dev run reads
  a status file and can cause one idempotent root reconcile; it cannot create
  a marker, because markers are addressed from `StateDir`, not this field.
  Mitigated by `.agents/docs/deploy/local.md`, which now tells a developer to
  point it at a scratch dir, and the trade-off is recorded in the field's own
  doc comment so the next reader sees the reasoning rather than the choice.
  Two agents raising it independently is why the rebuttal is written at this
  length instead of a sentence.
- **[low / confidence: medium] REBUTTED — `host_converge` skips silently
  when HEAD is not the deployed SHA.** (security) A hand `git checkout` in
  `/opt/styrman` pins HEAD away from the recorded SHA and the host then keeps
  the old root firewall script, with only a per-tick journal warning. The
  concern is right in principle and it is the drift class ADR-065 exists to
  close. It is already escalated where the operator looks, though, by the
  mechanism built for exactly this: the script stamps its version into the
  status file, the server refuses an enable below `MinScriptVersion` with a
  503 naming both numbers, and Settings → Host firewall shows the row in red.
  Any release that changes the script and bumps its version therefore turns a
  silent skip into a visible, blocking one. The narrow uncovered case is a
  material script change *without* a version bump, which is a discipline
  rule, not a code path; recorded as a follow-up rather than papered over
  with more logging. The gate itself was confirmed correct and fail-closed by
  the same review (signature required, `trusted_dir` on both targets,
  basename pin, 40-hex recorded SHA, reads from the verified git object, and
  `--check` returning before any state change — BUG-008 properly closed).
- **[low / confidence: high] ACCEPTED — a NUL byte in generated build
  output.** (security) `web/dist/client/_shell.html` carries a raw NUL at
  offset 7417 and git classifies it as binary. It is generated by the SPA
  build, predates this range, and the reviewer decoded both revisions to
  confirm only asset hashes changed. Not BUG-007, which was about hand-written
  source. Left alone in a release commit rather than patched under a gate;
  recorded as a follow-up.

## Gate rows

| Row | Result |
|---|---|
| Reviewer, whole range, strongest model | ran, 0 high / 1 medium / 2 low, all dispositioned |
| Security, whole range | ran, 0 high / 1 medium / 5 low, all dispositioned |
| Verification | today's `testing/2026-09-08-verify-lan-access.md`, PASS, 0 FAIL rows; the only delta from its tree to bdc15b7 was four docs-only files |
| Re-verification after the gate fixes | `go build`/`vet`/gofmt/`test -race ./...` ok; netns firewall test 83/83; `npm check`/`lint`/`test` 80/80; dist rebuilt; shellcheck and `bash -n` clean |
| Trail: newest `changes/` describes this work | pass, five entries dated today |
| Trail: no future-dated artifact | pass |
| Trail: `changes/` and DECISIONS append-only | pass, DECISIONS.md is 26 insertions and 0 deletions; no `changes/` entry modified |
| Trail: ROADMAP and TODO touched since base | pass |
| Docs freshness | pass, README, four specs and two deploy guides updated in range, plus a `documentation` agent pass today |
| Secret-shaped paths in range | none |
| Dependencies | unchanged; `govulncheck` is exactly the two accepted `docker/docker` advisories with no upstream fix; `npm audit` 0 |
| R12 unconditional clauses under the exception | hold — verified against this host's live chains by the security agent |

Prior coverage cited instead of re-reviewed: `2026-09-04-review-guest-seam.md`
and `2026-09-04-security-guest-seam.md` cover `dc9f6ae`, the `internal/httpapi`
exec seam that rides along in this range; the whole-range reviewer separately
confirmed it coexists coherently with the Phase 2 changes to the same files.

## Deferred to the operator

`REQUIREMENTS.md` is immutable without explicit approval, so the disclosure
sentence was put everywhere except R12. Proposed addition to R12's
local-network paragraph, for approval at your convenience — it changes no
behaviour and no code:

> Traffic allowed by this exception is masqueraded like any other bridge
> egress, so it reaches the local network as the HOST's address: a device
> whose access list trusts this machine by IP admits every LAN-enabled
> project, and the network cannot tell them apart.

## What was not checked

- **The live host half has never run.** Phase 5 is open: this machine still
  carries the pre-ADR-065 script (four DROPs, no `styrman-lan:` rule, no
  status file). Every claim about the composed chain is from reading the
  script plus read-only `iptables -S` on the current rules. End-to-end LAN
  reachability from a real microVM is unproven until the operator installs
  the host half and the applied-state check runs.
- `install.sh` and `host_converge` were linted, dry-run and read, never
  executed on a real host by an agent.
- Neither release review ran `integration.sh` or the browser e2e; both were
  run by the author earlier today (78/0/4 and 84/0/2) and are cited as such,
  not re-measured under the gate.
- No exploit was built for the install-race or migration-window findings;
  both are reasoned from code and verified directory ownership.
- IPv6 was read, not tested. `STYRMAN-FWD6` drops only `fc00::/7` and
  `fe80::/10`; global unicast is not dropped, mitigated by `disable_ipv6=1`
  per bridge. Carried verbatim from before this range, on the record here
  because a future IPv6-enabled bridge would matter.
- Docker live-restore preserving `DOCKER-USER`/`STYRMAN-*` across a daemon
  restart remains unverified; bounded exposure is ≤65 s LAN-closed, never
  LAN-open.
- `web/dist/` bundles were not audited beyond `_shell.html`.
