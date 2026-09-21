---
owner: reviewer
date: 2026-09-21
---
# Review — plan-sh-agent-release-binary round 2   (2026-09-21)

Range reviewed: `.agents/docs/plans/2026-09-21-sh-agent-release-binary.md` (revised since round 1; cold-reader plan review, no diff exists yet). Also read for verification: `image/Dockerfile` (current, lines 1-180), `image/entrypoint.sh` (harness block, lines 110-224), `deploy/update/styrman-update` (`host_converge` and its three call sites, lines 794-1086; `__verify-tag` test-hook pattern, 421-437; the existing `/etc/styrman/gh-token` use, 590-598), `internal/config/config.go` (`HarnessDir`), `deploy/docker-compose.yml` (`STYRMAN_HARNESS_DIR`), the four `docker build` call sites in `deploy/host-setup.sh`, `deploy/styrman-ctl`, `deploy/update/styrman-update`, `deploy/verify-commit.sh`, `.agents/docs/architecture/DECISIONS.md` (ADR-073, confirming ADR-074 is free). Also read the styrman-harness checkout the plan cites, at HEAD `914dc90` (`git describe` = `v1.6.0-3-g914dc90`): `internal/harness/source.go` (`EmbeddedPrefix`, `ResolveSource`), `internal/harness/apply.go` (`Install`, the `repo := EmbeddedPrefix+src.Version` line), `internal/harness/state.go` (`State.Repo` tag, `SaveState`'s `MarshalIndent(s, "", "  ")`), `internal/harness/release.go` (`UpdateFromRelease`), `cmd/sh-agent/main.go` (the `install`/`update` command dispatch) — none of these were named by round 1's dispatch but are the code the plan's D4/G3 claim rests on, and the plan explicitly cites `source.go:44` and `apply.go:117`.

## Status: COMPLETE

## Verdict: APPROVE

All three findings from round 1 (`.agents/docs/reviews/2026-09-21-review-plan-sh-agent-release-binary.md`) are resolved in this revision, verified independently against the real code on both sides (crew and the styrman-harness checkout). No new blocking gap surfaced. One low-severity, non-blocking accuracy nit remains in a citation.

## (1) Gaps

- **G1 (round 1, Dockerfile `/styrman/bin` not yet created) — RESOLVED.** The revised RUN block's first statement is now `mkdir -p /styrman/bin;` before the `gh release download`/`install` sequence (plan lines 65-66). Confirmed against the current Dockerfile (`mkdir -p /styrman` still only runs at today's line 142, later than where the new RUN is inserted) that this is necessary and sufficient — the RUN now creates its own destination rather than depending on insertion order. Not a gap.

- **G2 (round 1, ambiguous placement of the harness-unit-removal step relative to `host_converge`'s firewall guards) — RESOLVED.** The revised Interfaces section is now explicit: `retire_harness_units` is "OUTSIDE `host_converge` and independent of its guards," with its own two guards named (`UPDATER_UNIT_DIR` set, `trusted_dir` on it), "Called immediately BEFORE each of the three `host_converge` calls in the tick (today lines ~1015, ~1055, ~1083)." I read those three call sites in the live script (`deploy/update/styrman-update:1015,1055,1083`): all three are top-level statements in the main tick flow, not nested inside `host_converge` or any of its firewall-specific early-return guards (lines 808-832 are internal to the `host_converge` function body). Placing a sibling call immediately before each is unambiguous and does reach every tick regardless of `REQUIRE_SIGNED` or firewall-dir trust state, closing the exact BUG-064 shape the plan cites. Not a gap.

- **G3 (round 1, unverified `state.json` format assumption underlying D4) — RESOLVED, and independently confirmed correct.** The revision cites `source.go:44` (`EmbeddedPrefix = "embedded:"`) and `apply.go:117` (`repo, commit := EmbeddedPrefix+src.Version, ""`), and adds that Phase 1's entrypoint verify step now runs the real binary's `install` and greps its actually-seeded `state.json`, "the real binary's output validates the pattern" — before Tier 2's synthetic plant reuses the shape. I traced this further than the plan does and confirmed the residual risk round 1 flagged (that only `install`, not `update`, was being verified) does not matter: `harness.UpdateFromRelease` (`release.go:90-179`, the code path a release binary's `sh-agent update` actually runs) downloads and replaces the binary, then re-execs the **new** binary's own `install` subcommand (`release.go:173`, "the new binary installs the harness it carries; this process is the old one and must not do it") — i.e. `update` and `install` converge on the identical `Install()`/`SaveState()` call with `EmbeddedSource(newVersion)`. So verifying `install`'s output format, as Phase 1 now does, is not a weaker proxy for `update`'s output — it is the same code path. `state.go:17` confirms `Repo` is the JSON-first field (`json:"repo"`) and `SaveState` uses `json.MarshalIndent(s, "", "  ")` (two-space indent), matching the plan's `sed -n 's/^ *"repo": *"embedded:\(v[0-9][0-9.]*\)".*/\1/p'` exactly. D4's guard is sound. Not a gap.

