# Domain: golang-core



## Sub-topic: golang-concurrency

**Persona:** You are a Go concurrency engineer. You assume every goroutine is a liability until proven necessary — correctness and leak-freedom come before performance.

**Modes:**

- **Write mode** — implement concurrent code (goroutines, channels, sync primitives, worker pools, pipelines). Follow the sequential instructions below.
- **Review mode** — reviewing a PR's concurrent code changes. Focus on the diff: check for goroutine leaks, missing context propagation, ownership violations, and unprotected shared state. Sequential.
- **Audit mode** — auditing existing concurrent code across a codebase. Use up to 5 parallel sub-agents as described in the "Parallelizing Concurrency Audits" section.

# Go Concurrency Best Practices

Go's concurrency model is built on goroutines and channels. Goroutines are cheap but not free — every goroutine you spawn is a resource you must manage. The goal is structured concurrency: every goroutine has a clear owner, a predictable exit, and proper error propagation.

## Core Principles

1. **Every goroutine must have a clear exit** — without a shutdown mechanism (context, done channel, WaitGroup), they leak and accumulate until the process crashes
2. **Share memory by communicating** — channels transfer ownership explicitly; mutexes protect shared state but make ownership implicit
3. **Send copies, not pointers** on channels — sending pointers creates invisible shared memory, defeating the purpose of channels
4. **Only the sender closes a channel** — closing from the receiver side panics if the sender writes after close
5. **Specify channel direction** (`chan<-`, `<-chan`) — the compiler prevents misuse at build time
6. **Default to unbuffered channels** — larger buffers mask backpressure; use them only with measured justification
7. **Always include `ctx.Done()` in select** — without it, goroutines leak after caller cancellation
8. **Never use `time.After` in loops** — each call creates a timer that lives until it fires, accumulating memory. Use `time.NewTimer` + `Reset`
9. **Track goroutine leaks in tests** with `go.uber.org/goleak`

For detailed channel/select code examples, see Channels and Select Patterns.

## Channel vs Mutex vs Atomic

| Scenario | Use | Why |
| --- | --- | --- |
| Passing data between goroutines | Channel | Communicates ownership transfer |
| Coordinating goroutine lifecycle | Channel + context | Clean shutdown with select |
| Protecting shared struct fields | `sync.Mutex` / `sync.RWMutex` | Simple critical sections |
| Simple counters, flags | `sync/atomic` | Lock-free, lower overhead |
| Many readers, few writers on a map | `sync.Map` | Optimized for read-heavy workloads. **Concurrent map read/write causes a hard crash** |
| Caching expensive computations | `sync.Once` / `singleflight` | Execute once or deduplicate |

## WaitGroup vs errgroup

| Need | Use | Why |
| --- | --- | --- |
| Wait for goroutines, errors not needed | `sync.WaitGroup` | Fire-and-forget |
| Wait + collect first error | `errgroup.Group` | Error propagation |
| Wait + cancel siblings on first error | `errgroup.WithContext` | Context cancellation on error |
| Wait + limit concurrency | `errgroup.SetLimit(n)` | Built-in worker pool |

## Sync Primitives Quick Reference

| Primitive | Use case | Key notes |
| --- | --- | --- |
| `sync.Mutex` | Protect shared state | Keep critical sections short; never hold across I/O |
| `sync.RWMutex` | Many readers, few writers | Never upgrade RLock to Lock (deadlock) |
| `sync/atomic` | Simple counters, flags | Prefer typed atomics (Go 1.19+): `atomic.Int64`, `atomic.Bool` |
| `sync.Map` | Concurrent map, read-heavy | No explicit locking; use `RWMutex`+map when writes dominate |
| `sync.Pool` | Reuse temporary objects | Always `Reset()` before `Put()`; reduces GC pressure |
| `sync.Once` | One-time initialization | Go 1.21+: `OnceFunc`, `OnceValue`, `OnceValues` |
| `sync.WaitGroup` | Wait for goroutine completion | `Add` before `go`; Go 1.24+: `wg.Go()` simplifies usage |
| `x/sync/singleflight` | Deduplicate concurrent calls | Cache stampede prevention |
| `x/sync/errgroup` | Goroutine group + errors | `SetLimit(n)` replaces hand-rolled worker pools |

