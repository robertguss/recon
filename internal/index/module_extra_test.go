package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModulePath_EmptyModuleDirective(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module \n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	if _, err := ModulePath(root); err == nil || !strings.Contains(err.Error(), "module path not found in go.mod") {
		t.Fatalf("expected module path not found error, got %v", err)
	}
}
