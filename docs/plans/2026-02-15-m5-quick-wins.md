# M5 Quick Wins Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Fix the four quick-win bugs/gaps identified in the M5 dogfood
findings: orient text hides cold modules, heat window too narrow, no package
listing, and no short package name matching.

**Architecture:** Each fix is a small, isolated change to one service + its
renderer or CLI layer. All changes are in `internal/orient/` and
`internal/find/`. No new packages, no schema changes, no new dependencies.

**Tech Stack:** Go, SQLite, Cobra CLI, go-sqlmock for error tests.

---

### Task 1: Fix orient text to always show modules (BUG-1)

**Files:**

- Modify: `internal/orient/render.go:42-51`
- Modify: `internal/orient/render_test.go`

**Context:** `RenderText()` in `render.go:42-51` skips modules where
`m.Heat == "cold"`. When all modules are cold (no commits in the heat window),
the output says "Modules: (none)". The JSON output shows all modules correctly.
The fix: always render every module, but annotate cold ones with `[COLD]`
instead of hiding them.

**Step 1: Update the existing test to expect cold modules in output**

Open `internal/orient/render_test.go`. The test `TestRenderTextAllColdModules`
(line 42) currently asserts `(none)` appears when all modules are cold. Change
it to expect the cold modules to be listed with `[COLD]` annotation.

Also update `TestRenderTextColdModulesHidden` (line 25) — rename it to
`TestRenderTextColdModulesAnnotated`. It should verify that cold modules appear
with `[COLD]` and hot modules appear with `[HOT]`.

```go
func TestRenderTextColdModulesAnnotated(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		Modules: []ModuleSummary{
			{Path: "hot", Name: "hot", FileCount: 1, LineCount: 10, Heat: "hot"},
			{Path: "cold", Name: "cold", FileCount: 1, LineCount: 5, Heat: "cold"},
		},
	}
	got := RenderText(payload)
	if !strings.Contains(got, "hot (hot)") {
		t.Fatalf("expected hot module in output: %s", got)
	}
	if !strings.Contains(got, "[HOT]") {
		t.Fatalf("expected [HOT] annotation: %s", got)
	}
	if !strings.Contains(got, "cold (cold)") {
		t.Fatalf("expected cold module in output: %s", got)
	}
	if !strings.Contains(got, "[COLD]") {
		t.Fatalf("expected [COLD] annotation: %s", got)
	}
}

func TestRenderTextAllColdModules(t *testing.T) {
	payload := Payload{
		Project: ProjectInfo{Name: "x", ModulePath: "m", Language: "go"},
		Modules: []ModuleSummary{
			{Path: "a", Name: "a", FileCount: 1, LineCount: 10, Heat: "cold"},
			{Path: "b", Name: "b", FileCount: 2, LineCount: 20, Heat: "cold"},
		},
	}
	got := RenderText(payload)
	if strings.Contains(got, "- (none)") {
		t.Fatalf("should not say (none) when modules exist, got:\n%s", got)
	}
	if !strings.Contains(got, "a (a)") || !strings.Contains(got, "b (b)") {
		t.Fatalf("expected all cold modules listed, got:\n%s", got)
	}
	if !strings.Contains(got, "[COLD]") {
		t.Fatalf("expected [COLD] annotation, got:\n%s", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
`go test ./internal/orient/... -run "TestRenderTextColdModules|TestRenderTextAllCold" -v`
Expected: FAIL — the current code hides cold modules.

**Step 3: Fix the render logic**

In `internal/orient/render.go`, replace lines 40-52 (the module rendering block
inside the `else`) with code that always prints every module:

```go
} else {
	for _, m := range payload.Modules {
		fmt.Fprintf(&b, "- %s (%s): %d files, %d lines [%s]\n", m.Path, m.Name, m.FileCount, m.LineCount, strings.ToUpper(m.Heat))
	}
}
```

This removes the `if m.Heat == "cold" { continue }` filter and the
`printedModule` tracking. All modules are always printed. The
`[COLD]`/`[HOT]`/`[WARM]` annotation was already there via
`strings.ToUpper(m.Heat)`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/orient/... -v` Expected: ALL PASS

**Step 5: Run full test suite**

Run: `just test` Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/orient/render.go internal/orient/render_test.go
git commit -m "fix(orient): always show modules in text output, annotate cold ones

