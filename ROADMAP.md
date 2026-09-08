# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-08

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**v1.23.0 shipped and is live** — tag `v1.23.0` on commit `7bf0607`,
SSH-signed and verified against the host's allowed-signers file; the
tag-gated auto-updater deployed it in 160 s (`styrman-server` and
`styrman-caddy` both on `1.23.0`, server healthy,
`deployed-version=v1.23.0`). Release gate:
`.agents/docs/reviews/2026-09-08-preflight-v1.23.0.md` (VERDICT SHIP; 0
high, 2 medium, 6 low — five fixed in the release commit, two rebutted in
writing, one accepted as pre-existing generated build output). Ships
ADR-065, per-project local-network access: an admin-only, per-project,
runtime-switchable Ports-tab switch letting one project's microVM reach the
operator's LAN, off by default, applied by the root-owned firewall in about
0.2 s; R12 amended with the operator's approval. Six changelog entries:
`changes/2026-09-08-lan-access-phase{0,1,2,3,4}-*.md` plus
`2026-09-08-v1.23.0-release.md`, and the deploy record
`2026-09-08-v1.23.0-deployed.md`. The delivery gap ADR-065 part (6) exists
to close was demonstrated closed on its first release, unattended: the
updater's new `host_converge` installed the firewall script and its three
units from the verified object on the tick after the deploy, which also
converged pre-existing production drift (the live script had carried four
private-range DROPs since 2026-08-30 where the repo has had six since
2026-09-02) — `STYRMAN-FWD` now shows six, `/run/styrman-firewall/status`
reports `version=3`, timer and path units active, no root command from a
human needed. One operator step remains, and it is the only thing between
here and a usable feature: `sudo bash /opt/styrman/deploy/firewall/install.sh`
once on the host, which writes `/etc/styrman/firewall.conf` (the LAN
definition, discovered from the default route) and mounts the bounded spool
tmpfs — until it runs the feature is deployed but INERT and fails closed
(status reports no prefix configured, Settings shows the row red, every
enable refused with 503). After it, the last plan box is the manual
applied-state check on production (toggle on, in-guest ping to the gateway
succeeds while the host stays unreachable, toggle off cuts it), or the
opt-in `STYRMAN_IT_LIVE_LAN=1` integration block, logged into
`.agents/docs/testing/`. The plan
`.agents/docs/plans/2026-09-08-per-project-lan-access.md` has 42 of 44 boxes
ticked; the two open ones are that operator step and the check that follows
it.

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
