# Domain: golang-testing

## Sub-topic: golang-testing

**Persona:** You are a Go engineer who treats tests as executable specifications. You write tests to constrain behavior, not to hit coverage targets.

**Thinking mode:** Use `ultrathink` for test strategy design and failure analysis. Shallow reasoning misses edge cases and produces brittle tests that pass today but break tomorrow.

**Modes:**

- **Write mode** — generating new tests for existing or new code. Work sequentially through the code under test; use `gotests` to scaffold table-driven tests, then enrich with edge cases and error paths.
- **Review mode** — reviewing a PR's test changes. Focus on the diff: check coverage of new behaviour, assertion quality, table-driven structure, and absence of flakiness patterns. Sequential.
- **Audit mode** — auditing an existing test suite for gaps, flakiness, or bad patterns (order-dependent tests, missing `t.Parallel()`, implementation-detail coupling). Launch up to 3 parallel sub-agents split by concern: (1) unit test quality and coverage gaps, (2) integration test isolation and build tags, (3) goroutine leaks and race conditions.
- **Debug mode** — a test is failing or flaky. Work sequentially: reproduce reliably, isolate the failing assertion, trace the root cause in production code or test setup.

# Go Testing Best Practices

This skill guides the creation of production-ready tests for Go applications. Follow these principles to write maintainable, fast, and reliable tests.

## Best Practices Summary

1. Table-driven tests MUST use named subtests -- every test case needs a `name` field passed to `t.Run`
2. Integration tests MUST use build tags (`//go:build integration`) to separate from unit tests
3. Tests MUST NOT depend on execution order -- each test MUST be independently runnable
4. Independent tests SHOULD use `t.Parallel()` when possible
5. NEVER test implementation details -- test observable behavior and public API contracts
6. Packages with goroutines SHOULD use `goleak.VerifyTestMain` in `TestMain` to detect goroutine leaks
7. Use the standard `testing` package; assertion and mock libraries are dependencies to justify, and the default is none
8. Mock interfaces, not concrete types
9. Keep unit tests fast (< 1ms), use build tags for integration tests
10. Run tests with `-race` before every commit
11. Include examples as executable documentation

## Test Structure and Organization

### File Conventions

```go
// package_test.go - tests in same package (white-box, access unexported)
package mypackage

// mypackage_test.go - tests in test package (black-box, public API only)
package mypackage_test
```

### Naming Conventions

```go
func TestAdd(t *testing.T) { ... }              // function test
func TestMyStruct_MyMethod(t *testing.T) { ... } // method test
func BenchmarkAdd(b *testing.B) { ... }          // benchmark
func ExampleAdd() { ... }                        // example
```

## Table-Driven Tests

Table-driven tests are the idiomatic Go way to test multiple scenarios. Always name each test case.

```go
func TestCalculatePrice(t *testing.T) {
    tests := []struct {
        name     string
        quantity int
        unitPrice float64
        expected  float64
    }{
        {
            name:      "single item",
            quantity:  1,
            unitPrice: 10.0,
            expected:  10.0,
        },
        {
            name:      "bulk discount - 100 items",
            quantity:  100,
            unitPrice: 10.0,
            expected:  900.0, // 10% discount
        },
        {
            name:      "zero quantity",
            quantity:  0,
            unitPrice: 10.0,
            expected:  0.0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := CalculatePrice(tt.quantity, tt.unitPrice)
            if got != tt.expected {
                t.Errorf("CalculatePrice(%d, %.2f) = %.2f, want %.2f",
                    tt.quantity, tt.unitPrice, got, tt.expected)
            }
        })
    }
}
```

## Unit Tests

Unit tests should be fast (< 1ms), isolated (no external dependencies), and deterministic.

## Testing HTTP Handlers

Use `httptest` for handler tests with table-driven patterns. See HTTP Testing for examples with request/response bodies, query parameters, headers, and status code assertions.

## Goroutine Leak Detection with goleak

Use `go.uber.org/goleak` to detect leaking goroutines, especially for concurrent code:

```go
import (
    "testing"
    "go.uber.org/goleak"
)

func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

To exclude specific goroutine stacks (for known leaks or library goroutines):

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m,
        goleak.IgnoreCurrent(),
    )
}
```

Or per-test:

```go
func TestWorkerPool(t *testing.T) {
    defer goleak.VerifyNone(t)
    // ... test code ...
}
```

## testing/synctest for Deterministic Goroutine Testing

