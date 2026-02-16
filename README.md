# Recon

Code intelligence and knowledge CLI for Go repositories.

Recon indexes Go source code — packages, files, symbols, imports, and
dependencies — into a local SQLite database and provides commands for
navigating, searching, recording decisions with evidence, detecting patterns,
and orienting within a codebase.

## Quick Start

```bash
# Install
go install github.com/robertguss/recon/cmd/recon@latest

# Initialize in your Go project
cd your-go-project
recon init

# Index your codebase
recon sync

# Get project context
recon orient
```

## What Recon Does

**Index and search** — Parse Go source files and query symbols, files, imports,
and dependencies with structured output instead of regex guessing.

```bash
recon find HandleRequest                    # exact symbol lookup
recon find --kind func --package ./api/     # list functions in a package
recon find Receiver.Method                  # dot syntax for methods
```

**Record decisions** — Capture architectural decisions with evidence that is
automatically verified against the codebase. Decisions track confidence levels
and detect drift when the code changes.

```bash
recon decide "Use Cobra for CLI" \
  --reasoning "industry standard Go CLI framework" \
  --evidence-summary "go.mod contains cobra" \
  --check-type file_exists --check-path go.mod
```

**Detect patterns** — Record recurring code patterns with verification checks.

```bash
recon pattern "Error wrapping with %w" \
  --description "All errors wrapped with fmt.Errorf and %w" \
  --evidence-summary "grep finds consistent %w usage" \
  --check-type grep_pattern --check-pattern "Errorf.*%%w"
```

**Recall knowledge** — Full-text search across recorded decisions and patterns.

```bash
recon recall "error handling"
recon recall "database"
```

**Orient** — Get a structured context payload: project summary, architecture,
module heat map, active decisions, recent activity.

```bash
recon orient              # human-readable text
recon orient --json       # structured JSON for agents
```

## Commands

| Command         | Purpose                                                               |
| --------------- | --------------------------------------------------------------------- |
| `recon init`    | Initialize `.recon/` directory, database, and Claude Code integration |
| `recon sync`    | Index Go source code into the database                                |
| `recon orient`  | Project context: structure, activity, decisions, patterns             |
| `recon find`    | Search symbols, files, imports with filtering                         |
| `recon decide`  | Record decisions with evidence verification                           |
| `recon pattern` | Record recurring code patterns                                        |
| `recon recall`  | Full-text search across decisions and patterns                        |
| `recon status`  | Quick health check                                                    |

All commands support `--json` for machine-readable output and `--no-prompt` to
disable interactive prompts.

See [docs/users/commands.md](docs/users/commands.md) for the complete CLI
reference.

## Claude Code Integration

Recon is designed to work as a knowledge layer for AI coding agents. Running
`recon init` installs:

- A **SessionStart hook** that runs `recon orient --json-strict --auto-sync` at
  the start of each Claude Code session
- A **skill** (`/recon`) for structured symbol lookup and decision recording
- **Settings** that allow the recon tool to run without permission prompts

See
[docs/users/claude-code-integration.md](docs/users/claude-code-integration.md)
for details.

## Requirements

- Go 1.26+
- A Go module (project must have a `go.mod` file)

## Documentation

**For users:**

- [Getting Started](docs/users/getting-started.md) — Installation, setup, first
  workflow
- [Commands Reference](docs/users/commands.md) — Every command, flag, and option
- [Workflows](docs/users/workflows.md) — Decision lifecycle, pattern detection,
  recall
- [Claude Code Integration](docs/users/claude-code-integration.md) — Hook,
  skill, settings
- [Troubleshooting](docs/users/troubleshooting.md) — Common errors and solutions

**For developers:**

- [Architecture](docs/developers/architecture.md) — System design and component
  relationships
- [Database Schema](docs/developers/schema.md) — Tables, relationships, FTS5
  strategy
- [Services](docs/developers/services.md) — Domain service APIs and patterns
- [Testing](docs/developers/testing.md) — Test strategy, sqlmock vs real SQLite
- [Contributing](docs/developers/contributing.md) — Dev setup, conventions, PR
  workflow
- [ADRs](docs/developers/adr/) — Architecture Decision Records

## Development

This project uses [just](https://github.com/casey/just) as a command runner.

```bash
just build          # Build binary to ./bin/recon
just install        # Install to GOPATH/bin
just test           # Run full test suite
just test-race      # Run tests with race detector
just cover          # Generate coverage report
just fmt            # Format all Go files
```

## License

See [LICENSE](LICENSE) for details.
