package index

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindModuleRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, err := FindModuleRoot(nested)
	if err != nil {
		t.Fatalf("FindModuleRoot success case error: %v", err)
	}
	if got != root {
		t.Fatalf("FindModuleRoot() = %q, want %q", got, root)
	}
}

func TestFindModuleRootErrors(t *testing.T) {
	if _, err := FindModuleRoot(t.TempDir()); err == nil || !strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("expected not found error, got %v", err)
	}

	origAbs := moduleAbsPath
	defer func() { moduleAbsPath = origAbs }()
	moduleAbsPath = func(string) (string, error) {
		return "", errors.New("abs fail")
	}
	if _, err := FindModuleRoot("anything"); err == nil || !strings.Contains(err.Error(), "resolve absolute path") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestModulePath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/acme/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	got, err := ModulePath(root)
	if err != nil {
		t.Fatalf("ModulePath() error = %v", err)
	}
	if got != "github.com/acme/recon" {
		t.Fatalf("ModulePath() = %q", got)
	}
}

func TestModulePathErrors(t *testing.T) {
	if _, err := ModulePath(t.TempDir()); err == nil || !strings.Contains(err.Error(), "open go.mod") {
		t.Fatalf("expected open error, got %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("require x/y v1.0.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if _, err := ModulePath(root); err == nil || !strings.Contains(err.Error(), "module path not found") {
		t.Fatalf("expected module missing error, got %v", err)
	}

	long := strings.Repeat("a", 2_000_000)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(long), 0o644); err != nil {
		t.Fatalf("write long go.mod: %v", err)
	}
	if _, err := ModulePath(root); err == nil || !strings.Contains(err.Error(), "read go.mod") {
		t.Fatalf("expected scanner error, got %v", err)
	}

	origScanner := moduleScanner
	defer func() { moduleScanner = origScanner }()
	moduleScanner = func(io.Reader) *bufio.Scanner {
		return bufio.NewScanner(io.Reader(errorReader{}))
	}
	if _, err := ModulePath(root); err == nil || !strings.Contains(err.Error(), "read go.mod") {
		t.Fatalf("expected injected scanner error, got %v", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("boom") }
