package index

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCurrentGitState(t *testing.T) {
	ctx := context.Background()

	nonRepo := t.TempDir()
	commit, dirty := CurrentGitState(ctx, nonRepo)
	if commit != "" || dirty {
		t.Fatalf("expected empty git state for non-repo, got commit=%q dirty=%v", commit, dirty)
	}

	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	commit, dirty = CurrentGitState(ctx, repo)
	if commit == "" || dirty {
		t.Fatalf("expected clean git state after commit, got commit=%q dirty=%v", commit, dirty)
	}

	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/y\n"), 0o644); err != nil {
		t.Fatalf("modify go.mod: %v", err)
	}
	_, dirty = CurrentGitState(ctx, repo)
	if !dirty {
		t.Fatal("expected dirty repo")
	}
}
