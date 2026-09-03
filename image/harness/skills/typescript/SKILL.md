---
name: typescript
description: |
  TypeScript language standards: typing discipline, tsconfig, generics, narrowing,
  module/ESM conventions, and runtime validation at boundaries. Use when writing or
  reviewing TypeScript code, fixing type errors, designing types/interfaces, or
  configuring tsconfig — in any TS codebase, frontend or tooling.
  Do NOT use for UI, styling, components, or TanStack Start conventions (use frontend),
  backend API logic (use golang), or shell automation (use bash).
---

# TypeScript

Language-level standards for all TypeScript code. Framework and UI conventions live in the `frontend` skill.

## Compiler Baseline

- `strict: true` always — plus `noUncheckedIndexedAccess` and `noImplicitOverride`.
- Target the platform you deploy to; don't down-level further than needed.
- One config per package; extend a shared base instead of copying options.

## Typing Discipline

- **No `any`.** Use `unknown` at untrusted boundaries and narrow before use.
- Type every exported function signature, prop, and API payload explicitly.
- Prefer type inference for locals — annotate boundaries, not every variable.
- Model variants as **discriminated unions**, not optional-field grab bags or enums.
- Use `satisfies` for config objects to keep literal types while enforcing shape.
- Prefer `readonly` arrays/properties for data that shouldn't mutate.
- Derive types from values where possible (`typeof`, `ReturnType`, `as const`) instead of maintaining parallel type declarations.

## Boundaries & Validation

- Compile-time types vanish at runtime: validate external input (HTTP responses, env vars, form data, JSON) at the boundary, then trust types inward.
- Keep API payload types in one shared location (`lib/`) — never redefine them per call site.

## Modules & Style

- ESM only: `import`/`export`, no `require`. Use `import type` for type-only imports.
- No namespaces, no `/// <reference>`, no default exports for shared modules — named exports keep renames refactor-safe.
- Errors: throw `Error` subclasses, never strings; type caught errors as `unknown` and narrow.

## Escape Hatches

- `as` casts and `!` non-null assertions are last resorts — each one needs a comment explaining why the compiler is wrong.
- `@ts-expect-error` (with reason) over `@ts-ignore`; fix the type instead when feasible.
