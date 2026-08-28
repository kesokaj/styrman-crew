# Domain: golang-architecture



## Sub-topic: golang-code-style

# Go Code Style

Style rules that require human judgment — linters handle formatting, this skill handles clarity. For naming see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`); for design patterns see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`); for struct/interface design see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`).

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
- Functions SHOULD have **≤4 parameters**. Beyond that, use an options struct (see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`)).
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

Many rules are enforced automatically: `gofmt`, `gofumpt`, `goimports`, `gocritic`, `revive`, `wsl_v5`. → See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`).

## Cross-References

- → See the the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for identifier naming conventions
- → See the the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for pointer vs value receivers, interface design
- → See the the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for functional options, builders, constructors
- → See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for automated formatting enforcement

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

- **[Packages, Files & Import Aliasing](./references/packages-files.md)** — Package naming (single word, lowercase, no plurals), file naming conventions, import alias patterns (only use on collision to avoid cognitive load), and directory structure.

- **[Variables, Booleans, Receivers & Acronyms](./references/identifiers.md)** — Scope-based naming (length matches scope: `i` for 3-line loops, longer names for package-level), single-letter receiver conventions (`s` for Server), acronym casing (URL not Url, HTTPServer not HttpServer), and boolean naming patterns (isReady, hasPrefix).

- **[Functions, Methods & Options](./references/functions-methods.md)** — Getter/setter patterns (Go omits `Get` so `user.Name()` reads naturally), constructor conventions (`New` or `NewTypeName`), named returns (for documentation only), format function suffixes (`Errorf`, `Wrapf`), and functional options (`WithPort`, `WithLogger`).

- **[Types, Constants & Errors](./references/types-errors.md)** — Interface naming (`Reader`, `Closer` suffix with `-er`), struct naming (nouns, MixedCaps), constants (MixedCaps, not ALL_CAPS), enums (type name prefix like `StatusReady`), sentinel errors (`ErrNotFound` variables), error types (`PathError` suffix), and error message conventions (lowercase, no punctuation).

- **[Test Naming](./references/testing.md)** — Test function naming (`TestFunctionName`), table-driven test field conventions (`input`, `expected`), test helper naming, and subcase naming patterns.

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

Many naming convention issues are caught automatically by linters: `revive`, `predeclared`, `misspell`, `errname`. See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for configuration and usage.

## Cross-References

- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for broader formatting and style decisions
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for interface naming depth and receiver design
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for automated enforcement (revive, predeclared, misspell, errname)

## Sub-topic: golang-design-patterns

**Persona:** You are a Go architect who values simplicity and explicitness. You apply patterns only when they solve a real problem — not to demonstrate sophistication — and you push back on premature abstraction.

**Modes:**

- **Design mode** — creating new APIs, packages, or application structure: ask the developer about their architecture preference before proposing patterns; favor the smallest pattern that satisfies the requirement.
- **Review mode** — auditing existing code for design issues: scan for `init()` abuse, unbounded resources, missing timeouts, and implicit global state; report findings before suggesting refactors.

# Go Design Patterns & Idioms

Idiomatic Go patterns for production-ready code. For error handling details see the the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`); for context propagation see the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`); for struct/interface design see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`).

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

For code examples, see [Data Handling Patterns](references/data-handling.md).

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

For graceful shutdown, resource pools, and `runtime.AddCleanup`, see [Resource Management](references/resource-management.md).

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
| [Architecture Patterns](references/architecture.md) | High-level principles, when each architecture fits |
| [Clean Architecture](references/clean-architecture.md) | Use cases, dependency rule, layered adapters |
| [Hexagonal Architecture](references/hexagonal-architecture.md) | Ports and adapters, domain core isolation |
| [Domain-Driven Design](references/ddd.md) | Aggregates, value objects, bounded contexts |

## Code Philosophy

