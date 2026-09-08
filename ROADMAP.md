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
human needed. The remaining operator step has been run: on 2026-09-08, from `/opt/styrman`
at the v1.23.0 tag, after a dry run, `sudo bash
/opt/styrman/deploy/firewall/install.sh` wrote `/etc/styrman/firewall.conf`
with the LAN definition discovered from the default route
(`LAN_DEV=enp114s0`, `LAN_CIDRS=192.168.86.0/24`) and mounted the bounded
1 MiB spool tmpfs; the script and units were already in place from
`host_converge`, so those steps were no-ops on identical bytes. Two things
that could only be proven on the real host are now proven: (1) the spool
tmpfs, mounted minutes after the server container had started, appears in
that container's own mount table correctly owned and bounded with no
restart — the `rslave` propagation fix (BUG-016) working as designed; (2)
end-to-end reachability — with `STYRMAN_IT_LIVE_LAN=1` the integration suite
ran 83 passed / 0 failed / 3 skipped, showing in one run that an unmarked
project cannot ping the LAN gateway, a marked one can within 5 s, the host
itself stays unreachable from the guest either way, and removing the marker
cuts it again within 5 s. The host was left clean afterward: no markers, no
`styrman-lan:` rules, spool drained, only the two production containers
running. The plan `.agents/docs/plans/2026-09-08-per-project-lan-access.md`
is CLOSED at 44 of 44 boxes. Records:
`changes/2026-09-08-lan-access-host-installed-and-proven.md` and
`testing/2026-09-08-verify-lan-live.md`; commit `3d02384`. The only thing deliberately left unautomated is a human
flipping the switch in a browser once; every layer beneath it is covered by
tests. One flaw was recorded rather than fixed and is in `TODO.md`: the
opt-in live LAN block races the production server's 2-minute marker sweep
(it plants a marker for a project only the test server knows about, and
production correctly deletes markers it cannot account for) — it passed by
timing, not by design.

There is no active plan right now — v1.23.0 is shipped and fully verified,
and `Later / Ideas` is still deliberately empty awaiting operator input.

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
