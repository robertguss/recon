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

	mock.ExpectQuery("SELECT d.id").WithArgs("X", 10).WillReturnRows(
		sqlmock.NewRows([]string{"id", "title", "reasoning", "confidence", "updated_at", "summary", "drift_status"}).
			AddRow(1, "t", "r", "high", "u", "s", "ok").
			RowError(0, errors.New("iter fail")),
	)
	mock.ExpectQuery("SELECT d.id").WithArgs("%X%", "%X%", "%X%", 10).WillReturnError(errors.New("fallback fail"))
	_, err = NewService(db).Recall(context.Background(), "X", RecallOptions{Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "fallback recall query") {
		t.Fatalf("expected fallback recall query error due rows.Err path, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