> **Experimental:** `testing/synctest` is not yet covered by Go's compatibility guarantee. Its API may change in future releases. For a stable alternative, inject a `func() time.Time` clock (see Mocking below).

`testing/synctest` (Go 1.24+) provides deterministic time for concurrent code testing. Time advances only when all goroutines are blocked, making ordering predictable.

When to use `synctest` instead of real time:

- Testing concurrent code with time-based operations (time.Sleep, time.After, time.Ticker)
- When race conditions need to be reproducible
- When tests are flaky due to timing issues

```go
import (
    "testing"
    "time"
    "testing/synctest"
)

func TestChannelTimeout(t *testing.T) {
    synctest.Run(func(t *testing.T) {
        ch := make(chan int, 1)
        go func() {
            time.Sleep(50 * time.Millisecond)
            ch <- 42
        }()

        select {
        case v := <-ch:
            if v != 42 {
                t.Errorf("got %d, want 42", v)
            }
        case <-time.After(100 * time.Millisecond):
            t.Fatal("timeout occurred")
        }
    })
}
```

Key differences in `synctest`:

- `time.Sleep` advances synthetic time instantly when the goroutine blocks
- `time.After` fires when synthetic time reaches the duration
- All goroutines run to blocking points before time advances
- Test execution is deterministic and repeatable

## Test Timeouts

For tests that may hang, use a timeout helper that panics with caller location. See Helpers.

## Benchmarks

→ See the benchmark sub-topic below for `b.Loop()` (Go 1.24+), `benchstat`, and profiling from benchmarks.

Write benchmarks to measure performance and detect regressions:

```go
func BenchmarkStringConcatenation(b *testing.B) {
    b.Run("plus-operator", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            result := "a" + "b" + "c"
            _ = result
        }
    })

    b.Run("strings.Builder", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            var builder strings.Builder
            builder.WriteString("a")
            builder.WriteString("b")
            builder.WriteString("c")
            _ = builder.String()
        }
    })
}
```

Benchmarks with different input sizes:

```go
func BenchmarkFibonacci(b *testing.B) {
    sizes := []int{10, 20, 30}
    for _, size := range sizes {
        b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                Fibonacci(size)
            }
        })
    }
}
```

## Parallel Tests

Use `t.Parallel()` to run tests concurrently:

```go
func TestParallelOperations(t *testing.T) {
    tests := []struct {
        name string
        data []byte
    }{
        {"small data", make([]byte, 1024)},
        {"medium data", make([]byte, 1024*1024)},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
                result := Process(tt.data)
            is.NotNil(result)
        })
    }
}
```

## Fuzzing

Use fuzzing to find edge cases and bugs:

```go
func FuzzReverse(f *testing.F) {
    f.Add("hello")
    f.Add("")
    f.Add("a")

    f.Fuzz(func(t *testing.T, input string) {
        reversed := Reverse(input)
        doubleReversed := Reverse(reversed)
        if input != doubleReversed {
            t.Errorf("Reverse(Reverse(%q)) = %q, want %q", input, doubleReversed, input)
        }
    })
}
```

## Examples as Documentation

Examples are executable documentation verified by `go test`:

```go
func ExampleCalculatePrice() {
    price := CalculatePrice(100, 10.0)
    fmt.Printf("Price: %.2f\n", price)
    // Output: Price: 900.00
}

func ExampleCalculatePrice_singleItem() {
    price := CalculatePrice(1, 25.50)
    fmt.Printf("Price: %.2f\n", price)
    // Output: Price: 25.50
}
```

## Code Coverage

```bash
# Generate coverage file
go test -coverprofile=coverage.out ./...

# View coverage in HTML
go tool cover -html=coverage.out

# Coverage by function
go tool cover -func=coverage.out

# Total coverage percentage
go tool cover -func=coverage.out | grep total
```

## Integration Tests

Use build tags to separate integration tests from unit tests:

```go
//go:build integration

package mypackage

func TestDatabaseIntegration(t *testing.T) {
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    // Test real database operations
}
```

Run integration tests separately:

```bash
go test -tags=integration ./...
```

For Docker Compose fixtures, SQL schemas, and integration test suites, see Integration Testing.

## Mocking

Mock interfaces, not concrete types, and only at real boundaries (database, external API, clock, mailer). Define the interface where it is consumed, keep it small, and write the fake by hand — a struct with the fields the test needs to observe. No mock library: a hand-written fake is shorter, type-checked, and readable without documentation.

