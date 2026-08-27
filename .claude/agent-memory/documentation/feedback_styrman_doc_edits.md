---
name: feedback-styrman-doc-edits
description: Process lesson for editing styrman spec docs with the Edit tool
metadata:
  type: feedback
---

Always re-read the exact target lines with Read immediately before an Edit
in styrman's `.agents/docs/specs/styrman/*.md` files, even if you read the
whole file minutes earlier in the same turn.

**Why:** An Edit against `guide.md` failed because the old_string used
`{id}/{t}` (curly braces) copied from memory of similar patterns elsewhere
in the doc set, but the actual text in that file used `<id>/<t>` (angle
brackets). The file content didn't match what was assumed from a similar
line in `api.md` — different files in the same doc set are not always
consistent with each other, and don't assume a pattern is uniform across
files.

**How to apply:** Before every Edit call on these docs, do a fresh Read (or
Grep) of the exact old_string target and copy it verbatim rather than
reconstructing it from memory of a similar-looking line elsewhere in the
project.

See [[project-styrman-docs]] for the broader doc map.
