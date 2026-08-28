# Domain: golang-io-web



## Sub-topic: golang-http

**Persona:** You are a Go HTTP engineer. You build APIs that are observable, resilient, and correct — every handler has structured error handling, every request has a timeout, and every response has the right status code.

**Modes:**

- **Build mode** — implementing new HTTP handlers, middleware, or an API server from scratch. Follow the sections sequentially: handler signature → error handling → middleware → routing → JSON helpers → server config → graceful shutdown.
- **Review mode** — auditing existing HTTP handler code for correctness, security, and operability issues. Check the Common Mistakes table first, then verify error handling, middleware ordering, timeout configuration, and context propagation.

# Go HTTP Best Practices

Go's `net/http` standard library is production-ready. Prefer it over third-party routers unless you have a specific justification. Go 1.22+ enhanced the default `http.ServeMux` with method-based routing and path parameters, eliminating the most common reasons to reach for external routers.

This skill is not exhaustive. Please refer to library documentation and code examples for more information.

## Quick Reference

| Concern | Package / Tool |
| --- | --- |
| HTTP server & mux | `net/http` (stdlib) |
| Handler testing | `net/http/httptest` |
| JSON encode/decode | `encoding/json` |
| Request context | `r.Context()` |
| Path parameters (Go 1.22+) | `r.PathValue("name")` |
| Method routing (Go 1.22+) | `mux.HandleFunc("GET /users/{id}", handler)` |
| Middleware chaining | Manual composition or `alice` |
| Structured logging | `log/slog` |
| Reverse proxy | `net/http/httputil.ReverseProxy` with `.Rewrite` (Go 1.26+) |

## Handler Signatures

Go provides two handler interfaces. Prefer `http.HandlerFunc` for most handlers:

```go
// Standard handler — no error return, must handle errors inline
func handleGetUser(w http.ResponseWriter, r *http.Request) {
    // ...
}

// Register with method + path pattern (Go 1.22+)
mux.HandleFunc("GET /users/{id}", handleGetUser)
```

For complex APIs, use the error-aware handler pattern (see next section) to centralize error-to-response mapping.

## Error-Aware Handler Pattern

The standard `http.HandlerFunc` signature has no error return, forcing each handler to duplicate error-to-response logic. The error-aware pattern centralizes this:

```go
// appHandler is a handler that returns an error instead of writing error responses inline.
type appHandler func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler — it converts returned errors into HTTP responses
// and logs them at a single point (single handling rule).
func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if err := fn(w, r); err != nil {
        var apiErr *APIError
        if errors.As(err, &apiErr) {
            http.Error(w, apiErr.Message, apiErr.StatusCode)
        } else {
            http.Error(w, "internal error", http.StatusInternalServerError)
        }
        slog.ErrorContext(r.Context(), "request failed",
            "method", r.Method,
            "path", r.URL.Path,
            "error", err,
        )
    }
}
```

### Domain Error Types

```go
// APIError carries an HTTP status code for the error-aware handler wrapper.
type APIError struct {
    StatusCode int
    Message    string // user-facing message — never expose internal details
    Err        error  // wrapped internal error for logging
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) Unwrap() error { return e.Err }

// Convenience constructors
func NewBadRequestError(msg string) *APIError {
    return &APIError{StatusCode: http.StatusBadRequest, Message: msg}
}

func NewNotFoundError(msg string, err error) *APIError {
    return &APIError{StatusCode: http.StatusNotFound, Message: msg, Err: err}
}

func NewInternalError(err error) *APIError {
    return &APIError{StatusCode: http.StatusInternalServerError, Message: "internal error", Err: err}
}
```

### Handler Using the Pattern

```go
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) error {
    id := r.PathValue("id")
    if id == "" {
        return NewBadRequestError("missing user id")
    }

    user, err := h.service.FindByID(r.Context(), id)
    if err != nil {
        if errors.Is(err, ErrUserNotFound) {
            return NewNotFoundError("user not found", err)
        }
        return fmt.Errorf("getting user %s: %w", id, err)
    }

    return writeJSON(w, http.StatusOK, user)
}

// Register with the mux
mux.Handle("GET /users/{id}", appHandler(h.GetUser))
```

**Rules applied:**
- `r.Context()` propagated to service layer — context flows through the entire request lifecycle
- `errors.Is` for sentinel comparison — walks the error chain correctly
- Error wrapping with `fmt.Errorf("context: %w", err)` — adds call-site context
- Single handling rule — errors are logged in `ServeHTTP`, never in the handler itself
- Never expose internal errors to users — `APIError.Message` is user-facing

## Middleware

Middleware wraps handlers to add cross-cutting concerns. In Go, middleware is a function that takes an `http.Handler` and returns an `http.Handler`:

```go
func middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // before
        next.ServeHTTP(w, r)
        // after
    })
}
```

### Middleware Ordering

Order matters — outermost middleware runs first:

```go
// Applied bottom-up: Recovery → RequestID → Logging → handler
handler := recoveryMiddleware(
    requestIDMiddleware(
        loggingMiddleware(mux),
    ),
)
```

