package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenExistingDBOpenError(t *testing.T) {
	root := t.TempDir()
	reconDir := filepath.Join(root, ".recon")
	if err := os.MkdirAll(reconDir, 0o755); err != nil {
		t.Fatalf("mkdir recon dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(reconDir, "recon.db"), 0o755); err != nil {
		t.Fatalf("mkdir fake db dir: %v", err)
	}
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, err := openExistingDB(app); err == nil || !(strings.Contains(err.Error(), "open sqlite db") || strings.Contains(err.Error(), "enable foreign keys")) {
		t.Fatalf("expected open error, got %v", err)
	}
}
