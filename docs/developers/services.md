# Services

Each domain in Recon has its own package under `internal/` with a `Service`
struct wrapping `*sql.DB`. Services own their SQL queries directly — no ORM, no
shared query builder.

## index.Service

**Package:** `internal/index`

Parses Go source files and indexes them into the database.

### Methods

**`Sync(ctx, moduleRoot) (SyncResult, error)`**

Full indexing pass: walks the module directory, parses all `.go` files, and
upserts packages, files, symbols, imports, and symbol dependencies. Returns
counts and a git fingerprint.

### Types

```go
type SyncResult struct {
    IndexedFiles    int
    IndexedSymbols  int
    IndexedPackages int
    Fingerprint     string
    Commit          string
    Dirty           bool
    SyncedAt        time.Time
}
```

### Helpers

The index package also provides standalone functions used by the CLI layer:

- `FindModuleRoot(dir) (string, error)` — Walks up the directory tree to find
  `go.mod`

## find.Service

**Package:** `internal/find`

Symbol lookup and listing.

### Methods

**`Find(ctx, symbol, opts) (Result, error)`**

Exact symbol lookup with optional filtering by package, file, or kind. Returns
the symbol with its direct dependencies. Returns `NotFoundError` if no match,
`AmbiguousError` if multiple matches.

**`FindExact(ctx, symbol) (Result, error)`**

Convenience wrapper for `Find` with no filters.

**`List(ctx, opts, limit) (ListResult, error)`**

List symbols matching filter criteria without a specific symbol name.

**`ListPackages(ctx) ([]PackageSummary, error)`**

List all indexed packages with file and line counts.

### Types

```go
type Symbol struct {
    ID, LineStart, LineEnd int64
    Kind, Name, Signature, Body, Receiver, FilePath, Package string
}

type Result struct {
    Symbol       Symbol
    Dependencies []Symbol
}

type QueryOptions struct {
    PackagePath string
    FilePath    string
    Kind        string
}

type ListResult struct {
    Symbols []Symbol
    Total   int
    Limit   int
}

type PackageSummary struct {
    Path      string
    Name      string
    FileCount int
    LineCount int
}
```

### Error Types

```go
type NotFoundError struct {
    Symbol      string
    Suggestions []string
    Filtered    bool
    Filters     QueryOptions
}

type AmbiguousError struct {
    Symbol     string
    Candidates []Candidate
}
```

## knowledge.Service

**Package:** `internal/knowledge`

Decision lifecycle management.

### Methods

**`ProposeAndVerifyDecision(ctx, input) (ProposeDecisionResult, error)`**

Full lifecycle in one call: creates a proposal, runs the evidence check, and
promotes to an active decision if the check passes. Records a baseline snapshot
when verification succeeds.

**`ListDecisions(ctx) ([]DecisionListItem, error)`**

List all active decisions with their confidence and drift status.

**`ArchiveDecision(ctx, id) error`**

Soft-delete a decision by setting its status to `archived`.

**`UpdateConfidence(ctx, id, confidence) error`**

Update a decision's confidence level. Validates that confidence is one of `low`,
`medium`, `high`.

**`DecayConfidenceOnDrift(ctx) (int, error)`**

Batch operation: for all decisions with drifting evidence, step down their
confidence (`high` → `medium`, `medium` → `low`). Returns the count of affected
decisions.

**`RunCheckPublic(ctx, checkType, checkSpec, moduleRoot) CheckOutcome`**

Run an evidence check without creating any state. Used by the `--dry-run` flag.

### Evidence Check Types

The service supports three check types:

| Type            | Spec Format                             | What It Does                                      |
| --------------- | --------------------------------------- | ------------------------------------------------- |
| `file_exists`   | `{"path": "relative/path"}`             | Checks that a file exists relative to module root |
| `symbol_exists` | `{"name": "SymbolName"}`                | Queries the database for a matching symbol        |
| `grep_pattern`  | `{"pattern": "regex", "scope": "glob"}` | Runs a regex match across files (optional scope)  |