Previously, RenderText filtered out modules with heat=cold, showing
'(none)' when all modules were cold. Now all modules are always shown
with their heat annotation ([HOT], [WARM], [COLD]).

Fixes BUG-1 from M5 dogfood findings."
```

---

### Task 2: Widen heat window from 2 weeks to 30 days (BUG-3)

**Files:**

- Modify: `internal/orient/service.go:368`
- Modify: `internal/orient/service_test.go` (if any test references the window)

**Context:** `loadModuleHeat()` at `service.go:368` uses `--since=2 weeks ago`.
Any repo with a 2+ week development pause shows everything as cold. Widen to 30
days.

**Step 1: Check for existing tests that reference the heat window**

Run: `grep -n "2 weeks" internal/orient/` Note: `loadModuleHeat` shells out to
`git log` so it's not unit-testable with a mock DB. The change is a single
string constant. No new test needed — the existing integration behavior will use
the wider window.

**Step 2: Change the git log window**

In `internal/orient/service.go:368`, change:

```go
// Before:
cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "log", "--since=2 weeks ago", "--name-only", "--pretty=format:")

// After:
cmd := exec.CommandContext(ctx, "git", "-C", moduleRoot, "log", "--since=30 days ago", "--name-only", "--pretty=format:")
```

**Step 3: Run full test suite**

Run: `just test` Expected: ALL PASS

**Step 4: Commit**

```bash
git add internal/orient/service.go
git commit -m "fix(orient): widen heat window from 2 weeks to 30 days

A 2-week window meant any repo with a brief development pause showed
all modules as cold. 30 days is more practical for real development
cycles.

