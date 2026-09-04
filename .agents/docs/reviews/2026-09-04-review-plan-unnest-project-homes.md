---
owner: reviewer
date: 2026-09-04
---
# Review — plan: un-nest project homes from the setup VM's mount   (2026-09-04)

Range reviewed: `.agents/docs/plans/2026-09-04-unnest-project-homes.md` (plan document, cold-reader check — no diff exists yet)

## Verdict: REQUEST CHANGES

The plan's design is sound (`TenantDir`/`UserHome`-as-`base` cleanly separates the two meanings the current single `UserHome` function overloads), but it contains a direct self-contradiction over `internal/httpapi/server.go:726-727` that would misdirect a first-pass implementer into a real bug, and it omits a second `UserHome` caller (`cmd/styrman/main.go:182`, inside `migrateLegacy`) whose ordering relative to the new `migrateBaseHomes` step is unresolved and untested. A third item — the plan's own "audited, two callers" claim in Risks — is incomplete. None of the three are exotic; all three are visible from a straight `grep -rn UserHome( internal cmd` plus reading the four cited functions.

## Findings

- **[high / confidence: high]** `.agents/docs/plans/2026-09-04-unnest-project-homes.md:23` vs `:59-61` — the plan's own "Surfaces touched" list puts `internal/httpapi/server.go:726-727` in the "no code change, the path moves with the function" group, but the "Interfaces" section two paragraphs later specifies exactly the opposite: `createProject` must change both `os.MkdirAll` and `files.MkdirAllPerm` calls from `UserHome` to `TenantDir`. Verified against the code (`server.go:721-727`): this pre-create is load-bearing — the adjacent comment says it exists "so the Files 'Home' scope works before the first boot" — and `ProjectHome` is `HomesDir/<user>/p/<id>` (`config.go:355-357`), i.e. `TenantDir`, not `UserHome`/`base`. If an implementer follows the "no code change" bullet, every new project's pre-boot home lands at `HomesDir/<user>/base/p/<id>` instead of `HomesDir/<user>/p/<id>`: the Files pane is empty/wrong until first boot, a stray directory is left permanently under `base/p/<id>`, and the credential-revoke sweep's glob (`internal/httpapi/credentials.go:262`, `HomesDir/<user>/p/*`) never reaches it. Fix: delete the `server.go:726-727` item from the "no code change" list at line 23; the Interfaces section (lines 59-61) is the correct instruction.

- **[medium / confidence: high]** `cmd/styrman/main.go:177-190` (`migrateLegacy`) is not in the plan's "Surfaces touched" list at all, but it calls `cfg.UserHome(user)` at line 182 as the rename TARGET for the pre-multitenancy shared home (`StateDir/home` → `UserHome(user)`). Once `UserHome` means `HomesDir/<user>/base`, this `os.Rename` needs `HomesDir/<user>` to already exist — nothing creates it before this point: `main.go`'s startup loop (lines 37-42) only `MkdirAll`s `StateDir` and `HomesDir`, and the plan places `migrateBaseHomes` **after** `migrateLegacy` (Interfaces section, line 39: "called right after `migrateLegacy`"). `os.Rename` fails with ENOENT when the destination's parent directory doesn't exist, so for any environment where `StateDir/home` still exists (a very old, never-yet-fully-migrated install, or a test fixture recreating that shape), this step now fails where it previously succeeded. `migrateLegacy` has zero test coverage today (confirmed: no `TestMigrateLegacy` anywhere), and the plan's own upgrade fixture (Verification section) only exercises the already-multitenant old layout (`homes/<me>/...`), not this deeper pre-multitenancy case — so nothing in the plan's test plan would catch this. Likely dead on the current production host (its `StateDir/home` was probably renamed away years ago), but the plan doesn't say so, doesn't decide it's out of scope, and doesn't fix the ordering.