- **Avoid repetitive code** — but don't abstract prematurely
- **Minimize dependencies** — a little recode > a big dependency
- **Design for testability** — accept interfaces, inject dependencies, keep functions pure

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for data structure selection, internals, and container/ packages
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for error wrapping, sentinel errors, and the single handling rule
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for interface design and composition
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for goroutine lifecycle and graceful shutdown
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for timeout and cancellation patterns
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for architecture and directory structure

## Sub-topic: golang-modernize

<!-- markdownlint-disable ol-prefix -->

**Persona:** You are a Go modernization engineer. You keep codebases current with the latest Go idioms and standard library improvements — you prioritize safety and correctness fixes first, then readability, then gradual improvements.

**Modes:**

- **Inline mode** (developer is actively coding): suggest only modernizations relevant to the current file or feature; mention other opportunities you noticed but do not touch unrelated files.
- **Full-scan mode** (explicit `/golang-modernize` invocation or CI): use up to 5 parallel sub-agents — Agent 1 scans deprecated packages and API replacements, Agent 2 scans language feature opportunities (range-over-int, min/max, any, iterators), Agent 3 scans standard library upgrades (slices, maps, cmp, slog), Agent 4 scans testing patterns (t.Context, b.Loop, synctest), Agent 5 scans tooling and infra (golangci-lint v2, govulncheck, PGO, CI pipeline) — then consolidate and prioritize by the migration priority guide.

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
   - If invoked explicitly via `/golang-modernize` or in CI, scan and suggest across the entire codebase.
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

The `modernize` linter (available since **golangci-lint v2.6.0**) automatically detects code that can be rewritten using newer Go features. It originates from `golang.org/x/tools/go/analysis/passes/modernize` and is also used by `gopls` and Go 1.26's rewritten `go fix` command. See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for configuration.

## Version-specific modernizations

For detailed before/after examples for each Go version (1.21–1.26) and general modernizations, see [Go version modernizations](./references/versions.md).

## Tooling modernization

For CI tooling, govulncheck, PGO, golangci-lint v2, and AI-powered modernization pipelines, see [Tooling modernization](./references/tooling.md).

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
23. Add `govulncheck` to CI pipeline
24. Set up monthly modernization CI pipeline
25. Evaluate `encoding/json/v2` for new code _(Go 1.25+, experimental)_

## Related Skills

See the Core reference (`golang-core.md`), the Testing reference (`golang-testing.md`), the Ecosystem reference (`golang-ecosystem.md`), the Core reference (`golang-core.md`), the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`)s.

## Sub-topic: golang-structs-interfaces

**Persona:** You are a Go type system designer. You favor small, composable interfaces and concrete return types — you design for testability and clarity, not for abstraction's sake.

# Go Structs & Interfaces

## Interface Design Principles

### Keep Interfaces Small

> "The bigger the interface, the weaker the abstraction." — Go Proverbs

Interfaces SHOULD have 1-3 methods. Small interfaces are easier to implement, mock, and compose. If you need a larger contract, compose it from small interfaces:

→ See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for interface naming conventions (method + "-er" suffix, canonical names)

```go
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

// Composed from small interfaces
type ReadWriter interface {
    Reader
    Writer
}
```

Compose larger interfaces from smaller ones:

```go
type ReadWriteCloser interface {
    io.Reader
    io.Writer
    io.Closer
}
```

### Define Interfaces Where They're Consumed

Interfaces Belong to Consumers.

Interfaces MUST be defined where consumed, not where implemented. This keeps the consumer in control of the contract and avoids importing a package just for its interface.

```go
// package notification — defines only what it needs
type Sender interface {
    Send(to, body string) error
}

type Service struct {
    sender Sender
}
```

The `email` package exports a concrete `Client` struct — it doesn't need to know about `Sender`.

### Accept Interfaces, Return Structs

Functions SHOULD accept interface parameters for flexibility and return concrete types for clarity. Callers get full access to the returned type's fields and methods; consumers upstream can still assign the result to an interface variable if needed.

```go
// Good — accepts interface, returns concrete
func NewService(store UserStore) *Service { ... }

