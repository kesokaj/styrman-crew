---
owner: reviewer
date: 2026-09-08
---
# Review — LAN full parity plan (cold-reader check)   (2026-09-08)

Range reviewed: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (plan document, not a diff), cross-read against `deploy/firewall/styrman-firewall.sh`, `internal/netpolicy/{status,dir}.go`, `internal/credbroker/broker.go`, `internal/httpapi/network.go`, `internal/httpapi/setup.go`, `cmd/styrman/main.go`, `internal/orchestrator/orchestrator.go`, `internal/config/config.go`, `internal/credstore/*.go`, `REQUIREMENTS.md` R12.

## Verdict: REQUEST CHANGES (design), NOT implementation ready

The plan's iptables shape and status/UI design are careful and internally consistent, but two things need deciding before Phase 1/3 start: (1) the shell script's rule-family machinery (`scan_current`/`reconcile`/`count_applied`/`RULE_RE`) is currently hardwired to one chain and one regex, and the plan gives no answer for whether the two new families share that machinery or duplicate it — that's the difference between a ~150-line refactor and ~300 duplicated lines, and it changes what Phase 1/2 actually build; (2) the Surfaces-touched table names the wrong file for credbroker wiring, and the plan never says which Go type implements `LanChecker` or that `cmd/styrman/main.go` needs a new dependency edge to get there.

## Findings (cold-reader gaps and questions)

### (1) Gaps — must be decided before implementation

