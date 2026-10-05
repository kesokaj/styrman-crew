---
owner: reviewer
date: 2026-10-05
---
# Cold-reader check — v1.42.4 budget / fairness / cap   (2026-10-05)

Range reviewed: the plan file `.agents/docs/plans/2026-10-05-v1-42-4-budget-fairness-cap.md` (untracked, 316 lines), its recon `plans/2026-10-05-recon-v1-42-4.md`, and the code surfaces it names (HEAD 5563406)

## Status: COMPLETE

## Verdict: design needs revision / implementation not ready

The worst problem is that the BUG-074 fix opens a session-token leak. The plan swaps the single strip regex for this deployment's new cookie name, but sessions last 8 hours and the signing key is pinned. So the old `AUTHP_ACCESS_TOKEN` cookie stays valid for the whole rollout window. A parent-domain deployment's renamed cookie stays valid indefinitely. Both are forwarded into guest apps on preview and public hosts, and an app operator can replay them under the new name. The second blocker is that `ownerIsAdmin` is keyed on `cfg.DevAddr`. The e2e and Tier 2 servers run `AuthMode=none` on the main listener with no DevAddr set, so their projects would be capped at half the host. Tier 2's "unlimited sees every host CPU" checks would go red, and on the same server a PUT (judged by the requester's `Admin`) and a start (judged by `ownerIsAdmin`) would disagree. Three more items are undecided: the PUT rebuild decision, the "unchanged" sentinel in the settings struct, and the history-panel test (it needs a DOM, and the repo has no DOM test setup).

## (1) Gaps

