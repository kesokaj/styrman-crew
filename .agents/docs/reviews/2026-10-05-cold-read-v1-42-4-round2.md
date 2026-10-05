---
owner: reviewer
date: 2026-10-05
---
# Cold-reader check, Round 2 — v1.42.4 budget / fairness / cap   (2026-10-05)

Range reviewed: the plan file `.agents/docs/plans/2026-10-05-v1-42-4-budget-fairness-cap.md` (untracked, 412 lines, revised after round 1), and the code surfaces it names (HEAD 5563406). This report overwrites round 1.

## Status: COMPLETE

## Verdict: design needs revision / implementation not ready

Round 1's thirteen gaps are all addressed in the text: the strip covers the whole cookie family, `ownerIsAdmin` mirrors authGuard, there is one `capped()`, PATCH fields are pointers, there are list/history/credential weights, the `s.size` seam is named, there is a DIAG criterion and a pure controller, plus D7, `token_file`, the README clause, the test callers and the `MaxMemoryGiB()==0` case.

The worst remaining problem is the admin cap's effect on projects whose record is already above the cap. GET still returns the record's `cpus` (`resources.go:89`), while `limits.maxCpus` becomes the cap. The dialog fills its form from the record (`web/src/lib/resources.ts:25`) and refuses any value above `limits.maxCpus` (`:47`). The new PUT returns 400 for the same values. So a non-admin with a pre-cap 8-CPU record cannot save any change, the disk included, without first lowering the CPUs. The plan never decides what GET reports for such a record.

Two more problems:
- `UpdateOperator` validates `idle >= 10m`, but today `0` means "never" (`explorer.go:637`). An operator with idle 0 could then change no setting at all.
- The Phase 1 task still says `decodeJSON 408/413/400`, while D14 says "no 413".

## (1) Gaps

- **G1 [medium / high]** The plan does not decide how GET and PUT treat a record above the cap.
  - `projectResources` returns `CPUs: p.CPUs, MemoryGiB: p.MemoryGiB` from the record (`internal/httpapi/resources.go:89`). The plan makes `limits.maxCpus` the cap and keeps the record unchanged ("lifting the cap restores the stored wish").
  - `web/src/lib/resources.ts:25` fills the form from `r.cpus`. `:47` refuses `n > limits.maxCpus`. `ResourcesDialog.tsx:94-97` would show "8 … of 4".
  - The new PUT returns 400 for `cpus > cap`. Any edit that re-sends the stored value (changing only the disk, for example) fails.
  - This hits every non-admin project sized above the new default (half the host) on the day it ships.
  - To decide: either GET reports `capped(record)` values (the simplest), or the PUT accepts an unchanged over-cap field. Add a test: an over-cap record, then a disk-only PUT, expect 200.
- **G2 [medium / high]** `UpdateOperator` (plan :155-157) validates "idle >= 10m". The existing rule is "0 (never) or at least 10m" (`internal/httpapi/explorer.go:637`), and `IdleTimeoutD` documents 0 = never (`config.go:347`).
  - The function validates the result of `fn` applied to a copy of all current settings. So taken literally, an operator whose idle timeout is 0 gets 400 on every PATCH, even one that only changes the caps.
  - Fix: "idle 0 or >= 10m". Add a test that sets the caps while idle is 0.
- **G3 [medium / high]** The plan contradicts itself on 413.
  - Phase 1 task (:362) says "`decodeJSON` 408/413/400 + real-server test".
  - D14 (:406) says "No 413 mapping", and §4 (:231-233) says 408, else 400.
  - An implementer who works from the task checklist adds the 413 that round 1 struck as unrequested.
- **G4 [medium / medium]** The budget's fairness claim does not hold.
  - Failure modes (:307-308) and §1 (:81-82) say that a tenant at its share never blocks another tenant. That is true only while at most 3 tenants sit at their share: `budgetTotal = 4 × budgetPerTenant`, and REQUIREMENTS.md:11 allows 5 users. Four tenants at their share fill the budget, and the fifth gets 503.
  - Reaching the share costs a tenant nothing:
    - A reservation lives for the whole request, including the body read.
    - §4 sets a read deadline only on slot holders. The server itself has only a header timeout (`explorer.go:472-476`).
    - `body(r)` uses `r.ContentLength` with no cap (:102). One request with a forged `Content-Length` to a default route therefore reserves the full 256 MiB share (after clamping), yet can read at most 1 MiB, the csrfGuard cap at `server.go:704-717`. GET bodies are not capped by csrfGuard at all (`server.go:700`).
  - Fixes:
    - `body(r) = min(ContentLength, bodyCap(path))`.
    - Either state the 4-tenant limit as accepted (a D-row) or size `perTenant` below `total/5`.
    - Decide whether a reservation held while a body is read gets a deadline.
