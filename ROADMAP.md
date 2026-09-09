# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-09

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**v1.25.0 shipped 2026-09-09; ADR-067 is live in production.** The
per-project network mode is the bridge interface prefix (`sdb-` sandbox,
`lan-` LAN), chosen by the server; ADR-065/066's root-mediated machinery is
deleted. Tag `v1.25.0` on `6e73b43`, deployed by the auto-updater at 18:11,
firewall converged on the following tick, `iptables -S` unchanged. Gate:
`.agents/docs/reviews/2026-09-09-preflight-v1.25.0.md`. Plan
`.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md` is 43 of 44.

**Open, all the operator's:** the browser walk-through (the plan's one
unticked box); whether to disable sshd password auth before the first LAN
project (this host accepts passwords, and a LAN project reaches it); one run
of `sudo bash deploy/firewall/install.sh` from `/opt/styrman` at `v1.25.0`
to remove the retired, inert `.path` unit and spool mount.

**Next code task on this area** — top of `TODO.md`: replace the network-mode
PUT's record-first-then-rollback with persist-after-apply on tightening,
and compare against the live bridge name so a re-PUT is itself the
recovery. Three fail-opens (BUG-028/029/031) were found on that mechanism
in one day; AGENTS.md's rule after a third round is to delete it, not
defend it. Separate from that, and owed: the credential-broker reachability
test from a LAN project on the compose deployment
(`.agents/docs/reviews/2026-09-09-security-adr-067.md`) — a test, no code.

<details>
<summary>How it got there (written while ADR-067 was pending; kept as record)</summary>


**ADR-067 built and verified; release pending.** The per-project LAN switch is
now the bridge interface prefix, and ADR-065/066's root-mediated machinery is
deleted. Plan: `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md`
(its unticked boxes are the remaining work).

Phases 0-4 are committed. **Phase 5 is 3 of 4**: both suites were rewritten and
run green (integration 84/0/3, browser e2e 86/0/2), and the live adversarial
run passed all five steps on real hardware — `testing/2026-09-09-lan-mode-live.md`.
That run is the thing ADR-065 and ADR-066 never had: neither was ever once
applied on a real host. Its one open box is the operator's own browser
walk-through, which Playwright cannot stand in for because its host has no real
LAN to prove reachability against.

**Phase 6 is in progress.** Reviewed (approve-with-nits; all three findings
fixed), docs updated, version decided as **v1.25.0** (MINOR — an upgrading host
needs no action for the feature to work, since the mode is entirely
server-side). Still open: the security review's verdict, a final verification
pass over the fixed tree, the release changelog entry, and `/preflight`.
**Nothing is tagged or deployed yet** — production remains on v1.24.1.

Carried out of the live run and NOT resolved: a LAN-mode project reaches the
host at its address on OTHER projects' bridge interfaces. The other project's
container stays unreachable, so R12's cross-project clause holds, but the
surface is wider than ADR-067's wording, and the credential broker identifies
callers by source IP. It is the security review's primary question.

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

</details>

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
