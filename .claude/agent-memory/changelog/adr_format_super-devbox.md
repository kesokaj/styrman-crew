---
name: adr-format-super-devbox
description: ADR conventions actually used in super-devbox's .agents/docs/architecture/DECISIONS.md — numbering, heading format, body structure
metadata:
  type: project
---

super-devbox's `architecture/DECISIONS.md` ADRs (through ADR-041,
2026-08-30) follow a consistent local style, slightly different per era but
converged by ADR-022+. Latest entry as of 2026-08-30 (setup-terminal
Ctrl+C/targeted-tool ADR) is **ADR-041**; confirm the next-free number by
reading the file's tail before appending — it has climbed within the same
day multiple times (038→039→040→041 all landed 2026-08-30).

**Update (2026-09-01):** now at **ADR-052** (repo renamed styrman-crew).
ADR-052 records a durable operating *convention* ("privileged tests never
touch a production-aliasing path") triggered by an operational incident
(`.agents/docs/changes/2026-09-01-incident-state-loss.md` — production
state directory `/var/lib/styrman` emptied and `/etc/styrman` DR-secrets
directory deleted during root-run updater tests). This is a useful
precedent distinct from [[audit_entries_pattern]]'s "no ADR by default":
an *incident* record, unlike an *audit* record, can warrant an ADR when
the dispatcher identifies a durable decision it should bind future work to
(here: the scratch-prefix testing rule) — judge each case on whether a
real forward-looking decision was made, not just findings logged. Always
re-confirm the tail number before appending; it keeps climbing same-day.

**Update (2026-09-01, later same day):** now at **ADR-054**. This dispatch
paired two changelog entries (`2026-09-01-incident-reboot-outage.md` — a
pure incident record, no code — and
`2026-09-01-redfix-hardening-and-versioning.md` — the actual remediation
+ review/security/verify gates) with two new ADRs in one commit:
ADR-053 (immutable versioned tags, a durable rule) and ADR-054 (ingress
reload self-heal, a durable decision the incident forced). Confirms the
ADR-052 precedent generalizes: an incident write-up and its remediation
changelog can each motivate their own ADR in the same batch, as long as
each ADR names a real forward-looking decision, not just a fix. Also
confirmed here: a same-day incident (reboot) that happens *during* an
in-flight remediation task is recorded as ITS OWN changelog entry
separate from the remediation, with each entry cross-referencing the
other by relative path — do not merge an incident into the feature/fix
changelog it interrupted, even though they land in the same commit.

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
- The repo became a real git repo at some point after 2026-08-30's security
  remediation (env now reports "Is directory a git repo: Yes", with real
  commit history e.g. `d181249 docs: handoff — v1.5.2...`) — this reverses
  the earlier note below that it had no `.git`. `git log`/`git show` may now
  be usable for corroboration when the harness's tool access includes Bash;
  don't assume it's unavailable without checking first. The historical note
  that follows (no `.git`, Read/Grep/Glob/Edit/Write-only harness) describes
  an earlier state of the project and is kept for context, not as current
  fact: do not expect `git log` to corroborate ADR dates/authorship in that
  regime; rely on reading the actual code + existing changelog entries in
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

**Amendments/reversals across different days/topics are NEW sequential
ADRs, never edits to the old one** (the file is append-only). Precedent:
ADR-032a ("amends ADR-032") and ADR-038 ("amends ADR-032a") are both
separate numbered entries whose `**Context:**` paragraph names the ADR
they revise and says what changed; the old ADR's own text is left
untouched (a task may ask to add a note "under" the old ADR — translate
that into a new trailing entry that references it, not an in-place edit).
Use the phrase "superseded on the `<specific point>`" when only part of an
old ADR is wrong (e.g. ADR-032a's broker-tier shape stayed valid; only its
agy-storage assumption was wrong), rather than declaring the whole prior
ADR dead when it isn't.

**Update (2026-09-01, later still): now at ADR-056.** Two more same-day
ADRs (ADR-055 "no backup layer of any kind", ADR-056 "updater auth is a
GitHub token, not an SSH key") for the operator's evening "no backup at
all" / "host should not have a key" directives — both genuinely NEW
decisions (different topic, same day as ADR-053/054), not same-day
corrections, so they got new sequential numbers per the default rule, not
appended in-place. Useful cross-check discovered here: REQUIREMENTS.md's
R13 had already been amended (by the same task's code-side work) to cite
"ADR-055" by number *before* this ADR existed — i.e. the changelog/ADR
author and the code-diff author agreed on the next-free number in advance.
Always still verify by reading the file's actual tail before appending
(the R13 citation could in principle have been wrong), but a forward
reference like this in REQUIREMENTS.md or a comment is a strong corroborating
signal, not just the file's last heading.

**Revision as of 2026-09-01 (this repo, now called styrman-crew — same
DECISIONS.md, ADR count has climbed to ADR-051+): the SAME-DAY-correction
case is the one exception to "new ADR" above, and it is now the observed
convention, not a one-off.** When a reviewer/preflight gate catches, on
the same calendar day an ADR was authored, that the ADR's own Decision
text asserted something that didn't actually ship or didn't actually hold
(e.g. ADR-050's `/api/version` health check was abandoned before shipping
in favor of a docker-label read; ADR-051's `git verify-tag` was later found
not to pin the signer as claimed), the fix is a `**Correction (same-day
review remediation)**` or `**Correction (2026-09-01 preflight
remediation, commit <hash>)**` paragraph **appended inside the existing
ADR entry**, after Consequences and before the trailing `Source:` line —
not a new sequential ADR. Both ADR-050 and ADR-051 in this file carry such
a block. Rationale (inferred from the pattern, not stated anywhere): a
same-day correction is still describing the *original* decision's true
final shape, not a new decision made later — a new ADR number would be
noise for something that never actually shipped in the wrong form. Once a
day boundary is crossed, or the topic diverges enough to be its own
decision, go back to the "new sequential ADR" rule above.