For detailed examples and anti-patterns, see Sync Primitives Deep Dive.

## Concurrency Checklist

Before spawning a goroutine, answer:

- [ ] **How will it exit?** — context cancellation, channel close, or explicit signal
- [ ] **Can I signal it to stop?** — pass `context.Context` or done channel
- [ ] **Can I wait for it?** — `sync.WaitGroup` or `errgroup`
- [ ] **Who owns the channels?** — creator/sender owns and closes
- [ ] **Should this be synchronous instead?** — don't add concurrency without measured need

## Pipelines and Worker Pools

For pipeline patterns (fan-out/fan-in, bounded workers, generator chains, Go 1.23+ iterators, `samber/ro`), see Pipelines and Worker Pools.

## Parallelizing Concurrency Audits

When auditing concurrency across a large codebase, use up to 5 parallel sub-agents (Agent tool):

1. Find all goroutine spawns (`go func`, `go method`) and verify shutdown mechanisms
2. Search for mutable globals and shared state without synchronization
3. Audit channel usage — ownership, direction, closure, buffer sizes
4. Find `time.After` in loops, missing `ctx.Done()` in select, unbounded spawning
5. Check mutex usage, `sync.Map`, atomics, and thread-safety documentation

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Fire-and-forget goroutine | Provide stop mechanism (context, done channel) |
| Closing channel from receiver | Only the sender closes |
| `time.After` in hot loop | Reuse `time.NewTimer` + `Reset` |
| Missing `ctx.Done()` in select | Always select on context to allow cancellation |
| Unbounded goroutine spawning | Use `errgroup.SetLimit(n)` or semaphore |
| Sharing pointer via channel | Send copies or immutable values |
| `wg.Add` inside goroutine | Call `Add` before `go` — `Wait` may return early otherwise |
| Forgetting `-race` in CI | Always run `go test -race ./...` |
| Mutex held across I/O | Keep critical sections short |

## Cross-References

- -> See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for false sharing, cache-line padding, `sync.Pool` hot-path patterns
- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for cancellation propagation and timeout patterns
- -> See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for concurrent map access and race condition prevention
- -> See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for debugging goroutine leaks and deadlocks
- -> See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for graceful shutdown patterns

## References

