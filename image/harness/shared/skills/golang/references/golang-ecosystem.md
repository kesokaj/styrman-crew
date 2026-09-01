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

See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for the single handling rule. See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for using observability signals to diagnose production issues. See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for protecting pprof endpoints and avoiding PII in logs. See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for propagating trace context across service boundaries. See `samber/cc-skills@promql-cli` skill for querying and exploring PromQL expressions against Prometheus from the CLI.

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

- **[Structured Logging](references/logging.md)** — Why structured logging matters for log aggregation at scale. Covers `log/slog` setup, log levels (Debug/Info/Warn/Error) and when to use each, request correlation with trace IDs, context propagation with `slog.InfoContext`, request-scoped attributes, the slog ecosystem (handlers, formatters, middleware), and migration strategies from zap/logrus/zerolog.

- **[Metrics Collection](references/metrics.md)** — Prometheus client setup and the four metric types (Counter for rate-of-change, Gauge for snapshots, Histogram for latency aggregation). Deep dive: why Histograms beat Summaries (server-side aggregation, supports `histogram_quantile` PromQL), naming conventions, the PromQL-as-comments convention (write queries above metric declarations for discoverability), production-grade PromQL examples, multi-window SLO burn rate alerting, and the high-cardinality label problem (why unbounded values like user IDs destroy performance).

