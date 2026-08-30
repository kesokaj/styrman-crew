---
name: changelog-format-super-devbox
description: The changelog entry format actually used in super-devbox's .agents/docs/changes/, which differs from the generic Summary/Type/Files-changed/Verification/Requirements template
metadata:
  type: project
---

super-devbox's `.agents/docs/changes/` does NOT use the generic
Summary/Type/Files changed/Verification/Requirements template. The
established convention (seen across ~15 entries from 2026-08-26 onward) is:

- Filename: `YYYY-MM-DD-short-kebab-description.md`, one per completed
  operator batch (often several features/fixes bundled, not strictly one
  file per task).
- Title: `# <short, punchy description of the batch> (vX.Y.Z)` — the version
  string in parens is near-universal on entries from ~v1.5.0 onward (a couple
  of the earliest entries, pre-versioning, lack it). NOT
  `# YYYY-MM-DD — description` (that was this memory's earlier, wrong guess —
  corrected 2026-08-30 after re-checking ~5 more entries). The date instead
  opens the body as its own line: `Date: YYYY-MM-DD. <first sentence of
  context>`.
- `## Summary` (or just the opening paragraph after the `Date:` line, no
  literal `## Summary` heading in most entries) — what kind of batch this
  was, verification numbers up front (unit status, integration N/N, e2e
  N/N).
- Numbered or thematic subsections for each feature/fix, often with an ADR
  reference in parens like `(ADR-020)` when the change was significant
  enough to also get an ADR in `architecture/DECISIONS.md`.
- `## Failures / gotchas (honest log)` — optional but common: things that
  went wrong during the work, root causes, and the lesson learned. Distinct
  from Verification.
- `## Verification` — concrete suite counts and what new checks were added,
  sometimes bug-specific reproduction notes.
- No literal "Type:" or "Requirements:" fields are used in this repo's
  entries — REQUIREMENTS.md items aren't cross-referenced by ID in the
  changelog here. Prose is dense/technical, written by/for the operator
  who did the work, not a generic template fill-in.

**Why:** the repo's own historical entries are the style precedent, and the
task instructions for this project say to follow "existing style in that
directory" over the generic subagent template.

**How to apply:** when writing changelog entries in this repo, mirror this
structure (title, Summary with verification numbers, thematic/numbered
sections, optional Failures/gotchas, Verification) instead of the generic
Summary/Type/Files changed/Verification/Requirements skeleton. Still fine to
add a "Files changed" list as an extra section if the task description
supplies concrete file names, since some newer entries do.