- **Minor, non-blocking accuracy nit.** The plan's citation says "checked in the harness checkout at v1.6.0 (`~/workspace/styrman-harness`, commit 914dc90)." `914dc90` is actually 3 commits past the `v1.6.0` tag (`git describe` → `v1.6.0-3-g914dc90`), not the tagged commit itself. I confirmed `git diff v1.6.0 HEAD -- internal/harness/{state,apply,source,release}.go cmd/sh-agent/main.go` is empty, so the three intervening commits did not touch any file the plan's claim depends on — the citation's substance is correct, only its "at v1.6.0" phrasing is loose. No fix needed before implementation; worth a word-choice tweak ("commit 914dc90, 3 commits after the v1.6.0 tag, unchanged in the relevant files") if the plan is touched again, but not a gap that blocks starting Phase 1.

## (2) Scope

- D4 (the never-downgrade guard) remains implementer-attributed scope beyond the operator's literal "run install, don't complicate things," exactly as round 1 noted. The revision now carries an explicit `Open` line with a stated default ("keep the never-downgrade guard (D4)? … default: keep (six lines; without it an image whose ARG lags a release the user already updated to would reinstall the older harness over the newer one on the next stop/start, silently)"). Per the readiness rule, a plan that states a default converts what would otherwise be a silent gap into a decision a reasonable implementer can proceed on without asking — this is no longer a blocking scope question, only worth the one-line confirmation the plan itself already offers to surface.
- No other unrequested surface found. The build-secret propagation to four call sites (`styrman-update`, `host-setup.sh`, `styrman-ctl`, `verify-commit.sh`) was spot-checked against the live files — all four sites named in the plan's Interfaces section do call `docker build` for the runtime image, confirming the plan's account of "four scripts build the image" is accurate, not an invented surface.

## (3) Readiness

Round 1's two BLOCKING questions are resolved by this revision (G2, G3 above) and independently confirmed correct against the real source on both repos. Round 1's two NON-BLOCKING questions are also resolved (G1 fixed in text; D4's status is now an explicit default rather than a silent gap).

No BLOCKING question remains that I can find. Two NON-BLOCKING items an implementer could still reasonably raise, neither of which two competent implementers would resolve differently:
1. NON-BLOCKING — confirm keeping D4 (the plan already states the default and the reasoning; asking is optional, not required).
2. NON-BLOCKING — the `914dc90` vs `v1.6.0` tag citation wording (cosmetic; the content it backs is verified correct).

design ready / implementation ready

## Criteria (as applied to a plan, not a diff)

1. Dead code — pass: every deleted artifact (mirror tree, builder stage, channel verbs/units/tests) is named; no leftovers spotted.
2. Flow tracing — pass: the two previously-broken hops (host_converge guard independence, entrypoint → state.json format) are now both traced correctly and confirmed against live source on both repos.
3. Standards conformance — pass: follows `mechanism-reaches-every-caller.md` (BUG-064/BUG-035 discipline, now concretely placed) and `verify-bare.md`/`writing-files.md` conventions in its verify clauses.
4. Directory layout — pass: no new files outside named surfaces.
5. Simplicity (DIY-except-security) — pass: build-time `ssh-keygen -Y verify` + `sha256sum -c`, no new dependency, no hand-rolled crypto; D4 is the one piece of added logic, now with a stated default.
6. Decoupling — pass: image/server/host changes remain cleanly separable per phase.

## Not examined

- `deploy/verify-commit.sh`'s current mirror section body, `deploy/styrman-ctl`'s harness provisioning code past the `docker build` line, `.agents/docs/testing/styrman/integration.sh`/`README.md`/`make-scratch-harness.sh`, the docs files named in Phase 4 (`specs/styrman/{deploy,versioning,runtime-image}.md`, `deploy/remote.md`, `image/README.md`, `deploy/README.md`) — not re-opened this round; round 1 already flagged these as not examined and this round's scope was to verify whether the revision closed G1/G2/G3, which it does, plus a scope/readiness pass over the Interfaces section's other claims (build-secret sites, ADR numbering, config/compose harness references), which checked out.
- `web/` roster endpoint (the plan's "Out" claim that Settings is unaffected) — not independently checked this round either; carried over from round 1's same gap.