### Recovery Middleware

Catches panics in handlers and converts them to 500 responses instead of crashing the server. MUST be the outermost middleware:

```go
func recoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                slog.ErrorContext(r.Context(), "panic recovered",
                    "panic", rec,
                    "method", r.Method,
                    "path", r.URL.Path,
                    "stack", string(debug.Stack()),
                )
                http.Error(w, "internal error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

### Request ID Middleware

Injects a unique request ID into the context for log correlation. Extract from the `X-Request-ID` header if present (load balancers and API gateways often set this), otherwise generate one:

```go
type contextKey string

const requestIDKey contextKey = "request_id"

func requestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get("X-Request-ID")
        if id == "" {
            id = uuid.NewString() // or use a lighter ID generator
        }
        ctx := context.WithValue(r.Context(), requestIDKey, id)
        w.Header().Set("X-Request-ID", id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
    id, _ := ctx.Value(requestIDKey).(string)
    return id
}
```

### Logging Middleware

Logs every request with method, path, status, duration, and request ID. Uses a `responseWriter` wrapper to capture the status code:

```go
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

        next.ServeHTTP(wrapped, r)

        slog.InfoContext(r.Context(), "http request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", wrapped.statusCode,
            "duration_ms", time.Since(start).Milliseconds(),
            "request_id", RequestIDFromContext(r.Context()),
        )
    })
}
```

### Unwrapping ResponseWriter Interfaces

When wrapping `http.ResponseWriter`, the wrapper hides optional interfaces like `http.Flusher` and `http.Hijacker`. If your handlers or middleware use SSE (Server-Sent Events), WebSockets, or HTTP/2 push, check for and delegate these interfaces:

```go
func (rw *responseWriter) Flush() {
    if f, ok := rw.ResponseWriter.(http.Flusher); ok {
        f.Flush()
    }
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
    return rw.ResponseWriter
}
```

## Routing (Go 1.22+)

Go 1.22 enhanced `http.ServeMux` with method-based routing and path parameters. For most APIs, the stdlib mux is now sufficient:

```go
mux := http.NewServeMux()

// Method + path pattern — exact method match
mux.HandleFunc("GET /users", handleListUsers)
mux.HandleFunc("POST /users", handleCreateUser)
mux.HandleFunc("GET /users/{id}", handleGetUser)
mux.HandleFunc("PUT /users/{id}", handleUpdateUser)
mux.HandleFunc("DELETE /users/{id}", handleDeleteUser)

// Wildcard — matches any remaining path segments
mux.HandleFunc("GET /static/{path...}", handleStatic)

// Extract path parameters in the handler
func handleGetUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id") // Go 1.22+
    // ...
}
```

### When to Use a Third-Party Router

The stdlib mux covers most use cases. Consider a third-party router only when you need:

| Need | Stdlib covers? | Alternative |
| --- | --- | --- |
| Method routing (`GET /users`) | ✓ (Go 1.22+) | — |
| Path parameters (`/users/{id}`) | ✓ (Go 1.22+) | — |
| Wildcard paths (`/static/{path...}`) | ✓ (Go 1.22+) | — |
| Route groups with shared middleware | ✗ | `chi`, `gorilla/mux` |
| Automatic OPTIONS / HEAD responses | ✗ | `chi` |
| Route naming / URL generation | ✗ | `gorilla/mux` |
| OpenAPI schema generation from routes | ✗ | `huma`, `ogen` |

## JSON Request/Response Helpers

Centralize JSON encoding/decoding to enforce consistent content types and error handling:

```go
// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) error {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(v); err != nil {
        return fmt.Errorf("encoding json response: %w", err)
    }
    return nil
}

