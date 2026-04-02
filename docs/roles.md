# Reviewer Roles

Prism ships with 7 built-in reviewer roles. Each role is defined by a skill file in `skills/` that tells the agent what to look for, what to ignore, and how to format its findings.

## Know-It-All

**Focus:** Best practices, code smells, and language idioms.

The Know-It-All knows every linting rule, style guide, and anti-pattern. It catches naming issues, error handling inconsistencies, and code that doesn't follow established conventions for the language.

**Looks for:** Naming, error handling, anti-patterns, style consistency, code smells (long functions, deep nesting, god objects).

**Ignores:** Performance, architecture, correctness.

## Architect

**Focus:** System design, patterns, scalability, and abstractions.

The Architect thinks in systems. It evaluates whether changes fit the broader architecture, whether abstractions are clean, and whether the code introduces technical debt.

**Looks for:** Module boundaries, dependency direction, separation of concerns, coupling, API design, scalability.

**Ignores:** Line-level style, performance specifics, security.

## Solver

**Focus:** Correctness and completeness.

The Solver asks: does this PR actually solve the problem it claims to solve? It thinks about what could go wrong.

**Looks for:** Edge cases, boundary conditions, error paths, race conditions, missing cases, regression risk, test coverage.

**Ignores:** Code style, architecture, performance.

## Editor

**Focus:** Readability, simplicity, and clarity.

The Editor reviews code like a book editor reviews prose. Code is read far more often than it's written.

**Looks for:** Cognitive load, naming clarity, unnecessary complexity, duplication, function length, comment quality.

**Ignores:** Best practices that don't affect readability, performance, correctness.

## Optimizer

**Focus:** Performance, algorithmic complexity, and resource usage.

The Optimizer thinks in Big-O and resource budgets. It considers whether computational cost is justified.

**Looks for:** Algorithmic complexity, unnecessary allocations, N+1 queries, caching opportunities, I/O efficiency, concurrency.

**Important:** The Optimizer weighs tradeoffs — not every optimization is worth the complexity.

**Ignores:** Code style, architecture, correctness.

## Sentinel

**Focus:** Security vulnerabilities and dangerous code.

The Sentinel reviews code through the lens of an attacker. Every input is untrusted, every boundary is a potential breach.

**Looks for:** Injection flaws (SQL, command, XSS), auth gaps, secrets in code, SSRF, path traversal, unsafe deserialization, cryptography issues, CSRF, TOCTOU bugs.

**Ignores:** Code style, performance, readability.

## Test Engineer

**Focus:** Test coverage, edge cases, and testing improvements.

The Test Engineer evaluates whether tests are meaningful, complete, and resilient.

**Looks for:** Missing test coverage, untested edge cases, flaky test risk, superficial assertions, error path testing, regression coverage, test readability.

**Ignores:** Code style outside tests, architecture, performance, security.

## Selecting roles

Use the `--roles` flag to run specific agents:

```bash
# Security-only review
prism review 42 --roles sentinel

# Quick trio
prism review 42 --roles solver,sentinel,test-engineer

# All roles (default)
prism review 42
```

## Severity levels

All roles use the same three severity levels:

| Level | Meaning |
|-------|---------|
| **critical** | Will cause failures, security issues, or significant problems |
| **warning** | Should be addressed but isn't immediately harmful |
| **info** | Suggestion or minor improvement |

Only warning and critical findings are posted as inline PR comments when using `--comment`.
