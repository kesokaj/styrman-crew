---
name: adr-format-super-devbox
description: ADR conventions actually used in super-devbox's .agents/docs/architecture/DECISIONS.md — numbering, heading format, body structure
metadata:
  type: project
---

super-devbox's `architecture/DECISIONS.md` ADRs (through ADR-040,
2026-08-30) follow a consistent local style, slightly different per era but
converged by ADR-022+. Latest entry as of 2026-08-30 is **ADR-040**; confirm
the next-free number by reading the file's tail before appending — it has
been renumbered upward multiple sessions in a row (025→034→038→040).

**Tool-access note (2026-08-30):** the changelog subagent has sometimes run
in a harness with only Read/Grep/Glob/Edit/Write — no Bash/git tool, so
`git show <hash>` is not available even when a task cites specific commit
hashes as "source of truth." Workaround that worked well: treat the task
description's per-commit summary as the lead, then spot-verify claims by
grepping the CURRENT (post-merge) source tree for the finding IDs the review
used (e.g. "H2", "M17", "M21" as inline code-comment tags) — this codebase's
convention of citing finding IDs directly in comments next to the fix (see
[[changelog_format_super-devbox]]) makes this surprisingly reliable; most
claimed fixes in the 2026-08-30 security-review changelog/ADR-039/ADR-040
were confirmed this way rather than via a real diff.

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

**Amendments/reversals are NEW sequential ADRs, never edits to the old
one** (the file is append-only). Precedent: ADR-032a ("amends ADR-032") and
ADR-038 ("amends ADR-032a") are both separate numbered entries whose
`**Context:**` paragraph names the ADR they revise and says what changed;
the old ADR's own text is left untouched (a task may ask to add a note
"under" the old ADR — translate that into a new trailing entry that
references it, not an in-place edit). Use the phrase "superseded on the
`<specific point>`" when only part of an old ADR is wrong (e.g. ADR-032a's
broker-tier shape stayed valid; only its agy-storage assumption was wrong),
rather than declaring the whole prior ADR dead when it isn't.