// readJSON decodes the request body into v with a size limit.
func readJSON(r *http.Request, v any) error {
    const maxBytes = 1 << 20 // 1 MB
    r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(v); err != nil {
        return fmt.Errorf("decoding json request: %w", err)
    }
    return nil
}
```

**Key points:**
- `http.MaxBytesReader` prevents denial-of-service via oversized request bodies
- `DisallowUnknownFields` catches typos in JSON field names early
- Content-Type header MUST be set before `WriteHeader` — headers sent after `WriteHeader` are silently ignored

## Server Configuration

Always configure timeouts — a server without timeouts leaks connections from slow or malicious clients:

```go
srv := &http.Server{
    Addr:         ":8080",
    Handler:      handler,
    ReadTimeout:  5 * time.Second,   // max time to read the entire request (headers + body)
    WriteTimeout: 10 * time.Second,  // max time to write the response
    IdleTimeout:  120 * time.Second, // max time for keep-alive connections between requests
}
```

| Timeout | Purpose | Default |
| --- | --- | --- |
| `ReadTimeout` | Limits slow-read attacks (Slowloris) | None (unlimited) |
| `WriteTimeout` | Prevents hung responses from leaking goroutines | None (unlimited) |
| `IdleTimeout` | Recycles inactive keep-alive connections | None (unlimited) |
| `ReadHeaderTimeout` | Limits time to read request headers only | None (unlimited) |

> [!WARNING]
> A server with no timeouts configured has **unlimited** read, write, and idle durations. Under load, slow clients will exhaust goroutines and file descriptors.

## Graceful Shutdown

Use `signal.NotifyContext` for clean shutdown that drains in-flight requests:

```go
func run(ctx context.Context) error {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
    defer cancel()

    srv := &http.Server{
        Addr:         ":8080",
        Handler:      handler,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    // Start server in a goroutine
    errCh := make(chan error, 1)
    go func() {
        slog.Info("server starting", "addr", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
        close(errCh)
    }()

    // Wait for signal or server error
    select {
    case err := <-errCh:
        return err
    case <-ctx.Done():
        slog.Info("shutting down server")
    }

    // Give in-flight requests time to complete
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer shutdownCancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("server shutdown: %w", err)
    }
    slog.Info("server stopped gracefully")
    return nil
}
```

**Key points:**
- `srv.Shutdown` stops accepting new connections and waits for in-flight requests to complete
- The shutdown timeout prevents hanging forever on stuck requests
- `http.ErrServerClosed` is the expected error after a clean shutdown — don't treat it as a failure
- Use `context.Background()` for the shutdown context, not the cancelled parent

## Testing HTTP Handlers

Use `httptest.NewRecorder` for unit tests and `httptest.NewServer` for integration tests:

```go
func TestGetUser(t *testing.T) {
    is := assert.New(t)

    // Setup
    svc := &MockUserService{
        users: map[string]*User{"1": {ID: "1", Name: "Alice"}},
    }
    h := &UserHandler{service: svc}

    // Create request with path value (Go 1.22+)
    req := httptest.NewRequest("GET", "/users/1", nil)
    req.SetPathValue("id", "1")
    rec := httptest.NewRecorder()

    // Execute
    err := h.GetUser(rec, req)

    // Assert
    is.NoError(err)
    is.Equal(http.StatusOK, rec.Code)
    is.Contains(rec.Body.String(), "Alice")
}
```

For comprehensive HTTP testing patterns (request bodies, query parameters, headers, middleware testing, and integration tests with `httptest.NewServer`), see the Testing reference (`golang-testing.md`).

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| No timeouts on `http.Server` | Always set `ReadTimeout`, `WriteTimeout`, and `IdleTimeout` — unlimited defaults leak resources |
| Writing headers after `WriteHeader` | Headers set after `WriteHeader()` or `Write()` are silently ignored. Set headers first |
| Not reading/closing request body | Unread bodies prevent connection reuse. Use `io.Copy(io.Discard, r.Body)` or `defer r.Body.Close()` |
| `log.Fatal` in handlers | Kills the entire process. Return errors or use the error-aware handler pattern |
| Handling errors inline in every handler | Duplicates error-to-response logic. Use the error-aware handler pattern to centralize |
| `json.NewDecoder` without size limit | Clients can send arbitrarily large bodies. Use `http.MaxBytesReader` |
| Recovery middleware not outermost | A panic in outer middleware bypasses recovery. Place it first in the chain |
| Wrapping `ResponseWriter` without `Unwrap` | Hides `http.Flusher`, `http.Hijacker`. Implement `Unwrap()` to preserve optional interfaces |
| Using `fmt.Fprintf(w, ...)` for JSON | No Content-Type header, no consistent error handling. Use a `writeJSON` helper |
| Creating `http.Client` with no timeout | Default client has no timeout — requests can hang indefinitely. Set `client.Timeout` or use `context.WithTimeout` |
| Ignoring `http.ErrServerClosed` | This is the expected error after graceful shutdown — check with `errors.Is` and don't log as a failure |
| `context.Background()` inside handlers | Breaks cancellation propagation. Always use `r.Context()` |
| Raw string routing without method prefix | Pre-1.22 `mux.HandleFunc("/users", h)` matches all methods. Use `"GET /users"` (Go 1.22+) to restrict |

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for context propagation and timeout patterns
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for error wrapping, sentinel errors, and the single handling rule
- → See the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`) for functional options and server constructor patterns
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for otelhttp tracing middleware and Prometheus metrics middleware
- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for HTTP handler testing with httptest

## Sub-topic: golang-grpc

**Persona:** You are a Go distributed systems engineer. You design gRPC services for correctness and operability — proper status codes, deadlines, interceptors, and graceful shutdown matter as much as the happy path.

**Modes:**

- **Build mode** — implementing a new gRPC server or client from scratch.
- **Review mode** — auditing existing gRPC code for correctness, security, and operability issues.

# Go gRPC Best Practices

Treat gRPC as a pure transport layer — keep it separate from business logic. The official Go implementation is `google.golang.org/grpc`.

This skill is not exhaustive. Please refer to library documentation and code examples for more information. Context7 can help as a discoverability platform.

## Quick Reference

