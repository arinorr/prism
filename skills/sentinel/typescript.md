---
name: sentinel-typescript
description: JavaScript/TypeScript-specific security vulnerabilities and review checks for the Sentinel agent
---

## JavaScript/TypeScript Security Module

This module extends the Sentinel's review with JS/TS-specific vulnerabilities and security patterns.

**Sources**: OWASP Node.js Security Cheat Sheet, Snyk/Liran Tal, Socket.dev/Feross Aboukhadijeh, MDN, Philippe De Ryck, Node.js official security docs.

---

### Injection

- **`eval()`, `Function()`, `vm.runInNewContext()`**: Code injection vectors. An attacker can reach `process.mainModule.require('child_process').execSync(...)` from a single `eval()`. Flag any use with dynamic input.
- **`child_process.exec()`**: Passes arguments through `/bin/sh`, so shell metacharacters (`;`, `|`, `$()`, backticks) in user input execute arbitrary commands. Require `execFile()` or `spawn()` with argument arrays.
- **`setTimeout(string)` / `setInterval(string)`**: These are implicit `eval()`. Flag string arguments — require function arguments.
- **Template injection**: Server-side template engines (EJS, Pug, Handlebars) that interpolate user input without escaping.
- **SQL/NoSQL injection**: Unparameterized queries. With MongoDB, user input can contain operators like `$gt`, `$ne` — flag `req.body` used directly in query objects.
- **ReDoS**: Regular expressions with nested quantifiers (e.g. `(a+)+$`) can be exploited to hang the event loop. Flag dynamic regex construction from user input.

### Prototype Pollution

This is the most JS-specific and underappreciated threat class.

- **Attack vector**: User input containing `__proto__`, `constructor`, or `prototype` keys that modify `Object.prototype` via `Object.assign()`, spread operators, or recursive merge functions.
- **Impact**: Bypass access control (`if (!user.isAdmin)` passes when `Object.prototype.isAdmin = true`), modify `fetch()` behavior, inject HTML, bypass sanitizer configs.
- **Flag these patterns with user-controlled input**:
  - `Object.assign(target, userInput)`
  - `{ ...defaults, ...userInput }` (spread)
  - Recursive/deep merge functions
  - `for...in` loops over user-controlled objects
  - `obj[userInput]` bracket notation
- **Require instead**:
  - `Object.create(null)` for lookup tables with user-controlled keys
  - `Map` instead of plain objects for dynamic keys
  - `Object.hasOwn(obj, key)` instead of `in` or `hasOwnProperty`
  - `Object.keys()` + `for...of` instead of `for...in`
  - Zod with `.strict()` for input validation (strips unknown properties)

### Supply Chain and Dependencies

- **New dependencies**: Flag any PR that adds a new dependency to `package.json`. Verify it's legitimate and necessary.
- **Lockfile integrity**: Flag changes to `resolved` URLs or `integrity` hashes in lockfiles — potential lockfile poisoning.
- **Install scripts**: Flag any dependency with `hasInstallScript: true` appearing for the first time. Flag `preinstall`/`postinstall` scripts in the project's own `package.json`.
- **Version pinning**: Flag floating version ranges (`^`, `~`). Prefer exact pins for production dependencies.
- **CI hygiene**: Flag `npm install` in CI/CD scripts — should be `npm ci` (respects lockfile exactly).
- **Typosquatting**: Check that package names match what you expect — common attack vector.

### Authentication and Sessions

- **JWT in localStorage**: Accessible to any XSS attack via `window.localStorage`. Flag any token stored in `localStorage` or `sessionStorage`. Prefer HttpOnly + Secure + SameSite=Strict cookies, or in-memory storage with refresh tokens.
- **Algorithm pinning**: `jwt.verify(token, secret)` without `{ algorithms: ['RS256'] }` allows algorithm confusion attacks (attacker uses `none` algorithm). Flag missing explicit `algorithms` array.
- **Cookie flags**: Flag cookies without `httpOnly`, `secure`, and `sameSite` flags.
- **Timing-safe comparison**: Flag `===` or `==` comparisons of secrets, tokens, or hashes. Require `crypto.timingSafeEqual()`.
- **Password hashing**: Require `bcrypt`, `scrypt`, or `argon2`. Flag SHA/MD5 for passwords.
- **Short-lived tokens**: Access tokens should expire in 15 minutes or less. Flag long-lived tokens.

