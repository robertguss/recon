# Contributing

Guide for developing and contributing to Recon.

## Setup

### Prerequisites

- Go 1.26+
- [just](https://github.com/casey/just) command runner

### Clone and Build

```bash
git clone https://github.com/robertguss/recon.git
cd recon
just build
```

### Verify

```bash
just test
./bin/recon --help
```

## Development Workflow

### Building

```bash
just build          # Build to ./bin/recon
just install        # Install to $GOPATH/bin
just run orient     # Run via go run with args
```

### Testing

```bash
just test           # Full test suite
just test-race      # With race detector
just cover          # Coverage report
just cover-html     # HTML coverage in browser
```

Run a specific package:

```bash
go test ./internal/find/...
go test ./internal/find/... -run TestFindExact -v
```

### Formatting

```bash
just fmt            # Format all Go files
```

## Project Structure

All code lives in `internal/`. The public surface is the CLI binary only.

```
cmd/recon/main.go       Entry point
internal/cli/           Cobra commands (one file per command)
internal/db/            Database management and migrations
internal/index/         Go code parser and indexer
internal/find/          Symbol search service
internal/knowledge/     Decision management service
internal/pattern/       Pattern management service
internal/recall/        Knowledge retrieval service
internal/orient/        Context aggregation service
internal/install/       Claude Code integration installer
```

## Conventions

### Code Style

- **No ORM** — Services write raw SQL directly
- **Function-var injection** — Package-level function vars for testability
- **Error wrapping** — Use `fmt.Errorf("context: %w", err)`
- **Dual output** — All commands support text and JSON modes
- **No exports** — Everything is `internal/`

### Commit Messages

Use Conventional Commits:

```
feat(find): add --list-packages flag
fix(decide): classify update JSON errors
test(orient): add sqlmock error coverage
refactor(cli): extract output helpers
docs: update architecture diagram
chore: bump dependencies
```

### Branch Naming

Feature branches follow the pattern: `feat/<description>` or
`feat/m<number>-<description>`.

### Pull Requests

PRs should include:

- Clear description of what changed and why
- All tests passing (`just test`)
- Coverage maintained at 100% (`just cover`)
- Formatted code (`just fmt`)

## Adding a New Command

1. Create `internal/cli/<command>.go` with a `newXxxCommand(app *App)` function
2. Register it in `internal/cli/root.go`
3. Add `--json` flag for machine-readable output
4. Write CLI tests in the same package
5. Update the skill file at `internal/install/assets/SKILL.md` if the command is
   user-facing for agents

## Adding a New Service Method

1. Add the method to the appropriate service in `internal/<package>/service.go`
2. Write SQL queries inline (no query builder)
3. Write real SQLite tests for happy paths
4. Write sqlmock tests for error paths
5. Target 100% coverage for the new code

## Database Changes

1. Create a new migration pair in `internal/db/migrations/`:
   - `000004_<name>.up.sql`
   - `000004_<name>.down.sql`
2. Test the migration with a fresh database
3. Update `docs/developers/schema.md` with the new tables/columns

## ExecPlans

For complex features or significant refactors, write an ExecPlan following the
format in `.agent/PLANS.md`. ExecPlans are living documents that track progress,
decisions, and surprises throughout implementation. They require 100% test
coverage.
