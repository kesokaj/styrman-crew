---
owner: reviewer
date: 2026-09-09
---
# Cold-reader review — LAN mode by bridge prefix plan   (2026-09-09)

Document reviewed: `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md`
Code surfaces checked: `internal/config/config.go`, `internal/config/bridgename_test.go`,
`internal/orchestrator/orchestrator.go`, `internal/httpapi/network.go`,
`internal/httpapi/server.go`, `internal/httpapi/lanprune_test.go`,
`internal/project/store.go`, `internal/credbroker/broker.go`,
`cmd/styrman/main.go`, `REQUIREMENTS.md` (R12).

This is not a diff review (no source was modified — this task is a
cold-reader check of a plan file, per dispatch). No source, config, or test
file was edited.

## Verdict on the plan: design needs revision, and it is not implementation-ready

The plan's central mechanism — a bridge-name "mismatch heal" placed inside
`create()` that makes toggling "cheap" and needs no dedicated toggle-time
network surgery — sits in a function that the documented toggle flow never
reaches. That is a design-level misunderstanding of the existing
orchestrator, not a nit: as written, flipping LAN mode on a *running*
project (the plan's own stop-then-`EnsureRunning` flow) will not change the
container's network attachment at all, because `Stop()` never removes the
container and `EnsureRunning()`'s reuse branch (existing container) never
calls `create()`.

## (1) Gaps — must be decided before implementation

- **The mismatch-heal lives in the wrong code path.**
  `internal/orchestrator/orchestrator.go:795` (`create()`) is only invoked
  from `EnsureRunning` when `ContainerInspect` returns `errdefs.IsNotFound`
  (orchestrator.go:247-320) — i.e. only when the *container* does not exist.
  When the container exists but is stopped, `EnsureRunning` takes the
  `case err == nil:` branch (orchestrator.go:229-246) and only calls
  `ContainerStart` — it never calls `create()`. `Stop()`
  (orchestrator.go:918-951) only calls `ContainerStop`; it never removes the
  container. So the plan's documented toggle flow for a running project
  ("stop it and `EnsureRunning` with the new mode", Interfaces → API,
  plan:206-209) will stop the container, then restart the *same* container
  still attached to the *same* bridge with the *old* name — the mismatch
  check in `create()` never runs, and the network never changes. Whoever
  implements this must decide: does the toggle force a container
  **remove**+recreate (not stop/start), or does the mismatch-heal need to
  move into the `case err == nil:` reuse branch too (disconnect the
  container's endpoint, remove/recreate the network, reconnect or recreate
  the container)? The plan states neither.
- **Even if reached, the heal's own precondition is unmet in the common
  case.** The heal's comment (plan:183: "Disconnect self (port-preview
  proxy), remove, recreate") only accounts for the *server's* self-attach.
  `NetworkRemove` fails while any container — running or merely stopped —
  still has an endpoint on that network. After a plain `Stop()`, the
  project's own container is still attached. So even a fixed call site
  needs the project's own container disconnected or removed first; the
  pseudocode doesn't mention this at all.
- **`internal/httpapi/network.go:245`'s `pruneHostReachingCreds`, and its
  347-line test file `internal/httpapi/lanprune_test.go`, are not named
  anywhere in "Surfaces touched" or "Removed identifiers".** This function
  deletes the SSH key (and other host-reaching credential files) from a
  project's home the moment LAN is enabled (network.go:238-246,
  261-295) — a *different* mechanism from the `credbroker.LanChecker`
  "latch" the plan does list for deletion. Risk #4 states the design intent
  plainly: "The SSH key is served to a LAN project... The operator chose
  this over the latch" — i.e. the key should no longer be withheld or
  pruned at all. If an implementer follows only the plan's named identifier
  list, `pruneHostReachingCreds` and its call site survive untouched,
  silently deleting the SSH key on every LAN-enable and directly
  contradicting the plan's own accepted risk. This needs an explicit
  decision and must be added to the surfaces/removed-identifiers lists.
