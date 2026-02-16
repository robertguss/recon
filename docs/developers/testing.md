# Testing

Recon targets 100% test coverage using a combination of real SQLite databases
and SQL mocking.

## Running Tests

```bash
just test           # Full test suite
just test-race      # With race detector
just cover          # Coverage report

# Single package
go test ./internal/knowledge/...

# Single test
go test ./internal/orient/... -run TestOrientService

# Verbose
go test -v ./internal/find/...
```

## Test Strategy

### Real SQLite Tests

Most tests use real SQLite databases created in temporary directories. This
tests the full stack from service methods through SQL queries to actual SQLite
behavior.

```go
func TestFindService(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "test.db")
    conn, err := db.Open(dbPath)
    require.NoError(t, err)
    defer conn.Close()

    require.NoError(t, db.RunMigrations(conn))

    svc := find.NewService(conn)
    // ... test with real database
}
```

**When to use:** For all happy-path tests and integration scenarios. These tests
verify that SQL queries work correctly with real SQLite.

### go-sqlmock Tests

Files named `*_sqlmock_test.go` use `github.com/DATA-DOG/go-sqlmock` to test
error paths — what happens when the database returns unexpected errors.

```go
func TestFindService_DBError(t *testing.T) {
    mockDB, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer mockDB.Close()

    mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("disk error"))

    svc := find.NewService(mockDB)
    _, err = svc.Find(ctx, "Foo", find.QueryOptions{})
    require.Error(t, err)
}
```

**When to use:** For testing SQL error handling — simulating database failures,
constraint violations, and scan errors that are hard to reproduce with real
SQLite.

### CLI Tests

CLI tests execute Cobra commands with captured stdout/stderr:

```go
func TestFindCommand_NotFound(t *testing.T) {
    // Set up database with test data
    // ...

    cmd, _ := cli.NewRootCommand(context.Background())
    buf := new(bytes.Buffer)
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"find", "NonExistent", "--json"})

    err := cmd.Execute()
    // Assert error type and output
}
```

### Function-Var Override Tests

Tests replace package-level function variables to isolate behavior:

```go
func TestInitCommand_GoModMissing(t *testing.T) {
    origGetwd := osGetwd
    t.Cleanup(func() { osGetwd = origGetwd })

    osGetwd = func() (string, error) { return "/nonexistent", nil }

    // Test that init fails when go.mod is missing
}
```

**Always use `t.Cleanup`** to restore original values. This prevents test
pollution across runs.

## Test File Naming

| Pattern              | Purpose                                        |
| -------------------- | ---------------------------------------------- |
| `*_test.go`          | Primary test file — happy paths, core behavior |
| `*_sqlmock_test.go`  | SQL error path testing with go-sqlmock         |
| `*_extra_test.go`    | Additional edge cases for coverage             |
| `*_coverage_test.go` | Specific tests to fill coverage gaps           |

## Adding New Tests

### For a new service method

1. Write real SQLite tests first — verify the SQL works
2. Add sqlmock tests for error paths
3. Run `just cover` to check coverage
4. Add `*_extra_test.go` if edge cases remain uncovered

### For a new CLI command

1. Write CLI tests that execute the Cobra command
2. Test both text and JSON output modes
3. Test error cases (missing args, not initialized, not found)
4. Test with `--no-prompt` for non-interactive mode

### For a new function-var dependency

1. Declare the var at package level:
   ```go
   var myDep = realImplementation
   ```
2. In tests, override and cleanup:
   ```go
   orig := myDep
   t.Cleanup(func() { myDep = orig })
   myDep = func() error { return fmt.Errorf("mock error") }
   ```

## Coverage

The project targets 100% coverage. Check coverage with:

```bash
just cover          # Summary
just cover-html     # HTML report in browser
```

Coverage is measured across all packages. If a specific line is genuinely
unreachable (e.g., a defensive error check that can't be triggered), document
why in a `*_coverage_test.go` file.
