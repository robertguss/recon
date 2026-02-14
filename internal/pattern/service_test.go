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