- **G1 [high / confidence: high]** BUG-074: the strip regex no longer covers the old cookie name or other deployments' names. `generator.go:152` and `:437` strip exactly `AuthCookieName`. The plan (§8, last line) changes both to `AuthCookieName(cfg.BaseDomain)` and states (Failure modes) that the old cookie "is ignored". That holds for authz, but it does not hold for the strip. Sessions last 8 h (`generator.go:294`, `SessionLifetimeSeconds`) and are signed with the pinned `STYRMAN_JWT_KEY` (`generator.go:297`). After the rollout, a browser keeps sending its old `AUTHP_ACCESS_TOKEN` (domain = base) to every `*.preview` and public-app host for up to 8 h. The strip no longer removes it, so the guest app receives a valid JWT that it can replay as `AUTHP<hash>_ACCESS_TOKEN`. The new name is derivable from the public base domain. In BUG-074's own topology (`BUG-074…md:35-36`: NUC cookie domain `styrman.xyz`, GCP `gcp.styrman.xyz`), the NUC's renamed cookie is sent to every GCP preview host, and the GCP strip does not remove it. Today both strip the same name, so this leak is new. Fix to decide: strip the whole family, e.g. `(?:^|;\s*)AUTHP[0-9A-F]*_ACCESS_TOKEN=[^;]*`. The generator test should assert that the old name, this deployment's name and a foreign hash are all stripped.
- **G2 [high / confidence: high]** The plan misreads how the dev door works. §3 says `ownerIsAdmin` treats "the dev door (cfg.DevAddr set)" as admin "matching authGuard mode none (auth.go:91-92)". But mode "none" is the main listener's DEFAULT (`config.go:180` `STYRMAN_AUTH` default "none"; `server.go:425` `Routes()` uses `s.cfg.AuthMode`). `web/e2e/run.sh:135` and Tier 2 (`integration.sh:26,50`) start their server with neither `STYRMAN_DEV_ADDR` nor `STYRMAN_ALLOWED_EMAILS`. There, `DefaultUser()` returns `c.Username` (`config.go:447-451`), which `IsAdminUsername` does not know. Result: those projects are capped at half the host, and `integration.sh` "unlimited: the guest sees every host CPU" / "MemTotal ≥ 97%" fail. The PUT (requester `Admin`=true, `auth.go:91`) would also accept values that the start then clamps. The rule should key on `AuthMode=="none"` (both listeners), not DevAddr. The Scope line "both run as the dev door's admin" carries the same misreading.
- **G3 [medium / high]** The PUT's rebuild decision and its reply are not capped. `resources.go:194` `want := s.size.WantSizing(h, req, live)` decides between stop/persist/start and persist-only (`:199`), and `:195` replies `Limits: limitsFor(h, …)`. The plan caps only the GET's limits and `pending` and the start closure. Take a non-admin running at the cap who PUTs `0`: `want` is the whole host, which differs from `live`, so the project is stopped and rebuilt, then restarted at the same capped size. The reply's limits also show the host. The plan needs to say that the PUT computes `want` and `Limits` from the capped request.
- **G4 [medium / high]** `OperatorSettings` cannot express what the PATCH promises. The PATCH says "missing field = unchanged" and `0` = half the host. The struct's `UserMaxCPUs, UserMaxMemoryGiB int` leave no third state. The existing `SetOperator` uses "0 = keep" for disk (`config.go:354,366`). An implementer who copies that pattern makes "reset to half" impossible. The plan needs a sentinel (`-1`, like idle) or pointer fields. `operatorOverrides` also uses `omitempty` (`config.go:284`), which is compatible only if 0 is written as absent.
- **G5 [medium / medium]** "Everything else = small JSON" (weight table, last row) was never checked against guest-controlled lists. `listFiles` returns up to `maxListEntries=5000` (`files.go:43`) entries. Each `Entry` has `Name` and a full `Path` (`files.go:54-61`). A guest-planted directory of 5000 long control-byte names JSON-escapes 6× into tens of MiB, against a 2 MiB reservation. `listHistory` (one entry per transcript file, no count cap per the recon) and `credentialContent` are the same. The budget exists to stop guest-planted data from OOMing the server, so these routes need their own rows or a stated bound.
- **G6 [medium / high]** One verify clause names a fixture the code cannot provide. "tests for each of the five callers' path (fake orchestrator)" (Phase 2) does not work for `server.go:997` and `:1192`: they call `s.orch.EnsureRunning` on the concrete `*orchestrator.Orchestrator` (`server.go:143`), and there is no fake for it. Also, §3 has the closure use `s.orch.Host(ctx)`, but `RunOpts.Resources` is `func() (Resources, error)` with no ctx (`orchestrator.go:196`), and `runOptsFor` gets none. The testable seam is `s.size.Host` (`sizeRuntime`, `server.go:96-100`). The plan should say which ctx and which seam, and test the method itself rather than five callers.
- **G7 [medium / high]** The BUG-091 closure criterion can pass without the fix. "closed after two green e2e runs" is weak: the fixture already retries (`run.mjs:1028-1034`), so a run whose first attempt lost keystrokes still ends green and prints only `DIAG BUG-091`. The bug was intermittent before the change, too. The criterion should be "no `DIAG BUG-091` line in N runs", plus the must-fail run the rule requires.
- **G8 [medium / high]** The history-panel test rests on an unchecked assumption. "Edge — clearing during a search leaves the result list empty (vitest)" assumes vitest can render `SidePanelHistory`. It cannot: `web/package.json` has no jsdom, happy-dom or testing-library, and every existing test is a pure `src/lib/*.test.ts`. Either a new dependency (unjustified, stdlib/dependency rule) or extracting the search state into `src/lib/` (the repo's pattern). The plan does not decide. Also, `cancelSearch` bumps `searchGen`, so the aborted search's `finally` (`SidePanelHistory.tsx:51-53`) no longer resets `searching`. The plan's `cancelSearch` must reset it itself.
- **G9 [medium / medium]** A failure mode is missing: one user can hit 2-per-tenant file-slot fairness alone. `fileSlots` is shared by read, save, format and check. The editor's idle linter (`FileEditor.tsx:186-192`, one check in flight per editor), a save (`:252`), a format (`:238`) and a second pane or tab reading a file add up to 3 or more concurrent requests from one person. Under the new rule their save gets 429 "you already have 2 of these running". The plan lists no client handling (retry, or leaving checks out of the share), and it does not check whether e2e's single tenant now trips it.
- **G10 [low / high]** Some token-tooling work already exists, and the plan misses a scope edge. `styrman-ctl:569` already has `[ -n "${token}" ] || die "empty GitHub token"`, which covers both the prompt and a `--gh-token-file` read in `setup_auto_update`. So "the interactive prompt (:600) refuses an empty answer" is already done (line 600 is the temp-file write). Putting the refusal in `private_file` also changes `--env-file` (`:472`) and the signers file (`:571`), which the plan does not name.
- **G11 [low / high]** Missing docs clause: Phase 3's e2e task has none, but `testing/styrman/README.md:409` says "`run.sh` prints the one it used". It changes once a green run deletes the dir (rule tests-describe-behaviour / describing-text).
- **G12 [low / high]** Callers left out of Surfaces: replacing `SetOperator(diskGB, idle)` breaks `internal/orchestrator/resources_start_test.go:23` and `resources_test.go:24`, and neither is listed.
- **G13 [low / medium]** The memory cap is undefined on a host where `MaxMemoryGiB()==0` (`orchestrator/resources.go:46-57`). `half = max(1, 0/2) = 1` contradicts "never above the host maximum". The PUT's "0 or 1..cap" would then accept `1`, which the existing check refuses (`resources.go:155-157`).

Assumptions I checked and found to hold: `set cookie name prefix` renames the portal's access cookie to `<PREFIX>_ACCESS_TOKEN` and sets its validator (`caddyfile_authn_misc.go:65-77`; go-authcrunch `cookie_config.go:35` `DefaultAccessTokenCookieName = "ACCESS_TOKEN"`). The authz `set access_token cookie name` exists (`caddyfile_authz_misc.go:80-88`). The authz redirect uses a query parameter, not a cookie (`authz/config.go:177`), so renaming the referer cookie breaks nothing. `h.MaxMemoryGiB()` is what `limitsFor` shows (`resources.go:63`). The middleware order (csrfGuard outer, so the body is already wrapped) is correct.

## (2) Scope beyond `## Why`

Each item below has no D-row naming who asked for it:
- `GET /api/settings` returns BOTH the configured (`userMaxCpus`) and the effective (`userCapCpus`) values. That is a second shape for one fact, and the Web section consumes only `limits.capped` and the configured values.
- `decodeJSON` maps `*http.MaxBytesError` to **413**. Item 4 asked only for 408. Today an oversize body is 400 on writeFile, so this is an unrequested API change (`api.md` rows).
- `Budget.InUse()` for "the debug log". No debug log is specified anywhere else.
- Settings → Runtime read-only cap rows for non-admins (item 3 names admin settings only). This one is minor.

## (3) Readiness — questions I would have to ask

BLOCKING (two careful implementers would build different things):
1. `ownerIsAdmin`: does it key on `AuthMode=="none"` or on `cfg.DevAddr`? See G2; the Tier 2 and e2e results depend on it.
2. User-cap "unchanged" sentinel in `OperatorSettings`: `-1` or pointers? See G4.
3. PUT `/resources`: are the rebuild-decision `want` and the reply `Limits` computed from the capped request? See G3.
4. BUG-074 strip: this deployment's name only, or the `AUTHP*_ACCESS_TOKEN` family including the legacy name? See G1.
5. History-panel test: add jsdom and testing-library, or extract a pure `src/lib` module? See G8.

NON-BLOCKING (a default is stated, or an implementer can decide alone):
6. Cross-tenant ordering in `membudget`. The "other tenants are admitted" line implies per-tenant FIFO only.
7. How a test forces the budget small, given the constants (a `Server` field to inject a `*membudget.Budget`).
8. `body(r)` for chunked bodies: extract csrfGuard's per-path cap switch (`server.go:704-717`) into a shared function.
9. The `GET .../history/{id}` weight when `?bytes` is absent or 0: `history.Read` defaults to 256 KiB, and the formula gives 1 MiB.
10. Whether `GET /api/git/summary` falls under "every `.../git/*` route".
11. Which ctx the resources closure uses for `Host`, and that it goes through `s.size` (G6).
12. Whether SHOTS is removed in the EXIT trap or before `exit "${status}"` (`run.sh:174`).
13. How the test observes "a GET slot holder has no read deadline". Likely: a GET handler that outlives `slotReadTimeout` keeps a live `r.Context()`, since net/http's background read cancels it on the deadline.
14. Every `GET /api/settings` now calls `Host` (`docker info`, cached only after the first success).
15. Client behaviour when a save gets the new per-tenant 429 (G9).

## Not examined
- `history.List`/`Search` byte bounds, `credstore` value size cap, and `orchestrator.Logs` line-length cap (G5 stands on `files.List`).
- `format.go`/`check.go` decode sites and existing slot tests by name.
- `generator.go:263-300` beyond the lines quoted.
- `deploy.md:256-269`.
- `ResourcesDialog.tsx`, `resources.ts`, `settings.tsx`.
- `deploy/update/styrman-update`'s calling context for `fail` beyond its definition (`:61`, exit 1).
- `caddy adapt` itself was not run.
- No text in the files read was addressed to the reviewer.