// BAD — NEVER return interfaces from constructors
func NewService(store UserStore) ServiceInterface { ... }
```

### Don't Create Interfaces Prematurely

> "Don't design with interfaces, discover them."

NEVER create interfaces prematurely — wait for 2+ implementations or a testability requirement. Premature interfaces add indirection without value. Start with concrete types; extract an interface when a second consumer or a test mock demands it.

```go
// Bad — premature interface with a single implementation
type UserRepository interface {
    FindByID(ctx context.Context, id string) (*User, error)
}
type userRepository struct { db *sql.DB }

// Good — start concrete, extract an interface later when needed
type UserRepository struct { db *sql.DB }
```

## Make the Zero Value Useful

Design structs so they work without explicit initialization. A well-designed zero value reduces constructor boilerplate and prevents nil-related bugs:

```go
// Good — zero value is ready to use
var buf bytes.Buffer
buf.WriteString("hello")

var mu sync.Mutex
mu.Lock()

// Bad — zero value is broken, requires constructor
type Registry struct {
    items map[string]Item // nil map, panics on write
}

// Good — lazy initialization guards the zero value
func (r *Registry) Register(name string, item Item) {
    if r.items == nil {
        r.items = make(map[string]Item)
    }
    r.items[name] = item
}
```

## Avoid `any` / `interface{}` When a Specific Type Will Do

Since Go 1.18+, MUST prefer generics over `any` for type-safe operations. Use `any` only at true boundaries where the type is genuinely unknown (e.g., JSON decoding, reflection):

```go
// Bad — loses type safety
func Contains(slice []any, target any) bool { ... }

// Good — generic, type-safe
func Contains[T comparable](slice []T, target T) bool { ... }
```

## Key Standard Library Interfaces

| Interface     | Package         | Method                                |
| ------------- | --------------- | ------------------------------------- |
| `Reader`      | `io`            | `Read(p []byte) (n int, err error)`   |
| `Writer`      | `io`            | `Write(p []byte) (n int, err error)`  |
| `Closer`      | `io`            | `Close() error`                       |
| `Stringer`    | `fmt`           | `String() string`                     |
| `error`       | builtin         | `Error() string`                      |
| `Handler`     | `net/http`      | `ServeHTTP(ResponseWriter, *Request)` |
| `Marshaler`   | `encoding/json` | `MarshalJSON() ([]byte, error)`       |
| `Unmarshaler` | `encoding/json` | `UnmarshalJSON([]byte) error`         |

Canonical method signatures MUST be honored — if your type has a `String()` method, it must match `fmt.Stringer`. Don't invent `ToString()` or `ReadData()`.

## Compile-Time Interface Check

Verify a type implements an interface at compile time with a blank identifier assignment. Place it near the type definition:

```go
var _ io.ReadWriter = (*MyBuffer)(nil)
```

This costs nothing at runtime. If `MyBuffer` ever stops satisfying `io.ReadWriter`, the build fails immediately.

## Type Assertions & Type Switches

### Safe Type Assertion

Type assertions MUST use the comma-ok form to avoid panics:

```go
// Good — safe
s, ok := val.(string)
if !ok {
    // handle
}

// Bad — panics if val is not a string
s := val.(string)
```

### Type Switch

Discover the dynamic type of an interface value:

```go
switch v := val.(type) {
case string:
    fmt.Println(v)
case int:
    fmt.Println(v * 2)
case io.Reader:
    io.Copy(os.Stdout, v)
default:
    fmt.Printf("unexpected type %T\n", v)
}
```

### Optional Behavior with Type Assertions

Check if a value supports additional capabilities without requiring them upfront:

```go
type Flusher interface {
    Flush() error
}

