---
owner: reviewer
date: 2026-09-16
---
# Review — plan-harness-channel   (2026-09-16)

Range reviewed: `.agents/docs/plans/2026-09-16-harness-channel.md` (cold read, round 3)

## Verdict: REQUEST CHANGES

Every round-2 item is genuinely closed, and I verified the important one against the file rather
than the claim: the branch now sits after `deploy/update/styrman-update:262-272` (the `STATE_DIR`
creation and ownership assertions), after `:273` (the lock), after `:276` (`cd`), after `:288-297`
(`trusted_dir "${DEPLOY_DIR}"` and the two `.git` lines), and before `:318` (the crew token block) —
which is exactly right, and it also skips the `deploy/.env` assertion at `:300+` that the harness
mode has no use for. What remains is one surface the plan has never described: the contents of
`deploy/update/styrman-harness-update.service`. D19's newly shared lock makes that omission bite. The
plan says only "`DOCKER_CONFIG` as the crew unit sets it"; an implementer who takes that literally
copies `RuntimeDirectory=styrman-update`, and because systemd removes a `RuntimeDirectory` when
*either* unit that declares it stops, the harness tick that finds the crew lock held will exit
immediately and delete `/run/styrman-update` out from under a crew deploy that is mid-`docker build`.
The shared lock guarantees the two never build concurrently; it does not stop the loser from tearing
down the winner's docker config. The same unit file also has no `TimeoutStartSec`, where the crew
unit carries 45 minutes precisely because these builds take minutes. One blocking question, on one
file; everything else below is editorial drift or a low-severity carry.

## Findings

### (1) Gaps — decisions the plan does not make

- **[high / confidence: medium]** `plan` Dispatch + Invocation rows vs
  `deploy/update/styrman-update.service` (`RuntimeDirectory=styrman-update`,
  `Environment=DOCKER_CONFIG=%t/styrman-update/docker`) and `deploy/update/styrman-update:228-244` —
  **the new unit must not share the crew unit's `RuntimeDirectory`, and the plan's only instruction
  points the other way.** systemd removes a unit's `RuntimeDirectory` on stop unless
  `RuntimeDirectoryPreserve=` says otherwise. Two units declaring `styrman-update` therefore share a
  lifetime: with D19's shared lock the *normal* case is that one of them starts, finds
  `flock -n` held, logs "another run is in progress — skipping this tick" and exits within a second
  — and that exit deletes `/run/styrman-update`, including the `docker/` leaf the still-running
  unit exported as `DOCKER_CONFIG` and is using for a build that may run for many minutes. The lock
  prevents concurrent builds; it does not prevent this. Specify one of: `RuntimeDirectory=styrman-harness-update`
  with `DOCKER_CONFIG=%t/styrman-harness-update/docker`, or no `DOCKER_CONFIG` at all — the script's
  own `*)` branch at `:240-244` then mktemps a private dir and removes it on an EXIT trap, which is
  the documented hand-run path and is sufficient here. (Confidence medium only on how loudly the
  docker build fails when its config dir vanishes; the sharing itself is certain.)
- **[medium / confidence: high]** `plan` Invocation row vs `deploy/update/styrman-update.service`
  (`TimeoutStartSec=45min`) — **the harness unit's start timeout is unspecified.** The row gives the
  timer schedule and the `Environment=` list and nothing else about the unit. A `Type=oneshot`
  service without `TimeoutStartSec` inherits `DefaultTimeoutStartSec` (90 s on Debian), and a
  harness run does a `git fetch`, a `docker run` Go build and a smoke `docker run` inside it — the
  crew unit's own comment explains the 45-minute value as "a build can legitimately take many
  minutes; the flock in the script is what prevents overlap". With the lock now shared, the harness
  run can also be *waiting behind* nothing — it skips rather than blocks — so the timeout only has
  to cover its own work, but 90 s does not. Name the value, and while there name `After=`/`Requires=docker.service`
  and whether the crew unit's `NoNewPrivileges`/`ProtectHome`/`ProtectKernelTunables` hardening is
  carried across.
- **[low / confidence: high]** `plan` Risks (residual) vs the styrman-ctl row — **the recorded
  rollback residual claims a visibility the described `status` line does not provide.** The residual
  says that after a crew rollback to a pre-channel tag the harness timer keeps invoking a binary that
  rejects the verb, "a failed unit each tick, visible in `styrman-ctl status`". The `status` line
  this plan specifies has exactly three states — `<version> (<sha7>)`, `waiting for the first signed
  styrman-harness tag`, and `REFUSED <tag>: <reason>` — none of which reads a unit's failed state. On
  a rolled-back host the state files are untouched, so `status` would keep printing the last good
  harness and look healthy. Either add the unit-state read to the `status` line or restate the
  residual as "visible in `systemctl status` / the journal". Related and low: the skip path reuses
  the crew updater's message, so a skipped harness tick and a skipped crew tick are indistinguishable
  in the journal — worth a mode prefix in that one log line.
