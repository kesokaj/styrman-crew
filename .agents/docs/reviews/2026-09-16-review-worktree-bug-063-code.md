---
owner: reviewer
date: 2026-09-16
---
# Review — worktree-bug-063-code   (2026-09-16, round 2)

Range reviewed: working tree vs 976b565 — the five changed files and bug063_test.go
(round 2 also reads the docs the fixes touched: `.agents/docs/testing/styrman/README.md`,
`.agents/docs/specs/styrman/guide.md`, `.agents/docs/bugs/BUG-063-*.md`, `INDEX.md`)

## Verdict: APPROVE WITH NITS

All three round-1 findings are fixed, correctly and at the right place. The
`ContainerUpdate(… restartPolicy(false))` is now the first thing
`ensureRunningLocked` does in `case err == nil:`, above the `!insp.State.Running`
branch, so the invariant is asserted on every attach for every existing container —
that closes the half-failed-stop window I raised, and the new test plants exactly
that state (running, pinned at `no`) and proves the restore happens with no
Start/Stop/Remove. `go test ./internal/orchestrator/ -count=1` passes here (10.1 s,
rc=0, run bare). The README now reads 106 checks / `passed=103 failed=0 skipped=3`,
which matches both `grep -c '^ *check '` on the suite (106) and the completed Tier 2
run's own tally. The bug record and `INDEX.md` both read `fixed`. What is left is one
stale sentence: the record's own `## Fix` section still describes the ROUND-1 shape of
the code it is documenting.

## Findings

- **[low / confidence: high]** `.agents/docs/bugs/BUG-063-stop-races-restart-policy.md:70-72`
  — "`ensureRunningLocked`'s start path for an existing stopped container calls
  `ContainerUpdate(… restartPolicy(false))` … BEFORE `ContainerStart`" describes the
  round-1 code. The shipped code asserts the policy for EVERY existing container,
  running or not, above the start branch — which is the part that closes BUG-030's
  half-failed-stop window, and the reason the third test exists. The record is the
  artifact a future reader consults for what the fix does; as written it would let
  someone "restore" the narrower version. Fix: say "for every existing container,
  running or not, before the start branch", and add
  `TestEnsureRunningRestoresTheRestartPolicyOnARunningContainer` to the test list in
  the same section (the Repro section names only the first two tests).
  Same sentence, smaller: `specs/styrman/guide.md:363-366` says "Start switches it
  back", which is now "any attach switches it back" — accurate but narrow; one clause
  fixes it.
- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:347-358` — the
  update is unconditional, so every EnsureRunning issues a daemon config write even when
  the policy already reads `unless-stopped`. All four callers are user-initiated
  (`wsBoot` on a terminal connect, the background create boot, the start endpoint, the
  network toggle) and none polls, so the rate is bounded by user actions; a flapping
  terminal reconnect loop on a running project is the one shape that would repeat it.
  No failure mode demonstrated — recorded so the choice is on the record, not as a
  change request. If you want it cheap: skip the update when
  `insp.HostConfig.RestartPolicy.Name` already equals the wanted policy (the fake
  already reports it, so the existing tests would still cover both arms).
- **[low / confidence: high]** `.agents/docs/testing/2026-09-16-verify-bug-063.md` —
  the verifier's record is still `## Status: IN PROGRESS` with `(pending)` for Commands,
  Coverage and Environment. Not a code defect; naming it so it is not committed in that
  state at Done (step 1 of the Definition of Done owns it).

## Round-1 findings — disposition

1. **Policy never restored on a still-running container (medium)** — **FIXED.** Hoisted
   out of the `!Running` branch; comment rewritten and now states why (BUG-030's
   scenario), that it is idempotent, that it does not touch the guest (the 2026-09-16
   Kata probe), and that "every attach passes through here, which is what makes this the
   invariant's home". The Warn text says "until its next attach", matching the new reach.
   `TestEnsureRunningRestoresTheRestartPolicyOnARunningContainer` plants a running
   container at `"no"`, asserts it comes back `unless-stopped`, and asserts
   `ContainerStart`/`ContainerStop`/`ContainerRemove` are never called — it fails if the
   hoist is reverted (the update would sit behind a false `!Running`) and it cannot pass
   vacuously, since the fake only flips `restart` inside `ContainerUpdate`.
   `TestEnsureRunningLeavesARunningContainerAlone` (seam_test.go:113) still passes: it
   forbids Stop/Remove/Start, not Update, so it is not weakened by the change.
2. **Stale check count (medium)** — **FIXED.** README:15 reads 106 checks /
   `passed=103 failed=0 skipped=3` with a BUG-063 clause naming both new checks and
   their assertions. Verified independently: 106 `check` lines in `integration.sh`
   (104 at 976b565) and `passed=103 failed=0 skipped=3 aborted=0` in the completed run
   at `.agents/scratch/bug-063/integration-run1.txt`. The older "for 104 total" clause
   inside the same paragraph is the 2026-09-16/BUG-061 history, correctly left alone.
