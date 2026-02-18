# Knowledge Completeness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Close the remaining open ergonomics gaps from the E2E dogfood audit:
full decide/pattern update, import/dep search in find, edge title display,
archive rename, find disambiguation hints, and a recon reset command.

**Architecture:** All changes are additive — new Service methods, extended CLI
flags, and one new CLI command. No schema migrations are required. Tests follow
the project's established patterns: real SQLite DBs for happy paths, go-sqlmock
for SQL error paths.

**Tech Stack:** Go, Cobra, modernc.org/sqlite, go-sqlmock (test only).

---

## Orientation

Recon is a Go CLI tool. All commands live in `internal/cli/`. Domain logic lives
in `internal/<domain>/service.go`. The project uses Cobra for CLI parsing,
SQLite for storage, and a strict TDD approach.

Key files to understand before starting:

- `internal/cli/decide.go` — decide command with list/delete/update modes
- `internal/cli/pattern.go` — pattern command (parallel structure to decide)
- `internal/cli/find.go` — find command with exact lookup and list modes
- `internal/cli/edges.go` — edges command
- `internal/cli/root.go` — root command that wires all subcommands
- `internal/knowledge/service.go` — decision domain: ProposeAndVerifyDecision,
  UpdateConfidence, ArchiveDecision, ListDecisions
- `internal/find/service.go` — find domain: Find, List, ListPackages; imports
  table available for new queries
- `internal/edge/service.go` — edge domain: Create, Delete, ListFrom, ListTo,
  ListAll
- `internal/db/db.go` — db helpers: DBPath, ReconDir, Open, EnsureReconDir

Database schema (relevant tables):

    decisions: id, title, reasoning, confidence, status, created_at, updated_at
    patterns:  id, title, description, confidence, status, created_at, updated_at
    evidence:  id, entity_type, entity_id, summary, check_type, check_spec,
               baseline, last_verified_at, last_result, drift_status
    search_index: FTS5 virtual table — title, content, entity_type, entity_id
    imports:   id, from_file_id, to_path, to_package_id, alias, import_type
    symbol_deps: id, symbol_id, dep_name, dep_package, dep_kind
    packages:  id, path, name, import_path, file_count, line_count, ...
    edges:     id, from_type, from_id, to_type, to_ref, relation, source,
               confidence, created_at

Test runner: `just test` (runs `go test ./...`). Single package:
`go test ./internal/knowledge/...`. Single test:
`go test ./internal/knowledge/... -run TestFoo`.

All test commands should be run from the repository root (`/path/to/recon`).

---

## Task 1: Full decide update — reasoning and title

**What:** `recon decide --update <id> --reasoning "new text"` and
`recon decide --update <id> --title "new title"` update those fields on an
existing active decision. Currently `--update` only accepts `--confidence`.
Updating reasoning or title must also re-index the decision in `search_index`.
Updating title triggers auto-linker re-scan for new package paths mentioned.

**Files:**

- Modify: `internal/knowledge/service.go`
- Modify: `internal/cli/decide.go`
- Modify: `internal/knowledge/service_test.go` (or create
  `internal/knowledge/service_test.go` if it doesn't exist; check first with
  `ls internal/knowledge/`)

### Step 1: Check existing test file names

    ls internal/knowledge/

Note the test file names. There will be at least one `*_test.go` file.

### Step 2: Write failing tests for UpdateDecision in the knowledge service

In whichever `_test.go` file holds happy-path service tests for decisions, add:

    func TestUpdateDecisionReasoning(t *testing.T) {
        svc, _ := setupTestDB(t)  // use the existing helper in the test file
        // First create a decision (reuse existing helper or insert directly)
        id := insertActiveDecision(t, svc, "Original Title", "original reasoning")

        err := svc.UpdateDecision(context.Background(), id, knowledge.UpdateDecisionInput{
            Reasoning: "updated reasoning",
        })
        if err != nil {
            t.Fatalf("UpdateDecision: %v", err)
        }

        // Verify the change persisted
        items, _ := svc.ListDecisions(context.Background())
        var found knowledge.DecisionListItem
        for _, it := range items {
            if it.ID == id {
                found = it
            }
        }
        if found.ID == 0 {
            t.Fatal("decision not found after update")
        }
        // reasoning is not in DecisionListItem; use a GetDecision helper
        // or add a small direct DB query here if GetDecision doesn't exist yet
    }

    func TestUpdateDecisionTitle(t *testing.T) { ... }

    func TestUpdateDecision_NotFound(t *testing.T) {
        svc, _ := setupTestDB(t)
        err := svc.UpdateDecision(context.Background(), 9999, knowledge.UpdateDecisionInput{
            Title: "x",
        })
        if !errors.Is(err, knowledge.ErrNotFound) {
            t.Fatalf("expected ErrNotFound, got %v", err)
        }
    }

    func TestUpdateDecision_EmptyInput(t *testing.T) {
        // Updating with no fields set should return an error
        svc, _ := setupTestDB(t)
        id := insertActiveDecision(t, svc, "T", "r")
        err := svc.UpdateDecision(context.Background(), id, knowledge.UpdateDecisionInput{})
        if err == nil {
            t.Fatal("expected error for empty UpdateDecisionInput")
        }
    }

