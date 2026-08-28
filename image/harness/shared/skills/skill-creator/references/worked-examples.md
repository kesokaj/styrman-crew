# Worked Examples

Reference file for the skill-creator. Read this to see real before/after skill transformations and understand common improvement patterns.

---

## Example 1: Frontend Skill — From Design-Only to Production-Ready

### Before (179 lines, design-direction only)

```yaml
---
name: frontend-skill
description: Use when the task asks for a visually strong landing page, website, app, prototype, demo, or game UI. This skill enforces restrained composition, image-led hierarchy, cohesive content structure, and tasteful motion while avoiding generic cards, weak branding, and UI clutter.
---
```

**What it had:**
- Strong art direction (composition, imagery, copy, motion)
- Hard rules and litmus checks
- Clear aesthetic principles

**What it was missing:**
- No responsive/mobile guidance
- No accessibility (WCAG, contrast, focus states)
- No performance targets (Core Web Vitals, image optimization)
- No dark mode/theming strategy
- No "When NOT to Use" boundary
- No reference files with code patterns

### After (272 lines + 3 reference files, 1,238 lines total)

```yaml
---
name: frontend-skill
description: Use when the task asks for a visually strong landing page, website, app, prototype, demo, or game UI. This skill enforces restrained composition, image-led hierarchy, cohesive content structure, and tasteful motion while avoiding generic cards, weak branding, and UI clutter. Activate for any visual interface work — even simple requests like "build a homepage" or "create a dashboard" benefit from these principles. Do NOT use for purely headless/API-only work or CLI tools with no visual output.
---
```

**What was added to SKILL.md:**
- Responsive & Mobile section (content-driven breakpoints, fluid type, touch targets)
- Accessibility section (WCAG AA, contrast, focus, keyboard, reduced motion)
- Performance section (Core Web Vitals, image optimization, bundle splitting)
- Theming & Dark Mode section (semantic tokens, FOUC prevention)
- When NOT to Use section (CLI, headless, email templates)
- 3 additional litmus checks

**New reference files:**
- `references/component-patterns.md` (357 lines) — nav, modals, toasts, forms, loading, layout
- `references/accessibility.md` (264 lines) — WCAG checklist, contrast, focus, ARIA, testing
- `references/animation-recipes.md` (345 lines) — 8 Motion patterns with TypeScript

### Key Improvements

| Pattern | What Changed |
|---------|-------------|
| **Pushy description** | Added "Activate for any visual interface work" and explicit negative boundary |
| **Progressive disclosure** | Moved all code recipes to references, kept principles in SKILL.md |
| **Concrete numbers** | "4.5:1 contrast", "44×44px touch targets", "LCP < 2.5s" instead of vague guidance |
| **When NOT to Use** | Clear boundary prevents false triggering on CLI/API work |

---

## Example 2: Bash Skill — Fixing a Broken Reference and Adding Depth

### Before (40 lines + 1 reference file, 746 lines total)

**Problems found:**
1. Both reference pointers linked to the same file (duplicate link bug)
2. Description mentioned "Makefile targets" but zero Makefile content existed
3. No testing/linting guidance (shellcheck, bats)
4. No process management patterns
5. No "When NOT to Use" boundary
6. Zsh section was 5 thin bullets with no code

### After (66 lines + 3 reference files, 1,464 lines total)

**What was fixed in SKILL.md:**
- Fixed duplicate reference link → 3 distinct pointers to 3 distinct files
- Added Testing & Linting section (shellcheck + bats)
- Added When NOT to Use (switch to Go/Python at ~200 lines)
- Pushed description wider ("CI pipeline scripts", "process automation")

**New reference files:**
- `references/makefile-patterns.md` (247 lines) — variables, Docker, multi-platform, guards
- `references/testing-linting.md` (308 lines) — ShellCheck, Bats, testable scripts, CI

**Updated reference:**
- `references/bash-fundamentals.md` (706 → 843 lines) — added Process Management, Pipe Composition, trimmed Zsh

### Key Improvements

| Pattern | What Changed |
|---------|-------------|
| **Broken reference** | Critical bug — both bullets pointed to same file. Fixed by creating the missing file |
| **Description matches content** | "Makefile targets" in description now has a real 247-line reference backing it |
| **Boundary setting** | "Switch to Go at ~200 lines" prevents bash misuse for complex logic |

---

## Example 3: Terraform Skill — Trimming Bloat and Adding GCP

### Before (516 lines + 6 reference files, 4,607 lines total)

**Problems found:**
1. SKILL.md at 516 lines exceeded the 500-line limit
2. Code examples duplicated content already in reference files
3. All examples were AWS — no GCP despite user's GCP-first rules
4. No state operations (import, moved, removed blocks)
5. No workspaces guidance
6. GitHub Actions used stale v2/v3 versions

### After (317 lines + 6 reference files, 4,693 lines total)

**What was trimmed from SKILL.md:**
- Removed all duplicated HCL code blocks (existed in code-patterns.md)
- Kept decision matrices and tables (the most useful parts)
- Condensed Version-Specific Guidance into the Terraform vs OpenTofu summary

**What was added:**
- State Management section (workspaces + state operations table)
- Import/moved/removed blocks with code examples
- GCP patterns in code-patterns.md (GCS backend, Cloud Run, Secrets Manager)
- OpenTofu-specific features in quick-reference.md (state encryption, early eval)
- All GitHub Actions bumped to v4

### Key Improvements

| Pattern | What Changed |
|---------|-------------|
| **Trim, don't delete** | 199 lines removed from SKILL.md, zero information lost (all in references) |
| **Tables over code** | Decision matrices and feature tables stayed; code blocks moved to references |
| **Match user's stack** | GCP patterns added to match the user's global rules (GCP-first) |
| **Declarative over imperative** | State operations section teaches `import`/`moved`/`removed` blocks before CLI commands |

---

## Improvement Patterns Summary

These patterns recur across all skill improvements:

### 1. Push the Description
Add edge cases, broader categories, and explicit negative boundaries. The AI under-triggers by default.

### 2. Add "When NOT to Use"
Every skill needs a boundary. Without it, the skill triggers on tangentially related prompts and wastes context.

### 3. Move Code to References
SKILL.md should be the decision layer (principles, tables, pointers). Reference files hold the implementation layer (code recipes, checklists, patterns).

### 4. Use Concrete Numbers
"4.5:1 contrast ratio" is better than "ensure sufficient contrast". Numbers are unambiguous and testable.

### 5. Fix What the Description Promises
If the description says "Makefile targets", there must be Makefile content. Audit the description against the actual content.

### 6. Match the User's Stack
Skills should reflect the user's actual technology choices. A GCP user doesn't benefit from AWS-only examples.

### 7. Reference Files Should Be 300-800 Lines
Under 300 is too thin to be worth a separate file. Over 800 starts causing context pressure. Split or merge accordingly.
