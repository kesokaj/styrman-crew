---
owner: changelog
date: 2026-09-14
---
# Terminal WebSocket boot never attached server to project bridge     (date: 2026-09-14)

## Summary

The terminal-WebSocket boot callback (`ensureFor`) only called `EnsureRunning`, which does not self-attach the server to the project's Docker bridge. After every server restart, the first terminal of a stopped project ran its login shell against an unreachable credential broker address. The defect became visible after v1.30.1 (BUG-054's MOTD) when the broker timeout was printed on every affected terminal start. Fixed by introducing a new `wsBooter` interface and `wsBoot` function that boots the container then synchronously calls `EnsureSelfAttached` before the WebSocket upgrade — unlike the other boot paths (createProject/provisionRepo, startProject, LAN toggle), which attach in a goroutine after their response and so could not serve this one.

## Type

bugfix

## Files changed

- `internal/httpapi/server.go` — `ensureFor` calls new `wsBoot`; new `wsBooter` interface (EnsureRunning + EnsureSelfAttached, satisfied by *orchestrator.Orchestrator); new `wsBoot` function
- `internal/orchestrator/orchestrator.go` — comment clarifying which caller attaches when
- `internal/httpapi/wsboot_attach_test.go` — new tests: TestWSBootAttachesTheServerToTheProjectBridge (asserts EnsureSelfAttached is called), TestWSBootDoesNotAttachWhenTheBootFails
- `internal/terminal/ensure_before_upgrade_test.go` — new test: TestBootCallbackRunsBeforeTheUpgrade
- `.agents/rules/mechanism-reaches-every-caller.md` — BUG-060 paragraph added
- `AGENTS.md` — architecture bullet updated
- `.agents/docs/specs/styrman/deploy.md` — broker section expanded (when each path attaches)
- `.agents/docs/specs/styrman/guide.md` — provisioning paragraph updated
- `.agents/docs/testing/styrman/README.md` — Tier 1 enumeration and live-proof guidance updated

## Agents dispatched

explorer (goldfish diagnosis `.agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md`), verifier (`.agents/docs/testing/2026-09-14-wsboot-attach-verify.md` PASS, two rounds), reviewer (`.agents/docs/reviews/2026-09-14-review-wsboot-attach.md` APPROVE WITH NITS, one round converged), security (`.agents/docs/reviews/2026-09-14-security-wsboot-attach.md` FINDINGS all low: two rebutted in its Outcome, one pre-existing carried to the handoff), bugs (BUG-060 record + INDEX), documentation (four doc files).

## Verification

- Unit-verified: `go test ./...` passes; named tests (TestWSBootAttachesTheServerToTheProjectBridge, TestWSBootDoesNotAttachWhenTheBootFails, TestBootCallbackRunsBeforeTheUpgrade) green
- Live join (`docker network connect --ip 172.16.1.254 …`) not executed (permission classifier blocked)
- Live proof deferred to after next deploy: open a stopped project from the UI after server restart — no "broker did not answer" MOTD line; `gh auth status` succeeds

## Requirements

n/a

## Related

- BUG-060: `.agents/docs/bugs/BUG-060-terminal-ws-boot-never-joined-the-project-bridge.md` (root cause, live evidence, prevention rule)
- Plan: `.agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md`
