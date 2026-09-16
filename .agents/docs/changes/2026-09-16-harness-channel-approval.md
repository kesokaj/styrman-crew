---
owner: main
date: 2026-09-16
---
# Harness channel: scope confirmed, R5 amended, ADR-073, plan opened (2026-09-16)

## Summary

The operator confirmed the scope of the host-side, signature-gated harness channel on all ten open points ("Approve all your fixes") and approved the exact R5 replacement text and ADR-073 ("Approve"). This commit lands the trajectory change as the hard rule requires — the REQUIREMENTS.md edit, the ADR and `ROADMAP.md` "Now" in one commit — together with the plan and its recon brief. No source changed.

## Type

feature (plan and requirements; the phases follow)

## Files changed

- `REQUIREMENTS.md` — R5: the agent harness is carried by the host's verified harness channel (the newest SSH-signed `styrman-harness` tag that verifies against `/etc/styrman/allowed_signers`, exported under `/var/lib/styrman/harness/current`, mounted read-only into every project VM — ADR-073, operator approval 2026-09-16), the base image's pinned mirror remaining the fallback until the channel has delivered; the default shell config stays with the base image; both seeded identically
- `.agents/docs/architecture/DECISIONS.md` — ADR-073 appended (D1–D7; supersedes ADR-062 D1 and the reason of D4; keeps D2, D3, D5, D8 and ADR-072)
- `ROADMAP.md` — "Now" names `.agents/docs/plans/2026-09-16-harness-channel.md`
- `.agents/docs/plans/2026-09-16-harness-channel.md` — the plan: four phases, interfaces, failure modes, verification, D1–D15
- `.agents/docs/plans/2026-09-16-recon-harness-channel.md` — the explorer's brief over the updater, image, orchestrator, config, tests and docs

## Agents dispatched

explorer (the recon brief); reviewer (the cold-reader check of the plan, in flight at the time of this commit — its report lands with Phase 1)

## Next

Phase 1 of the plan once the cold read passes; nothing else.
