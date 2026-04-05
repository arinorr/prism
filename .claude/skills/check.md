---
name: check
description: Run all CI checks locally (format, lint, test, coverage, build) before pushing. Mirrors the GitHub Actions CI pipeline.
user_invocable: true
---

# /check — Run all CI checks locally

Run the full CI check suite that mirrors the GitHub Actions pipeline. Fix any issues found before pushing.

## Steps

Run these checks in order. Stop and fix issues at each step before proceeding:

### 1. Format
```bash
gofmt -l .
```
If any files are listed, fix them with `gofmt -w <file>`.

### 2. Lint
```bash
go vet ./...
```
If golangci-lint is installed, also run:
```bash
golangci-lint run ./...
```

### 3. Test
```bash
go test -race ./...
```

### 4. Coverage
```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
```
Coverage must be >= 90%. If below threshold, add tests for uncovered code.
Clean up: `rm -f coverage.out`

### 5. Build
```bash
go build ./...
```

## On failure
- Format issues: run `gofmt -w` on the listed files
- Lint issues: fix the reported code issues
- Test failures: investigate and fix the failing tests
- Coverage below 90%: identify uncovered functions with `go tool cover -func=coverage.out | grep -v "100.0%"` and add tests
- Build errors: fix compilation issues

## After all checks pass
Stage and commit the fixes, then push.
