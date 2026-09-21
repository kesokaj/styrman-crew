---
owner: reviewer
date: 2026-09-21
---
# Review — sh-agent-release-binary   (2026-09-21)

Range reviewed: working tree vs HEAD (`git diff HEAD` + staged deletions per dispatch:
`image/harness/**`, `internal/orchestrator/harness.go`+`harness_test.go`, three
`internal/config/*_test.go`, `deploy/update/styrman-harness-update.{service,timer}`,
`.agents/docs/testing/styrman/make-scratch-harness.sh`)

## Status: COMPLETE (budget-limited — see Not examined)

## Verdict: APPROVE WITH NITS

The removal of the ADR-073 host harness channel and its replacement with a
build-time `gh release download` into `image/Dockerfile` (ADR-074) is
executed cleanly and consistently across every build site (Dockerfile,
`deploy/update/styrman-update`, `deploy/host-setup.sh`, `deploy/styrman-ctl
install`, `deploy/verify-commit.sh`, `ensure-test-image.sh`), the orchestrator
(`internal/orchestrator/orchestrator.go`), config (`internal/config/config.go`),
the entrypoint's never-downgrade guard, `retire_harness_units`'s unconditional
placement ahead of every `host_converge` call (BUG-035-safe), and the Tier 2
integration-suite count/content in `.agents/docs/testing/styrman/README.md`
(verified the "114 checks" claim against an actual `grep -c 'check "'` of
`integration.sh`: 114, matches). The worst problem found is documentation,
not code: two living spec files the dispatch's grep sweep caught still
describe the deleted ADR-073 channel as the CURRENT mechanism because this
diff never touched them, even though the env var and behaviour they document
no longer exist in the code.

## Findings

- **[medium / confidence: high]** `.agents/docs/specs/styrman/api.md:263` — the
  `STYRMAN_HARNESS_DIR` row in the env-var reference table still describes it
  as live ("The host's harness channel content dir (ADR-073) ... Full rules:
  `deploy.md` 'Harness channel (ADR-073)'"). `internal/config/config.go`'s
  diff in this same change deletes the `HarnessDir` field and the
  `STYRMAN_HARNESS_DIR` env read entirely (confirmed via `grep -rn
  HarnessDir internal/ cmd/` → no hits). An operator or agent consulting
  api.md — the canonical env-var reference — for this variable is told about
  a mechanism and a bind-mount contract that no longer exists. Fix: delete
  the row (or replace it with a one-line pointer to ADR-074).
- **[medium / confidence: high]** `.agents/docs/specs/styrman/guide.md:28-32` —
  "the living full guide" (per `AGENTS.md`'s Key Files table, the top
  authoritative doc) still describes project-home seeding as coming "from
  the host's harness channel — the newest signed `styrman-harness` … ADR-073
  — falling back to the image's own mirror when the channel has [not
  delivered]". This diff deletes that channel outright (units, clone, state
  files, server binds) and rewrites the equivalent passage correctly in
  `runtime-image.md`, `deploy.md`, `AGENTS.md`, `README.md` and
  `deploy/README.md` — but not in `guide.md`, which the diff does not touch
  at all. Same family as the two findings above: a living doc left
  describing a deleted mechanism as current. Fix: port the `runtime-image.md`
  §6 rewrite (already done in this diff) into `guide.md`'s equivalent
  passage.
