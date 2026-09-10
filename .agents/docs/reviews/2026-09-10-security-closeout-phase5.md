---
owner: security
date: 2026-09-10
---
# Security review — closeout Phase 5 (orchestrator dockerAPI seam)   (2026-09-10)

Range reviewed: working tree vs HEAD (0378825), the ON-DISK files —
`internal/orchestrator/orchestrator.go` (1 line: `docker *client.Client` →
`docker dockerAPI`), new `internal/orchestrator/docker.go`, new
`internal/orchestrator/fakedocker_test.go`, new `internal/orchestrator/seam_test.go`.
Blast radius followed: every `o.docker.*` call site in the package (61 calls,
19 methods), `New`, and the credential-broker tenant resolution that rides the
same client (`ResolveProjectByIP`).

**The handed diff `.agents/scratch/phase5.diff` is STALE.** `docker.go` matches
disk; the two test files do not. On disk, `seam_test.go:104` already reads
`||` where the artifact reads `&&`, and `fakedocker_test.go` already takes
`f.mu` around every `c.running` mutation and copies `c.binds` — an earlier
round's nits, applied but not regenerated into the artifact. Everything below
is anchored to the files on disk, which is what ships; anyone else reading that
artifact will report two findings that no longer exist.

## Verdict: FINDINGS (all low; nothing blocking)

No new injection point and no production behaviour change. `docker` and
`dockerAPI` are both unexported, `New` (orchestrator.go:130) is the only
production writer, there is no exported setter, accessor, or type assertion on
the field, and no package outside `internal/orchestrator` can even name the
interface — an outside caller can only build a zero-value `Orchestrator`, whose
nil field panics exactly as a nil `*client.Client` did. The guards the task
asked about are genuinely load-bearing: nine mutations of the production code
each killed their test (table below). Every finding here is about what the fake
CANNOT express, and the risk is future rather than present: a fake permissive
enough to let a *new* test pass vacuously against a guard that is not there.
Worst of them is `ContainerList`/`NetworkList` returning `nil` unconditionally —
the keep-set that stops `CleanupLegacy` from deleting a live project's network
(R7) is derived entirely from that list, so a test written against this fake
would drive the destructive half of that sweep with an empty protect-set and
pass.

## Mutation evidence (question 2 — would each guard's test actually fail?)

Run in a scratch copy of the module (`.agents/scratch/secreview-phase5/`, made
from the on-disk sources; the repo was never edited), one mutation at a time,
`go test -run '^<the one test>$'`:

| # | Mutation of production code | Test | Result |
|---|---|---|---|
| M1 | drop the `projectHomeIsReal` call before the switch (orchestrator.go:341) | `TestEnsureRunningRefusesASymlinkedProjectHome` | **FAILED** — "want a project-home refusal, got \<nil\>" |
| M2 | swallow the `bridgeModeMismatch` error (BUG-031 fail-open, :322) | `TestEnsureRunningRefusesWhenTheNetworkCannotBeInspected` | **FAILED** — "want a refusal, got \<nil\>" |
| M3 | `lan, _ := opts.lan()` — mode-read error ignored (:266) | `TestEnsureRunningRefusesWhenTheModeCannotBeRead` | **FAILED** |
| M4a | `stopMarked` unmarks unconditionally after a failed stop (:1123) | `TestStopMarkedClearsTheMarkerOnlyWhenStillRunning` | **FAILED** |
| M4b | `unmarkIfStillRunning` clears even when the container is DOWN (:1164) | same | **FAILED** |
| M5 | `Status` folds any inspect error into `absent` (BUG-029, :150) | `TestStatusMapsAbsentStoppedRunningAndError` | **FAILED** |
| M6 | `currentBridge` folds an inspect error into not-found (:906) | `TestCurrentBridgeFoundMissingAndError` | **FAILED** |
| M7 | `CompareAndDelete` → `LoadAndDelete` (:1173) | `TestUnmarkDoesNotClearANewerStopsMarker` | **FAILED** |
| M8 | drop `o.stopped.Delete` on the intentional-start path (:442) | `TestEnsureRunningLeavesARunningContainerAlone` | **FAILED** |
| M4c | two samples → one in `unmarkIfStillRunning` (:1150) | `TestStopMarkedClearsTheMarkerOnlyWhenStillRunning` | *still passed* — F1 |
| M4d | mark AFTER the stop instead of before (:1113) | same | *still passed* — F2 |
| M9 | drop the CREATE-path `realBelow` re-check (:389) | `TestEnsureRunningRefusesASymlinkedProjectHome` | *still passed* — F3 |

