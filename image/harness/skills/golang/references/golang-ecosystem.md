# Domain: golang-ecosystem

## Sub-topic: golang-observability

**Persona:** You are a Go observability engineer. You treat every unobserved production system as a liability — instrument proactively, correlate signals to diagnose, and never consider a feature done until it is observable.

**Modes:**

- **Coding / instrumentation** (default): Add observability to new or existing code — declare metrics, add spans, set up structured logging, wire pprof toggles. Follow the sequential instrumentation guide.
- **Review mode** — reviewing a PR's instrumentation changes. Check that new code exports the expected signals (metrics declared, spans opened and closed, structured log fields consistent). Sequential.
- **Audit mode** — auditing existing observability coverage across a codebase. Launch up to 5 parallel sub-agents — one per signal (metrics, logging, tracing, profiling, RUM) — to check coverage simultaneously.

# Go Observability Best Practices

Observability is the ability to understand a system's internal state from its external outputs. In Go services, this means five complementary signals: **logs**, **metrics**, **traces**, **profiles**, and **RUM**. Each answers different questions, and together they give you full visibility into both system behavior and user experience.

When using observability libraries (Prometheus client, OpenTelemetry SDK, vendor integrations), refer to the library's official documentation and code examples for current API signatures.

## Best Practices Summary