- **[low / confidence: medium]** `image/entrypoint.sh:219-224` — in the
  never-downgrade branch (home's manifest names a release newer than the
  image's, lines 192-201), the actual `sh-agent install` step is skipped, yet
  the final success branch still logs `"agent harness deployed (version
  ${HARNESS_VER}, sh-agent ${HARNESS_REL}; browser MCP registered for claude
  and agy)"` — naming the IMAGE's release as "deployed" even though nothing
  was installed this boot and the home is actually running a different
  (newer) release. The preceding "left alone" log line states the true
  situation, but the "deployed" line immediately follows it and is not
  gated on whether the never-downgrade branch fired, so the boot log carries
  two lines that disagree about what happened. Confirmed the Tier 2 test
  (`integration.sh`) only asserts the "left alone" line is present in this
  case and does not assert the "deployed" line is absent, so this is not
  caught by CI. Not a functional bug (the stamp write itself is correct —
  verified against the Tier 2 never-downgrade section, which checks the
  binary/manifest are untouched and the stamp is rewritten to the image's
  ID) — a diagnostics-clarity issue for whoever reads the boot log to figure
  out which release actually shipped to a given project.

## Criteria

1. **Dead code** — pass. `HarnessDir`/`STYRMAN_HARNESS_DIR`, `harnessBinds`,
   `logHarnessChannel`, the harness-channel units, `HARNESS_REPO_DIR`/
   `HARNESS_ORIGIN` in `styrman-ctl`, and the `image/harness/` mirror tree are
   all removed with no dangling references found in `internal/` or `cmd/`
   (`grep -rn "harnessBinds\|logHarnessChannel\|HarnessDir"` → no hits).
2. **Flow tracing** — pass, with the two doc gaps above and the entrypoint
   log-line nit. The Dockerfile's build-secret block (`set -eu`, scoped
   `GH_TOKEN=... gh release download`, the `${HARNESS_SIGNER%% *}` principal
   extraction, the SHA256SUMS header/version check, the checksum verify, the
   installed binary's own `version` check) is internally consistent and
   traced end to end into `verify-commit.sh`'s matching assertion. The
   `retire_harness_units` placement (before every `host_converge` call, at
   all three call sites, unconditional, idempotent) correctly follows the
   BUG-035 lesson: the tick that ships this change runs the OLD updater
   (which cannot retire anything), but every subsequent tick — using the
   self-installed new script — retires unconditionally, not gated on
   "something changed". New Go test (`TestStyrmanUpdateRetiresHarnessUnits`)
   exercises the gated hook, the ordering of `systemctl` calls, and
   idempotency — a real, failable test. The Tier 2 never-downgrade block
   (six checks) is end-to-end against a real image and correctly plants,
   boots, and asserts both "left alone" and "untouched" outcomes.
3. **Standards conformance** — pass on the code changed; the two stale-doc
   findings above are the standards gap (the describing-text rule).
4. **Directory layout** — pass; no new files land outside expected locations.
5. **Simplicity / DIY-except-security** — pass. `ssh-keygen -Y verify`
   against a pinned public signer line is the existing project pattern
   reused, not new hand-rolled crypto; `sha256sum -c` and `gh release
   download` are stdlib/vendor-standard tooling, not composed primitives.
6. **Decoupling** — pass; the four build sites each implement the same
   "GH_TOKEN env, else file, else stop" rule independently but consistently,
   which is reasonable given they are four different scripts/tools with
   different invocation contexts (no shared shell library exists to factor
   it into, and none is proposed as a dependency).

## Not examined

- The full remainder of `deploy/styrman-ctl` beyond the sections diffed
  above (`cmd_uninstall`, the rest of `setup_auto_update`, the JSON schema
  docs further down) — read only the visible diff hunks, not the whole file
  post-change.
- `deploy/update/styrman-update`'s `self_install` function body beyond the
  opening ~30 lines shown.
- The three deleted `internal/config/*_test.go` files
  (`harnessdir_test.go`, `styrman_ctl_harness_test.go`,
  `styrman_harness_test.go`) were not individually diffed to confirm nothing
  beyond harness-channel-specific coverage was lost — inferred safe from
  their names and from `config.go`'s clean field removal, not directly read.
- `internal/orchestrator/harness.go`/`harness_test.go` deletions were not
  read in full (only the `orchestrator.go` diff that stopped calling into
  them was read); trusted the removal is complete based on the repo-wide
  grep finding no remaining references.
- `deploy/docker-compose.yml`, `deploy/host-setup.sh` beyond the hunks shown,
  `web/` (no web changes appeared in `git status`, but this was not
  independently confirmed with a full `git diff --stat` filter for `web/`).
- `image/configs/harness-migrate.sh` — referenced by name in several diffs
  as "unchanged" but not read to confirm it needs no change under ADR-074.
- Did not run `go build`, `go test`, or any linter — relied on reading the
  diff; no verification claim is made about compile/test status.

## Outcome

Written by the author, 2026-09-21.

1. `.agents/docs/specs/styrman/api.md` `STYRMAN_HARNESS_DIR` row — FIXED: the row deleted (the key no longer exists in `internal/config`).
2. `.agents/docs/specs/styrman/guide.md:28-32` — FIXED: the seeding sentence rewritten for ADR-074 (the image's `sh-agent` release, install on first boot and image bumps, never over a home moved forward with `sh-agent update`).
3. `image/entrypoint.sh` "deployed" after a skipped install — FIXED: the skipped branch sets `HI_SKIPPED=1` and the pass line reads `agent harness kept (version …: home sh-agent vA is newer than the image's vB, install skipped; …)`; `deployed` is logged only when install ran. Test built on the finding's arrangement: Tier 2's never-downgrade case now counts `agent harness deployed (version` lines before the stop and asserts the count is unchanged after the boot and the `kept` line present ("the left-alone boot logs 'kept', not a new 'deployed' line") — a check that fails without the fix, since the old code logged a fresh `deployed` line on that boot. Tier 2 re-run after the fix: see the addendum line below.

Addendum: Tier 2 re-run after the fixes printed `passed=112 failed=0 skipped=3 aborted=0` (115 checks), including `PASS the left-alone boot logs 'kept', not a new 'deployed' line` (`.agents/scratch/tier2b.log`).
