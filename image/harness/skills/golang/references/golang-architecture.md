# Domain: golang-architecture

## Sub-topic: golang-code-style

# Go Code Style

Style rules that require human judgment — linters handle formatting, this skill handles clarity. For naming and design patterns see the sub-topics below; for struct/interface design see the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`).

> "Clear is better than clever." — Go Proverbs

When ignoring a rule, add a comment to the code.

## Line Length & Breaking

No rigid line limit, but lines beyond ~120 characters MUST be broken. Break at **semantic boundaries**, not arbitrary column counts. Function calls with 4+ arguments MUST use one argument per line — even when the prompt asks for single-line code:

```go
// Good — each argument on its own line, closing paren separate
mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
    handleUsers(
        w,
        r,
        serviceName,
        cfg,
        logger,
        authMiddleware,
    )
})
```

When a function signature is too long, the real fix is often **fewer parameters** (use an options struct) rather than better line wrapping. For multi-line signatures, put each parameter on its own line.

## Variable Declarations

SHOULD use `:=` for non-zero values, `var` for zero-value initialization. The form signals intent: `var` means "this starts at zero."

```go
var count int              // zero value, set later
name := "default"          // non-zero, := is appropriate
var buf bytes.Buffer       // zero value is ready to use
```

### Slice & Map Initialization

Slices and maps MUST be initialized explicitly, never nil. Nil maps panic on write; nil slices serialize to `null` in JSON (vs `[]` for empty slices), surprising API consumers.

```go
users := []User{}                       // always initialized
m := map[string]int{}                   // always initialized
users := make([]User, 0, len(ids))      // preallocate when capacity is known
m := make(map[string]int, len(items))   // preallocate when size is known
```

Do not preallocate speculatively — `make([]T, 0, 1000)` wastes memory when the common case is 10 items.

### Composite Literals

Composite literals MUST use field names — positional fields break when the type adds or reorders fields:

```go
srv := &http.Server{
    Addr:         ":8080",
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
}
```

## Control Flow

### Reduce Nesting

Errors and edge cases MUST be handled first (early return). Keep the happy path at minimal indentation:

```go
func process(data []byte) (*Result, error) {
    if len(data) == 0 {
        return nil, errors.New("empty data")
    }

    parsed, err := parse(data)
    if err != nil {
        return nil, fmt.Errorf("parsing: %w", err)
    }

    return transform(parsed), nil
}
```

### Eliminate Unnecessary `else`

When the `if` body ends with `return`/`break`/`continue`, the `else` MUST be dropped. Use default-then-override for simple assignments — assign a default, then override with independent conditions or a `switch`:

```go
// Good — default-then-override with switch (cleanest for mutually exclusive overrides)
level := slog.LevelInfo
switch {
case debug:
    level = slog.LevelDebug
case verbose:
    level = slog.LevelWarn
}

// Bad — else-if chain hides that there's a default
if debug {
    level = slog.LevelDebug
} else if verbose {
    level = slog.LevelWarn
} else {
    level = slog.LevelInfo
}
```

### Complex Conditions & Init Scope

When an `if` condition has 3+ operands, MUST extract into named booleans — a wall of `||` is unreadable and hides business logic. Keep expensive checks inline for short-circuit benefit. Details

```go
// Good — named booleans make intent clear
isAdmin := user.Role == RoleAdmin
isOwner := resource.OwnerID == user.ID
isPublicVerified := resource.IsPublic && user.IsVerified
if isAdmin || isOwner || isPublicVerified || permissions.Contains(PermOverride) {
    allow()
}
```

Scope variables to `if` blocks when only needed for the check:

```go
if err := validate(input); err != nil {
    return err
}
```

### Switch Over If-Else Chains

When comparing the same variable multiple times, prefer `switch`:

```go
switch status {
case StatusActive:
    activate()
case StatusInactive:
    deactivate()
default:
    panic(fmt.Sprintf("unexpected status: %d", status))
}
```

## Function Design

- Functions SHOULD be **short and focused** — one function, one job.
- Functions SHOULD have **≤4 parameters**. Beyond that, use an options struct (see the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`)).
- **Parameter order**: `context.Context` first, then inputs, then output destinations.
- Naked returns help in very short functions (1-3 lines) where return values are obvious, but become confusing when readers must scroll to find what's returned — name returns explicitly in longer functions.

