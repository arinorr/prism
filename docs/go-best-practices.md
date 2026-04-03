# Go Best Practices Reference

A curated collection of Go best practices drawn from the Go community's most respected voices. Use this as a reference when writing or reviewing Go code.

## Sources

- **Rob Pike** — [Go Proverbs](https://go-proverbs.github.io/) (2015 Gopherfest)
- **Dave Cheney** — [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html)
- **Teiva Harsanyi** — [100 Go Mistakes and How to Avoid Them](https://github.com/teivah/100-go-mistakes) (Manning, 2022)
- **Google** — [Go Style Guide](https://google.github.io/styleguide/go/best-practices.html)
- **Go Team** — [Effective Go](https://go.dev/doc/effective_go), [Code Review Comments](https://go.dev/wiki/CodeReviewComments)

## Rob Pike's Go Proverbs

> "Don't communicate by sharing memory, share memory by communicating."

Use channels to pass data between goroutines instead of protecting shared state with mutexes — unless the mutex is clearly simpler.

> "The bigger the interface, the weaker the abstraction."

Interfaces should be small and focused. One or two methods is ideal. `io.Reader` has one method and is one of Go's most powerful abstractions.

> "Make the zero value useful."

Design types so that their zero value is ready to use without initialization. `sync.Mutex`, `bytes.Buffer`, and `http.Client` all work at zero value.

> "A little copying is better than a little dependency."

Don't import a package for one function. Copy the function if it's small. Dependencies have maintenance costs.

> "Errors are values."

Errors are not exceptions. They're regular values you can program with — store, compare, wrap, and pass around.

> "Don't just check errors, handle them gracefully."

Checking `if err != nil` is not handling. Handle means: log with context, wrap with `%w`, retry, degrade gracefully, or return a clear error to the caller.

> "Clear is better than clever."

Code is read far more often than it's written. Favor clarity and simplicity over cleverness and abstraction.

## Dave Cheney's Practical Go Principles

### Guiding values: Simplicity, Readability, Productivity

> "Simplicity is prerequisite for reliability." — Edsger Dijkstra

### Naming

- **Short names for short scopes**: `i` for a loop index, `r` for an `io.Reader` parameter
- **Longer names for wider scopes**: package-level variables need descriptive names
- **Don't stutter**: `http.Server` not `http.HTTPServer`; the package name is context
- **Use consistent naming**: if you call it `client` in one function, don't call it `c` in the next

### Error handling

- Always wrap errors with context: `fmt.Errorf("opening config: %w", err)`
- Error strings should be lowercase, no trailing punctuation
- Use `errors.Is()` and `errors.As()` for error checking, not string comparison
- Consider sentinel errors (`var ErrNotFound = errors.New("not found")`) for errors callers need to check

### Interface design

- Accept interfaces, return concrete types
- Define interfaces where they're consumed, not where they're implemented
- The standard library pattern: `io.Reader`, `io.Writer`, `fmt.Stringer` — all one method

### Package design

- A package should provide, not contain
- Package names should be lowercase, single-word, no underscores
- Avoid `util`, `common`, `misc` — these are signs of unclear responsibility

## 100 Go Mistakes (Teiva Harsanyi)

### Most relevant to this project

**#1: Unintended variable shadowing** — `:=` inside an `if` or `switch` block creates a new variable that shadows the outer one. The outer variable is unchanged.

**#5: Interface pollution** — Don't create interfaces preemptively. Create them when you have a concrete need for abstraction (testing, swapping implementations).

**#48: Forgetting about `errors.As`** — When wrapping errors, callers lose the ability to check specific types with `==`. Use `errors.As()` to unwrap.

**#54: Not handling defer errors** — `defer f.Close()` silently drops the error. For writes, check the close error.

**#61: Not understanding goroutine lifetime** — Always know when a goroutine will stop. A goroutine that runs forever is a goroutine leak.

**#77: Using context.Background everywhere** — Propagate context from the caller. `context.Background()` should only appear at the very top (main, test, init).

**#89: Writing inaccurate benchmarks** — Don't benchmark in isolation. Use `b.ResetTimer()`, avoid compiler optimizations with `sink` variables.

## Google Go Style Guide

### Code organization

- Prefer fewer, larger packages over many small ones
- Group related functionality, not related types
- Test files live next to the code they test

### Documentation

- Every exported name should have a doc comment
- Comments should be complete sentences
- Package comments go in a file named `doc.go` or at the top of the primary file

### Error handling

- Return errors, don't panic
- Error strings: lowercase, no punctuation, no "failed to" prefix (the caller adds context)

### Testing

- Test function names: `TestFoo_WhenBar_DoesQuux`
- Use table-driven tests for repetitive cases
- Use `t.Helper()` in test helper functions so stack traces point to the caller

## Anti-patterns to Avoid

1. **God structs** — a struct with 10+ fields and 20+ methods doing everything
2. **init() functions** — make setup explicit, not hidden in init()
3. **Global mutable state** — inject dependencies instead
4. **Naked returns** — only in very short functions (2-3 lines)
5. **Printf debugging** — use a logger or tests instead
6. **Ignoring context cancellation** — always check `ctx.Err()` in long operations
7. **Premature abstraction** — three concrete uses before extracting an interface
8. **Empty interfaces (any/interface{})** — avoid unless truly necessary (serialization)
