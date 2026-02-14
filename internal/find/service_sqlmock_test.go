package find

import (
	"context"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestFindExactScanAndRowsErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT s.id").WithArgs("X").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kind", "name", "signature", "body", "line_start", "line_end", "receiver", "path", "package"}).
			AddRow("bad-id", "func", "X", "", "", 1, 1, "", "f.go", "."),
	)
	_, err = NewService(db).FindExact(context.Background(), "X")
	if err == nil || !strings.Contains(err.Error(), "scan symbol row") {
		t.Fatalf("expected scan symbol row error, got %v", err)
	}

	mock.ExpectQuery("SELECT s.id").WithArgs("Y").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kind", "name", "signature", "body", "line_start", "line_end", "receiver", "path", "package"}).
			AddRow(1, "func", "Y", "", "", 1, 1, "", "f.go", ".").
			RowError(0, errors.New("row-iter")),
	)
	_, err = NewService(db).FindExact(context.Background(), "Y")
	if err == nil || !strings.Contains(err.Error(), "iterate symbol rows") {
		t.Fatalf("expected iterate symbol rows error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFindExactSuggestionsAndDepsErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT s.id").WithArgs("Z").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kind", "name", "signature", "body", "line_start", "line_end", "receiver", "path", "package"}),
	)
	mock.ExpectQuery("SELECT DISTINCT name").WithArgs("Z%").WillReturnError(errors.New("suggestion query fail"))
	_, err = NewService(db).FindExact(context.Background(), "Z")
	if err == nil || !strings.Contains(err.Error(), "query suggestions") {
		t.Fatalf("expected suggestions query error, got %v", err)
	}

	mock.ExpectQuery("SELECT s.id").WithArgs("A").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kind", "name", "signature", "body", "line_start", "line_end", "receiver", "path", "package"}).
			AddRow(1, "func", "A", "", "", 1, 1, "", "f.go", "."),
	)
	mock.ExpectQuery("SELECT DISTINCT s2.id").WithArgs(int64(1)).WillReturnError(errors.New("dep query fail"))
	_, err = NewService(db).FindExact(context.Background(), "A")
	if err == nil || !strings.Contains(err.Error(), "query dependencies") {
		t.Fatalf("expected dependencies query error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSuggestionsAndDirectDepsScanAndRowsErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	svc := NewService(db)

	mock.ExpectQuery("SELECT DISTINCT name").WithArgs("S%").WillReturnRows(
		sqlmock.NewRows([]string{"name"}).AddRow(nil),
	)
	if _, err := svc.suggestions(context.Background(), "S"); err == nil || !strings.Contains(err.Error(), "scan suggestion") {
		t.Fatalf("expected scan suggestion error, got %v", err)
	}

	mock.ExpectQuery("SELECT DISTINCT name").WithArgs("T%").WillReturnRows(
		sqlmock.NewRows([]string{"name"}).AddRow("ok").RowError(0, errors.New("iter fail")),
	)
	if _, err := svc.suggestions(context.Background(), "T"); err == nil || !strings.Contains(err.Error(), "iterate suggestions") {
		t.Fatalf("expected iterate suggestions error, got %v", err)
	}

	mock.ExpectQuery("SELECT DISTINCT s2.id").WithArgs(int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "kind", "name", "signature", "body", "line_start", "line_end", "receiver", "path", "package"}).
			AddRow("bad-id", "func", "d", "", "", 1, 1, "", "f.go", "."),
	)
	if _, err := svc.directDeps(context.Background(), 7); err == nil || !strings.Contains(err.Error(), "scan dependency row") {
		t.Fatalf("expected scan dependency row error, got %v", err)
	}

	mock.ExpectQuery("SELECT DISTINCT s2.id").WithArgs(int64(8)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "kind", "name", "signature", "body", "line_start", "line_end", "receiver", "path", "package"}).
			AddRow(1, "func", "d", "", "", 1, 1, "", "f.go", ".").
			RowError(0, errors.New("dep iter fail")),
	)
	if _, err := svc.directDeps(context.Background(), 8); err == nil || !strings.Contains(err.Error(), "iterate dependency rows") {
		t.Fatalf("expected iterate dependency rows error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
