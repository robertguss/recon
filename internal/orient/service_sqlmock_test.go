package orient

import (
	"context"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestLoadModulesAndDecisionsScanAndRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	svc := NewService(db)
	payload := &Payload{}

	mock.ExpectQuery("SELECT path, name, file_count, line_count").WithArgs(3).WillReturnRows(
		sqlmock.NewRows([]string{"path", "name", "file_count", "line_count"}).
			AddRow("p", "n", "bad-int", 1),
	)
	if err := svc.loadModules(context.Background(), 3, payload); err == nil || !strings.Contains(err.Error(), "scan module row") {
		t.Fatalf("expected module scan error, got %v", err)
	}

	mock.ExpectQuery("SELECT path, name, file_count, line_count").WithArgs(3).WillReturnRows(
		sqlmock.NewRows([]string{"path", "name", "file_count", "line_count"}).
			AddRow("p", "n", 1, 1).
			RowError(0, errors.New("module iter fail")),
	)
	if err := svc.loadModules(context.Background(), 3, payload); err == nil || !strings.Contains(err.Error(), "iterate module rows") {
		t.Fatalf("expected module iter error, got %v", err)
	}

	mock.ExpectQuery("SELECT d.id, d.title, d.confidence, d.updated_at").WithArgs(2).WillReturnRows(
		sqlmock.NewRows([]string{"id", "title", "confidence", "updated_at", "drift_status"}).
			AddRow("bad-id", "t", "c", "u", "ok"),
	)
	if err := svc.loadDecisions(context.Background(), 2, payload); err == nil || !strings.Contains(err.Error(), "scan decision row") {
		t.Fatalf("expected decision scan error, got %v", err)
	}

	mock.ExpectQuery("SELECT d.id, d.title, d.confidence, d.updated_at").WithArgs(2).WillReturnRows(
		sqlmock.NewRows([]string{"id", "title", "confidence", "updated_at", "drift_status"}).
			AddRow(1, "t", "c", "u", "ok").
			RowError(0, errors.New("decision iter fail")),
	)
	if err := svc.loadDecisions(context.Background(), 2, payload); err == nil || !strings.Contains(err.Error(), "iterate decision rows") {
		t.Fatalf("expected decision iter error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
