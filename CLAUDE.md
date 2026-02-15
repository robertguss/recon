# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What is Recon

Recon is a code intelligence and knowledge CLI for Go repositories. It indexes
Go source code (packages, files, symbols, imports, dependencies) into a local
SQLite database (`.recon/recon.db`) and provides commands for navigating,
searching, recording decisions with evidence, detecting patterns, and orienting
within a codebase.

## Build & Development Commands

This project uses [just](https://github.com/casey/just) as a command runner. All
recipes are in `justfile`.

```sh
just build          # Build binary to ./bin/recon
just install        # Install to GOPATH/bin
just run <args>     # Run via go run, forwarding args
just test           # Run full test suite (go test ./...)
just test-race      # Run tests with race detector
just cover          # Generate coverage.out and print summary
just fmt            # Format all Go packages
```

Run a single package's tests:

```sh
go test ./internal/knowledge/...
```

Run a single test by name:

```sh
go test ./internal/orient/... -run TestOrientService
```

Workflow commands (run recon itself):

```sh
just init           # Initialize .recon/ directory and schema
just sync           # Index current repository state
just orient         # Show status + suggested next actions
just find <args>    # Search indexed symbols/files/imports
just decide <args>  # Record a decision with evidence
just recall <args>  # Recall previously recorded decisions
just db-reset       # Delete .recon/recon.db
```

## Architecture

### Entry Point & CLI Layer

- `cmd/recon/main.go` — entry point, delegates to `internal/cli.NewRootCommand`
- `internal/cli/` — all Cobra command definitions and CLI wiring
  - `root.go` — builds the root command, registers subcommands (init, sync,
    orient, find, decide, pattern, recall, status)
  - `store.go` — helper to open existing DB, returns a typed error if DB not
    initialized
  - `output.go` — shared output formatting (JSON/text modes)
  - `exit_error.go` — typed `ExitError` for controlled exit codes
  - `json_errors.go` — structured JSON error classification

### Domain Services (internal/)

Each domain has its own package under `internal/` with a `Service` struct
wrapping `*sql.DB`:

| Package              | Purpose                                                                               |
| -------------------- | ------------------------------------------------------------------------------------- |
| `internal/knowledge` | Decision recording: propose, verify, promote, update, archive decisions with evidence |
| `internal/find`      | Symbol/file/import search across the indexed codebase                                 |
| `internal/recall`    | Query and retrieve previously recorded decisions (FTS-backed)                         |
| `internal/orient`    | Status aggregation and next-action suggestions                                        |
| `internal/pattern`   | Detect and record recurring code patterns                                             |
| `internal/index`     | Repository indexing: parse Go files, extract symbols/imports/deps, upsert into DB     |

### Database Layer

- `internal/db/db.go` — SQLite connection management (modernc.org/sqlite,
  pure-Go driver), `.recon/` directory management
- `internal/db/migrate.go` — schema migrations using golang-migrate
- `internal/db/migrations/` — numbered SQL migration files (up/down)
- Database lives at `<module-root>/.recon/recon.db`

Key tables: `packages`, `files`, `symbols`, `imports`, `symbol_deps`,
`decisions`, `evidence`, `proposals`, `sessions`, `sync_state`, `search_index`
(FTS5)

### Testing Patterns

- Tests use both real SQLite databases (temp directories) and `go-sqlmock` for
  error path testing
- Files named `*_sqlmock_test.go` test SQL error handling via mocked DB
  connections
- Files named `*_extra_test.go` and `*_coverage_test.go` fill coverage gaps
- The CLI layer is tested via Cobra command execution (`cmd.Execute()`) with
  captured stdout/stderr
- Test helpers inject function vars (e.g., `osGetwd`, `findModuleRoot`,
  `sqlOpen`) for isolation

## ExecPlans

When writing complex features or significant refactors, use an ExecPlan as
described in `.agent/PLANS.md`. ExecPlans are self-contained living documents
that follow strict TDD and require 100% test coverage.

## Conventions

- Conventional Commits for commit messages (e.g.,
  `fix(decide): classify update JSON errors`)
- Prefer `internal/` packages — nothing is exported outside the module
- Each service owns its SQL queries directly (no ORM, no shared query builder)
- Function-var injection pattern for testability (override package-level `var`
  in tests)
- `--no-prompt` flag disables interactive prompts globally
- Output supports both text and JSON modes via `internal/cli/output.go`