- **[low / confidence: medium]** Plan Risks section, line 98: "audited by `grep -rn 'HomesDir' internal cmd` in Phase 1 (the janitor and `files.Trust` are the known two)" undercounts what that grep actually returns — `internal/httpapi/credentials.go:262` also hand-builds `filepath.Join(s.cfg.HomesDir, user, "p", "*")`. It happens to need no change (the `p/` location is untouched by this plan), but the plan's own audit claim is wrong, which risks an implementer trusting the stated count instead of rerunning the grep.

## Verified against code (no discrepancies beyond the above)

- Every other `UserHome(` caller checked against the plan's split: `explorer.go:443`, `setup.go:34/127/214`, `credentials.go:263`, `git.go:73` — all correctly "means base home, no change." `orchestrator.go:226/272/275/282` (`EnsureRunning`/create path) — correctly "means the tenant dir, becomes `TenantDir`." `orchestrator.go:594/602` (`EnsureSetup`, non-stale-bind path) — correctly stays `UserHome`/base.
- `insp.Mounts` is `[]container.MountPoint` (docker/docker v28, aliased from the old `types.MountPoint`) with `.Destination`/`.Source` string fields — the plan's stale-bind check design is implementable as described.
- Guest uid: `config.go:165` sets `c.UID = os.Getuid()` as the default, later overridden at `:227-231` only if `c.HomeDir` (the legacy single home) still exists and is stat-able. The plan's citation is accurate for the steady state on a host where that legacy path is already gone.
- `files.Trust(cfg.HomesDir, …)` (`server.go:126`) confines by tenant (`HomesDir/<user>`) via `filepath.Rel` + first-path-component cut (`files.go:88-110`); `base/` and `p/` both stay under that same tenant root, so the plan's "unchanged" claim for `Trust` holds.
- `migrateLegacy` (main.go:44) runs before `userstore.New` (46) and `orchestrator.New` (56); `migrateWorkspaces` runs after `orchestrator.New` (66); `Adopt` runs in a background goroutine after that (81) — the plan's ordering claim ("before the orchestrator adopts state") is accurate for where it places the new call, independent of the bug above.
- The janitor (`main.go:359-383`, `pruneTranscripts`) already walks `HomesDir/<user>` via `os.OpenRoot` and lists `p/` through `fs.ReadDir` on that confined root — unaffected by `base/` being added alongside `p/`, consistent with the plan's "unchanged" claim.

## Criteria

1. Dead code — n/a (plan review, no diff).
2. Flow tracing — fail: the `server.go:726-727` contradiction and the `migrateLegacy` ordering gap are both real breaks in the call chain from config → migration → orchestrator that the plan does not resolve.
3. Standards conformance — n/a to a plan document; the described interfaces (small `TenantDir`/`UserHome` split, `os.Root`-confined creation) match the project's existing symlink-defense idiom (BUG-002/003/004, `guest-writable-paths.md`).
4. Directory layout — pass: no new files proposed outside existing conventions (`ADR-064` in `DECISIONS.md`, migration next to `migrateLegacy` in `main.go`).
5. Simplicity (DIY-except-security) — pass: no new dependency; the stdlib `os.Rename`/`os.MkdirAll` approach matches the existing migration style. No auth/crypto surface touched.
6. Decoupling — pass: `TenantDir`/`UserHome`/`ProjectHome` as three small accessor functions is the right seam size; the gap is completeness of callers audited, not the shape of the interface.

## Counts

- Gaps requiring a decision before implementation: 3.
- Questions a first-pass implementer would have to ask: 3 — (1) which instruction wins for `server.go:726-727`, the "no change" bullet or the `TenantDir` rewrite in Interfaces; (2) what `migrateLegacy` should do given the new `UserHome`, and whether `migrateBaseHomes` needs to run *before* it instead; (3) whether the Risks section's "two known callers" claim should be corrected to include `credentials.go:262` (even though it needs no code change) so the audit statement is accurate.

design needs revision
implementation not ready

## Round 2

Re-check of round 1's three findings against the revised plan, then a fresh read.

### Round 1 findings — status

