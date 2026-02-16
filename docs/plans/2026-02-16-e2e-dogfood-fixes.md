# E2E Dogfood Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Fix all P1/P2 issues found during E2E dogfood testing of recon
commands against both recon and cortex_code repos.

**Architecture:** Each fix is isolated to its own service/CLI layer. Pattern
lifecycle parity (list, delete, update) mirrors the existing decide command. The
grep_pattern bug is in `internal/knowledge/service.go`. Sync diff reporting adds
fields to the existing `SyncResult` struct. Dependency flow restructuring
changes the orient `Architecture` struct.

**Tech Stack:** Go, SQLite, Cobra CLI, go-sqlmock for testing

---

### Task 1: Fix grep_pattern verification — search source file content, not file paths

The `runGrepPattern` method in `internal/knowledge/service.go:392-441` calls
`index.CollectEligibleGoFiles` which skips `_test.go` files. The grep regex is
matched against `f.Content` which is correct. However, during E2E testing,
`grep_pattern "var osGetwd"` matched 0 of 27 files even though
`var osGetwd = os.Getwd` exists in `internal/cli/root.go`.

**Root cause investigation needed:** The files count was 27 which matches the
non-test file count. The pattern definitely exists in root.go. Possible issue:
the regex might need to match against string(f.Content) or there's a byte
encoding issue. Debug by adding a dry-run test.

**Files:**

- Debug: `internal/knowledge/service.go:392-441` (runGrepPattern)
- Test: `internal/knowledge/service_test.go`

**Step 1: Write a failing test that reproduces the bug**

```go
func TestRunGrepPattern_FindsVarDeclaration(t *testing.T) {
    // Create a temp module with a Go file containing "var osGetwd = os.Getwd"
    tmpDir := t.TempDir()
    os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644)
    os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nvar osGetwd = os.Getwd\n"), 0644)

    conn := setupTestDB(t)
    svc := knowledge.NewService(conn)

    spec := `{"pattern":"var osGetwd"}`
    outcome := svc.RunCheckPublic(context.Background(), "grep_pattern", spec, tmpDir)

    if !outcome.Passed {
        t.Fatalf("expected grep_pattern to pass, got: %s", outcome.Details)
    }
}
```

**Step 2: Run test to verify it fails (or passes — confirming this is
environment-specific)**

Run:
`go test ./internal/knowledge/... -run TestRunGrepPattern_FindsVarDeclaration -v`

**Step 3: If the test passes, the bug is environment-specific — investigate the
E2E context**

The issue may be that when run from the recon repo root, the
`CollectEligibleGoFiles` is returning files but the pattern isn't matching due
to the indexed file set vs source file set divergence. Test with the actual
recon module root:

```sh
./bin/recon decide --dry-run --check-type grep_pattern --check-pattern "var osGetwd" --json
```

If dry-run passes, the issue was with the E2E test context (stale index). If it
fails, dig deeper into the byte matching.

**Step 4: Fix the root cause and verify**

Run: `go test ./internal/knowledge/... -v`

**Step 5: Commit**

```bash
git add internal/knowledge/service.go internal/knowledge/service_test.go
git commit -m "fix(knowledge): fix grep_pattern verification to correctly match source content"
```

---

### Task 2: Add `--list` flag to pattern command

Mirrors `decide --list`. The pattern service needs a `ListPatterns` method and
the CLI needs a `--list` flag.

**Files:**

- Modify: `internal/pattern/service.go` — add `ListPatterns` method
- Modify: `internal/cli/pattern.go` — add `--list` flag and list mode
- Test: `internal/pattern/service_test.go`
- Test: `internal/cli/pattern_test.go` (if it exists, otherwise test via CLI
  execution)

**Step 1: Write the failing test for pattern service ListPatterns**

