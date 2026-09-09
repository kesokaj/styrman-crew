---
owner: reviewer
date: 2026-09-09
---
# Cold-reader review round 2 — LAN mode by bridge prefix plan   (2026-09-09)

Document reviewed: `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md`
Round-1 review read: `.agents/docs/reviews/2026-09-09-cold-reader-lan-mode-plan.md`
Code surfaces re-checked: `internal/config/config.go`, `internal/config/bridgename_test.go`,
`internal/orchestrator/orchestrator.go` (full `EnsureRunning`, `create`,
`recreateMissingNetwork`, `Stop`/`stopLocked`, `Destroy`, `BridgeNameForNetwork`),
`internal/httpapi/network.go`, `internal/httpapi/server.go`, `internal/httpapi/explorer.go`,
`internal/project/store.go`, `internal/credbroker/broker.go`, `cmd/styrman/main.go`,
`REQUIREMENTS.md` lines 60-120, `.agents/docs/architecture/DECISIONS.md` (ADR-036,
ADR-065/066 entries, the project's "supersede in place" convention), `ROADMAP.md`,
`TODO.md`, `web/src/lib/api.ts`, `web/src/routes/settings.tsx`.

## Verdict on round 1's six findings: all six are genuinely resolved, not just discussed

1. **Mismatch-heal in the wrong code path.** Fixed correctly, not just
   patched over. The plan now adds a THIRD force-recreate condition inside
   `EnsureRunning`, placed and guarded exactly like the existing image-upgrade
   block (`orchestrator.go:206-217`) — `err == nil && stopped` — which force-
   removes the container and sets `err = errdefs.NotFound(...)`, driving the
   switch into the `create()` branch. Traced against the live file: this is
   structurally sound and reuses the established pattern; it does not merely
   restart the same container on the same bridge as the first draft did.
2. **`NetworkRemove` fails while any endpoint remains.** Fixed by ordering:
   branch (a) `ContainerRemove`s the project's own container (which drops its
   endpoint) BEFORE `create()`'s network-rebuild step (b) ever runs, so by
   the time (b) inspects the network only the server's own self-attach can
   remain — exactly what (b)'s `NetworkDisconnect`+`NetworkRemove` handles.
   Verified against the actual `create()` body (`orchestrator.go:795-822`,
   which today does exactly the same `NetworkInspect` → not-found → create
   shape this snippet replaces).
3. **`pruneHostReachingCreds`.** Now named explicitly in "Removed
   identifiers" and Phase 2's task list, with its call site and
   `lanprune_test.go`. Confirmed live at `network.go:245,270` — matches.
4. **`ingressBr`/`ingressBrMu`/`ingressBrAt`/`BridgeNameForNetwork`.** Now
   named explicitly in "Removed identifiers" and Phase 2. Confirmed live at
   `server.go:112-116` and `orchestrator.go:375-396` — matches.
5. **R12 splice boundary ambiguous.** Fixed: Phase 0 now names exact line
   numbers (70-113) and Appendix A reproduces the full replacement text
   verbatim. Checked against the live file: line 70 is exactly
   `12. **R12 — Project network isolation**...` and line 113 is exactly
   `clause are unconditional.`; line 115 is item 13's first line, untouched.
   The boundary is a clean item-to-item replacement, not mid-sentence.
6. **`BridgeNameFor` signature break in existing test calls.** Not literally
   fixed in wording (Phase 1 still says "append" for `bridgename_test.go`,
   which also has two EXISTING one-argument calls that must be edited, not
   just appended to) — but this remains self-resolving via `go build`, as
   round 1 already noted. Not reclassified as a blocking gap.

Also independently verified and found consistent (not gaps): the DECISIONS.md
"supersede in place, keep old wording as history" pattern the plan proposes
for ADR-067 matches the project's own established convention (see e.g. the
ADR-066 entry itself, which does exactly this to ADR-065's part 4); and the
three Go call sites of `EnsureRunning` in `server.go` (229, 872, 1082) match
the plan's "3 call sites" claim.

## (1) Gaps — new, not raised by round 1

- **Two whole surfaces that reference the deleted `netpolicy` machinery are
  absent from "Surfaces touched" and every removed-identifiers list.**
  Traced by grepping every `s.netpol.` / `netpolicy.` caller, which the plan's
  own table does not cover completely:
  - `internal/httpapi/server.go:1019-1032` — `deleteProject`'s LAN-marker
    cleanup, under `s.netMu`, calls `s.netpol.SetMarker(p.ID, false)` and
    `s.netpol.Kick(p.ID)` before removing the project record. Once `netpol`
    is deleted this block cannot compile as-is, and the plan does not say
    what replaces it (delete the block outright? something else?) or what
    becomes of `s.netMu`'s scope here, which the comment says exists
    specifically "so a concurrent PUT cannot re-create the marker between
    its removal and the record's."
  - `internal/httpapi/explorer.go:455-460` — the admin-only `/settings`-style
    info endpoint puts `resp["hostFirewall"] = s.hostFirewall(s.netpol.Status(now), now)`
    into its response. `explorer.go` is named nowhere in the plan.
  - Downstream of that: `web/src/lib/api.ts:256` (`hostFirewall?: HostFirewall`)
    and `web/src/routes/settings.tsx` (imports `HostFirewall`, renders a
    dedicated `HostFirewallSection` component at line 949, gated on
    `data.isAdmin && data.hostFirewall`) both consume this field. Phase 4's
    task list names only `LocalNetwork.tsx` and "`lanView` shape" — it does
    not mention `settings.tsx` or the `HostFirewall` type at all. An
    implementer following the plan's Surfaces-touched table literally would
    not know this admin settings section exists until `tsc -b` (Phase 4's own
    verify step) fails on the missing type, which is self-resolving for the
    *compile* error but not for the *design* question: does the whole
    "host firewall" settings card disappear (there is no more root half to
    report on), or does something replace it? The plan should say.
  These are exactly the class of miss round 1 caught three of (an unnamed
  identifier reached only through a mechanism, not named directly) — this
  round found two more of the same shape that round 1 did not reach.

