---
name: init-project
description: Scaffold a new project with the standard agent directory structure, AGENTS.md, REQUIREMENTS.md, TODO.md, and ROADMAP.md.
disable-model-invocation: true
---

# Init Project Workflow

Scaffold a new project with the standard agent directory structure, AGENTS.md, REQUIREMENTS.md, TODO.md, and ROADMAP.md.

## Usage

```
/init-project
```

## Steps

### Step 1: Confirm Project Root

Confirm the current workspace is the intended project root. If unsure, ask the user.

### Step 2: Create AGENTS.md

Create `AGENTS.md` at the project root with the following template. Ask the user for the project overview before writing.

```markdown
# AGENTS.md

## Project Overview

<!-- Filled by user -->

## Setup Commands

<!-- Project-specific build/run/test commands -->

```bash
# Install dependencies
go mod download

# Build
go build ./cmd/...

# Run
go run ./cmd/server/

# Test
go test ./...

# Lint
go vet ./... && staticcheck ./...
```

## Architecture

<!-- Project-specific architecture notes -->
<!-- For ADRs, see .agents/docs/architecture/DECISIONS.md -->

## Project-Specific Standards

<!-- Only list standards that DIFFER from the global context (~/.claude/CLAUDE.md / ~/.gemini/GEMINI.md). -->
<!-- If nothing differs, delete this section. -->

## Testing

<!-- Project-specific testing instructions that supplement the global testing tiers. -->
<!-- For detailed guides, see .agents/docs/testing/ -->

## Key Files

<!-- Help agents navigate this codebase quickly. -->

| Path | Purpose |
|------|---------|
| `cmd/server/main.go` | Application entry point |
| `internal/config/` | Configuration loading |
| `internal/api/` | HTTP/gRPC handlers |
| `REQUIREMENTS.md` | Non-negotiable project requirements |
| `.agents/docs/` | All project documentation |
```

### Step 3: Create REQUIREMENTS.md

Create `REQUIREMENTS.md` at the project root. Use tables to quantify targets — vague requirements are useless:

```markdown
# Requirements

## Scale Targets

| Metric | Target | Notes |
|---|---|---|
| <!-- e.g., Concurrent users --> | <!-- e.g., 10,000 --> | <!-- Context --> |

## Latency SLA

| Metric | Target | Hard Limit |
|---|---|---|
| <!-- e.g., API response (p99) --> | <!-- e.g., < 50ms --> | <!-- e.g., < 200ms --> |

## Critical Requirements

1. <!-- Requirement 1 -->
2. <!-- Requirement 2 -->

## Constraints

- <!-- Constraint 1 -->

## Out of Scope

- <!-- What this project does NOT do -->
```

### Step 4: Create TODO.md

Create `TODO.md` at the project root:

```markdown
# TODO

Deferred work items. Not blocking current work, but should be addressed later.

<!-- Format: - [ ] Description (added YYYY-MM-DD) -->

## High Priority

## Medium Priority

## Low Priority

## Ideas / Nice-to-Have
```

### Step 5: Create ROADMAP.md

Create `ROADMAP.md` at the project root (this is the `templates/ROADMAP.md` template). Fill "Last reviewed" from `date +%F`, never from memory:

```markdown
# Roadmap

<!-- Division of labor: ROADMAP.md = where the project is going; TODO.md = small deferred items; .agents/docs/plans/ = how. -->

Last reviewed: YYYY-MM-DD

## Now

<!-- The current milestone, with a link to the active plan in .agents/docs/plans/. -->

## Next

<!-- 1-3 upcoming milestones, one line each. -->

## Later / Ideas

<!-- Unscheduled directions worth keeping visible. -->
```

### Step 6: Create .agents/ Directory

Create the full `.agents/` directory structure — the docs tree carries all seven directories the constitution names:

```
.agents/
├── rules/
├── workflows/
├── skills/
└── docs/
    ├── changes/
    ├── architecture/
    │   └── DECISIONS.md
    ├── specs/
    ├── testing/
    ├── plans/
    ├── handoffs/
    └── reviews/
```

Create `DECISIONS.md` with the ADR format:

```markdown
# Architectural Decision Records

<!-- Append-only. Mark superseded entries, never delete them. -->

<!--
## ADR NNN: [Title]
**Status:** Proposed | Accepted | Superseded by ADR NNN
**Date:** YYYY-MM-DD

### Context
What is the issue or decision? What constraints exist?

### Decision
What was decided and why? Include specific technologies, configurations, or trade-offs.
-->
```

### Step 7: Create README.md

If no `README.md` exists, create a basic one using the project overview from Step 2.

Also ensure `.gitignore` contains a line for per-project agent memory (append if absent):

```
.claude/agent-memory/
```

Subagents with `memory: project` write there in every project; without this line the
state accumulates as untracked noise in `git status`.

### Step 8: Summary

Report what was created and remind the user to:
1. Fill in `AGENTS.md` with project-specific details
2. Fill in `REQUIREMENTS.md` with project requirements
3. Add known deferred items to `TODO.md`
4. Set the current milestone in `ROADMAP.md` "Now"
5. Commit the scaffolded structure
