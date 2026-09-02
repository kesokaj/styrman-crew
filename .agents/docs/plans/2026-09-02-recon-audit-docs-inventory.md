# Recon — audit-docs-inventory   (2026-09-02)

## Question
Survey all documentation and supporting files (not application source) under
`.agents/docs/**`, root docs, and the listed supporting scripts/configs, and
answer: (1) is structure uniform (changes/, specs/, ADRs, reviews/, testing/
naming)? (2) what's orphaned/dead/broken-linked? (3) what coverage gaps
exist? (4) what facts are duplicated and can drift? (5) what's oversized or
malformed? Context: today shipped v1.18.0 (FAILED to build, auto-rolled-back)
then v1.18.1 (live); `specs/styrman/{api,guide,runtime-image}.md` and
`testing/styrman/README.md` were edited today by a documentation agent that
self-corrected twice — treat as higher risk of inaccuracy.

## Summary
The doc tree is large (171+ files) and mostly well-disciplined — ADRs are
genuinely gap-free and duplicate-free at the numeric level (001–058, one
lettered exception ADR-032a), and stale-feature references (removed backups,
removed keyring tier) are consistently marked historical, not orphaned. But
**the currently-live release, v1.18.1, has zero documentation anywhere** —
no changelog, review, security review, testing log, or ADR — while the one
changelog entry that exists (`v1.18.0-project-create-provisioning.md`)
describes a version that never actually shipped (it failed to build; the
auto-updater caught what the session's own verification gates missed). The
four "higher-risk" docs edited today are already stale against the v1.18.1
fix (`bootFailed`) they should describe, and `runtime-image.md` has a
confirmed broken path (`image/skel/.npmrc` should be `image/skel/npmrc`, no
dot). The release-version-derivation snippet is duplicated in three places
(README.md, guide.md, versioning.md) and one copy (guide.md) uses a
different git command that would silently violate the project's own
no-mutable-tags rule if followed literally. README.md's version banner
(v1.14.1) is ~9 releases stale. A 2026-09-01 review flagged
`deploy/caddy/VENDORED.md`'s "vendored verbatim" claim as false; it is still
unfixed today.

## Relevant files
- `.agents/docs/architecture/DECISIONS.md` (2,307 lines / 133 KB) — single
  append-only ADR log, ADR-001..058 + ADR-032a; the one file uniformity and
  size both hinge on.
- `.agents/docs/changes/` — 91 files (not ~80; user's estimate was low),
  one per shipped change/incident, inconsistent header shape.
- `.agents/docs/reviews/` — 34 files; review/security/audit/preflight
  reports, several outliers in naming.
- `.agents/docs/testing/` — 23 files directly under it + `spikes/` (1) +
  `styrman/` (2: `README.md`, `integration.sh`) — verify logs plus the two
  living test-guide/script files.
- `.agents/docs/specs/styrman/` — 6 files (`api.md`, `deploy.md`, `guide.md`,
  `runtime-image.md`, `terminal-protocol.md`, `versioning.md`) — the one
  app's low-level spec, per the constitution's rule.
- `.agents/docs/handoffs/` — 6 files, `.agents/docs/plans/` — 7 files
  (including this one).
- Root docs: `AGENTS.md`, `README.md`, `REQUIREMENTS.md`, `ROADMAP.md`,
  `TODO.md` (17.5 KB / 264 lines — has drifted far from "small deferred
  items" back into a de facto findings tracker despite the 2026-08-30
  "start fresh, empty by design" directive still stated at its top).
- `deploy/README.md`, `deploy/caddy/VENDORED.md` (dispatch said
  `deploy/VENDORED.md` — that path does not exist; the real file is one
  level deeper), `image/README.md`, `image/harness/VENDORED.md` (a second,
  differently-scoped file with the same basename), `web/README.md`.
- `.claude/agent-memory/` — mid-deletion (see Orphans); `.claude/` and
  `.agents/` both currently hold no `skills|commands|rules|workflows`, so
  the constitution's mirroring requirement has nothing to mirror.

## Flows — how the doc set is meant to connect
- Release flow (as documented): commit → sign+push tag → build all three
  images at that tag's `X.Y.Z` → `docker compose up` at the same version →
  verify → **owed DoD rows** (changelog/review/security/testing) land in
  `.agents/docs/`. `versioning.md`'s "Manual release procedure" is the
  canonical copy of this; README.md's Quick start and guide.md §3 each carry
  their own copy of the version-derivation + build/compose lines (see
  Duplication below) rather than linking out.
- ADR flow: a decision gets a new `## ADR-NNN:` section appended at the file
  end; later ADRs cite earlier ones by number (`ADR-030/032`, "amends
  ADR-032, which shipped agy as..."); `changes/` entries and `reviews/`
  cross-link back to the ADR they justify (e.g. ADR-058 lists its own
  `Verification:` block of changes/reviews/testing paths — the tightest
  cross-linking in the tree, and a shape other ADRs mostly don't follow).
- Test-count flow: `integration.sh` and `web/e2e/run.mjs` are the ground
  truth; counts get transcribed by hand into
  `.agents/docs/testing/styrman/README.md`, which `AGENTS.md`, `README.md`,
  and `guide.md` then all cite *by reference* (good — they don't restate the
  number) **except** `AGENTS.md` §Testing, which restates a stale number
  inline (see Duplication).
- Today's incident flow, as it actually happened (reconstructed from git,
  not from any doc): b321f35 (provisioning-order fix, tagged v1.18.0) was
  verified against the **working tree** and passed
  (`testing/2026-09-02-verify-v1.18.0-final.md`, "FINAL STATUS (Run 4):
  PASS"), but the **committed tree** was missing `bootFailed`'s definition
  (`provision.go` wasn't fully staged) — this only surfaced when the
  auto-updater built a fresh checkout of the pushed tag, which failed and
  auto-rolled-back to v1.17.3 per its own designed behavior (ADR-050/051).
  cb1c18c then added the missing code and is tagged v1.18.1 (confirmed
  live tag). **No doc in the tree records this incident, the rollback, or
  that v1.18.1 is what's actually live** — not even TODO.md, whose Inbox is
  otherwise a live incident/finding log.

## Conventions actually in use
- ADR header: two competing shapes coexist, split at a specific boundary —
  ADR-001..022 and ADR-027..033 use `## ADR-NNN: Title` + a separate
  `**Date:** YYYY-MM-DD · **Status:** accepted (...)` line; ADR-023..026 and
  ADR-034..058 use `## ADR-NNN: Title (YYYY-MM-DD)` with the date inline in
  the header and **no Status field at all**. No note in the file explains
  the switch or which shape is canonical going forward.
- `changes/` filename: `YYYY-MM-DD-<slug>.md`, one word per topic, version
  suffix when release-shaped (`-v1.17.3-...`). Uniform. First-line header is
  NOT uniform: some are `# YYYY-MM-DD — Title` (date folded into the H1,
  e.g. `2026-08-26-phase0-scaffold-and-spikes.md`), most are `# Title` with
  a `Date: YYYY-MM-DD` line a few lines down.
- `reviews/` filename: `YYYY-MM-DD-<kind>-<slug>.md` where kind is usually
  `review-`/`security-`/`audit-`/`preflight-`. Four outliers don't carry a
  kind prefix: `2026-08-27-1.1-review.md` (version-first, "review" as a
  *suffix* — the only file shaped this way), `full-project-review.md`,
  `pre-1.0.0-review.md`, `remediation-plan.md`.
- `testing/` filename: `YYYY-MM-DD-verify-<slug>.md`, 21 of 22 top-level
  files. The one outlier: `2026-09-01-audit-verify.md` (kind-word order
  reversed relative to the rest of the set).
- Severity markers in `TODO.md`: 🟡 / 🟢 prefixes on Inbox bullets (no 🔴
  instances currently) — used consistently but never defined inline; a
  reader has to already know the `review-standards` skill's scale.
- Historical-reference convention (good, worth naming as a pattern to keep):
  every removed-feature mention checked (backups/restic/GCS, the credential
  broker's keyring tier) is phrased as "X was removed (ADR-NNN, date) —
  reason" rather than left as a dangling prescriptive statement. This is
  the one place uniformity is excellent and should be the model for fixing
  the v1.18.0/v1.18.1 gap.

## Risks
1. **Live version is completely undocumented.** v1.18.1 (current tag, per
   `git tag`) has no `changes/`, `reviews/`, `testing/`, or ADR entry. The
   only entry for "v1.18.0" (`.agents/docs/changes/2026-09-02-v1.18.0-project-create-provisioning.md`)
   describes a build that failed and was rolled back — a reader trusting
   this file believes the wrong version is live and that it works.
2. **A PASS verdict survives a build that didn't build.**
   `testing/2026-09-02-verify-v1.18.0-final.md`'s "FINAL STATUS (Run 4):
   PASS" is now known-wrong for the actual tagged artifact (verified
   working-tree only, not the committed/pushed tree) but carries no
   retraction, despite the file's own stated append-only-history
   convention ("kept verbatim as history... superseded, not deleted") —
   there is nothing superseding it.
3. **The four docs edited today to describe this exact feature are already
   stale against it.** `api.md` and `guide.md` fully describe
   `provisionRepo`/`provisionSpec`/`waitForBroker` but never mention
   `bootFailed` — the function cb1c18c added specifically to fix a silent
   failure mode (GitHub repo creation failing quietly when the microVM
   itself never boots). The exact defect class the docs were just written
   to explain (silent credentialed-path failure) now has a second instance
   the docs don't cover.
4. **Confirmed broken path in a today-edited doc.** `runtime-image.md`'s
   credential table says `image/skel/.npmrc` "references `${NPM_TOKEN}`" —
   the real file is `image/skel/npmrc` (no leading dot; `entrypoint.sh`
   copies it to `~/.npmrc` at deploy time, adding the dot only in-guest).
   `ls image/skel/.npmrc` fails; `ls image/skel/npmrc` succeeds.
5. **Test-count duplication has already drifted.** `AGENTS.md` (edited
   today, commit `503fc8a`) states the e2e suite as "currently 54 passed +
   2 auto-SKIP" and calls `testing/styrman/README.md` "the source of
   truth" — but that same source-of-truth file (edited later today, in
   `b321f35`) now says "60 pass...plus 2 auto-SKIP" / "61 checks + a final
   client-error gate". The two docs disagree on the fact one of them
   explicitly delegates to the other.
6. **Release-procedure duplication, and one copy is actively wrong.** The
   `STYRMAN_VERSION=... && docker build ... && docker compose up` sequence
   is written out three times (README.md Quick start, `guide.md` §3, and
   `versioning.md`'s canonical "Manual release procedure"). README.md
   derives the version with `git describe --tags --abbrev=0 | sed
   's/^v//'` (clean `X.Y.Z`, matches the no-mutable-tags rule the same
   README explains inline). `guide.md` instead uses `git describe --tags
   --always`, which does **not** strip the `v` prefix and, on a HEAD not
   exactly on a tag, appends a `-N-gSHA` suffix — precisely the
   "SHA-suffixed value would land the build in a different tag namespace
   than the auto-updater's" failure mode README.md warns against, in the
   same document set. Anyone following `guide.md`'s install-from-scratch
   block literally on a non-tag HEAD gets a build that violates ADR-053.
7. **A known-flagged doc inaccuracy is still unfixed a day later.**
   `deploy/caddy/VENDORED.md` still opens "vendored **verbatim** from
   `~/workspace/devbox/caddy`... Upstream fixes should be copied here;
   styrman-specific changes belong in this copy." The 2026-09-01
   `.agents/docs/reviews/2026-09-01-audit-reviewer.md` (line 141) already
   flagged this: `git log -- deploy/caddy/ingress` shows 7 styrman-specific
   commits — "It is a fork, not a mirror." No commit since has touched
   `deploy/caddy/VENDORED.md`.
8. **README.md's version banner is ~9 releases stale.** "**v1.14.1**,
   deployed" (line 94) — actual tags run through v1.18.1; last touch to
   README.md was commit `ad1a4be` (2026-09-01), which edited the backups
   paragraph in that same Status section but left the version number
   untouched.
9. **ADR format split is silent and permanent-looking.** Two structurally
   different header/field shapes for ADR entries, split roughly at
   ADR-022/023 and ADR-026/027 and then permanently at ADR-033/034, with no
   note anywhere in the file about which is canonical. Not a functional
   defect (both are readable) but a genuine "pick one" cleanup opportunity
   whoever authors ADR-059 will hit.
10. **`TODO.md` no longer matches its own stated purpose.** The file's own
    header still frames it as "empty by design... add new items as they
    arise" (2026-08-30 directive), but by today it is a 264-line mix of
    live audit findings (with severity emoji), shipped-item checkmarks, and
    operator-decision trackers — effectively a second reviews/ folder
    living in a root file the constitution designates for "small deferred
    items." Two of its own path references
    (`reviews/2026-09-01-review-editor-ux.md` at line 175, and
    `guide.md`'s `testing/styrman/README.md` at line 229) drop the
    `.agents/docs/` prefix every other reference in the same files carries
    — they still resolve by inference but not as literally written.

## Open questions
- Was v1.18.1 verified/reviewed at all outside this session's own commit
  message, or does `cb1c18c`'s terse fix (23 + 54 lines, tests included)
  constitute the entire record? The commit itself is thorough (explains the
  root cause and adds `TestBootFailedReportsAndKeepsR13`), but per the
  constitution's Definition of Done, a changelog/testing-log entry for this
  commit is still owed and I found none — confirm whether one is about to
  be written before treating this as a gap to close versus one already in
  flight.
- Is `deploy/VENDORED.md` (the path the dispatch named) meant to exist as a
  future top-level file distinct from `deploy/caddy/VENDORED.md`, or was
  the dispatch's path shorthand for the real, one-level-deeper file? I
  treated it as the latter (only `deploy/caddy/VENDORED.md` and
  `image/harness/VENDORED.md` exist) but could not confirm intent from the
  docs alone.
- I did not open every one of the 91 `changes/` bodies or all 34 `reviews/`
  bodies for internal-date-vs-filename-date consistency — I spot-checked
  ~6 and found no mismatches; a full sweep of all 91 was out of budget for
  this pass and could still surface isolated cases.
- I could not find an explicit legend for TODO.md's 🟡/🟢 severity markers
  anywhere in-repo (root docs or `.agents/docs/`) — if one exists only
  inside a `review-standards` skill file outside the scope I was given
  (`.agents/skills/` or similar), it isn't cross-linked from TODO.md itself.

## Note
This file duplicates `.agents/docs/plans/2026-09-02-audit-docs-inventory.md`,
written earlier in this same run before the naming convention
(`recon-<slug>.md`) was confirmed by stop-hook feedback. Content is
identical; this is the canonically-named copy.
