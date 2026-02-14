package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestProposeAndVerifyDecisionMarshalAndRunCheckErrorBranches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	svc := NewService(db)

	origMarshal := marshalJSON
	defer func() { marshalJSON = origMarshal }()

	marshalJSON = func(any) ([]byte, error) { return nil, errors.New("marshal fail") }
	_, err = svc.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{Title: "t", Reasoning: "r", EvidenceSummary: "e", CheckType: "file_exists", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root})
	if err == nil || !strings.Contains(err.Error(), "marshal proposal data") {
		t.Fatalf("expected proposal marshal error, got %v", err)
	}

	marshalCalls := 0
	marshalJSON = func(v any) ([]byte, error) {
		marshalCalls++
		if marshalCalls == 2 {
			return nil, errors.New("baseline marshal fail")
		}
		return origMarshal(v)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{Title: "t2", Reasoning: "r", EvidenceSummary: "e", CheckType: "unknown", CheckSpec: `{"x":1}`, ModuleRoot: root})
	if err == nil || !strings.Contains(err.Error(), "marshal baseline") {
		t.Fatalf("expected baseline marshal error, got %v", err)
	}

	marshalCalls = 0
	marshalJSON = func(v any) ([]byte, error) {
		marshalCalls++
		if marshalCalls == 3 {
			return nil, errors.New("result marshal fail")
		}
		return origMarshal(v)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{Title: "t3", Reasoning: "r", EvidenceSummary: "e", CheckType: "unknown", CheckSpec: `{"x":1}`, ModuleRoot: root})
	if err == nil || !strings.Contains(err.Error(), "marshal check result") {
		t.Fatalf("expected result marshal error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestProposeAndVerifyDecisionSQLMockErrorBranches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	svc := NewService(db)

	in := ProposeDecisionInput{Title: "t", Reasoning: "r", EvidenceSummary: "e", CheckType: "file_exists", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewErrorResult(errors.New("proposal id fail")))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "read proposal id") {
		t.Fatalf("expected proposal id error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO decisions").WillReturnError(errors.New("insert decision fail"))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "insert decision") {
		t.Fatalf("expected insert decision error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO decisions").WillReturnResult(sqlmock.NewErrorResult(errors.New("decision id fail")))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "read decision id") {
		t.Fatalf("expected decision id error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO decisions").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnError(errors.New("evidence fail"))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "insert decision evidence") {
		t.Fatalf("expected decision evidence error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO decisions").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE proposals").WillReturnError(errors.New("update promoted fail"))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "update proposal status to promoted") {
		t.Fatalf("expected promoted update error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO decisions").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO search_index").WillReturnError(errors.New("search index fail"))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "insert search index") {
		t.Fatalf("expected search index error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO decisions").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO search_index").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit promoted fail"))
	_, err = svc.ProposeAndVerifyDecision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "commit decision tx") {
		t.Fatalf("expected commit promoted error, got %v", err)
	}

	inFail := ProposeDecisionInput{Title: "p", Reasoning: "r", EvidenceSummary: "e", CheckType: "file_exists", CheckSpec: `{"path":"missing"}`, ModuleRoot: root}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnError(errors.New("pending evidence fail"))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), inFail)
	if err == nil || !strings.Contains(err.Error(), "insert proposal evidence") {
		t.Fatalf("expected pending evidence error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(4, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE proposals").WillReturnError(errors.New("pending update fail"))
	mock.ExpectRollback()
	_, err = svc.ProposeAndVerifyDecision(context.Background(), inFail)
	if err == nil || !strings.Contains(err.Error(), "update proposal status to pending") {
		t.Fatalf("expected pending update error, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(5, 1))
	mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE proposals").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit pending fail"))
	_, err = svc.ProposeAndVerifyDecision(context.Background(), inFail)
	if err == nil || !strings.Contains(err.Error(), "commit pending proposal tx") {
		t.Fatalf("expected pending commit error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestListDecisionsScanAndIterateErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	svc := NewService(db)

	// query error
	mock.ExpectQuery("SELECT d.id").WillReturnError(errors.New("query fail"))
	_, err = svc.ListDecisions(context.Background())
	if err == nil || !strings.Contains(err.Error(), "query decisions") {
		t.Fatalf("expected query decisions error, got %v", err)
	}

	// scan error
	mock.ExpectQuery("SELECT d.id").WillReturnRows(
		sqlmock.NewRows([]string{"id", "title", "confidence", "status", "drift", "updated_at"}).
			AddRow("bad-id", "t", "h", "active", "ok", "2024-01-01"),
	)
	_, err = svc.ListDecisions(context.Background())
	if err == nil || !strings.Contains(err.Error(), "scan decision") {
		t.Fatalf("expected scan decision error, got %v", err)
	}

	// rows.Err
	mock.ExpectQuery("SELECT d.id").WillReturnRows(
		sqlmock.NewRows([]string{"id", "title", "confidence", "status", "drift", "updated_at"}).
			AddRow(1, "t", "h", "active", "ok", "2024-01-01").
			RowError(0, errors.New("iter fail")),
	)
	_, err = svc.ListDecisions(context.Background())
	if err == nil || !strings.Contains(err.Error(), "iter fail") {
		t.Fatalf("expected iterate error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