1. `server.go:726-727` "no code change" vs. Interfaces contradiction — **resolved**. Line 23 now reads "CHANGES: both calls mean the parent of `p/` and become `TenantDir`", matching the Interfaces section and the actual code at `server.go:721-727`.
2. `migrateLegacy` (`main.go:182`) ordering/parent-dir gap — **resolved**. Line 21 and the Interfaces section (lines 48-52) now specify the added `os.MkdirAll(cfg.TenantDir(user), 0o700)` before the rename, plus a new test; order stays `migrateLegacy` → `migrateBaseHomes`. Verified against `main.go:177-190`: the fix is correctly targeted (the rename target is `cfg.UserHome(user)`, whose parent must exist once `UserHome` means `.../base`).
3. Risks-section audit undercount (`credentials.go:262` missing) — **resolved**. Line 105 now lists `credentials.go:262` explicitly alongside `server.go:126` and the janitor.

### Fresh read — new items

- **Gap 1 (test-fixture defect):** the Verification upgrade fixture (line 85) plants the setup-done marker as `homes/<me>/.styrman-setup-done` (flat file). The real marker, per `internal/httpapi/setup.go:29`, is `const setupDoneMarker = ".styrman/setup-done"` — a subdirectory + file, not a flat dotfile. Built literally, the fixture creates a path the migration/status code never reads, so the fixture's own assertion ("`GET /api/setup/status` reports done") would need the corrected path to mean anything; as written it risks a false pass or a confusing failure blamed on the migration rather than the fixture. Must be corrected before the Phase 2 fixture is implemented.