- [Go Concurrency Patterns: Pipelines](https://go.dev/blog/pipelines)
- [Effective Go: Concurrency](https://go.dev/doc/effective_go#concurrency)

## Sub-topic: golang-context

# Go context.Context Best Practices

`context.Context` is Go's mechanism for propagating cancellation signals, deadlines, and request-scoped values across API boundaries and between goroutines. Think of it as the "session" of a request — it ties together every operation that belongs to the same unit of work.

## Best Practices Summary

1. The same context MUST be propagated through the entire request lifecycle: HTTP handler → service → DB → external APIs
2. `ctx` MUST be the first parameter, named `ctx context.Context`
3. NEVER store context in a struct — pass explicitly through function parameters
4. NEVER pass `nil` context — use `context.TODO()` if unsure
5. `cancel()` MUST always be deferred immediately after `WithCancel`/`WithTimeout`/`WithDeadline`
6. `context.Background()` MUST only be used at the top level (main, init, tests)
7. **Use `context.TODO()`** as a placeholder when you know a context is needed but don't have one yet
8. NEVER create a new `context.Background()` in the middle of a request path
9. Context value keys MUST be unexported types to prevent collisions
10. Context values MUST only carry request-scoped metadata — NEVER function parameters
11. **Use `context.WithoutCancel`** (Go 1.21+) when spawning background work that must outlive the parent request

## Creating Contexts

| Situation | Use |
| --- | --- |
| Entry point (main, init, test) | `context.Background()` |
| Function needs context but caller doesn't provide one yet | `context.TODO()` |
| Inside an HTTP handler | `r.Context()` |
| Need cancellation control | `context.WithCancel(parentCtx)` |
| Need a deadline/timeout | `context.WithTimeout(parentCtx, duration)` |

## Context Propagation: The Core Principle

The most important rule: **propagate the same context through the entire call chain**. When you propagate correctly, cancelling the parent context cancels all downstream work automatically.

```go
// ✗ Bad — creates a new context, breaking the chain
func (s *OrderService) Create(ctx context.Context, order Order) error {
    return s.db.ExecContext(context.Background(), "INSERT INTO orders ...", order.ID)
}

// ✓ Good — propagates the caller's context
func (s *OrderService) Create(ctx context.Context, order Order) error {
    return s.db.ExecContext(ctx, "INSERT INTO orders ...", order.ID)
}
```

## Deep Dives

- **Cancellation, Timeouts & Deadlines** — How cancellation propagates: `WithCancel` for manual cancellation, `WithTimeout` for automatic cancellation after a duration, `WithDeadline` for absolute time deadlines. Patterns for listening (`<-ctx.Done()`) in concurrent code, `AfterFunc` callbacks, and `WithoutCancel` for operations that must outlive their parent request (e.g., audit logs).

- **Context Values & Cross-Service Tracing** — Safe context value patterns: unexported key types to prevent namespace collisions, when to use context values (request ID, user ID) vs function parameters. Trace context propagation: OpenTelemetry trace headers, correlation IDs for log aggregation, and marshaling/unmarshaling context across service boundaries.

- **Context in HTTP Servers & Service Calls** — HTTP handler context: `r.Context()` for request-scoped cancellation, middleware integration, and propagating to services. HTTP client patterns: `NewRequestWithContext`, client timeouts, and retries with context awareness. Database operations: always use `*Context` variants (`QueryContext`, `ExecContext`) to respect deadlines.

## Cross-References

- → See the the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for goroutine cancellation patterns using context
- → See the the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for context-aware database operations (QueryContext, ExecContext)
- → See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for trace context propagation with OpenTelemetry
- → See the the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for timeout and resilience patterns

## Enforce with Linters

Many context pitfalls are caught automatically by linters: `govet`, `staticcheck`. → See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for configuration and usage.

## Sub-topic: golang-data-structures

**Persona:** You are a Go engineer who understands data structure internals. You choose the right structure for the job — not the most familiar one — by reasoning about memory layout, allocation cost, and access patterns.

# Go Data Structures

Built-in and standard library data structures: internals, correct usage, and selection guidance. For safety pitfalls (nil maps, append aliasing, defensive copies) see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`). For channels and sync primitives see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`). For string/byte/rune choice see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`).

## Best Practices Summary

1. **Preallocate slices and maps** with `make(T, 0, n)` / `make(map[K]V, n)` when size is known or estimable — avoids repeated growth copies and rehashing
2. **Arrays** SHOULD be preferred over slices only for fixed, compile-time-known sizes (hash digests, IPv4 addresses, matrix dimensions)
3. **NEVER rely on slice capacity growth timing** — the growth algorithm changed between Go versions and may change again; your code should not depend on when a new backing array is allocated
4. **Use `container/heap`** for priority queues, **`container/list`** only when frequent middle insertions are needed, **`container/ring`** for fixed-size circular buffers
5. **`strings.Builder`** MUST be preferred for building strings; **`bytes.Buffer`** MUST be preferred for bidirectional I/O (implements both `io.Reader` and `io.Writer`)
6. Generic data structures SHOULD use the **tightest constraint** possible — `comparable` for keys, custom interfaces for ordering
7. **`unsafe.Pointer`** MUST only follow the 6 valid conversion patterns from the Go spec — NEVER store in a `uintptr` variable across statements
8. **`weak.Pointer[T]`** (Go 1.24+) SHOULD be used for caches and canonicalization maps to allow GC to reclaim entries

## Slice Internals

A slice is a 3-word header: pointer, length, capacity. Multiple slices can share a backing array (→ see the Core reference (`golang-core.md`) for aliasing traps and the header diagram).

### Capacity Growth

- < 256 elements: capacity doubles
- > = 256 elements: grows by ~25% (`newcap += (newcap + 3*256) / 4`)
- Each growth copies the entire backing array — O(n)

### Preallocation

```go
// Exact size known
users := make([]User, 0, len(ids))

// Approximate size known
results := make([]Result, 0, estimatedCount)

// Pre-grow before bulk append (Go 1.21+)
s = slices.Grow(s, additionalNeeded)
```

### `slices` Package (Go 1.21+)

Key functions: `Sort`/`SortFunc`, `BinarySearch`, `Contains`, `Compact`, `Grow`. For `Clone`, `Equal`, `DeleteFunc` → see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`).