```go
func FetchUser(ctx context.Context, id string) (*User, error)
func SendEmail(ctx context.Context, msg EmailMessage) error  // grouped into struct
```

### Prefer `range` for Iteration

SHOULD use `range` over index-based loops. Use `range n` (Go 1.22+) for simple counting.

```go
for _, user := range users {
    process(user)
}
```

## Value vs Pointer Arguments

Pass small types (`string`, `int`, `bool`, `time.Time`) by value. Use pointers when mutating, for large structs (~128+ bytes), or when nil is meaningful. Details

## Code Organization Within Files

- **Group related declarations**: type, constructor, methods together
- **Order**: package doc, imports, constants, types, constructors, methods, helpers
- **One primary type per file** when it has significant methods
- **Blank imports** (`_ "pkg"`) register side effects (init functions). Restricting them to `main` and test packages makes side effects visible at the application root, not hidden in library code
- **Dot imports** pollute the namespace and make it impossible to tell where a name comes from — never use in library code
- **Unexport aggressively** — you can always export later; unexporting is a breaking change

## String Handling

Use `strconv` for simple conversions (faster), `fmt.Sprintf` for complex formatting. Use `%q` in error messages to make string boundaries visible. Use `strings.Builder` for loops, `+` for simple concatenation.

## Type Conversions

Prefer explicit, narrow conversions. Use generics over `any` when a concrete type will do:

```go
func Contains[T comparable](slice []T, target T) bool  // not []any
```

## Philosophy

- **"A little copying is better than a little dependency"**
- **Use `slices` and `maps` standard packages**; for filter/group-by/chunk, use `github.com/samber/lo`
- **"Reflection is never clear"** — avoid `reflect` unless necessary
- **Don't abstract prematurely** — extract when the pattern is stable
- **Minimize public surface** — every exported name is a commitment

## Parallelizing Code Style Reviews

When reviewing code style across a large codebase, use up to 5 parallel sub-agents (via the Agent tool), each targeting an independent style concern (e.g. control flow, function design, variable declarations, string handling, code organization).

## Enforce with Linters

Many rules are enforced automatically: `gofmt`, `gofumpt`, `goimports`, `gocritic`, `revive`, `wsl_v5`. → See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`).

## Cross-References

- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for identifier naming conventions
- → See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for pointer vs value receivers, interface design
- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for functional options, builders, constructors

## Sub-topic: golang-naming

# Go Naming Conventions

Go favors short, readable names. Capitalization controls visibility — uppercase is exported, lowercase is unexported. All identifiers MUST use MixedCaps, NEVER underscores.

> "Clear is better than clever." — Go Proverbs
>
> "Design the architecture, name the components, document the details." — Go Proverbs

To ignore a rule, just add a comment to the code.

## Quick Reference

| Element | Convention | Example |
| --- | --- | --- |
| Package | lowercase, single word, _test suffix OK for test files | `json`, `http`, `tabwriter`, `http_test` |
| File | lowercase, underscores OK | `user_handler.go` |
| Exported name | UpperCamelCase | `ReadAll`, `HTTPClient` |
| Unexported | lowerCamelCase | `parseToken`, `userCount` |
| Interface | method name + `-er` | `Reader`, `Closer`, `Stringer` |
| Struct | MixedCaps noun | `Request`, `FileHeader` |
| Constant | MixedCaps (not ALL_CAPS) | `MaxRetries`, `defaultTimeout` |
| Receiver | 1-2 letter abbreviation | `func (s *Server)`, `func (b *Buffer)` |
| Error variable | `Err` prefix | `ErrNotFound`, `ErrTimeout` |
| Error type | `Error` suffix | `PathError`, `SyntaxError` |
| Constructor | `New` (single type) or `NewTypeName` (multi-type) | `ring.New`, `http.NewRequest` |
| Boolean field | `is`, `has`, `can` prefix on **fields** and methods | `isReady`, `IsConnected()` |
| Test function | `Test` + function name | `TestParseToken` |
| Acronym | all caps or all lower | `URL`, `HTTPServer`, `xmlParser` |
| Variant: context | `WithContext` suffix | `FetchWithContext`, `QueryContext` |
| Variant: in-place | `In` suffix | `SortIn()`, `ReverseIn()` |
| Variant: error | `Must` prefix | `MustParse()`, `MustLoadConfig()` |
| Option func | `With` + field name | `WithPort()`, `WithLogger()` |
| Enum (iota) | type name prefix, zero-value = unknown | `StatusUnknown` at 0, `StatusReady` |
| Named return | descriptive, for docs only | `(n int, err error)` |
| Error string | lowercase (incl. acronyms), no punctuation | `"image: unknown format"`, `"invalid id"` |
| Import alias | short, only on collision | `mrand "math/rand"`, `pb "app/proto"` |
| Format func | `f` suffix | `Errorf`, `Wrapf`, `Logf` |
| Test table fields | `got`/`expected` prefixes | `input string`, `expected int` |

## MixedCaps

All Go identifiers MUST use `MixedCaps` (or `mixedCaps`). NEVER use underscores in identifiers — the only exceptions are test function subcases (`TestFoo_InvalidInput`), generated code, and OS/cgo interop. This is load-bearing, not cosmetic — Go's export mechanism relies on capitalization, and tooling assumes MixedCaps throughout.

```go
// ✓ Good
MaxPacketSize
userCount
parseHTTPResponse

