package recall

import (
	"context"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestScanItemsRowsErrBranch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("search_index.entity_type").WithArgs("X", 10).WillReturnRows(
		sqlmock.NewRows([]string{"entity_type", "entity_id", "title", "content", "confidence", "updated_at", "summary", "drift_status"}).
			AddRow("decision", 1, "t", "r", "high", "u", "s", "ok").
			RowError(0, errors.New("iter fail")),
	)
	mock.ExpectQuery("SELECT 'decision'").WithArgs("%X%", "%X%", "%X%", "%X%", "%X%", "%X%", 10).WillReturnError(errors.New("fallback fail"))
	_, err = NewService(db).Recall(context.Background(), "X", RecallOptions{Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "fallback recall query") {
		t.Fatalf("expected fallback recall query error due rows.Err path, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRecallFTSLegacyQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("search_index.entity_type").WithArgs("legacy", 5).WillReturnError(errors.New("legacy fail"))

	_, err = NewService(db).recallFTSLegacy(context.Background(), "legacy", 5)
	if err == nil || !strings.Contains(err.Error(), "fts recall query") {
		t.Fatalf("expected fts recall query error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRecallLikeLegacyQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT 'decision'").WithArgs("%legacy%", "%legacy%", "%legacy%", 5).WillReturnError(errors.New("legacy fail"))

	_, err = NewService(db).recallLikeLegacy(context.Background(), "%legacy%", 5)
	if err == nil || !strings.Contains(err.Error(), "fallback recall query") {
		t.Fatalf("expected fallback recall query error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestIsMissingTableError(t *testing.T) {
	if isMissingTableError(nil, "patterns") {
		t.Fatal("expected false for nil error")
	}

	if !isMissingTableError(errors.New("no such table: patterns"), "patterns") {
		t.Fatal("expected true for missing patterns table")
	}

	if !isMissingTableError(errors.New("NO SUCH TABLE: PATTERNS"), "patterns") {
		t.Fatal("expected case-insensitive table detection")
	}
}
