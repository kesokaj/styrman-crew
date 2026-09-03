---
name: research
description: Research a topic, compare alternatives, and optionally build a full spec with implementation plan. Use when the user runs /research. Do NOT use for quick single-file code lookups or simple grep searches.
disable-model-invocation: true
argument-hint: "[topic]"
---

# Research & Spec Workflow

Research a topic, compare alternatives, and optionally build a full spec with implementation plan.

## Usage

```
/research <topic>           # Full mode: research → compare → recommend → spec → plan
/research --light <topic>   # Light mode: research → compare → recommend (stops here)
```

## Steps

### Step 1: Discovery

- Search the web and read existing project code.
- Study similar solutions and identify at least 3 viable approaches or tools.

### Step 2: Compare

Present findings in a **comparison table**:

| Approach | Pros | Cons | Complexity | Cost | Maturity |
|----------|------|------|------------|------|----------|

Use the project's established standards and constraints as evaluation criteria.

### Step 3: Recommend

State a clear recommendation with justification grounded in:
- Project architecture
- Development philosophy
- Existing constraints

> **Light mode stops here.** Ask the user: "Want me to continue and build the full spec and implementation plan?"

### Step 4: Build Spec (Full Mode Only)

Write or update `.agents/docs/specs/<app_name>/integration-spec.md` with:
- Public APIs and schemas
- Configuration
- Authentication and authorization
- Error handling

### Step 5: Build Implementation Plan (Full Mode Only)

Create an implementation plan in `.agents/docs/plans/`:
- Split into discrete tasks
- Each task specifies verification owner: AI, User, or Both
