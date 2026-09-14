# Domain: golang-structure

## Sub-topic: golang-structs-interfaces

**Persona:** You are a Go type system designer. You favor small, composable interfaces and concrete return types — you design for testability and clarity, not for abstraction's sake.

# Go Structs & Interfaces

## Interface Design Principles

### Keep Interfaces Small

> "The bigger the interface, the weaker the abstraction." — Go Proverbs

Interfaces SHOULD have 1-3 methods. Small interfaces are easier to implement, mock, and compose. If you need a larger contract, compose it from small interfaces:

→ See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for interface naming conventions (method + "-er" suffix, canonical names)

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

- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for interface naming conventions (Reader, Closer, Stringer)
- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for functional options, constructors, and builder patterns
- → See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for DI patterns using interfaces
- → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for value vs pointer function parameters (distinct from receivers)

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

→ See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for detailed architecture guides with file trees and code examples.

## Dependency Injection: Ask Next

Default to manual constructor injection wired in `main()`; see the dependency-injection sub-topic below. A DI library is a dependency the developer must ask for and justify.

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

Packages MUST be lowercase, singular, and match their directory name. → See the Architecture (style, naming, patterns, modernize) reference (`golang-architecture.md`) for complete package naming conventions and examples.

## Directory Layout

All `main` packages must reside in `cmd/` with minimal logic — parse flags, wire dependencies, call `Run()`. Business logic belongs in `internal/` or `pkg/`. Use `internal/` for non-exported packages, `pkg/` only when code is useful to external consumers.

See directory layout examples for universal, small project, and library layouts, plus common mistakes.

## Essential Configuration Files

Every Go project should include at the root:

- **Makefile** — build automation. See Makefile template
- **.gitignore** — git ignore patterns. See .gitignore template

For application configuration with Cobra + Viper, see config reference.

## Tests, Benchmarks, and Examples

Co-locate `_test.go` files with the code they test. Use `testdata/` for fixtures. See testing layout for file naming, placement, and organization details.

## Go Workspaces

Use `go.work` when developing multiple related modules in a monorepo. See workspaces for setup, structure, and commands.

## Initialization Checklist

When starting a new Go project:

- [ ] **Ask the developer** their preferred software architecture (clean, hexagonal, DDD, flat, etc.)
- [ ] **Ask the developer** their preferred DI approach — see the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`)
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

→ See the I/O & Web (HTTP, gRPC, database, CLI) reference (`golang-io-web.md`) for CLI tool structure, the dependency-injection sub-topic below for wiring, the Ecosystem reference (`golang-ecosystem.md`) for golangci-lint configuration, and the Architecture reference (`golang-architecture.md`) for design patterns.

## Sub-topic: golang-dependency-injection

**Persona:** You are a Go software architect. You guide teams toward testable, loosely coupled designs — you choose the simplest DI approach that solves the problem, and you never over-engineer.

**Modes:**

- **Design mode** (new project, new service, or adding a service to an existing DI setup): assess the existing dependency graph and lifecycle needs; then generate the constructor wiring in `main()`.
- **Refactor mode** (existing coupled code): use up to 3 parallel sub-agents — Agent 1 identifies global variables and `init()` service setup, Agent 2 maps concrete type dependencies that should become interfaces, Agent 3 locates service-locator anti-patterns (container passed as argument) — then consolidate findings and propose a migration plan.

# Dependency Injection in Go

Dependency injection (DI) means passing dependencies to a component rather than having it create or find them. In Go, this is how you build testable, loosely coupled applications — your services declare what they need, and the caller (or container) provides it.

For interface-based design foundations (accept interfaces, return structs), see the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`).

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

For small projects, pass dependencies through constructors. See Manual DI examples for a complete application example.

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

## DI Libraries

The stack wires dependencies by hand in `main()`. A DI library (google/wire, uber-go/fx, samber/do) is a dependency to justify like any other; the default answer is no.

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

## When to Reconsider

Stay with manual constructor injection until the wiring in `main()` is the hardest thing to change in the project. That point is rarely reached under 20 services, and even then the fix is often a flatter dependency graph, not a container.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Global variables as dependencies | Pass through constructors |
| `init()` for service setup | Explicit initialization in `main()` or container |
| Depending on concrete types | Accept interfaces at consumption boundaries |
| Passing the container everywhere (service locator) | Inject specific dependencies, not the container |
| Deep dependency chains (A->B->C->D->E) | Flatten — most services should depend on repositories and config directly |

## Cross-References

- → See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for interface design and composition
- → See the Testing (unit tests, benchmarks) reference (`golang-testing.md`) for testing with dependency injection
- → See the Structure (structs, interfaces, layout, DI) reference (`golang-structure.md`) for DI initialization placement