| Concern | Package / Tool |
| --- | --- |
| Service definition | `protoc` or `buf` with `.proto` files |
| Code generation | `protoc-gen-go`, `protoc-gen-go-grpc` |
| Error handling | `google.golang.org/grpc/status` with `codes` |
| Rich error details | `google.golang.org/genproto/googleapis/rpc/errdetails` |
| Interceptors | `grpc.ChainUnaryInterceptor`, `grpc.ChainStreamInterceptor` |
| Middleware ecosystem | `github.com/grpc-ecosystem/go-grpc-middleware` |
| Testing | `google.golang.org/grpc/test/bufconn` |
| TLS / mTLS | `google.golang.org/grpc/credentials` |
| Health checks | `google.golang.org/grpc/health` |

## Proto File Organization

Organize by domain with versioned directories (`proto/user/v1/`). Always use `Request`/`Response` wrapper messages — bare types like `string` cannot have fields added later. Generate with `buf generate` or `protoc`.

[Proto & code generation reference](references/protoc-reference.md)

## Server Implementation

- Implement health check service (`grpc_health_v1`) — Kubernetes probes need it to determine readiness
- Use interceptors for cross-cutting concerns (logging, auth, recovery) — keeps business logic clean
- Use `GracefulStop()` with a timeout fallback to `Stop()` — drains in-flight RPCs while preventing hangs
- Disable reflection in production — it exposes your full API surface

```go
srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(loggingInterceptor, recoveryInterceptor),
)
pb.RegisterUserServiceServer(srv, svc)
healthpb.RegisterHealthServer(srv, health.NewServer())

go srv.Serve(lis)

// On shutdown signal:
stopped := make(chan struct{})
go func() { srv.GracefulStop(); close(stopped) }()
select {
case <-stopped:
case <-time.After(15 * time.Second):
    srv.Stop()
}
```

### Interceptor Pattern

```go
func loggingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    log.Printf("method=%s duration=%s code=%s", info.FullMethod, time.Since(start), status.Code(err))
    return resp, err
}
```

## Client Implementation

- Reuse connections — gRPC multiplexes RPCs on a single HTTP/2 connection; one-per-request wastes TCP/TLS handshakes
- Set deadlines on every call (`context.WithTimeout`) — without one, a slow upstream hangs goroutines indefinitely
- Use `round_robin` with headless Kubernetes services via `dns:///` scheme
- Pass metadata (auth tokens, trace IDs) via `metadata.NewOutgoingContext`

```go
conn, err := grpc.NewClient("dns:///user-service:50051",
    grpc.WithTransportCredentials(creds),
    grpc.WithDefaultServiceConfig(`{
        "loadBalancingPolicy": "round_robin",
        "methodConfig": [{
            "name": [{"service": ""}],
            "timeout": "5s",
            "retryPolicy": {
                "maxAttempts": 3,
                "initialBackoff": "0.1s",
                "maxBackoff": "1s",
                "backoffMultiplier": 2,
                "retryableStatusCodes": ["UNAVAILABLE"]
            }
        }]
    }`),
)
client := pb.NewUserServiceClient(conn)
```

## Error Handling

Always return gRPC errors using `status.Error` with a specific code — a raw `error` becomes `codes.Unknown`, telling the client nothing actionable. Clients use codes to decide retry vs fail-fast vs degrade.

| Code                 | When to Use                                 |
| -------------------- | ------------------------------------------- |
| `InvalidArgument`    | Malformed input (missing field, bad format) |
| `NotFound`           | Entity does not exist                       |
| `AlreadyExists`      | Create failed, entity exists                |
| `PermissionDenied`   | Caller lacks permission                     |
| `Unauthenticated`    | Missing or invalid token                    |
| `FailedPrecondition` | System not in required state                |
| `ResourceExhausted`  | Rate limit or quota exceeded                |
| `Unavailable`        | Transient issue, safe to retry              |
| `Internal`           | Unexpected bug                              |
| `DeadlineExceeded`   | Timeout                                     |

```go
// ✗ Bad — caller gets codes.Unknown, can't decide whether to retry
return nil, fmt.Errorf("user not found")

// ✓ Good — specific code lets clients act appropriately
if errors.Is(err, ErrNotFound) {
    return nil, status.Errorf(codes.NotFound, "user %q not found", req.UserId)
}
return nil, status.Errorf(codes.Internal, "lookup failed: %v", err)
```

For field-level validation errors, attach `errdetails.BadRequest` via `status.WithDetails`.

## Streaming

| Pattern | Use Case |
| --- | --- |
| Server streaming | Server sends a sequence (log tailing, result sets) |
| Client streaming | Client sends a sequence, server responds once (file upload, batch) |
| Bidirectional | Both send independently (chat, real-time sync) |

Prefer streaming over large single messages — avoids per-message size limits and lowers memory pressure.

```go
func (s *server) ListUsers(req *pb.ListUsersRequest, stream pb.UserService_ListUsersServer) error {
    for _, u := range users {
        if err := stream.Send(u); err != nil {
            return err
        }
    }
    return nil
}
```

## Testing

Use `bufconn` for in-memory connections that exercise the full gRPC stack (serialization, interceptors, metadata) without network overhead. Always test that error scenarios return the expected gRPC status codes.

