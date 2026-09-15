---
name: golang
description: "Golang expert. Use whenever working on Go code, writing Go, reviewing style, or building Go applications. Contains coding standards, error handling rules, and routes to the reference library for detailed patterns."
---

# Golang

You have access to a Go reference library. Consult it when working on specific domains instead of guessing patterns.

**When NOT to use this skill:** Frontend-only work, infrastructure-as-code (use the `terraform` skill), Kubernetes manifests (use the `gke-basics` skill), or non-Go languages.

## Coding Standards

These rules apply to all Go code in this environment.

### Error Handling
- **Never** use `panic()` for recoverable errors — return them explicitly.
- Wrap errors with context: `fmt.Errorf("loading config: %w", err)`.
- Handle every error at the call site — do not discard errors with `_`.

### Structure & Style
- Layout `cmd/<app>/` + `internal/`; `pkg/` only for code another module imports.
- Always run `gofmt` / `goimports`. Code must pass `go vet` and `staticcheck`. A vet, lint or test failure is fixed in the source; a `//nolint` or a skip is for a demonstrated false positive and names the linter and the reason.
- Prefer focused, single-responsibility files.
- Exported symbols must have a doc comment following `godoc` conventions.
- No global mutable state — pass dependencies explicitly, wired by hand in `main()`.
- No `init()` functions unless absolutely required (e.g., registering database drivers).

### Logging
- Use `log/slog` with **JSON output** for all structured logging.
- Include contextual fields (request IDs, resource names).
- Never log sensitive data (tokens, passwords, API keys, PII).

### Concurrency
- Prefer channels over mutexes for coordination.
- Always use `context.Context` for cancellation and timeouts.
- Never launch goroutines without a way to stop them.

### Dependencies
- Standard library first. Every external dependency is justified in the change entry before it is added.
- No assertion, mock, or DI library by default: `testing`, hand-written fakes, and constructors in `main()`.

## Reference Library

Each file is a set of sub-topics; read the one that matches the work before writing code.

| Domain | File |
|---|---|
| Core language: concurrency, context, data structures, errors, safety | `./references/golang-core.md` |
| Architecture: code style, naming, design patterns, modernizing old code | `./references/golang-architecture.md` |
| Structure: structs and interfaces, project layout, dependency injection | `./references/golang-structure.md` |
| Testing: unit tests, table-driven tests, fakes, fuzzing, benchmarks | `./references/golang-testing.md` |
| I/O & Web: HTTP, gRPC, database/sql, CLI | `./references/golang-io-web.md` |
| Ecosystem: observability, dependency management, linters, troubleshooting, security, performance, libraries, documentation | `./references/golang-ecosystem.md` |
| Samber libraries (lo, mo, oops, ro, hot, slog-*): only when stdlib is demonstrably not enough | `./references/golang-samber.md` |

## Further Reading

- [Google Go Style Guide](https://google.github.io/styleguide/go/) — Google's internal Go conventions (used as a reference, not adopted wholesale)
