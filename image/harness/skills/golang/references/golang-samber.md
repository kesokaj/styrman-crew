# Domain: golang-samber

Third-party helpers by samber. The operator's rule is stdlib first: reach for one of these only when the standard library is demonstrably not enough, and justify the dependency in the change entry.

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

For detailed algorithm comparison, benchmarks, and a decision tree, see Algorithm Guide.

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

For advanced patterns (revalidation, sharding, missing cache, monitoring setup), see Production Patterns.

For the complete API surface, see API Reference.

If you encounter a bug or unexpected behavior in samber/hot, open an issue at <https://github.com/samber/hot/issues>.

## Cross-References

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

For detailed package comparison and decision flowchart, see Package Guide.

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

For the complete function catalog (300+ functions), see API Reference.

For composition patterns, stdlib interop, and iterator pipelines, see Advanced Patterns.

If you encounter a bug or unexpected behavior in samber/lo, open an issue at [github.com/samber/lo/issues](https://github.com/samber/lo/issues).

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for choosing the right underlying data structure

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

For an introduction to functional programming concepts and why monads are valuable in Go, see Monads Guide.

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

For full API reference, see Option Reference.

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

For full API reference, see Result Reference.

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

For full API reference, see Either Reference.

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

For detailed pipeline API reference, see Pipelines Reference.

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

For advanced types (Future, IO, Task, State), see Advanced Types Reference.

If you encounter a bug or unexpected behavior in samber/mo, open an issue at <https://github.com/samber/mo/issues>.

## Cross-References

- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for idiomatic Go error handling patterns
- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for nil-safety and defensive Go coding
- -> See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for database access patterns
- -> See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for functional options and other Go patterns

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

For assertions, configuration, and additional logger examples, see Advanced patterns.

## References

- [github.com/samber/oops](https://github.com/samber/oops)
- [pkg.go.dev/github.com/samber/oops](https://pkg.go.dev/github.com/samber/oops)

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for general error handling patterns

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

For subject details and hot observable patterns, see Subjects Guide.

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

For the complete operator catalog (150+ operators with signatures), see Operators Guide.

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

For the full plugin catalog with import paths and usage examples, see Plugin Ecosystem.

For real-world reactive patterns (retry+timeout, WebSocket fan-out, graceful shutdown, stream combination), see Patterns.

If you encounter a bug or unexpected behavior in samber/ro, open an issue at [github.com/samber/ro/issues](https://github.com/samber/ro/issues).

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for goroutine/channel patterns when reactive streams are overkill

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

For full code examples of every pattern, see Pipeline Patterns.

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

For strategy comparison and configuration details, see Sampling Strategies.

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

For framework-specific setup, see HTTP Middlewares.

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

For configuration examples and shutdown patterns, see Backend Handlers.

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

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for the log-or-return rule

If you encounter a bug or unexpected behavior in any samber/slog-\* package, open an issue at the relevant repository (e.g., [slog-multi/issues](https://github.com/samber/slog-multi/issues), [slog-sampling/issues](https://github.com/samber/slog-sampling/issues)).