- **[low / confidence: high]** `plan` Surfaces touched and Verification vs Phase 1 and D16 —
  **the superseded shell-test design is still named in four places.** Surfaces lists
  `deploy/update/harness-channel-test.sh` (new); the Verification "Tiers" line lists
  "shell: `deploy/update/harness-channel-test.sh` against scratch repos, keys and dirs, unelevated";
  Phase 1 box 2's verify clause says "the test script green"; the Phase 1 Checkpoint expects "test
  script green". D16 replaced all of it with `internal/config/styrman_harness_test.go` plus
  `.agents/docs/testing/styrman/make-scratch-harness.sh`. Carried unaddressed from round 2, where
  only box 1 was rewritten. Phase 3's box also still carries the duplicated fragment "new checks in
  the harness section: new checks in the harness section:".
- **[low / confidence: high]** `plan` Surfaces touched vs the Server row — **the two rows still
  contradict each other** (carried from round 2). Surfaces says "the image-upgrade recreate at
  280-311 stays image-only"; the Server row adds a third recreate condition in that exact range
  (D18/D20). Surfaces also still omits `internal/config/styrman_harness_test.go`,
  `make-scratch-harness.sh`, and `internal/config/styrman_update_test.go` — the last is no longer
  incidental, because the Dispatch row now leans on it by name ("`TestStyrmanUpdateVerifyTagMarkers`
  still covers it") as the evidence that the `git()` wrapper change is safe.
- **[low / confidence: high]** `plan` Phase 1 box 1 — **a skipped test exits 0** (carried from round
  2). The box specifies `t.Skip` when `git`, `ssh-keygen`, `bash` or `docker` is missing or
  `STYRMAN_HARNESS_SMOKE_IMAGE` names no local image, then expects "PASS for every case".
  `go test -run StyrmanHarness` exits 0 when every case skips, so the stated expectation cannot
  distinguish a green channel from one that never ran — the `.agents/rules/verify-bare.md` / BUG-034
  family. `-v` is already in the command; make the expectation name `--- PASS` per case and no
  `--- SKIP`.
- **[low / confidence: medium]** `plan` Phase 4 — **the post-deploy check still does not prove the
  timer is enabled** (carried from round 2). "The harness timer's first tick logs the waiting line"
  is good evidence the unit ran, but a hand `systemctl start` produces the same line, and BUG-035's
  recurrence was specifically `is-enabled` reading `disabled` after a release that looked fine.
  `.agents/rules/mechanism-reaches-every-caller.md` names the command. Add
  `systemctl is-enabled styrman-harness-update.timer`, and note that the first tick after v1.35.0
  runs the OLD updater binary, so `host_converge`'s enable lands on the second tick.

### (2) Scope — built but not named in `## Why`

Clean, third round running. D1-D21 attribute every surface; `## Out` and the Follow-ups are explicit;
the three new items this round (the shared lock, the optional self-install entries, the
`--harness-token-file` option) are each a D-row answering a review finding rather than new ambition,
and D19 is honestly marked as superseding its own earlier wording. Nothing in the plan builds an
endpoint, a second shape for one operation, or configuration for a value that never varies. The one
genuine second delivery shape — the image mirror as fallback — remains D2 and ADR-073 D6 with a named
removal follow-up.

### (3) Readiness — questions a first-pass implementer must ask

**BLOCKING** (two careful implementers would build different things):

1. What is in `deploy/update/styrman-harness-update.service`? Specifically: does it declare its own
   `RuntimeDirectory` (it must — sharing `styrman-update` lets the unit that skips on the shared lock
   delete the runtime dir of the unit that holds it), and what is its `TimeoutStartSec` (the default
   90 s cannot cover the export's Go build plus the smoke)? An implementer reading
   "`DOCKER_CONFIG` as the crew unit sets it" will copy the crew unit verbatim, which is the variant
   with the defect.

**NON-BLOCKING** (the plan states a default, or a reasonable implementer decides alone): whether the
skip log line gains a mode prefix; whether `styrman-ctl status` reads the harness unit's failed state
or the residual is reworded; which of the four stale references to `harness-channel-test.sh` are
deleted; whether `Surfaces touched` is reconciled with the Server row; how the Phase 1 expectation
distinguishes PASS from SKIP; whether Phase 4 adds `is-enabled`; how many `<commit>` directories to
retain; whether `repo/VENDORED.md` is written into the export.

## Criteria

1. **Dead code** — pass for code; the plan creates nothing orphaned, and the mirror's survival is a
   decision with a named removal follow-up. The four surviving references to the deleted
   `harness-channel-test.sh` are stale text, listed above.
2. **Flow tracing** — pass. The dispatch point is now correct and I verified each hop against the
   file: `:262-272` state dir created and asserted, `:273` lock, `:276` `cd`, `:288-297` clone trust,
   branch, `:318` crew token never reached. The `git()` wrapper's second caller is named and its
   covering test cited. The create→start→recreate path is traced with the absent/untrusted split
   (D20) and the correct Docker bind-source reason. `self_install`'s fail-closed loop is addressed by
   making the harness pair optional (D21), which also answers the rollback case. The only untraced
   hop is systemd's own: unit → `RuntimeDirectory` lifetime, finding 1.
3. **Standards conformance** — pass. R5 and ADR-073 landed before Phase 1 as `REQUIREMENTS.md`
   immutability requires (`REQUIREMENTS.md:53-57`, `DECISIONS.md:2723`), so Phase 4's ticked box is
   truthful. `.agents/rules/tests-describe-behaviour.md` is satisfied — both stamp checks
   (`integration.sh:157` and the BUG-062 check at `:418`) are named and re-pointed at `HARNESS_ID`.
   `.agents/rules/mechanism-reaches-every-caller.md` is satisfied for `self_install`, `host_converge`
   and the `__verify-tag` hook; its verification clause (`is-enabled`) is the low carry above.
4. **Directory layout** — pass. Units beside the crew updater, the Go test on the existing
   `styrman_update_test.go` precedent, the Tier 2 helper with the other Tier 2 scripts, content under
   `/var/lib/styrman/harness`, run-state under `/etc/styrman/update`.
5. **Simplicity (DIY-except-security)** — pass. No new dependency; no hand-rolled crypto — signature
   verification remains `git verify-tag` against the existing root-owned allowed-signers file, and
   D17's "no unsigned mode" is the right asymmetry for something that mounts an executable into every
   project VM. D19's reversal to a shared lock is a simplification that removes a second lock file
   and a second failure mode; finding 1 is the one consequence it did not follow through.
6. **Decoupling** — pass. The host/server seam is still the narrow filesystem contract
   (`current/repo/{COMMIT,TAG,SHA256SUMS}`, `current/bin/sh-agent`); D13's fallback keeps Phase 2
   shippable before Phase 1 exists on a host; D21's optional self-install entries mean a rollback
   across the seam degrades rather than breaks.

## Not examined

- `deploy/update/styrman-update.service` was read in full in round 1 and re-read for finding 1; the
  timer unit (`styrman-update.timer`) was never opened, so the harness timer's claimed 7/5 offset is
  checked only against the recon brief's report of the crew timer's 5/5.
- `deploy/styrman-ctl` `:520-570` (the signers prompt and origin derivation) — still not read, so
  where `--harness-token-file` and the third prompt slot in is inferred from `:492-516` and
  `:570-600`.
- `deploy/host-setup.sh` beyond `:178-186` and `:375-376`; its step 8c, referenced by the updater's
  own failure message at `:248`, was not read.
- `deploy/verify-commit.sh` — not opened in any round. The plan asserts it is unchanged because the
  mirror stays; I have not confirmed its `find image/harness/skills` count check and its
  "no `/styrman/harness/.git`" assertion still hold once a project's `/styrman/harness` can be a bind
  rather than the image's copy.
- `internal/config/config.go` and `internal/config/styrman_update_test.go` — neither opened;
  `HarnessDir`'s shape and the test's coverage of the `git()` wrapper are taken from the recon brief
  and from the script's comment at `:183-186`.
- `internal/orchestrator/{seam,fakedocker}_test.go` — not opened, so whether the fake Docker seam can
  express the bind assertions Phase 2 and D18/D20 need is unverified.
- `.agents/docs/specs/styrman/{versioning,deploy,runtime-image,guide}.md`, `.agents/docs/deploy/remote.md`,
  `README.md`, `deploy/README.md` — Phase 4's docs clauses were checked for presence, not against the
  sections' contents.
- `.agents/docs/bugs/INDEX.md` — not read in any round; BUG references here come from the
  restatements in `.agents/rules/`.
- systemd's `RuntimeDirectory` removal semantics in finding 1 are asserted from the documented
  default behaviour, not reproduced on this host.

---

**design needs revision** (7 findings above; 4 carried unaddressed from round 2, all low).
**implementation not ready** (1 BLOCKING question in section 3, scoped to one unit file — down from
6 in round 1 and 1 in round 2).