- **[Distributed Tracing](references/tracing.md)** — When and how to use OpenTelemetry SDK to trace request flows across services. Covers spans (creating, attributes, status recording), `otelhttp` middleware for HTTP instrumentation, error recording with `span.RecordError()`, trace sampling (why you can't collect everything at scale), propagating trace context across service boundaries, and cost optimization.

- **[Profiling](references/profiling.md)** — On-demand profiling with pprof (CPU, heap, goroutine, mutex, block profiles) — how to enable it in production, secure it with auth, and toggle via environment variables without redeploying. Continuous profiling with Pyroscope for always-on performance visibility. Cost implications of each profiling type and mitigation strategies.

- **[Real User Monitoring](references/rum.md)** — Understanding how users actually experience your service. Covers product analytics (event tracking, funnels), Customer Data Platform integration, and critical compliance: GDPR/CCPA consent checks, data subject rights (user deletion endpoints), and privacy checklist for tracking. Server-side event tracking (PostHog, Segment) and identity key best practices.

- **[Alerting](references/alerting.md)** — Proactive problem detection. Covers the four golden signals (latency, traffic, errors, saturation), [awesome-prometheus-alerts](https://samber.github.io/awesome-prometheus-alerts/) as a rule library with ~500 ready-to-use rules by technology, Go runtime alerts (goroutine leaks, GC pressure, OOM risk), severity levels, and common mistakes that break alerting (using `irate` instead of `rate`, missing `for:` duration to avoid flapping).

- **[Grafana Dashboards](references/dashboards.md)** — Prebuilt dashboards for Go runtime monitoring (heap allocation, GC pause frequency, goroutine count, CPU). Explains the standard dashboards to install, how to customize them for your service, and when each dashboard answers a different operational question.

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

## Sub-topic: golang-continuous-integration

**Persona:** You are a Go DevOps engineer. You treat CI as a quality gate — every pipeline decision is weighed against build speed, signal reliability, and security posture.

**Modes:**

- **Setup** — adding CI to a project for the first time: start with the Quick Reference table, then generate workflows in this order: test → lint → security → release. Prefer the latest stable major version for each GitHub Action.
- **Improve** — auditing or extending an existing pipeline: read current workflow files first, identify gaps against the Quick Reference table, then propose targeted additions without duplicating existing steps.

# Go Continuous Integration

Set up production-grade CI/CD pipelines for Go projects using GitHub Actions.

## Action Versions

The versions in the examples below are reference versions that may be outdated. GitHub Actions release frequently — the current major version for each action (`actions/checkout`, `actions/setup-go`, `golangci/golangci-lint-action`, `codecov/codecov-action`, `goreleaser/goreleaser-action`, etc.) may differ from what is shown here.

## Quick Reference

| Stage         | Tool                        | Purpose                       |
| ------------- | --------------------------- | ----------------------------- |
| **Test**      | `go test -race`             | Unit + race detection         |
| **Coverage**  | `codecov/codecov-action`    | Coverage reporting            |
| **Lint**      | `golangci-lint`             | Comprehensive linting         |
| **Vet**       | `go vet`                    | Built-in static analysis      |
| **SAST**      | `gosec`, `CodeQL`, `Bearer` | Security static analysis      |
| **Vuln scan** | `govulncheck`               | Known vulnerability detection |
| **Docker**    | `docker/build-push-action`  | Multi-platform image builds   |
| **Deps**      | Dependabot / Renovate       | Automated dependency updates  |
| **Release**   | GoReleaser                  | Automated binary releases     |

---

## Testing

`.github/workflows/test.yml` — see [test.yml](./assets/test.yml)

Adapt the Go version matrix to match `go.mod`:

```
go 1.23   → matrix: ["1.23", "1.24", "1.25", "1.26", "stable"]
go 1.24   → matrix: ["1.24", "1.25", "1.26", "stable"]
go 1.25   → matrix: ["1.25", "1.26", "stable"]
go 1.26   → matrix: ["1.26", "stable"]
```

Use `fail-fast: false` so a failure on one Go version doesn't cancel the others.

Test flags:

- `-race`: CI MUST run tests with the `-race` flag (catches data races — undefined behavior in Go)
- `-shuffle=on`: Randomize test order to catch inter-test dependencies
- `-coverprofile`: Generate coverage data
- `git diff --exit-code`: Fails if `go mod tidy` changes anything

### Coverage Configuration

CI SHOULD enforce code coverage thresholds. Configure thresholds in `codecov.yml` at the repo root — see [codecov.yml](./assets/codecov.yml)

---

## Integration Tests

`.github/workflows/integration.yml` — see [integration.yml](./assets/integration.yml)

Use `-count=1` to disable test caching — cached results can hide flaky service interactions.

---

## Linting

`golangci-lint` MUST be run in CI on every PR. `.github/workflows/lint.yml` — see [lint.yml](./assets/lint.yml)

### golangci-lint Configuration

Create `.golangci.yml` at the root of the project. See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for the recommended configuration.

---

## Security & SAST

`.github/workflows/security.yml` — see [security.yml](./assets/security.yml)

CI MUST run `govulncheck`. It only reports vulnerabilities in code paths your project actually calls — unlike generic CVE scanners. CodeQL results appear in the repository's Security tab. Bearer is good at detecting sensitive data flow issues.

### CodeQL Configuration

Create `.github/codeql/codeql-config.yml` to use the extended security query suite — see [codeql-config.yml](./assets/codeql-config.yml)

Available query suites:

- **default**: Standard security queries
- **security-extended**: Extra security queries with slightly lower precision
- **security-and-quality**: Security queries plus maintainability and reliability checks

### Container Image Scanning

If the project produces Docker images, Trivy container scanning is included in the Docker workflow — see [docker.yml](./assets/docker.yml)

---

## Dependency Management

### Dependabot

`.github/dependabot.yml` — see [dependabot.yml](./assets/dependabot.yml)

Minor/patch updates are grouped into a single PR. Major updates get individual PRs since they may have breaking changes.

#### Auto-Merge for Dependabot

`.github/workflows/dependabot-auto-merge.yml` — see [dependabot-auto-merge.yml](./assets/dependabot-auto-merge.yml)

> **Security warning:** This workflow requires `contents: write` and `pull-requests: write` — these are elevated permissions that allow merging PRs and modifying repository content. The `if: github.actor == 'dependabot[bot]'` guard restricts execution to Dependabot only. Do not remove this guard. Note that `github.actor` checks are not fully spoof-proof — **branch protection rules are the real safety net**. Ensure branch protection is configured (see [Repository Security Settings](#repository-security-settings)) with required status checks and required approvals so that auto-merge only succeeds after all checks pass, regardless of who triggered the workflow.

### Renovate (alternative)

Renovate is a more mature and configurable alternative to Dependabot. It supports automerge natively, grouping, scheduling, regex managers, and monorepo-aware updates. If Dependabot feels too limited, Renovate is the go-to choice.

Install the [Renovate GitHub App](https://github.com/apps/renovate), then create `renovate.json` at the repo root — see [renovate.json](./assets/renovate.json)

Key advantages over Dependabot:

- **`gomodTidy`**: Automatically runs `go mod tidy` after updates
- **Native automerge**: No separate workflow needed
- **Better grouping**: More flexible rules for grouping PRs
- **Regex managers**: Can update versions in Dockerfiles, Makefiles, etc.
- **Monorepo support**: Handles Go workspaces and multi-module repos

---

## Release Automation

GoReleaser automates binary builds, checksums, and GitHub Releases. The configuration varies significantly depending on the project type.

### Release Workflow

`.github/workflows/release.yml` — see [release.yml](./assets/release.yml)

> **Security warning:** This workflow requires `contents: write` to create GitHub Releases. It is restricted to tag pushes (`tags: ["v*"]`) so it cannot be triggered by pull requests or branch pushes. Only users with push access to the repository can create tags.

### GoReleaser for CLI/Programs

Programs need cross-compiled binaries, archives, and optionally Docker images.

`.goreleaser.yml` — see [goreleaser-cli.yml](./assets/goreleaser-cli.yml)

### GoReleaser for Libraries

Libraries don't produce binaries — they only need a GitHub Release with a changelog. Use a minimal config that skips the build.

`.goreleaser.yml` — see [goreleaser-lib.yml](./assets/goreleaser-lib.yml)

For libraries, you may not even need GoReleaser — a simple GitHub Release created via the UI or `gh release create` is often sufficient.

### GoReleaser for Monorepos / Multi-Binary

When a repository contains multiple commands (e.g., `cmd/api/`, `cmd/worker/`).

`.goreleaser.yml` — see [goreleaser-monorepo.yml](./assets/goreleaser-monorepo.yml)

### Docker Build & Push

For projects that produce Docker images. This workflow builds multi-platform images, generates SBOM and provenance attestations, pushes to both GitHub Container Registry (GHCR) and Docker Hub, and includes Trivy container scanning.

`.github/workflows/docker.yml` — see [docker.yml](./assets/docker.yml)

> **Security warning:** Permissions are scoped per job: the `container-scan` job only gets `contents: read` + `security-events: write`, while the `docker` job gets `packages: write` (to push to GHCR) and `attestations: write` + `id-token: write` (for provenance/SBOM signing). This ensures the scan job cannot push images even if compromised. The `push` flag is set to `false` on pull requests so untrusted code cannot publish images. The `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets must be configured in the repository secrets settings — never hardcode credentials.

Key details:

- **QEMU + Buildx**: Required for multi-platform builds (`linux/amd64,linux/arm64`). Remove platforms you don't need.
- **`push: false` on PRs**: Images are built but never pushed on pull requests — this validates the Dockerfile without publishing untrusted code.
- **Metadata action**: Automatically generates semver tags (`v1.2.3` → `1.2.3`, `1.2`, `1`), branch tags (`main`), and SHA tags.
- **Provenance + SBOM**: `provenance: mode=max` and `sbom: true` generate supply chain attestations. These require `attestations: write` and `id-token: write` permissions.
- **Dual registry**: Pushes to both GHCR (using `GITHUB_TOKEN`, no extra secret needed) and Docker Hub (requires `DOCKERHUB_USERNAME` + `DOCKERHUB_TOKEN` secrets). Remove the Docker Hub login and image line if not needed.
- **Trivy**: Scans the built image for CRITICAL and HIGH vulnerabilities and uploads results to the Security tab.
- Adapt the image names and registries to your project. For GHCR-only, remove the Docker Hub login step and the `docker.io/` line from `images:`.

---

## Repository Security Settings

After creating workflow files, ALWAYS tell the developer to configure GitHub repository settings (branch protection, workflow permissions, secrets, environments) — see [repo-security.md](./references/repo-security.md)

---

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Missing `-race` in CI tests | Always use `go test -race` |
| No `-shuffle=on` | Randomize test order to catch inter-test dependencies |
| Caching integration test results | Use `-count=1` to disable caching |
| `go mod tidy` not checked | Add `go mod tidy && git diff --exit-code` step |
| Missing `fail-fast: false` | One Go version failing shouldn't cancel other jobs |
| Not pinning action versions | GitHub Actions MUST use pinned major versions (e.g. `@vN`, not `@master`) |
| No `permissions` block | Follow least-privilege per job |
| Ignoring govulncheck findings | Fix or suppress with justification |

## Related Skills

See the Ecosystem reference (`golang-ecosystem.md`), the Ecosystem reference (`golang-ecosystem.md`), the Testing reference (`golang-testing.md`), the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`)s.

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

The the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) contains a curated list of vetted, production-ready libraries. Prefer recommending packages from that list. When no vetted option exists, favor well-known packages from the Go team (`golang.org/x/...`) or established organizations over obscure alternatives.

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

- **[Versioning & MVS](./references/versioning.md)** — Semantic versioning rules (major.minor.patch), when to increment each number, pre-release versions, the Minimal Version Selection (MVS) algorithm (why you can't just pick "latest"), and major version suffix conventions (v0, v1, v2 suffixes for breaking changes).

- **[Auditing Dependencies](./references/auditing.md)** — Vulnerability scanning with `govulncheck`, tracking outdated dependencies, analyzing which dependencies make the binary large (`goweight`), and distinguishing test-only vs binary dependencies to keep `go.mod` clean.

- **[Dependency Conflicts & Resolution](./references/conflicts.md)** — Diagnosing version conflicts (what `go get` does when you request incompatible versions), resolution strategies (`replace` directives for local development, `exclude` for broken versions, `retract` for published versions that should be skipped), and workflows for conflicts across your dependency tree.

- **[Go Workspaces](./references/workspaces.md)** — `go.work` files for multi-module development (e.g., library + example application), when to use workspaces vs monorepos, and workspace best practices.

- **[Automated Dependency Updates](./references/automated-updates.md)** — Setting up Dependabot or Renovate for automatic dependency update PRs, auto-merge strategies (when to merge automatically vs require review), and handling security updates.

- **[Visualizing the Dependency Graph](./references/visualization.md)** — `go mod graph` to inspect the full dependency tree, `modgraphviz` to visualize it, and interactive tools to find which dependency chains cause bloat.

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for Dependabot/Renovate CI setup
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for vulnerability scanning with govulncheck
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for vetted library recommendations

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

- **Setup mode** — configuring `.golangci.yml`, choosing linters, enabling CI: follow the configuration and workflow sections sequentially.
- **Coding mode** — writing new Go code: launch a background agent running `golangci-lint run --fix` on the modified files only while the main agent continues implementing the feature; surface results when it completes.
- **Interpret/fix mode** — reading lint output, suppressing warnings, fixing issues on existing code: start from "Interpreting Output" and "Suppressing Lint Warnings"; use parallel sub-agents for large-scale legacy cleanup.

# Go Linting

## Overview

`golangci-lint` is the standard Go linting tool. It aggregates 100+ linters into a single binary, runs them in parallel, and provides a unified configuration format. Run it frequently during development and always in CI.

Every Go project MUST have a `.golangci.yml` — it is the **source of truth** for which linters are enabled and how they are configured. See the [recommended configuration](./assets/.golangci.yml) for a production-ready setup with 33 linters enabled.

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

The [recommended .golangci.yml](./assets/.golangci.yml) provides a production-ready setup with 33 linters. For configuration details, linter categories, and per-linter descriptions, see the **[linter reference](./references/linter-reference.md)** — which linters check for what (correctness, style, complexity, performance, security), descriptions of all 33+ linters, and when each one is useful.

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

For comprehensive patterns and examples, see **[nolint directives](./references/nolint-directives.md)** — when to suppress, how to write justifications, patterns for per-line vs per-function suppression, and anti-patterns.

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

For CI pipeline setup (GitHub Actions with `golangci-lint-action`), see the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`).

## Interpreting Output

Each issue follows this format:

```
path/to/file.go:42:10: message describing the issue (linter-name)
```

The linter name in parentheses tells you which linter flagged it. Use this to:

- Look up the linter in the [reference](./references/linter-reference.md) to understand what it checks
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

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for CI pipeline with golangci-lint-action
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for style rules that linters enforce
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for SAST tools beyond linting (gosec, govulncheck)

## Sub-topic: golang-stay-updated

<!-- markdownlint-disable table-column-style -->

# Stay Updated with Go

A curated guide to keeping your finger on the pulse of the Go ecosystem.

## Official Go Resources

| Resource            | URL                                          |
| ------------------- | -------------------------------------------- |
| **go.dev**          | Official Go website with tutorials and tools |
| **pkg.go.dev**      | Discover Go packages and documentation       |
| **tour.golang.org** | Interactive Go tutorial                      |
| **play.golang.org** | Go playground for testing code               |
| **go.dev/blog**     | Official Go blog                             |

## Newsletters

| Newsletter | Description | Subscribe |
| --- | --- | --- |
| **Golang Weekly** | Weekly curated Go content, news, and articles | <https://golangweekly.com/> |
| **Awesome Go Newsletter** | Updates on new Go libraries and tools | <https://go.libhunt.com/> |

## Reddit & Communities

| Community | Description | URL |
| --- | --- | --- |
| r/golang | Main Go subreddit with 300K+ members | <https://www.reddit.com/r/golang> |
| golang wiki | Official wiki with resources and FAQs | <https://go.dev/wiki/> |
| gophers.slack.com | Official Go Slack community | <https://invite.slack.golangbridge.org> |
| Go Forum | Official Go discussion forum | <https://forum.golangbridge.org> |
| Discuss Go | Official Go team discussion | <https://groups.google.com/g/golang-nuts> |

## Famous Go Developers

Follow these influential Go developers and contributors:

### Core Go Team

| Name | GitHub | Twitter/X | LinkedIn | Bluesky |
| --- | --- | --- | --- | --- |
| **Rob Pike** | robpike |  |  |  |
| **Ken Thompson** | ken |  |  |  |
| **Russ Cox** | rsc | @\_rsc | <https://www.linkedin.com/in/swtch> | <https://bsky.app/profile/swtch.com> |
| **Brad Fitzpatrick** | bradfitz | @bradfitz | <https://www.linkedin.com/in/bradfitz/> | <https://bsky.app/profile/bradfitz.com> |
| **Andrew Gerrand** | adg |  |  |  |
| **Robert Griesemer** | griesemer |  |  |  |
| **Dmitry Vyukov** | dvyukov | @dvyukov |  |  |

### Go Tooling & Infrastructure

| Name | GitHub | Twitter/X | LinkedIn | Bluesky |
| --- | --- | --- | --- | --- |
| **Sam Boyer** | sdboyer | @sdboyer |  |  |
| **Daniel Theophanes** | kardianos | @kardianos |  |  |
| **Matt Butcher** | technosophos |  |  |  |
| **Jaana Dogan** | rakyll | @rakyll | <https://www.linkedin.com/in/rakyll/> |  |

### Popular Go Authors & Educators

| Name | GitHub | Twitter/X | LinkedIn | Bluesky |
| --- | --- | --- | --- | --- |
| **Mat Ryer** | matryer | @matryer | <https://linkedin.com/in/matryer> |  |
| **Dave Cheney** | davecheney | @davecheney | <https://linkedin.com/in/davecheney> |  |
| **Katherine Cox-Buday** | kat-co |  | <https://linkedin.com/in/katherinecoxbuday> |  |
| **Johnny Boursiquot** | jboursiquot | @jboursiquot | <https://linkedin.com/in/jboursiquot> |  |
| **Michał Łowicki** | mlowicki | @mlowicki | <https://linkedin.com/in/michał-łowicki-a60402b> |  |

### Library & Framework Authors

| Name | GitHub | Twitter/X | LinkedIn | Bluesky |
| --- | --- | --- | --- | --- |
| **Steve Francia** | spf13 | @spf13 | <https://linkedin.com/in/spf13> |  |
| **Samuel Berthe** | samber | @samuelberthe | <https://linkedin.com/in/samuelberthe> | <https://bsky.app/profile/samber.bsky.social> |
| **Mitchell Hashimoto** | mitchellh | @mitchellh | <https://linkedin.com/in/mitchellh> | <https://bsky.app/profile/mitchellh.com> |
| **Matt Holt** | mholt | @mholt6 |  |  |
| **Tomás Senart** | tsenart | @tsenart | <https://www.linkedin.com/in/tsenart/> |  |
| **Björn Rabenstein** | beorn7 |  |  |  |

### Conference Speakers & Community Leaders

| Name | GitHub | Twitter/X | LinkedIn | Bluesky |
| --- | --- | --- | --- | --- |
| **Carlisia Campos** | carlisia | @carlisia | <https://linkedin.com/in/carlisia> |  |
| **Erik St. Martin** | erikstmartin | @erikstmartin |  |  |
| **Brian Ketelsen** | bketelsen |  |  | @brian.dev |

## Must-Follow Blogs

| Blog            | Author       | URL                                  |
| --------------- | ------------ | ------------------------------------ |
| The Go Blog     | Go Team      | <https://go.dev/blog>                |
| Rob Pike's Blog | Rob Pike     | <https://commandcenter.blogspot.com> |
| Dave Cheney     | Dave Cheney  | <https://dave.cheney.net>            |
| Ardan Labs Blog | Bill Kennedy | <https://www.ardanlabs.com/blog>     |

## YouTube Channels

| Channel | Content | URL |
| --- | --- | --- |
| Go | Official Go team | <https://www.youtube.com/@golang> |
| Gopher Academy | Talks & tutorials | <https://www.youtube.com/@GopherAcademy> |
| GopherCon Europe | European conference talks | <https://www.youtube.com/@GopherConEurope> |
| GopherCon UK | UK conference talks | <https://www.youtube.com/@GopherConUK> |
| Golang Singapore | Singapore meetup & conf talks | <https://www.youtube.com/@golangSG> |
| Ardan Labs | Go training & tips | <https://www.youtube.com/@ArdanLabs> |
| Applied Go | Go tutorials | <https://youtube.com/appliedgocode> |
| Learn Go Programming | Beginner tutorials | <https://youtube.com/learn_goprogramming> |

## Quick Tips for Staying Updated

1. **Subscribe to 1-2 newsletters** - Don't overload yourself
2. **Follow 10-20 key people** on X/Bluesky who post regularly
3. **Check Go.dev/blog weekly** for official announcements
4. **Join Go Slack** for real-time discussions
5. **Bookmark pkg.go.dev** to discover new libraries
6. **Attend a GopherCon** (virtual or in-person) yearly

---

_Note: This guide is regularly updated. Suggest additions via GitHub issues._

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
  → See [compilation.md](./references/compilation.md)

"Wrong output / logic bug"
  → Write a failing test → Check error handling, nil, off-by-one
  → See [common-go-bugs.md](./references/common-go-bugs.md), [testing-debug.md](./references/testing-debug.md)

"Random crashes / panics"
  → GOTRACEBACK=all ./app → go test -race ./...
  → See [common-go-bugs.md](./references/common-go-bugs.md), [diagnostic-tools.md](./references/diagnostic-tools.md)

"Sometimes works, sometimes fails"
  → go test -race ./...
  → See [concurrency-debug.md](./references/concurrency-debug.md), [testing-debug.md](./references/testing-debug.md)

"Program hangs / frozen"
  → curl localhost:6060/debug/pprof/goroutine?debug=2
  → See [concurrency-debug.md](./references/concurrency-debug.md), [pprof.md](./references/pprof.md)

"High CPU usage"
  → pprof CPU profiling
  → See [performance-debug.md](./references/performance-debug.md), [pprof.md](./references/pprof.md)

"Memory growing over time"
  → pprof heap profiling
  → See [performance-debug.md](./references/performance-debug.md), [concurrency-debug.md](./references/concurrency-debug.md)

"Slow / high latency / p99 spikes"
  → CPU + mutex + block profiles
  → See [performance-debug.md](./references/performance-debug.md), [diagnostic-tools.md](./references/diagnostic-tools.md)

"Simple bug, easy to reproduce"
  → Write a test, add fmt.Println / log.Debug
  → See [testing-debug.md](./references/testing-debug.md)
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

- **[General Debugging Methodology](./references/methodology.md)** — The systematic 10-step process: define symptoms, isolate reproduction, form one hypothesis, test it, verify the root cause, and defend against regressions. Escalation guide: when to escalate from `fmt.Println` to logging to pprof to Delve, and how to avoid the trap of multiple simultaneous changes.

- **[Common Go Bugs](./references/common-go-bugs.md)** — The bugs that crash Go code: nil pointer dereferences, interface nil gotcha (typed nil ≠ nil), variable shadowing, slice/map/defer/error/context pitfalls, race conditions, JSON unmarshaling surprises, unclosed resources. Each with reproduction patterns and fixes.

- **[Test-Driven Debugging](./references/testing-debug.md)** — Why writing a failing test is the first step of debugging. Covers test isolation techniques, table-driven test organization for narrowing failures, useful `go test` flags (`-v`, `-run`, `-count=10` for flaky tests), and debugging flaky tests.

- **[Concurrency Debugging](./references/concurrency-debug.md)** — Race conditions, deadlocks, goroutine leaks. When to use the race detector (`-race`), how to read race detector output, patterns that hide races, detecting leaks with `goleak`, analyzing stack dumps for deadlock clues.

- **[Performance Troubleshooting](./references/performance-debug.md)** — When your code is slow: CPU profiling workflow, memory analysis (heap vs alloc_objects profiles, finding leaks), lock contention (mutex profile), and I/O blocking (goroutine profile). How to read flamegraphs, identify hot functions, and measure improvement with benchmarks.

- **[pprof Reference](./references/pprof.md)** — Complete pprof manual. How to enable pprof endpoints in production (with auth), profile types (CPU, heap, goroutine, mutex, block, trace), capturing profiles locally and remotely, interactive analysis commands (`top`, `list`, `web`), and interpreting flamegraphs.

- **[Diagnostic Tools](./references/diagnostic-tools.md)** — Auxiliary tools for specific symptoms. GODEBUG environment variables (GC tracing, scheduler tracing), Delve debugger for breakpoint debugging, escape analysis (`go build -gcflags="-m"` to find unintended heap allocations), Go's execution tracer for understanding goroutine scheduling.

- **[Production Debugging](./references/production-debug.md)** — Debugging live production systems without stopping them. Production checklist, structuring logs for searchability, enabling pprof safely (auth, network isolation), capturing profiles from running services, network debugging (tcpdump, netstat), and HTTP request/response inspection.

- **[Compilation Issues](./references/compilation.md)** — Build failures: module version conflicts, CGO linking problems, version mismatch between `go.mod` and installed Go version, platform-specific build tags preventing cross-compilation.

- **Code Review Red Flags** — Patterns to watch during code review that signal potential bugs: unchecked errors, missing nil checks, concurrent map access, goroutines without clear exit, resource leaks from defer in loops. Covered by the `review-standards` skill's `go-review` reference — load that skill for review work.

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for optimization patterns after identifying bottlenecks
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for metrics, alerting, and Grafana dashboards for Go runtime monitoring
- → See `samber/cc-skills@promql-cli` skill for querying Prometheus metrics during production incident investigation
- → See the Core reference (`golang-core.md`), the Core reference (`golang-core.md`), the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`)s

## Sub-topic: golang-samber-do

**Persona:** You are a Go architect setting up dependency injection. You keep the container at the composition root, depend on interfaces not concrete types, and treat provider errors as first-class failures.

# Using samber/do for Dependency Injection in Go

Type-safe dependency injection toolkit for Go based on Go 1.18+ generics.

**Official Resources:**

- [pkg.go.dev/github.com/samber/do/v2](https://pkg.go.dev/github.com/samber/do/v2)
- [do.samber.dev](https://do.samber.dev)
- [github.com/samber/do/v2](https://github.com/samber/do)

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

DO NOT USE v1 OF THIS LIBRARY. INSTALL v2 INSTEAD:

```bash
go get -u github.com/samber/do/v2
```

## Core Concepts

### The Injector (Container)

```go
import "github.com/samber/do/v2"

injector := do.New()
```

### Service Types

- **Lazy** (default): Created when first requested
- **Eager**: Created immediately when the container starts
- **Transient**: New instance created on every request
- **Value**: Pre-created value, no instantiation

### Provider Functions

Services MUST be registered via provider functions:

```go
type Provider[T any] func(i Injector) (T, error)
```

## Basic Usage

### 1. Define and Register Services

Follow "Accept Interfaces, Return Structs":

```go
// Register a service (lazy by default)
do.Provide(injector, func(i do.Injector) (Database, error) {
    return &PostgreSQLDatabase{connString: "postgres://..."}, nil
})

// Register a pre-created value
do.ProvideValue(injector, &Config{Port: 8080})

// Register a transient service (new instance each time)
do.ProvideTransient(injector, func(i do.Injector) (*Logger, error) {
    return &Logger{}, nil
})

// Register an eager service (created immediately)
do.Provide(injector, do.Eager(&Config{Port: 8080}))
```

### 2. Invoke Services

The container MUST only be accessed at the composition root:

```go
// Invoke with error handling
db, err := do.Invoke[Database](injector)

// MustInvoke panics on error (use when confident service exists)
db := do.MustInvoke[Database](injector)
```

### 3. Service Dependencies

```go
func NewUserService(i do.Injector) (UserService, error) {
    db := do.MustInvoke[Database](i)
    cache := do.MustInvoke[Cache](i)
    return &userService{db: db, cache: cache}, nil
}

do.Provide(injector, NewUserService)
```

### 4. Implicit Aliasing (Preferred)

Register a concrete type and invoke as an interface without explicit aliasing:

```go
// Register concrete type
do.Provide(injector, func(i do.Injector) (*PostgreSQLDatabase, error) {
    return &PostgreSQLDatabase{}, nil
})

// Invoke directly as interface (implicit aliasing)
db := do.MustInvokeAs[Database](injector)
```

### 5. Named Services

Register multiple services of the same type:

```go
do.ProvideNamed(injector, "primary-db", func(i do.Injector) (*Database, error) {
    return &Database{URL: "postgres://primary..."}, nil
})

mainDB := do.MustInvokeNamed[*Database](injector, "primary-db")
```

## Package Organization

Use `do.Package()` to organize service registration by module:

```go
// infrastructure/package.go
var Package = do.Package(
    do.Lazy(func(i do.Injector) (*postgres.DB, error) {
        cfg := do.MustInvoke[*Config](i)
        return postgres.Connect(cfg.DatabaseURL)
    }),
    do.Lazy(func(i do.Injector) (*redis.Client, error) {
        cfg := do.MustInvoke[*Config](i)
        return redis.NewClient(cfg.RedisURL), nil
    }),
)

// main.go
injector := do.New(infrastructure.Package, service.Package)
```

## Full Application Setup

```go
func main() {
    injector := do.New(
        infrastructure.Package,
        repository.Package,
        service.Package,
        transport.Package,
    )

    server := do.MustInvoke[*http.Server](injector)
    go server.ListenAndServe()

    _ = injector.ShutdownOnSignalsWithContext(context.Background(), os.Interrupt)
}
```

## Best Practices

1. Depend on interfaces, not concrete types — lets you swap implementations in tests without touching production code
2. Each service should have one job — services with multiple responsibilities are harder to test and harder to replace
3. Keep dependency trees shallow — chains beyond 3-4 levels make initialization order fragile and errors harder to trace
4. Handle errors in provider functions — a silently failing provider creates a broken service that crashes later in unexpected places
5. Use scopes to organize services by lifecycle — request-scoped services prevent leaks, global services prevent redundant initialization

For scopes, lifecycle management, struct injection, and debugging, see [Advanced Usage](./references/advanced.md).

For testing patterns (cloning, overrides, mocks), see [Testing](./references/testing.md).

## Quick Reference

### Registration

| Function                        | Purpose                          |
| ------------------------------- | -------------------------------- |
| `do.Provide[T]()`               | Register lazy service (default)  |
| `do.ProvideNamed[T]()`          | Register named lazy service      |
| `do.ProvideValue[T]()`          | Register pre-created value       |
| `do.ProvideNamedValue[T]()`     | Register named value             |
| `do.ProvideTransient[T]()`      | Register new instance each time  |
| `do.ProvideNamedTransient[T]()` | Register named transient service |
| `do.Package()`                  | Group service registrations      |

### Invocation

| Function                   | Purpose                                   |
| -------------------------- | ----------------------------------------- |
| `do.Invoke[T]()`           | Get service (with error)                  |
| `do.InvokeNamed[T]()`      | Get named service                         |
| `do.InvokeAs[T]()`         | Get first service matching interface      |
| `do.InvokeStruct[T]()`     | Inject into struct fields using tags      |
| `do.MustInvoke[T]()`       | Get service (panic on error)              |
| `do.MustInvokeNamed[T]()`  | Get named service (panic on error)        |
| `do.MustInvokeAs[T]()`     | Get service by interface (panic on error) |
| `do.MustInvokeStruct[T]()` | Inject into struct (panic on error)       |

## Cross-References

- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for DI concepts, comparison, and when to adopt a DI library
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for interface design patterns
- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for general testing patterns

## Sub-topic: golang-samber-hot

**Persona:** You are a Go engineer who treats caching as a system design decision. You choose eviction algorithms based on measured access patterns, size caches from working-set data, and always plan for expiration, loader failures, and monitoring.

# Using samber/hot for In-Memory Caching in Go

Generic, type-safe in-memory caching library for Go 1.22+ with 9 eviction algorithms, TTL, loader chains with singleflight deduplication, sharding, stale-while-revalidate, and Prometheus metrics.

**Official Resources:**

- [pkg.go.dev/github.com/samber/hot](https://pkg.go.dev/github.com/samber/hot)
- [github.com/samber/hot](https://github.com/samber/hot)

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

```bash
go get -u github.com/samber/hot
```

## Algorithm Selection

Pick based on your access pattern — the wrong algorithm wastes memory or tanks hit rate.

| Algorithm | Constant | Best for | Avoid when |
| --- | --- | --- | --- |
| **W-TinyLFU** | `hot.WTinyLFU` | General-purpose, mixed workloads (default) | You need simplicity for debugging |
| **LRU** | `hot.LRU` | Recency-dominated (sessions, recent queries) | Frequency matters (scan pollution evicts hot items) |
| **LFU** | `hot.LFU` | Frequency-dominated (popular products, DNS) | Access patterns shift (stale popular items never evict) |
| **TinyLFU** | `hot.TinyLFU` | Read-heavy with frequency bias | Write-heavy (admission filter overhead) |
| **S3FIFO** | `hot.S3FIFO` | High throughput, scan-resistant | Small caches (<1000 items) |
| **ARC** | `hot.ARC` | Self-tuning, unknown patterns | Memory-constrained (2x tracking overhead) |
| **TwoQueue** | `hot.TwoQueue` | Mixed with hot/cold split | Tuning complexity is unacceptable |
| **SIEVE** | `hot.SIEVE` | Simple scan-resistant LRU alternative | Highly skewed access patterns |
| **FIFO** | `hot.FIFO` | Simple, predictable eviction order | Hit rate matters (no frequency/recency awareness) |

**Decision shortcut:** Start with `hot.WTinyLFU`. Switch only when profiling shows the miss rate is too high for your SLO.

For detailed algorithm comparison, benchmarks, and a decision tree, see [Algorithm Guide](./references/algorithm-guide.md).

## Core Usage

### Basic Cache with TTL

```go
import "github.com/samber/hot"

cache := hot.NewHotCache[string, *User](hot.WTinyLFU, 10_000).
    WithTTL(5 * time.Minute).
    WithJanitor().
    Build()
defer cache.StopJanitor()

cache.Set("user:123", user)
cache.SetWithTTL("session:abc", session, 30*time.Minute)

value, found, err := cache.Get("user:123")
```

### Loader Pattern (Read-Through)

Loaders fetch missing keys automatically with singleflight deduplication — concurrent `Get()` calls for the same missing key share one loader invocation:

```go
cache := hot.NewHotCache[int, *User](hot.WTinyLFU, 10_000).
    WithTTL(5 * time.Minute).
    WithLoaders(func(ids []int) (map[int]*User, error) {
        return db.GetUsersByIDs(ctx, ids) // batch query
    }).
    WithJanitor().
    Build()
defer cache.StopJanitor()

user, found, err := cache.Get(123) // triggers loader on miss
```

## Capacity Sizing

Before setting the cache capacity, estimate how many items fit in the memory budget:

1. **Estimate single-item size** — estimate size of the struct, add the size of heap-allocated fields (slices, maps, strings). Include the key size. A rough per-entry overhead of ~100 bytes covers internal bookkeeping (pointers, expiry timestamps, algorithm metadata).
2. **Ask the developer** how much memory is dedicated to this cache in production (e.g., 256 MB, 1 GB). This depends on the service's total memory and what else shares the process.
3. **Compute capacity** — `capacity = memoryBudget / estimatedItemSize`. Round down to leave headroom.

```
Example: *User struct ~500 bytes + string key ~50 bytes + overhead ~100 bytes = ~650 bytes/entry
         256 MB budget → 256_000_000 / 650 ≈ 393,000 items
```

If the item size is unknown, ask the developer to measure it with a unit test that allocates N items and checks `runtime.ReadMemStats`. Guessing capacity without measuring leads to OOM or wasted memory.

## Common Mistakes

1. **Forgetting `WithJanitor()`** — without it, expired entries stay in memory until the algorithm evicts them. Always chain `.WithJanitor()` in the builder and `defer cache.StopJanitor()`.
2. **Calling `SetMissing()` without missing cache config** — panics at runtime. Enable `WithMissingCache(algorithm, capacity)` or `WithMissingSharedCache()` in the builder first.
3. **`WithoutLocking()` + `WithJanitor()`** — mutually exclusive, panics. `WithoutLocking()` is only safe for single-goroutine access without background cleanup.
4. **Oversized cache** — a cache holding everything is a map with overhead. Size to your working set (typically 10-20% of total data). Monitor hit rate to validate.
5. **Ignoring loader errors** — `Get()` returns `(zero, false, err)` on loader failure. Always check `err`, not just `found`.

## Best Practices

1. Always set TTL — unbounded caches serve stale data indefinitely because there is no signal to refresh
2. Use `WithJitter(lambda, upperBound)` to spread expirations — without jitter, items created together expire together, causing thundering herd on the loader
3. Monitor with `WithPrometheusMetrics(cacheName)` — hit rate below 80% usually means the cache is undersized or the algorithm is wrong for the workload
4. Use `WithCopyOnRead(fn)` / `WithCopyOnWrite(fn)` for mutable values — without copies, callers mutate cached objects and corrupt shared state

For advanced patterns (revalidation, sharding, missing cache, monitoring setup), see [Production Patterns](./references/production-patterns.md).

For the complete API surface, see [API Reference](./references/api-reference.md).

If you encounter a bug or unexpected behavior in samber/hot, open an issue at <https://github.com/samber/hot/issues>.

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for general caching strategy and when to use in-memory cache vs Redis vs CDN
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for Prometheus metrics integration and monitoring
- → See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for database query patterns that pair with cache loaders
- → See `samber/cc-skills@promql-cli` skill for querying Prometheus cache metrics via CLI

## Sub-topic: golang-samber-lo

**Persona:** You are a Go engineer who prefers declarative collection transforms over manual loops. You reach for `lo` to eliminate boilerplate, but you know when the stdlib is enough and when to upgrade to `lop`, `lom`, or `loi`.

# samber/lo — Functional Utilities for Go

Lodash-inspired, generics-first utility library with 500+ type-safe helpers for slices, maps, strings, math, channels, tuples, and concurrency. Zero external dependencies. Immutable by default.

**Official Resources:**

- [github.com/samber/lo](https://github.com/samber/lo)
- [lo.samber.dev](https://lo.samber.dev)
- [pkg.go.dev/github.com/samber/lo](https://pkg.go.dev/github.com/samber/lo)

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

## Why samber/lo

Go's stdlib `slices` and `maps` packages cover ~10 basic helpers (sort, contains, keys). Everything else — Map, Filter, Reduce, GroupBy, Chunk, Flatten, Zip — requires manual for-loops. `lo` fills this gap:

- **Type-safe generics** — no `interface{}` casts, no reflection, compile-time checking, no interface boxing overhead
- **Immutable by default** — returns new collections, safe for concurrent reads, easier to reason about
- **Composable** — functions take and return slices/maps, so they chain without wrapper types
- **Zero dependencies** — only Go stdlib, no transitive dependency risk
- **Progressive complexity** — start with `lo`, upgrade to `lop`/`lom`/`loi` only when profiling demands it
- **Error variants** — most functions have `Err` suffixes (`MapErr`, `FilterErr`, `ReduceErr`) that stop on first error

## Installation

```bash
go get github.com/samber/lo
```

| Package | Import | Alias | Go version |
| --- | --- | --- | --- |
| Core (immutable) | `github.com/samber/lo` | `lo` | 1.18+ |
| Parallel | `github.com/samber/lo/parallel` | `lop` | 1.18+ |
| Mutable | `github.com/samber/lo/mutable` | `lom` | 1.18+ |
| Iterator | `github.com/samber/lo/it` | `loi` | 1.23+ |
| SIMD (experimental) | `github.com/samber/lo/exp/simd` | — | 1.25+ (amd64 only) |

## Choose the Right Package

Start with `lo`. Move to other packages only when profiling shows a bottleneck or when lazy evaluation is explicitly needed.

| Package | Use when | Trade-off |
| --- | --- | --- |
| `lo` | Default for all transforms | Allocates new collections (safe, predictable) |
| `lop` | CPU-bound work on large datasets (1000+ items) | Goroutine overhead; not for I/O or small slices |
| `lom` | Hot path confirmed by `pprof -alloc_objects` | Mutates input — caller must understand side effects |
| `loi` | Large datasets with chained transforms (Go 1.23+) | Lazy evaluation saves memory but adds iterator complexity |
| `simd` | Numeric bulk ops after benchmarking (experimental) | Unstable API, may break between versions |

**Key rules:**

- `lop` is for CPU parallelism, not I/O concurrency — for I/O fan-out, use `errgroup` instead
- `lom` breaks immutability — only use when allocation pressure is measured, never assumed
- `loi` eliminates intermediate allocations in chains like `Map → Filter → Take` by evaluating lazily
- For reactive/streaming pipelines over infinite event streams, → see the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) + `samber/ro` package

For detailed package comparison and decision flowchart, see [Package Guide](./references/package-guide.md).

## Core Patterns

### Transform a slice

```go
// ✓ lo — declarative, type-safe
names := lo.Map(users, func(u User, _ int) string {
    return u.Name
})

// ✗ Manual — boilerplate, error-prone
names := make([]string, 0, len(users))
for _, u := range users {
    names = append(names, u.Name)
}
```

### Filter + Reduce

```go
total := lo.Reduce(
    lo.Filter(orders, func(o Order, _ int) bool {
        return o.Status == "paid"
    }),
    func(sum float64, o Order, _ int) float64 {
        return sum + o.Amount
    },
    0,
)
```

### GroupBy

```go
byStatus := lo.GroupBy(tasks, func(t Task, _ int) string {
    return t.Status
})
// map[string][]Task{"open": [...], "closed": [...]}
```

### Error variant — stop on first error

```go
results, err := lo.MapErr(urls, func(url string, _ int) (Response, error) {
    return http.Get(url)
})
```

## Common Mistakes

| Mistake | Why it fails | Fix |
| --- | --- | --- |
| Using `lo.Contains` when `slices.Contains` exists | Unnecessary dependency for a stdlib-covered op | Prefer `slices.Contains`, `slices.Sort`, `maps.Keys` since Go 1.21+ |
| Using `lop.Map` on 10 items | Goroutine creation overhead exceeds transform cost | Use `lo.Map` — `lop` benefits start at ~1000+ items for CPU-bound work |
| Assuming `lo.Filter` modifies the input | `lo` is immutable by default — it returns a new slice | Use `lom.Filter` if you explicitly need in-place mutation |
| Using `lo.Must` in production code paths | `Must` panics on error — fine in tests and init, dangerous in request handlers | Use the non-Must variant and handle the error |
| Chaining many eager transforms on large data | Each step allocates an intermediate slice | Use `loi` (lazy iterators) to avoid intermediate allocations |

## Best Practices

1. **Prefer stdlib when available** — `slices.Contains`, `slices.Sort`, `maps.Keys` carry no dependency. Use `lo` for transforms the stdlib doesn't offer (Map, Filter, Reduce, GroupBy, Chunk, Flatten)
2. **Compose lo functions** — chain `lo.Filter` → `lo.Map` → `lo.GroupBy` instead of writing nested loops. Each function is a building block
3. **Profile before optimizing** — switch from `lo` to `lom`/`lop` only after `go tool pprof` confirms allocation or CPU as the bottleneck
4. **Use error variants** — prefer `lo.MapErr` over `lo.Map` + manual error collection. Error variants stop early and propagate cleanly
5. **Use `lo.Must` only in tests and init** — in production, handle errors explicitly

## Quick Reference

| Function | What it does |
| --- | --- |
| `lo.Map` | Transform each element |
| `lo.Filter` / `lo.Reject` | Keep / remove elements matching predicate |
| `lo.Reduce` | Fold elements into a single value |
| `lo.ForEach` | Side-effect iteration |
| `lo.GroupBy` | Group elements by key |
| `lo.Chunk` | Split into fixed-size batches |
| `lo.Flatten` | Flatten nested slices one level |
| `lo.Uniq` / `lo.UniqBy` | Remove duplicates |
| `lo.Find` / `lo.FindOrElse` | First match or default |
| `lo.Contains` / `lo.Every` / `lo.Some` | Membership tests |
| `lo.Keys` / `lo.Values` | Extract map keys or values |
| `lo.PickBy` / `lo.OmitBy` | Filter map entries |
| `lo.Zip2` / `lo.Unzip2` | Pair/unpair two slices |
| `lo.Range` / `lo.RangeFrom` | Generate number sequences |
| `lo.Ternary` / `lo.If` | Inline conditionals |
| `lo.ToPtr` / `lo.FromPtr` | Pointer helpers |
| `lo.Must` / `lo.Try` | Panic-on-error / recover-as-bool |
| `lo.Async` / `lo.Attempt` | Async execution / retry with backoff |
| `lo.Debounce` / `lo.Throttle` | Rate limiting |
| `lo.ChannelDispatcher` | Fan-out to multiple channels |

For the complete function catalog (300+ functions), see [API Reference](./references/api-reference.md).

For composition patterns, stdlib interop, and iterator pipelines, see [Advanced Patterns](./references/advanced-patterns.md).

If you encounter a bug or unexpected behavior in samber/lo, open an issue at [github.com/samber/lo/issues](https://github.com/samber/lo/issues).

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for reactive/streaming pipelines over infinite event streams (`samber/ro` package)
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for monadic types (Option, Result, Either) that compose with lo transforms
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for choosing the right underlying data structure
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for profiling methodology before switching to `lom`/`lop`

## Sub-topic: golang-samber-mo

**Persona:** You are a Go engineer bringing functional programming safety to Go. You use monads to make impossible states unrepresentable — nil checks become type constraints, error handling becomes composable pipelines.

**Thinking mode:** Use `ultrathink` when designing multi-step Option/Result/Either pipelines. Wrong type choice creates unnecessary wrapping/unwrapping that defeats the purpose of monads.

# samber/mo — Monads and Functional Abstractions for Go

Go 1.18+ library providing type-safe monadic types with zero dependencies. Inspired by Scala, Rust, and fp-ts.

**Official Resources:**

- [pkg.go.dev/github.com/samber/mo](https://pkg.go.dev/github.com/samber/mo)
- [github.com/samber/mo](https://github.com/samber/mo)

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

```bash
go get github.com/samber/mo
```

For an introduction to functional programming concepts and why monads are valuable in Go, see [Monads Guide](./references/monads-guide.md).

## Core Types at a Glance

| Type | Purpose | Think of it as... |
| --- | --- | --- |
| `Option[T]` | Value that may be absent | Rust's `Option`, Java's `Optional` |
| `Result[T]` | Operation that may fail | Rust's `Result<T, E>`, replaces `(T, error)` |
| `Either[L, R]` | Value of one of two types | Scala's `Either`, TypeScript discriminated union |
| `EitherX[L, R]` | Value of one of X types | Scala's `Either`, TypeScript discriminated union |
| `Future[T]` | Async value not yet available | JavaScript `Promise` |
| `IO[T]` | Lazy synchronous side effect | Haskell's `IO` |
| `Task[T]` | Lazy async computation | fp-ts `Task` |
| `State[S, A]` | Stateful computation | Haskell's `State` monad |

## Option[T] — Nullable Values Without nil

Represents a value that is either present (`Some`) or absent (`None`). Eliminates nil pointer risks at the type level.

```go
import "github.com/samber/mo"

name := mo.Some("Alice")          // Option[string] with value
empty := mo.None[string]()        // Option[string] without value
fromPtr := mo.PointerToOption(ptr) // nil pointer -> None

// Safe extraction
name.OrElse("Anonymous")  // "Alice"
empty.OrElse("Anonymous")  // "Anonymous"

// Transform if present, skip if absent
upper := name.Map(func(s string) (string, bool) {
    return strings.ToUpper(s), true
})
```

**Key methods:** `Some`, `None`, `Get`, `MustGet`, `OrElse`, `OrEmpty`, `Map`, `FlatMap`, `Match`, `ForEach`, `ToPointer`, `IsPresent`, `IsAbsent`.

Option implements `json.Marshaler/Unmarshaler`, `sql.Scanner`, `driver.Valuer` — use it directly in JSON structs and database models.

For full API reference, see [Option Reference](./references/option.md).

## Result[T] — Error Handling as Values

Represents success (`Ok`) or failure (`Err`). Equivalent to `Either[error, T]` but specialized for Go's error pattern.

```go
// Wrap Go's (value, error) pattern
result := mo.TupleToResult(os.ReadFile("config.yaml"))

// Same-type transform — errors short-circuit automatically
upper := mo.Ok("hello").Map(func(s string) (string, error) {
    return strings.ToUpper(s), nil
})
// Ok("HELLO")

// Extract with fallback
val := upper.OrElse("default")
```

**Go limitation:** Direct methods (`.Map`, `.FlatMap`) cannot change the type parameter — `Result[T].Map` returns `Result[T]`, not `Result[U]`. Go methods cannot introduce new type parameters. For type-changing transforms (e.g. `Result[[]byte]` to `Result[Config]`), use sub-package functions or `mo.Do`:

```go
import "github.com/samber/mo/result"

// Type-changing pipeline: []byte -> Config -> ValidConfig
parsed := result.Pipe2(
    mo.TupleToResult(os.ReadFile("config.yaml")),
    result.Map(func(data []byte) Config { return parseConfig(data) }),
    result.FlatMap(func(cfg Config) mo.Result[ValidConfig] { return validate(cfg) }),
)
```

**Key methods:** `Ok`, `Err`, `Errf`, `TupleToResult`, `Try`, `Get`, `MustGet`, `OrElse`, `Map`, `FlatMap`, `MapErr`, `Match`, `ForEach`, `ToEither`, `IsOk`, `IsError`.

For full API reference, see [Result Reference](./references/result.md).

## Either[L, R] — Discriminated Union of Two Types

Represents a value that is one of two possible types. Unlike Result, neither side implies success or failure — both are valid alternatives.

```go
// API that returns either cached data or fresh data
func fetchUser(id string) mo.Either[CachedUser, FreshUser] {
    if cached, ok := cache.Get(id); ok {
        return mo.Left[CachedUser, FreshUser](cached)
    }
    return mo.Right[CachedUser, FreshUser](db.Fetch(id))
}

// Pattern match
result.Match(
    func(cached CachedUser) mo.Either[CachedUser, FreshUser] { /* use cached */ },
    func(fresh FreshUser) mo.Either[CachedUser, FreshUser] { /* use fresh */ },
)
```

**When to use Either vs Result:** Use `Result[T]` when one path is an error. Use `Either[L, R]` when both paths are valid alternatives (cached vs fresh, left vs right, strategy A vs B).

`Either3[T1, T2, T3]`, `Either4`, and `Either5` extend this to 3-5 type variants.

For full API reference, see [Either Reference](./references/either.md).

## Do Notation — Imperative Style with Monadic Safety

`mo.Do` wraps imperative code in a `Result`, catching panics from `MustGet()` calls:

```go
result := mo.Do(func() int {
    // MustGet panics on None/Err — Do catches it as Result error
    a := mo.Some(21).MustGet()
    b := mo.Ok(2).MustGet()
    return a * b  // 42
})
// result is Ok(42)

result := mo.Do(func() int {
    val := mo.None[int]().MustGet()  // panics
    return val
})
// result is Err("no such element")
```

Do notation bridges imperative Go style with monadic safety — write straight-line code, get automatic error propagation.

## Pipeline Sub-Packages vs Direct Chaining

samber/mo provides two ways to compose operations:

**Direct methods** (`.Map`, `.FlatMap`) — work when the output type equals the input type:

```go
opt := mo.Some(42)
doubled := opt.Map(func(v int) (int, bool) {
    return v * 2, true
})  // Option[int]
```

**Sub-package functions** (`option.Map`, `result.Map`) — required when the output type differs from input:

```go
import "github.com/samber/mo/option"

// int -> string type change: use sub-package Map
strOpt := option.Map(func(v int) string {
    return fmt.Sprintf("value: %d", v)
})(mo.Some(42))  // Option[string]
```

**Pipe functions** (`option.Pipe3`, `result.Pipe3`) — chain multiple type-changing transformations readably:

```go
import "github.com/samber/mo/option"

result := option.Pipe3(
    mo.Some(42),
    option.Map(func(v int) string { return strconv.Itoa(v) }),
    option.Map(func(s string) []byte { return []byte(s) }),
    option.FlatMap(func(b []byte) mo.Option[string] {
        if len(b) > 0 { return mo.Some(string(b)) }
        return mo.None[string]()
    }),
)
```

**Rule of thumb:** Use direct methods for same-type transforms. Use sub-package functions + pipes when types change across steps.

For detailed pipeline API reference, see [Pipelines Reference](./references/pipelines.md).

## Common Patterns

### JSON API responses with Option

```go
type UserResponse struct {
    Name     string            `json:"name"`
    Nickname mo.Option[string] `json:"nickname"`  // omits null gracefully
    Bio      mo.Option[string] `json:"bio"`
}
```

### Database nullable columns

```go
type User struct {
    ID       int
    Email    string
    Phone    mo.Option[string]  // implements sql.Scanner + driver.Valuer
}

err := row.Scan(&u.ID, &u.Email, &u.Phone)
```

### Wrapping existing Go APIs

```go
// Convert map lookup to Option
func MapGet[K comparable, V any](m map[K]V, key K) mo.Option[V] {
    return mo.TupleToOption(m[key])  // m[key] returns (V, bool)
}
```

### Uniform extraction with Fold

`mo.Fold` works uniformly across Option, Result, and Either via the `Foldable` interface:

```go
str := mo.Fold[error, int, string](
    mo.Ok(42),  // works with Option, Result, or Either
    func(v int) string { return fmt.Sprintf("got %d", v) },
    func(err error) string { return "failed" },
)
// "got 42"
```

## Best Practices

1. **Prefer `OrElse` over `MustGet`** — `MustGet` panics on absent/error values; use it only inside `mo.Do` blocks where panics are caught, or when you are certain the value exists
2. **Use `TupleToResult` at API boundaries** — convert Go's `(T, error)` to `Result[T]` at the boundary, then chain with `Map`/`FlatMap` inside your domain logic
3. **Use `Result[T]` for errors, `Either[L, R]` for alternatives** — Result is specialized for success/failure; Either is for two valid types
4. **Option for nullable fields, not zero values** — `Option[string]` distinguishes "absent" from "empty string"; use plain `string` when empty string is a valid value
5. **Chain, don't nest** — `result.Map(...).FlatMap(...).OrElse(default)` reads left-to-right; avoid nested if/else patterns when monadic chaining is cleaner
6. **Use sub-package pipes for multi-step type transformations** — when 3+ steps each change the type, `option.Pipe3(...)` is more readable than nested function calls

For advanced types (Future, IO, Task, State), see [Advanced Types Reference](./references/advanced-types.md).

If you encounter a bug or unexpected behavior in samber/mo, open an issue at <https://github.com/samber/mo/issues>.

## Cross-References

- -> See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for functional collection transforms (Map, Filter, Reduce on slices) that compose with mo types
- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for idiomatic Go error handling patterns
- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for nil-safety and defensive Go coding
- -> See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for database access patterns
- -> See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for functional options and other Go patterns

## Sub-topic: golang-samber-oops

**Persona:** You are a Go engineer who treats errors as structured data. Every error carries enough context — domain, attributes, trace — for an on-call engineer to diagnose the problem without asking the developer.

# samber/oops Structured Error Handling

**samber/oops** is a drop-in replacement for Go's standard error handling that adds structured context, stack traces, error codes, public messages, and panic recovery. Variable data goes in `.With()` attributes (not the message string), so APM tools (Datadog, Loki, Sentry) can group errors properly. Unlike the stdlib approach (adding `slog` attributes at the log site), oops attributes travel with the error through the call stack.

## Why use samber/oops

Standard Go errors lack context — you see `connection failed` but not which user triggered it, what query was running, or the full call stack. `samber/oops` provides:

- **Structured context** — key-value attributes on any error
- **Stack traces** — automatic call stack capture
- **Error codes** — machine-readable identifiers
- **Public messages** — user-safe messages separate from technical details
- **Low-cardinality messages** — variable data in `.With()` attributes, not the message string, so APM tools group errors properly

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

## Core pattern: Error builder chain

All `oops` errors use a fluent builder pattern:

```go
err := oops.
    In("user-service").           // domain/feature
    Tags("database", "postgres").  // categorization
    Code("network_failure").       // machine-readable identifier
    User("user-123", "email", "foo@bar.com").  // user context
    With("query", query).          // custom attributes
    Errorf("failed to fetch user: %s", "timeout")
```

Terminal methods:

- `.Errorf(format, args...)` — create a new error
- `.Wrap(err)` — wrap an existing error
- `.Wrapf(err, format, args...)` — wrap with a message
- `.Join(err1, err2, ...)` — combine multiple errors
- `.Recover(fn)` / `.Recoverf(fn, format, args...)` — convert panic to error

### Error builder methods

| Methods | Use case |
| --- | --- |
| `.With("key", value)` | Add custom key-value attribute (lazy `func() any` values supported) |
| `.WithContext(ctx, "key1", "key2")` | Extract values from Go context into attributes (lazy values supported) |
| `.In("domain")` | Set the feature/service/domain |
| `.Tags("auth", "sql")` | Add categorization tags (query with `err.HasTag("tag")`) |
| `.Code("iam_authz_missing_permission")` | Set machine-readable error identifier/slug |
| `.Public("Could not fetch user.")` | Set user-safe message (separate from technical details) |
| `.Hint("Runbook: https://doc.acme.org/doc/abcd.md")` | Add debugging hint for developers |
| `.Owner("team/slack")` | Identify responsible team/owner |
| `.User(id, "k", "v")` | Add user identifier and attributes |
| `.Tenant(id, "k", "v")` | Add tenant/organization context and attributes |
| `.Trace(id)` | Add trace / correlation ID (default: ULID) |
| `.Span(id)` | Add span ID representing a unit of work/operation (default: ULID) |
| `.Time(t)` | Override error timestamp (default: `time.Now()`) |
| `.Since(t)` | Set duration based on time since `t` (exposed via `err.Duration()`) |
| `.Duration(d)` | Set explicit error duration |
| `.Request(req, includeBody)` | Attach `*http.Request` (optionally including body) |
| `.Response(res, includeBody)` | Attach `*http.Response` (optionally including body) |
| `oops.FromContext(ctx)` | Start from an `OopsErrorBuilder` stored in a Go context |

## Common scenarios

### Database/repository layer

```go
func (r *UserRepository) FetchUser(id string) (*User, error) {
    query := "SELECT * FROM users WHERE id = $1"
    row, err := r.db.Query(query, id)
    if err != nil {
        return nil, oops.
            In("user-repository").
            Tags("database", "postgres").
            With("query", query).
            With("user_id", id).
            Wrapf(err, "failed to fetch user from database")
    }
    // ...
}
```

### HTTP handler layer

```go
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)

    err := h.service.CreateUser(r.Context(), userID)
    if err != nil {
        return oops.
            In("http-handler").
            Tags("endpoint", "/users").
            Request(r, false).
            User(userID).
            Wrapf(err, "create user failed")
    }

    w.WriteHeader(http.StatusCreated)
}
```

### Service layer with reusable builder

```go
func (s *UserService) CreateOrder(ctx context.Context, req CreateOrderRequest) error {
    builder := oops.
        In("order-service").
        Tags("orders", "checkout").
        Tenant(req.TenantID, "plan", req.Plan).
        User(req.UserID, "email", req.UserEmail)

    product, err := s.catalog.GetProduct(ctx, req.ProductID)
    if err != nil {
        return builder.
            With("product_id", req.ProductID).
            Wrapf(err, "product lookup failed")
    }

    if product.Stock < req.Quantity {
        return builder.
            Code("insufficient_stock").
            Public("Not enough items in stock.").
            With("requested", req.Quantity).
            With("available", product.Stock).
            Errorf("insufficient stock for product %s", req.ProductID)
    }

    return nil
}
```

## Error wrapping best practices

### DO: Wrap directly, no nil check needed

```go
// ✓ Good — Wrap returns nil if err is nil
return oops.Wrapf(err, "operation failed")

// ✗ Bad — unnecessary nil check
if err != nil {
    return oops.Wrapf(err, "operation failed")
}
return nil
```

### DO: Add context at each layer

Each architectural layer SHOULD add context via Wrap/Wrapf — at least once per package boundary (not necessarily at every function call).

```go
// ✓ Good — each layer adds relevant context
func Controller() error {
    return oops.In("controller").Trace(traceID).Wrapf(Service(), "user request failed")
}

func Service() error {
    return oops.In("service").With("op", "create_user").Wrapf(Repository(), "db operation failed")
}

func Repository() error {
    return oops.In("repository").Tags("database", "postgres").Errorf("connection timeout")
}
```

### DO: Keep error messages low-cardinality

Error messages MUST be low-cardinality for APM aggregation. Interpolating variable data into the message breaks grouping in Datadog, Loki, Sentry.

```go
// ✗ Bad — high-cardinality, breaks APM grouping
oops.Errorf("failed to process user %s in tenant %s", userID, tenantID)

// ✓ Good — static message + structured attributes
oops.With("user_id", userID).With("tenant_id", tenantID).Errorf("failed to process user")
```

## Panic recovery

`oops.Recover()` MUST be used in goroutine boundaries. Convert panics to structured errors:

```go
func ProcessData(data string) (err error) {
    return oops.
        In("data-processor").
        Code("panic_recovered").
        Hint("Check input data format and dependencies").
        With("panic_value", r).
        Recover(func() {
            riskyOperation(data)
        })
}
```

## Accessing error information

`samber/oops` errors implement the standard `error` interface. Access additional info:

```go
if oopsErr, ok := err.(oops.OopsError); ok {
    fmt.Println("Code:", oopsErr.Code())
    fmt.Println("Domain:", oopsErr.Domain())
    fmt.Println("Tags:", oopsErr.Tags())
    fmt.Println("Context:", oopsErr.Context())
    fmt.Println("Stacktrace:", oopsErr.Stacktrace())
}

// Get public-facing message with fallback
publicMsg := oops.GetPublic(err, "Something went wrong")
```

### Output formats

```go
fmt.Printf("%+v\n", err)       // verbose with stack trace
bytes, _ := json.Marshal(err)  // JSON for logging
slog.Error(err.Error(), slog.Any("error", err))  // slog integration
```

## Context propagation

Carry error context through Go contexts:

```go
func middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        builder := oops.
            In("http").
            Request(r, false).
            Trace(r.Header.Get("X-Trace-ID"))

        ctx := oops.WithBuilder(r.Context(), builder)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func handler(ctx context.Context) error {
    return oops.FromContext(ctx).Tags("handler", "users").Errorf("something failed")
}
```

For assertions, configuration, and additional logger examples, see [Advanced patterns](./references/advanced.md).

## References

- [github.com/samber/oops](https://github.com/samber/oops)
- [pkg.go.dev/github.com/samber/oops](https://pkg.go.dev/github.com/samber/oops)

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for general error handling patterns
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for logger integration and structured logging

## Sub-topic: golang-samber-ro

**Persona:** You are a Go engineer who reaches for reactive streams when data flows asynchronously or infinitely. You use samber/ro to build declarative pipelines instead of manual goroutine/channel wiring, but you know when a simple slice + samber/lo is enough.

**Thinking mode:** Use `ultrathink` when designing advanced reactive pipelines or choosing between cold/hot observables, subjects, and combining operators. Wrong architecture leads to resource leaks or missed events.

# samber/ro — Reactive Streams for Go

Go implementation of [ReactiveX](https://reactivex.io/). Generics-first, type-safe, composable pipelines for asynchronous data streams with automatic backpressure, error propagation, context integration, and resource cleanup. 150+ operators, 5 subject types, 40+ plugins.

**Official Resources:**

- [github.com/samber/ro](https://github.com/samber/ro)
- [ro.samber.dev](https://ro.samber.dev)
- [pkg.go.dev/github.com/samber/ro](https://pkg.go.dev/github.com/samber/ro)

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

## Why samber/ro (Streams vs Slices)

Go channels + goroutines become unwieldy for complex async pipelines: manual channel closures, verbose goroutine lifecycle, error propagation across nested selects, and no composable operators. `samber/ro` solves this with declarative, chainable stream operators.

**When to use which tool:**

| Scenario | Tool | Why |
| --- | --- | --- |
| Transform a slice (map, filter, reduce) | `samber/lo` | Finite, synchronous, eager — no stream overhead needed |
| Simple goroutine fan-out with error handling | `errgroup` | Standard lib, lightweight, sufficient for bounded concurrency |
| Infinite event stream (WebSocket, tickers, file watcher) | `samber/ro` | Declarative pipeline with backpressure, retry, timeout, combine |
| Real-time data enrichment from multiple async sources | `samber/ro` | CombineLatest/Zip compose dependent streams without manual select |
| Pub/sub with multiple consumers sharing one source | `samber/ro` | Hot observables (Share/Subjects) handle multicast natively |

**Key differences: lo vs ro**

| Aspect | `samber/lo` | `samber/ro` |
| --- | --- | --- |
| Data | Finite slices | Infinite streams |
| Execution | Synchronous, blocking | Asynchronous, non-blocking |
| Evaluation | Eager (allocates intermediate slices) | Lazy (processes items as they arrive) |
| Timing | Immediate | Time-aware (delay, throttle, interval, timeout) |
| Error model | Return `(T, error)` per call | Error channel propagates through pipeline |
| Use case | Collection transforms | Event-driven, real-time, async pipelines |

## Installation

```bash
go get github.com/samber/ro
```

## Core Concepts

Four building blocks:

1. **Observable** — a data source that emits values over time. Cold by default: each subscriber triggers independent execution from scratch
2. **Observer** — a consumer with three callbacks: `onNext(T)`, `onError(error)`, `onComplete()`
3. **Operator** — a function that transforms an observable into another observable, chained via `Pipe`
4. **Subscription** — the connection between observable and observer. Call `.Wait()` to block or `.Unsubscribe()` to cancel

```go
observable := ro.Pipe2(
    ro.RangeWithInterval(0, 5, 1*time.Second),
    ro.Filter(func(x int) bool { return x%2 == 0 }),
    ro.Map(func(x int) string { return fmt.Sprintf("even-%d", x) }),
)

observable.Subscribe(ro.NewObserver(
    func(s string) { fmt.Println(s) },      // onNext
    func(err error) { log.Println(err) },    // onError
    func() { fmt.Println("Done!") },         // onComplete
))
// Output: "even-0", "even-2", "even-4", "Done!"

// Or collect synchronously:
values, err := ro.Collect(observable)
```

## Cold vs Hot Observables

**Cold** (default): each `.Subscribe()` starts a new independent execution. Safe and predictable — use by default.

**Hot**: multiple subscribers share a single execution. Use when the source is expensive (WebSocket, DB poll) or subscribers must see the same events.

| Convert with | Behavior |
| --- | --- |
| `Share()` | Cold → hot with reference counting. Last unsubscribe tears down |
| `ShareReplay(n)` | Same as Share + buffers last N values for late subscribers |
| `Connectable()` | Cold → hot, but waits for explicit `.Connect()` call |
| Subjects | Natively hot — call `.Send()`, `.Error()`, `.Complete()` directly |

| Subject | Constructor | Replay behavior |
| --- | --- | --- |
| `PublishSubject` | `NewPublishSubject[T]()` | None — late subscribers miss past events |
| `BehaviorSubject` | `NewBehaviorSubject[T](initial)` | Replays last value to new subscribers |
| `ReplaySubject` | `NewReplaySubject[T](bufferSize)` | Replays last N values |
| `AsyncSubject` | `NewAsyncSubject[T]()` | Emits only last value, only on complete |
| `UnicastSubject` | `NewUnicastSubject[T](bufferSize)` | Single subscriber only |

For subject details and hot observable patterns, see [Subjects Guide](./references/subjects-guide.md).

## Operator Quick Reference

| Category | Key operators | Purpose |
| --- | --- | --- |
| Creation | `Just`, `FromSlice`, `FromChannel`, `Range`, `Interval`, `Defer`, `Future` | Create observables from various sources |
| Transform | `Map`, `MapErr`, `FlatMap`, `Scan`, `Reduce`, `GroupBy` | Transform or accumulate stream values |
| Filter | `Filter`, `Take`, `TakeLast`, `Skip`, `Distinct`, `Find`, `First`, `Last` | Selectively emit values |
| Combine | `Merge`, `Concat`, `Zip2`–`Zip6`, `CombineLatest2`–`CombineLatest5`, `Race` | Merge multiple observables |
| Error | `Catch`, `OnErrorReturn`, `OnErrorResumeNextWith`, `Retry`, `RetryWithConfig` | Recover from errors |
| Timing | `Delay`, `DelayEach`, `Timeout`, `ThrottleTime`, `SampleTime`, `BufferWithTime` | Control emission timing |
| Side effect | `Tap`/`Do`, `TapOnNext`, `TapOnError`, `TapOnComplete` | Observe without altering stream |
| Terminal | `Collect`, `ToSlice`, `ToChannel`, `ToMap` | Consume stream into Go types |

Use typed `Pipe2`, `Pipe3` ... `Pipe25` for compile-time type safety across operator chains. The untyped `Pipe` uses `any` and loses type checking.

For the complete operator catalog (150+ operators with signatures), see [Operators Guide](./references/operators-guide.md).

## Common Mistakes

| Mistake | Why it fails | Fix |
| --- | --- | --- |
| Using `ro.OnNext()` without error handler | Errors are silently dropped — bugs hide in production | Use `ro.NewObserver(onNext, onError, onComplete)` with all 3 callbacks |
| Using untyped `Pipe()` instead of `Pipe2`/`Pipe3` | Loses compile-time type safety, errors surface at runtime | Use `Pipe2`, `Pipe3`...`Pipe25` for typed operator chains |
| Forgetting `.Unsubscribe()` on infinite streams | Goroutine leak — the observable runs forever | Use `TakeUntil(signal)`, context cancellation, or explicit `Unsubscribe()` |
| Using `Share()` when cold is sufficient | Unnecessary complexity, harder to reason about lifecycle | Use hot observables only when multiple consumers need the same stream |
| Using `samber/ro` for finite slice transforms | Stream overhead (goroutines, subscriptions) for a synchronous operation | Use `samber/lo` — it's simpler, faster, and purpose-built for slices |
| Not propagating context for cancellation | Streams ignore shutdown signals, causing resource leaks on termination | Chain `ContextWithTimeout` or `ThrowOnContextCancel` in the pipeline |

## Best Practices

1. **Always handle all three events** — use `NewObserver(onNext, onError, onComplete)`, not just `OnNext`. Unhandled errors cause silent data loss
2. **Use `Collect()` for synchronous consumption** — when the stream is finite and you need `[]T`, `Collect` blocks until complete and returns the slice + error
3. **Prefer typed Pipe functions** — `Pipe2`, `Pipe3`...`Pipe25` catch type mismatches at compile time. Reserve untyped `Pipe` for dynamic operator chains
4. **Bound infinite streams** — use `Take(n)`, `TakeUntil(signal)`, `Timeout(d)`, or context cancellation. Unbounded streams leak goroutines
5. **Use `Tap`/`Do` for observability** — log, trace, or meter emissions without altering the stream. Chain `TapOnError` for error monitoring
6. **Prefer `samber/lo` for simple transforms** — if the data is a finite slice and you need Map/Filter/Reduce, use `lo`. Reach for `ro` when data arrives over time, from multiple sources, or needs retry/timeout/backpressure

## Plugin Ecosystem

40+ plugins extend ro with domain-specific operators:

| Category | Plugins | Import path prefix |
| --- | --- | --- |
| Encoding | JSON, CSV, Base64, Gob | `plugins/encoding/...` |
| Network | HTTP, I/O, FSNotify | `plugins/http`, `plugins/io`, `plugins/fsnotify` |
| Scheduling | Cron, ICS | `plugins/cron`, `plugins/ics` |
| Observability | Zap, Slog, Zerolog, Logrus, Sentry, Oops | `plugins/observability/...`, `plugins/samber/oops` |
| Rate limiting | Native, Ulule | `plugins/ratelimit/...` |
| Data | Bytes, Strings, Sort, Strconv, Regexp, Template | `plugins/bytes`, `plugins/strings`, etc. |
| System | Process, Signal | `plugins/proc`, `plugins/signal` |

For the full plugin catalog with import paths and usage examples, see [Plugin Ecosystem](./references/plugin-ecosystem.md).

For real-world reactive patterns (retry+timeout, WebSocket fan-out, graceful shutdown, stream combination), see [Patterns](./references/patterns.md).

If you encounter a bug or unexpected behavior in samber/ro, open an issue at [github.com/samber/ro/issues](https://github.com/samber/ro/issues).

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for finite slice transforms (Map, Filter, Reduce, GroupBy) — use lo when data is already in a slice
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for monadic types (Option, Result, Either) that compose with ro pipelines
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for in-memory caching (also available as an ro plugin)
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for goroutine/channel patterns when reactive streams are overkill
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for monitoring reactive pipelines in production

## Sub-topic: golang-samber-slog

**Persona:** You are a Go logging architect. You design log pipelines where every record flows through the right handlers — sampling drops noise early, formatters strip PII before records leave the process, and routers send errors to Sentry while info goes to Loki.

# samber/slog-\*\*\*\* — Structured Logging Pipeline for Go

20+ composable `slog.Handler` packages for Go 1.21+. Three core pipeline libraries plus HTTP middlewares and backend sinks that all implement the standard `slog.Handler` interface.

**Official resources:**

- [github.com/samber/slog-multi](https://github.com/samber/slog-multi) — handler composition
- [github.com/samber/slog-sampling](https://github.com/samber/slog-sampling) — throughput control
- [github.com/samber/slog-formatter](https://github.com/samber/slog-formatter) — attribute transformation

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

## The Pipeline Model

Every samber/slog pipeline follows a canonical ordering. Records flow left to right — place sampling first to drop early and avoid wasting CPU on records that never reach a sink.

```
record → [Sampling] → [Pipe: trace/PII] → [Router] → [Sinks]
```

Order matters: sampling before formatting saves CPU. Formatting before routing ensures all sinks receive clean attributes. Reversing this wastes work on records that get dropped.

## Core Libraries

| Library | Purpose | Key constructors |
| --- | --- | --- |
| `slog-multi` | Handler composition | `Fanout`, `Router`, `FirstMatch`, `Failover`, `Pool`, `Pipe` |
| `slog-sampling` | Throughput control | `UniformSamplingOption`, `ThresholdSamplingOption`, `AbsoluteSamplingOption`, `CustomSamplingOption` |
| `slog-formatter` | Attribute transforms | `PIIFormatter`, `ErrorFormatter`, `FormatByType[T]`, `FormatByKey`, `FlattenFormatterMiddleware` |

## slog-multi — Handler Composition

Six composition patterns, each for a different routing need:

| Pattern | Behavior | Latency impact |
| --- | --- | --- |
| `Fanout(handlers...)` | Broadcast to all handlers sequentially | Sum of all handler latencies |
| `Router().Add(h, predicate).Handler()` | Route to ALL matching handlers | Sum of matching handlers |
| `Router().Add(...).FirstMatch().Handler()` | Route to FIRST match only | Single handler latency |
| `Failover()(handlers...)` | Try sequentially until one succeeds | Primary handler latency (happy path) |
| `Pool()(handlers...)` | Concurrent broadcast to all handlers | Max of all handler latencies |
| `Pipe(middlewares...).Handler(sink)` | Middleware chain before sink | Middleware overhead + sink |

```go
// Route errors to Sentry, all logs to stdout
logger := slog.New(
    slogmulti.Router().
        Add(sentryHandler, slogmulti.LevelIs(slog.LevelError)).
        Add(slog.NewJSONHandler(os.Stdout, nil)).
        Handler(),
)
```

Built-in predicates: `LevelIs`, `LevelIsNot`, `MessageIs`, `MessageIsNot`, `MessageContains`, `MessageNotContains`, `AttrValueIs`, `AttrKindIs`.

For full code examples of every pattern, see [Pipeline Patterns](references/pipeline-patterns.md).

## slog-sampling — Throughput Control

| Strategy | Behavior | Best for |
| --- | --- | --- |
| Uniform | Drop fixed % of all records | Dev/staging noise reduction |
| Threshold | Log first N per interval, then sample at rate R | Production — preserves initial visibility |
| Absolute | Cap at N records per interval globally | Hard cost control |
| Custom | User function returns sample rate per record | Level-aware or time-aware rules |

Sampling MUST be the outermost handler in the pipeline — placing it after formatting wastes CPU on records that get dropped.

```go
// Threshold: log first 10 per 5s, then 10% — errors always pass through via Router
logger := slog.New(
    slogmulti.
        Pipe(slogsampling.ThresholdSamplingOption{
            Tick: 5 * time.Second, Threshold: 10, Rate: 0.1,
        }.NewMiddleware()).
        Handler(innerHandler),
)
```

Matchers group similar records for deduplication: `MatchByLevel()`, `MatchByMessage()`, `MatchByLevelAndMessage()` (default), `MatchBySource()`, `MatchByAttribute(groups, key)`.

For strategy comparison and configuration details, see [Sampling Strategies](references/sampling-strategies.md).

## slog-formatter — Attribute Transformation

Apply as a `Pipe` middleware so all downstream handlers receive clean attributes.

```go
logger := slog.New(
    slogmulti.Pipe(slogformatter.NewFormatterMiddleware(
        slogformatter.PIIFormatter("user"),          // mask PII fields
        slogformatter.ErrorFormatter("error"),       // structured error info
        slogformatter.IPAddressFormatter("client"),  // mask IP addresses
    )).Handler(slog.NewJSONHandler(os.Stdout, nil)),
)
```

Key formatters: `PIIFormatter`, `ErrorFormatter`, `TimeFormatter`, `UnixTimestampFormatter`, `IPAddressFormatter`, `HTTPRequestFormatter`, `HTTPResponseFormatter`. Generic formatters: `FormatByType[T]`, `FormatByKey`, `FormatByKind`, `FormatByGroup`, `FormatByGroupKey`. Flatten nested attributes with `FlattenFormatterMiddleware`.

## HTTP Middlewares

Consistent pattern across frameworks: `router.Use(slogXXX.New(logger))`.

Available: `slog-gin`, `slog-echo`, `slog-fiber`, `slog-chi`, `slog-http` (net/http).

All share a `Config` struct with: `DefaultLevel`, `ClientErrorLevel`, `ServerErrorLevel`, `WithRequestBody`, `WithResponseBody`, `WithUserAgent`, `WithRequestID`, `WithTraceID`, `WithSpanID`, `Filters`.

```go
// Gin with filters — skip health checks
router.Use(sloggin.NewWithConfig(logger, sloggin.Config{
    DefaultLevel:     slog.LevelInfo,
    ClientErrorLevel: slog.LevelWarn,
    ServerErrorLevel: slog.LevelError,
    WithRequestBody:  true,
    Filters: []sloggin.Filter{
        sloggin.IgnorePath("/health", "/metrics"),
    },
}))
```

For framework-specific setup, see [HTTP Middlewares](references/http-middlewares.md).

## Backend Sinks

All follow the `Option{}.NewXxxHandler()` constructor pattern.

| Category     | Packages                                                   |
| ------------ | ---------------------------------------------------------- |
| Cloud        | `slog-datadog`, `slog-sentry`, `slog-loki`, `slog-graylog` |
| Messaging    | `slog-kafka`, `slog-fluentd`, `slog-logstash`, `slog-nats` |
| Notification | `slog-slack`, `slog-telegram`, `slog-webhook`              |
| Storage      | `slog-parquet`                                             |
| Bridges      | `slog-zap`, `slog-zerolog`, `slog-logrus`                  |

**Batch handlers require graceful shutdown** — `slog-datadog`, `slog-loki`, `slog-kafka`, and `slog-parquet` buffer records internally. Flush on shutdown (e.g., `handler.Stop(ctx)` for Datadog, `lokiClient.Stop()` for Loki, `writer.Close()` for Kafka) or buffered logs are lost.

For configuration examples and shutdown patterns, see [Backend Handlers](references/backend-handlers.md).

## Common Mistakes

| Mistake | Why it fails | Fix |
| --- | --- | --- |
| Sampling after formatting | Wastes CPU formatting records that get dropped | Place sampling as outermost handler |
| Fanout to many synchronous handlers | Blocks caller — latency is sum of all handlers | Use `Pool()` for concurrent dispatch |
| Missing shutdown flush on batch handlers | Buffered logs lost on shutdown | `defer handler.Stop(ctx)` (Datadog), `defer lokiClient.Stop()` (Loki), `defer writer.Close()` (Kafka) |
| Router without default/catch-all handler | Unmatched records silently dropped | Add a handler with no predicate as catch-all |
| `AttrFromContext` without HTTP middleware | Context has no request attributes to extract | Install `slog-gin`/`echo`/`fiber`/`chi` middleware first |
| Using `Pipe` with no middleware | No-op wrapper adding per-record overhead | Remove `Pipe()` if no middleware needed |

## Performance Warnings

- **Fanout latency** = sum of all handler latencies (sequential). With 5 handlers at 10ms each, every log call costs 50ms. Use `Pool()` to reduce to max(latencies)
- **Pipe middleware** adds per-record function call overhead — keep chains short (2-4 middlewares)
- **slog-formatter** processes attributes sequentially — many formatters compound. For hot-path attribute formatting, prefer implementing `slog.LogValuer` on your types instead
- **Benchmark** your pipeline with `go test -bench` before production deployment

**Diagnose:** measure per-record allocation and latency of your pipeline and identify which handler in the chain allocates most.

## Best Practices

1. **Sample first, format second, route last** — this canonical ordering minimizes wasted work and ensures all sinks see clean data
2. **Use Pipe for cross-cutting concerns** — trace ID injection and PII scrubbing belong in middleware, not per-handler logic
3. **Test pipelines with `slogmulti.NewHandleInlineHandler`** — assert on records reaching each stage without real sinks
4. **Use `AttrFromContext`** to propagate request-scoped attributes from HTTP middleware to all handlers
5. **Prefer Router over Fanout** when handlers need different record subsets — Router evaluates predicates and skips non-matching handlers

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for slog fundamentals (levels, context, handler setup, migration)
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for the log-or-return rule
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for PII handling in logs
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for structured error context with `samber/oops`

If you encounter a bug or unexpected behavior in any samber/slog-\* package, open an issue at the relevant repository (e.g., [slog-multi/issues](https://github.com/samber/slog-multi/issues), [slog-sampling/issues](https://github.com/samber/slog-sampling/issues)).

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

Levels align with [DREAD scoring](./references/threat-modeling.md).

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

For the full methodology with Go examples, DFD trust boundaries, DREAD scoring, and OWASP Top 10 mapping, see **[Threat Modeling Guide](./references/threat-modeling.md)**.

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

- **[Cryptography](./references/cryptography.md)** — Algorithms, key derivation, TLS configuration.
- **[Injection Vulnerabilities](./references/injection.md)** — SQL, command, template injection, XSS, SSRF.
- **[Filesystem Security](./references/filesystem.md)** — Path traversal, zip bombs, file permissions, symlinks.
- **[Network/Web Security](./references/network.md)** — SSRF, open redirects, HTTP headers, timing attacks, session fixation.
- **[Cookie Security](./references/cookies.md)** — Secure, HttpOnly, SameSite flags.
- **[Third-Party Data Leaks](./references/third-party.md)** — Analytics privacy risks, GDPR/CCPA compliance.
- **[Memory Safety](./references/memory-safety.md)** — Integer overflow, memory aliasing, `unsafe` usage.
- **[Secrets Management](./references/secrets.md)** — Hardcoded credentials, env vars, secret managers.
- **[Logging Security](./references/logging.md)** — PII in logs, log injection, sanitization.
- **[Threat Modeling Guide](./references/threat-modeling.md)** — STRIDE, DREAD scoring, trust boundaries, OWASP Top 10.
- **[Security Architecture](./references/architecture.md)** — Defense-in-depth, Zero Trust, auth patterns, rate limiting, anti-patterns.

## Code Review Checklist

For the full security review checklist organized by domain (input handling, database, crypto, web, auth, errors, dependencies, concurrency), see **[Security Review Checklist](./references/checklist.md)** — a comprehensive checklist for code review with coverage of all major vulnerability categories.

## Tooling & Verification

### Static Analysis & Linting

Security-relevant linters: `bodyclose`, `sqlclosecheck`, `nilerr`, `errcheck`, `govet`, `staticcheck`. See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for configuration and usage.

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

See **[Security Architecture](./references/architecture.md)** for detailed anti-patterns with Go code examples.

## Cross-References

See the I/O reference (`golang-io-web.md`), the Core reference (`golang-core.md`), the Ecosystem reference (`golang-ecosystem.md`), the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`)s.

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

1. **Profile before optimizing** — intuition about bottlenecks is wrong ~80% of the time. Use pprof to find actual hot spots (→ See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`))
2. **Allocation reduction yields the biggest ROI** — Go's GC is fast but not free. Reducing allocations per request often matters more than micro-optimizing CPU
3. **Document optimizations** — add code comments explaining why a pattern is faster, with benchmark numbers when available. Future readers need context to avoid reverting an "unnecessary" optimization

## Rule Out External Bottlenecks First

Before optimizing Go code, verify the bottleneck is in your process — if 90% of latency is a slow DB query or API call, reducing allocations won't help.

**Diagnose:** 1- `fgprof` — captures on-CPU and off-CPU (I/O wait) time; if off-CPU dominates, the bottleneck is external 2- `go tool pprof` (goroutine profile) — many goroutines blocked in `net.(*conn).Read` or `database/sql` = external wait 3- Distributed tracing (OpenTelemetry) — span breakdown shows which upstream is slow

**When external:** optimize that component instead — query tuning, caching, connection pools, circuit breakers (→ See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`), [Caching Patterns](references/caching.md)).

## Iterative Optimization Methodology

### The cycle: Define Goals → Benchmark → Diagnose → Improve → Benchmark

1. **Define your metric** — latency, throughput, memory, or CPU? Without a target, optimizations are random
2. **Write an atomic benchmark** — isolate one function per benchmark to avoid result contamination (→ See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`))
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
| Too many allocations | `alloc_objects` high in heap profile | [Memory optimization](references/memory.md) |
| CPU-bound hot loop | function dominates CPU profile | [CPU optimization](references/cpu.md) |
| GC pauses / OOM | high GC%, container limits | [Runtime tuning](references/runtime.md) |
| Network / I/O latency | goroutines blocked on I/O | [I/O & networking](references/io-networking.md) |
| Repeated expensive work | same computation/fetch multiple times | [Caching patterns](references/caching.md) |
| Wrong algorithm | O(n²) where O(n) exists | [Algorithmic complexity](references/caching.md#algorithmic-complexity) |
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

- [Memory Optimization](references/memory.md) — allocation patterns, backing array leaks, sync.Pool, struct alignment
- [CPU Optimization](references/cpu.md) — inlining, cache locality, false sharing, ILP, reflection avoidance
- [I/O & Networking](references/io-networking.md) — HTTP transport config, streaming, JSON performance, cgo, batch operations
- [Runtime Tuning](references/runtime.md) — GOGC, GOMEMLIMIT, GC diagnostics, GOMAXPROCS, PGO
- [Caching Patterns](references/caching.md) — algorithmic complexity, compiled patterns, singleflight, work avoidance
- [Production Observability](references/observability.md) — Prometheus metrics, PromQL queries, continuous profiling, alerting rules

## CI Regression Detection

Automate benchmark comparison in CI to catch regressions before they reach production. → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for `benchdiff` and `cob` setup.

## Cross-References

- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for benchmarking methodology, `benchstat`, and `b.Loop()` (Go 1.24+)
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for pprof workflow, escape analysis diagnostics, and performance debugging
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for slice/map preallocation and `strings.Builder`
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for worker pools, `sync.Pool` API, goroutine lifecycle, and lock contention
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for defer in loops, slice backing array aliasing
- → See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for connection pool tuning and batch processing
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for continuous profiling in production

## Sub-topic: golang-popular-libraries

**Persona:** You are a Go ecosystem expert. You know the library landscape well enough to recommend the simplest production-ready option — and to tell the developer when the standard library is already enough.

# Go Libraries and Frameworks Recommendations

## Core Philosophy

When recommending libraries, prioritize:

1. **Production-readiness** - Mature, well-maintained libraries with active communities
2. **Simplicity** - Go's philosophy favors simple, idiomatic solutions
3. **Performance** - Libraries that leverage Go's strengths (concurrency, compiled performance)
4. **Standard Library First** - SHOULD prefer stdlib when it covers the use case; only recommend external libs when they provide clear value

## Reference Catalogs

- [Standard Library - New & Experimental](./references/stdlib.md) — v2 packages, promoted x/exp packages, golang.org/x extensions
- [Libraries by Category](./references/libraries.md) — vetted third-party libraries for web, database, testing, logging, messaging, and more
- [Development Tools](./references/tools.md) — debugging, linting, testing, and dependency management tools

Find more libraries here: <https://github.com/avelino/awesome-go>

This skill is not exhaustive. Please refer to library documentation and code examples for more information.

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

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for adding, auditing, and managing dependencies
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for samber/do dependency injection details
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for samber/oops error handling details
- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for testify testing details
- → See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for gRPC implementation details

## Sub-topic: golang-documentation

**Persona:** You are a Go technical writer and API designer. You treat documentation as a first-class deliverable — accurate, example-driven, and written for the reader who has never seen this codebase before.

**Modes:**

- **Write mode** — generating or filling in missing documentation (doc comments, README, CONTRIBUTING, CHANGELOG, llms.txt). Work sequentially through the checklist in Step 2, or parallelize across packages/files using sub-agents.
- **Review mode** — auditing existing documentation for completeness, accuracy, and style. Use up to 5 parallel sub-agents: one per documentation layer (doc comments, README, CONTRIBUTING, CHANGELOG, library-specific extras).

# Go Documentation

Write documentation that serves both humans and AI agents. Good documentation makes code discoverable, understandable, and maintainable.

## Cross-References

See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for naming conventions in doc comments. See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for Example test functions. See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for where documentation files belong.

## Step 1: Detect Project Type

Before documenting, determine the project type — it changes what documentation is needed:

**Library** — no `main` package, meant to be imported by other projects:

- Focus on godoc comments, `ExampleXxx` functions, playground demos, pkg.go.dev rendering
- See [Library Documentation](./references/library.md)

**Application/CLI** — has `main` package, `cmd/` directory, produces a binary or Docker image:

- Focus on installation instructions, CLI help text, configuration docs
- See [Application Documentation](./references/application.md)

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

For the full comment format, deprecated markers, interface docs, and file-level comments, see **[Code Comments](./references/code-comments.md)** — how to document packages, functions, interfaces, and when to use `Deprecated:` markers and `BUG:` notes.

## Step 4: README Structure

README SHOULD follow this exact section order. Copy the template from [templates/README.md](./assets/templates/README.md):

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

For the full README guidance and application-specific sections, see [Project Docs](./references/project-docs.md#readme).

## Step 5: CONTRIBUTING & Changelog

**CONTRIBUTING.md** — Help contributors get started in under 10 minutes. Include: prerequisites, clone, build, test, PR process. If setup takes longer than 10 minutes, then you should improve the process: add a Makefile, docker-compose, or devcontainer to simplify it. See [Project Docs](./references/project-docs.md#contributingmd).

**Changelog** — Track changes using [Keep a Changelog](https://keepachangelog.com/) format or GitHub Releases. Copy the template from [templates/CHANGELOG.md](./assets/templates/CHANGELOG.md). See [Project Docs](./references/project-docs.md#changelog).

## Step 6: Library-Specific Documentation

For Go libraries, add these on top of the basics:

- **Go Playground demos** — create runnable demos and link them in doc comments with `// Play: https://go.dev/play/p/xxx`. Use the go-playground MCP tool when available to create and share playground URLs.
- **Example test functions** — write `func ExampleXxx()` in `_test.go` files. These are executable documentation verified by `go test`.
- **Generous code examples** — include multiple examples in doc comments showing common use cases.
- **godoc** — your doc comments render on [pkg.go.dev](https://pkg.go.dev). Use `go doc` locally to preview.
- **Documentation website** — for large libraries, consider Docusaurus or MkDocs Material with sections: Getting Started, Tutorial, How-to Guides, Reference, Explanation.
- **Register for discoverability** — add to Context7, DeepWiki, OpenDeep, zRead. Even for private libraries.

See [Library Documentation](./references/library.md) for details.

## Step 7: Application-Specific Documentation

For Go applications/CLIs:

- **Installation methods** — pre-built binaries (GoReleaser), `go install`, Docker images, Homebrew...
- **CLI help text** — make `--help` comprehensive; it's the primary documentation
- **Configuration docs** — document all env vars, config files, CLI flags

See [Application Documentation](./references/application.md) for details.

## Step 8: API Documentation

If your project exposes an API:

| API Style    | Format      | Tool                                         |
| ------------ | ----------- | -------------------------------------------- |
| REST/HTTP    | OpenAPI 3.x | swaggo/swag (auto-generate from annotations) |
| Event-driven | AsyncAPI    | Manual or code-gen                           |
| gRPC         | Protobuf    | buf, grpc-gateway                            |

Prefer auto-generation from code annotations when possible. See [Application Documentation](./references/application.md#api-documentation) for details.

## Step 9: AI-Friendly Documentation

Make your project consumable by AI agents:

- **llms.txt** — add a `llms.txt` file at the repository root. Copy the template from [templates/llms.txt](./assets/templates/llms.txt). This file gives LLMs a structured overview of your project.
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

See [Project Docs](./references/project-docs.md#delivery) for Dockerfile best practices and Homebrew tap setup.