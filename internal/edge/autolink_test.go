package edge

import (
	"context"
	"testing"
)

func TestAutoLink_FindsPackagePaths(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/orient', 'orient', 'example.com/test/internal/orient', ?, ?)`, now, now)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1, "ExitError convention", "Used in internal/cli for all commands, also affects internal/orient")

	pkgRefs := map[string]bool{}
	for _, e := range edges {
		if e.ToType == "package" {
			pkgRefs[e.ToRef] = true
		}
	}
	if !pkgRefs["internal/cli"] {
		t.Fatal("expected internal/cli in auto-linked edges")
	}
	if !pkgRefs["internal/orient"] {
		t.Fatal("expected internal/orient in auto-linked edges")
	}
}

func TestAutoLink_FindsDistinctiveSymbols(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	conn.ExecContext(context.Background(),
		`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at) VALUES (?, 'internal/cli/exit_error.go', 'go', 20, 'abc', ?, ?)`, pkgID, now, now)
	var fileID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM files WHERE path = 'internal/cli/exit_error.go'`).Scan(&fileID)
	conn.ExecContext(context.Background(),
		`INSERT INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver) VALUES (?, 'type', 'ExitError', '', '{}', 1, 5, 1, '')`, fileID)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1, "ExitError is the standard error type", "All commands return ExitError")

	symRefs := map[string]bool{}
	for _, e := range edges {
		if e.ToType == "symbol" {
			symRefs[e.ToRef] = true
		}
	}
	if !symRefs["internal/cli.ExitError"] {
		t.Fatalf("expected internal/cli.ExitError in auto-linked edges, got %v", symRefs)
	}
}

func TestAutoLink_SkipsRootPackagePath(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	// Insert root package "."
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at)
		 VALUES ('.', 'main', 'example.com/test', ?, ?)`, now, now)
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at)
		 VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1,
		"Some decision.", "This is about internal/cli package.")

	for _, e := range edges {
		if e.ToRef == "." {
			t.Fatal("should not auto-link root package path '.'")
		}
	}
	// Should still find internal/cli
	found := false
	for _, e := range edges {
		if e.ToRef == "internal/cli" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected internal/cli in auto-linked edges")
	}
}

func TestAutoLink_FindsFilePaths(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at)
		 VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	conn.QueryRowContext(context.Background(),
		`SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	conn.ExecContext(context.Background(),
		`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at)
		 VALUES (?, 'internal/cli/exit_error.go', 'go', 20, 'abc', ?, ?)`, pkgID, now, now)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1,
		"ExitError convention",
		"Defined in internal/cli/exit_error.go, used everywhere")

	fileRefs := map[string]bool{}
	for _, e := range edges {
		if e.ToType == "file" {
			fileRefs[e.ToRef] = true
		}
	}
	if !fileRefs["internal/cli/exit_error.go"] {
		t.Fatal("expected internal/cli/exit_error.go in auto-linked edges")
	}
}

func TestAutoLink_SkipsShortSymbolNames(t *testing.T) {
	conn, cleanup := edgeTestDB(t)
	defer cleanup()
	now := "2024-01-01T00:00:00Z"
	conn.ExecContext(context.Background(),
		`INSERT INTO packages (path, name, import_path, created_at, updated_at) VALUES ('internal/cli', 'cli', 'example.com/test/internal/cli', ?, ?)`, now, now)
	var pkgID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM packages WHERE path = 'internal/cli'`).Scan(&pkgID)
	conn.ExecContext(context.Background(),
		`INSERT INTO files (package_id, path, language, lines, hash, created_at, updated_at) VALUES (?, 'internal/cli/run.go', 'go', 10, 'abc', ?, ?)`, pkgID, now, now)
	var fileID int64
	conn.QueryRowContext(context.Background(), `SELECT id FROM files WHERE path = 'internal/cli/run.go'`).Scan(&fileID)
	conn.ExecContext(context.Background(),
		`INSERT INTO symbols (file_id, kind, name, signature, body, line_start, line_end, exported, receiver) VALUES (?, 'func', 'Run', '', '{}', 1, 5, 1, '')`, fileID)

	linker := NewAutoLinker(conn)
	edges := linker.Detect(context.Background(), "decision", 1, "Run function", "We use Run everywhere")

	for _, e := range edges {
		if e.ToType == "symbol" {
			t.Fatalf("should not auto-link short symbol name 'Run', got %+v", e)
		}
	}
}

func TestContainsWord_EdgeCases(t *testing.T) {
	tests := []struct {
		text string
		word string
		want bool
	}{
		{"ExitError is used", "ExitError", true},  // word at start
		{"uses ExitError", "ExitError", true},     // word at end
		{"the ExitError type", "ExitError", true}, // word in middle
		{"NotExitError", "ExitError", false},      // prefix of longer word
		{"ExitErrorHandler", "ExitError", false},  // suffix into longer word
		{"foo.ExitError.bar", "ExitError", true},  // bounded by dots
		{"(ExitError)", "ExitError", true},        // bounded by parens
		{"", "ExitError", false},                  // empty text
		{"ExitError", "ExitError", true},          // exact match
	}
	for _, tt := range tests {
		t.Run(tt.text+"_"+tt.word, func(t *testing.T) {
			got := containsWord(tt.text, tt.word)
			if got != tt.want {
				t.Fatalf("containsWord(%q, %q) = %v, want %v", tt.text, tt.word, got, tt.want)
			}
		})
	}
}