Run: `go test ./internal/knowledge/... -run TestUpdateDecision` Expected: FAIL —
UpdateDecision undefined.

### Step 3: Add UpdateDecisionInput type and UpdateDecision method to service

In `internal/knowledge/service.go`, after the `UpdateConfidence` method, add:

    type UpdateDecisionInput struct {
        Title     string
        Reasoning string
    }

    func (s *Service) UpdateDecision(ctx context.Context, id int64, in UpdateDecisionInput) error {
        if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Reasoning) == "" {
            return fmt.Errorf("at least one field (title, reasoning) is required")
        }

        now := time.Now().UTC().Format(time.RFC3339)

        // Build dynamic SET clause — only update what was provided
        setClauses := []string{"updated_at = ?"}
        args := []any{now}

        if strings.TrimSpace(in.Title) != "" {
            setClauses = append(setClauses, "title = ?")
            args = append(args, strings.TrimSpace(in.Title))
        }
        if strings.TrimSpace(in.Reasoning) != "" {
            setClauses = append(setClauses, "reasoning = ?")
            args = append(args, strings.TrimSpace(in.Reasoning))
        }
        args = append(args, id)

        query := "UPDATE decisions SET " + strings.Join(setClauses, ", ") +
            " WHERE id = ? AND status = 'active';"
        res, err := s.db.ExecContext(ctx, query, args...)
        if err != nil {
            return fmt.Errorf("update decision: %w", err)
        }
        n, _ := res.RowsAffected()
        if n == 0 {
            return fmt.Errorf("decision %d: %w", id, ErrNotFound)
        }

        // Re-index in search_index: fetch current values, merge, update FTS row
        var title, reasoning string
        if err := s.db.QueryRowContext(ctx,
            `SELECT title, reasoning FROM decisions WHERE id = ?`, id,
        ).Scan(&title, &reasoning); err != nil {
            return fmt.Errorf("read updated decision for reindex: %w", err)
        }

        if _, err := s.db.ExecContext(ctx,
            `UPDATE search_index SET title = ?, content = ? WHERE entity_type = 'decision' AND entity_id = ?`,
            title, reasoning, id,
        ); err != nil {
            return fmt.Errorf("reindex decision: %w", err)
        }

        return nil
    }

Note on the SET clause order: SQLite accepts any column order in SET. The code
builds `setClauses` starting with `updated_at = ?` and appending optional
fields. The matching `args` slice is built in the same order so positions align.

### Step 4: Run the tests

    go test ./internal/knowledge/... -run TestUpdateDecision

Expected: PASS.

### Step 5: Write failing CLI test for --update with --reasoning/--title

In `internal/cli/commands_test.go`, add tests using the `runCmd` helper (already
exists in the file, takes `*cobra.Command`, `[]string` args, and returns
`(string, error)`):

    func TestDecideUpdateReasoning(t *testing.T) {
        // Setup: create a decision first, then update its reasoning
        app, conn := setupCLITest(t) // use existing helper
        defer conn.Close()
        // insert a decision directly or via CLI create flow
        ...
        out, err := runCmd(newDecideCommand(app), []string{"--update", "1", "--reasoning", "new reasoning"})
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if !strings.Contains(out, "updated") {
            t.Errorf("expected updated message, got: %s", out)
        }
    }

    func TestDecideUpdateTitle(t *testing.T) { ... }

    func TestDecideUpdate_NoFieldsError(t *testing.T) {
        // --update without --reasoning or --title or --confidence should error
        ...
    }

Run: `go test ./internal/cli/... -run TestDecideUpdate` Expected: FAIL.

### Step 6: Extend the CLI update mode in decide.go

In `internal/cli/decide.go`, in the `// Update mode` block (around line 102):

1.  Add `title` and `newReasoning` as new flag variables at the top of the
    function (alongside `reasoning`, `confidence`, etc.):

         updateTitle     string
         updateReasoning string

