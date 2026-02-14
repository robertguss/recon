package index

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func TestSyncHappyPath(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite("go.mod", "module example.com/recon\n")
	mustWrite("main.go", `package main
import (
  "fmt"
  "example.com/recon/sub"
)
func Call() { fmt.Println(sub.Helper()) }
`)
	mustWrite("sub/sub.go", `package sub
func Helper() string { return "ok" }
`)

	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	res, err := NewService(conn).Sync(context.Background(), root)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if res.IndexedFiles != 2 || res.IndexedPackages == 0 || res.IndexedSymbols == 0 || res.Fingerprint == "" {
		t.Fatalf("unexpected sync result: %+v", res)
	}

	var pkgCount, fileCount, symCount, depCount int
	if err := conn.QueryRow("SELECT COUNT(*) FROM packages;").Scan(&pkgCount); err != nil {
		t.Fatalf("count packages: %v", err)
	}
	if err := conn.QueryRow("SELECT COUNT(*) FROM files;").Scan(&fileCount); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if err := conn.QueryRow("SELECT COUNT(*) FROM symbols;").Scan(&symCount); err != nil {
		t.Fatalf("count symbols: %v", err)
	}
	if err := conn.QueryRow("SELECT COUNT(*) FROM symbol_deps;").Scan(&depCount); err != nil {
		t.Fatalf("count deps: %v", err)
	}
	if pkgCount < 2 || fileCount != 2 || symCount == 0 || depCount == 0 {
		t.Fatalf("unexpected counts pkg=%d files=%d syms=%d deps=%d", pkgCount, fileCount, symCount, depCount)
	}

	res2, err := NewService(conn).Sync(context.Background(), root)
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if res2.IndexedFiles != res.IndexedFiles {
		t.Fatalf("expected stable file count, got %d vs %d", res2.IndexedFiles, res.IndexedFiles)
	}
}

func TestSyncErrors(t *testing.T) {
	if _, err := NewService(nil).Sync(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected module path error with missing go.mod")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.go"), []byte("package bad\nfunc x("), 0o644); err != nil {
		t.Fatalf("write bad.go: %v", err)
	}
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if _, err := NewService(conn).Sync(context.Background(), root); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}

	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if _, err := db.EnsureReconDir(root2); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn2, err := db.Open(db.DBPath(root2))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn2.Close()
	if _, err := NewService(conn2).Sync(context.Background(), root2); err == nil || !strings.Contains(err.Error(), "reset index tables") {
		t.Fatalf("expected reset table error, got %v", err)
	}

	conn3, err := db.Open(db.DBPath(root2))
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	if err := conn3.Close(); err != nil {
		t.Fatalf("close conn3: %v", err)
	}
	if _, err := NewService(conn3).Sync(context.Background(), root2); err == nil || !strings.Contains(err.Error(), "begin sync tx") {
		t.Fatalf("expected begin tx error, got %v", err)
	}

	root3 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root3, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	origCollect := collectEligibleFiles
	defer func() { collectEligibleFiles = origCollect }()
	collectEligibleFiles = func(string) ([]SourceFile, error) { return nil, errors.New("collect fail") }
	if _, err := NewService(conn2).Sync(context.Background(), root3); err == nil || !strings.Contains(err.Error(), "collect fail") {
		t.Fatalf("expected collect files error, got %v", err)
	}
}