func writeData(w io.Writer, data []byte) error {
    if _, err := w.Write(data); err != nil {
        return err
    }
    // Flush only if the writer supports it
    if f, ok := w.(Flusher); ok {
        return f.Flush()
    }
    return nil
}
```

This pattern is used extensively in the standard library (e.g., `http.Flusher`, `io.ReaderFrom`).

## Struct & Interface Embedding

### Struct Embedding

Embedding promotes the inner type's methods and fields to the outer type — composition, not inheritance:

```go
type Logger struct {
    *slog.Logger
}

type Server struct {
    Logger
    addr string
}

// s.Info(...) works — promoted from slog.Logger through Logger
s := Server{Logger: Logger{slog.Default()}, addr: ":8080"}
s.Info("starting", "addr", s.addr)
```

The receiver of promoted methods is the _inner_ type, not the outer. The outer type can override by defining its own method with the same name.

### When to Embed vs Named Field

| Use | When |
| --- | --- |
| **Embed** | You want to promote the full API of the inner type — the outer type "is a" enhanced version |
| **Named field** | You only need the inner type internally — the outer type "has a" dependency |

```go
// Embed — Server exposes all http.Handler methods
type Server struct {
    http.Handler
}

// Named field — Server uses the store but doesn't expose its methods
type Server struct {
    store *DataStore
}
```

## Dependency Injection via Interfaces

Accept dependencies as interfaces in constructors. This decouples components and makes testing straightforward:

```go
type UserStore interface {
    FindByID(ctx context.Context, id string) (*User, error)
}

type UserService struct {
    store UserStore
}

