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