func TestSymbolHelpers(t *testing.T) {
	src := `package p
func F() { a(); b.C() }
type T struct{}
func (t *T) M() {}
const C = 1
var V = 2
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var all []symbolRecord
	for _, d := range file.Decls {
		all = append(all, symbolRecordsFromDecl(fset, []byte(src), d)...)
	}
	if len(all) < 5 {
		t.Fatalf("expected symbol records, got %d", len(all))
	}

	if got := collectCallNames(nil); got != nil {
		t.Fatalf("collectCallNames(nil) = %+v, want nil", got)
	}

	fnDecl := file.Decls[0].(*ast.FuncDecl)
	deps := collectCallNames(fnDecl.Body)
	if len(deps) == 0 {
		t.Fatalf("expected call deps, got %+v", deps)
	}

	if receiverName(fnDecl) != "" {
		t.Fatal("expected empty receiver for top-level func")
	}
	methodDecl := file.Decls[2].(*ast.FuncDecl)
	if receiverName(methodDecl) == "" {
		t.Fatal("expected method receiver name")
	}
	identMethodSrc := `package p
type T struct{}
func (t T) M() {}
`
	fset2 := token.NewFileSet()
	file2, err := parser.ParseFile(fset2, "y.go", identMethodSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse ident receiver source: %v", err)
	}
	if got := receiverName(file2.Decls[1].(*ast.FuncDecl)); got != "T" {
		t.Fatalf("expected ident receiver T, got %q", got)
	}
	if exprString(nil) != "" {
		t.Fatal("exprString(nil) should be empty")
	}
	if exprString(struct{}{}) != "" {
		t.Fatal("exprString(unsupported) should be empty")
	}

	if textForPos(fset, []byte(src), token.NoPos, token.NoPos) != "" {
		t.Fatal("expected empty text for invalid positions")
	}
	if text := textForPos(fset, []byte(src), fnDecl.Pos(), fnDecl.End()); !strings.Contains(text, "func F") {
		t.Fatalf("expected function body text, got %q", text)
	}
	if got := textForPos(fset, []byte(src), token.Pos(1<<30), token.Pos(1<<30+1)); got != "" {
		t.Fatalf("expected empty text for missing file mapping, got %q", got)
	}
	if boolToInt(true) != 1 || boolToInt(false) != 0 {
		t.Fatal("boolToInt unexpected values")
	}
}

func TestCollectCallDepsWithContext(t *testing.T) {
	src := `package p
import local "example.com/recon/pkg1"
import local2 "example.com/recon/pkg2"

func F(v thing) {
	Local()
	local.External()
	local2.External()
	Method()
	v.Method()
	(func() {})()
	time.Now().Format(time.RFC3339)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	var fnDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok {
			fnDecl = f
			break
		}
	}
	if fnDecl == nil {
		t.Fatal("expected function declaration")
	}
	deps := collectCallDeps(fnDecl.Body, depContext{
		PackagePath: ".",
		LocalImports: map[string]string{
			"local":  "pkg1",
			"local2": "pkg2",
			"time":   "",
		},
	})

	want := map[string]depRef{
		"Local\x00.\x00func":       {Name: "Local", PackagePath: ".", Kind: "func"},
		"External\x00pkg1\x00func": {Name: "External", PackagePath: "pkg1", Kind: "func"},
		"External\x00pkg2\x00func": {Name: "External", PackagePath: "pkg2", Kind: "func"},
		"Method\x00.\x00method":    {Name: "Method", PackagePath: ".", Kind: "method"},
		"Method\x00.\x00func":      {Name: "Method", PackagePath: ".", Kind: "func"},
	}
	if len(deps) != len(want) {
		t.Fatalf("unexpected dep count %d: %+v", len(deps), deps)
	}
	for _, dep := range deps {
		key := dep.Name + "\x00" + dep.PackagePath + "\x00" + dep.Kind
		if got, ok := want[key]; !ok || got != dep {
			t.Fatalf("unexpected dep %q => %+v", key, dep)
		}
	}
}

func TestSyncImportUnquoteFallbackAndAliasLocalImportBranches(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite("go.mod", "module example.com/recon\n")
	mustWrite("main.go", `package main
import (
  alias "example.com/recon"
)
func Use() { _ = alias.Use }
`)

	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	origUnquote := importPathUnquote
	defer func() { importPathUnquote = origUnquote }()
	importPathUnquote = func(string) (string, error) { return "", errors.New("unquote fail") }

	if _, err := NewService(conn).Sync(context.Background(), root); err != nil {
		t.Fatalf("Sync with unquote fallback error: %v", err)
	}
}