- **G5 [medium / high]** Some weights are lower than the handlers' own peak memory, so the reservation underprices the request.
  - `GET .../history/search` is priced at 8 MiB ("scan buffers"). `searchOne` (`internal/history/history.go:172-185`) holds the 4 MiB tail `buf`, `string(buf)`, the ANSI-stripped copy, two `ReplaceAll` copies, and `strings.ToLower(text)`. `strings.Map` writes a 3-byte U+FFFD for each invalid byte, so a 4 MiB guest-written tail of non-UTF-8 data lowers to 12 MiB. The peak is about 25-30 MiB. The code's own comment (`explorer.go:443-447`) says "several MiB per transcript (the 4 MiB tail and its cleaned, lowered copies)".
  - `history/{id}` (7n + 1 MiB) leaves out the raw and stripped copies.
  - `GET /api/git/summary` is priced as one exec, but it runs 4 execs at once (`git.go:972`), each up to `maxExecOutput`.
  - The slot pools still bound the real total, so this is an accounting error, not an OOM by itself. But the budget's purpose is to be an upper bound. Price search at >= 32 MiB and summary at >= 4 × exec.
- **G6 [medium / medium]** The plan does not decide what triggers a history search. "`SidePanelHistory.tsx` calls `run` from the input" (:270) has two readings:
  - Today a search runs only on Enter (`SidePanelHistory.tsx:83`); `onChange` only sets the query. Calling `run` from `onChange` would be search-as-you-type, which nobody asked for, and under the 2-search share it fires 429s while the user types.
  - Calling it on Enter means that emptying the input with backspace does not cancel, but item 6 asks that "clearing … cancels".
  - The clear button renders only when `matches !== null` (`:91-109`), so there is nothing to press during a first search.
  - To decide: Enter calls `run(q)`; `onChange` to "" calls `cancel()`; and what renders (Searching…, clear) during a first search.
- **G7 [medium / medium]** The BUG-091 must-fail run (:279-281) has no stated expected outcome.
  - Forcing "the identity check red" makes the commit and stash checks that depend on it fail too (`run.mjs:1009-1013`: "failing six checks later"). So the run gives `failed>1` by construction.
  - Rule `e2e-checks-leave-the-suite-as-they-found-it` treats `failed=1` as the pass mark and anything more as a defect in the check.
  - The plan does not say how the failure is forced or which count passes:
    - Lose only the first attempt: expect a `DIAG BUG-091` line and `failed=0`. This proves the retry restores the identity.
    - Lose both attempts: expect `failed=N`, with the dependent checks named.
- **G8 [medium / medium]** D7 (:399) changes a number the operator confirmed: Why item 2 says "at most 2 slots", and the operator said "go all". D7 makes it half of each pool, `by: cold read round 1 G9`. The global AGENTS.md says the operator decides; a reviewer's finding is not the operator's yes. Failure modes (:310) still says "you already have 2", which is a stale restatement. Get the operator's confirmation and fix :310.
- **G9 [low / high]** The plan reads a field that does not exist. The closure uses `!s.ownerIsAdmin(sp.user)` (:187), but `userSpace` is `{projects, terms, profile}` (`server.go:48-52`). `runOptsFor` needs a `user string` parameter. All five callers have one: `ensureFor`'s `user`; `u.Username` at `network.go:230`, `resources.go:234`, `server.go:997` and `server.go:1192`. The compiler will catch it, but the stated signature cannot be implemented.
- **G10 [low / high]** Scope edge: the setup VM is not covered. It is created at the whole host for every user, non-admins included (`orchestrator.go:834-840`, `WantSizing(host, Resources{}, nil)`). The plan does not say whether the cap applies to it. The literal reading ("non-admin projects") leaves it uncapped, which is defensible, but the plan should say so in Scope.
- **G11 [low / high]** The BUG-074 verify clause names the wrong JSON field (:345).
  - `access_token_cookie_name` is the portal's and token-grantor's field (go-authcrunch `authn/cookie/cookie_config.go:54`, `authz/options/options.go:31`). The authz policy's field is `access_token_cookie_names`, an array (`authz/config.go:84`).
  - A grep for the singular passes on the portal alone, and it also substring-matches the plural. So it does not prove the policy carries the new name, and the policy is the half that signs every user out for good if it is wrong.
  - Fix: assert `cookie_name_prefix` in the portal and `access_token_cookie_names: ["AUTHP<hash>_ACCESS_TOKEN"]` in each policy.
  - For ADR-088: with the names set, the gatekeeper also accepts a header and a query parameter named `authp<hash>_access_token` (`authz/gatekeeper.go:135-139`).