### React and Frontend XSS

- **`dangerouslySetInnerHTML`**: Direct XSS vector. Flag any use without DOMPurify or equivalent sanitization.
- **`javascript:` URLs**: `<a href={userInput}>` where input can be `javascript:alert(1)`. Flag `href`, `src`, `action` props that accept user input without URL protocol validation.
- **`ref.current.innerHTML`**: Bypasses React's escaping. Flag any use.
- **Dynamic component rendering**: `const Component = components[userInput]; <Component />` — flag without an explicit allowlist.
- **SSR concatenation**: Flag server-side rendering output concatenated with unsanitized user data.

### Data Exposure

- **Stack traces in responses**: Flag error handlers that send `err.message`, `err.stack`, or raw error objects to clients. Require generic messages to clients, detailed logs server-side.
- **Over-fetching**: Flag `SELECT *` or returning full database objects without explicit field selection.
- **Secrets in source**: Flag hardcoded API keys, passwords, tokens, connection strings. Flag any `VITE_` prefixed env var containing secrets (Vite exposes these to the browser bundle).
- **Sensitive data in logs**: Flag logging of credentials, tokens, or PII. Log references (IDs) only.
- **`X-Powered-By` header**: Flag if not removed (use Helmet.js). Technology disclosure aids attackers.

### Node.js Backend

- **Synchronous crypto in hot paths**: `pbkdf2Sync`, `scryptSync`, `randomFillSync` block the event loop. Flag in request handlers — require async variants.
- **Path traversal**: Flag any `fs` operations with user input. Require path validation against a base directory using `path.resolve()` + prefix check.
- **SSRF**: Flag `fetch()`, `axios`, or `http.request()` with user-controlled URLs. Require URL allowlists or hostname validation.
- **Request size limits**: Flag `express.json()` without a `limit` option — allows memory exhaustion. Require explicit limits (e.g. `{ limit: '100kb' }`).
- **Security headers**: Flag missing Helmet.js or equivalent. Verify CSP does not include `'unsafe-eval'` or `'unsafe-inline'` for scripts.
- **Unhandled rejections**: Flag missing `process.on('unhandledRejection', ...)` — can crash the process or leave inconsistent state.

### Input Validation

- **TypeScript types are NOT runtime validation**: Types vanish at compile time. All data from external sources (API requests, URL params, form input, third-party APIs) must be validated at runtime.
- **Zod at boundaries**: Flag any `req.body`, `req.query`, or `req.params` used without Zod, Yup, ajv, or equivalent validation. Prefer Zod with `.strict()` to reject unknown properties (also prevents prototype pollution).
- **Type coercion**: Flag `==` comparisons — use `===`. URL query params can parse as arrays or objects depending on the parser library.

### Configuration

- **CSP**: Flag `'unsafe-eval'` in Content-Security-Policy — it re-enables `eval()`, defeating CSP's purpose. Avoid `'unsafe-inline'` for scripts — use nonces or hashes.
- **CORS**: Flag `origin: '*'` with `credentials: true` — browsers block this, but the misconfiguration signals misunderstanding. Flag wildcard origins on authenticated endpoints.
- **Rate limiting**: Flag missing rate limiting on authentication endpoints (login, password reset, OTP verification).
- **Environment files**: Flag `.env` files in version control or missing from `.gitignore`. Validate environment variables at startup (use `envalid` or Zod).
- **HTTPS**: Flag missing HSTS header or HTTP-to-HTTPS redirect in production configs.
