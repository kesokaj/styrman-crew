# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-08

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**Active plan: `.agents/docs/plans/2026-09-08-per-project-lan-access.md`** —
a per-project, admin-only, runtime-switchable local-network (LAN) allowance
for a project's microVM. DRAFT as of 2026-09-08: awaiting the operator's
decisions D1–D7 (the R12 amendment + ADR-065 must be approved before any
code) and the cold-reader check. Phase 0 also converges a pre-existing
production drift found during research: the live firewall script has four
DROPs where the repo's has six (100.64/10 and 198.18/15 missing since
2026-09-02).

**v1.22.0 shipped 2026-09-04** (`.agents/docs/changes/`, six phase entries plus the release entry; gate: `.agents/docs/reviews/2026-09-04-preflight-v1.22.0.md`): `GOPATH=$HOME/.go` in the seeded skel (NEW project homes only — existing homes keep `$HOME/go`), terminal/editor font settings, file-explorer back/forward/up with a readable crumb bar, the Recent strip as a collapsible resizable grid, git diffs in the main pane plus a UUID fast-commit, and six new git verbs (fetch, pull, branches, log, stash). Before it, v1.21.0 (ADR-064, the un-nested project homes).

Follow-up after the release: a `guestRuntime` seam (`Status` + `Exec`) now sits in front of `internal/httpapi`'s 15 call sites, with 26 handler tests in `guestexec_test.go` — closes two of the three missing-test gaps the gate recorded (the six git HTTP handlers, and the older `checkFile`/`formatFile` gap), both ticked in `TODO.md`. The third is still open: BUG-003's residual — the symlink guard on the Docker bind source that keeps one tenant's home out of another's mount — needs a fake Docker client, a different seam, since `o.docker` is a concrete `*client.Client` reached through 19 methods and `EnsureRunning` alone is 166 lines. `TODO.md` carries it at that size.

Carried forward, unstarted: the operator's picks from `TODO.md` — the legacy `migrateWorkspaces` path, the `git.go:73` confined read, the upstream `sh-agent` items, the ingress lows, and the incomplete coverage tracing (largest untraced surface is `SidePanelGit.tsx`).

## Next

Closed 2026-09-03 (ADR-062): `integration.sh` now asserts the seeded harness
inside a booted project (`sh-agent status` no drift, the two-line
`CLAUDE.md`, 28 skills / 7 agents, no previous-harness artifacts) and bounds
boot→ready by R1's 15 s hard limit from the container's own timestamps — the
"no automated assertion that a seeded agent harness is correct" item and its
boot-latency gap. Tier 5 in `.agents/docs/testing/styrman/README.md` keeps
only what a script cannot judge.

## Later / Ideas

<!-- Deliberately empty — awaiting operator input. Add items here as they arise, don't invent them. -->
