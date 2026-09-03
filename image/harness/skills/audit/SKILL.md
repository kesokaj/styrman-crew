---
name: audit
description: Perform a comprehensive audit of a directory or the entire project. Use when the user runs /audit. Do NOT use for routine code review (use the reviewer agent or the vendor's built-in review command) or for running tests alone (use the verifier agent).
disable-model-invocation: true
argument-hint: "[directory]"
---

# Audit Workflow

Perform a comprehensive audit of the specified directory or the entire project.

## Usage

```
/audit [directory]
```

If no directory is specified, audit the entire project.

**Docs root** (`<docs-root>` below): `.agents/docs` if it exists, else `docs/` if it exists, else create `.agents/docs`. All dates come from `date +%F`, never from memory.

Before scanning, read `AGENTS.md`, `REQUIREMENTS.md`, and the three newest `<docs-root>/changes/` entries so findings are judged against recorded intent, not guesses.

## Steps

### Step 1: Dispatch the scan — four agents, in parallel

Do not scan inline — never self-audit. Dispatch these subagents in parallel (on Claude Code through the Agent tool, on Antigravity through `invoke_subagent` — same names), each with the target directory, today's date, and the instruction to write its artifact to `<docs-root>`:

- `explorer` — inventory and structure map of the target: uniformity across the project, orphaned/dead-code candidates. Brief lands in `plans/`.
- `reviewer` — standards pass, loading the `review-standards` skill: naming conventions, syntax errors, badly written functions, adherence to API specifications (check `<docs-root>/specs/`). Report lands in `reviews/`.
- `security` — vulnerability pass: auth, secrets, input parsing, dependency surfaces, GCP IAM. Findings land in `reviews/`.
- `verifier` — runs the project's own test, vet, and lint commands and maps every source file in the target to a covering test; audited code without a covering test is a FAIL line regardless. Log lands in `testing/`.

**Quality standard:** the `review-standards` skill is the audit standard — its six criteria (dead code, flow tracing, standards conformance, directory layout, simplicity, decoupling) plus its `references/go-review.md` and `references/typescript-review.md` checklists; the `golang`/`typescript`/`frontend` skills are the idiom authority.

### Step 2: Honest assessment

Merge the four ten-line replies and read their artifacts. Lead with what is broken; state severity and confidence; never restate a failure as success.

### Step 3: Detailed fixes and optimisations

Present findings in **table format**, grouped by audit category.

**Summary table first:** total counts per severity level.

**Detail tables:** columns Severity (🔴/🟡/🟢), File, Line, Issue, Suggested Fix.

### Step 4: Write the report

Write the full merged report to `<docs-root>/reviews/$(date +%F)-audit-<scope>.md` with frontmatter `owner: main` and `date:` — never chat-only. Link the four agent artifacts rather than pasting them.

### Step 5: Documentation and record

- If the audit shows README/spec drift, dispatch the `documentation` agent to update `<docs-root>` and ensure every subproject has an accurate `README.md`.
- Dispatch the `changelog` agent to record the audit in `<docs-root>/changes/`, passing today's date.
- Commit and push per the global git workflow.