[Testing patterns and examples](references/testing.md)

## Security

- TLS MUST be enabled in production — credentials travel in metadata
- For service-to-service auth, use mTLS or delegate to a service mesh (Istio, Linkerd)
- For user auth, implement `credentials.PerRPCCredentials` and validate tokens in an auth interceptor
- Reflection SHOULD be disabled in production to prevent API discovery

## Performance

| Setting | Purpose | Typical Value |
| --- | --- | --- |
| `keepalive.ServerParameters.Time` | Ping interval for idle connections | 30s |
| `keepalive.ServerParameters.Timeout` | Ping ack timeout | 10s |
| `grpc.MaxRecvMsgSize` | Override 4 MB default for large payloads | 16 MB |
| Connection pooling | Multiple conns for high-load streaming | 4 connections |

Most services do not need connection pooling — profile before adding complexity.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Returning raw `error` | Becomes `codes.Unknown` — client can't decide whether to retry. Use `status.Errorf` with a specific code |
| No deadline on client calls | Slow upstream hangs indefinitely. Always `context.WithTimeout` |
| New connection per request | Wastes TCP/TLS handshakes. Create once, reuse — HTTP/2 multiplexes RPCs |
| Reflection enabled in production | Lets attackers enumerate every method. Enable only in dev/staging |
| `codes.Internal` for all errors | Wrong codes break client retry logic. `Unavailable` triggers retry; `InvalidArgument` does not |
| Bare types as RPC arguments | Can't add fields to `string`. Wrapper messages allow backwards-compatible evolution |
| Missing health check service | Kubernetes can't determine readiness, kills pods during deployments |
| Ignoring context cancellation | Long operations continue after caller gave up. Check `ctx.Err()` |

## Cross-References

- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for deadline and cancellation patterns
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for gRPC error to Go error mapping
- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for gRPC interceptors (logging, tracing, metrics)
- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for gRPC testing with bufconn

## Sub-topic: golang-database

**Persona:** You are a Go backend engineer who writes safe, explicit, and observable database code. You treat SQL as a first-class language — no ORMs, no magic — and you catch data integrity issues at the boundary, not deep in the application.

**Modes:**

- **Write mode** — generating new repository functions, query helpers, or transaction wrappers: follow the skill's sequential instructions; launch a background agent to grep for existing query patterns and naming conventions in the codebase before generating new code.
- **Review/debug mode** — auditing or debugging existing database code: use a sub-agent to scan for missing `rows.Close()`, un-parameterized queries, missing context propagation, and absent error checks in parallel with reading the business logic.

# Go Database Best Practices

Go's `database/sql` provides a solid foundation for database access. Use `sqlx` or `pgx` on top of it for ergonomics — never an ORM.

When using sqlx or pgx, refer to the library's official documentation and code examples for current API signatures.

## Best Practices Summary

1. **Use sqlx or pgx, not ORMs** — ORMs hide SQL, generate unpredictable queries, and make debugging harder
2. Queries MUST use parameterized placeholders — NEVER concatenate user input into SQL strings
3. Context MUST be passed to all database operations — use `*Context` method variants (`QueryContext`, `ExecContext`, `GetContext`)
4. `sql.ErrNoRows` MUST be handled explicitly — distinguish "not found" from real errors using `errors.Is`
5. Rows MUST be closed after iteration — `defer rows.Close()` immediately after `QueryContext` calls
6. NEVER use `db.Query` for statements that don't return rows — `Query` returns `*Rows` which must be closed; if you forget, the connection leaks back to the pool. Use `db.Exec` instead
7. **Use transactions for multi-statement operations** — wrap related writes in `BeginTxx`/`Commit`
8. **Use `SELECT ... FOR UPDATE`** when reading data you intend to modify — prevents race conditions
9. **Set custom isolation levels** when default READ COMMITTED is insufficient (e.g., serializable for financial operations)
10. **Handle NULLable columns** with pointer fields (`*string`, `*int`) or `sql.NullXxx` types
11. Connection pool MUST be configured — `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`, `SetConnMaxIdleTime`
12. **Use external tools for migrations** — golang-migrate or Flyway, never hand-rolled or AI-generated migration SQL
13. **Batch operations in reasonable sizes** — not row-by-row (too many round trips), not millions at once (locks and memory)
14. **Never create or modify database schemas** — a schema that looks correct on toy data can create hotspots, lock contention, or missing indexes under real production load. Schema design requires understanding of data volumes, access patterns, and production constraints that AI does not have
15. **Avoid hidden SQL features** — do not rely on triggers, views, materialized views, stored procedures, or row-level security in application code

## Library Choice

| Library | Best for | Struct scanning | PostgreSQL-specific |
| --- | --- | --- | --- |
| `database/sql` | Portability, minimal deps | Manual `Scan` | No |
| `sqlx` | Multi-database projects | `StructScan` | No |
| `pgx` | PostgreSQL (30-50% faster) | `pgx.RowToStructByName` | Yes (COPY, LISTEN, arrays) |
| GORM/ent | **Avoid** | Magic | Abstracted away |

