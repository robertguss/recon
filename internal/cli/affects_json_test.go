package cli

import (
	"context"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func TestDecide_AffectsWorksInJSONMode(t *testing.T) {
	_, app := m4Setup(t)

	// Run decide with --json --affects
	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"Test decision JSON affects", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--affects", "pkg1",
		"--json",
	})
	if err != nil {
		t.Fatalf("expected success, got %v; out=%s", err, out)
	}

	// Open DB and verify edges were created
	conn, err := db.Open(db.DBPath(app.ModuleRoot))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	var count int
	err = conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM edges WHERE from_type='decision' AND relation='affects' AND source='manual'`).Scan(&count)
	if err != nil {
		t.Fatalf("query edges: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one edge from --affects flag in JSON mode, got 0")
	}
}

func TestPattern_AffectsWorksInJSONMode(t *testing.T) {
	_, app := m4Setup(t)

	// Run pattern with --json --affects
	out, _, err := runCommandWithCapture(t, newPatternCommand(app), []string{
		"Test pattern JSON affects", "--reasoning", "d", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod",
		"--affects", "pkg1",
		"--json",
	})
	if err != nil {
		t.Fatalf("expected success, got %v; out=%s", err, out)
	}

	// Open DB and verify edges were created
	conn, err := db.Open(db.DBPath(app.ModuleRoot))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	var count int
	err = conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM edges WHERE from_type='pattern' AND relation='affects' AND source='manual'`).Scan(&count)
	if err != nil {
		t.Fatalf("query edges: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one edge from --affects flag in JSON mode, got 0")
	}
}