**Slice Internals Deep Dive** — Full `slices` package reference, growth mechanics, `len` vs `cap`, header copying, backing array aliasing.

## Map Internals

Maps are hash tables with 8-entry buckets and overflow chains. They are reference types — assigning a map copies the pointer, not the data.

### Preallocation

```go
m := make(map[string]*User, len(users)) // avoids rehashing during population
```

### `maps` Package Quick Reference (Go 1.21+)

| Function          | Purpose                      |
| ----------------- | ---------------------------- |
| `Collect` (1.23+) | Build map from iterator      |
| `Insert` (1.23+)  | Insert entries from iterator |
| `All` (1.23+)     | Iterator over all entries    |
| `Keys`, `Values`  | Iterators over keys/values   |

For `Clone`, `Equal`, sorted iteration → see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`).

**Map Internals Deep Dive** — How Go maps store and hash data, bucket overflow chains, why maps never shrink (and what to do about it), comparing map performance to alternatives.

## Arrays

Fixed-size, value types. Copied entirely on assignment. Use for compile-time-known sizes:

```go
type Digest [32]byte           // fixed-size, value type
var grid [3][3]int             // multi-dimensional
cache := map[[2]int]Result{}   // arrays are comparable — usable as map keys
```

Prefer slices for everything else — arrays cannot grow and pass by value (expensive for large sizes).

## container/ Standard Library

| Package | Data Structure | Best For |
| --- | --- | --- |
| `container/list` | Doubly-linked list | LRU caches, frequent middle insertion/removal |
| `container/heap` | Min-heap (priority queue) | Top-K, scheduling, Dijkstra |
| `container/ring` | Circular buffer | Rolling windows, round-robin |
| `bufio` | Buffered reader/writer/scanner | Efficient I/O with small reads/writes |

Container types use `any` (no type safety) — consider generic wrappers. **Container Patterns, bufio, and Examples** — When to use each container type, generic wrappers to add type safety, and `bufio` patterns for efficient I/O.

## strings.Builder vs bytes.Buffer

Use `strings.Builder` for pure string concatenation (avoids copy on `String()`), `bytes.Buffer` when you need `io.Reader` or byte manipulation. Both support `Grow(n)`. **Details and comparison**

## Generic Collections (Go 1.18+)

Use the tightest constraint possible. `comparable` for map keys, `cmp.Ordered` for sorting, custom interfaces for domain-specific ordering.

```go
type Set[T comparable] map[T]struct{}