- **`Server.ingressBr` / `ingressBrMu` / `ingressBrAt` (server.go:112-116)
  and `Orchestrator.BridgeNameForNetwork` (orchestrator.go:375-396) are
  orphaned by the plan but not named.** Both exist solely to feed
  `ingressBridgeMismatch` (network.go:103), which the plan does list for
  deletion. Once that's gone, these fields and this exported method have
  zero callers outside `lanprune_test.go`'s test helper (which itself needs
  a decision — see above). The plan should either explicitly schedule their
  deletion or say why `BridgeNameForNetwork` is being kept for some other
  reason (it isn't — the comment ties it 1:1 to ADR-066).
- **The R12 replacement text does not actually exist in the plan.** The
  "Why" section claims operator approval was given "on ... the exact R12
  replacement text reproduced in Phase 0" (plan:63-66), but Phase 0's actual
  task (plan:284) only describes a splice boundary in prose ("everything
  from 'Enforced by naming the bridge interface' to the end of R12") — no
  replacement text is reproduced anywhere in the document. Read against the
  live `REQUIREMENTS.md:77-109`, that splice point falls *mid-sentence*
  inside R12's existing paragraph (the phrase begins partway through a
  sentence about bridge naming, not at a paragraph boundary), which makes
  the boundary itself ambiguous. An implementer cannot execute Phase 0
  without either the missing text or a second round with the operator.

## (2) Readiness — could a first-pass implementer ship this without asking?

No. Questions that would have to be asked before or during implementation:

1. Does the LAN toggle on a *running* project require a container
   **remove**+recreate (losing in-flight terminal sessions immediately, as
   `Stop()` already documents is avoided elsewhere in this file for the
   image-upgrade case), or should the mismatch-heal be duplicated/moved
   into the "reuse existing container" branch of `EnsureRunning`? The plan
   picks neither.
2. When the network must be recreated on a running project, what
   disconnects the *project's own* container endpoint before
   `NetworkRemove`, given Docker refuses to remove a network with any
   attached endpoint (running or stopped)?
3. Is `pruneHostReachingCreds` (network.go:270) deleted, along with its
   call site and `lanprune_test.go`, or kept? The plan's Risk #4 implies
   "deleted" but never says so, and never lists it as a surface to touch.
4. What happens to `Server.ingressBr`/`ingressBrMu`/`ingressBrAt` and
   `Orchestrator.BridgeNameForNetwork` once their only caller
   (`ingressBridgeMismatch`) is deleted — dead code left in place, or
   removed in the same phase?
5. What is the exact R12 replacement text? Phase 0 cannot be executed as
   written; the plan claims it was already reproduced and approved, but it
   is not present in this document.
6. (Minor, self-resolving via `go build`, but worth naming) `BridgeNameFor`'s
   signature change from one argument to two (`config.go:493`) breaks the
   three existing single-argument call sites in
   `internal/config/bridgename_test.go:13,16` and
   `internal/orchestrator/orchestrator.go:802,1217` — Phase 1's task list
   mentions the two orchestrator call sites but frames the test file change
   only as "append (the file exists)", when the existing calls in that same
   file also need editing, not just appending to.

## Not examined

- `web/src/components/LocalNetwork.tsx`, `web/src/lib/api.ts` (Phase 4
  surfaces) — not read in detail; UI contract plausibility not checked
  against the actual `restarted` API shape beyond what's in the plan text.
- `deploy/firewall/styrman-firewall.sh`, `install.sh`, `test.sh` (Phase 3) —
  not read; the one-shot stale-rule cleanup and BUG-013 full-content-match
  deletion approach were taken on the plan's word.
- `.agents/docs/architecture/DECISIONS.md` ADR-065/066 full text (only
  referenced, not read) — did not verify the "supersedes... keeping them as
  history" mechanics are consistent with the project's append-only rule for
  that file.
- `internal/credbroker/broker.go` beyond the `LanChecker`/`lan` field grep —
  did not trace every caller of `New(...)` to confirm all call sites can
  drop the `lan` argument cleanly.
- Live `iptables -S` state was not re-verified (dispatch offered `sudo
  iptables -S`); the plan's own cited output (lines 44-49) was taken as
  given, not re-run.
- `internal/project/store.go:128` (`LanChangedAt` stamped on load) was
  spotted as needing removal alongside the field deletion but not chased
  further; low-severity, likely obvious to an implementer editing the same
  file.

design needs revision
implementation not ready