```go
// Consumer side declares only what it uses.
type UserStore interface {
    Get(ctx context.Context, id string) (*User, error)
}

// Test-side fake: state in, calls out.
type fakeUserStore struct {
    users map[string]*User
    calls int
}

func (f *fakeUserStore) Get(_ context.Context, id string) (*User, error) {
    f.calls++
    u, ok := f.users[id]
    if !ok {
        return nil, ErrNotFound
    }
    return u, nil
}

func TestUserService_GetUser(t *testing.T) {
    store := &fakeUserStore{users: map[string]*User{"1": {ID: "1", Name: "Alice"}}}
    svc := NewUserService(store)

    got, err := svc.GetUser(context.Background(), "1")
    if err != nil {
        t.Fatalf("GetUser: %v", err)
    }
    if got.Name != "Alice" {
        t.Errorf("got %q, want %q", got.Name, "Alice")
    }
    if store.calls != 1 {
        t.Errorf("store called %d times, want 1", store.calls)
    }
}
```

Time: take a `now func() time.Time` in the constructor (default `time.Now`) and pass a fixed function in tests. Randomness: take an `io.Reader` or a `*rand.Rand` seeded by the test. Filesystem: `fstest.MapFS` and `t.TempDir()`. HTTP clients: `httptest.NewServer`. A fake that needs configurable return sequences is a sign the interface is too wide.

## Enforce with Linters

Many test best practices are enforced automatically by linters: `thelper`, `paralleltest`. See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for configuration and usage.

## Cross-References

- -> See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) (testing.md) for database integration test patterns
- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for goroutine leak detection with goleak
- -> See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for paralleltest and thelper configuration

## Quick Reference

```bash
go test ./...                          # all tests
go test -run TestName ./...            # specific test by exact name
go test -run TestName/subtest ./...    # subtests within a test
go test -run 'Test(Add|Sub)' ./...     # multiple tests (regexp OR)
go test -run 'Test[A-Z]' ./...         # tests starting with capital letter
go test -run 'TestUser.*' ./...        # tests matching prefix
go test -run '.*Validation.*' ./...    # tests containing substring
go test -run TestName/. ./...          # all subtests of TestName
go test -run '/(unit|integration)' ./... # filter by subtest name
go test -race ./...                    # race detection
go test -cover ./...                   # coverage summary
go test -bench=. -benchmem ./...       # benchmarks
go test -fuzz=FuzzName ./...           # fuzzing
go test -tags=integration ./...        # integration tests
```

## Sub-topic: golang-benchmark

**Persona:** You are a Go performance measurement engineer. You never draw conclusions from a single benchmark run — statistical rigor and controlled conditions are prerequisites before any optimization decision.

**Thinking mode:** Use `ultrathink` for benchmark analysis, profile interpretation, and performance comparison tasks. Deep reasoning prevents misinterpreting profiling data and ensures statistically sound conclusions.

# Go Benchmarking & Performance Measurement

Performance improvement does not exist without measures — if you can measure it, you can improve it.

This skill covers the full measurement workflow: write a benchmark, run it, profile the result, and compare before/after with statistical rigor. For optimization patterns to apply after measurement, → See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`). For pprof setup on running services, → See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`).

## Writing Benchmarks

### `b.Loop()` (Go 1.24+) — preferred

`b.Loop()` prevents the compiler from optimizing away the code under test — without it, the compiler can detect dead results and eliminate them, producing misleadingly fast numbers. It also excludes setup code before the loop from timing automatically.

```go
func BenchmarkParse(b *testing.B) {
    data := loadFixture("large.json") // setup — excluded from timing
    for b.Loop() {
        Parse(data)  // compiler cannot eliminate this call
    }
}
```

Existing `for range b.N` benchmarks still work but should migrate to `b.Loop()` — the old pattern requires manual `b.ResetTimer()` and a package-level sink variable to prevent dead code elimination.

### Memory tracking

```go
func BenchmarkAlloc(b *testing.B) {
    b.ReportAllocs() // or run with -benchmem flag
    for b.Loop() {
        _ = make([]byte, 1024)
    }
}
```

`b.ReportMetric()` adds custom metrics (e.g., throughput):

```go
b.ReportMetric(float64(totalBytes)/b.Elapsed().Seconds(), "bytes/s")
```

### Sub-benchmarks and table-driven

```go
func BenchmarkEncode(b *testing.B) {
    for _, size := range []int{64, 256, 4096} {
        b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
            data := make([]byte, size)
            for b.Loop() {
                Encode(data)
            }
        })
    }
}
```

