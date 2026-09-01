---
name: audit
description: Perform a comprehensive audit of a directory or the entire project.
disable-model-invocation: true
argument-hint: [directory]
---

# Audit Workflow

Perform a comprehensive audit of the specified directory or the entire project.

## Usage

```
/audit [directory]
```

If no directory is specified, audit the entire project.

**Docs root** (`<docs-root>` below): `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`.

On Antigravity, first run `~/.agents/bin/resume-context.sh`. Then on either vendor run `~/.agents/bin/trail-check.sh --check` for a trail-freshness report (Claude Code's SessionStart hook already ran resume-context, but not the check).

## Steps

### Step 1: Dispatch the Scan — in parallel

Do not scan inline — never self-audit. Dispatch four subagents in parallel (on Claude Code these are subagents; on Antigravity use `invoke_subagent` — same names) and let the main loop only merge their findings:

- `explorer` — inventory and structure map of the target: uniformity across the project, orphaned/dead-code candidates
- `reviewer` — standards pass, loading the `review-standards` skill: naming conventions, syntax errors, badly written functions, adherence to API specifications (check `<docs-root>/specs/`)
- `security` — vulnerability pass: auth, secrets, input parsing, dependency surfaces
- `verifier` — verification pass: run the local test suite and builds per `<docs-root>/testing/`; changed or audited code without covering tests is a finding

**Quality Standards:** the `review-standards` skill is the audit standard — its six criteria (dead code, flow tracing, standards conformance, directory layout, simplicity, decoupling) plus its `references/go-review.md` and `references/typescript-review.md` checklists; the `golang`/`typescript`/`frontend` skills are the idiom authority.

### Step 2: Honest Assessment

Merge the subagent reports into an honest assessment grounded in software best practices and project documentation. Lead with what is broken; state severity and confidence; never restate a failure as success.

### Step 3: Detailed Fixes & Optimisations

Present findings in **table format**, grouped by audit category.

**Summary table first:** Total counts per severity level.

**Detail tables:** Columns: Severity (🔴/🟡/🟢), File, Line, Issue, Suggested Fix.

### Step 4: Write the Report

Write the full merged report to `<docs-root>/reviews/$(date +%F)-audit-<scope>.md` — never chat-only; the date comes from `date +%F`, never memory.

### Step 5: Documentation Update

- If the audit shows README/spec drift, dispatch the `documentation` agent to update `<docs-root>` and ensure every subproject/sub-directory has an accurate `README.md`.
- Dispatch the `changelog` agent to record the audit in `<docs-root>/changes/` — pass today's date from `date +%F` in the dispatch prompt.
