---
name: know-it-all-go
description: Go-specific best practices and idioms for the Know-It-All agent
---

## Go Module

This module extends the Know-It-All's review with Go-specific idioms, conventions, and best practices.

**Sources**: Rob Pike (Go Proverbs), Dave Cheney (Practical Go), Teiva Harsanyi (100 Go Mistakes), Google Go Style Guide, Go Team (Effective Go, Code Review Comments).

---

### Error Handling

- **Wrap errors with context**: Always use `fmt.Errorf("doing X: %w", err)`. Use `%w` not `%v` — `%v` breaks the error chain, making `errors.Is()` and `errors.As()` fail. Flag `%v` when wrapping errors.
- **Error strings**: Lowercase, no trailing punctuation, no "failed to" prefix. The caller adds context, so `"opening config: %w"` not `"Failed to open config: %w"`.
- **Sentinel errors**: Use `var ErrNotFound = errors.New("not found")` for errors callers need to check. Check with `errors.Is()` and `errors.As()`, never string comparison.
- **Never ignore errors**: Flag `result, _ := doSomething()` without a comment. If intentionally ignoring, require `_ = f.Close() // best-effort cleanup`.
- **Handle defer errors**: `defer f.Close()` silently drops the error. For writes, the close error matters — check it.
- **Custom error types**: Use structured error types (`type OrderError struct { ... }`) with `Unwrap()` when errors need rich context beyond a string.

### Interfaces

- **Small and focused**: One or two methods is ideal. `io.Reader` has one method and is one of Go's most powerful abstractions. Flag interfaces with 5+ methods — they're likely too broad.
- **Define where consumed, not where implemented**: The package that *uses* the interface should define it, not the package that implements it. This is the opposite of Java.
- **Accept interfaces, return concrete types**: Function parameters should be interfaces (for flexibility), return values should be concrete types (for clarity).
- **Don't create interfaces preemptively**: Create them when you have a concrete need (testing, multiple implementations). Three concrete uses before extracting.
- **Compile-time checks**: Use `var _ Interface = (*ConcreteType)(nil)` to verify implementations at compile time.

### Zero Values

- **Make the zero value useful**: Design types so they work without initialization. `sync.Mutex`, `bytes.Buffer`, and `http.Client` all work at zero value. Flag types that require `New()` or `Init()` when they could work at zero value.

### Context Propagation

- **`context.Background()` is for the top level only**: It should appear in `main()`, test functions, and `init()`. Flag it anywhere else — the caller's context should be threaded through.
- **Always pass context as the first parameter**: `func DoThing(ctx context.Context, ...) error`.
- **Check `ctx.Err()` in long operations**: Loops, retries, and batch processing should check for cancellation.
- **Set timeouts on outbound calls**: Network requests, database queries, and external API calls should have deadlines.

### Naming

- **Short names for short scopes**: `i` for a loop index, `r` for an `io.Reader` parameter, `ctx` for context.
- **Longer names for wider scopes**: Package-level variables and exported names need descriptive names.
- **Don't stutter**: `http.Server` not `http.HTTPServer`. The package name provides context.
- **Package names**: Lowercase, single word, no underscores. A package should provide, not contain. Avoid `util`, `common`, `misc`.
- **Receiver names**: Short (1-2 letters), consistent across methods. `s` for a service, not `self` or `this`.

### Documentation

- **Every exported name gets a doc comment**: Complete sentences starting with the name. `// Server handles incoming requests.` not `// this is the server`.
- **Package comments**: Go in `doc.go` or at the top of the primary file.

### Concurrency

- **Share memory by communicating**: Use channels to pass data between goroutines when the pattern fits. Use mutexes when they're clearly simpler.
- **Know when goroutines stop**: Every goroutine should have a clear termination condition. A goroutine that runs forever is a leak. Flag goroutines without cancellation paths.
- **Protect shared state**: Flag concurrent access to maps, slices, or struct fields without synchronization. Use `sync.RWMutex` when reads dominate.
- **`defer mu.Unlock()`**: Always defer the unlock immediately after locking to prevent forgetting.

### Code Organization

- **File organization order**: package declaration, imports (stdlib / external / internal), constants, variables, types, constructor, public methods, private methods.
- **Import grouping**: Standard library, then external packages, then internal packages — separated by blank lines.
- **Prefer fewer, larger packages**: Over many small ones. Group related functionality, not related types.
- **Test files next to code**: `strategy.go` and `strategy_test.go` in the same directory.

### Testing

- **Table-driven tests**: Use `[]struct{ name string; ... }` with `t.Run(tt.name, ...)` for repetitive test cases. Run subtests in parallel with `t.Parallel()` when safe.
- **`t.Helper()`**: Call in test helper functions so failure stack traces point to the caller, not the helper.
- **Test naming**: `TestFoo_WhenBar_DoesQuux` for clarity.
- **Constructor injection for testability**: Accept interfaces, inject mocks via constructors.

### Style

- **`gofmt` is non-negotiable**: All Go code must be gofmt'd. This isn't a style preference — it's a language norm.
- **Naked returns only in short functions**: 2-3 lines max. In longer functions, named returns hurt readability.
- **Constants for repeated strings**: Extract string literals appearing 3+ times to named constants.
- **Avoid `init()` functions**: Make setup explicit. `init()` runs implicitly, hides dependencies, and makes testing harder.
- **No global mutable state**: Inject dependencies via constructors instead.

### Anti-Patterns Quick Reference

Flag these when found:
1. `%v` instead of `%w` when wrapping errors
2. `context.Background()` outside of `main()`, tests, or `init()`
3. Interfaces with 5+ methods (interface pollution)
4. `result, _ := doSomething()` without a justification comment
5. Type stutter (`http.HTTPServer`, `config.ConfigLoader`)
6. Unexported `init()` functions hiding side effects
7. Goroutines without clear termination conditions
8. Unchecked type assertions (`x := val.(Type)` without `, ok`)
9. `errors.New()` in-line when a sentinel error would enable callers to check
10. Preemptive interfaces with only one implementation
11. `any`/`interface{}` when a concrete or constrained type would work
12. String comparison for error checking instead of `errors.Is()`/`errors.As()`
