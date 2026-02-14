package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCommandWithCapture(t *testing.T, cmd interface{ SetArgs([]string); ExecuteContext(context.Context) error }, args []string) (string, string, error) {
	t.Helper()
	origOut := os.Stdout
	origErr := os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	cmd.SetArgs(args)
	execErr := cmd.ExecuteContext(context.Background())

	_ = wOut.Close()
	_ = wErr.Close()
	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	_ = rOut.Close()
	_ = rErr.Close()
	os.Stdout = origOut
	os.Stderr = origErr

	return string(outBytes), string(errBytes), execErr
}

func setupModuleRoot(t *testing.T) string {
	t.Helper()
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
import "example.com/recon/pkg1"
func Alpha() { pkg1.Ambig() }
func main() {}
`)
	mustWrite("pkg1/a.go", `package pkg1
func Ambig() {}
`)
	mustWrite("pkg2/a.go", `package pkg2
func Ambig() {}
`)
	return root
}

func TestOpenExistingDB(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	if _, err := openExistingDB(app); err == nil || !strings.Contains(err.Error(), "run `recon init` first") {
		t.Fatalf("expected missing db error, got %v", err)
	}

	fileRoot := filepath.Join(t.TempDir(), "rootfile")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	app2 := &App{Context: context.Background(), ModuleRoot: fileRoot}
	if _, err := openExistingDB(app2); err == nil || !strings.Contains(err.Error(), "stat db file") {
		t.Fatalf("expected stat db file error, got %v", err)
	}

	cmd := newInitCommand(app)
	if _, _, err := runCommandWithCapture(t, cmd, []string{"--json"}); err != nil {
		t.Fatalf("init command: %v", err)
	}
	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB success error: %v", err)
	}
	_ = conn.Close()
}

func TestCommandsEndToEndAndBranches(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err == nil {
		t.Fatal("expected sync error before init")
	}

	out, _, err := runCommandWithCapture(t, newInitCommand(app), []string{"--json"})
	if err != nil || !strings.Contains(out, "\"ok\": true") {
		t.Fatalf("init --json failed out=%q err=%v", out, err)
	}
	out, _, err = runCommandWithCapture(t, newInitCommand(app), nil)
	if err != nil || !strings.Contains(out, "Initialized recon") {
		t.Fatalf("init text failed out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newSyncCommand(app), []string{"--json"})
	if err != nil || !strings.Contains(out, "indexed_files") {
		t.Fatalf("sync --json failed out=%q err=%v", out, err)
	}
	out, _, err = runCommandWithCapture(t, newSyncCommand(app), nil)
	if err != nil || !strings.Contains(out, "Synced") {
		t.Fatalf("sync text failed out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newOrientCommand(app), []string{"--json"})
	if err != nil || !strings.Contains(out, "\"project\"") {
		t.Fatalf("orient --json failed out=%q err=%v", out, err)
	}

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Alpha(){ }\n"), 0o644); err != nil {
		t.Fatalf("touch main.go for stale: %v", err)
	}

	origInteractive := isInteractive
	origAsk := askYesNo
	defer func() {
		isInteractive = origInteractive
		askYesNo = origAsk
	}()

	isInteractive = func() bool { return false }
	out, stderr, err := runCommandWithCapture(t, newOrientCommand(app), nil)
	if err != nil {
		t.Fatalf("orient non-interactive stale failed: %v", err)
	}
	if !strings.Contains(stderr, "warning: stale context") || !strings.Contains(out, "STALE CONTEXT") {
		t.Fatalf("expected stale warning/output, out=%q stderr=%q", out, stderr)
	}

	isInteractive = func() bool { return true }
	askYesNo = func(string, bool) (bool, error) { return false, nil }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), nil); err != nil {
		t.Fatalf("orient interactive no-sync failed: %v", err)
	}

	askYesNo = func(string, bool) (bool, error) { return false, errors.New("input failed") }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), nil); err == nil || !strings.Contains(err.Error(), "read stale prompt") {
		t.Fatalf("expected prompt error, got %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Alpha(){ fmt.Println() }\n"), 0o644); err != nil {
		t.Fatalf("touch main.go for stale sync branch: %v", err)
	}
	askYesNo = func(string, bool) (bool, error) { return true, nil }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), nil); err != nil {
		t.Fatalf("orient interactive sync failed: %v", err)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--json"})
	if err != nil || !strings.Contains(out, "\"symbol\"") {
		t.Fatalf("find Alpha --json failed out=%q err=%v", out, err)
	}
	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Alpha"})
	if err != nil || !strings.Contains(out, "Body:") {
		t.Fatalf("find Alpha text failed out=%q err=%v", out, err)
	}
	_, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--json"})
	if err == nil {
		t.Fatal("expected ambiguous find error")
	}
	_, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Al", "--json"})
	if err == nil {
		t.Fatal("expected not found find error")
	}

	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{"Use Cobra", "--reasoning", "because", "--evidence-summary", "go.mod exists", "--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`, "--json"})
	if err != nil || !strings.Contains(out, "\"promoted\": true") {
		t.Fatalf("decide promoted failed out=%q err=%v", out, err)
	}
	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{"Pending", "--reasoning", "because", "--evidence-summary", "missing", "--check-type", "file_exists", "--check-spec", `{"path":"missing"}`})
	if err != nil || !strings.Contains(out, "Decision pending") {
		t.Fatalf("decide pending text failed out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newRecallCommand(app), []string{"Cobra", "--json"})
	if err != nil || !strings.Contains(out, "Use Cobra") {
		t.Fatalf("recall --json failed out=%q err=%v", out, err)
	}
	out, _, err = runCommandWithCapture(t, newRecallCommand(app), []string{"nohits"})
	if err != nil || !strings.Contains(out, "No promoted knowledge found") {
		t.Fatalf("recall text nohits failed out=%q err=%v", out, err)
	}

	_, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{"bad"})
	if err == nil {
		t.Fatal("expected decide required-flag error")
	}
	_, _, err = runCommandWithCapture(t, newRecallCommand(app), []string{})
	if err == nil {
		t.Fatal("expected recall arg validation error")
	}
	_, _, err = runCommandWithCapture(t, newFindCommand(app), []string{})
	if err == nil {
		t.Fatal("expected find arg validation error")
	}

	_ = fmt.Sprintf("%v", app.Context)
}
