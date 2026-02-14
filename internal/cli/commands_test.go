package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/recon/internal/index"
	"github.com/robertguss/recon/internal/orient"
	"github.com/spf13/cobra"
)

func runCommandWithCapture(t *testing.T, cmd interface {
	SetArgs([]string)
	ExecuteContext(context.Context) error
}, args []string) (string, string, error) {
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
	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{"Pending", "--reasoning", "because", "--evidence-summary", "missing", "--check-type", "file_exists", "--check-spec", `{"path":"missing"}`, "--json"})
	if err == nil || !strings.Contains(out, `"code": "verification_failed"`) {
		t.Fatalf("decide pending json failed out=%q err=%v", out, err)
	}
	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{"Pending text", "--reasoning", "because", "--evidence-summary", "missing", "--check-type", "file_exists", "--check-spec", `{"path":"missing"}`})
	if err == nil || !strings.Contains(out, "Decision pending") {
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

func TestInitCommandErrorBranches(t *testing.T) {
	root := setupModuleRoot(t)
	origRunMigrations := runMigrations
	defer func() { runMigrations = origRunMigrations }()

	// Missing go.mod error.
	noModRoot := t.TempDir()
	if _, _, err := runCommandWithCapture(t, newInitCommand(&App{Context: context.Background(), ModuleRoot: noModRoot}), nil); err == nil || !strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("expected missing go.mod error, got %v", err)
	}

	// EnsureReconDir error: .recon exists as file.
	rootEnsureDir := setupModuleRoot(t)
	if err := os.WriteFile(filepath.Join(rootEnsureDir, ".recon"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write .recon file: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newInitCommand(&App{Context: context.Background(), ModuleRoot: rootEnsureDir}), nil); err == nil {
		t.Fatal("expected EnsureReconDir error")
	}

	// go.mod stat error on invalid module root.
	fileRoot := filepath.Join(t.TempDir(), "as-file")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fileRoot: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newInitCommand(&App{Context: context.Background(), ModuleRoot: fileRoot}), nil); err == nil || !strings.Contains(err.Error(), "stat go.mod") {
		t.Fatalf("expected stat go.mod error, got %v", err)
	}

	// db.Open error: .recon/recon.db exists as directory.
	if err := os.MkdirAll(filepath.Join(root, ".recon", "recon.db"), 0o755); err != nil {
		t.Fatalf("mkdir fake db path: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newInitCommand(&App{Context: context.Background(), ModuleRoot: root}), nil); err == nil {
		t.Fatal("expected db open error")
	}

	// EnsureGitIgnore error: .gitignore is a directory.
	root2 := setupModuleRoot(t)
	if err := os.MkdirAll(filepath.Join(root2, ".gitignore"), 0o755); err != nil {
		t.Fatalf("mkdir .gitignore dir: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newInitCommand(&App{Context: context.Background(), ModuleRoot: root2}), nil); err == nil {
		t.Fatal("expected EnsureGitIgnore error")
	}

	// RunMigrations error.
	root3 := setupModuleRoot(t)
	runMigrations = func(*sql.DB) error { return errors.New("migrate fail") }
	if _, _, err := runCommandWithCapture(t, newInitCommand(&App{Context: context.Background(), ModuleRoot: root3}), nil); err == nil {
		t.Fatal("expected RunMigrations error")
	}
}

func TestCommandErrorBranches(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	origRunSync := runSync
	origBuildOrient := buildOrient
	origRunOrientSync := runOrientSync
	defer func() {
		runSync = origRunSync
		buildOrient = origBuildOrient
		runOrientSync = origRunOrientSync
	}()

	// Decide openExistingDB error.
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{"x", "--reasoning", "r", "--evidence-summary", "e", "--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`}); err == nil {
		t.Fatal("expected decide openExistingDB error")
	}

	// Recall openExistingDB error.
	if _, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{"q"}); err == nil {
		t.Fatal("expected recall openExistingDB error")
	}

	// Find openExistingDB error.
	if _, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"X"}); err == nil {
		t.Fatal("expected find openExistingDB error")
	}

	// Orient openExistingDB error.
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), nil); err == nil {
		t.Fatal("expected orient openExistingDB error")
	}

	// Sync openExistingDB error.
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err == nil {
		t.Fatal("expected sync openExistingDB error")
	}

	// Init and sync once.
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Sync command service error and commit print branch.
	runSync = func(context.Context, *sql.DB, string) (index.SyncResult, error) {
		return index.SyncResult{}, errors.New("sync fail")
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err == nil {
		t.Fatal("expected sync service error branch")
	}
	runSync = func(context.Context, *sql.DB, string) (index.SyncResult, error) {
		return index.SyncResult{IndexedFiles: 1, IndexedSymbols: 2, IndexedPackages: 1, Fingerprint: "f", Commit: "abc", Dirty: true, SyncedAt: time.Now()}, nil
	}
	out, _, err := runCommandWithCapture(t, newSyncCommand(app), nil)
	if err != nil || !strings.Contains(out, "Git commit: abc") {
		t.Fatalf("expected commit print branch, out=%q err=%v", out, err)
	}
	runSync = origRunSync

	// find default error branch (non typed error) via schema break.
	conn, err := openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB: %v", err)
	}
	if _, err := conn.Exec(`DROP TABLE symbols;`); err != nil {
		_ = conn.Close()
		t.Fatalf("drop symbols: %v", err)
	}
	_ = conn.Close()
	if _, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--json"}); err == nil {
		t.Fatal("expected default find error branch")
	}

	// recall service error branch via schema break.
	conn, err = openExistingDB(app)
	if err != nil {
		t.Fatalf("openExistingDB second: %v", err)
	}
	if _, err := conn.Exec(`DROP TABLE decisions;`); err != nil {
		_ = conn.Close()
		t.Fatalf("drop decisions: %v", err)
	}
	_ = conn.Close()
	if _, _, err := runCommandWithCapture(t, newRecallCommand(app), []string{"q"}); err == nil {
		t.Fatal("expected recall error branch")
	}

	// decide service error branch on unmigrated DB file.
	root3 := setupModuleRoot(t)
	if err := os.MkdirAll(filepath.Join(root3, ".recon"), 0o755); err != nil {
		t.Fatalf("mkdir .recon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root3, ".recon", "recon.db"), []byte{}, 0o644); err != nil {
		t.Fatalf("seed db file: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newDecideCommand(&App{Context: context.Background(), ModuleRoot: root3}), []string{"x", "--reasoning", "r", "--evidence-summary", "e", "--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`}); err == nil {
		t.Fatal("expected decide service error")
	}

	// recall text branch with items.
	root4 := setupModuleRoot(t)
	app4 := &App{Context: context.Background(), ModuleRoot: root4}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app4), nil); err != nil {
		t.Fatalf("init root4: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app4), nil); err != nil {
		t.Fatalf("sync root4: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newDecideCommand(app4), []string{"Use X", "--reasoning", "r", "--evidence-summary", "go.mod exists", "--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`}); err != nil {
		t.Fatalf("decide root4: %v", err)
	}
	out, _, err = runCommandWithCapture(t, newRecallCommand(app4), []string{"Use"})
	if err != nil || !strings.Contains(out, "- #") {
		t.Fatalf("expected recall item text output, out=%q err=%v", out, err)
	}

	// decide promoted text branch.
	out, _, err = runCommandWithCapture(t, newDecideCommand(app4), []string{"Use Y", "--reasoning", "r", "--evidence-summary", "go.mod exists", "--check-type", "file_exists", "--check-spec", `{"path":"go.mod"}`})
	if err != nil || !strings.Contains(out, "Decision promoted") {
		t.Fatalf("expected decide promoted text, out=%q err=%v", out, err)
	}

	// Orient command explicit build/sync error branches.
	call := 0
	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		call++
		if call == 1 {
			return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
		}
		return orient.Payload{}, errors.New("build fail second")
	}
	runOrientSync = func(context.Context, *sql.DB, string) error { return nil }
	isInteractive = func() bool { return true }
	askYesNo = func(string, bool) (bool, error) { return true, nil }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app4), nil); err == nil {
		t.Fatal("expected second build error branch")
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error { return errors.New("sync fail") }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app4), nil); err == nil {
		t.Fatal("expected orient sync error branch")
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{}, errors.New("build fail first")
	}
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app4), nil); err == nil {
		t.Fatal("expected orient initial build error branch")
	}

	// Find receiver/dependency text branches.
	root5 := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root5, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write("go.mod", "module example.com/recon\n")
	write("main.go", "package main\ntype R struct{}\nfunc (r R) Solo() { Dep() }\nfunc Dep() {}\n")
	app5 := &App{Context: context.Background(), ModuleRoot: root5}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app5), nil); err != nil {
		t.Fatalf("init root5: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app5), nil); err != nil {
		t.Fatalf("sync root5: %v", err)
	}
	out, _, err = runCommandWithCapture(t, newFindCommand(app5), []string{"Solo"})
	if err != nil || !strings.Contains(out, "Receiver: R") || !strings.Contains(out, "- func Dep") {
		t.Fatalf("expected receiver and dependency lines, out=%q err=%v", out, err)
	}
}

