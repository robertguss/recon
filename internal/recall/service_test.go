package recall

import (
	"context"
	"database/sql"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func recallTestDB(t *testing.T) (*sql.DB, func()) {
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
	_, _ = conn.Exec(`INSERT INTO decisions(id,title,reasoning,confidence,status,created_at,updated_at) VALUES (1,'Use Cobra','Because subcommands','high','active','x','2026-01-01T00:00:00Z');`)
	_, _ = conn.Exec(`INSERT INTO evidence(entity_type,entity_id,summary,drift_status) VALUES ('decision',1,'cobra in go.mod','ok');`)
	_, _ = conn.Exec(`INSERT INTO search_index(title,content,entity_type,entity_id) VALUES ('Use Cobra','Because subcommands\ncobra in go.mod','decision',1);`)
	return conn, func() { _ = conn.Close() }
}

func TestRecallFTSAndFallback(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	svc := NewService(conn)
	res, err := svc.Recall(context.Background(), "Cobra", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall fts error: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "Use Cobra" {
		t.Fatalf("unexpected recall result: %+v", res)
	}

	res, err = svc.Recall(context.Background(), "\"", RecallOptions{Limit: 1})
	if err != nil {
		t.Fatalf("Recall fallback error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected empty fallback for unmatched query, got %+v", res.Items)
	}
}

func TestRecallErrorsAndScanItems(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()
	svc := NewService(conn)

	if err := conn.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if _, err := svc.Recall(context.Background(), "\"", RecallOptions{}); err == nil {
		t.Fatal("expected recall error when DB is closed")
	}

	root := t.TempDir()
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn2, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open second DB: %v", err)
	}
	defer conn2.Close()
	rows, err := conn2.Query(`SELECT 1;`)
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if _, err := scanItems(rows); err == nil {
		t.Fatal("expected scan error from wrong columns")
	}
}