// ✗ Bad — these conventions conflict with Go's export mechanism and tooling expectations
MAX_PACKET_SIZE   // C/Python style
max_packet_size   // snake_case
kMaxBufferSize    // Hungarian notation
```

## Avoid Stuttering

Go call sites always include the package name, so repeating it in the identifier wastes the reader's time — `http.HTTPClient` forces parsing "HTTP" twice. A name MUST NOT repeat information already present in the package name, type name, or surrounding context.

```go
// Good — clean at the call site
http.Client       // not http.HTTPClient
json.Decoder      // not json.JSONDecoder
user.New()        // not user.NewUser()
config.Parse()    // not config.ParseConfig()

// In package sqldb:
type Connection struct{}  // not DBConnection — "db" is already in the package name

// Anti-stutter applies to ALL exported types, not just the primary struct:
// In package dbpool:
type Pool struct{}        // not DBPool
type Status struct{}      // not PoolStatus — callers write dbpool.Status
type Option func(*Pool)   // not PoolOption
```

## Frequently Missed Conventions

These conventions are correct but non-obvious — they are the most common source of naming mistakes:

**Constructor naming:** When a package exports a single primary type, the constructor is `New()`, not `NewTypeName()`. This avoids stuttering — callers write `apiclient.New()` not `apiclient.NewClient()`. Use `NewTypeName()` only when a package has multiple constructible types (like `http.NewRequest`, `http.NewServeMux`).

**Boolean struct fields:** Unexported boolean fields MUST use `is`/`has`/`can` prefix — `isConnected`, `hasPermission`, not bare `connected` or `permission`. The exported getter keeps the prefix: `IsConnected() bool`. This reads naturally as a question and distinguishes booleans from other types.

**Error strings are fully lowercase — including acronyms.** Write `"invalid message id"` not `"invalid message ID"`, because error strings are often concatenated with other context (`fmt.Errorf("parsing token: %w", err)`) and mixed case looks wrong mid-sentence. Sentinel errors should include the package name as prefix: `errors.New("apiclient: not found")`.

**Enum zero values:** Always place an explicit `Unknown`/`Invalid` sentinel at iota position 0. A `var s Status` silently becomes 0 — if that maps to a real state like `StatusReady`, code can behave as if a status was deliberately chosen when it wasn't.

**Subtest names:** Table-driven test case names in `t.Run()` should be fully lowercase descriptive phrases: `"valid id"`, `"empty input"` — not `"valid ID"` or `"Valid Input"`.

## Detailed Categories

For complete rules, examples, and rationale, see:

- **Packages, Files & Import Aliasing** — Package naming (single word, lowercase, no plurals), file naming conventions, import alias patterns (only use on collision to avoid cognitive load), and directory structure.

- **Variables, Booleans, Receivers & Acronyms** — Scope-based naming (length matches scope: `i` for 3-line loops, longer names for package-level), single-letter receiver conventions (`s` for Server), acronym casing (URL not Url, HTTPServer not HttpServer), and boolean naming patterns (isReady, hasPrefix).

- **Functions, Methods & Options** — Getter/setter patterns (Go omits `Get` so `user.Name()` reads naturally), constructor conventions (`New` or `NewTypeName`), named returns (for documentation only), format function suffixes (`Errorf`, `Wrapf`), and functional options (`WithPort`, `WithLogger`).

- **Types, Constants & Errors** — Interface naming (`Reader`, `Closer` suffix with `-er`), struct naming (nouns, MixedCaps), constants (MixedCaps, not ALL_CAPS), enums (type name prefix like `StatusReady`), sentinel errors (`ErrNotFound` variables), error types (`PathError` suffix), and error message conventions (lowercase, no punctuation).

- **Test Naming** — Test function naming (`TestFunctionName`), table-driven test field conventions (`input`, `expected`), test helper naming, and subcase naming patterns.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| `ALL_CAPS` constants | Go reserves casing for visibility, not emphasis — use `MixedCaps` (`MaxRetries`) |
| `GetName()` getter | Go omits `Get` because `user.Name()` reads naturally at call sites. But `Is`/`Has`/`Can` prefixes are kept for boolean predicates: `IsHealthy() bool` not `Healthy() bool` |
| `Url`, `Http`, `Json` acronyms | Mixed-case acronyms create ambiguity (`HttpsUrl` — is it `Https+Url`?). Use all caps or all lower |
| `this` or `self` receiver | Go methods are called frequently — use 1-2 letter abbreviation (`s` for `Server`) to reduce visual noise |
| `util`, `helper` packages | These names say nothing about content — use specific names that describe the abstraction |
| `http.HTTPClient` stuttering | Package name is always present at call site — `http.Client` avoids reading "HTTP" twice |
| `user.NewUser()` constructor | Single primary type uses `New()` — `user.New()` avoids repeating the type name |
| `connected bool` field | Bare adjective is ambiguous — use `isConnected` so the field reads as a true/false question |
| `"invalid message ID"` error | Error strings must be fully lowercase including acronyms — `"invalid message id"` |
| `StatusReady` at iota 0 | Zero value should be a sentinel — `StatusUnknown` at 0 catches uninitialized values |
| `"not found"` error string | Sentinel errors should include the package name — `"mypackage: not found"` identifies the origin |
| `userSlice` type-in-name | Types encode implementation detail — `users` describes what it holds, not how |
| Inconsistent receiver names | Switching names across methods of the same type confuses readers — use one name consistently |
| `snake_case` identifiers | Underscores conflict with Go's MixedCaps convention and tooling expectations — use `mixedCaps` |
| Long names for short scopes | Name length should match scope — `i` is fine for a 3-line loop, `userIndex` is noise |
| Naming constants by value | Values change, roles don't — `DefaultPort` survives a port change, `Port8080` doesn't |
| `FetchCtx()` context variant | `WithContext` is the standard Go suffix — `FetchWithContext()` is instantly recognizable |
| `sort()` in-place but no `In` | Readers assume functions return new values. `SortIn()` signals mutation |
| `parse()` panicking on error | `MustParse()` warns callers that failure panics — surprises belong in the name |
| Mixing `With*`, `Set*`, `Use*` | Consistency across the codebase — `With*` is the Go convention for functional options |
| Plural package names | Go convention is singular (`net/url` not `net/urls`) — keeps import paths consistent |
| `Wrapf` without `f` suffix | The `f` suffix signals format-string semantics — `Wrapf`, `Errorf` tell callers to pass format args |
| Unnecessary import aliases | Aliases add cognitive load. Only alias on collision — `mrand "math/rand"` |
| Inconsistent concept names | Using `user`/`account`/`person` for the same concept forces readers to track synonyms — pick one name |

## Initialism and Acronym Edge Cases

The Quick Reference table says "all caps or all lower" for acronyms. These edge cases clarify ambiguous situations:

| Term | Exported | Unexported | Notes |
| --- | --- | --- | --- |
| `URL` | `URL`, `URLParser` | `url`, `urlParser` | Standard — all caps when exported |
| `HTTP` | `HTTPServer` | `httpServer` | Standard |
| `ID` | `ID`, `UserID` | `id`, `userID` | Two-letter acronym — still all caps |
| `gRPC` | `GRPCServer` | `grpcServer` | Brand uses mixed case, but Go normalizes to all-caps in exported names |
| `iOS` | N/A — avoid in exported names | `iosClient` | Platform name with unconventional casing. Avoid as an exported prefix; restructure to `ClientIOS` or use a different name |
| `BigQuery` | `BigQueryClient` | `bigQueryClient` | Multi-word brand name — preserve the brand's own casing (not `BIGQUERYClient`) |
| `YouTube` | `YouTubeAPI` | `youTubeAPI` | Same as BigQuery — respect the brand's internal casing |
| `OAuth` | `OAuthToken` | `oauthToken` | Treat as a brand name, not a pure acronym |

**Rule of thumb:** Pure acronyms (URL, HTTP, ID, API) → all caps or all lower. Brand names with internal casing (gRPC, iOS, BigQuery, OAuth) → preserve the brand's casing in the non-prefix position, normalize to all-caps only when it's the leading word in an exported name.

## Enforce with Linters

Many naming convention issues are caught automatically by linters: `revive`, `predeclared`, `misspell`, `errname`. See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for configuration and usage.

## Cross-References

- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for broader formatting and style decisions
- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for interface naming depth and receiver design

## Sub-topic: golang-design-patterns

**Persona:** You are a Go architect who values simplicity and explicitness. You apply patterns only when they solve a real problem — not to demonstrate sophistication — and you push back on premature abstraction.

**Modes:**

- **Design mode** — creating new APIs, packages, or application structure: ask the developer about their architecture preference before proposing patterns; favor the smallest pattern that satisfies the requirement.
- **Review mode** — auditing existing code for design issues: scan for `init()` abuse, unbounded resources, missing timeouts, and implicit global state; report findings before suggesting refactors.

# Go Design Patterns & Idioms

Idiomatic Go patterns for production-ready code. For error handling details see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`); for context propagation see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`); for struct/interface design see the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`).

