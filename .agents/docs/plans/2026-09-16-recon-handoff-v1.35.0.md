---
owner: explorer
date: 2026-09-16
---
# Recon — resume-protocol audit of the v1.35.0 handoff (2026-09-16)

## Question
Read AGENTS.md → TODO.md → newest handoff → ROADMAP "Now" → the named plan
→ three newest changes/ entries → bugs/INDEX.md as a fresh reader, and
answer six specific checks about consistency, missing files, and gaps.

## Summary
The trail is internally consistent: TODO/ROADMAP are empty, the harness-channel
plan's every box is ticked, and every file the handoff names (using its literal
glob patterns, e.g. `BUG-064-*.md`) resolves to a real file. No contradictions
found between handoff, ROADMAP, TODO and the plan. Four other plans have
unticked `- [ ]` boxes outside code fences and ROADMAP "Now" does not name any
of them (it names none — "Nothing in progress"); none is mentioned in the
2026-09-16 handoff, so whether they are abandoned, done-but-untidied, or
awaiting pickup is a genuine gap for a fresh reader. Every commit since
fdbc6ce that touched non-`.agents/docs` paths is covered by the six
2026-09-16 changes/ entries, principally the omnibus `2026-09-16-v1.35.0-release.md`
which explicitly lists the review/security/gate-round commits (cafd632,
5ba2b1b, 2f86816, e3aef79, edda5cd, 19d882c) that have no standalone entry.

## Answers

**(1) Status / next step as the trail states them.** Status: v1.35.0 shipped
and live on host `dave`; channel not yet delivering (no clone, no signed
tag). Single next step: operator action #1 in the handoff — mint a
fine-grained GitHub token for `styrman-harness` and run
`sudo styrman-ctl update --setup --harness-token-file <file>` on the host,
then verify with `styrman-ctl status` and
`systemctl is-enabled styrman-harness-update.timer`. (Step 2, cutting the
first signed tag, is the operator's separate follow-on; steps 3–4 are
informational, not actionable now.)

**(2) Files the handoff names that don't exist.** None. Checked every path
literally, including the two glob patterns (`reviews/2026-09-16-review-*`,
`reviews/2026-09-16-security-*`, `testing/2026-09-16-verify-harness-channel-phase-{1,2,3}.md`,
`testing/2026-09-16-verify-v1.35.0-release{,-round-2,-round-3,-round-4}.md`,
`bugs/BUG-064-*.md`, `bugs/BUG-065-*.md`) — all resolve. The bug files'
literal basenames (`BUG-064-harness-units-not-installed-every-tick.md`,
`BUG-065-vendored-harness-checksum-drift.md`) differ from the shorthand the
handoff body uses ("BUG-064", "BUG-065") but the Artifacts-to-Read glob
`BUG-064-*.md` / `BUG-065-*.md` matches them fine — not a defect.

**(3) Contradictions among handoff / ROADMAP / TODO / plan.** None found.
ROADMAP "Now" says the plan is closed and empty past Now, matching the
handoff's "ROADMAP.md 'Now' cleared, TODO.md empty" and the plan's own
Phase-4 checkpoint box. The plan's Phase-4 deploy box explicitly reconciles
its own PASS-condition ("enabled") with what actually happened
("disabled", pending the operator's clone) rather than silently PASSing —
consistent with the handoff's Next Steps framing.

**(4) Plans with unticked `- [ ]` boxes outside fences, not named by
ROADMAP "Now".** ROADMAP "Now" names nothing (empty), so by construction
every plan with open boxes is "not named". The awk command found:
- `.agents/docs/plans/2026-09-03-harness-migration.md`
- `.agents/docs/plans/2026-09-08-lan-full-parity.md`
- `.agents/docs/plans/2026-09-11-browser-view.md`
- `.agents/docs/plans/2026-09-11-installer-ctl.md`
None of these four is mentioned anywhere in the 2026-09-16 handoff or in
the three newest changes/ entries checked (harness-channel phase-3,
bug-064, v1.35.0-release). Not examined further: whether these plans are
superseded/closed-by-other-means (their own later checkpoint sections
weren't read) or genuinely stale open work — a fresh reader would have to
open each file to tell.

**(5) Commits fdbc6ce..HEAD touching non-`.agents/docs` paths without a
covering changes/ entry.** None found uncovered. Every commit in the range
(567b81f through 08d8fa9, 15 commits) touches source under
`internal/`, `deploy/`, `image/`, `README.md`, `AGENTS.md`, or
`REQUIREMENTS.md`/`ROADMAP.md`, and all are named explicitly by one of:
`2026-09-16-harness-channel-{approval,phase-1,phase-2,phase-3}.md`,
`2026-09-16-bug-064-harness-units-every-tick.md`, or the omnibus
`2026-09-16-v1.35.0-release.md`, whose "Commits in the range" section
lists the otherwise-unentered review/security/gate-round commits
(cafd632, 5ba2b1b, 2f86816, e3aef79, edda5cd, 19d882c) by hash.

**(6) What a fresh reader would still have to guess.** (a) The status/fate
of the four other open plans found in (4) — nothing in the trail says
whether they're queued, abandoned, or already superseded. (b) Whether the
operator has since performed Next Steps #1–2 (the token mint / clone /
first signed tag) — this snapshot predates that by construction and the
handoff is explicit it hasn't happened yet, but a reader picking this up
today (2026-09-16, same date) would need to check `styrman-ctl status`
live rather than assume. (c) argolis-devbox's parallel harness-channel
consumption (Next Step #3) is asserted as independent/out of scope but not
verified from this repo.

## Not examined
- Contents/checkpoint sections of the four other open plans (item 4) beyond confirming their open boxes exist.
- Whether earlier (pre-2026-09-16) handoffs already flagged those four plans as done-elsewhere; only handoffs/changes dated 2026-09-16 were grepped per the dispatch's scope.
- Live host state (`styrman-ctl status` was not re-run; relied on the handoff's and testing log's recorded output).
