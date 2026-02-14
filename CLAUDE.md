# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What is Recon

Recon is a code intelligence and knowledge CLI for Go repositories. It indexes
Go source code into a local SQLite database (`.recon/recon.db`), then provides
commands to query symbols, record architectural decisions with evidence
verification, and serve startup context for AI coding agents.

## Build & Development Commands

```bash
just build          # Build binary to ./bin/recon
just install        # Install to GOPATH/bin
just run <args>     # Run via go run, forwarding args
just test           # Run full test suite (go test ./...)
just test-race      # Run tests with race detector
just cover          # Generate coverage.out and print summary
just fmt            # Format all Go packages
just db-reset       # Delete local SQLite database
```

Run a single test:

```bash
go test ./internal/find -run TestFindExact -v
```

## Architecture

### CLI Layer (`cmd/recon/`, `internal/cli/`)

Cobra-based CLI. `cmd/recon/main.go` calls `cli.NewRootCommand()` which
registers all subcommands. Each command file in `internal/cli/` wires flags,
opens the DB via `openExistingDB()`, instantiates the appropriate service, and
formats output (text or `--json`).

Commands: `init`, `sync`, `orient`, `find`, `decide`, `recall`.

### Service Layer (`internal/`)

Each domain has its own package with a `Service` struct created via
`NewService(conn)`:

- **`internal/index`** — Parses Go source with `go/ast`, extracts symbols
  (funcs, methods, types, vars, consts) with bodies and dependency edges, writes
  to SQLite. `Sync()` is the main entry point.
- **`internal/find`** — Queries indexed symbols by name with optional filters
  (package, file, kind). Returns symbol details and direct in-project
  dependencies.
- **`internal/orient`** — Builds a startup context payload (freshness, modules,
  recent decisions) for AI agents. Handles stale-index detection and optional
  auto-sync.
- **`internal/knowledge`** — `ProposeAndVerifyDecision()` records decisions with
  evidence checks (`file_exists`, `symbol_exists`, `grep_pattern`).
  Auto-promotes when verification passes.
- **`internal/recall`** — Searches promoted decisions by keyword.

### Database Layer (`internal/db/`)

SQLite via `modernc.org/sqlite` (pure Go, no CGO). Migrations use
`golang-migrate` with embedded SQL files in `internal/db/migrations/`.
`db.Open()` enforces `MaxOpenConns(1)` and enables foreign keys.

### Testability Pattern

Services and OS calls are injected via package-level `var` functions (e.g.,
`var runSync = ...`, `var osGetwd = os.Getwd`). Tests override these to inject
mocks or sqlmock instances. Both sqlmock-based unit tests and integration tests
using real SQLite exist side-by-side.

## ExecPlans

Feature work follows the ExecPlan process defined in `.agent/PLANS.md`. Plans
are living documents stored in `docs/plans/` with required sections: Progress,
Surprises & Discoveries, Decision Log, and Outcomes & Retrospective. Plans must
be fully self-contained and follow strict TDD.

## Conventions

- All commands support `--json` for machine-readable output with structured
  error codes (`not_found`, `ambiguous`, `invalid_input`, `internal_error`).
- `ExitError{Code, Message}` is the standard way to signal non-zero exit codes
  from command handlers.
- Module root is auto-detected by walking up to find `go.mod`; falls back to
  cwd.
- The `.recon/` directory and its database are local per-repo, gitignored.
