---
name: know-it-all-typescript
description: TypeScript/JavaScript/React best practices and idioms for the Know-It-All agent
---

## TypeScript/JavaScript Module

This module extends the Know-It-All's review with TypeScript, JavaScript, and React-specific best practices.

**Sources**: Matt Pocock (Total TypeScript), Kent C. Dodds, Dan Abramov, React official docs, Prettier defaults.

---

### Type System

- **`type` over `interface`**: Prefer `type` aliases by default. Interfaces have surprising declaration merging — two interfaces with the same name silently merge, causing hard-to-find bugs. Use `interface` only when you need `extends` for object inheritance.
- **No `any`**: `any` disables type checking and propagates silently. Use `unknown` and narrow with type guards. Flag every use of `any` that isn't behind a clearly documented escape hatch.
- **`satisfies` for precise validation**: Use `satisfies` when you want to validate a value conforms to a type while retaining its narrower literal type. Default to `: Type` annotations. Treat `as` type assertions as a last resort.
- **Discriminated unions for state**: Model states as discriminated unions, not parallel booleans. `{ status: 'loading' } | { status: 'success'; data: T } | { status: 'error'; error: E }` makes invalid states unrepresentable. Flag parallel booleans like `isLoading` + `isError` + `data`.
- **Let inference work**: Don't add type annotations the compiler can already infer. Flag redundant annotations like `const x: string = "hello"`. Add explicit return types for complex functions, public APIs, and functions with multiple branches.
- **`as const` for literals**: Use `as const` on objects and arrays to preserve literal types deeply. Critical for config objects, route definitions, and anything feeding type-safe APIs.
- **Strict mode always**: `tsconfig.json` must have `"strict": true`. The entire TypeScript ecosystem assumes strict mode.

### React Components

- **Arrow function components**: Use arrow functions for component definitions with destructured, explicitly typed props.
- **One component per file**: Keep components focused. File name matches the component name.
- **400-line limit**: Components over 400 lines should be refactored — extract sub-components, custom hooks, or utility functions.
- **Pure renders**: Components must be idempotent. Side effects belong in event handlers or effects, never during render. Never mutate props or state directly.
- **Composition over prop drilling**: Prefer component composition (children, render props) before reaching for Context. Flag deep prop drilling through 3+ intermediate components.
- **No render helper functions**: Extract helpers as separate named components rather than defining render functions inside a component.
- **Semantic theme classes**: Use semantic Tailwind classes (`bg-card`, `text-foreground`, `text-destructive`) instead of hardcoded colors (`text-gray-600`, `bg-white`). Hardcoded colors break light/dark theme support.

### Hooks

- **Rules of Hooks (non-negotiable)**: Only call hooks at the top level — never inside loops, conditions, or nested functions. Only call hooks from React function components or custom hooks.
- **useEffect is synchronization, not lifecycle**: Don't think "componentDidMount equivalent." Ask: "What should this stay synchronized with?" Never lie about dependencies — trust the `exhaustive-deps` lint rule. Flag `// eslint-disable-next-line react-hooks/exhaustive-deps`.
- **Cleanup effects**: Always return cleanup functions for subscriptions, timers, and abort controllers. Never use `async` directly as the effect callback.
- **React Compiler reduces manual memoization**: The React Compiler auto-memoizes at build time. Stop wrapping every callback in `useCallback` or every derived value in `useMemo` without profiling evidence. Retain manual memoization only for genuinely expensive computations verified by profiling.
- **Functional state updates**: Use `setState(prev => prev + 1)` to remove state from dependency arrays. Use `useReducer` when state updates depend on multiple values.

### State Management