```go
func TestListPatterns_ReturnsActivePatterns(t *testing.T) {
    conn := setupTestDB(t)
    svc := NewService(conn)

    // Insert a test pattern directly
    now := time.Now().UTC().Format(time.RFC3339)
    conn.ExecContext(context.Background(),
        `INSERT INTO patterns (title, description, confidence, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
        "Test pattern", "desc", "high", now, now)

    items, err := svc.ListPatterns(context.Background())
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 1 {
        t.Fatalf("expected 1 pattern, got %d", len(items))
    }
    if items[0].Title != "Test pattern" {
        t.Fatalf("expected title 'Test pattern', got %q", items[0].Title)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pattern/... -run TestListPatterns -v` Expected: FAIL —
`ListPatterns` method does not exist

**Step 3: Implement ListPatterns in pattern service**

Add to `internal/pattern/service.go`:

```go
type PatternListItem struct {
    ID         int64  `json:"id"`
    Title      string `json:"title"`
    Confidence string `json:"confidence"`
    Status     string `json:"status"`
    Drift      string `json:"drift_status"`
    UpdatedAt  string `json:"updated_at"`
}

func (s *Service) ListPatterns(ctx context.Context) ([]PatternListItem, error) {
    rows, err := s.db.QueryContext(ctx, `
SELECT p.id, p.title, p.confidence, p.status, COALESCE(e.drift_status, 'ok'), p.updated_at
FROM patterns p
LEFT JOIN evidence e ON e.entity_type = 'pattern' AND e.entity_id = p.id
WHERE p.status = 'active'
ORDER BY p.updated_at DESC;
`)
    if err != nil {
        return nil, fmt.Errorf("query patterns: %w", err)
    }
    defer rows.Close()
    items := []PatternListItem{}
    for rows.Next() {
        var item PatternListItem
        if err := rows.Scan(&item.ID, &item.Title, &item.Confidence, &item.Status, &item.Drift, &item.UpdatedAt); err != nil {
            return nil, fmt.Errorf("scan pattern: %w", err)
        }
        items = append(items, item)
    }
    return items, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pattern/... -run TestListPatterns -v` Expected: PASS

**Step 5: Add --list flag to CLI pattern command**

Modify `internal/cli/pattern.go` — add `listFlag` variable and list mode handler
at the top of RunE (same structure as decide.go:37-67):

```go
// Add variable
var listFlag bool

// Add to RunE, before existing logic:
if listFlag {
    conn, err := openExistingDB(app)
    if err != nil {
        if jsonOut {
            return exitJSONCommandError(err)
        }
        return err
    }
    defer conn.Close()

    items, err := pattern.NewService(conn).ListPatterns(cmd.Context())
    if err != nil {
        if jsonOut {
            _ = writeJSONError("internal_error", err.Error(), nil)
            return ExitError{Code: 2}
        }
        return err
    }

    if jsonOut {
        return writeJSON(items)
    }
    if len(items) == 0 {
        fmt.Println("No active patterns.")
        return nil
    }
    for _, item := range items {
        fmt.Printf("#%d %s (confidence=%s, drift=%s)\n", item.ID, item.Title, item.Confidence, item.Drift)
    }
    return nil
}

// Add flag registration
cmd.Flags().BoolVar(&listFlag, "list", false, "List active patterns")
```

Also change `Args: cobra.ExactArgs(1)` to `Args: cobra.MaximumNArgs(1)` and add
the same title-required check as decide.go:187-194 in the propose section.

**Step 6: Run full test suite**

Run: `go test ./internal/pattern/... ./internal/cli/... -v` Expected: PASS

**Step 7: Commit**

```bash
git add internal/pattern/service.go internal/pattern/service_test.go internal/cli/pattern.go
git commit -m "feat(pattern): add --list flag to pattern command"
```

---

### Task 3: Add `--delete` flag to pattern command

Mirrors `decide --delete`. Needs `ArchivePattern` in pattern service and
`--delete` flag in CLI.

**Files:**

- Modify: `internal/pattern/service.go` — add `ArchivePattern` method
- Modify: `internal/cli/pattern.go` — add `--delete` flag
- Test: `internal/pattern/service_test.go`

**Step 1: Write the failing test for ArchivePattern**

```go
func TestArchivePattern_SoftDeletes(t *testing.T) {
    conn := setupTestDB(t)
    svc := NewService(conn)

    now := time.Now().UTC().Format(time.RFC3339)
    res, _ := conn.ExecContext(context.Background(),
        `INSERT INTO patterns (title, description, confidence, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
        "To archive", "desc", "medium", now, now)
    id, _ := res.LastInsertId()

    err := svc.ArchivePattern(context.Background(), id)
    if err != nil {
        t.Fatal(err)
    }

    // Verify it's archived
    var status string
    conn.QueryRowContext(context.Background(), "SELECT status FROM patterns WHERE id = ?", id).Scan(&status)
    if status != "archived" {
        t.Fatalf("expected status 'archived', got %q", status)
    }
}

func TestArchivePattern_NotFound(t *testing.T) {
    conn := setupTestDB(t)
    svc := NewService(conn)

    err := svc.ArchivePattern(context.Background(), 9999)
    if err == nil {
        t.Fatal("expected error for non-existent pattern")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pattern/... -run TestArchivePattern -v` Expected: FAIL
— `ArchivePattern` does not exist

**Step 3: Implement ArchivePattern**

Add to `internal/pattern/service.go`:

```go
var ErrNotFound = fmt.Errorf("not found")

func (s *Service) ArchivePattern(ctx context.Context, id int64) error {
    res, err := s.db.ExecContext(ctx,
        `UPDATE patterns SET status = 'archived', updated_at = ? WHERE id = ? AND status = 'active';`,
        time.Now().UTC().Format(time.RFC3339), id)
    if err != nil {
        return fmt.Errorf("archive pattern: %w", err)
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return fmt.Errorf("pattern %d: %w", id, ErrNotFound)
    }
    return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pattern/... -run TestArchivePattern -v` Expected: PASS

**Step 5: Add --delete flag to CLI**

Add to `internal/cli/pattern.go` RunE (after list mode, before propose mode):

```go
var deleteID int64

// In RunE:
if deleteID > 0 {
    conn, err := openExistingDB(app)
    if err != nil {
        if jsonOut {
            return exitJSONCommandError(err)
        }
        return err
    }
    defer conn.Close()

    err = pattern.NewService(conn).ArchivePattern(cmd.Context(), deleteID)
    if err != nil {
        if jsonOut {
            code := "internal_error"
            if errors.Is(err, pattern.ErrNotFound) {
                code = "not_found"
            }
            _ = writeJSONError(code, err.Error(), map[string]any{"id": deleteID})
            return ExitError{Code: 2}
        }
        return err
    }
    if jsonOut {
        return writeJSON(map[string]any{"archived": true, "id": deleteID})
    }
    fmt.Printf("Pattern %d archived.\n", deleteID)
    return nil
}

// Add flag:
cmd.Flags().Int64Var(&deleteID, "delete", 0, "Archive (soft-delete) a pattern by ID")
```

**Step 6: Run full test suite**

Run: `go test ./internal/pattern/... ./internal/cli/... -v` Expected: PASS

**Step 7: Commit**

```bash
git add internal/pattern/service.go internal/pattern/service_test.go internal/cli/pattern.go
git commit -m "feat(pattern): add --delete flag for pattern lifecycle management"
```

---

### Task 4: Add sync diff reporting (what changed since last sync)

Currently `sync` only reports totals. Add fields showing what changed: files
added/removed/modified, symbols added/removed.

**Files:**

- Modify: `internal/index/service.go` — add diff computation to Sync
- Modify: `internal/cli/sync.go` — display diff in text mode
- Test: `internal/index/service_test.go`

**Step 1: Write the failing test**

```go
func TestSync_ReportsDiff(t *testing.T) {
    conn := setupTestDB(t)
    svc := NewService(conn)
    tmpDir := createTestModule(t) // helper that creates go.mod + a main.go

    // First sync
    result1, err := svc.Sync(context.Background(), tmpDir)
    if err != nil {
        t.Fatal(err)
    }

    // Add a file
    os.WriteFile(filepath.Join(tmpDir, "extra.go"), []byte("package main\n\nfunc Extra() {}\n"), 0644)

    // Second sync
    result2, err := svc.Sync(context.Background(), tmpDir)
    if err != nil {
        t.Fatal(err)
    }

    if result2.Diff == nil {
        t.Fatal("expected diff to be populated on re-sync")
    }
    if result2.Diff.FilesAdded != 1 {
        t.Fatalf("expected 1 file added, got %d", result2.Diff.FilesAdded)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/index/... -run TestSync_ReportsDiff -v` Expected: FAIL
— `Diff` field does not exist on SyncResult

**Step 3: Implement diff computation**

Add to `internal/index/service.go`:

```go
type SyncDiff struct {
    FilesAdded     int `json:"files_added"`
    FilesRemoved   int `json:"files_removed"`
    FilesModified  int `json:"files_modified"`
    SymbolsBefore  int `json:"symbols_before"`
    SymbolsAfter   int `json:"symbols_after"`
    PackagesBefore int `json:"packages_before"`
    PackagesAfter  int `json:"packages_after"`
}
```

Add `Diff *SyncDiff` to `SyncResult` struct (pointer so it's omitted on first
sync).

In the `Sync` method, before the DELETE statements, query the current counts:

```go
var prevFiles, prevSymbols, prevPackages int
_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&prevFiles)
_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols").Scan(&prevSymbols)
_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages").Scan(&prevPackages)

// Also collect previous file hashes for diff
prevHashes := map[string]string{}
hashRows, _ := tx.QueryContext(ctx, "SELECT path, hash FROM files")
if hashRows != nil {
    for hashRows.Next() {
        var p, h string
        hashRows.Scan(&p, &h)
        prevHashes[p] = h
    }
    hashRows.Close()
}
```

After the sync completes (before tx.Commit), compute the diff:

```go
if prevFiles > 0 || prevSymbols > 0 {
    newPaths := map[string]string{}
    for _, f := range files {
        newPaths[f.RelPath] = f.Hash
    }

    added, removed, modified := 0, 0, 0
    for p, oldHash := range prevHashes {
        newHash, exists := newPaths[p]
        if !exists {
            removed++
        } else if oldHash != newHash {
            modified++
        }
    }
    for p := range newPaths {
        if _, existed := prevHashes[p]; !existed {
            added++
        }
    }

    diff = &SyncDiff{
        FilesAdded: added, FilesRemoved: removed, FilesModified: modified,
        SymbolsBefore: prevSymbols, SymbolsAfter: symbolCount,
        PackagesBefore: prevPackages, PackagesAfter: len(packageStats),
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/index/... -run TestSync_ReportsDiff -v` Expected: PASS

**Step 5: Update CLI sync output**

Modify `internal/cli/sync.go` text output to include diff when available:

```go
if result.Diff != nil {
    fmt.Printf("Changes: +%d files, -%d files, ~%d modified\n",
        result.Diff.FilesAdded, result.Diff.FilesRemoved, result.Diff.FilesModified)
    fmt.Printf("Symbols: %d → %d | Packages: %d → %d\n",
        result.Diff.SymbolsBefore, result.Diff.SymbolsAfter,
        result.Diff.PackagesBefore, result.Diff.PackagesAfter)
}
```

**Step 6: Run full test suite**

Run: `go test ./... -count=1` Expected: PASS

**Step 7: Commit**

```bash
git add internal/index/service.go internal/index/service_test.go internal/cli/sync.go
git commit -m "feat(sync): report file and symbol diff on re-sync"
```

---

### Task 5: Restructure orient dependency_flow from string to structured data

Currently `dependency_flow` is a semicolon-delimited string. Change it to an
array of `{from, to}` edges for programmatic consumption.

**Files:**

- Modify: `internal/orient/service.go` — change `Architecture.DependencyFlow`
  type
- Modify: `internal/orient/render.go` — update text rendering to format from new
  structure
- Test: `internal/orient/service_test.go`
- Test: `internal/orient/render_test.go`

**Step 1: Write the failing test**

```go
func TestBuild_DependencyFlowIsStructured(t *testing.T) {
    // ... setup test DB with packages and imports ...

    payload, err := svc.Build(ctx, orient.BuildOptions{ModuleRoot: tmpDir})
    if err != nil {
        t.Fatal(err)
    }

    if len(payload.Architecture.DependencyFlow) == 0 {
        t.Fatal("expected structured dependency flow")
    }
    // Verify it's an array of edges, not a string
    edge := payload.Architecture.DependencyFlow[0]
    if edge.From == "" || len(edge.To) == 0 {
        t.Fatal("expected non-empty from and to fields")
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`go test ./internal/orient/... -run TestBuild_DependencyFlowIsStructured -v`
Expected: FAIL — type mismatch

**Step 3: Change Architecture struct and update loadArchitecture**

In `internal/orient/service.go`:

```go
type DependencyEdge struct {
    From string   `json:"from"`
    To   []string `json:"to"`
}

type Architecture struct {
    EntryPoints    []string         `json:"entry_points"`
    DependencyFlow []DependencyEdge `json:"dependency_flow"`
}
```

Update `loadArchitecture` to return `[]DependencyEdge` instead of calling
`formatDependencyFlow`:

```go
edges := make([]DependencyEdge, 0, len(flowParts))
for from, tos := range flowParts {
    sort.Strings(tos)
    edges = append(edges, DependencyEdge{From: from, To: tos})
}
sort.Slice(edges, func(i, j int) bool {
    return edges[i].From < edges[j].From
})
payload.Architecture = Architecture{EntryPoints: entryPoints, DependencyFlow: edges}
```

**Step 4: Update render.go text output**

In `internal/orient/render.go`, update the dependency flow rendering to format
from the new structure, producing the same human-readable string as before:

```go
// Format edges as "from → to" or "from → {to1, to2}"
parts := make([]string, 0, len(payload.Architecture.DependencyFlow))
for _, edge := range payload.Architecture.DependencyFlow {
    if len(edge.To) == 1 {
        parts = append(parts, edge.From+" → "+edge.To[0])
    } else {
        parts = append(parts, edge.From+" → {"+strings.Join(edge.To, ", ")+"}")
    }
}
depFlow := strings.Join(parts, "; ")
```

**Step 5: Run full test suite**

Run: `go test ./internal/orient/... -v` Expected: PASS

**Step 6: Run full test suite to catch any downstream breakage**

Run: `go test ./... -count=1` Expected: PASS

**Step 7: Commit**

```bash
git add internal/orient/service.go internal/orient/render.go internal/orient/service_test.go internal/orient/render_test.go
git commit -m "refactor(orient): restructure dependency_flow as structured edge array"
```

---

### Task 6: Improve find suggestions on not_found

Currently `find` suggestions only use prefix matching (`name LIKE ?%`). Add
fuzzy/substring matching and suggest using `--kind` or `--list-packages` when no
matches found.

**Files:**

- Modify: `internal/find/service.go:360-385` — improve `suggestions` method
- Modify: `internal/cli/find.go:126-134` — add guidance text
- Test: `internal/find/service_test.go`

**Step 1: Write the failing test**

```go
func TestSuggestions_SubstringMatch(t *testing.T) {
    conn := setupTestDB(t)
    // Insert a symbol named "ExitError"
    insertSymbol(t, conn, "ExitError", "type", "internal/cli/exit_error.go")

    svc := NewService(conn)
    // Search for "exit" (lowercase substring) — should suggest "ExitError"
    _, err := svc.Find(context.Background(), "exit", find.QueryOptions{})
    var nfe NotFoundError
    if !errors.As(err, &nfe) {
        t.Fatal("expected NotFoundError")
    }
    if len(nfe.Suggestions) == 0 {
        t.Fatal("expected at least one suggestion")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/find/... -run TestSuggestions_SubstringMatch -v`
Expected: FAIL — no suggestions returned for substring search

**Step 3: Improve suggestions method**

Replace the current prefix-only query with a two-pass approach:

```go
func (s *Service) suggestions(ctx context.Context, symbol string) ([]string, error) {
    // Pass 1: prefix match (fast, existing behavior)
    rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT name FROM symbols WHERE name LIKE ? ORDER BY name LIMIT 5;
`, symbol+"%")
    if err != nil {
        return nil, fmt.Errorf("query suggestions: %w", err)
    }
    defer rows.Close()

    out := make([]string, 0, 5)
    for rows.Next() {
        var name string
        if err := rows.Scan(&name); err != nil {
            return nil, fmt.Errorf("scan suggestion: %w", err)
        }
        out = append(out, name)
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate suggestions: %w", err)
    }

    // Pass 2: case-insensitive substring match if prefix found nothing
    if len(out) == 0 {
        rows2, err := s.db.QueryContext(ctx, `
SELECT DISTINCT name FROM symbols WHERE LOWER(name) LIKE LOWER(?) ORDER BY name LIMIT 5;
`, "%"+symbol+"%")
        if err != nil {
            return out, nil // non-fatal
        }
        defer rows2.Close()
        for rows2.Next() {
            var name string
            if err := rows2.Scan(&name); err != nil {
                break
            }
            out = append(out, name)
        }
    }

    return out, nil
}
```

**Step 4: Add guidance text to CLI find not_found output**

In `internal/cli/find.go`, after printing "not found" and suggestions, add:

```go
if len(e.Suggestions) == 0 {
    fmt.Println("Tip: try --kind func|type|var|method|const to browse, or --list-packages to see indexed packages")
}
```

For JSON mode, add a `"tip"` field to the details map when suggestions are
empty.

**Step 5: Run tests**

Run: `go test ./internal/find/... ./internal/cli/... -v` Expected: PASS

**Step 6: Commit**

```bash
git add internal/find/service.go internal/find/service_test.go internal/cli/find.go
git commit -m "feat(find): improve not-found suggestions with substring matching and guidance"
```

---

### Task 7: Add `recon version` command

Basic hygiene — shows binary version, Go version, and commit hash.

**Files:**

- Create: `internal/cli/version.go`
- Modify: `internal/cli/root.go` — register version command
- Test: via CLI execution

**Step 1: Write version command**

Create `internal/cli/version.go`:

```go
package cli

import (
    "fmt"
    "runtime"

    "github.com/spf13/cobra"
)

var (
    Version = "dev"
    Commit  = "unknown"
)

func newVersionCommand() *cobra.Command {
    var jsonOut bool

    cmd := &cobra.Command{
        Use:   "version",
        Short: "Print recon version information",
        RunE: func(cmd *cobra.Command, args []string) error {
            if jsonOut {
                return writeJSON(map[string]string{
                    "version":    Version,
                    "commit":     Commit,
                    "go_version": runtime.Version(),
                })
            }
            fmt.Printf("recon %s (commit %s, %s)\n", Version, Commit, runtime.Version())
            return nil
        },
    }
    cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
    return cmd
}
```

**Step 2: Register in root.go**

Add to `internal/cli/root.go` after existing AddCommand calls:

```go
root.AddCommand(newVersionCommand())
```

**Step 3: Wire ldflags in justfile build command**

Update the `just build` recipe to pass `-ldflags`:

```
go build -ldflags "-X github.com/robertguss/recon/internal/cli.Version={{version}} -X github.com/robertguss/recon/internal/cli.Commit=$(git rev-parse --short HEAD)" -o ./bin/recon ./cmd/recon
```

**Step 4: Test manually**

Run: `go run ./cmd/recon version --json` Expected:
`{"version":"dev","commit":"unknown","go_version":"go1.2X.X"}`

**Step 5: Run full test suite**

Run: `go test ./... -count=1` Expected: PASS

**Step 6: Commit**

```bash
git add internal/cli/version.go internal/cli/root.go justfile
git commit -m "feat(cli): add version command with build info"
```

---

### Task 8: Add orient freshness detail — show what changed since last sync

When orient reports staleness, include a summary of how many commits and files
changed.

**Files:**

- Modify: `internal/orient/service.go` — add `StaleSummary` field to `Freshness`
- Test: `internal/orient/service_test.go`

**Step 1: Write the failing test**

```go
func TestBuild_StaleFreshnessIncludesSummary(t *testing.T) {
    // ... setup with a stale index ...
    payload, err := svc.Build(ctx, opts)
    if err != nil {
        t.Fatal(err)
    }
    if payload.Freshness.IsStale && payload.Freshness.StaleSummary == "" {
        t.Fatal("expected stale summary when index is stale")
    }
}
```

**Step 2: Run to verify it fails**

Run:
`go test ./internal/orient/... -run TestBuild_StaleFreshnessIncludesSummary -v`

**Step 3: Implement stale summary**

Add to `Freshness` struct:

```go
type Freshness struct {
    // ... existing fields ...
    StaleSummary string `json:"stale_summary,omitempty"`
}
```

In the stale detection code in `Build`, after determining staleness reason is
`git_head_changed_since_last_sync`, compute the summary:

```go
if state.LastSyncCommit != "" && currentCommit != "" && state.LastSyncCommit != currentCommit {
    summary := computeStaleSummary(ctx, opts.ModuleRoot, state.LastSyncCommit, currentCommit)
    payload.Freshness = Freshness{
        // ... existing fields ...
        StaleSummary: summary,
    }
}

func computeStaleSummary(ctx context.Context, moduleRoot, fromCommit, toCommit string) string {
    // Count commits
    cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "rev-list", "--count", fromCommit+".."+toCommit)
    out, err := cmd.Output()
    if err != nil {
        return ""
    }
    commitCount := strings.TrimSpace(string(out))

    // Count changed files
    cmd2 := exec.CommandContext(ctx, "git", "-C", moduleRoot, "diff", "--name-only", fromCommit+".."+toCommit)
    out2, _ := cmd2.Output()
    fileCount := 0
    for _, line := range strings.Split(string(out2), "\n") {
        if strings.TrimSpace(line) != "" {
            fileCount++
        }
    }

    return fmt.Sprintf("%s commits, %d files changed since last sync", commitCount, fileCount)
}
```

**Step 4: Run tests**

Run: `go test ./internal/orient/... -v` Expected: PASS

**Step 5: Commit**

```bash
git add internal/orient/service.go internal/orient/service_test.go
git commit -m "feat(orient): add stale summary showing commits and files changed since last sync"
```

---

## Summary of Tasks

| Task | Priority | Description                                    |
| ---- | -------- | ---------------------------------------------- |
| 1    | P1       | Fix/investigate grep_pattern verification bug  |
| 2    | P1       | Add `--list` flag to pattern command           |
| 3    | P1       | Add `--delete` flag to pattern command         |
| 4    | P2       | Add sync diff reporting                        |
| 5    | P2       | Restructure dependency_flow as structured data |
| 6    | P3       | Improve find not-found suggestions             |
| 7    | P3       | Add version command                            |
| 8    | P3       | Add orient freshness detail                    |

**Dependencies:** Task 2 should be done before Task 3 (they both modify
`pattern.go`). All other tasks are independent and can be done in parallel.