2.  Register them as flags at the bottom of `newDecideCommand`:

         cmd.Flags().StringVar(&updateTitle, "title", "", "New title for --update mode")
         // Note: --reasoning already exists as a flag; reuse it for --update mode

    Actually, `--reasoning` already exists for create mode. Rather than adding a
    new flag that would conflict, reuse `--reasoning` for both create and update
    modes. Check `cmd.Flags().Changed("reasoning")` in update mode to know if it
    was explicitly passed.

3.  In the update mode block, change the logic from:

         if !cmd.Flags().Changed("confidence") {
             msg := "--confidence is required when using --update"
             ...
         }

    To:

         titleChanged := cmd.Flags().Changed("title")
         reasoningChanged := cmd.Flags().Changed("reasoning")
         confidenceChanged := cmd.Flags().Changed("confidence")

         if !titleChanged && !reasoningChanged && !confidenceChanged {
             msg := "--update requires at least one of --confidence, --reasoning, or --title"
             ...return error...
         }

         // Handle confidence update separately (existing logic)
         if confidenceChanged {
             err = knowledge.NewService(conn).UpdateConfidence(cmd.Context(), updateID, confidence)
             ...handle error...
         }

         // Handle title/reasoning update
         if titleChanged || reasoningChanged {
             err = knowledge.NewService(conn).UpdateDecision(cmd.Context(), updateID,
                 knowledge.UpdateDecisionInput{
                     Title:     updateTitle,
                     Reasoning: reasoning,
                 })
             ...handle error...
         }

         if jsonOut {
             fields := map[string]any{"updated": true, "id": updateID}
             if confidenceChanged { fields["confidence"] = confidence }
             if titleChanged { fields["title"] = updateTitle }
             if reasoningChanged { fields["reasoning"] = reasoning }
             return writeJSON(fields)
         }
         fmt.Printf("Decision %d updated.\n", updateID)
         return nil

    Note: add `title` flag registration:

         cmd.Flags().StringVar(&updateTitle, "title", "", "New title (for --update mode)")

### Step 7: Run CLI tests

    go test ./internal/cli/... -run TestDecideUpdate

Expected: PASS.

### Step 8: Repeat for pattern update

Pattern has the same structure. The `patterns` table has `title` and
`description` (not `reasoning`). In `internal/pattern/service.go`, add an
`UpdatePattern` method that mirrors `UpdateDecision` but targets the `patterns`
table and uses the `description` column instead of `reasoning`.

Add `UpdatePatternInput` and `UpdatePattern` to `internal/pattern/service.go`.
Wire it in `internal/cli/pattern.go` — add `--update <id>` flag and the same
three-field update logic. Add tests in `internal/pattern/` and
`internal/cli/commands_test.go`.

    go test ./internal/pattern/... -run TestUpdatePattern
    go test ./internal/cli/... -run TestPatternUpdate

### Step 9: Run full test suite

    just test

Expected: all tests pass.