- **Colocate state**: Keep state as close to where it's needed as possible. Lift to the nearest common ancestor when sharing. Flag all-app-state-in-one-global-store patterns.
- **Server state is not client state**: Server data is a cache — stale, shared, and asynchronous. Use purpose-built tools (TanStack Query, SWR) for server state. Flag `useState` + `useEffect` for data fetching — the classic fetch-in-effect anti-pattern.
- **URL as state**: Search params and URL segments are state. Use type-safe routing for shareable, back-button-friendly state.
- **Separate concerns**: UI state (modals, forms), server cache (API data), and URL state are different things requiring different tools. Flag modal/form state mixed into global stores.

### Error Handling

- **Typed catch blocks**: Always use `catch(e: unknown)` — never `catch(e: any)` or untyped `catch(e)`. Narrow with `instanceof` or type guards before accessing error properties.
- **Result pattern**: For operations that can fail, consider discriminated unions: `{ ok: true; value: T } | { ok: false; error: E }`. Makes error states explicit and forces callers to handle them.
- **Error Boundaries for UI**: Use React Error Boundaries to catch rendering errors and show fallback UI. Combine with Suspense boundaries for loading + error states.
- **No swallowed errors**: Flag empty `catch` blocks. Flag thrown strings instead of Error objects.

### Testing

- **Testing Trophy — mostly integration**: Prioritize integration tests for the best confidence-to-effort ratio. Static analysis (TypeScript + ESLint) catches the first layer for free.
- **Never test implementation details**: Tests should not break when you refactor. Don't test internal state, private methods, or component internals. *"The more your tests resemble the way your software is used, the more confidence they can give you."*
- **Query priority**: `getByRole` > `getByLabelText` > `getByPlaceholderText` > `getByText` > `getByTestId` (last resort). Flag `getByTestId` as the primary query strategy.
- **Always `userEvent` over `fireEvent`**: `userEvent` simulates real user interaction sequences. Flag `fireEvent` usage.
- **Use `screen` for queries**: Don't destructure from `render()`. Use `screen.getByRole(...)`.
- **`findBy*` for async**: Use `findBy*` for elements that appear asynchronously, not `waitFor` + `getBy*`.
- **Minimize mocking**: Mock only external boundaries (network, timers). Flag tests that mock everything.

### Naming Conventions

- **Components**: `PascalCase` (`UserCard.tsx`)
- **Hooks**: `useCamelCase` (`useAuth.ts`)
- **Types/Interfaces**: `PascalCase`, no `I` prefix. Component props use `Props` suffix.
- **Event handlers**: `handle` prefix for internal handlers (`handleClick`), `on` prefix for callback props (`onClick`)
- **Booleans**: `is`/`has`/`should` prefix (`isLoading`, `hasError`, `shouldRefetch`)
- **Constants**: `SCREAMING_SNAKE_CASE` for true compile-time constants
- **Files**: `.tsx` for files with JSX, `.ts` otherwise. Match the default export name.
- **Import ordering**: React first, third-party, internal absolute paths, relative paths.

### Style

- **Prettier defaults as baseline**: Trailing commas `"all"`, semicolons, arrow parens `"always"`, 80 char print width. Formatting is a team responsibility, not personal preference.
- **No barrel file re-exports that defeat tree-shaking**: Import specific modules, not `import _ from 'lodash'`. Prefer `import { debounce } from 'lodash/debounce'`.

### Anti-Patterns Quick Reference

Flag these when found:
1. `any` instead of `unknown` or proper types
2. Parallel booleans (`isLoading` + `isError`) instead of discriminated unions
3. `useState` + `useEffect` for data fetching instead of a query library
4. Empty `useEffect` dependency array `[]` when props/state are read inside
5. Disabling `exhaustive-deps` ESLint rule
6. `getByTestId` as the default query strategy
7. `fireEvent` instead of `userEvent`
8. `useMemo`/`useCallback` wrapping everything without profiling evidence
9. `as` type assertions where type guards or `satisfies` would be safer
10. Interfaces with accidental declaration merging
11. Swallowed errors in empty `catch` blocks
12. Importing entire libraries for one utility
13. Hardcoded color values instead of semantic theme tokens
14. Render helper functions instead of extracted components
