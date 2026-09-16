---
owner: main
date: 2026-09-16
---
# Handoff — 2026-09-16 — preflight v1.34.0 NO-SHIP: a stale Tier 2 check and an entrypoint log filter, both recorded, neither fixed

Session ID: 742de840-bb85-4deb-a1e1-fc40fde28110        (same machine: `claude --resume 742de840-bb85-4deb-a1e1-fc40fde28110`)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator asked to ship the harness refresh (mirror at styrman-harness `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`) as v1.34.0 through `/preflight`, then verify the attribution key on one re-seeded project, and to stop at any gate the skill names. The gate is **NO-SHIP**: nothing was tagged, pushed as a release, or deployed; the live host stays at v1.33.0.

## Completed

- Gate over `v1.33.0..9dab61d`, every row dispatched (the 4aa19be commit had never had a code lens): `.agents/docs/reviews/2026-09-16-preflight-v1.34.0.md` — VERDICT: NO-SHIP, three blocking conditions in order: the code review's high finding (`integration.sh:137-138` asserts `~/.claude/settings.json` absent; the harness now creates it), the verifier's FAIL (Tier 2 `passed=93 failed=1 skipped=3` in four runs, that one check), and the security row partial by the letter (15 trail files under `.agents/docs/` in its `## Not examined`).
- Green in the same gate: `deploy/verify-commit.sh 9dab61d` ("safe to tag", runtime image seeds 27/7 at 4aa19be, web trio, both images); root 291/0/1, ingress 53/0/0, mirror 79/0/1; vulncheck only the accepted docker/docker pair; security no high (1 medium carried, 3 low left to styrman-harness); docs lens 3 findings, fixed.
- Records: `BUG-061` (the stale check plus the "stdout discarded" comments and `testing/styrman/README.md:448`) and `BUG-062` (the entrypoint's `^settings:` filter drops the installer's `note: ` refusal lines); three docs statements fixed by `documentation` (`.agents/docs/testing/styrman/README.md:402-408`, `.agents/docs/specs/styrman/runtime-image.md:244-246`); `changes/2026-09-16-preflight-v1.34.0-no-ship.md`.
- Trail commit `6bdbe02` (everything above), this handoff on top; `main` pushed with no tag — the updater acts on signed tags only, so the push deploys nothing.

## Open / In Progress

- BUG-061 and BUG-062: open, fixes pending (Next Steps 1). Nothing else in flight.
- Requirements edits awaiting approval: none.

## Key Decisions

- NO-SHIP rather than fix-and-reship in the same session: the skill's Step 6 says stop on NO-SHIP, and the operator's brief said "stop at any gate /preflight names"; the fix is a bug task, then a fresh gate.
- The security row's 15 unread trail files were recorded as blocking by the gate's literal rule even though they are prose records — the operator asked that a partial row block; my assessment (low risk) is in the report, not a rebuttal.
- The one running project (`local-lan-test`, LAN mode, on `styrman-runtime:1.33.0`) was not stopped or restarted: it had an attached, focused tmux client. The post-release check will use a throwaway project instead (Next Steps 3).
- The three docs-lens findings were fixed before the trail commit (a description that outlives its behaviour is worse than none); `README.md:174-175` (the v1.20.0 bullet) left as dated history.

## Corrections

- None from the user this session.

## Agent notes (unverified)

- The verifier stopped at its 30-turn limit twice and re-ran the integration suite four times chasing a boot-log capture it cannot get after the suite's LAN toggle recreates the container (the seed runs once per stamp). A `settings:` line in a REAL project boot log has therefore never been observed; the wording was confirmed by a direct `sh-agent install` in a throwaway container of `styrman-runtime:test`, and the suite's own "boot log records the harness deployed" check passed at 4aa19be.
- Draft corrections by the main agent: verifier log, one count (three runs → four); `bugs`, one invented count ("Three records" → "Both"); `changelog`, two (an invented "(HIGH)" on the verifier row; "one FAIL in four runs" made exact) plus two omitted facts added.
- `~/workspace/styrman-harness` HEAD is `8b95f22`, ahead of the mirrored 4aa19be (the verifier noticed). Whether to refresh the mirror again before re-gating is the operator's call; this range only claims 4aa19be.
- Rule candidate for the next retro (from BUG-061): the docs lens greps living docs but not test scripts; when a requirement's wording changes, `.agents/docs/testing/**/*.sh` and `web/e2e/` need the same grep.

## Environment State