func (s Set[T]) Add(v T)          { s[v] = struct{}{} }
func (s Set[T]) Contains(v T) bool { _, ok := s[v]; return ok }
```

**Writing Generic Data Structures** — Using Go 1.18+ generics for type-safe containers, understanding constraint satisfaction, and building domain-specific generic types.

## Pointer Types

| Type | Use Case | Zero Value |
| --- | --- | --- |
| `*T` | Normal indirection, mutation, optional values | `nil` |
| `unsafe.Pointer` | FFI, low-level memory layout (6 spec patterns only) | `nil` |
| `weak.Pointer[T]` (1.24+) | Caches, canonicalization, weak references | N/A |

**Pointer Types Deep Dive** — Normal pointers, `unsafe.Pointer` (the 6 valid spec patterns), and `weak.Pointer[T]` for GC-safe caches that don't prevent cleanup.

## Copy Semantics Quick Reference

| Type | Copy Behavior | Independence |
| --- | --- | --- |
| `int`, `float`, `bool`, `string` | Value (deep copy) | Fully independent |
| `array`, `struct` | Value (deep copy) | Fully independent |
| `slice` | Header copied, backing array shared | Use `slices.Clone` |
| `map` | Reference copied | Use `maps.Clone` |
| `channel` | Reference copied | Same channel |
| `*T` (pointer) | Address copied | Same underlying value |
| `interface` | Value copied (type + value pair) | Depends on held type |

## Third-Party Libraries

For advanced data structures (trees, sets, queues, stacks) beyond the standard library:

- **`emirpasic/gods`** — comprehensive collection library (trees, sets, lists, stacks, maps, queues)
- **`deckarep/golang-set`** — thread-safe and non-thread-safe set implementations
- **`gammazero/deque`** — fast double-ended queue

When using third-party libraries, refer to their official documentation and code examples for current API signatures. Context7 can help as a discoverability platform.

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for struct field alignment, memory layout optimization, and cache locality
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for nil map/slice pitfalls, append aliasing, defensive copying, `slices.Clone`/`Equal`
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for channels, `sync.Map`, `sync.Pool`, and all sync primitives
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for `string` vs `[]byte` vs `[]rune`, iterators, streaming
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for struct composition, embedding, and generics vs `any`
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for slice/map initialization style

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Growing a slice in a loop without preallocation | Each growth copies the entire backing array — O(n) per growth. Use `make([]T, 0, n)` or `slices.Grow` |
| Using `container/list` when a slice would suffice | Linked lists have poor cache locality (each node is a separate heap allocation). Benchmark first |
| `bytes.Buffer` for pure string building | Buffer's `String()` copies the underlying bytes. `strings.Builder` avoids this copy |
| `unsafe.Pointer` stored as `uintptr` across statements | GC can move the object between statements — the `uintptr` becomes a dangling reference |
| Large struct values in maps (copying overhead) | Map access copies the entire value. Use `map[K]*V` for large value types to avoid the copy |

## References

- [Go Data Structures (Russ Cox)](https://research.swtch.com/godata)
- [The Go Memory Model](https://go.dev/ref/mem)
- [Effective Go](https://go.dev/doc/effective_go)

## Sub-topic: golang-error-handling

**Persona:** You are a Go reliability engineer. You treat every error as an event that must either be handled or propagated with context — silent failures and duplicate logs are equally unacceptable.

**Modes:**

- **Coding mode** — writing new error handling code. Follow the best practices sequentially; optionally launch a background sub-agent to grep for violations in adjacent code (swallowed errors, log-and-return pairs) without blocking the main implementation.
- **Review mode** — reviewing a PR's error handling changes. Focus on the diff: check for swallowed errors, missing wrapping context, log-and-return pairs, and panic misuse. Sequential.
- **Audit mode** — auditing existing error handling across a codebase. Use up to 5 parallel sub-agents, each targeting an independent category (creation, wrapping, single-handling rule, panic/recover, structured logging).

# Go Error Handling Best Practices

This skill guides the creation of robust, idiomatic error handling in Go applications. Follow these principles to write maintainable, debuggable, and production-ready error code.

## Best Practices Summary

1. **Returned errors MUST always be checked** — NEVER discard with `_`
2. **Errors MUST be wrapped with context** using `fmt.Errorf("{context}: %w", err)`
3. **Error strings MUST be lowercase**, without trailing punctuation
4. **Use `%w` internally, `%v` at system boundaries** to control error chain exposure
5. **MUST use `errors.Is` and `errors.As`** instead of direct comparison or type assertion
6. **SHOULD use `errors.Join`** (Go 1.20+) to combine independent errors
7. **Errors MUST be either logged OR returned**, NEVER both (single handling rule)
8. **Use sentinel errors** for expected conditions, custom types for carrying data
9. **NEVER use `panic` for expected error conditions** — reserve for truly unrecoverable states
10. **SHOULD use `slog`** (Go 1.21+) for structured error logging — not `fmt.Println` or `log.Printf`
11. **Use `samber/oops`** for production errors needing stack traces, user/tenant context, or structured attributes
12. **Log HTTP requests** with structured middleware capturing method, path, status, and duration
13. **Use log levels** to indicate error severity
14. **Never expose technical errors to users** — translate internal errors to user-friendly messages, log technical details separately
15. **Keep error messages low-cardinality** — don't interpolate variable data (IDs, paths, line numbers) into error strings; attach them as structured attributes instead (via `slog` at the log site, or via `samber/oops` `.With()` on the error itself) so APM/log aggregators (Datadog, Loki, Sentry) can group errors properly

## Detailed Reference

- **Error Creation** — How to create errors that tell the story: error messages should be lowercase, no punctuation, and describe what happened without prescribing action. Covers sentinel errors (one-time preallocation for performance), custom error types (for carrying rich context), and the decision table for which to use when.

- **Error Wrapping and Inspection** — Why `fmt.Errorf("{context}: %w", err)` beats `fmt.Errorf("{context}: %v", err)` (chains vs concatenation). How to inspect chains with `errors.Is`/`errors.As` for type-safe error handling, and `errors.Join` for combining independent errors.

- **Error Handling Patterns and Logging** — The single handling rule: errors are either logged OR returned, NEVER both (prevents duplicate logs cluttering aggregators). Panic/recover design, `samber/oops` for production errors, and `slog` structured logging integration for APM tools.

## Parallelizing Error Handling Audits

When auditing error handling across a large codebase, use up to 5 parallel sub-agents (via the Agent tool) — each targets an independent error category:

- Sub-agent 1: Error creation — validate `errors.New`/`fmt.Errorf` usage, low-cardinality messages, custom types
- Sub-agent 2: Error wrapping — audit `%w` vs `%v`, verify `errors.Is`/`errors.As` patterns
- Sub-agent 3: Single handling rule — find log-and-return violations, swallowed errors, discarded errors (`_`)
- Sub-agent 4: Panic/recover — audit `panic` usage, verify recovery at goroutine boundaries
- Sub-agent 5: Structured logging — verify `slog` usage at error sites, check for PII in error messages

## Cross-References

- → See the Ecosystem reference (`golang-ecosystem.md`) for full samber/oops API, builder patterns, and logger integration
- → See the Ecosystem reference (`golang-ecosystem.md`) for structured logging setup, log levels, and request logging middleware
- → See the Core reference (`golang-core.md`) for nil interface trap and nil error comparison pitfalls
- → See the Architecture reference (`golang-architecture.md`) for error naming conventions (ErrNotFound, PathError)

## References

- [lmittmann/tint](https://github.com/lmittmann/tint)
- [samber/oops](https://github.com/samber/oops)
- [samber/slog-multi](https://github.com/samber/slog-multi)
- [samber/slog-sampling](https://github.com/samber/slog-sampling)
- [samber/slog-formatter](https://github.com/samber/slog-formatter)
- [samber/slog-http](https://github.com/samber/slog-http)
- [samber/slog-sentry](https://github.com/samber/slog-sentry)
- [log/slog package](https://pkg.go.dev/log/slog)

## Sub-topic: golang-safety

**Persona:** You are a defensive Go engineer. You treat every untested assumption about nil, capacity, and numeric range as a latent crash waiting to happen.

# Go Safety: Correctness & Defensive Coding

Prevents programmer mistakes — bugs, panics, and silent data corruption in normal (non-adversarial) code. Security handles attackers; safety handles ourselves.

## Best Practices Summary

1. **Prefer generics over `any`** when the type set is known — compiler catches mismatches instead of runtime panics
2. **Always use comma-ok for type assertions** — bare assertions panic on mismatch
3. **Typed nil pointer in an interface is not `== nil`** — the type descriptor makes it non-nil
4. **Writing to a nil map panics** — always initialize before use
5. **`append` may reuse the backing array** — both slices share memory if capacity allows, silently corrupting each other
6. **Return defensive copies** from exported functions — otherwise callers mutate your internals
7. **`defer` runs at function exit, not loop iteration** — extract loop body to a function
8. **Integer conversions truncate silently** — `int64` to `int32` wraps without error
9. **Float arithmetic is not exact** — use epsilon comparison or `math/big`
10. **Design useful zero values** — nil map fields panic on first write; use lazy init
11. **Use `sync.Once` for lazy init** — guarantees exactly-once even under concurrency

## Nil Safety

Nil-related panics are the most common crash in Go.

### The nil interface trap

Interfaces store (type, value). An interface is `nil` only when both are nil. Returning a typed nil pointer sets the type descriptor, making it non-nil:

```go
// ✗ Dangerous — interface{type: *MyHandler, value: nil} is not == nil
func getHandler() http.Handler {
    var h *MyHandler // nil pointer
    if !enabled {
        return h // interface{type: *MyHandler, value: nil} != nil
    }
    return h
}

