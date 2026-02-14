# Milestone 4: Agent Dogfood Findings — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Transform recon from a working reference tool into essential agent
infrastructure by closing gaps discovered during real CLI usage.

**Architecture:** Builds on M1-M3 foundation. Enriches existing commands
(`orient`, `find`, `decide`) and adds two new commands (`pattern`, `status`).
New DB migration for `patterns` and `pattern_files` tables. All changes maintain
100% test coverage and strict TDD.

**Tech Stack:** Go 1.26, Cobra, SQLite (modernc.org/sqlite), FTS5

**Decisions (from open questions):**

- Orient heat: git log (session tracking deferred to M5)
- Orient token budget: priority truncation (omit COLD in text, cap
  decisions/patterns at 5, JSON gets all)
- Pattern approval flow: same as decide (auto-promote on verification pass)
- Find list mode limit: 50 default with `--limit` flag, JSON includes `total`
  field
- Snapshot durability: deferred to M5

---

## Phase A: Orient + Find Enrichment

### Task 1: Add `--file` suffix matching to `find` service

**Files:**

- Modify: `internal/find/service.go:179-194` (filterMatches function)
- Test: `internal/find/service_test.go`

**Step 1: Write the failing test**

Add to `internal/find/service_test.go`:

```go
func TestFindFileFilterSuffixMatch(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	// "Ambig" in file "other.go" (file_id=2) should match --file "other.go"
	res, err := NewService(conn).Find(context.Background(), "Ambig", QueryOptions{FilePath: "other.go"})
	if err != nil {
		t.Fatalf("Find with suffix file filter error: %v", err)
	}
	if res.Symbol.FilePath != "other.go" {
		t.Fatalf("expected file other.go, got %s", res.Symbol.FilePath)
	}

	// Full path should still work
	res, err = NewService(conn).Find(context.Background(), "Target", QueryOptions{FilePath: "main.go"})
	if err != nil {
		t.Fatalf("Find with exact file filter error: %v", err)
	}
	if res.Symbol.Name != "Target" {
		t.Fatalf("expected Target, got %s", res.Symbol.Name)
	}

	// Path with slash should do substring match
	// Add a package-scoped file for this test
	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (2,'pkg/sub','sub','example.com/recon/pkg/sub',1,5,'x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (3,2,'pkg/sub/service.go','go',5,'h3','x','x');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (5,3,'func','UniqueInSub','func()','func UniqueInSub(){}',1,1,1,'');`)

	res, err = NewService(conn).Find(context.Background(), "UniqueInSub", QueryOptions{FilePath: "pkg/sub/service.go"})
	if err != nil {
		t.Fatalf("Find with path-containing file filter error: %v", err)
	}
	if res.Symbol.Name != "UniqueInSub" {
		t.Fatalf("expected UniqueInSub, got %s", res.Symbol.Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/find/ -run TestFindFileFilterSuffixMatch -v` Expected:
FAIL (suffix match not yet implemented)

**Step 3: Write minimal implementation**

In `internal/find/service.go`, replace the `filterMatches` function:

```go
func filterMatches(matches []Symbol, opts QueryOptions) []Symbol {
	filtered := make([]Symbol, 0, len(matches))
	for _, match := range matches {
		if opts.PackagePath != "" && match.Package != opts.PackagePath {
			continue
		}
		if opts.FilePath != "" && !matchFilePath(normalizeFilePath(match.FilePath), opts.FilePath) {
			continue
		}
		if opts.Kind != "" && strings.ToLower(match.Kind) != opts.Kind {
			continue
		}
		filtered = append(filtered, match)
	}
	return filtered
}

func matchFilePath(symbolPath, filter string) bool {
	if symbolPath == filter {
		return true
	}
	// No slash = suffix/filename match
	if !strings.Contains(filter, "/") {
		return filepath.Base(symbolPath) == filter
	}
	// Has slash = substring match against relative path
	return strings.Contains(symbolPath, filter) || strings.HasSuffix(symbolPath, filter)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/find/ -run TestFindFileFilterSuffixMatch -v` Expected:
PASS

**Step 5: Run all find tests**

Run: `go test ./internal/find/ -v` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/find/service.go internal/find/service_test.go
git commit -m "feat(find): add --file suffix matching for filename-only filters"
```

---

### Task 2: Add `find` browse/list mode to the service layer

**Files:**

- Modify: `internal/find/service.go`
- Test: `internal/find/service_test.go`

**Step 1: Write the failing test**

Add to `internal/find/service_test.go`:

```go
type ListResult struct {
	Symbols []Symbol `json:"symbols"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
}

func TestListByPackage(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{PackagePath: "."}, 50)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if result.Total < 3 {
		t.Fatalf("expected at least 3 symbols in root package, got %d", result.Total)
	}
	if result.Limit != 50 {
		t.Fatalf("expected limit 50, got %d", result.Limit)
	}
	// Symbols should not have bodies in list mode
	for _, s := range result.Symbols {
		if s.Body != "" {
			t.Fatalf("expected empty body in list mode, got body for %s", s.Name)
		}
	}
}

func TestListByKind(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{Kind: "method"}, 50)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 method, got %d", result.Total)
	}
	if result.Symbols[0].Kind != "method" {
		t.Fatalf("expected method kind, got %s", result.Symbols[0].Kind)
	}
}

func TestListByFile(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{FilePath: "main.go"}, 50)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if result.Total < 2 {
		t.Fatalf("expected at least 2 symbols in main.go, got %d", result.Total)
	}
}

func TestListNoFiltersReturnsError(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	_, err := NewService(conn).List(context.Background(), QueryOptions{}, 50)
	if err == nil {
		t.Fatal("expected error for list with no filters")
	}
}