func TestFindCommandTextErrorOutput(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Ambig"})
	if err == nil || !strings.Contains(out, "ambiguous") || !strings.Contains(out, "pkg1/a.go") {
		t.Fatalf("expected ambiguous text output with candidates, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Al"})
	if err == nil || !strings.Contains(out, "not found") || !strings.Contains(out, "Suggestions:") {
		t.Fatalf("expected not-found text output with suggestions, out=%q err=%v", out, err)
	}
}

func TestFindCommandTextAmbiguousReceiverOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainGo := `package main
type A struct{}
type B struct{}
func (A) Clash() {}
func (B) Clash() {}
`
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Clash"})
	if err == nil || !strings.Contains(out, "A.Clash") || !strings.Contains(out, "B.Clash") {
		t.Fatalf("expected receiver-qualified candidates, out=%q err=%v", out, err)
	}
}

func TestOrientCommandMachineFlags(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	origBuildOrient := buildOrient
	origRunOrientSync := runOrientSync
	origInteractive := isInteractive
	origAsk := askYesNo
	defer func() {
		buildOrient = origBuildOrient
		runOrientSync = origRunOrientSync
		isInteractive = origInteractive
		askYesNo = origAsk
	}()

	isInteractive = func() bool { return false }
	askYesNo = func(string, bool) (bool, error) { return false, nil }

	buildCalls := 0
	syncCalls := 0
	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		buildCalls++
		return orient.Payload{}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error {
		syncCalls++
		return nil
	}
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--sync", "--json"}); err != nil {
		t.Fatalf("orient --sync failed: %v", err)
	}
	if syncCalls != 1 || buildCalls != 1 {
		t.Fatalf("expected sync-before-build once, syncCalls=%d buildCalls=%d", syncCalls, buildCalls)
	}

	buildCalls = 0
	syncCalls = 0
	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		buildCalls++
		if buildCalls == 1 {
			return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
		}
		return orient.Payload{}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error {
		syncCalls++
		return nil
	}
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--auto-sync", "--json"}); err != nil {
		t.Fatalf("orient --auto-sync failed: %v", err)
	}
	if syncCalls != 1 || buildCalls != 2 {
		t.Fatalf("expected one auto-sync and rebuild, syncCalls=%d buildCalls=%d", syncCalls, buildCalls)
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error { return errors.New("sync now fail") }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--sync", "--json"}); err == nil {
		t.Fatal("expected orient --sync error")
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error { return errors.New("auto sync fail") }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--auto-sync", "--json"}); err == nil {
		t.Fatal("expected orient --auto-sync sync error")
	}

	buildCalls = 0
	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		buildCalls++
		if buildCalls == 1 {
			return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
		}
		return orient.Payload{}, errors.New("build after auto-sync failed")
	}
	runOrientSync = func(context.Context, *sql.DB, string) error { return nil }
	if _, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--auto-sync", "--json"}); err == nil {
		t.Fatal("expected orient --auto-sync rebuild error")
	}

	buildOrient = func(context.Context, *sql.DB, string) (orient.Payload, error) {
		return orient.Payload{Freshness: orient.Freshness{IsStale: true, Reason: "stale"}}, nil
	}
	runOrientSync = func(context.Context, *sql.DB, string) error { return nil }
	out, stderr, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--json-strict"})
	if err != nil {
		t.Fatalf("orient --json-strict failed: %v", err)
	}
	if stderr != "" || !strings.Contains(out, "\"freshness\"") {
		t.Fatalf("expected strict json-only output, out=%q stderr=%q", out, stderr)
	}
}