- **G12 [low / high]** Token check: `[ ! -s ]` misses a file that holds only a newline (from `echo > f`). `styrman-ctl` `run_host_setup` (:446) turns that file into an empty `GH_TOKEN`. Test for emptiness after `tr -d '\n'`, as `setup_auto_update` already does (:569).
- **G13 [low / high]** A docs clause is missing. The `files.List` byte-bound task (Phase 1, :360) has none, but `specs/styrman/api.md:166` describes the listing as "capped at 5000 entries"; after this change that is incomplete.
- **G14 [low / medium]** The PATCH semantics for `0` and `""` are unstated. The client always sends both fields: `dockerDiskGB: Number(disk)` and `idleTimeout: idle.trim()` (`settings.tsx` RuntimeEditor). Today 0 and "" mean "keep". Once the fields are pointers, an emptied disk field (`Number("") = 0`) returns 400 and "" fails to parse. The plan should say whether these two keep their "keep" meaning.

Assumptions checked and found to hold:
- `set cookie name prefix` renames all six portal cookies and the portal validator to `<PREFIX>_<NAME>` (`caddyfile_authn_misc.go:65-77`). The proposed strip regex `AUTHP[0-9A-F]*_[A-Z_]+` matches every default suffix (`cookie_config.go:22-37`) and the legacy names.
- The five `runOptsFor` callers are the only `EnsureRunning` paths for projects.
- `MaxMemoryGiB()` is the dialog's maximum (`resources.go:63`).
- The weight-table patterns exist as registered (`server.go:431-574`).
- `/link`, `/shared`, `/proxy` and `/p` are registered for every method, so their weight-0 rows must be keyed on all methods, not only GET.

## (2) Scope beyond `## Why`

- D7 changes the operator's number (G8); this is the one scope deviation without an operator D-row.
- Everything else the plan adds has a D-row: the `files.List` byte bound (D12), the controller (D13), pointer PATCH fields (D11), the cookie-family strip (D8). `limits.capped`, the "Unlimited (up to N)" label and `Retry-After: 1` are presentation of what item 3 and item 1 ask for. I found no extra endpoint and no configuration for values that never vary.

## (3) Readiness — questions I would have to ask

BLOCKING (two careful implementers would build different things):
1. For an over-cap record, does GET report the record or `capped(record)`, and does the PUT accept an unchanged over-cap field? (G1)
2. Does `UpdateOperator` validate idle as ">= 10m" or as "0 or >= 10m"? (G2)
3. Is there a 413 mapping or not: the task line or D14? (G3)
4. History search: Enter only, or `onChange`? Does emptying the input cancel? What renders during a first search? (G6)
5. BUG-091 must-fail: how is the failure forced, and which `failed=` count passes? (G7)
6. Operator confirmation of D7's per-tenant shares (4/4/2) instead of "2". (G8)

NON-BLOCKING (a default is stated, or an implementer can decide alone):
7. `body(r)` should be `min(ContentLength, cap)`, and the 4-tenant budget limit should be accepted or resized. (G4)
8. The exact values for the search, history-read and git-summary weights. (G5)
9. `runOptsFor` gains a `user` parameter. (G9)
10. The setup VM stays uncapped. (G10)
11. The caddy-adapt assertion fields. (G11)
12. The token emptiness check after newline stripping. (G12)
13. The api.md listing row. (G13)
14. PATCH `0` / `""` keep their "keep" meaning for disk and idle. (G14)
15. How the "every table pattern resolves" test builds a request from a wildcard pattern: substitute a literal for each `{x}`, then compare against `mux.Handler`'s returned pattern.

## Not examined

- The BUG-074 and BUG-091 records themselves; the recon file `plans/2026-10-05-recon-v1-42-4.md` (not read this round).
- `format.go` and `check.go` decode sites; `explorer.go:329` (multipart streaming) and the upload weight.
- The `credstore` value path behind `credentialContent`, beyond the 256 KiB file cap; `orchestrator.Exec` (whether stderr is buffered too, which would double git weights).
- `.agents/docs/specs/styrman/deploy.md:256-269` and `deploy/remote.md:81-96` (the token snippet).
- `internal/orchestrator/resources_start_test.go` and `resources_test.go` callers (taken from round 1).
- `caddy adapt` was not run.
- The new untracked file `.agents/docs/reviews/2026-10-05-cold-read-v1-42-4-round1.md` (written by another agent during this run) was seen in a grep but not read.
- No text in the files read was addressed to the reviewer.
