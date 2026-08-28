---
name: skill-creator
description: Create new skills, modify and improve existing skills, and measure skill quality. Use when users want to create a skill from scratch, edit or optimize an existing skill, run test cases to verify a skill works correctly, or optimize a skill's description for better triggering accuracy. Activate this skill whenever a user mentions creating, editing, testing, or improving agent skills — even if they don't say "skill" explicitly.
---

A skill for creating new skills and iteratively improving them, adapted for **Gemini CLI** and **Antigravity**.

## Overview

The process of creating a skill:

1. Decide what the skill should do and roughly how it should work
2. Write a draft of the skill
3. Create test prompts and run them with the skill active
4. Evaluate the results qualitatively
5. Rewrite the skill based on feedback
6. Repeat until satisfied

Your job is to figure out where the user is in this process and help them progress. Maybe they want to create a skill from scratch, or maybe they already have a draft and want to improve it.

## Skill Architecture

### File Structure
```
skill-name/
├── SKILL.md              (required — YAML frontmatter + markdown instructions)
└── references/           (optional — deep-dive docs loaded on demand)
    ├── patterns.md
    └── examples.md
```

### YAML Frontmatter (Required)
```yaml
---
name: my-skill-name
description: When to trigger and what the skill does. Be specific about contexts.
---
```

### Progressive Disclosure — Three Levels
1. **Metadata** (name + description) — Always in context (~100 words). This is the trigger mechanism.
2. **SKILL.md body** — Loaded when skill triggers (<500 lines ideal)
3. **Reference files** — Loaded on demand when deeper context is needed (unlimited size)

### Installation Locations

Global skills are discovered from the vendor's skills directory:

| Agent | Global skills directory |
|---|---|
| Claude Code | `~/.claude/skills/` |
| Antigravity | `~/.gemini/config/skills/` |

Each skill lives in its own directory:
```
<skills-dir>/
├── my-new-skill/
│   └── SKILL.md
├── another-skill/
│   ├── SKILL.md
│   └── references/
│       └── details.md
```

**Global skills belong in the harness repo** (`harness/shared/skills/` in the
devshell_prompt repo), deployed with `./harness/setup.sh` — do not write directly
into the vendor directories above: the deploy uses `rsync --delete`, so anything
added there by hand is removed on the next run.

Project-specific skills go in the project's `.agents/skills/` directory (the agent
directory convention). Copy into the vendor's project-level discovery directory if
the agent requires it (e.g. `.claude/skills/` for Claude Code).

---

## Phase 1: Capture Intent

Start by understanding what the user wants. The current conversation might already contain a workflow to capture (e.g., "turn this into a skill"). If so, extract from the conversation history first.

Ask the user:
1. **What should this skill enable the AI to do?**
2. **When should it trigger?** (what user phrases, contexts, or file types)
3. **What's the expected behavior or output format?**
4. **Are there edge cases or things the skill should NOT do?**

If you're extracting from conversation history, present your understanding and let the user confirm before proceeding.

## Phase 2: Research and Interview

Proactively ask about:
- Edge cases and failure modes
- Input/output formats and examples
- Success criteria — how will the user know the skill is working?
- Dependencies — does this need specific tools (go, docker, gcloud, etc.)?

Search the web for existing skills or best practices that might inform the design.

## Phase 3: Write the SKILL.md

### Required Components

**`name`**: Short, hyphenated identifier (e.g., `go-api-patterns`)

**`description`**: This is the PRIMARY triggering mechanism. Include both what the skill does AND specific contexts for when to use it.

Write descriptions that are slightly "pushy" — the AI tends to under-trigger skills. Instead of:
> "Guidance for building Go APIs"

Write:
> "Guidance for building Go APIs. Use when creating HTTP handlers, middleware, routing, REST endpoints, gRPC services, or any Go server-side code. Activate even if the user doesn't explicitly mention 'API' — any Go server code qualifies."

**Body**: The actual instructions. Written in imperative form.

### Writing Principles

1. **Explain the why.** Today's models are smart. Rather than rigid MUST/NEVER rules, explain reasoning so the model can generalize. A yellow flag is finding yourself writing ALWAYS or NEVER in all caps — reframe with reasoning instead.

2. **Keep it lean.** Remove things that aren't pulling their weight. If instructions cause the model to waste time on unproductive patterns, cut them.

3. **Generalize, don't overfit.** Skills are used across many prompts. Don't write instructions that only work for a specific example — think about the broad category of tasks.

4. **Use progressive disclosure.** Keep SKILL.md under 500 lines. For detailed reference material, use `references/` files with clear pointers:
   ```markdown
   For detailed patterns, see [API Patterns](references/api-patterns.md).
   ```

5. **Include examples.** Show input → output when the format matters:
   ```markdown
   ## Commit message format
   **Example:**
   Input: Added user authentication with JWT tokens
   Output: feat(auth): implement JWT-based authentication
   ```