func TestOrientJSONEmptyLists(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}

	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newOrientCommand(app), []string{"--json"})
	if err != nil {
		t.Fatalf("orient --json: %v", err)
	}
	if !strings.Contains(out, `"modules": []`) {
		t.Fatalf("expected modules empty array, out=%q", out)
	}
	if !strings.Contains(out, `"active_decisions": []`) {
		t.Fatalf("expected active_decisions empty array, out=%q", out)
	}
}

func TestDecideTypedCheckFlags(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newDecideCommand(app), []string{
		"typed file", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod", "--json",
	})
	if err != nil || !strings.Contains(out, `"promoted": true`) {
		t.Fatalf("expected typed file check success, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"typed symbol", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "symbol_exists", "--check-symbol", "Alpha", "--json",
	})
	if err != nil || !strings.Contains(out, `"promoted": true`) {
		t.Fatalf("expected typed symbol check success, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"typed pattern", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "grep_pattern", "--check-pattern", "package", "--json",
	})
	if err != nil || !strings.Contains(out, `"promoted": true`) {
		t.Fatalf("expected typed pattern check success, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"typed conflict", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-path", "go.mod", "--check-spec", `{"path":"go.mod"}`, "--json",
	})
	if err == nil || !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected typed/raw conflict error envelope, out=%q err=%v", out, err)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	root := setupModuleRoot(t)
	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Ambig", "--json"})
	if err == nil || !strings.Contains(out, `"code": "ambiguous"`) {
		t.Fatalf("expected ambiguous envelope, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Al", "--json"})
	if err == nil || !strings.Contains(out, `"code": "not_found"`) {
		t.Fatalf("expected not_found envelope, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"verification failed", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "file_exists", "--check-spec", `{"path":"missing"}`, "--json",
	})
	if err == nil || !strings.Contains(out, `"code": "verification_failed"`) {
		t.Fatalf("expected verification_failed envelope, out=%q err=%v", out, err)
	}

	out, _, err = runCommandWithCapture(t, newDecideCommand(app), []string{
		"invalid check type", "--reasoning", "r", "--evidence-summary", "e",
		"--check-type", "nope", "--check-spec", `{}`, "--json",
	})
	if err == nil || !strings.Contains(out, `"code": "invalid_input"`) {
		t.Fatalf("expected invalid_input envelope, out=%q err=%v", out, err)
	}
}