## Best Practices Summary

1. Constructors SHOULD use **functional options** — they scale better as APIs evolve (one function per option, no breaking changes)
2. Functional options MUST **return an error** if validation can fail — catch bad config at construction, not at runtime
3. **Avoid `init()`** — runs implicitly, cannot return errors, makes testing unpredictable. Use explicit constructors
4. Enums SHOULD **start at 1** (or Unknown sentinel at 0) — Go's zero value silently passes as the first enum member
5. Error cases MUST be **handled first** with early return — keep happy path flat
6. **Panic is for bugs, not expected errors** — callers can handle returned errors; panics crash the process
7. **`defer Close()` immediately after opening** — later code changes can accidentally skip cleanup
8. **`runtime.AddCleanup`** over `runtime.SetFinalizer` — finalizers are unpredictable and can resurrect objects
9. Every external call SHOULD **have a timeout** — a slow upstream hangs your goroutine indefinitely
10. **Limit everything** (pool sizes, queue depths, buffers) — unbounded resources grow until they crash
11. Retry logic MUST **check context cancellation** between attempts
12. **Use `strings.Builder`** for concatenation in loops → see the Architecture reference (`golang-architecture.md`)
13. string vs []byte: **use `[]byte` for mutation and I/O**, `string` for display and keys — conversions allocate
14. Iterators (Go 1.23+): **use for lazy evaluation** — avoid loading everything into memory
15. **Stream large transfers** — loading millions of rows causes OOM; stream keeps memory constant
16. `//go:embed` for **static assets** — embeds at compile time, eliminates runtime file I/O errors
17. **Use `crypto/rand`** for keys/tokens — `math/rand` is predictable → see the Ecosystem reference (`golang-ecosystem.md`)
18. Regexp MUST be **compiled once at package level** — compilation is O(n) and allocates
19. Compile-time interface checks: **`var _ Interface = (*Type)(nil)`**
20. **A little recode > a big dependency** — each dep adds attack surface and maintenance burden
21. **Design for testability** — accept interfaces, inject dependencies

