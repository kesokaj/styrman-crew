---
owner: reviewer
date: 2026-10-05
---
# Cold-reader check, Round 3 — v1.42.4 budget / fairness / cap   (2026-10-05)

Range reviewed: plan file `.agents/docs/plans/2026-10-05-v1-42-4-budget-fairness-cap.md` (untracked, 456 lines) and the code surfaces it names (HEAD 5563406). This report overwrites round 2.

## Status: COMPLETE

## Verdict: design needs revision / implementation not ready

Round 2's findings are addressed in the text: GET now reports capped values (D17), idle 0 is accepted by `UpdateOperator`, the 413 is gone from Phase 1, and the D16, D18 and D19 rows exist.

The worst remaining problem is a new one, and the D17 fix introduced it. The plan defines one `capped()` that maps **0 → cap** (plan :209, :220-223), and the GET now reports `capped(record)` (:229-231). The dialog ticks "Unlimited" only when `cpus === 0` (`web/src/lib/resources.ts:23-25`). So a non-admin's "Unlimited" project never opens as Unlimited. The planned "Unlimited (up to N)" label (:251-252) can never show ticked. Any later save, even a disk-only one, writes the cap number over the stored 0, which contradicts D5 ("lifting the cap restores the stored wish").

The second problem is D18's middleware read deadline. In Go 1.25 it does more than bound the body read: once the body reaches EOF, the deadline cancels the handler's `r.Context()` 30 s after it was set.

## Findings

### (1) Gaps

- **G1 [high / confidence: high] — `capped()` turns a non-admin "Unlimited" into a number, in GET and then in the record.**
  - Plan :209 maps "CPUs 0 or > cap → cap", and :220-223 says the same function serves start, GET and PUT. :229-231 makes the GET report `capped(record)`.
  - `resources.ts:23` sets `cpusUnlimited: r.cpus === 0` (`:25` does the same for memory), and `unchanged()` (`resources.ts:71-73`) compares the form against those GET values.
  - Result: a non-admin record of `{cpus:0}` opens as "N of N", not Unlimited. Saving the dialog for a disk change PUTs `cpus: N`, and the record then stores N. When an admin later raises the cap, that project does not grow (D5 is broken). The "Unlimited (up to N)" label (:251) is dead UI for exactly the owners it was written for.
  - Two implementers will build this differently: one follows "one capped() for GET", the other keeps 0 in the GET.
  - Decide it: for example, the GET reports `0` as `0` and clamps only values above the cap (a GET/PUT mapping that keeps 0), while the start closure and `WantSizing` input map 0 → cap. Then the plan must say which function each of the three sites calls.

- **G2 [medium / confidence: high] — D18's read deadline cancels the request context, not just the body read.**
  - In Go 1.25 `net/http`, once the body hits EOF the server starts a background read on the connection. When the deadline left on the connection fires, `connReader.backgroundRead` sees a timeout that `abortPendingRead` did not cause and calls `handleReadErrorLocked` → `cr.conn.cancelCtx()` (`$GOROOT/src/net/http/server.go:701-773`, toolchain go1.25.0).
  - So the middleware deadline (plan :110-115, D18) cancels `r.Context()` of every charged bodied request 30 s after it was set. This is the same mechanism the §4 test "a GET slot holder has no read deadline" (:262-264) guards for GETs, but the plan says nothing about it for bodied requests.
  - Today's exposure:
    - `gitCheckout` and `gitStash` derive their 30 s timeouts from `r.Context()` (`internal/httpapi/git.go:665`, `:784`) and both read a JSON body, so the two bounds now race.
    - Push, fetch and pull send no body (`web/src/lib/api.ts:803-818`), and the network and resources PUTs detach from `r.Context()` (`network.go:120`, `resources.go:142`), so those are safe today, but only by accident.
  - Decide one of two:
    - (a) Clear the deadline once the body reaches EOF (`SetReadDeadline(time.Time{})` from a body wrapper), so it bounds only the trickle the plan names (:113-114).
    - (b) State that every bodied charged handler's context is capped at 30 s, and add a test that pins it.
  - Either way, add a test: a bodied request whose handler runs past `slotReadTimeout` after the body is read either still completes (a) or is cancelled (b).

- **G3 [low / confidence: high] — `UpdateOperator` validates the whole result, but the loaders accept values outside those bounds.**
  - Plan :174-175: "validates the result (disk 5-500, idle 0 = never or >= 10m …)".
  - `STYRMAN_IDLE_TIMEOUT` is read with no lower bound (`internal/config/config.go:177`), and `loadOverrides` accepts any `d >= 0` (`config.go:316-319`).
  - A host with idle 5m (env or a hand-edited settings.json) would then have every runtime PATCH refused, including a change to the cap alone. Today's `SetOperator` validates only the fields it is given (`config.go:356-358`, `explorer.go:635-640`).
  - Fix: validate only the fields `fn` changed, or bring the loaders up to the same bounds (warn and fall back, as the disk loader does at `config.go:308-314`).

- **G4 [low / confidence: high] — a wrong line reference.** Plan :281 says `exit "${status}"` is at `run.sh:174`. The file has 154 lines, and the exit is at `web/e2e/run.sh:154`.

