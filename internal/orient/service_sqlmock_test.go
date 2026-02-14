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

func TestLoadPatternsScanAndIterateErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	svc := NewService(db)
	payload := &Payload{}

	// iterate pattern rows error
	mock.ExpectQuery("SELECT p.id, p.title, p.confidence, p.updated_at").WithArgs(5).WillReturnRows(
		sqlmock.NewRows([]string{"id", "title", "confidence", "updated_at", "drift"}).
			AddRow(1, "t", "h", "2024-01-01", "ok").
			RowError(0, errors.New("pattern iter fail")),
	)
	if err := svc.loadPatterns(context.Background(), 5, payload); err == nil || !strings.Contains(err.Error(), "iterate pattern rows") {
		t.Fatalf("expected iterate pattern rows error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestLoadArchitectureScanAndIterateErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	svc := NewService(db)
	payload := &Payload{}

	// scan entry point error
	mock.ExpectQuery("SELECT f.path").WillReturnRows(
		sqlmock.NewRows([]string{"path"}).AddRow(nil),
	)
	if err := svc.loadArchitecture(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "scan entry point") {
		t.Fatalf("expected scan entry point error, got %v", err)
	}

	// iterate entry points error
	mock.ExpectQuery("SELECT f.path").WillReturnRows(
		sqlmock.NewRows([]string{"path"}).
			AddRow("main.go").
			RowError(0, errors.New("entry iter fail")),
	)
	if err := svc.loadArchitecture(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "iterate entry points") {
		t.Fatalf("expected iterate entry points error, got %v", err)
	}

	// scan dep flow error
	mock.ExpectQuery("SELECT f.path").WillReturnRows(
		sqlmock.NewRows([]string{"path"}).AddRow("main.go"),
	)
	mock.ExpectQuery("SELECT DISTINCT p1.path").WillReturnRows(
		sqlmock.NewRows([]string{"from_pkg", "to_pkg"}).AddRow(nil, nil),
	)
	if err := svc.loadArchitecture(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "scan dep flow") {
		t.Fatalf("expected scan dep flow error, got %v", err)
	}

	// iterate dep flow error
	mock.ExpectQuery("SELECT f.path").WillReturnRows(
		sqlmock.NewRows([]string{"path"}).AddRow("main.go"),
	)
	mock.ExpectQuery("SELECT DISTINCT p1.path").WillReturnRows(
		sqlmock.NewRows([]string{"from_pkg", "to_pkg"}).
			AddRow("a", "b").
			RowError(0, errors.New("dep iter fail")),
	)
	if err := svc.loadArchitecture(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "iterate dep flow") {
		t.Fatalf("expected iterate dep flow error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