6. **Define output formats explicitly** when they matter:
   ```markdown
   ## Report structure
   Use this template:
   # [Title]
   ## Executive Summary
   ## Key Findings
   ## Recommendations
   ```

### Domain Organization
When a skill covers multiple domains, use reference files:
```
cloud-deploy/
├── SKILL.md (workflow + selection logic)
└── references/
    ├── gcp.md
    ├── docker.md
    └── kubernetes.md
```
The model reads only the relevant reference file based on context.

---

## Phase 4: Test the Skill

### Create Test Cases
Come up with 2-3 realistic test prompts — the kind of thing a real user would actually say. Share them with the user: "Here are a few test cases I'd like to try. Do these look right, or do you want to add more?"

### Run Test Cases
For each test case:
1. Apply the skill's instructions to accomplish the test prompt
2. Save the output for review
3. Compare: does the output match what the user expects?

Organize results in the skill's workspace:
```
skill-name-workspace/
├── iteration-1/
│   ├── test-1/
│   │   ├── prompt.md
│   │   └── output/
│   └── test-2/
│       ├── prompt.md
│       └── output/
└── iteration-2/
    └── ...
```

### Present Results to the User
Show each test case with:
- The prompt that was given
- The output produced
- What you think worked well / could improve

Ask for feedback: "How does this look? Anything you'd change?"

---

## Phase 5: Iterate

After receiving feedback:

1. **Generalize from the feedback.** Don't make fiddly changes that only fix one test case. Understand the underlying issue and fix the root cause.

2. **Read the outputs carefully.** If the model is wasting time on unproductive steps, remove the instructions causing that.

3. **Look for repeated patterns.** If every test case results in the model doing the same setup work, bake that into the skill (maybe as a reference file or a script).

4. **Revise the skill.** Apply improvements, then run the test cases again into a new `iteration-N/` directory.

5. **Show the user the new results** alongside the previous version so they can see improvements.

Keep iterating until:
- The user says they're happy
- The feedback is all positive
- You're not making meaningful progress

---

## Phase 6: Optimize the Description

The `description` field determines whether the skill triggers. After the skill content is solid, optimize the description:

### Generate Trigger Evaluation Queries
Create 16-20 test queries — a mix of should-trigger and should-not-trigger:

**Should-trigger queries (8-10):** Different phrasings of the same intent — some formal, some casual. Include cases where the user doesn't explicitly name the skill but clearly needs it.

**Should-not-trigger queries (8-10):** Near-misses that share keywords but actually need something different. These should be genuinely tricky, not obviously irrelevant.

Make queries realistic — include file paths, personal context, abbreviations, casual speech:

Bad: `"Format this data"`, `"Create a chart"`

Good: `"ok so I need to add prometheus metrics to my go service, the main handler is in cmd/server/main.go and I'm using chi for routing. should I use the prometheus client directly or go through otel?"`

### Review with User
Present the query set. Let them edit, add, remove. Then mentally test each query against the description — would the AI correctly decide to load this skill?

### Refine the Description
Based on results, make the description:
- More specific about contexts (mention file types, frameworks, tools)
- Clear about what it does NOT cover
- Slightly pushy for borderline cases

---

## Hard Rules

1. Skills must not contain malware, exploit code, or anything that compromises security
2. A skill's contents should not surprise the user in intent if described
3. Don't create skills designed to facilitate unauthorized access or data exfiltration
4. All skills must be compatible with Gemini CLI / Antigravity — do NOT reference Claude-specific features like `claude -p`, subagents, Cowork, or `/plugin` commands
5. Keep SKILL.md under 500 lines — use `references/` for depth
6. The `description` field must explain BOTH what and when

## Reference Library

For evaluation criteria and real-world examples, consult the reference files:

* **Quality Checklist (Pre-ship evaluation, frontmatter, structure, description quality)**
  Read → `./references/quality-checklist.md`

* **Worked Examples (Before/after skill transformations, common improvement patterns)**
  Read → `./references/worked-examples.md`

> [!TIP]
> Before finalizing a skill, read the quality checklist to verify it meets all production-ready criteria.

## Quick Reference: Skill Template

```markdown
---
name: my-skill-name
description: |
  What this skill does in one sentence. Use when [specific triggers].
  Activate for [broader category] even when [edge case user might not think of].
---

Brief intro — one paragraph max.

## When to Use
- Trigger condition 1
- Trigger condition 2

## When NOT to Use
- Anti-trigger 1 (use X skill instead)

## Core Instructions
[The actual expertise — imperative form]

## Examples
**Example 1:**
Input: ...
Output: ...

## Cross-References
- See `skill-name@related-skill` for [related topic]

## Detailed Guides
- [Topic](references/topic.md) — When to read and what it covers
```
