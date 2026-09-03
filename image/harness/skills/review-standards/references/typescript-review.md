# TypeScript review checklist

Concrete checks for TypeScript diffs. Language authority: the `typescript` skill; UI/TanStack Start conventions: the `frontend` skill. Cite file:line in findings.

## Typing discipline

- [ ] No `any` — including implicit `any` smuggled in via untyped JSON, `as any`, or loose generics. Untrusted input enters as `unknown` and is narrowed.
- [ ] Type assertions (`as X`) are findings unless accompanied by a runtime check or an adjacent comment proving the invariant; `!` non-null assertions get the same bar.
- [ ] `strict: true` assumptions hold: no disabling flags in the diff (`// @ts-ignore`, `// @ts-expect-error` without a linked issue, tsconfig loosening).
- [ ] Discriminated unions + exhaustive `switch` (with a `never` default) for state machines and variant types — flag boolean-flag combinatorics that a union would eliminate.
- [ ] Derive types from values where a source of truth exists (`typeof config`, `z.infer<...>`, `ReturnType<...>`) instead of writing parallel type declarations that can drift.

## Async & errors

- [ ] No floating promises: every promise is `await`ed, `return`ed, or explicitly `void`-ed with a comment saying why fire-and-forget is safe.
- [ ] `Promise.all` (or `allSettled`) for independent awaits — flag sequential `await`s in a loop that could run concurrently.
- [ ] `catch (err)` treats `err` as `unknown`: narrowed via `instanceof` before use; no `err.message` on an unnarrowed value.
- [ ] Thrown values are `Error` subclasses, never strings or object literals; rethrows use `{ cause: err }` to preserve the chain.
- [ ] Long-lived or user-cancellable operations accept an `AbortSignal` and pass it to `fetch`; listeners and timers registered in a scope are cleaned up when it ends.

## Boundaries & validation

- [ ] Every external boundary — HTTP responses, request bodies, env vars, storage reads, message payloads — validated at runtime (zod or equivalent) before the data is typed; `JSON.parse` result never cast straight to a domain type.
- [ ] Server/client boundary respected in TanStack Start: secrets and privileged clients only in server functions; nothing Node-only leaks into shipped client code (plain-V8 rule).
- [ ] Env access centralized in one validated config module — not `process.env.X` scattered through features.

## Modules & structure

- [ ] ESM only; imports use explicit named symbols — flag `export default` for anything but framework-mandated files (routes) and `import *` grab-bags.
- [ ] No circular imports (they surface as `undefined` at runtime under ESM hoisting); dependency direction flows feature → shared, never shared → feature.
- [ ] All frontend code stays inside the one `web/` or `ui/` directory; module side effects at import time (I/O, global mutation) are findings.
- [ ] Barrel files (`index.ts` re-export hubs) only at package boundaries — flag intra-feature barrels that hide the real dependency graph.

## Tests

- [ ] Changed behavior has a covering test beside the source (`foo.test.ts` next to `foo.ts`), run under the repo's runner (vitest by default).
- [ ] Tests exercise the public surface with realistic data — flag mocks of the module under test or assertions on internal call order.
- [ ] Async tests await their assertions; a test that passes when its promise is dropped is a finding.
- [ ] Type-level regressions guarded where types are the contract: `expect-type`/`ts-expect-error` cases for exported generics.