- **The toggle handler's synchronous-rebuild budget is unstated.** The API
  section has the PUT handler call `Stop()` then `EnsureRunning()` inline and
  return `rebuilt: true/false` describing what already happened — i.e. the
  HTTP response blocks until the container rebuild (remove, network rebuild,
  recreate, `waitReady` up to 90s, ADR-028 apt manifest replay) completes.
  This is not a novel problem — `startProject` (`server.go:1071-1091`)
  already blocks the same way under a `context.WithTimeout(r.Context(), 2*time.Minute)`
  — but a rebuild is strictly more work than a plain start (extra
  remove/network-recreate round trips ahead of the same 90s `waitReady`), and
  the plan never states what timeout budget the toggle handler uses or
  whether 2 minutes (the existing `startProject` precedent) is enough headroom
  for a rebuild specifically. Low severity because the fix is "copy the
  existing pattern," but the plan should say so rather than leave it silent,
  since a first-pass implementer copying `startProject`'s 2-minute budget
  verbatim could tie a rebuild too close to the edge on a loaded host.

## (2) Readiness — questions a first-pass implementer would still have to ask

1. What happens to `internal/httpapi/server.go:1019-1032`'s
   `s.netpol.SetMarker`/`s.netpol.Kick` call in `deleteProject`, and does
   `s.netMu` still guard anything there once the marker is gone?
2. What replaces `explorer.go`'s `resp["hostFirewall"]` in the admin info
   response, and does the whole `HostFirewallSection` in
   `web/src/routes/settings.tsx` get deleted, or replaced with something
   (e.g. a plain "LAN mode is per-project now" note)? Neither file nor the
   `HostFirewall` TS type appears anywhere in the plan.
3. What context timeout does the new `PUT /api/projects/{id}/network`
   handler use for the synchronous rebuild path — is `startProject`'s
   existing 2-minute budget (`server.go:1077`) reused as-is, or does a
   rebuild (strictly more Docker round trips ahead of the same 90s
   `waitReady`) get its own, larger one?
4. (Minor, self-resolving via `go build`, carried over unresolved from round
   1's finding #6) `bridgename_test.go`'s two existing one-argument
   `BridgeNameFor` calls need editing, not just the "append" the Phase 1 task
   literally says — worth a one-line correction so nobody is confused when
   the append conflicts with the writing-files rule's spirit.

Everything round 1 asked (its 6 numbered questions) is now answered by the
plan text itself and confirmed against the live code, so none of those are
repeated here.

## Not examined

- `deploy/firewall/styrman-firewall.sh`, `install.sh`, `test.sh` (Phase 3) —
  not read in this round either; taken on the plan's word, as round 1 also
  left it.
- Full `internal/credbroker/broker.go` and `broker_test.go` beyond confirming
  `New()`'s current 4-arg signature and its one caller (`main.go:182`) —
  did not check every test helper for a stray `LanChecker` implementation.
- `web/e2e/run.mjs`'s two existing LAN blocks (Phase 5) — not read; only the
  plan's description was checked.
- Live `iptables -S` was not re-run this round (round 1 also took the plan's
  cited output as given); the dispatch offered `sudo iptables -S` but the
  Docker per-bridge DROP and base-rule claims were not independently
  re-verified live in this pass — they were already treated as established
  fact by round 1 and the plan's own "verified live on this host" claim.
- `internal/project/store_test.go` — did not check whether an existing test
  already exercises `SetLanAccess`/`RestoreLanAccess` in a way the plan's
  described simplification would break beyond what's described.

## Bottom line

Round 1's design-level misunderstanding (the mismatch-heal living in a
function the toggle flow never reached) is genuinely fixed, correctly, with
the fix traced against the actual file rather than taken on faith. The R12
splice boundary is now unambiguous and verified against the live line
numbers. What remains are two concrete, previously-unnamed surfaces
(`server.go`'s delete-path marker cleanup, and `explorer.go`/`settings.tsx`'s
admin `hostFirewall` card) that reference the machinery being deleted and are
absent from every touched-files list, plus one unstated timeout budget for
the new synchronous rebuild path. None of these overturn the plan's
mechanism — they are the same class of "reached through a mechanism, not
named by it" miss the plan's own text calls out three times already, just
two more instances of it.

design needs revision
implementation not ready
