# Repository Guidelines

## Project Overview

Recon is a code intelligence and knowledge CLI for Go repositories. See
[CLAUDE.md](CLAUDE.md) for architecture, build commands, and conventions.

## Project Structure

```
cmd/recon/          Entry point (delegates to internal/cli)
internal/cli/       Cobra command definitions and CLI wiring
internal/db/        SQLite connection management and migrations
internal/index/     Go source code parsing and indexing
internal/find/      Symbol/file/import search
internal/knowledge/ Decision lifecycle management
internal/pattern/   Pattern detection and recording
internal/recall/    FTS-backed knowledge retrieval
internal/orient/    Status aggregation and context building
internal/install/   Claude Code integration file installation
docs/               Documentation, plans, brainstorms
```

## Build, Test, and Development Commands

This project uses [just](https://github.com/casey/just) as a command runner.

```sh
just build          # Build binary to ./bin/recon
just install        # Install to GOPATH/bin
just test           # Run full test suite (go test ./...)
just test-race      # Run tests with race detector
just cover          # Generate coverage.out and print summary
just fmt            # Format all Go packages
```

Run a single package's tests:

```sh
go test ./internal/knowledge/...
```

## Coding Style & Conventions

- All code lives in `internal/` — nothing is exported outside the module
- Each service owns its SQL queries directly (no ORM, no shared query builder)
- Function-var injection pattern for testability
- Conventional Commits for messages (e.g., `feat(find): add --list-packages`)
- `--no-prompt` flag disables interactive prompts globally
- Output supports both text and JSON modes

## Testing Guidelines

- Tests use both real SQLite databases (temp directories) and `go-sqlmock`
- Files named `*_sqlmock_test.go` test SQL error handling
- Files named `*_extra_test.go` and `*_coverage_test.go` fill coverage gaps
- CLI tests use Cobra command execution with captured stdout/stderr
- Test helpers inject function vars for isolation
- Target: 100% test coverage

## Commit & Pull Request Guidelines

Use Conventional Commits: `feat(scope): description`, `fix(scope): description`,
`test(scope): description`.

## ExecPlans

When writing complex features or significant refactors, use an ExecPlan as
described in `.agent/PLANS.md`.
