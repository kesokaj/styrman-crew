---
owner: reviewer
date: 2026-09-13
---
# Review — audit: internal/orchestrator (2026-09-13)

Scope: all non-test `.go` files in `internal/orchestrator/` at HEAD `e56e775`:
`orchestrator.go`, `ports.go`, `exec.go`, `docker.go`, `browserpeer.go`.
This is an `/audit` scan (recorded intent vs. code), not a diff review — no
git range applies; every finding is pinned to the file at `e56e775`.

## Verdict: APPROVE WITH NITS

Nothing broken enough to block. The package is unusually well hardened —
almost every non-obvious line carries a comment tracing back to a specific
BUG-NNN or "security review" round, and the two hazards this audit expected
to find repeated (BUG-002/003 symlink-planted mount sources, BUG-023/BUG-031
fail-open network-mode races) are both closed and covered by
`seam_test.go`. The one real defect is an inconsistency in the container
"recreate" ladder inside `ensureRunningLocked`: the workspace-bind recreate
step is missing the running-container guard its two neighbors (image
upgrade, network-mode change) explicitly carry, so it can force-kill a
**running** project's terminals — the exact invariant the surrounding code
was written to protect.

## Findings

- **[medium / confidence: high]** `internal/orchestrator/orchestrator.go:280` —
  The "recreate for the ADR-027 workspace bind" branch fires on `err == nil
  && !hasWorkspaceBind(...)` with **no running-container guard**. Its two
  neighbors immediately below it — the image-upgrade recreate (line 299) and
  the network-mode recreate (line 319) — both gate on `insp.State == nil ||
  !insp.State.Running`, with a comment explaining why: "recreating a RUNNING
  one would kill live terminal sessions". The workspace-bind branch has no
  such gate: if `hasWorkspaceBind` is false for a container that is
  currently **running**, it calls `ContainerStop` + `ContainerRemove` on it
  unconditionally. Historically this only affected containers created
  before ADR-027 shipped (one-time migration), so most of its blast radius
  is already behind this deployment. But the check re-runs on every
  `EnsureRunning`, and `hasWorkspaceBind` compares against the *live*
  `o.cfg.ProjectWorkspace(user, project)` path — so a change to the
  operator-tunable workspace root (or any other event that makes the
  configured workspace path no longer match a running container's actual
  bind) will make this fire against a running project on its very next
  terminal reconnect/attach, killing every live tmux session with no
  warning and no confirmation — the failure mode R6 ("terminals survive
  browser disconnects") and the neighboring code's own stated invariant
  both exist to prevent. Fix: gate this branch the same way as the other
  two, `if err == nil && (insp.State == nil || !insp.State.Running) &&
  !hasWorkspaceBind(...)`, and let a running mismatched container pick up
  the bind on its next natural stop/start the same way the other two do.
  Not covered by any test — `orchestrator_workspace_test.go` only unit-tests
  the pure `hasWorkspaceBind` predicate, never `EnsureRunning`'s behavior
  against a *running* container missing the bind (contrast
  `TestEnsureRunningLeavesARunningContainerAlone`, which exists for the
  mode-mismatch case but has no counterpart here).

- **[low / confidence: medium]** `internal/orchestrator/orchestrator.go:222-233` (`lockedProject.ModeMismatch`) vs. `:566-591` (`bridgeModeMismatch`/`currentBridge`) — two independent implementations of "does the project's bridge interface disagree with the wanted mode", each re-deriving `config.BridgeNameFor(netName, lan) != cur`, with deliberately different (and correct, per their own comments) handling of a *missing* network. This is intentional and each branch is tested, but it is still the same fact computed twice from the same primitives (`currentBridge` + `BridgeNameFor`); a future change to the rebuild criterion in one has no compiler-enforced reason to reach the other. No concrete failure today — flagging only because ADR-067's bridge-mode logic has already produced three same-day recurrences in this exact area (BUG-028/029/031); a shared low-level helper (e.g. `currentBridge` returning an enum {match, mismatch, missing}) consumed differently by each caller would remove the duplication without losing the different missing-network semantics.

## Criteria

1. **Dead code** — pass. Every exported function in the five files (`EnsureSelfAttached`, `ResolveProjectByIP`, `RootRemoveContents`, `Logs`, `SetupStatus`, `DestroySetup`, `RemoveStaleSetupVMs`, `CleanupLegacy`, `Adopt`, `Ping`, `BrowserPeer`, `IPAddress`, `ListeningPorts`, `Touch`, `Stopped`, `WithProject`, etc.) has at least one real caller outside its own package's tests (checked by grep across `internal/` and `cmd/`). No TODO/FIXME/XXX markers, no commented-out code, no unreachable branches found.
2. **Flow tracing** — one real gap: see the workspace-bind finding above (a recreate step whose invariant — "never kill a running project" — is asserted by its neighbors but not honored by itself). Every other changed-shape function (network mode, self-attach, idle reaper, stop marker) was traced end to end from caller to Docker call and back, including error propagation (`fmt.Errorf("...: %w", ...)` used consistently; no discarded errors found except deliberate best-effort spots that are commented as such, e.g. `CleanupLegacy`'s `_ = o.docker.ContainerRemove(...)`). Context/cancellation: `Exec` guarantees a 30s deadline when the caller's context has none; `unmarkIfStillRunning`'s background goroutine deliberately uses a fresh context rather than the caller's (documented, correct — the caller's ctx is the likely reason the stop failed in the first place).
3. **Standards conformance** — pass. Idiomatic Go: errors wrapped with `%w`, `sync.Map`/mutex use is consistent with documented lock scope, `docker.go`'s `dockerAPI` seam is a small caller-scoped interface (not a mirror of the SDK) matching the `golang` skill's "small interfaces at the seam" guidance, with a compile-time `var _ dockerAPI = (*client.Client)(nil)` assertion and a comment recording exactly how it was derived (grep + count) — self-verifying against drift.
4. **Directory layout** — pass. All production code under `internal/orchestrator/`; tests sit beside their code (`*_test.go` in the same directory), nothing stray.
5. **Simplicity (DIY-except-security)** — pass. No new dependencies in this slice; stdlib (`net`, `path/filepath`, `sync`) plus the Docker SDK are used directly; no speculative abstraction (the one seam, `dockerAPI`, has a real second implementation — the fake used in tests — so it earns its existence). No auth/crypto/session code lives in this package.
6. **Decoupling** — pass, with the one low note above (duplicated bridge-mismatch logic across `lockedProject.ModeMismatch` and `bridgeModeMismatch`). The `Project`/`lockedProject` interface at `WithProject` is a clean, narrowly-scoped seam for the network-mode toggle handler in `internal/httpapi` to make a run of decisions under one lock without re-entering `Orchestrator`'s own locking methods — a good example of the module boundary the review-standards skill asks for.

## Not examined

- `internal/config` (BridgeNameFor, NetworkName, path helpers) — read only far enough to confirm the orchestrator calls them consistently; not audited for its own correctness (it has its own tests, e.g. `bridgename_test.go`, `firewall_script_test.go`).
- `internal/httpapi` callers (`network.go`, `server.go`, `browser.go`, `proxy.go`, `credharvest.go`) — traced only at the call sites needed to confirm orchestrator's exported functions are reached correctly; not reviewed for their own internal correctness.
- `internal/terminal/bridge.go` — traced only for `Touch`/`Stopped` usage, not reviewed in full.
- `image/` (the in-guest launcher, `styrman-browser`, entrypoint) and `deploy/` — out of scope for this slice; referenced only via comments in `ports.go`/`browserpeer.go`.
- Test files under `internal/orchestrator/*_test.go` were read for coverage-gap purposes only (confirming/denying a covering test for each finding), not reviewed line by line for their own quality.
- No live Kata/Docker environment was exercised — findings are static-analysis / code-reading based, confirmed against `go build`/`go vet` (both clean) but not run against a live daemon.