func NewUserService(store UserStore) *UserService {
    return &UserService{store: store}
}
```

In tests, pass a mock or stub that satisfies `UserStore` — no real database needed.

## Struct Field Tags

Use field tags for serialization control. Exported fields in serialized structs MUST have field tags:

```go
type Order struct {
    ID        string    `json:"id"         db:"id"`
    UserID    string    `json:"user_id"    db:"user_id"`
    Total     float64   `json:"total"      db:"total"`
    Items     []Item    `json:"items"      db:"-"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    DeletedAt time.Time `json:"-"          db:"deleted_at"`
    Internal  string    `json:"-"          db:"-"`
}
```

| Directive               | Meaning                                     |
| ----------------------- | ------------------------------------------- |
| `json:"name"`           | Field name in JSON output                   |
| `json:"name,omitempty"` | Omit field if zero value                    |
| `json:"-"`              | Always exclude from JSON                    |
| `json:",string"`        | Encode number/bool as JSON string           |
| `db:"column"`           | Database column mapping (sqlx, etc.)        |
| `yaml:"name"`           | YAML field name                             |
| `xml:"name,attr"`       | XML attribute                               |
| `validate:"required"`   | Struct validation (go-playground/validator) |

## Pointer vs Value Receivers

| Use pointer `(s *Server)` | Use value `(s Server)` |
| --- | --- |
| Method modifies the receiver | Receiver is small and immutable |
| Receiver contains `sync.Mutex` or similar | Receiver is a basic type (int, string) |
| Receiver is a large struct | Method is a read-only accessor |
| Consistency: if any method uses a pointer, all should | Map and function values (already reference types) |

Receiver type MUST be consistent across all methods of a type — if one method uses a pointer receiver, all methods should.

## Preventing Struct Copies with `noCopy`

Some structs must never be copied after first use (e.g., those containing a mutex, a channel, or internal pointers). Embed a `noCopy` sentinel to make `go vet` catch accidental copies:

```go
// noCopy may be added to structs which must not be copied after first use.
// See https://pkg.go.dev/sync#noCopy
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type ConnPool struct {
    noCopy noCopy
    mu     sync.Mutex
    conns  []*Conn
}
```

`go vet` reports an error if a `ConnPool` value is copied (passed by value, assigned, etc.). This is the same technique the standard library uses for `sync.WaitGroup`, `sync.Mutex`, `strings.Builder`, and others.

Always pass these structs by pointer:

```go
// Good
func process(pool *ConnPool) { ... }

// Bad — go vet will flag this
func process(pool ConnPool) { ... }
```

## Cross-References

- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for interface naming conventions (Reader, Closer, Stringer)
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for functional options, constructors, and builder patterns
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for DI patterns using interfaces
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for value vs pointer function parameters (distinct from receivers)

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Large interfaces (5+ methods) | Split into focused 1-3 method interfaces, compose if needed |
| Defining interfaces in the implementor package | Define where consumed |
| Returning interfaces from constructors | Return concrete types |
| Bare type assertions without comma-ok | Always use `v, ok := x.(T)` |
| Embedding when you only need a few methods | Use a named field and delegate explicitly |
| Missing field tags on serialized structs | Tag all exported fields in marshaled types |
| Mixing pointer and value receivers on a type | Pick one and be consistent |
| Forgetting compile-time interface check | Add `var _ Interface = (*Type)(nil)` |
| Using `ToString()` instead of `String()` | Honor canonical method names |
| Premature interface with a single implementation | Start concrete, extract interface when needed |
| Nil map/slice in zero value struct | Use lazy initialization in methods |
| Using `any` for type-safe operations | Use generics (`[T comparable]`) instead |

## Sub-topic: golang-project-layout

**Persona:** You are a Go project architect. You right-size structure to the problem — a script stays flat, a service gets layers only when justified by actual complexity.

# Go Project Layout

## Architecture Decision: Ask First

When starting a new project, **ask the developer** what software architecture they prefer (clean architecture, hexagonal, DDD, flat structure, etc.). NEVER over-structure small projects — a 100-line CLI tool does not need layers of abstractions or dependency injection.

→ See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for detailed architecture guides with file trees and code examples.

## Dependency Injection: Ask Next

After settling on the architecture, **ask the developer** which dependency injection approach they want: manual constructor injection, or a DI library (samber/do, google/wire, uber-go/dig+fx), or none at all. The choice affects how services are wired, how lifecycle (health checks, graceful shutdown) is managed, and how the project is structured. See the the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for a full comparison and decision table.

## 12-Factor App

For applications (services, APIs, workers), follow [12-Factor App](https://12factor.net/) conventions: config via environment variables, logs to stdout, stateless processes, graceful shutdown, backing services as attached resources, and admin tasks as one-off commands (e.g., `cmd/migrate/`).

## Quick Start: Choose Your Project Type

| Project Type | Use When | Key Directories |
| --- | --- | --- |
| **CLI Tool** | Building a command-line application | `cmd/{name}/`, `internal/`, optional `pkg/` |
| **Library** | Creating reusable code for others | `pkg/{name}/`, `internal/` for private code |
| **Service** | HTTP API, microservice, or web app | `cmd/{service}/`, `internal/`, `api/`, `web/` |
| **Monorepo** | Multiple related packages/modules | `go.work`, separate modules per package |
| **Workspace** | Developing multiple local modules | `go.work`, replace directives |

## Module Naming Conventions

### Module Name (go.mod)

Your module path in `go.mod` should:

- **MUST match your repository URL**: `github.com/username/project-name`
- **Use lowercase only**: `github.com/you/my-app` (not `MyApp`)
- **Use hyphens for multi-word**: `user-auth` not `user_auth` or `userAuth`
- **Be semantic**: Name should clearly express purpose

**Examples:**

```go
// ✅ Good
module github.com/jdoe/payment-processor
module github.com/company/cli-tool

// ❌ Bad
module myproject
module github.com/jdoe/MyProject
module utils
```

### Package Naming

Packages MUST be lowercase, singular, and match their directory name. → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for complete package naming conventions and examples.

## Directory Layout

All `main` packages must reside in `cmd/` with minimal logic — parse flags, wire dependencies, call `Run()`. Business logic belongs in `internal/` or `pkg/`. Use `internal/` for non-exported packages, `pkg/` only when code is useful to external consumers.

See [directory layout examples](references/directory-layouts.md) for universal, small project, and library layouts, plus common mistakes.

## Essential Configuration Files

Every Go project should include at the root:

- **Makefile** — build automation. See [Makefile template](assets/Makefile)
- **.gitignore** — git ignore patterns. See [.gitignore template](assets/.gitignore)
- **.golangci.yml** — linter config. See the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for the recommended configuration

For application configuration with Cobra + Viper, see [config reference](references/config.md).

## Tests, Benchmarks, and Examples

Co-locate `_test.go` files with the code they test. Use `testdata/` for fixtures. See [testing layout](references/testing-layout.md) for file naming, placement, and organization details.

## Go Workspaces

Use `go.work` when developing multiple related modules in a monorepo. See [workspaces](references/workspaces.md) for setup, structure, and commands.

## Initialization Checklist

When starting a new Go project:

- [ ] **Ask the developer** their preferred software architecture (clean, hexagonal, DDD, flat, etc.)
- [ ] **Ask the developer** their preferred DI approach — see the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`)
- [ ] Decide project type (CLI, library, service, monorepo)
- [ ] Right-size the structure to the project scope
- [ ] Choose module name (matches repo URL, lowercase, hyphens)
- [ ] Run `go version` to detect the current go version
- [ ] Run `go mod init github.com/user/project-name`
- [ ] Create `cmd/{name}/main.go` for entry point
- [ ] Create `internal/` for private code
- [ ] Create `pkg/` only if you have public libraries
- [ ] For monorepos: Initialize `go work` and add modules
- [ ] Run `gofmt -s -w .` to ensure formatting
- [ ] Add `.gitignore` with `/vendor/` and binary patterns

## Related Skills

→ See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for CLI tool structure and Cobra/Viper patterns. → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for DI approach comparison and wiring. → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for golangci-lint configuration. → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for CI/CD pipeline setup. → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for architectural patterns.

## Sub-topic: golang-dependency-injection

**Persona:** You are a Go software architect. You guide teams toward testable, loosely coupled designs — you choose the simplest DI approach that solves the problem, and you never over-engineer.

**Modes:**

- **Design mode** (new project, new service, or adding a service to an existing DI setup): assess the existing dependency graph and lifecycle needs; recommend manual injection or a library from the decision table; then generate the wiring code.
- **Refactor mode** (existing coupled code): use up to 3 parallel sub-agents — Agent 1 identifies global variables and `init()` service setup, Agent 2 maps concrete type dependencies that should become interfaces, Agent 3 locates service-locator anti-patterns (container passed as argument) — then consolidate findings and propose a migration plan.

# Dependency Injection in Go

Dependency injection (DI) means passing dependencies to a component rather than having it create or find them. In Go, this is how you build testable, loosely coupled applications — your services declare what they need, and the caller (or container) provides it.

This skill is not exhaustive. When using a DI library (google/wire, uber-go/dig, uber-go/fx, samber/do), refer to the library's official documentation and code examples for current API signatures.

For interface-based design foundations (accept interfaces, return structs), see the the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`).

## Best Practices Summary

1. Dependencies MUST be injected via constructors — NEVER use global variables or `init()` for service setup
2. Small projects (< 10 services) SHOULD use manual constructor injection — no library needed
3. Interfaces MUST be defined where consumed, not where implemented — accept interfaces, return structs
4. NEVER use global registries or package-level service locators
5. The DI container MUST only exist at the composition root (`main()` or app startup) — NEVER pass the container as a dependency
6. **Prefer lazy initialization** — only create services when first requested
7. **Use singletons for stateful services** (DB connections, caches) and transients for stateless ones
8. **Mock at the interface boundary** — DI makes this trivial
9. **Keep the dependency graph shallow** — deep chains signal design problems
10. **Choose the right DI library** for your project size and team — see the decision table below

## Why Dependency Injection?

| Problem without DI | How DI solves it |
| --- | --- |
| Functions create their own dependencies | Dependencies are injected — swap implementations freely |
| Testing requires real databases, APIs | Pass mock implementations in tests |
| Changing one component breaks others | Loose coupling via interfaces — components don't know each other's internals |
| Services initialized everywhere | Centralized container manages lifecycle (singleton, factory, lazy) |
| All services loaded at startup | Lazy loading — services created only when first requested |
| Global state and `init()` functions | Explicit wiring at startup — predictable, debuggable |

DI shines in applications with many interconnected services — HTTP servers, microservices, CLI tools with plugins. For a small script with 2-3 functions, manual wiring is fine. Don't over-engineer.

## Manual Constructor Injection (No Library)

For small projects, pass dependencies through constructors. See [Manual DI examples](./references/manual-di.md) for a complete application example.

```go
// ✓ Good — explicit dependencies, testable
type UserService struct {
    db     UserStore
    mailer Mailer
    logger *slog.Logger
}

func NewUserService(db UserStore, mailer Mailer, logger *slog.Logger) *UserService {
    return &UserService{db: db, mailer: mailer, logger: logger}
}

// main.go — manual wiring
func main() {
    logger := slog.Default()
    db := postgres.NewUserStore(connStr)
    mailer := smtp.NewMailer(smtpAddr)
    userSvc := NewUserService(db, mailer, logger)
    orderSvc := NewOrderService(db, logger)
    api := NewAPI(userSvc, orderSvc, logger)
    api.ListenAndServe(":8080")
}
```

```go
// ✗ Bad — hardcoded dependencies, untestable
type UserService struct {
    db *sql.DB
}

func NewUserService() *UserService {
    db, _ := sql.Open("postgres", os.Getenv("DATABASE_URL")) // hidden dependency
    return &UserService{db: db}
}
```

Manual DI breaks down when:

- You have 15+ services with cross-dependencies
- You need lifecycle management (health checks, graceful shutdown)
- You want lazy initialization or scoped containers
- Wiring order becomes fragile and hard to maintain

## DI Library Comparison

Go has three main approaches to DI libraries:

- [google/wire examples](./references/google-wire.md) — Compile-time code generation
- [uber-go/dig + fx examples](./references/uber-dig-fx.md) — Reflection-based framework
- [samber/do examples](./references/samber-do.md) — Generics-based, no code generation

### Decision Table

| Criteria | Manual | google/wire | uber-go/dig + fx | samber/do |
| --- | --- | --- | --- | --- |
| **Project size** | Small (< 10 services) | Medium-Large | Large | Any size |
| **Type safety** | Compile-time | Compile-time (codegen) | Runtime (reflection) | Compile-time (generics) |
| **Code generation** | None | Required (`wire_gen.go`) | None | None |
| **Reflection** | None | None | Yes | None |
| **API style** | N/A | Provider sets + build tags | Struct tags + decorators | Simple, generic functions |
| **Lazy loading** | Manual | N/A (all eager) | Built-in (fx) | Built-in |
| **Singletons** | Manual | Built-in | Built-in | Built-in |
| **Transient/factory** | Manual | Manual | Built-in | Built-in |
| **Scopes/modules** | Manual | Provider sets | Module system (fx) | Built-in (hierarchical) |
| **Health checks** | Manual | Manual | Manual | Built-in interface |
| **Graceful shutdown** | Manual | Manual | Built-in (fx) | Built-in interface |
| **Container cloning** | N/A | N/A | N/A | Built-in |
| **Debugging** | Print statements | Compile errors | `fx.Visualize()` | `ExplainInjector()`, web interface |
| **Go version** | Any | Any | Any | 1.18+ (generics) |
| **Learning curve** | None | Medium | High | Low |

### Quick Comparison: Same App, Four Ways

The dependency graph: `Config -> Database -> UserStore -> UserService -> API`

**Manual**:

```go
cfg := NewConfig()
db := NewDatabase(cfg)
store := NewUserStore(db)
svc := NewUserService(store)
api := NewAPI(svc)
api.Run()
// No automatic shutdown, health checks, or lazy loading
```

**google/wire**:

```go
// wire.go — then run: wire ./...
func InitializeAPI() (*API, error) {
    wire.Build(NewConfig, NewDatabase, NewUserStore, NewUserService, NewAPI)
    return nil, nil
}
// No shutdown or health check support
```

**uber-go/fx**:

```go
app := fx.New(
    fx.Provide(NewConfig, NewDatabase, NewUserStore, NewUserService),
    fx.Invoke(func(api *API) { api.Run() }),
)
app.Run() // manages lifecycle, but reflection-based
```

**samber/do**:

```go
i := do.New()
do.Provide(i, NewConfig)
do.Provide(i, NewDatabase)    // auto shutdown + health check
do.Provide(i, NewUserStore)
do.Provide(i, NewUserService)
api := do.MustInvoke[*API](i)
api.Run()
// defer i.Shutdown() — handles all cleanup automatically
```

## Testing with DI

DI makes testing straightforward — inject mocks instead of real implementations:

```go
// Define a mock
type MockUserStore struct {
    users map[string]*User
}

func (m *MockUserStore) FindByID(ctx context.Context, id string) (*User, error) {
    u, ok := m.users[id]
    if !ok {
        return nil, ErrNotFound
    }
    return u, nil
}

// Test with manual injection
func TestUserService_GetUser(t *testing.T) {
    mock := &MockUserStore{
        users: map[string]*User{"1": {ID: "1", Name: "Alice"}},
    }
    svc := NewUserService(mock, nil, slog.Default())

    user, err := svc.GetUser(context.Background(), "1")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if user.Name != "Alice" {
        t.Errorf("got %q, want %q", user.Name, "Alice")
    }
}
```

### Testing with samber/do — Clone and Override

Container cloning creates an isolated copy where you override only the services you need to mock:

```go
func TestUserService_WithDo(t *testing.T) {
    // Create a test injector with mock implementation
    testInjector := do.New()

    // Provide the mock UserStore interface
    do.Override[UserStore](testInjector, &MockUserStore{
        users: map[string]*User{"1": {ID: "1", Name: "Alice"}},
    })

    // Provide other real services as needed
    do.Provide[*slog.Logger](testInjector, func(i *do.Injector) (*slog.Logger, error) {
        return slog.Default(), nil
    })

    svc := do.MustInvoke[*UserService](testInjector)
    user, err := svc.GetUser(context.Background(), "1")
    // ... assertions
}
```

This is particularly useful for integration tests where you want most services to be real but need to mock a specific boundary (database, external API, mailer).

## When to Adopt a DI Library

| Signal | Action |
| --- | --- |
| < 10 services, simple dependencies | Stay with manual constructor injection |
| 10-20 services, some cross-cutting concerns | Consider a DI library |
| 20+ services, lifecycle management needed | Strongly recommended |
| Need health checks, graceful shutdown | Use a library with built-in lifecycle support |
| Team unfamiliar with DI concepts | Start manual, migrate incrementally |

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Global variables as dependencies | Pass through constructors or DI container |
| `init()` for service setup | Explicit initialization in `main()` or container |
| Depending on concrete types | Accept interfaces at consumption boundaries |
| Passing the container everywhere (service locator) | Inject specific dependencies, not the container |
| Deep dependency chains (A->B->C->D->E) | Flatten — most services should depend on repositories and config directly |
| Creating a new container per request | One container per application; use scopes for request-level isolation |

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for detailed samber/do usage patterns
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for interface design and composition
- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for testing with dependency injection
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for DI initialization placement

## References

- [samber/do/v2 documentation](https://do.samber.dev) | [github.com/samber/do/v2](https://github.com/samber/do)
- [google/wire user guide](https://github.com/google/wire/blob/main/docs/guide.md)
- [uber-go/fx documentation](https://uber-go.github.io/fx/)
- [uber-go/dig](https://github.com/uber-go/dig)