package pattern

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	svc := NewService(conn)
	ctx := context.Background()

	// Missing title
	_, err := svc.ProposeAndVerifyPattern(ctx, ProposePatternInput{})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title error, got %v", err)
	}

	// Missing evidence summary
	_, err = svc.ProposeAndVerifyPattern(ctx, ProposePatternInput{Title: "t"})
	if err == nil || !strings.Contains(err.Error(), "evidence summary is required") {
		t.Fatalf("expected evidence summary error, got %v", err)
	}

	// Missing check type
	_, err = svc.ProposeAndVerifyPattern(ctx, ProposePatternInput{Title: "t", EvidenceSummary: "e"})
	if err == nil || !strings.Contains(err.Error(), "check type is required") {
		t.Fatalf("expected check type error, got %v", err)
	}

	// Missing check spec
	_, err = svc.ProposeAndVerifyPattern(ctx, ProposePatternInput{Title: "t", EvidenceSummary: "e", CheckType: "file_exists"})
	if err == nil || !strings.Contains(err.Error(), "check spec is required") {
		t.Fatalf("expected check spec error, got %v", err)
	}
}

func TestProposePatternDefaultConfidence(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	defer cleanup()

	result, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "Default confidence",
		Description:     "desc",
		EvidenceSummary: "file exists",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"go.mod"}`,
		ModuleRoot:      root,
		Confidence:      "", // empty should default to medium
	})
	if err != nil {
		t.Fatalf("ProposeAndVerifyPattern: %v", err)
	}
	if !result.Promoted {
		t.Fatalf("expected promoted, got %+v", result)
	}

	var confidence string
	if err := conn.QueryRow(`SELECT confidence FROM patterns WHERE id = ?`, result.PatternID).Scan(&confidence); err != nil {
		t.Fatalf("query confidence: %v", err)
	}
	if confidence != "medium" {
		t.Fatalf("expected medium confidence default, got %q", confidence)
	}
}

func seedSymbol(t *testing.T, conn *sql.DB, name string) {
	t.Helper()
	now := "2024-01-01T00:00:00Z"
	_, err := conn.Exec(`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('pkg', 'pkg', 'example.com/test/pkg', ?, ?)`, now, now)
	if err != nil {
		// package may already exist from a prior seed call
		if !strings.Contains(err.Error(), "UNIQUE") {
			t.Fatalf("insert package: %v", err)
		}
	}
	var pkgID int64
	if err := conn.QueryRow(`SELECT id FROM packages WHERE path = 'pkg'`).Scan(&pkgID); err != nil {
		t.Fatalf("select package: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at) VALUES (?, 'pkg/file.go', 'go', 10, 'abc', ?, ?)`, pkgID, now, now)
	if err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") {
			t.Fatalf("insert file: %v", err)
		}
	}
	var fileID int64
	if err := conn.QueryRow(`SELECT id FROM files WHERE path = 'pkg/file.go'`).Scan(&fileID); err != nil {
		t.Fatalf("select file: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver) VALUES (?, 'func', ?, 'func()','{}', 1, 5, 1, '')`, fileID, name); err != nil {
		t.Fatalf("insert symbol: %v", err)
	}
}

func TestProposePatternSymbolExistsPromoted(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	defer cleanup()

	seedSymbol(t, conn, "MyHandler")

	result, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "Handler naming convention",
		Description:     "Handlers are named with Handler suffix",
		Example:         "func MyHandler() {}",
		Confidence:      "high",
		EvidenceSummary: "symbol MyHandler exists in index",
		CheckType:       "symbol_exists",
		CheckSpec:       `{"name":"MyHandler"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("ProposeAndVerifyPattern: %v", err)
	}
	if !result.Promoted {
		t.Fatalf("expected promoted, got %+v", result)
	}
	if result.PatternID == 0 {
		t.Fatalf("expected non-zero pattern ID")
	}
	if !result.VerificationPassed {
		t.Fatalf("expected verification passed")
	}
	if !strings.Contains(result.VerificationDetails, "MyHandler") {
		t.Fatalf("expected details to mention MyHandler, got %q", result.VerificationDetails)
	}
}

func TestProposePatternSymbolExistsNotPromoted(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	defer cleanup()

	// Do NOT seed any symbols — the symbol check should fail
	result, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "NonExistent symbol pattern",
		Description:     "Checks for a symbol that does not exist",
		Confidence:      "low",
		EvidenceSummary: "symbol does not exist",
		CheckType:       "symbol_exists",
		CheckSpec:       `{"name":"DoesNotExist"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("ProposeAndVerifyPattern: %v", err)
	}
	if result.Promoted {
		t.Fatalf("expected not promoted, got %+v", result)
	}
	if result.VerificationPassed {
		t.Fatalf("expected verification not passed")
	}
	if !strings.Contains(result.VerificationDetails, "DoesNotExist") {
		t.Fatalf("expected details to mention DoesNotExist, got %q", result.VerificationDetails)
	}
}

func TestProposePatternFileExistsNotPromoted(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	defer cleanup()

	result, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "Missing file pattern",
		Description:     "Checks for a file that does not exist",
		Confidence:      "medium",
		EvidenceSummary: "file should exist",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"nonexistent.go"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("ProposeAndVerifyPattern: %v", err)
	}
	if result.Promoted {
		t.Fatalf("expected not promoted, got %+v", result)
	}
	if result.VerificationPassed {
		t.Fatalf("expected verification not passed")
	}
}

func TestListPatterns_ReturnsActivePatterns(t *testing.T) {
	conn, _, cleanup := patternTestDB(t)
	defer cleanup()
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

func TestListPatterns_ExcludesArchived(t *testing.T) {
	conn, _, cleanup := patternTestDB(t)
	defer cleanup()
	svc := NewService(conn)

	now := time.Now().UTC().Format(time.RFC3339)
	conn.ExecContext(context.Background(),
		`INSERT INTO patterns (title, description, confidence, status, created_at, updated_at) VALUES (?, ?, ?, 'archived', ?, ?)`,
		"Archived pattern", "desc", "high", now, now)

	items, err := svc.ListPatterns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 patterns, got %d", len(items))
	}
}

func TestProposePatternDBError(t *testing.T) {
	conn, root, cleanup := patternTestDB(t)
	cleanup() // Close immediately
	_, err := NewService(conn).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "t",
		EvidenceSummary: "e",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"go.mod"}`,
		ModuleRoot:      root,
	})
	if err == nil || !strings.Contains(err.Error(), "begin pattern tx") {
		t.Fatalf("expected begin tx error, got %v", err)
	}
}