So all four guards the task named — the symlinked project home, the BUG-031
network-inspect refusal, the mode read under the lock, and the BUG-030
stop-marker discipline — are covered by tests that provably fail when the guard
is removed. Three narrower rules inside them are not (F1-F3).

Baseline: `go test ./internal/orchestrator/ -race -count=1 -v` → 24 top-level
tests, 24 PASS, 0 FAIL, 21.6 s, no data race. `go build ./...` and
`go vet ./internal/orchestrator/` clean.

## Findings

- **[low / confidence: high]** `internal/orchestrator/fakedocker_test.go:176` (`ContainerList`) and `:266` (`NetworkList`) — both `return nil, nil` unconditionally: they ignore `f.containers`/`f.networks` AND the label/name filters their callers pass. Six production call sites read them and three are destructive or trust-bearing. `CleanupLegacy` (orchestrator.go:825 and :857) derives its keep-set of networks-NOT-to-delete from `ContainerList` and then removes every `sdb-` network absent from it — with this fake the keep-set is always empty, so a future test that plants a live project and asserts "the sweep spared its network" would pass with the protecting code deleted, and one that asserts an orphan is removed proves nothing about the keep-set. `Adopt` (:1389) returns an empty map, so a restart-reconciliation test is vacuous. `ResolveProjectByIP` (:549), the credential broker's tenant lookup, can never reach its subnet loop. Remediation: return `f.containers`/`f.networks` honouring at least the `label`/`name` filter args, or put a refusal in the doc comment — no test may assert on a list-driven path until they do.
- **[low / confidence: high]** `internal/orchestrator/fakedocker_test.go:162` (`ContainerRemove`) — removes a container whatever its `running` state and ignores `RemoveOptions.Force`; real Docker answers 409 `cannot remove a running container` without `Force`. All 11 production call sites pass `Force: true` today (orchestrator.go:286, 305, 327, 699, 711, 790, 843, 1360, 1520, 1639 — verified individually), so nothing is masked now; the gap is that a future call site that forgets `Force` on a running container fails only in production. The same shape at `:245` (`NetworkRemove` deletes a network that still has an attached container, where Docker returns 403 `has active endpoints` — precisely the condition `lockedProject.ModeMismatch`'s comment at orchestrator.go:245 says "took manual intervention", so the rebuild ORDER container-then-network is untested) and at `:112` (`ContainerCreate` silently overwrites an existing name instead of Docker's 409 conflict, so a rebuild path that forgot to remove first would look fine — `TestEnsureRunningRebuildsAStoppedContainerOnTheWrongBridge` catches today's path only because it counts `ContainerRemove == 1`). Remediation: three lines each — refuse a running container without `Force`, refuse a network with a connected container, refuse a duplicate name.
- **[low / confidence: high]** F1 — `internal/orchestrator/orchestrator.go:1150` — the two-samples-2s-apart rule in `unmarkIfStillRunning` (v1.25.0 preflight round 4: one sample cannot tell "still running" from "a deadline fired mid-`ContainerStop` and the daemon is still stopping") has no coverage. Collapsing the loop to a single sample leaves `TestStopMarkedClearsTheMarkerOnlyWhenStillRunning` green (M4c), while the test's own comment claims "after two samples". The exposure if the rule is later dropped is BUG-030's opposite: a marker cleared on a container that is on its way down, and the next terminal auto-reconnect resurrects a VM the user stopped. Remediation: script `ContainerInspect` to answer Running on sample 1 and not-Running on sample 2, and assert the marker is KEPT.
- **[low / confidence: high]** F2 — `internal/orchestrator/orchestrator.go:1113` — the mark-BEFORE-stop ordering, which the comment there calls the whole reason a terminal auto-reconnect cannot boot the VM straight back up, is not covered: moving the `stopped.Store` to after the failed stop leaves the test green (M4d), because the test only observes the marker's final state. Remediation: have the fake's `ContainerStop` call back into `o.Stopped(user, project)` while the stop is in flight and record what it saw.
- **[low / confidence: high]** F3 — `internal/orchestrator/orchestrator.go:389` — the CREATE-path `realBelow` re-check is not exercised. This is BUG-003's residual guard: `mkdirAllConfined` returns nil *through* a symlink-to-dir, so the home is re-verified after it is created and before it becomes a bind source. `TestEnsureRunningRefusesASymlinkedProjectHome` plants an EXISTING container, so it takes the start path and only proves the `projectHomeIsReal` check at :341 (M1 kills it; M9, deleting the create-path check, does not). `realBelow` itself has a direct unit test (`realbelow_test.go`), so the function is safe — its call site on the path that first creates a project is what is unguarded by tests. This is the guard the guest-writable-paths rule cares most about, on the one path the new tests do not take. Remediation: a second case with NO container planted (so `errdefs.IsNotFound` routes to the create branch) and a symlink at `p/<uuid>`.
- **[low / confidence: high]** `internal/orchestrator/fakedocker_test.go:96` (`ContainerInspect`) — populates only `ContainerJSONBase`: `InspectResponse.Config` and `.NetworkSettings` stay nil. Any test of the cross-tenant guards in `ResolveProjectByIP` (orchestrator.go:544 and :575, `cinsp.Config.Labels[config.LabelUser]`; :582, `containerHoldsIP`) would nil-panic rather than pass — loud, so not a false-pass risk, but it means the credential broker's owner check, the highest-value boundary reachable through this seam, is out of the fake's reach. `NetworkInspect` (`:232`) likewise returns no `IPAM`, so the subnet match can never fire. Remediation: give `fakeContainer` `labels` and `ips` fields and `fakeNetwork` a `subnet`, or say in the doc comment that the broker path is out of scope.
- **[low / confidence: medium]** `internal/orchestrator/seam_test.go:161-168` — `TestWithProjectStopPersistStartOrderAndFailedStart` asserts `strings.Join(order, ",") == "stop,persist,start"` on a slice the test body appends to itself, with `"persist"` appended between two calls and no persistence call anywhere in the test. The ordering the name promises (stop → write the record → start, under one lock hold) is therefore not verified. What the test DOES verify is real and worth keeping: that `Status`/`ModeMismatch`/`Stop`/`EnsureRunning` all work under a single `WithProject` hold, and that a FAILED restart leaves the stop marker set. Remediation: rename it, or drive the order from `fd.calls` plus a real record write.
- **[low / confidence: low]** `internal/orchestrator/fakedocker_test.go:232` — `NetworkInspect` reads `n.bridge` after releasing `f.mu`, and `plant` (seam_test.go:44) writes `fd.containers`/`fd.networks` with no lock at all. Both are safe today because a fake is populated before any orchestrator goroutine exists and `bridge` is never mutated, and `-race` is clean over the whole package; it is named only so the next test that starts a goroutine before planting knows the invariant it must keep. (The larger version of this — unlocked `c.running` mutation against the `unmarkIfStillRunning` goroutine — is present in the handed diff artifact but already FIXED on disk.)

## Checked

- **Secrets** — clean. `grep -Ei "secret|token|password|api[_-]?key|BEGIN .*PRIVATE KEY|Authorization"` over the three files: no hits. The only literals introduced are a test UUID (`11111111-2222-4333-8444-555555555555`) and `tUser = "alice"`. Nothing new is logged: the fake's call log is test-only, and no production log line was added or changed.
- **Injection** — no new external input reaches the changed code. `dockerAPI` is a byte-for-byte pass-through of `*client.Client`'s own v28 signatures (compile-proved by `var _ dockerAPI = (*client.Client)(nil)`, docker.go:46); container and network names still come from `config.ContainerName`/`config.NetworkName` over the project UUID, not from anything in this diff. No shell, SQL, path, or template boundary is touched. The one string built by concatenation in the new code is the fake's call log.
- **Authz/authn** — unchanged and untouched. No handler, middleware, ingress-token check, or identity path is in the diff. The tenant-scoping the orchestrator does carry (`cfg.TenantDir(user)`, the `LabelUser` owner check in `ResolveProjectByIP`) is called exactly as before — verified by reading all 61 `o.docker.*` call sites, none of which changed. Retyping the field changes no dispatch a request can influence.
- **Crypto** — nothing hand-rolled, nothing touched. No auth, hashing, session, or JWT code in the diff.
- **Dependencies** — no new module and no version change. `docker.go` and `fakedocker_test.go` import only packages the module already requires (`docker/docker/{api/types,api/types/container,api/types/image,api/types/network,client,errdefs}`, `opencontainers/image-spec`). `git status --porcelain go.mod go.sum` is empty, so the root module's dependency set is untouched by this phase (the `deploy/caddy/ingress/go.{mod,sum}` changes in the working tree belong to another phase's review).
- **GCP surface** — none. Self-hosted project; `.agents/docs/deploy/gcp.md` records it as not applicable. No IAM binding, service account, Cloud Run flag, or Secret Manager reference anywhere in the diff.

