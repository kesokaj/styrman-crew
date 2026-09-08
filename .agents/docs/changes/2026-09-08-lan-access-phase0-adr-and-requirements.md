---
owner: changelog
date: 2026-09-08
---

# Per-project LAN access — architecture decision and R12 amendment (Phase 0)

Date: 2026-09-08. Phase 0 of the per-project local-network access feature: ADR-065 drafted and approved, R12 amended, design reviewed cold-reader-ready.

**Summary:** The operator approved the per-project LAN access feature proposal on 2026-09-08 ("Go do work" on a seven-decision framework). ADR-065 records the full design: server intent (projects.json LanAccess), marker files, root enforcement via a self-healing firewall script, sub-second delivery via a path unit, and the delivery gap fix (host_converge in the auto-updater). R12 is amended with the local-network exception and the corrected list of six dropped IPv4 ranges. The architecture was reviewed in two cold-reader rounds; research employed a 15-agent workflow plus adversarial critique with seven verified refutations.

**Files:**
- `.agents/docs/architecture/DECISIONS.md` — ADR-065 and ADR-065 amendment appended, both dated 2026-09-08.
- `REQUIREMENTS.md` — R12 amended with the local-network exception and the corrected enforcement clause.
- `.agents/docs/plans/2026-09-08-per-project-lan-access.md` — full design document including research notes, cold-reader feedback, failure modes, verification tiers, interfaces, and five-phase scope (Phase 1 host-half ships this session; phases 2–5 TBD).

**Verification:** Cold-reader check passed 2026-09-02 round two (ready to implement). Adversarial critic's seven original needs-fixes items and thirteen holes all folded into the plan and verified on this host (three judges synthesized the result). No code shipped this phase.

**Next (Phase 1):** Host-half implementation (firewall script, install.sh, systemd units, Go-side marker mirroring, auto-updater integration). See 2026-09-08-lan-access-phase1-host-half.md.
