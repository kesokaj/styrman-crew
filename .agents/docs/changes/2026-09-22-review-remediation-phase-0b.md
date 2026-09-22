---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 0b — `internal/files` hardening   (2026-09-22)

Phase 0b of `plans/2026-09-22-project-review-remediation.md`, acting on the
three findings Phase 0's security review of `internal/files` produced
(`reviews/2026-09-22-security-files.md`). Two of them were the BUG-035 shape:
a mechanism that already existed in this very file and had not reached every
caller.

## What changed

- **`internal/files/files.go` — `List` is bounded.** It read the whole
  directory with `ReadDir(-1)` while `ListNames` twelve lines below capped at
  `maxListNames`, carrying the reason in its comment since the 2026-09-03
  security review: the directory is guest-writable and the explorer polls it.
  `List` now reads `maxListEntries + 1` (5000) and returns
  `*Listing{Entries, Truncated}` so a cut listing cannot read as a complete
  one. Single caller, `internal/httpapi/explorer.go`.
- **`sortEntries` is no longer quadratic.** It was a hand-rolled insertion
  sort over a guest-controlled input; it is now `slices.SortStableFunc` over a
  `compareEntries` comparator. Stable on purpose: names equal under case
  folding keep the order the filesystem gave them, which is what the insertion
  sort did.
- **`ReadRawNoFollow` opens its parent with `O_NONBLOCK`.** It was the only
  open in the package without it — lines 154, 186, 296, 353, 454, 484 and 518
  all had it, and 454 is the next open in the same function. A guest replacing
  `~/.ssh` with a FIFO wedged the serving goroutine and its OS thread in
  `open(2)`.
- **`internal/httpapi/explorer.go` — `managedHomeTarget` is now what its
  comment always claimed.** The four platform-managed names were matched as
  strings while `os.Root` follows in-root symlinks, so a link reached the
  subtree through an unmanaged name. `traversesALink` lstats every component
  of the path except the last, fails closed on any error it cannot attribute
  to a missing name, and leaves the last component alone — acting on a link
  itself removes only the link.
- **The explorer says when a listing was cut** (`web/src/lib/api.ts`,
  `web/src/components/FileExplorer.tsx`).

## Tests

+13 (311 → 324 pass, 0 fail, 1 skip; no test removed, checked with
`git diff -U0 | grep -E "^-func (Test|Benchmark)"`). New files
`internal/files/listbounds_test.go` and
`internal/httpapi/managedtarget_test.go`; a trusted-base case appended to
`rmcheck_test.go`.

That last one closes a blind spot the review named: `rmcheck_test.go`'s
cross-tenant wipe test ran on a root no `files.Trust` base covers — the
untrusted branch of `openRoot`, not the one the server takes. BUG-004's exact
blind spot, inside the test written to prevent a cross-tenant wipe. The same
attack is now asserted on both branches. The cap is likewise asserted below a
trusted base, not only on a plain root.

## How the fixes were proved, not assumed

Each mutation below compiled, and made a NAMED test fail; each file was
restored from a checksummed copy and verified byte-identical afterwards
(BUG-071 was a mutation restore that corrupted a line).

| Mutation | Named failure |
|---|---|
| `O_NONBLOCK` removed from the parent open | `TestReadRawNoFollowDoesNotWedgeOnAFifoParent`, 5.00 s timeout |
| cap and stable sort reverted to the old code | `TestListIsCappedAndReportsTruncation`, `TestListCapAppliesBelowATrustedBase` |
| symlink guard call removed | `TestManagedHomeTargetRefusesASymlinkedFirstComponent`, `TestManagedHomeTargetFailsClosedWhenItCannotLook` |

The cost the cap and the sort exist to bound, measured on the real `List`
rather than a copy, 20 000 entries, `-benchtime=3x`:
**5 881 226 101 ns/op → 13 434 963 ns/op**, a 438× reduction.

## What the reviews caught, and what it cost

`reviewer` and `security` were dispatched over the diff and **both
independently found the same bypass**: the first version of the guard
inspected only the path's FIRST component, so `work/sub/link ->
../../.dockerd` walked past it while `os.Root` followed the link. The fix
above is the second version. The test was written before it and failed against
the pre-fix code with the message it was written to produce.

Security found one more that this phase had created: the new cap made a
truncated listing render as complete, because nothing consumed `Truncated` —
and since truncation happens on raw directory order before the sort, what is
hidden is arbitrary. A concealment primitive introduced by a fix for a
denial-of-service lever. Fixed in the web tier in this same change.

An earlier version of the guard was also too broad and broke
`TestFileDeleteSymlinkEscape`: deleting a bad symlink itself must stay a 204,
since it removes only the link. Caught by a test that already existed. The
distinction now has its own assertions at both depths so it is not
re-broadened.

**Not covered:** the truncation notice has no unit test. `FileExplorer.tsx` is
one of the 33 `web/src` files with none, and the project has no
component-testing infrastructure; standing one up is a dependency decision,
not a side effect of a security fix. The assertion is a Phase 5 task against
Tier 4. `tsc -b`, `oxlint` and `vitest` passing is type-checking and lint, not
evidence that the notice renders.

The lstat-then-act race in the guard is accepted, not closed: both sides are
the same principal inside the same confined home, the tenancy boundary is
`os.Root`'s and is atomic, and closing it would mean threading one root handle
through every `files.*` verb. The trade-off is recorded in the function's own
doc comment, where a reader meets it.

## Agents dispatched

`reviewer` ×1 (`reviews/2026-09-22-review-phase0b-code.md`, REQUEST CHANGES),
`security` ×1 (`reviews/2026-09-22-security-phase0b.md`, FINDINGS). Each
report has an `## Outcome` appended by the main agent naming every finding as
fixed, rebutted or left. No `changelog` agent: this entry is the main agent's
own, from evidence it held (`.agents/rules/changelog-brief.md`). Corrections
needed to agent drafts: none — neither agent wrote prose that went into a
project file.

## Verification

Every command run bare as its own statement, status read before judging
(`.agents/rules/verify-bare.md`):

- `go vet ./...` → 0
- `go test ./...` → 0 (324 pass, 0 fail, 1 skip)
- `(cd deploy/caddy/ingress && go test ./...)` → 0
- `(cd web && npm run check)` → 0
- `(cd web && npm run lint)` → 0
- `(cd web && npm test)` → 0 (15 files, 178 tests)

Not run, and not claimed: Tier 2 (`integration.sh`), Tier 4 (`web/e2e/run.sh`),
`deploy/vulncheck.sh`, any container build. Those need the live host and are
Phase 5.

## Learnings for the local host

None: nothing in this phase touches `host-setup.sh`, `styrman-ctl`, the
updater or the image.