- **[medium / confidence: high]** `deploy/firewall/styrman-firewall.sh:191-417` — `scan_current`, `rule_args`, `reconcile`, `count_applied`, `RULE_RE`, and `delete_malformed` are all hardcoded to chain `STYRMAN-FWD` and the single `styrman-lan:` comment tag. Phase 1 wants a `styrman-lanhost:` family in a *different* chain (`STYRMAN-IN`), and Phase 2 wants a `styrman-lanpub:` family in `STYRMAN-FWD` *alongside* the existing family with a different regex (adds `-m conntrack --ctstate DNAT`) and a different "wanted count" (per allowed-bridge, not per LAN_CIDRS entry, and the bridge set itself is discovered fresh each run — a third source of churn beyond the LAN_CIDRS and DHCP-address churn the plan does name). Nothing in the plan says whether these three chain/regex pairs share one generalized (chain, regex, want-count) implementation or are three near-copies of the same ~120 lines. That's a real architectural fork the checklist items ("collect/scan/reconcile/count, delete by spec") gloss over, and it affects file size, review effort, and where the next BUG-013-style bug is likely to hide.
- **[medium / confidence: high]** `deploy/firewall/styrman-firewall.sh:359-372` (`delete_malformed`, hardcoded to `STYRMAN-FWD`) — a malformed-line cleanup path for `STYRMAN-IN` (the new `styrman-lanhost:` family's chain) is not mentioned anywhere; if the same generalization question above isn't resolved, this function needs its own `STYRMAN-IN` twin that the plan doesn't list as a task.
- **[medium / confidence: high]** Plan's Surfaces table (`internal/httpapi/setup.go` row) is factually wrong: `internal/httpapi/setup.go:1-30` is the onboarding/setup-status endpoint (`setupStatus`, `SSHKey`, `GitIdentity` — unrelated), not where the broker is constructed. `credbroker.New(cstore, orch, logger)` is actually called at `cmd/styrman/main.go:180`, and `orch` (the `*orchestrator.Orchestrator` implementing `Resolver` today, `internal/orchestrator/orchestrator.go:413`) has no dependency on `internal/project` and no visibility into `Project.LanAccess` — that lives in per-user stores owned by `httpapi.Server` (`s.space(user).projects`, used in `internal/httpapi/network.go:187`). So `LanChecker` cannot be implemented by the same object already passed as `Resolver`; it needs a new method, almost certainly on `httpapi.Server` (already constructed at `main.go:104`, before the broker at `main.go:180`, so ordering isn't a blocker) — but the plan never says this, never lists `cmd/styrman/main.go` in Surfaces touched, and points the reader at the wrong file for "broker wiring."
- **[low / confidence: medium]** `LanChecker.LanEnabled(user, project string) (bool, error)` (plan's Go sketch) doesn't say whether it reads the project record (`Project.LanAccess`, immediate) or the converged host-firewall state (`netpolicy.Status.Applied`, lags up to ~60s per the plan's own DHCP-address failure mode). The rest of this codebase treats exactly this record-vs-converged gap as security-relevant (the `Derive`/`stale`/`PendingGrace` state machine in `internal/netpolicy/status.go:161-212` exists for it). On disable, if `LanChecker` reads the record, the broker could re-serve `~/.ssh/id_ed25519` to a project whose firewall rule removal hasn't converged yet — a project that (briefly) still has both the key and LAN reach. The plan's Failure-modes table covers the enable-side prune ("best-effort... documented, not solved") but not this disable-side re-serve timing. Worth an explicit line either way (record-based is probably fine given the login-shell only re-fetches `/files` on new sessions, but the plan should say so rather than leave it silent).

### (2) Readiness — questions a first-pass implementer would have to ask

1. Do the three rule families (`styrman-lan:`, `styrman-lanhost:`, `styrman-lanpub:`) share one generic scan/reconcile/count implementation parameterized by chain+regex+want-count, or are Phase 1 and Phase 2 expected to write near-duplicate blocks? (Gap 1 above.)
2. Does `STYRMAN-IN`'s malformed-rule cleanup reuse `delete_malformed` generalized to a chain parameter, or is a second copy expected? (Gap 2.)
3. What Go type implements `credbroker.LanChecker`, and where is it wired — is it a new method on `httpapi.Server` passed alongside `orch` into `credbroker.New` at `cmd/styrman/main.go:180`, or something else? The plan's own Surfaces table doesn't point at a real answer. (Gap 3.)
4. Is `LanChecker.LanEnabled` backed by the project record or by converged firewall status, and is the disable-side timing (key re-served before the firewall rule is actually gone) accepted as-is or does it need the same staleness handling `netpolicy.Derive` already has? (Gap 4.)

None of these are large — each is answerable in one or two sentences from whoever wrote the plan — but a first-pass implementer would have to stop and ask all four rather than guess, because guessing wrong on (1)/(2) changes the shape of `styrman-firewall.sh` for the rest of its life, and guessing wrong on (3)/(4) touches the credential-broker security boundary the plan itself treats as the highest-risk surface (Risk 4: "Both need `security` on the diff").

## Criteria

1. Dead code — n/a (plan document, no code yet).
2. Flow tracing — mostly good: the plan traces `Availability`/`Derive`/status-file flow correctly and correctly identifies `internal/netpolicy/dir.go` needs no change (confirmed by reading it: `SetMarker`/`Sync` are marker-name-only and family-agnostic). The one broken hop is credbroker wiring (finding 3).
3. Standards conformance — the fail-closed, delete-by-spec, `-w 5`, and root-owned-conf conventions are followed faithfully in the plan's rule-shape section; consistent with `deploy/firewall/styrman-firewall.sh`'s existing style.
4. Directory layout — surfaces table is plausible except the `internal/httpapi/setup.go` misattribution (finding 3); no new files proposed outside existing conventions.
5. Simplicity — the plan explicitly rejected the more complex L2/macvlan design in favor of extending the existing iptables approach; reasonable. The unresolved generalize-vs-duplicate question (finding 1) is exactly the kind of decision that determines whether the result stays simple.
6. Decoupling — the credbroker/orchestrator/httpapi layering question (finding 3) is a real decoupling gap: `LanChecker` sits at a package boundary the plan hasn't actually drawn.

design needs revision
implementation not ready
