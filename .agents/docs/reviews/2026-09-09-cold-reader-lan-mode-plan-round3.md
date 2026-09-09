---
owner: reviewer
date: 2026-09-09
---
# Cold-reader review round 3 (final) — LAN mode by bridge prefix plan   (2026-09-09)

Document reviewed: `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md`
Round-1 and round-2 reports read:
`.agents/docs/reviews/2026-09-09-cold-reader-lan-mode-plan.md` (6 findings),
`.agents/docs/reviews/2026-09-09-cold-reader-lan-mode-plan-round2.md` (3 findings).
Code surfaces re-checked live against the current tree: `internal/config/config.go`,
`internal/config/bridgename_test.go`, `internal/orchestrator/orchestrator.go`
(`EnsureRunning` 172-321, `create` 795-894, `recreateMissingNetwork` 1212-1229,
`BridgeNameForNetwork` 375-396, `Destroy` 1097-1110), `internal/httpapi/network.go`
(full file), `internal/httpapi/server.go` (`netMu`/`netpol` fields, `deleteProject`
1019-1032, `projectView` 607-627, `lanFields` 630-, both `EnsureRunning` list/get
call sites ~683-696 and ~900-919, all 3 `EnsureRunning` call sites),
`internal/httpapi/explorer.go` (440-461), `internal/project/store.go` (full LAN
section), `internal/credbroker/broker.go` (`New`, `LanChecker`), `cmd/styrman/main.go`
(182), `REQUIREMENTS.md` lines 68-116, `web/src/lib/api.ts`, `web/src/lib/lan.ts`,
`web/src/lib/lan.test.ts`, `web/src/components/LanBadge.tsx`,
`web/src/components/LocalNetwork.tsx`, `web/src/routes/settings.tsx`,
`web/src/routes/index.tsx`, `web/src/components/Sidebar.tsx`.

## Verdict on rounds 1 and 2's nine findings: all nine genuinely resolved against live code

Round 1's six and round 2's three findings were each re-verified by reading the
actual current file, not just the plan's own claim:

- Mismatch-heal placement, `NetworkRemove` endpoint ordering, `pruneHostReachingCreds`,
  `ingressBr*`/`BridgeNameForNetwork`, and the R12 splice boundary (round 1,
  items 1-5): confirmed fixed in the plan text and consistent with the live
  `orchestrator.go`/`network.go`/`server.go`/`REQUIREMENTS.md:70-113` (item 13
  still begins at line 115, untouched).
