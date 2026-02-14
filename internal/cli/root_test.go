package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRootCommandAndErrors(t *testing.T) {
	origGetwd := osGetwd
	origFind := findModuleRoot
	defer func() {
		osGetwd = origGetwd
		findModuleRoot = origFind
	}()

	root := t.TempDir()
	osGetwd = func() (string, error) { return root, nil }
	findModuleRoot = func(string) (string, error) { return root, nil }

	cmd, err := NewRootCommand(context.Background())
	if err != nil {
		t.Fatalf("NewRootCommand success error: %v", err)
	}
	if cmd.Use != "recon" {
		t.Fatalf("unexpected root use: %q", cmd.Use)
	}
	if len(cmd.Commands()) != 8 {
		t.Fatalf("expected 8 subcommands, got %d", len(cmd.Commands()))
	}

	osGetwd = func() (string, error) { return "", errors.New("cwd fail") }
	if _, err := NewRootCommand(context.Background()); err == nil || !strings.Contains(err.Error(), "resolve cwd") {
		t.Fatalf("expected cwd error, got %v", err)
	}
}

func TestNewRootCommandFallbackModuleRoot(t *testing.T) {
	origGetwd := osGetwd
	origFind := findModuleRoot
	defer func() {
		osGetwd = origGetwd
		findModuleRoot = origFind
	}()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	osGetwd = func() (string, error) { return root, nil }
	findModuleRoot = func(string) (string, error) { return "", errors.New("not found") }

	cmd, err := NewRootCommand(context.Background())
	if err != nil {
		t.Fatalf("NewRootCommand fallback error: %v", err)
	}

	out, _, execErr := runCommandWithCapture(t, cmd, []string{"init", "--json"})
	if execErr != nil {
		t.Fatalf("execute root init: %v", execErr)
	}
	if !strings.Contains(out, root) {
		t.Fatalf("expected fallback cwd module_root in output, got %q", out)
	}
}