func TestListRespectsLimit(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{PackagePath: "."}, 2)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(result.Symbols) > 2 {
		t.Fatalf("expected at most 2 symbols, got %d", len(result.Symbols))
	}
	if result.Total < 3 {
		t.Fatalf("expected total >= 3, got %d", result.Total)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/find/ -run TestList -v` Expected: FAIL (List method
doesn't exist)

**Step 3: Write minimal implementation**

Add to `internal/find/service.go`:

```go
type ListResult struct {
	Symbols []Symbol `json:"symbols"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
}

func (s *Service) List(ctx context.Context, opts QueryOptions, limit int) (ListResult, error) {
	opts = normalizeQueryOptions(opts)
	if !hasActiveFilters(opts) {
		return ListResult{}, fmt.Errorf("list mode requires at least one filter (--package, --file, or --kind)")
	}
	if limit <= 0 {
		limit = 50
	}

	where, args := buildListWhere(opts)

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM symbols s JOIN files f ON f.id = s.file_id LEFT JOIN packages p ON p.id = f.package_id WHERE " + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count list symbols: %w", err)
	}

	// Get limited results (no body)
	selectQuery := `
SELECT s.id, s.kind, s.name, COALESCE(s.signature, ''), '',
       s.line_start, s.line_end, COALESCE(s.receiver, ''), f.path, COALESCE(p.path, '.')
FROM symbols s
JOIN files f ON f.id = s.file_id
LEFT JOIN packages p ON p.id = f.package_id
WHERE ` + where + `
ORDER BY p.path, f.path, s.kind, s.name
LIMIT ?;`
	rows, err := s.db.QueryContext(ctx, selectQuery, append(args, limit)...)
	if err != nil {
		return ListResult{}, fmt.Errorf("query list symbols: %w", err)
	}
	defer rows.Close()

	symbols := make([]Symbol, 0, limit)
	for rows.Next() {
		var sym Symbol
		if err := rows.Scan(&sym.ID, &sym.Kind, &sym.Name, &sym.Signature, &sym.Body,
			&sym.LineStart, &sym.LineEnd, &sym.Receiver, &sym.FilePath, &sym.Package); err != nil {
			return ListResult{}, fmt.Errorf("scan list symbol: %w", err)
		}
		symbols = append(symbols, sym)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate list symbols: %w", err)
	}

	return ListResult{Symbols: symbols, Total: total, Limit: limit}, nil
}

func buildListWhere(opts QueryOptions) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	if opts.PackagePath != "" {
		clauses = append(clauses, "COALESCE(p.path, '.') = ?")
		args = append(args, opts.PackagePath)
	}
	if opts.FilePath != "" {
		if !strings.Contains(opts.FilePath, "/") {
			// Filename-only: match basename using LIKE suffix
			clauses = append(clauses, "(f.path = ? OR f.path LIKE ?)")
			args = append(args, opts.FilePath, "%/"+opts.FilePath)
		} else {
			clauses = append(clauses, "(f.path = ? OR f.path LIKE ?)")
			args = append(args, opts.FilePath, "%"+opts.FilePath+"%")
		}
	}
	if opts.Kind != "" {
		clauses = append(clauses, "LOWER(s.kind) = ?")
		args = append(args, opts.Kind)
	}
	return strings.Join(clauses, " AND "), args
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/find/ -run TestList -v` Expected: All PASS

**Step 5: Run all find tests**

Run: `go test ./internal/find/ -v` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/find/service.go internal/find/service_test.go
git commit -m "feat(find): add List method for browse/list mode with filters"
```

---

### Task 3: Wire `find` browse/list mode into CLI

**Files:**

- Modify: `internal/cli/find.go:22-26` (Args validator),
  `internal/cli/find.go:26-140` (RunE)
- Test: `internal/cli/commands_test.go`

**Step 1: Write the failing test**

Add to `internal/cli/commands_test.go`:

```go
func TestFindListMode(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// List all symbols in root package
	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"--package", ".", "--json"})
	if err != nil {
		t.Fatalf("find list mode --json error: %v", err)
	}
	if !strings.Contains(out, `"symbols"`) || !strings.Contains(out, `"total"`) {
		t.Fatalf("expected list mode JSON, out=%q", out)
	}

	// List mode text output
	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"--package", "."}
	if err != nil {
		t.Fatalf("find list mode text error: %v", err)
	}
	if !strings.Contains(out, "Alpha") {
		t.Fatalf("expected Alpha in text list, out=%q", out)
	}

	// No args, no filters → error
	_, _, err = runCommandWithCapture(t, newFindCommand(app), []string{})
	if err == nil {
		t.Fatal("expected error for find with no args and no filters")
	}

	// List with --limit
	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"--package", ".", "--limit", "1", "--json"})
	if err != nil {
		t.Fatalf("find list --limit error: %v", err)
	}
	if !strings.Contains(out, `"limit": 1`) {
		t.Fatalf("expected limit 1, out=%q", out)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestFindListMode -v` Expected: FAIL

**Step 3: Write minimal implementation**

Modify `internal/cli/find.go`:

1. Change `Args` from `cobra.ExactArgs(1)` to `cobra.MaximumNArgs(1)`
2. Add `limit` flag variable
3. In `RunE`, branch: if no symbol arg and filters present → list mode; if no
   symbol arg and no filters → structured error; else → existing find logic

```go
func newFindCommand(app *App) *cobra.Command {
	var (
		jsonOut       bool
		noBody        bool
		maxBodyLines  int
		packageFilter string
		fileFilter    string
		kindFilter    string
		limit         int
	)

	cmd := &cobra.Command{
		Use:   "find [<symbol>]",
		Short: "Find exact symbol or list symbols by filter",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedKind, err := normalizeFindKind(kindFilter)
			if err != nil {
				if jsonOut {
					details := map[string]any{"kind": strings.TrimSpace(kindFilter)}
					_ = writeJSONError("invalid_input", err.Error(), details)
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: err.Error()}
			}

			queryOptions := find.QueryOptions{
				PackagePath: strings.TrimSpace(packageFilter),
				FilePath:    normalizeFindPath(fileFilter),
				Kind:        normalizedKind,
			}

			// No symbol arg: check for list mode vs missing arg error
			if len(args) == 0 {
				hasFilters := queryOptions.PackagePath != "" || queryOptions.FilePath != "" || queryOptions.Kind != ""
				if !hasFilters {
					msg := "find requires a <symbol> argument or filter flags (--package, --file, --kind)"
					if jsonOut {
						_ = writeJSONError("missing_argument", msg, map[string]any{"command": "find"})
						return ExitError{Code: 2}
					}
					return ExitError{Code: 2, Message: msg}
				}
				return runFindListMode(cmd, app, queryOptions, limit, jsonOut)
			}

			// Existing find logic (symbol arg provided)
			symbol := args[0]
			if maxBodyLines < 0 {
				msg := "--max-body-lines must be >= 0"
				if jsonOut {
					details := map[string]any{"flag": "max_body_lines", "value": maxBodyLines}
					_ = writeJSONError("invalid_input", msg, details)
					return ExitError{Code: 2}
				}
				return ExitError{Code: 2, Message: msg}
			}

			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			result, err := find.NewService(conn).Find(cmd.Context(), symbol, queryOptions)
			if err != nil {
				// ... existing error handling unchanged ...
			}

			// ... existing success rendering unchanged ...
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&noBody, "no-body", false, "Omit symbol body in text output")
	cmd.Flags().IntVar(&maxBodyLines, "max-body-lines", 0, "Maximum symbol body lines in text output (0 = no limit)")
	cmd.Flags().StringVar(&packageFilter, "package", "", "Filter by package path")
	cmd.Flags().StringVar(&fileFilter, "file", "", "Filter by file path")
	cmd.Flags().StringVar(&kindFilter, "kind", "", "Filter by symbol kind (func, method, type, var, const)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum symbols in list mode")
	return cmd
}

func runFindListMode(cmd *cobra.Command, app *App, opts find.QueryOptions, limit int, jsonOut bool) error {
	conn, err := openExistingDB(app)
	if err != nil {
		if jsonOut {
			return exitJSONCommandError(err)
		}
		return err
	}
	defer conn.Close()

	result, err := find.NewService(conn).List(cmd.Context(), opts, limit)
	if err != nil {
		if jsonOut {
			_ = writeJSONError("internal_error", err.Error(), nil)
			return ExitError{Code: 2}
		}
		return err
	}

	if jsonOut {
		return writeJSON(result)
	}

	fmt.Printf("Symbols (%d of %d):\n", len(result.Symbols), result.Total)
	for _, s := range result.Symbols {
		label := s.Name
		if s.Receiver != "" {
			label = s.Receiver + "." + s.Name
		}
		fmt.Printf("- %s %s (%s:%d-%d) pkg=%s\n", s.Kind, label, s.FilePath, s.LineStart, s.LineEnd, s.Package)
	}
	return nil
}
```

Note: The existing find error handling and success rendering in the `RunE` block
remains **exactly as-is** — only the Args validator and the no-arg branch are
new.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestFindListMode -v` Expected: PASS

**Step 5: Run all CLI tests**

Run: `go test ./internal/cli/ -v` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/cli/find.go internal/cli/commands_test.go
git commit -m "feat(cli): add find browse/list mode with --limit flag"
```

---

### Task 4: Richer `orient` output — architecture section

**Files:**

- Modify: `internal/orient/service.go` (Payload struct, Build method)
- Modify: `internal/orient/render.go` (RenderText function)
- Test: `internal/orient/service_test.go`

**Step 1: Write the failing test**

Add to `internal/orient/service_test.go`:

```go
func TestBuildArchitectureSection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "recon"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/recon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "recon", "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	conn := setupOrientDB(t, root)
	defer conn.Close()

	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.Architecture.EntryPoints) == 0 {
		t.Fatal("expected entry points")
	}
	found := false
	for _, ep := range payload.Architecture.EntryPoints {
		if strings.Contains(ep, "cmd/recon/main.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cmd/recon/main.go in entry points, got %v", payload.Architecture.EntryPoints)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/orient/ -run TestBuildArchitectureSection -v` Expected:
FAIL (Architecture field doesn't exist)

**Step 3: Write minimal implementation**

Add to `internal/orient/service.go`:

```go
type Architecture struct {
	EntryPoints    []string `json:"entry_points"`
	DependencyFlow string   `json:"dependency_flow"`
}
```

Add `Architecture` field to `Payload`:

```go
type Payload struct {
	Project         ProjectInfo      `json:"project"`
	Architecture    Architecture     `json:"architecture"`
	Freshness       Freshness        `json:"freshness"`
	// ... rest unchanged
}
```

Add `loadArchitecture` method:

```go
func (s *Service) loadArchitecture(ctx context.Context, payload *Payload) error {
	// Find entry points: files named main.go in packages named main
	rows, err := s.db.QueryContext(ctx, `
SELECT f.path
FROM files f
JOIN packages p ON p.id = f.package_id
WHERE p.name = 'main' AND f.path LIKE '%main.go'
ORDER BY f.path;
`)
	if err != nil {
		return fmt.Errorf("query entry points: %w", err)
	}
	defer rows.Close()

	entryPoints := []string{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return fmt.Errorf("scan entry point: %w", err)
		}
		entryPoints = append(entryPoints, path)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entry points: %w", err)
	}

	// Build dependency flow from import relationships
	depRows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT p1.path AS from_pkg, p2.path AS to_pkg
FROM imports i
JOIN files f ON f.id = i.from_file_id
JOIN packages p1 ON p1.id = f.package_id
JOIN packages p2 ON p2.id = i.to_package_id
WHERE p1.id != p2.id
ORDER BY p1.path, p2.path;
`)
	if err != nil {
		return fmt.Errorf("query dependency flow: %w", err)
	}
	defer depRows.Close()

	flowParts := map[string][]string{}
	for depRows.Next() {
		var from, to string
		if err := depRows.Scan(&from, &to); err != nil {
			return fmt.Errorf("scan dep flow: %w", err)
		}
		flowParts[from] = append(flowParts[from], to)
	}
	if err := depRows.Err(); err != nil {
		return fmt.Errorf("iterate dep flow: %w", err)
	}

	flow := formatDependencyFlow(flowParts)
	payload.Architecture = Architecture{EntryPoints: entryPoints, DependencyFlow: flow}
	return nil
}

func formatDependencyFlow(deps map[string][]string) string {
	if len(deps) == 0 {
		return ""
	}
	parts := make([]string, 0, len(deps))
	for from, tos := range deps {
		if len(tos) == 1 {
			parts = append(parts, from+" → "+tos[0])
		} else {
			parts = append(parts, from+" → {"+strings.Join(tos, ", ")+"}")
		}
	}
	// Sort for deterministic output
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}
```

Add `"sort"` to imports.

Call `s.loadArchitecture(ctx, &payload)` in `Build()` after `loadDecisions`.

Update `RenderText` in `render.go` to include architecture:

```go
// After project info block, before freshness:
if len(payload.Architecture.EntryPoints) > 0 {
	fmt.Fprintf(&b, "Entry points: %s\n", strings.Join(payload.Architecture.EntryPoints, ", "))
}
if payload.Architecture.DependencyFlow != "" {
	fmt.Fprintf(&b, "Dependency flow: %s\n", payload.Architecture.DependencyFlow)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/orient/ -run TestBuildArchitectureSection -v` Expected:
PASS

**Step 5: Run all orient tests**

Run: `go test ./internal/orient/ -v` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/orient/service.go internal/orient/render.go internal/orient/service_test.go
git commit -m "feat(orient): add architecture section with entry points and dependency flow"
```

---

### Task 5: Richer `orient` output — module heat via git log

**Files:**

- Modify: `internal/orient/service.go` (ModuleSummary struct, new loadHeat
  method)
- Modify: `internal/orient/render.go`
- Test: `internal/orient/service_test.go`

**Step 1: Write the failing test**

Add to `internal/orient/service_test.go`:

```go
func TestBuildModuleHeat(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatalf("write pkg/a.go: %v", err)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	run("add", ".")
	run("commit", "-m", "init")

	// Make multiple recent commits touching main.go to make root "hot"
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("package main\nfunc main(){}\n// change %d\n", i)
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("write main.go change %d: %v", i, err)
		}
		run("add", "main.go")
		run("commit", "-m", fmt.Sprintf("change %d", i))
	}

	conn := setupOrientDB(t, root)
	defer conn.Close()
	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, m := range payload.Modules {
		if m.Path == "." && m.Heat != "hot" {
			t.Fatalf("expected root module to be hot, got %s", m.Heat)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/orient/ -run TestBuildModuleHeat -v` Expected: FAIL
(Heat field doesn't exist)

**Step 3: Write minimal implementation**

Add `Heat` and `RecentCommits` to `ModuleSummary`:

```go
type ModuleSummary struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	FileCount     int    `json:"file_count"`
	LineCount     int    `json:"line_count"`
	Heat          string `json:"heat"`
	RecentCommits int    `json:"recent_commits"`
}
```

Add heat computation method:

```go
func (s *Service) loadModuleHeat(ctx context.Context, moduleRoot string, payload *Payload) {
	// Get recent file changes from git log
	cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "log", "--since=2 weeks ago", "--name-only", "--pretty=format:")
	out, err := cmd.Output()
	if err != nil {
		return // Non-fatal: heat is optional
	}

	// Count commits per module directory
	counts := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dir := filepath.Dir(line)
		if dir == "." {
			counts["."]++
		} else {
			// Walk up to find matching module path
			for _, m := range payload.Modules {
				if strings.HasPrefix(filepath.ToSlash(dir), m.Path) || m.Path == "." && !strings.Contains(dir, "/") {
					counts[m.Path]++
					break
				}
			}
		}
	}

	// Assign heat labels
	for i := range payload.Modules {
		c := counts[payload.Modules[i].Path]
		payload.Modules[i].RecentCommits = c
		switch {
		case c >= 4:
			payload.Modules[i].Heat = "hot"
		case c >= 1:
			payload.Modules[i].Heat = "warm"
		default:
			payload.Modules[i].Heat = "cold"
		}
	}
}
```

Add `"os/exec"` to imports. Call after `loadModules`.

Update `RenderText` in `render.go`:

```go
// Replace module line:
fmt.Fprintf(&b, "- %s (%s): %d files, %d lines [%s]\n", m.Path, m.Name, m.FileCount, m.LineCount, strings.ToUpper(m.Heat))
```

In text mode, omit COLD modules (priority truncation):

```go
for _, m := range payload.Modules {
	if m.Heat == "cold" {
		continue // Omit cold modules in text mode
	}
	fmt.Fprintf(&b, "- %s (%s): %d files, %d lines [%s]\n", m.Path, m.Name, m.FileCount, m.LineCount, strings.ToUpper(m.Heat))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/orient/ -run TestBuildModuleHeat -v` Expected: PASS

**Step 5: Run all orient tests**

Run: `go test ./internal/orient/ -v` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/orient/service.go internal/orient/render.go internal/orient/service_test.go
git commit -m "feat(orient): add module heat labels from git log"
```

---

### Task 6: Richer `orient` output — recent activity

**Files:**

- Modify: `internal/orient/service.go`
- Modify: `internal/orient/render.go`
- Test: `internal/orient/service_test.go`

**Step 1: Write the failing test**

Add to `internal/orient/service_test.go`:

```go
func TestBuildRecentActivity(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	run("add", ".")
	run("commit", "-m", "init")

	conn := setupOrientDB(t, root)
	defer conn.Close()
	if _, err := index.NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("sync: %v", err)
	}

	payload, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.RecentActivity) == 0 {
		t.Fatal("expected recent activity")
	}
	if payload.RecentActivity[0].File != "main.go" && payload.RecentActivity[0].File != "go.mod" {
		t.Fatalf("unexpected recent activity file: %s", payload.RecentActivity[0].File)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/orient/ -run TestBuildRecentActivity -v` Expected: FAIL

**Step 3: Write minimal implementation**

Add to `internal/orient/service.go`:

```go
type RecentFile struct {
	File         string `json:"file"`
	LastModified string `json:"last_modified"`
}
```

Add `RecentActivity` to `Payload`:

```go
RecentActivity  []RecentFile     `json:"recent_activity"`
```

Add method:

```go
func (s *Service) loadRecentActivity(ctx context.Context, moduleRoot string, payload *Payload) {
	cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "log", "-n", "20", "--pretty=format:%aI", "--name-only", "--diff-filter=ACMR")
	out, err := cmd.Output()
	if err != nil {
		return // Non-fatal
	}

	seen := map[string]bool{}
	activity := []RecentFile{}
	var currentDate string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ISO date lines start with 2
		if len(line) > 10 && line[4] == '-' {
			currentDate = line
			continue
		}
		if !seen[line] && currentDate != "" {
			seen[line] = true
			activity = append(activity, RecentFile{File: line, LastModified: currentDate})
			if len(activity) >= 5 {
				break
			}
		}
	}
	payload.RecentActivity = activity
}
```

Initialize in `Build()`:

```go
payload.RecentActivity = []RecentFile{}
```

Call `s.loadRecentActivity(ctx, opts.ModuleRoot, &payload)` in Build after heat.

Update `RenderText`:

```go
if len(payload.RecentActivity) > 0 {
	b.WriteString("\nRecent activity:\n")
	for _, a := range payload.RecentActivity {
		fmt.Fprintf(&b, "- %s (%s)\n", a.File, a.LastModified)
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/orient/ -run TestBuildRecentActivity -v` Expected: PASS

**Step 5: Run all orient + CLI tests**

Run: `go test ./internal/orient/ ./internal/cli/ -v` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/orient/service.go internal/orient/render.go internal/orient/service_test.go
git commit -m "feat(orient): add recent activity from git log"
```

---

## Phase B: Knowledge Expansion

### Task 7: DB migration for `patterns` and `pattern_files` tables

**Files:**

- Create: `internal/db/migrations/000003_patterns.up.sql`
- Create: `internal/db/migrations/000003_patterns.down.sql`
- Test: `internal/db/migrate_test.go` (or a new test verifying tables exist)

**Step 1: Write the failing test**

Add a test that runs migrations and checks for the patterns table:

```go
func TestMigration003PatternsTable(t *testing.T) {
	root := t.TempDir()
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify patterns table exists
	_, err = conn.Exec(`INSERT INTO patterns (title, description, confidence, status, created_at, updated_at) VALUES ('test', 'desc', 'medium', 'active', 'x', 'x')`)
	if err != nil {
		t.Fatalf("patterns table should exist: %v", err)
	}

	// Verify pattern_files table exists
	_, err = conn.Exec(`INSERT INTO pattern_files (pattern_id, file_path) VALUES (1, 'main.go')`)
	if err != nil {
		t.Fatalf("pattern_files table should exist: %v", err)
	}
}
```

Place this test in an appropriate test file (e.g.,
`internal/db/migrate_test.go`).

**Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestMigration003PatternsTable -v` Expected:
FAIL (table doesn't exist)

**Step 3: Write the migration**

Create `internal/db/migrations/000003_patterns.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS patterns (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    confidence  TEXT NOT NULL DEFAULT 'medium',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pattern_files (
    id         INTEGER PRIMARY KEY,
    pattern_id INTEGER REFERENCES patterns(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL
);
```

Create `internal/db/migrations/000003_patterns.down.sql`:

```sql
DROP TABLE IF EXISTS pattern_files;
DROP TABLE IF EXISTS patterns;
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestMigration003PatternsTable -v` Expected:
PASS

**Step 5: Run all tests**

Run: `go test ./... -count=1` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/db/migrations/000003_patterns.up.sql internal/db/migrations/000003_patterns.down.sql internal/db/migrate_test.go
git commit -m "feat(db): add migration 003 for patterns and pattern_files tables"
```

---

### Task 8: `pattern` service layer

**Files:**

- Create: `internal/pattern/service.go`
- Create: `internal/pattern/service_test.go`

**Step 1: Write the failing test**

Create `internal/pattern/service_test.go`:

```go
package pattern

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func patternTestDB(t *testing.T) (*sql.DB, string, func()) {
	t.Helper()
	root := t.TempDir()
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	// Seed a go file for grep checks
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Errorf(\"fail: %w\", err) }\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	return conn, root, func() { _ = conn.Close() }
}

func TestProposeAndVerifyPatternPromoted(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	defer cleanup()

	result, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "Error wrapping with %w",
		Description:     "All errors use fmt.Errorf with %w wrapping",
		Example:         `return fmt.Errorf("fail: %w", err)`,
		Confidence:      "high",
		EvidenceSummary: "grep finds %w usage",
		CheckType:       "grep_pattern",
		CheckSpec:       `{"pattern":"fmt\\.Errorf.*%w","scope":"*.go"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("ProposeAndVerifyPattern: %v", err)
	}
	if !result.Promoted || result.PatternID == 0 {
		t.Fatalf("expected promoted pattern, got %+v", result)
	}
}

func TestProposeAndVerifyPatternNotPromoted(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	defer cleanup()

	result, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "Uses panic",
		Description:     "Code uses panic for error handling",
		Confidence:      "medium",
		EvidenceSummary: "grep finds panic usage",
		CheckType:       "grep_pattern",
		CheckSpec:       `{"pattern":"panic\\(","scope":"*.go"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("ProposeAndVerifyPattern: %v", err)
	}
	if result.Promoted {
		t.Fatalf("expected not promoted, got %+v", result)
	}
}

func TestProposePatternValidation(t *testing.T) {
	conn, _, cleanup := patternTestDB(t)
	defer cleanup()

	_, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pattern/ -v` Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

Create `internal/pattern/service.go`:

```go
package pattern

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robertguss/recon/internal/knowledge"
)

type ProposePatternInput struct {
	Title           string
	Description     string
	Example         string
	Confidence      string
	EvidenceSummary string
	CheckType       string
	CheckSpec       string
	ModuleRoot      string
}

type ProposePatternResult struct {
	ProposalID          int64  `json:"proposal_id"`
	PatternID           int64  `json:"pattern_id,omitempty"`
	Promoted            bool   `json:"promoted"`
	VerificationPassed  bool   `json:"verification_passed"`
	VerificationDetails string `json:"verification_details"`
}

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) ProposeAndVerifyPattern(ctx context.Context, in ProposePatternInput) (ProposePatternResult, error) {
	if strings.TrimSpace(in.Title) == "" {
		return ProposePatternResult{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.EvidenceSummary) == "" {
		return ProposePatternResult{}, fmt.Errorf("evidence summary is required")
	}
	if strings.TrimSpace(in.CheckType) == "" {
		return ProposePatternResult{}, fmt.Errorf("check type is required")
	}
	if strings.TrimSpace(in.CheckSpec) == "" {
		return ProposePatternResult{}, fmt.Errorf("check spec is required")
	}

	confidence := strings.TrimSpace(in.Confidence)
	if confidence == "" {
		confidence = "medium"
	}

	// Reuse the knowledge service's check runner
	knowledgeSvc := knowledge.NewService(s.db)
	outcome := knowledgeSvc.RunCheckPublic(ctx, in.CheckType, in.CheckSpec, in.ModuleRoot)

	now := time.Now().UTC().Format(time.RFC3339)

	entityData := map[string]any{
		"title":            in.Title,
		"description":      in.Description,
		"example":          in.Example,
		"confidence":       confidence,
		"evidence_summary": in.EvidenceSummary,
		"check_type":       in.CheckType,
		"check_spec":       in.CheckSpec,
	}
	entityDataJSON, err := json.Marshal(entityData)
	if err != nil {
		return ProposePatternResult{}, fmt.Errorf("marshal proposal data: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposePatternResult{}, fmt.Errorf("begin pattern tx: %w", err)
	}
	defer tx.Rollback()

	// Insert proposal
	res, err := tx.ExecContext(ctx, `
INSERT INTO proposals (session_id, entity_type, entity_data, status, proposed_at)
VALUES (NULL, 'pattern', ?, 'pending', ?);
`, string(entityDataJSON), now)
	if err != nil {
		return ProposePatternResult{}, fmt.Errorf("insert proposal: %w", err)
	}
	proposalID, _ := res.LastInsertId()

	baselineJSON, _ := json.Marshal(outcome.Baseline)
	lastResultJSON, _ := json.Marshal(map[string]any{"passed": outcome.Passed, "details": outcome.Details})

	if outcome.Passed {
		patternRes, err := tx.ExecContext(ctx, `
INSERT INTO patterns (title, description, confidence, status, created_at, updated_at)
VALUES (?, ?, ?, 'active', ?, ?);
`, in.Title, in.Description, confidence, now, now)
		if err != nil {
			return ProposePatternResult{}, fmt.Errorf("insert pattern: %w", err)
		}
		patternID, _ := patternRes.LastInsertId()

		if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence (entity_type, entity_id, summary, check_type, check_spec, baseline, last_verified_at, last_result, drift_status)
VALUES ('pattern', ?, ?, ?, ?, ?, ?, ?, 'ok');
`, patternID, in.EvidenceSummary, in.CheckType, in.CheckSpec, string(baselineJSON), now, string(lastResultJSON)); err != nil {
			return ProposePatternResult{}, fmt.Errorf("insert pattern evidence: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE proposals SET status = 'promoted', verified_at = ?, promoted_at = ? WHERE id = ?;
`, now, now, proposalID); err != nil {
			return ProposePatternResult{}, fmt.Errorf("update proposal: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO search_index (title, content, entity_type, entity_id)
VALUES (?, ?, 'pattern', ?);
`, in.Title, in.Description+"\n"+in.Example+"\n"+in.EvidenceSummary, patternID); err != nil {
			return ProposePatternResult{}, fmt.Errorf("insert search index: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return ProposePatternResult{}, fmt.Errorf("commit pattern tx: %w", err)
		}
		return ProposePatternResult{ProposalID: proposalID, PatternID: patternID, Promoted: true, VerificationPassed: true, VerificationDetails: outcome.Details}, nil
	}

	// Not promoted
	if _, err := tx.ExecContext(ctx, `
INSERT INTO evidence (entity_type, entity_id, summary, check_type, check_spec, baseline, last_verified_at, last_result, drift_status)
VALUES ('proposal', ?, ?, ?, ?, ?, ?, ?, 'broken');
`, proposalID, "verification failed: "+outcome.Details, in.CheckType, in.CheckSpec, string(baselineJSON), now, string(lastResultJSON)); err != nil {
		return ProposePatternResult{}, fmt.Errorf("insert proposal evidence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ProposePatternResult{}, fmt.Errorf("commit pending pattern tx: %w", err)
	}

	return ProposePatternResult{ProposalID: proposalID, Promoted: false, VerificationPassed: false, VerificationDetails: outcome.Details}, nil
}
```

**NOTE:** This requires exposing the check runner from the knowledge service.
Add a public method to `internal/knowledge/service.go`:

```go
type CheckOutcome struct {
	Passed   bool
	Details  string
	Baseline map[string]any
}

func (s *Service) RunCheckPublic(ctx context.Context, checkType, checkSpec, moduleRoot string) CheckOutcome {
	outcome, err := s.runCheck(ctx, ProposeDecisionInput{
		CheckType:  checkType,
		CheckSpec:  checkSpec,
		ModuleRoot: moduleRoot,
	})
	if err != nil {
		return CheckOutcome{Passed: false, Details: err.Error(), Baseline: map[string]any{"error": err.Error()}}
	}
	return CheckOutcome{Passed: outcome.Passed, Details: outcome.Details, Baseline: outcome.Baseline}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pattern/ -v` Expected: All PASS

**Step 5: Run all tests**

Run: `go test ./... -count=1` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/pattern/service.go internal/pattern/service_test.go internal/knowledge/service.go
git commit -m "feat(pattern): add pattern service with propose/verify/promote lifecycle"
```

---

### Task 9: `pattern` CLI command

**Files:**

- Create: `internal/cli/pattern.go`
- Modify: `internal/cli/root.go:44-49` (add pattern command)
- Test: `internal/cli/commands_test.go`

**Step 1: Write the failing test**

Add to `internal/cli/commands_test.go`:

```go
func TestPatternCommand(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write("go.mod", "module example.com/test\n")
	write("main.go", "package main\nimport \"fmt\"\nfunc main() { fmt.Errorf(\"err: %w\", err) }\n")

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Promoted pattern
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Error wrapping",
		"--description", "Use %w wrapping",
		"--evidence-summary", "grep finds %w",
		"--check-type", "grep_pattern",
		"--check-pattern", "Errorf",
		"--json",
	})
	if err != nil || !strings.Contains(out, `"promoted": true`) {
		t.Fatalf("pattern promoted failed out=%q err=%v", out, err)
	}

	// Pattern text output
	out, _, err = runCommandWithCapture(t, newPatternCommand(app), []string{
		"Another pattern",
		"--description", "desc",
		"--evidence-summary", "go.mod exists",
		"--check-type", "file_exists",
		"--check-path", "go.mod",
	})
	if err != nil || !strings.Contains(out, "Pattern promoted") {
		t.Fatalf("pattern text failed out=%q err=%v", out, err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestPatternCommand -v` Expected: FAIL
(newPatternCommand doesn't exist)

**Step 3: Write minimal implementation**

Create `internal/cli/pattern.go` following the `decide.go` pattern closely. Same
flag structure (--check-type, --check-path, --check-symbol, --check-pattern,
--check-scope, --check-spec) plus --description, --example, --evidence-summary,
--confidence. Reuse `buildCheckSpec` from decide.go.

Wire into `root.go`:

```go
root.AddCommand(newPatternCommand(app))
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestPatternCommand -v` Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... -count=1` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/cli/pattern.go internal/cli/root.go internal/cli/commands_test.go
git commit -m "feat(cli): add pattern command with propose/verify/promote lifecycle"
```

---

### Task 10: Decision lifecycle management (`--list`, `--delete`, `--update`)

**Files:**

- Modify: `internal/knowledge/service.go`
- Modify: `internal/cli/decide.go`
- Test: `internal/knowledge/service_test.go`, `internal/cli/commands_test.go`

**Step 1: Write the failing test**

Add to `internal/knowledge/service_test.go`:

```go
func TestListDecisions(t *testing.T) {
	conn, cleanup := knowledgeTestDB(t)
	defer cleanup()

	items, err := NewService(conn).ListDecisions(context.Background())
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	// Should have at least the seeded decisions
	if len(items) == 0 {
		t.Fatal("expected decisions")
	}
}

func TestArchiveDecision(t *testing.T) {
	conn, cleanup := knowledgeTestDB(t)
	defer cleanup()

	err := NewService(conn).ArchiveDecision(context.Background(), 1)
	if err != nil {
		t.Fatalf("ArchiveDecision: %v", err)
	}

	items, err := NewService(conn).ListDecisions(context.Background())
	if err != nil {
		t.Fatalf("ListDecisions after archive: %v", err)
	}
	for _, item := range items {
		if item.ID == 1 {
			t.Fatal("archived decision should not appear in list")
		}
	}
}

func TestUpdateConfidence(t *testing.T) {
	conn, cleanup := knowledgeTestDB(t)
	defer cleanup()

	err := NewService(conn).UpdateConfidence(context.Background(), 1, "high")
	if err != nil {
		t.Fatalf("UpdateConfidence: %v", err)
	}
}
```

Note: You'll need to verify/adjust which test DB helper exists in
`knowledge/service_test.go` and what test data it seeds. If `knowledgeTestDB`
doesn't exist, create a setup helper similar to `findTestDB`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/knowledge/ -run "TestList|TestArchive|TestUpdate" -v`
Expected: FAIL

**Step 3: Write minimal implementation**

Add to `internal/knowledge/service.go`:

```go
type DecisionListItem struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Confidence string `json:"confidence"`
	Status     string `json:"status"`
	Drift      string `json:"drift_status"`
	UpdatedAt  string `json:"updated_at"`
}

func (s *Service) ListDecisions(ctx context.Context) ([]DecisionListItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.title, d.confidence, d.status, COALESCE(e.drift_status, 'ok'), d.updated_at
FROM decisions d
LEFT JOIN evidence e ON e.entity_type = 'decision' AND e.entity_id = d.id
WHERE d.status = 'active'
ORDER BY d.updated_at DESC;
`)
	if err != nil {
		return nil, fmt.Errorf("query decisions: %w", err)
	}
	defer rows.Close()
	items := []DecisionListItem{}
	for rows.Next() {
		var item DecisionListItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Confidence, &item.Status, &item.Drift, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ArchiveDecision(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE decisions SET status = 'archived', updated_at = ? WHERE id = ? AND status = 'active';`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("archive decision: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("decision %d not found or already archived", id)
	}
	return nil
}

func (s *Service) UpdateConfidence(ctx context.Context, id int64, confidence string) error {
	confidence = strings.TrimSpace(strings.ToLower(confidence))
	switch confidence {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("confidence must be low, medium, or high")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE decisions SET confidence = ?, updated_at = ? WHERE id = ? AND status = 'active';`, confidence, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update confidence: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("decision %d not found or archived", id)
	}
	return nil
}
```

Then wire into `decide.go` with new flags: `--list`, `--delete <id>`,
`--update <id>`. When `--list` is set, skip the normal propose flow and call
`ListDecisions`. When `--delete` is set, call `ArchiveDecision`. When `--update`
is set with `--confidence`, call `UpdateConfidence`.

Make the `<title>` arg optional when --list, --delete, or --update are used:
change `cobra.ExactArgs(1)` to a custom validator.

**Step 4: Run tests**

Run: `go test ./internal/knowledge/ -run "TestList|TestArchive|TestUpdate" -v`
Run: `go test ./internal/cli/ -v` Expected: All PASS

**Step 5: Commit**

```bash
git add internal/knowledge/service.go internal/knowledge/service_test.go internal/cli/decide.go internal/cli/commands_test.go
git commit -m "feat(decide): add --list, --delete, --update for decision lifecycle management"
```

---

### Task 11: `recon status` command

**Files:**

- Create: `internal/cli/status.go`
- Modify: `internal/cli/root.go` (add status command)
- Test: `internal/cli/commands_test.go`

**Step 1: Write the failing test**

Add to `internal/cli/commands_test.go`:

```go
func TestStatusCommand(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	// Before init
	_, _, err := runCommandWithCapture(t, newStatusCommand(app), []string{"--json"})
	if err == nil {
		t.Fatal("expected error before init")
	}

	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	// After init, before sync
	out, _, err := runCommandWithCapture(t, newStatusCommand(app), []string{"--json"})
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if !strings.Contains(out, `"initialized": true`) {
		t.Fatalf("expected initialized true, out=%q", out)
	}

	// After sync
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	out, _, err = runCommandWithCapture(t, newStatusCommand(app), []string{"--json"})
	if err != nil {
		t.Fatalf("status --json after sync: %v", err)
	}
	if !strings.Contains(out, `"files"`) || !strings.Contains(out, `"symbols"`) {
		t.Fatalf("expected counts, out=%q", out)
	}

	// Text output
	out, _, err = runCommandWithCapture(t, newStatusCommand(app), nil)
	if err != nil {
		t.Fatalf("status text: %v", err)
	}
	if !strings.Contains(out, "Initialized: yes") {
		t.Fatalf("expected text status, out=%q", out)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestStatusCommand -v` Expected: FAIL

**Step 3: Write minimal implementation**

Create `internal/cli/status.go`:

```go
package cli

import (
	"fmt"

	"github.com/robertguss/recon/internal/db"
	"github.com/spf13/cobra"
)

type statusPayload struct {
	Initialized bool         `json:"initialized"`
	Stale       bool         `json:"stale"`
	LastSyncAt  string       `json:"last_sync_at,omitempty"`
	Counts      statusCounts `json:"counts"`
}

type statusCounts struct {
	Files            int `json:"files"`
	Symbols          int `json:"symbols"`
	Packages         int `json:"packages"`
	Decisions        int `json:"decisions"`
	DecisionsDrifting int `json:"decisions_drifting"`
	Patterns         int `json:"patterns"`
}

func newStatusCommand(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Quick health check for recon state",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := openExistingDB(app)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			defer conn.Close()

			var payload statusPayload
			payload.Initialized = true

			state, exists, err := db.LoadSyncState(cmd.Context(), conn)
			if err != nil {
				if jsonOut {
					return exitJSONCommandError(err)
				}
				return err
			}
			if exists {
				payload.LastSyncAt = state.LastSyncAt.Format("2006-01-02T15:04:05Z07:00")
			}

			ctx := cmd.Context()
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&payload.Counts.Files)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols").Scan(&payload.Counts.Symbols)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages").Scan(&payload.Counts.Packages)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions WHERE status = 'active'").Scan(&payload.Counts.Decisions)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM evidence WHERE entity_type = 'decision' AND drift_status != 'ok'").Scan(&payload.Counts.DecisionsDrifting)
			_ = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM patterns WHERE status = 'active'").Scan(&payload.Counts.Patterns)

			if jsonOut {
				return writeJSON(payload)
			}

			fmt.Printf("Initialized: yes\n")
			if payload.LastSyncAt != "" {
				fmt.Printf("Last sync: %s\n", payload.LastSyncAt)
			} else {
				fmt.Printf("Last sync: never\n")
			}
			fmt.Printf("Files: %d | Symbols: %d | Packages: %d\n",
				payload.Counts.Files, payload.Counts.Symbols, payload.Counts.Packages)
			fmt.Printf("Decisions: %d (%d drifting) | Patterns: %d\n",
				payload.Counts.Decisions, payload.Counts.DecisionsDrifting, payload.Counts.Patterns)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	return cmd
}
```

Wire into `root.go`:

```go
root.AddCommand(newStatusCommand(app))
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestStatusCommand -v` Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... -count=1` Expected: All PASS

**Step 6: Commit**

```bash
git add internal/cli/status.go internal/cli/root.go internal/cli/commands_test.go
git commit -m "feat(cli): add recon status command for quick health check"
```

---

## Phase C: Workflow Polish

### Task 12: `decide --dry-run`

**Files:**

- Modify: `internal/cli/decide.go`
- Modify: `internal/knowledge/service.go`
- Test: `internal/cli/commands_test.go`

**Step 1: Write the failing test**

```go
func TestDecideDryRun(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Dry run that would pass
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"dry test", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--dry-run", "--json",
	})
	if err != nil {
		t.Fatalf("dry-run pass: %v", err)
	}
	if !strings.Contains(out, `"passed": true`) {
		t.Fatalf("expected dry-run passed, out=%q", out)
	}
	// Should NOT contain proposal_id (no state created)
	if strings.Contains(out, `"proposal_id"`) {
		t.Fatalf("dry-run should not create proposal, out=%q", out)
	}

	// Dry run that would fail
	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"dry fail", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "missing.txt",
		"--dry-run", "--json",
	})
	if err == nil {
		t.Fatal("expected dry-run failure exit")
	}
	if !strings.Contains(out, `"passed": false`) {
		t.Fatalf("expected dry-run failed, out=%q", out)
	}
}
```

**Step 2-6:** Add `--dry-run` flag to decide.go. When set, run the check via
`knowledge.Service.RunCheckPublic()` and return result without creating any DB
state. Return a `DryRunResult` struct with `Passed`, `Details` fields. Commit.

---

### Task 13: Better missing-args errors

**Files:**

- Modify: `internal/cli/find.go` (already handled in Task 3 for find)
- Modify: `internal/cli/decide.go`
- Modify: `internal/cli/recall.go`
- Test: `internal/cli/commands_test.go`

**Step 1: Write the failing test**

```go
func TestMissingArgsStructuredErrors(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	// recall with no args, --json
	out, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{"--json"})
	if err == nil {
		t.Fatal("expected recall missing arg error")
	}
	if !strings.Contains(out, `"code": "missing_argument"`) {
		t.Fatalf("expected missing_argument envelope for recall, out=%q", out)
	}

	// decide with no args, --json (no title)
	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod", "--json",
	})
	if err == nil {
		t.Fatal("expected decide missing arg error")
	}
	if !strings.Contains(out, `"code": "missing_argument"`) {
		t.Fatalf("expected missing_argument envelope for decide, out=%q", out)
	}
}
```

**Step 2-6:** For `recall` and `decide`, change `cobra.ExactArgs(1)` to a custom
`Args` function that emits structured JSON errors when `--json` is set. Pattern:
use `cobra.MaximumNArgs(1)` or a custom function, check `len(args) == 0` at the
top of RunE. Commit.

---

### Task 14: `--check-type` validation fix

**Files:**

- Modify: `internal/cli/decide.go:114-171` (buildCheckSpec function)
- Test: `internal/cli/commands_test.go`

**Step 1: Write the failing test**

```go
func TestDecideInvalidCheckTypeError(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"bad type", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "invalid_type", "--check-path", "go.mod", "--json",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out, "unknown check type") || !strings.Contains(out, "invalid_type") {
		t.Fatalf("expected specific error about unknown check type, out=%q", out)
	}
}
```

**Step 2-6:** In `buildCheckSpec`, validate `checkType` against known types
**before** checking whether typed flags are provided. Move the
`supportedCheckType` check earlier. Return:
`unsupported check type "invalid_type"; must be one of: grep_pattern, symbol_exists, file_exists`.
Commit.

---

## Phase D: Nice to Have

### Task 15: `find` receiver syntax (`Service.Find`)

**Files:**

- Modify: `internal/find/service.go`
- Modify: `internal/cli/find.go`
- Test: `internal/find/service_test.go`

**Step 1: Write the failing test**

```go
func TestFindReceiverDotSyntax(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	// "T.Ambig" should match the method with receiver T
	res, err := NewService(conn).Find(context.Background(), "T.Ambig", QueryOptions{})
	if err != nil {
		t.Fatalf("Find T.Ambig error: %v", err)
	}
	if res.Symbol.Receiver != "T" || res.Symbol.Name != "Ambig" {
		t.Fatalf("expected T.Ambig, got %s.%s", res.Symbol.Receiver, res.Symbol.Name)
	}
}
```

**Step 2-6:** In `Find()`, check if `symbol` contains `.`. If so, split into
receiver and name parts. Query with `WHERE s.name = ? AND s.receiver = ?`.
Commit.

---

### Task 16: `recall` search improvement

**Files:**

- Modify: `internal/knowledge/service.go` (search_index population)
- Modify: `internal/recall/service.go`
- Test: `internal/recall/service_test.go`

**Step 1: Write the failing test**

```go
func TestRecallFindsDecisionByRelatedTerms(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	// Create a decision with title "Use Cobra for CLI"
	svc := knowledge.NewService(conn)
	// ... create decision with title "Use Cobra for CLI", reasoning "Cobra CLI framework is standard for Go CLIs"
	// ... (use ProposeAndVerifyDecision with a file_exists check for go.mod)

	recallSvc := NewService(conn)
	result, err := recallSvc.Recall(context.Background(), "CLI framework", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected recall to find 'Use Cobra for CLI' via 'CLI framework'")
	}
}
```

**Step 2-6:** Ensure the FTS5 search_index content includes
`title + " " + reasoning + " " + evidenceSummary` (currently it's
`reasoning + "\n" + evidenceSummary` for content, with title separate — the FTS
should match across both columns). If porter tokenizer is working, "CLI
framework" should match "CLI" in title and "framework" in reasoning. Verify and
fix if needed. Commit.

---

### Task 17: Confidence decay on drift

**Files:**

- Modify: `internal/index/service.go` (or a new drift check method)
- Modify: `internal/knowledge/service.go`
- Test: `internal/knowledge/service_test.go`

**Step 1: Write the failing test**

```go
func TestConfidenceDecaysOnDrift(t *testing.T) {
	// Create a decision with high confidence
	// Manually set its evidence drift_status to 'drifting'
	// Run the decay check
	// Verify confidence is now 'medium'
}
```

**Step 2-6:** Add a `DecayConfidenceOnDrift(ctx)` method to the knowledge
service that queries decisions where evidence is drifting/broken and downgrades
confidence. Call this during sync or orient. Commit.

---

### Task 18: Surface patterns in orient output

**Files:**

- Modify: `internal/orient/service.go`
- Modify: `internal/orient/render.go`
- Test: `internal/orient/service_test.go`

**Step 1: Write the failing test**

```go
func TestOrientShowsActivePatterns(t *testing.T) {
	// Setup DB with a promoted pattern
	// Build orient payload
	// Verify ActivePatterns field is populated
}
```

**Step 2-6:** Add `ActivePatterns []PatternDigest` to Payload. Query patterns
table in `loadPatterns()`. Cap at 5 for text mode. Render in text output.
Commit.

---

## Final Verification

### Task 19: Full test suite and coverage check

**Step 1:** Run full test suite:

```bash
go test ./... -count=1 -v
```

**Step 2:** Check coverage:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1
```

Expected: 100% or very close (matching M1-M3 standard)

**Step 3:** Run the CLI e2e manually:

```bash
go build -o recon ./cmd/recon
./recon init --json
./recon sync --json
./recon status --json
./recon orient --json
./recon find --package internal/cli --json
./recon find Service --file service.go --json
./recon pattern "Test pattern" --description "test" --evidence-summary "file exists" --check-type file_exists --check-path go.mod --json
./recon decide --list --json
./recon decide "test" --reasoning "r" --evidence-summary "e" --check-type file_exists --check-path go.mod --dry-run --json
./recon recall "pattern" --json
```

**Step 4:** Commit any final fixes, then create a final commit:

```bash
git commit -m "chore: milestone 4 dogfood findings implementation complete"
```
