# Quality Checklist

Reference file for the skill-creator. Use this checklist when evaluating whether a skill is production-ready.

## Pre-Ship Checklist

Run through every item before considering a skill complete.

### Structure

- [ ] **SKILL.md exists** with valid YAML frontmatter (`name` + `description`)
- [ ] **Under 500 lines** — if over, move content to `references/`
- [ ] **References directory** exists if the skill has deep patterns or code recipes
- [ ] **Reference files** are 300-800 lines each (avoid very large single files)
- [ ] **All reference links resolve** — every `[link](./references/file.md)` points to a file that exists

### Description (Triggering)

- [ ] **Explains WHAT the skill does** in one sentence
- [ ] **Explains WHEN to trigger** with specific contexts (tools, file types, frameworks)
- [ ] **Is slightly pushy** — includes edge cases where the skill should still activate
- [ ] **Has explicit boundaries** — states what it does NOT cover
- [ ] **Would trigger on a casual user prompt** — test against "build me a homepage" not just "create a React application with Tailwind CSS"

### Content

- [ ] **Brief intro** — one paragraph max before the first section
- [ ] **When to Use** section — explicit trigger conditions
- [ ] **When NOT to Use** section — boundary conditions, redirect to other skills
- [ ] **Core instructions in imperative form** — "Do X" not "You should consider X"
- [ ] **Explains WHY, not just WHAT** — reasoning over rigid rules
- [ ] **No ALWAYS/NEVER in caps** — reframe with reasoning instead
- [ ] **Examples where format matters** — input → output when the output structure is important

### Progressive Disclosure

- [ ] **SKILL.md is lean** — contains decision logic, principles, and pointers
- [ ] **Reference files have depth** — code recipes, checklists, patterns with full examples
- [ ] **Clear pointers** — every section that has a reference file links to it with context ("For X, see [file](./references/file.md)")
- [ ] **Reference files start with a header** explaining what they cover and when to read them

### Self-Consistency

- [ ] **No contradictions** between SKILL.md and reference files
- [ ] **No duplicated content** — if the same code block appears in SKILL.md and a reference, remove it from SKILL.md and add a pointer
- [ ] **Naming is consistent** — terms used in SKILL.md match terms used in references
- [ ] **Cross-references are correct** — if the skill mentions another skill, the name is accurate

### Testing

- [ ] **2-3 realistic test prompts** have been tried against the skill
- [ ] **At least one edge case** tested (casual phrasing, implicit need)
- [ ] **At least one negative case** verified (prompt that should NOT trigger the skill)
- [ ] **Output matches user expectations** — not just technically correct, but stylistically right

---

## Common Failure Patterns

### Description Too Passive

❌ Bad:
```yaml
description: Guidance for building Go APIs
```

✅ Good:
```yaml
description: Guidance for building Go APIs. Use when creating HTTP handlers, middleware, routing, REST endpoints, gRPC services, or any Go server-side code. Activate even if the user doesn't explicitly mention 'API' — any Go server code qualifies.
```

**Why it fails:** The passive description only triggers on exact matches. The pushy description triggers on the broader category of work.

### SKILL.md Too Long

**Symptom:** SKILL.md is 500+ lines with detailed code blocks.

**Fix:** Move code blocks to `references/`. Keep SKILL.md as the decision-making layer:
- What to do (principles, rules)
- When to do it (conditions, decision matrices)
- Where to find depth (pointers to reference files)

### Missing "When NOT to Use"

**Symptom:** Skill triggers for unrelated prompts.

**Fix:** Add explicit boundaries:
```markdown
## When NOT to Use
- CLI tools with no visual output (use bash skill)
- Headless API work (no UI involved)
```

### Duplicated Content

**Symptom:** Same HCL/TypeScript/Go code block appears in both SKILL.md and a reference file.

**Fix:** Remove from SKILL.md, keep in reference, add pointer:
```markdown
**For code examples, see:** [Code Patterns](references/code-patterns.md#section)
```

### No Examples

**Symptom:** Skill gives correct but inconsistently formatted output.

**Fix:** Add input → output examples for anything where format matters:
```markdown
## Commit Message Format
**Input:** Added user authentication with JWT tokens
**Output:** feat(auth): implement JWT-based authentication
```

---

## Sizing Guidelines

| Component | Target | Hard Limit |
|-----------|--------|------------|
| `description` field | 1-3 sentences (~50-150 words) | No hard limit, but long descriptions waste context |
| SKILL.md body | 200-350 lines | 500 lines |
| Reference file | 300-800 lines | No hard limit, but 1000+ may cause context pressure |
| Total skill (all files) | 1,000-3,000 lines | No hard limit |
| Reference files count | 2-6 files | No hard limit |

### When to Split a Reference File

Split when:
- The file exceeds 800 lines
- It covers two clearly distinct topics (e.g., "testing" and "deployment")
- Users would only need one section at a time

### When to Merge

Merge when:
- Two reference files are under 200 lines each
- They cover the same topic from different angles
- Users would always need both together
