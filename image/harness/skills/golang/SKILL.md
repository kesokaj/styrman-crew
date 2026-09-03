---
name: golang
description: "Golang expert. Use whenever working on Go code, writing Go, reviewing style, or building Go applications. Contains coding standards, error handling rules, and routes to the reference library for detailed patterns."
---

# Golang

You have access to a comprehensive Go reference library. Consult it when working on specific domains instead of guessing patterns.

**When NOT to use this skill:** Frontend-only work, infrastructure-as-code (use the `terraform` skill), Kubernetes manifests (use the `gke-basics` skill), or non-Go languages.

## Coding Standards

These rules apply to all Go code in this environment.

### Error Handling
- **Never** use `panic()` for recoverable errors — return them explicitly.
- Wrap errors with context: `fmt.Errorf("loading config: %w", err)`.
- Handle every error at the call site — do not discard errors with `_`.

### Structure & Style
- Adhere to standard Go directory layout: `cmd/`, `internal/`, `pkg/`.
- Always run `gofmt` / `goimports`. Code must pass `go vet` and `staticcheck`.
- Prefer focused, single-responsibility files.
- Exported symbols must have a doc comment following `godoc` conventions.
- No global mutable state — pass dependencies explicitly.
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
- Prefer the standard library. Justify external dependencies before adding them.
- Never add dependencies without a clear reason.

## Reference Library

For detailed patterns, examples, and templates, consult the reference files in `./references/`:

* **Core Language Mechanics (Concurrency, Errors, Data Structures)**
  Read → `./references/golang-core.md`

* **Architecture (Code Style, Naming, Layout, Go Design Patterns)**
  Read → `./references/golang-architecture.md`

* **Testing (Unit Tests, Benchmarks, Stretchr/Testify)**
  Read → `./references/golang-testing.md`

* **I/O & Web (HTTP, gRPC, Database/SQL, CLI patterns)**
  Read → `./references/golang-io-web.md`

* **Ecosystem (CI/CD, Observability, Linters, Samber UI libraries)**
  Read → `./references/golang-ecosystem.md`

> [!TIP]
> When the user asks you to implement a specific sub-system (e.g. database logic or a concurrency worker pool), immediately read the relevant reference file before writing code.

## Further Reading

- [Google Go Style Guide](https://google.github.io/styleguide/go/) — Google's internal Go conventions covering guide, decisions, and best practices (used as a reference, not adopted wholesale)