func TestFindBodyFlags(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	write("go.mod", "module example.com/recon\n")
	write("main.go", `package main

func Alpha() {
	lineOne()
	lineTwo()
	lineThree()
}

func lineOne() {}
func lineTwo() {}
func lineThree() {}
`)

	app := &App{Context: context.Background(), ModuleRoot: root}
	if _, _, err := runCommandWithCapture(t, newInitCommand(app), nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newSyncCommand(app), nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, _, err := runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--no-body"})
	if err != nil {
		t.Fatalf("find --no-body: %v", err)
	}
	if strings.Contains(out, "\nBody:\n") {
		t.Fatalf("expected body omitted, out=%q", out)
	}

	out, _, err = runCommandWithCapture(t, newFindCommand(app), []string{"Alpha", "--max-body-lines", "2"})
	if err != nil {
		t.Fatalf("find --max-body-lines: %v", err)
	}
	if !strings.Contains(out, "\nBody:\n") || !strings.Contains(out, "... (truncated)") {
		t.Fatalf("expected truncated body marker, out=%q", out)
	}
}

func TestNoPromptDisablesOrientPrompt(t *testing.T) {
	root := setupModuleRoot(t)

	origGetwd := osGetwd
	origFind := findModuleRoot
	origInteractive := isInteractive
	origAsk := askYesNo
	defer func() {
		osGetwd = origGetwd
		findModuleRoot = origFind
		isInteractive = origInteractive
		askYesNo = origAsk
	}()

	osGetwd = func() (string, error) { return root, nil }
	findModuleRoot = func(string) (string, error) { return root, nil }

	newRoot := func(t *testing.T) *cobra.Command {
		t.Helper()
		cmd, err := NewRootCommand(context.Background())
		if err != nil {
			t.Fatalf("new root: %v", err)
		}
		return cmd
	}

	if _, _, err := runCommandWithCapture(t, newRoot(t), []string{"init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runCommandWithCapture(t, newRoot(t), []string{"sync"}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Alpha(){ }\n"), 0o644); err != nil {
		t.Fatalf("touch main.go: %v", err)
	}

	promptCalls := 0
	isInteractive = func() bool { return true }
	askYesNo = func(string, bool) (bool, error) {
		promptCalls++
		return true, nil
	}

	if _, _, err := runCommandWithCapture(t, newRoot(t), []string{"--no-prompt", "orient"}); err != nil {
		t.Fatalf("orient --no-prompt: %v", err)
	}
	if promptCalls != 0 {
		t.Fatalf("expected no prompt calls, got %d", promptCalls)
	}
}