## Constructor Patterns: Functional Options vs Builder

### Functional Options (Preferred)

```go
type Server struct {
    addr         string
    readTimeout  time.Duration
    writeTimeout time.Duration
    maxConns     int
}

type Option func(*Server)

func WithReadTimeout(d time.Duration) Option {
    return func(s *Server) { s.readTimeout = d }
}

func WithWriteTimeout(d time.Duration) Option {
    return func(s *Server) { s.writeTimeout = d }
}

func WithMaxConns(n int) Option {
    return func(s *Server) { s.maxConns = n }
}

func NewServer(addr string, opts ...Option) *Server {
    // Default options
    s := &Server{
        addr:         addr,
        readTimeout:  5 * time.Second,
        writeTimeout: 10 * time.Second,
        maxConns:     100,
    }
    for _, opt := range opts {
        opt(s)
    }
    return s
}

// Usage
srv := NewServer(":8080",
    WithReadTimeout(30*time.Second),
    WithMaxConns(500),
)
```

Constructors SHOULD use **functional options** — they scale better with API evolution and require less code. Use builder pattern only if you need complex validation between configuration steps.

## Constructors & Initialization

### Avoid `init()` and Mutable Globals

`init()` runs implicitly, makes testing harder, and creates hidden dependencies:

- Multiple `init()` functions run in declaration order, across files in **filename alphabetical order** — fragile
- Cannot return errors — failures must panic or `log.Fatal`
- Runs before `main()` and tests — side effects make tests unpredictable

