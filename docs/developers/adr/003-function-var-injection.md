# ADR-003: Function-Var Injection for Testability

## Status

Accepted (2026-02-13)

## Context

The codebase needs testability without heavy abstractions. Go's standard
approach of using interfaces and dependency injection adds ceremony that may not
be justified for a CLI tool with straightforward dependencies.

## Decision

Use **package-level function variables** that can be replaced in tests.

## Rationale

The pattern looks like:

```go
// Production code
var osGetwd = os.Getwd
var findModuleRoot = index.FindModuleRoot
var sqlOpen = sql.Open

// In tests
func TestSomething(t *testing.T) {
    osGetwd = func() (string, error) { return "/mock/path", nil }
    t.Cleanup(func() { osGetwd = os.Getwd })
    // ...
}
```

### Benefits

- **Zero ceremony** — No interfaces, no constructors with dependencies, no mock
  generators
- **Explicit** — The var declaration documents exactly what's replaceable
- **Co-located** — Override and test are in the same file
- **Lightweight** — No dependency injection framework

### Trade-offs

- **Not thread-safe** — Tests that replace function vars must not run in
  parallel within the same package (this is fine for `go test` which runs
  packages sequentially by default)
- **Global state** — Function vars are package-level globals, which can be
  surprising if not documented
- **Cleanup required** — Tests must restore original values (use `t.Cleanup`)

### Alternatives considered

- **Interfaces + dependency injection** — More idiomatic Go for large projects,
  but adds interface definitions, constructors, and mock implementations for
  what are usually simple function calls
- **Test doubles via build tags** — Compile-time replacement is clean but makes
  test files harder to understand
- **No mocking (integration tests only)** — Would require real filesystem and
  database for every test, making tests slower and less isolated

## Consequences

- All replaceable dependencies are declared as `var` at package level
- Test files override these vars and restore them via `t.Cleanup`
- Tests within a package run sequentially (no `-parallel` within package)
- The pattern is documented in CLAUDE.md conventions so new contributors follow
  it consistently
