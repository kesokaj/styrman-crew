# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: 2026-09-10

Empty by design past "Now" (operator directive 2026-08-30: "I want nothing
in todo and roadmap — start fresh with new things"). See `TODO.md` for the
matching empty-slate Inbox and **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) for how the previous roadmap was
closed out. History lives in `.agents/docs/changes/` and git.

## Now

**v1.28.0 shipped 2026-09-11** (MINOR) — the Browser view (ADR-068: one
headless Chromium per project microVM, shown in a fourth main-pane view and
driven by `claude`/`agy` through the `styrman-browser` MCP server; plan
`.agents/docs/plans/2026-09-11-browser-view.md`, all four phases committed)
and `deploy/styrman-ctl` (one front door: status / install / update / edit /
uninstall; plan `.agents/docs/plans/2026-09-11-installer-ctl.md`, all four
phases committed). Tag `v1.28.0` on release commit `eb7b883`, SSH-signed and
verified against `/etc/styrman/allowed_signers`; deployed by the
auto-updater at 15:37, `styrman-ctl status` verdict OK, server/caddy/runtime
images all at 1.28.0. Gate:
`.agents/docs/reviews/2026-09-11-preflight-v1.28.0.md`; release entry:
`.agents/docs/changes/2026-09-11-v1.28.0-release.md`; handoff:
`.agents/docs/handoffs/2026-09-11-session-end-v1.28.0-retro-applied.md`. Two
steps stay the operator's, not blocking: the manual agent walk in the
Browser view (instructions at the end of
`.agents/docs/testing/2026-09-11-verify-browser-phase4.md`) and a fresh-host
`styrman-ctl install` on a Debian 13 VM when one is available (never on the
production host). Follow-ups listed in both plans; nothing else queued.

**v1.27.1 shipped 2026-09-10** (PATCH) — tag on release commit `4686792`,
deployed by the auto-updater at 14:38, server healthy at 14:39:03. One fix:
the updater enables the boot-time firewall unit on every converge tick
(v1.27.0's converge only did so when a unit file changed, so the unit
stayed disabled on a host that had not run `install.sh`; BUG-035
recurrence). Proven live: main disabled the unit after the deploy and the
new updater's next tick re-enabled it. Gate:
`.agents/docs/reviews/2026-09-10-preflight-v1.27.1.md`; release entry:
`.agents/docs/changes/2026-09-10-v1.27.1-release.md`.

v1.27.0 shipped earlier the same day. Tag on release commit `face4ee` (gated
commit `0201b8c`), deployed by the auto-updater at 13:31, server healthy at
13:32:01, all three images at 1.27.0. Gate:
`.agents/docs/reviews/2026-09-10-preflight-v1.27.0.md`; release entry:
`.agents/docs/changes/2026-09-10-v1.27.0-release.md`. What it carries for a
user: the sidebar's "provisioning" state, terminal revocation within a
minute of a user's removal, the editor's checkable-only gutter and real
Discard, tofu diagnostics, the settings git card and ssh line from one
parser; for operators: the ingress dependencies converged, the boot-time
firewall unit, the Docker address pool, the legacy workspace migration
removed; internally the orchestrator's first unit tests. History: v1.25.0
shipped 2026-09-09 with ADR-067 live in production — the per-project
network mode is the bridge interface prefix, and ADR-065/066's
root-mediated machinery is deleted; v1.26.0 followed the same day with the
network-mode PUT deciding under the project lock, the ssh public key copy
button, and two security fixes from the gate.

**Nothing open for the operator.** Decided 2026-09-10: sshd keeps password
login (LAN mode is for specific projects; the operator accepts the
exposure, recorded in `.agents/docs/deploy/remote.md`). Done 2026-09-10 by
the operator: the browser walk-through (plan now 44 of 44: sandbox ping
fails, LAN ping succeeds after a roughly 30 s rebuild) and the `install.sh`
run that removed the retired `.path` unit and spool mount, plus the two
root commands, `sudo bash deploy/firewall/install.sh` (boot-time firewall
unit, now enabled) and `sudo bash deploy/host-setup.sh` (Docker address
pool 172.16.0.0/12, now in place). The backups clean-up was verified
already gone. `TODO.md` has no open item.

**Next code task on this area** — the network-mode PUT (commit `42716a8`,
`.agents/docs/changes/2026-09-10-network-mode-put-locked.md`, BUG-032) now
decides under the project lock inside `Orchestrator.WithProject`, persists
the record only after applying with nothing in force (no rollback), and
treats a no-op as agreement with the live bridge, so a re-PUT is itself the
recovery; every start reads the mode under that same lock, closing the
toggle-vs-start race — shipped in v1.26.0. The credential-broker
reachability test from a LAN project
(`.agents/docs/reviews/2026-09-09-security-adr-067.md`) was run 2026-09-10
on the production deployment with two users and passed on every probe
(`.agents/docs/testing/2026-09-10-broker-reachability-live.md`).

**TODO close-out shipped in v1.27.0:** plan
`.agents/docs/plans/2026-09-10-todo-closeout.md` is complete except Phase
6's final trail box (this edit). Five phase commits on `main` since
`v1.26.0`: `7dae197` server fixes, `3ce8a37` web + terminal
re-authorization, `46f3ec2` deploy and scripts, `0201b8c` the Docker seam
with the orchestrator's first unit tests, docs and BUG-034..037. Twelve
TODO items were struck with reasons by the operator's decision, the rest
fixed. `TODO.md` now has ONE open item, the operator's backups clean-up
(GCS bucket + service account in project `driven-eft-h4it`, and shredding
`/var/lib/styrman/backups/` on the host). Final verification on the
committed tree: integration 84/0/3, e2e 87/0/2, go test -race, vitest 147,
`deploy/verify-commit.sh` on `0201b8c`
(`testing/2026-09-10-verify-closeout-final.md`). The credential-broker
reachability test from a LAN project was run on 2026-09-10 on production
with two users and passed every probe
(`testing/2026-09-10-broker-reachability-live.md`).

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

<!-- Nothing queued. The last closed item (ADR-062, 2026-09-03) lives in changes/. -->

## Later / Ideas

<!-- Deliberately empty — awaiting operator input. Add items here as they arise, don't invent them. -->
