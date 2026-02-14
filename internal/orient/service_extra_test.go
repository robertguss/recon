package orient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func TestLoadSummaryStepwiseErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn, err := db.Open(filepath.Join(root, "x.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	svc := NewService(conn)
	ctx := context.Background()
	payload := &Payload{}

	_, _ = conn.Exec(`CREATE TABLE files (id INTEGER);`)
	if err := svc.loadSummary(ctx, payload); err == nil || !strings.Contains(err.Error(), "count symbols") {
		t.Fatalf("expected symbols error, got %v", err)
	}
	_, _ = conn.Exec(`DROP TABLE files;`)
	_, _ = conn.Exec(`CREATE TABLE files (id INTEGER);`)
	_, _ = conn.Exec(`CREATE TABLE symbols (id INTEGER);`)
	if err := svc.loadSummary(ctx, payload); err == nil || !strings.Contains(err.Error(), "count packages") {
		t.Fatalf("expected packages error, got %v", err)
	}
	_, _ = conn.Exec(`CREATE TABLE packages (id INTEGER);`)
	if err := svc.loadSummary(ctx, payload); err == nil || !strings.Contains(err.Error(), "count decisions") {
		t.Fatalf("expected decisions error, got %v", err)
	}
}

func TestLoadModulesAndDecisionsErrors(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	svc := NewService(conn)
	ctx := context.Background()
	payload := &Payload{}

	if err := svc.loadModules(ctx, 1, payload); err == nil || !strings.Contains(err.Error(), "query modules") {
		t.Fatalf("expected query modules error, got %v", err)
	}
	if err := svc.loadDecisions(ctx, 1, payload); err == nil || !strings.Contains(err.Error(), "query decisions") {
		t.Fatalf("expected query decisions error, got %v", err)
	}
}

func TestBuildErrorBranchesForModulesDecisionsAndSyncState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	conn, err := db.Open(filepath.Join(root, "x.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	// Build hits loadModules error (missing columns in packages table).
	_, _ = conn.Exec(`CREATE TABLE files (id INTEGER);`)
	_, _ = conn.Exec(`CREATE TABLE symbols (id INTEGER);`)
	_, _ = conn.Exec(`CREATE TABLE decisions (id INTEGER, status TEXT);`)
	_, _ = conn.Exec(`CREATE TABLE packages (id INTEGER);`)
	if _, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root}); err == nil || !strings.Contains(err.Error(), "query modules") {
		t.Fatalf("expected build loadModules error, got %v", err)
	}

	// Fix modules query, break decisions query.
	_, _ = conn.Exec(`DROP TABLE packages;`)
	_, _ = conn.Exec(`CREATE TABLE packages (id INTEGER PRIMARY KEY, path TEXT, name TEXT, file_count INTEGER, line_count INTEGER);`)
	_, _ = conn.Exec(`DROP TABLE decisions;`)
	_, _ = conn.Exec(`CREATE TABLE decisions (id INTEGER, status TEXT);`)
	if _, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root}); err == nil || !strings.Contains(err.Error(), "query decisions") {
		t.Fatalf("expected build loadDecisions error, got %v", err)
	}

	// Fix decisions query, break LoadSyncState parse.
	_, _ = conn.Exec(`DROP TABLE decisions;`)
	_, _ = conn.Exec(`CREATE TABLE decisions (id INTEGER, title TEXT, confidence TEXT, updated_at TEXT, status TEXT);`)
	_, _ = conn.Exec(`CREATE TABLE evidence (entity_type TEXT, entity_id INTEGER, drift_status TEXT);`)
	// Recreate files with proper columns so loadArchitecture succeeds
	_, _ = conn.Exec(`DROP TABLE files;`)
	_, _ = conn.Exec(`CREATE TABLE files (id INTEGER, path TEXT, package_id INTEGER);`)
	_, _ = conn.Exec(`CREATE TABLE imports (id INTEGER, from_file_id INTEGER, to_path TEXT, to_package_id INTEGER, alias TEXT, import_type TEXT);`)
	_, _ = conn.Exec(`CREATE TABLE patterns (id INTEGER, title TEXT, description TEXT, confidence TEXT, status TEXT, updated_at TEXT, created_at TEXT);`)
	_, _ = conn.Exec(`CREATE TABLE sync_state (id INTEGER PRIMARY KEY, last_sync_at TEXT, last_sync_commit TEXT, last_sync_dirty INTEGER, indexed_file_count INTEGER, index_fingerprint TEXT);`)
	_, _ = conn.Exec(`INSERT INTO sync_state(id,last_sync_at,last_sync_commit,last_sync_dirty,indexed_file_count,index_fingerprint) VALUES (1,'bad-time','c',0,0,'f');`)
	if _, err := NewService(conn).Build(context.Background(), BuildOptions{ModuleRoot: root}); err == nil || !strings.Contains(err.Error(), "parse sync timestamp") {
		t.Fatalf("expected sync state parse error, got %v", err)
	}
}
