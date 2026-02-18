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
just cover-html     # Open HTML coverage report in browser (requires coverage.out)
just fmt            # Format all Go packages
just clean          # Remove ./bin and coverage.out
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
    orient, find, decide, pattern, recall, status, edges, version)
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
| `internal/edge`      | Dependency edge queries: resolve import/symbol relationships between packages         |
| `internal/install`   | Hook installation: embed and write Claude Code session hooks into `.claude/hooks/`    |

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

## Recon (Code Intelligence)

This project uses [recon](https://github.com/robertguss/recon) for code
intelligence. Recon indexes Go source code into a local database and maintains a
knowledge layer — decisions, patterns, and relationships — that persists across
sessions. Each session builds on what previous sessions learned, rather than
starting from scratch.

A recon orient payload is automatically injected at session start via hook,
giving you project structure, hot modules, and active decisions upfront.

Recon is a two-way knowledge cycle: you **consume** knowledge (orient, find,
recall) and **produce** knowledge (decide, pattern). Recording what you discover
is as important as querying what's already known.

### When to use recon

- **When exploring or understanding the codebase** — `recon find` gives
  structured symbol lookups with dependencies, `--list-packages` shows package
  structure with activity heat, and `recon orient` gives a full project overview
  — all faster and richer than manual file exploration
- **When following established patterns** — `recon pattern --list` shows
  recorded conventions and `recon recall` surfaces how similar problems were
  solved before, so your code matches the project's style
- **When writing tests** — `recon find` shows a symbol's dependencies so you
  know what to mock, and `recon recall` surfaces existing testing patterns and
  conventions
- **Before modifying existing code** — `recon recall` surfaces decisions
  explaining why code is structured the way it is, preventing you from undoing
  intentional design
- **After discovering something significant** — record it with `recon decide` or
  `recon pattern` so future sessions benefit
- **After major code changes** — `recon sync` re-indexes the codebase

### Command reference

Run `recon <command> --help` for flags and usage. Use the `/recon` skill for the
full reference. All commands support `--json` for structured output.

Commands: `init`, `sync`, `orient`, `find`, `decide`, `pattern`, `recall`,
`status`, `edges`, `reset`, `version`

New flags (see `/recon` skill for full reference):

- `recon decide --archive <id>` (was `--delete`; `--delete` kept as hidden
  alias)
- `recon decide --update <id> --title "..."` and `--reasoning "..."`
- `recon pattern --archive <id>`, `--update <id> --title/--reasoning`
- `recon find --imports-of <pkg>` — list what a package imports
- `recon find --imported-by <pkg>` — list what imports a package
- `recon reset [--force]` — delete the database for a clean slate
