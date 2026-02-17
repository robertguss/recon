package recall

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/robertguss/recon/internal/db"
)

func TestRecall_SQLMock_ErrorPaths(t *testing.T) {
	t.Run("query error surfaces from Recall", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer mockDB.Close()

		// FTS query fails
		mock.ExpectQuery("search_index.entity_type").WithArgs("Cobra", 10).
			WillReturnError(errors.New("fts fail"))
		// LIKE fallback also fails
		mock.ExpectQuery("SELECT 'decision'").WithArgs("%Cobra%", "%Cobra%", "%Cobra%", "%Cobra%", "%Cobra%", "%Cobra%", 10).
			WillReturnError(errors.New("like fail"))

		svc := NewService(mockDB)
		_, err = svc.Recall(context.Background(), "Cobra", RecallOptions{})
		if err == nil {
			t.Fatal("expected error from Recall when both FTS and LIKE fail")
		}
		if !strings.Contains(err.Error(), "fallback recall query") {
			t.Fatalf("expected fallback recall query error, got: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("expectations: %v", err)
		}
	})

	t.Run("scan error from malformed rows", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer mockDB.Close()

		// Return rows with wrong column types to trigger scan error
		mock.ExpectQuery("search_index.entity_type").WithArgs("Cobra", 10).WillReturnRows(
			sqlmock.NewRows([]string{"entity_type", "entity_id", "title", "reasoning", "confidence", "updated_at", "summary", "drift_status"}).
				AddRow("decision", "not_an_int", "t", "r", "high", "u", "s", "ok"),
		)
		// LIKE fallback also fails
		mock.ExpectQuery("SELECT 'decision'").WithArgs("%Cobra%", "%Cobra%", "%Cobra%", "%Cobra%", "%Cobra%", "%Cobra%", 10).
			WillReturnError(errors.New("like fail"))

		svc := NewService(mockDB)
		_, err = svc.Recall(context.Background(), "Cobra", RecallOptions{})
		if err == nil {
			t.Fatal("expected error from Recall when scan fails")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("expectations: %v", err)
		}
	})
}

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

func TestRecallFindsDecisionByRelatedTerms(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	// recallTestDB seeds "Use Cobra" with content "Because subcommands\ncobra in go.mod"
	// Search for "CLI subcommands" — "subcommands" should match via porter tokenizer
	svc := NewService(conn)
	result, err := svc.Recall(context.Background(), "subcommands", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected recall to find 'Use Cobra' via 'subcommands' in content")
	}
	if result.Items[0].Title != "Use Cobra" {
		t.Fatalf("expected 'Use Cobra', got %q", result.Items[0].Title)
	}

	// Add a second decision with reasoning mentioning "CLI framework"
	_, _ = conn.Exec(`INSERT INTO decisions(id,title,reasoning,confidence,status,created_at,updated_at) VALUES (2,'Use spf13/cobra for CLI','Cobra CLI framework is standard for Go CLIs','high','active','x','2026-01-02T00:00:00Z');`)
	_, _ = conn.Exec(`INSERT INTO evidence(entity_type,entity_id,summary,drift_status) VALUES ('decision',2,'cobra in go.mod','ok');`)
	_, _ = conn.Exec(`INSERT INTO search_index(title,content,entity_type,entity_id) VALUES ('Use spf13/cobra for CLI','Cobra CLI framework is standard for Go CLIs\ncobra in go.mod','decision',2);`)

	// "CLI framework" should match across title+content
	result, err = svc.Recall(context.Background(), "CLI framework", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall CLI framework: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected recall to find decision via 'CLI framework'")
	}

	// Also search for patterns if any exist
	_, _ = conn.Exec(`INSERT INTO patterns(id,title,description,confidence,status,created_at,updated_at) VALUES (1,'Error wrapping','Use fmt.Errorf with %%w','high','active','x','2026-01-01T00:00:00Z');`)
	_, _ = conn.Exec(`INSERT INTO search_index(title,content,entity_type,entity_id) VALUES ('Error wrapping','Use fmt.Errorf with %%w\nerror handling pattern','pattern',1);`)

	result, err = svc.Recall(context.Background(), "error wrapping", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall pattern: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected recall to find pattern via 'error wrapping'")
	}
}