**Why NOT ORMs:**

- Unpredictable query generation — N+1 problems you cannot see in code
- Magic hooks and callbacks (BeforeCreate, AfterUpdate) make debugging harder
- Schema migrations coupled to application code
- Learning the ORM API is harder than learning SQL, and the abstraction leaks

## Parameterized Queries

```go
// ✗ VERY BAD — SQL injection vulnerability
query := fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", email)

// ✓ Good — parameterized (PostgreSQL)
var user User
err := db.GetContext(ctx, &user, "SELECT id, name, email FROM users WHERE email = $1", email)

// ✓ Good — parameterized (MySQL)
err := db.GetContext(ctx, &user, "SELECT id, name, email FROM users WHERE email = ?", email)
```

### Dynamic IN clauses

```go
query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", ids)
if err != nil {
    return fmt.Errorf("building IN clause: %w", err)
}
query = db.Rebind(query) // adjust placeholders for your driver
err = db.SelectContext(ctx, &users, query, args...)
```

### Dynamic column names

Never interpolate column names from user input. Use an allowlist:

```go
allowed := map[string]bool{"name": true, "email": true, "created_at": true}
if !allowed[sortCol] {
    return fmt.Errorf("invalid sort column: %s", sortCol)
}
query := fmt.Sprintf("SELECT id, name, email FROM users ORDER BY %s", sortCol)
```

For more injection prevention patterns, see the the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`).

## Struct Scanning and NULLable Columns

Use `db:"column_name"` tags for sqlx, `pgx.CollectRows` with `pgx.RowToStructByName` for pgx. Handle NULLable columns with pointer fields (`*string`, `*time.Time`) — they work cleanly with both scanning and JSON marshaling. See [Scanning Reference](./references/scanning.md) for examples of all approaches.

## Error Handling

```go
func GetUser(id string) (*User, error) {
    var user User

    err := db.GetContext(ctx, &user, "SELECT id, name FROM users WHERE id = $1", id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, ErrUserNotFound // translate to domain error
        }
        return nil, fmt.Errorf("querying user %s: %w", id, err)
    }

    return &user, nil
}
```

or:

```go
func GetUser(id string) (u *User, exists bool, err error) {
    var user User

    err := db.GetContext(ctx, &user, "SELECT id, name FROM users WHERE id = $1", id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, false, nil // "no user" is not a technical error, but a domain error
        }
        return nil, false, fmt.Errorf("querying user %s: %w", id, err)
    }

    return &user, true, nil
}
```

### Always close rows

```go
rows, err := db.QueryContext(ctx, "SELECT id, name FROM users")
if err != nil {
    return fmt.Errorf("querying users: %w", err)
}
defer rows.Close() // prevents connection leaks

for rows.Next() {
    // ...
}
if err := rows.Err(); err != nil { // always check after iteration
    return fmt.Errorf("iterating users: %w", err)
}
```

### Common database error patterns

| Error | How to detect | Action |
| --- | --- | --- |
| Row not found | `errors.Is(err, sql.ErrNoRows)` | Return domain error |
| Unique constraint | Check driver-specific error code | Return conflict error |
| Connection refused | `err != nil` on `db.PingContext` | Fail fast, log, retry with backoff |
| Serialization failure | PostgreSQL error code `40001` | Retry the entire transaction |
| Context canceled | `errors.Is(err, context.Canceled)` | Stop processing, propagate |

## Context Propagation

Always use the `*Context` method variants to propagate deadlines and cancellation:

```go
// ✗ Bad — no context, query runs until completion even if client disconnects
db.Query("SELECT ...")

// ✓ Good — respects context cancellation and timeouts
db.QueryContext(ctx, "SELECT ...")
```

For context patterns in depth, see the the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`).

## Transactions, Isolation Levels, and Locking

For transaction patterns, isolation levels, `SELECT FOR UPDATE`, and locking variants, see [Transactions](./references/transactions.md).

## Connection Pool

```go
db.SetMaxOpenConns(25)              // limit total connections
db.SetMaxIdleConns(10)              // keep warm connections ready
db.SetConnMaxLifetime(5 * time.Minute)  // recycle stale connections
db.SetConnMaxIdleTime(1 * time.Minute)  // close idle connections faster
```

For sizing guidance and formulas, see [Database Performance](./references/performance.md).

## Migrations

Use an external migration tool. Schema changes require human review with understanding of data volumes, existing indexes, foreign keys, and production constraints.

Recommended tools:

