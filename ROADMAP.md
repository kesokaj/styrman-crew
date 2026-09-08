# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-08

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**Unreleased security fixes are sitting on `main`.** Production runs v1.24.0
(`7cbced7`); eight commits since then include the whole-project audit and its
fixes. The one that matters: a **cross-tenant DoS** — six opens on
guest-writable trees lacked the `O_NONBLOCK` that `internal/files` sets
elsewhere for exactly this reason, so any project owner could plant a FIFO in
their own home and pin a goroutine *and* an OS thread per request, walking the
runtime into its 10k-thread limit and taking every tenant down with the
process. Fixed in `271a7e8`, **not deployed**. Also unreleased: the ingress
Caddyfile email injection, the updater's unasserted worktree root, the
`docker/docker` convergence, and a path traversal.

None is remotely exploitable — all need an authenticated project owner — but
they are fixes to live isolation code that production does not have. Cutting
v1.25.0 is a short job: the tree is clean, `verify-commit.sh` is green on
`80765ca`, and the release path is the documented one.

**Whole-project audit, 2026-09-08** — `reviews/2026-09-08-audit-project.md`.
No high findings; no path found that escapes a project to another tenant or
the host. 11 medium, 11 low. Ten fixed with mutation-checked tests, one
identified as a false finding, nine recorded as not-now with reasons
(refactors and design decisions, not fixes). The recurring shape: a guard
correct where it was written and absent where it was not — the same pattern
the ADR-066 gate hit sixteen times, found in code that predates it.

**v1.24.0 shipped and is live** — tag on `7cbced7`, SSH-signed, verified
against the host's allowed-signers file before pushing, and deployed
unattended by the tag-gated updater. `styrman-server` and `styrman-caddy` on
`1.24.0`, server healthy; `host_converge` installed the v4 firewall script
from the tagged object with no human root command.

Ships ADR-066 — **full same-network parity** for the per-project LAN switch:
a LAN-enabled project reaches everything a peer on the operator's LAN
reaches, host and host-published Docker ports included, each behind its own
root-owned conf opt-in that **defaults closed**. NAT-only outbound; LAN hosts
still cannot initiate in. In exchange, a project that has ever had LAN access
is permanently no longer served the operator's SSH key (the latch amendment).
Three rule families share one generic reconcile; the published-port allowance
matches only DNATed traffic against a positive bridge allowlist.

**Deploying it changed nothing on this host** — the conf has neither opt-in
key, so `STYRMAN-IN` is still just `DROP` and `STYRMAN-FWD` still has zero
allowance rules. That was the point.

Release gate: `reviews/2026-09-08-preflight-v1.24.0.md`. Sixteen findings
across three rounds, all closed — eleven fixed, one deleted by the latch
amendment, four subsumed. Worth carrying forward: in each of the three
rounds, the previous round's fixes introduced the next round's worst defect,
and the two best outcomes were both deletions rather than repairs. The
mechanical gate (`verify-commit.sh` and every suite) was green before all
three rounds and caught none of the sixteen.

**Open, and both needing an operator action:** no LAN allowance has ever been
applied by the v4 script on real hardware, and the published-port path has
never been exercised live. Both are blocked behind the opt-in, and
`--lan-allow-host 1` correctly refuses on this host until sshd stops
accepting password authentication (R12, enforced in `install.sh`). The
five-step hardening sequence is in `deploy/remote.md`.

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