Fixes BUG-3 from M5 dogfood findings."
```

---

### Task 3: Add short package name matching to `--package` filter (BUG-2)

**Files:**

- Modify: `internal/find/service.go:132-138` (`buildListWhere`)
- Modify: `internal/find/service.go:281-296` (`filterMatches`)
- Modify: `internal/find/service_test.go`

**Context:** `--package index` fails when the stored path is `internal/index`.
File filtering already supports suffix matching (lines 140-147) but package
filtering uses exact equality only (line 136 and line 284). The fix: add
last-segment matching so `--package index` matches `internal/index`.

**Step 1: Write failing tests for short package name matching**

Add these tests to `internal/find/service_test.go`:

```go
func TestFindWithShortPackageName(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	// Add a symbol in a nested package
	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (2,'internal/index','index','example.com/recon/internal/index',1,50,'x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (3,2,'internal/index/service.go','go',50,'h3','x','x');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (5,3,'func','NewService','func()','func NewService(){}',1,1,1,'');`)

	// Short name should match
	res, err := NewService(conn).Find(context.Background(), "NewService", QueryOptions{PackagePath: "index"})
	if err != nil {
		t.Fatalf("Find with short package name error: %v", err)
	}
	if res.Symbol.Package != "internal/index" {
		t.Fatalf("expected package internal/index, got %s", res.Symbol.Package)
	}
}

func TestListWithShortPackageName(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	// Add a symbol in a nested package
	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (2,'internal/index','index','example.com/recon/internal/index',1,50,'x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (3,2,'internal/index/service.go','go',50,'h3','x','x');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (5,3,'func','NewService','func()','func NewService(){}',1,1,1,'');`)

	// Short name should match in list mode
	result, err := NewService(conn).List(context.Background(), QueryOptions{PackagePath: "index"}, 50)
	if err != nil {
		t.Fatalf("List with short package name error: %v", err)
	}
	if len(result.Symbols) == 0 {
		t.Fatalf("expected symbols from short package name, got none")
	}
	if result.Symbols[0].Package != "internal/index" {
		t.Fatalf("expected package internal/index, got %s", result.Symbols[0].Package)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
`go test ./internal/find/... -run "TestFindWithShortPackageName|TestListWithShortPackageName" -v`
Expected: FAIL — short package names don't match.

**Step 3: Add suffix matching to `buildListWhere`**

In `internal/find/service.go`, replace the package filtering in `buildListWhere`
(lines 135-138):

```go
// Before:
if opts.PackagePath != "" {
	clauses = append(clauses, "COALESCE(p.path, '.') = ?")
	args = append(args, opts.PackagePath)
}

// After:
if opts.PackagePath != "" {
	if !strings.Contains(opts.PackagePath, "/") {
		// Short name: match exact or last path segment
		clauses = append(clauses, "(COALESCE(p.path, '.') = ? OR COALESCE(p.path, '.') LIKE ?)")
		args = append(args, opts.PackagePath, "%/"+opts.PackagePath)
	} else {
		clauses = append(clauses, "COALESCE(p.path, '.') = ?")
		args = append(args, opts.PackagePath)
	}
}
```

**Step 4: Add suffix matching to `filterMatches`**

In `internal/find/service.go`, replace the package check in `filterMatches`
(line 284):

```go
// Before:
if opts.PackagePath != "" && match.Package != opts.PackagePath {
	continue
}

// After:
if opts.PackagePath != "" && !matchPackagePath(match.Package, opts.PackagePath) {
	continue
}
```

Add the helper function after `filterMatches`:

```go
func matchPackagePath(pkgPath, filter string) bool {
	if pkgPath == filter {
		return true
	}
	// Short name: match last segment
	if !strings.Contains(filter, "/") {
		lastSeg := pkgPath
		if idx := strings.LastIndex(pkgPath, "/"); idx >= 0 {
			lastSeg = pkgPath[idx+1:]
		}
		return lastSeg == filter
	}
	return false
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/find/... -v` Expected: ALL PASS

**Step 6: Run full test suite**

Run: `just test` Expected: ALL PASS

**Step 7: Commit**

```bash
git add internal/find/service.go internal/find/service_test.go
git commit -m "fix(find): support short package names in --package filter

--package 'index' now matches 'internal/index' by last-segment matching,
consistent with how --file already supports basename matching. Full paths
still work as before.

Fixes BUG-2 from M5 dogfood findings."
```

---

### Task 4: Add `--list-packages` flag to find command (MISS-2)

**Files:**

- Modify: `internal/find/service.go` (add `ListPackages` method)
- Modify: `internal/find/service_test.go` (add tests)
- Modify: `internal/cli/find.go` (add flag and rendering)

**Context:** There's no way to list packages. An agent must hack it with
`find --kind type | grep pkg=`. Add a `--list-packages` flag that queries the
`packages` table directly and shows file/line counts.

**Step 1: Write failing test for `ListPackages` service method**

Add to `internal/find/service_test.go`:

```go
func TestListPackages(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	// Add a second package
	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (2,'internal/index','index','example.com/recon/internal/index',3,150,'x','x');`)

	pkgs, err := NewService(conn).ListPackages(context.Background())
	if err != nil {
		t.Fatalf("ListPackages error: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	// Should be ordered by line_count DESC
	if pkgs[0].Path != "internal/index" {
		t.Fatalf("expected largest package first, got %s", pkgs[0].Path)
	}
	if pkgs[0].FileCount != 3 || pkgs[0].LineCount != 150 {
		t.Fatalf("unexpected counts: %+v", pkgs[0])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/find/... -run TestListPackages -v` Expected: FAIL —
`ListPackages` method doesn't exist.

**Step 3: Add `PackageSummary` type and `ListPackages` method**

Add to `internal/find/service.go`, after the existing `ListResult` type (around
line 80):

```go
type PackageSummary struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
	LineCount int    `json:"line_count"`
}

func (s *Service) ListPackages(ctx context.Context) ([]PackageSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, name, file_count, line_count FROM packages ORDER BY line_count DESC`)
	if err != nil {
		return nil, fmt.Errorf("query packages: %w", err)
	}
	defer rows.Close()

	var pkgs []PackageSummary
	for rows.Next() {
		var p PackageSummary
		if err := rows.Scan(&p.Path, &p.Name, &p.FileCount, &p.LineCount); err != nil {
			return nil, fmt.Errorf("scan package: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate packages: %w", err)
	}
	return pkgs, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/find/... -run TestListPackages -v` Expected: PASS

**Step 5: Write failing test for CLI `--list-packages` flag**

Add to `internal/cli/commands_test.go` or the appropriate CLI test file — this
depends on where find CLI tests live. If they're tested via the Cobra command
execution pattern, add a test that calls find with `--list-packages`. Otherwise,
the service-level test from Step 1 is sufficient.

**Step 6: Add `--list-packages` flag to CLI**

In `internal/cli/find.go`, add a new flag variable:

```go
var (
	// ... existing vars ...
	listPackages bool
)
```

Add the flag registration (after line 165):

```go
cmd.Flags().BoolVar(&listPackages, "list-packages", false, "List all indexed packages")
```

Add handling at the top of the `RunE` function (after line 27, before the kind
normalization), so it short-circuits before other processing:

```go
if listPackages {
	conn, connErr := openExistingDB(app)
	if connErr != nil {
		if jsonOut {
			return exitJSONCommandError(connErr)
		}
		return connErr
	}
	defer conn.Close()

	pkgs, err := find.NewService(conn).ListPackages(cmd.Context())
	if err != nil {
		if jsonOut {
			_ = writeJSONError("internal_error", err.Error(), nil)
			return ExitError{Code: 2}
		}
		return err
	}

	if jsonOut {
		return writeJSON(pkgs)
	}

	fmt.Printf("Packages (%d):\n", len(pkgs))
	for _, p := range pkgs {
		fmt.Printf("- %s  %d files  %d lines\n", p.Path, p.FileCount, p.LineCount)
	}
	return nil
}
```

**Step 7: Run full test suite**

Run: `just test` Expected: ALL PASS

**Step 8: Manual smoke test**

Run: `just sync && just find -- --list-packages` Expected: Lists all 9 packages
with file/line counts, ordered by size.

Run: `just find -- --list-packages --json` Expected: JSON array of package
objects.

**Step 9: Commit**

```bash
git add internal/find/service.go internal/find/service_test.go internal/cli/find.go
git commit -m "feat(find): add --list-packages flag for package listing

Adds a quick way to see all indexed packages with file and line counts,
ordered by size. Supports both text and JSON output.

Implements MISS-2 from M5 dogfood findings."
```

---

### Task 5: Add sqlmock error tests for new code

**Files:**

- Modify: `internal/find/service_sqlmock_test.go`

**Context:** The project targets 100% test coverage. The new `ListPackages`
method and `matchPackagePath` helper need error-path tests.

**Step 1: Write sqlmock test for ListPackages errors**

Add to `internal/find/service_sqlmock_test.go`:

```go
func TestListPackagesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT path, name, file_count, line_count FROM packages").
		WillReturnError(errors.New("query fail"))

	_, err = NewService(db).ListPackages(context.Background())
	if err == nil || !strings.Contains(err.Error(), "query packages") {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestListPackagesScanError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT path, name, file_count, line_count FROM packages").
		WillReturnRows(sqlmock.NewRows([]string{"path", "name", "file_count", "line_count"}).
			AddRow("p", "n", "bad-int", 1))

	_, err = NewService(db).ListPackages(context.Background())
	if err == nil || !strings.Contains(err.Error(), "scan package") {
		t.Fatalf("expected scan error, got %v", err)
	}
}

func TestListPackagesRowsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT path, name, file_count, line_count FROM packages").
		WillReturnRows(sqlmock.NewRows([]string{"path", "name", "file_count", "line_count"}).
			AddRow("p", "n", 1, 1).
			RowError(0, errors.New("row iter fail")))

	_, err = NewService(db).ListPackages(context.Background())
	if err == nil || !strings.Contains(err.Error(), "iterate packages") {
		t.Fatalf("expected iterate error, got %v", err)
	}
}
```

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/find/... -v` Expected: ALL PASS

**Step 3: Check coverage**

Run: `just cover` Expected: 100% or close to it for `internal/find/`.

**Step 4: Commit**

```bash
git add internal/find/service_sqlmock_test.go
git commit -m "test(find): add sqlmock error tests for ListPackages

Covers query, scan, and row iteration error paths for the new
ListPackages method to maintain 100% coverage."
```

---

### Task 6: Final verification and cleanup

**Step 1: Run full test suite with race detector**

Run: `just test-race` Expected: ALL PASS, no races.

**Step 2: Run formatter**

Run: `just fmt` Expected: No changes (code should already be formatted).

**Step 3: Check coverage**

Run: `just cover` Expected: Coverage at or above previous levels.

**Step 4: Smoke test all four fixes end-to-end**

```bash
just sync

# BUG-1: Orient should show all modules (no "(none)" if modules exist)
just orient

# BUG-3: Heat should use 30-day window (verify via orient --json)
just orient -- --json | grep -o '"heat":"[^"]*"' | sort | uniq -c

# BUG-2: Short package name should work
just find -- NewService --package find

# MISS-2: List packages
just find -- --list-packages
just find -- --list-packages --json
```

**Step 5: Final commit if any cleanup needed**

Only if previous steps revealed issues. Otherwise, all four fixes are already
committed.
