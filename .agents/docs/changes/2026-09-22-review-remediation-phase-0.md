---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 0 — close the review's own blind spots   (2026-09-22)

Phase 0 of `plans/2026-09-22-project-review-remediation.md`, which acts on
`reviews/2026-09-22-review-project-merged.md` (the `/review --project --all
--high` wave at c05219c). The phase edits no source by design: it exists to
find out whether the two things that review did not check would reshape the
rest of the plan. They did.

## What this phase did

- **A security lens over `internal/files/`** — the package no lens in the
  repository-wide wave opened, and the one
  `.agents/rules/guest-writable-paths.md` was written for (BUG-002, 003, 004,
  009). Report: `reviews/2026-09-22-security-files.md`.
- **Rebuilt `deadcode`** against the go1.27.1 toolchain (the installed binary
  was go1.26 and errored out) and ran the unexported-dead-code sweep that the
  merged record listed as impossible.

## What it found

**The confinement boundary holds.** Both claims the rule file makes were
verified for the first time: `openRoot` reaches the root as
`os.OpenRoot(<base>/<tenant>)` + `Root.OpenRoot(rest)` (`files.go:126-135`),
with no `os.OpenRoot(path)`, `os.ReadDir` or `os.Stat` by name below a mounted
home; and no `DirEntry.Info()` survives in the package or in any caller
(`files.List`, `history.List`, `cmd/styrman`'s prune all stat through the
root). **No path escape was found.**

**Three findings, carried to Phase 0b of the plan.** Two are the BUG-035 shape
— a mechanism that exists in this very file and did not reach every caller,
which is why they survived four earlier audits of it:

- `files.go:191` — `List` does an uncapped `ReadDir(-1)` on a guest-owned
  directory, while `ListNames:489-492` caps at `maxListNames` carrying the
  reasoning in a comment: "the directory is guest-writable and this is polled
  — an unbounded ReadDir is a per-tenant lever on the shared server" (security
  review 2026-09-03). On top of it, `sortEntries:232-246` is a hand-rolled
  insertion sort, O(n²): the reviewer measured a copy at 5k entries 102 ms,
  20k 1.57 s, 50k 9.83 s single-core — in the one process serving every
  tenant. Verified by the main agent by reading both functions.
- `files.go:446` — `ReadRawNoFollow` opens the parent with `rt.Open`, the only
  open in the package without `O_NONBLOCK`; lines 154, 186, 296, 353, 454, 484
  and 518 all carry it, several with explicit guest-planted-FIFO comments, and
  454 is the next open in the same function. A guest replacing `~/.ssh` with a
  FIFO wedges `httpapi/setup.go:247`'s goroutine and its OS thread. Verified by
  the main agent with a grep of every open in the package.
- `internal/httpapi/explorer.go:99-111` — `managedHomeTarget` matches the first
  string component while `os.Root` follows in-root symlinks (the reviewer
  probed it: with `x -> .dockerd`, `Root.RemoveAll("x/disk.img")` succeeded).
  Same principal throughout, so not an escape; the defect is that the comment
  reads like a boundary.

Also recorded, not yet acted on: only 4 of 15 root functions have a test case
below a `files.Trust` base, and `rmcheck_test.go`'s cross-tenant wipe test runs
on the untrusted path — BUG-004's exact blind spot, inside the test written to
prevent it.

**Dead code: none in the root module.** `deadcode ./...` exits 0 with zero
unreachable functions, independently corroborating the simplify lens's hand
check of 21 low-reference candidates. That silence was validated before it was
believed: a scratch module containing a deliberately dead function reports it,
so the tool can emit (`.agents/rules/` § "silence from your own instrumentation
is not evidence"; BUG-034). The ingress module cannot be analysed this way —
`deadcode: no main packages`, it being one library package whose entry points
are Caddy registry callbacks registered in `init()`. A synthetic main would
report every handler as a false positive. Recorded as a tool limitation; that
module's coverage remains its own 53-test suite and the simplify lens's export
sweep.

## Effect on the plan

Phase 0 was written expecting to find nothing, and said so in advance ("a clean
result here is the point of the phase"). Because it found a per-tenant lever on
the shared server instead, its findings become **Phase 0b**, promoted ahead of
the docs phase. The plan's ordering premise — by what can still surprise us —
is what produced this, and it is recorded here as evidence for the premise
rather than as a change of mind.

## Agents dispatched

`security` ×1 (`reviews/2026-09-22-security-files.md`). No `changelog` agent:
this entry is the main agent's own, written from evidence it held and verified
itself (`.agents/rules/changelog-brief.md`). No source, test or config file was
edited in this phase; the only non-artifact change is `ROADMAP.md`'s "Now",
which now names the active plan.

## Verification

The reviewer ran `go test ./internal/files/` (exit 0) and `govulncheck ./...`
(2 called vulnerabilities, GO-2026-4887 and GO-2026-4883, both
`github.com/docker/docker@v28.5.2+incompatible`, both "Fixed in: N/A", both
already recorded and outside this package's blast radius). The main agent
re-verified the two sharpest findings directly against the source rather than
accepting the report: `sed -n '185,250p'` and `grep -n` over every open in
`internal/files/files.go`, both quoted above.

Not run in this phase, and not claimed: Tier 2, Tier 4, any container build,
`deploy/verify-commit.sh`. Those are Phase 5.