- **Gap 2 (incomplete caller audit / missing test coverage):** the plan's "Surfaces touched" no-change list (line 24) enumerates `setup.go`, `credentials.go:263`, `git.go:73`, `explorer.go:443` as `UserHome` callers needing no change, but a full `grep -rn '\.UserHome(' --include=*.go` turns up two more it never mentions: `internal/config/config.go:364` (`LegacyWorkspaceExists`) and `cmd/styrman/main.go:228` (`migrateWorkspaces`, which runs *after* `orchestrator.New`, i.e. after `migrateBaseHomes` has already relocated any stray top-level `projects/` dir into `base/`). I traced both by hand and they resolve correctly under the new layout *only because* `migrateBaseHomes` is guaranteed to run before `migrateWorkspaces` — a dependency the plan states nowhere and the upgrade fixture never exercises (no legacy `~/projects/<dir>` case is in the fixture). This is the same class of gap round 1 caught twice already (an audit claiming completeness that a straight grep contradicts); it should either be added to the caller list with the ordering dependency stated, or the fixture should add a legacy-workspace case to prove it, before implementation.

- **Gap 3 (minor, lower confidence):** several inline comments the plan's own changed lines sit inside — `orchestrator.go:263-297` ("Created through an os.Root at the base home... The base home itself is below the server-controlled HomesDir"), `orchestrator.go:592-593` ("uses the per-user BASE home... create it before the mount"), `server.go:719-722` ("Through an os.Root at the base home"), and `main.go:369-378` ("Everything below the tenant's base home is guest-writable... the project homes sit INSIDE it") — describe the *pre-change* topology (tenant root doubling as the guest-mounted base home) at exactly the call sites the plan repoints to `TenantDir`. Left unedited, they will misstate the guest-writable boundary this whole plan is designed to fix, in the file where the next BUG-002/003/004-style security audit would read them first. The plan's Surfaces Touched list doesn't call for touching these comments.

### Counts

- Gaps requiring a decision before implementation: 3 (2 medium — fixture defect, incomplete caller audit/missing coverage; 1 low — stale security-rationale comments).
- Questions a first-pass implementer would have to ask: 3 — (1) fix the marker path in the upgrade fixture to `.styrman/setup-done`, or is a different check intended?; (2) should `config.go:364` and `main.go:228` be added to the audited caller list, and should the fixture gain a legacy `~/projects/<dir>` case to prove the `migrateBaseHomes`-before-`migrateWorkspaces` ordering holds?; (3) should the four cited security-rationale comments be rewritten for `TenantDir` vs. `base`, or left as author's-discretion cleanup?

design needs revision
implementation not ready

## Round 3

Fresh cold-read against the revised plan (status line: "cold-read rounds 1–2 applied"), plus independent re-verification of round 2's three findings against the current code.

### Round 2 findings — status

1. Fixture marker path (`.styrman-setup-done` vs `.styrman/setup-done`) — **resolved**. Plan's Verification section now plants `homes/<me>/.styrman/setup-done`, matching `internal/httpapi/setup.go:29` (`const setupDoneMarker = ".styrman/setup-done"`, verified by reading the file).
2. Incomplete caller audit (`config.go:364`, `main.go:228`) — **resolved**. Both are now in "Surfaces touched" (plan line 25) with the ordering dependency stated explicitly (`migrateBaseHomes` before `migrateWorkspaces`, which is the actual boot order in `cmd/styrman/main.go`: `migrateLegacy`→`migrateBaseHomes`(new)→`orchestrator.New`→`migrateWorkspaces`→`ensureRepos`, all confirmed by reading `main.go:44-66,303-318`). The fixture now includes a legacy-workspace case (`homes/<me>/projects/legacy-ws/keep.txt`, expected at `base/projects/legacy-ws/`), and a unit test is specified for `migrateBaseHomes` moving `projects/` too. I additionally traced a third caller on the same dependency, `ensureRepos`→`LegacyWorkspaceExists` (`main.go:315`), which the plan doesn't name individually but which resolves correctly under the identical, already-stated ordering guarantee — not a new gap.
3. Stale security-rationale comments — **resolved**. All four sites plus the rule file are now in "Surfaces touched" (plan line 26) and I verified each still holds the pre-change wording today: `orchestrator.go:263-297`, `:592-593`, `server.go:719-722`, `main.go:369-378`, `.agents/rules/guest-writable-paths.md`'s first paragraph.

### Independent re-verification (fresh read)

I re-ran the caller audit myself (`grep -n '\.UserHome(' --include=*.go`, non-test) and got exactly 14 hits; every one is accounted for in the plan's Surfaces/Interfaces split, with the correct disposition (5 "no change", 5 orchestrator/server sites → `TenantDir`, 2 `EnsureSetup` sites staying `UserHome`, plus `migrateLegacy`'s target). The four `EnsureRunning` sites the plan cites (226, 272, 275, 282) are confirmed to be exactly the four `UserHome(user)` occurrences inside that function (verified by grep + function-boundary check), including the `realBelow` call at :282, which is easy to miss but the plan does catch it. `EnsureSetup`'s two `UserHome` sites (594, 602) correctly stay as base-home per the plan. The bind-mount destination format (`"/home/" + guestUser`, `orchestrator.go:814`) matches the stale-bind check design's `"/home/<guestUser>"` comparison. `files.Trust(cfg.HomesDir, …)` confinement is unaffected since `TenantDir(user) == HomesDir/user` unchanged. ADR-064 is the next free number (`DECISIONS.md` ends at ADR-063). No discrepancies found beyond what round 2 already caught and the plan already fixed.

### Nits (not gaps — an implementer resolves these alone)

- Verification section's pure-helper signature, `setupHomeStale(mounts []types.MountPoint, …)`, names the deprecated alias; `insp.Mounts` is actually `[]container.MountPoint` and the file already imports `container`, not `types` (verified in `docker/docker@v28.5.2`'s `types_deprecated.go` and `orchestrator.go`'s import block). Any implementer writing against the real field will use the correct type without needing to ask.
- The plan doesn't state whether the new stale-bind check runs before or after the existing `containerGuest(...) != guestUser` check inside `EnsureSetup` — either order produces the same force-remove-and-recreate outcome, so this is a mechanical choice, not a design question.
- The literal error strings `"create base home"` / `"create project home"` at the `EnsureRunning` call sites being repointed to `TenantDir` read oddly once the comment above them is rewritten for the new boundary; folding this into the same edit is obvious, not a decision.

### Counts

- Gaps requiring a decision before implementation: 0.
- Questions a first-pass implementer would have to ask: 0.
- Nits: 3 (listed above).

design ready
implementation ready