### Step 10: Commit

    git add internal/knowledge/service.go internal/cli/decide.go \
            internal/pattern/service.go internal/cli/pattern.go \
            internal/knowledge/*_test.go internal/pattern/*_test.go \
            internal/cli/commands_test.go
    git commit -m "feat(decide,pattern): add --title and --reasoning to --update mode"

---

## Task 2: Import/dependency search in find

**What:** Two new flags for `recon find`:

- `--imports-of <package-path>` — lists all packages that the given package
  imports (what does `internal/cli` depend on?)
- `--imported-by <package-path>` — lists all packages that import the given
  package (what depends on `internal/db`?)

The data is already in the `imports` table:
`from_file_id → to_path → to_package_id`. And in `packages` table for name/path
lookup.

**Files:**

- Modify: `internal/find/service.go`
- Modify: `internal/cli/find.go`
- Test: existing find service test file or new one

### Step 1: Understand the imports table

The `imports` table links files to package paths they import:

    imports: from_file_id (FK→files.id), to_path (TEXT), to_package_id (FK→packages.id), alias, import_type

To find what `internal/cli` imports, join files→packages on from_file_id, filter
by package path, group by to_path.

To find what imports `internal/db`, filter by
`to_path = 'github.com/.../internal/db'` or by `to_package_id` where that
package's path is `internal/db`.

### Step 2: Write failing service tests

In the find service test file, add:

    func TestImportsOf(t *testing.T) {
        svc, conn := setupTestDB(t)
        // Insert test data: two packages, a file in pkg A that imports pkg B
        // Use helper functions or direct inserts
        ...
        result, err := svc.ImportsOf(context.Background(), "internal/a")
        if err != nil {
            t.Fatal(err)
        }
        if len(result) == 0 {
            t.Fatal("expected at least one import")
        }
        // Check that internal/b appears
        found := false
        for _, p := range result { if p.Path == "internal/b" { found = true } }
        if !found { t.Error("expected internal/b in imports") }
    }

    func TestImportedBy(t *testing.T) { ... }

    func TestImportsOf_NotFound(t *testing.T) {
        // Package with no imports returns empty slice, not error
        ...
    }

Run: `go test ./internal/find/... -run TestImports` Expected: FAIL —
ImportsOf/ImportedBy undefined.

### Step 3: Add ImportsOf and ImportedBy to find/service.go

Add a new result type and two methods after the existing `ListPackages` method:

    type ImportResult struct {
        Path string `json:"path"`
        Name string `json:"name,omitempty"`
    }

    // ImportsOf returns the distinct packages imported by the given package path.
    func (s *Service) ImportsOf(ctx context.Context, pkgPath string) ([]ImportResult, error) {
        rows, err := s.db.QueryContext(ctx, `
    SELECT DISTINCT i.to_path, COALESCE(p2.name, '')
    FROM imports i
    JOIN files f ON f.id = i.from_file_id
    JOIN packages p ON p.id = f.package_id
    LEFT JOIN packages p2 ON p2.id = i.to_package_id
    WHERE p.path = ?
    ORDER BY i.to_path;
    `, pkgPath)
        if err != nil {
            return nil, fmt.Errorf("query imports of %s: %w", pkgPath, err)
        }
        defer rows.Close()
        var out []ImportResult
        for rows.Next() {
            var r ImportResult
            if err := rows.Scan(&r.Path, &r.Name); err != nil {
                return nil, fmt.Errorf("scan import: %w", err)
            }
            out = append(out, r)
        }
        return out, rows.Err()
    }

    // ImportedBy returns the distinct packages that import the given package path.
    // pkgPath is matched against the to_path column (which stores the full import
    // path as written in Go source) and also against the path column of packages
    // (the module-relative path). Both are tried so that both
    // "github.com/owner/repo/internal/db" and "internal/db" work.
    func (s *Service) ImportedBy(ctx context.Context, pkgPath string) ([]ImportResult, error) {
        rows, err := s.db.QueryContext(ctx, `
    SELECT DISTINCT p.path, p.name
    FROM imports i
    JOIN files f ON f.id = i.from_file_id
    JOIN packages p ON p.id = f.package_id
    WHERE i.to_path = ?
       OR i.to_path LIKE ?
    ORDER BY p.path;
    `, pkgPath, "%/"+pkgPath)
        if err != nil {
            return nil, fmt.Errorf("query imported by %s: %w", pkgPath, err)
        }
        defer rows.Close()
        var out []ImportResult
        for rows.Next() {
            var r ImportResult
            if err := rows.Scan(&r.Path, &r.Name); err != nil {
                return nil, fmt.Errorf("scan imported-by: %w", err)
            }
            out = append(out, r)
        }
        return out, rows.Err()
    }

### Step 4: Run service tests

    go test ./internal/find/... -run TestImports

Expected: PASS.

### Step 5: Write failing CLI tests

In `internal/cli/commands_test.go`, add:

    func TestFindImportsOf(t *testing.T) {
        app, conn := setupCLITest(t)
        defer conn.Close()
        // seed packages and imports...
        out, err := runCmd(newFindCommand(app), []string{"--imports-of", "internal/cli"})
        if err != nil { t.Fatalf("unexpected error: %v", err) }
        if !strings.Contains(out, "internal/db") {
            t.Errorf("expected internal/db in output, got: %s", out)
        }
    }

    func TestFindImportedBy(t *testing.T) { ... }

Run: `go test ./internal/cli/... -run TestFindImports` Expected: FAIL.

### Step 6: Add --imports-of and --imported-by flags to find.go

In `internal/cli/find.go`, in `newFindCommand`:

1.  Add two flag variables at the top of the function:

        importsOf  string
        importedBy string

2.  Add early-return branches at the top of RunE (before the `listPackages`
    block), checking `importsOf != ""` and `importedBy != ""`:

         if importsOf != "" {
             conn, connErr := openExistingDB(app)
             if connErr != nil { ... return error ... }
             defer conn.Close()
             results, err := find.NewService(conn).ImportsOf(cmd.Context(), importsOf)
             if err != nil { ... return error ... }
             if jsonOut { return writeJSON(results) }
             if len(results) == 0 {
                 fmt.Printf("No imports found for %s\n", importsOf)
                 return nil
             }
             fmt.Printf("Imports of %s (%d):\n", importsOf, len(results))
             for _, r := range results { fmt.Printf("- %s\n", r.Path) }
             return nil
         }

         if importedBy != "" {
             ... same pattern ...
         }

3.  Register the flags:

        cmd.Flags().StringVar(&importsOf, "imports-of", "", "List packages imported by this package")
        cmd.Flags().StringVar(&importedBy, "imported-by", "", "List packages that import this package")

### Step 7: Run CLI tests

    go test ./internal/cli/... -run TestFindImports

Expected: PASS.

### Step 8: Run full test suite

    just test

Expected: all tests pass.

### Step 9: Commit

    git add internal/find/service.go internal/cli/find.go \
            internal/find/*_test.go internal/cli/commands_test.go
    git commit -m "feat(find): add --imports-of and --imported-by flags"

---

## Task 3: Edge title display in edges --list/--from/--to

**What:** Currently `edges --list` prints:

    #1 decision:1 -[affects]-> package:internal/cli (source=manual, confidence=high)

After this task it prints:

    #1 decision:1 "ExitError is the standard error type" -[affects]-> package:internal/cli (source=manual, confidence=high)

Entity titles are resolved by joining against `decisions` and `patterns` tables
for `from_type` knowledge entities. Code targets (packages, files, symbols) do
not need title enrichment in text mode — the ref is already human-readable.

**Files:**

- Modify: `internal/edge/service.go` (add a new enriched query method)
- Modify: `internal/cli/edges.go` (enrich renderEdges for text mode)

### Step 1: Write failing tests

In the edge service test file, add:

    func TestListAllWithTitles(t *testing.T) {
        svc, conn := setupEdgeTestDB(t)
        // insert a decision and an edge pointing to a package
        ...
        edges, err := svc.ListAllWithTitles(context.Background())
        if err != nil { t.Fatal(err) }
        if len(edges) == 0 { t.Fatal("expected edges") }
        if edges[0].FromTitle == "" {
            t.Error("expected non-empty FromTitle")
        }
    }

Run: `go test ./internal/edge/... -run TestListAllWithTitles` Expected: FAIL.

### Step 2: Add EdgeWithTitle type and enriched query methods to edge/service.go

In `internal/edge/service.go`, add:

    type EdgeWithTitle struct {
        Edge
        FromTitle string `json:"from_title,omitempty"`
    }

    func (s *Service) ListAllWithTitles(ctx context.Context) ([]EdgeWithTitle, error) {
        return s.queryWithTitles(ctx, `
    SELECT e.id, e.from_type, e.from_id, e.to_type, e.to_ref, e.relation,
           e.source, e.confidence, e.created_at,
           COALESCE(d.title, p.title, '') as from_title
    FROM edges e
    LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id
    LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id
    ORDER BY e.from_type, e.from_id, e.relation, e.to_type, e.to_ref;
    `)
    }

    func (s *Service) ListFromWithTitles(ctx context.Context, fromType string, fromID int64) ([]EdgeWithTitle, error) {
        return s.queryWithTitles(ctx, `
    SELECT e.id, e.from_type, e.from_id, e.to_type, e.to_ref, e.relation,
           e.source, e.confidence, e.created_at,
           COALESCE(d.title, p.title, '') as from_title
    FROM edges e
    LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id
    LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id
    WHERE e.from_type = ? AND e.from_id = ?
    ORDER BY e.relation, e.to_type, e.to_ref;
    `, fromType, fromID)
    }

    func (s *Service) ListToWithTitles(ctx context.Context, toType, toRef string) ([]EdgeWithTitle, error) {
        return s.queryWithTitles(ctx, `
    SELECT e.id, e.from_type, e.from_id, e.to_type, e.to_ref, e.relation,
           e.source, e.confidence, e.created_at,
           COALESCE(d.title, p.title, '') as from_title
    FROM edges e
    LEFT JOIN decisions d ON e.from_type = 'decision' AND e.from_id = d.id
    LEFT JOIN patterns p ON e.from_type = 'pattern' AND e.from_id = p.id
    WHERE e.to_type = ? AND e.to_ref = ?
    ORDER BY e.relation, e.from_type, e.from_id;
    `, toType, toRef)
    }

    func (s *Service) queryWithTitles(ctx context.Context, q string, args ...any) ([]EdgeWithTitle, error) {
        rows, err := s.db.QueryContext(ctx, q, args...)
        if err != nil {
            return nil, fmt.Errorf("query edges with titles: %w", err)
        }
        defer rows.Close()
        edges := make([]EdgeWithTitle, 0)
        for rows.Next() {
            var e EdgeWithTitle
            if err := rows.Scan(
                &e.ID, &e.FromType, &e.FromID, &e.ToType, &e.ToRef,
                &e.Relation, &e.Source, &e.Confidence, &e.CreatedAt,
                &e.FromTitle,
            ); err != nil {
                return nil, fmt.Errorf("scan edge with title: %w", err)
            }
            edges = append(edges, e)
        }
        return edges, rows.Err()
    }

### Step 3: Run service tests

    go test ./internal/edge/... -run TestListAllWithTitles

Expected: PASS.

### Step 4: Update edges.go to use enriched methods in text mode

In `internal/cli/edges.go`, change the list/from/to modes to use
`ListAllWithTitles`, `ListFromWithTitles`, and `ListToWithTitles` when
`!jsonOut`. In JSON mode, continue using the plain `List*` methods (the JSON
output for `EdgeWithTitle` is backward-compatible — it just adds `from_title`).
Actually, use the enriched methods in both modes for simplicity.

Update `renderEdges` to accept `[]edge.EdgeWithTitle` and show the title in text
mode. For JSON mode, the `Edge` embedded struct is serialized as before and
`from_title` is an additional field.

The updated `renderEdges` signature:

    func renderEdges(edges []edge.EdgeWithTitle, jsonOut bool) error {
        if jsonOut {
            return writeJSON(edges)
        }
        if len(edges) == 0 {
            fmt.Println("No edges found.")
            return nil
        }
        for _, e := range edges {
            title := ""
            if e.FromTitle != "" {
                title = fmt.Sprintf(" %q", e.FromTitle)
            }
            fmt.Printf("#%d %s:%d%s -[%s]-> %s:%s (source=%s, confidence=%s)\n",
                e.ID, e.FromType, e.FromID, title, e.Relation,
                e.ToType, e.ToRef, e.Source, e.Confidence)
        }
        return nil
    }

Update all callers of `renderEdges` in `edges.go` to pass `[]EdgeWithTitle`.

### Step 5: Write CLI tests

    func TestEdgesListShowsTitles(t *testing.T) {
        // Create a decision edge and verify the title appears in text output
        ...
    }

### Step 6: Run tests

    go test ./internal/edge/... ./internal/cli/...

Expected: PASS.

### Step 7: Run full test suite

    just test

### Step 8: Commit

    git add internal/edge/service.go internal/cli/edges.go \
            internal/edge/*_test.go internal/cli/commands_test.go
    git commit -m "feat(edges): show entity titles in list output"

---

## Task 4: Rename --delete to --archive

**What:** `recon decide --delete <id>` and `recon pattern --delete <id>` output
`"Decision N archived."` but the flag name says `--delete`, which is misleading.
Rename the flag to `--archive` while keeping `--delete` as a hidden alias for
backward compatibility.

**Files:**

- Modify: `internal/cli/decide.go`
- Modify: `internal/cli/pattern.go`

### Step 1: Write a failing test

In `internal/cli/commands_test.go`, add:

    func TestDecideArchiveFlag(t *testing.T) {
        app, conn := setupCLITest(t)
        defer conn.Close()
        // create a decision...
        out, err := runCmd(newDecideCommand(app), []string{"--archive", "1"})
        if err != nil { t.Fatalf("unexpected error: %v", err) }
        if !strings.Contains(out, "archived") {
            t.Errorf("expected archived message, got: %s", out)
        }
    }

Run: `go test ./internal/cli/... -run TestDecideArchiveFlag` Expected: FAIL —
unknown flag --archive.

### Step 2: Add --archive flag and deprecate --delete in decide.go

In `internal/cli/decide.go`, change from:

    cmd.Flags().Int64Var(&deleteID, "delete", 0, "Archive (soft-delete) a decision by ID")

To:

    cmd.Flags().Int64Var(&deleteID, "archive", 0, "Archive (soft-delete) a decision by ID")
    // Keep --delete as a hidden alias for backward compatibility
    cmd.Flags().Int64Var(&deleteID, "delete", 0, "")
    _ = cmd.Flags().MarkHidden("delete")

Cobra shares the same variable for both flags, so either `--archive 1` or
`--delete 1` sets `deleteID = 1`.

Do the same for `internal/cli/pattern.go`.

### Step 3: Run tests

    go test ./internal/cli/... -run TestDecideArchiveFlag
    go test ./internal/cli/... -run TestPatternArchiveFlag

Expected: PASS.

### Step 4: Verify old --delete still works (backward compat)

    func TestDecideDeleteFlagStillWorks(t *testing.T) {
        // --delete should still work after the rename
        ...
    }

### Step 5: Run full test suite

    just test

### Step 6: Commit

    git add internal/cli/decide.go internal/cli/pattern.go \
            internal/cli/commands_test.go
    git commit -m "feat(decide,pattern): rename --delete to --archive, keep --delete as hidden alias"

---

## Task 5: recon reset command

**What:** `recon reset` deletes `.recon/recon.db` and gives a clean slate,
without requiring the user to know the path or run `rm -rf`. This is more
ergonomic than the justfile's `just db-reset` which only works inside the recon
repo itself.

With `--force` it skips the confirmation prompt. The `--no-prompt` global flag
also skips the prompt (consistent with how init handles interactive prompts).

**Files:**

- Create: `internal/cli/reset.go`
- Modify: `internal/cli/root.go` (register the new command)
- Test: `internal/cli/commands_test.go`

### Step 1: Write failing tests

In `internal/cli/commands_test.go`, add:

    func TestResetCommand(t *testing.T) {
        // Create a temp .recon/recon.db, run reset --force, verify it's gone
        app, conn := setupCLITest(t)
        conn.Close() // close before deletion
        out, err := runCmd(newResetCommand(app), []string{"--force"})
        if err != nil { t.Fatalf("unexpected error: %v", err) }
        if !strings.Contains(out, "reset") {
            t.Errorf("expected reset message, got: %s", out)
        }
        // Verify the DB file is gone
        dbPath := db.DBPath(app.ModuleRoot)
        if _, statErr := os.Stat(dbPath); !errors.Is(statErr, os.ErrNotExist) {
            t.Error("expected db file to be deleted after reset")
        }
    }

    func TestResetCommand_NotInitialized(t *testing.T) {
        // If no DB exists, reset should say "nothing to reset" and exit 0
        ...
    }

Run: `go test ./internal/cli/... -run TestResetCommand` Expected: FAIL —
newResetCommand undefined.

### Step 2: Create internal/cli/reset.go

    package cli

    import (
        "fmt"
        "os"

        "github.com/robertguss/recon/internal/db"
        "github.com/spf13/cobra"
    )

    func newResetCommand(app *App) *cobra.Command {
        var (
            force   bool
            jsonOut bool
        )

        cmd := &cobra.Command{
            Use:   "reset",
            Short: "Delete the recon database and start fresh",
            Args:  cobra.NoArgs,
            RunE: func(cmd *cobra.Command, args []string) error {
                path := db.DBPath(app.ModuleRoot)

                if _, err := os.Stat(path); os.IsNotExist(err) {
                    if jsonOut {
                        return writeJSON(map[string]any{"reset": false, "reason": "not initialized"})
                    }
                    fmt.Println("Nothing to reset: database not initialized.")
                    return nil
                }

                if !force && !app.NoPrompt {
                    fmt.Printf("This will delete %s. Continue? [y/N] ", path)
                    var confirm string
                    fmt.Scan(&confirm)
                    if confirm != "y" && confirm != "Y" {
                        fmt.Println("Aborted.")
                        return nil
                    }
                }

                if err := os.Remove(path); err != nil {
                    if jsonOut {
                        _ = writeJSONError("internal_error", err.Error(), nil)
                        return ExitError{Code: 2}
                    }
                    return fmt.Errorf("delete database: %w", err)
                }

                if jsonOut {
                    return writeJSON(map[string]any{"reset": true, "path": path})
                }
                fmt.Printf("Database reset. Run `recon init` to reinitialize.\n")
                return nil
            },
        }

        cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
        cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
        return cmd
    }

### Step 3: Register in root.go

In `internal/cli/root.go`, add:

    root.AddCommand(newResetCommand(app))

### Step 4: Run tests

    go test ./internal/cli/... -run TestResetCommand

Expected: PASS.

### Step 5: Run full test suite

    just test

### Step 6: Commit

    git add internal/cli/reset.go internal/cli/root.go \
            internal/cli/commands_test.go
    git commit -m "feat(reset): add recon reset command to delete database"

---

## Task 6: find disambiguation hints

**What:** When `recon find Service` returns an `AmbiguousError` (multiple
candidates), text mode currently prints the candidates but gives no suggestion
for how to proceed. Add a hint line like
`Try: recon find Service --package internal/find` using the first candidate's
package path.

This is a text-mode-only UX improvement. The JSON output already includes
`candidates` so agents have all they need.

**Files:**

- Modify: `internal/cli/find.go` (text-mode AmbiguousError handler)

### Step 1: Write a failing test

In `internal/cli/commands_test.go`, add:

    func TestFindAmbiguousShowsHint(t *testing.T) {
        app, conn := setupCLITest(t)
        defer conn.Close()
        // Insert two "Service" symbols in different packages
        ...
        _, err := runCmd(newFindCommand(app), []string{"Service"})
        // It should fail (ambiguous = exit 2) but output should contain "Try:"
        // We need to capture output even on error. Check how existing ambiguous
        // tests work in the test file to see the helper pattern.
        ...
    }

Run: `go test ./internal/cli/... -run TestFindAmbiguousShowsHint` Expected:
FAIL.

### Step 2: Add hint to AmbiguousError text handler in find.go

In `internal/cli/find.go`, in the `case find.AmbiguousError:` block, after
printing the candidates, add:

    if len(e.Candidates) > 0 {
        c := e.Candidates[0]
        label := symbol
        if c.Receiver != "" {
            label = c.Receiver + "." + symbol
        }
        fmt.Printf("\nTry: recon find %s --package %s\n", label, c.Package)
    }

### Step 3: Run tests

    go test ./internal/cli/... -run TestFindAmbiguousShowsHint

Expected: PASS.

### Step 4: Run full test suite

    just test

### Step 5: Commit

    git add internal/cli/find.go internal/cli/commands_test.go
    git commit -m "feat(find): add disambiguation hint for ambiguous symbol results"

---

## Task 7: Update /recon skill and CLAUDE.md

**What:** After all the new flags are added, the `/recon` skill reference and
CLAUDE.md command reference must reflect the new flags:

- `recon decide --archive <id>` (was `--delete`)
- `recon decide --update <id> --title "..."` and `--reasoning "..."`
- `recon pattern --archive <id>`, `--update <id>`
- `recon find --imports-of <pkg>`, `--imported-by <pkg>`
- `recon reset [--force]`

**Files:**

- Modify: `.claude/skills/recon/SKILL.md` (the skill file loaded by `/recon`)
- Modify: `CLAUDE.md` (the command reference section)

### Step 1: Update SKILL.md

Read `.claude/skills/recon/SKILL.md`, then add:

Under `### recon decide`:

- Add `--title <text>` to the `--update <id>` flag description
- Add `--reasoning <text>` to the `--update <id>` flag description
- Change `--delete <id>` examples to use `--archive <id>`

Under `### recon pattern`:

- Same changes as decide

Under `### recon find`:

- Add `--imports-of <package>` — list packages imported by this package
- Add `--imported-by <package>` — list packages that import this package
- Add examples: recon find --imports-of internal/cli recon find --imported-by
  internal/db

Add a new section `### recon reset`: Delete the recon database for a clean
slate.

        recon reset           # interactive confirmation
        recon reset --force   # skip confirmation

    Flags:
    - `--force` — skip confirmation prompt
    - `--json` — output JSON

### Step 2: Update CLAUDE.md

In the command reference section at the bottom, add `reset` to the command list
and note the new flags.

### Step 3: Commit

    git add .claude/skills/recon/SKILL.md CLAUDE.md
    git commit -m "docs(recon): update skill and CLAUDE.md with new flags and reset command"

---

## Validation

After all tasks are complete, run a full test pass and do a quick smoke test:

    just test

Then smoke test against the live database:

    # Full update workflow
    just run decide --list
    just run decide --update 4 --reasoning "Updated reasoning text"
    just run decide --list  # verify confidence unchanged, reasoning updated

    # Import search
    just run find --imports-of internal/cli
    just run find --imported-by internal/db

    # Edge titles
    just run edges --list  # should show titles alongside IDs

    # Archive rename
    just run decide --list
    just run decide --archive <some-id>  # archive a test decision

    # Reset (skip this in live repo unless testing specifically)
    # just run reset --force  # only run if you want to drop the DB

    # Disambiguation hint
    just run find Service  # if Service is ambiguous, should show Try: hint

All commands should produce clean output with no regression in existing
behavior.

---

## Notes

- `internal/pattern/service.go` may not have an `UpdatePattern` method yet.
  Check with `grep -n "func.*Update" internal/pattern/service.go` before writing
  the test.
- The `search_index` FTS5 table must be kept in sync whenever `title` or
  `reasoning`/`description` changes. Always update `search_index` in the same
  transaction (or immediately after) an update to `decisions` or `patterns`.
- `fmt.Scan` for interactive prompts in the reset command will fail in tests
  because stdin is not a terminal. Use `app.NoPrompt` or `--force` in tests to
  bypass this. Never read from stdin in tests.
- Pattern's text column is called `description`, not `reasoning`. The CLI flag
  is `--reasoning` (renamed from `--description` in an earlier milestone) but
  the DB column remains `description`. Keep this mapping in mind when writing
  `UpdatePattern`.