- [golang-migrate](https://github.com/golang-migrate/migrate) — CLI + Go library, supports all major databases
- [Flyway](https://flywaydb.org/) — JVM-based, widely used in enterprise environments
- [Atlas](https://atlasgo.io/) — modern, declarative schema management

Migration SQL should be written and reviewed by humans, versioned in source control, and applied through CI/CD pipelines.

## Avoid Hidden SQL Features

Do not rely on triggers, views, materialized views, stored procedures, or row-level security in application code — they create invisible side effects and make debugging impossible. Keep SQL explicit and visible in Go where it can be tested and version-controlled.

## Schema Creation

**This skill does NOT cover schema creation.** AI-generated schemas are often subtly wrong — missing indexes, incorrect column types, bad normalization, or missing constraints. Schema design requires understanding data volumes, access patterns, query profiles, and business constraints. Use dedicated database tooling and human review.

## Deep Dives

- **[Transactions](./references/transactions.md)** — Transaction boundaries, isolation levels, deadlock prevention, `SELECT FOR UPDATE`
- **[Testing Database Code](./references/testing.md)** — Mock connections, integration tests with containers, fixtures, schema setup/teardown
- **[Database Performance](./references/performance.md)** — Connection pool sizing, batch processing, indexing strategy, query optimization
- **[Struct Scanning](./references/scanning.md)** — Struct tags, NULLable column handling, JSON marshaling patterns

## Cross-References

- → See the Ecosystem (observability, CI/CD, linters, samber libs) reference (`golang-ecosystem.md`) for SQL injection prevention patterns
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for context propagation to database operations
- → See the Core (concurrency, context, data structures, errors, safety) reference (`golang-core.md`) for database error wrapping patterns
- → See the Testing (unit tests, benchmarks, testify) reference (`golang-testing.md`) for database integration test patterns

## References

- [database/sql tutorial](https://go.dev/doc/database/)
- [sqlx](https://github.com/jmoiron/sqlx)
- [pgx](https://github.com/jackc/pgx)
- [golang-migrate](https://github.com/golang-migrate/migrate)

## Sub-topic: golang-cli

**Persona:** You are a Go CLI engineer. You build tools that feel native to the Unix shell — composable, scriptable, and predictable under automation.

**Modes:**

- **Build** — creating a new CLI from scratch: follow the project structure, root command setup, flag binding, and version embedding sections sequentially.
- **Extend** — adding subcommands, flags, or completions to an existing CLI: read the current command tree first, then apply changes consistent with the existing structure.
- **Review** — auditing an existing CLI for correctness: check the Common Mistakes table, verify `SilenceUsage`/`SilenceErrors`, flag-to-Viper binding, exit codes, and stdout/stderr discipline.

# Go CLI Best Practices

Use Cobra + Viper as the default stack for Go CLI applications. Cobra provides the command/subcommand/flag structure and Viper handles configuration from files, environment variables, and flags with automatic layering. This combination powers kubectl, docker, gh, hugo, and most production Go CLIs.

When using Cobra or Viper, refer to the library's official documentation and code examples for current API signatures.

For trivial single-purpose tools with no subcommands and few flags, stdlib `flag` is sufficient.

## Quick Reference

| Concern             | Package / Tool                       |
| ------------------- | ------------------------------------ |
| Commands & flags    | `github.com/spf13/cobra`             |
| Configuration       | `github.com/spf13/viper`             |
| Flag parsing        | `github.com/spf13/pflag` (via Cobra) |
| Colored output      | `github.com/fatih/color`             |
| Table output        | `github.com/olekukonko/tablewriter`  |
| Interactive prompts | `github.com/charmbracelet/bubbletea` |
| Version injection   | `go build -ldflags`                  |
| Distribution        | `goreleaser`                         |

## Project Structure

Organize CLI commands in `cmd/myapp/` with one file per command. Keep `main.go` minimal — it only calls `Execute()`.

```
myapp/
├── cmd/
│   └── myapp/
│       ├── main.go              # package main, only calls Execute()
│       ├── root.go              # Root command + Viper init
│       ├── serve.go             # "serve" subcommand
│       ├── migrate.go           # "migrate" subcommand
│       └── version.go           # "version" subcommand
├── go.mod
└── go.sum
```

`main.go` should be minimal — see [assets/examples/main.go](assets/examples/main.go).

## Root Command Setup

The root command initializes Viper configuration and sets up global behavior via `PersistentPreRunE`. See [assets/examples/root.go](assets/examples/root.go).

Key points:

- `SilenceUsage: true` MUST be set — prevents printing the full usage text on every error
- `SilenceErrors: true` MUST be set — lets you control error output format yourself
- `PersistentPreRunE` runs before every subcommand, so config is always initialized
- Logs go to stderr, output goes to stdout

## Subcommands

Add subcommands by creating separate files in `cmd/myapp/` and registering them in `init()`. See [assets/examples/serve.go](assets/examples/serve.go) for a complete subcommand example including command groups.

## Flags

See [assets/examples/flags.go](assets/examples/flags.go) for all flag patterns:

### Persistent vs Local

- **Persistent** flags are inherited by all subcommands (e.g., `--config`)
- **Local** flags only apply to the command they're defined on (e.g., `--port`)

### Required Flags

Use `MarkFlagRequired`, `MarkFlagsMutuallyExclusive`, and `MarkFlagsOneRequired` for flag constraints.

### Flag Validation with RegisterFlagCompletionFunc

Provide completion suggestions for flag values.

### Always Bind Flags to Viper

This ensures `viper.GetInt("port")` returns the flag value, env var `MYAPP_PORT`, or config file value — whichever has highest precedence.

## Argument Validation

Cobra provides built-in validators for positional arguments. See [assets/examples/args.go](assets/examples/args.go) for both built-in and custom validation examples.

| Validator                   | Description                          |
| --------------------------- | ------------------------------------ |
| `cobra.NoArgs`              | Fails if any args provided           |
| `cobra.ExactArgs(n)`        | Requires exactly n args              |
| `cobra.MinimumNArgs(n)`     | Requires at least n args             |
| `cobra.MaximumNArgs(n)`     | Allows at most n args                |
| `cobra.RangeArgs(min, max)` | Requires between min and max         |
| `cobra.ExactValidArgs(n)`   | Exactly n args, must be in ValidArgs |

## Configuration with Viper

Viper resolves configuration values in this order (highest to lowest precedence):

1. **CLI flags** (explicit user input)
2. **Environment variables** (deployment config)
3. **Config file** (persistent settings)
4. **Defaults** (set in code)

See [assets/examples/config.go](assets/examples/config.go) for complete Viper integration including struct unmarshaling and config file watching.

### Example Config File (.myapp.yaml)

```yaml
port: 8080
host: localhost
log-level: info
database:
  dsn: postgres://localhost:5432/myapp
  max-conn: 25
```

With the setup above, these are all equivalent:

- Flag: `--port 9090`
- Env var: `MYAPP_PORT=9090`
- Config file: `port: 9090`

## Version and Build Info

Version SHOULD be embedded at compile time using `ldflags`. See [assets/examples/version.go](assets/examples/version.go) for the version command and build instructions.

## Exit Codes

Exit codes MUST follow Unix conventions:

| Code  | Meaning           | When to Use                               |
| ----- | ----------------- | ----------------------------------------- |
| 0     | Success           | Operation completed normally              |
| 1     | General error     | Runtime failure                           |
| 2     | Usage error       | Invalid flags or arguments                |
| 64-78 | BSD sysexits      | Specific error categories                 |
| 126   | Cannot execute    | Permission denied                         |
| 127   | Command not found | Missing dependency                        |
| 128+N | Signal N          | Terminated by signal (e.g., 130 = SIGINT) |

See [assets/examples/exit_codes.go](assets/examples/exit_codes.go) for a pattern mapping errors to exit codes.

## I/O Patterns

See [assets/examples/output.go](assets/examples/output.go) for all I/O patterns:

- **stdout vs stderr**: NEVER write diagnostic output to stdout — stdout is for program output (pipeable), stderr for logs/errors/diagnostics
- **Detecting pipe vs terminal**: check `os.ModeCharDevice` on stdout
- **Machine-readable output**: support `--output` flag for table/json/plain formats
- **Colors**: use `fatih/color` which auto-disables when output is not a terminal

## Signal Handling

Signal handling MUST use `signal.NotifyContext` to propagate cancellation through context. See [assets/examples/signal.go](assets/examples/signal.go) for graceful HTTP server shutdown.

## Shell Completions

Cobra generates completions for bash, zsh, fish, and PowerShell automatically. See [assets/examples/completion.go](assets/examples/completion.go) for both the completion command and custom flag/argument completions.

## Testing CLI Commands

Test commands by executing them programmatically and capturing output. See [assets/examples/cli_test.go](assets/examples/cli_test.go).

Use `cmd.OutOrStdout()` and `cmd.ErrOrStderr()` in commands (instead of `os.Stdout` / `os.Stderr`) so output can be captured in tests.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Writing to `os.Stdout` directly | Tests can't capture output. Use `cmd.OutOrStdout()` which tests can redirect to a buffer |
| Calling `os.Exit()` inside `RunE` | Cobra's error handling, deferred functions, and cleanup code never run. Return an error, let `main()` decide |
| Not binding flags to Viper | Flags won't be configurable via env/config. Call `viper.BindPFlag` for every configurable flag |
| Missing `viper.SetEnvPrefix` | `PORT` collides with other tools. Use a prefix (`MYAPP_PORT`) to namespace env vars |
| Logging to stdout | Unix pipes chain stdout — logs corrupt the data stream for the next program. Logs go to stderr |
| Printing usage on every error | Full help text on every error is noise. Set `SilenceUsage: true`, save full usage for `--help` |
| Config file required | Users without a config file get a crash. Ignore `viper.ConfigFileNotFoundError` — config should be optional |
| Not using `PersistentPreRunE` | Config initialization must happen before any subcommand. Use root's `PersistentPreRunE` |
| Hardcoded version string | Version gets out of sync with tags. Inject via `ldflags` at build time from git tags |
| Not supporting `--output` format | Scripts can't parse human-readable output. Add JSON/table/plain for machine consumption |

## Related Skills

See the Architecture reference (`golang-architecture.md`), the Architecture reference (`golang-architecture.md`), the Testing reference (`golang-testing.md`), the Architecture (style, naming, patterns, layout, structs) reference (`golang-architecture.md`)s.