package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func writeModuleForSync(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	return root
}

func expectResetTables(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM symbol_deps").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM imports").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM symbols").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM files").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM packages").WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestSyncSQLMockErrorBranches(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		setupMock func(sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "insert package error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnError(errors.New("pkg fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert package",
		},
		{
			name: "package id error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewErrorResult(errors.New("pkg id fail")))
				mock.ExpectRollback()
			},
			wantErr: "read package id",
		},
		{
			name: "insert file error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnError(errors.New("file fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert file",
		},
		{
			name: "file id error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewErrorResult(errors.New("file id fail")))
				mock.ExpectRollback()
			},
			wantErr: "read file id",
		},
		{
			name: "import insert error",
			src:  "package main\nimport _ \"fmt\"\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectExec("INSERT INTO imports").WillReturnError(errors.New("import fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert import",
		},
		{
			name: "symbol insert error",
			src:  "package main\nfunc A(){}\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectExec("INSERT INTO symbols").WillReturnError(errors.New("symbol fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert symbol",
		},
		{
			name: "symbol id resolve error",
			src:  "package main\nfunc A(){}\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectExec("INSERT INTO symbols").WillReturnResult(sqlmock.NewResult(3, 1))
				mock.ExpectQuery("SELECT id FROM symbols").WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectRollback()
			},
			wantErr: "resolve symbol id",
		},
		{
			name: "symbol dep insert error",
			src:  "package main\nfunc A(){B()}\nfunc B(){}\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectExec("INSERT INTO symbols").WillReturnResult(sqlmock.NewResult(3, 1))
				mock.ExpectQuery("SELECT id FROM symbols").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
				mock.ExpectExec("INSERT OR IGNORE INTO symbol_deps").WillReturnError(errors.New("dep fail"))
				mock.ExpectRollback()
			},
			wantErr: "insert symbol dep",
		},
		{
			name: "count symbols error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("count fail"))
				mock.ExpectRollback()
			},
			wantErr: "count symbols",
		},
		{
			name: "update package stats error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec("UPDATE packages").WillReturnError(errors.New("update pkg fail"))
				mock.ExpectRollback()
			},
			wantErr: "update package stats",
		},
		{
			name: "upsert sync state error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec("UPDATE packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO sync_state").WillReturnError(errors.New("sync state fail"))
				mock.ExpectRollback()
			},
			wantErr: "upsert sync state",
		},
		{
			name: "commit error",
			src:  "package main\n",
			setupMock: func(mock sqlmock.Sqlmock) {
				expectResetTables(mock)
				mock.ExpectExec("INSERT INTO packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO files").WillReturnResult(sqlmock.NewResult(2, 1))
				mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec("UPDATE packages").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO sync_state").WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit().WillReturnError(errors.New("commit fail"))
			},
			wantErr: "commit sync tx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeModuleForSync(t, tc.src)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()

			tc.setupMock(mock)
			_, err = NewService(db).Sync(context.Background(), root)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}
