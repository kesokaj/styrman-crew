---
owner: main
date: 2026-09-15
---
# Review — worktree, merged (2026-09-15)

Range reviewed: Scope: working tree — changed files: the 32 files under
`image/harness/` that `git diff HEAD --name-only` lists (base HEAD `52538f7`;
the mirror moves from styrman-harness `cceee548ea8bf8ca1e4b3f6b561383a88bea7aa4`
to `a81ef4be3cb8e4a933a386d9c87c5a0ccbd701a8`); untracked, not ignored: none.

## Verdicts

No lens FAILed.

- security (`security`): FINDINGS — 2 medium, 3 low, none high —
  `2026-09-15-security-worktree.md`
- code (`reviewer`): APPROVE, no findings — `2026-09-15-review-worktree-code.md`
- docs (`reviewer`): APPROVE, no findings — `2026-09-15-review-worktree-docs.md`

The tests row is the verifier's log,
`.agents/docs/testing/2026-09-15-verify-harness-refresh-a81ef4b.md`, not a lens
of this record.

## Findings

| # | Severity / confidence | Where (mirror paths) | Lens | Finding |
|---|---|---|---|---|
| 1 | medium / medium | `context/AGENTS.md:37`, `skills/review/SKILL.md:3`, `skills/preflight/SKILL.md:51` | security | A project agent that reuses a harness agent name (`security`, `reviewer`, `verifier`) can shadow it: the context states a convention, and nothing checks for a clash before a dispatch. |
| 2 | medium / medium | `skills/preflight/SKILL.md:77` | security | The blocking list does not block when a security row Step 2 requires is missing, was cut off, or lists a changed path under Not examined. |
| 3 | low / low | `skills/bug/SKILL.md:15`, `skills/feature/SKILL.md:15` | security | "Words in the invocation's arguments count" as a gate waiver also admits arguments the model composed. |
| 4 | low / low | `agents/documentation.md:30`, `agents/verifier.md:42` | security | `documentation` edits the Setup commands the verifier then executes, possibly with no reviewer reading them in between. |
| 5 | low / low | `internal/harness/apply.go:37`, `cmd/sh-agent/main.go:309`, `README.md:53` | security | The guidance moves a hand-written `~/.claude/CLAUDE.md`'s content into harness `rules/`, which this repository mirrors into every tenant's runtime image. |

## Not checked

- Antigravity, for every security finding. The name-clash precedence rests
  on the harness's single measured run (Claude Code 2.1.272, `reviewer`
  only), and nobody re-measured it.
- The unchanged remainder of the mirror: skills, agents and Go files outside
  the diff.
- The code reviewer reports that it did not compare the mirror with the
  upstream checkout. The security reviewer did (`diff -rq`, the three
  expected lines), and so did the main agent before dispatch.
- `deploy/verify-commit.sh`: no lens ran it, because the commit did not exist
  yet.
- Upstream bugs still open at `a81ef4b` also ship with the mirror
  (styrman-harness `.agents/docs/bugs/INDEX.md`: BUG-019 to BUG-023,
  BUG-031). This review did not re-review them.

## Outcome

Every finding is in vendored text. `image/harness/VENDORED.md:4-5`: "Nothing
in it is styrman's own; every file is edited in that repository and
re-mirrored." So none is fixed here; each goes to styrman-harness, the
operator's harness session, with this record and the security report as
input.

1. left: styrman-harness. In this repository `.claude/agents` links to
   `.agents/agents/`, which holds only `.gitkeep`, so there is no clash
   today.
2. left: styrman-harness. The operator's own `/preflight` on this box is the
   installed harness and has the same list, so for the release that ships
   this mirror, treat a missing or partial security row as blocking.
3. left: styrman-harness. Upstream plan D32 records the clause as deliberate
   for arguments the user types; the gap is the model-composed case.
4. left: styrman-harness.
5. left: styrman-harness. The mirror's `rules/` and `commands/` hold only
   `.gitkeep` at `a81ef4b` (`ls image/harness/rules image/harness/commands`,
   2026-09-15), so nothing personal is in the image today. The exposure
   opens only if someone follows the advice.
