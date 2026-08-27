---
name: adr-format-super-devbox
description: ADR conventions actually used in super-devbox's .agents/docs/architecture/DECISIONS.md — numbering, heading format, body structure
metadata:
  type: project
---

super-devbox's `architecture/DECISIONS.md` ADRs (through ADR-025,
2026-08-27) follow a consistent local style, slightly different per era but
converged by ADR-022+:

- Heading: `## ADR-NNN: <Title> (YYYY-MM-DD)` — sequential, no gaps. Earlier
  entries (ADR-001–021) put `**Date:** ... · **Status:** accepted (...)` on
  its own line below the heading instead of inlining the date; ADR-022
  onward inlines the date in the heading and drops the `**Status:**` line
  entirely (status is implied — everything in the file is accepted; a
  reversal gets marked `Superseded by ADR-NNN` inline, not a status field).
  Prefer the newer (ADR-022+) style for new entries: `## ADR-NNN: Title
  (YYYY-MM-DD)` with no separate Date/Status line.
- Body: bold inline labels as paragraph leads — `**Context.**`,
  `**Decision.**`, `**Consequences.**` (period after the bold word, not a
  colon-header on its own line). Optional `**Alternatives.**` paragraph when
  options were rejected (seen in ADR-022, ADR-023). Dense, technical prose —
  written for the operator, references real file paths, function/type names
  (`project.Project.SharedPorts`, `previewHostRewrite`, etc.) so claims are
  independently greppable.
- `---` horizontal rules separate entries throughout the file. Append a new
  `---` before each new ADR when appending to the end.
- A trailing `Source: path/a.go, path/b.go` line (or "Verified empirically",
  "Verification: .agents/docs/testing/...") is common when the ADR is
  significant enough to want a code pointer — not mandatory but matches
  recent entries (ADR-015, 024, 025).
- Repo has no `.git` (not a git repo per the environment info at time of
  writing) — do not expect `git log` to corroborate ADR dates/authorship;
  rely on reading the actual code + existing changelog entries in
  `.agents/docs/changes/` instead, which often predate/motivate an ADR (e.g.
  `2026-08-27-public-ttl-links-ingress.md` → ADR-025).

**Why:** matching house style (not the generic template) keeps DECISIONS.md
internally consistent; the repo's own precedent is the authority per the
task instructions ("read the existing ADR style first and match it").

**How to apply:** when appending new ADRs here, use the ADR-022+ inline-date
heading style, bold-lead paragraph body, `---` separators, and a trailing
`Source:` line pointing at the real files. Cross-check technical claims
against the code and (if one exists) the changelog entry that shipped the
feature before writing — don't take a task description's claims on faith.
See also [[changelog_format_super-devbox]] for the sibling changes/
convention.
