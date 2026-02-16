# Architecture

Recon is a Go CLI tool that indexes Go source code into a local SQLite database
and provides commands for navigation, search, knowledge recording, and
orientation.

## System Overview

```mermaid
graph TD
    User[User / AI Agent] --> CLI[CLI Layer<br/>internal/cli]
    CLI --> Init[init]
    CLI --> Sync[sync]
    CLI --> Orient[orient]
    CLI --> Find[find]
    CLI --> Decide[decide]
    CLI --> Pattern[pattern]
    CLI --> Recall[recall]
    CLI --> Status[status]

    Init --> DB[Database Layer<br/>internal/db]
    Init --> Install[Install<br/>internal/install]
    Sync --> Index[Index Service<br/>internal/index]
    Orient --> OrientSvc[Orient Service<br/>internal/orient]
    Find --> FindSvc[Find Service<br/>internal/find]
    Decide --> Knowledge[Knowledge Service<br/>internal/knowledge]
    Pattern --> PatternSvc[Pattern Service<br/>internal/pattern]
    Recall --> RecallSvc[Recall Service<br/>internal/recall]

    Index --> DB
    OrientSvc --> DB
    FindSvc --> DB
    Knowledge --> DB
    PatternSvc --> DB
    RecallSvc --> DB

    DB --> SQLite[(SQLite<br/>.recon/recon.db)]
```

## Entry Point

`cmd/recon/main.go` is the binary entry point. It delegates to
`internal/cli.NewRootCommand()` which builds a Cobra command tree with 8
subcommands. The entry point handles exit codes via `cli.ExitError`.

## Layers

### CLI Layer (`internal/cli/`)

The CLI layer owns user interaction: argument parsing, output formatting, and
error presentation. Each command file (`init.go`, `sync.go`, `find.go`, etc.)
defines a Cobra command with its flags and `RunE` handler.

Key responsibilities:

- Parse flags and arguments
- Open the database via `openExistingDB()`
- Call the appropriate domain service
- Format output (text or JSON via `output.go`)
- Return structured errors (`ExitError`, JSON error envelopes)

The CLI layer does not contain business logic. It's a thin adapter between user
input and domain services.

### Domain Services (`internal/`)

Each domain has its own package with a `Service` struct that wraps `*sql.DB`:

| Package              | Service             | Responsibility                                                     |
| -------------------- | ------------------- | ------------------------------------------------------------------ |
| `internal/index`     | `index.Service`     | Parse Go files, extract symbols/imports/deps, upsert into DB       |
| `internal/find`      | `find.Service`      | Symbol lookup, list mode, package listing                          |
| `internal/knowledge` | `knowledge.Service` | Decision lifecycle: propose, verify, promote, update, archive      |
| `internal/pattern`   | `pattern.Service`   | Pattern lifecycle: propose, verify, promote                        |
| `internal/recall`    | `recall.Service`    | Full-text search across decisions and patterns                     |
| `internal/orient`    | `orient.Service`    | Aggregate project context (summary, architecture, heat, decisions) |

Each service owns its SQL queries directly — there is no ORM, no shared query
builder, and no repository abstraction. This keeps queries co-located with the
logic that uses them.

### Database Layer (`internal/db/`)

- `db.go` — SQLite connection management using `modernc.org/sqlite` (pure-Go
  driver). Opens with `MaxOpenConns(1)` and `PRAGMA foreign_keys = ON`.
- `migrate.go` — Schema migrations using `golang-migrate`.
- `migrations/` — Numbered SQL migration files (up/down).
- `sync_state.go` — Sync state management (last sync time, commit, fingerprint).

The database lives at `<module-root>/.recon/recon.db`.

### Install Layer (`internal/install/`)

Handles Claude Code integration file installation:

- `InstallHook()` — Writes the SessionStart hook script
- `InstallSkill()` — Writes the `/recon` skill definition
- `InstallSettings()` — Configures hooks in `.claude/settings.json`
- `InstallClaudeSection()` — Appends/updates the Recon section in `CLAUDE.md`

Uses `embed.FS` to bundle asset files into the binary.