```go
// Bad — hidden global state
var db *sql.DB

func init() {
    var err error
    db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
}

// Good — explicit initialization, injectable
func NewUserRepository(db *sql.DB) *UserRepository {
    return &UserRepository{db: db}
}
```

### Enums: Start at 1

Zero values should represent invalid/unset state:

```go
type Status int

const (
    StatusUnknown Status = iota // 0 = invalid/unset
    StatusActive                // 1
    StatusInactive              // 2
    StatusSuspended             // 3
)
```

### Compile Regexp Once

```go
// Good — compiled once at package level
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func ValidateEmail(email string) bool {
    return emailRegex.MatchString(email)
}
```

### Use `//go:embed` for Static Assets

```go
import "embed"

//go:embed templates/*
var templateFS embed.FS

//go:embed version.txt
var version string
```

### Compile-Time Interface Checks

→ See the Architecture reference (`golang-architecture.md`) for the `var _ Interface = (*Type)(nil)` pattern.

## Error Flow Patterns

Error cases MUST be handled first with early return — keep the happy path at minimal indentation. → See the Architecture reference (`golang-architecture.md`) for the full pattern and examples.

### When to Panic vs Return Error

- **Return error**: network failures, file not found, invalid input — anything a caller can handle
- **Panic**: nil pointer in a place that should be impossible, violated invariant, `Must*` constructors used at init time
- **`.Close()` errors**: acceptable to not check — `defer f.Close()` is fine without error handling

## Data Handling

### string vs []byte vs []rune

| Type     | Default for | Use when                                            |
| -------- | ----------- | --------------------------------------------------- |
| `string` | Everything  | Immutable, safe, UTF-8                              |
| `[]byte` | I/O         | Writing to `io.Writer`, building strings, mutations |
| `[]rune` | Unicode ops | `len()` must mean characters, not bytes             |

Avoid repeated conversions — each one allocates. Stay in one type until you need the other.

### Iterators & Streaming for Large Data

Use iterators (Go 1.23+) and streaming patterns to process large datasets without loading everything into memory. For large transfers between services (e.g., 1M rows DB to HTTP), stream to prevent OOM.

For code examples, see Data Handling Patterns.

## Resource Management

`defer Close()` immediately after opening — don't wait, don't forget:

```go
f, err := os.Open(path)
if err != nil {
    return err
}
defer f.Close() // right here, not 50 lines later

rows, err := db.QueryContext(ctx, query)
if err != nil {
    return err
}
defer rows.Close()
```

For graceful shutdown, resource pools, and `runtime.AddCleanup`, see Resource Management.

## Resilience & Limits

### Timeout Every External Call

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

resp, err := httpClient.Do(req.WithContext(ctx))
```

### Retry & Context Checks

Retry logic MUST check `ctx.Err()` between attempts and use exponential/linear backoff via `select` on `ctx.Done()`. Long loops MUST check `ctx.Err()` periodically. → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`).

## Database Patterns

→ See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for sqlx/pgx, transactions, nullable columns, connection pools, repository interfaces, testing.

## Architecture

Ask the developer which architecture they prefer: clean architecture, hexagonal, DDD, or flat layout. Don't impose complex architecture on a small project.

Core principles regardless of architecture:

- **Keep domain pure** — no framework dependencies in the domain layer
- **Fail fast** — validate at boundaries, trust internal code
- **Make illegal states unrepresentable** — use types to enforce invariants
- **Respect 12-factor app** principles — → see the Architecture reference (`golang-architecture.md`)

## Detailed Guides

| Guide | Scope |
| --- | --- |
| Architecture Patterns | High-level principles, when each architecture fits |
| Clean Architecture | Use cases, dependency rule, layered adapters |
| Hexagonal Architecture | Ports and adapters, domain core isolation |
| Domain-Driven Design | Aggregates, value objects, bounded contexts |

## Code Philosophy

