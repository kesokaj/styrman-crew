---
owner: reviewer
date: 2026-09-11
---
# Review — cold-read of `.agents/docs/plans/2026-09-11-installer-ctl.md` (round 2)   (2026-09-11)

Range reviewed: plan file `.agents/docs/plans/2026-09-11-installer-ctl.md` (revised) plus the code surfaces it names: `deploy/host-setup.sh`, `deploy/firewall/install.sh`, `deploy/update/styrman-update`, `deploy/docker-compose.yml`, `deploy/.env.example`, `internal/config/config.go`, `internal/config/firewall_script_test.go`, `.agents/docs/specs/styrman/{deploy,versioning}.md`, `.agents/rules/*`, `REQUIREMENTS.md`, `AGENTS.md`. No diff exists — nothing is implemented yet. Round-1 report read to check each finding against the revision.

## Verdict: DESIGN NEEDS REVISION

Four of round 1's five gaps are cleanly resolved in this revision (clone/`.env` ownership now matches `host-setup.sh`'s root:root convention, the KSM tmpfiles path is now named explicitly with an explicit "leave KSM itself alone" decision, `validate()` now assigns a rule to every key in `.env.example`, and `OPERATOR`'s default chain is now explicit). But the one finding that mattered most — uninstall level 1's container selector — was fixed in the Scope section and **not** in the Interfaces "exact flow" section, so the plan now contradicts itself on the exact bug round 1 flagged, and an implementer following the more literal/copy-pasteable section would reproduce it.

## Gaps (must be decided before implementation)

1. **[high / confidence: high]** Self-contradiction on uninstall level 1's container filter, reintroducing round 1's bug. Scope (lines 56–64) now correctly says containers are "selected by the label `styrman.managed=true`" and explains why a project-label filter would miss the setup VM. But the Interfaces § `uninstall` "exact" flow (lines 276–277) still reads: "for each container `styrman-*` (label `styrman.project`): `docker rm -f`" — unchanged from round 1, still `styrman.project`, not `styrman.managed=true`. `internal/config/config.go:27-33` confirms `LabelManaged` (`styrman.managed`) is the label both project containers (`orchestrator.go:420-421`) and the setup VM (`orchestrator.go:748-749`, `LabelSystem: "setup"`, no `LabelProject`) carry, while `LabelProject` is project-only. Whichever section an implementer codes from — and the Interfaces section is written to be the literal, orderable command list — half the document tells them to use the selector round 1 already proved wrong. Must fix the Interfaces line to `styrman.managed=true` (or explicitly reconcile which text governs) before Phase 4.

2. **[low / confidence: medium]** Both of round 1's non-blocking minor questions remain open in this revision: whether the `GIT_ASKPASS` one-shot helper in `update --setup` step 2 (lines 228–231) also covers the `git -C ... fetch` branch for an existing clone, and what exact command produces "the host's name" for level-3 confirmation and the `status` header (lines 76–78, 295) — neither `host-setup.sh`, `firewall/install.sh`, nor `styrman-update` defines or uses a hostname command the plan could be citing. Not blocking (round 1 also called these minor), but still undecided.

## Resolved since round 1 (verified against code)

- Clone/`.env` ownership (round-1 finding 2): now `install -m 600 -o root -g root` + `chown -R root:root`, matching `host-setup.sh`'s 8c prose (`deploy/host-setup.sh:320,324-325`). Resolved.
- KSM tmpfiles path (round-1 finding 3): now names `/etc/tmpfiles.d/styrman-ksm.conf` explicitly and states KSM itself is left alone. Resolved.
- `validate()` coverage (round-1 finding 4): now assigns a rule to every key in `deploy/.env.example` (checked against the file's full 25-key list — all covered, including `STYRMAN_BROKER_ADDR`, `DEFAULT_RPS`, `CADDY_ADMIN`, `AUTH_DOMAIN`/`SHELL_DOMAIN`/`APPS_DOMAIN`). Resolved.
- `OPERATOR` default (round-1 finding 5): now explicit — `--user`, else `$SUDO_USER`, else die. Resolved.

## Readiness

Not implementation-ready. One question an implementer must ask before coding uninstall (blocking):
- Which text governs level 1's container selector — the Scope section's `styrman.managed=true` or the Interfaces section's `label styrman.project` (finding 1)? These are not equivalent and only one is correct.

Two minor, non-blocking questions carried over from round 1 (would not stop a first pass, but should be settled before merge):
- Does the `GIT_ASKPASS` helper in `update --setup` cover the `fetch` branch too?
- What command produces "the host's name" for level-3 confirmation / `status`'s header?

## Not examined

`deploy/update/styrman-update`'s full body beyond the interface points cross-checked (`--check`/`--force`, `STYRMAN_UPDATE_STATE_DIR` validation, `host_converge`); `deploy/README.md`'s current full text; `.agents/docs/specs/styrman/deploy.md` in full; no code was written or run — this is a plan-only cold read.

design needs revision