## Data Flow

### Indexing (`recon sync`)

```mermaid
sequenceDiagram
    participant CLI
    participant IndexSvc as index.Service
    participant Go as Go Parser
    participant DB as SQLite

    CLI->>IndexSvc: Sync(ctx, moduleRoot)
    IndexSvc->>Go: Parse all .go files
    Go-->>IndexSvc: AST (packages, symbols, imports)
    IndexSvc->>DB: Upsert packages
    IndexSvc->>DB: Upsert files (with hash)
    IndexSvc->>DB: Upsert symbols
    IndexSvc->>DB: Upsert imports
    IndexSvc->>DB: Upsert symbol_deps
    IndexSvc->>DB: Update sync_state
    IndexSvc-->>CLI: SyncResult
```

### Decision Recording (`recon decide`)

```mermaid
sequenceDiagram
    participant CLI
    participant KnowledgeSvc as knowledge.Service
    participant DB as SQLite

    CLI->>KnowledgeSvc: ProposeAndVerifyDecision(input)
    KnowledgeSvc->>DB: Insert proposal
    KnowledgeSvc->>KnowledgeSvc: Run evidence check
    alt Check passes
        KnowledgeSvc->>DB: Insert decision
        KnowledgeSvc->>DB: Insert evidence (with baseline)
        KnowledgeSvc->>DB: Update FTS index
        KnowledgeSvc->>DB: Promote proposal
        KnowledgeSvc-->>CLI: Promoted=true
    else Check fails
        KnowledgeSvc-->>CLI: Promoted=false, details
    end
```

### Orient Context (`recon orient`)

```mermaid
sequenceDiagram
    participant CLI
    participant OrientSvc as orient.Service
    participant DB as SQLite
    participant Git as Git

    CLI->>OrientSvc: Build(ctx, opts)
    OrientSvc->>DB: Load summary (file/symbol/package counts)
    OrientSvc->>DB: Load modules (packages with stats)
    OrientSvc->>DB: Load active decisions
    OrientSvc->>DB: Load active patterns
    OrientSvc->>DB: Load architecture (entry points)
    OrientSvc->>Git: Check current commit
    OrientSvc->>Git: Get recent file activity
    OrientSvc->>OrientSvc: Calculate module heat
    OrientSvc-->>CLI: Payload
```

## Key Design Decisions

### Pure-Go SQLite

Using `modernc.org/sqlite` instead of `mattn/go-sqlite3` because:

- No CGO dependency — builds anywhere Go builds
- Single binary distribution with no native library requirements
- Slightly slower but acceptable for local CLI usage

### No ORM

Each service writes raw SQL queries. This provides:

- Full control over query optimization
- No abstraction leakage
- Queries are co-located with the logic that uses them
- Easy to understand and debug

### Function-Var Injection

Testability is achieved via package-level function variables:

```go
var osGetwd = os.Getwd

// In tests:
osGetwd = func() (string, error) { return "/mock/path", nil }
```

This avoids interfaces and dependency injection frameworks while keeping tests
isolated.

### Agent-First Design

Recon is designed primarily for AI coding agents:

- All commands support `--json` for machine-readable output
- `--no-prompt` disables interactive prompts for non-interactive use
- `--json-strict` suppresses warnings that could confuse JSON parsers
- The orient payload provides everything an agent needs to start working
- The SessionStart hook auto-injects context

### Single-File Database

SQLite with a single `.recon/recon.db` file means:

- No server to run or configure
- No network dependencies
- Database is local to the project
- Easy to reset (`rm .recon/recon.db`)
- Gitignored by default

## Module Structure

All code lives in `internal/` — nothing is exported outside the module. The
public surface is the CLI binary only.

```
cmd/recon/main.go          → Entry point
internal/cli/              → CLI commands (Cobra)
internal/db/               → Database management
internal/index/            → Code indexing
internal/find/             → Symbol search
internal/knowledge/        → Decision management
internal/pattern/          → Pattern management
internal/recall/           → Knowledge retrieval
internal/orient/           → Context aggregation
internal/install/          → Claude Code integration
```
