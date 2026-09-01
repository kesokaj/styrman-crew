---
name: audit-entries-pattern
description: How whole-project /audit findings get recorded — one changelog entry summarizing the merged report, findings triaged one-line-each into TODO.md, no ADR unless a decision was actually made
metadata:
  type: project
---

This repo runs periodic whole-project `/audit` passes (scanning beyond the
diff-scoped release gate) in addition to per-release `/preflight` gates.
Observed convention for recording one (2026-09-01, v1.16.0, merged report
at `.agents/docs/reviews/2026-09-01-audit-whole-project.md`):

- **One changelog entry** at `changes/YYYY-MM-DD-audit-vX.Y.Z.md`
  summarizing the merged multi-scanner report (explorer + reviewer +
  security + verifier run in parallel, each producing its own report under
  `reviews/`/`testing/`/`plans/`). Follow [[changelog_format_super-devbox]]
  for structure — title, `Date:` opening line, thematic sections, no
  literal Type/Requirements fields.
- Audits explicitly are NOT remediations — state plainly and early in the
  entry that no fixes were applied, so a future reader doesn't assume
  findings were resolved just because a changelog entry exists.
- Every open finding gets triaged as a **one-line item in `TODO.md`** with
  a severity marker (🔴/🟡/🟢), a file:line reference, and a short parenthetical
  citing the audit date + report path. Do not write a longer TODO entry
  than that — the changelog entry and the linked report carry the detail;
  TODO.md stays scannable.
- **No ADR for the audit itself** — an audit is a record of findings, not
  an architectural decision. Only write an ADR if the audit's dispatcher
  explicitly asks you to record a *decision* (e.g., adopting a new gate,
  reversing a prior ADR) separately from the findings triage. When in
  doubt, the default is no ADR.
- It's normal and worth recording explicitly when the audit surfaces a
  finding through its own verification activity (e.g., a documented test
  procedure accidentally mutating a production-referenced artifact,
  discovered and restored during the audit) — this is signal about gate
  coverage gaps, not noise to omit.

**Why:** matches how the 2026-09-01 whole-project audit was actually
recorded — confirmed against the dispatcher's explicit instructions for
that task, which distinguished "record an audit" from "record a
remediation" and explicitly said an ADR was likely unwarranted.

**How to apply:** when dispatched to record a `/audit` (as opposed to a
task/bugfix or a `/preflight` release gate), reach for this pattern first:
one changelog entry over the merged report, TODO.md one-liners per open
finding, no ADR by default.
