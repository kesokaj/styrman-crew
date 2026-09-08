---
owner: bugs
date: 2026-09-08
id: BUG-014
status: fixed
area: internal/netpolicy, cmd/styrman (per-project LAN access, ADR-065)
---
# BUG-014: a live LAN grant reads as "unavailable" whenever root's status file cannot be read

## Symptom
Security review of the LAN-access server/UI half: with the root status file
unreadable (run dir unmounted, `STYRMAN_FIREWALL_DIR` cleared, or the state
dir's group changed), a project whose RETURN rule was live in `STYRMAN-FWD`
was reported "unavailable" / "host firewall support is not installed" and its
LAN chip vanished from the UI — while the marker stayed on disk, `Sync` kept
recreating it, and root kept applying the rule. "Empty `STYRMAN_FIREWALL_DIR`
disables the feature" (the config comment and api.md's claim) was false for
any project already granted.

## Repro
Not reproduced live; found by code reading in
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` (finding 1,
`internal/netpolicy/status.go:157-163`).

## Root cause
`Derive` treated "status unreadable" (`st == nil`) as equivalent to "feature
absent", collapsing two different facts (root cannot be read vs. root has
nothing granted) into one UI state. The state used for a privilege root
enforces was derived from the absence of information rather than from root's
last confirmed word.

## Fix
`Derive`: a record with `lan == true` and an unreadable status now returns
`StateError` ("cannot confirm … the rule may still be present") instead of
`StateUnavailable`; `lanChip` renders the flag for any `lanAccess:true`
regardless of derived state; disabling the feature (`FirewallRunDir` empty)
now makes `Sync` remove every marker at boot instead of just refusing new
ones. In the working tree per the review; lands with the Phase 2 checkpoint
commit that follows this record.

## Prevention
A UI state for a privilege that root enforces must be derived from root's
LAST WORD, never inferred from the absence of one — an unreadable status
reads "cannot confirm", never "off"/"absent". A feature "disable" switch
must tear down existing grants, not just refuse new ones — the two are
different operations and code review should ask for both explicitly.

## Related
`.agents/docs/reviews/2026-09-08-security-lan-phase2-3.md` finding 1;
BUG-019 (same status-derivation surface, UI-switch angle, e2e-found).