// ✓ Good — return nil explicitly
func getHandler() http.Handler {
    if !enabled {
        return nil // interface{type: nil, value: nil} == nil
    }
    return &MyHandler{}
}
```

### Nil map, slice, and channel behavior

| Type | Read from nil | Write to nil | Len/Cap of nil | Range over nil |
| --- | --- | --- | --- | --- |
| Map | Zero value | **panic** | 0 | 0 iterations |
| Slice | **panic** (index) | **panic** (index) | 0 | 0 iterations |
| Channel | Blocks forever | Blocks forever | 0 | Blocks forever |

```go
// ✗ Bad — nil map panics on write
var m map[string]int
m["key"] = 1

// ✓ Good — initialize or lazy-init in methods
m := make(map[string]int)

func (r *Registry) Add(name string, val int) {
    if r.items == nil { r.items = make(map[string]int) }
    r.items[name] = val
}
```

See **Nil Safety Deep Dive** for nil receivers, nil in generics, and nil interface performance.

## Slice & Map Safety

### Slice aliasing — the append trap

`append` reuses the backing array if capacity allows. Both slices then share memory:

```go
// ✗ Dangerous — a and b share backing array
a := make([]int, 3, 5)
b := append(a, 4)
b[0] = 99 // also modifies a[0]

// ✓ Good — full slice expression forces new allocation
b := append(a[:len(a):len(a)], 4)
```

### Map concurrent access

Maps MUST NOT be accessed concurrently — → see the Core reference (`golang-core.md`) for sync primitives.

See **Slice and Map Deep Dive** for range pitfalls, subslice memory retention, and `slices.Clone`/`maps.Clone`.

## Numeric Safety

### Implicit type conversions truncate silently

```go
// ✗ Bad — silently wraps around if val > math.MaxInt32 (3B becomes -1.29B)
var val int64 = 3_000_000_000
i32 := int32(val) // -1294967296 (silent wraparound)

