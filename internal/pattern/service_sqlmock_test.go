package pattern

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// mockRoot creates a temp dir with a go.mod file so file_exists checks work.
func mockRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return root
}

func TestProposePatternSQLMockPromotedErrors(t *testing.T) {
	root := mockRoot(t) // go.mod exists -> file_exists passes -> promoted path

	cases := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "insert proposal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnError(errors.New("proposal fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert proposal",
		},
		{
			name: "insert pattern error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO patterns").WillReturnError(errors.New("pattern fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert pattern",
		},
		{
			name: "insert pattern evidence error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO patterns").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO evidence").WillReturnError(errors.New("evidence fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert pattern evidence",
		},
		{
			name: "update proposal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO patterns").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("UPDATE proposals").WillReturnError(errors.New("update fail"))
				mock.ExpectRollback()
			},
			wantErr: "update proposal",
		},
		{
			name: "insert search index error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO patterns").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("UPDATE proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO search_index").WillReturnError(errors.New("search fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert search index",
		},
		{
			name: "commit pattern tx error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO patterns").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("UPDATE proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO search_index").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit().WillReturnError(errors.New("commit fail"))
			},
			wantErr: "commit pattern tx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer mockDB.Close()

			tc.setupMock(mock)

			_, err = NewService(mockDB).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
				Title:           "t",
				Description:     "d",
				Confidence:      "high",
				EvidenceSummary: "e",
				CheckType:       "file_exists",
				CheckSpec:       `{"path":"go.mod"}`,
				ModuleRoot:      root,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestProposePatternSQLMockNotPromotedErrors(t *testing.T) {
	root := t.TempDir() // no go.mod -> file_exists fails -> not-promoted path

	cases := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "insert proposal evidence error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO evidence").WillReturnError(errors.New("evidence fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert proposal evidence",
		},
		{
			name: "commit pending pattern tx error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO proposals").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO evidence").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit().WillReturnError(errors.New("commit fail"))
			},
			wantErr: "commit pending pattern tx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer mockDB.Close()

			tc.setupMock(mock)

			_, err = NewService(mockDB).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
				Title:           "t",
				Description:     "d",
				Confidence:      "medium",
				EvidenceSummary: "e",
				CheckType:       "file_exists",
				CheckSpec:       `{"path":"nonexistent.go"}`,
				ModuleRoot:      root,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestProposePatternMarshalProposalDataError(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) {
		return nil, errors.New("marshal fail")
	}
	t.Cleanup(func() { jsonMarshal = orig })

	_, err = NewService(mockDB).ProposeAndVerifyPattern(context.Background(), ProposePatternInput{
		Title:           "t",
		Description:     "d",
		Confidence:      "medium",
		EvidenceSummary: "e",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"go.mod"}`,
		ModuleRoot:      t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "marshal proposal data") {
		t.Fatalf("expected marshal proposal data error, got %v", err)
	}
}
