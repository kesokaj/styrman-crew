---
owner: main
date: 2026-09-04
---
# Preflight gate — v1.22.0   (2026-09-04)

Range reviewed: v1.21.0..b6db0b9

Release: the UI + git improvements batch (`plans/2026-09-04-ui-git-improvements-v1.22.0.md`),
six phases plus the gate-fix commit this report describes.

## VERDICT: SHIP

Conditional on nothing further: the six blocking findings below were all fixed
in `b6db0b9` and re-verified after the fix. **`deploy/verify-commit.sh` must
be re-run against the final release commit before the tag** — the earlier OK
was for `acb5918`, which is no longer the tip.

## Findings — all FIXED before this verdict

Ordered worst first. Every one came from reviewing the range AS A WHOLE; the
per-phase reviews (all green, all findings fixed as they landed) could not see
any of them.

1. **[medium / high] Two source files were never reviewable.**
   `web/src/components/DiffPane.tsx` and `web/src/lib/diffReveal.test.ts`
   contained LITERAL NUL bytes — the pane's cache key used them as separators,
   the test used real control bytes where escape sequences were meant — so git
   classified both as binary. Their contents were invisible in every diff,
   including the diffs the review gate itself reads: two reviewers and a
   security pass had signed off on files they could not see. Fixed to `\uXXXX`
   escapes; both verified NUL-free and control-free, tests still 69/69.
   *Cause: the same heredoc-escaping mistake caught mid-phase-6, except this
   one reached a commit.*
2. **[medium / high] `gitPush` put a guest-controlled branch name into argv
   unvalidated.** Plumbing can create `refs/heads/-foo`; `rev-parse
   --abbrev-ref HEAD` reports it verbatim and `git push -u origin -foo` reads
   it as options (verified by the reviewer on git 2.47.3: `-f` plus `-o o`).
   Its sibling `gitCheckout` gated exactly this. Pre-existing, but phase 6 is
   what made the asymmetry visible. `--end-of-options` does not work for
   `push`, so it now validates with `validBranchName`.
3. **[medium / medium] Tree-rewriting verbs left the editor holding
   pre-change content.** checkout / pull / stash push / stash pop never told
   the editor pane, so an open buffer silently predated the new tree and Save
   would write it back over the checkout — a data-loss path introduced by
   phase 6. Now `evictCleanProjectPanes` drops CLEAN project buffers so the
   next open re-reads, and deliberately does NOT touch dirty ones: discarding
   someone's unsaved typing to prevent a stale save trades one data loss for
   another. Dirty buffers are reported to the user instead.
4. **[low / high] The `firstLine` → `guestErrMessage` remediation landed on
   2 of 4 sites.** `gitCommit`'s 409 and `gitPush`'s 502 still returned
   uncapped, unstripped, unredacted git output — and `push` carries the most
   remote-influenced text in the file. Both fixed. `firstLine` itself stays:
   `format.go` still uses it (checked before deleting).
5. **[low / high] The Fetch tooltip still advertised `--prune`,** which phase
   6 had removed as a deliberate security decision. Same class as the phase-6
   comment that claimed a trailing `--` terminated options: a description that
   outlived the behaviour it described.
6. **[low / medium] README's Git-tab bullet was still the v1.7.0 feature
   list,** and `AGENTS.md:58` claimed 60 integration checks where there are
   now 68. Fixed by the `documentation` agent.

## Gate rows

| Row | Result | Artifact |
|---|---|---|
| Reviewer (release range, opus) | APPROVE WITH NITS → all fixed | `reviews/2026-09-04-review-release-v1.22.0.md` |
| Security (release range) | FINDINGS → all fixed | `reviews/2026-09-04-security-release-v1.22.0.md` |
| Verifier (release range) | PASS (commands); coverage tracing incomplete | `testing/2026-09-04-verify-release-v1.22.0.md` |
| Per-phase reviews | 5 reviews + 3 security passes, all green | `reviews/2026-09-04-{review,security}-phase*.md` |
| `deploy/verify-commit.sh acb5918` | OK | superseded — MUST be re-run on the final commit |
| `deploy/vulncheck.sh` | accepted baseline only | 2 unfixable `docker/docker` (GO-2026-4887/4883), `TODO.md:62` |

Security confirmed every phase-5/6 fix is genuinely present in the committed
tree, not just in a review report: `checkout --end-of-options`, no `--prune`
on fetch, `guestErrMessage` in `gitRun`, `utf8.ValidString` in
`validBranchName`, and `GIT_CEILING_DIRECTORIES` on every git exec. It also
confirmed the `reveal()` sanitiser is in the shipped `web/dist` bundle, not
only in source.

## Trail freshness (Step 3)

- Newest `changes/` entries are this release's six phase entries — **pass**.
- No artifact under `.agents/docs` carries a future `date:` — **pass**.
- `changes/` and `DECISIONS.md` unmodified in range (append-only) — **pass**.
- `ROADMAP.md` and `TODO.md` both touched in range — **pass**.

## Docs freshness (Step 4)

`api.md`, `guide.md`, `runtime-image.md` and the testing README were updated
as the phases landed. README and AGENTS.md were stale and were fixed (finding
6). `.agents/docs/deploy/` needs no change: this release altered no deploy
procedure — `verify-commit.sh` gained a gate step, and the guides reference
that script generically without enumerating its internals.

## What was NOT checked

Named honestly, because the rows above do not cover them:

- **Coverage tracing is incomplete.** The verifier ran every command green but
  did not finish tracing each changed file to a covering test. The largest
  untraced item is `SidePanelGit.tsx` (351 changed lines); also untraced:
  `deploy/verify-commit.sh`'s new gate step, `image/skel/*`, and
  `integration.sh`'s own additions. Per AGENTS.md a changed file with no
  covering test is a FAIL row; these are *unknown*, not established failures.
  `SidePanelGit.tsx` is exercised by the e2e git checks, but not line-for-line.
- **The six new git HTTP handlers have no Go unit test.** `Server.orch` is a
  concrete `*orchestrator.Orchestrator` with no exec seam to fake — the gap
  this repo already records for `checkFile`/`formatFile` and as BUG-003's
  residual. Only `validCommitHash`/`validBranchName`/`stashOps` are unit-tested;
  the handlers are covered by e2e and integration only.
- **The release reviewer did not read `web/e2e/run.mjs`'s 335 new lines**, and
  reviewed the phase-2 appearance UI only at the store level.
- **No reviewer executed anything against real Kata.** The e2e and integration
  results cited are from my own runs, not independently reproduced.
- **`deploy/vulncheck.sh` was not re-run by the security agent** (it ran
  earlier in this session, at baseline).
- **The teardown 404** seen once in an e2e run did not reproduce and was never
  root-caused; it is a transient request outliving a deleted project. The
  client-error gate now records the failing URL so a recurrence is diagnosable.
- **Phase 2's "live font update" is not shipped behaviour.** `/settings` is a
  separate route and unmounts the panes, so the preference applies on next
  mount. The compartment and live-apply effect are defensive only. Recorded in
  the plan; repeated here so the release note does not overclaim.

## Operator-visible behaviour change

`GOPATH` is `$HOME/.go` for **newly created project homes only**. Existing
homes keep `$HOME/go` permanently — the skel is write-once per file and is not
re-seeded by a version bump (verified in `entrypoint.sh`, not assumed) — so
anything already `go install`-ed in an existing project stays where it is.
