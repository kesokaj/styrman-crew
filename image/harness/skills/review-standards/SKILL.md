---
name: review-standards
description: "Per-language code-excellence standards for Go and TypeScript, loaded by the reviewer agent, /review, and /preflight. Load when applying the project review criteria: dead code, function flow, directory layout, simplicity, security-exempt DIY rule, decoupling. Not a review workflow — `/review` dispatches the reviewing."
---

# Review Standards

A standards **library**, not an actor. Whoever is reviewing (the `reviewer` agent, `/review`, `/preflight`) loads this file and judges the diff against the six criteria below. Every finding gets a severity (high / medium / low) and a confidence level, and lands in the docs root under `reviews/` — never chat-only. Docs root: `.agents/docs/` if it exists, else `docs/` if it exists, else create `.agents/docs/`.

Language deep-dives (load only for the languages present in the diff):

- [references/go-review.md](references/go-review.md) — Go checklist
- [references/typescript-review.md](references/typescript-review.md) — TypeScript checklist

## 1. Dead code — flag for REMOVAL, not for a comment

- [ ] Exported functions, types, and constants with zero callers outside their own tests.
- [ ] Unused dependencies (`go mod tidy` diff; `npm ls`/depcheck for TS) and unused imports.
- [ ] Commented-out code blocks — version control remembers; the file must not.
- [ ] Unreachable branches: conditions that can never be true, code after unconditional return/throw, feature flags that no longer vary.
- [ ] Leftover debug output and temporary scaffolding added for the change (prints, `console.log`, debug logs).
- [ ] Verdict is always "delete", with the evidence (no callers, no references). Never suggest wrapping dead code in a comment or a TODO.

## 2. Flow tracing — every changed function, caller to callee

Installers count as callers. A change that an installer, updater or converge step must act on (a unit `[Install]` section, a new flag, a changed default) is traced to EVERY such step, and to the question "which version of it runs the first time after this ships?" — a self-updating tool runs its OLD code on the tick that installs the change, so a step guarded by "only when something changed" never runs. Require the step to be unconditional or idempotent, and a post-deploy check by the exact command (`systemctl is-enabled …`), not by reading the unit file.

- [ ] Trace each changed function from its callers down through its callees; confirm the change is consistent at every hop (arguments, invariants, locking, units).
- [ ] Error paths propagate: Go — every `err` wrapped with context (`fmt.Errorf("...: %w", err)`) or explicitly handled, never discarded with `_` or an empty branch; TS — no floating promises (every promise awaited, returned, or explicitly voided with a reason), caught errors typed `unknown` and narrowed.
- [ ] Context and cancellation flow end-to-end: Go — `context.Context` passed down, honored in loops and I/O, goroutines have an exit path; TS — `AbortSignal` reaches fetches/long operations that outlive a request or component.
- [ ] Resource lifetimes: everything opened is closed on every path, including the error paths.
- [ ] Every new test can FAIL: would it still pass if the behaviour it names were reverted? Reject tests that assert on source text rather than behaviour, that re-assert a constant against itself, or that exercise a path the test setup makes unreachable. A test that cannot fail is worse than no test — it reports coverage that does not exist.
- [ ] A write that can fail halfway leaves no partial state; a check-then-act on a file or row is not a TOCTOU race; a fallback does not mask the error it replaces; empty, NaN, Unicode and timezone inputs are handled where the input allows them.
- [ ] A source change whose generated counterpart (route tree, protobuf, `go generate` output, embedded build) is absent from the diff.
- [ ] An assertion changed in an existing test traces to a behaviour change in the source diff; a suppression directive, skip, `.only` or lint-config exclusion the diff adds names its rule or condition and a reason other than making the check pass — otherwise it is a finding.

## 3. Standards conformance — the language skills are the authority

- [ ] Go changes conform to the `golang` skill; TypeScript to the `typescript` skill; UI/TanStack Start work to the `frontend` skill. Cite the specific rule when flagging.
- [ ] Where this file and a language skill overlap, the language skill wins for idiom; this file wins for review scope and severity.
- [ ] Formatting/lint gates already automated (gofmt, vet, eslint) are verified as run, not re-litigated line-by-line.
- [ ] Comments, doc-comments, test names and spec rows that name behaviour the diff changed still describe it — the global context's describing-text rule; a stale one is a finding, not a comment request.

## 4. Directory layout

- [ ] Go: entrypoints in `cmd/<app>/`, private code in `internal/`, `pkg/` only for deliberately public libraries — not as a dumping ground.
- [ ] Frontend fully contained in one `web/` or `ui/` directory inside the Go project; no stray JS/TS at the repo root.
- [ ] Tests live beside the code they cover (`foo_test.go` next to `foo.go`, `foo.test.ts` next to `foo.ts`) — not in a parallel tree.
- [ ] New files land where an unfamiliar engineer would look for them first; flag surprising placement even when it compiles.

## 5. Simplicity — stdlib-first, with one hard exception

- [ ] Prefer the standard library; every new dependency is justified in the PR description or the changelog entry — "saves 20 lines" is not a justification.
- [ ] Flag speculative abstraction: interfaces with one implementation and no test double, config for values that never vary, layers that only forward calls.
- [ ] **Security exemption (mandatory, overrides stdlib-first):** never hand-rolled auth, crypto, password hashing, session handling, or JWT/OIDC — vetted libraries only. DIY code in these areas is a high-severity finding regardless of quality.
- [ ] The simplest correct solution wins; complexity must buy something observable (performance, correctness, a real second caller).

## 6. Decoupling

- [ ] Small interfaces at the seams — use the `codebase-design` skill's vocabulary (module, interface, seam, adapter, depth): prefer deep modules, flag shallow ones whose interface is as complex as their implementation.
- [ ] Interfaces declared where they are consumed, sized to what the consumer needs — not exported wholesale from the implementing package.
- [ ] No cross-package reach-ins: no importing another package's internals, no reaching through a struct to a third package's state, no test importing another package's unexported helpers.
- [ ] Files independently editable: two engineers changing adjacent features should not collide in the same file; flag god-files and grab-bag `util` packages that force serialized edits.
