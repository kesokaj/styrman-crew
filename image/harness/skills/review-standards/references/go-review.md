# Go review checklist

Concrete checks for Go diffs. Idiom authority: the `golang` skill. Cite file:line in findings.

## Errors

- [ ] Every `err` handled at the call site. `_ = doThing()` and empty `if err != nil {}` are findings.
- [ ] Wrapped with context on the way up: `fmt.Errorf("loading config %q: %w", path, err)` — `%w`, not `%v`, when callers may `errors.Is`/`errors.As`.
- [ ] Sentinel comparisons use `errors.Is(err, os.ErrNotExist)`, never `err == ...` or string matching on `err.Error()`.
- [ ] No `panic()` for recoverable conditions; `log.Fatal` only in `main`/`init` paths, never in libraries.
- [ ] Errors logged **or** returned, not both — double-logging the same failure at every layer is a finding.

## Concurrency & context

- [ ] `context.Context` is the first parameter of anything that blocks, does I/O, or fans out; never stored in a struct.
- [ ] Every goroutine has a defined exit: context cancellation, channel close, or `errgroup.Group` — `go func()` with no stop condition is a finding.
- [ ] `select` loops include `<-ctx.Done()`; tickers and timers are stopped.
- [ ] Shared maps/slices guarded (mutex or confined to one goroutine); flag data races the tests wouldn't catch — ask whether `go test -race` ran.
- [ ] `sync.WaitGroup.Add` called before the goroutine starts, not inside it.

## Resources

- [ ] `defer f.Close()` immediately after a successful open; `Close()` errors checked on writers (a swallowed close loses the final flush).
- [ ] No `defer` inside a loop that accumulates resources — extract the body into a function.
- [ ] HTTP: `resp.Body` closed on every path, including non-200; clients carry timeouts (`http.Client{Timeout: ...}` or per-request context) — the zero-value client's infinite wait is a finding in server code.

## API & structure

- [ ] Accept interfaces, return concrete types; interfaces defined in the consuming package and kept small (1–3 methods).
- [ ] Zero values useful where feasible; constructors (`NewX`) validate what the zero value cannot.
- [ ] Exported symbols have doc comments; anything exported without an external caller belongs in `internal/`.
- [ ] No package-level mutable state; dependencies passed explicitly (struct fields or parameters).
- [ ] `cmd/` stays thin — flag business logic in `main.go`.

## Cloud & ops (GCP)

- [ ] Structured logging via `log/slog` with JSON output; no secrets, tokens, or PII in log fields.
- [ ] Config from environment at startup, validated once — not `os.Getenv` scattered through handlers.
- [ ] Cloud Run/GKE fit: honors `PORT`, graceful shutdown on SIGTERM (`signal.NotifyContext` + `srv.Shutdown`), health endpoint cheap and dependency-free.
- [ ] GCP clients (`cloud.google.com/go/...`) created once and reused; `defer client.Close()` at owner scope; ADC assumed — flag hardcoded credentials or key-file paths.

## Tests

- [ ] Changed behavior has a covering test in the same package (`_test.go` beside the source); table-driven with `t.Run` subtests for multi-case logic.
- [ ] Tests assert observable behavior through the public interface, not internal call sequences; `t.Helper()` in helpers.
- [ ] Parallel-safe tests marked `t.Parallel()`; no sleeps as synchronization — use channels or `testing/synctest`-style fakes.