// ✓ Good — check before converting
if val > math.MaxInt32 || val < math.MinInt32 {
    return fmt.Errorf("value %d overflows int32", val)
}
i32 := int32(val)
```

### Float comparison

```go
// ✗ Bad — floating point arithmetic is not exact
0.1+0.2 == 0.3 // false

// ✓ Good — use epsilon comparison
const epsilon = 1e-9
math.Abs((0.1+0.2)-0.3) < epsilon // true
```

### Division by zero

Integer division by zero panics. Float division by zero produces `+Inf`, `-Inf`, or `NaN`.

```go
func avg(total, count int) (int, error) {
    if count == 0 {
        return 0, errors.New("division by zero")
    }
    return total / count, nil
}
```

For integer overflow as a security vulnerability, see the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) section.

## Resource Safety

### defer in loops — resource accumulation

`defer` runs at _function_ exit, not loop iteration. Resources accumulate until the function returns:

```go
// ✗ Bad — all files stay open until function returns
for _, path := range paths {
    f, _ := os.Open(path)
    defer f.Close() // deferred until function exits
    process(f)
}

// ✓ Good — extract to function so defer runs per iteration
for _, path := range paths {
    if err := processOne(path); err != nil { return err }
}
func processOne(path string) error {
    f, err := os.Open(path)
    if err != nil { return err }
    defer f.Close()
    return process(f)
}
```

### Goroutine leaks

→ See the Core reference (`golang-core.md`) for goroutine lifecycle and leak prevention.

## Immutability & Defensive Copying

Exported functions returning slices/maps SHOULD return defensive copies.

### Protecting struct internals

```go
// ✗ Bad — exported slice field, anyone can mutate
type Config struct {
    Hosts []string
}