- Branch: `main`, clean; `6bdbe02` + this handoff, pushed; no tag.
- Running processes: none of ours. The production stack (`styrman-server:1.33.0`, `styrman-caddy:1.33.0`) and the operator's project microVM are untouched; `styrman-update.timer` active, last tick "already up to date".
- Scratch (gitignored): `.agents/scratch/verify-v1.34.0-release/` (the verifier's outputs, four `integration*.txt`), `.agents/scratch/preflight-v1.34.0/allowed_signers` (a root-read copy of the updater's signer allowlist — the operator's public key, one line).
- Docker Hub anonymous pull budget: 100/100 at 13:15; the next gate's builds (verify-commit ×2, test image, updater ×3) fit in one hour.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-16-preflight-v1.34.0.md` — the gate: findings, rows, what was not checked, the Next section the fix task appends `## Outcome` to.
- `.agents/docs/bugs/BUG-061-harness-refresh-settings-check-stale.md`, `.agents/docs/bugs/BUG-062-entrypoint-settings-filter-drops-note-lines.md` — the two defects with their pending fixes.
- `.agents/docs/reviews/2026-09-16-review-5418f19-9dab61d-code.md` — the code review (five findings).
- `.agents/docs/reviews/2026-09-16-security-v1.33.0-9dab61d.md` — the security row; its `## Not examined` lists the 15 files the next security dispatch must read.
- `.agents/docs/testing/2026-09-16-verify-v1.34.0-release.md` — the verifier log (FAIL, with the green rows).
- `.agents/docs/changes/2026-09-16-preflight-v1.34.0-no-ship.md` — the session's entry.

## Next Steps

1. **Fix BUG-061 and BUG-062 under the `bug` skill** (one task, smallest fix): in `.agents/docs/testing/styrman/integration.sh:137-138` keep the `.agents/bin` and `GEMINI.md` clauses and replace the settings.json clause with a positive assertion (`jq -e '.attribution.commit == "" and .attribution.pr == "" and (keys == ["attribution"])'` on `${GH}/.claude/settings.json`), rename the check; fix the "stdout discarded" comments at `integration.sh:119-121` and `image/entrypoint.sh:126-128` and `testing/styrman/README.md:448`; in `image/entrypoint.sh:159-170` log `^settings:` and `^note: ` lines (bounded as now) and make the comment say what is visible. Verifier: Tier 2 must print `passed=94 failed=0 skipped=3` (brief it per the gate report's verifier notes: one run, capture the itest container's `docker logs` right after the harness-deployed check, write early). Reviewer over the fix; `bugs` closes the two records; append `## Outcome` to the gate report in the fix commit.
2. **Re-run `/preflight v1.34.0`** over `v1.33.0..<new HEAD>`; brief the security agent to read every path in that diff end to end, trail files included, so `## Not examined` names none of them; the release entry names COMMIT `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`; sign the tag with `git -c gpg.format=ssh -c user.signingkey=~/.ssh/id_ed25519.pub tag -s v1.34.0 …` (no git config change); the updater deploys within one 5-minute tick.
3. **After the release is live, verify on one re-seeded project** without touching `local-lan-test`: create a throwaway project through the API from inside the server container (`docker exec styrman-server sh -c 'curl -s -X POST -H "X-Styrman-Ingress: $STYRMAN_INGRESS_TOKEN" -H "X-Caddy-Email: <operator email>" -H "Content-Type: application/json" -d "{\"name\":\"verify-v1-34-0\"}" http://localhost:8080/api/projects'`; project names match `^[a-z][a-z0-9-]{0,30}$`), then `docker logs styrman-<uuid>` for `agent harness deployed (version 1.34.0, styrman-harness 4aa19be…)` and one `settings: /home/<user>/.claude/settings.json created with …` line; in the guest as the project user with a login shell (`bash -lc`, which materializes the broker's credentials): `grep -c '"attribution"' ~/.claude/settings.json` → 1, `sh-agent status` → "repo has 27 skills, 7 agents" and no note line, and a commit made by `claude -p` in `~/projects/<dir>` with no `Co-Authored-By` trailer (`git log -1 --format=%B`). Record as `.agents/docs/testing/<date>-verify-release-v1.34.0.md` with the commands and output as printed; lead with anything that did not match; then delete the throwaway project through the API (`DELETE /api/projects/<uuid>`).
4. Operator's call, not queued: refresh the mirror to the harness's current HEAD (`8b95f22`) before step 2, or ship 4aa19be as gated.