3. **Bug record open (low)** — **FIXED**, with the residue in finding 1:
   `status: fixed`, INDEX row `fixed`, Repro section names the two pre-fix failures with
   their actual messages.
4. **NotFound short-circuit in `stopLocked` (low, optional)** — unchanged, as expected;
   still consistent with the pre-existing `ContainerStop` NotFound handling.

## Criteria

1. **Dead code — pass.** Nothing orphaned; `callIndex` has callers, the fake's new fields
   are read by three tests and by `ContainerInspect`.
2. **Flow tracing — pass.** The invariant now has one home (`ensureRunningLocked`,
   `case err == nil:`) and one inverse (`stopLocked`). The three recreate paths above it
   (pre-ADR-027 bind, image upgrade, network-mode mismatch) all set
   `err = errdefs.NotFound(...)` before the switch, so a container about to be rebuilt is
   never updated — the create path sets the policy from `spec.noRestart` instead. The
   other two `ContainerStart` sites were re-checked: `:1095` is `create`, `:743` is the
   setup VM (always `restartPolicy(true)`, untouched). Both `stopLocked` callers
   (`stopMarked`, `stopIfIdle`) get the policy-off, and the failure path they share now
   self-heals on the next attach. Errors are Warn-and-continue by design, with the
   consequence named in the message.
3. **Standards conformance — pass with finding 1.** Go idiom, seam-level call, comments
   say why. Describing text: `restartPolicy`'s doc-comment, `dockerAPI`'s provenance
   note, the guide's two paragraphs and the testing README were all updated in the same
   change; the bug record's Fix section is the one restatement left behind.
4. **Directory layout — pass.** Tests beside the code, non-colliding filename; docs under
   the paths that own them.
5. **Simplicity (DIY-except-security) — pass.** No new dependency; one seam method, two
   call sites. No auth/crypto/session/token surface is touched.
6. **Decoupling — pass.** `dockerAPI` grew by exactly the method the caller needs; the
   fake implements it in the same file; no cross-package reach-in.

## Not examined

- Tier 2 was NOT re-run after the round-2 hoist. The run I read
  (`.agents/scratch/bug-063/integration-run1.txt`, `passed=103 failed=0 skipped=3
  aborted=0`, both BUG-063 checks PASS) was produced by the round-1 code. The hoist
  changes the start path, so a fresh run is what proves the Tier 2 "start switched the
  restart policy back on" check still holds against the shipped ordering — logically it
  must (the update moved earlier, not away), but it is unverified.
- The live Docker behaviour of `ContainerUpdate` on a RUNNING Kata container is taken
  from the brief's 2026-09-16 probe; the round-2 change makes that path routine rather
  than exceptional, and I did not re-probe it.
- `go vet`, the web trio and the browser e2e were not run; only
  `go test ./internal/orchestrator/ -count=1` (bare, rc=0).
- Files outside the range (`web/`, the ingress module, the rest of `internal/`) were not
  reviewed.

## Outcome

- Round 1, finding 1 (medium) — fixed: the `ContainerUpdate(unless-stopped)` is hoisted above the `!Running` branch of `ensureRunningLocked`'s `case err == nil:`, so every attach asserts the policy on any existing container; `TestEnsureRunningRestoresTheRestartPolicyOnARunningContainer` covers the running-and-pinned case (bug063_test.go). Round 2 verified it.
- Round 1, finding 2 (medium) — fixed by the documentation agent: testing README line 15 reads 106 checks, `passed=103 failed=0 skipped=3`, with the BUG-063 clause.
- Round 1, finding 3 (low) — fixed by the bugs agent: BUG-063 `status: fixed`, INDEX row fixed.
- Round 2, nit 1 (low) — fixed: the record's Fix section now says "every existing container, running or not" and names the third test; guide.md's Stop bullet says "the next Start or terminal attach switches it back".
- Round 2, nit 2 (low) — fixed: the update is skipped when the inspect already shows the policy on (`insp.HostConfig.RestartPolicy.Name`), so an attach to a healthy project writes nothing; `go test ./internal/orchestrator/ -count=1` ok after the change.
- Round 2, nit 3 (low) — not a finding on the change: the verifier's log was still being written when the reviewer read it; its Tier 2 run ended `passed=103 failed=0 skipped=3`.
- Tier 2 on the final tree (hoist + skip): run bare by the main agent after this record (`.agents/scratch/bug-063/integration-run2.txt`); the release gate's verifier is the record for the committed tree. Round 2 closed 2026-09-16.
