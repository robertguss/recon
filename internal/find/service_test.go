package find

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

func TestFindWithFilters(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	res, err := NewService(conn).Find(context.Background(), "Ambig", QueryOptions{Kind: "method"})
	if err != nil {
		t.Fatalf("Find with kind filter error: %v", err)
	}
	if res.Symbol.Kind != "method" {
		t.Fatalf("expected method kind, got %+v", res.Symbol)
	}

	res, err = NewService(conn).Find(context.Background(), "Ambig", QueryOptions{FilePath: "./other.go"})
	if err != nil {
		t.Fatalf("Find with file filter error: %v", err)
	}
	if res.Symbol.FilePath != "other.go" {
		t.Fatalf("expected other.go, got %+v", res.Symbol)
	}

	_, err = NewService(conn).Find(context.Background(), "Ambig", QueryOptions{PackagePath: "missing"})
	nf, ok := err.(NotFoundError)
	if !ok {
		t.Fatalf("expected filtered NotFoundError, got %T (%v)", err, err)
	}
	if !nf.Filtered || len(nf.Suggestions) != 0 {
		t.Fatalf("expected filtered not-found with empty suggestions, got %+v", nf)
	}
	if got := nf.Error(); !strings.Contains(got, "provided filters") {
		t.Fatalf("expected filtered not-found message, got %q", got)
	}
}

func TestQueryOptionHelpers(t *testing.T) {
	opts := normalizeQueryOptions(QueryOptions{PackagePath: " . ", FilePath: "./pkg/../file.go", Kind: " METHOD "})
	if opts.PackagePath != "." {
		t.Fatalf("expected package path '.', got %q", opts.PackagePath)
	}
	if opts.FilePath != "file.go" {
		t.Fatalf("expected normalized file path, got %q", opts.FilePath)
	}
	if opts.Kind != "method" {
		t.Fatalf("expected lowercase kind, got %q", opts.Kind)
	}

	if !hasActiveFilters(opts) {
		t.Fatal("expected active filters")
	}
	if hasActiveFilters(QueryOptions{}) {
		t.Fatal("did not expect active filters")
	}

	if got := normalizeFilePath("  "); got != "" {
		t.Fatalf("expected empty normalized path, got %q", got)
	}
	if got := normalizeFilePath("./a/../b.go"); got != "b.go" {
		t.Fatalf("expected clean path b.go, got %q", got)
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

func TestFindFileFilterSuffixMatch(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	// "Ambig" in file "other.go" (file_id=2) should match --file "other.go"
	res, err := NewService(conn).Find(context.Background(), "Ambig", QueryOptions{FilePath: "other.go"})
	if err != nil {
		t.Fatalf("Find with suffix file filter error: %v", err)
	}
	if res.Symbol.FilePath != "other.go" {
		t.Fatalf("expected file other.go, got %s", res.Symbol.FilePath)
	}

	// Full path should still work
	res, err = NewService(conn).Find(context.Background(), "Target", QueryOptions{FilePath: "main.go"})
	if err != nil {
		t.Fatalf("Find with exact file filter error: %v", err)
	}
	if res.Symbol.Name != "Target" {
		t.Fatalf("expected Target, got %s", res.Symbol.Name)
	}

	// Path with slash should do substring match
	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (2,'pkg/sub','sub','example.com/recon/pkg/sub',1,5,'x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (3,2,'pkg/sub/service.go','go',5,'h3','x','x');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (5,3,'func','UniqueInSub','func()','func UniqueInSub(){}',1,1,1,'');`)

	res, err = NewService(conn).Find(context.Background(), "UniqueInSub", QueryOptions{FilePath: "pkg/sub/service.go"})
	if err != nil {
		t.Fatalf("Find with path-containing file filter error: %v", err)
	}
	if res.Symbol.Name != "UniqueInSub" {
		t.Fatalf("expected UniqueInSub, got %s", res.Symbol.Name)
	}

	// Filename-only should match a file stored with directory prefix
	res, err = NewService(conn).Find(context.Background(), "UniqueInSub", QueryOptions{FilePath: "service.go"})
	if err != nil {
		t.Fatalf("Find with filename-only suffix filter error: %v", err)
	}
	if res.Symbol.Name != "UniqueInSub" {
		t.Fatalf("expected UniqueInSub via suffix match, got %s", res.Symbol.Name)
	}
}

func TestListByPackage(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{PackagePath: "."}, 50)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if result.Total < 3 {
		t.Fatalf("expected at least 3 symbols in root package, got %d", result.Total)
	}
	if result.Limit != 50 {
		t.Fatalf("expected limit 50, got %d", result.Limit)
	}
	// Symbols should not have bodies in list mode
	for _, s := range result.Symbols {
		if s.Body != "" {
			t.Fatalf("expected empty body in list mode, got body for %s", s.Name)
		}
	}
}

func TestListByKind(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{Kind: "method"}, 50)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 method, got %d", result.Total)
	}
	if result.Symbols[0].Kind != "method" {
		t.Fatalf("expected method kind, got %s", result.Symbols[0].Kind)
	}
}

func TestListByFile(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{FilePath: "main.go"}, 50)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if result.Total < 2 {
		t.Fatalf("expected at least 2 symbols in main.go, got %d", result.Total)
	}
}

func TestListNoFiltersReturnsError(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	_, err := NewService(conn).List(context.Background(), QueryOptions{}, 50)
	if err == nil {
		t.Fatal("expected error for list with no filters")
	}
}

func TestListRespectsLimit(t *testing.T) {
	conn, cleanup := findTestDB(t)
	defer cleanup()

	result, err := NewService(conn).List(context.Background(), QueryOptions{PackagePath: "."}, 2)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(result.Symbols) > 2 {
		t.Fatalf("expected at most 2 symbols, got %d", len(result.Symbols))
	}
	if result.Total < 3 {
		t.Fatalf("expected total >= 3, got %d", result.Total)
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
