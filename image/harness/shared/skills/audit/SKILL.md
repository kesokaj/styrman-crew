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

## Steps

### Step 1: Scan & Verify

Scan all files in the target and verify:

- Naming conventions
- Syntax errors
- Badly written functions
- Adherence to API specifications (check `.agents/docs/specs/`)
- Feature verification and testing
- Orphaned or dead code
- Uniform structure across the project

**Quality Standards:** If the `code-review` skill is available, use its language-specific guides as the audit standard:
- Language guides (Go, TypeScript, Python, etc.) for idiomatic patterns and anti-patterns
- [Security Review Guide](reference/security-review-guide.md) for vulnerability checks
- [Common Bugs Checklist](reference/common-bugs-checklist.md) for language-specific bug patterns
- [Architecture Review Guide](reference/architecture-review-guide.md) for structural issues

### Step 2: Honest Assessment

Write an honest report grounded in software best practices and project documentation.

### Step 3: Detailed Fixes & Optimisations

Output findings in **table format**, grouped by audit category.

**Summary table first:** Total counts per severity level.

**Detail tables:** Columns: Severity (🔴/🟡/🟢), File, Line, Issue, Suggested Fix.

### Step 4: Documentation Update

- Update all relevant documentation files in `.agents/docs/`.
- Ensure every subproject/sub-directory has an accurate `README.md`.
- Create a changelog entry in `.agents/docs/changes/` for the audit findings.
