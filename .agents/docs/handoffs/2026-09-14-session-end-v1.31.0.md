---
owner: main
date: 2026-09-14
---
# Handoff — 2026-09-14 — v1.31.0 live (harness mirror at styrman-harness 8046e4d)

Session ID: session_013YDhc5mpFupSmv2u2SHGTa        (same machine: `claude --resume`; matches the feedback log's session field)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator pasted the harness's mirror-refresh prompt
(`styrman-harness/.agents/docs/plans/2026-09-14-mirror-refresh.md`): refresh `image/harness/`
(a plain, .git-less copy of styrman-harness) to the harness's `main` at or
after its BUG-015 fix (2c8990e), fix every live description that went
stale, verify per the Testing section, and cut v1.31.0 (MINOR). One Task
under the Definition of Done, then `/preflight`.

## Completed

- Mirror refreshed 6893e22 → `8046e4d5f24b009bd2461ebef28931cade7a4acd` by
  `image/harness/VENDORED.md` § Refresh verbatim (three `diff -rq` lines,
  `SHA256SUMS` over 154 files); skills 28 → 27 (`audit`,
  `gcloud-auth-verification` gone; `review` added), subagents 7.
- `image/entrypoint.sh`: the install line dropped `GIT_CONFIG_GLOBAL=/dev/null
  GIT_CONFIG_NOSYSTEM=1` (install on a plain copy never runs git; every git
  call site in the mirror sits behind `harness.IsCheckout`); comments say why
  the 20 s cap holds. `deploy/verify-commit.sh` proves the built image's
  mirror has no `.git`. `integration.sh`: 28 → 27 and two new harness checks
  (97 checks; green run `passed=94 failed=0 skipped=3`). Stale text fixed in
  README.md, image/README.md, `specs/styrman/runtime-image.md`, the testing
  README, VENDORED.md.
- Bugs: BUG-057 (`grep -q` under pipefail → SIGPIPE false FAIL, fixed),
  BUG-058 (the vendored `TestDryRunNeverBuilds` assumes a checkout — upstream,
  open), BUG-059 (a Dockerfile guard `.dockerignore` made unreachable —
  deleted, replaced by the verify-commit assertion).
- Gate `.agents/docs/reviews/2026-09-14-preflight-v1.31.0.md` (SHIP): reviewer
  ×2, security ×3, docs lens, verifier ×3 — converged in three rounds; one
  medium rebutted (below), the lows fixed or rebutted in writing.
- Commits `cde7ec2` (the task), `c3b4417` (release: gate, release entry,
  README/ROADMAP). Tag `v1.31.0` on c3b4417, SSH-signed via `git -c` (no
  config change), pushed 12:52:54. Updater tick 12:55:47 "signature OK
  (allowed signer)", 12:56:18 "OK — v1.31.0 is live". `styrman-ctl status`
  verdict OK on 1.31.0 (sha c3b4417); the deployed `styrman-runtime:1.31.0`
  carries the mirror at 8046e4d, 27/7, plain-copy dry-run, no `.git`.

## Open / In Progress

- Nothing queued; `TODO.md` and `ROADMAP.md` empty (ROADMAP "Now" names the
  newest release entry only).
- Requirements edits awaiting approval: none.

## Key Decisions

- The git-config override on the install line was dropped, not kept as
  belt-and-braces: a guard for a call that no longer happens is a false
  description (global rule: a description that outlives its behaviour).
- The no-`.git` invariant is `image/.dockerignore`'s (`.git`, `**/.git`); the
  Dockerfile assertion added in review round 1 could never fire and was
  deleted (BUG-059); the release gate asserts it on the built image instead.
- Security's medium — the harness's "Root steps on the operator's own Linux
  box … do not hand it back" sentence is rendered into every project VM's
  AGENTS.md — rebutted for this release: the project user already holds
  sudo and the docker group inside its own VM by design; the mirror is
  verbatim (ADR-062). Carried to the operator for the harness (below).

## Corrections

- none this session (no feedback-log entry for this session; today's single
  entry is the harness session's).

## Agent notes (unverified)

- The changelog agent's two drafts each needed read-through corrections
  again (the task entry: floor attributed to ADR-062, full SHAs missing,
  verification credited to a log not yet written, a misdescribed README
  edit; the release entry: invented what `--vendor` does, overstated the
  verifier rounds). Sixth and seventh corrections in two days; the `/retro`
  item from the v1.30.0 handoff still stands.
- The verifier's round 3 reported two FAILs against my dispatch's own wording
  (a "comment-only" delta diffed against HEAD; a grep count that ignored the
  comment quoting the variable name) — explained in the gate report; the
  code facts hold. Dispatch deltas as a range or a base, never as prose.
- The 8 git-tracked files under `.claude/agent-memory/` mention `/audit` and the
  old harness; the harness no longer uses private agent memory. Listed, not
  deleted: `.claude/agent-memory/changelog/{MEMORY.md,
  adr_format_super-devbox.md, audit_entries_pattern.md,
  changelog_format_super-devbox.md}`, `.claude/agent-memory/documentation/
  {MEMORY.md, feedback_styrman_doc_edits.md,
  project_styrman_backup_removal_v1_17_0.md, project_styrman_docs.md}`.

## Environment State

- Branch: main, clean after this handoff's commit, level with origin. Tag
  v1.31.0 on c3b4417, pushed.
- Production: https://crew.styrman.xyz on 1.31.0 (server, caddy, runtime),
  live 12:56:18; 0 running microVMs at the time.
- `styrman-runtime:test` built from the final tree (12:33). Scratch under
  `.agents/scratch/` (gitignored): stage lists, a root-read copy of
  `/etc/styrman/allowed_signers`.

## Artifacts to Read

- `.agents/docs/reviews/2026-09-14-preflight-v1.31.0.md` — the gate, every
  finding's disposition, the observed deploy.
- `.agents/docs/changes/2026-09-14-v1.31.0-release.md` and
  `2026-09-14-harness-refresh-8046e4d.md` — what changed and the review rounds.
- `.agents/docs/bugs/BUG-057-grep-q-pipeline-sigpipe-under-pipefail.md`,
  `BUG-058-vendored-harness-test-assumes-git-checkout-shape.md`,
  `BUG-059-harness-refresh-dockerfile-dead-git-check.md` — the three new records.
- `styrman-harness/.agents/docs/bugs/BUG-015-install-rebuild-plain-copy.md`
  — why the floor was 2c8990e.
- `.agents/docs/plans/2026-09-14-coldread-handoff-v1.31.0.md` — the cold-reader
  check of this handoff (no dangling references, no contradictions).

## Suggested Skills

- `/retro` — the changelog agent's invented-evidence pattern (BUG-053 family)
  recurred twice more; and the "prove a new guard can fail" rule (BUG-059)
  is a candidate for `.agents/rules/`.

## Next Steps

1. Operator, on the live site (versioning.md step 5): a fresh Google sign-in
   showing 1.31.0 in Settings; start a project — its boot log shows `agent
   harness deployed (version 1.31.0, styrman-harness 8046e4d …)` and
   `sh-agent status` in its terminal starts with `repo /styrman/harness
   (plain copy, …)` and lists 27 skills. BUG-054's `gh auth status` proof
   from the v1.30.1 handoff is still outstanding.
2. Operator, in styrman-harness: (a) BUG-058 — `TestDryRunNeverBuilds`
   should build its own checkout shape (a temp dir with a `.git` entry) or
   skip when `IsCheckout` is false; (b) the root-steps sentence in
   `context/dev-setup.md` reaches every consumer VM — scope it (a
   project-level rule, or a precondition the agent must check); (c) the
   path-validation lows (`apply.go` manifest paths, frontmatter `name:` →
   path) — `filepath.IsLocal` at the parse boundary. (d) argolis-devbox's
   refresh from the same prompt is a separate session.
3. Operator: decide whether this host gets a `docker login` against Docker
   Hub's anonymous pull limit (BUG-056, still open; today's gate did not hit
   it).
4. Nothing else is queued.
