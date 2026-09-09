# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-08

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**ADR-067 in flight — the per-project LAN switch is being rebuilt as the bridge
interface prefix, and ADR-065/066's root-mediated machinery deleted.** Plan:
`.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md` (its unticked
boxes are the remaining work). Phase 0 (R12 amendment + ADR-067) is committed;
Phases 1-6 are code, deletion, firewall shrink, UI, live verification, release.

The operator's correction on 2026-09-09: *"I just wanted you to give the option
to enable local lan access as normal docker container have. Not copying a lot
of keys and setting up scripts etc."* What ADR-065/066 shipped instead was
~3,500 lines, 13 of the project's 20 bug records, and a feature that **was
never once applied on real hardware** — `iptables -S` on 2026-09-09 showed zero
`styrman-lan*` rules with the v4 script installed and its status file current.
It was blocked behind an sshd-hardening precondition that needed the operator's
own client key.

The replacement is one fact: every styrman rule keys on the bridge INTERFACE
name (`-i sdb-+`), and that name is chosen by the server at `docker network
create`. Sandbox keeps `sdb-<hash>` and is matched by the drops; LAN gets
`lan-<hash>` and is matched by nothing, making it exactly a normal Docker
container. Cross-project isolation is unaffected because it is **Docker's**,
not styrman's — `-A DOCKER ! -i <br> -o <br> -j DROP`, one per bridge. Both
facts were measured on this host before the design was accepted
(`testing/2026-09-09-lan-prefix-probe.md`), including the NET_ADMIN
manual-route attack R12 names. Net: ~2,600 lines deleted, ~150 added, and no
root involvement in the feature at all.

Operator decisions recorded in ADR-067: a toggle may rebuild the project
container ("Yes — restart is fine"), and the permanent SSH-key withholding
latch is deleted ("Delete it").

**Shipped before this:** v1.24.1 (the whole-project audit's fixes — a
cross-tenant DoS from six missing `O_NONBLOCK`s, the ingress Caddyfile email
injection, a path traversal, the firewall prefilter trap, `docker/docker`
convergence); v1.24.0 (ADR-066 full same-network parity, now being superseded);
v1.23.0 (ADR-065, likewise); v1.22.0 (git side panel, file-explorer
navigation, terminal/editor fonts); v1.21.0 (ADR-064, un-nested project homes).

Carried forward, unstarted: the operator's picks from `TODO.md` — the legacy
`migrateWorkspaces` path, the `git.go:73` confined read, the upstream
`sh-agent` items, the ingress lows, and the incomplete v1.22.0 coverage tracing
(largest untraced surface is `SidePanelGit.tsx`).

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
