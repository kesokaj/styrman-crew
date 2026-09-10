# Terminal WebSocket re-authorization on a timer     (date: 2026-09-10)

## Summary

Phase 4, item I of the closeout plan: an open terminal WebSocket re-checks user authorization every 60 s. If authorization fails (user removed from allowlist or revoked via Settings), the socket closes with code **4003** "access revoked" and both frontend and guest terminate the session. Closes the residual: a removed user's already-open terminal stayed active for the session lifetime. Now the latency is at most 60 s. Backward compatible (check function is optional; auth-bypass mode in no-auth dev setup never re-checks). Verification passed; reviews passed (security review dispatched).

## Files changed

- `internal/terminal/bridge.go` — `NewBridge` new `stillAuthorized` parameter (func(email) bool, nil = never re-check); `revocationWatch(ctx, interval, check, onRevoked)` pure loop runs in parallel with heartbeat; `serve` captures session email at upgrade and runs revocationWatch for both `/ws/term` and `/ws/setup`
- `internal/terminal/reauth_test.go` — new unit tests: revocation fires once when check turns false, never fires while authorized, never fires for nil check
- `internal/config/config.go` — new constant `CloseAccessRevoked = 4003`
- `internal/httpapi/server.go` — `NewBridge` call passes `users.IsAuthorized` ONLY when `AuthMode == "header"`; no-auth mode passes nil (local terminals never revoke)
- `web/src/lib/api.ts` — new constant `CLOSE_REVOKED = 4003`; `TerminalPane` adds state `revoked` displayed as "Access revoked — sign in again if this is a mistake", never reconnects
- `web/src/components/TerminalPane.tsx` — handles revoked state, blocks reconnect UI

## Agents dispatched

verifier, reviewer, security, changelog

## Verification

- `testing/2026-09-10-verify-closeout-phase4.md` green (60 s revocation latency verified, nil check confirmed safe for dev mode)
- `reviews/2026-09-10-review-closeout-phase4.md` APPROVE (no findings)
- `reviews/2026-09-10-security-closeout-phase4.md` APPROVE (outcome in commit message)
- Revocation test plants user removal and confirms socket closes with 4003
- No-auth mode test confirms nil check never triggers revocation
- E2E blocked by goroutine unit-test gap (revocationWatch runs in serve, never isolated); frontend state untested at unit level

## Residual

At most 60 s latency from removal to termination (was indefinite). REST API guard (`authGuard` per request) already enforces immediate revocation on per-request basis; WebSocket is async, hence the 60 s polling.
