package index

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiverNameAndTextForPosExtraBranches(t *testing.T) {
	decl := &ast.FuncDecl{Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Elt: &ast.Ident{Name: "X"}}}}}}
	if got := receiverName(decl); got == "" {
		t.Fatal("expected receiverName fallback string for unsupported receiver type")
	}

	fset := token.NewFileSet()
	file := fset.AddFile("x.go", -1, 10)
	file.SetLines([]int{0, 5})
	if got := textForPos(fset, []byte("1234567890"), file.Pos(8), file.Pos(3)); got != "" {
		t.Fatalf("expected empty text for inverted range, got %q", got)
	}
	if got := textForPos(fset, []byte("1234567890"), file.Pos(5), file.Pos(5)); got != "" {
		t.Fatalf("expected empty text for zero-length range, got %q", got)
	}
}

func TestModulePathBlankModuleLine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module \n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if _, err := ModulePath(root); err == nil || !strings.Contains(err.Error(), "module path not found") {
		t.Fatalf("expected module path not found, got %v", err)
	}
}
