package find

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func findTestDB(t *testing.T) (*sql.DB, func()) {
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

	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (1,'.','main','example.com/recon',1,10,'x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (1,1,'main.go','go',10,'h','x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (2,1,'other.go','go',10,'h2','x','x');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (1,1,'func','Target','func()','func Target(){}',1,1,1,'');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (2,1,'func','Dep','func()','func Dep(){}',2,2,1,'');`)
	_, _ = conn.Exec(`INSERT INTO symbol_deps(symbol_id,dep_name) VALUES (1,'Dep');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (3,2,'func','Ambig','func()','func Ambig(){}',1,1,1,'');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (4,1,'method','Ambig','func()','func (t T) Ambig(){}',1,1,1,'T');`)

	return conn, func() { _ = conn.Close() }
}

func TestFindExactSuccess(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	res, err := NewService(conn).FindExact(context.Background(), "Target")
	if err != nil {
		t.Fatalf("FindExact success error: %v", err)
	}
	if res.Symbol.Name != "Target" || len(res.Dependencies) != 1 || res.Dependencies[0].Name != "Dep" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestFindExactAmbiguousAndNotFound(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	_, err := NewService(conn).FindExact(context.Background(), "Ambig")
	if _, ok := err.(AmbiguousError); !ok {
		t.Fatalf("expected AmbiguousError, got %T (%v)", err, err)
	}

	_, err = NewService(conn).FindExact(context.Background(), "Tar")
	nf, ok := err.(NotFoundError)
	if !ok {
		t.Fatalf("expected NotFoundError, got %T (%v)", err, err)
	}
	if len(nf.Suggestions) == 0 {
		t.Fatalf("expected suggestions in not found error: %+v", nf)
	}
}

func TestFindExactQueryError(t *testing.T) {
	root := t.TempDir()
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := NewService(conn).FindExact(context.Background(), "X"); err == nil {
		t.Fatal("expected query error for closed DB")
	}
}

func TestErrorStrings(t *testing.T) {
	nf := NotFoundError{Symbol: "x", Suggestions: nil}
	if nf.Error() == "" {
		t.Fatal("expected non-empty NotFoundError string")
	}
	nf2 := NotFoundError{Symbol: "x", Suggestions: []string{"y"}}
	if nf2.Error() == "" {
		t.Fatal("expected non-empty NotFoundError string with suggestions")
	}
	ae := AmbiguousError{Symbol: "x", Candidates: []Candidate{{FilePath: filepath.Base("a.go")}}}
	if ae.Error() == "" {
		t.Fatal("expected non-empty AmbiguousError string")
	}
	_ = os.ErrNotExist
}