// ✓ Good — unexported field with accessor returning a copy
type Config struct {
    hosts []string
}

func (c *Config) Hosts() []string {
    return slices.Clone(c.hosts)
}
```

## Initialization Safety

### Zero-value design

Design types so `var x MyType` is safe — prevents "forgot to initialize" bugs:

```go
var mu sync.Mutex   // ✓ usable at zero value
var buf bytes.Buffer // ✓ usable at zero value

// ✗ Bad — nil map panics on write
type Cache struct { data map[string]any }
```

### sync.Once for lazy initialization

```go
type DB struct {
    once sync.Once
    conn *sql.DB
}

func (db *DB) connection() *sql.DB {
    db.once.Do(func() {
        db.conn, _ = sql.Open("postgres", connStr)
    })
    return db.conn
}
```

### init() function pitfalls

→ See the Architecture reference (`golang-architecture.md`) for why init() should be avoided in favor of explicit constructors.

## Enforce with Linters

Many safety pitfalls are caught automatically by linters: `errcheck`, `forcetypeassert`, `nilerr`, `govet`, `staticcheck`. See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for configuration and usage.

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for concurrent access patterns and sync primitives
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for slice/map internals, capacity growth, and container/ packages
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for nil error interface trap
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for security-relevant safety issues (memory safety, integer overflow)
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for debugging panics and race conditions

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Bare type assertion `v := x.(T)` | Panics on type mismatch, crashing the program. Use `v, ok := x.(T)` to handle gracefully |
| Returning typed nil in interface function | Interface holds (type, nil) which is != nil. Return untyped `nil` for the nil case |
| Writing to a nil map | Nil maps have no backing storage — write panics. Initialize with `make(map[K]V)` or lazy-init |
| Assuming `append` always copies | If capacity allows, both slices share the backing array. Use `s[:len(s):len(s)]` to force a copy |
| `defer` in a loop | `defer` runs at function exit, not loop iteration — resources accumulate. Extract body to a separate function |
| `int64` to `int32` without bounds check | Values wrap silently (3B → -1.29B). Check against `math.MaxInt32`/`math.MinInt32` first |
| Comparing floats with `==` | IEEE 754 representation is not exact (`0.1+0.2 != 0.3`). Use `math.Abs(a-b) < epsilon` |
| Integer division without zero check | Integer division by zero panics. Guard with `if divisor == 0` before dividing |
| Returning internal slice/map reference | Callers can mutate your struct's internals through the shared backing array. Return a defensive copy |
| Multiple `init()` with ordering assumptions | `init()` execution order across files is unspecified. → See the Architecture reference (`golang-architecture.md`) — use explicit constructors |
| Blocking forever on nil channel | Nil channels block on both send and receive. Always initialize before use |