func TestRecall_ReasoningAndEvidenceSummarySeparate(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	svc := NewService(conn)
	res, err := svc.Recall(context.Background(), "Cobra", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected recall results")
	}
	item := res.Items[0]
	// Reasoning should be ONLY the reasoning text, not contain evidence summary
	if item.Reasoning != "Because subcommands" {
		t.Fatalf("expected reasoning='Because subcommands', got %q", item.Reasoning)
	}
	if item.EvidenceSummary != "cobra in go.mod" {
		t.Fatalf("expected evidence_summary='cobra in go.mod', got %q", item.EvidenceSummary)
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

func TestRecallSkipsArchivedDecisionInFTS(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	if _, err := conn.Exec(`UPDATE decisions SET status = 'archived' WHERE id = 1;`); err != nil {
		t.Fatalf("archive seeded decision: %v", err)
	}

	res, err := NewService(conn).Recall(context.Background(), "Cobra", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall archived decision: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected archived decision to be excluded, got %+v", res.Items)
	}
}

func TestRecall_IncludesConnectedEdges(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	// recallTestDB seeds decision 1 "Use Cobra"
	// Add an edge from that decision to a package
	_, _ = conn.Exec(`INSERT INTO edges(from_type,from_id,to_type,to_ref,relation,source,confidence,created_at) VALUES ('decision',1,'package','internal/cli','affects','manual','high','2026-01-01T00:00:00Z')`)

	svc := NewService(conn)
	res, err := svc.Recall(context.Background(), "Cobra", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected recall results")
	}
	item := res.Items[0]
	if len(item.ConnectedEdges) == 0 {
		t.Fatal("expected connected edges on recall result")
	}
	if item.ConnectedEdges[0].ToType != "package" || item.ConnectedEdges[0].ToRef != "internal/cli" {
		t.Fatalf("unexpected connected edge: %+v", item.ConnectedEdges[0])
	}
}

func TestRecallWithKindFilter(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	// Add a pattern so we have both entity types
	_, _ = conn.Exec(`INSERT INTO patterns(id,title,description,confidence,status,created_at,updated_at) VALUES (1,'Cobra pattern','Use cobra for CLI','high','active','x','2026-01-01T00:00:00Z');`)
	_, _ = conn.Exec(`INSERT INTO evidence(entity_type,entity_id,summary,drift_status) VALUES ('pattern',1,'cobra used','ok');`)
	_, _ = conn.Exec(`INSERT INTO search_index(title,content,entity_type,entity_id) VALUES ('Cobra pattern','Use cobra for CLI','pattern',1);`)

	svc := NewService(conn)

	// No filter — should return both
	res, err := svc.Recall(context.Background(), "Cobra", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall error: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items (decision + pattern), got %d", len(res.Items))
	}

	// Filter to decisions only
	res, err = svc.Recall(context.Background(), "Cobra", RecallOptions{Kind: "decision"})
	if err != nil {
		t.Fatalf("Recall with kind=decision error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(res.Items))
	}
	if res.Items[0].EntityType != "decision" {
		t.Fatalf("expected entity_type=decision, got %s", res.Items[0].EntityType)
	}

	// Filter to patterns only
	res, err = svc.Recall(context.Background(), "Cobra", RecallOptions{Kind: "pattern"})
	if err != nil {
		t.Fatalf("Recall with kind=pattern error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(res.Items))
	}
	if res.Items[0].EntityType != "pattern" {
		t.Fatalf("expected entity_type=pattern, got %s", res.Items[0].EntityType)
	}
}

func TestRecallLegacyQueriesWhenPatternsTableMissing(t *testing.T) {
	conn, cleanup := recallTestDB(t)
	defer cleanup()

	if _, err := conn.Exec(`DROP TABLE patterns;`); err != nil {
		t.Fatalf("drop patterns table: %v", err)
	}

	svc := NewService(conn)
	// FTS path should fall back to a decisions-only query on legacy DBs.
	res, err := svc.Recall(context.Background(), "Cobra", RecallOptions{})
	if err != nil {
		t.Fatalf("Recall on legacy DB: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "Use Cobra" {
		t.Fatalf("unexpected legacy FTS result: %+v", res.Items)
	}

	// LIKE path should also stay functional without patterns.
	items, err := svc.recallLike(context.Background(), "Cobra", 10)
	if err != nil {
		t.Fatalf("recallLike on legacy DB: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Use Cobra" {
		t.Fatalf("unexpected legacy LIKE result: %+v", items)
	}
}