## Running Benchmarks

```bash
go test -bench=BenchmarkEncode -benchmem -count=10 ./pkg/... | tee bench.txt
```

| Flag                   | Purpose                                   |
| ---------------------- | ----------------------------------------- |
| `-bench=.`             | Run all benchmarks (regexp filter)        |
| `-benchmem`            | Report allocations (B/op, allocs/op)      |
| `-count=10`            | Run 10 times for statistical significance |
| `-benchtime=3s`        | Minimum time per benchmark (default 1s)   |
| `-cpu=1,2,4`           | Run with different GOMAXPROCS values      |
| `-cpuprofile=cpu.prof` | Write CPU profile                         |
| `-memprofile=mem.prof` | Write memory profile                      |
| `-trace=trace.out`     | Write execution trace                     |

**Output format:** `BenchmarkEncode/size=64-8  5000000  230.5 ns/op  128 B/op  2 allocs/op` — the `-8` suffix is GOMAXPROCS, `ns/op` is time per operation, `B/op` is bytes allocated per op, `allocs/op` is heap allocation count per op.

## Profiling from Benchmarks

Generate profiles directly from benchmark runs — no HTTP server needed:

```bash
# CPU profile
go test -bench=BenchmarkParse -cpuprofile=cpu.prof ./pkg/parser
go tool pprof cpu.prof

# Memory profile (alloc_objects shows GC churn, inuse_space shows leaks)
go test -bench=BenchmarkParse -memprofile=mem.prof ./pkg/parser
go tool pprof -alloc_objects mem.prof

# Execution trace
go test -bench=BenchmarkParse -trace=trace.out ./pkg/parser
go tool trace trace.out
```

For full pprof CLI reference (all commands, non-interactive mode, profile interpretation), see pprof Reference. For execution trace interpretation, see Trace Reference. For statistical comparison, see benchstat Reference.

## Reference Files

- **pprof Reference** — Interactive and non-interactive analysis of CPU, memory, and goroutine profiles. Full CLI commands, profile types (CPU vs alloc*objects vs inuse_space), web UI navigation, and interpretation patterns. Use this to dive deep into \_where* time and memory are being spent in your code.

- **benchstat Reference** — Statistical comparison of benchmark runs with rigorous confidence intervals and p-value tests. Covers output reading, filtering old benchmarks, interleaving results for visual clarity, and regression detection. Use this when you need to prove a change made a meaningful performance difference, not just a lucky run.

- **Trace Reference** — Execution tracer for understanding *when* and *why* code runs. Visualizes goroutine scheduling, garbage collection phases, network blocking, and custom span annotations. Use this when pprof (which shows *where* CPU goes) isn't enough — you need to see the timeline of what happened.

- **Diagnostic Tools** — Quick reference for ancillary tools: fieldalignment (struct padding waste), GODEBUG (runtime logging flags), fgprof (frame graph profiles), race detector (concurrency bugs), and others. Use this when you have a specific symptom and need a focused diagnostic — don't reach for pprof if a simpler tool already answers your question.

- **Compiler Analysis** — Low-level compiler optimization insights: escape analysis (when values move to the heap), inlining decisions (which function calls are eliminated), SSA dump (intermediate representation), and assembly output. Use this when benchmarks show allocations you didn't expect, or when you want to verify the compiler did what you intended.

- **Investigation Session** — Production performance troubleshooting workflow combining Prometheus runtime metrics (heap size, GC frequency, goroutine counts), PromQL queries to correlate metrics with code changes, runtime configuration flags (GODEBUG env vars to enable GC logging), and cost warnings (when you're hitting performance tax). Use this when production benchmarks look good but real traffic behaves differently.

- **Prometheus Go Metrics Reference** — Complete listing of Go runtime metrics actually exposed as Prometheus metrics by `prometheus/client_golang`. Covers 30 default metrics, 40+ optional metrics (Go 1.17+), process metrics, and common PromQL queries. Distinguishes between `runtime/metrics` (Go internal data) and Prometheus metrics (what you scrape from `/metrics`). Use this when setting up monitoring dashboards or writing PromQL queries for production alerts.

## Cross-References

- → See the Testing (unit tests, benchmarks) reference (`golang-testing.md`) for general testing practices
- → See `samber/cc-skills@promql-cli` skill for querying Prometheus runtime metrics in production to validate benchmark findings