### Types

```go
type ProposeDecisionInput struct {
    Title, Reasoning, Confidence string
    EvidenceSummary, CheckType, CheckSpec, ModuleRoot string
}

type ProposeDecisionResult struct {
    ProposalID, DecisionID     int64
    Promoted, VerificationPassed bool
    VerificationDetails        string
}

type DecisionListItem struct {
    ID                            int64
    Title, Confidence, Status, Drift, UpdatedAt string
}

type CheckOutcome struct {
    Passed  bool
    Details string
    Baseline map[string]any
}
```

## pattern.Service

**Package:** `internal/pattern`

Pattern lifecycle management. Follows the same propose/verify/promote flow as
decisions.

### Methods

**`ProposeAndVerifyPattern(ctx, input) (ProposePatternResult, error)`**

Creates a proposal, runs the evidence check, and promotes if verification
passes. Uses the same evidence check infrastructure as `knowledge.Service`.

### Types

```go
type ProposePatternInput struct {
    Title, Description, Example, Confidence string
    EvidenceSummary, CheckType, CheckSpec, ModuleRoot string
}

type ProposePatternResult struct {
    ProposalID, PatternID        int64
    Promoted, VerificationPassed bool
    VerificationDetails          string
}
```

## recall.Service

**Package:** `internal/recall`

Full-text search across promoted decisions and patterns.

### Methods

**`Recall(ctx, query, opts) (Result, error)`**

Search strategy:

1. Try FTS5 match (with Porter stemming, ranked by relevance)
2. If FTS fails or returns no results, fall back to LIKE query
3. Both strategies include a legacy path for databases without the patterns
   table (pre-migration-003)

The service searches across:

- Decision titles, reasoning, and evidence summaries
- Pattern titles, descriptions, and evidence summaries

Only active entities are returned (archived items excluded).

### Types

```go
type RecallOptions struct {
    Limit int  // defaults to 10 if ≤ 0
}

type Item struct {
    DecisionID, PatternID            int64
    EntityType, Title, Reasoning     string
    Confidence, UpdatedAt            string
    EvidenceSummary, EvidenceDrift    string
}

type Result struct {
    Query string
    Items []Item
}
```

## orient.Service

**Package:** `internal/orient`

Aggregates project context into a structured payload.

### Methods

**`Build(ctx, opts) (Payload, error)`**

Builds the full orient payload by querying multiple database tables and running
git commands:

1. Load project info from `go.mod`
2. Load summary counts (files, symbols, packages, decisions)
3. Load module list with file/line counts
4. Load active decisions with drift status
5. Load active patterns with drift status
6. Detect architecture (entry points, dependency flow)
7. Calculate module heat from git log (30-day window)
8. Get recent file activity from git

### Types

```go
type BuildOptions struct {
    ModuleRoot   string
    MaxModules   int
    MaxDecisions int
}

type Payload struct {
    Project         ProjectInfo
    Architecture    Architecture
    Freshness       Freshness
    Summary         Summary
    Modules         []ModuleSummary
    ActiveDecisions []DecisionDigest
    ActivePatterns  []PatternDigest
    RecentActivity  []RecentFile
    Warnings        []string
}
```

The `Payload` type is the primary output for agents — it contains everything
needed to understand the project state at a glance.

## Common Patterns

### Service Construction

All services follow the same pattern:

```go
type Service struct {
    db *sql.DB
}

func NewService(conn *sql.DB) *Service {
    return &Service{db: conn}
}
```

### Error Handling

Services wrap SQL errors with `fmt.Errorf` and `%w` for error chain
preservation. The CLI layer catches domain-specific error types (like
`find.NotFoundError`) and formats them for output.

### Query Style

Services write raw SQL directly in their methods. No query builders, no ORMs.
Complex queries use `LEFT JOIN` and `COALESCE` for optional fields. FTS5 queries
use `MATCH` with `ORDER BY rank`.
