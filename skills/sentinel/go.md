---
name: sentinel-go
description: Go-specific security vulnerabilities and review checks for the Sentinel agent
---

## Go Security Module

This module extends the Sentinel's review with Go-specific security vulnerabilities and patterns.

**Sources**: OWASP Go Security Cheat Sheet, gosec rules, Go official security best practices, CWE entries for Go.

---

### Command Injection

- **`exec.Command` with user input**: Flag any use of `exec.Command()` or `exec.CommandContext()` where command name or arguments include user-controlled data. Especially dangerous with shell invocation (`/bin/sh -c`).
- **`os.StartProcess`**: Lower-level than `exec.Command` — same risks apply.
- **Mitigation**: Allowlist permitted commands. Never interpolate user input into command strings. Use argument arrays, not shell expansion.

### SQL Injection

- **String concatenation in queries**: Flag `fmt.Sprintf("SELECT ... WHERE id = '%s'", userInput)` and any SQL built with `+` concatenation. Require parameterized queries (`$1`, `$2`).
- **`database/sql` and `pgx`**: Both support parameterized queries natively. Flag any query where user input is not a parameter.
- **ORM pitfalls**: Even with ORMs like GORM, raw SQL methods (`db.Raw()`, `db.Exec()`) can introduce injection if not parameterized.

### Path Traversal

- **`os.Open`, `os.ReadFile`, `os.Create` with user input**: Flag any filesystem operation where the path includes user-controlled data. Require `filepath.Clean()` + prefix validation.
- **`filepath.Join` is not sufficient**: `filepath.Join("/safe/dir", "../../../etc/passwd")` resolves to `/etc/passwd`. After joining, verify the result starts with the expected base directory using `strings.HasPrefix(filepath.Clean(path), baseDir)`.
- **`http.ServeFile` and `http.FileServer`**: If serving user-requested paths, ensure the root is restricted and `..` traversal is blocked.

### Unsafe Package

- **`unsafe.Pointer`**: Bypasses Go's type safety and memory safety guarantees. Flag any use in application code. Acceptable only in low-level libraries with extensive testing and justification.
- **`reflect` with unsafe**: `reflect.SliceHeader` and `reflect.StringHeader` combined with `unsafe.Pointer` can corrupt memory. Flag in application code.

### Cryptography

- **Weak algorithms**: Flag `md5`, `sha1` for any security purpose (hashing passwords, generating tokens, integrity checks). Require `sha256` minimum, `bcrypt`/`scrypt`/`argon2` for passwords.
- **Hardcoded keys/IVs/salts**: Flag any cryptographic key, IV, or salt defined as a string or byte literal in source code. Require runtime generation or environment-based loading.
- **`math/rand` for security**: Flag `math/rand` used for tokens, secrets, or any security-sensitive random generation. Require `crypto/rand`.
- **`crypto/rand` correctly**: Ensure `crypto/rand.Read()` error is checked. The error is rare but catastrophic if ignored.

### Integer Overflow

- **Unchecked type conversions**: `int64` to `int32`, `uint` to `int`, or any narrowing conversion without bounds checking. Go doesn't panic on overflow — it silently wraps.
- **`strconv.Atoi` for untrusted input**: The result fits in `int` (platform-dependent size). For user input that must fit specific bounds, validate after conversion.

### HTTP Security

- **Missing timeouts**: `http.Server{}` without `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` is vulnerable to slowloris attacks. Flag `http.ListenAndServe()` (uses a server with no timeouts). Require explicit `http.Server` with timeouts set.
- **Missing TLS in production**: Flag `http.ListenAndServe()` without TLS in production configurations. Require `http.ListenAndServeTLS()` or a reverse proxy.
- **Response body not closed**: `http.Get()` and `http.Client.Do()` return a response whose `Body` must be closed. Flag missing `defer resp.Body.Close()`. Read the body fully before closing to reuse connections.
- **SSRF**: Flag `http.Get(userURL)` or any HTTP client call with user-controlled URLs. Require URL validation and hostname allowlists.
- **Unvalidated redirects**: `http.Client` follows redirects by default. For security-sensitive requests, set `CheckRedirect` to limit or block redirects.

### Concurrency Safety

- **Race conditions on maps**: Go maps are not safe for concurrent access. Flag concurrent read/write to the same map without `sync.RWMutex` or `sync.Map`. Data races can cause crashes.
- **Race conditions on slices**: Concurrent append to the same slice is a data race. Flag shared slice mutations without synchronization.
- **TOCTOU (Time-of-Check-Time-of-Use)**: Flag patterns where a condition is checked and then acted on without holding a lock. Example: checking file existence then opening — another goroutine may change it between.

### Secrets and Credentials

- **Hardcoded credentials**: Flag string literals that look like API keys, passwords, tokens, or connection strings in source code. Require environment variables or secret managers.
- **Credentials in struct tags**: Flag `json:"password"` or similar tags that might serialize secrets. Require `json:"-"` for sensitive fields.
- **Logging secrets**: Flag `log.Printf("key: %s", apiKey)` or structured logging that includes credential fields. Log references (IDs), not values.
- **Secrets in error messages**: Flag `fmt.Errorf("auth failed with key %s", key)`. Error messages may surface in logs, responses, or monitoring.

### Input Validation

- **Unbounded reads**: `io.ReadAll(r)` on untrusted input can exhaust memory. Require `io.LimitReader(r, maxBytes)` for user-supplied request bodies or file uploads.
- **Missing `http.MaxBytesReader`**: For HTTP handlers, flag `io.ReadAll(r.Body)` without `http.MaxBytesReader` wrapping. Require explicit request body size limits.
- **Regex with user input**: `regexp.Compile(userInput)` can be used for ReDoS. If user-supplied patterns are needed, use `regexp.CompilePOSIX` or set timeouts.
- **XML/JSON parsing of untrusted data**: `encoding/xml` is vulnerable to billion laughs (entity expansion) attacks. For untrusted XML, consider limiting nesting depth or using a safer parser.

### CGo

- **`import "C"`**: CGo code bypasses Go's memory safety, garbage collector, and goroutine scheduler. Flag in application code unless there's a documented need. CGo code can introduce buffer overflows, use-after-free, and memory leaks.
- **CGo and goroutines**: Each CGo call pins an OS thread. Flag CGo in hot paths or inside goroutines that may run at high concurrency.

### Dependency Security

- **`go.sum` integrity**: Flag PRs that modify `go.sum` entries for existing dependencies — verify the change is intentional (version bump, not tampering).
- **`replace` directives**: Flag `replace` directives in `go.mod` that point to local paths or unexpected repositories.
- **Minimum Go version**: Flag `go.mod` specifying a Go version with known security vulnerabilities. Check against [Go's release history](https://go.dev/doc/devel/release).
- **`govulncheck`**: Recommend running `govulncheck ./...` in CI to catch known vulnerabilities in dependencies.

### Anti-Patterns Quick Reference

Flag these when found:
1. `exec.Command()` with user-controlled arguments
2. SQL queries built with `fmt.Sprintf` or string concatenation
3. `filepath.Join` with user input and no prefix validation
4. `math/rand` for tokens, secrets, or security-sensitive values
5. `http.ListenAndServe()` without explicit server timeouts
6. `unsafe.Pointer` in application code
7. Missing `defer resp.Body.Close()` after HTTP requests
8. Concurrent map/slice access without synchronization
9. `io.ReadAll` on untrusted input without size limits
10. Credentials or secrets in string literals, logs, or error messages
11. `md5` or `sha1` used for security purposes
12. Missing `crypto/rand.Read()` error check