No other misunderstandings of existing code were found:
- The five `runOptsFor` callers plus the test caller `internal/httpapi/resources_test.go:407` (not listed; the compiler will catch it) are the only `EnsureRunning` paths (grep `EnsureRunning(` over `internal/` and `cmd/`).
- Size rebuilds touch stopped containers only (`orchestrator.go:386`), so the "trimmed at next start" claim holds.
- `Host()` takes its own `hostMu` (`orchestrator/resources.go:80-82`), so calling it inside the locked closure cannot deadlock.
- The caddy-security directives at `caddyfile_authn_misc.go:65-77` and `caddyfile_authz_misc.go:80-88` behave as the plan says. Every prefixed name is `<PREFIX>_{SESSION_ID,REDIRECT_URL,SANDBOX_ID,ID_TOKEN,ACCESS_TOKEN,REFRESH_TOKEN}` (`go-authcrunch@v1.1.41/pkg/authn/cookie/cookie_config.go:20-38`), and the strip regex `AUTHP[0-9A-F]*_[A-Z_]+` matches all of them.
- The server's own proxy already drops `Cookie` (`internal/httpapi/proxy.go:178`).

The plan's deferred question about git weights is answered by the code. `exec.go:193-194` demultiplexes stdout and stderr into one `cappedBuffer{max: maxExecOutput}`, so stderr is not buffered separately and the weight does not double. 14 MiB stands.

### (2) Scope

Nothing is built beyond the request without a D-row naming who asked:
- the `files.List` byte bound: D12
- the history controller: D13
- the read deadline on every bodied charged request: D18
- `token_file`: D15
- the GET capped values: D17

`limits.capped` and the "(up to N)" label express item 3's "Unlimited means up to the cap". The new `GET /api/settings` fields are needed by the editor that item 3 asks for. No second shape for an operation, and no configuration for constant values (D3 rejects it).

### (3) Readiness — questions an implementer would have to ask

1. **BLOCKING** (G1): For a non-admin record of 0, does the resources GET report 0 or the cap? Is `capped()` one function for start, GET and PUT, or two mappings (keep-0 for GET/PUT, 0→cap for start/`WantSizing`)?
2. **BLOCKING** (G2): Does the middleware's 30 s read deadline stay on the connection after the body is fully read? Staying cancels the handler's context at 30 s; clearing it does not.
3. NON-BLOCKING (G3): Does `UpdateOperator` reject a PATCH because an *unchanged* field (an env idle of 5m) is outside the API bounds? Default: validate changed fields only.
4. NON-BLOCKING: The weight table rows are shorthand (`GET /ws/*`, `.../file`, "every `/git/*` route"). The implementer expands them to the literal registered patterns (`server.go:432-520`), and the table-resolution test pins it.
5. NON-BLOCKING: BUG-091 must-fail (a) says "blur … before the FIRST `setIdentity`". The expected `DIAG` + `failed=0` happens only if the blur sits *after* the new focus wait. Blurring before the wait makes the wait time out outside the `try` (`run.mjs:1022-1026`), and the check fails instead. A reasonable implementer places it after the wait; the plan could say so in four words.
6. NON-BLOCKING: The `history/{id}` weight function must mirror the handler's parse (negative or non-numeric `?bytes` → 0 → 256 KiB, `explorer.go:539-546`). The plan says "absent/0" only.
7. NON-BLOCKING: Is the middleware read deadline set before or after the ≤ 2 s budget wait? Either works with a 30 s bound.

## Criteria (cold-read lens)
1. Dead code: pass. The free `runOptsFor`/`resourcesOf` and `SetOperator` are deleted, not left beside their replacements.
2. Flow tracing: fail. G1 (GET→dialog→PUT round trip loses "Unlimited"), and G2 (the deadline reaches the request context through net/http's background read).
3. Standards conformance: pass, apart from G4's line reference. Every task that changes an endpoint or command has a docs clause (api.md, guide.md, remote.md, deploy.md, README.md).
4. Directory layout: pass. `internal/membudget/`, `internal/httpapi/budget.go` and `web/src/lib/historySearch.ts` follow the repo's conventions.
5. Simplicity (DIY-except-security): pass. Stdlib only, no new dependency. The cookie hash is a `sha256` name derivation, not a crypto primitive.
6. Decoupling: pass. The budget is one middleware seam with a package of its own, and `sizeRuntime` is reused as the test seam.

## Not examined
- `internal/history/history.go:172-185` (the search memory profile), `credstore.go:307` and `orchestrator.go:1720`: the weight figures' sources were taken as cited.
- `web/src/routes/settings.tsx` `RuntimeEditor` (:965) and `.agents/docs/testing/styrman/README.md:409`.
- The `remote.md` and `deploy.md` token snippets.
- `deploy/caddy/ingress/generator.go:263-300` beyond the lines quoted.
- go-authcrunch `authz/config.go:84`: the `access_token_cookie_names` JSON field was not checked.
- The recon brief `plans/2026-10-05-recon-v1-42-4.md`.

design needs revision
implementation not ready