## Not examined

- `govulncheck` was NOT run for this review. The project's gate is `deploy/vulncheck.sh`, which builds/pulls a pinned builder image and covers both modules — a whole-repo, pre-tag gate rather than a per-phase one — and this diff adds no dependency. It must still run before the release tag, as usual. `npm audit` is not applicable to a Go-only diff.
- The other working-tree changes (`internal/httpapi/`, `internal/terminal/bridge.go`, `internal/config/config.go`, `deploy/`, `web/`) are outside this diff; each has its own review file dated today.
- The Kata integration suite (`.agents/docs/testing/styrman/integration.sh`) and the browser e2e were not run — this review is the unit seam only, and no claim here is evidence about the real Engine.
- `internal/orchestrator/exec.go` is exercised only incidentally by the new tests (the fake always returns exit `f.execExit` immediately); the stdcopy demux against a hostile guest stream was not re-reviewed here.
- Whether the stale `.agents/scratch/phase5.diff` was also the artifact given to the code reviewer for this phase — if it was, that review's `&&` and data-race findings were already fixed and its coverage of the on-disk text is unproven.

## Response (main, 2026-09-10)

- Stale artifact — acknowledged (the reviewer's two fixes had already
  landed from the reviewer round); the on-disk files were reviewed.
- **Minimal fake (low) — documented, not extended.** The fake's doc
  comment now says exactly what is minimal (List stubs, Force-less Remove,
  duplicate Create) and that a test of `CleanupLegacy`, `Adopt`,
  `ResolveProjectByIP` or a non-Force remove must extend it first, so no
  such test can pass vacuously by accident.
- **Uncovered narrower rules (low) — two of three now asserted.** The
  stop-marker test asserts the marker is set when `ContainerStop` reaches
  Docker (an `onCall` hook) and that the clear happens only after exactly
  two inspects. The create-path `realBelow` re-check needs a home that
  appears between the first check and `create`; left as a noted gap.