- **Avoid repetitive code** — but don't abstract prematurely
- **Minimize dependencies** — a little recode > a big dependency
- **Design for testability** — accept interfaces, inject dependencies, keep functions pure

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for data structure selection, internals, and container/ packages
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for error wrapping, sentinel errors, and the single handling rule
- → See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for interface design and composition
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for goroutine lifecycle and graceful shutdown
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for timeout and cancellation patterns
- → See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for architecture and directory structure

## Sub-topic: golang-modernize

<!-- markdownlint-disable ol-prefix -->

**Persona:** You are a Go modernization engineer. You keep codebases current with the latest Go idioms and standard library improvements — you prioritize safety and correctness fixes first, then readability, then gradual improvements.

**Modes:**

- **Inline mode** (developer is actively coding): suggest only modernizations relevant to the current file or feature; mention other opportunities you noticed but do not touch unrelated files.
- **Full-scan mode** (explicit `/golang-modernize` invocation): use up to 5 parallel sub-agents — Agent 1 scans deprecated packages and API replacements, Agent 2 scans language feature opportunities (range-over-int, min/max, any, iterators), Agent 3 scans standard library upgrades (slices, maps, cmp, slog), Agent 4 scans testing patterns (t.Context, b.Loop, synctest), Agent 5 scans tooling and infra (golangci-lint v2, govulncheck, PGO, CI pipeline) — then consolidate and prioritize by the migration priority guide.

# Go Code Modernization Guide

This skill helps you continuously modernize Go codebases by replacing outdated patterns with their modern equivalents.

**Scope**: This skill covers the last 3 years of Go modernization (Go 1.21 through Go 1.26, released 2023-2026). While this skill can be used for projects targeting Go 1.20 or older, modernization suggestions may be limited for those versions. For best results, consider upgrading the Go version first. Some older modernizations (e.g., `any` instead of `interface{}`, `errors.Is`/`errors.As`, `strings.Cut`) are included because they are still commonly missed, but many pre-1.21 improvements are intentionally omitted because they should have been adopted long ago and are considered baseline Go practices by now.

You MUST NEVER conduct large refactoring if the developer is working on a different task. But TRY TO CONVINCE your human it would improve the code quality.

## Workflow

When invoked:

1. **Check the project's `go.mod` or `go.work`** to determine the current Go version (`go` directive)
2. **Check the latest Go version** using the Go Version Changelogs table below and suggest upgrading if the project's `go.mod` is behind
3. **Read `.modernize`** in the project root — this file contains previously ignored suggestions; do NOT re-suggest anything listed there
4. **Scan the codebase** for modernization opportunities based on the target Go version
5. **Run `golangci-lint`** with the `modernize` linter if available
6. **Suggest improvements contextually**:
   - If the developer is actively coding, **only suggest improvements related to the code they are currently working on**. Do not refactor unrelated files. Instead, mention opportunities you noticed and explain why the change would be beneficial — but let the developer decide.
   - If invoked explicitly via `/golang-modernize`, scan and suggest across the entire codebase.
7. **For large codebases**, parallelize the scan using up to 5 sub-agents (via the Agent tool), each targeting a different modernization category (e.g. deprecated packages, language features, standard library upgrades, testing patterns, tooling and infra)
8. **Before suggesting a dependency update**, run `go mod tidy` and the test suite to verify compatibility. Ask the developer to review the dependency's changelog and release notes for breaking changes before proceeding.
9. **If the developer explicitly ignores a suggestion**, write a short memo to `.modernize` in the project root so it is not suggested again. Format: one line per ignored suggestion, with a short description.

### `.modernize` file format

```
# Ignored modernization suggestions
# Format: <date> <category> <description>
2026-01-15 slog-migration Team decided to keep zap for now
2026-02-01 math-rand-v2 Legacy module requires math/rand compatibility
```

## Go Version Changelogs

Reference the relevant changelog when suggesting a modernization:

| Version | Release       | Changelog                   |
| ------- | ------------- | --------------------------- |
| Go 1.21 | August 2023   | <https://go.dev/doc/go1.21> |
| Go 1.22 | February 2024 | <https://go.dev/doc/go1.22> |
| Go 1.23 | August 2024   | <https://go.dev/doc/go1.23> |
| Go 1.24 | February 2025 | <https://go.dev/doc/go1.24> |
| Go 1.25 | August 2025   | <https://go.dev/doc/go1.25> |
| Go 1.26 | February 2026 | <https://go.dev/doc/go1.26> |

For versions newer than Go 1.26, consult the official Go release notes.