- `bridgename_test.go` "append" wording (round 1 #6): still literally says
  "append" in Phase 1, still self-resolving via `go build` (two existing
  one-argument calls at lines 13 and 16 need editing, not just appending
  around). Carried forward as non-blocking a third time, consistent with
  rounds 1 and 2's own classification.
- `deleteProject`'s `netpol.SetMarker`/`Kick` block and `netMu`'s narrowed
  scope (round 2 #1): now covered by name at `server.go:1019-1032` in the
  Surfaces-touched table and by the dedicated "What `netMu` is for after this
  change" section. Live code at those exact lines matches what the plan
  describes stripping.
- `explorer.go`'s admin `hostFirewall` field and `settings.tsx`'s
  `HostFirewallSection` (round 2 #2): now named explicitly
  (`explorer.go:455-460`, `settings.tsx:949` + call site `:856-857` + import).
  Live `explorer.go:459` (`resp["hostFirewall"] = s.hostFirewall(...)`) and
  `settings.tsx:3,856-857,949` match the cited lines exactly.
- Toggle-handler timeout budget (round 2 #3): now stated explicitly (3 minutes,
  with the reasoning against `startProject`'s existing 2-minute precedent at
  `server.go:1104`).

## (1) Gaps — new, not raised by round 1 or round 2

- **A third UI surface that reads the deleted LAN state machine is absent from
  every touched-files list, and it is the most visible one.**
  `web/src/components/LanBadge.tsx` renders a small "LAN" chip next to every
  project's name in the sidebar (`Sidebar.tsx:289`, every row) and the
  dashboard (`routes/index.tsx:100`, every card) — i.e. it is on screen far
  more often than the settings-page switch the plan does redesign. It reads
  `project.lanState`/`lanMessage` through `lib/lan.ts`'s `lanChip()`
  (`LanBadge.tsx:9-19`, `lan.ts:25-43`), which implements the full five-state
  derivation (`off | pending | on | error | unavailable`) the plan explicitly
  kills ("503 / version handshake / pending / unavailable / error states are
  all DELETED. There is no root half to be unavailable.", Interfaces → API).
  None of `web/src/lib/lan.ts`, `web/src/lib/lan.test.ts`, or
  `web/src/components/LanBadge.tsx` appears anywhere in "Surfaces touched",
  "Removed identifiers", or Phase 4's task list — only `LocalNetwork.tsx` and
  `settings.tsx`'s `HostFirewallSection` are named.
  The matching Go-side surface is also unnamed: `internal/httpapi/server.go`'s
  `projectView` struct (607-627) carries `LanState`/`LanMessage` fields, filled
  at **two** call sites — the project-list handler (~683-696) and the
  single-project GET handler (~900-919) — both via `s.lanFields(...)`, which
  the plan's Removed-identifiers list DOES name for deletion. Deleting
  `lanFields` breaks both call sites (self-resolving via `go build`), but
  *what replaces `LanState`/`LanMessage` on the wire* — deleted fields, or a
  simple two-value derivative of `LanAccess` for the badge to key on — is a
  design decision the plan never makes, unlike `LocalNetwork.tsx`'s explicit
  "a two-state mode switch, a one-line explanation of each mode" guidance.
  Frontend-side this is **not** self-resolving via the compiler: `api.ts`'s
  `Project.lanState?`/`lanMessage?` are typed optional strings backed by a
  runtime JSON payload, not statically checked against the Go struct — if the
  backend stops sending them but the plan's stated `api.ts` edit (delete
  `HostFirewall`, `host?`, `hostFirewall?`) doesn't also touch `lanState`/
  `lanMessage`/`LanState` (the union type) or `lan.ts`, `LanBadge.tsx` keeps
  compiling and keeps rendering off `undefined` — silently wrong, not a build
  failure, exactly the failure shape this project's `writing-files` rule and
  BUG history warn about (a change that looks complete because nothing red
  flags it).
  This is the same class of miss round 1 caught three times and round 2 caught
  twice (a mechanism reached through a shared helper/type, not named
  directly) — a third round, on the same Surfaces-touched table, still missed
  one instance of it.

## (2) Non-blocking — an implementer could reasonably resolve these during the phase, but they are real

- **The API interface block contradicts the plan's own prose on the response
  field name.** Interfaces → API's code fence declares
  `-> 200 { "lanAccess": bool, "restarted": bool }` (plan line 263), but the
  prose four bullets later says explicitly "The response field is `rebuilt`,
  not `restarted`, because that is what happens" (line 282), and the
  Verification section's unit-test bullet still says
  "no-op `restarted:false`" (line 359) — three places, two different names,
  inside one document. An implementer who reads the whole API section (not
  just the top code fence) gets the right answer from the plan's own explicit
  correction, so this does not block starting work, but a first-pass
  implementer who copies the code-fence contract verbatim (the normal way to
  read an "Interfaces" section) or the Verification bullet's literal field
  name would write `restarted` and have to redo it once the prose is noticed
  — a one-line fix to the plan (update the code fence and the Verification
  bullet to `rebuilt`) removes the risk entirely.
- **`create()`'s network-rebuild snippet skips the existing `SelfContainer ==
  ""` guard.** `Destroy()` (`orchestrator.go:1107-1110`, the pattern the plan
  cites as precedent) only calls `NetworkDisconnect` when
  `o.cfg.SelfContainer != ""` (host-mode dev has no self-container to detach).
  The plan's pseudocode (line 232) calls
  `o.docker.NetworkDisconnect(ctx, netName, o.cfg.SelfContainer, true)`
  unconditionally. With an empty container ID this errors and is discarded
  (`_ =`), so `NetworkRemove` still runs and the outcome is likely harmless in
  practice, but it is a real, silent divergence from the precedent the plan
  itself cites as the reason no new code path is needed. Worth a one-line
  guard to match `Destroy()` exactly, not worth stopping for.

## Readiness — questions a first-pass implementer would still have to ask

1. What happens to `LanBadge.tsx`, `lib/lan.ts` (`lanChip`/`lanLine`), and
   `lib/lan.test.ts` once the five-state model they implement no longer
   exists server-side — does the sidebar/dashboard badge become a plain
   on/off chip keyed on `lanAccess`, or is it removed? And correspondingly,
   what does `projectView.LanState`/`LanMessage` (`server.go:607-627`,
   filled at two call sites) become once `lanFields` is deleted — dropped
   fields, or a simplified two-value replacement?

Everything rounds 1 and 2 asked is now answered by the plan text and confirmed
against the live code, so none of those nine questions are repeated here.

## Not examined

- `deploy/firewall/styrman-firewall.sh`, `install.sh`, `test.sh` (Phase 3) —
  not read in any of the three rounds; taken on the plan's word throughout.
- `web/e2e/run.mjs`'s LAN blocks (Phase 5) — not read this round either.
- Live `iptables -S` was not re-run in this round; the Docker per-bridge DROP
  and base-rule claims remain taken on the plan's own "verified live" text,
  as in rounds 1 and 2.
- `internal/credbroker/broker_test.go` beyond confirming `New`'s one caller —
  did not check every test helper for a stray `LanChecker` implementation.
- `web/e2e/run.mjs`, `.agents/docs/testing/styrman/integration.sh`'s full
  current LAN block content — only grepped for the field names, not read end
  to end.
- ADR-065/066 full text in `DECISIONS.md` — not re-read this round; round 2's
  verification of the "supersede in place" convention was taken as settled.

## Bottom line

Every finding from rounds 1 and 2 — nine in total — is genuinely fixed in the
plan text and confirmed against the live tree, not merely discussed. This
round found one new BLOCKING gap of the same recurring shape (a shared
helper/type reached by files the plan's own table doesn't name — this time
`LanBadge.tsx`/`lib/lan.ts`, the most visible LAN UI surface in the app, plus
`projectView.LanState`/`LanMessage` on the Go side) and two NON-BLOCKING loose
ends (a stale `restarted`/`rebuilt` field-name mismatch inside the plan's own
text, and a missing `SelfContainer==""` guard in the network-rebuild
pseudocode). This is the last round the workflow allows; the LanBadge gap
should be closed by a one-line addition to the plan (name the three files,
state the fields' fate) before implementation starts, rather than left for
Phase 4 to discover via a broken sidebar badge nobody's test catches.

design needs revision
implementation not ready
