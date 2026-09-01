---
name: preflight
description: |
  Pre-release gate: run before shipping any version — a tag, release, deploy,
  or "ship it". Dispatches reviewer, security, and verifier in parallel over the
  release diff, checks trail and docs freshness, and writes a merged gate report
  with an explicit SHIP / NO-SHIP verdict. No version ships without this gate.
  It does not replace the Definition of Done rows.
disable-model-invocation: true
argument-hint: [version or base ref]
---

# Preflight — Release Gate

Gate a release candidate before anything ships. Everything of record lands on disk; the verdict is never chat-only.

**Docs root** (`<docs-root>` below): `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`.

```bash
DOCS_ROOT=.agents/docs
[ -d "$DOCS_ROOT" ] || { [ -d docs ] && DOCS_ROOT=docs; } || true
mkdir -p "$DOCS_ROOT/reviews"
```

All dates come from `date +%F` — never from memory.

On Antigravity, first run `~/.agents/bin/resume-context.sh`. Then on either vendor run `~/.agents/bin/trail-check.sh --check` for a trail-freshness report (Claude Code's SessionStart hook already ran resume-context, but not the check).

## Usage

```
/preflight            # gate the diff since the last tag
/preflight v1.4.0     # gate toward a named version
/preflight <ref>      # gate the diff since a user-given base ref
```

## Step 1: Determine the Release Diff

```bash
git status --porcelain          # SOURCE paths MUST be clean
git describe --tags --abbrev=0  # last tag = default base ref
git log <base>..HEAD --oneline
git diff <base>..HEAD --stat
```

**ABORT if any SOURCE path is dirty** — modified or untracked files outside `<docs-root>` — and tell the user plainly that preflight only gates committed source state, then stop. Untracked or modified files **under `<docs-root>` are exempt**: a previous run's gate artifacts (reviews, verification logs) must not deadlock a rerun — they ride along in the release commit (Step 6). Use the user-given ref as base when provided; otherwise the last tag; if no tag exists, ask for a base ref.

## Step 2: Dispatch the Gate Owners — in parallel

Never self-review, self-verify, or self-audit. Dispatch all three at once and wait (on Claude Code these are subagents; on Antigravity use `invoke_subagent` — same names):

- `reviewer` — review the **full release diff**, loading the `review-standards` skill.
- `security` — full pass over every auth, IAM, IaC, and dependency surface touched in the diff.
- `verifier` — full local suite plus dual-arch (ARM64 + AMD64) image builds; local tier first.

## Step 3: Trail Freshness

Run `~/.agents/bin/trail-check.sh --check`. Its failing checks: the newest `changes/` entry covers `HEAD`, and no wrong-dated filenames — any of those is a finding in the gate report. It also reports whether `ROADMAP.md`/`TODO.md` have been touched since `HEAD`, but that is **report-only** (`note:` lines) — mention lag in the report; it is not by itself a violation.

## Step 4: Docs Freshness

Confirm `README.md` and `<docs-root>/specs/` reflect the release diff (interfaces, behavior, deployment). If they do not, dispatch the `documentation` agent and record the gap as a finding.

## Step 5: Write the Merged Gate Report

Write to `<docs-root>/reviews/$(date +%F)-preflight-<version>.md`:

- **FAIL-first ordering** — lead with what is broken.
- Every finding carries **severity and confidence**.
- An explicit verdict line: `VERDICT: SHIP` or `VERDICT: NO-SHIP`.
- A **"What was not checked"** section — name the gaps honestly.

Never restate a failure as success. Any blocking finding from Steps 2–4 means NO-SHIP.

## Step 6: Ship or Stop

- **NO-SHIP:** stop. Report the blocking findings and the report path. Never ship on NO-SHIP — no override.
- **SHIP:** the gate has already run against the clean source tree (Step 1); now assemble **one release commit**:
  1. Dispatch the `changelog` agent with today's date (`date +%F`) for the release entry.
  2. Bump the version **only where the repo already tracks one** (do not introduce version tracking).
  3. Commit the gate artifacts (gate report, verification logs), the changelog entry, and the version bump together as a single release commit, and tag **that** commit.

  The tagged commit is the gated tree plus its records — nothing else rides along.

This gate is additive: the Definition of Done rows (review, verification, changelog, documentation, …) still apply to the work being released — preflight verifies they happened; it does not perform them retroactively.
