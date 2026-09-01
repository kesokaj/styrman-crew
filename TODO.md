# TODO

Empty by design (operator directive 2026-08-30: "I want nothing in todo and
roadmap — start fresh with new things"). The entire previous roadmap was
either SHIPPED in v1.14.0 or RETIRED with reasons in **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) — including the standing
WON'T-DO decisions. History lives in `.agents/docs/changes/` and git.

Add new items below as they arise.

## Inbox

- [ ] Settings harness roster is permanently empty. `internal/httpapi/setup.go:230-252`
  reports the harness roster (`Skills`, `Agents`, `Commands`) from
  `s.cfg.UserHome(u.Username)` — the base/setup home. The entrypoint never
  seeds that home, because the setup VM runs with `STYRMAN_MINIMAL=1`
  (`internal/orchestrator/orchestrator.go:565` → `image/entrypoint.sh`), so
  all three fields are always empty and `omitempty` hides them. Confirmed on
  the host: `/var/lib/styrman/homes/ostling-simon/.claude/agents` does not
  exist. Consequence: the 28 skills / 6 subagents shipped in v1.15.0 can
  never surface in the UI. Pre-existing, deliberately deferred out of the
  v1.15.0 vendoring release; both the reviewer and the agent agree the
  deferral is correct because a real fix needs a product decision (point the
  lookup at a project home, or delete the three fields as dead reporting).
  Secondary: if it is ever repointed at a seeded home, `listNames`
  (`setup.go:167-178`) strips only a `.md` suffix, so harness re-seed
  backups would be listed as agents — a real example exists on the host:
  `~/.claude/agents/testing.md.bak.20260901-065226`. Filter `*.bak.*` if
  repointed. (deferred: needs a product decision, not a code fix)