1. **Use structured logging** with `log/slog` — production services MUST emit structured logs (JSON), not freeform strings
2. **Choose the right log level** — Debug for development, Info for normal operations, Warn for degraded states, Error for failures requiring attention
3. **Log with context** — use `slog.InfoContext(ctx, ...)` to correlate logs with traces
4. **Prefer Histogram over Summary** for latency metrics — Histograms support server-side aggregation and percentile queries. Every HTTP endpoint MUST have latency and error rate metrics.
5. **Keep label cardinality low** in Prometheus — NEVER use unbounded values (user IDs, full URLs) as label values
6. **Track percentiles** (P50, P90, P99, P99.9) using Histograms + `histogram_quantile()` in PromQL
7. **Set up OpenTelemetry tracing on new projects** — configure the TracerProvider early, then add spans everywhere
8. **Add spans to every meaningful operation** — service methods, DB queries, external API calls, message queue operations
9. **Propagate context everywhere** — context is the vehicle that carries trace_id, span_id, and deadlines across service boundaries
10. **Enable profiling via environment variables** — toggle pprof and continuous profiling on/off without redeploying
11. **Correlate signals** — inject trace_id into logs, use exemplars to link metrics to traces
12. **A feature is not done until it is observable** — declare metrics, add proper logging, create spans
13. **Use [awesome-prometheus-alerts](https://samber.github.io/awesome-prometheus-alerts/) as a starting point** for infrastructure and dependency alerting — browse by technology, copy rules, customize thresholds

## Cross-References

See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for the single handling rule. See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for using observability signals to diagnose production issues. See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for protecting pprof endpoints and avoiding PII in logs. See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for propagating trace context across service boundaries. See `samber/cc-skills@promql-cli` skill for querying and exploring PromQL expressions against Prometheus from the CLI.

## The Five Signals

| Signal | Question it answers | Tool | When to use |
| --- | --- | --- | --- |
| **Logs** | What happened? | `log/slog` | Discrete events, errors, audit trails |
| **Metrics** | How much / how fast? | Prometheus client | Aggregated measurements, alerting, SLOs |
| **Traces** | Where did time go? | OpenTelemetry | Request flow across services, latency breakdown |
| **Profiles** | Why is it slow / using memory? | pprof, Pyroscope | CPU hotspots, memory leaks, lock contention |
| **RUM** | How do users experience it? | PostHog, Segment | Product analytics, funnels, session replay |

## Detailed Guides

Each signal has a dedicated guide with full code examples, configuration patterns, and cost analysis:

- **Structured Logging** — Why structured logging matters for log aggregation at scale. Covers `log/slog` setup, log levels (Debug/Info/Warn/Error) and when to use each, request correlation with trace IDs, context propagation with `slog.InfoContext`, request-scoped attributes, the slog ecosystem (handlers, formatters, middleware), and migration strategies from zap/logrus/zerolog.

- **Metrics Collection** — Prometheus client setup and the four metric types (Counter for rate-of-change, Gauge for snapshots, Histogram for latency aggregation). Deep dive: why Histograms beat Summaries (server-side aggregation, supports `histogram_quantile` PromQL), naming conventions, the PromQL-as-comments convention (write queries above metric declarations for discoverability), production-grade PromQL examples, multi-window SLO burn rate alerting, and the high-cardinality label problem (why unbounded values like user IDs destroy performance).

- **Distributed Tracing** — When and how to use OpenTelemetry SDK to trace request flows across services. Covers spans (creating, attributes, status recording), `otelhttp` middleware for HTTP instrumentation, error recording with `span.RecordError()`, trace sampling (why you can't collect everything at scale), propagating trace context across service boundaries, and cost optimization.

- **Profiling** — On-demand profiling with pprof (CPU, heap, goroutine, mutex, block profiles) — how to enable it in production, secure it with auth, and toggle via environment variables without redeploying. Continuous profiling with Pyroscope for always-on performance visibility. Cost implications of each profiling type and mitigation strategies.

- **Real User Monitoring** — Understanding how users actually experience your service. Covers product analytics (event tracking, funnels), Customer Data Platform integration, and critical compliance: GDPR/CCPA consent checks, data subject rights (user deletion endpoints), and privacy checklist for tracking. Server-side event tracking (PostHog, Segment) and identity key best practices.

- **Alerting** — Proactive problem detection. Covers the four golden signals (latency, traffic, errors, saturation), [awesome-prometheus-alerts](https://samber.github.io/awesome-prometheus-alerts/) as a rule library with ~500 ready-to-use rules by technology, Go runtime alerts (goroutine leaks, GC pressure, OOM risk), severity levels, and common mistakes that break alerting (using `irate` instead of `rate`, missing `for:` duration to avoid flapping).

- **Grafana Dashboards** — Prebuilt dashboards for Go runtime monitoring (heap allocation, GC pause frequency, goroutine count, CPU). Explains the standard dashboards to install, how to customize them for your service, and when each dashboard answers a different operational question.

## Correlating Signals

Signals are most powerful when connected. A trace_id in your logs lets you jump from a log line to the full request trace. An exemplar on a metric links a latency spike to the exact trace that caused it.

### Logs + Traces: `otelslog` bridge

```go
import "go.opentelemetry.io/contrib/bridges/otelslog"

// Create a logger that automatically injects trace_id and span_id
logger := otelslog.NewHandler("my-service")
slog.SetDefault(slog.New(logger))

// Now every slog call with context includes trace correlation
slog.InfoContext(ctx, "order created", "order_id", orderID)
// Output includes: {"trace_id":"abc123", "span_id":"def456", "msg":"order created", ...}
```

### Metrics + Traces: Exemplars

```go
// When recording a histogram observation, attach the trace_id as an exemplar
// so you can jump from a P99 spike directly to the offending trace
histogram.WithLabelValues("POST", "/orders").
    Exemplar(prometheus.Labels{"trace_id": traceID}, duration)
```

## Migrating Legacy Loggers

If the project currently uses `zap`, `logrus`, or `zerolog`, migrate to `log/slog`. It is the standard library logger since Go 1.21, has a stable API, and the ecosystem has consolidated around it. Continuing with third-party loggers means maintaining an extra dependency for no benefit.

**Migration strategy:**

1. Add `slog` as the new logger with `slog.SetDefault()`
2. Use bridge handlers during migration to route slog output through the existing logger: [samber/slog-zap](https://github.com/samber/slog-zap), [samber/slog-logrus](https://github.com/samber/slog-logrus), [samber/slog-zerolog](https://github.com/samber/slog-zerolog)
3. Gradually replace all `zap.L().Info(...)` / `logrus.Info(...)` / `log.Info().Msg(...)` calls with `slog.Info(...)`
4. Once fully migrated, remove the bridge handler and the old logger dependency

## Definition of Done for Observability

A feature is not production-ready until it is observable. Before marking a feature as done, verify:

- [ ] **Metrics declared** — counters for operations/errors, histograms for latencies, gauges for saturation. Each metric var has PromQL queries and alert rules as comments above its declaration.
- [ ] **Logging is proper** — structured key-value pairs with `slog`, context variants used (`slog.InfoContext`), no PII in logs, errors MUST be either logged OR returned (NEVER both).
- [ ] **Spans created** — every service method, DB query, and external API call has a span with relevant attributes, errors recorded with `span.RecordError()`.
- [ ] **Dashboards and alerts exist** — the PromQL from your metric comments is wired into Grafana dashboards and Prometheus alerting rules. Check [awesome-prometheus-alerts](https://samber.github.io/awesome-prometheus-alerts/) for ready-to-use rules covering your infrastructure dependencies (databases, caches, brokers, proxies).
- [ ] **RUM events tracked** — key business events tracked server-side (PostHog/Segment), identity key is `user_id` (not email), consent checked before tracking.

## Common Mistakes

```go
// ✗ Bad — log AND return (error gets logged multiple times up the chain)
if err != nil {
    slog.Error("query failed", "error", err)
    return fmt.Errorf("query: %w", err)
}

// ✓ Good — return with context, log once at the top level
if err != nil {
    return fmt.Errorf("querying users: %w", err)
}
```

```go
// ✗ Bad — high-cardinality label (unbounded user IDs)
httpRequests.WithLabelValues(r.Method, r.URL.Path, userID).Inc()

// ✓ Good — bounded label values only
httpRequests.WithLabelValues(r.Method, routePattern).Inc()
```

```go
// ✗ Bad — not passing context (breaks trace propagation)
result, err := db.Query("SELECT ...")

// ✓ Good — context flows through, trace continues
result, err := db.QueryContext(ctx, "SELECT ...")
```

```go
// ✗ Bad — using Summary for latency (can't aggregate across instances)
prometheus.NewSummary(prometheus.SummaryOpts{
    Name:       "http_request_duration_seconds",
    Objectives: map[float64]float64{0.99: 0.001},
})

// ✓ Good — use Histogram (aggregatable, supports histogram_quantile)
prometheus.NewHistogram(prometheus.HistogramOpts{
    Name:    "http_request_duration_seconds",
    Buckets: prometheus.DefBuckets,
})
```

## Sub-topic: golang-dependency-management

**Persona:** You are a Go dependency steward. You treat every new dependency as a long-term maintenance commitment — you ask whether the standard library already solves the problem before reaching for an external package.

# Go Dependency Management

## AI Agent Rule: Ask Before Adding Dependencies

**Before running `go get` to add any new dependency, AI agents MUST ask the user for confirmation.** AI agents can suggest packages that are unmaintained, low-quality, or unnecessary when the standard library already provides equivalent functionality. Using `go get -u` to upgrade an existing dependency is safe.

Before proposing a dependency, evaluate:

- Does the standard library already cover the use case?
- Is the license compatible?
- Are there well-known alternatives?
- What it does and why it's needed?

The popular-libraries sub-topic below says how to judge a library. When no vetted option exists, favor well-known packages from the Go team (`golang.org/x/...`) or established organizations over obscure alternatives.

## Key Rules

- `go.sum` MUST be committed — it records cryptographic checksums of every dependency version, letting `go mod verify` detect supply-chain tampering. Without it, a compromised proxy could silently substitute malicious code
- `govulncheck ./...` before every release — catches known CVEs in your dependency tree before they reach production
- Check maintenance status, license, and stdlib alternatives before adding a dependency — every dependency increases attack surface, maintenance burden, and binary size
- `go mod tidy` before every commit that changes dependencies — removes unused modules and adds missing ones, keeping go.mod honest

## go.mod & go.sum

### Essential Commands

| Command           | Purpose                                      |
| ----------------- | -------------------------------------------- |
| `go mod tidy`     | Add missing deps, remove unused ones         |
| `go mod download` | Download modules to local cache              |
| `go mod verify`   | Verify cached modules match go.sum checksums |
| `go mod vendor`   | Copy deps into `vendor/` directory           |
| `go mod edit`     | Edit go.mod programmatically (scripts, CI)   |
| `go mod graph`    | Print the module requirement graph           |
| `go mod why`      | Explain why a module or package is needed    |

### Vendoring

Use `go mod vendor` when you need hermetic builds (no network access), reproducibility guarantees beyond checksums, or when deploying to environments without module proxy access. CI pipelines and Docker builds sometimes benefit from vendoring. Run `go mod vendor` after any dependency change and commit the `vendor/` directory.

## Installing & Upgrading Dependencies

### Adding a Dependency

```bash
go get github.com/pkg/errors           # Latest version
go get github.com/pkg/errors@v0.9.1    # Specific version
go get github.com/pkg/errors@latest    # Explicitly latest
go get github.com/pkg/errors@master    # Specific branch (pseudo-version)
```

### Upgrading

```bash
go get -u ./...            # Upgrade ALL direct+indirect deps to latest minor/patch
go get -u=patch ./...      # Upgrade to latest patch only (safer)
go get github.com/pkg@v1.5 # Upgrade specific package
```

**Prefer `go get -u=patch`** for routine updates — patch versions change no public API (semver promise), so they're unlikely to break your build. Minor version upgrades may add new APIs but can also deprecate or change behavior unexpectedly.

### Removing a Dependency

```bash
go get github.com/pkg/errors@none   # Mark for removal
go mod tidy                          # Clean up go.mod and go.sum
```

### Installing CLI Tools

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

`go install` builds and installs a binary to `$GOPATH/bin`. Use `@latest` or a specific version tag — never `@master` for tools you depend on.

### The tools.go Pattern

Pin tool versions in your module without importing them in production code:

```go
//go:build tools

package tools

import (
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
    _ "golang.org/x/vuln/cmd/govulncheck"
)
```

The build constraint ensures this file is never compiled. The blank imports keep the tools in `go.mod` so `go install` uses the pinned version. Run `go mod tidy` after creating this file.

## Deep Dives

- **Versioning & MVS** — Semantic versioning rules (major.minor.patch), when to increment each number, pre-release versions, the Minimal Version Selection (MVS) algorithm (why you can't just pick "latest"), and major version suffix conventions (v0, v1, v2 suffixes for breaking changes).

- **Auditing Dependencies** — Vulnerability scanning with `govulncheck`, tracking outdated dependencies, analyzing which dependencies make the binary large (`goweight`), and distinguishing test-only vs binary dependencies to keep `go.mod` clean.

- **Dependency Conflicts & Resolution** — Diagnosing version conflicts (what `go get` does when you request incompatible versions), resolution strategies (`replace` directives for local development, `exclude` for broken versions, `retract` for published versions that should be skipped), and workflows for conflicts across your dependency tree.

- **Go Workspaces** — `go.work` files for multi-module development (e.g., library + example application), when to use workspaces vs monorepos, and workspace best practices.

- **Automated Dependency Updates** — Setting up Dependabot or Renovate for automatic dependency update PRs, auto-merge strategies (when to merge automatically vs require review), and handling security updates.

- **Visualizing the Dependency Graph** — `go mod graph` to inspect the full dependency tree, `modgraphviz` to visualize it, and interactive tools to find which dependency chains cause bloat.

## Cross-References

## Quick Reference

```bash
# Start a new module
go mod init github.com/user/project

# Add a dependency
go get github.com/pkg/errors@v0.9.1

# Upgrade all deps (patch only, safer)
go get -u=patch ./...

# Remove unused deps
go mod tidy

# Check for vulnerabilities
govulncheck ./...

# Check for outdated deps
go list -u -m -json all | go-mod-outdated -update -direct

# Analyze binary size by dependency
goweight

# Understand why a dep exists
go mod why -m github.com/some/module

# Visualize dependency graph
go mod graph | modgraphviz | dot -Tpng -o deps.png

# Verify checksums
go mod verify
```

## Sub-topic: golang-linter

**Persona:** You are a Go code quality engineer. You treat linting as a first-class part of the development workflow — not a post-hoc cleanup step.

**Modes:**

- **Setup mode** — configuring `.golangci.yml` and choosing linters: follow the configuration and workflow sections sequentially.
- **Coding mode** — writing new Go code: launch a background agent running `golangci-lint run --fix` on the modified files only while the main agent continues implementing the feature; surface results when it completes.
- **Interpret/fix mode** — reading lint output, suppressing warnings, fixing issues on existing code: start from "Interpreting Output" and "Suppressing Lint Warnings"; use parallel sub-agents for large-scale legacy cleanup.

# Go Linting

## Overview

`golangci-lint` is the standard Go linting tool. It aggregates 100+ linters into a single binary, runs them in parallel, and provides a unified configuration format. Run it frequently during development and before every commit.

A Go project that runs golangci-lint keeps a `.golangci.yml` as the **source of truth** for which linters are enabled and how they are configured; `go vet` and `staticcheck` are the baseline every Go project passes (`golang` skill). Start from a small set (`govet`, `staticcheck`, `errcheck`, `gosec`, `unused`, `misspell`, `nolintlint`) and add linters when a real defect motivates each one.

## Quick Reference

```bash
# Run all configured linters
golangci-lint run ./...

# Auto-fix issues where possible
golangci-lint run --fix ./...

# Format code (golangci-lint v2+)
golangci-lint fmt ./...

# Run a single linter only
golangci-lint run --enable-only govet ./...

# List all available linters
golangci-lint linters

# Verbose output with timing info
golangci-lint run --verbose ./...
```

## Configuration

Keep `.golangci.yml` in the repo root and commit it; `golangci-lint linters` lists what is available and what each linter checks (correctness, style, complexity, performance, security).

## Suppressing Lint Warnings

Use `//nolint` directives sparingly — fix the root cause first.

```go
// Good: specific linter + justification
//nolint:errcheck // fire-and-forget logging, error is not actionable
_ = logger.Sync()

// Bad: blanket suppression without reason
//nolint
_ = logger.Sync()
```

Rules:

1. **//nolint directives MUST specify the linter name**: `//nolint:errcheck` not `//nolint`
2. **//nolint directives MUST include a justification comment**: `//nolint:errcheck // reason`
3. **The `nolintlint` linter enforces both rules above** — it flags bare `//nolint` and missing reasons
4. **NEVER suppress security linters** (bodyclose, sqlclosecheck) without a very strong reason

For comprehensive patterns and examples, see **nolint directives** — when to suppress, how to write justifications, patterns for per-line vs per-function suppression, and anti-patterns.

## Development Workflow

1. **Linters SHOULD be run after every significant change**: `golangci-lint run ./...`
2. **Auto-fix what you can**: `golangci-lint run --fix ./...`
3. **Format before committing**: `golangci-lint fmt ./...`
4. **Incremental adoption on legacy code**: set `issues.new-from-rev` in `.golangci.yml` to only lint new/changed code, then gradually clean up old code

Makefile targets (recommended):

```makefile
lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

fmt:
	golangci-lint fmt ./...
```

## Interpreting Output

Each issue follows this format:

```
path/to/file.go:42:10: message describing the issue (linter-name)
```

The linter name in parentheses tells you which linter flagged it. Use this to:

- Look up the linter in the reference to understand what it checks
- Suppress with `//nolint:linter-name // reason` if it's a false positive
- Use `golangci-lint run --verbose` for additional context and timing

## Common Issues

| Problem | Solution |
| --- | --- |
| "deadline exceeded" | Increase `run.timeout` in `.golangci.yml` (default: 5m) |
| Too many issues on legacy code | Set `issues.new-from-rev: HEAD~1` to lint only new code |
| Linter not found | Check `golangci-lint linters` — linter may need a newer version |
| Conflicts between linters | Disable the less useful one with a comment explaining why |
| v1 config errors after upgrade | Run `golangci-lint migrate` to convert config format |
| Slow on large repos | Reduce `run.concurrency` or exclude directories in `run.skip-dirs` |

## Parallelizing Legacy Codebase Cleanup

When adopting linting on a legacy codebase, use up to 5 parallel sub-agents (via the Agent tool) to fix independent linter categories simultaneously:

- Sub-agent 1: Run `golangci-lint run --fix ./...` for auto-fixable issues
- Sub-agent 2: Fix security linter findings (bodyclose, sqlclosecheck, gosec)
- Sub-agent 3: Fix error handling issues (errcheck, nilerr, wrapcheck)
- Sub-agent 4: Fix style and formatting (gofumpt, goimports, revive)
- Sub-agent 5: Fix code quality (gocritic, unused, ineffassign)

## Cross-References

- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for style rules that linters enforce

## Sub-topic: golang-troubleshooting

**Persona:** You are a Go systems debugger. You follow evidence, not intuition — instrument, reproduce, and trace root causes systematically.

**Thinking mode:** Use `ultrathink` for debugging and root cause analysis. Rushed reasoning leads to symptom fixes — deep thinking finds the actual root cause.

**Modes:**

- **Single-issue debug** (default): Follow the sequential Golden Rules — read the error, reproduce, one hypothesis at a time. Do not launch sub-agents; focused sequential investigation is faster for a single known symptom.
- **Codebase bug hunt** (explicit audit of a large codebase): Launch up to 5 parallel sub-agents, one per bug category (nil/interface, resources, error handling, races, context/slice/map). Use this mode when the user asks for a broad sweep, not when debugging a specific reported issue.

# Go Troubleshooting Guide

**NO FIXES WITHOUT ROOT CAUSE INVESTIGATION FIRST.** Symptom fixes create new bugs and waste time. This process applies ESPECIALLY under time pressure — rushing leads to cascading failures that take longer to resolve.

When the user reports a bug, crash, performance problem, or unexpected behavior in Go code:

1. **Start with the Decision Tree** below to identify the symptom category and jump to the relevant section.
2. **Follow the Golden Rules** — especially: reproduce before you fix, one hypothesis at a time, find the root cause.
3. **Work through the General Debugging Methodology** step by step. Do not skip steps.
4. **Watch for Red Flags** in your own reasoning. If you catch yourself guessing at fixes without understanding the cause, stop and gather more evidence.
5. **Escalate tools incrementally.** Start with the simplest diagnostic (`fmt.Println`, test isolation) and only reach for pprof, Delve, or GODEBUG when simpler tools are insufficient.
6. **Never propose a fix you cannot explain.** If you do not understand why the bug happens, say so and investigate further.

## Quick Decision Tree

```
WHAT ARE YOU SEEING?

"Build won't compile"
  → go build ./... 2>&1, go vet ./...
  → See compilation.md

"Wrong output / logic bug"
  → Write a failing test → Check error handling, nil, off-by-one
  → See common-go-bugs.md, testing-debug.md

"Random crashes / panics"
  → GOTRACEBACK=all ./app → go test -race ./...
  → See common-go-bugs.md, diagnostic-tools.md

"Sometimes works, sometimes fails"
  → go test -race ./...
  → See concurrency-debug.md, testing-debug.md

"Program hangs / frozen"
  → curl localhost:6060/debug/pprof/goroutine?debug=2
  → See concurrency-debug.md, pprof.md

"High CPU usage"
  → pprof CPU profiling
  → See performance-debug.md, pprof.md

"Memory growing over time"
  → pprof heap profiling
  → See performance-debug.md, concurrency-debug.md

"Slow / high latency / p99 spikes"
  → CPU + mutex + block profiles
  → See performance-debug.md, diagnostic-tools.md

"Simple bug, easy to reproduce"
  → Write a test, add fmt.Println / log.Debug
  → See testing-debug.md
```

**Remember:** Read the Error → Reproduce → Measure One Thing → Fix → Verify

Most Go bugs are: missing error checks, nil pointers, forgotten context cancel, unclosed resources, race conditions, or silent error swallowing.

## The Golden Rules

### 1. Read the Error Message First

Go error messages are precise. Read them fully before doing anything else:

- **File and line number** → go directly there
- **Type mismatch** → check function signatures, interface satisfaction
- **"undefined"** → check imports, exported names, build tags
- **"cannot use X as Y"** → check concrete types vs interfaces

### 2. Reproduce Before You Fix

NEVER debug by guessing — reproduce first. Always:

- Write a failing test that captures the bug
- Make it deterministic
- Isolate the minimal failing example
- Use `git bisect` to find the breaking commit

### 3. If You Don't Measure It, You're Guessing

Never rely on intuition for performance or concurrency bugs:

- **pprof over intuition**
- **race detector over reasoning**
- **benchmarks over assumptions**

### 4. One Hypothesis at a Time

Change one thing, measure, confirm. If you change three things at once, you learn nothing.

### 5. Find the Root Cause — No Workarounds

A band-aid fix that masks the symptom IS NOT ACCEPTABLE. You MUST understand **why** the bug happens before writing a fix.

When you don't understand the issue:

- **Trace the data flow backwards** from the symptom to its origin.
- **Question your assumptions.** The code you trust might be wrong.
- **Ask "why" five times.** Keep going until you reach the actual root cause.
- **Perform more troubleshooting checks.** More fmt.Println, more output inspection...

### 6. Research the Codebase, Not Just the Diff

Before flagging a bug or proposing a fix, trace the data flow and check for upstream handling. A function that looks broken in isolation may be correct in context — callers may validate inputs, middleware may enforce invariants, or the surrounding code may guarantee conditions the function relies on.

1. **Trace callers** — who calls this function and with what values? Use Grep/Agent to find all call sites.
2. **Check upstream validation** — input parsing, type conversions, or guard clauses earlier in the chain may make the "bug" unreachable.
3. **Read the surrounding code** — middleware, interceptors, or init functions may set up state the function depends on.

**When the context reduces severity but doesn't eliminate the issue:** still report it at reduced priority with a note explaining which upstream guarantees protect it. Add a brief inline comment (e.g., `// note: safe because caller validates via parseID() which returns uint`) so the reasoning is documented for future reviewers.

### 7. Start Simple

Sometimes `fmt.Println` IS the right tool for local debugging. Escalate tools only when simpler approaches fail. NEVER use `fmt.Println` for production debugging — use `slog`.

## Red Flags: You're Debugging Wrong

If any of these are happening, stop and return to Step 1:

- **"Quick fix for now, investigate later"** — There is no "later". Find the root cause.
- **Multiple simultaneous changes** — One hypothesis at a time.
- **Proposing fixes without understanding the cause** — "Maybe if I add a nil check here..." is guessing, not debugging.
- **Each fix reveals a new problem** — You're treating symptoms. The real bug is elsewhere.
- **3+ fix attempts on the same issue** — You have the wrong mental model. Re-read the code, trace the data flow from scratch.
- **"It works on my machine"** — You haven't isolated the environmental difference.
- **Blaming the framework/stdlib/compiler** — It's almost never a Go bug. Verify your code first.

## Reference Files

- **General Debugging Methodology** — The systematic 10-step process: define symptoms, isolate reproduction, form one hypothesis, test it, verify the root cause, and defend against regressions. Escalation guide: when to escalate from `fmt.Println` to logging to pprof to Delve, and how to avoid the trap of multiple simultaneous changes.

- **Common Go Bugs** — The bugs that crash Go code: nil pointer dereferences, interface nil gotcha (typed nil ≠ nil), variable shadowing, slice/map/defer/error/context pitfalls, race conditions, JSON unmarshaling surprises, unclosed resources. Each with reproduction patterns and fixes.

- **Test-Driven Debugging** — Why writing a failing test is the first step of debugging. Covers test isolation techniques, table-driven test organization for narrowing failures, useful `go test` flags (`-v`, `-run`, `-count=10` for flaky tests), and debugging flaky tests.

- **Concurrency Debugging** — Race conditions, deadlocks, goroutine leaks. When to use the race detector (`-race`), how to read race detector output, patterns that hide races, detecting leaks with `goleak`, analyzing stack dumps for deadlock clues.

- **Performance Troubleshooting** — When your code is slow: CPU profiling workflow, memory analysis (heap vs alloc_objects profiles, finding leaks), lock contention (mutex profile), and I/O blocking (goroutine profile). How to read flamegraphs, identify hot functions, and measure improvement with benchmarks.

- **pprof Reference** — Complete pprof manual. How to enable pprof endpoints in production (with auth), profile types (CPU, heap, goroutine, mutex, block, trace), capturing profiles locally and remotely, interactive analysis commands (`top`, `list`, `web`), and interpreting flamegraphs.

- **Diagnostic Tools** — Auxiliary tools for specific symptoms. GODEBUG environment variables (GC tracing, scheduler tracing), Delve debugger for breakpoint debugging, escape analysis (`go build -gcflags="-m"` to find unintended heap allocations), Go's execution tracer for understanding goroutine scheduling.

- **Production Debugging** — Debugging live production systems without stopping them. Production checklist, structuring logs for searchability, enabling pprof safely (auth, network isolation), capturing profiles from running services, network debugging (tcpdump, netstat), and HTTP request/response inspection.

- **Compilation Issues** — Build failures: module version conflicts, CGO linking problems, version mismatch between `go.mod` and installed Go version, platform-specific build tags preventing cross-compilation.

- **Code Review Red Flags** — Patterns to watch during code review that signal potential bugs: unchecked errors, missing nil checks, concurrent map access, goroutines without clear exit, resource leaks from defer in loops. Covered by the `review-standards` skill's `go-review` reference — load that skill for review work.

## Cross-References

- → See `samber/cc-skills@promql-cli` skill for querying Prometheus metrics during production incident investigation
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for the language-level bugs behind most symptoms

## Sub-topic: golang-security

**Persona:** You are a senior Go security engineer. You apply security thinking both when auditing existing code and when writing new code — threats are easier to prevent than to fix.

**Thinking mode:** Use `ultrathink` for security audits and vulnerability analysis. Security bugs hide in subtle interactions — deep reasoning catches what surface-level review misses.

**Modes:**

- **Review mode** — reviewing a PR for security issues. Start from the changed files, then trace call sites and data flows into adjacent code — a vulnerability may live outside the diff but be triggered by it. Sequential.
- **Audit mode** — full codebase security scan. Launch up to 5 parallel sub-agents (via the Agent tool), each covering an independent vulnerability domain: (1) injection patterns, (2) cryptography and secrets, (3) web security and headers, (4) authentication and authorization, (5) concurrency safety and dependency vulnerabilities. Aggregate findings, score with DREAD, and report by severity.
- **Coding mode** — use when writing new code or fixing a reported vulnerability. Follow the skill's sequential guidance. Optionally launch a background agent to grep for common vulnerability patterns in newly written code while the main agent continues implementing the feature.

# Go Security

## Overview

Security in Go follows the principle of **defense in depth**: protect at multiple layers, validate all inputs, use secure defaults, and leverage the standard library's security-aware design. Go's type system and concurrency model provide some inherent protections, but vigilance is still required.

## Security Thinking Model

Before writing or reviewing code, ask three questions:

1. **What are the trust boundaries?** — Where does untrusted data enter the system? (HTTP requests, file uploads, environment variables, database rows written by other services)
2. **What can an attacker control?** — Which inputs flow into sensitive operations? (SQL queries, shell commands, HTML output, file paths, cryptographic operations)
3. **What is the blast radius?** — If this defense fails, what's the worst outcome? (Data leak, RCE, privilege escalation, denial of service)

## Severity Levels

| Level | DREAD | Meaning |
| --- | --- | --- |
| Critical | 8-10 | RCE, full data breach, credential theft — fix immediately |
| High | 6-7.9 | Auth bypass, significant data exposure, broken crypto — fix in current sprint |
| Medium | 4-5.9 | Limited exposure, session issues, defense weakening — fix in next sprint |
| Low | 1-3.9 | Minor info disclosure, best-practice deviations — fix opportunistically |

Levels align with DREAD scoring.

## Research Before Reporting

Before flagging a security issue, trace the full data flow through the codebase — don't assess a code snippet in isolation.

1. **Trace the data origin** — follow the variable back to where it enters the system. Is it user input, a hardcoded constant, or an internal-only value?
2. **Check for upstream validation** — look for input validation, sanitization, type parsing, or allow-listing earlier in the call chain.
3. **Examine the trust boundary** — if the data never crosses a trust boundary (e.g., internal service-to-service with mTLS), the risk profile is different.
4. **Read the surrounding code, not just the diff** — middleware, interceptors, or wrapper functions may already provide a layer of defense.

**Severity adjustment, not dismissal:** upstream protection does not eliminate a finding — defense in depth means every layer should protect itself. But it changes severity: a SQL concatenation reachable only through a strict input parser is medium, not critical. Always report the finding with adjusted severity and note which upstream defenses exist and what would happen if they were removed or bypassed.

**When downgrading or skipping a finding:** add a brief inline comment (e.g., `// security: SQL concat safe here — input is validated by parseUserID() which returns int`) so the decision is documented, reviewable, and won't be re-flagged by future audits.

## Threat Modeling (STRIDE)

Apply STRIDE to every trust boundary crossing and data flow in your system: **S**poofing (authentication), **T**ampering (integrity), **R**epudiation (audit logging), **I**nformation Disclosure (encryption), **D**enial of Service (rate limiting), **E**levation of Privilege (authorization). Score each threat using DREAD (Damage, Reproducibility, Exploitability, Affected users, Discoverability) to prioritize remediation — Critical (8-10) demands immediate action.

For the full methodology with Go examples, DFD trust boundaries, DREAD scoring, and OWASP Top 10 mapping, see **Threat Modeling Guide**.

## Quick Reference

| Severity | Vulnerability | Defense | Standard Library Solution |
| --- | --- | --- | --- |
| Critical | SQL Injection | Parameterized queries separate data from code | `database/sql` with `?` placeholders |
| Critical | Command Injection | Pass args separately, never via shell concatenation | `exec.Command` with separate args |
| High | XSS | Auto-escaping renders user data as text, not HTML/JS | `html/template`, `text/template` |
| High | Path Traversal | Scope file access to a root, prevent `../` escapes | `os.Root` (Go 1.24+), `filepath.Clean` |
| Medium | Timing Attacks | Constant-time comparison avoids byte-by-byte leaks | `crypto/subtle.ConstantTimeCompare` |
| High | Crypto Issues | Use vetted algorithms; never roll your own | `crypto/aes`, `crypto/rand` |
| Medium | HTTP Security | TLS + security headers prevent downgrade attacks | `net/http`, configure TLSConfig |
| Low | Missing Headers | HSTS, CSP, X-Frame-Options prevent browser attacks | Security headers middleware |
| Medium | Rate Limiting | Rate limits prevent brute-force and resource exhaustion | `golang.org/x/time/rate`, server timeouts |
| High | Race Conditions | Protect shared state to prevent data corruption | `sync.Mutex`, channels, avoid shared state |

## Detailed Categories

For complete examples, code snippets, and CWE mappings, see:

- **Cryptography** — Algorithms, key derivation, TLS configuration.
- **Injection Vulnerabilities** — SQL, command, template injection, XSS, SSRF.
- **Filesystem Security** — Path traversal, zip bombs, file permissions, symlinks.
- **Network/Web Security** — SSRF, open redirects, HTTP headers, timing attacks, session fixation.
- **Cookie Security** — Secure, HttpOnly, SameSite flags.
- **Third-Party Data Leaks** — Analytics privacy risks, GDPR/CCPA compliance.
- **Memory Safety** — Integer overflow, memory aliasing, `unsafe` usage.
- **Secrets Management** — Hardcoded credentials, env vars, secret managers.
- **Logging Security** — PII in logs, log injection, sanitization.
- **Threat Modeling Guide** — STRIDE, DREAD scoring, trust boundaries, OWASP Top 10.
- **Security Architecture** — Defense-in-depth, Zero Trust, auth patterns, rate limiting, anti-patterns.

## Code Review Checklist

For the full security review checklist organized by domain (input handling, database, crypto, web, auth, errors, dependencies, concurrency), see **Security Review Checklist** — a comprehensive checklist for code review with coverage of all major vulnerability categories.

## Tooling & Verification

### Static Analysis & Linting

Security-relevant linters: `bodyclose`, `sqlclosecheck`, `nilerr`, `errcheck`, `govet`, `staticcheck`. See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for configuration and usage.

For deeper security-specific analysis:

```bash
# Go security checker (SAST)
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...

# Vulnerability scanner — see golang-dependency-management for full govulncheck usage
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

### Security Testing

```bash
# Race detector
go test -race ./...

# Fuzz testing
go test -fuzz=Fuzz
```

## Common Mistakes

| Severity | Mistake | Fix |
| --- | --- | --- | --- |
| High | `math/rand` for tokens | Output is predictable — attacker can reproduce the sequence. Use `crypto/rand` |
| Critical | SQL string concatenation | Attacker can modify query logic. Parameterized queries keep data and code separate |
| Critical | `exec.Command("bash -c")` | Shell interprets metacharacters (`;`, ` | `, `` ` ``). Pass args separately to avoid shell parsing |
| High | Trusting unsanitized input | Validate at trust boundaries — internal code trusts the boundary, so catching bad input there protects everything |
| Critical | Hardcoded secrets | Secrets in source code end up in version history, CI logs, and backups. Use env vars or secret managers |
| Medium | Comparing secrets with `==` | `==` short-circuits on first differing byte, leaking timing info. Use `crypto/subtle.ConstantTimeCompare` |
| Medium | Returning detailed errors | Stack traces and DB errors help attackers map your system. Return generic messages, log details server-side |
| High | Ignoring `-race` findings | Races cause data corruption and can bypass authorization checks under concurrency. Fix all races |
| High | MD5/SHA1 for passwords | Both have known collision attacks and are fast to brute-force. Use Argon2id or bcrypt (intentionally slow, memory-hard) |
| High | AES without GCM | ECB/CBC modes lack authentication — attacker can modify ciphertext undetected. GCM provides encrypt+authenticate |
| Medium | Binding to 0.0.0.0 | Exposes service to all network interfaces. Bind to specific interface to limit attack surface |

## Security Anti-Patterns

| Severity | Anti-Pattern | Why It Fails | Fix |
| --- | --- | --- | --- |
| High | Security through obscurity | Hidden URLs are discoverable via fuzzing, logs, or source | Authentication + authorization on all endpoints |
| High | Trusting client headers | `X-Forwarded-For`, `X-Is-Admin` are trivially forged | Server-side identity verification |
| High | Client-side authorization | JavaScript checks are bypassed by any HTTP client | Server-side permission checks on every handler |
| High | Shared secrets across envs | Staging breach compromises production | Per-environment secrets via secret manager |
| Critical | Ignoring crypto errors | `_, _ = encrypt(data)` silently proceeds unencrypted | Always check errors — fail closed, never open |
| Critical | Rolling your own crypto | Custom encryption hasn't been analyzed by cryptographers | Use `crypto/aes` GCM, `golang.org/x/crypto/argon2` |

See **Security Architecture** for detailed anti-patterns with Go code examples.

## Cross-References

See the I/O & Web reference (`golang-io-web.md`) for HTTP and database hardening, the Core reference (`golang-core.md`) for nil and numeric safety, and the dependency-management sub-topic above for `govulncheck`.

## Additional Resources

- [Go Security Best Practices](https://go.dev/doc/security/best-practices)
- [gosec Security Linter](https://github.com/securego/gosec)
- [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
- [OWASP Go Secure Coding Practices](https://owasp.org/www-project-go-secure-coding-practices-guide/)

## Sub-topic: golang-performance

**Persona:** You are a Go performance engineer. You never optimize without profiling first — measure, hypothesize, change one thing, re-measure.

**Thinking mode:** Use `ultrathink` for performance optimization. Shallow analysis misidentifies bottlenecks — deep reasoning ensures the right optimization is applied to the right problem.

**Modes:**

- **Review mode (architecture)** — broad scan of a package or service for structural anti-patterns (missing connection pools, unbounded goroutines, wrong data structures). Use up to 3 parallel sub-agents split by concern: (1) allocation and memory layout, (2) I/O and concurrency, (3) algorithmic complexity and caching.
- **Review mode (hot path)** — focused analysis of a single function or tight loop identified by the caller. Work sequentially; one sub-agent is sufficient.
- **Optimize mode** — a bottleneck has been identified by profiling. Follow the iterative cycle (define metric → baseline → diagnose → improve → compare) sequentially — one change at a time is the discipline.

# Go Performance Optimization

## Core Philosophy

1. **Profile before optimizing** — intuition about bottlenecks is wrong ~80% of the time. Use pprof to find actual hot spots (→ See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`))
2. **Allocation reduction yields the biggest ROI** — Go's GC is fast but not free. Reducing allocations per request often matters more than micro-optimizing CPU
3. **Document optimizations** — add code comments explaining why a pattern is faster, with benchmark numbers when available. Future readers need context to avoid reverting an "unnecessary" optimization

## Rule Out External Bottlenecks First

Before optimizing Go code, verify the bottleneck is in your process — if 90% of latency is a slow DB query or API call, reducing allocations won't help.

**Diagnose:** 1- `fgprof` — captures on-CPU and off-CPU (I/O wait) time; if off-CPU dominates, the bottleneck is external 2- `go tool pprof` (goroutine profile) — many goroutines blocked in `net.(*conn).Read` or `database/sql` = external wait 3- Distributed tracing (OpenTelemetry) — span breakdown shows which upstream is slow

**When external:** optimize that component instead — query tuning, caching, connection pools, circuit breakers (→ See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`), Caching Patterns).

## Iterative Optimization Methodology

### The cycle: Define Goals → Benchmark → Diagnose → Improve → Benchmark

1. **Define your metric** — latency, throughput, memory, or CPU? Without a target, optimizations are random
2. **Write an atomic benchmark** — isolate one function per benchmark to avoid result contamination (→ See the Testing (unit tests, benchmarks) reference (`golang-testing.md`))
3. **Measure baseline** — `go test -bench=BenchmarkMyFunc -benchmem -count=6 ./pkg/... | tee /tmp/report-1.txt`
4. **Diagnose** — use the **Diagnose** lines in each deep-dive section to pick the right tool
5. **Improve** — apply ONE optimization at a time with an explanatory comment
6. **Compare** — `benchstat /tmp/report-1.txt /tmp/report-2.txt` to confirm statistical significance
7. **Commit** — paste the benchstat output in the commit body so reviewers and future readers see the exact improvement; follow the `perf(scope): summary` commit type
8. **Repeat** — increment report number, tackle next bottleneck

Refer to library documentation for known patterns before inventing custom solutions. Keep all `/tmp/report-*.txt` files as an audit trail.

## Decision Tree: Where Is Time Spent?

| Bottleneck | Signal (from pprof) | Action |
| --- | --- | --- |
| Too many allocations | `alloc_objects` high in heap profile | Memory optimization |
| CPU-bound hot loop | function dominates CPU profile | CPU optimization |
| GC pauses / OOM | high GC%, container limits | Runtime tuning |
| Network / I/O latency | goroutines blocked on I/O | I/O & networking |
| Repeated expensive work | same computation/fetch multiple times | Caching patterns |
| Wrong algorithm | O(n²) where O(n) exists | Algorithmic complexity |
| Lock contention | mutex/block profile hot | → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) |
| Slow queries | DB time dominates traces | → See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) |

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Optimizing without profiling | Profile with pprof first — intuition is wrong ~80% of the time |
| Default `http.Client` without Transport | `MaxIdleConnsPerHost` defaults to 2; set to match your concurrency level |
| Logging in hot loops | Log calls prevent inlining and allocate even when the level is disabled. Use `slog.LogAttrs` |
| `panic`/`recover` as control flow | panic allocates a stack trace and unwinds the stack; use error returns |
| `unsafe` without benchmark proof | Only justified when profiling shows >10% improvement in a verified hot path |
| No GC tuning in containers | Set `GOMEMLIMIT` to 80-90% of container memory to prevent OOM kills |
| `reflect.DeepEqual` in production | 50-200x slower than typed comparison; use `slices.Equal`, `maps.Equal`, `bytes.Equal` |

## Deep Dives

- Memory Optimization — allocation patterns, backing array leaks, sync.Pool, struct alignment
- CPU Optimization — inlining, cache locality, false sharing, ILP, reflection avoidance
- I/O & Networking — HTTP transport config, streaming, JSON performance, cgo, batch operations
- Runtime Tuning — GOGC, GOMEMLIMIT, GC diagnostics, GOMAXPROCS, PGO
- Caching Patterns — algorithmic complexity, compiled patterns, singleflight, work avoidance
- Production Observability — Prometheus metrics, PromQL queries, continuous profiling, alerting rules

## Cross-References

- → See the Testing (unit tests, benchmarks) reference (`golang-testing.md`) for benchmarking methodology, `benchstat`, and `b.Loop()` (Go 1.24+)
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for slice/map preallocation and `strings.Builder`
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for worker pools, `sync.Pool` API, goroutine lifecycle, and lock contention
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for defer in loops, slice backing array aliasing
- → See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for connection pool tuning and batch processing

## Sub-topic: golang-popular-libraries

**Persona:** You are a Go ecosystem expert. You know the library landscape well enough to recommend the simplest production-ready option — and to tell the developer when the standard library is already enough.

# Go Libraries and Frameworks Recommendations

## Core Philosophy

When recommending libraries, prioritize:

1. **Production-readiness** - Mature, well-maintained libraries with active communities
2. **Simplicity** - Go's philosophy favors simple, idiomatic solutions
3. **Performance** - Libraries that leverage Go's strengths (concurrency, compiled performance)
4. **Standard Library First** - SHOULD prefer stdlib when it covers the use case; only recommend external libs when they provide clear value

## Where to Look

The standard library first, then `golang.org/x/...`, then <https://github.com/avelino/awesome-go>. Every third-party dependency is justified in the change entry before it is added.

## General Guidelines

When recommending libraries:

1. **Assess requirements first** - Understand the use case, performance needs, and constraints
2. **Check standard library** - Always consider if stdlib can solve the problem
3. **Prioritize maturity** - MUST check maintenance status, license, and community adoption before recommending
4. **Consider complexity** - Simpler solutions are usually better in Go
5. **Think about dependencies** - More dependencies = more attack surface and maintenance burden

Remember: The best library is often no library at all. Go's standard library is excellent and sufficient for many use cases.

## Anti-Patterns to Avoid

- Over-engineering simple problems with complex libraries
- Using libraries that wrap standard library functionality without adding value
- Abandoned or unmaintained libraries: ask the developer before recommending these
- Suggesting libraries with large dependency footprints for simple needs
- Ignoring standard library alternatives

## Cross-References

- → See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for gRPC implementation details

## Sub-topic: golang-documentation

**Persona:** You are a Go technical writer and API designer. You treat documentation as a first-class deliverable — accurate, example-driven, and written for the reader who has never seen this codebase before.

**Modes:**

- **Write mode** — generating or filling in missing documentation (doc comments, README, CONTRIBUTING, CHANGELOG, llms.txt). Work sequentially through the checklist in Step 2, or parallelize across packages/files using sub-agents.
- **Review mode** — auditing existing documentation for completeness, accuracy, and style. Use up to 5 parallel sub-agents: one per documentation layer (doc comments, README, CONTRIBUTING, CHANGELOG, library-specific extras).

# Go Documentation

Write documentation that serves both humans and AI agents. Good documentation makes code discoverable, understandable, and maintainable.

## Cross-References

See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for naming conventions in doc comments. See the Testing (unit tests, benchmarks) reference (`golang-testing.md`) for Example test functions. See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for where documentation files belong.

## Step 1: Detect Project Type

Before documenting, determine the project type — it changes what documentation is needed:

**Library** — no `main` package, meant to be imported by other projects:

- Focus on godoc comments, `ExampleXxx` functions, playground demos, pkg.go.dev rendering
- See Library Documentation

**Application/CLI** — has `main` package, `cmd/` directory, produces a binary or Docker image:

- Focus on installation instructions, CLI help text, configuration docs
- See Application Documentation

**Both apply**: function comments, README, CONTRIBUTING, CHANGELOG.

**Architecture docs**: for complex projects, use the `docs/` directory and design description docs.

## Step 2: Documentation Checklist

Every Go project needs these (ordered by priority):

| Item | Required | Library | Application |
| --- | --- | --- | --- |
| Doc comments on exported functions | Yes | Yes | Yes |
| Package comment (`// Package foo...`) — MUST exist | Yes | Yes | Yes |
| README.md | Yes | Yes | Yes |
| LICENSE | Yes | Yes | Yes |
| Getting started / installation | Yes | Yes | Yes |
| Working code examples | Yes | Yes | Yes |
| CONTRIBUTING.md | Recommended | Yes | Yes |
| CHANGELOG.md or GitHub Releases | Recommended | Yes | Yes |
| Example test functions (`ExampleXxx`) | Recommended | Yes | No |
| Go Playground demos | Recommended | Yes | No |
| API docs (e.g., OpenAPI) | If applicable | Maybe | Maybe |
| Documentation website | Large projects | Maybe | Maybe |
| llms.txt | Recommended | Yes | Yes |

A private project might not need a documentation website, llms.txt, Go Playground demos...

## Parallelizing Documentation Work

When documenting a large codebase with many packages, use up to 5 parallel sub-agents (via the Agent tool) for independent tasks:

- Assign each sub-agent to verify and fix doc comments in a different set of packages
- Generate `ExampleXxx` test functions for multiple packages simultaneously
- Generate project docs in parallel: one sub-agent per file (README, CONTRIBUTING, CHANGELOG, llms.txt)

## Step 3: Function & Method Doc Comments

Every exported function and method MUST have a doc comment. Document complex internal functions too. Skip test functions.

The comment starts with the function name and a verb phrase. Focus on **why** and **when**, not restating what the code already shows. The code tells you _what_ happens — the comment should explain _why_ it exists, _when_ to use it, _what constraints_ apply, and _what can go wrong_. Include parameters, return values, error cases, and a usage example:

```go
// CalculateDiscount computes the final price after applying tiered discounts.
// Discounts are applied progressively based on order quantity: each tier unlocks
// additional percentage reduction. Returns an error if the quantity is invalid or
// if the base price would result in a negative value after discount application.
//
// Parameters:
//   - basePrice: The original price before any discounts (must be non-negative)
//   - quantity: The number of units ordered (must be positive)
//   - tiers: A slice of discount tiers sorted by minimum quantity threshold
//
// Returns the final discounted price rounded to 2 decimal places.
// Returns ErrInvalidPrice if basePrice is negative.
// Returns ErrInvalidQuantity if quantity is zero or negative.
//
// Play: https://go.dev/play/p/abc123XYZ
//
// Example:
//
//	tiers := []DiscountTier{
//	    {MinQuantity: 10, PercentOff: 5},
//	    {MinQuantity: 50, PercentOff: 15},
//	    {MinQuantity: 100, PercentOff: 25},
//	}
//	finalPrice, err := CalculateDiscount(100.00, 75, tiers)
//	if err != nil {
//	    log.Fatalf("Discount calculation failed: %v", err)
//	}
//	log.Printf("Ordered 75 units at $100 each: final price = $%.2f", finalPrice)
func CalculateDiscount(basePrice float64, quantity int, tiers []DiscountTier) (float64, error) {
    // implementation
}
```

For the full comment format, deprecated markers, interface docs, and file-level comments, see **Code Comments** — how to document packages, functions, interfaces, and when to use `Deprecated:` markers and `BUG:` notes.

## Step 4: README Structure

README SHOULD follow this exact section order. Copy the template from templates/README.md:

1. **Title** — project name as `# heading`
2. **Badges** — shields.io pictograms (Go version, license, CI, coverage, Go Report Card...)
3. **Summary** — 1-2 sentences explaining what the project does
4. **Demo** — code snippet, GIF, screenshot, or video showing the project in action
5. **Getting Started** — installation + minimal working example
6. **Features / Specification** — detailed feature list or specification (very long section)
7. **Contributing** — link to CONTRIBUTING.md or inline if very short
8. **Contributors** — thank contributors (badge or list)
9. **License** — license name + link

Common badges for Go projects:

```markdown
[![Go Version](https://img.shields.io/github/go-mod/go-version/{owner}/{repo})](https://go.dev/) [![License](https://img.shields.io/github/license/{owner}/{repo})](./LICENSE) [![Build Status](https://img.shields.io/github/actions/workflow/status/{owner}/{repo}/test.yml?branch=main)](https://github.com/{owner}/{repo}/actions) [![Coverage](https://img.shields.io/codecov/c/github/{owner}/{repo})](https://codecov.io/gh/{owner}/{repo}) [![Go Report Card](https://goreportcard.com/badge/github.com/{owner}/{repo})](https://goreportcard.com/report/github.com/{owner}/{repo}) [![Go Reference](https://pkg.go.dev/badge/github.com/{owner}/{repo}.svg)](https://pkg.go.dev/github.com/{owner}/{repo})
```

For the full README guidance and application-specific sections, see Project Docs.

## Step 5: CONTRIBUTING & Changelog

**CONTRIBUTING.md** — Help contributors get started in under 10 minutes. Include: prerequisites, clone, build, test, PR process. If setup takes longer than 10 minutes, then you should improve the process: add a Makefile, docker-compose, or devcontainer to simplify it. See Project Docs.

**Changelog** — Track changes using [Keep a Changelog](https://keepachangelog.com/) format or GitHub Releases. Copy the template from templates/CHANGELOG.md. See Project Docs.

## Step 6: Library-Specific Documentation

For Go libraries, add these on top of the basics:

- **Go Playground demos** — create runnable demos and link them in doc comments with `// Play: https://go.dev/play/p/xxx`. Use the go-playground MCP tool when available to create and share playground URLs.
- **Example test functions** — write `func ExampleXxx()` in `_test.go` files. These are executable documentation verified by `go test`.
- **Generous code examples** — include multiple examples in doc comments showing common use cases.
- **godoc** — your doc comments render on [pkg.go.dev](https://pkg.go.dev). Use `go doc` locally to preview.
- **Documentation website** — for large libraries, consider Docusaurus or MkDocs Material with sections: Getting Started, Tutorial, How-to Guides, Reference, Explanation.
- **Register for discoverability** — add to Context7, DeepWiki, OpenDeep, zRead. Even for private libraries.

See Library Documentation for details.

## Step 7: Application-Specific Documentation

For Go applications/CLIs:

- **Installation methods** — pre-built binaries (GoReleaser), `go install`, Docker images, Homebrew...
- **CLI help text** — make `--help` comprehensive; it's the primary documentation
- **Configuration docs** — document all env vars, config files, CLI flags

See Application Documentation for details.

## Step 8: API Documentation

If your project exposes an API:

| API Style    | Format      | Tool                                         |
| ------------ | ----------- | -------------------------------------------- |
| REST/HTTP    | OpenAPI 3.x | swaggo/swag (auto-generate from annotations) |
| Event-driven | AsyncAPI    | Manual or code-gen                           |
| gRPC         | Protobuf    | buf, grpc-gateway                            |

Prefer auto-generation from code annotations when possible. See Application Documentation for details.

## Step 9: AI-Friendly Documentation

Make your project consumable by AI agents:

- **llms.txt** — add a `llms.txt` file at the repository root. Copy the template from templates/llms.txt. This file gives LLMs a structured overview of your project.
- **Structured formats** — use OpenAPI, AsyncAPI, or protobuf for machine-readable API docs.
- **Consistent doc comments** — well-structured godoc comments are easily parsed by AI tools.
- **Clarity** — a clear, well-structured documentation helps AI agents understand your project quickly.

## Step 10: Delivery Documentation

Document how users get your project:

**Libraries:**

```bash
go get github.com/{owner}/{repo}
```

**Applications:**

```bash
# Pre-built binary
curl -sSL https://github.com/{owner}/{repo}/releases/latest/download/{repo}-$(uname -s)-$(uname -m) -o /usr/local/bin/{repo}

# From source
go install github.com/{owner}/{repo}@latest

# Docker
docker pull {registry}/{owner}/{repo}:latest
```

See Project Docs for Dockerfile best practices and Homebrew tap setup.