When the project's `go.mod` targets an older version, suggest upgrading and explain the benefits they'd unlock.

## Using the modernize linter

The `modernize` linter (available since **golangci-lint v2.6.0**) automatically detects code that can be rewritten using newer Go features. It originates from `golang.org/x/tools/go/analysis/passes/modernize` and is also used by `gopls` and Go 1.26's rewritten `go fix` command. See the Ecosystem (observability, linters, security, performance) reference (`golang-ecosystem.md`) for configuration.

## Version-specific modernizations

For detailed before/after examples for each Go version (1.21–1.26) and general modernizations, see Go version modernizations.

## Tooling modernization

For govulncheck, PGO, and golangci-lint v2, see Tooling modernization.

## Deprecated Packages Migration

| Deprecated | Replacement | Since |
| --- | --- | --- |
| `math/rand` | `math/rand/v2` | Go 1.22 |
| `crypto/elliptic` (most functions) | `crypto/ecdh` | Go 1.21 |
| `reflect.SliceHeader`, `StringHeader` | `unsafe.Slice`, `unsafe.String` | Go 1.21 |
| `reflect.PtrTo` | `reflect.PointerTo` | Go 1.22 |
| `runtime.GOROOT()` | `go env GOROOT` | Go 1.24 |
| `runtime.SetFinalizer` | `runtime.AddCleanup` | Go 1.24 |
| `crypto/cipher.NewOFB`, `NewCFB*` | AEAD modes or `NewCTR` | Go 1.24 |
| `golang.org/x/crypto/sha3` | `crypto/sha3` | Go 1.24 |
| `golang.org/x/crypto/hkdf` | `crypto/hkdf` | Go 1.24 |
| `golang.org/x/crypto/pbkdf2` | `crypto/pbkdf2` | Go 1.24 |
| `testing/synctest.Run` | `testing/synctest.Test` | Go 1.25 |
| `crypto.EncryptPKCS1v15` | OAEP encryption | Go 1.26 |
| `net/http/httputil.ReverseProxy.Director` | `ReverseProxy.Rewrite` | Go 1.26 |

## Migration Priority Guide

When modernizing a codebase, prioritize changes by impact:

### High priority (safety and correctness)

1. Remove loop variable shadow copies _(Go 1.22+)_ — prevents subtle bugs
2. Replace `math/rand` with `math/rand/v2` _(Go 1.22+)_ — remove `rand.Seed` calls
3. Use `os.Root` for user-supplied file paths _(Go 1.24+)_ — prevents path traversal
4. Run `govulncheck` _(Go 1.22+)_ — catch known vulnerabilities
5. Use `errors.Is`/`errors.As` instead of direct comparison _(Go 1.13+)_
6. Migrate deprecated crypto packages _(Go 1.24+)_ — security critical

### Medium priority (readability and maintainability)

7. Replace `interface{}` with `any` _(Go 1.18+)_
8. Use `min`/`max` builtins _(Go 1.21+)_
9. Use `range` over int _(Go 1.22+)_
10. Use `slices` and `maps` packages _(Go 1.21+)_
11. Use `cmp.Or` for default values _(Go 1.22+)_
12. Use `sync.OnceValue`/`sync.OnceFunc` _(Go 1.21+)_
13. Use `sync.WaitGroup.Go` _(Go 1.25+)_
14. Use `t.Context()` in tests _(Go 1.24+)_
15. Use `b.Loop()` in benchmarks _(Go 1.24+)_

### Lower priority (gradual improvement)

16. Migrate to `slog` from third-party loggers _(Go 1.21+)_
17. Adopt iterators where they simplify code _(Go 1.23+)_
18. Replace `sort.Slice` with `slices.SortFunc` _(Go 1.21+)_
19. Use `strings.SplitSeq` and iterator variants _(Go 1.24+)_
20. Move tool deps to `go.mod` tool directives _(Go 1.24+)_
21. Enable PGO for production builds _(Go 1.21+)_
22. Upgrade to golangci-lint v2 with modernize linter _(golangci-lint v2.6.0+)_
23. Run `govulncheck ./...` before every release
25. Evaluate `encoding/json/v2` for new code _(Go 1.25+, experimental)_

## Related Skills

See the Core reference (`golang-core.md`) for the language features being modernized, the Testing reference (`golang-testing.md`) for `b.Loop()` and `testing/synctest`, and the Ecosystem reference (`golang-ecosystem.md`) for the `modernize` linter and dependency upgrades.
