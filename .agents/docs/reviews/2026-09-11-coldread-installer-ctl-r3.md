---
owner: reviewer
date: 2026-09-11
---
# Review — cold-read of `.agents/docs/plans/2026-09-11-installer-ctl.md` (round 3, final)   (2026-09-11)

Range reviewed: plan file `.agents/docs/plans/2026-09-11-installer-ctl.md` (revised) plus
the named code surfaces: `deploy/host-setup.sh`, `deploy/firewall/install.sh`,
`deploy/firewall/styrman-firewall.sh`, `deploy/update/styrman-update` and its
`.service`/`.timer` units, `deploy/firewall/styrman-firewall.service`/`.timer`,
`deploy/docker-compose.yml`, `deploy/.env.example`, `.agents/docs/specs/styrman/{deploy,versioning}.md`,
`internal/config/config.go`, `internal/orchestrator/orchestrator.go` (label usage cross-check),
`internal/config/firewall_script_test.go`, `.agents/rules/*`, `REQUIREMENTS.md`, `AGENTS.md`.
Round-2 report read to confirm its findings are resolved. No diff exists — nothing implemented yet.

## Verdict: DESIGN NEEDS REVISION

Round 2's blocking finding (uninstall level 1's container selector contradicting
itself) is now genuinely fixed — the Interfaces section's exact flow (plan
lines 281–284) matches the Scope section and `internal/config/config.go`'s
`LabelManaged`/`LabelSystem` reality. Round 2's two minor open questions
(the `GIT_ASKPASS` helper covering `fetch`, and the hostname command) are
also now resolved explicitly in the text. But this fresh read against the
actual scripts found two new, concrete problems in the parts of the plan
round 2 did not check byte-for-byte: the uninstall's iptables cleanup omits
half of what the firewall script actually installs, and `update --setup`'s
"exact" flow specifies a manual tag checkout step that (a) has no defined
version source for its own standalone invocation and (b) is not what
`host-setup.sh`'s own canonical instructions or the updater's actual first
tick do — it may be redundant with, or in conflict with, `styrman-update`'s
own `git fetch --tags` + newest-stable-tag `checkout --detach --force`.

## Findings

1. **[medium / confidence: high]** Uninstall level 2's iptables cleanup (plan
   lines 287–288: `iptables -F STYRMAN-FWD`, `-D DOCKER-USER -j STYRMAN-FWD`,
   `-X STYRMAN-FWD` "(same for `STYRMAN-IN` in `INPUT`)") only ever removes the
   IPv4 chains. `deploy/firewall/styrman-firewall.sh:97-110` also creates and
   jumps to IPv6 chains `STYRMAN-IN6` (from `INPUT`) and `STYRMAN-FWD6` (from
   `FORWARD`), guarded only by `command -v ip6tables` — no host-disabled-IPv6
   check, so on any host with the `ip6tables` binary present these chains and
   jumps exist after `install`. A level-2 uninstall claims to leave the host
   free of "the firewall chains"; as written it leaves root-owned IPv6 chains
   and two `INPUT`/`FORWARD` jump rules behind. Must decide: does level 2 also
   flush/delete `STYRMAN-IN6`/`STYRMAN-FWD6` and their jumps (mirroring the v4
   commands), or is this an intentional exclusion that the plan should state
   and justify? As written it is neither — the omission looks like an oversight
   because the IPv4 companion commands are spelled out in the same paragraph.

2. **[medium-high / confidence: high]** `update --setup` step 2 (plan lines
   228–236) directs, unconditionally, `git -C "${STYRMAN_DEPLOY_DIR}" checkout
   "v${V}"` "for the tag being deployed" after the clone/fetch. Two problems:
   (a) `update --setup` is invocable standalone (Invocation section, line 140:
   `update [--redeploy] [--setup [--gh-token-file F] [--allowed-signers-file
   F]] ...`) with no `--version` flag anywhere under `update`, so when it is
   NOT reached via `install`'s step 6 (where `V` was resolved at install step
   4), there is no defined source for `${V}` — an implementer must guess
   whether to reuse `install`'s `--version`-or-`git describe` resolution, read
   `deployed_version()`, or something else. (b) `host-setup.sh`'s own 8c
   instructions (`deploy/host-setup.sh:320-336`, and the identical prose in
   `.agents/docs/specs/styrman/deploy.md:502-530`) clone the repo and enable
   the timer WITHOUT any manual `git checkout <tag>` step — the first
   `systemctl start styrman-update.service` tick is what selects and checks
   out the newest signed stable tag itself (`deploy/update/styrman-update:231-238,
   355,568`: `git fetch --tags` then `git checkout --detach --force "${ref}"`
   picking the newest stable tag, not an operator-supplied one). The plan's
   manual pre-checkout to `v${V}` is not in the script this task explicitly
   wraps (Out-of-scope: "no rewriting host-setup.sh behaviour") and duplicates
   or could disagree with what step 5's `systemctl start styrman-update.service`
   (the same flow's very next step) will independently select. Must decide
   whether `update --setup` performs this checkout at all, and if so, from
   what version source in the standalone-invocation case.

## Readiness

Not implementation-ready. Two concrete questions an implementer must ask
before coding uninstall level 2 and `update --setup`:
- Does uninstall level 2 also remove `STYRMAN-IN6`/`STYRMAN-FWD6` and their
  `INPUT`/`FORWARD` jumps, or is IPv6 cleanup deliberately out of scope
  (and if so, on what basis, given it's part of "the firewall chains")?
- What supplies `${V}` for `update --setup`'s `git checkout "v${V}"` when
  invoked standalone (not via `install`), and is this manual checkout step
  needed at all given the updater's own first tick already selects and
  checks out the newest signed stable tag?

Everything checked from round 2 (uninstall level 1's selector, the
`GIT_ASKPASS`/`fetch` coverage, the hostname command, `validate()`'s 25-key
coverage, clone/`.env` ownership, the KSM tmpfiles path, `OPERATOR`'s default
chain) is confirmed correct against the current code on this pass.

## Not examined

`deploy/update/styrman-update`'s full body beyond the sections cited above
(rollback logic, `host_converge` in full, the flock/lease mechanics); the
full text of `deploy/README.md` and `.agents/docs/specs/styrman/deploy.md`
beyond the sections grepped; `internal/config/firewall_script_test.go`'s
`TestFirewallScriptInNetns` body past its skip guards; no code was written
or run — this is a plan-only cold read.

design needs revision
